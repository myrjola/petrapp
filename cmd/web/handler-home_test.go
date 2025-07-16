package main

import (
	"github.com/PuerkitoBio/goquery"
	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/testhelpers"
	"testing"
)

func testLookupEnv(key string) (string, bool) {
	switch key {
	case "PETRAPP_SQLITE_URL":
		return ":memory:", true
	case "PETRAPP_ADDR":
		return "localhost:0", true
	default:
		return "", false
	}
}

func Test_application_home(t *testing.T) {
	var (
		ctx = t.Context()
		doc *goquery.Document
	)
	server, err := e2etest.StartServer(t.Context(), testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	client := server.Client()

	t.Run("Initial state", func(t *testing.T) {
		doc, err = client.GetDoc(ctx, "/")
		if err != nil {
			t.Fatalf("Failed to get document: %v", err)
		}

		checkButtonPresence(t, doc, "Sign in", 1)
		checkButtonPresence(t, doc, "Register", 1)
	})

	t.Run("After registration", func(t *testing.T) {
		doc, err = client.Register(ctx)
		if err != nil {
			t.Fatalf("Failed to register: %v", err)
		}

		checkButtonPresence(t, doc, "Sign in", 0)
		checkButtonPresence(t, doc, "Register", 0)
	})

	t.Run("After logout", func(t *testing.T) {
		doc, err = client.Logout(ctx)
		if err != nil {
			t.Fatalf("Failed to logout: %v", err)
		}

		checkButtonPresence(t, doc, "Sign in", 1)
		checkButtonPresence(t, doc, "Register", 1)
	})

	t.Run("After login", func(t *testing.T) {
		doc, err = client.Login(ctx)
		if err != nil {
			t.Fatalf("Failed to login: %v", err)
		}

		checkButtonPresence(t, doc, "Sign in", 0)
		checkButtonPresence(t, doc, "Register", 0)
	})
}

func checkButtonPresence(t *testing.T, doc *goquery.Document, buttonText string, expectedCount int) {
	t.Helper()
	count := doc.Find("button:contains('" + buttonText + "')").Length()
	if count != expectedCount {
		t.Errorf("Expected %d '%s' button(s), but found %d", expectedCount, buttonText, count)
	}
}
