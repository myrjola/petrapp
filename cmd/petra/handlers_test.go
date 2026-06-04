package main

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// Test_pageTemplate_cachesAndReturnsSameInstanceInProdMode verifies that
// successive calls in production mode return the same cached
// *template.Template pointer (no per-render clone) and that concurrent
// access does not race.
func Test_pageTemplate_cachesAndReturnsSameInstanceInProdMode(t *testing.T) {
	t.Parallel()

	templatePath, err := filepath.Abs(filepath.Join("ui", "templates"))
	if err != nil {
		t.Fatalf("resolve template path: %v", err)
	}
	app := &application{ //nolint:exhaustruct // only the fields touched by pageTemplate matter here.
		templateFS:      os.DirFS(templatePath),
		parsedTemplates: newTemplateCache(),
		devMode:         false,
	}

	first, err := app.pageTemplate("home")
	if err != nil {
		t.Fatalf("first pageTemplate: %v", err)
	}
	second, err := app.pageTemplate("home")
	if err != nil {
		t.Fatalf("second pageTemplate: %v", err)
	}
	if first != second {
		t.Errorf("expected pageTemplate to return the same cached pointer; got distinct instances")
	}
	if cached := app.parsedTemplates.get("home"); cached != first {
		t.Errorf("expected cache to retain the parsed template for 'home'")
	}

	// Concurrent access must not race.
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, gerr := app.pageTemplate("home"); gerr != nil {
				t.Errorf("concurrent pageTemplate: %v", gerr)
			}
		})
	}
	wg.Wait()
}

// Test_pageTemplate_coldStartRaceFreeInProdMode hammers a fresh cache from
// multiple goroutines so that the get-miss / parse / set sequence runs
// concurrently. Detects races in the cache-populate path; the resulting
// cached pointer is whichever parse won the set-race, and every subsequent
// caller reads that same pointer.
func Test_pageTemplate_coldStartRaceFreeInProdMode(t *testing.T) {
	t.Parallel()

	templatePath, err := filepath.Abs(filepath.Join("ui", "templates"))
	if err != nil {
		t.Fatalf("resolve template path: %v", err)
	}
	app := &application{ //nolint:exhaustruct // only the fields touched by pageTemplate matter here.
		templateFS:      os.DirFS(templatePath),
		parsedTemplates: newTemplateCache(),
		devMode:         false,
	}

	const goroutines = 16
	results := make([]*template.Template, goroutines)
	errs := make([]error, goroutines)
	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Go(func() {
			results[i], errs[i] = app.pageTemplate("home")
		})
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
	}
	cached := app.parsedTemplates.get("home")
	if cached == nil {
		t.Fatal("expected cached template to be populated after concurrent first hits")
	}
	// The cached pointer must equal at least one of the parsed results
	// (whichever parse won the set race).
	matches := 0
	for _, got := range results {
		if got == cached {
			matches++
		}
	}
	if matches == 0 {
		t.Errorf("no goroutine observed the winning cached template")
	}
}

// Test_pageTemplate_skipsCacheInDevMode verifies that dev mode re-parses on
// every call so template edits are reflected on the next refresh.
func Test_pageTemplate_skipsCacheInDevMode(t *testing.T) {
	t.Parallel()

	templatePath, err := filepath.Abs(filepath.Join("ui", "templates"))
	if err != nil {
		t.Fatalf("resolve template path: %v", err)
	}
	app := &application{ //nolint:exhaustruct // only the fields touched by pageTemplate matter here.
		templateFS:      os.DirFS(templatePath),
		parsedTemplates: newTemplateCache(),
		devMode:         true,
	}
	if _, err = app.pageTemplate("home"); err != nil {
		t.Fatalf("pageTemplate: %v", err)
	}
	if cached := app.parsedTemplates.get("home"); cached != nil {
		t.Errorf("dev mode should not populate the template cache")
	}
}

func Test_formatFloat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input float64
		want  string
	}{
		{62.54, "62.5"},              // weight progression artifact (0.85 factor)
		{60.900000000000006, "60.9"}, // original floating-point artifact
		{75.0, "75"},                 // whole number, no trailing zero
		{0.0, "0"},                   // zero
		{100.0, "100"},               // larger whole number
		{62.5, "62.5"},               // already at one decimal place
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%v", tt.input), func(t *testing.T) {
			t.Parallel()

			if got := formatFloat(tt.input); got != tt.want {
				t.Errorf("formatFloat(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
