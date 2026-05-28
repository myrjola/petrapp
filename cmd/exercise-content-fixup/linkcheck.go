package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const headCheckTimeout = 5 * time.Second

// CheckURLs HEAD-requests every URL with a 5s per-request timeout and
// returns a map from URL to alive-status (true iff the final response is
// 2xx or 3xx). Placeholder URLs are skipped — they are always treated as
// dead.
//
// Network failures, timeouts, and 4xx/5xx responses all map to false.
// Progress is printed to stdout so a human watching the run can see what's
// happening on a list of ~150 URLs.
func CheckURLs(ctx context.Context, urls []string) map[string]bool {
	client := &http.Client{Timeout: headCheckTimeout}
	results := make(map[string]bool, len(urls))

	for _, u := range urls {
		if _, seen := results[u]; seen {
			continue
		}
		if strings.HasPrefix(u, placeholderURLPrefix) {
			results[u] = false
			fmt.Printf("  [dead]  %s  (placeholder)\n", u) //nolint:forbidigo // human-facing progress output.
			continue
		}
		alive, reason := headCheck(ctx, client, u)
		results[u] = alive
		if alive {
			fmt.Printf("  [live]  %s\n", u) //nolint:forbidigo // human-facing progress output.
		} else {
			fmt.Printf("  [dead]  %s  (%s)\n", u, reason) //nolint:forbidigo // human-facing progress output.
		}
	}
	return results
}

func headCheck(ctx context.Context, client *http.Client, url string) (bool, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return false, fmt.Sprintf("bad url: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, err.Error()
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return false, fmt.Sprintf("status %d", resp.StatusCode)
	}
	return true, ""
}
