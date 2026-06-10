package service_test

import (
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/petra/domain"
)

func Test_WeeklyMuscleGroupVolume_AggregatesPrimaryAndSecondary(t *testing.T) {
	t.Parallel()

	ctx, svc := setupTestService(t)

	// Two synthetic exercises sharing a secondary muscle group so we can verify
	// both the primary (1.0/set) and secondary (0.5/set) weights and that
	// contributions accumulate across exercises.
	bench := domain.Exercise{ //nolint:exhaustruct // ID, Description etc. unused by the aggregator.
		Name:                  "Bench",
		PrimaryMuscleGroups:   []string{"Chest"},
		SecondaryMuscleGroups: []string{"Triceps", "Shoulders"},
	}
	dip := domain.Exercise{ //nolint:exhaustruct // ID, Description etc. unused by the aggregator.
		Name:                  "Dip",
		PrimaryMuscleGroups:   []string{"Triceps"},
		SecondaryMuscleGroups: []string{"Chest"},
	}

	completed := time.Now().UTC()
	completedSet := domain.Set{ //nolint:exhaustruct // Value and weight are not relevant for volume.
		TargetValue: 12,
		CompletedAt: &completed,
	}
	plannedSet := domain.Set{ //nolint:exhaustruct // CompletedAt nil → planned but not completed.
		TargetValue: 12,
	}

	benchSet := domain.ExerciseSlot{ //nolint:exhaustruct // ID + WarmupCompletedAt are repository-managed.
		Exercise: bench,
		Sets:     []domain.Set{completedSet, completedSet, plannedSet},
	}
	dipSet := domain.ExerciseSlot{ //nolint:exhaustruct // ID + WarmupCompletedAt are repository-managed.
		Exercise: dip,
		Sets:     []domain.Set{plannedSet, plannedSet},
	}
	sessions := []domain.Session{
		{ //nolint:exhaustruct // Date and timestamps are not relevant for the volume aggregator.
			Slots: []domain.ExerciseSlot{benchSet, dipSet},
		},
	}

	got, err := svc.WeeklyMuscleGroupVolume(ctx, sessions)
	if err != nil {
		t.Fatalf("WeeklyMuscleGroupVolume: %v", err)
	}

	byName := make(map[string]domain.MuscleGroupVolume, len(got))
	for _, v := range got {
		byName[v.Name] = v
	}

	// Chest: primary on bench (3 sets) + secondary on dip (2 sets * 0.5)
	//        = 3 planned, 2 completed (bench had 2 completed); plus 1.0 secondary completed = 0
	// Bench: 3 planned (2 completed) primary  → planned 3.0, completed 2.0.
	// Dip: 2 planned secondary on chest      → planned 1.0, completed 0.0.
	// Total chest: planned 4.0, completed 2.0.
	if v := byName["Chest"]; v.PlannedVolume != 4.0 || v.CompletedVolume != 2.0 {
		t.Errorf(
			"Chest: want planned=4.0 completed=2.0, got planned=%v completed=%v",
			v.PlannedVolume,
			v.CompletedVolume,
		)
	}

	// Triceps: secondary on bench (3 sets * 0.5 = 1.5 planned, 2*0.5 = 1.0 completed)
	//          + primary on dip (2 sets planned, 0 completed).
	// Total: planned 3.5, completed 1.0.
	if v := byName["Triceps"]; v.PlannedVolume != 3.5 || v.CompletedVolume != 1.0 {
		t.Errorf("Triceps: want planned=3.5 completed=1.0, got planned=%v completed=%v",
			v.PlannedVolume, v.CompletedVolume)
	}

	// Shoulders: secondary on bench only (3 sets * 0.5 = 1.5 planned, 2 * 0.5 = 1.0 completed).
	if v := byName["Shoulders"]; v.PlannedVolume != 1.5 || v.CompletedVolume != 1.0 {
		t.Errorf("Shoulders: want planned=1.5 completed=1.0, got planned=%v completed=%v",
			v.PlannedVolume, v.CompletedVolume)
	}

	// Untouched group must appear with zero load (UI shows it as a flat bar).
	if v, ok := byName["Traps"]; !ok || v.PlannedVolume != 0 || v.CompletedVolume != 0 {
		t.Errorf("Traps: want zero-load entry, got %#v (present=%v)", v, ok)
	}

	// Targets are joined from muscle_group_weekly_targets seed (Chest floor=10, Calves floor=8).
	if v := byName["Chest"]; v.MinSets != 10 || v.MaxSets != 20 {
		t.Errorf("Chest target band: want 10-20, got %d-%d", v.MinSets, v.MaxSets)
	}
	if v := byName["Calves"]; v.MinSets != 8 {
		t.Errorf("Calves target: want 8 (now seeded), got %d", v.MinSets)
	}
	if v := byName["Traps"]; v.MinSets != 0 || v.MaxSets != 0 {
		t.Errorf("Traps target band: want 0-0 (tag-only, not seeded), got %d-%d", v.MinSets, v.MaxSets)
	}

	// Result must list every muscle group exactly once, in alphabetical order.
	allNames, err := svc.ListMuscleGroups(ctx)
	if err != nil {
		t.Fatalf("ListMuscleGroups: %v", err)
	}
	if len(got) != len(allNames) {
		t.Errorf("result count: want %d (all groups), got %d", len(allNames), len(got))
	}
	for i, v := range got {
		if v.Name != allNames[i] {
			t.Errorf("result[%d].Name: want %q, got %q", i, allNames[i], v.Name)
		}
	}
}

func Test_WeeklyMuscleGroupVolume_EmptyWeek(t *testing.T) {
	t.Parallel()

	ctx, svc := setupTestService(t)

	got, err := svc.WeeklyMuscleGroupVolume(ctx, nil)
	if err != nil {
		t.Fatalf("WeeklyMuscleGroupVolume on nil sessions: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("want one entry per muscle group even when sessions are empty, got 0")
	}
	for _, v := range got {
		if v.PlannedVolume != 0 || v.CompletedVolume != 0 {
			t.Errorf("%s: want zero load on empty week, got planned=%v completed=%v",
				v.Name, v.PlannedVolume, v.CompletedVolume)
		}
	}
}
