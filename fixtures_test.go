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

// Go port of test/unit-tests.js and test/utils/evaluate-testcase.js.

package esprima

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The fixture suite of the original (test/unit-tests.js with
// test/utils/evaluate-testcase.js): every test/fixtures/**/<key>.js (or
// <key>.source.js, a script whose `source` variable holds the code) has an
// expected <key>.tree.json, <key>.tokens.json or <key>.failure.json written by
// the original parser.

const fixturesDir = "test/fixtures"

type fixture struct {
	key    string
	source string
	tree   string
	tokens string
	fail   string
}

func loadFixtures(t *testing.T) []*fixture {
	t.Helper()
	byKey := map[string]*fixture{}
	get := func(key string) *fixture {
		if byKey[key] == nil {
			byKey[key] = &fixture{key: key}
		}
		return byKey[key]
	}
	err := filepath.WalkDir(fixturesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(fixturesDir, path)
		if err != nil {
			return err
		}
		switch {
		case strings.HasSuffix(rel, ".source.js"):
			get(strings.TrimSuffix(rel, ".source.js")).source = path + "!source"
		case strings.HasSuffix(rel, ".js"):
			get(strings.TrimSuffix(rel, ".js")).source = path
		case strings.HasSuffix(rel, ".tree.json"):
			get(strings.TrimSuffix(rel, ".tree.json")).tree = path
		case strings.HasSuffix(rel, ".tokens.json"):
			get(strings.TrimSuffix(rel, ".tokens.json")).tokens = path
		case strings.HasSuffix(rel, ".failure.json"):
			get(strings.TrimSuffix(rel, ".failure.json")).fail = path
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reading %s: %v", fixturesDir, err)
	}
	keys := make([]string, 0, len(byKey))
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	list := make([]*fixture, 0, len(keys))
	for _, k := range keys {
		list = append(list, byKey[k])
	}
	return list
}

// code returns the fixture's source text. A .source.js fixture is a script
// declaring `var source = '...'`; the string literal's value is the code.
func (f *fixture) code(t *testing.T) string {
	t.Helper()
	path, isSource := strings.CutSuffix(f.source, "!source")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %v", f.key, err)
	}
	code := DecodeUTF8(data)
	if !isSource {
		return code
	}
	program, err := ParseScript(code, nil, nil)
	if err != nil {
		t.Fatalf("%s: parsing the source script: %v", f.key, err)
	}
	for _, stmt := range program.list("body") {
		decl := stmt.(*Node)
		if decl.Type() != "VariableDeclaration" {
			continue
		}
		for _, d := range decl.list("declarations") {
			declarator := d.(*Node)
			if declarator.node("id").str("name") == "source" {
				return declarator.node("init").str("value")
			}
		}
	}
	t.Fatalf("%s: no source variable", f.key)
	return ""
}

// generic decodes JSON into maps and slices for an order-insensitive
// comparison (the original's test sorts keys the same way).
func generic(t *testing.T, key, text string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		t.Fatalf("%s: decoding %q: %v", key, text[:min(len(text), 200)], err)
	}
	return nullUnicodeRegexValues(v)
}

// nullUnicodeRegexValues sets the value of regular expression literals with
// the u or y flag to null, as the original's test does, so the comparison
// does not depend on the engine's support for those flags.
func nullUnicodeRegexValues(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, c := range x {
			x[k] = nullUnicodeRegexValues(c)
		}
		if re, ok := x["regex"].(map[string]any); ok {
			if flags, _ := re["flags"].(string); strings.ContainsAny(flags, "uy") {
				if _, has := x["value"]; has {
					x["value"] = nil
				}
			}
		}
	case []any:
		for i, c := range x {
			x[i] = nullUnicodeRegexValues(c)
		}
	}
	return v
}

func readJSON(t *testing.T, key, path string) any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %v", key, err)
	}
	return generic(t, key, string(data))
}

// errorObject is the original test's errorToObject plus the description.
func errorObject(e *Error) *Node {
	return ObjectOf("index", float64(e.Index), "lineNumber", float64(e.LineNumber), "column", float64(e.Column),
		"message", "Error: "+e.Message)
}

func hasAttachedComment(v any) bool {
	switch x := v.(type) {
	case map[string]any:
		for k, c := range x {
			if k == "leadingComments" || k == "trailingComments" || k == "innerComments" || hasAttachedComment(c) {
				return true
			}
		}
	case []any:
		for _, c := range x {
			if hasAttachedComment(c) {
				return true
			}
		}
	}
	return false
}

func TestFixtures(t *testing.T) {
	fixtures := loadFixtures(t)
	if len(fixtures) < 1600 {
		t.Fatalf("found %d fixtures, want the whole suite", len(fixtures))
	}
	for _, f := range fixtures {
		f := f
		t.Run(f.key, func(t *testing.T) {
			switch {
			case f.tree != "":
				testParseFixture(t, f)
			case f.tokens != "":
				testTokenizeFixture(t, f)
			case f.fail != "":
				testFailureFixture(t, f)
			default:
				t.Fatalf("%s has no expected result", f.key)
			}
		})
	}
}

func testParseFixture(t *testing.T, f *fixture) {
	expected := readJSON(t, f.key, f.tree).(map[string]any)
	opts := &Options{JSX: true, Range: true, Loc: true, Tokens: true}
	opts.SourceType, _ = expected["sourceType"].(string)
	if tokens, ok := expected["tokens"].([]any); ok && len(tokens) > 0 {
		first := tokens[0].(map[string]any)
		_, opts.Range = first["range"]
		_, opts.Loc = first["loc"]
	}
	if comments, ok := expected["comments"].([]any); ok {
		opts.Comment = true
		opts.AttachComment = hasAttachedComment(expected)
		if len(comments) > 0 {
			first := comments[0].(map[string]any)
			_, opts.Range = first["range"]
			_, opts.Loc = first["loc"]
		}
	}
	if _, ok := expected["errors"]; ok {
		opts.Tolerant = true
	}
	if loc, ok := expected["loc"].(map[string]any); ok && opts.Loc {
		opts.Source, _ = loc["source"].(string)
	}

	program, err := Parse(f.code(t), opts, nil)
	if err != nil {
		t.Fatalf("Parse(%s) failed: %v", f.key, err)
	}
	if errs, ok := program.Get("errors").([]any); ok {
		converted := make([]any, len(errs))
		for i, e := range errs {
			converted[i] = errorObject(e.(*Error))
		}
		program.Set("errors", converted)
	}
	actual := generic(t, f.key, JSON(program, JSONOptions{}))
	if !reflect.DeepEqual(expected, actual) {
		text := JSON(program, JSONOptions{})
		t.Errorf("Parse(%s) = %s\nwant the tree in %s", f.key, text[:min(400, len(text))], f.tree)
	}
}

func testTokenizeFixture(t *testing.T, f *fixture) {
	expected := readJSON(t, f.key, f.tokens)
	tokens, _, err := Tokenize(f.code(t), &Options{Comment: true, Tolerant: true, Loc: true, Range: true})
	if err != nil {
		t.Fatalf("Tokenize(%s) failed: %v", f.key, err)
	}
	text := JSON(tokens, JSONOptions{})
	if actual := generic(t, f.key, text); !reflect.DeepEqual(expected, actual) {
		t.Errorf("Tokenize(%s) = %s\nwant the tokens in %s", f.key, text[:min(400, len(text))], f.tokens)
	}
}

var linePrefixPattern = regexp.MustCompile(`Error: Line [0-9]+: `)

func testFailureFixture(t *testing.T, f *fixture) {
	expected := readJSON(t, f.key, f.fail).(map[string]any)
	tokenize, _ := expected["tokenize"].(bool)
	delete(expected, "tokenize")
	message, _ := expected["message"].(string)
	expected["description"] = linePrefixPattern.ReplaceAllString(message, "")

	sourceType := ""
	if strings.HasSuffix(f.key, ".module") {
		sourceType = "module"
	}
	code := f.code(t)
	for _, opts := range []Options{
		{JSX: true, SourceType: sourceType},
		{JSX: true, Comment: true, SourceType: sourceType},
	} {
		var err error
		if tokenize {
			_, _, err = Tokenize(code, &opts)
		} else {
			_, err = Parse(code, &opts, nil)
		}
		syntaxErr, ok := err.(*Error)
		if !ok {
			t.Fatalf("%s with %+v: got %v, want the error in %s", f.key, opts, err, f.fail)
		}
		obj := errorObject(syntaxErr)
		obj.Set("description", syntaxErr.Description)
		if actual := generic(t, f.key, JSON(obj, JSONOptions{})); !reflect.DeepEqual(expected, actual) {
			t.Errorf("%s with %+v: error %s, want %v", f.key, opts, JSON(obj, JSONOptions{}), expected)
		}
	}
}
