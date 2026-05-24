// deployprobe polls a URL on a fixed cadence and classifies each response so
// you can quantify the user-visible impact of a deploy. Run it in one terminal,
// trigger the deploy in another, hit Ctrl+C when CD reports done, and read the
// summary: total requests, failure breakdown, and the longest contiguous
// failure window (the closest proxy for "downtime as seen from outside").
//
// Usage:
//
//	go run ./cmd/deployprobe https://petra-staging.fly.dev
//	go run ./cmd/deployprobe -interval=100ms -timeout=3s petra.fly.dev
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/myrjola/petrapp/internal/testhelpers"
)

type outcome string

const (
	outcomeOK      outcome = "ok"
	outcome5xx     outcome = "5xx"
	outcomeRefused outcome = "refused"
	outcomeTimeout outcome = "timeout"
	outcomeReset   outcome = "reset"
	outcomeDNS     outcome = "dns"
	outcomeOther   outcome = "other"
)

const (
	defaultInterval = 250 * time.Millisecond
	defaultTimeout  = 2 * time.Second
	usageExitCode   = 2
	percent         = 100
)

type result struct {
	at       time.Time
	out      outcome
	status   int
	duration time.Duration
	errStr   string
}

func classify(resp *http.Response, err error) outcome {
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) {
			return outcomeDNS
		}
		s := err.Error()
		switch {
		case strings.Contains(s, "connection refused"):
			return outcomeRefused
		case strings.Contains(s, "deadline exceeded"),
			strings.Contains(s, "Client.Timeout"),
			strings.Contains(s, "i/o timeout"):
			return outcomeTimeout
		case strings.Contains(s, "connection reset"),
			strings.Contains(s, "EOF"),
			strings.Contains(s, "broken pipe"):
			return outcomeReset
		default:
			return outcomeOther
		}
	}
	if resp.StatusCode >= http.StatusInternalServerError {
		return outcome5xx
	}
	return outcomeOK
}

func probe(ctx context.Context, client *http.Client, target string) result {
	start := time.Now()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	resp, err := client.Do(req)
	r := result{at: start, duration: time.Since(start)} //nolint:exhaustruct // out/status/errStr set below
	if resp != nil {
		_ = resp.Body.Close()
		r.status = resp.StatusCode
	}
	r.out = classify(resp, err)
	if err != nil {
		r.errStr = err.Error()
	}
	return r
}

func resolveTarget(raw, path string) (string, error) {
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err //nolint:wrapcheck // returned to main only.
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = path
	}
	return u.String(), nil
}

func main() {
	interval := flag.Duration("interval", defaultInterval, "polling interval")
	timeout := flag.Duration("timeout", defaultTimeout, "per-request timeout")
	path := flag.String("path", "/api/healthy", "URL path appended when target has no path")
	flag.Parse()

	logger := testhelpers.NewLogger(os.Stdout)
	ctx := context.Background()

	if flag.NArg() != 1 {
		logger.LogAttrs(ctx, slog.LevelError, "usage: deployprobe [flags] <hostname-or-url>")
		os.Exit(usageExitCode)
	}

	target, err := resolveTarget(flag.Arg(0), *path)
	if err != nil {
		logger.LogAttrs(ctx, slog.LevelError, "bad target", slog.Any("error", err))
		os.Exit(usageExitCode)
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// DisableKeepAlives so each probe opens a fresh TCP connection. Without
	// this, a half-dead pooled connection produces reset/EOF errors that look
	// like an outage but only affect the probe, not real users.
	transport := &http.Transport{DisableKeepAlives: true}
	client := &http.Client{
		Timeout:   *timeout,
		Transport: transport,
	}

	start := time.Now()
	logger.LogAttrs(ctx, slog.LevelInfo, "probe starting",
		slog.String("target", target),
		slog.Duration("interval", *interval),
		slog.Duration("timeout", *timeout))

	results := runProbe(ctx, logger, client, target, *interval, start)
	summarize(ctx, logger, start, results)
}

func runProbe(
	ctx context.Context, logger *slog.Logger, client *http.Client,
	target string, interval time.Duration, start time.Time,
) []result {
	var results []result
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return results
		case <-ticker.C:
			reqCtx, cancel := context.WithTimeout(context.Background(), client.Timeout)
			r := probe(reqCtx, client, target)
			cancel()
			results = append(results, r)
			logger.LogAttrs(ctx, levelFor(r.out), "tick",
				slog.Int64("offset_ms", r.at.Sub(start).Milliseconds()),
				slog.String("outcome", string(r.out)),
				slog.Int("status", r.status),
				slog.Int64("dur_ms", r.duration.Milliseconds()),
				slog.String("error", r.errStr))
		}
	}
}

func levelFor(o outcome) slog.Level {
	if o == outcomeOK {
		return slog.LevelInfo
	}
	return slog.LevelWarn
}

func summarize(ctx context.Context, logger *slog.Logger, start time.Time, results []result) {
	logCtx := context.WithoutCancel(ctx)
	if len(results) == 0 {
		logger.LogAttrs(logCtx, slog.LevelInfo, "summary",
			slog.Duration("wall", time.Since(start).Round(time.Millisecond)),
			slog.Int("total", 0))
		return
	}

	counts := map[outcome]int{}
	for _, r := range results {
		counts[r.out]++
	}
	totalFail := len(results) - counts[outcomeOK]
	failRate := percent * float64(totalFail) / float64(len(results))

	longest, runStart, runEnd := longestFailureRun(results)
	attrs := []slog.Attr{
		slog.Duration("wall", time.Since(start).Round(time.Millisecond)),
		slog.Int("total", len(results)),
		slog.Int("failures", totalFail),
		slog.Float64("fail_pct", failRate),
		slog.Int("ok", counts[outcomeOK]),
		slog.Int("5xx", counts[outcome5xx]),
		slog.Int("refused", counts[outcomeRefused]),
		slog.Int("timeout", counts[outcomeTimeout]),
		slog.Int("reset", counts[outcomeReset]),
		slog.Int("dns", counts[outcomeDNS]),
		slog.Int("other", counts[outcomeOther]),
		slog.Int("longest_run", longest),
	}
	if longest > 0 {
		attrs = append(attrs,
			slog.Duration("longest_gap", runEnd.Sub(runStart).Round(time.Millisecond)),
			slog.Int64("longest_start_ms", runStart.Sub(start).Milliseconds()),
			slog.Int64("longest_end_ms", runEnd.Sub(start).Milliseconds()))
	}
	logger.LogAttrs(logCtx, slog.LevelInfo, "summary", attrs...)
}

func longestFailureRun(results []result) (int, time.Time, time.Time) {
	var (
		longest                  int
		longestStart, longestEnd time.Time
		curLen                   int
		curStart, curEnd         time.Time
	)
	for _, r := range results {
		if r.out == outcomeOK {
			curLen = 0
			continue
		}
		if curLen == 0 {
			curStart = r.at
		}
		curEnd = r.at
		curLen++
		if curLen > longest {
			longest, longestStart, longestEnd = curLen, curStart, curEnd
		}
	}
	return longest, longestStart, longestEnd
}
