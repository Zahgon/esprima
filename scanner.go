/*
  Copyright JS Foundation and other contributors, https://js.foundation/

  Redistribution and use in source and binary forms, with or without
  modification, are permitted provided that the following conditions are met:

    * Redistributions of source code must retain the above copyright
      notice, this list of conditions and the following disclaimer.
    * Redistributions in binary form must reproduce the above copyright
      notice, this list of conditions and the following disclaimer in the
      documentation and/or other materials provided with the distribution.

  THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
  AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
  IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
  ARE DISCLAIMED. IN NO EVENT SHALL <COPYRIGHT HOLDER> BE LIABLE FOR ANY
  DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES
  (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES;
  LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND
  ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
  (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF
  THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
*/

// Go port of src/scanner.ts.

package esprima

import "strings"

// rawToken is a token as the scanner produces it. Positions are UTF-16
// offsets into the source.
type rawToken struct {
	typ   tokenType
	str   string  // the value of every token except numeric literals
	num   float64 // the value of a numeric literal
	isNum bool

	// Regular expression literals.
	pattern string
	flags   string
	regex   *RegExp // nil when the host engine rejects the pattern and flags

	octal bool

	// Template elements.
	cooked                string
	cookedNull            bool
	notEscapeSequenceHead string // "" when every escape sequence is valid
	head, tail            bool

	lineNumber int
	lineStart  int
	start      int
	end        int
}

// value returns the token value as the original holds it: a number for
// numeric literals and a string otherwise.
func (t *rawToken) value() any {
	if t.isNum {
		return t.num
	}
	return t.str
}

type sourceLocation struct {
	startLine, startColumn int
	endLine, endColumn     int
}

// scannedComment is a comment found while skipping whitespace.
type scannedComment struct {
	multiLine bool
	slice     [2]int
	rng       [2]int
	loc       sourceLocation
}

type scannerState struct {
	index      int
	lineNumber int
	lineStart  int
	curlyStack []string
}

// scanner turns UTF-16 source text into tokens.
type scanner struct {
	source       []uint16
	errorHandler *errorHandler
	trackComment bool
	isModule     bool

	index      int
	lineNumber int
	lineStart  int
	curlyStack []string
	length     int
}

func newScanner(code []uint16, handler *errorHandler) *scanner {
	s := &scanner{source: code, errorHandler: handler, length: len(code)}
	if len(code) > 0 {
		s.lineNumber = 1
	}
	return s
}

// cu returns the code unit at i, or -1 past either end (the original's NaN
// or undefined).
func (s *scanner) cu(i int) int {
	if i < 0 || i >= s.length {
		return -1
	}
	return int(s.source[i])
}

// slice returns the source text between two offsets as a JavaScript string.
func (s *scanner) slice(start, end int) string {
	if start < 0 {
		start = 0
	}
	if end > s.length {
		end = s.length
	}
	if start >= end {
		return ""
	}
	return unitsString(s.source[start:end])
}

func (s *scanner) saveState() scannerState {
	return scannerState{
		index:      s.index,
		lineNumber: s.lineNumber,
		lineStart:  s.lineStart,
		curlyStack: append([]string(nil), s.curlyStack...),
	}
}

func (s *scanner) restoreState(st scannerState) {
	s.index = st.index
	s.lineNumber = st.lineNumber
	s.lineStart = st.lineStart
	s.curlyStack = st.curlyStack
}

func (s *scanner) eof() bool {
	return s.index >= s.length
}

func (s *scanner) throwUnexpectedToken(message string) {
	s.errorHandler.throwError(s.index, s.lineNumber, s.index-s.lineStart+1, message)
}

func (s *scanner) tolerateUnexpectedToken(message string) {
	s.errorHandler.tolerateError(s.index, s.lineNumber, s.index-s.lineStart+1, message)
}

// https://tc39.github.io/ecma262/#sec-comments

func (s *scanner) skipSingleLineComment(offset int) []scannedComment {
	var comments []scannedComment
	var start int
	var loc sourceLocation
	if s.trackComment {
		start = s.index - offset
		loc.startLine = s.lineNumber
		loc.startColumn = s.index - s.lineStart - offset
	}

	for !s.eof() {
		ch := s.cu(s.index)
		s.index++
		if isLineTerminator(ch) {
			if s.trackComment {
				loc.endLine = s.lineNumber
				loc.endColumn = s.index - s.lineStart - 1
				comments = append(comments, scannedComment{
					slice: [2]int{start + offset, s.index - 1},
					rng:   [2]int{start, s.index - 1},
					loc:   loc,
				})
			}
			if ch == 13 && s.cu(s.index) == 10 {
				s.index++
			}
			s.lineNumber++
			s.lineStart = s.index
			return comments
		}
	}

	if s.trackComment {
		loc.endLine = s.lineNumber
		loc.endColumn = s.index - s.lineStart
		comments = append(comments, scannedComment{
			slice: [2]int{start + offset, s.index},
			rng:   [2]int{start, s.index},
			loc:   loc,
		})
	}
	return comments
}

func (s *scanner) skipMultiLineComment() []scannedComment {
	var comments []scannedComment
	var start int
	var loc sourceLocation
	if s.trackComment {
		start = s.index - 2
		loc.startLine = s.lineNumber
		loc.startColumn = s.index - s.lineStart - 2
	}

	for !s.eof() {
		ch := s.cu(s.index)
		switch {
		case isLineTerminator(ch):
			if ch == 0x0D && s.cu(s.index+1) == 0x0A {
				s.index++
			}
			s.lineNumber++
			s.index++
			s.lineStart = s.index
		case ch == 0x2A:
			// Block comment ends with '*/'.
			if s.cu(s.index+1) == 0x2F {
				s.index += 2
				if s.trackComment {
					loc.endLine = s.lineNumber
					loc.endColumn = s.index - s.lineStart
					comments = append(comments, scannedComment{
						multiLine: true,
						slice:     [2]int{start + 2, s.index - 2},
						rng:       [2]int{start, s.index},
						loc:       loc,
					})
				}
				return comments
			}
			s.index++
		default:
			s.index++
		}
	}

	// Ran off the end of the file: the whole rest is a comment.
	if s.trackComment {
		loc.endLine = s.lineNumber
		loc.endColumn = s.index - s.lineStart
		comments = append(comments, scannedComment{
			multiLine: true,
			slice:     [2]int{start + 2, s.index},
			rng:       [2]int{start, s.index},
			loc:       loc,
		})
	}
	s.tolerateUnexpectedToken(msgUnexpectedTokenIllegal)
	return comments
}

// scanComments skips whitespace and comments, returning the comments when
// they are tracked.
func (s *scanner) scanComments() []scannedComment {
	var comments []scannedComment
	start := s.index == 0
	for !s.eof() {
		ch := s.cu(s.index)
		switch {
		case isWhiteSpace(ch):
			s.index++
		case isLineTerminator(ch):
			s.index++
			if ch == 0x0D && s.cu(s.index) == 0x0A {
				s.index++
			}
			s.lineNumber++
			s.lineStart = s.index
			start = true
		case ch == 0x2F: // '/'
			ch = s.cu(s.index + 1)
			if ch == 0x2F {
				s.index += 2
				comments = append(comments, s.skipSingleLineComment(2)...)
				start = true
			} else if ch == 0x2A { // '*'
				s.index += 2
				comments = append(comments, s.skipMultiLineComment()...)
			} else {
				return comments
			}
		case start && ch == 0x2D: // '-'
			// '-->' is a single-line comment at the start of a line.
			if s.cu(s.index+1) == 0x2D && s.cu(s.index+2) == 0x3E {
				s.index += 3
				comments = append(comments, s.skipSingleLineComment(3)...)
			} else {
				return comments
			}
		case ch == 0x3C && !s.isModule: // '<'
			if s.slice(s.index+1, s.index+4) == "!--" {
				s.index += 4 // `<!--`
				comments = append(comments, s.skipSingleLineComment(4)...)
			} else {
				return comments
			}
		default:
			return comments
		}
	}
	return comments
}

// https://tc39.github.io/ecma262/#sec-future-reserved-words

func isFutureReservedWord(id string) bool {
	switch id {
	case "enum", "export", "import", "super":
		return true
	}
	return false
}

func isStrictModeReservedWord(id string) bool {
	switch id {
	case "implements", "interface", "package", "private", "protected", "public", "static", "yield", "let":
		return true
	}
	return false
}

func isRestrictedWord(id string) bool {
	return id == "eval" || id == "arguments"
}

// https://tc39.github.io/ecma262/#sec-keywords
func isKeyword(id string) bool {
	switch id {
	case "if", "in", "do",
		"var", "for", "new", "try", "let",
		"this", "else", "case", "void", "with", "enum",
		"while", "break", "catch", "throw", "const", "yield", "class", "super",
		"return", "typeof", "delete", "switch", "export", "import",
		"default", "finally", "extends",
		"function", "continue", "debugger",
		"instanceof":
		return true
	}
	return false
}

func (s *scanner) codePointAt(i int) int {
	cp := s.cu(i)
	if cp >= 0xD800 && cp <= 0xDBFF {
		second := s.cu(i + 1)
		if second >= 0xDC00 && second <= 0xDFFF {
			cp = (cp-0xD800)*0x400 + second - 0xDC00 + 0x10000
		}
	}
	return cp
}

// scanHexEscape reads the 4 (\u) or 2 (\x) hex digits of an escape and
// returns the code unit, or false when they are missing.
func (s *scanner) scanHexEscape(prefix byte) (string, bool) {
	n := 2
	if prefix == 'u' {
		n = 4
	}
	code := 0
	for i := 0; i < n; i++ {
		if s.eof() || !isHexDigit(s.cu(s.index)) {
			return "", false
		}
		code = code*16 + hexValue(s.cu(s.index))
		s.index++
	}
	return unitsString([]uint16{uint16(code)}), true
}

func (s *scanner) tryToScanUnicodeCodePointEscape() (string, bool) {
	ch := s.cu(s.index)
	code := 0
	// At least one hex digit is required.
	if ch == '}' {
		return "", false
	}
	for !s.eof() {
		ch = s.cu(s.index)
		s.index++
		if !isHexDigit(ch) {
			break
		}
		if code <= 0x10FFFF {
			code = code*16 + hexValue(ch)
		}
	}
	if code > 0x10FFFF || ch != '}' {
		return "", false
	}
	return fromCodePoint(code), true
}

func (s *scanner) scanUnicodeCodePointEscape() string {
	result, ok := s.tryToScanUnicodeCodePointEscape()
	if !ok {
		s.throwUnexpectedToken(msgUnexpectedTokenIllegal)
	}
	return result
}

func (s *scanner) getIdentifier() string {
	start := s.index
	s.index++
	for !s.eof() {
		ch := s.cu(s.index)
		if ch == 0x5C {
			// A backslash starts a Unicode escape sequence.
			s.index = start
			return s.getComplexIdentifier()
		} else if ch >= 0xD800 && ch < 0xDFFF {
			// Surrogate pairs need the code point path.
			s.index = start
			return s.getComplexIdentifier()
		}
		if !isIdentifierPart(ch) {
			break
		}
		s.index++
	}
	return s.slice(start, s.index)
}

func (s *scanner) getComplexIdentifier() string {
	cp := s.codePointAt(s.index)
	id := fromCodePoint(cp)
	s.index += unitLength(id)

	// '\u' (U+005C, U+0075) denotes an escaped character.
	if cp == 0x5C {
		if s.cu(s.index) != 0x75 {
			s.throwUnexpectedToken(msgUnexpectedTokenIllegal)
		}
		s.index++
		var ch string
		if s.cu(s.index) == '{' {
			s.index++
			ch = s.scanUnicodeCodePointEscape()
		} else {
			var ok bool
			ch, ok = s.scanHexEscape('u')
			if !ok || ch == "\\" || !isIdentifierStart(int(Units(ch)[0])) {
				s.throwUnexpectedToken(msgUnexpectedTokenIllegal)
			}
		}
		id = ch
	}

	for !s.eof() {
		cp = s.codePointAt(s.index)
		if !isIdentifierPart(cp) {
			break
		}
		ch := fromCodePoint(cp)
		id += ch
		s.index += unitLength(ch)

		// '\u' (U+005C, U+0075) denotes an escaped character.
		if cp == 0x5C {
			id = id[:len(id)-1]
			if s.cu(s.index) != 0x75 {
				s.throwUnexpectedToken(msgUnexpectedTokenIllegal)
			}
			s.index++
			if s.cu(s.index) == '{' {
				s.index++
				ch = s.scanUnicodeCodePointEscape()
			} else {
				var ok bool
				ch, ok = s.scanHexEscape('u')
				if !ok || ch == "\\" || !isIdentifierPart(int(Units(ch)[0])) {
					s.throwUnexpectedToken(msgUnexpectedTokenIllegal)
				}
			}
			id += ch
		}
	}
	return id
}

// octalToDecimal reads the rest of a legacy octal escape whose first digit
// is ch.
func (s *scanner) octalToDecimal(ch int) (code int, octal bool) {
	// \0 is not an octal escape sequence.
	octal = ch != '0'
	code = ch - '0'
	if !s.eof() && isOctalDigit(s.cu(s.index)) {
		octal = true
		code = code*8 + s.cu(s.index) - '0'
		s.index++
		// Three digits are only allowed when the escape starts with 0-3.
		if ch >= '0' && ch <= '3' && !s.eof() && isOctalDigit(s.cu(s.index)) {
			code = code*8 + s.cu(s.index) - '0'
			s.index++
		}
	}
	return code, octal
}

// https://tc39.github.io/ecma262/#sec-names-and-keywords
func (s *scanner) scanIdentifier() *rawToken {
	start := s.index
	var id string
	if s.cu(start) == 0x5C {
		id = s.getComplexIdentifier()
	} else {
		id = s.getIdentifier()
	}

	// There is no keyword or literal with only one character.
	var typ tokenType
	switch {
	case unitLength(id) == 1:
		typ = tokIdentifier
	case isKeyword(id):
		typ = tokKeyword
	case id == "null":
		typ = tokNullLiteral
	case id == "true" || id == "false":
		typ = tokBooleanLiteral
	default:
		typ = tokIdentifier
	}

	if typ != tokIdentifier && start+unitLength(id) != s.index {
		restore := s.index
		s.index = start
		s.tolerateUnexpectedToken(msgInvalidEscapedReservedWord)
		s.index = restore
	}

	return &rawToken{typ: typ, str: id, lineNumber: s.lineNumber, lineStart: s.lineStart, start: start, end: s.index}
}

// https://tc39.github.io/ecma262/#sec-punctuators
func (s *scanner) scanPunctuator() *rawToken {
	start := s.index
	str := s.slice(s.index, s.index+1)
	switch str {
	case "(", "{":
		if str == "{" {
			s.curlyStack = append(s.curlyStack, "{")
		}
		s.index++
	case ".":
		s.index++
		if s.cu(s.index) == '.' && s.cu(s.index+1) == '.' {
			// Spread operator: ...
			s.index += 2
			str = "..."
		}
	case "}":
		s.index++
		if len(s.curlyStack) > 0 {
			s.curlyStack = s.curlyStack[:len(s.curlyStack)-1]
		}
	case "?":
		s.index++
		if s.cu(s.index) == '?' {
			s.index++
			str = "??"
		}
		if s.cu(s.index) == '.' && !isDecimalDigit(s.cu(s.index+1)) {
			// "?." in "foo?.3:0" is not optional chaining.
			s.index++
			str = "?."
		}
	case ")", ";", ",", "[", "]", ":", "~":
		s.index++
	default:
		// 4-character punctuator.
		str = s.slice(s.index, s.index+4)
		if str == ">>>=" {
			s.index += 4
			break
		}
		// 3-character punctuators.
		str = s.slice(s.index, s.index+3)
		switch str {
		case "===", "!==", ">>>", "<<=", ">>=", "**=":
			s.index += 3
			return &rawToken{typ: tokPunctuator, str: str, lineNumber: s.lineNumber, lineStart: s.lineStart, start: start, end: s.index}
		}
		// 2-character punctuators.
		str = s.slice(s.index, s.index+2)
		switch str {
		case "&&", "||", "??", "==", "!=", "+=", "-=", "*=", "/=", "++", "--", "<<", ">>",
			"&=", "|=", "^=", "%=", "<=", ">=", "=>", "**":
			s.index += 2
		default:
			// 1-character punctuators.
			str = s.slice(s.index, s.index+1)
			if len(str) == 1 && strings.Contains("<>=!+-*%&|^/", str) {
				s.index++
			}
		}
	}

	if s.index == start {
		s.throwUnexpectedToken(msgUnexpectedTokenIllegal)
	}
	return &rawToken{typ: tokPunctuator, str: str, lineNumber: s.lineNumber, lineStart: s.lineStart, start: start, end: s.index}
}

// https://tc39.github.io/ecma262/#sec-literals-numeric-literals

func (s *scanner) numberToken(value float64, octal bool, start int) *rawToken {
	return &rawToken{typ: tokNumericLiteral, num: value, isNum: true, octal: octal,
		lineNumber: s.lineNumber, lineStart: s.lineStart, start: start, end: s.index}
}

func (s *scanner) scanHexLiteral(start int) *rawToken {
	digitsStart := s.index
	for !s.eof() && isHexDigit(s.cu(s.index)) {
		s.index++
	}
	num := s.slice(digitsStart, s.index)
	if num == "" {
		s.throwUnexpectedToken(msgUnexpectedTokenIllegal)
	}
	if isIdentifierStart(s.cu(s.index)) {
		s.throwUnexpectedToken(msgUnexpectedTokenIllegal)
	}
	return s.numberToken(parseIntDigits(num, 16), false, start)
}

func (s *scanner) scanBinaryLiteral(start int) *rawToken {
	digitsStart := s.index
	for !s.eof() {
		ch := s.cu(s.index)
		if ch != '0' && ch != '1' {
			break
		}
		s.index++
	}
	num := s.slice(digitsStart, s.index)
	if num == "" {
		// only 0b or 0B
		s.throwUnexpectedToken(msgUnexpectedTokenIllegal)
	}
	if !s.eof() {
		ch := s.cu(s.index)
		if isIdentifierStart(ch) || isDecimalDigit(ch) {
			s.throwUnexpectedToken(msgUnexpectedTokenIllegal)
		}
	}
	return s.numberToken(parseIntDigits(num, 2), false, start)
}

func (s *scanner) scanOctalLiteral(prefix int, start int) *rawToken {
	num := ""
	octal := false
	if isOctalDigit(prefix) {
		octal = true
		num = "0" + s.slice(s.index, s.index+1)
		s.index++
	} else {
		s.index++
	}
	digitsStart := s.index
	for !s.eof() && isOctalDigit(s.cu(s.index)) {
		s.index++
	}
	num += s.slice(digitsStart, s.index)

	if !octal && num == "" {
		// only 0o or 0O
		s.throwUnexpectedToken(msgUnexpectedTokenIllegal)
	}
	if isIdentifierStart(s.cu(s.index)) || isDecimalDigit(s.cu(s.index)) {
		s.throwUnexpectedToken(msgUnexpectedTokenIllegal)
	}
	return s.numberToken(parseIntDigits(num, 8), octal, start)
}

// isImplicitOctalLiteral reports whether a literal starting with 0 is a
// legacy octal one (Annex B.1.1): it is unless a digit 8 or 9 follows.
func (s *scanner) isImplicitOctalLiteral() bool {
	for i := s.index + 1; i < s.length; i++ {
		ch := s.cu(i)
		if ch == '8' || ch == '9' {
			return false
		}
		if !isOctalDigit(ch) {
			return true
		}
	}
	return true
}

func (s *scanner) scanNumericLiteral() *rawToken {
	start := s.index
	ch := s.cu(start)
	var num []byte
	if ch != '.' {
		num = append(num, byte(ch))
		s.index++
		ch = s.cu(s.index)

		// Hex number starts with '0x', octal with '0o' (or a legacy 0),
		// binary with '0b'.
		if num[0] == '0' {
			if ch == 'x' || ch == 'X' {
				s.index++
				return s.scanHexLiteral(start)
			}
			if ch == 'b' || ch == 'B' {
				s.index++
				return s.scanBinaryLiteral(start)
			}
			if ch == 'o' || ch == 'O' {
				return s.scanOctalLiteral(ch, start)
			}
			if isOctalDigit(ch) && s.isImplicitOctalLiteral() {
				return s.scanOctalLiteral(ch, start)
			}
		}
		for isDecimalDigit(s.cu(s.index)) {
			num = append(num, byte(s.cu(s.index)))
			s.index++
		}
		ch = s.cu(s.index)
	}

	if ch == '.' {
		num = append(num, '.')
		s.index++
		for isDecimalDigit(s.cu(s.index)) {
			num = append(num, byte(s.cu(s.index)))
			s.index++
		}
		ch = s.cu(s.index)
	}

	if ch == 'e' || ch == 'E' {
		num = append(num, byte(ch))
		s.index++
		ch = s.cu(s.index)
		if ch == '+' || ch == '-' {
			num = append(num, byte(ch))
			s.index++
		}
		if isDecimalDigit(s.cu(s.index)) {
			for isDecimalDigit(s.cu(s.index)) {
				num = append(num, byte(s.cu(s.index)))
				s.index++
			}
		} else {
			s.throwUnexpectedToken(msgUnexpectedTokenIllegal)
		}
	}

	if isIdentifierStart(s.cu(s.index)) {
		s.throwUnexpectedToken(msgUnexpectedTokenIllegal)
	}
	return s.numberToken(parseFloatPrefix(string(num)), false, start)
}

// https://tc39.github.io/ecma262/#sec-literals-string-literals
func (s *scanner) scanStringLiteral() *rawToken {
	start := s.index
	quote := s.cu(start)
	s.index++
	octal := false
	var str []byte

	for !s.eof() {
		ch := s.cu(s.index)
		s.index++
		if ch == quote {
			quote = 0
			break
		}
		if ch == '\\' {
			ch = s.cu(s.index)
			s.index++
			if ch < 0 || !isLineTerminator(ch) {
				switch ch {
				case 'u':
					if s.cu(s.index) == '{' {
						s.index++
						str = append(str, s.scanUnicodeCodePointEscape()...)
					} else {
						unescaped, ok := s.scanHexEscape('u')
						if !ok {
							s.throwUnexpectedToken(msgUnexpectedTokenIllegal)
						}
						str = append(str, unescaped...)
					}
				case 'x':
					unescaped, ok := s.scanHexEscape('x')
					if !ok {
						s.throwUnexpectedToken(msgInvalidHexEscapeSequence)
					}
					str = append(str, unescaped...)
				case 'n':
					str = append(str, '\n')
				case 'r':
					str = append(str, '\r')
				case 't':
					str = append(str, '\t')
				case 'b':
					str = append(str, '\b')
				case 'f':
					str = append(str, '\f')
				case 'v':
					str = append(str, '\x0B')
				case '8', '9':
					str = append(str, byte(ch))
					s.tolerateUnexpectedToken(msgUnexpectedTokenIllegal)
				default:
					if isOctalDigit(ch) {
						code, isOctal := s.octalToDecimal(ch)
						octal = isOctal || octal
						str = appendUnit(str, uint16(code))
					} else if ch >= 0 {
						str = appendUnit(str, uint16(ch))
					}
				}
			} else {
				s.lineNumber++
				if ch == '\r' && s.cu(s.index) == '\n' {
					s.index++
				}
				s.lineStart = s.index
			}
		} else if isLineTerminator(ch) {
			break
		} else {
			str = appendUnit(str, uint16(ch))
		}
	}

	if quote != 0 {
		s.index = start
		s.throwUnexpectedToken(msgUnexpectedTokenIllegal)
	}
	return &rawToken{typ: tokStringLiteral, str: string(str), octal: octal,
		lineNumber: s.lineNumber, lineStart: s.lineStart, start: start, end: s.index}
}

// https://tc39.github.io/ecma262/#sec-template-literal-lexical-components
func (s *scanner) scanTemplate() *rawToken {
	var cooked []byte
	terminated := false
	start := s.index
	head := s.cu(start) == '`'
	tail := false
	notEscapeSequenceHead := ""
	rawOffset := 2

	s.index++
	for !s.eof() {
		ch := s.cu(s.index)
		s.index++
		if ch == '`' {
			rawOffset = 1
			tail = true
			terminated = true
			break
		} else if ch == '$' {
			if s.cu(s.index) == '{' {
				s.curlyStack = append(s.curlyStack, "${")
				s.index++
				terminated = true
				break
			}
			cooked = append(cooked, '$')
		} else if notEscapeSequenceHead != "" {
			continue
		} else if ch == '\\' {
			ch = s.cu(s.index)
			s.index++
			if ch < 0 {
				internalError("Cannot read properties of undefined (reading 'charCodeAt')")
			}
			if !isLineTerminator(ch) {
				switch ch {
				case 'n':
					cooked = append(cooked, '\n')
				case 'r':
					cooked = append(cooked, '\r')
				case 't':
					cooked = append(cooked, '\t')
				case 'u':
					if s.cu(s.index) == '{' {
						s.index++
						if esc, ok := s.tryToScanUnicodeCodePointEscape(); ok {
							cooked = append(cooked, esc...)
						} else {
							notEscapeSequenceHead = "u"
						}
					} else {
						if esc, ok := s.scanHexEscape('u'); ok {
							cooked = append(cooked, esc...)
						} else {
							notEscapeSequenceHead = "u"
						}
					}
				case 'x':
					if esc, ok := s.scanHexEscape('x'); ok {
						cooked = append(cooked, esc...)
					} else {
						notEscapeSequenceHead = "x"
					}
				case 'b':
					cooked = append(cooked, '\b')
				case 'f':
					cooked = append(cooked, '\f')
				case 'v':
					cooked = append(cooked, '\v')
				default:
					if ch == '0' {
						if isDecimalDigit(s.cu(s.index)) {
							// NotEscapeSequence: \01 \02 and so on
							notEscapeSequenceHead = "0"
						} else {
							cooked = append(cooked, 0)
						}
					} else if isDecimalDigit(ch) {
						// NotEscapeSequence: \1 \2
						notEscapeSequenceHead = string(rune(ch))
					} else {
						cooked = appendUnit(cooked, uint16(ch))
					}
				}
			} else {
				s.lineNumber++
				if ch == '\r' && s.cu(s.index) == '\n' {
					s.index++
				}
				s.lineStart = s.index
			}
		} else if isLineTerminator(ch) {
			s.lineNumber++
			if ch == '\r' && s.cu(s.index) == '\n' {
				s.index++
			}
			s.lineStart = s.index
			cooked = append(cooked, '\n')
		} else {
			cooked = appendUnit(cooked, uint16(ch))
		}
	}

	if !terminated {
		s.throwUnexpectedToken(msgUnexpectedTokenIllegal)
	}
	if !head && len(s.curlyStack) > 0 {
		s.curlyStack = s.curlyStack[:len(s.curlyStack)-1]
	}

	return &rawToken{
		typ:                   tokTemplate,
		str:                   s.slice(start+1, s.index-rawOffset),
		cooked:                string(cooked),
		cookedNull:            notEscapeSequenceHead != "",
		head:                  head,
		tail:                  tail,
		notEscapeSequenceHead: notEscapeSequenceHead,
		lineNumber:            s.lineNumber,
		lineStart:             s.lineStart,
		start:                 start,
		end:                   s.index,
	}
}

// https://tc39.github.io/ecma262/#sec-literals-regular-expression-literals

// testRegExp checks a regular expression literal the way the original does
// on its host engine: the pattern (with the code point escapes and surrogate
// pairs of a u-flagged pattern replaced by BMP stand-ins) must be a valid
// pattern without flags, or the literal is an error; the value is then the
// expression the engine builds for the pattern and flags, or nil when it
// rejects them.
func (s *scanner) testRegExp(pattern, flags string) *RegExp {
	tmp := Units(pattern)
	if strings.Contains(flags, "u") {
		tmp = s.replaceUnicodeEscapes(tmp)
		tmp = replaceSurrogatePairs(tmp)
	}
	if !validRegExp(tmp, "") {
		s.throwUnexpectedToken(msgInvalidRegExp)
	}
	if !validRegExp(Units(pattern), flags) {
		return nil
	}
	return &RegExp{Pattern: pattern, Flags: flags}
}

// replaceUnicodeEscapes replaces every \u{...} and \uXXXX sequence, scanning
// left to right, with the code unit it denotes or U+FFFF for an astral code
// point; a code point above U+10FFFF is an invalid regular expression.
func (s *scanner) replaceUnicodeEscapes(p []uint16) []uint16 {
	out := make([]uint16, 0, len(p))
	for i := 0; i < len(p); {
		if p[i] == '\\' && i+1 < len(p) && p[i+1] == 'u' {
			// \u{hex+}
			if i+2 < len(p) && p[i+2] == '{' {
				j := i + 3
				for j < len(p) && isHexDigit(int(p[j])) {
					j++
				}
				if j > i+3 && j < len(p) && p[j] == '}' {
					out = append(out, s.escapeStandIn(p[i+3:j])...)
					i = j + 1
					continue
				}
			}
			// \uXXXX
			if i+5 < len(p)+0 && i+6 <= len(p) {
				ok := true
				for k := i + 2; k < i+6; k++ {
					if !isHexDigit(int(p[k])) {
						ok = false
						break
					}
				}
				if ok {
					out = append(out, s.escapeStandIn(p[i+2:i+6])...)
					i += 6
					continue
				}
			}
		}
		out = append(out, p[i])
		i++
	}
	return out
}

func (s *scanner) escapeStandIn(hex []uint16) []uint16 {
	digits := make([]byte, len(hex))
	for i, u := range hex {
		digits[i] = byte(u)
	}
	cp := parseIntDigits(string(digits), 16)
	if cp > 0x10FFFF {
		s.throwUnexpectedToken(msgInvalidRegExp)
	}
	if cp <= 0xFFFF {
		return []uint16{uint16(cp)}
	}
	return []uint16{0xFFFF}
}

// replaceSurrogatePairs replaces each surrogate pair with U+FFFF.
func replaceSurrogatePairs(p []uint16) []uint16 {
	out := make([]uint16, 0, len(p))
	for i := 0; i < len(p); i++ {
		if p[i] >= 0xD800 && p[i] <= 0xDBFF && i+1 < len(p) && p[i+1] >= 0xDC00 && p[i+1] <= 0xDFFF {
			out = append(out, 0xFFFF)
			i++
			continue
		}
		out = append(out, p[i])
	}
	return out
}

func (s *scanner) scanRegExpBody() string {
	start := s.index
	s.index++ // the opening slash
	classMarker := false
	terminated := false

	for !s.eof() {
		ch := s.cu(s.index)
		s.index++
		if ch == '\\' {
			ch = s.cu(s.index)
			s.index++
			if ch < 0 {
				internalError("Cannot read properties of undefined (reading 'charCodeAt')")
			}
			// https://tc39.github.io/ecma262/#sec-literals-regular-expression-literals
			if isLineTerminator(ch) {
				s.throwUnexpectedToken(msgUnterminatedRegExp)
			}
		} else if isLineTerminator(ch) {
			s.throwUnexpectedToken(msgUnterminatedRegExp)
		} else if classMarker {
			if ch == ']' {
				classMarker = false
			}
		} else if ch == '/' {
			terminated = true
			break
		} else if ch == '[' {
			classMarker = true
		}
	}

	if !terminated {
		s.throwUnexpectedToken(msgUnterminatedRegExp)
	}
	// Exclude the leading and trailing slash.
	return s.slice(start+1, s.index-1)
}

func (s *scanner) scanRegExpFlags() string {
	var flags []byte
	for !s.eof() {
		ch := s.cu(s.index)
		if !isIdentifierPart(ch) {
			break
		}
		s.index++
		if ch == '\\' && !s.eof() {
			if s.cu(s.index) == 'u' {
				s.index++
				restore := s.index
				if char, ok := s.scanHexEscape('u'); ok {
					flags = append(flags, char...)
				} else {
					s.index = restore
					flags = append(flags, 'u')
				}
				s.tolerateUnexpectedToken(msgUnexpectedTokenIllegal)
			} else {
				s.tolerateUnexpectedToken(msgUnexpectedTokenIllegal)
			}
		} else {
			flags = appendUnit(flags, uint16(ch))
		}
	}
	return string(flags)
}

func (s *scanner) scanRegExp() *rawToken {
	start := s.index
	pattern := s.scanRegExpBody()
	flags := s.scanRegExpFlags()
	value := s.testRegExp(pattern, flags)
	return &rawToken{typ: tokRegularExpression, pattern: pattern, flags: flags, regex: value,
		lineNumber: s.lineNumber, lineStart: s.lineStart, start: start, end: s.index}
}

func (s *scanner) lex() *rawToken {
	if s.eof() {
		return &rawToken{typ: tokEOF, lineNumber: s.lineNumber, lineStart: s.lineStart, start: s.index, end: s.index}
	}

	cp := s.cu(s.index)
	if isIdentifierStart(cp) {
		return s.scanIdentifier()
	}
	// Very common: ( and ) and ;
	if cp == 0x28 || cp == 0x29 || cp == 0x3B {
		return s.scanPunctuator()
	}
	// String literals start with a single or a double quote.
	if cp == 0x27 || cp == 0x22 {
		return s.scanStringLiteral()
	}
	// A dot can also start a floating-point number.
	if cp == 0x2E {
		if isDecimalDigit(s.cu(s.index + 1)) {
			return s.scanNumericLiteral()
		}
		return s.scanPunctuator()
	}
	if isDecimalDigit(cp) {
		return s.scanNumericLiteral()
	}
	// Template literals start with ` for a head, or } for a middle or tail.
	if cp == 0x60 || (cp == 0x7D && len(s.curlyStack) > 0 && s.curlyStack[len(s.curlyStack)-1] == "${") {
		return s.scanTemplate()
	}
	// Possible identifier start in a surrogate pair.
	if cp >= 0xD800 && cp < 0xDFFF && isIdentifierStart(s.codePointAt(s.index)) {
		return s.scanIdentifier()
	}
	return s.scanPunctuator()
}
