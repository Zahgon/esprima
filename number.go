package esprima

import (
	"math"
	"math/big"
	"strconv"
	"strings"
)

// NumberToString formats a float64 the way JavaScript's Number.prototype.
// toString() does for radix 10 (ECMA-262 Number::toString): the shortest
// digits that round-trip, written in plain notation for exponents from -7 to
// 20 and in exponential notation (with an explicit sign) otherwise.
func NumberToString(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	case f == 0:
		return "0"
	}
	if f < 0 {
		return "-" + NumberToString(-f)
	}
	// strconv gives the shortest round-trip digits; take them apart into the
	// digit string and the decimal exponent n (value = 0.digits * 10^n).
	s := strconv.FormatFloat(f, 'e', -1, 64)
	mant, exp, _ := strings.Cut(s, "e")
	digits := strings.Replace(mant, ".", "", 1)
	e, _ := strconv.Atoi(exp)
	k := len(digits)
	n := e + 1
	switch {
	case k <= n && n <= 21:
		return digits + strings.Repeat("0", n-k)
	case 0 < n && n <= 21:
		return digits[:n] + "." + digits[n:]
	case -6 < n && n <= 0:
		return "0." + strings.Repeat("0", -n) + digits
	}
	sign := "+"
	if n-1 < 0 {
		sign = "-"
	}
	ex := strconv.Itoa(abs(n - 1))
	if k == 1 {
		return digits + "e" + sign + ex
	}
	return digits[:1] + "." + digits[1:] + "e" + sign + ex
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// toUint16 is ECMA-262 ToUint16.
func toUint16(f float64) uint16 {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	f = math.Trunc(f)
	m := math.Mod(f, 65536)
	if m < 0 {
		m += 65536
	}
	return uint16(m)
}

// parseIntDigits is parseInt for a string of digits that are all valid in the
// radix. The result is the correctly rounded double, as V8 computes it.
func parseIntDigits(digits string, radix int) float64 {
	if digits == "" {
		return math.NaN()
	}
	n, ok := new(big.Int).SetString(digits, radix)
	if !ok {
		return math.NaN()
	}
	f, _ := new(big.Float).SetInt(n).Float64()
	return f
}

// parseFloatPrefix is parseFloat applied to a numeric literal body such as
// "1.", ".5" or "3e-7" that the scanner has already validated.
func parseFloatPrefix(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		if ne, ok := err.(*strconv.NumError); ok && ne.Err == strconv.ErrRange {
			return f
		}
		return math.NaN()
	}
	return f
}

// parseIntLeading is parseInt(s, radix) for radix 10 or 16: leading
// whitespace is not expected, an optional 0x prefix is skipped for radix 16,
// and the longest run of valid digits is converted.
func parseIntLeading(s string, radix int) float64 {
	if radix == 16 && len(s) >= 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		s = s[2:]
	}
	end := 0
	for end < len(s) && digitValue(s[end]) < radix {
		end++
	}
	if end == 0 {
		return math.NaN()
	}
	if radix == 10 {
		return parseFloatPrefix(s[:end])
	}
	return parseIntDigits(s[:end], radix)
}

func digitValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'z':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'Z':
		return int(c-'A') + 10
	}
	return 99
}
