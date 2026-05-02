package main

import (
	"net/http"
	"strings"
	"testing"

	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

func Test_fileServer_servesExistingFile(t *testing.T) {
	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	resp, err := server.Client().Get(t.Context(), "/main.css")
	if err != nil {
		t.Fatalf("Failed to GET /main.css: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 for existing static file, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/css") {
		t.Errorf("Expected text/css Content-Type, got %q", ct)
	}
}

func Test_fileServer_missingFileReturnsCustom404(t *testing.T) {
	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	resp, err := server.Client().Get(t.Context(), "/this-file-does-not-exist.css")
	if err != nil {
		t.Fatalf("Failed to GET nonexistent static file: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected 404 for missing static file, got %d", resp.StatusCode)
	}
	body := make([]byte, 4096)
	n, _ := resp.Body.Read(body)
	bodyStr := string(body[:n])
	if strings.Contains(bodyStr, "404 page not found") && !strings.Contains(bodyStr, "Page Not Found") {
		t.Errorf("Expected custom 404 page (containing 'Page Not Found'), got default Go file-server body")
	}
	if !strings.Contains(bodyStr, "Page Not Found") {
		t.Errorf("Expected custom 404 body to contain 'Page Not Found', got: %s", bodyStr)
	}
}

func Test_fileServer_directoryTraversalReturns404(t *testing.T) {
	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	resp, err := server.Client().Get(t.Context(), "/../../../etc/passwd")
	if err != nil {
		t.Fatalf("Failed to GET traversal path: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected 404 or 400 for directory traversal, got %d", resp.StatusCode)
	}
}
