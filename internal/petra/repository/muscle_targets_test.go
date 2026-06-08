package repository_test

import (
	"sort"
	"testing"
)

func TestMuscleGroupTargetRepository_ListReturnsSeededRangeTargets(t *testing.T) {
	t.Parallel()

	ctx, repos := setupTestRepos(t)

	got, err := repos.MuscleTargets.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 14 {
		t.Fatalf("expected 14 seeded range targets, got %d", len(got))
	}

	// Verify alphabetical ordering — the planner relies on it.
	names := make([]string, len(got))
	byName := make(map[string][2]int, len(got))
	for i, target := range got {
		names[i] = target.MuscleGroupName
		byName[target.MuscleGroupName] = [2]int{target.MinSets, target.MaxSets}
	}
	if !sort.StringsAreSorted(names) {
		t.Errorf("muscle-group targets must be sorted by name: %v", names)
	}

	// Spot-check spec values, including newly targeted groups.
	want := map[string][2]int{
		"Chest":      {10, 20},
		"Side Delts": {8, 18},
		"Rear Delts": {8, 18},
		"Calves":     {8, 16},
		"Lower Back": {4, 8},
	}
	for name, w := range want {
		if byName[name] != w {
			t.Errorf("target %q = %v, want %v", name, byName[name], w)
		}
	}

	// Every row must satisfy 0 < MinSets <= MaxSets (defends the planner and
	// the CHECK constraints).
	for _, target := range got {
		if target.MinSets <= 0 || target.MaxSets < target.MinSets {
			t.Errorf("muscle-group %q has invalid range Min=%d Max=%d",
				target.MuscleGroupName, target.MinSets, target.MaxSets)
		}
	}
}
