package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/myrjola/petrapp/internal/platform/testkit"
)

func TestHandlers_CreateThenListShowsItem(t *testing.T) {
	t.Parallel()
	logger := testkit.NewLogger(testkit.NewWriter(t))
	app, cleanup, err := newTestApplication(t, logger)
	if err != nil {
		t.Fatalf("newTestApplication: %v", err)
	}
	t.Cleanup(cleanup)
	h := app.routes()

	form := url.Values{"title": {"walk the dog"}}
	postReq := httptest.NewRequest(http.MethodPost, "/todos", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postRec := httptest.NewRecorder()
	h.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusSeeOther {
		t.Fatalf("POST /todos = %d, want 303", postRec.Code)
	}

	listRec := httptest.NewRecorder()
	h.ServeHTTP(listRec, httptest.NewRequest(http.MethodGet, "/", nil))
	if listRec.Code != http.StatusOK || !strings.Contains(listRec.Body.String(), "walk the dog") {
		t.Fatalf("GET / = %d, body missing item: %s", listRec.Code, listRec.Body.String())
	}
}
