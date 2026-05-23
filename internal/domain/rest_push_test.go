package domain_test

import (
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
)

func TestPlanRestPush(t *testing.T) {
	t.Parallel()

	repMin5, repMax5 := 5, 5     // strength → 180s rest
	repMin12, repMax15 := 12, 15 // hypertrophy → 90s rest
	startSecs := 30              // for the timed-exercise case

	squat := domain.Exercise{ //nolint:exhaustruct
		Name:         "Squat",
		ExerciseType: domain.ExerciseTypeWeighted,
		RepMin:       &repMin5,
		RepMax:       &repMax5,
	}
	plank := domain.Exercise{ //nolint:exhaustruct
		Name:                   "Plank",
		ExerciseType:           domain.ExerciseTypeTime,
		DefaultStartingSeconds: &startSecs,
	}
	curl := domain.Exercise{ //nolint:exhaustruct
		Name:         "Bicep Curl",
		ExerciseType: domain.ExerciseTypeWeighted,
		RepMin:       &repMin12,
		RepMax:       &repMax15,
	}

	completedAt := time.Date(2026, 5, 23, 10, 0, 0, 0, time.UTC)
	v := 5
	w := 100.0
	done := time.Date(2026, 5, 23, 9, 59, 0, 0, time.UTC)
	completedSet := domain.Set{ //nolint:exhaustruct
		WeightKg: &w, TargetValue: 5, CompletedValue: &v, CompletedAt: &done,
	}
	incompleteSet := domain.Set{ //nolint:exhaustruct
		WeightKg: &w, TargetValue: 5,
	}

	tests := []struct {
		name     string
		slot     domain.ExerciseSet
		pt       domain.PeriodizationType
		isDeload bool
		want     domain.RestPushDecision
	}{
		{
			name: "empty slot returns Cancel",
			slot: domain.ExerciseSet{ //nolint:exhaustruct
				ID: 1, Exercise: squat, Sets: []domain.Set{},
			},
			pt:   domain.PeriodizationStrength,
			want: domain.RestPushDecision{Action: domain.RestPushActionCancel}, //nolint:exhaustruct
		},
		{
			name: "all sets complete returns Cancel",
			slot: domain.ExerciseSet{ //nolint:exhaustruct
				ID: 1, Exercise: squat,
				Sets: []domain.Set{completedSet, completedSet, completedSet},
			},
			pt:   domain.PeriodizationStrength,
			want: domain.RestPushDecision{Action: domain.RestPushActionCancel}, //nolint:exhaustruct
		},
		{
			name: "no sets completed yet (warmup-just-done) schedules set 1",
			slot: domain.ExerciseSet{ //nolint:exhaustruct
				ID: 1, Exercise: squat,
				Sets: []domain.Set{incompleteSet, incompleteSet, incompleteSet},
			},
			pt: domain.PeriodizationStrength,
			want: domain.RestPushDecision{
				Action: domain.RestPushActionSchedule,
				FireAt: completedAt.Add(180 * time.Second),
				Payload: domain.RestPushPayload{
					Title:         "Rest over",
					Body:          "Time for set 1 of 3 — Squat",
					ExerciseName:  "Squat",
					NextSetNumber: 1,
					SetsTotal:     3,
				},
			},
		},
		{
			name: "mid-exercise schedules next set",
			slot: domain.ExerciseSet{ //nolint:exhaustruct
				ID: 1, Exercise: squat,
				Sets: []domain.Set{completedSet, completedSet, incompleteSet},
			},
			pt: domain.PeriodizationStrength,
			want: domain.RestPushDecision{
				Action: domain.RestPushActionSchedule,
				FireAt: completedAt.Add(180 * time.Second),
				Payload: domain.RestPushPayload{
					Title:         "Rest over",
					Body:          "Time for set 3 of 3 — Squat",
					ExerciseName:  "Squat",
					NextSetNumber: 3,
					SetsTotal:     3,
				},
			},
		},
		{
			name: "time-based exercise (RestSecondsFor returns 0) returns NoOp",
			slot: domain.ExerciseSet{ //nolint:exhaustruct
				ID: 1, Exercise: plank,
				Sets: []domain.Set{incompleteSet, incompleteSet},
			},
			pt:   domain.PeriodizationStrength,
			want: domain.RestPushDecision{Action: domain.RestPushActionNoOp}, //nolint:exhaustruct
		},
		{
			name: "deload session uses deload rest mapping",
			slot: domain.ExerciseSet{ //nolint:exhaustruct
				ID: 1, Exercise: curl,
				Sets: []domain.Set{incompleteSet, incompleteSet},
			},
			pt:       domain.PeriodizationStrength,
			isDeload: true,
			want: domain.RestPushDecision{
				Action: domain.RestPushActionSchedule,
				FireAt: completedAt.Add(90 * time.Second),
				Payload: domain.RestPushPayload{
					Title:         "Rest over",
					Body:          "Time for set 1 of 2 — Bicep Curl",
					ExerciseName:  "Bicep Curl",
					NextSetNumber: 1,
					SetsTotal:     2,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := domain.PlanRestPush(tt.slot, tt.pt, tt.isDeload, completedAt)
			if got != tt.want {
				t.Errorf("PlanRestPush() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
