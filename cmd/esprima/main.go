// Command esprima parses or tokenizes one JavaScript file and prints the
// result as JSON.
//
//	esprima [flags] FILE
//	  --tokenize          tokenize instead of parsing
//	  --module            parse a module (default: a script)
//	  --jsx, --range, --loc, --tokens, --comment, --attach-comment, --tolerant
//	                      turn the parser option on
//	  --source NAME       record NAME as the source of every location
//	  --delegate          also list every node (and comment) as it is finished
//
// On success the program (or the token list) is printed as indented JSON and
// the exit status is 0. A syntax error is printed as {"error": {...}} with
// exit status 1. Input on which the original Esprima fails with a JavaScript
// runtime error exits with status 3, and a usage error with status 2.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"

	"github.com/jquery/esprima"
)

// usageError reports a command line that cannot be run.
type usageError struct{ msg string }

// Error returns the usage problem.
func (e *usageError) Error() string { return e.msg }

type config struct {
	tokenize bool
	delegate bool
	opts     esprima.Options
	file     string
}

func parseArgs(args []string) (*config, error) {
	c := &config{}
	var positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--":
			positional = append(positional, args[i+1:]...)
			i = len(args)
		case "--jsx":
			c.opts.JSX = true
		case "--range":
			c.opts.Range = true
		case "--loc":
			c.opts.Loc = true
		case "--tokens":
			c.opts.Tokens = true
		case "--comment":
			c.opts.Comment = true
		case "--attach-comment":
			c.opts.AttachComment = true
		case "--tolerant":
			c.opts.Tolerant = true
		case "--tokenize":
			c.tokenize = true
		case "--module":
			c.opts.SourceType = "module"
		case "--delegate":
			c.delegate = true
		case "--source":
			if i+1 >= len(args) {
				return nil, &usageError{"flag --source needs a value"}
			}
			i++
			c.opts.Source = args[i]
		default:
			if len(a) > 1 && a[0] == '-' {
				return nil, &usageError{"unknown flag " + a}
			}
			positional = append(positional, a)
		}
	}
	if len(positional) != 1 {
		return nil, &usageError{"expected exactly one FILE argument"}
	}
	c.file = positional[0]
	return c, nil
}

// errorJSON is the JSON form of a syntax error.
func errorJSON(err *esprima.Error) *esprima.Node {
	return esprima.ObjectOf(
		"index", float64(err.Index),
		"lineNumber", float64(err.LineNumber),
		"column", float64(err.Column),
		"message", err.Message,
		"description", err.Description,
	)
}

func run(c *config, code string) (any, error) {
	if c.tokenize {
		tokens, errs, err := esprima.Tokenize(code, &c.opts)
		if err != nil {
			return nil, err
		}
		list := make([]any, len(tokens))
		for i, t := range tokens {
			list[i] = t
		}
		if !c.opts.Tolerant {
			return list, nil
		}
		errList := make([]any, len(errs))
		for i, e := range errs {
			errList[i] = e
		}
		return esprima.ObjectOf("tokens", list, "errors", errList), nil
	}

	var visits []any
	var delegate esprima.Delegate
	if c.delegate {
		delegate = func(node *esprima.Node, meta esprima.Metadata) {
			visits = append(visits, esprima.ObjectOf(
				"type", node.Type(),
				"start", position(meta.Start),
				"end", position(meta.End),
			))
		}
	}
	program, err := esprima.Parse(code, &c.opts, delegate)
	if err != nil {
		return nil, err
	}
	if c.delegate {
		if visits == nil {
			visits = []any{}
		}
		return esprima.ObjectOf("program", program, "delegate", visits), nil
	}
	return program, nil
}

func position(p esprima.Position) *esprima.Node {
	return esprima.ObjectOf("line", float64(p.Line), "column", float64(p.Column), "offset", float64(p.Offset))
}

func main() {
	os.Exit(mainStatus(os.Args[1:]))
}

func mainStatus(args []string) int {
	c, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "esprima: %v\n", err)
		return 2
	}
	data, err := os.ReadFile(c.file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "esprima: cannot read %s\n", c.file)
		return 2
	}

	result, err := run(c, esprima.DecodeUTF8(data))
	status := 0
	var syntaxErr *esprima.Error
	var internalErr *esprima.InternalError
	switch {
	case errors.As(err, &syntaxErr):
		result = esprima.ObjectOf("error", errorJSON(syntaxErr))
		status = 1
	case errors.As(err, &internalErr):
		fmt.Fprintf(os.Stderr, "esprima: internal error: %s\n", internalErr.Message)
		return 3
	case err != nil:
		fmt.Fprintf(os.Stderr, "esprima: internal error: %v\n", err)
		return 3
	}

	out := bufio.NewWriter(os.Stdout)
	text := esprima.JSON(result, esprima.JSONOptions{Indent: "  ", RegExpAsString: true, ErrorFields: true})
	if _, err := out.WriteString(text + "\n"); err != nil {
		fmt.Fprintf(os.Stderr, "esprima: %v\n", err)
		return 2
	}
	if err := out.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "esprima: %v\n", err)
		return 2
	}
	return status
}
