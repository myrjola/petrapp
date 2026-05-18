package loadtest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	pprofFilePerm           = 0o644
	pprofHTTPTimeoutPadding = 10 * time.Second
)

// CapturePprofProfile fetches a CPU profile from pprofBaseURL (e.g.
// "http://localhost:6060") for the given seconds and writes it to
// <outDir>/cpu-<timestamp>.pb.gz. Returns the saved file path.
//
// pprofBaseURL is expected to expose the standard net/http/pprof routes
// at /debug/pprof/. Reach Fly machines via `fly proxy --app <app> 6060:6060`
// before calling this.
func CapturePprofProfile(
	ctx context.Context, pprofBaseURL, outDir string, seconds time.Duration,
) (string, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("create pprof outDir: %w", err)
	}
	outPath := filepath.Join(outDir, fmt.Sprintf("cpu-%s.pb.gz", timestampNow()))
	secondsParam := max(int(seconds.Seconds()), 1)
	url := fmt.Sprintf("%s/debug/pprof/profile?seconds=%d", pprofBaseURL, secondsParam)
	if err := downloadTo(ctx, url, outPath, seconds+pprofHTTPTimeoutPadding); err != nil {
		return "", fmt.Errorf("capture cpu profile: %w", err)
	}
	return outPath, nil
}

// CapturePprofHeap fetches the heap profile from pprofBaseURL and writes it
// to <outDir>/heap-<timestamp>.pb.gz. Returns the saved file path.
func CapturePprofHeap(ctx context.Context, pprofBaseURL, outDir string) (string, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("create pprof outDir: %w", err)
	}
	outPath := filepath.Join(outDir, fmt.Sprintf("heap-%s.pb.gz", timestampNow()))
	url := pprofBaseURL + "/debug/pprof/heap"
	if err := downloadTo(ctx, url, outPath, pprofHTTPTimeoutPadding); err != nil {
		return "", fmt.Errorf("capture heap profile: %w", err)
	}
	return outPath, nil
}

// WriteJSONReport marshals snap to <outDir>/report-<timestamp>.json and
// returns the saved path. Used to bundle the latency snapshot alongside the
// pprof captures from the same run.
func WriteJSONReport(outDir string, snap *Snapshot) (string, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("create report outDir: %w", err)
	}
	outPath := filepath.Join(outDir, fmt.Sprintf("report-%s.json", timestampNow()))
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal snapshot: %w", err)
	}
	if err = os.WriteFile(outPath, data, pprofFilePerm); err != nil {
		return "", fmt.Errorf("write report: %w", err)
	}
	return outPath, nil
}

// downloadTo fetches url and streams it into outPath. Returns an error on
// non-2xx responses.
func downloadTo(ctx context.Context, url, outPath string, timeout time.Duration) error {
	httpCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(httpCtx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("get %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("pprof endpoint %s returned status %d", url, resp.StatusCode)
	}
	f, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, pprofFilePerm)
	if err != nil {
		return fmt.Errorf("open output: %w", err)
	}
	defer f.Close()
	if _, err = io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("write body: %w", err)
	}
	return nil
}

// timestampNow returns an ISO-8601 UTC timestamp suitable for filenames.
func timestampNow() string {
	return time.Now().UTC().Format("20060102T150405Z")
}
