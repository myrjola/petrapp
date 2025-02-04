package errors_test

import (
	"bytes"
	"fmt"
	"github.com/myrjola/petrapp/internal/errors"
	"github.com/myrjola/petrapp/internal/testhelpers"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestAnnotatedError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "simple error",
			err:  errors.NewSentinel("simple error"),
			want: "simple error",
		},
		{
			name: "annotated error",
			err:  errors.Wrap(errors.NewSentinel("root cause"), "context", slog.String("key", "value")),
			want: "context: root cause",
		},
		{
			name: "nested annotated error",
			err: errors.Wrap(
				errors.Wrap(errors.NewSentinel("root cause"), "inner context"),
				"outer context",
			),
			want: "outer context: inner context: root cause",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUnwrap(t *testing.T) {
	rootErr := errors.NewSentinel("root error")
	wrappedErr := fmt.Errorf("context: %w", rootErr)

	if unwrapped := errors.Unwrap(wrappedErr); !errors.Is(unwrapped, rootErr) {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, rootErr)
	}

	if unwrapped := errors.Unwrap(rootErr); unwrapped != nil {
		t.Errorf("Unwrap() = %v, want nil", unwrapped)
	}
}

func TestIs(t *testing.T) {
	rootErr := errors.NewSentinel("root error")
	wrappedErr := errors.Wrap(rootErr, "context")

	if !errors.Is(wrappedErr, rootErr) {
		t.Errorf("Is() = false, want true for wrapped error")
	}

	if errors.Is(wrappedErr, errors.NewSentinel("different error")) {
		t.Errorf("Is() = true, want false for different error")
	}
}

func TestAs(t *testing.T) {
	rootErr := &customError{"custom error"}
	wrappedErr := errors.Wrap(rootErr, "context")

	var target *customError
	if !errors.As(wrappedErr, &target) {
		t.Errorf("As() = false, want true")
	}

	if target != rootErr {
		t.Errorf("As() target = %v, want %v", target, rootErr)
	}

	var wrongTarget *wrongError
	if errors.As(wrappedErr, &wrongTarget) {
		t.Errorf("As() = true, want false for wrong error type")
	}
}

func TestSlogError(t *testing.T) {
	err := errors.Wrap(errors.NewSentinel("root cause"), "context",
		slog.String("key", "value"), slog.Duration("duration", time.Second))
	var buf bytes.Buffer
	l := testhelpers.NewLogger(&buf)
	l.Info("test", errors.SlogError(err))
	logLine := buf.String()
	expectedContent := []string{
		"error.annotations.key=value",
		"error.annotations.duration=1s",
		"annotatederror_test.go:95",
	}
	for _, content := range expectedContent {
		if !strings.Contains(logLine, content) {
			t.Errorf("expected log line %s to contain %s", logLine, content)
		}
	}

	// Assert we didn't mess up the stack trace skips.
	if strings.Contains(logLine, "annotatederror.go") {
		t.Fatal("expected annotatederror.go NOT to be in log line")
	}

	// Try to break things by passing a nil error and other wonkiness.
	errors.SlogError(errors.Join(nil, nil, errors.NewSentinel("sentinel"), errors.New("test")))
	errors.SlogError(nil)
	errors.SlogError(fmt.Errorf("test: %w", errors.NewSentinel("sentinel")))
	errors.SlogError(errors.Join(errors.NewSentinel("sentinel1"), errors.NewSentinel("sentinel2")))
	errors.SlogError(errors.Wrap(nil, "wrap error"))
	errors.SlogError(errors.Wrap(errors.Join(nil, nil), "wrap error"))
	_ = errors.Unwrap(errors.Wrap(errors.NewSentinel("sentinel"), "wrap error"))
}

type customError struct {
	msg string
}

func (e *customError) Error() string {
	return e.msg
}

type wrongError struct{}

func (e *wrongError) Error() string {
	return "wrong error"
}

func TestDecoratePanic(t *testing.T) {
	defer func() {
		excp := recover()
		err := errors.DecoratePanic(excp)
		if err == nil {
			t.Fatal("expected error")
		}
		if got, want := err.Error(), "panic: test"; got != want {
			t.Errorf("err.Error(): got %q, want %q", got, want)
		}
		attr := errors.SlogError(err)
		if got, contains := attr.String(), "annotatederror_test.go:156"; !strings.Contains(got, contains) {
			t.Errorf("attr.String(): expected %q to contain %q", got, contains)
		}
	}()
	panic("test")
}
