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

func TestMesocycleRampProgress(t *testing.T) {
	t.Parallel()

	anchor := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC) // Monday, week 0.
	week := func(n int) time.Time { return anchor.AddDate(0, 0, 7*n) }

	tests := []struct {
		name          string
		date          time.Time
		length        int
		deloadEnabled bool
		want          float64
	}{
		// 4-week block: weeks 0,1,2 train; week 3 deloads.
		{"week 0 is ramp start", week(0), 4, true, 0.0},
		{"week 1 is halfway", week(1), 4, true, 0.5},
		{"week 2 is peak (last training week)", week(2), 4, true, 1.0},
		{"deload week is excluded (progress 0)", week(3), 4, true, 0.0},
		{"block repeats: week 4 == week 0", week(4), 4, true, 0.0},
		// Feature-off states collapse to 0 (static floor).
		{"deload disabled → 0", week(2), 4, false, 0.0},
		{"length below cadence → 0", week(2), 1, true, 0.0},
		// length 2: one training week + deload, no room to ramp.
		{"length 2 training week → 0", week(0), 2, true, 0.0},
		{"length 2 deload week → 0", week(1), 2, true, 0.0},
		// length 3: weeks 0,1 train; week 2 deloads.
		{"length 3 week 0 → 0", week(0), 3, true, 0.0},
		{"length 3 week 1 → peak 1.0", week(1), 3, true, 1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := domain.MesocycleRampProgress(tt.date, anchor, tt.length, tt.deloadEnabled)
			if got != tt.want {
				t.Errorf("MesocycleRampProgress = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMesocycleRampProgress_ZeroAnchor(t *testing.T) {
	t.Parallel()
	if got := domain.MesocycleRampProgress(time.Now(), time.Time{}, 4, true); got != 0 {
		t.Errorf("zero anchor: got %v, want 0", got)
	}
}

func TestSetsForWeek(t *testing.T) {
	t.Parallel()

	anchor := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC) // Monday, week 0.
	week := func(n int) time.Time { return anchor.AddDate(0, 0, 7*n) }

	tests := []struct {
		name          string
		date          time.Time
		length        int
		deloadEnabled bool
		want          int
	}{
		{"week 0 base = 3", week(0), 4, true, 3},
		{"week 1 rounds 0.5 ramp up to 4", week(1), 4, true, 4}, // 3 + round(0.5*1) = 4.
		{"week 2 peak = 4", week(2), 4, true, 4},
		{"deload week back to base 3 (reduction applied downstream)", week(3), 4, true, 3},
		{"feature off → base 3", week(2), 4, false, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := domain.SetsForWeek(tt.date, anchor, tt.length, tt.deloadEnabled)
			if got != tt.want {
				t.Errorf("SetsForWeek = %d, want %d", got, tt.want)
			}
		})
	}
}
