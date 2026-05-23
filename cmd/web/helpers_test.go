package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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
