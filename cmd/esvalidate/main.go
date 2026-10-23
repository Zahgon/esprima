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

// Go port of bin/esvalidate.js.

// Command esvalidate checks JavaScript files for syntax errors and reports
// them in plain text or as JUnit XML.
//
//	esvalidate [options] [file.js...]
//
// The exit status is 1 when an error was reported (in the JUnit format, only
// when a file could not be parsed at all).
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jquery/esprima"
	"github.com/jquery/esprima/internal/cli"
)

var linePrefix = regexp.MustCompile(`^Line [0-9]*: `)

func showUsage(w *cli.Writer) {
	w.Printf("%s", strings.Join([]string{
		"Usage:",
		"   esvalidate [options] [file.js...]",
		"",
		"Available options:",
		"",
		"  --format=type  Set the report format, plain (default) or junit",
		"  -v, --version  Print program version",
		"",
		"",
	}, "\n"))
}

func main() {
	out := cli.NewWriter(os.Stdout)
	status := run(os.Args[1:], out)
	if err := out.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "esvalidate: %v\n", err)
		status = 1
	}
	os.Exit(status)
}

// validator reports the files it checks and counts the reported failures.
type validator struct {
	out    *cli.Writer
	format string
	count  int
}

func run(args []string, stdout *cli.Writer) int {
	v := &validator{out: stdout, format: "plain"}
	var fnames []string
	forceFile := false

	for _, entry := range args {
		switch {
		case forceFile || entry == "-" || !strings.HasPrefix(entry, "-"):
			fnames = append(fnames, entry)
		case entry == "-h" || entry == "--help":
			showUsage(stdout)
			return 1
		case entry == "-v" || entry == "--version":
			stdout.Println("ECMAScript Validator (using Esprima version", esprima.Version, ")")
			stdout.Println()
			return 0
		case strings.HasPrefix(entry, "--format="):
			v.format = entry[len("--format="):]
			if v.format != "plain" && v.format != "junit" {
				stdout.Printf("Error: unknown report format %s.\n", v.format)
				return 1
			}
		case entry == "--":
			forceFile = true
		default:
			stdout.Printf("Error: unknown option %s.\n", entry)
			return 1
		}
	}
	if len(fnames) == 0 {
		fnames = append(fnames, "")
	}

	if v.format == "junit" {
		stdout.Println(`<?xml version="1.0" encoding="UTF-8"?>`)
		stdout.Println("<testsuites>")
	}

	for _, fname := range fnames {
		var content []byte
		var err error
		if fname != "" && (fname != "-" || forceFile) {
			content, err = os.ReadFile(fname)
			if err != nil {
				err = errors.New(cli.ReadErrorMessage(err, fname))
			}
		} else {
			fname = ""
			content, err = io.ReadAll(os.Stdin)
		}
		v.validate(fname, content, err)
	}

	if v.format == "junit" {
		stdout.Println("</testsuites>")
	}
	if v.count > 0 {
		return 1
	}
	return 0
}

// seconds rounds an elapsed time to whole seconds like Math.round.
func seconds(d time.Duration) string {
	return strconv.FormatInt((d.Milliseconds()+500)/1000, 10)
}

func (v *validator) validate(fname string, content []byte, readErr error) {
	if readErr != nil {
		// The file was never parsed, so no time was taken.
		v.report(fname, readErr.Error(), "", "NaN")
		return
	}
	if len(content) >= 2 && content[0] == '#' && content[1] == '!' {
		content = append([]byte("//"), content[2:]...)
	}
	start := time.Now()
	syntax, err := esprima.Parse(esprima.DecodeUTF8(content), &esprima.Options{Tolerant: true}, nil)
	if err != nil {
		lineNumber := ""
		var syntaxErr *esprima.Error
		if errors.As(err, &syntaxErr) {
			lineNumber = strconv.Itoa(syntaxErr.LineNumber)
		}
		v.report(fname, esprima.ToUTF8(err.Error()), lineNumber, seconds(time.Since(start)))
		return
	}

	errs := syntax.Get("errors").([]any)
	if v.format == "junit" {
		name := fname
		if i := strings.LastIndex(name, "/"); i >= 0 {
			name = name[i+1:]
		}
		n := strconv.Itoa(len(errs))
		v.out.Printf("<testsuite name=\"%s\" errors=\"0\"  failures=\"%s\"  tests=\"%s\"  time=\"%s\">\n",
			fname, n, n, seconds(time.Since(start)))
		for _, e := range errs {
			syntaxErr := e.(*esprima.Error)
			message := esprima.ToUTF8(syntaxErr.Message)
			msg := linePrefix.ReplaceAllString(message, "")
			v.out.Printf("  <testcase name=\"Line %d: %s\"  time=\"0\">\n", syntaxErr.LineNumber, msg)
			v.out.Printf("    <error type=\"SyntaxError\" message=\"%s\">%s(%s:%d)</error>\n",
				message, message, name, syntaxErr.LineNumber)
			v.out.Println("  </testcase>")
		}
		v.out.Println("</testsuite>")
		return
	}
	for _, e := range errs {
		syntaxErr := e.(*esprima.Error)
		msg := linePrefix.ReplaceAllString(esprima.ToUTF8(syntaxErr.Message), "")
		v.out.Printf("%s:%d: %s\n", fname, syntaxErr.LineNumber, msg)
		v.count++
	}
}

// report writes a file that could not be parsed: message is the failure,
// lineNumber its line ("" when it has none) and elapsed the time attribute.
func (v *validator) report(fname, message, lineNumber, elapsed string) {
	v.count++
	if v.format == "junit" {
		location := fname
		if lineNumber != "" {
			location += ":" + lineNumber
		}
		v.out.Printf("<testsuite name=\"%s\" errors=\"1\" failures=\"0\" tests=\"1\"  time=\"%s\">\n", fname, elapsed)
		v.out.Printf(" <testcase name=\"%s\"  time=\"0\">\n", message)
		v.out.Printf(" <error type=\"ParseError\" message=\"%s\">%s(%s)</error>\n", message, message, location)
		v.out.Println(" </testcase>")
		v.out.Println("</testsuite>")
		return
	}
	if lineNumber == "" {
		lineNumber = "undefined"
	}
	v.out.Printf("%s:%s: %s\n", fname, lineNumber, linePrefix.ReplaceAllString(message, ""))
}
