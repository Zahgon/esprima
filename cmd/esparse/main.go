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

// Go port of bin/esparse.js.

// Command esparse parses a JavaScript file and prints its syntax tree as
// JSON.
//
//	esparse [options] [file.js]
//
// See showUsage for the options. Without a file the program is read from
// standard input.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jquery/esprima"
	"github.com/jquery/esprima/internal/cli"
)

func showUsage(w *cli.Writer) {
	w.Printf("%s", strings.Join([]string{
		"Usage:",
		"   esparse [options] [file.js]",
		"",
		"Available options:",
		"",
		"  --comment      Gather all line and block comments in an array",
		"  --loc          Include line-column location info for each syntax node",
		"  --range        Include index-based range for each syntax node",
		"  --raw          Display the raw value of literals",
		"  --tokens       List all tokens in an array",
		"  --tolerant     Tolerate errors on a best-effort basis (experimental)",
		"  -v, --version  Shows program version",
		"",
		"",
	}, "\n"))
}

func main() {
	out := cli.NewWriter(os.Stdout)
	status := run(os.Args[1:], out)
	if err := out.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "esparse: %v\n", err)
		status = 1
	}
	os.Exit(status)
}

func run(args []string, stdout *cli.Writer) int {
	var opts esprima.Options
	fname := ""
	haveFile := false
	forceFile := false

	for _, entry := range args {
		switch {
		case forceFile || entry == "-" || !strings.HasPrefix(entry, "-"):
			if haveFile {
				stdout.Println("Error: more than one input file.")
				return 1
			}
			fname, haveFile = entry, true
		case entry == "-h" || entry == "--help":
			showUsage(stdout)
			return 1
		case entry == "-v" || entry == "--version":
			stdout.Println("ECMAScript Parser (using Esprima version", esprima.Version, ")")
			stdout.Println()
			return 0
		case entry == "--comment":
			opts.Comment = true
		case entry == "--loc":
			opts.Loc = true
		case entry == "--range":
			opts.Range = true
		case entry == "--raw":
			// Accepted for compatibility; literals always carry their raw text.
		case entry == "--tokens":
			opts.Tokens = true
		case entry == "--tolerant":
			opts.Tolerant = true
		case entry == "--":
			forceFile = true
		default:
			stdout.Printf("Error: unknown option %s.\n", entry)
			return 1
		}
	}

	var content []byte
	var err error
	if fname != "" && (fname != "-" || forceFile) {
		content, err = os.ReadFile(fname)
		if err != nil {
			stdout.Printf("Error: %s\n", cli.ReadErrorMessage(err, fname))
			return 1
		}
	} else {
		content, err = io.ReadAll(os.Stdin)
		if err != nil {
			stdout.Printf("Error: %v\n", err)
			return 1
		}
	}

	syntax, err := esprima.Parse(esprima.DecodeUTF8(content), &opts, nil)
	if err != nil {
		stdout.Printf("Error: %s\n", esprima.ToUTF8(err.Error()))
		return 1
	}
	text := esprima.JSON(syntax, esprima.JSONOptions{Indent: "    ", RegExpAsString: true})
	stdout.Println(text)
	return 0
}
