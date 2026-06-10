package domain_test

import (
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/petra/domain"
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
		Slots: []domain.ExerciseSlot{
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
	if got[0].PlannedVolume != 2*domain.PrimarySetFraction {
		t.Errorf("PlannedVolume = %v, want %v", got[0].PlannedVolume, 2*domain.PrimarySetFraction)
	}
	if got[0].CompletedVolume != domain.PrimarySetFraction {
		t.Errorf("CompletedVolume = %v, want %v", got[0].CompletedVolume, domain.PrimarySetFraction)
	}
}

func TestWeeklyPlannedVolume(t *testing.T) {
	t.Parallel()

	bench := domain.Exercise{ //nolint:exhaustruct // Test exercise omits display fields.
		ID:                    1,
		Name:                  "Bench Press",
		Category:              domain.CategoryUpper,
		ExerciseType:          domain.ExerciseTypeWeighted,
		PrimaryMuscleGroups:   []string{"Chest", "Triceps"},
		SecondaryMuscleGroups: []string{"Shoulders"},
	}
	pulldown := domain.Exercise{ //nolint:exhaustruct // Test exercise omits display fields.
		ID:                    2,
		Name:                  "Pulldown",
		Category:              domain.CategoryUpper,
		ExerciseType:          domain.ExerciseTypeWeighted,
		PrimaryMuscleGroups:   []string{"Lats"},
		SecondaryMuscleGroups: []string{"Biceps", "Shoulders"},
	}

	// One session with two exercises: bench 4 sets, pulldown 3 sets.
	session := domain.Session{ //nolint:exhaustruct // Rest fields unused in this test.
		Slots: []domain.ExerciseSlot{
			{ //nolint:exhaustruct // WarmupCompletedAt unused in this test.
				Exercise: bench,
				Sets:     make([]domain.Set, 4),
			},
			{ //nolint:exhaustruct // WarmupCompletedAt unused in this test.
				Exercise: pulldown,
				Sets:     make([]domain.Set, 3),
			},
		},
	}

	got := domain.WeeklyPlannedVolume([]domain.Session{session})

	want := map[string]float64{
		"Chest":     4.0,       // 4 × 1.0 primary
		"Triceps":   4.0,       // 4 × 1.0 primary
		"Shoulders": 2.0 + 1.5, // bench secondary 4×0.5 + pulldown secondary 3×0.5
		"Lats":      3.0,       // 3 × 1.0 primary
		"Biceps":    1.5,       // 3 × 0.5 secondary
	}
	for mg, w := range want {
		if got[mg] != w {
			t.Errorf("load[%q] = %v, want %v", mg, got[mg], w)
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %d MGs, want %d (extra entries: %v)", len(got), len(want), diffKeys(got, want))
	}
}

func Test_RegionFor_DeltHeads(t *testing.T) {
	t.Parallel()

	cases := map[string]domain.MuscleGroupRegion{
		domain.MuscleGroupSideDelts: domain.RegionUpperPush,
		domain.MuscleGroupRearDelts: domain.RegionUpperPull,
		domain.MuscleGroupShoulders: domain.RegionUpperPush,
		domain.MuscleGroupChest:     domain.RegionUpperPush,
		domain.MuscleGroupLats:      domain.RegionUpperPull,
		domain.MuscleGroupQuads:     domain.RegionLegs,
		"Unknown Muscle":            domain.RegionOther,
	}
	for name, want := range cases {
		if got := domain.RegionFor(name); got != want {
			t.Errorf("RegionFor(%q) = %q, want %q", name, got, want)
		}
	}
}

func diffKeys(got map[string]float64, want map[string]float64) []string {
	var extra []string
	for k := range got {
		if _, ok := want[k]; !ok {
			extra = append(extra, k)
		}
	}
	return extra
}

func Test_MuscleGroupVolume_Status(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		planned float64
		min     int
		max     int
		want    domain.MuscleGroupVolumeStatus
	}{
		{"no seeded target", 12, 0, 0, domain.MuscleVolumeNoTarget},
		{"below the floor", 9.5, 10, 20, domain.MuscleVolumeUnder},
		{"exactly the floor", 10, 10, 20, domain.MuscleVolumeOnTarget},
		{"inside the band", 15, 10, 20, domain.MuscleVolumeOnTarget},
		{"exactly the ceiling", 20, 10, 20, domain.MuscleVolumeOnTarget},
		{"above the ceiling", 20.5, 10, 20, domain.MuscleVolumeOver},
		// The planner's late-cycle ramp targets MaxSets; the display must not
		// flag the app's own prescription as excessive.
		{"late-cycle ramp at MaxSets", 16, 8, 16, domain.MuscleVolumeOnTarget},
	}
	for _, tc := range cases {
		v := domain.MuscleGroupVolume{
			Name:            "Chest",
			CompletedVolume: 0,
			PlannedVolume:   tc.planned,
			MinSets:         tc.min,
			MaxSets:         tc.max,
		}
		if got := v.Status(); got != tc.want {
			t.Errorf("%s: Status() = %q, want %q", tc.name, got, tc.want)
		}
	}
}
