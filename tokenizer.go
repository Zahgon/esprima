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

// Go port of src/tokenizer.ts.

package esprima

// reader tracks the tokens read so far to decide whether a slash starts a
// regular expression or is a division operator.
// See https://github.com/mozilla/sweet.js/wiki/design.
type reader struct {
	// values holds the punctuator or keyword of each token, or "" for any
	// other token.
	values []string
	isPunc []bool
	curly  int
	paren  int
}

func newReader() *reader {
	return &reader{curly: -1, paren: -1}
}

// at returns the value at index i and whether it is a punctuator or keyword
// (an index out of range is undefined, like any other token).
func (r *reader) at(i int) (string, bool) {
	if i < 0 || i >= len(r.values) {
		return "", false
	}
	return r.values[i], r.isPunc[i]
}

// beforeFunctionExpression reports whether a function after this token is
// an expression.
func beforeFunctionExpression(t string) bool {
	switch t {
	case "(", "{", "[", "in", "typeof", "instanceof", "new",
		"return", "case", "delete", "throw", "void",
		// assignment operators
		"=", "+=", "-=", "*=", "**=", "/=", "%=", "<<=", ">>=", ">>>=",
		"&=", "|=", "^=", ",",
		// binary/unary operators
		"+", "-", "*", "**", "/", "%", "++", "--", "<<", ">>", ">>>", "&",
		"|", "^", "!", "~", "&&", "||", "??", "?", ":", "===", "==", ">=",
		"<=", "<", ">", "!=", "!==":
		return true
	}
	return false
}

func (r *reader) isRegexStart() bool {
	if len(r.values) == 0 {
		// The previous value is undefined, which is not null.
		return true
	}
	previous, known := r.at(len(r.values) - 1)
	if !known {
		return false
	}
	regex := true

	switch previous {
	case "this", "]":
		regex = false
	case ")":
		keyword, _ := r.at(r.paren - 1)
		regex = keyword == "if" || keyword == "while" || keyword == "for" || keyword == "with"
	case "}":
		// Dividing a function by anything makes little sense, but it has
		// to be checked.
		regex = true
		if v, _ := r.at(r.curly - 3); v == "function" {
			// Anonymous function, e.g. function(){} /42
			check, ok := r.at(r.curly - 4)
			regex = ok && !beforeFunctionExpression(check)
		} else if v, _ := r.at(r.curly - 4); v == "function" {
			// Named function, e.g. function f(){} /42/
			check, ok := r.at(r.curly - 5)
			regex = !ok || !beforeFunctionExpression(check)
		}
	}
	return regex
}

func (r *reader) push(token *rawToken) {
	if token.typ == tokPunctuator || token.typ == tokKeyword {
		if token.str == "{" {
			r.curly = len(r.values)
		} else if token.str == "(" {
			r.paren = len(r.values)
		}
		r.values = append(r.values, token.str)
		r.isPunc = append(r.isPunc, true)
	} else {
		r.values = append(r.values, "")
		r.isPunc = append(r.isPunc, false)
	}
}

// tokenizer produces the token list of Tokenize.
type tokenizer struct {
	errorHandler *errorHandler
	scanner      *scanner
	trackRange   bool
	trackLoc     bool
	buffer       []*Node
	reader       *reader
}

func newTokenizer(code []uint16, opts *Options) *tokenizer {
	t := &tokenizer{errorHandler: &errorHandler{}, reader: newReader()}
	if opts != nil {
		t.errorHandler.tolerant = opts.Tolerant
		t.trackRange = opts.Range
		t.trackLoc = opts.Loc
	}
	t.scanner = newScanner(code, t.errorHandler)
	if opts != nil {
		t.scanner.trackComment = opts.Comment
	}
	return t
}

// getNextToken returns the next token or comment entry, or nil at the end.
func (t *tokenizer) getNextToken() *Node {
	if len(t.buffer) == 0 {
		comments := t.scanner.scanComments()
		if t.scanner.trackComment {
			for _, e := range comments {
				typ := "LineComment"
				if e.multiLine {
					typ = "BlockComment"
				}
				comment := newNode("", "type", typ, "value", t.scanner.slice(e.slice[0], e.slice[1]))
				if t.trackRange {
					comment.Set("range", []any{e.rng[0], e.rng[1]})
				}
				if t.trackLoc {
					comment.Set("loc", locNode(e.loc.startLine, e.loc.startColumn, e.loc.endLine, e.loc.endColumn))
				}
				t.buffer = append(t.buffer, comment)
			}
		}

		if !t.scanner.eof() {
			startLine := t.scanner.lineNumber
			startColumn := t.scanner.index - t.scanner.lineStart

			var token *rawToken
			if t.scanner.cu(t.scanner.index) == '/' && t.reader.isRegexStart() {
				state := t.scanner.saveState()
				if err := catch(func() { token = t.scanner.scanRegExp() }); err != nil {
					t.scanner.restoreState(state)
					token = t.scanner.lex()
				}
			} else {
				token = t.scanner.lex()
			}

			t.reader.push(token)
			entry := newNode("", "type", tokenName(token.typ), "value", t.scanner.slice(token.start, token.end))
			if t.trackRange {
				entry.Set("range", []any{token.start, token.end})
			}
			if t.trackLoc {
				entry.Set("loc", locNode(startLine, startColumn, t.scanner.lineNumber, t.scanner.index-t.scanner.lineStart))
			}
			if token.typ == tokRegularExpression {
				entry.Set("regex", newNode("", "pattern", token.pattern, "flags", token.flags))
			}
			t.buffer = append(t.buffer, entry)
		}
	}

	if len(t.buffer) == 0 {
		return nil
	}
	next := t.buffer[0]
	t.buffer = t.buffer[1:]
	return next
}
