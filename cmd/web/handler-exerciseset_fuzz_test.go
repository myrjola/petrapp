package main

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/myrjola/petrapp/internal/domain"
)

// FuzzParseFormWeight asserts the structural contract of parseFormWeight over
// arbitrary form bytes: it never panics, it is deterministic, it accepts a
// value exactly when strconv.ParseFloat yields a finite number (after the
// locale comma-to-dot substitution), and on success it preserves the input
// magnitude while applying the assisted-sign convention — assisted exercises
// with the flag set never store a positive load, every other combination keeps
// the parsed value verbatim.
func FuzzParseFormWeight(f *testing.F) {
	seeds := []struct {
		raw      string
		assisted bool
		typeIdx  int
	}{
		{"100", false, 0},   // Plain weighted load.
		{"42.5", false, 0},  // Dot decimal.
		{"42,5", false, 0},  // Comma decimal (European keyboard).
		{"20", true, 1},     // Assisted with flag → negative.
		{"-20", true, 1},    // Already negative, assisted (idempotent).
		{"20", true, 0},     // Assisted flag ignored by weighted type.
		{"", false, 0},      // Empty → error.
		{"abc", false, 0},   // Non-numeric → error.
		{"NaN", false, 0},   // Non-finite literal → error.
		{"Inf", false, 1},   // Non-finite literal → error.
		{"1e999", false, 0}, // Overflows float64 to +Inf → error.
		{"0", false, 2},     // Zero, bodyweight.
	}
	for _, s := range seeds {
		f.Add(s.raw, s.assisted, s.typeIdx)
	}

	// The closed set of exercise types parseFormWeight is exercised against;
	// only ExerciseTypeAssisted carries the assisted-sign convention, so
	// covering all four pins down that the others ignore the flag.
	exerciseTypes := []domain.ExerciseType{
		domain.ExerciseTypeWeighted,
		domain.ExerciseTypeAssisted,
		domain.ExerciseTypeBodyweight,
		domain.ExerciseTypeTime,
	}

	f.Fuzz(func(t *testing.T, raw string, assisted bool, typeIdx int) {
		// Map the fuzzed index onto a real exercise type (Go's modulo keeps the
		// sign of the dividend, so guard against a negative result).
		idx := typeIdx % len(exerciseTypes)
		if idx < 0 {
			idx += len(exerciseTypes)
		}
		exType := exerciseTypes[idx]
		exercise := domain.Exercise{ //nolint:exhaustruct // Only ExerciseType drives parseFormWeight.
			ExerciseType: exType,
		}

		got, err := parseFormWeight(raw, assisted, exercise)

		// Determinism: the same input must always yield the same outcome.
		got2, err2 := parseFormWeight(raw, assisted, exercise)
		if (err == nil) != (err2 == nil) || got != got2 {
			t.Fatalf("parseFormWeight not deterministic: (%v,%v) then (%v,%v)", got, err, got2, err2)
		}

		// Independent re-derivation of whether the input is a finite number.
		parsed, parseErr := strconv.ParseFloat(strings.Replace(raw, ",", ".", 1), 64)
		finite := parseErr == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0)

		if err != nil {
			if finite {
				t.Fatalf("parseFormWeight(%q) errored but %q is a finite number", raw, raw)
			}
			if got != 0 {
				t.Fatalf("parseFormWeight(%q) errored but returned %v, want 0", raw, got)
			}
			return
		}

		if !finite {
			t.Fatalf("parseFormWeight(%q) accepted a non-finite value %v", raw, parsed)
		}

		// Magnitude is never altered by parsing — only the sign may change.
		if math.Abs(got) != math.Abs(parsed) {
			t.Fatalf("parseFormWeight(%q) = %v changed magnitude of %v", raw, got, parsed)
		}

		if exType == domain.ExerciseTypeAssisted && assisted {
			// Assisted convention: stored as the negative magnitude, so a
			// positive load can never survive.
			if got > 0 {
				t.Fatalf("assisted parseFormWeight(%q, assisted) = %v; want <= 0", raw, got)
			}
		} else if got != parsed {
			// Every other combination passes the parsed value through verbatim.
			t.Fatalf("parseFormWeight(%q) = %v; want unchanged %v for type %s (assisted=%v)",
				raw, got, parsed, exType, assisted)
		}
	})
}
