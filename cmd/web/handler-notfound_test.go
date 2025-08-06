package main

import (
	"github.com/PuerkitoBio/goquery"
	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/testhelpers"
	"net/http"
	"strings"
	"testing"
)

func Test_application_notFound(t *testing.T) {
	var (
		ctx = t.Context()
		err error
	)

	server, err := e2etest.StartServer(ctx, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	client := server.Client()

	t.Run("Invalid exercise ID returns custom 404", func(t *testing.T) {
		// Register a user first (required for mustSession routes)
		if _, err = client.Register(ctx); err != nil {
			t.Fatalf("Failed to register: %v", err)
		}

		// Test invalid exercise info route - should trigger our custom 404
		resp, err := client.Get(ctx, "/workouts/2024-01-01/exercises/invalid-id/info")
		if err != nil {
			t.Fatalf("Failed to get invalid exercise info: %v", err)
		}
		resp.Body.Close()

		// Verify we get a 404 status code
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status code %d for invalid exercise ID, got %d", http.StatusNotFound, resp.StatusCode)
		}

		// Get the document to check content (need to parse 404 responses manually)
		resp404, err := client.Get(ctx, "/workouts/2024-01-01/exercises/invalid-id/info")
		if err != nil {
			t.Fatalf("Failed to get 404 response for invalid exercise ID: %v", err)
		}
		defer resp404.Body.Close()

		if resp404.StatusCode != http.StatusNotFound {
			t.Errorf("Expected 404 status for invalid exercise ID, got %d", resp404.StatusCode)
		}

		doc, err := goquery.NewDocumentFromReader(resp404.Body)
		if err != nil {
			t.Fatalf("Failed to parse 404 document for invalid exercise ID: %v", err)
		}

		// Check for custom 404 content
		checkCustom404Content(t, doc, "invalid exercise ID")
	})

	t.Run("Invalid date returns custom 404", func(t *testing.T) {
		// Test invalid date format - should trigger our custom 404
		resp, err := client.Get(ctx, "/workouts/invalid-date/exercises/1/info")
		if err != nil {
			t.Fatalf("Failed to get invalid date exercise info: %v", err)
		}
		resp.Body.Close()

		// Verify we get a 404 status code
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status code %d for invalid date, got %d", http.StatusNotFound, resp.StatusCode)
		}

		// Get the document to check content (need to parse 404 responses manually)
		resp404, err := client.Get(ctx, "/workouts/invalid-date/exercises/1/info")
		if err != nil {
			t.Fatalf("Failed to get 404 response for invalid date: %v", err)
		}
		defer resp404.Body.Close()

		if resp404.StatusCode != http.StatusNotFound {
			t.Errorf("Expected 404 status for invalid date, got %d", resp404.StatusCode)
		}

		doc, err := goquery.NewDocumentFromReader(resp404.Body)
		if err != nil {
			t.Fatalf("Failed to parse 404 document for invalid date: %v", err)
		}

		// Check for custom 404 content
		checkCustom404Content(t, doc, "invalid date")
	})

	t.Run("Nonexistent path returns custom 404", func(t *testing.T) {
		// Test nonexistent path - should trigger our custom 404
		resp, err := client.Get(ctx, "/nonexistent")
		if err != nil {
			t.Fatalf("Failed to get nonexistent path: %v", err)
		}
		defer resp.Body.Close()

		// Verify we get a 404 status code
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status code %d for nonexistent path, got %d", http.StatusNotFound, resp.StatusCode)
		}

		// Get the document to check content (need to parse 404 responses manually)
		resp404, err := client.Get(ctx, "/nonexistent")
		if err != nil {
			t.Fatalf("Failed to get 404 response for nonexistent path: %v", err)
		}
		defer resp404.Body.Close()

		if resp404.StatusCode != http.StatusNotFound {
			t.Errorf("Expected 404 status for nonexistent path, got %d", resp404.StatusCode)
		}

		doc, err := goquery.NewDocumentFromReader(resp404.Body)
		if err != nil {
			t.Fatalf("Failed to parse 404 document for nonexistent path: %v", err)
		}

		// Check for custom 404 content
		checkCustom404Content(t, doc, "nonexistent path")
	})
}

func Test_application_notFound_template_content(t *testing.T) {
	var (
		ctx = t.Context()
		err error
		doc *goquery.Document
	)

	server, err := e2etest.StartServer(ctx, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	client := server.Client()

	// Register a user to access authenticated routes
	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Failed to register: %v", err)
	}

	// Get a 404 page to test
	resp404, err := client.Get(ctx, "/workouts/2024-01-01/exercises/nonexistent/info")
	if err != nil {
		t.Fatalf("Failed to get 404 response: %v", err)
	}
	defer resp404.Body.Close()

	if resp404.StatusCode != http.StatusNotFound {
		t.Fatalf("Expected 404 status, got %d", resp404.StatusCode)
	}

	doc, err = goquery.NewDocumentFromReader(resp404.Body)
	if err != nil {
		t.Fatalf("Failed to parse 404 document: %v", err)
	}

	t.Run("Contains 404 title", func(t *testing.T) {
		title := doc.Find("h1").First().Text()
		if !strings.Contains(title, "404") {
			t.Errorf("Expected 404 page to contain '404' in title, got: %s", title)
		}
	})

	t.Run("Contains 'Page Not Found' subtitle", func(t *testing.T) {
		subtitle := doc.Find("h2").First().Text()
		if !strings.Contains(subtitle, "Page Not Found") {
			t.Errorf("Expected 404 page to contain 'Page Not Found' subtitle, got: %s", subtitle)
		}
	})

	t.Run("Contains Go Home link", func(t *testing.T) {
		homeLinks := doc.Find("a[href='/']")
		if homeLinks.Length() == 0 {
			t.Error("Expected 404 page to contain a link to home page (/)")
		}

		homeText := homeLinks.First().Text()
		if !strings.Contains(homeText, "Go Home") {
			t.Errorf("Expected home link to contain 'Go Home', got: %s", homeText)
		}
	})

	t.Run("Contains Go Back button", func(t *testing.T) {
		backButtons := doc.Find("button:contains('Go Back')")
		if backButtons.Length() == 0 {
			t.Error("Expected 404 page to contain a 'Go Back' button")
		}
	})
}

// Helper function to check for custom 404 content
func checkCustom404Content(t *testing.T, doc *goquery.Document, context string) {
	t.Helper()

	// Check for 404 title
	title := doc.Find("h1").Text()
	if !strings.Contains(title, "404") {
		t.Errorf("Expected custom 404 page title for %s to contain '404', got: %s", context, title)
	}

	// Check for "Page Not Found" subtitle
	subtitle := doc.Find("h2").Text()
	if !strings.Contains(subtitle, "Page Not Found") {
		t.Errorf("Expected custom 404 page for %s to contain 'Page Not Found', got: %s", context, subtitle)
	}

	// Check for Go Home link
	homeLinks := doc.Find("a[href='/']")
	if homeLinks.Length() == 0 {
		t.Errorf("Expected custom 404 page for %s to contain home link", context)
	} else {
		homeText := homeLinks.First().Text()
		if !strings.Contains(homeText, "Go Home") {
			t.Errorf("Expected home link for %s to say 'Go Home', got: %s", context, homeText)
		}
	}

	// Check for Go Back button
	backButtons := doc.Find("button:contains('Go Back')")
	if backButtons.Length() == 0 {
		t.Errorf("Expected custom 404 page for %s to contain 'Go Back' button", context)
	}
}
