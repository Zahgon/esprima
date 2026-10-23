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

// Go port of src/esprima.ts.

// Package esprima is an ECMAScript (ES2020) parser with JSX support: a Go
// port of Esprima 4.0.0-dev. It produces the original's syntax tree, token
// list, comments and errors exactly, including every node location.
//
// Source text is a JavaScript string: valid UTF-8, or the per-code-unit
// encoding described at Units when it holds lone surrogates (DecodeUTF8
// turns the bytes of a file into one). All offsets, ranges and columns count
// UTF-16 code units, as in JavaScript.
package esprima

// Options configures Parse and Tokenize.
type Options struct {
	// Range adds [start, end] offsets to every node and token.
	Range bool
	// Loc adds line and column locations to every node and token.
	Loc bool
	// Source, when Loc is set and Source is not empty, is recorded in every
	// location.
	Source string
	// Tokens collects the token list in the program's "tokens" property.
	Tokens bool
	// Comment collects the comments in the program's "comments" property
	// (Tokenize: lists comments among the tokens).
	Comment bool
	// AttachComment attaches comments to nodes as leadingComments,
	// trailingComments and innerComments.
	AttachComment bool
	// Tolerant records recoverable errors in the program's "errors" property
	// instead of failing.
	Tolerant bool
	// JSX enables JSX syntax.
	JSX bool
	// SourceType is "module" to parse a module; anything else parses a
	// script.
	SourceType string
}

// Parse parses a script, or a module when opts.SourceType is "module".
// The delegate, when not nil, is called with every node as it is finished
// (and with every comment when comments are collected). A syntax error is
// returned as *Error; input on which the original fails with a JavaScript
// runtime exception is reported as *InternalError.
func Parse(code string, opts *Options, delegate Delegate) (*Node, error) {
	var o Options
	if opts != nil {
		o = *opts
	}

	var handler *commentHandler
	var parserDelegate Delegate
	proxy := func(node *Node, meta Metadata) {
		if delegate != nil {
			delegate(node, meta)
		}
		if handler != nil {
			handler.visit(node, meta)
		}
	}
	if delegate != nil {
		parserDelegate = proxy
	}
	collectComment := o.Comment
	if o.Comment || o.AttachComment {
		handler = &commentHandler{attach: o.AttachComment}
		o.Comment = true
		parserDelegate = proxy
	}

	var p *parser
	var program *Node
	err := catch(func() {
		// Creating the parser already reads the first token.
		p = newParser(Units(code), &o, parserDelegate, o.JSX)
		if o.SourceType == "module" {
			program = p.parseModule()
		} else {
			program = p.parseScript()
		}
	})
	if err != nil {
		return nil, err
	}

	if collectComment && handler != nil {
		program.Set("comments", append([]any{}, handler.comments...))
	}
	if p.config.tokens {
		program.Set("tokens", append([]any{}, p.tokens...))
	}
	if p.config.tolerant {
		errs := make([]any, len(p.errorHandler.errors))
		for i, e := range p.errorHandler.errors {
			errs[i] = e
		}
		program.Set("errors", errs)
	}
	return program, nil
}

// ParseScript parses code as a script.
func ParseScript(code string, opts *Options, delegate Delegate) (*Node, error) {
	o := Options{}
	if opts != nil {
		o = *opts
	}
	o.SourceType = "script"
	return Parse(code, &o, delegate)
}

// ParseModule parses code as a module.
func ParseModule(code string, opts *Options, delegate Delegate) (*Node, error) {
	o := Options{}
	if opts != nil {
		o = *opts
	}
	o.SourceType = "module"
	return Parse(code, &o, delegate)
}

// Tokenize splits code into tokens without parsing it. Only Range, Loc,
// Comment and Tolerant apply. Without Tolerant the first error is returned
// as err; with it the tokens read before the error are returned together
// with the recorded errors.
func Tokenize(code string, opts *Options) (tokens []*Node, errors []*Error, err error) {
	t := newTokenizer(Units(code), opts)
	tokens = []*Node{}

	failure := catch(func() {
		for {
			token := t.getNextToken()
			if token == nil {
				return
			}
			tokens = append(tokens, token)
		}
	})
	if failure != nil {
		syntaxErr, ok := failure.(*Error)
		if !t.errorHandler.tolerant || !ok {
			return nil, nil, failure
		}
		t.errorHandler.recordError(syntaxErr)
	}
	if t.errorHandler.tolerant {
		errors = append([]*Error{}, t.errorHandler.errors...)
	}
	return tokens, errors, nil
}
