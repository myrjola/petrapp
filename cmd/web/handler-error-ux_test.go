package main

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

// prodLookupEnv returns the same values as testLookupEnv but also sets
// FLY_APP_NAME so app.devMode becomes false. Uses a "pr-" prefix so the
// VAPID-keys check generates an ephemeral pair instead of failing.
func prodLookupEnv(key string) (string, bool) {
	if key == "FLY_APP_NAME" {
		return "pr-test", true
	}
	return testLookupEnv(key)
}

func Test_application_devErrorUX_render(t *testing.T) {
	ctx := t.Context()

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	doc, err := client.GetDoc(ctx, "/dev/error-ux")
	if err != nil {
		t.Fatalf("Failed to get /dev/error-ux: %v", err)
	}

	for _, heading := range []string{
		"Server-side validation error",
		"Expected business error",
		"Unexpected system error",
		"Client-side network failure",
		"Full-page server error",
	} {
		if doc.Find("h2:contains('"+heading+"')").Length() == 0 {
			t.Errorf("expected card heading %q on /dev/error-ux", heading)
		}
	}

	// Banner-variant reference is present.
	if doc.Find(".banner.banner--success").Length() == 0 {
		t.Error("expected the banner-variant reference to include a success example")
	}
	if doc.Find(".banner.banner--info").Length() == 0 {
		t.Error("expected the banner-variant reference to include an info example")
	}

	// Forms point at the trigger sub-paths.
	for _, action := range []string{
		"/dev/error-ux/trigger/validation",
		"/dev/error-ux/trigger/business",
		"/dev/error-ux/trigger/system",
	} {
		if doc.Find("form[action='"+action+"']").Length() == 0 {
			t.Errorf("expected a form posting to %s", action)
		}
	}
}

func Test_application_devErrorUX_gated_outside_dev_mode(t *testing.T) {
	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), prodLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	for _, path := range []string{
		"/dev/error-ux",
		"/dev/error-ux/server-error",
	} {
		resp, getErr := server.Client().Get(t.Context(), path)
		if getErr != nil {
			t.Fatalf("Failed to GET %s: %v", path, getErr)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s in non-dev mode: got status %d, want 404", path, resp.StatusCode)
		}
	}
}

func Test_application_devErrorUX_triggerValidation_surfacesMessage(t *testing.T) {
	ctx := t.Context()

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	doc, err := client.GetDoc(ctx, "/dev/error-ux")
	if err != nil {
		t.Fatalf("Failed to get /dev/error-ux: %v", err)
	}

	doc, err = client.SubmitForm(ctx, doc, "/dev/error-ux/trigger/validation", nil)
	if err != nil {
		t.Fatalf("Failed to submit validation trigger: %v", err)
	}

	banner := doc.Find(".banner.banner--error[role='alert']")
	if banner.Length() == 0 {
		t.Fatal("expected an error banner with role=alert after validation trigger")
	}
	if !strings.Contains(banner.Text(), "Name must be") {
		t.Errorf("validation banner missing expected message; got %q", banner.Text())
	}
}

func Test_application_devErrorUX_triggerSystem_surfacesGenericMessage(t *testing.T) {
	ctx := t.Context()

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	doc, err := client.GetDoc(ctx, "/dev/error-ux")
	if err != nil {
		t.Fatalf("Failed to get /dev/error-ux: %v", err)
	}

	doc, err = client.SubmitForm(ctx, doc, "/dev/error-ux/trigger/system", nil)
	if err != nil {
		t.Fatalf("Failed to submit system trigger: %v", err)
	}

	banner := doc.Find(".banner.banner--error[role='alert']")
	if banner.Length() == 0 {
		t.Fatal("expected an error banner with role=alert after system trigger")
	}
	if !strings.Contains(banner.Text(), "Couldn't complete that action") {
		t.Errorf("system banner missing generic message; got %q", banner.Text())
	}
}

func Test_application_devErrorUX_triggerUnknownKind_returns404(t *testing.T) {
	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	form := url.Values{}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
		server.URL()+"/dev/error-ux/trigger/bogus", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := server.Client().HTTPClient().Do(req)
	if err != nil {
		t.Fatalf("Failed to POST trigger/bogus: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("POST /dev/error-ux/trigger/bogus: got %d, want 404", resp.StatusCode)
	}
}
