package main

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	_ "github.com/mattn/go-sqlite3"
	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

func Test_application_preferences(t *testing.T) {
	var (
		ctx = t.Context()
		doc *goquery.Document
		err error
	)

	server, err := e2etest.StartServer(t.Context(), testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	t.Cleanup(server.Shutdown)

	client := server.Client()

	// Requires authentication
	var resp *http.Response
	resp, err = client.Get(ctx, "/preferences")
	if err != nil {
		t.Fatalf("Failed to get preferences: %v", err)
	}
	if got, want := resp.Request.URL.Path, "/"; got != want {
		t.Errorf("Expected redirect to %q, got %q", want, got)
	}

	// Shows default preferences
	// First register to get authenticated
	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Failed to register: %v", err)
	}

	// Then navigate to preferences
	if doc, err = client.GetDoc(ctx, "/preferences"); err != nil {
		t.Fatalf("Failed to get preferences: %v", err)
	}

	// Check that we're on the preferences page
	if doc.Find("h1").Text() != "Weekly Schedule" {
		t.Error("Expected to be on preferences page")
	}

	// By default, all days should be set to rest day (0 minutes)
	weekdays := map[string]int{
		"monday":    0,
		"tuesday":   0,
		"wednesday": 0,
		"thursday":  0,
		"friday":    0,
		"saturday":  0,
		"sunday":    0,
	}
	verifySelected(t, doc, weekdays)

	// Can update preferences.
	// Submit form with Monday at 60 minutes and Wednesday at 45 minutes
	formData := map[string]string{
		"Monday":    "60",
		"Wednesday": "45",
	}
	if doc, err = client.SubmitForm(ctx, doc, "/preferences", formData); err != nil {
		t.Fatalf("Failed to submit form: %v", err)
	}

	// After submission, we go to the home page.
	if doc.Url.Path != "/" {
		t.Errorf("Expected to be on home page, got %q", doc.Url.Path)
	}

	// Navigate to preferences again
	if doc, err = client.GetDoc(ctx, "/preferences"); err != nil {
		t.Fatalf("Failed to get preferences: %v", err)
	}

	weekdaysAfterSubmit := map[string]int{
		"monday":    60,
		"tuesday":   0,
		"wednesday": 45,
		"thursday":  0,
		"friday":    0,
		"saturday":  0,
		"sunday":    0,
	}
	verifySelected(t, doc, weekdaysAfterSubmit)

	// First logout
	if _, err = client.Logout(ctx); err != nil {
		t.Fatalf("Failed to logout: %v", err)
	}

	// Then login again
	if _, err = client.Login(ctx); err != nil {
		t.Fatalf("Failed to login: %v", err)
	}

	// Navigate to preferences
	if doc, err = client.GetDoc(ctx, "/preferences"); err != nil {
		t.Fatalf("Failed to get preferences: %v", err)
	}

	// Verify preferences were persisted
	weekdaysAfterPersistence := map[string]int{
		"monday":    60,
		"tuesday":   0,
		"wednesday": 45,
		"thursday":  0,
		"friday":    0,
		"saturday":  0,
		"sunday":    0,
	}
	verifySelected(t, doc, weekdaysAfterPersistence)
}

func Test_application_exportUserData(t *testing.T) {
	var (
		ctx = t.Context()
		doc *goquery.Document
		err error
	)

	server, err := e2etest.StartServer(t.Context(), testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	client := server.Client()

	// First register to get authenticated
	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Failed to register: %v", err)
	}

	// Start a workout to generate some data for the user
	today := time.Now().Format("2006-01-02")
	formData := map[string]string{}
	if doc, err = client.GetDoc(ctx, "/"); err != nil {
		t.Fatalf("Failed to get home page: %v", err)
	}

	// Find and submit the start workout form for today
	startForm := doc.Find(fmt.Sprintf("form[action='/workouts/%s/start']", today))
	if startForm.Length() == 0 {
		t.Fatalf("Start workout form not found for today's date: %s", today)
	}

	if _, err = client.SubmitForm(ctx, doc, fmt.Sprintf("/workouts/%s/start", today), formData); err != nil {
		t.Fatalf("Failed to start workout: %v", err)
	}

	// Export the user's data
	var resp *http.Response
	resp, err = client.Get(ctx, "/preferences/export-data")
	if err != nil {
		t.Fatalf("Failed to export user data: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	// Verify we got a successful response
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	// Verify the content type is SQLite
	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/x-sqlite3" {
		t.Errorf("Expected Content-Type application/x-sqlite3, got %s", contentType)
	}

	// Verify Content-Disposition header for download
	contentDisposition := resp.Header.Get("Content-Disposition")
	if !strings.Contains(contentDisposition, "attachment") || !strings.Contains(contentDisposition, ".sqlite3") {
		t.Errorf("Expected Content-Disposition header with attachment and .sqlite3 filename, got %s", contentDisposition)
	}

	// Read the SQLite data into a temporary file
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "user-export.sqlite3")

	file, err := os.Create(tempFile)
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() {
		_ = file.Close()
	}()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		t.Fatalf("Failed to copy export data to temp file: %v", err)
	}

	// Close the file so we can open it with sql.Open
	if err = file.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	// Open the downloaded SQLite file and verify contents
	db, err := sql.Open("sqlite3", tempFile)
	if err != nil {
		t.Fatalf("Failed to open SQLite file: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	// Verify there's a single user
	var userCount int
	err = db.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
	if err != nil {
		t.Fatalf("Failed to query users table: %v", err)
	}
	if userCount != 1 {
		t.Errorf("Expected 1 user in export, got %d", userCount)
	}

	// Verify there's a single workout session
	var sessionCount int
	err = db.QueryRow("SELECT COUNT(*) FROM workout_sessions").Scan(&sessionCount)
	if err != nil {
		t.Fatalf("Failed to query workout_sessions table: %v", err)
	}
	if sessionCount != 1 {
		t.Errorf("Expected 1 workout session in export, got %d", sessionCount)
	}

	// Verify the session has the correct date (today)
	var sessionDate string
	err = db.QueryRow("SELECT workout_date FROM workout_sessions LIMIT 1").Scan(&sessionDate)
	if err != nil {
		t.Fatalf("Failed to query session date: %v", err)
	}
	if sessionDate != today {
		t.Errorf("Expected session date %s, got %s", today, sessionDate)
	}

	// Verify the session is started (has started_at timestamp)
	var startedAt sql.NullString
	err = db.QueryRow("SELECT started_at FROM workout_sessions LIMIT 1").Scan(&startedAt)
	if err != nil {
		t.Fatalf("Failed to query session started_at: %v", err)
	}
	if !startedAt.Valid || startedAt.String == "" {
		t.Error("Expected session to be started (started_at should not be NULL or empty)")
	}
}

func Test_application_deleteUser(t *testing.T) {
	var (
		ctx = t.Context()
		doc *goquery.Document
		err error
	)

	server, err := e2etest.StartServer(t.Context(), testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	t.Cleanup(server.Shutdown)

	client := server.Client()

	// First register to get authenticated
	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Failed to register: %v", err)
	}

	// Verify the user can log in because we check that it fails after deleting the user
	_, err = client.Logout(ctx)
	if err != nil {
		t.Fatalf("Failed to logout: %v", err)
	}
	_, err = client.Login(ctx)
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}

	// Navigate to preferences page
	if doc, err = client.GetDoc(ctx, "/preferences"); err != nil {
		t.Fatalf("Failed to get preferences: %v", err)
	}

	// Verify we can see the danger zone
	dangerZone := doc.Find(".danger-zone")
	if dangerZone.Length() == 0 {
		t.Fatal("Expected to find danger zone section")
	}

	// Verify danger zone contains proper warning text
	if dangerZone.Text() == "" ||
		doc.Find(".danger-zone h2:contains('Danger Zone')").Length() == 0 ||
		doc.Find(".danger-zone p:contains('Permanently delete')").Length() == 0 {
		t.Error("Expected to find proper danger zone warning text")
	}

	// Find the delete user form
	deleteForm := doc.Find("form[action='/preferences/delete-user']")
	if deleteForm.Length() == 0 {
		t.Fatal("Expected to find delete user form")
	}

	// Verify the delete button exists
	deleteButton := deleteForm.Find("button:contains('Delete my data')")
	if deleteButton.Length() == 0 {
		t.Fatal("Expected to find delete user button")
	}

	// Submit the delete user form
	if doc, err = client.SubmitForm(ctx, doc, "/preferences/delete-user", nil); err != nil {
		t.Fatalf("Failed to submit delete user form: %v", err)
	}

	// After deletion, user should be redirected to home page and logged out
	if doc.Url.Path != "/" {
		t.Errorf("Expected to be redirected to home page after deletion, got %q", doc.Url.Path)
	}

	// Trying to access preferences should now redirect to home (user not authenticated)
	var resp *http.Response
	resp, err = client.Get(ctx, "/preferences")
	if err != nil {
		t.Fatalf("Failed to get preferences after deletion: %v", err)
	}
	if got, want := resp.Request.URL.Path, "/"; got != want {
		t.Errorf("Expected redirect to %q after user deletion, got %q", want, got)
	}

	// Verify the user can't login with old credentials (user was deleted)
	_, err = client.Login(ctx)
	if err == nil {
		t.Fatal("Expected error when trying to login with deleted user credentials, got none")
	}
}

func verifySelected(t *testing.T, doc *goquery.Document, weekdays map[string]int) {
	t.Helper()
	form, err := e2etest.FindForm(doc, "/preferences")
	if err != nil {
		t.Fatalf("Failed to find form: %v", err)
	}

	for day, expectedMinutes := range weekdays {
		selectName := day + "_minutes"
		selectElement := form.Find("select[name='" + selectName + "']")
		if selectElement.Length() == 0 {
			t.Fatalf("Failed to find select element for %s", selectName)
		}

		selectedOption := selectElement.Find("option[selected]")
		if selectedOption.Length() == 0 {
			// If no option is explicitly selected, find the first option (default)
			selectedOption = selectElement.Find("option").First()
		}

		selectedValue := selectedOption.AttrOr("value", "")
		if selectedValue == "" {
			t.Fatalf("No selected value found for %s", selectName)
		}

		actualMinutes := 0
		if selectedValue != "0" {
			actualMinutes, err = strconv.Atoi(selectedValue)
			if err != nil {
				t.Fatalf("Failed to parse selected value %s for %s: %v", selectedValue, selectName, err)
			}
		}

		if actualMinutes != expectedMinutes {
			t.Errorf("Expected %s to have %d minutes selected, got %d", day, expectedMinutes, actualMinutes)
		}
	}
}
