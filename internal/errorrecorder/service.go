package errorrecorder

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const (
	sessionKeyAttrSessionHash = "session_hash"
	sessionKeyAttrTraceID     = "trace_id"
	rateLimitWindow           = 1 * time.Hour
	defaultMaxPerSession      = 500
	defaultMaxSessions        = 1000
	defaultJobsBuffer         = 8
	prunerInterval            = 1 * time.Minute
	shutdownGrace             = 5 * time.Second
	// backgroundGoroutines is the number of long-lived goroutines New
	// spawns (pruner + worker) and Close waits on.
	backgroundGoroutines = 2
)

// Config configures a Service. Inner and LogsDirectory are required when
// the recorder is enabled (LogsDirectory != ""). The remaining fields
// default to sensible production values.
type Config struct {
	// Inner is the wrapped slog.Handler. Records are forwarded here first;
	// the recorder also emits its own log lines through this handler to
	// avoid feedback loops.
	Inner slog.Handler
	// LogsDirectory is the root under which YYYY/MM/DD/<file>.jsonl dump
	// files are written. Empty disables the recorder entirely.
	LogsDirectory string
	// Window is the lookback window for buffered records. Records older
	// than now-Window are pruned.
	Window time.Duration
	// RateLimit is the maximum number of dumps the recorder will produce
	// in a rolling one-hour window across all sessions.
	RateLimit int
	// HandlerOptions configures the JSONHandler used to format dump
	// files. Defaults to {Level: slog.LevelDebug}.
	HandlerOptions *slog.HandlerOptions
	// Clock is the time source. Defaults to realClock.
	Clock Clock
}

// bufferedRecord holds a cloned slog.Record alongside the time it was added,
// so the pruner can drop entries older than the configured window.
type bufferedRecord struct {
	record  slog.Record
	addedAt time.Time
}

// sessionBuffer is a fixed-size ring of bufferedRecord for one key.
// The slice has length up to maxPerSession; full is true once it has
// wrapped at least once.
type sessionBuffer struct {
	records  []bufferedRecord
	head     int
	full     bool
	lastSeen time.Time
}

// dumpJob is the unit of work handed from observe to the worker goroutine.
type dumpJob struct {
	key     string
	records []bufferedRecord
	ts      time.Time
}

// Service owns the in-memory buffers, rate-limit budget, and on-disk writer
// for the error recorder. A single Service is shared by all clones of the
// Handler produced via WithAttrs / WithGroup.
type Service struct {
	mu              sync.Mutex
	sessions        map[string]*sessionBuffer
	dumpTimes       []time.Time
	maxPerSession   int
	maxSessions     int
	window          time.Duration
	rateLimit       int
	clock           Clock
	inner           slog.Handler // for the recorder's own log lines and the wrapped chain.
	logsDirectory   string
	handlerOptions  *slog.HandlerOptions
	jobs            chan dumpJob
	stop            chan struct{}
	wg              sync.WaitGroup
	pendingJobsMu   sync.Mutex
	pendingJobs     int
	completedDumps  int
	completedSignal chan struct{}
	disabled        bool
}

// New constructs a Service. When cfg.LogsDirectory == "", New returns a
// disabled Service whose Handler() returns cfg.Inner unchanged and whose
// Close is a no-op. Otherwise New spawns the pruner and worker goroutines.
func New(cfg Config) (*Service, error) {
	if cfg.Inner == nil {
		return nil, errors.New("errorrecorder: Config.Inner is required")
	}
	if cfg.LogsDirectory == "" {
		return &Service{ //nolint:exhaustruct // disabled mode only needs inner and disabled.
			inner:    cfg.Inner,
			disabled: true,
		}, nil
	}
	if cfg.Clock == nil {
		cfg.Clock = realClock{}
	}
	if cfg.Window <= 0 {
		return nil, errors.New("errorrecorder: Config.Window must be positive")
	}
	if cfg.RateLimit <= 0 {
		return nil, errors.New("errorrecorder: Config.RateLimit must be positive")
	}
	if cfg.HandlerOptions == nil {
		//nolint:exhaustruct // AddSource/ReplaceAttr defaults are intentional.
		cfg.HandlerOptions = &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}
	}

	s := &Service{ //nolint:exhaustruct // pendingJobsMu/completedDumps/pendingJobs zero-init.
		sessions:        map[string]*sessionBuffer{},
		maxPerSession:   defaultMaxPerSession,
		maxSessions:     defaultMaxSessions,
		window:          cfg.Window,
		rateLimit:       cfg.RateLimit,
		clock:           cfg.Clock,
		inner:           cfg.Inner,
		logsDirectory:   cfg.LogsDirectory,
		handlerOptions:  cfg.HandlerOptions,
		jobs:            make(chan dumpJob, defaultJobsBuffer),
		stop:            make(chan struct{}),
		completedSignal: make(chan struct{}, 1),
	}

	s.wg.Add(backgroundGoroutines)
	go s.runPruner()
	go s.runWorker()
	s.emitInfo("error recorder started",
		slog.String("logs_directory", cfg.LogsDirectory),
		slog.Duration("window", cfg.Window),
		slog.Int("rate_limit", cfg.RateLimit),
	)
	return s, nil
}

// Handler returns the slog.Handler to install. When the recorder is
// disabled, this returns the inner handler unchanged.
func (s *Service) Handler() slog.Handler {
	if s.disabled {
		return s.inner
	}
	return &Handler{service: s, inner: s.inner} //nolint:exhaustruct // withAttrs/withinGroup zero-init.
}

// Close stops the pruner and worker goroutines and drains queued dumps
// up to a fixed grace period. Safe to call on a disabled Service.
func (s *Service) Close() error {
	if s.disabled {
		return nil
	}
	close(s.stop)
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(shutdownGrace):
	}
	s.emitInfo("error recorder stopped")
	return nil
}

// WaitForDumps blocks until completedDumps reaches n or timeout elapses.
// Intended for test synchronisation; safe to call from production code
// but has no purpose there.
func (s *Service) WaitForDumps(n int, timeout time.Duration) error {
	return s.waitForDumps(n, timeout)
}

// observe is called by the Handler after the inner.Handle returned. It
// buffers the record and, if it is an error, schedules a dump.
func (s *Service) observe(rec slog.Record) {
	key := resolveKey(rec)
	if key == "" {
		return
	}
	s.record(key, rec)
	if rec.Level < slog.LevelError {
		return
	}
	if !s.tryReserveDump() {
		s.emitWarn("error recorder rate limit exceeded",
			slog.String("session_key_prefix", keyPrefix(key)))
		return
	}
	snap := s.snapshot(key)
	if len(snap) == 0 {
		return
	}
	s.pendingJobsMu.Lock()
	s.pendingJobs++
	s.pendingJobsMu.Unlock()
	job := dumpJob{key: key, records: snap, ts: s.clock.Now()}
	select {
	case s.jobs <- job:
	default:
		s.pendingJobsMu.Lock()
		s.pendingJobs--
		s.pendingJobsMu.Unlock()
		s.emitWarn("error recorder dropped dump",
			slog.String("session_key_prefix", keyPrefix(key)))
	}
}

func (s *Service) runPruner() {
	defer s.wg.Done()
	ticker := time.NewTicker(prunerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.pruneOnce()
		}
	}
}

func (s *Service) runWorker() {
	defer s.wg.Done()
	for {
		select {
		case <-s.stop:
			// Drain any queued jobs before exiting.
			for {
				select {
				case job := <-s.jobs:
					s.handleDumpSafely(job)
				default:
					return
				}
			}
		case job := <-s.jobs:
			s.handleDumpSafely(job)
		}
	}
}

func (s *Service) handleDumpSafely(job dumpJob) {
	defer func() {
		if r := recover(); r != nil {
			s.emitWarn("error recorder worker panic",
				slog.Any("recovered", r))
		}
		s.pendingJobsMu.Lock()
		s.pendingJobs--
		s.completedDumps++
		s.pendingJobsMu.Unlock()
		select {
		case s.completedSignal <- struct{}{}:
		default:
		}
	}()

	path, err := writeDump(s.logsDirectory, job.key, job.ts, job.records, s.handlerOptions)
	if err != nil {
		s.emitWarn("error recorder write failed",
			slog.String("session_key_prefix", keyPrefix(job.key)),
			slog.Any("error", err))
		return
	}
	s.emitInfo("captured error context",
		slog.String("file", path),
		slog.Int("records", len(job.records)),
		slog.String("session_key_prefix", keyPrefix(job.key)),
	)
}

func (s *Service) emitInfo(msg string, attrs ...slog.Attr) {
	s.emit(slog.LevelInfo, msg, attrs...)
}

func (s *Service) emitWarn(msg string, attrs ...slog.Attr) {
	s.emit(slog.LevelWarn, msg, attrs...)
}

func (s *Service) emit(level slog.Level, msg string, attrs ...slog.Attr) {
	rec := slog.NewRecord(s.clock.Now(), level, msg, 0)
	rec.AddAttrs(attrs...)
	_ = s.inner.Handle(context.Background(), rec)
}

// keyPrefix returns the first keyPrefixLen chars of key (or fewer if
// shorter). Used in operator log lines so the recorder's own output
// never carries the full session key.
func keyPrefix(key string) string {
	if len(key) > keyPrefixLen {
		return key[:keyPrefixLen]
	}
	return key
}

// waitForDumps blocks until completedDumps reaches n or timeout elapses.
// Test-only helper. Returns an error if the timeout expires before n
// dumps have completed.
func (s *Service) waitForDumps(n int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		s.pendingJobsMu.Lock()
		if s.completedDumps >= n && s.pendingJobs == 0 {
			s.pendingJobsMu.Unlock()
			return nil
		}
		s.pendingJobsMu.Unlock()
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("waitForDumps: timed out after %s; completed=%d pending=%d",
				timeout, s.completedDumps, s.pendingJobs)
		}
		select {
		case <-s.completedSignal:
		case <-time.After(remaining):
		}
	}
}

// resolveKey returns the grouping key for rec, or "" if neither
// session_hash nor trace_id is present in its attrs.
func resolveKey(rec slog.Record) string {
	var sessionHash, traceID string
	rec.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case sessionKeyAttrSessionHash:
			sessionHash = a.Value.String()
		case sessionKeyAttrTraceID:
			traceID = a.Value.String()
		}
		return sessionHash == "" // keep scanning unless we already have the winning key.
	})
	if sessionHash != "" {
		return sessionHash
	}
	return traceID
}

// record appends rec to the session buffer for key. Caller must NOT hold s.mu.
// The record is cloned so subsequent slog handling cannot mutate the buffered
// copy.
func (s *Service) record(key string, rec slog.Record) {
	cloned := rec.Clone()
	now := s.clock.Now()
	s.mu.Lock()
	defer s.mu.Unlock()

	buf, ok := s.sessions[key]
	if !ok {
		// New session — evict LRU if we are at the cap.
		if len(s.sessions) >= s.maxSessions {
			s.evictLRULocked()
		}
		buf = &sessionBuffer{ //nolint:exhaustruct // head/full/lastSeen are zero-initialised.
			records: make([]bufferedRecord, 0, s.maxPerSession),
		}
		s.sessions[key] = buf
	}
	buf.lastSeen = now
	if len(buf.records) < s.maxPerSession {
		buf.records = append(buf.records, bufferedRecord{record: cloned, addedAt: now})
		return
	}
	buf.records[buf.head] = bufferedRecord{record: cloned, addedAt: now}
	buf.head = (buf.head + 1) % s.maxPerSession
	buf.full = true
}

// snapshot returns the buffered records for key in chronological order.
// Returns an empty slice if key is unknown. Caller must NOT hold s.mu.
func (s *Service) snapshot(key string) []bufferedRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	buf, ok := s.sessions[key]
	if !ok {
		return nil
	}
	if !buf.full {
		out := make([]bufferedRecord, len(buf.records))
		copy(out, buf.records)
		return out
	}
	out := make([]bufferedRecord, 0, len(buf.records))
	out = append(out, buf.records[buf.head:]...)
	out = append(out, buf.records[:buf.head]...)
	return out
}

// pruneOnce drops sessions whose lastSeen is older than now-window. Safe to
// call concurrently with record/snapshot. Exposed package-private so tests
// can drive pruning deterministically without waiting on the ticker.
func (s *Service) pruneOnce() {
	cutoff := s.clock.Now().Add(-s.window)
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, buf := range s.sessions {
		if buf.lastSeen.Before(cutoff) {
			delete(s.sessions, key)
		}
	}
}

// evictLRULocked drops the session with the oldest lastSeen. Caller MUST hold s.mu.
func (s *Service) evictLRULocked() {
	var oldestKey string
	var oldestSeen time.Time
	first := true
	for k, b := range s.sessions {
		if first || b.lastSeen.Before(oldestSeen) {
			oldestKey = k
			oldestSeen = b.lastSeen
			first = false
		}
	}
	if oldestKey != "" {
		delete(s.sessions, oldestKey)
	}
}

// tryReserveDump returns true iff a dump may proceed under the global
// rate limit. Side effect on success: records the dump time in the
// sliding window.
func (s *Service) tryReserveDump() bool {
	now := s.clock.Now()
	cutoff := now.Add(-rateLimitWindow)
	s.mu.Lock()
	defer s.mu.Unlock()
	// Drop entries that fell out of the sliding window.
	kept := s.dumpTimes[:0]
	for _, t := range s.dumpTimes {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	s.dumpTimes = kept
	if len(s.dumpTimes) >= s.rateLimit {
		return false
	}
	s.dumpTimes = append(s.dumpTimes, now)
	return true
}
