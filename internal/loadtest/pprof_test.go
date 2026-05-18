//nolint:testpackage // exercises unexported helpers; see metrics_test.go.
package loadtest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCapturePprofProfile_writesSeekableFile(t *testing.T) {
	body := []byte("fake pprof body")
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/profile", func(w http.ResponseWriter, r *http.Request) {
		// stresstest must pass seconds= so the remote knows how long to sample.
		if r.URL.Query().Get("seconds") == "" {
			t.Errorf("missing seconds query param on profile request")
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(body)
	})
	mux.HandleFunc("/debug/pprof/heap", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("heap body"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	ctx := t.Context()

	cpuPath, err := CapturePprofProfile(ctx, srv.URL, dir, 1*time.Second)
	if err != nil {
		t.Fatalf("CapturePprofProfile: %v", err)
	}
	if !strings.HasPrefix(filepath.Base(cpuPath), "cpu-") || !strings.HasSuffix(cpuPath, ".pb.gz") {
		t.Errorf("cpu profile filename shape unexpected: %s", cpuPath)
	}
	data, err := os.ReadFile(cpuPath)
	if err != nil {
		t.Fatalf("read cpu profile: %v", err)
	}
	if string(data) != string(body) {
		t.Errorf("cpu profile body: got %q, want %q", data, body)
	}

	heapPath, err := CapturePprofHeap(ctx, srv.URL, dir)
	if err != nil {
		t.Fatalf("CapturePprofHeap: %v", err)
	}
	if !strings.HasPrefix(filepath.Base(heapPath), "heap-") {
		t.Errorf("heap profile filename shape unexpected: %s", heapPath)
	}
}

func TestWriteJSONReport_roundTrips(t *testing.T) {
	dir := t.TempDir()
	rec := NewRecorder()
	rec.Record(http.MethodGet, "/x", http.StatusOK, 5*time.Millisecond)

	path, err := WriteJSONReport(dir, rec.Snapshot())
	if err != nil {
		t.Fatalf("WriteJSONReport: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var snap Snapshot
	if err = json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if got := snap.find(http.MethodGet, "/x"); got == nil || got.Count != 1 {
		t.Errorf("round-tripped snapshot missing GET /x")
	}
}

func TestCapturePprofProfile_returnsErrorOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	if _, err := CapturePprofProfile(ctx, srv.URL, dir, 100*time.Millisecond); err == nil {
		t.Fatalf("expected error on 502 from pprof endpoint")
	}
}
