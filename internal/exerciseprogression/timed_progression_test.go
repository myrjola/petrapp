package exerciseprogression_test

import (
	"testing"

	"github.com/myrjola/petrapp/internal/exerciseprogression"
)

func TestTimedProgressionCurrentSet(t *testing.T) {
	t.Parallel()

	type setup struct {
		startingSeconds int
		completed       []exerciseprogression.TimedSetResult
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
			in: setup{startingSeconds: 30, completed: []exerciseprogression.TimedSetResult{
				{ActualSeconds: 30, Signal: exerciseprogression.SignalOnTarget},
			}},
			want: 30,
		},
		{
			name: "too_light under 60s bumps by 5",
			in: setup{startingSeconds: 30, completed: []exerciseprogression.TimedSetResult{
				{ActualSeconds: 30, Signal: exerciseprogression.SignalTooLight},
			}},
			want: 35,
		},
		{
			name: "too_light at 60s bumps by 10",
			in: setup{startingSeconds: 60, completed: []exerciseprogression.TimedSetResult{
				{ActualSeconds: 60, Signal: exerciseprogression.SignalTooLight},
			}},
			want: 70,
		},
		{
			name: "too_light at 120s bumps by 15",
			in: setup{startingSeconds: 120, completed: []exerciseprogression.TimedSetResult{
				{ActualSeconds: 120, Signal: exerciseprogression.SignalTooLight},
			}},
			want: 135,
		},
		{
			name: "too_heavy under 60s drops by 5",
			in: setup{startingSeconds: 30, completed: []exerciseprogression.TimedSetResult{
				{ActualSeconds: 20, Signal: exerciseprogression.SignalTooHeavy},
			}},
			want: 15,
		},
		{
			name: "too_heavy uses ladder step when it exceeds 10% decrement",
			in: setup{startingSeconds: 90, completed: []exerciseprogression.TimedSetResult{
				{ActualSeconds: 70, Signal: exerciseprogression.SignalTooHeavy},
			}},
			// 10% of 70 = 7, snap5 = 5, ladder at 60-119s = 10 → max(10,5) = 10 → 60
			want: 60,
		},
		{
			name: "too_heavy at 120s drops by 15s ladder step",
			in: setup{startingSeconds: 130, completed: []exerciseprogression.TimedSetResult{
				{ActualSeconds: 120, Signal: exerciseprogression.SignalTooHeavy},
			}},
			// ladder at >=120s = 15; 10% of 120 = 12, snap5(12) = 10; max(15, 10) = 15 → 105
			want: 105,
		},
		{
			name: "too_heavy at 200s where 10% percentage exceeds ladder step",
			in: setup{startingSeconds: 210, completed: []exerciseprogression.TimedSetResult{
				{ActualSeconds: 200, Signal: exerciseprogression.SignalTooHeavy},
			}},
			// ladder at >=120s = 15; 10% of 200 = 20, snap5(20) = 20; max(15, 20) = 20 → 180
			want: 180,
		},
		{
			name: "too_light snaps off-grid actual to nearest 5",
			in: setup{startingSeconds: 30, completed: []exerciseprogression.TimedSetResult{
				{ActualSeconds: 27, Signal: exerciseprogression.SignalTooLight},
			}},
			// 27 + 5 (ladder) = 32, snap5(32) = 30
			want: 30,
		},
		{
			name: "too_heavy floors at 5s",
			in: setup{startingSeconds: 5, completed: []exerciseprogression.TimedSetResult{
				{ActualSeconds: 5, Signal: exerciseprogression.SignalTooHeavy},
			}},
			want: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := exerciseprogression.NewTimedFromHistory(
				exerciseprogression.TimedConfig{StartingSeconds: tt.in.startingSeconds},
				tt.in.completed,
			)
			got := p.CurrentSet().TargetSeconds
			if got != tt.want {
				t.Errorf("TargetSeconds = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestTimedProgressionRecordCompletion(t *testing.T) {
	t.Parallel()

	p := exerciseprogression.NewTimed(exerciseprogression.TimedConfig{StartingSeconds: 30})
	if got := p.SetsCompleted(); got != 0 {
		t.Fatalf("SetsCompleted before any record = %d, want 0", got)
	}
	p.RecordCompletion(exerciseprogression.TimedSetResult{
		ActualSeconds: 30,
		Signal:        exerciseprogression.SignalOnTarget,
	})
	if got := p.SetsCompleted(); got != 1 {
		t.Errorf("SetsCompleted after one record = %d, want 1", got)
	}
	if got := p.CurrentSet().TargetSeconds; got != 30 {
		t.Errorf("TargetSeconds after on_target = %d, want 30", got)
	}
}
