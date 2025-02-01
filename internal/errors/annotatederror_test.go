package errors_test

import (
	"bytes"
	"fmt"
	"github.com/myrjola/petrapp/internal/errors"
	"github.com/myrjola/petrapp/internal/testhelpers"
	"github.com/stretchr/testify/require"
	"log/slog"
	"testing"
	"time"
)

func TestAnnotatedError(t *testing.T) {
	annotatedError := errors.New("test error", slog.Int("id", 123))
	require.Equal(t, "test error", annotatedError.Error())

	// Assert that wrapping sentinel errors work as expected.
	sentinel := errors.NewSentinel("test error")
	require.NotErrorIs(t, annotatedError, errors.NewSentinel("test error"))
	wrapped := errors.Join(annotatedError, sentinel)
	require.ErrorIs(t, wrapped, sentinel)
	require.True(t, errors.Is(wrapped, sentinel))
	require.Equal(t, "test error\ntest error", wrapped.Error())

	// Ensure the latest annotations override older ones
	wrapped = errors.Wrap(wrapped, "wrap error 1", slog.String("user", "johndoe"))
	wrapped = errors.Wrap(wrapped, "wrap error 2", slog.Duration("duration", time.Second))
	//goland:noinspection GoDfaNilDereference
	require.Equal(t, "wrap error 2: wrap error 1: test error\ntest error", wrapped.Error())

	// Assert that we can find the annotated error
	annotated := errors.AnnotatedError{}
	require.True(t, errors.As(wrapped, &annotated))
	require.Equal(t, "wrap error 2", annotated.Error())

	var buf bytes.Buffer
	l := testhelpers.NewLogger(&buf)
	l.Info("test", errors.SlogError(wrapped))
	logLine := buf.String()
	require.Contains(t, logLine, "id=123")
	require.Contains(t, logLine, "user=johndoe")
	require.Contains(t, logLine, "duration=1s")
	require.Contains(t, logLine, "annotatederror_test.go:15")
	require.Contains(t, logLine, "annotatederror_test.go:27")
	require.Contains(t, logLine, "annotatederror_test.go:28")
	// Assert we didn't mess up the stack trace skips.
	require.NotContains(t, logLine, "annotatederror.go")

	// Try to break things by passing a nil error and other wonkiness.
	errors.SlogError(errors.Join(nil, nil, errors.NewSentinel("sentinel"), errors.New("test")))
	errors.SlogError(nil)
	errors.SlogError(fmt.Errorf("test: %w", errors.NewSentinel("sentinel")))
	errors.SlogError(errors.Join(errors.NewSentinel("sentinel1"), errors.NewSentinel("sentinel2")))
	errors.SlogError(errors.Wrap(nil, "wrap error"))
	errors.SlogError(errors.Wrap(errors.Join(nil, nil), "wrap error"))
	_ = errors.Unwrap(errors.Wrap(errors.NewSentinel("sentinel"), "wrap error"))
}
