//nolint:testpackage // exercises the unexported internalError helper; lives in-package by design.
package auth

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func Test_WebAuthnHandler_InternalErrorHandler_FallbackWhenNil(t *testing.T) {
	t.Parallel()

	h := &WebAuthnHandler{logger: slog.New(slog.DiscardHandler)} //nolint:exhaustruct // only logger needed.

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/whatever", nil)

	h.internalError(w, r, errors.New("boom"))

	if got := w.Code; got != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", got, http.StatusInternalServerError)
	}
}

func Test_WebAuthnHandler_InternalErrorHandler_CallbackInvoked(t *testing.T) {
	t.Parallel()

	called := false
	var capturedErr error
	h := &WebAuthnHandler{ //nolint:exhaustruct // only logger + InternalErrorHandler needed.
		logger: slog.New(slog.DiscardHandler),
		InternalErrorHandler: func(_ http.ResponseWriter, _ *http.Request, err error) {
			called = true
			capturedErr = err
		},
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/whatever", nil)
	want := errors.New("boom")

	h.internalError(w, r, want)

	if !called {
		t.Error("expected InternalErrorHandler callback to be invoked")
	}
	if !errors.Is(capturedErr, want) {
		t.Errorf("captured error = %v, want %v", capturedErr, want)
	}
	// When the callback handles it, internalError must NOT also write 500.
	if got := w.Code; got != http.StatusOK {
		t.Errorf("status = %d, want %d (callback owns the response)", got, http.StatusOK)
	}
}
