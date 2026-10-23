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

// Go port of src/jsx-parser.ts.

package esprima

import "strings"

// JSX extension of the parser, used when Options.JSX is set.

// metaJSXElement is an element whose children are still being parsed.
type metaJSXElement struct {
	node     marker
	opening  *Node
	closing  *Node
	children []any
}

// qualifiedElementName returns the fully qualified element name, e.g.
// "svg:path" for <svg:path>.
func qualifiedElementName(elementName *Node) string {
	if elementName == nil {
		internalError("Cannot read properties of undefined (reading 'type')")
	}
	switch elementName.Type() {
	case "JSXIdentifier":
		return elementName.str("name")
	case "JSXNamespacedName":
		return qualifiedElementName(elementName.node("namespace")) + ":" + qualifiedElementName(elementName.node("name"))
	case "JSXMemberExpression":
		return qualifiedElementName(elementName.node("object")) + "." + qualifiedElementName(elementName.node("property"))
	}
	return "undefined"
}

func (p *parser) startJSX() {
	// Unwind the scanner to before the lookahead token.
	p.scanner.index = p.startMarker.index
	p.scanner.lineNumber = p.startMarker.line
	p.scanner.lineStart = p.startMarker.index - p.startMarker.column
}

func (p *parser) finishJSX() {
	// Prime the next lookahead.
	p.nextToken()
}

func (p *parser) reenterJSX() {
	p.startJSX()
	p.expectJSX("}")
	// Pop the closing '}' added from the lookahead.
	if p.config.tokens {
		p.popToken()
	}
}

func (p *parser) createJSXNode() marker {
	p.collectComments()
	return p.createJSXChildNode()
}

func (p *parser) createJSXChildNode() marker {
	return marker{index: p.scanner.index, line: p.scanner.lineNumber, column: p.scanner.index - p.scanner.lineStart}
}

// scanXHTMLEntity reads a character reference after '&' inside an attribute
// string (quote is its quote character) and returns its replacement text, or
// the characters read when it is not a reference the original resolves.
func (p *parser) scanXHTMLEntity(quote int) string {
	s := p.scanner
	result := []uint16{'&'}

	valid := true
	terminated := false
	numeric := false
	hex := false

	for !s.eof() && valid && !terminated {
		ch := s.cu(s.index)
		if ch == quote {
			break
		}
		terminated = ch == ';'
		result = append(result, uint16(ch))
		s.index++
		if !terminated {
			switch len(result) {
			case 2:
				// e.g. '&#123;'
				numeric = ch == '#'
			case 3:
				if numeric {
					// e.g. '&#x41;'
					hex = ch == 'x'
					valid = hex || isDecimalDigit(ch)
					numeric = numeric && !hex
				}
			default:
				valid = valid && !(numeric && !isDecimalDigit(ch))
				valid = valid && !(hex && !isHexDigit(ch))
			}
		}
	}

	if valid && terminated && len(result) > 2 {
		// e.g. '&#x41;' becomes just '#x41'
		str := unitsString(result[1 : len(result)-1])
		switch {
		case numeric && len(str) > 1:
			return fromCharCode(parseIntLeading(str[1:], 10))
		case hex && len(str) > 2:
			return fromCharCode(parseIntLeading("0"+str[1:], 16))
		case !numeric && !hex:
			if v, ok := xhtmlEntity(str); ok {
				return v
			}
		}
	}
	return unitsString(result)
}

// lexJSX scans the next JSX token; it replaces the scanner's lex in JSX
// mode.
func (p *parser) lexJSX() *rawToken {
	s := p.scanner
	cp := s.cu(s.index)

	// < > / : = { }
	switch cp {
	case 60, 62, 47, 58, 61, 123, 125:
		value := s.slice(s.index, s.index+1)
		s.index++
		return &rawToken{typ: tokPunctuator, str: value, lineNumber: s.lineNumber, lineStart: s.lineStart, start: s.index - 1, end: s.index}
	}

	// " '
	if cp == 34 || cp == 39 {
		start := s.index
		quote := cp
		s.index++
		var str []byte
		for !s.eof() {
			ch := s.cu(s.index)
			s.index++
			if ch == quote {
				break
			} else if ch == '&' {
				str = append(str, p.scanXHTMLEntity(quote)...)
			} else {
				str = appendUnit(str, uint16(ch))
			}
		}
		return &rawToken{typ: tokStringLiteral, str: string(str), lineNumber: s.lineNumber, lineStart: s.lineStart, start: start, end: s.index}
	}

	// ... or .
	if cp == 46 {
		value := "."
		if s.cu(s.index+1) == 46 && s.cu(s.index+2) == 46 {
			value = "..."
		}
		start := s.index
		s.index += len(value)
		return &rawToken{typ: tokPunctuator, str: value, lineNumber: s.lineNumber, lineStart: s.lineStart, start: start, end: s.index}
	}

	// ` is only a placeholder: it is rescanned as a real template literal.
	if cp == 96 {
		return &rawToken{typ: tokTemplate, lineNumber: s.lineNumber, lineStart: s.lineStart, start: s.index, end: s.index}
	}

	// An identifier cannot contain a backslash but can contain a hyphen.
	if isIdentifierStart(cp) && cp != 92 {
		start := s.index
		s.index++
		for !s.eof() {
			ch := s.cu(s.index)
			if (isIdentifierPart(ch) && ch != 92) || ch == 45 {
				s.index++
			} else {
				break
			}
		}
		return &rawToken{typ: tokJSXIdentifier, str: s.slice(start, s.index), lineNumber: s.lineNumber, lineStart: s.lineStart, start: start, end: s.index}
	}

	return s.lex()
}

func (p *parser) nextJSXToken() *rawToken {
	p.collectComments()

	p.startMarker = p.createJSXChildNode()
	token := p.lexJSX()
	p.lastMarker = p.createJSXChildNode()

	if p.config.tokens {
		p.tokens = append(p.tokens, p.convertToken(token))
	}
	return token
}

func (p *parser) nextJSXText() *rawToken {
	s := p.scanner
	p.startMarker = p.createJSXChildNode()

	start := s.index
	var text []byte
	for !s.eof() {
		ch := s.cu(s.index)
		if ch == '{' || ch == '<' {
			break
		}
		s.index++
		text = appendUnit(text, uint16(ch))
		if isLineTerminator(ch) {
			s.lineNumber++
			if ch == '\r' && s.cu(s.index) == '\n' {
				s.index++
			}
			s.lineStart = s.index
		}
	}

	p.lastMarker = p.createJSXChildNode()

	token := &rawToken{typ: tokJSXText, str: string(text), lineNumber: s.lineNumber, lineStart: s.lineStart, start: start, end: s.index}
	if len(text) > 0 && p.config.tokens {
		p.tokens = append(p.tokens, p.convertToken(token))
	}
	return token
}

func (p *parser) peekJSXToken() *rawToken {
	state := p.scanner.saveState()
	p.scanner.scanComments()
	next := p.lexJSX()
	p.scanner.restoreState(state)
	return next
}

// expectJSX consumes the next JSX token, which must be the given punctuator.
func (p *parser) expectJSX(value string) {
	token := p.nextJSXToken()
	if token.typ != tokPunctuator || token.str != value {
		p.throwUnexpectedToken(token, "")
	}
}

// matchJSX reports whether the next JSX token is the given punctuator.
func (p *parser) matchJSX(value string) bool {
	next := p.peekJSXToken()
	return next.typ == tokPunctuator && next.str == value
}

func (p *parser) parseJSXIdentifier() *Node {
	node := p.createJSXNode()
	token := p.nextJSXToken()
	if token.typ != tokJSXIdentifier {
		p.throwUnexpectedToken(token, "")
	}
	return p.finalize(node, nJSXIdentifier(token.str))
}

func (p *parser) parseJSXElementName() *Node {
	node := p.createJSXNode()
	elementName := p.parseJSXIdentifier()

	if p.matchJSX(":") {
		namespace := elementName
		p.expectJSX(":")
		name := p.parseJSXIdentifier()
		elementName = p.finalize(node, nJSXNamespacedName(namespace, name))
	} else if p.matchJSX(".") {
		for p.matchJSX(".") {
			object := elementName
			p.expectJSX(".")
			property := p.parseJSXIdentifier()
			elementName = p.finalize(node, nJSXMemberExpression(object, property))
		}
	}
	return elementName
}

func (p *parser) parseJSXAttributeName() *Node {
	node := p.createJSXNode()

	identifier := p.parseJSXIdentifier()
	if p.matchJSX(":") {
		namespace := identifier
		p.expectJSX(":")
		name := p.parseJSXIdentifier()
		return p.finalize(node, nJSXNamespacedName(namespace, name))
	}
	return identifier
}

func (p *parser) parseJSXStringLiteralAttribute() *Node {
	node := p.createJSXNode()
	token := p.nextJSXToken()
	if token.typ != tokStringLiteral {
		p.throwUnexpectedToken(token, "")
	}
	return p.finalize(node, nLiteral(token.value(), p.getTokenRaw(token)))
}

func (p *parser) parseJSXExpressionAttribute() *Node {
	node := p.createJSXNode()

	p.expectJSX("{")
	p.finishJSX()

	if p.match("}") {
		p.tolerateError(msgJSXEmptyAttributeExpression)
	}

	expression := p.parseAssignmentExpression()
	p.reenterJSX()
	return p.finalize(node, nJSXExpressionContainer(expression))
}

func (p *parser) parseJSXAttributeValue() *Node {
	if p.matchJSX("{") {
		return p.parseJSXExpressionAttribute()
	}
	if p.matchJSX("<") {
		return p.parseJSXElement()
	}
	return p.parseJSXStringLiteralAttribute()
}

func (p *parser) parseJSXNameValueAttribute() *Node {
	node := p.createJSXNode()
	name := p.parseJSXAttributeName()
	var value *Node
	if p.matchJSX("=") {
		p.expectJSX("=")
		value = p.parseJSXAttributeValue()
	}
	return p.finalize(node, nJSXAttribute(name, value))
}

func (p *parser) parseJSXSpreadAttribute() *Node {
	node := p.createJSXNode()
	p.expectJSX("{")
	p.expectJSX("...")

	p.finishJSX()
	argument := p.parseAssignmentExpression()
	p.reenterJSX()
	return p.finalize(node, nJSXSpreadAttribute(argument))
}

func (p *parser) parseJSXAttributes() []any {
	attributes := []any{}
	for !p.matchJSX("/") && !p.matchJSX(">") {
		if p.matchJSX("{") {
			attributes = append(attributes, p.parseJSXSpreadAttribute())
		} else {
			attributes = append(attributes, p.parseJSXNameValueAttribute())
		}
	}
	return attributes
}

func (p *parser) parseJSXOpeningElement() *Node {
	node := p.createJSXNode()

	p.expectJSX("<")
	if p.matchJSX(">") {
		p.expectJSX(">")
		return p.finalize(node, nJSXOpeningFragment(false))
	}

	name := p.parseJSXElementName()
	attributes := p.parseJSXAttributes()
	selfClosing := p.matchJSX("/")
	if selfClosing {
		p.expectJSX("/")
	}
	p.expectJSX(">")
	return p.finalize(node, nJSXOpeningElement(name, selfClosing, attributes))
}

func (p *parser) parseJSXBoundaryElement() *Node {
	node := p.createJSXNode()

	p.expectJSX("<")
	if p.matchJSX("/") {
		p.expectJSX("/")
		if p.matchJSX(">") {
			p.expectJSX(">")
			return p.finalize(node, nJSXClosingFragment())
		}
		elementName := p.parseJSXElementName()
		p.expectJSX(">")
		return p.finalize(node, nJSXClosingElement(elementName))
	}

	name := p.parseJSXElementName()
	attributes := p.parseJSXAttributes()
	selfClosing := p.matchJSX("/")
	if selfClosing {
		p.expectJSX("/")
	}
	p.expectJSX(">")
	return p.finalize(node, nJSXOpeningElement(name, selfClosing, attributes))
}

func (p *parser) parseJSXEmptyExpression() *Node {
	node := p.createJSXChildNode()
	p.collectComments()
	p.lastMarker = p.createJSXChildNode()
	return p.finalize(node, nJSXEmptyExpression())
}

func (p *parser) parseJSXExpressionContainer() *Node {
	node := p.createJSXNode()
	p.expectJSX("{")

	var expression *Node
	if p.matchJSX("}") {
		expression = p.parseJSXEmptyExpression()
		p.expectJSX("}")
	} else {
		p.finishJSX()
		expression = p.parseAssignmentExpression()
		p.reenterJSX()
	}
	return p.finalize(node, nJSXExpressionContainer(expression))
}

func (p *parser) parseJSXChildren() []any {
	children := []any{}

	for !p.scanner.eof() {
		node := p.createJSXChildNode()
		token := p.nextJSXText()
		if token.start < token.end {
			child := p.finalize(node, nJSXText(token.str, p.getTokenRaw(token)))
			children = append(children, child)
		}
		if p.scanner.cu(p.scanner.index) != '{' {
			break
		}
		children = append(children, p.parseJSXExpressionContainer())
	}
	return children
}

func (p *parser) parseComplexJSXElement(el *metaJSXElement) *metaJSXElement {
	var stack []*metaJSXElement

	for !p.scanner.eof() {
		el.children = append(el.children, p.parseJSXChildren()...)
		node := p.createJSXChildNode()
		element := p.parseJSXBoundaryElement()
		if element.Type() == "JSXOpeningElement" {
			if element.Get("selfClosing").(bool) {
				child := p.finalize(node, nJSXElement(element, []any{}, nil))
				el.children = append(el.children, child)
			} else {
				stack = append(stack, el)
				el = &metaJSXElement{node: node, opening: element, children: []any{}}
			}
		}
		if element.Type() == "JSXClosingElement" {
			el.closing = element
			open := qualifiedElementName(el.opening.node("name"))
			closing := qualifiedElementName(el.closing.node("name"))
			if open != closing {
				p.tolerateError(msgJSXClosingTagMismatch, open)
			}
			if len(stack) == 0 {
				break
			}
			child := p.finalize(el.node, nJSXElement(el.opening, el.children, el.closing))
			el = stack[len(stack)-1]
			el.children = append(el.children, child)
			stack = stack[:len(stack)-1]
		}
		if element.Type() == "JSXClosingFragment" {
			el.closing = element
			if el.opening.Type() != "JSXOpeningFragment" {
				p.tolerateError(msgJSXFragmentClosingMismatch)
			} else {
				break
			}
		}
	}
	return el
}

func (p *parser) parseJSXElement() *Node {
	node := p.createJSXNode()

	opening := p.parseJSXOpeningElement()
	children := []any{}
	var closing *Node

	if !opening.Get("selfClosing").(bool) {
		el := p.parseComplexJSXElement(&metaJSXElement{node: node, opening: opening, children: children})
		children = el.children
		closing = el.closing
	}
	return p.finalize(node, nJSXElement(opening, children, closing))
}

func (p *parser) parseJSXRoot() *Node {
	// Pop the opening '<' added from the lookahead.
	if p.config.tokens {
		p.popToken()
	}

	p.startJSX()
	element := p.parseJSXElement()
	p.finishJSX()
	return element
}

// xhtmlEntity looks a character reference name up the way the original's
// plain-object entity table does, including the members every JavaScript
// object inherits from Object.prototype.
func xhtmlEntity(name string) (string, bool) {
	if v, ok := xhtmlEntities[name]; ok {
		return v, true
	}
	if v, ok := objectPrototypeMembers[name]; ok {
		return v, true
	}
	return "", false
}

// objectPrototypeMembers is the string conversion of the members of
// Object.prototype, which an entity name such as "constructor" resolves to.
var objectPrototypeMembers = map[string]string{
	"constructor":          nativeFunction("Object"),
	"__defineGetter__":     nativeFunction("__defineGetter__"),
	"__defineSetter__":     nativeFunction("__defineSetter__"),
	"hasOwnProperty":       nativeFunction("hasOwnProperty"),
	"__lookupGetter__":     nativeFunction("__lookupGetter__"),
	"__lookupSetter__":     nativeFunction("__lookupSetter__"),
	"isPrototypeOf":        nativeFunction("isPrototypeOf"),
	"propertyIsEnumerable": nativeFunction("propertyIsEnumerable"),
	"toString":             nativeFunction("toString"),
	"valueOf":              nativeFunction("valueOf"),
	"toLocaleString":       nativeFunction("toLocaleString"),
	"__proto__":            "[object Object]",
}

func nativeFunction(name string) string {
	return strings.Join([]string{"function ", name, "() { [native code] }"}, "")
}
