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
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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

func Test_application_devErrorUX_triggerSystem_navigatesToErrorPage(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	// GET first so we have a session + a Referer that resolves to /dev/error-ux.
	if _, err = client.GetDoc(ctx, "/dev/error-ux"); err != nil {
		t.Fatalf("Failed to get /dev/error-ux: %v", err)
	}

	// POST the system trigger directly with the stacknav header so we exercise
	// the shim wire contract. SubmitForm follows redirects; here we want to
	// observe the X-Location header on the response, not the followed page.
	target := server.URL() + "/dev/error-ux/trigger/system"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(""))
	if err != nil {
		t.Fatalf("Build POST request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "stacknav")
	req.Header.Set("Referer", server.URL()+"/dev/error-ux")

	httpClient := *client.HTTPClient()
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("POST system trigger: %v", err)
	}
	if cerr := resp.Body.Close(); cerr != nil {
		t.Fatalf("Close response body: %v", cerr)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	want := "/error?from=%2Fdev%2Ferror-ux"
	if got := resp.Header.Get("X-Location"); got != want {
		t.Errorf("X-Location = %q, want %q", got, want)
	}
}

func Test_application_home_devLinks_devMode(t *testing.T) {
	t.Parallel()

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	doc, err := server.Client().GetDoc(t.Context(), "/")
	if err != nil {
		t.Fatalf("Failed to GET /: %v", err)
	}

	for _, href := range []string{"/dev/styleguide", "/dev/error-ux"} {
		if doc.Find("a[href='"+href+"']").Length() == 0 {
			t.Errorf("expected dev-mode home to surface a link to %s", href)
		}
	}
}

func Test_application_home_devLinks_hiddenOutsideDevMode(t *testing.T) {
	t.Parallel()

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), prodLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	doc, err := server.Client().GetDoc(t.Context(), "/")
	if err != nil {
		t.Fatalf("Failed to GET /: %v", err)
	}

	for _, href := range []string{"/dev/styleguide", "/dev/error-ux"} {
		if doc.Find("a[href='"+href+"']").Length() != 0 {
			t.Errorf("non-dev home should not surface a link to %s", href)
		}
	}
}

func Test_application_devErrorUX_triggerUnknownKind_returns404(t *testing.T) {
	t.Parallel()

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
