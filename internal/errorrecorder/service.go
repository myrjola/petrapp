package errorrecorder

import (
	"log/slog"
	"sync"
	"time"
)

// sessionKeyAttrSessionHash is the record-attr key the recorder scans for
// first when grouping a record into a buffer.
const sessionKeyAttrSessionHash = "session_hash"

// sessionKeyAttrTraceID is the fallback record-attr key used when no
// session_hash is present.
const sessionKeyAttrTraceID = "trace_id"

// rateLimitWindow is the time over which rateLimit caps the number of dumps.
const rateLimitWindow = 1 * time.Hour

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

// Service owns the in-memory buffers, rate-limit budget, and on-disk writer
// for the error recorder. A single Service is shared by all clones of the
// Handler produced via WithAttrs / WithGroup.
type Service struct {
	mu            sync.Mutex
	sessions      map[string]*sessionBuffer
	dumpTimes     []time.Time
	maxPerSession int
	maxSessions   int
	window        time.Duration
	rateLimit     int
	clock         Clock
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
