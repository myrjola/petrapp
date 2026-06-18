package repository_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/myrjola/petrapp/internal/petra/domain"
)

// newTestExercise returns a fresh exercise with a name unlikely to conflict
// with fixture data.
func newTestExercise() domain.Exercise {
	return domain.Exercise{ //nolint:exhaustruct // DefaultStartingSeconds nil for non-time_based.
		Name:                  "Test_Repo_Bench",
		Category:              domain.CategoryUpper,
		ExerciseType:          domain.ExerciseTypeWeighted,
		Instructions:          []string{"Lie on the bench", "Press the bar up"},
		CommonMistakes:        []string{"Flaring the elbows"},
		Resources:             []domain.Resource{{Title: "Form guide", URL: "https://example.com/bench"}},
		PrimaryMuscleGroups:   []string{"Chest"},
		SecondaryMuscleGroups: []string{"Triceps"},
		RepMin:                new(5),
		RepMax:                new(10),
	}
}

func TestExerciseRepository_GetMissingReturnsErrNotFound(t *testing.T) {
	t.Parallel()

	ctx, repos := setupTestRepos(t)

	_, err := repos.Exercises.Get(ctx, 999_999)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("want domain.ErrNotFound for missing exercise, got %v", err)
	}
}

func TestExerciseRepository_CreateAssignsID(t *testing.T) {
	t.Parallel()

	ctx, repos := setupTestRepos(t)

	created, err := repos.Exercises.Create(ctx, newTestExercise())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID <= 0 {
		t.Errorf("expected assigned positive ID, got %d", created.ID)
	}
}

func TestExerciseRepository_CreateThenGetRoundTrip(t *testing.T) {
	t.Parallel()

	ctx, repos := setupTestRepos(t)

	created, err := repos.Exercises.Create(ctx, newTestExercise())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repos.Exercises.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "Test_Repo_Bench" {
		t.Errorf("Name: want %q, got %q", "Test_Repo_Bench", got.Name)
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
	if !slices.Equal(got.Instructions, []string{"Lie on the bench", "Press the bar up"}) {
		t.Errorf("Instructions round-trip: got %v", got.Instructions)
	}
	if !slices.Equal(got.CommonMistakes, []string{"Flaring the elbows"}) {
		t.Errorf("CommonMistakes round-trip: got %v", got.CommonMistakes)
	}
	if len(got.Resources) != 1 || got.Resources[0].Title != "Form guide" ||
		got.Resources[0].URL != "https://example.com/bench" {
		t.Errorf("Resources round-trip: got %v", got.Resources)
	}
}

func TestExerciseRepository_UpdatePersistsChanges(t *testing.T) {
	t.Parallel()

	ctx, repos := setupTestRepos(t)

	created, err := repos.Exercises.Create(ctx, newTestExercise())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	updatedName := "Test_Repo_Bench Updated"
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
	t.Parallel()

	ctx, repos := setupTestRepos(t)

	created, err := repos.Exercises.Create(ctx, newTestExercise())
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
	if got.Name != "Test_Repo_Bench" {
		t.Errorf("expected rollback to preserve original name %q, got %q", "Test_Repo_Bench", got.Name)
	}
}

func TestExerciseRepository_DeltTaxonomySeed(t *testing.T) {
	t.Parallel()

	ctx, repos := setupTestRepos(t)

	groups, err := repos.Exercises.ListMuscleGroups(ctx)
	if err != nil {
		t.Fatalf("ListMuscleGroups: %v", err)
	}
	if !slices.Contains(groups, "Side Delts") || !slices.Contains(groups, "Rear Delts") {
		t.Errorf("muscle groups missing delt heads: %v", groups)
	}
	if slices.Contains(groups, "Hip Flexors") {
		t.Errorf("Hip Flexors should be removed: %v", groups)
	}

	// Lateral Raise (5): side-delt prime mover, no longer generic Shoulders.
	lateralRaise, err := repos.Exercises.Get(ctx, 5)
	if err != nil {
		t.Fatalf("Get(5): %v", err)
	}
	if !slices.Contains(lateralRaise.PrimaryMuscleGroups, "Side Delts") ||
		slices.Contains(lateralRaise.PrimaryMuscleGroups, "Shoulders") {
		t.Errorf("Lateral Raise primaries = %v, want Side Delts and no Shoulders",
			lateralRaise.PrimaryMuscleGroups)
	}

	// Face Pull (34): rear-delt prime mover.
	facePull, err := repos.Exercises.Get(ctx, 34)
	if err != nil {
		t.Fatalf("Get(34): %v", err)
	}
	if !slices.Contains(facePull.PrimaryMuscleGroups, "Rear Delts") ||
		slices.Contains(facePull.PrimaryMuscleGroups, "Shoulders") {
		t.Errorf("Face Pull primaries = %v, want Rear Delts and no Shoulders",
			facePull.PrimaryMuscleGroups)
	}

	// Seated Cable Row (11): rear delts as a synergist.
	row, err := repos.Exercises.Get(ctx, 11)
	if err != nil {
		t.Fatalf("Get(11): %v", err)
	}
	if !slices.Contains(row.SecondaryMuscleGroups, "Rear Delts") {
		t.Errorf("Seated Cable Row secondaries = %v, want Rear Delts",
			row.SecondaryMuscleGroups)
	}

	// Hanging Leg Raise (39): Hip Flexors gone, Abs still prime mover.
	hlr, err := repos.Exercises.Get(ctx, 39)
	if err != nil {
		t.Fatalf("Get(39): %v", err)
	}
	if slices.Contains(hlr.PrimaryMuscleGroups, "Hip Flexors") ||
		slices.Contains(hlr.SecondaryMuscleGroups, "Hip Flexors") {
		t.Errorf("Hanging Leg Raise still references Hip Flexors: P=%v S=%v",
			hlr.PrimaryMuscleGroups, hlr.SecondaryMuscleGroups)
	}
	if !slices.Contains(hlr.PrimaryMuscleGroups, "Abs") {
		t.Errorf("Hanging Leg Raise primaries = %v, want Abs", hlr.PrimaryMuscleGroups)
	}
}
