package exerciseprogression_test

import (
	"testing"

	"github.com/myrjola/petrapp/internal/exerciseprogression"
)

func TestConvertWeight(t *testing.T) {
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
			got := exerciseprogression.ConvertWeight(tt.weight, tt.fromReps, tt.toReps)
			if got != tt.want {
				t.Errorf("ConvertWeight(%v, %d, %d) = %v; want %v",
					tt.weight, tt.fromReps, tt.toReps, got, tt.want)
			}
		})
	}
}

func TestTargetReps(t *testing.T) {
	tests := []struct {
		name string
		p    exerciseprogression.PeriodizationType
		want int
	}{
		{"strength", exerciseprogression.Strength, 5},
		{"hypertrophy", exerciseprogression.Hypertrophy, 8},
		{"endurance", exerciseprogression.Endurance, 15},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := exerciseprogression.TargetReps(tt.p)
			if got != tt.want {
				t.Errorf("TargetReps(%v) = %d; want %d", tt.p, got, tt.want)
			}
		})
	}
}
