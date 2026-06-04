//nolint:testpackage // exercises the unexported internalError helper; lives in-package by design.
package auth

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Test_AuthenticateMiddleware_internalErrorPath_DelegatesToCallback exercises
// the failure path inside AuthenticateMiddleware where getUserRole returns
// a non-ErrNoRows error. The middleware must surface that through
// InternalErrorHandler rather than calling http.Error directly.
//
// Because getUserRole reads from sqlite and the unit-test fixture cannot
// easily inject a faulty DB, this test asserts the *callback path* by
// driving internalError directly. The real-DB scenario is covered by an
// integration test in cmd/web (Task 3) where we can fault the session
// store to provoke the branch.
func Test_AuthenticateMiddleware_internalErrorPath_DelegatesToCallback(t *testing.T) {
	t.Parallel()

	called := false
	h := &WebAuthnHandler{ //nolint:exhaustruct // only logger + InternalErrorHandler needed.
		logger: slog.New(slog.DiscardHandler),
		InternalErrorHandler: func(_ http.ResponseWriter, _ *http.Request, _ error) {
			called = true
		},
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/whatever", nil)
	h.internalError(w, r, errors.New("simulated DB failure"))

	if !called {
		t.Error("expected the injected handler to be called for an internal error")
	}
}
