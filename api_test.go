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

// Go port of test/api-tests.js.

package esprima

import (
	"strings"
	"testing"
)

// API checks carried over from the original's test/api-tests.js, plus the
// JavaScript semantics the output depends on.

func TestVersion(t *testing.T) {
	if Version != "4.0.0-dev" {
		t.Errorf("Version = %q, want 4.0.0-dev", Version)
	}
}

func TestSourceType(t *testing.T) {
	for _, tc := range []struct {
		parse func(string, *Options, Delegate) (*Node, error)
		want  string
	}{{ParseScript, "script"}, {ParseModule, "module"}} {
		program, err := tc.parse("answer = 42", nil, nil)
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		if got := program.str("sourceType"); got != tc.want {
			t.Errorf("sourceType = %q, want %q", got, tc.want)
		}
	}
	if _, err := ParseScript("import x from 'y'", nil, nil); err == nil || err.Error() != "Line 1: Unexpected token" {
		t.Errorf("ParseScript(import) error = %v, want Line 1: Unexpected token", err)
	}
}

func TestSyntaxError(t *testing.T) {
	_, err := Parse("var x = ;", nil, nil)
	syntaxErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("Parse error = %v, want *Error", err)
	}
	want := Error{Index: 8, LineNumber: 1, Column: 9, Description: "Unexpected token ;", Message: "Line 1: Unexpected token ;"}
	if *syntaxErr != want {
		t.Errorf("Parse error = %+v, want %+v", *syntaxErr, want)
	}
}

func TestTolerantErrors(t *testing.T) {
	program, err := Parse("'use strict'; with (x) y; delete z", &Options{Tolerant: true}, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	errs := program.list("errors")
	if len(errs) != 2 || errs[0].(*Error).Description != msgStrictModeWith || errs[1].(*Error).Description != msgStrictDelete {
		t.Errorf("errors = %v, want the with and delete strict mode errors", JSON(errs, JSONOptions{}))
	}
}

func TestDelegateOrder(t *testing.T) {
	var types []string
	_, err := Parse("a(b) /* c */", &Options{Comment: true}, func(node *Node, meta Metadata) {
		types = append(types, node.Type())
	})
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	want := "Identifier Identifier BlockComment CallExpression ExpressionStatement Program"
	if got := strings.Join(types, " "); got != want {
		t.Errorf("delegate saw %q, want %q", got, want)
	}
}

func TestTokenize(t *testing.T) {
	tokens, _, err := Tokenize("x = /re/g / 2", nil)
	if err != nil {
		t.Fatalf("Tokenize failed: %v", err)
	}
	var kinds []string
	for _, tok := range tokens {
		kinds = append(kinds, tok.str("type")+":"+tok.str("value"))
	}
	want := "Identifier:x Punctuator:= RegularExpression:/re/g Punctuator:/ Numeric:2"
	if got := strings.Join(kinds, " "); got != want {
		t.Errorf("tokens = %q, want %q", got, want)
	}
}

func TestNumberToString(t *testing.T) {
	for in, want := range map[float64]string{
		0: "0", 1: "1", 0.1: "0.1", 1e21: "1e+21", 1e20: "100000000000000000000", 1e-7: "1e-7",
		0.000001: "0.000001", 123e-20: "1.23e-18", 5e-324: "5e-324", 1.7976931348623157e308: "1.7976931348623157e+308",
		0.30000000000000004: "0.30000000000000004", -2.5: "-2.5",
	} {
		if got := NumberToString(in); got != want {
			t.Errorf("NumberToString(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestJSONStrings(t *testing.T) {
	s := unitsString([]uint16{'<', '/', 0x2028, 0x1F, 0xD800, 'a', 0xD83D, 0xDE00, '"'})
	if got, want := JSON(s, JSONOptions{}), "\"</ \\u001f\\ud800a\U0001F600\\\"\""; got != want {
		t.Errorf("JSON(%q) = %s, want %s", s, got, want)
	}
}

func TestRegExpValidity(t *testing.T) {
	for _, tc := range []struct {
		pattern, flags string
		want           bool
	}{
		{"a{,5}", "", true}, {"{1}", "", false}, {"(?<a>.)\\k<b>", "", false}, {"\\k", "", true},
		{"(?<a>.)\\k", "", false}, {"[\\d-a]", "", true}, {"[\\d-a]", "u", false}, {"(?=a)*", "", true},
		{"(?=a)*", "u", false}, {"(?<=a)*", "", false}, {"a", "uv", false}, {"a", "gg", false},
		{"\\p{Lu}", "u", true}, {"\\p{Foo}", "u", false}, {"[^\\q{abc}]", "v", false}, {"[a&&b]", "v", true},
	} {
		if got := validRegExp(Units(tc.pattern), tc.flags); got != tc.want {
			t.Errorf("validRegExp(/%s/%s) = %v, want %v", tc.pattern, tc.flags, got, tc.want)
		}
	}
	if got := (&RegExp{Pattern: "a/b", Flags: "yig"}).String(); got != "/a\\/b/giy" {
		t.Errorf("RegExp string = %q, want /a\\/b/giy", got)
	}
}

func TestDecodeUTF8(t *testing.T) {
	got := Units(DecodeUTF8([]byte("a\xE2\x82b\xF0\x9F\x98\x80\xFF")))
	want := []uint16{'a', 0xFFFD, 'b', 0xD83D, 0xDE00, 0xFFFD}
	if len(got) != len(want) {
		t.Fatalf("DecodeUTF8 = %x, want %x", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("DecodeUTF8 = %x, want %x", got, want)
		}
	}
}
