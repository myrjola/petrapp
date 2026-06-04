package domain_test

import (
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/petra/domain"
)

func TestWeekInBlock(t *testing.T) {
	t.Parallel()

	anchor := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC) // Monday

	tests := []struct {
		name   string
		date   time.Time
		length int
		want   int
	}{
		{"anchor itself", anchor, 5, 0},
		{"one week after", anchor.AddDate(0, 0, 7), 5, 1},
		{"deload week (length-1)", anchor.AddDate(0, 0, 28), 5, 4},
		{"wraps to next block", anchor.AddDate(0, 0, 35), 5, 0},
		{"mid-block, 4-week cadence", anchor.AddDate(0, 0, 14), 4, 2},
		{"date before anchor returns 0", anchor.AddDate(0, 0, -7), 5, 0},
		{"non-monday date snaps to its week", anchor.AddDate(0, 0, 10), 5, 1}, // Thu of week 1
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.WeekInBlock(tt.date, anchor, tt.length); got != tt.want {
				t.Errorf("WeekInBlock(%s, %s, %d) = %d, want %d",
					tt.date.Format("2006-01-02"), anchor.Format("2006-01-02"), tt.length, got, tt.want)
			}
		})
	}
}

func TestIsDeloadWeek(t *testing.T) {
	t.Parallel()

	anchor := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC) // Monday

	tests := []struct {
		name    string
		enabled bool
		date    time.Time
		length  int
		want    bool
	}{
		{"week 0 of 5 — not deload", true, anchor, 5, false},
		{"week 4 of 5 — is deload", true, anchor.AddDate(0, 0, 28), 5, true},
		{"week 5 (wraps to 0) — not deload", true, anchor.AddDate(0, 0, 35), 5, false},
		{"week 3 of 4 — is deload", true, anchor.AddDate(0, 0, 21), 4, true},
		{"feature disabled", false, anchor.AddDate(0, 0, 28), 5, false},
		{"zero anchor returns false", true, anchor.AddDate(0, 0, 28), 5, false}, // overridden below
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a := anchor
			if tt.name == "zero anchor returns false" {
				a = time.Time{}
			}
			if got := domain.IsDeloadWeek(tt.date, a, tt.length, tt.enabled); got != tt.want {
				t.Errorf("IsDeloadWeek = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsDeloadWeek_LengthBounds(t *testing.T) {
	t.Parallel()

	anchor := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	for _, length := range []int{0, 1, -1} {
		if got := domain.IsDeloadWeek(anchor, anchor, length, true); got {
			t.Errorf("IsDeloadWeek(length=%d) = true, want false (defensive)", length)
		}
	}
}
