package main

import (
	"testing"

	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/platform/testkit"
)

// Regression test for the rest-timer hotfix: submitting the onboarding
// /schedule form must not flip rest_notifications_enabled back to false.
// schedulePOST had the same partial-literal bug as preferencesPOST and ran
// during onboarding before users could reach /preferences, so every new
// user lost the default-true silently.
func Test_application_schedulePOST_preservesRestNotificationsEnabled(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	server, err := e2etest.StartServer(t, testkit.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Failed to register: %v", err)
	}

	// Fresh users default to rest_notifications_enabled = true. Confirm the
	// checkbox is rendered checked so the test's premise is solid.
	doc, err := client.GetDoc(ctx, "/preferences")
	if err != nil {
		t.Fatalf("Failed to get preferences: %v", err)
	}
	checkbox := doc.Find("input[name='rest_notifications_enabled']")
	if checkbox.Length() == 0 {
		t.Fatal("Expected rest_notifications_enabled checkbox to be rendered")
	}
	if _, checked := checkbox.Attr("checked"); !checked {
		t.Fatal("Expected rest_notifications_enabled checkbox to start checked (default true)")
	}

	// Submit the onboarding schedule form with one valid workout day so it
	// passes IsEmpty() validation. Pre-fix this clobbered the column.
	scheduleDoc, err := client.GetDoc(ctx, "/schedule")
	if err != nil {
		t.Fatalf("Failed to get schedule: %v", err)
	}
	if _, err = client.SubmitForm(ctx, scheduleDoc, "/schedule", map[string]string{
		"Monday": "60",
	}); err != nil {
		t.Fatalf("Failed to submit schedule form: %v", err)
	}

	// Re-fetch /preferences and assert the checkbox is still checked.
	doc, err = client.GetDoc(ctx, "/preferences")
	if err != nil {
		t.Fatalf("Failed to re-get preferences: %v", err)
	}
	checkbox = doc.Find("input[name='rest_notifications_enabled']")
	if checkbox.Length() == 0 {
		t.Fatal("Expected rest_notifications_enabled checkbox to be rendered after submit")
	}
	if _, checked := checkbox.Attr("checked"); !checked {
		t.Error("rest_notifications_enabled was cleared by /schedule submit; should be preserved")
	}
}
