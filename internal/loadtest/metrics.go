package loadtest

import (
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// Recorder collects per-route latency samples and error counts. Safe for
// concurrent use by many request goroutines.
type Recorder struct {
	mu      sync.Mutex
	buckets map[routeKey]*routeBucket
}

// routeKey identifies a (method, normalized-path) tuple.
type routeKey struct {
	Method string
	Path   string
}

// routeBucket accumulates samples for a single route. Latencies are stored as
// nanoseconds so percentile sorting stays allocation-light.
type routeBucket struct {
	count     int
	errors4xx int
	errors5xx int
	latencyNs []int64
}

// NewRecorder returns a fresh Recorder.
func NewRecorder() *Recorder {
	return &Recorder{
		mu:      sync.Mutex{},
		buckets: make(map[routeKey]*routeBucket),
	}
}

// Record adds one sample to the appropriate bucket. urlPath is normalized so
// dates and numeric IDs collapse into placeholders (see normalizePath).
func (r *Recorder) Record(method, urlPath string, status int, latency time.Duration) {
	key := routeKey{Method: method, Path: normalizePath(urlPath)}
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.buckets[key]
	if !ok {
		b = &routeBucket{count: 0, errors4xx: 0, errors5xx: 0, latencyNs: nil}
		r.buckets[key] = b
	}
	b.count++
	switch {
	case status >= statusServer && status < statusUnknown:
		b.errors5xx++
	case status >= statusClient && status < statusServer:
		b.errors4xx++
	}
	b.latencyNs = append(b.latencyNs, latency.Nanoseconds())
}

// RouteStats is the report shape for one route bucket.
type RouteStats struct {
	Method    string        `json:"method"`
	Path      string        `json:"path"`
	Count     int           `json:"count"`
	Status4xx int           `json:"status_4xx"`
	Status5xx int           `json:"status_5xx"`
	P50       time.Duration `json:"p50"`
	P95       time.Duration `json:"p95"`
	P99       time.Duration `json:"p99"`
	Max       time.Duration `json:"max"`
}

// Snapshot is the report bundle for a single load run.
type Snapshot struct {
	Routes []RouteStats `json:"routes"`
}

// find returns the matching route stats or nil.
func (s *Snapshot) find(method, path string) *RouteStats {
	for i := range s.Routes {
		if s.Routes[i].Method == method && s.Routes[i].Path == path {
			return &s.Routes[i]
		}
	}
	return nil
}

// Snapshot computes percentiles for every route bucket and returns a stable,
// sorted snapshot. Cheap copy semantics — callers may keep the result.
func (r *Recorder) Snapshot() *Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := &Snapshot{Routes: make([]RouteStats, 0, len(r.buckets))}
	for key, b := range r.buckets {
		latencies := append([]int64(nil), b.latencyNs...)
		slices.Sort(latencies)
		out.Routes = append(out.Routes, RouteStats{
			Method:    key.Method,
			Path:      key.Path,
			Count:     b.count,
			Status4xx: b.errors4xx,
			Status5xx: b.errors5xx,
			P50:       percentile(latencies, percentile50),
			P95:       percentile(latencies, percentile95),
			P99:       percentile(latencies, percentile99),
			Max:       maxLatency(latencies),
		})
	}
	sort.Slice(out.Routes, func(i, j int) bool {
		if out.Routes[i].Method != out.Routes[j].Method {
			return out.Routes[i].Method < out.Routes[j].Method
		}
		return out.Routes[i].Path < out.Routes[j].Path
	})
	return out
}

const (
	percentile50  = 50
	percentile95  = 95
	percentile99  = 99
	percentMax    = 100
	statusClient  = 400
	statusServer  = 500
	statusUnknown = 600
)

// percentile returns the p-th percentile via the nearest-rank method on a
// pre-sorted slice. Returns 0 for empty input.
func percentile(sorted []int64, p int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	rank := (p * len(sorted)) / percentMax
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return time.Duration(sorted[rank])
}

func maxLatency(sorted []int64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	return time.Duration(sorted[len(sorted)-1])
}

// pathSegmentReplacement maps a per-segment regex to a placeholder.
type pathSegmentReplacement struct {
	pattern     *regexp.Regexp
	placeholder string
}

// pathSegmentReplacementList is the rewrite list used by normalizePath. It is
// immutable after init — the regex objects themselves are read-only — so a
// package-level value avoids re-compiling on every call without introducing
// mutable shared state.
//
//nolint:gochecknoglobals // see comment above; these are effectively const.
var pathSegmentReplacementList = []pathSegmentReplacement{
	{pattern: regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`), placeholder: "{date}"},
	{pattern: regexp.MustCompile(`^\d+$`), placeholder: "{id}"},
}

// normalizePath collapses request paths so per-resource variants share a
// bucket: dates become {date}, numeric IDs become {id}. Static segments and
// the leading / are preserved verbatim. Each / -split segment is rewritten
// independently so /workouts/{date}/start keeps its /start tail.
func normalizePath(p string) string {
	if p == "" || p == "/" {
		return p
	}
	segments := strings.Split(p, "/")
	for i, seg := range segments {
		for _, repl := range pathSegmentReplacementList {
			if repl.pattern.MatchString(seg) {
				segments[i] = repl.placeholder
				break
			}
		}
	}
	out := strings.Join(segments, "/")
	if !strings.HasPrefix(out, "/") {
		out = "/" + out
	}
	return out
}

// recordingTransport wraps an http.RoundTripper and records every request's
// method, normalized path, status code, and wall-clock latency into a Recorder.
type recordingTransport struct {
	base http.RoundTripper
	rec  *Recorder
}

// NewRecordingTransport returns a RoundTripper that delegates to base and
// records each request into rec. Errors are recorded as status 0 with the
// observed latency.
func NewRecordingTransport(base http.RoundTripper, rec *Recorder) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &recordingTransport{base: base, rec: rec}
}

// RoundTrip implements http.RoundTripper.
func (t *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := t.base.RoundTrip(req)
	latency := time.Since(start)
	status := 0
	if resp != nil {
		status = resp.StatusCode
	}
	t.rec.Record(req.Method, req.URL.Path, status, latency)
	return resp, err //nolint:wrapcheck // pass-through transport, matches secFetchSiteTransport.
}
