package esprima

import (
	"strings"
)

// The original validates regular expression literals by handing them to the
// JavaScript engine it runs on (V8 in Node.js 22): a literal whose pattern
// the engine rejects is an error, and a literal whose flags or pattern the
// engine rejects in the flags' mode gets a null value. validRegExp
// reproduces the engine's syntax rules, including the Annex B extensions
// that apply without the u and v flags.

const reEnd = -1

func setOf(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

// validFlags reports whether the engine accepts the flags: each of
// d g i m s u v y at most once, and not both u and v.
func validFlags(flags string) (unicode, sets, ok bool) {
	seen := map[rune]bool{}
	for _, c := range flags {
		if !strings.ContainsRune("dgimsuvy", c) || seen[c] {
			return false, false, false
		}
		seen[c] = true
	}
	if seen['u'] && seen['v'] {
		return false, false, false
	}
	return seen['u'] || seen['v'], seen['v'], true
}

// validRegExp reports whether new RegExp(pattern, flags) succeeds.
func validRegExp(pattern []uint16, flags string) bool {
	unicode, sets, ok := validFlags(flags)
	if !ok {
		return false
	}
	p := &reParser{src: pattern, unicode: unicode, sets: sets, names: map[string]bool{}}
	return p.parse()
}

// canonicalFlags orders flags the way RegExp.prototype.flags does.
func canonicalFlags(flags string) string {
	var b strings.Builder
	for _, c := range "dgimsuvy" {
		if strings.ContainsRune(flags, c) {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// escapeRegExpSource is the engine's source escaping for RegExp.prototype.
// source: an unescaped slash outside a class and the line terminators are
// escaped, and an empty pattern becomes (?:).
func escapeRegExpSource(pattern string) string {
	src := Units(pattern)
	if len(src) == 0 {
		return "(?:)"
	}
	var out []uint16
	inClass := false
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case c == '\\':
			if i+1 < len(src) && isLineTerminator(int(src[i+1])) {
				// This '\' is dropped: the next character is escaped itself.
				continue
			}
			out = append(out, c)
			i++
			if i == len(src) {
				return unitsString(out)
			}
			out = append(out, src[i])
			continue
		case c == '/' && !inClass:
			out = append(out, '\\')
		case c == '[':
			inClass = true
		case c == ']':
			inClass = false
		case c == '\n':
			out = append(out, '\\', 'n')
			continue
		case c == '\r':
			out = append(out, '\\', 'r')
			continue
		case c == 0x2028:
			out = append(out, Units(` `)...)
			continue
		case c == 0x2029:
			out = append(out, Units(` `)...)
			continue
		}
		out = append(out, c)
	}
	return unitsString(out)
}

type groupKind int

const (
	groupCapture groupKind = iota
	groupPlain
	groupLookahead
	groupLookbehind
)

// reParser is a syntax checker that follows V8's regular expression parser.
type reParser struct {
	src          []uint16
	pos          int
	unicode      bool // u or v
	sets         bool // v
	forceUnicode bool // while reading a group name

	capturesStarted int
	captureCount    int
	scanned         bool
	hasNamed        bool
	names           map[string]bool
	namedRefs       []string
	failed          bool
}

func (p *reParser) uni() bool { return p.unicode || p.forceUnicode }

// at reads the character at i: a code unit, or a code point made of a
// surrogate pair in unicode mode. It returns reEnd past the end.
func (p *reParser) at(i int) (int, int) {
	if i >= len(p.src) {
		return reEnd, 0
	}
	c := int(p.src[i])
	if p.uni() && c >= 0xD800 && c <= 0xDBFF && i+1 < len(p.src) {
		d := int(p.src[i+1])
		if d >= 0xDC00 && d <= 0xDFFF {
			return (c-0xD800)<<10 + (d - 0xDC00) + 0x10000, 2
		}
	}
	return c, 1
}

func (p *reParser) current() int {
	c, _ := p.at(p.pos)
	return c
}

func (p *reParser) next() int {
	_, n := p.at(p.pos)
	if n == 0 {
		return reEnd
	}
	c, _ := p.at(p.pos + n)
	return c
}

func (p *reParser) advance() {
	_, n := p.at(p.pos)
	if n == 0 {
		n = 1
	}
	p.pos += n
}

func (p *reParser) advanceN(n int) {
	for i := 0; i < n; i++ {
		p.advance()
	}
}

func (p *reParser) fail() bool {
	p.failed = true
	return false
}

func (p *reParser) parse() bool {
	if !p.parseDisjunction() {
		return false
	}
	for _, ref := range p.namedRefs {
		if !p.names[ref] {
			return false
		}
	}
	return !p.failed
}

func isSyntaxCharacterOrSlash(c int) bool {
	return c >= 0 && c < 0x80 && strings.ContainsRune(`^$\.*+?()[]{}|/`, rune(c))
}

func isClassSetSyntaxCharacter(c int) bool {
	return c >= 0 && c < 0x80 && strings.ContainsRune(`()[]{}/-\|`, rune(c))
}

func isClassSetReservedPunctuator(c int) bool {
	return c >= 0 && c < 0x80 && strings.ContainsRune("&-!#%,:;<=>@`~", rune(c))
}

func isDigit(c int) bool { return c >= '0' && c <= '9' }

type reFrame struct {
	kind groupKind
	name string
}

func (p *reParser) parseDisjunction() bool {
	var stack []reFrame
	for {
		// The atom (or assertion) at the current position; a closed
		// lookbehind (or lookahead in unicode mode) takes no quantifier.
		lookbehind, lookahead := false, false
		switch c := p.current(); c {
		case reEnd:
			if len(stack) > 0 {
				return p.fail() // Unterminated group
			}
			return true
		case ')':
			if len(stack) == 0 {
				return p.fail() // Unmatched ')'
			}
			p.advance()
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if top.kind == groupCapture && top.name != "" {
				if p.names[top.name] {
					return p.fail() // Duplicate capture group name
				}
				p.names[top.name] = true
			}
			lookbehind = top.kind == groupLookbehind
			lookahead = top.kind == groupLookahead
		case '|':
			p.advance()
			continue
		case '*', '+', '?':
			return p.fail() // Nothing to repeat
		case '^', '$':
			p.advance()
			continue
		case '.':
			p.advance()
		case '(':
			frame, ok := p.parseOpenParenthesis()
			if !ok {
				return false
			}
			stack = append(stack, frame)
			continue
		case '[':
			if !p.parseCharacterClass() {
				return false
			}
		case '\\':
			assertion, ok := p.parseAtomEscape(stack)
			if !ok {
				return false
			}
			if assertion {
				continue
			}
		case '{':
			if _, _, ok := p.parseIntervalQuantifier(); ok {
				return p.fail() // Nothing to repeat
			}
			if p.unicode {
				return p.fail() // Lone quantifier brackets
			}
			p.advance()
		case '}', ']':
			if p.unicode {
				return p.fail() // Lone quantifier brackets
			}
			p.advance()
		default:
			p.advance()
		}

		// Quantifier.
		switch p.current() {
		case '*', '+', '?':
			p.advance()
		case '{':
			min, max, ok := p.parseIntervalQuantifier()
			if ok {
				if max < min {
					return p.fail() // numbers out of order in {} quantifier
				}
			} else {
				if p.unicode {
					return p.fail() // Incomplete quantifier
				}
				continue
			}
		default:
			continue
		}
		if p.current() == '?' {
			p.advance()
		}
		if lookbehind || (lookahead && p.unicode) {
			return p.fail() // Invalid quantifier
		}
	}
}

// parseAtomEscape parses an escape outside a character class. It reports
// whether the escape was an assertion (\b or \B), which takes no
// quantifier.
func (p *reParser) parseAtomEscape(stack []reFrame) (assertion bool, ok bool) {
	switch n := p.next(); {
	case n == reEnd:
		return false, p.fail() // \ at end of pattern
	case n >= '1' && n <= '9':
		start := p.pos
		if p.parseBackReferenceIndex() {
			return false, true
		}
		p.pos = start
		if p.unicode {
			return false, p.fail() // Invalid escape
		}
		if n == '8' || n == '9' {
			p.advanceN(2)
			return false, true
		}
		p.advance()
		p.parseOctalLiteral()
		return false, true
	case n == '0':
		p.advance()
		if p.unicode && isDigit(p.next()) {
			return false, p.fail() // Invalid decimal escape
		}
		p.parseOctalLiteral()
		return false, true
	case n == 'b' || n == 'B':
		p.advanceN(2)
		return true, true
	case n == 'd' || n == 'D' || n == 's' || n == 'S' || n == 'w' || n == 'W':
		p.advanceN(2)
		return false, true
	case (n == 'p' || n == 'P') && p.unicode:
		p.advanceN(2)
		_, ok := p.parsePropertyEscape(n == 'P')
		return false, ok
	case n == 'k' && (p.unicode || p.hasNamedCaptures(false)):
		p.advanceN(2)
		return false, p.parseNamedBackReference(stack)
	}
	_, ok = p.parseCharacterEscape(false)
	return false, ok
}

// parseBackReferenceIndex reads \N when N is at most the number of capture
// groups in the whole pattern; otherwise it leaves the position for the
// caller to reset.
func (p *reParser) parseBackReferenceIndex() bool {
	value := p.next() - '0'
	p.advanceN(2)
	for isDigit(p.current()) {
		value = 10*value + p.current() - '0'
		if value > 1<<16 {
			return false
		}
		p.advance()
	}
	if value > p.capturesStarted {
		if !p.scanned {
			p.scanForCaptures(false)
		}
		if value > p.captureCount {
			return false
		}
	}
	return true
}

func (p *reParser) parseOctalLiteral() {
	value := p.current() - '0'
	p.advance()
	if c := p.current(); c >= '0' && c <= '7' {
		value = value*8 + c - '0'
		p.advance()
		if c := p.current(); value < 32 && c >= '0' && c <= '7' {
			p.advance()
		}
	}
}

// hasNamedCaptures reports whether the pattern has a named capture group.
func (p *reParser) hasNamedCaptures(inClass bool) bool {
	if p.hasNamed || p.scanned {
		return p.hasNamed
	}
	p.scanForCaptures(inClass)
	return p.hasNamed
}

// scanForCaptures counts the capture groups from the current position on and
// notes whether one of them is named.
func (p *reParser) scanForCaptures(inClass bool) {
	saved := p.pos
	count := p.capturesStarted
	if inClass {
		for c := p.current(); c != reEnd; c = p.current() {
			p.advance()
			if c == '\\' {
				p.advance()
			} else if c == ']' {
				break
			}
		}
	}
	for c := p.current(); c != reEnd; c = p.current() {
		p.advance()
		switch c {
		case '\\':
			p.advance()
		case '[':
			for d := p.current(); d != reEnd; d = p.current() {
				p.advance()
				if d == '\\' {
					p.advance()
				} else if d == ']' {
					break
				}
			}
		case '(':
			if p.current() == '?' {
				p.advance()
				if p.current() != '<' {
					continue
				}
				p.advance()
				if p.current() == '=' || p.current() == '!' {
					continue
				}
				p.hasNamed = true
			}
			count++
		}
	}
	p.captureCount = count
	p.scanned = true
	p.pos = saved
}

func (p *reParser) parseOpenParenthesis() (reFrame, bool) {
	frame := reFrame{kind: groupCapture}
	p.advance()
	if p.current() == '?' {
		switch p.next() {
		case ':':
			p.advanceN(2)
			frame.kind = groupPlain
		case '=', '!':
			p.advanceN(2)
			frame.kind = groupLookahead
		case '<':
			p.advance()
			if n := p.next(); n == '=' || n == '!' {
				p.advanceN(2)
				frame.kind = groupLookbehind
				break
			}
			p.hasNamed = true
			p.advance()
			p.capturesStarted++
			name, ok := p.parseCaptureGroupName()
			if !ok {
				return frame, false
			}
			frame.name = name
			return frame, true
		default:
			return frame, p.fail() // Invalid group
		}
	}
	if frame.kind == groupCapture {
		p.capturesStarted++
	}
	return frame, true
}

// parseCaptureGroupName reads a group name up to and including '>'.
func (p *reParser) parseCaptureGroupName() (string, bool) {
	p.forceUnicode = true
	defer func() { p.forceUnicode = false }()

	var name []byte
	for atStart := true; ; atStart = false {
		c := p.current()
		p.advance()
		if c == '\\' && p.current() == 'u' {
			p.advance()
			v, ok := p.parseUnicodeEscape()
			if !ok {
				return "", p.fail() // Invalid Unicode escape sequence
			}
			c = v
		} else if c == '\\' {
			return "", p.fail() // Invalid capture group name
		}
		if atStart {
			if c < 0 || !inRanges(groupNameStart, c) {
				return "", p.fail()
			}
		} else if c == '>' {
			break
		} else if c < 0 || !inRanges(groupNamePart, c) {
			return "", p.fail()
		}
		name = append(name, fromCodePoint(c)...)
	}
	return string(name), true
}

func (p *reParser) parseNamedBackReference(stack []reFrame) bool {
	if p.current() != '<' {
		return p.fail() // Invalid named reference
	}
	p.advance()
	name, ok := p.parseCaptureGroupName()
	if !ok {
		return false
	}
	for _, f := range stack {
		if f.name == name {
			return true // a reference inside the group it names matches empty
		}
	}
	p.namedRefs = append(p.namedRefs, name)
	return true
}

// parseHexEscape reads exactly n hex digits, restoring the position when
// they are not there.
func (p *reParser) parseHexEscape(n int) (int, bool) {
	start := p.pos
	value := 0
	for i := 0; i < n; i++ {
		d := hexValue(p.current())
		if d < 0 {
			p.pos = start
			return 0, false
		}
		value = value*16 + d
		p.advance()
	}
	return value, true
}

// parseUnicodeEscape reads the rest of a \u escape: \uXXXX, a surrogate
// pair of such escapes, or \u{...} in unicode mode.
func (p *reParser) parseUnicodeEscape() (int, bool) {
	if p.current() == '{' && p.uni() {
		start := p.pos
		p.advance()
		value := 0
		digits := 0
		for d := hexValue(p.current()); d >= 0; d = hexValue(p.current()) {
			value = value*16 + d
			digits++
			if value > 0x10FFFF {
				p.pos = start
				return 0, false
			}
			p.advance()
		}
		if digits > 0 && p.current() == '}' {
			p.advance()
			return value, true
		}
		p.pos = start
		return 0, false
	}
	value, ok := p.parseHexEscape(4)
	if ok && p.uni() && value >= 0xD800 && value <= 0xDBFF && p.current() == '\\' {
		start := p.pos
		if p.next() == 'u' {
			p.advanceN(2)
			if trail, ok := p.parseHexEscape(4); ok && trail >= 0xDC00 && trail <= 0xDFFF {
				return (value-0xD800)<<10 + (trail - 0xDC00) + 0x10000, true
			}
		}
		p.pos = start
	}
	return value, ok
}

// parseCharacterEscape parses a character escape (the current character is
// the backslash) and returns the character it denotes.
func (p *reParser) parseCharacterEscape(inClass bool) (int, bool) {
	p.advance() // the backslash
	c := p.current()
	switch c {
	case 'f', 'n', 'r', 't', 'v':
		p.advance()
		return c, true
	case 'c':
		control := p.next()
		letter := control &^ ('A' ^ 'a')
		if control >= 0 && letter >= 'A' && letter <= 'Z' {
			p.advanceN(2)
			return control & 0x1F, true
		}
		if p.unicode {
			return 0, p.fail() // Invalid Unicode escape
		}
		if inClass && (isDigit(control) || control == '_') {
			p.advanceN(2)
			return control & 0x1F, true
		}
		// Read the backslash as a literal character.
		return '\\', true
	case '0':
		if !isDigit(p.next()) {
			p.advance()
			return 0, true
		}
		fallthrough
	case '1', '2', '3', '4', '5', '6', '7':
		if p.unicode {
			return 0, p.fail() // Invalid class escape
		}
		start := p.pos
		p.parseOctalLiteral()
		value := 0
		for i := start; i < p.pos; i++ {
			value = value*8 + int(p.src[i]) - '0'
		}
		return value, true
	case 'x':
		p.advance()
		if v, ok := p.parseHexEscape(2); ok {
			return v, true
		}
		if p.unicode {
			return 0, p.fail() // Invalid escape
		}
		return 'x', true
	case 'u':
		p.advance()
		if v, ok := p.parseUnicodeEscape(); ok {
			return v, true
		}
		if p.unicode {
			return 0, p.fail() // Invalid Unicode escape
		}
		return 'u', true
	}

	if !p.unicode {
		if c == 'k' && p.hasNamedCaptures(inClass) {
			return 0, p.fail() // Invalid escape
		}
		p.advance()
		return c, true
	}
	if isSyntaxCharacterOrSlash(c) {
		p.advance()
		return c, true
	}
	if p.sets && inClass && isClassSetReservedPunctuator(c) {
		p.advance()
		return c, true
	}
	return 0, p.fail() // Invalid escape
}

// parseIntervalQuantifier reads {n}, {n,} or {n,m}; on failure the position
// is unchanged.
func (p *reParser) parseIntervalQuantifier() (int, int, bool) {
	const infinity = 1<<31 - 1
	start := p.pos
	p.advance()
	readNumber := func() int {
		n := 0
		for isDigit(p.current()) {
			d := p.current() - '0'
			if n > (infinity-d)/10 {
				for isDigit(p.current()) {
					p.advance()
				}
				return infinity
			}
			n = 10*n + d
			p.advance()
		}
		return n
	}
	if !isDigit(p.current()) {
		p.pos = start
		return 0, 0, false
	}
	min := readNumber()
	max := min
	switch p.current() {
	case '}':
		p.advance()
	case ',':
		p.advance()
		if p.current() == '}' {
			max = infinity
			p.advance()
			break
		}
		max = readNumber()
		if p.current() != '}' {
			p.pos = start
			return 0, 0, false
		}
		p.advance()
	default:
		p.pos = start
		return 0, 0, false
	}
	return min, max, true
}

// parsePropertyEscape reads {name} or {name=value} after \p or \P and
// reports whether the property may match strings.
func (p *reParser) parsePropertyEscape(negate bool) (strings bool, ok bool) {
	if p.current() != '{' {
		return false, p.fail()
	}
	var name, value []byte
	hasValue := false
	p.advance()
	for c := p.current(); c != '}' && c != '='; c = p.current() {
		if !isPropertyCharacter(c) || p.pos+1 >= len(p.src) {
			return false, p.fail()
		}
		name = append(name, byte(c))
		p.advance()
	}
	if p.current() == '=' {
		hasValue = true
		p.advance()
		for c := p.current(); c != '}'; c = p.current() {
			if !isPropertyCharacter(c) || p.pos+1 >= len(p.src) {
				return false, p.fail()
			}
			value = append(value, byte(c))
			p.advance()
		}
	}
	p.advance()

	if hasValue {
		var values map[string]bool
		switch string(name) {
		case "General_Category", "gc":
			values = propGC
		case "Script", "sc":
			values = propScript
		case "Script_Extensions", "scx":
			values = propScriptExt
		default:
			return false, p.fail()
		}
		if !values[string(value)] {
			return false, p.fail()
		}
		return false, true
	}
	n := string(name)
	switch {
	case !p.sets:
		return false, propLoneU[n] || p.fail()
	case negate:
		return false, propNegV[n] || p.fail()
	case propLoneV[n]:
		return !propLoneU[n], true
	}
	return false, p.fail()
}

func isPropertyCharacter(c int) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || isDigit(c) || c == '_'
}

// parseCharacterClass parses a class starting at '['.
func (p *reParser) parseCharacterClass() bool {
	_, ok := p.parseClass()
	return ok
}

// parseClass parses a class and, in v mode, reports whether it may match
// strings.
func (p *reParser) parseClass() (bool, bool) {
	p.advance() // '['
	negated := false
	if p.current() == '^' {
		negated = true
		p.advance()
	}
	if p.current() == ']' {
		p.advance()
		return false, true
	}
	if p.sets {
		return p.parseClassSetExpression(negated)
	}

	for p.current() != reEnd && p.current() != ']' {
		char1, isClass1, ok := p.parseClassEscape()
		if !ok {
			return false, false
		}
		if p.current() != '-' {
			continue
		}
		p.advance()
		if p.current() == reEnd {
			break
		}
		if p.current() == ']' {
			break
		}
		char2, isClass2, ok := p.parseClassEscape()
		if !ok {
			return false, false
		}
		if isClass1 || isClass2 {
			if p.unicode {
				return false, p.fail() // Invalid character class
			}
			continue
		}
		if char1 > char2 {
			return false, p.fail() // Range out of order in character class
		}
	}
	if p.current() == reEnd {
		return false, p.fail() // Unterminated character class
	}
	p.advance()
	return false, true
}

// parseClassEscape reads one class atom outside v mode, reporting whether it
// is a class escape such as \d.
func (p *reParser) parseClassEscape() (int, bool, bool) {
	c := p.current()
	if c != '\\' {
		p.advance()
		return c, false, true
	}
	switch n := p.next(); {
	case n == 'b':
		p.advanceN(2)
		return '\b', false, true
	case n == '-' && p.unicode:
		p.advanceN(2)
		return '-', false, true
	case n == reEnd:
		return 0, false, p.fail() // \ at end of pattern
	case n == 'd' || n == 'D' || n == 's' || n == 'S' || n == 'w' || n == 'W':
		p.advanceN(2)
		return 0, true, true
	case (n == 'p' || n == 'P') && p.unicode:
		p.advanceN(2)
		_, ok := p.parsePropertyEscape(n == 'P')
		return 0, true, ok
	}
	v, ok := p.parseCharacterEscape(true)
	return v, false, ok
}

type operandKind int

const (
	operandCharacter operandKind = iota
	operandEscape
	operandStrings
	operandNested
	operandRange
)

// parseClassSetOperand reads a v-mode operand: a nested class, a \q{...}
// string disjunction, a class escape or a single character.
func (p *reParser) parseClassSetOperand() (kind operandKind, char int, mayStrings bool, ok bool) {
	c := p.current()
	if c == '\\' {
		switch n := p.next(); {
		case n == 'q':
			s, ok := p.parseClassStringDisjunction()
			return operandStrings, 0, s, ok
		case n == 'd' || n == 'D' || n == 's' || n == 'S' || n == 'w' || n == 'W':
			p.advanceN(2)
			return operandEscape, 0, false, true
		case n == 'p' || n == 'P':
			p.advanceN(2)
			s, ok := p.parsePropertyEscape(n == 'P')
			return operandEscape, 0, s, ok
		}
	}
	if c == '[' {
		s, ok := p.parseClass()
		return operandNested, 0, s, ok
	}
	ch, ok := p.parseClassSetCharacter()
	return operandCharacter, ch, false, ok
}

func (p *reParser) parseClassSetCharacter() (int, bool) {
	c := p.current()
	if c == '\\' {
		switch p.next() {
		case 'b':
			p.advanceN(2)
			return '\b', true
		case reEnd:
			return 0, p.fail() // \ at end of pattern
		}
		return p.parseCharacterEscape(true)
	}
	if isClassSetSyntaxCharacter(c) {
		return 0, p.fail() // Invalid character in character class
	}
	if c >= 0 && c == p.next() && strings.ContainsRune("&!#$%*+,.:;<=>?@^`~", rune(c)) {
		return 0, p.fail() // Invalid set operation in character class
	}
	p.advance()
	return c, true
}

// parseClassStringDisjunction reads \q{...} and reports whether one of its
// alternatives is not a single character.
func (p *reParser) parseClassStringDisjunction() (bool, bool) {
	p.advanceN(2)
	if p.current() != '{' {
		return false, p.fail() // Invalid escape
	}
	p.advance()
	mayStrings := false
	length := 0
	for p.current() != reEnd && p.current() != '}' {
		if p.current() == '|' {
			if length != 1 {
				mayStrings = true
			}
			length = 0
			p.advance()
			continue
		}
		if _, ok := p.parseClassSetCharacter(); !ok {
			return false, false
		}
		length++
	}
	if length != 1 {
		mayStrings = true
	}
	p.advance()
	return mayStrings, true
}

func (p *reParser) parseClassSetExpression(negated bool) (bool, bool) {
	kind, char, mayStrings, ok := p.parseClassSetOperand()
	if !ok {
		return false, false
	}
	if p.current() == '-' && p.next() == '-' {
		return p.parseClassSubtraction(negated, mayStrings)
	}
	if p.current() == '&' && p.next() == '&' {
		return p.parseClassIntersection(negated, mayStrings)
	}
	return p.parseClassUnion(negated, kind, char, mayStrings)
}

func (p *reParser) parseClassUnion(negated bool, last operandKind, char int, mayStrings bool) (bool, bool) {
	for p.current() != reEnd && p.current() != ']' {
		if p.current() == '-' {
			if last != operandCharacter {
				return false, p.fail() // Invalid character in character class
			}
			from := char
			p.advance()
			if p.current() == '-' {
				return false, p.fail() // Invalid set operation in character class
			}
			to, ok := p.parseClassSetCharacter()
			if !ok {
				return false, false
			}
			if from > to {
				return false, p.fail() // Range out of order in character class
			}
			last = operandRange
			continue
		}
		if p.current() == '&' && p.next() == '&' {
			return false, p.fail() // Invalid set operation in character class
		}
		kind, ch, s, ok := p.parseClassSetOperand()
		if !ok {
			return false, false
		}
		last, char = kind, ch
		mayStrings = mayStrings || s
	}
	if p.current() == reEnd {
		return false, p.fail() // Unterminated character class
	}
	p.advance()
	if negated && mayStrings {
		return false, p.fail() // Negated character class may contain strings
	}
	return mayStrings, true
}

func (p *reParser) parseClassIntersection(negated, mayStrings bool) (bool, bool) {
	for p.current() != reEnd && p.current() != ']' {
		if p.current() != '&' || p.next() != '&' {
			return false, p.fail() // Invalid set operation in character class
		}
		p.advanceN(2)
		if p.current() == '&' {
			return false, p.fail() // Invalid character in character class
		}
		_, _, s, ok := p.parseClassSetOperand()
		if !ok {
			return false, false
		}
		if !s {
			mayStrings = false
		}
	}
	if p.current() == reEnd {
		return false, p.fail() // Unterminated character class
	}
	if negated && mayStrings {
		return false, p.fail() // Negated character class may contain strings
	}
	p.advance()
	return mayStrings, true
}

func (p *reParser) parseClassSubtraction(negated, mayStrings bool) (bool, bool) {
	for p.current() != reEnd && p.current() != ']' {
		if p.current() != '-' || p.next() != '-' {
			return false, p.fail() // Invalid set operation in character class
		}
		p.advanceN(2)
		if _, _, _, ok := p.parseClassSetOperand(); !ok {
			return false, false
		}
	}
	if p.current() == reEnd {
		return false, p.fail() // Unterminated character class
	}
	if negated && mayStrings {
		return false, p.fail() // Negated character class may contain strings
	}
	p.advance()
	return mayStrings, true
}
