package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// Test_pageTemplate_cachesAndReturnsCloneInProdMode verifies that successive
// calls in production mode return distinct *template.Template instances (so
// per-request Funcs() overrides don't bleed across goroutines) while reusing
// the cached parsed tree (the cache's whole point).
func Test_pageTemplate_cachesAndReturnsCloneInProdMode(t *testing.T) {
	templatePath, err := filepath.Abs(filepath.Join("..", "..", "ui", "templates"))
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
	if first == second {
		t.Errorf("expected pageTemplate to return clones (distinct pointers), got the same instance")
	}
	if cached := app.parsedTemplates.get("home"); cached == nil {
		t.Errorf("expected cache to retain a parsed template for 'home'")
	}

	// Concurrent access must not race or duplicate work observably.
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

// Test_pageTemplate_skipsCacheInDevMode verifies that dev mode re-parses on
// every call so template edits are reflected on the next refresh.
func Test_pageTemplate_skipsCacheInDevMode(t *testing.T) {
	templatePath, err := filepath.Abs(filepath.Join("..", "..", "ui", "templates"))
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
			if got := formatFloat(tt.input); got != tt.want {
				t.Errorf("formatFloat(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
