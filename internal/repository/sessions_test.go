package repository_test

import (
	"errors"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
)

func newTestExerciseFor(t *testing.T) domain.Exercise {
	t.Helper()
	return domain.Exercise{ //nolint:exhaustruct // DefaultStartingSeconds nil for non-time_based.
		Name:                  "Test_Repo_Bench_Press_Sessions",
		Category:              domain.CategoryUpper,
		ExerciseType:          domain.ExerciseTypeWeighted,
		DescriptionMarkdown:   "# Test_Repo_Bench_Press_Sessions",
		PrimaryMuscleGroups:   []string{"Chest"},
		SecondaryMuscleGroups: []string{"Triceps"},
		RepMin:                new(5),
		RepMax:                new(10),
	}
}

func TestSessionRepository_GetMissingReturnsErrNotFound(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	missing := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := repos.Sessions.Get(ctx, missing)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("want domain.ErrNotFound, got %v", err)
	}
}

func TestSessionRepository_CreateBatchThenGetHydratesExercise(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	exercise, err := repos.Exercises.Create(ctx, newTestExerciseFor(t))
	if err != nil {
		t.Fatalf("Create exercise: %v", err)
	}

	date := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct // StartedAt/CompletedAt zero by design.
		Date:              date,
		PeriodizationType: domain.PeriodizationStrength,
		ExerciseSets: []domain.ExerciseSet{
			{ //nolint:exhaustruct // ID assigned by DB; WarmupCompletedAt nil.
				Exercise: exercise,
				Sets:     []domain.Set{{TargetValue: 5}}, //nolint:exhaustruct // Other fields nil.
			},
		},
	}
	if err = repos.Sessions.CreateBatch(ctx, []domain.Session{sess}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}

	got, err := repos.Sessions.Get(ctx, date)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.ExerciseSets) != 1 {
		t.Fatalf("want 1 ExerciseSet, got %d", len(got.ExerciseSets))
	}
	hydrated := got.ExerciseSets[0].Exercise
	if hydrated.ID != exercise.ID {
		t.Errorf("Exercise.ID: want %d, got %d", exercise.ID, hydrated.ID)
	}
	if hydrated.Name != "Test_Repo_Bench_Press_Sessions" {
		t.Errorf("Exercise.Name: want Test_Repo_Bench_Press_Sessions, got %q", hydrated.Name)
	}
	if len(hydrated.PrimaryMuscleGroups) != 1 || hydrated.PrimaryMuscleGroups[0] != "Chest" {
		t.Errorf("Exercise.PrimaryMuscleGroups: want [Chest], got %v", hydrated.PrimaryMuscleGroups)
	}
}

func TestSessionRepository_UpdatePreservesSlotID(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	exercise, err := repos.Exercises.Create(ctx, newTestExerciseFor(t))
	if err != nil {
		t.Fatalf("Create exercise: %v", err)
	}
	date := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct // StartedAt/CompletedAt zero.
		Date:              date,
		PeriodizationType: domain.PeriodizationStrength,
		ExerciseSets: []domain.ExerciseSet{
			{ //nolint:exhaustruct // ID assigned by DB.
				Exercise: exercise,
				Sets:     []domain.Set{{TargetValue: 5}}, //nolint:exhaustruct // Other fields nil.
			},
		},
	}
	if err = repos.Sessions.CreateBatch(ctx, []domain.Session{sess}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}

	fetched, err := repos.Sessions.Get(ctx, date)
	if err != nil {
		t.Fatalf("Get after create: %v", err)
	}
	originalSlotID := fetched.ExerciseSets[0].ID
	if originalSlotID == 0 {
		t.Fatalf("expected non-zero slot ID after insert")
	}

	if err = repos.Sessions.Update(ctx, date, func(s *domain.Session) error {
		s.StartedAt = time.Now().UTC()
		return nil
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	after, err := repos.Sessions.Get(ctx, date)
	if err != nil {
		t.Fatalf("Get after Update: %v", err)
	}
	if len(after.ExerciseSets) != 1 {
		t.Fatalf("want 1 slot after Update, got %d", len(after.ExerciseSets))
	}
	if after.ExerciseSets[0].ID != originalSlotID {
		t.Errorf("slot ID changed across Update: %d → %d", originalSlotID, after.ExerciseSets[0].ID)
	}
	if after.StartedAt.IsZero() {
		t.Errorf("expected StartedAt to be set after Update closure")
	}
}

func TestSessionRepository_UpdateRollsBackOnError(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	exercise, err := repos.Exercises.Create(ctx, newTestExerciseFor(t))
	if err != nil {
		t.Fatalf("Create exercise: %v", err)
	}
	date := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct // StartedAt/CompletedAt zero.
		Date:              date,
		PeriodizationType: domain.PeriodizationStrength,
		ExerciseSets: []domain.ExerciseSet{
			{ //nolint:exhaustruct // ID assigned by DB.
				Exercise: exercise,
				Sets:     []domain.Set{{TargetValue: 5}}, //nolint:exhaustruct // Other fields nil.
			},
		},
	}
	if err = repos.Sessions.CreateBatch(ctx, []domain.Session{sess}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}

	wantErr := errors.New("user-injected failure")
	if err = repos.Sessions.Update(ctx, date, func(s *domain.Session) error {
		s.StartedAt = time.Now().UTC()
		return wantErr
	}); !errors.Is(err, wantErr) {
		t.Fatalf("Update: want injected error, got %v", err)
	}

	after, err := repos.Sessions.Get(ctx, date)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !after.StartedAt.IsZero() {
		t.Errorf("expected rollback to leave StartedAt zero, got %v", after.StartedAt)
	}
}

func TestSessionRepository_UpdatePropagatesDomainSentinel(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	exercise, err := repos.Exercises.Create(ctx, newTestExerciseFor(t))
	if err != nil {
		t.Fatalf("Create exercise: %v", err)
	}
	date := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)
	now := time.Now().UTC()
	sess := domain.Session{ //nolint:exhaustruct // CompletedAt zero.
		Date:              date,
		PeriodizationType: domain.PeriodizationStrength,
		StartedAt:         now,
		ExerciseSets: []domain.ExerciseSet{
			{ //nolint:exhaustruct // ID assigned by DB.
				Exercise: exercise,
				Sets:     []domain.Set{{TargetValue: 5}}, //nolint:exhaustruct // Other fields nil.
			},
		},
	}
	if err = repos.Sessions.CreateBatch(ctx, []domain.Session{sess}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}

	err = repos.Sessions.Update(ctx, date, func(s *domain.Session) error {
		return s.Start(time.Now().UTC()) // already started: returns ErrAlreadyStarted
	})
	if !errors.Is(err, domain.ErrAlreadyStarted) {
		t.Errorf("want domain.ErrAlreadyStarted to propagate, got %v", err)
	}
}

func TestSessionRepository_DeleteWeek(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	monday := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)
	exercise, err := repos.Exercises.Create(ctx, newTestExerciseFor(t))
	if err != nil {
		t.Fatalf("Create exercise: %v", err)
	}
	mkSession := func(day time.Time) domain.Session {
		return domain.Session{ //nolint:exhaustruct // StartedAt/CompletedAt zero.
			Date:              day,
			PeriodizationType: domain.PeriodizationStrength,
			ExerciseSets: []domain.ExerciseSet{
				{ //nolint:exhaustruct // ID assigned by DB.
					Exercise: exercise,
					Sets:     []domain.Set{{TargetValue: 5}}, //nolint:exhaustruct // Other fields nil.
				},
			},
		}
	}
	if err = repos.Sessions.CreateBatch(ctx, []domain.Session{
		mkSession(monday),
		mkSession(monday.AddDate(0, 0, 2)),
		mkSession(monday.AddDate(0, 0, 4)),
	}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}

	if err = repos.Sessions.DeleteWeek(ctx, monday); err != nil {
		t.Fatalf("DeleteWeek: %v", err)
	}

	for _, day := range []time.Time{monday, monday.AddDate(0, 0, 2), monday.AddDate(0, 0, 4)} {
		_, err = repos.Sessions.Get(ctx, day)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("Get %s after DeleteWeek: want ErrNotFound, got %v", day.Format(time.DateOnly), err)
		}
	}
}
