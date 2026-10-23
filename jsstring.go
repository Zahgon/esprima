package esprima

import (
	"unicode/utf16"
	"unicode/utf8"
)

// JavaScript strings are sequences of UTF-16 code units and may hold lone
// surrogates, which a UTF-8 Go string cannot carry. Every string value this
// package produces (identifier names, literal values, comment text, raw
// slices) is therefore stored in a Go string with each UTF-16 code unit
// encoded on its own, CESU-8 style: code units below U+D800 and above U+DFFF
// are plain UTF-8 and a surrogate is written as its own three-byte sequence.
// ASCII text is unchanged, so names and keywords compare directly with Go
// string literals. Units decodes such a string back to its code units and
// ToUTF8 converts it to ordinary UTF-8.

// appendUnit appends the encoding of one UTF-16 code unit.
func appendUnit(b []byte, u uint16) []byte {
	switch {
	case u < 0x80:
		return append(b, byte(u))
	case u < 0x800:
		return append(b, 0xC0|byte(u>>6), 0x80|byte(u&0x3F))
	default:
		return append(b, 0xE0|byte(u>>12), 0x80|byte((u>>6)&0x3F), 0x80|byte(u&0x3F))
	}
}

// unitsString encodes a slice of UTF-16 code units as a JavaScript string.
func unitsString(units []uint16) string {
	ascii := true
	for _, u := range units {
		if u >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		b := make([]byte, len(units))
		for i, u := range units {
			b[i] = byte(u)
		}
		return string(b)
	}
	b := make([]byte, 0, len(units)*2)
	for _, u := range units {
		b = appendUnit(b, u)
	}
	return string(b)
}

// Units decodes a JavaScript string produced by this package into its UTF-16
// code units.
func Units(s string) []uint16 {
	out := make([]uint16, 0, len(s))
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c < 0x80:
			out = append(out, uint16(c))
			i++
		case c < 0xE0 && i+1 < len(s):
			out = append(out, uint16(c&0x1F)<<6|uint16(s[i+1]&0x3F))
			i += 2
		case c < 0xF0 && i+2 < len(s):
			out = append(out, uint16(c&0x0F)<<12|uint16(s[i+1]&0x3F)<<6|uint16(s[i+2]&0x3F))
			i += 3
		default:
			// A four-byte UTF-8 sequence (only found in strings that did not
			// come from this package) stands for a surrogate pair.
			r, n := utf8.DecodeRuneInString(s[i:])
			if r >= 0x10000 {
				hi, lo := utf16.EncodeRune(r)
				out = append(out, uint16(hi), uint16(lo))
			} else {
				out = append(out, uint16(r))
			}
			i += n
		}
	}
	return out
}

// unitLength returns the number of UTF-16 code units of a JavaScript string.
func unitLength(s string) int {
	n := 0
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c < 0x80:
			i++
		case c < 0xE0:
			i += 2
		case c < 0xF0:
			i += 3
		default:
			i += 4
			n++
		}
		n++
	}
	return n
}

// ToUTF8 converts a JavaScript string to UTF-8, joining surrogate pairs and
// replacing lone surrogates with U+FFFD.
func ToUTF8(s string) string {
	units := Units(s)
	b := make([]byte, 0, len(s))
	for i := 0; i < len(units); i++ {
		u := units[i]
		if u >= 0xD800 && u <= 0xDBFF && i+1 < len(units) && units[i+1] >= 0xDC00 && units[i+1] <= 0xDFFF {
			b = utf8.AppendRune(b, utf16.DecodeRune(rune(u), rune(units[i+1])))
			i++
			continue
		}
		if u >= 0xD800 && u <= 0xDFFF {
			b = utf8.AppendRune(b, utf8.RuneError)
			continue
		}
		b = utf8.AppendRune(b, rune(u))
	}
	return string(b)
}

// fromCodePoint is String.fromCodePoint for a valid code point: astral code
// points become a surrogate pair.
func fromCodePoint(cp int) string {
	if cp < 0x10000 {
		return unitsString([]uint16{uint16(cp)})
	}
	return unitsString([]uint16{uint16(0xD800 + ((cp - 0x10000) >> 10)), uint16(0xDC00 + ((cp - 0x10000) & 1023))})
}

// fromCharCode is String.fromCharCode: the number is reduced to a 16-bit code
// unit the way ToUint16 does.
func fromCharCode(code float64) string {
	return unitsString([]uint16{toUint16(code)})
}

// DecodeUTF8 decodes the bytes of a file into a JavaScript string the way
// Node.js reads a file as 'utf-8' (the WHATWG UTF-8 decoder): each maximal
// invalid subpart of an ill-formed sequence becomes one U+FFFD and a byte
// order mark is kept.
func DecodeUTF8(b []byte) string {
	return unitsString(decodeUTF8Units(b))
}

func decodeUTF8Units(b []byte) []uint16 {
	out := make([]uint16, 0, len(b))
	for i := 0; i < len(b); {
		c := b[i]
		if c < 0x80 {
			out = append(out, uint16(c))
			i++
			continue
		}
		var need int
		var cp rune
		lower, upper := byte(0x80), byte(0xBF)
		switch {
		case c >= 0xC2 && c <= 0xDF:
			need, cp = 1, rune(c&0x1F)
		case c >= 0xE0 && c <= 0xEF:
			need, cp = 2, rune(c&0x0F)
			if c == 0xE0 {
				lower = 0xA0
			} else if c == 0xED {
				upper = 0x9F
			}
		case c >= 0xF0 && c <= 0xF4:
			need, cp = 3, rune(c&0x07)
			if c == 0xF0 {
				lower = 0x90
			} else if c == 0xF4 {
				upper = 0x8F
			}
		default:
			out = append(out, 0xFFFD)
			i++
			continue
		}
		j := i + 1
		ok := true
		for k := 0; k < need; k++ {
			if j >= len(b) || b[j] < lower || b[j] > upper {
				ok = false
				break
			}
			cp = cp<<6 | rune(b[j]&0x3F)
			lower, upper = 0x80, 0xBF
			j++
		}
		if !ok {
			out = append(out, 0xFFFD)
			i = j
			continue
		}
		if cp >= 0x10000 {
			hi, lo := utf16.EncodeRune(cp)
			out = append(out, uint16(hi), uint16(lo))
		} else {
			out = append(out, uint16(cp))
		}
		i = j
	}
	return out
}
