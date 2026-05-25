package domain_test

import (
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
)

func Test_WeeklyMuscleGroupVolume_PlannedAndCompleted(t *testing.T) {
	t.Parallel()

	chest := domain.Exercise{ //nolint:exhaustruct // test fixture only needs these fields
		ID:                  1,
		Name:                "Bench Press",
		PrimaryMuscleGroups: []string{"Chest"},
	}
	completedAt := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	completedValue := 8

	sess := domain.Session{ //nolint:exhaustruct // test fixture only needs these fields
		Date: time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
		ExerciseSets: []domain.ExerciseSet{
			{
				Exercise:          chest,
				WarmupCompletedAt: nil,
				Sets: []domain.Set{
					//nolint:exhaustruct // test fixture only needs these fields
					{TargetValue: 5, CompletedAt: &completedAt, CompletedValue: &completedValue},
					{TargetValue: 5}, //nolint:exhaustruct // Not completed; test fixture only needs TargetValue.
				},
			},
		},
	}

	got := domain.WeeklyMuscleGroupVolume(
		[]domain.Session{sess},
		nil,
		[]string{"Chest"},
	)

	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1", len(got))
	}
	if got[0].Name != "Chest" {
		t.Errorf("Name = %q, want Chest", got[0].Name)
	}
	if got[0].PlannedLoad != 2*domain.PrimarySetWeight {
		t.Errorf("PlannedLoad = %v, want %v", got[0].PlannedLoad, 2*domain.PrimarySetWeight)
	}
	if got[0].CompletedLoad != domain.PrimarySetWeight {
		t.Errorf("CompletedLoad = %v, want %v", got[0].CompletedLoad, domain.PrimarySetWeight)
	}
}
