//nolint:testpackage // parseTimestamp and formatTimestamp are unexported; round-tripping them needs the internal package.
package repository

import (
	"database/sql"
	"testing"
	"time"
)

// Bounds of the round-trippable range for the canonical timestamp format,
// whose year field is exactly four digits (layout "2006"): a time outside
// years 1..9999 formats to a string parseTimestamp can no longer read back.
// Real timestamps (workout sessions, set completions) sit deep inside this
// range, so constraining the fuzzer here keeps every iteration meaningful.
const (
	minTimestampMilli = -62135596800000 // 0001-01-01T00:00:00.000Z.
	maxTimestampMilli = 253402300799999 // 9999-12-31T23:59:59.999Z.
)

// FuzzTimestampRoundTrip asserts that formatTimestamp and parseTimestamp are
// inverses on the canonical wire format. For any instant in the supported
// range, formatting then parsing must succeed, recover the same instant at
// millisecond resolution (the format's precision), and re-format to a byte-
// identical string — the property that lets the repository serialise a
// time.Time to SQLite and read it back without drift.
func FuzzTimestampRoundTrip(f *testing.F) {
	seeds := []int64{
		0,               // 1970-01-01T00:00:00.000Z (unix epoch).
		1735689600000,   // 2025-01-01T00:00:00.000Z.
		1735689600123,   // Non-zero millisecond fraction.
		-62135596800000, // Lower bound (year 1).
		253402300799999, // Upper bound (year 9999).
		-1,              // Just before the epoch (sub-second negative).
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, unixMilli int64) {
		if unixMilli < minTimestampMilli || unixMilli > maxTimestampMilli {
			t.Skip()
		}
		original := time.UnixMilli(unixMilli).UTC()

		formatted := formatTimestamp(original)

		parsed, err := parseTimestamp(sql.NullString{String: formatted, Valid: true})
		if err != nil {
			t.Fatalf("parseTimestamp(%q) returned error: %v", formatted, err)
		}

		// Millisecond resolution is exact: UnixMilli has no sub-millisecond
		// component, and the format carries three fractional digits.
		if !parsed.Equal(original) {
			t.Fatalf("round trip lost the instant: formatted %q parsed to %v, want %v",
				formatted, parsed, original)
		}

		// Re-formatting the parsed value must reproduce the canonical string.
		if reformatted := formatTimestamp(parsed); reformatted != formatted {
			t.Fatalf("re-format not stable: %q -> parse -> %q", formatted, reformatted)
		}
	})
}

// FuzzParseTimestamp asserts that parseTimestamp never panics on arbitrary
// stored bytes, that a NULL column always reads back as the zero time with no
// error, and that formatTimestamp is an idempotent canonicaliser. time.Parse
// is deliberately lenient (it accepts, e.g., extra fractional-second digits
// the formatter never emits), so an accepted string need not be byte-identical
// to its formatting — but once canonicalised, every further round trip must be
// stable in both the string and the instant it represents.
func FuzzParseTimestamp(f *testing.F) {
	f.Add("2025-01-01T00:00:00.000Z")
	f.Add("")
	f.Add("not-a-timestamp")
	f.Add("2025-13-99T99:99:99.999Z")
	f.Add("2025-01-01T00:00:00.0000Z") // Extra fractional digit: lenient parse, non-canonical.

	f.Fuzz(func(t *testing.T, stored string) {
		got, err := parseTimestamp(sql.NullString{String: stored, Valid: false})
		if err != nil || !got.IsZero() {
			t.Fatalf("NULL column: parseTimestamp returned (%v, %v); want (zero time, nil)", got, err)
		}

		parsed, err := parseTimestamp(sql.NullString{String: stored, Valid: true})
		if err != nil {
			return // Rejecting malformed input is the expected outcome.
		}

		// Canonicalise once, then assert the canonical form is a fixed point.
		canonical := formatTimestamp(parsed)
		reparsed, err := parseTimestamp(sql.NullString{String: canonical, Valid: true})
		if err != nil {
			t.Fatalf("canonical form %q (from %q) failed to re-parse: %v", canonical, stored, err)
		}
		if !reparsed.Equal(parsed) {
			t.Fatalf("canonicalisation drifted the instant: %q -> %v -> %q -> %v",
				stored, parsed, canonical, reparsed)
		}
		if again := formatTimestamp(reparsed); again != canonical {
			t.Fatalf("format not idempotent: %q -> %q -> %q", stored, canonical, again)
		}
	})
}
