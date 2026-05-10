package repository_test

import (
	"errors"
	"testing"

	"github.com/myrjola/petrapp/internal/domain"
)

func ptrInt(v int) *int { return &v }

// testExerciseName returns a name unlikely to conflict with fixture data.
func testExerciseName(base string) string {
	return "Test_Repo_" + base
}

func newTestExercise(name string) domain.Exercise {
	return domain.Exercise{ //nolint:exhaustruct // DefaultStartingSeconds nil for non-time_based.
		Name:                  testExerciseName(name),
		Category:              domain.CategoryUpper,
		ExerciseType:          domain.ExerciseTypeWeighted,
		DescriptionMarkdown:   "# " + name,
		PrimaryMuscleGroups:   []string{"Chest"},
		SecondaryMuscleGroups: []string{"Triceps"},
		RepMin:                ptrInt(5),
		RepMax:                ptrInt(10),
	}
}

func TestExerciseRepository_GetMissingReturnsErrNotFound(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	_, err := repos.Exercises.Get(ctx, 999_999)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("want domain.ErrNotFound for missing exercise, got %v", err)
	}
}

func TestExerciseRepository_CreateAssignsID(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	created, err := repos.Exercises.Create(ctx, newTestExercise("Bench"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID <= 0 {
		t.Errorf("expected assigned positive ID, got %d", created.ID)
	}
}

func TestExerciseRepository_CreateThenGetRoundTrip(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	created, err := repos.Exercises.Create(ctx, newTestExercise("Bench"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repos.Exercises.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != testExerciseName("Bench") {
		t.Errorf("Name: want %q, got %q", testExerciseName("Bench"), got.Name)
	}
	if got.Category != domain.CategoryUpper {
		t.Errorf("Category: want %q, got %q", domain.CategoryUpper, got.Category)
	}
	if len(got.PrimaryMuscleGroups) != 1 || got.PrimaryMuscleGroups[0] != "Chest" {
		t.Errorf("PrimaryMuscleGroups: want [Chest], got %v", got.PrimaryMuscleGroups)
	}
	if len(got.SecondaryMuscleGroups) != 1 || got.SecondaryMuscleGroups[0] != "Triceps" {
		t.Errorf("SecondaryMuscleGroups: want [Triceps], got %v", got.SecondaryMuscleGroups)
	}
}

func TestExerciseRepository_UpdatePersistsChanges(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	created, err := repos.Exercises.Create(ctx, newTestExercise("Bench"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	updatedName := testExerciseName("Bench Updated")
	if err = repos.Exercises.Update(ctx, created.ID, func(ex *domain.Exercise) error {
		ex.Name = updatedName
		ex.PrimaryMuscleGroups = []string{"Chest", "Shoulders"}
		return nil
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err := repos.Exercises.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != updatedName {
		t.Errorf("Name after update: want %q, got %q", updatedName, got.Name)
	}
	if len(got.PrimaryMuscleGroups) != 2 {
		t.Errorf("PrimaryMuscleGroups after update: want 2, got %v", got.PrimaryMuscleGroups)
	}
}

func TestExerciseRepository_UpdateRollsBackOnError(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	created, err := repos.Exercises.Create(ctx, newTestExercise("Bench"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	wantErr := errors.New("user-injected failure")
	if err = repos.Exercises.Update(ctx, created.ID, func(ex *domain.Exercise) error {
		ex.Name = "MUTATED"
		return wantErr
	}); !errors.Is(err, wantErr) {
		t.Fatalf("Update: want injected error, got %v", err)
	}
	got, err := repos.Exercises.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != testExerciseName("Bench") {
		t.Errorf("expected rollback to preserve original name %q, got %q", testExerciseName("Bench"), got.Name)
	}
}
