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

// Go port of src/error-handler.ts and src/assert.ts.

package esprima

import "strconv"

// Error is a syntax error. Index is the zero-based UTF-16 offset, LineNumber
// and Column are one-based, and Message is "Line <n>: <description>".
type Error struct {
	Index       int
	LineNumber  int
	Column      int
	Description string
	Message     string
}

// Error returns the message, "Line <n>: <description>".
func (e *Error) Error() string {
	return e.Message
}

// InternalError reports input on which the original JavaScript implementation
// fails with a runtime exception of its own (a TypeError or a failed internal
// assertion) instead of a syntax error.
type InternalError struct {
	Message string
}

// Error returns the message of the runtime exception.
func (e *InternalError) Error() string {
	return e.Message
}

// errorHandler records errors in tolerant mode and raises them otherwise.
// Raising unwinds the recursive-descent parser with a panic carrying the
// *Error; the public entry points recover it and return it as an error value.
type errorHandler struct {
	errors   []*Error
	tolerant bool
}

func (h *errorHandler) recordError(err *Error) {
	h.errors = append(h.errors, err)
}

func (h *errorHandler) tolerate(err *Error) {
	if !h.tolerant {
		panic(err)
	}
	h.recordError(err)
}

func (h *errorHandler) createError(index, line, col int, description string) *Error {
	return &Error{
		Index:       index,
		LineNumber:  line,
		Column:      col,
		Description: description,
		Message:     "Line " + strconv.Itoa(line) + ": " + description,
	}
}

func (h *errorHandler) throwError(index, line, col int, description string) {
	panic(h.createError(index, line, col, description))
}

func (h *errorHandler) tolerateError(index, line, col int, description string) {
	err := h.createError(index, line, col, description)
	if !h.tolerant {
		panic(err)
	}
	h.recordError(err)
}

// internalError raises the runtime exception the original would hit at this
// point (reading a character past the end of the input, a failed assertion).
func internalError(msg string) {
	panic(&InternalError{Message: msg})
}

// catch runs f and returns the syntax or internal error it raised, if any.
// Other panics are programming errors and keep unwinding.
func catch(f func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			switch e := r.(type) {
			case *Error:
				err = e
			case *InternalError:
				err = e
			default:
				panic(r)
			}
		}
	}()
	f()
	return nil
}
