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

// Go port of src/character.ts.

package esprima

import "sort"

// Character classification on code points (or single UTF-16 code units).
// A negative value stands for a position past the end of the input.

func inRanges(table []uint32, cp int) bool {
	n := len(table) / 2
	i := sort.Search(n, func(i int) bool { return table[2*i+1] >= uint32(cp) })
	return i < n && table[2*i] <= uint32(cp)
}

// https://tc39.github.io/ecma262/#sec-white-space
func isWhiteSpace(cp int) bool {
	switch cp {
	case 0x20, 0x09, 0x0B, 0x0C, 0xA0, 0x1680, 0x2000, 0x2001, 0x2002, 0x2003, 0x2004, 0x2005, 0x2006,
		0x2007, 0x2008, 0x2009, 0x200A, 0x202F, 0x205F, 0x3000, 0xFEFF:
		return true
	}
	return false
}

// https://tc39.github.io/ecma262/#sec-line-terminators
func isLineTerminator(cp int) bool {
	return cp == 0x0A || cp == 0x0D || cp == 0x2028 || cp == 0x2029
}

// https://tc39.github.io/ecma262/#sec-names-and-keywords
func isIdentifierStart(cp int) bool {
	return cp == 0x24 || cp == 0x5F || // $ and _
		(cp >= 0x41 && cp <= 0x5A) || // A..Z
		(cp >= 0x61 && cp <= 0x7A) || // a..z
		cp == 0x5C || // \ (backslash)
		(cp >= 0x80 && inRanges(idStartRanges, cp))
}

func isIdentifierPart(cp int) bool {
	return cp == 0x24 || cp == 0x5F ||
		(cp >= 0x41 && cp <= 0x5A) ||
		(cp >= 0x61 && cp <= 0x7A) ||
		(cp >= 0x30 && cp <= 0x39) ||
		cp == 0x5C ||
		(cp >= 0x80 && inRanges(idPartRanges, cp))
}

// https://tc39.github.io/ecma262/#sec-literals-numeric-literals
func isDecimalDigit(cp int) bool {
	return cp >= 0x30 && cp <= 0x39
}

func isHexDigit(cp int) bool {
	return (cp >= 0x30 && cp <= 0x39) || (cp >= 0x41 && cp <= 0x46) || (cp >= 0x61 && cp <= 0x66)
}

func isOctalDigit(cp int) bool {
	return cp >= 0x30 && cp <= 0x37
}

// hexValue returns the value of a hexadecimal digit code unit.
func hexValue(cu int) int {
	switch {
	case cu >= '0' && cu <= '9':
		return cu - '0'
	case cu >= 'a' && cu <= 'f':
		return cu - 'a' + 10
	case cu >= 'A' && cu <= 'F':
		return cu - 'A' + 10
	}
	return -1
}
