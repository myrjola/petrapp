package main

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/alexedwards/scs/v2"
	"github.com/alexedwards/scs/v2/memstore"
	"github.com/myrjola/petrapp/internal/domain"
)

// newTestSessionManager builds an in-memory scs session manager for tests
// that need to round-trip flash messages. The session is not persisted;
// each test gets a fresh empty store.
func newTestSessionManager(t *testing.T) *scs.SessionManager {
	t.Helper()
	sm := scs.New()
	sm.Store = memstore.New()
	return sm
}

// newTestApplicationForTemplateRender returns a minimal *application ready to
// render templates against the on-disk ui/templates tree. Used by tests that
// exercise the non-stacknav serverError path which renders error.gohtml.
func newTestApplicationForTemplateRender(t *testing.T) *application {
	t.Helper()
	templatePath, err := resolveAndVerifyTemplatePath("")
	if err != nil {
		t.Fatalf("resolve template path: %v", err)
	}
	return &application{ //nolint:exhaustruct // only the fields touched by render matter here.
		logger:          slog.New(slog.DiscardHandler),
		templateFS:      os.DirFS(templatePath),
		parsedTemplates: newTemplateCache(),
		devMode:         true,
	}
}

func Test_redirect_StackNavRequest_Returns200WithXLocation(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/whatever", nil)
	r.Header.Set("X-Requested-With", "stacknav")

	redirect(w, r, "/target")

	if got := w.Code; got != http.StatusOK {
		t.Errorf("status = %d, want %d", got, http.StatusOK)
	}
	if got := w.Header().Get("X-Location"); got != "/target" {
		t.Errorf("X-Location = %q, want %q", got, "/target")
	}
	if got := w.Header().Get("Location"); got != "" {
		t.Errorf("Location should not be set for stacknav request, got %q", got)
	}
	if got := w.Body.Len(); got != 0 {
		t.Errorf("body length = %d, want 0", got)
	}
}

func Test_redirect_PlainRequest_Returns303SeeOther(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/whatever", nil)

	redirect(w, r, "/target")

	if got := w.Code; got != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", got, http.StatusSeeOther)
	}
	if got := w.Header().Get("Location"); got != "/target" {
		t.Errorf("Location = %q, want %q", got, "/target")
	}
	if got := w.Header().Get("X-Location"); got != "" {
		t.Errorf("X-Location should not be set for plain request, got %q", got)
	}
}

func Test_redirectReplace_StackNavRequest_SetsXLocationAndXReplaceURL(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/whatever", nil)
	r.Header.Set("X-Requested-With", "stacknav")

	redirectReplace(w, r, "/target")

	if got := w.Code; got != http.StatusOK {
		t.Errorf("status = %d, want %d", got, http.StatusOK)
	}
	if got := w.Header().Get("X-Location"); got != "/target" {
		t.Errorf("X-Location = %q, want %q", got, "/target")
	}
	if got := w.Header().Get("X-Replace-Url"); got != "true" {
		t.Errorf("X-Replace-Url = %q, want %q", got, "true")
	}
	if got := w.Header().Get("Location"); got != "" {
		t.Errorf("Location should not be set for stacknav request, got %q", got)
	}
	if got := w.Body.Len(); got != 0 {
		t.Errorf("body length = %d, want 0", got)
	}
}

func Test_redirectReplace_PlainRequest_Returns303SeeOtherWithoutXReplace(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/whatever", nil)

	redirectReplace(w, r, "/target")

	if got := w.Code; got != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", got, http.StatusSeeOther)
	}
	if got := w.Header().Get("Location"); got != "/target" {
		t.Errorf("Location = %q, want %q", got, "/target")
	}
	if got := w.Header().Get("X-Replace-Url"); got != "" {
		t.Errorf("X-Replace-Url should not be set for plain request, got %q", got)
	}
	if got := w.Header().Get("X-Location"); got != "" {
		t.Errorf("X-Location should not be set for plain request, got %q", got)
	}
}

func Test_serverError_StackNavRequest_NavigatesToErrorPage(t *testing.T) {
	t.Parallel()

	app := &application{logger: slog.New(slog.DiscardHandler)} //nolint:exhaustruct // only logger needed.

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/workouts/2026-05-24/complete", nil)
	r.Header.Set("X-Requested-With", "stacknav")
	r.Header.Set("Referer", "https://example.test/workouts/2026-05-24")
	r.Host = "example.test"

	app.serverError(w, r, errors.New("boom"))

	if got := w.Code; got != http.StatusOK {
		t.Errorf("status = %d, want %d", got, http.StatusOK)
	}
	want := "/error?from=%2Fworkouts%2F2026-05-24"
	if got := w.Header().Get("X-Location"); got != want {
		t.Errorf("X-Location = %q, want %q", got, want)
	}
	if got := w.Body.Len(); got != 0 {
		t.Errorf("body length = %d, want 0", got)
	}
}

func Test_serverError_StackNavRequest_CrossOriginReferer_OmitsFrom(t *testing.T) {
	t.Parallel()

	app := &application{logger: slog.New(slog.DiscardHandler)} //nolint:exhaustruct // only logger needed.

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/workouts/2026-05-24/complete", nil)
	r.Header.Set("X-Requested-With", "stacknav")
	r.Header.Set("Referer", "https://other.example/workouts/2026-05-24")
	r.Host = "example.test"

	app.serverError(w, r, errors.New("boom"))

	if got := w.Header().Get("X-Location"); got != "/error" {
		t.Errorf("X-Location = %q, want /error (cross-origin Referer should be dropped)", got)
	}
}

func Test_serverError_StackNavRequest_NoReferer_BareErrorPath(t *testing.T) {
	t.Parallel()

	app := &application{logger: slog.New(slog.DiscardHandler)} //nolint:exhaustruct // only logger needed.

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/workouts/2026-05-24/complete", nil)
	r.Header.Set("X-Requested-With", "stacknav")

	app.serverError(w, r, errors.New("boom"))

	if got := w.Header().Get("X-Location"); got != "/error" {
		t.Errorf("X-Location = %q, want /error", got)
	}
}

func Test_serverError_NonStackNavRequest_Renders500Body(t *testing.T) {
	t.Parallel()

	// This case uses the real template renderer, so set up the template FS.
	app := newTestApplicationForTemplateRender(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/something", nil)

	app.serverError(w, r, errors.New("boom"))

	if got := w.Code; got != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", got, http.StatusInternalServerError)
	}
	if !strings.Contains(w.Body.String(), "Something went wrong") {
		t.Errorf("expected the error page body, got %q", w.Body.String())
	}
}

func Test_userError_ValidationError_FlashesAndRedirects(t *testing.T) {
	t.Parallel()

	app := &application{ //nolint:exhaustruct // only fields touched by userError matter here.
		logger:         slog.New(slog.DiscardHandler),
		sessionManager: newTestSessionManager(t),
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/whatever", nil)
	ctx, err := app.sessionManager.Load(r.Context(), "")
	if err != nil {
		t.Fatalf("session load: %v", err)
	}
	r = r.WithContext(ctx)

	ve := domain.ValidationError{Message: "Name must be 1–50 characters."}
	app.userError(w, r, ve, "/safe")

	if got := w.Code; got != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", got, http.StatusSeeOther)
	}
	if got := w.Header().Get("Location"); got != "/safe" {
		t.Errorf("Location = %q, want /safe", got)
	}
	// The flash must be populated for the safe URL's GET to surface the banner.
	if got := app.popFlashError(r.Context()); got != "Name must be 1–50 characters." {
		t.Errorf("flash = %q, want validation message", got)
	}
}

func Test_userError_NonValidation_StackNav_DelegatesToServerError(t *testing.T) {
	t.Parallel()

	app := &application{ //nolint:exhaustruct // only fields touched by userError matter here.
		logger:         slog.New(slog.DiscardHandler),
		sessionManager: newTestSessionManager(t),
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/workouts/2026-05-24/add-exercise", nil)
	r.Header.Set("X-Requested-With", "stacknav")
	r.Header.Set("Referer", "https://example.test/workouts/2026-05-24")
	r.Host = "example.test"
	ctx, err := app.sessionManager.Load(r.Context(), "")
	if err != nil {
		t.Fatalf("session load: %v", err)
	}
	r = r.WithContext(ctx)

	app.userError(w, r, errors.New("db hiccup"), "/workouts/2026-05-24")

	// Shim path: serverError handles it.
	if got := w.Code; got != http.StatusOK {
		t.Errorf("status = %d, want %d (delegation to serverError shim path)", got, http.StatusOK)
	}
	want := "/error?from=%2Fworkouts%2F2026-05-24"
	if got := w.Header().Get("X-Location"); got != want {
		t.Errorf("X-Location = %q, want %q", got, want)
	}
	// Flash must NOT be populated — the banner-on-safe-URL UX is intentionally
	// abandoned for non-validation errors.
	if got := app.popFlashError(r.Context()); got != "" {
		t.Errorf("flash = %q, want empty (non-validation must not flash)", got)
	}
}

func Test_recoverPanic_StackNavRequest_NavigatesToErrorPage(t *testing.T) {
	t.Parallel()

	app := &application{logger: slog.New(slog.DiscardHandler)} //nolint:exhaustruct // only logger needed.
	panicking := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("simulated handler panic")
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/whatever", nil)
	r.Header.Set("X-Requested-With", "stacknav")
	r.Header.Set("Referer", "https://example.test/some/page")
	r.Host = "example.test"

	app.recoverPanic(panicking).ServeHTTP(w, r)

	if got := w.Code; got != http.StatusOK {
		t.Errorf("status = %d, want %d", got, http.StatusOK)
	}
	want := "/error?from=%2Fsome%2Fpage"
	if got := w.Header().Get("X-Location"); got != want {
		t.Errorf("X-Location = %q, want %q", got, want)
	}
}
