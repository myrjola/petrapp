package testhelpers

import (
	"io"
	"strings"
	"testing"
)

// Writer implements io.Writer and writes to t.Log.
// This allows test logs to be automatically shown only for failed tests,
// following Go testing best practices.
type Writer struct {
	t *testing.T
}

// NewWriter creates a new Writer that writes to t.Log.
// This is a drop-in replacement for testhelpers.NewWriter(t) in tests.
func NewWriter(t *testing.T) io.Writer {
	return &Writer{t: t}
}

// Write implements io.Writer by writing to t.Log.
func (w *Writer) Write(p []byte) (int, error) {
	// Remove trailing newlines to avoid double-spacing in test output.
	output := strings.TrimSuffix(string(p), "\n")
	if output != "" {
		w.t.Log(output)
	}
	return len(p), nil
}
