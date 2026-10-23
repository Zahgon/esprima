[![Tests](https://github.com/jquery/esprima/workflows/Tests/badge.svg)](https://github.com/jquery/esprima/actions)

**Esprima** ([esprima.org](http://esprima.org), BSD license) is a high performance,
standard-compliant [ECMAScript](http://www.ecma-international.org/publications/standards/Ecma-262.htm)
parser. This is its Go implementation: a module with no dependencies beyond the Go standard library
that produces exactly the syntax trees, tokens, comments and errors of Esprima 4.0.0-dev.
Esprima is created and maintained by [Ariya Hidayat](https://twitter.com/ariyahidayat),
with the help of [many contributors](https://github.com/jquery/esprima/contributors).

### Features

- Full support for ECMAScript 2019 and the ES2020 additions Esprima implements (optional chaining,
  nullish coalescing, `import.meta`, dynamic `import()`)
- Sensible [syntax tree format](https://github.com/estree/estree/blob/master/es5.md) as standardized by [ESTree project](https://github.com/estree/estree)
- Experimental support for [JSX](https://facebook.github.io/jsx/), a syntax extension for [React](https://facebook.github.io/react/)
- Optional tracking of syntax node location (index-based and line-column, counted in UTF-16 code units)
- Comment collection and attachment, tolerant parsing and a node delegate
- Heavily tested: the ~1600 [unit test fixtures](test/fixtures) and the third-party library baselines

### API

Esprima can be used to perform [lexical analysis](https://en.wikipedia.org/wiki/Lexical_analysis) (tokenization) or [syntactic analysis](https://en.wikipedia.org/wiki/Parsing) (parsing) of a JavaScript program.

```go
import "github.com/jquery/esprima"

tokens, _, err := esprima.Tokenize("const answer = 42", nil)
program, err := esprima.ParseScript("const answer = 42", &esprima.Options{Range: true, Loc: true}, nil)
fmt.Println(esprima.JSON(program, esprima.JSONOptions{Indent: "  "}))
```

`Parse`, `ParseScript` and `ParseModule` take the options `Range`, `Loc`, `Source`, `Tokens`, `Comment`,
`AttachComment`, `Tolerant` and `JSX`, and an optional delegate that sees every node as it is finished.
`Tokenize` takes `Range`, `Loc`, `Comment` and `Tolerant`. Syntax errors are returned as `*esprima.Error`
with the index, line, column and description of the original.

### Command line

```
go build -o bin/ ./cmd/...
bin/esparse [--comment] [--loc] [--range] [--tokens] [--tolerant] file.js
bin/esvalidate [--format=plain|junit] file.js...
bin/esprima [--tokenize] [--module] [--jsx] [--range] [--loc] [--source NAME] [--tokens] [--comment] [--attach-comment] [--tolerant] [--delegate] file.js
```

`esparse` prints the syntax tree as JSON, `esvalidate` reports syntax errors in plain text or JUnit XML,
and `esprima` exposes every parser and tokenizer option.

### Testing

```
go vet ./...
go test ./...
```

For more information, please read the [complete documentation](http://esprima.org/doc).
