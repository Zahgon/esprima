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

// Go port of src/parser.ts.

package esprima

import (
	"regexp"
	"strconv"
	"strings"
)

type parserConfig struct {
	rng       bool
	loc       bool
	source    string
	hasSource bool
	tokens    bool
	comment   bool
	tolerant  bool
}

type parserContext struct {
	isModule                       bool
	allowIn                        bool
	allowStrictDirective           bool
	allowYield                     bool
	isAsync                        bool
	firstCoverInitializedNameError *rawToken
	isAssignmentTarget             bool
	isBindingElement               bool
	inFunctionBody                 bool
	inIteration                    bool
	inSwitch                       bool
	inClassConstructor             bool
	labelSet                       map[string]bool
	strict                         bool
}

type marker struct {
	index  int
	line   int
	column int
}

// paramInfo collects what parsing a parameter list learns: the parameters,
// whether the list is simple, and the first parameter (a token or an
// Identifier node) that breaks a strict mode rule together with its message.
type paramInfo struct {
	simple                     bool
	hasDuplicateParameterNames bool
	params                     []any
	paramSet                   map[string]bool
	firstRestricted            any
	stricted                   any
	message                    string
}

const arrowParameterPlaceHolder = "ArrowParameterPlaceHolder"

// parser is a recursive-descent ECMAScript parser. A failure that the
// original throws is raised as a panic carrying an *Error and recovered by
// the public entry points.
type parser struct {
	config       parserConfig
	delegate     Delegate
	errorHandler *errorHandler
	scanner      *scanner
	jsx          bool

	lookahead         *rawToken
	hasLineTerminator bool

	context     parserContext
	tokens      []any
	startMarker marker
	lastMarker  marker
}

var operatorPrecedence = map[string]int{
	")": 0, ";": 0, ",": 0, "=": 0, "]": 0,
	"??": 5, "||": 6, "&&": 7, "|": 8, "^": 9, "&": 10,
	"==": 11, "!=": 11, "===": 11, "!==": 11,
	"<": 12, ">": 12, "<=": 12, ">=": 12,
	"<<": 13, ">>": 13, ">>>": 13,
	"+": 14, "-": 14,
	"*": 15, "/": 15, "%": 15,
}

func newParser(code []uint16, opts *Options, delegate Delegate, jsx bool) *parser {
	p := &parser{delegate: delegate, jsx: jsx}
	if opts != nil {
		p.config = parserConfig{rng: opts.Range, loc: opts.Loc, tokens: opts.Tokens, comment: opts.Comment, tolerant: opts.Tolerant}
		if p.config.loc && opts.Source != "" {
			p.config.source = opts.Source
			p.config.hasSource = true
		}
	}
	p.errorHandler = &errorHandler{tolerant: p.config.tolerant}
	p.scanner = newScanner(code, p.errorHandler)
	p.scanner.trackComment = p.config.comment

	p.lookahead = &rawToken{typ: tokEOF, lineNumber: p.scanner.lineNumber}
	p.context = parserContext{
		allowIn:              true,
		allowStrictDirective: true,
		allowYield:           true,
		labelSet:             map[string]bool{},
	}
	p.startMarker = marker{line: p.scanner.lineNumber}
	p.lastMarker = marker{line: p.scanner.lineNumber}
	p.nextToken()
	p.lastMarker = marker{index: p.scanner.index, line: p.scanner.lineNumber, column: p.scanner.index - p.scanner.lineStart}
	return p
}

// jsString converts a value the way JavaScript's String() does for the
// values a message can be formatted with.
func jsString(v any) string {
	switch x := v.(type) {
	case nil:
		return "undefined"
	case string:
		return x
	case float64:
		return NumberToString(x)
	case bool:
		return strconv.FormatBool(x)
	}
	return "[object Object]"
}

var placeholder = regexp.MustCompile(`%(\d)`)

// formatMessage replaces every %N placeholder with the N-th value.
func formatMessage(format string, values []any) string {
	return placeholder.ReplaceAllStringFunc(format, func(m string) string {
		idx := int(m[1] - '0')
		if idx >= len(values) {
			internalError("ASSERT: Message reference must be in range")
		}
		return jsString(values[idx])
	})
}

// replaceFirst is String.prototype.replace with a string pattern and a
// replacement string: only the first occurrence is replaced and the
// replacement's $ patterns are expanded.
func replaceFirst(s, pattern, replacement string) string {
	idx := strings.Index(s, pattern)
	if idx < 0 {
		return s
	}
	var b strings.Builder
	b.WriteString(s[:idx])
	for i := 0; i < len(replacement); i++ {
		c := replacement[i]
		if c == '$' && i+1 < len(replacement) {
			switch replacement[i+1] {
			case '$':
				b.WriteByte('$')
				i++
				continue
			case '&':
				b.WriteString(pattern)
				i++
				continue
			case '`':
				b.WriteString(s[:idx])
				i++
				continue
			case '\'':
				b.WriteString(s[idx+len(pattern):])
				i++
				continue
			}
		}
		b.WriteByte(c)
	}
	b.WriteString(s[idx+len(pattern):])
	return b.String()
}

func (p *parser) throwError(format string, values ...any) {
	msg := formatMessage(format, values)
	panic(p.errorHandler.createError(p.lastMarker.index, p.lastMarker.line, p.lastMarker.column+1, msg))
}

func (p *parser) tolerateError(format string, values ...any) {
	msg := formatMessage(format, values)
	p.errorHandler.tolerateError(p.lastMarker.index, p.scanner.lineNumber, p.lastMarker.column+1, msg)
}

// unexpectedTokenError builds the error for an unexpected token. The token
// is a *rawToken, an Identifier *Node standing for a parameter, or nil; an
// empty message selects one from the token kind.
func (p *parser) unexpectedTokenError(token any, message string) *Error {
	msg := message
	if msg == "" {
		msg = msgUnexpectedToken
	}
	var value string
	tok, isToken := token.(*rawToken)
	if isToken && tok == nil {
		token = nil
	}
	if token != nil {
		if isToken {
			if message == "" {
				switch tok.typ {
				case tokEOF:
					msg = msgUnexpectedEOS
				case tokIdentifier:
					msg = msgUnexpectedIdentifier
				case tokNumericLiteral:
					msg = msgUnexpectedNumber
				case tokStringLiteral:
					msg = msgUnexpectedString
				case tokTemplate:
					msg = msgUnexpectedTemplate
				default:
					msg = msgUnexpectedToken
				}
				if tok.typ == tokKeyword {
					if isFutureReservedWord(tok.str) {
						msg = msgUnexpectedReserved
					} else if p.context.strict && isStrictModeReservedWord(tok.str) {
						msg = msgStrictReservedWord
					}
				}
			}
			value = jsString(tok.value())
		} else {
			// A node has no value property.
			value = "undefined"
		}
	} else {
		value = "ILLEGAL"
	}

	msg = replaceFirst(msg, "%0", value)

	if isToken && tok != nil {
		lastMarkerLineStart := p.lastMarker.index - p.lastMarker.column
		column := tok.start - lastMarkerLineStart + 1
		return p.errorHandler.createError(tok.start, tok.lineNumber, column, msg)
	}
	return p.errorHandler.createError(p.lastMarker.index, p.lastMarker.line, p.lastMarker.column+1, msg)
}

func (p *parser) throwUnexpectedToken(token any, message string) {
	panic(p.unexpectedTokenError(token, message))
}

func (p *parser) tolerateUnexpectedToken(token any, message string) {
	p.errorHandler.tolerate(p.unexpectedTokenError(token, message))
}

func (p *parser) tolerateInvalidLoopStatement() {
	if p.matchKeyword("class") || p.matchKeyword("function") {
		p.tolerateError(msgUnexpectedToken, p.lookahead)
	}
}

func (p *parser) collectComments() {
	if !p.config.comment {
		p.scanner.scanComments()
		return
	}
	comments := p.scanner.scanComments()
	if len(comments) == 0 || p.delegate == nil {
		return
	}
	for _, e := range comments {
		typ := "LineComment"
		if e.multiLine {
			typ = "BlockComment"
		}
		node := newNode("", "type", typ, "value", p.scanner.slice(e.slice[0], e.slice[1]))
		if p.config.rng {
			node.Set("range", []any{e.rng[0], e.rng[1]})
		}
		if p.config.loc {
			node.Set("loc", locNode(e.loc.startLine, e.loc.startColumn, e.loc.endLine, e.loc.endColumn))
		}
		p.delegate(node, Metadata{
			Start: Position{Line: e.loc.startLine, Column: e.loc.startColumn, Offset: e.rng[0]},
			End:   Position{Line: e.loc.endLine, Column: e.loc.endColumn, Offset: e.rng[1]},
		})
	}
}

func locNode(startLine, startColumn, endLine, endColumn int) *Node {
	return newNode("",
		"start", newNode("", "line", startLine, "column", startColumn),
		"end", newNode("", "line", endLine, "column", endColumn))
}

func (p *parser) getTokenRaw(token *rawToken) string {
	return p.scanner.slice(token.start, token.end)
}

// convertToken turns a scanned token into its entry in the token list.
func (p *parser) convertToken(token *rawToken) *Node {
	t := newNode("", "type", tokenName(token.typ), "value", p.getTokenRaw(token))
	if p.config.rng {
		t.Set("range", []any{token.start, token.end})
	}
	if p.config.loc {
		t.Set("loc", locNode(p.startMarker.line, p.startMarker.column, p.scanner.lineNumber, p.scanner.index-p.scanner.lineStart))
	}
	if token.typ == tokRegularExpression {
		t.Set("regex", newNode("", "pattern", token.pattern, "flags", token.flags))
	}
	return t
}

func (p *parser) nextToken() *rawToken {
	token := p.lookahead

	p.lastMarker.index = p.scanner.index
	p.lastMarker.line = p.scanner.lineNumber
	p.lastMarker.column = p.scanner.index - p.scanner.lineStart

	p.collectComments()

	if p.scanner.index != p.startMarker.index {
		p.startMarker.index = p.scanner.index
		p.startMarker.line = p.scanner.lineNumber
		p.startMarker.column = p.scanner.index - p.scanner.lineStart
	}

	next := p.scanner.lex()
	p.hasLineTerminator = token.lineNumber != next.lineNumber

	if p.context.strict && next.typ == tokIdentifier && isStrictModeReservedWord(next.str) {
		next.typ = tokKeyword
	}
	p.lookahead = next

	if p.config.tokens && next.typ != tokEOF {
		p.tokens = append(p.tokens, p.convertToken(next))
	}
	return token
}

func (p *parser) popToken() {
	if len(p.tokens) > 0 {
		p.tokens = p.tokens[:len(p.tokens)-1]
	}
}

func (p *parser) nextRegexToken() *rawToken {
	p.collectComments()

	token := p.scanner.scanRegExp()
	if p.config.tokens {
		// Pop the previous token, '/' or '/=', added from the lookahead.
		p.popToken()
		p.tokens = append(p.tokens, p.convertToken(token))
	}

	// Prime the next lookahead.
	p.lookahead = token
	p.nextToken()
	return token
}

func (p *parser) createNode() marker {
	return p.startMarker
}

func (p *parser) startNode(token *rawToken, lastLineStart int) marker {
	column := token.start - token.lineStart
	line := token.lineNumber
	if column < 0 {
		column += lastLineStart
		line--
	}
	return marker{index: token.start, line: line, column: column}
}

func (p *parser) finalize(m marker, node *Node) *Node {
	if p.config.rng {
		node.Set("range", []any{m.index, p.lastMarker.index})
	}
	if p.config.loc {
		loc := locNode(m.line, m.column, p.lastMarker.line, p.lastMarker.column)
		if p.config.hasSource {
			loc.Set("source", p.config.source)
		}
		node.Set("loc", loc)
	}
	if p.delegate != nil {
		p.delegate(node, Metadata{
			Start: Position{Line: m.line, Column: m.column, Offset: m.index},
			End:   Position{Line: p.lastMarker.line, Column: p.lastMarker.column, Offset: p.lastMarker.index},
		})
	}
	return node
}

// expect consumes the next token, which must be the given punctuator.
func (p *parser) expect(value string) {
	token := p.nextToken()
	if token.typ != tokPunctuator || token.str != value {
		p.throwUnexpectedToken(token, "")
	}
}

// expectCommaSeparator quietly expects a comma in tolerant mode.
func (p *parser) expectCommaSeparator() {
	if !p.config.tolerant {
		p.expect(",")
		return
	}
	token := p.lookahead
	if token.typ == tokPunctuator && token.str == "," {
		p.nextToken()
	} else if token.typ == tokPunctuator && token.str == ";" {
		p.nextToken()
		p.tolerateUnexpectedToken(token, "")
	} else {
		p.tolerateUnexpectedToken(token, msgUnexpectedToken)
	}
}

func (p *parser) expectKeyword(keyword string) {
	token := p.nextToken()
	if token.typ != tokKeyword || token.str != keyword {
		p.throwUnexpectedToken(token, "")
	}
}

func (p *parser) match(value string) bool {
	return p.lookahead.typ == tokPunctuator && p.lookahead.str == value
}

func (p *parser) matchKeyword(keyword string) bool {
	return p.lookahead.typ == tokKeyword && p.lookahead.str == keyword
}

// matchContextualKeyword matches an identifier that is a keyword only in
// some contexts.
func (p *parser) matchContextualKeyword(keyword string) bool {
	return p.lookahead.typ == tokIdentifier && p.lookahead.str == keyword
}

func (p *parser) matchAssign() bool {
	if p.lookahead.typ != tokPunctuator {
		return false
	}
	switch p.lookahead.str {
	case "=", "*=", "**=", "/=", "%=", "+=", "-=", "<<=", ">>=", ">>>=", "&=", "^=", "|=":
		return true
	}
	return false
}

// Cover grammar support: isolateCoverGrammar runs a parse function in a
// fresh cover grammar context and reports a pending CoverInitializedName
// error; inheritCoverGrammar propagates the flags to the caller.

func (p *parser) isolateCoverGrammar(parse func() *Node) *Node {
	prevBinding := p.context.isBindingElement
	prevTarget := p.context.isAssignmentTarget
	prevCover := p.context.firstCoverInitializedNameError

	p.context.isBindingElement = true
	p.context.isAssignmentTarget = true
	p.context.firstCoverInitializedNameError = nil

	result := parse()
	if p.context.firstCoverInitializedNameError != nil {
		p.throwUnexpectedToken(p.context.firstCoverInitializedNameError, "")
	}

	p.context.isBindingElement = prevBinding
	p.context.isAssignmentTarget = prevTarget
	p.context.firstCoverInitializedNameError = prevCover
	return result
}

func (p *parser) inheritCoverGrammar(parse func() *Node) *Node {
	prevBinding := p.context.isBindingElement
	prevTarget := p.context.isAssignmentTarget
	prevCover := p.context.firstCoverInitializedNameError

	p.context.isBindingElement = true
	p.context.isAssignmentTarget = true
	p.context.firstCoverInitializedNameError = nil

	result := parse()

	p.context.isBindingElement = p.context.isBindingElement && prevBinding
	p.context.isAssignmentTarget = p.context.isAssignmentTarget && prevTarget
	if prevCover != nil {
		p.context.firstCoverInitializedNameError = prevCover
	}
	return result
}

func (p *parser) consumeSemicolon() {
	if p.match(";") {
		p.nextToken()
	} else if !p.hasLineTerminator {
		if p.lookahead.typ != tokEOF && !p.match("}") {
			p.throwUnexpectedToken(p.lookahead, "")
		}
		p.lastMarker = p.startMarker
	}
}

// https://tc39.github.io/ecma262/#sec-primary-expression

func (p *parser) parsePrimaryExpression() *Node {
	if p.jsx && p.match("<") {
		return p.parseJSXRoot()
	}
	node := p.createNode()

	var expr *Node
	switch p.lookahead.typ {
	case tokIdentifier:
		if (p.context.isModule || p.context.isAsync) && p.lookahead.str == "await" {
			p.tolerateUnexpectedToken(p.lookahead, "")
		}
		if p.matchAsyncFunction() {
			expr = p.parseFunctionExpression()
		} else {
			expr = p.finalize(node, nIdentifier(p.nextToken().str))
		}

	case tokNumericLiteral, tokStringLiteral:
		if p.context.strict && p.lookahead.octal {
			p.tolerateUnexpectedToken(p.lookahead, msgStrictOctalLiteral)
		}
		p.context.isAssignmentTarget = false
		p.context.isBindingElement = false
		token := p.nextToken()
		expr = p.finalize(node, nLiteral(token.value(), p.getTokenRaw(token)))

	case tokBooleanLiteral:
		p.context.isAssignmentTarget = false
		p.context.isBindingElement = false
		token := p.nextToken()
		expr = p.finalize(node, nLiteral(token.str == "true", p.getTokenRaw(token)))

	case tokNullLiteral:
		p.context.isAssignmentTarget = false
		p.context.isBindingElement = false
		token := p.nextToken()
		expr = p.finalize(node, nLiteral(nil, p.getTokenRaw(token)))

	case tokTemplate:
		expr = p.parseTemplateLiteral(false)

	case tokPunctuator:
		switch p.lookahead.str {
		case "(":
			p.context.isBindingElement = false
			expr = p.inheritCoverGrammar(p.parseGroupExpression)
		case "[":
			expr = p.inheritCoverGrammar(p.parseArrayInitializer)
		case "{":
			expr = p.inheritCoverGrammar(p.parseObjectInitializer)
		case "/", "/=":
			p.context.isAssignmentTarget = false
			p.context.isBindingElement = false
			p.scanner.index = p.startMarker.index
			token := p.nextRegexToken()
			expr = p.finalize(node, nRegexLiteral(token.regex, p.getTokenRaw(token), token.pattern, token.flags))
		default:
			p.throwUnexpectedToken(p.nextToken(), "")
		}

	case tokKeyword:
		if !p.context.strict && p.context.allowYield && p.matchKeyword("yield") {
			expr = p.parseIdentifierName()
		} else if !p.context.strict && p.matchKeyword("let") {
			expr = p.finalize(node, nIdentifier(p.nextToken().str))
		} else {
			p.context.isAssignmentTarget = false
			p.context.isBindingElement = false
			switch {
			case p.matchKeyword("function"):
				expr = p.parseFunctionExpression()
			case p.matchKeyword("this"):
				p.nextToken()
				expr = p.finalize(node, nThisExpression())
			case p.matchKeyword("class"):
				expr = p.parseClassExpression()
			case p.matchImportCall():
				expr = p.parseImportCall()
			case p.matchImportMeta():
				if !p.context.isModule {
					p.tolerateUnexpectedToken(p.lookahead, msgCannotUseImportMetaOutsideAModule)
				}
				expr = p.parseImportMeta()
			default:
				p.throwUnexpectedToken(p.nextToken(), "")
			}
		}

	default:
		p.throwUnexpectedToken(p.nextToken(), "")
	}
	return expr
}

// https://tc39.github.io/ecma262/#sec-array-initializer

func (p *parser) parseSpreadElement() *Node {
	node := p.createNode()
	p.expect("...")
	arg := p.inheritCoverGrammar(p.parseAssignmentExpression)
	return p.finalize(node, nSpreadElement(arg))
}

func (p *parser) parseArrayInitializer() *Node {
	node := p.createNode()
	elements := []any{}

	p.expect("[")
	for !p.match("]") {
		if p.match(",") {
			p.nextToken()
			elements = append(elements, nil)
		} else if p.match("...") {
			element := p.parseSpreadElement()
			if !p.match("]") {
				p.context.isAssignmentTarget = false
				p.context.isBindingElement = false
				p.expect(",")
			}
			elements = append(elements, element)
		} else {
			elements = append(elements, p.inheritCoverGrammar(p.parseAssignmentExpression))
			if !p.match("]") {
				p.expect(",")
			}
		}
	}
	p.expect("]")
	return p.finalize(node, nArrayExpression(elements))
}

// https://tc39.github.io/ecma262/#sec-object-initializer

func (p *parser) parsePropertyMethod(params *paramInfo) *Node {
	p.context.isAssignmentTarget = false
	p.context.isBindingElement = false

	prevStrict := p.context.strict
	prevAllowStrictDirective := p.context.allowStrictDirective
	p.context.allowStrictDirective = params.simple
	body := p.isolateCoverGrammar(p.parseFunctionSourceElements)
	if p.context.strict && params.firstRestricted != nil {
		p.tolerateUnexpectedToken(params.firstRestricted, params.message)
	}
	if p.context.strict && params.stricted != nil {
		p.tolerateUnexpectedToken(params.stricted, params.message)
	}
	p.context.strict = prevStrict
	p.context.allowStrictDirective = prevAllowStrictDirective
	return body
}

func (p *parser) parsePropertyMethodFunction(isGenerator bool) *Node {
	node := p.createNode()

	prevAllowYield := p.context.allowYield
	p.context.allowYield = true
	params := p.parseFormalParameters(nil)
	method := p.parsePropertyMethod(params)
	p.context.allowYield = prevAllowYield

	return p.finalize(node, nFunction("FunctionExpression", nil, params.params, method, isGenerator, false))
}

func (p *parser) parsePropertyMethodAsyncFunction(isGenerator bool) *Node {
	node := p.createNode()

	prevAllowYield := p.context.allowYield
	prevIsAsync := p.context.isAsync
	p.context.allowYield = false
	p.context.isAsync = true

	params := p.parseFormalParameters(nil)
	method := p.parsePropertyMethod(params)
	p.context.allowYield = prevAllowYield
	p.context.isAsync = prevIsAsync

	return p.finalize(node, nFunction("FunctionExpression", nil, params.params, method, isGenerator, true))
}

func (p *parser) parseObjectPropertyKey() *Node {
	node := p.createNode()
	token := p.nextToken()

	var key *Node
	switch token.typ {
	case tokStringLiteral, tokNumericLiteral:
		if p.context.strict && token.octal {
			p.tolerateUnexpectedToken(token, msgStrictOctalLiteral)
		}
		key = p.finalize(node, nLiteral(token.value(), p.getTokenRaw(token)))
	case tokIdentifier, tokBooleanLiteral, tokNullLiteral, tokKeyword:
		key = p.finalize(node, nIdentifier(token.str))
	case tokPunctuator:
		if token.str == "[" {
			key = p.isolateCoverGrammar(p.parseAssignmentExpression)
			p.expect("]")
		} else {
			p.throwUnexpectedToken(token, "")
		}
	default:
		p.throwUnexpectedToken(token, "")
	}
	return key
}

func isPropertyKey(key *Node, value string) bool {
	switch key.Type() {
	case "Identifier":
		return key.str("name") == value
	case "Literal":
		s, ok := key.Get("value").(string)
		return ok && s == value
	}
	return false
}

func (p *parser) parseObjectProperty(hasProto *bool) *Node {
	node := p.createNode()
	token := p.lookahead

	var kind string
	var key, value *Node
	computed, method, shorthand, isAsync, isGenerator := false, false, false, false, false

	if token.typ == tokIdentifier {
		id := token.str
		p.nextToken()
		computed = p.match("[")
		isAsync = !p.hasLineTerminator && id == "async" && !p.match(":") && !p.match("(") && !p.match(",")
		isGenerator = p.match("*")
		if isGenerator {
			p.nextToken()
		}
		if isAsync {
			key = p.parseObjectPropertyKey()
		} else {
			key = p.finalize(node, nIdentifier(id))
		}
	} else if p.match("*") {
		p.nextToken()
	} else {
		computed = p.match("[")
		key = p.parseObjectPropertyKey()
	}

	lookaheadPropertyKey := p.qualifiedPropertyName(p.lookahead)
	switch {
	case token.typ == tokIdentifier && !isAsync && token.str == "get" && lookaheadPropertyKey:
		kind = "get"
		computed = p.match("[")
		key = p.parseObjectPropertyKey()
		p.context.allowYield = false
		value = p.parseGetterMethod()

	case token.typ == tokIdentifier && !isAsync && token.str == "set" && lookaheadPropertyKey:
		kind = "set"
		computed = p.match("[")
		key = p.parseObjectPropertyKey()
		value = p.parseSetterMethod()

	case token.typ == tokPunctuator && token.str == "*" && lookaheadPropertyKey:
		kind = "init"
		computed = p.match("[")
		key = p.parseObjectPropertyKey()
		value = p.parseGeneratorMethod()
		method = true

	default:
		if key == nil {
			p.throwUnexpectedToken(p.lookahead, "")
		}
		kind = "init"
		if p.match(":") && !isAsync {
			if !computed && isPropertyKey(key, "__proto__") {
				if *hasProto {
					p.tolerateError(msgDuplicateProtoProperty)
				}
				*hasProto = true
			}
			p.nextToken()
			value = p.inheritCoverGrammar(p.parseAssignmentExpression)
		} else if p.match("(") {
			if isAsync {
				value = p.parsePropertyMethodAsyncFunction(isGenerator)
			} else {
				value = p.parsePropertyMethodFunction(isGenerator)
			}
			method = true
		} else if token.typ == tokIdentifier {
			id := p.finalize(node, nIdentifier(token.str))
			if p.match("=") {
				p.context.firstCoverInitializedNameError = p.lookahead
				p.nextToken()
				shorthand = true
				init := p.isolateCoverGrammar(p.parseAssignmentExpression)
				value = p.finalize(node, nAssignmentPattern(id, init))
			} else {
				shorthand = true
				value = id
			}
		} else {
			p.throwUnexpectedToken(p.nextToken(), "")
		}
	}

	return p.finalize(node, nProperty(kind, key, computed, value, method, shorthand))
}

func (p *parser) parseObjectInitializer() *Node {
	node := p.createNode()

	p.expect("{")
	properties := []any{}
	hasProto := false
	for !p.match("}") {
		if p.match("...") {
			properties = append(properties, p.parseSpreadElement())
		} else {
			properties = append(properties, p.parseObjectProperty(&hasProto))
		}
		if !p.match("}") {
			p.expectCommaSeparator()
		}
	}
	p.expect("}")
	return p.finalize(node, nObjectExpression(properties))
}

// https://tc39.es/proposal-template-literal-revision/#sec-static-semantics-template-early-errors
func (p *parser) throwTemplateLiteralEarlyErrors(token *rawToken) {
	switch token.notEscapeSequenceHead {
	case "u":
		p.throwUnexpectedToken(token, msgInvalidUnicodeEscapeSequence)
	case "x":
		p.throwUnexpectedToken(token, msgInvalidHexEscapeSequence)
	case "8", "9":
		p.throwUnexpectedToken(token, msgTemplateEscape89)
	default: // 0-7
		p.throwUnexpectedToken(token, msgTemplateOctalLiteral)
	}
}

// https://tc39.github.io/ecma262/#sec-template-literals

func (p *parser) templateElement(token *rawToken) *Node {
	var cooked any = token.cooked
	if token.cookedNull {
		cooked = nil
	}
	return nTemplateElement(token.str, cooked, token.tail)
}

func (p *parser) parseTemplateHead(isTagged bool) *Node {
	if !p.lookahead.head {
		internalError("ASSERT: Template literal must start with a template head")
	}
	node := p.createNode()
	token := p.nextToken()
	if !isTagged && token.notEscapeSequenceHead != "" {
		p.throwTemplateLiteralEarlyErrors(token)
	}
	return p.finalize(node, p.templateElement(token))
}

func (p *parser) parseTemplateElement(isTagged bool) *Node {
	if p.lookahead.typ != tokTemplate {
		p.throwUnexpectedToken(nil, "")
	}
	node := p.createNode()
	token := p.nextToken()
	if !isTagged && token.notEscapeSequenceHead != "" {
		p.throwTemplateLiteralEarlyErrors(token)
	}
	return p.finalize(node, p.templateElement(token))
}

func (p *parser) parseTemplateLiteral(isTagged bool) *Node {
	node := p.createNode()

	expressions := []any{}
	quasis := []any{}

	quasi := p.parseTemplateHead(isTagged)
	quasis = append(quasis, quasi)
	for !quasi.Get("tail").(bool) {
		expressions = append(expressions, p.parseExpression())
		quasi = p.parseTemplateElement(isTagged)
		quasis = append(quasis, quasi)
	}
	return p.finalize(node, nTemplateLiteral(quasis, expressions))
}

// https://tc39.github.io/ecma262/#sec-grouping-operator

func (p *parser) reinterpretExpressionAsPattern(expr *Node) {
	switch expr.Type() {
	case "Identifier", "MemberExpression", "RestElement", "AssignmentPattern":
	case "SpreadElement":
		expr.Set("type", "RestElement")
		p.reinterpretExpressionAsPattern(expr.node("argument"))
	case "ArrayExpression":
		expr.Set("type", "ArrayPattern")
		for _, el := range expr.list("elements") {
			if el != nil {
				p.reinterpretExpressionAsPattern(el.(*Node))
			}
		}
	case "ObjectExpression":
		expr.Set("type", "ObjectPattern")
		for _, prop := range expr.list("properties") {
			property := prop.(*Node)
			if property.Type() == "SpreadElement" {
				p.reinterpretExpressionAsPattern(property)
			} else {
				p.reinterpretExpressionAsPattern(property.node("value"))
			}
		}
	case "AssignmentExpression":
		expr.Set("type", "AssignmentPattern")
		expr.Delete("operator")
		p.reinterpretExpressionAsPattern(expr.node("left"))
	default:
		// Other node types are allowed for tolerant parsing.
	}
}

func placeHolder(params []any, async bool) *Node {
	return newNode("", "type", arrowParameterPlaceHolder, "params", params, "async", async)
}

func (p *parser) parseGroupExpression() *Node {
	var expr *Node

	p.expect("(")
	if p.match(")") {
		p.nextToken()
		if !p.match("=>") {
			p.expect("=>")
		}
		return placeHolder([]any{}, false)
	}

	startToken := p.lookahead
	var params []any
	if p.match("...") {
		expr = p.parseRestElement(&params)
		p.expect(")")
		if !p.match("=>") {
			p.expect("=>")
		}
		return placeHolder([]any{expr}, false)
	}

	arrow := false
	p.context.isBindingElement = true
	expr = p.inheritCoverGrammar(p.parseAssignmentExpression)

	if p.match(",") {
		expressions := []any{}

		p.context.isAssignmentTarget = false
		expressions = append(expressions, expr)
		for p.lookahead.typ != tokEOF {
			if !p.match(",") {
				break
			}
			p.nextToken()
			if p.match(")") {
				p.nextToken()
				for _, e := range expressions {
					p.reinterpretExpressionAsPattern(e.(*Node))
				}
				arrow = true
				expr = placeHolder(expressions, false)
			} else if p.match("...") {
				if !p.context.isBindingElement {
					p.throwUnexpectedToken(p.lookahead, "")
				}
				expressions = append(expressions, p.parseRestElement(&params))
				p.expect(")")
				if !p.match("=>") {
					p.expect("=>")
				}
				p.context.isBindingElement = false
				for _, e := range expressions {
					p.reinterpretExpressionAsPattern(e.(*Node))
				}
				arrow = true
				expr = placeHolder(expressions, false)
			} else {
				expressions = append(expressions, p.inheritCoverGrammar(p.parseAssignmentExpression))
			}
			if arrow {
				break
			}
		}
		if !arrow {
			expr = p.finalize(p.startNode(startToken, 0), nSequenceExpression(expressions))
		}
	}

	if !arrow {
		p.expect(")")
		if p.match("=>") {
			if expr.Type() == "Identifier" && expr.str("name") == "yield" {
				arrow = true
				expr = placeHolder([]any{expr}, false)
			}
			if !arrow {
				if !p.context.isBindingElement {
					p.throwUnexpectedToken(p.lookahead, "")
				}
				var parameters []any
				if expr.Type() == "SequenceExpression" {
					for _, e := range expr.list("expressions") {
						p.reinterpretExpressionAsPattern(e.(*Node))
					}
					parameters = expr.list("expressions")
				} else {
					p.reinterpretExpressionAsPattern(expr)
					parameters = []any{expr}
				}
				expr = placeHolder(parameters, false)
			}
		}
		p.context.isBindingElement = false
	}
	return expr
}

// https://tc39.github.io/ecma262/#sec-left-hand-side-expressions

func (p *parser) parseArgumentList(parseArgument func() *Node) []any {
	p.expect("(")
	args := []any{}
	if !p.match(")") {
		for {
			var expr *Node
			if p.match("...") {
				expr = p.parseSpreadElement()
			} else {
				expr = p.isolateCoverGrammar(parseArgument)
			}
			args = append(args, expr)
			if p.match(")") {
				break
			}
			p.expectCommaSeparator()
			if p.match(")") {
				break
			}
		}
	}
	p.expect(")")
	return args
}

func (p *parser) parseArguments() []any {
	return p.parseArgumentList(p.parseAssignmentExpression)
}

func isIdentifierName(token *rawToken) bool {
	return token.typ == tokIdentifier || token.typ == tokKeyword || token.typ == tokBooleanLiteral || token.typ == tokNullLiteral
}

func (p *parser) parseIdentifierName() *Node {
	node := p.createNode()
	token := p.nextToken()
	if !isIdentifierName(token) {
		p.throwUnexpectedToken(token, "")
	}
	return p.finalize(node, nIdentifier(token.str))
}

func (p *parser) parseNewExpression() *Node {
	node := p.createNode()

	id := p.parseIdentifierName()
	if id.str("name") != "new" {
		internalError("ASSERT: New expression must start with `new`")
	}

	var expr *Node
	if p.match(".") {
		p.nextToken()
		if p.lookahead.typ == tokIdentifier && p.context.inFunctionBody && p.lookahead.str == "target" {
			property := p.parseIdentifierName()
			expr = nMetaProperty(id, property)
		} else {
			p.throwUnexpectedToken(p.lookahead, "")
		}
	} else if p.matchKeyword("import") {
		p.throwUnexpectedToken(p.lookahead, "")
	} else {
		callee := p.isolateCoverGrammar(p.parseLeftHandSideExpression)
		args := []any{}
		if p.match("(") {
			args = p.parseArguments()
		}
		expr = nNewExpression(callee, args)
		p.context.isAssignmentTarget = false
		p.context.isBindingElement = false
	}
	return p.finalize(node, expr)
}

func (p *parser) parseAsyncArgument() *Node {
	arg := p.parseAssignmentExpression()
	p.context.firstCoverInitializedNameError = nil
	return arg
}

func (p *parser) parseAsyncArguments() []any {
	return p.parseArgumentList(p.parseAsyncArgument)
}

// peekToken scans the token after the lookahead without consuming it.
func (p *parser) peekToken() *rawToken {
	state := p.scanner.saveState()
	p.scanner.scanComments()
	next := p.scanner.lex()
	p.scanner.restoreState(state)
	return next
}

func (p *parser) matchImportCall() bool {
	if !p.matchKeyword("import") {
		return false
	}
	next := p.peekToken()
	return next.typ == tokPunctuator && next.str == "("
}

func (p *parser) parseImportCall() *Node {
	node := p.createNode()
	p.expectKeyword("import")
	return p.finalize(node, nImport())
}

func (p *parser) matchImportMeta() bool {
	if !p.matchKeyword("import") {
		return false
	}
	state := p.scanner.saveState()
	p.scanner.scanComments()
	dot := p.scanner.lex()
	match := false
	if dot.typ == tokPunctuator && dot.str == "." {
		p.scanner.scanComments()
		meta := p.scanner.lex()
		match = meta.typ == tokIdentifier && meta.str == "meta"
		if match && meta.end-meta.start != len("meta") {
			p.tolerateUnexpectedToken(meta, msgInvalidEscapedReservedWord)
		}
	}
	p.scanner.restoreState(state)
	return match
}

func (p *parser) parseImportMeta() *Node {
	node := p.createNode()
	id := p.parseIdentifierName() // 'import', already ensured by matchImportMeta
	p.expect(".")
	property := p.parseIdentifierName() // 'meta', already ensured by matchImportMeta
	p.context.isAssignmentTarget = false
	return p.finalize(node, nMetaProperty(id, property))
}

func (p *parser) parseLeftHandSideExpressionAllowCall() *Node {
	startToken := p.lookahead
	maybeAsync := p.matchContextualKeyword("async")

	prevAllowIn := p.context.allowIn
	p.context.allowIn = true

	var expr *Node
	isSuper := p.matchKeyword("super")
	if isSuper && p.context.inFunctionBody {
		node := p.createNode()
		p.nextToken()
		expr = p.finalize(node, nSuper())
		if !p.match("(") && !p.match(".") && !p.match("[") {
			p.throwUnexpectedToken(p.lookahead, "")
		}
	} else if p.matchKeyword("new") {
		expr = p.inheritCoverGrammar(p.parseNewExpression)
	} else {
		expr = p.inheritCoverGrammar(p.parsePrimaryExpression)
	}

	if isSuper && p.match("(") && !p.context.inClassConstructor {
		p.tolerateError(msgUnexpectedSuper)
	}

	hasOptional := false
	for {
		optional := false
		if p.match("?.") {
			optional = true
			hasOptional = true
			p.expect("?.")
		}

		if p.match("(") {
			asyncArrow := maybeAsync && startToken.lineNumber == p.lookahead.lineNumber
			p.context.isBindingElement = false
			p.context.isAssignmentTarget = false
			var args []any
			if asyncArrow {
				args = p.parseAsyncArguments()
			} else {
				args = p.parseArguments()
			}
			if expr.Type() == "Import" && len(args) != 1 {
				p.tolerateError(msgBadImportCallArity)
			}
			expr = p.finalize(p.startNode(startToken, 0), nCallExpression(expr, args, optional))
			if asyncArrow && p.match("=>") {
				for _, a := range args {
					p.reinterpretExpressionAsPattern(a.(*Node))
				}
				expr = placeHolder(args, true)
			}
		} else if p.match("[") {
			p.context.isBindingElement = false
			p.context.isAssignmentTarget = !optional
			p.expect("[")
			property := p.isolateCoverGrammar(p.parseExpression)
			p.expect("]")
			expr = p.finalize(p.startNode(startToken, 0), nComputedMemberExpression(expr, property, optional))
		} else if p.lookahead.typ == tokTemplate && p.lookahead.head {
			// An optional template literal is not part of the specification.
			if optional {
				p.throwUnexpectedToken(p.lookahead, "")
			}
			if hasOptional {
				p.throwError(msgInvalidTaggedTemplateOnOptionalChain)
			}
			quasi := p.parseTemplateLiteral(true)
			expr = p.finalize(p.startNode(startToken, 0), nTaggedTemplateExpression(expr, quasi))
		} else if p.match(".") || optional {
			p.context.isBindingElement = false
			p.context.isAssignmentTarget = !optional
			if !optional {
				p.expect(".")
			}
			property := p.parseIdentifierName()
			expr = p.finalize(p.startNode(startToken, 0), nStaticMemberExpression(expr, property, optional))
		} else {
			break
		}
	}
	p.context.allowIn = prevAllowIn
	if hasOptional {
		return nChainExpression(expr)
	}
	return expr
}

func (p *parser) parseSuper() *Node {
	node := p.createNode()
	p.expectKeyword("super")
	if !p.match("[") && !p.match(".") {
		p.throwUnexpectedToken(p.lookahead, "")
	}
	return p.finalize(node, nSuper())
}

func (p *parser) parseLeftHandSideExpression() *Node {
	if !p.context.allowIn {
		internalError("ASSERT: callee of new expression always allow in keyword.")
	}

	node := p.startNode(p.lookahead, 0)
	var expr *Node
	if p.matchKeyword("super") && p.context.inFunctionBody {
		expr = p.parseSuper()
	} else if p.matchKeyword("new") {
		expr = p.inheritCoverGrammar(p.parseNewExpression)
	} else {
		expr = p.inheritCoverGrammar(p.parsePrimaryExpression)
	}

	hasOptional := false
	for {
		optional := false
		if p.match("?.") {
			optional = true
			hasOptional = true
			p.expect("?.")
		}
		if p.match("[") {
			p.context.isBindingElement = false
			p.context.isAssignmentTarget = !optional
			p.expect("[")
			property := p.isolateCoverGrammar(p.parseExpression)
			p.expect("]")
			expr = p.finalize(node, nComputedMemberExpression(expr, property, optional))
		} else if p.lookahead.typ == tokTemplate && p.lookahead.head {
			// An optional template literal is not part of the specification.
			if optional {
				p.throwUnexpectedToken(p.lookahead, "")
			}
			if hasOptional {
				p.throwError(msgInvalidTaggedTemplateOnOptionalChain)
			}
			quasi := p.parseTemplateLiteral(true)
			expr = p.finalize(node, nTaggedTemplateExpression(expr, quasi))
		} else if p.match(".") || optional {
			p.context.isBindingElement = false
			p.context.isAssignmentTarget = !optional
			if !optional {
				p.expect(".")
			}
			property := p.parseIdentifierName()
			expr = p.finalize(node, nStaticMemberExpression(expr, property, optional))
		} else {
			break
		}
	}
	if hasOptional {
		return nChainExpression(expr)
	}
	return expr
}

// https://tc39.github.io/ecma262/#sec-update-expressions

func (p *parser) parseUpdateExpression() *Node {
	var expr *Node
	startToken := p.lookahead

	if p.match("++") || p.match("--") {
		node := p.startNode(startToken, 0)
		token := p.nextToken()
		expr = p.inheritCoverGrammar(p.parseUnaryExpression)
		if p.context.strict && expr.Type() == "Identifier" && isRestrictedWord(expr.str("name")) {
			p.tolerateError(msgStrictLHSPrefix)
		}
		if !p.context.isAssignmentTarget {
			p.tolerateError(msgInvalidLHSInAssignment)
		}
		expr = p.finalize(node, nUpdateExpression(token.str, expr, true))
		p.context.isAssignmentTarget = false
		p.context.isBindingElement = false
	} else {
		expr = p.inheritCoverGrammar(p.parseLeftHandSideExpressionAllowCall)
		if !p.hasLineTerminator && p.lookahead.typ == tokPunctuator {
			if p.match("++") || p.match("--") {
				if p.context.strict && expr.Type() == "Identifier" && isRestrictedWord(expr.str("name")) {
					p.tolerateError(msgStrictLHSPostfix)
				}
				if !p.context.isAssignmentTarget {
					p.tolerateError(msgInvalidLHSInAssignment)
				}
				p.context.isAssignmentTarget = false
				p.context.isBindingElement = false
				operator := p.nextToken().str
				expr = p.finalize(p.startNode(startToken, 0), nUpdateExpression(operator, expr, false))
			}
		}
	}
	return expr
}

// https://tc39.github.io/ecma262/#sec-unary-operators

func (p *parser) parseAwaitExpression() *Node {
	node := p.createNode()
	p.nextToken()
	argument := p.parseUnaryExpression()
	return p.finalize(node, nAwaitExpression(argument))
}

func (p *parser) parseUnaryExpression() *Node {
	var expr *Node
	if p.match("+") || p.match("-") || p.match("~") || p.match("!") ||
		p.matchKeyword("delete") || p.matchKeyword("void") || p.matchKeyword("typeof") {
		node := p.startNode(p.lookahead, 0)
		token := p.nextToken()
		expr = p.inheritCoverGrammar(p.parseUnaryExpression)
		expr = p.finalize(node, nUnaryExpression(token.str, expr))
		if p.context.strict && expr.str("operator") == "delete" && expr.node("argument").Type() == "Identifier" {
			p.tolerateError(msgStrictDelete)
		}
		p.context.isAssignmentTarget = false
		p.context.isBindingElement = false
	} else if p.context.isAsync && p.matchContextualKeyword("await") {
		expr = p.parseAwaitExpression()
	} else {
		expr = p.parseUpdateExpression()
	}
	return expr
}

func (p *parser) parseExponentiationExpression() *Node {
	startToken := p.lookahead

	expr := p.inheritCoverGrammar(p.parseUnaryExpression)
	if expr.Type() != "UnaryExpression" && p.match("**") {
		p.nextToken()
		p.context.isAssignmentTarget = false
		p.context.isBindingElement = false
		left := expr
		right := p.isolateCoverGrammar(p.parseExponentiationExpression)
		expr = p.finalize(p.startNode(startToken, 0), nBinaryExpression("**", left, right))
	}
	return expr
}

// https://tc39.github.io/ecma262/#sec-multiplicative-operators and the
// following binary operator sections.

func (p *parser) binaryPrecedence(token *rawToken) int {
	switch token.typ {
	case tokPunctuator:
		return operatorPrecedence[token.str]
	case tokKeyword:
		if token.str == "instanceof" || (p.context.allowIn && token.str == "in") {
			return 12
		}
	}
	return 0
}

func (p *parser) parseBinaryExpression() *Node {
	startToken := p.lookahead

	expr := p.inheritCoverGrammar(p.parseExponentiationExpression)

	allowAndOr := true
	allowNullishCoalescing := true
	updateNullishCoalescingRestrictions := func(token *rawToken) {
		if token.str == "&&" || token.str == "||" {
			allowNullishCoalescing = false
		}
		if token.str == "??" {
			allowAndOr = false
		}
	}

	token := p.lookahead
	prec := p.binaryPrecedence(token)
	if prec <= 0 {
		return expr
	}
	updateNullishCoalescingRestrictions(token)
	p.nextToken()

	p.context.isAssignmentTarget = false
	p.context.isBindingElement = false

	markers := []*rawToken{startToken, p.lookahead}
	left := expr
	right := p.isolateCoverGrammar(p.parseExponentiationExpression)

	// The stack alternates operands (*Node) and operators (string).
	stack := []any{left, token.str, right}
	precedences := []int{prec}
	for {
		prec = p.binaryPrecedence(p.lookahead)
		if prec <= 0 {
			break
		}
		if (!allowAndOr && (p.lookahead.str == "&&" || p.lookahead.str == "||")) ||
			(!allowNullishCoalescing && p.lookahead.str == "??") {
			p.throwUnexpectedToken(p.lookahead, "")
		}
		updateNullishCoalescingRestrictions(p.lookahead)

		// Reduce: make a binary expression from the three topmost entries.
		for len(stack) > 2 && prec <= precedences[len(precedences)-1] {
			right := stack[len(stack)-1]
			operator := stack[len(stack)-2].(string)
			left := stack[len(stack)-3]
			stack = stack[:len(stack)-3]
			precedences = precedences[:len(precedences)-1]
			markers = markers[:len(markers)-1]
			m := markers[len(markers)-1]
			node := p.startNode(m, m.lineStart)
			stack = append(stack, p.finalize(node, nBinaryExpression(operator, left, right)))
		}

		// Shift.
		stack = append(stack, p.nextToken().str)
		precedences = append(precedences, prec)
		markers = append(markers, p.lookahead)
		stack = append(stack, p.isolateCoverGrammar(p.parseExponentiationExpression))
	}

	// Final reduce to clean up the stack.
	i := len(stack) - 1
	result := stack[i]
	lastMarker := markers[len(markers)-1]
	markers = markers[:len(markers)-1]
	for i > 1 {
		m := markers[len(markers)-1]
		markers = markers[:len(markers)-1]
		lastLineStart := 0
		if lastMarker != nil {
			lastLineStart = lastMarker.lineStart
		}
		node := p.startNode(m, lastLineStart)
		operator := stack[i-1].(string)
		result = p.finalize(node, nBinaryExpression(operator, stack[i-2], result))
		i -= 2
		lastMarker = m
	}
	return result.(*Node)
}

// https://tc39.github.io/ecma262/#sec-conditional-operator

func (p *parser) parseConditionalExpression() *Node {
	startToken := p.lookahead

	expr := p.inheritCoverGrammar(p.parseBinaryExpression)
	if p.match("?") {
		p.nextToken()

		prevAllowIn := p.context.allowIn
		p.context.allowIn = true
		consequent := p.isolateCoverGrammar(p.parseAssignmentExpression)
		p.context.allowIn = prevAllowIn

		p.expect(":")
		alternate := p.isolateCoverGrammar(p.parseAssignmentExpression)

		expr = p.finalize(p.startNode(startToken, 0), nConditionalExpression(expr, consequent, alternate))
		p.context.isAssignmentTarget = false
		p.context.isBindingElement = false
	}
	return expr
}

// https://tc39.github.io/ecma262/#sec-assignment-operators

func (p *parser) checkPatternParam(options *paramInfo, param *Node) {
	switch param.Type() {
	case "Identifier":
		p.validateParam(options, param, param.str("name"))
	case "RestElement":
		p.checkPatternParam(options, param.node("argument"))
	case "AssignmentPattern":
		p.checkPatternParam(options, param.node("left"))
	case "ArrayPattern":
		for _, el := range param.list("elements") {
			if el != nil {
				p.checkPatternParam(options, el.(*Node))
			}
		}
	case "ObjectPattern":
		for _, prop := range param.list("properties") {
			property := prop.(*Node)
			if property.Type() == "RestElement" {
				p.checkPatternParam(options, property)
			} else {
				p.checkPatternParam(options, property.node("value"))
			}
		}
	}
	options.simple = options.simple && param.class == "Identifier"
}

// coverFormals is the parameter list recovered from an arrow function's
// cover grammar.
type coverFormals struct {
	simple          bool
	params          []any
	stricted        any
	firstRestricted any
	message         string
}

func (p *parser) reinterpretAsCoverFormalsList(expr *Node) *coverFormals {
	params := []any{expr}
	options := &paramInfo{simple: true, paramSet: map[string]bool{}}

	asyncArrow := false
	switch expr.Type() {
	case "Identifier":
	case arrowParameterPlaceHolder:
		params = expr.list("params")
		asyncArrow = expr.Get("async").(bool)
	default:
		return nil
	}

	for i, pa := range params {
		param := pa.(*Node)
		if param.Type() == "AssignmentPattern" {
			right := param.node("right")
			if right.Type() == "YieldExpression" {
				if right.Get("argument") != nil {
					p.throwUnexpectedToken(p.lookahead, "")
				}
				right.Set("type", "Identifier")
				right.Set("name", "yield")
				right.Delete("argument")
				right.Delete("delegate")
			}
		} else if asyncArrow && param.Type() == "Identifier" && param.str("name") == "await" {
			p.throwUnexpectedToken(p.lookahead, "")
		}
		p.checkPatternParam(options, param)
		params[i] = param
	}

	if p.context.strict || !p.context.allowYield {
		for _, pa := range params {
			if pa.(*Node).Type() == "YieldExpression" {
				p.throwUnexpectedToken(p.lookahead, "")
			}
		}
	}

	if options.hasDuplicateParameterNames {
		token := options.firstRestricted
		if p.context.strict {
			token = options.stricted
		}
		p.throwUnexpectedToken(token, msgDuplicateParameter)
	}

	return &coverFormals{
		simple:          options.simple,
		params:          params,
		stricted:        options.stricted,
		firstRestricted: options.firstRestricted,
		message:         options.message,
	}
}

func (p *parser) parseAssignmentExpression() *Node {
	if !p.context.allowYield && p.matchKeyword("yield") {
		return p.parseYieldExpression()
	}

	startToken := p.lookahead
	token := startToken
	expr := p.parseConditionalExpression()

	if token.typ == tokIdentifier && token.lineNumber == p.lookahead.lineNumber && token.str == "async" {
		if p.lookahead.typ == tokIdentifier || p.matchKeyword("yield") {
			arg := p.parsePrimaryExpression()
			p.reinterpretExpressionAsPattern(arg)
			expr = placeHolder([]any{arg}, true)
		}
	}

	if expr.Type() == arrowParameterPlaceHolder || p.match("=>") {
		// https://tc39.github.io/ecma262/#sec-arrow-function-definitions
		p.context.isAssignmentTarget = false
		p.context.isBindingElement = false
		isAsync, _ := expr.Get("async").(bool)
		list := p.reinterpretAsCoverFormalsList(expr)

		if list != nil {
			if p.hasLineTerminator {
				p.tolerateUnexpectedToken(p.lookahead, "")
			}
			p.context.firstCoverInitializedNameError = nil

			prevStrict := p.context.strict
			prevAllowStrictDirective := p.context.allowStrictDirective
			p.context.allowStrictDirective = list.simple

			prevAllowYield := p.context.allowYield
			prevIsAsync := p.context.isAsync
			p.context.allowYield = true
			p.context.isAsync = isAsync

			node := p.startNode(startToken, 0)
			p.expect("=>")
			var body *Node
			if p.match("{") {
				prevAllowIn := p.context.allowIn
				p.context.allowIn = true
				body = p.parseFunctionSourceElements()
				p.context.allowIn = prevAllowIn
			} else {
				body = p.isolateCoverGrammar(p.parseAssignmentExpression)
			}
			expression := body.Type() != "BlockStatement"

			if p.context.strict && list.firstRestricted != nil {
				p.throwUnexpectedToken(list.firstRestricted, list.message)
			}
			if p.context.strict && list.stricted != nil {
				p.tolerateUnexpectedToken(list.stricted, list.message)
			}
			expr = p.finalize(node, nArrowFunctionExpression(list.params, body, expression, isAsync))

			p.context.strict = prevStrict
			p.context.allowStrictDirective = prevAllowStrictDirective
			p.context.allowYield = prevAllowYield
			p.context.isAsync = prevIsAsync
		}
	} else if p.matchAssign() {
		if !p.context.isAssignmentTarget {
			p.tolerateError(msgInvalidLHSInAssignment)
		}

		if p.context.strict && expr.Type() == "Identifier" {
			name := expr.str("name")
			if isRestrictedWord(name) {
				p.tolerateUnexpectedToken(token, msgStrictLHSAssignment)
			}
			if isStrictModeReservedWord(name) {
				p.tolerateUnexpectedToken(token, msgStrictReservedWord)
			}
		}

		if !p.match("=") {
			p.context.isAssignmentTarget = false
			p.context.isBindingElement = false
		} else {
			p.reinterpretExpressionAsPattern(expr)
		}

		token = p.nextToken()
		operator := token.str
		right := p.isolateCoverGrammar(p.parseAssignmentExpression)
		expr = p.finalize(p.startNode(startToken, 0), nAssignmentExpression(operator, expr, right))
		p.context.firstCoverInitializedNameError = nil
	}
	return expr
}

// https://tc39.github.io/ecma262/#sec-comma-operator

func (p *parser) parseExpression() *Node {
	startToken := p.lookahead
	expr := p.isolateCoverGrammar(p.parseAssignmentExpression)

	if p.match(",") {
		expressions := []any{expr}
		for p.lookahead.typ != tokEOF {
			if !p.match(",") {
				break
			}
			p.nextToken()
			expressions = append(expressions, p.isolateCoverGrammar(p.parseAssignmentExpression))
		}
		expr = p.finalize(p.startNode(startToken, 0), nSequenceExpression(expressions))
	}
	return expr
}

// https://tc39.github.io/ecma262/#sec-block

func (p *parser) parseStatementListItem() *Node {
	p.context.isAssignmentTarget = true
	p.context.isBindingElement = true
	if p.lookahead.typ != tokKeyword {
		return p.parseStatement()
	}
	switch p.lookahead.str {
	case "export":
		if !p.context.isModule {
			p.tolerateUnexpectedToken(p.lookahead, msgIllegalExportDeclaration)
		}
		return p.parseExportDeclaration()
	case "import":
		if p.matchImportCall() {
			return p.parseExpressionStatement()
		}
		if p.matchImportMeta() {
			return p.parseStatement()
		}
		if !p.context.isModule {
			p.tolerateUnexpectedToken(p.lookahead, msgIllegalImportDeclaration)
		}
		return p.parseImportDeclaration()
	case "const":
		return p.parseLexicalDeclaration(false)
	case "function":
		return p.parseFunctionDeclaration(false)
	case "class":
		return p.parseClassDeclaration(false)
	case "let":
		if p.isLexicalDeclaration() {
			return p.parseLexicalDeclaration(false)
		}
		return p.parseStatement()
	}
	return p.parseStatement()
}

func (p *parser) parseBlock() *Node {
	node := p.createNode()

	p.expect("{")
	block := []any{}
	for !p.match("}") {
		block = append(block, p.parseStatementListItem())
	}
	p.expect("}")
	return p.finalize(node, nBlockStatement(block))
}

// https://tc39.github.io/ecma262/#sec-let-and-const-declarations

func (p *parser) parseLexicalBinding(kind string, inFor bool) *Node {
	node := p.createNode()
	var params []any
	id := p.parsePattern(&params, kind)

	if p.context.strict && id.Type() == "Identifier" && isRestrictedWord(id.str("name")) {
		p.tolerateError(msgStrictVarName)
	}

	var init *Node
	if kind == "const" {
		if !p.matchKeyword("in") && !p.matchContextualKeyword("of") {
			if p.match("=") {
				p.nextToken()
				init = p.isolateCoverGrammar(p.parseAssignmentExpression)
			} else {
				p.throwError(msgDeclarationMissingInitializer, "const")
			}
		}
	} else if (!inFor && id.Type() != "Identifier") || p.match("=") {
		p.expect("=")
		init = p.isolateCoverGrammar(p.parseAssignmentExpression)
	}
	return p.finalize(node, nVariableDeclarator(id, init))
}

func (p *parser) parseBindingList(kind string, inFor bool) []any {
	list := []any{p.parseLexicalBinding(kind, inFor)}
	for p.match(",") {
		p.nextToken()
		list = append(list, p.parseLexicalBinding(kind, inFor))
	}
	return list
}

func (p *parser) isLexicalDeclaration() bool {
	next := p.peekToken()
	return next.typ == tokIdentifier ||
		(next.typ == tokPunctuator && next.str == "[") ||
		(next.typ == tokPunctuator && next.str == "{") ||
		(next.typ == tokKeyword && next.str == "let") ||
		(next.typ == tokKeyword && next.str == "yield")
}

func (p *parser) parseLexicalDeclaration(inFor bool) *Node {
	node := p.createNode()
	kind := p.nextToken().str
	if kind != "let" && kind != "const" {
		internalError("ASSERT: Lexical declaration must be either let or const")
	}
	declarations := p.parseBindingList(kind, inFor)
	p.consumeSemicolon()
	return p.finalize(node, nVariableDeclaration(declarations, kind))
}

// https://tc39.github.io/ecma262/#sec-destructuring-binding-patterns

func (p *parser) parseBindingRestElement(params *[]any, kind string) *Node {
	node := p.createNode()
	p.expect("...")
	arg := p.parsePattern(params, kind)
	return p.finalize(node, nRestElement(arg))
}

func (p *parser) parseArrayPattern(params *[]any, kind string) *Node {
	node := p.createNode()

	p.expect("[")
	elements := []any{}
	for !p.match("]") {
		if p.match(",") {
			p.nextToken()
			elements = append(elements, nil)
			continue
		}
		if p.match("...") {
			elements = append(elements, p.parseBindingRestElement(params, kind))
			break
		}
		elements = append(elements, p.parsePatternWithDefault(params, kind))
		if !p.match("]") {
			p.expect(",")
		}
	}
	p.expect("]")
	return p.finalize(node, nArrayPattern(elements))
}

func (p *parser) parsePropertyPattern(params *[]any, kind string) *Node {
	node := p.createNode()

	computed := false
	shorthand := false
	var key, value *Node

	if p.lookahead.typ == tokIdentifier {
		keyToken := p.lookahead
		key = p.parseVariableIdentifier("")
		init := p.finalize(node, nIdentifier(keyToken.str))
		if p.match("=") {
			*params = append(*params, keyToken)
			shorthand = true
			p.nextToken()
			expr := p.parseAssignmentExpression()
			value = p.finalize(p.startNode(keyToken, 0), nAssignmentPattern(init, expr))
		} else if !p.match(":") {
			*params = append(*params, keyToken)
			shorthand = true
			value = init
		} else {
			p.expect(":")
			value = p.parsePatternWithDefault(params, kind)
		}
	} else {
		computed = p.match("[")
		key = p.parseObjectPropertyKey()
		p.expect(":")
		value = p.parsePatternWithDefault(params, kind)
	}
	return p.finalize(node, nProperty("init", key, computed, value, false, shorthand))
}

func (p *parser) parseRestProperty(params *[]any) *Node {
	node := p.createNode()
	p.expect("...")
	arg := p.parsePattern(params, "")
	if p.match("=") {
		p.throwError(msgDefaultRestProperty)
	}
	if !p.match("}") {
		p.throwError(msgPropertyAfterRestProperty)
	}
	return p.finalize(node, nRestElement(arg))
}

func (p *parser) parseObjectPattern(params *[]any, kind string) *Node {
	node := p.createNode()
	properties := []any{}

	p.expect("{")
	for !p.match("}") {
		if p.match("...") {
			properties = append(properties, p.parseRestProperty(params))
		} else {
			properties = append(properties, p.parsePropertyPattern(params, kind))
		}
		if !p.match("}") {
			p.expect(",")
		}
	}
	p.expect("}")
	return p.finalize(node, nObjectPattern(properties))
}

func (p *parser) parsePattern(params *[]any, kind string) *Node {
	if p.match("[") {
		return p.parseArrayPattern(params, kind)
	}
	if p.match("{") {
		return p.parseObjectPattern(params, kind)
	}
	if p.matchKeyword("let") && (kind == "const" || kind == "let") {
		p.tolerateUnexpectedToken(p.lookahead, msgLetInLexicalBinding)
	}
	*params = append(*params, p.lookahead)
	return p.parseVariableIdentifier(kind)
}

func (p *parser) parsePatternWithDefault(params *[]any, kind string) *Node {
	startToken := p.lookahead

	pattern := p.parsePattern(params, kind)
	if p.match("=") {
		p.nextToken()
		prevAllowYield := p.context.allowYield
		p.context.allowYield = true
		right := p.isolateCoverGrammar(p.parseAssignmentExpression)
		p.context.allowYield = prevAllowYield
		pattern = p.finalize(p.startNode(startToken, 0), nAssignmentPattern(pattern, right))
	}
	return pattern
}

// https://tc39.github.io/ecma262/#sec-variable-statement

func (p *parser) parseVariableIdentifier(kind string) *Node {
	node := p.createNode()

	token := p.nextToken()
	if token.typ == tokKeyword && token.str == "yield" {
		if p.context.strict {
			p.tolerateUnexpectedToken(token, msgStrictReservedWord)
		} else if !p.context.allowYield {
			p.throwUnexpectedToken(token, "")
		}
	} else if token.typ != tokIdentifier {
		if p.context.strict && token.typ == tokKeyword && isStrictModeReservedWord(token.str) {
			p.tolerateUnexpectedToken(token, msgStrictReservedWord)
		} else if p.context.strict || token.str != "let" || kind != "var" {
			p.throwUnexpectedToken(token, "")
		}
	} else if (p.context.isModule || p.context.isAsync) && token.typ == tokIdentifier && token.str == "await" {
		p.tolerateUnexpectedToken(token, "")
	}

	return p.finalize(node, nIdentifier(token.str))
}

func (p *parser) parseVariableDeclaration(inFor bool) *Node {
	node := p.createNode()

	var params []any
	id := p.parsePattern(&params, "var")

	if p.context.strict && id.Type() == "Identifier" && isRestrictedWord(id.str("name")) {
		p.tolerateError(msgStrictVarName)
	}

	var init *Node
	if p.match("=") {
		p.nextToken()
		init = p.isolateCoverGrammar(p.parseAssignmentExpression)
	} else if id.Type() != "Identifier" && !inFor {
		p.expect("=")
	}
	return p.finalize(node, nVariableDeclarator(id, init))
}

func (p *parser) parseVariableDeclarationList(inFor bool) []any {
	list := []any{p.parseVariableDeclaration(inFor)}
	for p.match(",") {
		p.nextToken()
		list = append(list, p.parseVariableDeclaration(inFor))
	}
	return list
}

func (p *parser) parseVariableStatement() *Node {
	node := p.createNode()
	p.expectKeyword("var")
	declarations := p.parseVariableDeclarationList(false)
	p.consumeSemicolon()
	return p.finalize(node, nVariableDeclaration(declarations, "var"))
}

// https://tc39.github.io/ecma262/#sec-empty-statement

func (p *parser) parseEmptyStatement() *Node {
	node := p.createNode()
	p.expect(";")
	return p.finalize(node, nEmptyStatement())
}

// https://tc39.github.io/ecma262/#sec-expression-statement

func (p *parser) parseExpressionStatement() *Node {
	node := p.createNode()
	expr := p.parseExpression()
	p.consumeSemicolon()
	return p.finalize(node, nExpressionStatement(expr))
}

// https://tc39.github.io/ecma262/#sec-if-statement

func (p *parser) parseIfClause() *Node {
	if p.context.strict && p.matchKeyword("function") {
		p.tolerateError(msgStrictFunction)
	}
	return p.parseStatement()
}

func (p *parser) parseIfStatement() *Node {
	node := p.createNode()
	var consequent, alternate *Node

	p.expectKeyword("if")
	p.expect("(")
	test := p.parseExpression()

	if !p.match(")") && p.config.tolerant {
		p.tolerateUnexpectedToken(p.nextToken(), "")
		consequent = p.finalize(p.createNode(), nEmptyStatement())
	} else {
		p.expect(")")
		consequent = p.parseIfClause()
		if p.matchKeyword("else") {
			p.nextToken()
			alternate = p.parseIfClause()
		}
	}
	return p.finalize(node, nIfStatement(test, consequent, alternate))
}

// https://tc39.github.io/ecma262/#sec-do-while-statement

func (p *parser) parseDoWhileStatement() *Node {
	node := p.createNode()
	p.expectKeyword("do")

	p.tolerateInvalidLoopStatement()

	prevInIteration := p.context.inIteration
	p.context.inIteration = true
	body := p.parseStatement()
	p.context.inIteration = prevInIteration

	p.expectKeyword("while")
	p.expect("(")
	test := p.parseExpression()

	if !p.match(")") && p.config.tolerant {
		p.tolerateUnexpectedToken(p.nextToken(), "")
	} else {
		p.expect(")")
		if p.match(";") {
			p.nextToken()
		}
	}
	return p.finalize(node, nDoWhileStatement(body, test))
}

// https://tc39.github.io/ecma262/#sec-while-statement

func (p *parser) parseWhileStatement() *Node {
	node := p.createNode()
	var body *Node

	p.expectKeyword("while")
	p.expect("(")
	test := p.parseExpression()

	if !p.match(")") && p.config.tolerant {
		p.tolerateUnexpectedToken(p.nextToken(), "")
		body = p.finalize(p.createNode(), nEmptyStatement())
	} else {
		p.expect(")")
		prevInIteration := p.context.inIteration
		p.context.inIteration = true
		body = p.parseStatement()
		p.context.inIteration = prevInIteration
	}
	return p.finalize(node, nWhileStatement(test, body))
}

// https://tc39.github.io/ecma262/#sec-for-statement
// https://tc39.github.io/ecma262/#sec-for-in-and-for-of-statements

func (p *parser) parseForStatement() *Node {
	var init, test, update, left, right *Node
	hasLeft := false
	forIn := true
	await := false

	node := p.createNode()
	p.expectKeyword("for")
	if p.matchContextualKeyword("await") {
		if !p.context.isAsync {
			p.tolerateUnexpectedToken(p.lookahead, "")
		}
		await = true
		p.nextToken()
	}

	p.expect("(")

	if p.match(";") {
		p.nextToken()
	} else if p.matchKeyword("var") {
		initMarker := p.createNode()
		p.nextToken()

		prevAllowIn := p.context.allowIn
		p.context.allowIn = false
		declarations := p.parseVariableDeclarationList(true)
		p.context.allowIn = prevAllowIn

		if !await && len(declarations) == 1 && p.matchKeyword("in") {
			decl := declarations[0].(*Node)
			declID := decl.node("id").Type()
			if decl.Get("init") != nil && (declID == "ArrayPattern" || declID == "ObjectPattern" || p.context.strict) {
				p.tolerateError(msgForInOfLoopInitializer, "for-in")
			}
			left = p.finalize(initMarker, nVariableDeclaration(declarations, "var"))
			p.nextToken()
			hasLeft = true
			right = p.parseExpression()
		} else if len(declarations) == 1 && declarations[0].(*Node).Get("init") == nil && p.matchContextualKeyword("of") {
			left = p.finalize(initMarker, nVariableDeclaration(declarations, "var"))
			p.nextToken()
			hasLeft = true
			right = p.parseAssignmentExpression()
			forIn = false
		} else {
			init = p.finalize(initMarker, nVariableDeclaration(declarations, "var"))
			p.expect(";")
		}
	} else if p.matchKeyword("const") || p.matchKeyword("let") {
		initMarker := p.createNode()
		kind := p.nextToken().str

		if !p.context.strict && p.lookahead.str == "in" && !p.lookahead.isNum {
			left = p.finalize(initMarker, nIdentifier(kind))
			p.nextToken()
			hasLeft = true
			right = p.parseExpression()
		} else {
			prevAllowIn := p.context.allowIn
			p.context.allowIn = false
			declarations := p.parseBindingList(kind, true)
			p.context.allowIn = prevAllowIn

			if len(declarations) == 1 && declarations[0].(*Node).Get("init") == nil && p.matchKeyword("in") {
				left = p.finalize(initMarker, nVariableDeclaration(declarations, kind))
				p.nextToken()
				hasLeft = true
				right = p.parseExpression()
			} else if len(declarations) == 1 && declarations[0].(*Node).Get("init") == nil && p.matchContextualKeyword("of") {
				left = p.finalize(initMarker, nVariableDeclaration(declarations, kind))
				p.nextToken()
				hasLeft = true
				right = p.parseAssignmentExpression()
				forIn = false
			} else {
				p.consumeSemicolon()
				init = p.finalize(initMarker, nVariableDeclaration(declarations, kind))
			}
		}
	} else {
		initStartToken := p.lookahead
		prevBinding := p.context.isBindingElement
		prevTarget := p.context.isAssignmentTarget
		prevCover := p.context.firstCoverInitializedNameError

		prevAllowIn := p.context.allowIn
		p.context.allowIn = false
		init = p.inheritCoverGrammar(p.parseAssignmentExpression)
		p.context.allowIn = prevAllowIn

		if p.matchKeyword("in") {
			if !p.context.isAssignmentTarget || init.Type() == "AssignmentExpression" {
				p.tolerateError(msgInvalidLHSInForIn)
			}
			p.nextToken()
			p.reinterpretExpressionAsPattern(init)
			left = init
			hasLeft = true
			right = p.parseExpression()
			init = nil
		} else if p.matchContextualKeyword("of") {
			if !p.context.isAssignmentTarget || init.Type() == "AssignmentExpression" {
				p.tolerateError(msgInvalidLHSInForLoop)
			}
			p.nextToken()
			p.reinterpretExpressionAsPattern(init)
			left = init
			hasLeft = true
			right = p.parseAssignmentExpression()
			init = nil
			forIn = false
		} else {
			// The init expression was not parsed isolated, but it should have
			// been.
			p.context.isBindingElement = prevBinding
			p.context.isAssignmentTarget = prevTarget
			p.context.firstCoverInitializedNameError = prevCover

			if p.match(",") {
				initSeq := []any{init}
				for p.match(",") {
					p.nextToken()
					initSeq = append(initSeq, p.isolateCoverGrammar(p.parseAssignmentExpression))
				}
				init = p.finalize(p.startNode(initStartToken, 0), nSequenceExpression(initSeq))
			}
			p.expect(";")
		}
	}

	if !hasLeft {
		if !p.match(";") {
			test = p.isolateCoverGrammar(p.parseExpression)
		}
		p.expect(";")
		if !p.match(")") {
			update = p.isolateCoverGrammar(p.parseExpression)
		}
	}

	var body *Node
	if !p.match(")") && p.config.tolerant {
		p.tolerateUnexpectedToken(p.nextToken(), "")
		body = p.finalize(p.createNode(), nEmptyStatement())
	} else {
		p.expect(")")
		p.tolerateInvalidLoopStatement()

		prevInIteration := p.context.inIteration
		p.context.inIteration = true
		body = p.isolateCoverGrammar(p.parseStatement)
		p.context.inIteration = prevInIteration
	}

	switch {
	case !hasLeft:
		return p.finalize(node, nForStatement(init, test, update, body))
	case forIn:
		return p.finalize(node, nForInStatement(left, right, body))
	default:
		return p.finalize(node, nForOfStatement(left, right, body, await))
	}
}

// https://tc39.github.io/ecma262/#sec-continue-statement

func (p *parser) parseContinueStatement() *Node {
	node := p.createNode()
	p.expectKeyword("continue")

	var label *Node
	if p.lookahead.typ == tokIdentifier && !p.hasLineTerminator {
		id := p.parseVariableIdentifier("")
		label = id
		if !p.context.labelSet["$"+id.str("name")] {
			p.throwError(msgUnknownLabel, id.str("name"))
		}
	}

	p.consumeSemicolon()
	if label == nil && !p.context.inIteration {
		p.throwError(msgIllegalContinue)
	}
	return p.finalize(node, nContinueStatement(label))
}

// https://tc39.github.io/ecma262/#sec-break-statement

func (p *parser) parseBreakStatement() *Node {
	node := p.createNode()
	p.expectKeyword("break")

	var label *Node
	if p.lookahead.typ == tokIdentifier && !p.hasLineTerminator {
		id := p.parseVariableIdentifier("")
		if !p.context.labelSet["$"+id.str("name")] {
			p.throwError(msgUnknownLabel, id.str("name"))
		}
		label = id
	}

	p.consumeSemicolon()
	if label == nil && !p.context.inIteration && !p.context.inSwitch {
		p.throwError(msgIllegalBreak)
	}
	return p.finalize(node, nBreakStatement(label))
}

// https://tc39.github.io/ecma262/#sec-return-statement

func (p *parser) parseReturnStatement() *Node {
	if !p.context.inFunctionBody {
		p.tolerateError(msgIllegalReturn)
	}

	node := p.createNode()
	p.expectKeyword("return")

	hasArgument := (!p.match(";") && !p.match("}") && !p.hasLineTerminator && p.lookahead.typ != tokEOF) ||
		p.lookahead.typ == tokStringLiteral || p.lookahead.typ == tokTemplate

	var argument *Node
	if hasArgument {
		argument = p.parseExpression()
	}
	p.consumeSemicolon()
	return p.finalize(node, nReturnStatement(argument))
}

// https://tc39.github.io/ecma262/#sec-with-statement

func (p *parser) parseWithStatement() *Node {
	if p.context.strict {
		p.tolerateError(msgStrictModeWith)
	}

	node := p.createNode()
	var body *Node

	p.expectKeyword("with")
	p.expect("(")
	object := p.parseExpression()

	if !p.match(")") && p.config.tolerant {
		p.tolerateUnexpectedToken(p.nextToken(), "")
		body = p.finalize(p.createNode(), nEmptyStatement())
	} else {
		p.expect(")")
		body = p.parseStatement()
	}
	return p.finalize(node, nWithStatement(object, body))
}

// https://tc39.github.io/ecma262/#sec-switch-statement

func (p *parser) parseSwitchCase() *Node {
	node := p.createNode()

	var test *Node
	if p.matchKeyword("default") {
		p.nextToken()
	} else {
		p.expectKeyword("case")
		test = p.parseExpression()
	}
	p.expect(":")

	consequent := []any{}
	for !p.match("}") && !p.matchKeyword("default") && !p.matchKeyword("case") {
		consequent = append(consequent, p.parseStatementListItem())
	}
	return p.finalize(node, nSwitchCase(test, consequent))
}

func (p *parser) parseSwitchStatement() *Node {
	node := p.createNode()
	p.expectKeyword("switch")

	p.expect("(")
	discriminant := p.parseExpression()
	p.expect(")")

	prevInSwitch := p.context.inSwitch
	p.context.inSwitch = true

	cases := []any{}
	defaultFound := false
	p.expect("{")
	for !p.match("}") {
		clause := p.parseSwitchCase()
		if clause.Get("test") == nil {
			if defaultFound {
				p.throwError(msgMultipleDefaultsInSwitch)
			}
			defaultFound = true
		}
		cases = append(cases, clause)
	}
	p.expect("}")

	p.context.inSwitch = prevInSwitch
	return p.finalize(node, nSwitchStatement(discriminant, cases))
}

// https://tc39.github.io/ecma262/#sec-labelled-statements

func (p *parser) parseLabelledStatement() *Node {
	node := p.createNode()
	expr := p.parseExpression()

	var statement *Node
	if expr.Type() == "Identifier" && p.match(":") {
		p.nextToken()

		name := expr.str("name")
		key := "$" + name
		if p.context.labelSet[key] {
			p.throwError(msgRedeclaration, "Label", name)
		}

		p.context.labelSet[key] = true
		var body *Node
		if p.matchKeyword("class") {
			p.tolerateUnexpectedToken(p.lookahead, "")
			body = p.parseClassDeclaration(false)
		} else if p.matchKeyword("function") {
			token := p.lookahead
			declaration := p.parseFunctionDeclaration(false)
			if p.context.strict {
				p.tolerateUnexpectedToken(token, msgStrictFunction)
			} else if declaration.Get("generator").(bool) {
				p.tolerateUnexpectedToken(token, msgGeneratorInLegacyContext)
			}
			body = declaration
		} else {
			body = p.parseStatement()
		}
		delete(p.context.labelSet, key)

		statement = nLabeledStatement(expr, body)
	} else {
		p.consumeSemicolon()
		statement = nExpressionStatement(expr)
	}
	return p.finalize(node, statement)
}

// https://tc39.github.io/ecma262/#sec-throw-statement

func (p *parser) parseThrowStatement() *Node {
	node := p.createNode()
	p.expectKeyword("throw")

	if p.hasLineTerminator {
		p.throwError(msgNewlineAfterThrow)
	}

	argument := p.parseExpression()
	p.consumeSemicolon()
	return p.finalize(node, nThrowStatement(argument))
}

// https://tc39.github.io/ecma262/#sec-try-statement

func (p *parser) parseCatchClause() *Node {
	node := p.createNode()

	p.expectKeyword("catch")

	var param *Node
	if p.match("(") {
		p.expect("(")
		if p.match(")") {
			p.throwUnexpectedToken(p.lookahead, "")
		}

		var params []any
		param = p.parsePattern(&params, "")
		paramMap := map[string]bool{}
		for _, t := range params {
			value := t.(*rawToken).value()
			key := "$" + jsString(value)
			if paramMap[key] {
				p.tolerateError(msgDuplicateBinding, value)
			}
			paramMap[key] = true
		}

		if p.context.strict && param.Type() == "Identifier" && isRestrictedWord(param.str("name")) {
			p.tolerateError(msgStrictCatchVariable)
		}

		p.expect(")")
	}
	body := p.parseBlock()
	return p.finalize(node, nCatchClause(param, body))
}

func (p *parser) parseFinallyClause() *Node {
	p.expectKeyword("finally")
	return p.parseBlock()
}

func (p *parser) parseTryStatement() *Node {
	node := p.createNode()
	p.expectKeyword("try")

	block := p.parseBlock()
	var handler, finalizer *Node
	if p.matchKeyword("catch") {
		handler = p.parseCatchClause()
	}
	if p.matchKeyword("finally") {
		finalizer = p.parseFinallyClause()
	}

	if handler == nil && finalizer == nil {
		p.throwError(msgNoCatchOrFinally)
	}
	return p.finalize(node, nTryStatement(block, handler, finalizer))
}

// https://tc39.github.io/ecma262/#sec-debugger-statement

func (p *parser) parseDebuggerStatement() *Node {
	node := p.createNode()
	p.expectKeyword("debugger")
	p.consumeSemicolon()
	return p.finalize(node, nDebuggerStatement())
}

// https://tc39.github.io/ecma262/#sec-ecmascript-language-statements-and-declarations

func (p *parser) parseStatement() *Node {
	switch p.lookahead.typ {
	case tokBooleanLiteral, tokNullLiteral, tokNumericLiteral, tokStringLiteral, tokTemplate, tokRegularExpression:
		return p.parseExpressionStatement()

	case tokPunctuator:
		switch p.lookahead.str {
		case "{":
			return p.parseBlock()
		case ";":
			return p.parseEmptyStatement()
		}
		return p.parseExpressionStatement()

	case tokIdentifier:
		if p.matchAsyncFunction() {
			return p.parseFunctionDeclaration(false)
		}
		return p.parseLabelledStatement()

	case tokKeyword:
		switch p.lookahead.str {
		case "break":
			return p.parseBreakStatement()
		case "continue":
			return p.parseContinueStatement()
		case "debugger":
			return p.parseDebuggerStatement()
		case "do":
			return p.parseDoWhileStatement()
		case "for":
			return p.parseForStatement()
		case "function":
			return p.parseFunctionDeclaration(false)
		case "if":
			return p.parseIfStatement()
		case "return":
			return p.parseReturnStatement()
		case "switch":
			return p.parseSwitchStatement()
		case "throw":
			return p.parseThrowStatement()
		case "try":
			return p.parseTryStatement()
		case "var":
			return p.parseVariableStatement()
		case "while":
			return p.parseWhileStatement()
		case "with":
			return p.parseWithStatement()
		}
		return p.parseExpressionStatement()
	}
	p.throwUnexpectedToken(p.lookahead, "")
	return nil
}

// https://tc39.github.io/ecma262/#sec-function-definitions

func (p *parser) parseFunctionSourceElements() *Node {
	node := p.createNode()

	p.expect("{")
	body := p.parseDirectivePrologues()

	prevLabelSet := p.context.labelSet
	prevInIteration := p.context.inIteration
	prevInSwitch := p.context.inSwitch
	prevInFunctionBody := p.context.inFunctionBody

	p.context.labelSet = map[string]bool{}
	p.context.inIteration = false
	p.context.inSwitch = false
	p.context.inFunctionBody = true

	for p.lookahead.typ != tokEOF && !p.match("}") {
		body = append(body, p.parseStatementListItem())
	}

	p.expect("}")

	p.context.labelSet = prevLabelSet
	p.context.inIteration = prevInIteration
	p.context.inSwitch = prevInSwitch
	p.context.inFunctionBody = prevInFunctionBody

	return p.finalize(node, nBlockStatement(body))
}

// validateParam records a parameter name; param is the token or Identifier
// node reported if the name breaks a strict mode rule.
func (p *parser) validateParam(options *paramInfo, param any, name string) {
	key := "$" + name
	if p.context.strict {
		if isRestrictedWord(name) {
			options.stricted = param
			options.message = msgStrictParamName
		}
		if options.paramSet[key] {
			options.stricted = param
			options.hasDuplicateParameterNames = true
		}
	} else if options.firstRestricted == nil {
		if isRestrictedWord(name) {
			options.firstRestricted = param
			options.message = msgStrictParamName
		} else if isStrictModeReservedWord(name) {
			options.firstRestricted = param
			options.message = msgStrictReservedWord
		} else if options.paramSet[key] {
			options.stricted = param
			options.hasDuplicateParameterNames = true
		}
	}
	options.paramSet[key] = true
}

func (p *parser) parseRestElement(params *[]any) *Node {
	node := p.createNode()

	p.expect("...")
	arg := p.parsePattern(params, "")
	if p.match("=") {
		p.throwError(msgDefaultRestParameter)
	}
	if !p.match(")") {
		p.throwError(msgParameterAfterRestParameter)
	}
	return p.finalize(node, nRestElement(arg))
}

func (p *parser) parseFormalParameter(options *paramInfo) {
	var params []any
	var param *Node
	if p.match("...") {
		param = p.parseRestElement(&params)
	} else {
		param = p.parsePatternWithDefault(&params, "")
	}
	for _, t := range params {
		tok := t.(*rawToken)
		p.validateParam(options, tok, jsString(tok.value()))
	}
	options.simple = options.simple && param.class == "Identifier"
	options.params = append(options.params, param)
}

func (p *parser) parseFormalParameters(firstRestricted *rawToken) *paramInfo {
	options := &paramInfo{simple: true, params: []any{}}
	if firstRestricted != nil {
		options.firstRestricted = firstRestricted
	}

	p.expect("(")
	if !p.match(")") {
		options.paramSet = map[string]bool{}
		for p.lookahead.typ != tokEOF {
			p.parseFormalParameter(options)
			if p.match(")") {
				break
			}
			p.expect(",")
			if p.match(")") {
				break
			}
		}
	}
	p.expect(")")

	if options.hasDuplicateParameterNames && (p.context.strict || p.context.isAsync || !options.simple) {
		p.throwError(msgDuplicateParameter)
	}
	return options
}

func (p *parser) matchAsyncFunction() bool {
	if !p.matchContextualKeyword("async") {
		return false
	}
	state := p.scanner.saveState()
	p.scanner.scanComments()
	next := p.scanner.lex()
	p.scanner.restoreState(state)
	return state.lineNumber == next.lineNumber && next.typ == tokKeyword && next.str == "function"
}

// restrictedName checks a function name: in strict code eval and arguments
// are tolerated errors; otherwise the token is remembered (with its message)
// in case the body turns out to be strict.
func (p *parser) restrictedName(token *rawToken) (*rawToken, string) {
	if p.context.strict {
		if isRestrictedWord(token.str) {
			p.tolerateUnexpectedToken(token, msgStrictFunctionName)
		}
		return nil, ""
	}
	if isRestrictedWord(token.str) {
		return token, msgStrictFunctionName
	}
	if isStrictModeReservedWord(token.str) {
		return token, msgStrictReservedWord
	}
	return nil, ""
}

// parseFunctionRest parses the parameters and body of a function whose name
// (if any) has been read, restoring the context afterwards.
func (p *parser) parseFunctionRest(firstRestricted *rawToken, message string) (*paramInfo, *Node) {
	formalParameters := p.parseFormalParameters(firstRestricted)
	stricted := formalParameters.stricted
	restricted := formalParameters.firstRestricted
	if formalParameters.message != "" {
		message = formalParameters.message
	}

	prevStrict := p.context.strict
	prevAllowStrictDirective := p.context.allowStrictDirective
	p.context.allowStrictDirective = formalParameters.simple
	body := p.parseFunctionSourceElements()
	if p.context.strict && restricted != nil {
		p.throwUnexpectedToken(restricted, message)
	}
	if p.context.strict && stricted != nil {
		p.tolerateUnexpectedToken(stricted, message)
	}
	p.context.strict = prevStrict
	p.context.allowStrictDirective = prevAllowStrictDirective
	return formalParameters, body
}

func (p *parser) parseFunctionDeclaration(identifierIsOptional bool) *Node {
	node := p.createNode()

	isAsync := p.matchContextualKeyword("async")
	if isAsync {
		if p.context.inIteration {
			p.tolerateError(msgAsyncFunctionInSingleStatementContext)
		}
		p.nextToken()
	}

	p.expectKeyword("function")

	isGenerator := p.match("*")
	if isGenerator {
		p.nextToken()
	}

	var id *Node
	var firstRestricted *rawToken
	var message string
	if !identifierIsOptional || !p.match("(") {
		token := p.lookahead
		id = p.parseVariableIdentifier("")
		firstRestricted, message = p.restrictedName(token)
	}

	prevIsAsync := p.context.isAsync
	prevAllowYield := p.context.allowYield
	p.context.isAsync = isAsync
	p.context.allowYield = !isGenerator

	params, body := p.parseFunctionRest(firstRestricted, message)

	p.context.isAsync = prevIsAsync
	p.context.allowYield = prevAllowYield

	return p.finalize(node, nFunction("FunctionDeclaration", id, params.params, body, isGenerator, isAsync))
}

func (p *parser) parseFunctionExpression() *Node {
	node := p.createNode()

	isAsync := p.matchContextualKeyword("async")
	if isAsync {
		p.nextToken()
	}

	p.expectKeyword("function")

	isGenerator := p.match("*")
	if isGenerator {
		p.nextToken()
	}

	var id *Node
	var firstRestricted *rawToken
	var message string

	prevIsAsync := p.context.isAsync
	prevAllowYield := p.context.allowYield
	p.context.isAsync = isAsync
	p.context.allowYield = !isGenerator

	if !p.match("(") {
		token := p.lookahead
		if !p.context.strict && !isGenerator && p.matchKeyword("yield") {
			id = p.parseIdentifierName()
		} else {
			id = p.parseVariableIdentifier("")
		}
		firstRestricted, message = p.restrictedName(token)
	}

	params, body := p.parseFunctionRest(firstRestricted, message)

	p.context.isAsync = prevIsAsync
	p.context.allowYield = prevAllowYield

	return p.finalize(node, nFunction("FunctionExpression", id, params.params, body, isGenerator, isAsync))
}

// https://tc39.github.io/ecma262/#sec-directive-prologues-and-the-use-strict-directive

func (p *parser) parseDirective() *Node {
	token := p.lookahead

	node := p.createNode()
	expr := p.parseExpression()
	directive := ""
	if expr.Type() == "Literal" {
		raw := Units(p.getTokenRaw(token))
		if len(raw) >= 2 {
			directive = unitsString(raw[1 : len(raw)-1])
		}
	}
	p.consumeSemicolon()

	if directive != "" {
		return p.finalize(node, nDirective(expr, directive))
	}
	return p.finalize(node, nExpressionStatement(expr))
}

func (p *parser) parseDirectivePrologues() []any {
	var firstRestricted *rawToken

	body := []any{}
	for {
		token := p.lookahead
		if token.typ != tokStringLiteral {
			break
		}

		statement := p.parseDirective()
		body = append(body, statement)
		directive, ok := statement.Get("directive").(string)
		if !ok {
			break
		}

		if directive == "use strict" {
			p.context.strict = true
			if firstRestricted != nil {
				p.tolerateUnexpectedToken(firstRestricted, msgStrictOctalLiteral)
			}
			if !p.context.allowStrictDirective {
				p.tolerateUnexpectedToken(token, msgIllegalLanguageModeDirective)
			}
		} else if firstRestricted == nil && token.octal {
			firstRestricted = token
		}
	}
	return body
}

// https://tc39.github.io/ecma262/#sec-method-definitions

func (p *parser) qualifiedPropertyName(token *rawToken) bool {
	switch token.typ {
	case tokIdentifier, tokStringLiteral, tokBooleanLiteral, tokNullLiteral, tokNumericLiteral, tokKeyword:
		return true
	case tokPunctuator:
		return token.str == "["
	}
	return false
}

func (p *parser) parseGetterMethod() *Node {
	node := p.createNode()

	prevAllowYield := p.context.allowYield
	p.context.allowYield = true
	formalParameters := p.parseFormalParameters(nil)
	if len(formalParameters.params) > 0 {
		p.tolerateError(msgBadGetterArity)
	}
	method := p.parsePropertyMethod(formalParameters)
	p.context.allowYield = prevAllowYield

	return p.finalize(node, nFunction("FunctionExpression", nil, formalParameters.params, method, false, false))
}

func (p *parser) parseSetterMethod() *Node {
	node := p.createNode()

	prevAllowYield := p.context.allowYield
	p.context.allowYield = true
	formalParameters := p.parseFormalParameters(nil)
	if len(formalParameters.params) != 1 {
		p.tolerateError(msgBadSetterArity)
	} else if formalParameters.params[0].(*Node).class == "RestElement" {
		p.tolerateError(msgBadSetterRestParameter)
	}
	method := p.parsePropertyMethod(formalParameters)
	p.context.allowYield = prevAllowYield

	return p.finalize(node, nFunction("FunctionExpression", nil, formalParameters.params, method, false, false))
}

func (p *parser) parseGeneratorMethod() *Node {
	node := p.createNode()

	prevAllowYield := p.context.allowYield
	p.context.allowYield = true
	params := p.parseFormalParameters(nil)
	p.context.allowYield = false
	method := p.parsePropertyMethod(params)
	p.context.allowYield = prevAllowYield

	return p.finalize(node, nFunction("FunctionExpression", nil, params.params, method, true, false))
}

// https://tc39.github.io/ecma262/#sec-generator-function-definitions

func (p *parser) isStartOfExpression() bool {
	value := p.lookahead.str
	switch p.lookahead.typ {
	case tokPunctuator:
		switch value {
		case "[", "(", "{", "+", "-", "!", "~", "++", "--", "/", "/=":
			return true
		}
		return p.jsx && value == "<"
	case tokKeyword:
		switch value {
		case "class", "delete", "function", "let", "new", "super", "this", "typeof", "void", "yield":
			return true
		}
		return false
	}
	return true
}

func (p *parser) parseYieldExpression() *Node {
	node := p.createNode()
	p.expectKeyword("yield")

	var argument *Node
	delegate := false
	if !p.hasLineTerminator {
		prevAllowYield := p.context.allowYield
		p.context.allowYield = false
		delegate = p.match("*")
		if delegate {
			p.nextToken()
			argument = p.parseAssignmentExpression()
		} else if p.isStartOfExpression() {
			argument = p.parseAssignmentExpression()
		}
		p.context.allowYield = prevAllowYield
	}
	return p.finalize(node, nYieldExpression(argument, delegate))
}

// https://tc39.github.io/ecma262/#sec-class-definitions

func (p *parser) parseClassElement(hasConstructor *bool) *Node {
	token := p.lookahead
	node := p.createNode()

	kind := ""
	var key, value *Node
	computed, method, isStatic, isAsync, isGenerator := false, false, false, false, false

	if p.match("*") {
		p.nextToken()
	} else {
		computed = p.match("[")
		key = p.parseObjectPropertyKey()
		if key.str("name") == "static" && (p.qualifiedPropertyName(p.lookahead) || p.match("*")) {
			token = p.lookahead
			isStatic = true
			computed = p.match("[")
			if p.match("*") {
				p.nextToken()
			} else {
				key = p.parseObjectPropertyKey()
			}
		}
		if token.typ == tokIdentifier && !p.hasLineTerminator && token.str == "async" {
			punctuator := p.lookahead.str
			if (punctuator != ":" && punctuator != "(") || p.lookahead.isNum {
				isAsync = true
				isGenerator = p.match("*")
				if isGenerator {
					p.nextToken()
				}
				token = p.lookahead
				computed = p.match("[")
				key = p.parseObjectPropertyKey()
				if token.typ == tokIdentifier && token.str == "constructor" {
					p.tolerateUnexpectedToken(token, msgConstructorIsAsync)
				}
			}
		}
	}

	lookaheadPropertyKey := p.qualifiedPropertyName(p.lookahead)
	if token.typ == tokIdentifier {
		if token.str == "get" && lookaheadPropertyKey {
			kind = "get"
			computed = p.match("[")
			key = p.parseObjectPropertyKey()
			p.context.allowYield = false
			value = p.parseGetterMethod()
		} else if token.str == "set" && lookaheadPropertyKey {
			kind = "set"
			computed = p.match("[")
			key = p.parseObjectPropertyKey()
			value = p.parseSetterMethod()
		}
	} else if token.typ == tokPunctuator && token.str == "*" && lookaheadPropertyKey {
		kind = "init"
		computed = p.match("[")
		key = p.parseObjectPropertyKey()
		value = p.parseGeneratorMethod()
		method = true
	}

	if kind == "" && key != nil && p.match("(") {
		prevInClassConstructor := p.context.inClassConstructor
		p.context.inClassConstructor = !token.isNum && token.str == "constructor"
		kind = "init"
		if isAsync {
			value = p.parsePropertyMethodAsyncFunction(isGenerator)
		} else {
			value = p.parsePropertyMethodFunction(isGenerator)
		}
		p.context.inClassConstructor = prevInClassConstructor
		method = true
	}

	if kind == "" {
		p.throwUnexpectedToken(p.lookahead, "")
	}
	if kind == "init" {
		kind = "method"
	}

	if !computed {
		if isStatic && isPropertyKey(key, "prototype") {
			p.throwUnexpectedToken(token, msgStaticPrototype)
		}
		if !isStatic && isPropertyKey(key, "constructor") {
			if kind != "method" || !method || (value != nil && value.Get("generator").(bool)) {
				p.throwUnexpectedToken(token, msgConstructorSpecialMethod)
			}
			if *hasConstructor {
				p.throwUnexpectedToken(token, msgDuplicateConstructor)
			} else {
				*hasConstructor = true
			}
			kind = "constructor"
		}
	}
	return p.finalize(node, nMethodDefinition(key, computed, value, kind, isStatic))
}

func (p *parser) parseClassElementList() []any {
	body := []any{}
	hasConstructor := false

	p.expect("{")
	for !p.match("}") {
		if p.match(";") {
			p.nextToken()
		} else {
			body = append(body, p.parseClassElement(&hasConstructor))
		}
	}
	p.expect("}")
	return body
}

func (p *parser) parseClassBody() *Node {
	node := p.createNode()
	elementList := p.parseClassElementList()
	return p.finalize(node, nClassBody(elementList))
}

func (p *parser) parseClassDeclaration(identifierIsOptional bool) *Node {
	node := p.createNode()

	prevStrict := p.context.strict
	p.context.strict = true
	p.expectKeyword("class")

	var id *Node
	if !identifierIsOptional || p.lookahead.typ == tokIdentifier {
		id = p.parseVariableIdentifier("")
	}
	var superClass *Node
	if p.matchKeyword("extends") {
		p.nextToken()
		superClass = p.isolateCoverGrammar(p.parseLeftHandSideExpressionAllowCall)
	}
	classBody := p.parseClassBody()
	p.context.strict = prevStrict

	return p.finalize(node, nClass("ClassDeclaration", id, superClass, classBody))
}

func (p *parser) parseClassExpression() *Node {
	node := p.createNode()

	prevStrict := p.context.strict
	p.context.strict = true
	p.expectKeyword("class")
	var id *Node
	if p.lookahead.typ == tokIdentifier {
		id = p.parseVariableIdentifier("")
	}
	var superClass *Node
	if p.matchKeyword("extends") {
		p.nextToken()
		superClass = p.isolateCoverGrammar(p.parseLeftHandSideExpressionAllowCall)
	}
	classBody := p.parseClassBody()
	p.context.strict = prevStrict

	return p.finalize(node, nClass("ClassExpression", id, superClass, classBody))
}

// https://tc39.github.io/ecma262/#sec-scripts
// https://tc39.github.io/ecma262/#sec-modules

func (p *parser) parseModule() *Node {
	p.context.strict = true
	p.context.isModule = true
	p.scanner.isModule = true
	node := p.createNode()
	body := p.parseDirectivePrologues()
	for p.lookahead.typ != tokEOF {
		body = append(body, p.parseStatementListItem())
	}
	return p.finalize(node, nProgram(body, "module"))
}

func (p *parser) parseScript() *Node {
	node := p.createNode()
	body := p.parseDirectivePrologues()
	for p.lookahead.typ != tokEOF {
		body = append(body, p.parseStatementListItem())
	}
	return p.finalize(node, nProgram(body, "script"))
}

// https://tc39.github.io/ecma262/#sec-imports

func (p *parser) parseModuleSpecifier() *Node {
	node := p.createNode()

	if p.lookahead.typ != tokStringLiteral {
		p.throwError(msgInvalidModuleSpecifier)
	}

	token := p.nextToken()
	return p.finalize(node, nLiteral(token.value(), p.getTokenRaw(token)))
}

// import {<foo as bar>} ...;
func (p *parser) parseImportSpecifier() *Node {
	node := p.createNode()

	var imported, local *Node
	if p.lookahead.typ == tokIdentifier {
		imported = p.parseVariableIdentifier("")
		local = imported
		if p.matchContextualKeyword("as") {
			p.nextToken()
			local = p.parseVariableIdentifier("")
		}
	} else {
		imported = p.parseIdentifierName()
		local = imported
		if p.matchContextualKeyword("as") {
			p.nextToken()
			local = p.parseVariableIdentifier("")
		} else {
			p.throwUnexpectedToken(p.nextToken(), "")
		}
	}
	return p.finalize(node, nImportSpecifier(local, imported))
}

// {foo, bar as bas}
func (p *parser) parseNamedImports() []any {
	p.expect("{")
	specifiers := []any{}
	for !p.match("}") {
		specifiers = append(specifiers, p.parseImportSpecifier())
		if !p.match("}") {
			p.expect(",")
		}
	}
	p.expect("}")
	return specifiers
}

// import <foo> ...;
func (p *parser) parseImportDefaultSpecifier() *Node {
	node := p.createNode()
	local := p.parseIdentifierName()
	return p.finalize(node, nImportDefaultSpecifier(local))
}

// import <* as foo> ...;
func (p *parser) parseImportNamespaceSpecifier() *Node {
	node := p.createNode()

	p.expect("*")
	if !p.matchContextualKeyword("as") {
		p.throwError(msgNoAsAfterImportNamespace)
	}
	p.nextToken()
	local := p.parseIdentifierName()
	return p.finalize(node, nImportNamespaceSpecifier(local))
}

// throwMissingFrom reports a missing "from" clause; the message names the
// lookahead token when its value is truthy.
func (p *parser) throwMissingFrom() {
	value := p.lookahead.value()
	truthy := false
	switch v := value.(type) {
	case string:
		truthy = v != ""
	case float64:
		truthy = v != 0 && v == v
	}
	if truthy {
		p.throwError(msgUnexpectedToken, value)
	}
	p.throwError(msgMissingFromClause, value)
}

func (p *parser) parseImportDeclaration() *Node {
	if p.context.inFunctionBody {
		p.throwError(msgIllegalImportDeclaration)
	}

	node := p.createNode()
	p.expectKeyword("import")

	var src *Node
	specifiers := []any{}
	if p.lookahead.typ == tokStringLiteral {
		// import 'foo';
		src = p.parseModuleSpecifier()
	} else {
		if p.match("{") {
			// import {bar}
			specifiers = append(specifiers, p.parseNamedImports()...)
		} else if p.match("*") {
			// import * as foo
			specifiers = append(specifiers, p.parseImportNamespaceSpecifier())
		} else if isIdentifierName(p.lookahead) && !p.matchKeyword("default") {
			// import foo
			specifiers = append(specifiers, p.parseImportDefaultSpecifier())
			if p.match(",") {
				p.nextToken()
				if p.match("*") {
					// import foo, * as foo
					specifiers = append(specifiers, p.parseImportNamespaceSpecifier())
				} else if p.match("{") {
					// import foo, {bar}
					specifiers = append(specifiers, p.parseNamedImports()...)
				} else {
					p.throwUnexpectedToken(p.lookahead, "")
				}
			}
		} else {
			p.throwUnexpectedToken(p.nextToken(), "")
		}

		if !p.matchContextualKeyword("from") {
			p.throwMissingFrom()
		}
		p.nextToken()
		src = p.parseModuleSpecifier()
	}
	p.consumeSemicolon()
	return p.finalize(node, nImportDeclaration(specifiers, src))
}

// https://tc39.github.io/ecma262/#sec-exports

func (p *parser) parseExportSpecifier() *Node {
	node := p.createNode()

	local := p.parseIdentifierName()
	exported := local
	if p.matchContextualKeyword("as") {
		p.nextToken()
		exported = p.parseIdentifierName()
	}
	return p.finalize(node, nExportSpecifier(local, exported))
}

func (p *parser) parseExportDeclaration() *Node {
	if p.context.inFunctionBody {
		p.throwError(msgIllegalExportDeclaration)
	}

	node := p.createNode()
	p.expectKeyword("export")

	switch {
	case p.matchKeyword("default"):
		// export default ...
		p.nextToken()
		var declaration *Node
		switch {
		case p.matchKeyword("function"):
			// export default function foo () {}
			// export default function () {}
			declaration = p.parseFunctionDeclaration(true)
		case p.matchKeyword("class"):
			// export default class foo {}
			declaration = p.parseClassDeclaration(true)
		case p.matchContextualKeyword("async"):
			// export default async function f () {}
			// export default async function () {}
			// export default async x => x
			if p.matchAsyncFunction() {
				declaration = p.parseFunctionDeclaration(true)
			} else {
				declaration = p.parseAssignmentExpression()
			}
		default:
			if p.matchContextualKeyword("from") {
				p.throwError(msgUnexpectedToken, p.lookahead.value())
			}
			// export default {};
			// export default [];
			// export default (1 + 2);
			switch {
			case p.match("{"):
				declaration = p.parseObjectInitializer()
			case p.match("["):
				declaration = p.parseArrayInitializer()
			default:
				declaration = p.parseAssignmentExpression()
			}
			p.consumeSemicolon()
		}
		return p.finalize(node, nExportDefaultDeclaration(declaration))

	case p.match("*"):
		// export * from 'foo';
		p.nextToken()
		if !p.matchContextualKeyword("from") {
			p.throwMissingFrom()
		}
		p.nextToken()
		src := p.parseModuleSpecifier()
		p.consumeSemicolon()
		return p.finalize(node, nExportAllDeclaration(src))

	case p.lookahead.typ == tokKeyword:
		// export var f = 1;
		var declaration *Node
		switch p.lookahead.str {
		case "let", "const":
			declaration = p.parseLexicalDeclaration(false)
		case "var", "class", "function":
			declaration = p.parseStatementListItem()
		default:
			p.throwUnexpectedToken(p.lookahead, "")
		}
		return p.finalize(node, nExportNamedDeclaration(declaration, []any{}, nil))

	case p.matchAsyncFunction():
		declaration := p.parseFunctionDeclaration(false)
		return p.finalize(node, nExportNamedDeclaration(declaration, []any{}, nil))
	}

	specifiers := []any{}
	var source *Node
	isExportFromIdentifier := false

	p.expect("{")
	for !p.match("}") {
		isExportFromIdentifier = isExportFromIdentifier || p.matchKeyword("default")
		specifiers = append(specifiers, p.parseExportSpecifier())
		if !p.match("}") {
			p.expect(",")
		}
	}
	p.expect("}")

	if p.matchContextualKeyword("from") {
		// export {default} from 'foo';
		// export {foo} from 'foo';
		p.nextToken()
		source = p.parseModuleSpecifier()
		p.consumeSemicolon()
	} else if isExportFromIdentifier {
		// export {default}; // missing fromClause
		p.throwMissingFrom()
	} else {
		// export {foo};
		p.consumeSemicolon()
	}
	return p.finalize(node, nExportNamedDeclaration(nil, specifiers, source))
}
