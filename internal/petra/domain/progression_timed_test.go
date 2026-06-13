package domain_test

import (
	"testing"

	"github.com/myrjola/petrapp/internal/petra/domain"
)

func TestTimedProgressionCurrentSet(t *testing.T) {
	t.Parallel()

	type setup struct {
		startingSeconds int
		completed       []domain.SetResult
	}
	tests := []struct {
		name string
		in   setup
		want int
	}{
		{
			name: "first set returns starting seconds",
			in:   setup{startingSeconds: 30, completed: nil},
			want: 30,
		},
		{
			name: "on_target keeps target",
			in: setup{startingSeconds: 30, completed: []domain.SetResult{
				{ActualValue: 30, Signal: domain.SignalOnTarget, WeightKg: 0},
			}},
			want: 30,
		},
		{
			name: "too_light under 60s bumps by 5",
			in: setup{startingSeconds: 30, completed: []domain.SetResult{
				{ActualValue: 30, Signal: domain.SignalTooLight, WeightKg: 0},
			}},
			want: 35,
		},
		{
			name: "too_light at 60s bumps by 10",
			in: setup{startingSeconds: 60, completed: []domain.SetResult{
				{ActualValue: 60, Signal: domain.SignalTooLight, WeightKg: 0},
			}},
			want: 70,
		},
		{
			name: "too_light at 120s bumps by 15",
			in: setup{startingSeconds: 120, completed: []domain.SetResult{
				{ActualValue: 120, Signal: domain.SignalTooLight, WeightKg: 0},
			}},
			want: 135,
		},
		{
			name: "too_heavy under 60s drops by 5",
			in: setup{startingSeconds: 30, completed: []domain.SetResult{
				{ActualValue: 20, Signal: domain.SignalTooHeavy, WeightKg: 0},
			}},
			want: 15,
		},
		{
			name: "too_heavy uses ladder step when it exceeds 10% decrement",
			in: setup{startingSeconds: 90, completed: []domain.SetResult{
				{ActualValue: 70, Signal: domain.SignalTooHeavy, WeightKg: 0},
			}},
			// 10% of 70 = 7, snap5 = 5, ladder at 60-119s = 10 → max(10,5) = 10 → 60
			want: 60,
		},
		{
			name: "too_heavy at 120s drops by 15s ladder step",
			in: setup{startingSeconds: 130, completed: []domain.SetResult{
				{ActualValue: 120, Signal: domain.SignalTooHeavy, WeightKg: 0},
			}},
			// ladder at >=120s = 15; 10% of 120 = 12, snap5(12) = 10; max(15, 10) = 15 → 105
			want: 105,
		},
		{
			name: "too_heavy at 200s where 10% percentage exceeds ladder step",
			in: setup{startingSeconds: 210, completed: []domain.SetResult{
				{ActualValue: 200, Signal: domain.SignalTooHeavy, WeightKg: 0},
			}},
			// ladder at >=120s = 15; 10% of 200 = 20, snap5(20) = 20; max(15, 20) = 20 → 180
			want: 180,
		},
		{
			name: "too_light snaps off-grid actual to nearest 5",
			in: setup{startingSeconds: 30, completed: []domain.SetResult{
				{ActualValue: 27, Signal: domain.SignalTooLight, WeightKg: 0},
			}},
			// 27 + 5 (ladder) = 32, snap5(32) = 30
			want: 30,
		},
		{
			name: "too_heavy floors at 5s",
			in: setup{startingSeconds: 5, completed: []domain.SetResult{
				{ActualValue: 5, Signal: domain.SignalTooHeavy, WeightKg: 0},
			}},
			want: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := domain.NewTimedProgressionFromHistory(
				domain.TimedConfig{StartingSeconds: tt.in.startingSeconds},
				tt.in.completed,
			)
			got := p.CurrentSet().TargetValue
			if got != tt.want {
				t.Errorf("TargetValue = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestAdjustedSeconds_UnknownSignalDoesNotPanic(t *testing.T) {
	t.Parallel()
	p := domain.NewTimedProgressionFromHistory(
		domain.TimedConfig{StartingSeconds: 30},
		[]domain.SetResult{{ActualValue: 45, Signal: domain.Signal("bogus"), WeightKg: 0}},
	)
	got := p.CurrentSet()
	if got.TargetValue != 45 {
		t.Errorf("unknown signal: got seconds %v, want unchanged 45", got.TargetValue)
	}
}

func TestTimedProgressionRecordCompletion(t *testing.T) {
	t.Parallel()

	p := domain.NewTimedProgression(domain.TimedConfig{StartingSeconds: 30})
	if got := p.SetsCompleted(); got != 0 {
		t.Fatalf("SetsCompleted before any record = %d, want 0", got)
	}
	p.RecordCompletion(domain.SetResult{
		ActualValue: 30,
		Signal:      domain.SignalOnTarget,
		WeightKg:    0,
	})
	if got := p.SetsCompleted(); got != 1 {
		t.Errorf("SetsCompleted after one record = %d, want 1", got)
	}
	if got := p.CurrentSet().TargetValue; got != 30 {
		t.Errorf("TargetValue after on_target = %d, want 30", got)
	}
}
