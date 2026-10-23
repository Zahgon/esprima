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

// Go port of test/regression-tests.js.

package esprima

import (
	"os"
	"reflect"
	"testing"
)

// The regression suite of the original (test/regression-tests.js): the
// libraries under test/3rdparty parse and tokenize to the recorded baselines
// in test/3rdparty/syntax.
func TestThirdPartyLibraries(t *testing.T) {
	for _, name := range []string{
		"underscore-1.5.2", "backbone-1.1.0", "mootools-1.4.5", "jquery-1.9.1",
		"yui-3.12.0", "jquery.mobile-1.4.2", "angular-1.2.5",
	} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile("test/3rdparty/" + name + ".js")
			if err != nil {
				t.Fatal(err)
			}
			code := DecodeUTF8(data)

			program, err := Parse(code, &Options{Range: true, Loc: true}, nil)
			if err != nil {
				t.Fatalf("Parse(%s) failed: %v", name, err)
			}
			want := readJSON(t, name, "test/3rdparty/syntax/"+name+".json")
			if got := generic(t, name, JSON(program, JSONOptions{})); !reflect.DeepEqual(got, want) {
				t.Errorf("Parse(%s) differs from test/3rdparty/syntax/%s.json", name, name)
			}

			tokens, _, err := Tokenize(code, &Options{Range: true})
			if err != nil {
				t.Fatalf("Tokenize(%s) failed: %v", name, err)
			}
			want = readJSON(t, name, "test/3rdparty/syntax/"+name+".tokens")
			if got := generic(t, name, JSON(tokens, JSONOptions{})); !reflect.DeepEqual(got, want) {
				t.Errorf("Tokenize(%s) differs from test/3rdparty/syntax/%s.tokens", name, name)
			}
		})
	}
}
