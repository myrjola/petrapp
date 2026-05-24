package main

import (
	"testing"

	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/testhelpers"
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

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
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
