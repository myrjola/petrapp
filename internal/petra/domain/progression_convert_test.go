package domain_test

import (
	"math"
	"testing"

	"github.com/myrjola/petrapp/internal/petra/domain"
)

func TestConvertWeight(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		weight   float64
		fromReps int
		toReps   int
		want     float64
	}{
		{
			name:     "same reps returns input unchanged",
			weight:   100.0,
			fromReps: 5,
			toReps:   5,
			want:     100.0,
		},
		{
			name:     "strength 100kg x5 to hypertrophy 8 reps",
			weight:   100.0,
			fromReps: 5,
			toReps:   8,
			want:     92.0,
		},
		{
			name:     "hypertrophy 80kg x8 to strength 5 reps",
			weight:   80.0,
			fromReps: 8,
			toReps:   5,
			want:     87.0,
		},
		{
			name:     "strength 100kg x5 to endurance 15 reps",
			weight:   100.0,
			fromReps: 5,
			toReps:   15,
			want:     78.0,
		},
		{
			name:     "endurance 60kg x15 to strength 5 reps",
			weight:   60.0,
			fromReps: 15,
			toReps:   5,
			want:     77.0,
		},
		{
			name:     "above threshold result is snapped to 0.5 kg",
			weight:   42.5,
			fromReps: 5,
			toReps:   8,
			want:     39.0,
		},
		{
			name:     "dumbbell-range hypertrophy to strength snaps to whole kg",
			weight:   8.0,
			fromReps: 8,
			toReps:   5,
			want:     9.0, // 8 * (1 + 8/30) / (1 + 5/30) ≈ 8.69 → snaps to 9.
		},
		{
			name:     "zero weight returned unchanged",
			weight:   0.0,
			fromReps: 5,
			toReps:   8,
			want:     0.0,
		},
		{
			name:     "assisted strength -50kg x5 to hypertrophy 8 reps adds assistance",
			weight:   -50.0,
			fromReps: 5,
			toReps:   8,
			want:     -54.5, // -50 * (1 + 8/30) / (1 + 5/30) ≈ -54.29 → snaps to -54.5.
		},
		{
			name:     "assisted hypertrophy -50kg x8 to strength 5 reps removes assistance",
			weight:   -50.0,
			fromReps: 8,
			toReps:   5,
			want:     -46.0, // -50 * (1 + 5/30) / (1 + 8/30) ≈ -46.05 → snaps to -46.0.
		},
		{
			name:     "assisted strength -50kg x5 to endurance 15 reps adds more assistance",
			weight:   -50.0,
			fromReps: 5,
			toReps:   15,
			want:     -64.5, // -50 * (1 + 15/30) / (1 + 5/30) ≈ -64.29 → snaps to -64.5.
		},
		{
			name:     "assisted dumbbell-range -5kg x5 to 8 reps snaps to whole kg",
			weight:   -5.0,
			fromReps: 5,
			toReps:   8,
			want:     -5.0, // -5 * 38/35 ≈ -5.43 → snaps to -5.
		},
		{
			name:     "zero fromReps returned unchanged",
			weight:   100.0,
			fromReps: 0,
			toReps:   8,
			want:     100.0,
		},
		{
			name:     "zero toReps returned unchanged",
			weight:   100.0,
			fromReps: 5,
			toReps:   0,
			want:     100.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := domain.ConvertWeight(tt.weight, tt.fromReps, tt.toReps)
			if got != tt.want {
				t.Errorf("ConvertWeight(%v, %d, %d) = %v; want %v",
					tt.weight, tt.fromReps, tt.toReps, got, tt.want)
			}
		})
	}
}

// FuzzConvertWeight asserts the structural invariants ConvertWeight must hold
// across its realistic input domain (finite loads up to 1e6 kg, rep counts
// within ±100 so we exercise both the non-positive guard and the 1..50 working
// range). Concrete numeric outputs are pinned by TestConvertWeight; the fuzzer
// guards the properties that must hold for every input: the output is finite,
// the passthrough cases return the input untouched, the sign is never flipped,
// and converted loads always land on the 0.5 kg grid.
func FuzzConvertWeight(f *testing.F) {
	seeds := []struct {
		weight   float64
		fromReps int
		toReps   int
	}{
		{100.0, 5, 5},  // Same reps (passthrough).
		{100.0, 5, 8},  // Positive load, lightened.
		{80.0, 8, 5},   // Positive load, made heavier.
		{0.0, 5, 8},    // Zero weight (passthrough).
		{-50.0, 5, 8},  // Assisted load, more assistance.
		{-50.0, 8, 5},  // Assisted load, less assistance.
		{0.3, 5, 8},    // Tiny positive load that snaps to zero.
		{8.0, 8, 5},    // Dumbbell range, whole-kg snap.
		{42.5, 5, 8},   // Above threshold, 0.5 kg snap.
		{100.0, 0, 8},  // Zero fromReps (guard).
		{100.0, 5, 0},  // Zero toReps (guard).
		{100.0, -3, 8}, // Negative fromReps (guard).
	}
	for _, s := range seeds {
		f.Add(s.weight, s.fromReps, s.toReps)
	}

	f.Fuzz(func(t *testing.T, weight float64, fromReps, toReps int) {
		if math.IsNaN(weight) || math.IsInf(weight, 0) || math.Abs(weight) > 1e6 {
			t.Skip()
		}
		if fromReps < -100 || fromReps > 100 || toReps < -100 || toReps > 100 {
			t.Skip()
		}

		got := domain.ConvertWeight(weight, fromReps, toReps)

		// Finite, bounded input must never produce NaN or ±Inf.
		if math.IsNaN(got) || math.IsInf(got, 0) {
			t.Fatalf("ConvertWeight(%v, %d, %d) = %v; want finite", weight, fromReps, toReps, got)
		}

		// Zero weight, non-positive reps, or equal reps return the input
		// unchanged — no scaling reference exists.
		passthrough := weight == 0 || fromReps <= 0 || toReps <= 0 || fromReps == toReps
		if passthrough {
			if got != weight {
				t.Fatalf("ConvertWeight(%v, %d, %d) = %v; want passthrough %v",
					weight, fromReps, toReps, got, weight)
			}
			return
		}

		// Sign is never flipped: a positive load stays >= 0, an assisted
		// (negative) load stays <= 0. Tiny loads may legitimately snap to zero.
		if (weight > 0 && got < 0) || (weight < 0 && got > 0) {
			t.Fatalf("ConvertWeight(%v, %d, %d) = %v flipped sign", weight, fromReps, toReps, got)
		}

		// Converted loads always land on the 0.5 kg grid (whole-kg snapping in
		// the dumbbell range is a subset of the 0.5 kg grid).
		if got != math.Round(got*2)/2 {
			t.Fatalf("ConvertWeight(%v, %d, %d) = %v; not on the 0.5 kg grid", weight, fromReps, toReps, got)
		}
	})
}
