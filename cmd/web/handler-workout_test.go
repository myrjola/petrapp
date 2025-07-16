package main

import (
	"github.com/PuerkitoBio/goquery"
	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/testhelpers"
	"testing"
	"time"
)

func Test_application_addWorkout(t *testing.T) {
	var (
		ctx = t.Context()
		doc *goquery.Document
		err error
	)

	server, err := e2etest.StartServer(ctx, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	client := server.Client()

	// Register a user
	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Failed to register: %v", err)
	}

	// Set workout preferences (enable Monday workouts)
	formData := map[string]string{
		"monday": "true",
	}
	if doc, err = client.GetDoc(ctx, "/preferences"); err != nil {
		t.Fatalf("Failed to get preferences: %v", err)
	}
	if doc, err = client.SubmitForm(ctx, doc, "/preferences", formData); err != nil {
		t.Fatalf("Failed to submit preferences form: %v", err)
	}

	// Start a workout for today
	today := time.Now().Format("2006-01-02")
	if doc, err = client.SubmitForm(ctx, doc, "/workouts/"+today+"/start", nil); err != nil {
		t.Fatalf("Failed to submit start workout form: %v", err)
	}

	// Verify we're on the workout page
	if doc.Find("a.exercise").Length() == 0 {
		t.Error("Expected to find exercises on the workout page")
	}

	// Count initial exercises
	initialExerciseCount := doc.Find("a.exercise").Length()

	// Navigate to add exercise page
	if doc, err = client.GetDoc(ctx, "/workouts/"+today+"/add-exercise"); err != nil {
		t.Fatalf("Failed to get add exercise page: %v", err)
	}

	// Verify we're on the add exercise page
	if doc.Find("h1:contains('Add Exercise')").Length() == 0 {
		t.Error("Expected to find 'Add Exercise' heading")
	}

	if doc, err = client.SubmitForm(ctx, doc, "/workouts/"+today+"/add-exercise", nil); err != nil {
		t.Fatalf("Failed to submit add exercise form: %v", err)
	}

	// Verify we're back on the workout page
	if doc.Find("a.exercise").Length() <= initialExerciseCount {
		t.Errorf("Expected more exercises after adding one, got %d (was %d)",
			doc.Find("a.exercise").Length(), initialExerciseCount)
	}

	// Verify the exercise was added (by checking the count increased)
	newExerciseCount := doc.Find("a.exercise").Length()
	if newExerciseCount != initialExerciseCount+1 {
		t.Errorf("Expected exercise count to increase by 1, got %d (was %d)",
			newExerciseCount, initialExerciseCount)
	}
}
