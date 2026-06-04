package logging

import "testing"

func TestNewTraceID_GeneratesDistinctValues(t *testing.T) {
	t.Parallel()
	a := NewTraceID()
	b := NewTraceID()
	if a == "" || b == "" {
		t.Fatalf("got empty trace IDs: %q, %q", a, b)
	}
	if a == b {
		t.Fatalf("expected distinct trace IDs, got %q twice", a)
	}
}
