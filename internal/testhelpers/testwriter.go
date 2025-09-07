package testhelpers

import (
	"io"
	"strings"
	"testing"
)

// Writer implements io.Writer and writes to t.Log.
// This allows test logs to be automatically shown only for failed tests.
type Writer struct {
	t        *testing.T
	testDone chan struct{}
}

// NewWriter creates a new Writer that writes to t.Log.
// This is a drop-in replacement for testhelpers.NewWriter(t) in tests.
func NewWriter(t *testing.T) io.Writer {
	w := &Writer{
		t:        t,
		testDone: make(chan struct{}),
	}
	// Close the writer when the test finishes to prevent data races
	t.Cleanup(func() {
		close(w.testDone)
	})
	return w
}

// Write implements io.Writer by writing to t.Log.
func (w *Writer) Write(p []byte) (int, error) {
	select {
	// If the test has finished, panic to catch server shutdown issues
	case <-w.testDone:
		panic("testwriter: attempted to write after test completion. Did you remember to t.Cleanup(server.Shutdown)?")
	default:
		// Remove trailing newlines to avoid double-spacing in test output.
		output := strings.TrimSuffix(string(p), "\n")
		if output != "" {
			w.t.Log(output)
		}
		return len(p), nil
	}
}
