package repository_test

import (
	"sort"
	"testing"
)

func TestMuscleGroupTargetRepository_ListReturnsSeededTargets(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	got, err := repos.MuscleTargets.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) == 0 {
		t.Fatalf("expected seeded muscle-group targets, got 0 rows")
	}
	// Verify alphabetical ordering — the planner relies on it.
	names := make([]string, len(got))
	for i, t := range got {
		names[i] = t.MuscleGroupName
	}
	if !sort.StringsAreSorted(names) {
		t.Errorf("muscle-group targets must be sorted by name: %v", names)
	}
	// Verify every row has a positive weekly set target — defensive against
	// schema regressions that would let the planner divide by zero.
	for _, target := range got {
		if target.WeeklySetTarget <= 0 {
			t.Errorf("muscle-group %q has non-positive WeeklySetTarget %d",
				target.MuscleGroupName, target.WeeklySetTarget)
		}
	}
}
