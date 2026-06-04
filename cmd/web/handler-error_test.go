package main

import (
	"strings"
	"testing"

	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/platform/testkit"
)

func Test_sanitiseFromPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "absolute path", in: "/workouts/2026-05-24", want: "/workouts/2026-05-24"},
		{name: "absolute path with query", in: "/workouts/2026-05-24?x=1", want: "/workouts/2026-05-24?x=1"},
		{name: "protocol-relative URL rejected", in: "//evil.example.com/foo", want: ""},
		{name: "absolute http URL rejected", in: "http://evil.example.com/foo", want: ""},
		{name: "relative path without slash rejected", in: "workouts/2026-05-24", want: ""},
		{name: "double-slash inside path rejected", in: "/foo//bar", want: ""},
		{name: "javascript scheme rejected", in: "javascript:alert(1)", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := sanitiseFromPath(tc.in); got != tc.want {
				t.Errorf("sanitiseFromPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func Test_application_errorGET_noFromParam_rendersGoHomeOnly(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	server, err := e2etest.StartServer(t, testkit.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	doc, err := client.GetDoc(ctx, "/error")
	if err != nil {
		t.Fatalf("Get /error: %v", err)
	}

	if doc.Find("h1:contains('Something went wrong')").Length() == 0 {
		t.Error("expected the error title on /error")
	}
	if doc.Find("a[href='/']:contains('Go Home')").Length() == 0 {
		t.Error("expected a Go Home link on /error")
	}
	// No ?from= ⇒ no back link.
	if doc.Find("a.error-back-link").Length() != 0 {
		t.Error("expected no back link when ?from= is absent")
	}
}

func Test_application_errorGET_withSafeFromParam_rendersBackLink(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	server, err := e2etest.StartServer(t, testkit.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	doc, err := client.GetDoc(ctx, "/error?from=%2Fworkouts%2F2026-05-24")
	if err != nil {
		t.Fatalf("Get /error?from=...: %v", err)
	}

	link := doc.Find("a.error-back-link").First()
	if link.Length() == 0 {
		t.Fatal("expected a back link on /error when ?from= is a safe path")
	}
	if href, _ := link.Attr("href"); href != "/workouts/2026-05-24" {
		t.Errorf("back link href = %q, want %q", href, "/workouts/2026-05-24")
	}
	// Progressive enhancement: a child <script> upgrades the click to
	// history.back() so the previous doc restores from bfcache (preserving
	// form state) instead of triggering a fresh GET to the href. The href
	// is the no-JS / no-history fallback and must stay.
	script := link.Find("script").First()
	if script.Length() == 0 {
		t.Fatal("expected a progressive-enhancement <script> inside the back link")
	}
	if body := script.Text(); !strings.Contains(body, "history.back()") {
		t.Errorf("enhancement script must call history.back(); got: %q", body)
	}
	// Retry button is gone — its inline reload script bypassed CSP / Trusted
	// Types only because it lived in this template. Removing the button
	// removes the rationale; the back link is a plain anchor.
	if doc.Find(".error-actions button:contains('Retry')").Length() != 0 {
		t.Error("expected the legacy Retry button to be removed")
	}
}

func Test_application_errorGET_withUnsafeFromParam_omitsBackLink(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	server, err := e2etest.StartServer(t, testkit.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	// Protocol-relative URL — must be rejected by sanitiseFromPath.
	doc, err := client.GetDoc(ctx, "/error?from=%2F%2Fevil.example.com%2Ffoo")
	if err != nil {
		t.Fatalf("Get /error?from=//evil...: %v", err)
	}

	if doc.Find("a.error-back-link").Length() != 0 {
		t.Error("expected no back link when ?from= fails sanitisation")
	}
	// Page still renders.
	if doc.Find("h1:contains('Something went wrong')").Length() == 0 {
		t.Error("expected the error title to still render with unsafe ?from=")
	}
}
