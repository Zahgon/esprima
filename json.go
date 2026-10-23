package esprima

import (
	"math"
	"strings"
)

// JSONOptions controls how JSON writes values that JSON.stringify would hand
// to a replacer function.
type JSONOptions struct {
	// Indent is the gap string of JSON.stringify's third argument.
	Indent string
	// RegExpAsString writes a regular expression literal's value as the
	// RegExp's string form ("/source/flags") instead of {}.
	RegExpAsString bool
	// ErrorFields writes an *Error as an object holding index, lineNumber,
	// column, message and description. Without it an error is written with
	// its enumerable own properties only: index, lineNumber and description.
	ErrorFields bool
}

// JSON serialises a value produced by this package exactly as
// JSON.stringify(value, replacer, indent) does in JavaScript: object keys in
// insertion order, numbers in JavaScript notation (non-finite numbers become
// null) and strings escaped with lone surrogates written as \u escapes.
func JSON(v any, opts JSONOptions) string {
	var b strings.Builder
	writeJSON(&b, v, opts, "")
	return b.String()
}

func writeJSON(b *strings.Builder, v any, opts JSONOptions, indent string) {
	switch x := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		if x {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			b.WriteString("null")
		} else {
			b.WriteString(NumberToString(x))
		}
	case int:
		b.WriteString(NumberToString(float64(x)))
	case string:
		writeJSONString(b, x)
	case *RegExp:
		if opts.RegExpAsString {
			writeJSONString(b, x.String())
		} else {
			b.WriteString("{}")
		}
	case *Error:
		obj := &Node{}
		obj.Set("index", float64(x.Index))
		obj.Set("lineNumber", float64(x.LineNumber))
		if opts.ErrorFields {
			obj.Set("column", float64(x.Column))
			obj.Set("message", x.Message)
		}
		obj.Set("description", x.Description)
		writeJSON(b, obj, opts, indent)
	case *Node:
		if len(x.keys) == 0 {
			b.WriteString("{}")
			return
		}
		inner := indent + opts.Indent
		b.WriteByte('{')
		for i, k := range x.keys {
			if i > 0 {
				b.WriteByte(',')
			}
			if opts.Indent != "" {
				b.WriteByte('\n')
				b.WriteString(inner)
			}
			writeJSONString(b, k)
			b.WriteByte(':')
			if opts.Indent != "" {
				b.WriteByte(' ')
			}
			writeJSON(b, x.vals[i], opts, inner)
		}
		if opts.Indent != "" {
			b.WriteByte('\n')
			b.WriteString(indent)
		}
		b.WriteByte('}')
	case []any:
		writeJSONArray(b, len(x), func(i int) any { return x[i] }, opts, indent)
	case []*Node:
		writeJSONArray(b, len(x), func(i int) any { return x[i] }, opts, indent)
	case []*Error:
		writeJSONArray(b, len(x), func(i int) any { return x[i] }, opts, indent)
	case []float64:
		writeJSONArray(b, len(x), func(i int) any { return x[i] }, opts, indent)
	default:
		b.WriteString("null")
	}
}

func writeJSONArray(b *strings.Builder, n int, at func(int) any, opts JSONOptions, indent string) {
	if n == 0 {
		b.WriteString("[]")
		return
	}
	inner := indent + opts.Indent
	b.WriteByte('[')
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		if opts.Indent != "" {
			b.WriteByte('\n')
			b.WriteString(inner)
		}
		writeJSON(b, at(i), opts, inner)
	}
	if opts.Indent != "" {
		b.WriteByte('\n')
		b.WriteString(indent)
	}
	b.WriteByte(']')
}

const hexDigits = "0123456789abcdef"

// writeJSONString quotes a JavaScript string like JSON.stringify: the short
// escapes for quote, backslash and \b \f \n \r \t, \u00XX for other control
// characters and \uXXXX for lone surrogates; everything else is written as
// UTF-8.
func writeJSONString(b *strings.Builder, s string) {
	units := Units(s)
	b.WriteByte('"')
	for i := 0; i < len(units); i++ {
		u := units[i]
		switch {
		case u == '"':
			b.WriteString(`\"`)
		case u == '\\':
			b.WriteString(`\\`)
		case u == '\b':
			b.WriteString(`\b`)
		case u == '\f':
			b.WriteString(`\f`)
		case u == '\n':
			b.WriteString(`\n`)
		case u == '\r':
			b.WriteString(`\r`)
		case u == '\t':
			b.WriteString(`\t`)
		case u < 0x20:
			b.WriteString(`\u00`)
			b.WriteByte(hexDigits[u>>4])
			b.WriteByte(hexDigits[u&0xF])
		case u >= 0xD800 && u <= 0xDBFF && i+1 < len(units) && units[i+1] >= 0xDC00 && units[i+1] <= 0xDFFF:
			b.WriteRune(0x10000 + (rune(u)-0xD800)<<10 + rune(units[i+1]) - 0xDC00)
			i++
		case u >= 0xD800 && u <= 0xDFFF:
			b.WriteString(`\u`)
			b.WriteByte(hexDigits[u>>12])
			b.WriteByte(hexDigits[(u>>8)&0xF])
			b.WriteByte(hexDigits[(u>>4)&0xF])
			b.WriteByte(hexDigits[u&0xF])
		default:
			b.WriteRune(rune(u))
		}
	}
	b.WriteByte('"')
}
