package main

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// hash8 returns the first assetHashLen hex chars of the SHA-256 of data,
// mirroring assetManifest.register so tests can predict fingerprinted URLs.
func hash8(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:assetHashLen]
}

func mapFile(data string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte(data)} //nolint:exhaustruct // tests only need Data.
}

func testStaticFS() fstest.MapFS {
	return fstest.MapFS{
		"main.css":      mapFile("body{color:red}"),
		"logo.svg":      mapFile("<svg/>"),
		"manifest.json": mapFile(`{"icons":[{"src":"/logo.svg"}]}`),
	}
}

func Test_assetManifest_URL_hashesKnownAndPassesUnknown(t *testing.T) {
	t.Parallel()
	fsys := testStaticFS()
	m, err := buildAssetManifest(fsys)
	if err != nil {
		t.Fatalf("buildAssetManifest: %v", err)
	}

	wantCSS := "/main." + hash8(fsys["main.css"].Data) + ".css"
	if got := m.URL("/main.css"); got != wantCSS {
		t.Errorf("URL(/main.css) = %q, want %q", got, wantCSS)
	}
	if got := m.URL("/does-not-exist.css"); got != "/does-not-exist.css" {
		t.Errorf("unknown asset should pass through, got %q", got)
	}
}

func Test_assetManifest_nilSafe(t *testing.T) {
	t.Parallel()
	var m *assetManifest
	if got := m.URL("/main.css"); got != "/main.css" {
		t.Errorf("nil URL = %q, want passthrough", got)
	}
	if realPath, exact := m.resolve("/main.css"); realPath != "/main.css" || exact {
		t.Errorf("nil resolve = (%q, %v), want (/main.css, false)", realPath, exact)
	}
}

func Test_assetManifest_resolve(t *testing.T) {
	t.Parallel()
	fsys := testStaticFS()
	m, err := buildAssetManifest(fsys)
	if err != nil {
		t.Fatalf("buildAssetManifest: %v", err)
	}
	hashedCSS := m.URL("/main.css")

	tests := []struct {
		name     string
		reqPath  string
		wantReal string
		wantExct bool
	}{
		{"current hash is exact", hashedCSS, "/main.css", true},
		{"plain path not exact", "/main.css", "/main.css", false},
		{"stale hash strips to real, not exact", "/main.0123abcd.css", "/main.css", false},
		{"non-fingerprinted name passes through", "/logo.svg", "/logo.svg", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			realPath, exact := m.resolve(tt.reqPath)
			if realPath != tt.wantReal || exact != tt.wantExct {
				t.Errorf("resolve(%q) = (%q, %v), want (%q, %v)",
					tt.reqPath, realPath, exact, tt.wantReal, tt.wantExct)
			}
		})
	}
}

func Test_buildAssetManifest_rewritesManifestIcons(t *testing.T) {
	t.Parallel()
	fsys := testStaticFS()
	m, err := buildAssetManifest(fsys)
	if err != nil {
		t.Fatalf("buildAssetManifest: %v", err)
	}

	pa, ok := m.processedAssetFor("/manifest.json")
	if !ok {
		t.Fatal("manifest.json should be a processed asset")
	}
	wantLogo := "/logo." + hash8(fsys["logo.svg"].Data) + ".svg"
	if !strings.Contains(string(pa.body), wantLogo) {
		t.Errorf("processed manifest %q should contain hashed icon %q", pa.body, wantLogo)
	}
	if strings.Contains(string(pa.body), `"/logo.svg"`) {
		t.Errorf("processed manifest still contains plain icon src: %s", pa.body)
	}
	if pa.contentType != "application/manifest+json" {
		t.Errorf("manifest content-type = %q", pa.contentType)
	}

	// The manifest's own fingerprint must reflect the rewritten bytes.
	wantManifestURL := "/manifest." + hash8(pa.body) + ".json"
	if got := m.URL("/manifest.json"); got != wantManifestURL {
		t.Errorf("URL(/manifest.json) = %q, want %q", got, wantManifestURL)
	}
}

func Test_stripAssetHash(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in, want string
	}{
		{"/main.0123abcd.css", "/main.css"},
		{"/main.css", "/main.css"},                   // no hash
		{"/logo-maskable.svg", "/logo-maskable.svg"}, // "maskable" is not 8 hex
		{"/main.0123abcz.css", "/main.0123abcz.css"}, // not hex
		{"/main.0123abc.css", "/main.0123abc.css"},   // 7 chars, wrong length
		{"/robots.txt", "/robots.txt"},               // no hash
	}
	for _, tt := range tests {
		if got := stripAssetHash(tt.in); got != tt.want {
			t.Errorf("stripAssetHash(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// staticTestApp builds the minimal *application and handler needed to exercise
// staticAssetHandler without the session-bearing middleware stacks.
func staticTestApp(t *testing.T, devMode bool) (*application, http.HandlerFunc, *bool) {
	t.Helper()
	fsys := testStaticFS()
	m, err := buildAssetManifest(fsys)
	if err != nil {
		t.Fatalf("buildAssetManifest: %v", err)
	}
	app := &application{ //nolint:exhaustruct // only fields touched by staticAssetHandler matter.
		logger:  slog.New(slog.DiscardHandler),
		devMode: devMode,
		assets:  m,
	}
	notFoundCalled := false
	notFound := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		notFoundCalled = true
		w.WriteHeader(http.StatusNotFound)
	})
	return app, app.staticAssetHandler(http.FileServerFS(fsys), notFound), &notFoundCalled
}

func Test_staticAssetHandler_cacheHeaders(t *testing.T) {
	t.Parallel()
	app, handler, _ := staticTestApp(t, false)
	hashedCSS := app.assets.URL("/main.css")

	tests := []struct {
		name      string
		path      string
		wantCache string
		wantBody  string
	}{
		{"exact hash is immutable", hashedCSS,
			"public, max-age=31536000, immutable", "body{color:red}"},
		{"plain path revalidates", "/main.css",
			"public, max-age=0, must-revalidate", "body{color:red}"},
		{"stale hash serves current, revalidates", "/main.0123abcd.css",
			"public, max-age=0, must-revalidate", "body{color:red}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			handler(w, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", w.Code)
			}
			if got := w.Header().Get("Cache-Control"); got != tt.wantCache {
				t.Errorf("Cache-Control = %q, want %q", got, tt.wantCache)
			}
			if got := w.Body.String(); got != tt.wantBody {
				t.Errorf("body = %q, want %q", got, tt.wantBody)
			}
		})
	}
}

func Test_staticAssetHandler_devNoStoreEvenWhenHashed(t *testing.T) {
	t.Parallel()
	app, handler, _ := staticTestApp(t, true)
	hashedCSS := app.assets.URL("/main.css")

	w := httptest.NewRecorder()
	handler(w, httptest.NewRequest(http.MethodGet, hashedCSS, nil))
	if got := w.Header().Get("Cache-Control"); got != "no-store, max-age=0, must-revalidate" {
		t.Errorf("dev Cache-Control = %q, want no-store", got)
	}
}

func Test_staticAssetHandler_servesProcessedManifest(t *testing.T) {
	t.Parallel()
	app, handler, _ := staticTestApp(t, false)

	for _, path := range []string{"/manifest.json", app.assets.URL("/manifest.json")} {
		w := httptest.NewRecorder()
		handler(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d", path, w.Code)
		}
		if ct := w.Header().Get("Content-Type"); ct != "application/manifest+json" {
			t.Errorf("GET %s Content-Type = %q", path, ct)
		}
		if !strings.Contains(w.Body.String(), "/logo.") || strings.Contains(w.Body.String(), `"/logo.svg"`) {
			t.Errorf("GET %s body not rewritten: %s", path, w.Body.String())
		}
	}
}

func Test_staticAssetHandler_missingFileFallsThroughTo404(t *testing.T) {
	t.Parallel()
	_, handler, notFoundCalled := staticTestApp(t, false)

	w := httptest.NewRecorder()
	handler(w, httptest.NewRequest(http.MethodGet, "/does-not-exist.css", nil))
	if !*notFoundCalled {
		t.Error("missing file should invoke the notFound handler")
	}
	if got := w.Header().Get("Cache-Control"); got != "" {
		t.Errorf("404 should not carry static Cache-Control, got %q", got)
	}
}
