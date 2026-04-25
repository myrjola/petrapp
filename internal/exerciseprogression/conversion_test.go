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
			name:     "result is rounded to nearest 0.5 kg",
			weight:   42.5,
			fromReps: 5,
			toReps:   8,
			want:     39.0,
		},
		{
			name:     "zero weight returned unchanged",
			weight:   0.0,
			fromReps: 5,
			toReps:   8,
			want:     0.0,
		},
		{
			name:     "negative weight returned unchanged",
			weight:   -5.0,
			fromReps: 5,
			toReps:   8,
			want:     -5.0,
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
