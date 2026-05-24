package main

import (
	"net/http"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

func Test_application_forbiddenGET_rendersPageWith403(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	resp, err := client.Get(ctx, "/forbidden")
	if err != nil {
		t.Fatalf("Get /forbidden: %v", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			t.Errorf("Close response body: %v", cerr)
		}
	}()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		t.Fatalf("Parse /forbidden body: %v", err)
	}

	if doc.Find("h1:contains('403')").Length() == 0 {
		t.Error("expected the 403 title on /forbidden")
	}
	if doc.Find("h2:contains('Forbidden')").Length() == 0 {
		t.Error("expected the Forbidden subtitle on /forbidden")
	}
	if doc.Find("a[href='/']:contains('Go Home')").Length() == 0 {
		t.Error("expected a Go Home link on /forbidden")
	}
}

func Test_application_mustAdmin_NonAdmin_LandsOnForbiddenPage(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Failed to register: %v", err)
	}

	// Default client follows the 303 redirect; we land on /forbidden which
	// renders a 403 body.
	resp, err := client.Get(ctx, "/admin/feature-flags")
	if err != nil {
		t.Fatalf("Get /admin/feature-flags as non-admin: %v", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			t.Errorf("Close response body: %v", cerr)
		}
	}()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status after redirect = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
	if got := resp.Request.URL.Path; got != "/forbidden" {
		t.Errorf("final URL path = %q, want /forbidden", got)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		t.Fatalf("Parse /forbidden body: %v", err)
	}
	if doc.Find("h1:contains('403')").Length() == 0 {
		t.Error("expected the 403 title after non-admin hits an admin route")
	}
	if doc.Find("h2:contains('Forbidden')").Length() == 0 {
		t.Error("expected the Forbidden subtitle after non-admin hits an admin route")
	}
}
