//nolint:testpackage // these tests poke at unexported helpers; live inside the package by design.
package loadtest

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// httptestServerReturning returns a test server whose handler always replies with the given status.
func httptestServerReturning(t *testing.T, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	}))
}

func TestRecorder_Snapshot_aggregatesPerNormalizedRoute(t *testing.T) {
	rec := NewRecorder()
	// Three GETs to two different dates should collapse to one /workouts/{date} bucket.
	rec.Record(http.MethodGet, "/workouts/2026-05-18", http.StatusOK, 10*time.Millisecond)
	rec.Record(http.MethodGet, "/workouts/2026-05-19", http.StatusOK, 20*time.Millisecond)
	rec.Record(http.MethodGet, "/workouts/2026-05-18", http.StatusOK, 30*time.Millisecond)
	// A POST and a different path live in their own buckets.
	rec.Record(http.MethodPost, "/preferences", http.StatusSeeOther, 5*time.Millisecond)
	rec.Record(http.MethodGet, "/workouts/2026-05-19/exercises/42", http.StatusOK, 7*time.Millisecond)

	snap := rec.Snapshot()

	getWorkouts := snap.find(http.MethodGet, "/workouts/{date}")
	if getWorkouts == nil {
		t.Fatalf("expected bucket GET /workouts/{date}, got routes %v", routeKeys(snap))
	}
	if getWorkouts.Count != 3 {
		t.Errorf("workouts count: got %d, want 3", getWorkouts.Count)
	}
	if getWorkouts.P50 < 19*time.Millisecond || getWorkouts.P50 > 21*time.Millisecond {
		t.Errorf("workouts p50: got %v, want ~20ms", getWorkouts.P50)
	}

	getExercise := snap.find(http.MethodGet, "/workouts/{date}/exercises/{id}")
	if getExercise == nil {
		t.Fatalf("expected bucket GET /workouts/{date}/exercises/{id}, got routes %v", routeKeys(snap))
	}

	postPrefs := snap.find(http.MethodPost, "/preferences")
	if postPrefs == nil || postPrefs.Count != 1 {
		t.Errorf("expected one POST /preferences sample, got %+v", postPrefs)
	}
}

func TestRecorder_Snapshot_countsErrorsByClass(t *testing.T) {
	rec := NewRecorder()
	rec.Record(http.MethodGet, "/x", http.StatusOK, time.Millisecond)
	rec.Record(http.MethodGet, "/x", http.StatusBadRequest, time.Millisecond)
	rec.Record(http.MethodGet, "/x", http.StatusInternalServerError, time.Millisecond)
	rec.Record(http.MethodGet, "/x", http.StatusServiceUnavailable, time.Millisecond)

	got := rec.Snapshot().find(http.MethodGet, "/x")
	if got == nil {
		t.Fatalf("missing bucket")
	}
	if got.Count != 4 {
		t.Errorf("count: got %d, want 4", got.Count)
	}
	if got.Status4xx != 1 {
		t.Errorf("4xx: got %d, want 1", got.Status4xx)
	}
	if got.Status5xx != 2 {
		t.Errorf("5xx: got %d, want 2", got.Status5xx)
	}
}

func TestNormalizePath(t *testing.T) {
	cases := map[string]string{
		"/":                                 "/",
		"/preferences":                      "/preferences",
		"/workouts/2026-05-18":              "/workouts/{date}",
		"/workouts/2026-05-18/start":        "/workouts/{date}/start",
		"/workouts/2026-05-18/exercises/42": "/workouts/{date}/exercises/{id}",
		"/workouts/2026-05-18/exercises/42/sets/0":        "/workouts/{date}/exercises/{id}/sets/{id}",
		"/workouts/2026-05-18/exercises/42/sets/0/update": "/workouts/{date}/exercises/{id}/sets/{id}/update",
		"/api/healthy": "/api/healthy",
	}
	for in, want := range cases {
		if got := normalizePath(in); got != want {
			t.Errorf("normalizePath(%q) = %q, want %q", in, got, want)
		}
	}
}

// routeKeys returns the list of bucket keys in a snapshot, for error messages.
func routeKeys(s *Snapshot) []string {
	out := make([]string, 0, len(s.Routes))
	for _, r := range s.Routes {
		out = append(out, r.Method+" "+r.Path)
	}
	return out
}

func TestRecordingTransport_records(t *testing.T) {
	srv := httptestServerReturning(t, http.StatusOK)
	defer srv.Close()

	rec := NewRecorder()
	client := &http.Client{Transport: NewRecordingTransport(nil, rec)}
	resp, err := client.Get(srv.URL + "/workouts/2026-05-18/exercises/9")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	resp.Body.Close()

	got := rec.Snapshot().find(http.MethodGet, "/workouts/{date}/exercises/{id}")
	if got == nil {
		t.Fatalf("missing recorded bucket")
	}
	if got.Count != 1 {
		t.Errorf("count: got %d, want 1", got.Count)
	}
}
