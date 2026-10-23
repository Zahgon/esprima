// Package cli holds what the esparse and esvalidate commands share.
package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"syscall"
)

// ReadErrorMessage words a failure to read a file the way Node.js's
// fs.readFileSync does, which is the text the commands report.
func ReadErrorMessage(err error, name string) string {
	var errno syscall.Errno
	switch {
	case errors.As(err, &errno) && errno == syscall.EISDIR:
		return "EISDIR: illegal operation on a directory, read"
	case errors.Is(err, fs.ErrNotExist):
		return "ENOENT: no such file or directory, open '" + name + "'"
	case errors.Is(err, fs.ErrPermission):
		return "EACCES: permission denied, open '" + name + "'"
	}
	return err.Error()
}

// Writer writes a command's report and keeps the first write error, so the
// command can report it once when it finishes.
type Writer struct {
	w   io.Writer
	err error
}

// NewWriter returns a Writer writing to w.
func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

// Printf writes formatted text unless an earlier write failed.
func (w *Writer) Printf(format string, args ...any) {
	if w.err == nil {
		_, w.err = fmt.Fprintf(w.w, format, args...)
	}
}

// Println writes its operands separated by spaces and a newline unless an
// earlier write failed.
func (w *Writer) Println(args ...any) {
	if w.err == nil {
		_, w.err = fmt.Fprintln(w.w, args...)
	}
}

// Err returns the first write error.
func (w *Writer) Err() error {
	return w.err
}
