package main

import (
	"net/http"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

//nolint:paralleltest // Setup subtest registers + promotes an admin that later subtests rely on.
func Test_application_adminExercises(t *testing.T) {
	var (
		ctx = t.Context()
		doc *goquery.Document
		err error
	)

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	client := server.Client()

	// First register to access admin pages
	t.Run("Setup", func(t *testing.T) {
		if _, err = client.Register(ctx); err != nil {
			t.Fatalf("Failed to register: %v", err)
		}

		// Assert that the user is not an admin: mustAdmin redirects to /forbidden.
		httpClient := *client.HTTPClient() // shallow copy preserves jar + transport.
		httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}
		var req *http.Request
		if req, err = http.NewRequestWithContext(ctx, http.MethodGet,
			server.URL()+"/admin/exercises", nil); err != nil {
			t.Fatalf("Build admin exercises request: %v", err)
		}
		var resp *http.Response
		if resp, err = httpClient.Do(req); err != nil {
			t.Fatalf("Failed to get admin exercises page: %v", err)
		}
		if cerr := resp.Body.Close(); cerr != nil {
			t.Fatalf("Close response body: %v", cerr)
		}
		if got, want := resp.StatusCode, http.StatusSeeOther; got != want {
			t.Fatalf("Got status code %d, want %d", got, want)
		}
		if got, want := resp.Header.Get("Location"), "/forbidden"; got != want {
			t.Fatalf("Got Location %q, want %q", got, want)
		}

		// Promote user to admin
		_, err = server.DB().Exec("UPDATE users SET is_admin = 1 WHERE TRUE")
		if err != nil {
			t.Fatalf("Failed to promote user to admin: %v", err)
		}
	})

	// Test viewing the exercises admin page
	t.Run("View exercises admin page", func(t *testing.T) {
		if doc, err = client.GetDoc(ctx, "/admin/exercises"); err != nil {
			t.Fatalf("Failed to get admin exercises page: %v", err)
		}

		// Verify page content
		if doc.Find("h1").Text() != "Exercise Administration" {
			t.Error("Expected 'Exercise Administration' heading")
		}

		// Check for exercise table
		if doc.Find("table").Length() == 0 {
			t.Error("Expected to find exercise table")
		}

		// Check for the form to add a new exercise
		if doc.Find("form[action='/admin/exercises/generate']").Length() == 0 {
			t.Error("Expected to find form to add new exercise")
		}

		// Admin nav renders with Exercises marked active.
		nav := doc.Find(`nav.admin-nav[aria-label="Admin sections"]`)
		if nav.Length() == 0 {
			t.Fatal("expected admin-nav landmark on /admin/exercises")
		}
		if nav.Find(`a.admin-nav__tab[href="/admin/exercises"][aria-current="page"]`).Length() == 0 {
			t.Error("expected Exercises tab to be marked aria-current=page")
		}
		if nav.Find(`a.admin-nav__tab[href="/admin/feature-flags"][aria-current="page"]`).Length() != 0 {
			t.Error("Feature Flags tab must not be active on the exercises page")
		}
	})

	// Test creating a new exercise
	t.Run("Create new exercise", func(t *testing.T) {
		if doc, err = client.GetDoc(ctx, "/admin/exercises"); err != nil {
			t.Fatalf("Failed to get admin exercises page: %v", err)
		}

		// Find the creation form
		form := doc.Find("form[action='/admin/exercises/generate']")
		if form.Length() == 0 {
			t.Fatalf("Exercise creation form not found")
		}

		// Submit the form with test data
		formData := map[string]string{
			"Name": "Test Squat",
		}

		if doc, err = client.SubmitForm(ctx, doc, "/admin/exercises/generate", formData); err != nil {
			t.Fatalf("Failed to submit exercise creation form: %v", err)
		}

		if doc.Find("h1").Text() != "Edit Exercise: Test Squat" {
			t.Error("Expected to be redirected to exercise editing page")
		}

		// The edit page also renders admin-nav with Exercises active.
		editNav := doc.Find(`nav.admin-nav[aria-label="Admin sections"]`)
		if editNav.Length() == 0 {
			t.Fatal("expected admin-nav landmark on the exercise edit page")
		}
		if editNav.Find(`a.admin-nav__tab[href="/admin/exercises"][aria-current="page"]`).Length() == 0 {
			t.Error("expected Exercises tab to be marked aria-current=page on the edit page")
		}
	})

	// Test editing an exercise
	t.Run("Edit exercise", func(t *testing.T) {
		if doc, err = client.GetDoc(ctx, "/admin/exercises"); err != nil {
			t.Fatalf("Failed to get admin exercises page: %v", err)
		}

		// Find the edit link for our created exercise
		var editURL string
		doc.Find("tr:contains('Test Squat') td a:contains('Edit')").Each(func(_ int, s *goquery.Selection) {
			if href, exists := s.Attr("href"); exists {
				editURL = href
			}
		})

		if editURL == "" {
			t.Fatalf("Edit link for Test Squat not found")
		}

		// Navigate to the edit page
		if doc, err = client.GetDoc(ctx, editURL); err != nil {
			t.Fatalf("Failed to get exercise edit page: %v", err)
		}

		// Verify edit page content
		if doc.Find("h1:contains('Edit Exercise: Test Squat')").Length() == 0 {
			t.Error("Expected to find 'Edit Exercise: Test Squat' heading")
		}

		// Update the exercise
		formData := map[string]string{
			"Name":        "Updated Test Squat",
			"Category":    "lower",
			"Type":        "weighted",
			"Primary":     "Quads,Glutes",
			"Secondary":   "Hamstrings,Calves,Abs,Lower Back",
			"Description": "An updated test squat exercise description",
			"rep_min":     "5",
			"rep_max":     "10",
		}

		if doc, err = client.SubmitForm(ctx, doc, editURL, formData); err != nil {
			t.Fatalf("Failed to submit exercise update form: %v", err)
		}

		// Verify we're back at the exercise list page
		if doc.Find("h1").Text() != "Exercise Administration" {
			t.Error("Expected to be redirected back to 'Exercise Administration' page")
		}

		// Check that our exercise was updated
		if doc.Find("td:contains('Updated Test Squat')").Length() == 0 {
			t.Error("Expected to find the updated exercise name in the table")
		}

		// Re-open the edit page and assert rep window round-tripped.
		if doc, err = client.GetDoc(ctx, editURL); err != nil {
			t.Fatalf("Failed to re-open exercise edit page: %v", err)
		}
		if got, _ := doc.Find("input[name='rep_min']").Attr("value"); got != "5" {
			t.Errorf("expected rep_min='5', got %q", got)
		}
		if got, _ := doc.Find("input[name='rep_max']").Attr("value"); got != "10" {
			t.Errorf("expected rep_max='10', got %q", got)
		}
	})

	// User input errors must surface as a 200 with a flash message, not a 500.
	t.Run("Empty name shows validation error instead of 500", func(t *testing.T) {
		if doc, err = client.GetDoc(ctx, "/admin/exercises"); err != nil {
			t.Fatalf("Failed to get admin exercises page: %v", err)
		}

		var editURL string
		doc.Find("tr:contains('Updated Test Squat') td a:contains('Edit')").Each(
			func(_ int, s *goquery.Selection) {
				if href, exists := s.Attr("href"); exists {
					editURL = href
				}
			})
		if editURL == "" {
			t.Fatalf("Edit link for Updated Test Squat not found")
		}

		if doc, err = client.GetDoc(ctx, editURL); err != nil {
			t.Fatalf("Failed to get exercise edit page: %v", err)
		}

		formData := map[string]string{
			"Name":        "",
			"Category":    "lower",
			"Type":        "weighted",
			"Primary":     "Quads,Glutes",
			"Secondary":   "",
			"Description": "",
			"rep_min":     "5",
			"rep_max":     "10",
		}
		if doc, err = client.SubmitForm(ctx, doc, editURL, formData); err != nil {
			t.Fatalf("Failed to submit empty-name update: %v", err)
		}

		// Should land back on the edit page with an alert, not a 500 page.
		if doc.Find("h1:contains('Edit Exercise')").Length() == 0 {
			t.Errorf("Expected to land back on the edit page after validation error")
		}
		if doc.Find("[role=alert]").Length() == 0 {
			t.Errorf("Expected validation alert on the edit page")
		}
		if !strings.Contains(doc.Find("[role=alert]").Text(), "Name is required") {
			t.Errorf("Expected 'Name is required' in alert, got: %s", doc.Find("[role=alert]").Text())
		}
	})

	// Verifies the form size limit (largeMaxFormSize) is large enough to
	// accept descriptions up to the schema's 20KB cap with headroom for
	// other fields and form encoding overhead.
	t.Run("Edit exercise with 15KB description", func(t *testing.T) {
		if doc, err = client.GetDoc(ctx, "/admin/exercises"); err != nil {
			t.Fatalf("Failed to get admin exercises page: %v", err)
		}

		var editURL string
		doc.Find("tr:contains('Updated Test Squat') td a:contains('Edit')").Each(
			func(_ int, s *goquery.Selection) {
				if href, exists := s.Attr("href"); exists {
					editURL = href
				}
			})
		if editURL == "" {
			t.Fatalf("Edit link for Updated Test Squat not found")
		}

		if doc, err = client.GetDoc(ctx, editURL); err != nil {
			t.Fatalf("Failed to get exercise edit page: %v", err)
		}

		const descriptionSize = 15 * 1024
		largeDescription := strings.Repeat("a", descriptionSize)
		formData := map[string]string{
			"Name":        "Updated Test Squat",
			"Category":    "lower",
			"Type":        "weighted",
			"Primary":     "Quads,Glutes",
			"Secondary":   "Hamstrings,Calves,Abs,Lower Back",
			"Description": largeDescription,
			"rep_min":     "5",
			"rep_max":     "10",
		}
		if doc, err = client.SubmitForm(ctx, doc, editURL, formData); err != nil {
			t.Fatalf("Failed to submit exercise update with large description: %v", err)
		}
		if doc.Find("h1").Text() != "Exercise Administration" {
			t.Error("Expected to be redirected back to 'Exercise Administration' page")
		}

		// Round-trip: re-open the edit page and assert the description survived.
		if doc, err = client.GetDoc(ctx, editURL); err != nil {
			t.Fatalf("Failed to re-open exercise edit page: %v", err)
		}
		got := doc.Find("textarea[name='description']").Text()
		if len(got) != descriptionSize {
			t.Errorf("expected description length %d, got %d", descriptionSize, len(got))
		}
	})

	// Rep window is required for non-time-based exercises and must satisfy
	// min <= max within [1, 50]. Validation errors surface via flash + redirect,
	// matching the empty-name pattern.
	t.Run("Invalid rep window shows validation error", func(t *testing.T) {
		if doc, err = client.GetDoc(ctx, "/admin/exercises"); err != nil {
			t.Fatalf("Failed to get admin exercises page: %v", err)
		}
		var editURL string
		doc.Find("tr:contains('Updated Test Squat') td a:contains('Edit')").Each(
			func(_ int, s *goquery.Selection) {
				if href, exists := s.Attr("href"); exists {
					editURL = href
				}
			})
		if editURL == "" {
			t.Fatalf("Edit link for Updated Test Squat not found")
		}
		if doc, err = client.GetDoc(ctx, editURL); err != nil {
			t.Fatalf("Failed to get exercise edit page: %v", err)
		}

		formData := map[string]string{
			"Name":        "Updated Test Squat",
			"Category":    "lower",
			"Type":        "weighted",
			"Primary":     "Quads,Glutes",
			"Secondary":   "",
			"Description": "",
			"rep_min":     "12",
			"rep_max":     "8",
		}
		if doc, err = client.SubmitForm(ctx, doc, editURL, formData); err != nil {
			t.Fatalf("Failed to submit invalid rep window: %v", err)
		}
		if doc.Find("[role=alert]").Length() == 0 {
			t.Errorf("Expected validation alert on the edit page")
		}
		if !strings.Contains(doc.Find("[role=alert]").Text(), "less than or equal to") {
			t.Errorf("Expected min<=max message in alert, got: %s", doc.Find("[role=alert]").Text())
		}
	})

	// The generate form's empty-name case must surface as a flash banner on the
	// admin page, not a 500. Today the handler returns serverError(w, r, nil).
	t.Run("Generate with empty name shows validation error", func(t *testing.T) {
		if doc, err = client.GetDoc(ctx, "/admin/exercises"); err != nil {
			t.Fatalf("Failed to get admin exercises page: %v", err)
		}
		formData := map[string]string{"Name": ""}
		if doc, err = client.SubmitForm(ctx, doc, "/admin/exercises/generate", formData); err != nil {
			t.Fatalf("Failed to submit generate form with empty name: %v", err)
		}
		if doc.Find("h1").Text() != "Exercise Administration" {
			t.Error("Expected to land back on the exercise admin page")
		}
		if doc.Find("[role=alert]").Length() == 0 {
			t.Error("Expected a validation alert after submitting an empty exercise name")
		}
		if !strings.Contains(doc.Find("[role=alert]").Text(), "Exercise name is required") {
			t.Errorf("Expected 'Exercise name is required' in alert, got: %s",
				doc.Find("[role=alert]").Text())
		}
	})
}
