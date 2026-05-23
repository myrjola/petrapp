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
	t.Parallel()

	ctx, repos := setupTestRepos(t)

	missing := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := repos.Sessions.Get(ctx, missing)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("want domain.ErrNotFound, got %v", err)
	}
}

func TestSessionRepository_CreateBatchThenGetHydratesExercise(t *testing.T) {
	t.Parallel()

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

func TestSessionRepository_ListHydratesEverySession(t *testing.T) {
	t.Parallel()

	ctx, repos := setupTestRepos(t)

	exercise, err := repos.Exercises.Create(ctx, newTestExerciseFor(t))
	if err != nil {
		t.Fatalf("Create exercise: %v", err)
	}
	earlier := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)
	later := time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC)
	mkSession := func(day time.Time) domain.Session {
		return domain.Session{ //nolint:exhaustruct // StartedAt/CompletedAt zero.
			Date:              day,
			PeriodizationType: domain.PeriodizationStrength,
			ExerciseSets: []domain.ExerciseSet{
				{ //nolint:exhaustruct // ID assigned by DB; WarmupCompletedAt nil.
					Exercise: exercise,
					Sets:     []domain.Set{{TargetValue: 5}}, //nolint:exhaustruct // Other fields nil.
				},
			},
		}
	}
	if err = repos.Sessions.CreateBatch(ctx, []domain.Session{
		mkSession(earlier), mkSession(later),
	}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}

	got, err := repos.Sessions.List(ctx, earlier)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 sessions, got %d", len(got))
	}
	// Newest first.
	if !got[0].Date.Equal(later) || !got[1].Date.Equal(earlier) {
		t.Fatalf("want dates [%s, %s], got [%s, %s]",
			later.Format(time.DateOnly), earlier.Format(time.DateOnly),
			got[0].Date.Format(time.DateOnly), got[1].Date.Format(time.DateOnly))
	}
	// The batched query must hydrate every session, not just the first.
	for _, sess := range got {
		label := sess.Date.Format(time.DateOnly)
		if len(sess.ExerciseSets) != 1 {
			t.Fatalf("session %s: want 1 ExerciseSet, got %d", label, len(sess.ExerciseSets))
		}
		ex := sess.ExerciseSets[0].Exercise
		if ex.ID != exercise.ID || ex.Name != exercise.Name {
			t.Errorf("session %s: exercise not hydrated: %+v", label, ex)
		}
		if len(ex.PrimaryMuscleGroups) != 1 || ex.PrimaryMuscleGroups[0] != "Chest" {
			t.Errorf("session %s: PrimaryMuscleGroups = %v, want [Chest]", label, ex.PrimaryMuscleGroups)
		}
		if len(sess.ExerciseSets[0].Sets) != 1 || sess.ExerciseSets[0].Sets[0].TargetValue != 5 {
			t.Errorf("session %s: sets not hydrated: %+v", label, sess.ExerciseSets[0].Sets)
		}
	}
}

func TestSessionRepository_UpdatePreservesSlotID(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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

func TestSessionRepository_RoundTripIsDeload(t *testing.T) {
	t.Parallel()

	ctx, repos := setupTestRepos(t)

	date := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct // only IsDeload round-trip is exercised
		Date:              date,
		PeriodizationType: domain.PeriodizationHypertrophy,
		IsDeload:          true,
	}
	if err := repos.Sessions.CreateBatch(ctx, []domain.Session{sess}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}
	got, err := repos.Sessions.Get(ctx, date)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.IsDeload {
		t.Error("IsDeload = false, want true")
	}
}

func TestSessionRepository_StartingWeight_SkipsDeloadSessions(t *testing.T) {
	t.Parallel()

	ctx, repos := setupTestRepos(t)

	exercise, err := repos.Exercises.Create(ctx, newTestExerciseFor(t))
	if err != nil {
		t.Fatalf("Create exercise: %v", err)
	}

	mondayNormal := time.Date(2026, time.April, 27, 0, 0, 0, 0, time.UTC)
	mondayDeload := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)

	weight100 := 100.0
	weight90 := 90.0
	onTarget := domain.SignalOnTarget
	completedAt := time.Date(2026, time.April, 27, 10, 0, 0, 0, time.UTC)

	normal := domain.Session{ //nolint:exhaustruct // only fields relevant to deload skip test
		Date:              mondayNormal,
		PeriodizationType: domain.PeriodizationHypertrophy,
		IsDeload:          false,
		ExerciseSets: []domain.ExerciseSet{
			{ //nolint:exhaustruct // ID and WarmupCompletedAt not needed for round-trip test
				Exercise: exercise,
				Sets: []domain.Set{
					{
						TargetValue:    10,
						WeightKg:       &weight100,
						CompletedValue: new(10),
						CompletedAt:    &completedAt,
						Signal:         &onTarget,
					},
				},
			},
		},
	}
	deload := domain.Session{ //nolint:exhaustruct // only fields relevant to deload skip test
		Date:              mondayDeload,
		PeriodizationType: domain.PeriodizationHypertrophy,
		IsDeload:          true,
		ExerciseSets: []domain.ExerciseSet{
			{ //nolint:exhaustruct // ID and WarmupCompletedAt not needed for round-trip test
				Exercise: exercise,
				Sets: []domain.Set{
					{
						TargetValue:    10,
						WeightKg:       &weight90,
						CompletedValue: new(10),
						CompletedAt:    &completedAt,
						Signal:         &onTarget,
					},
				},
			},
		},
	}

	if err = repos.Sessions.CreateBatch(ctx, []domain.Session{normal, deload}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}

	got, err := repos.Sessions.GetLatestStartingWeightBefore(ctx, exercise.ID, mondayDeload.AddDate(0, 0, 7))
	if err != nil {
		t.Fatalf("GetLatestStartingWeightBefore: %v", err)
	}
	if got.WeightKg != 100.0 {
		t.Errorf("WeightKg = %v, want 100.0 (deload session must be excluded)", got.WeightKg)
	}
}

func TestSessionRepository_Create_InsertsSingleSession(t *testing.T) {
	t.Parallel()

	ctx, repos := setupTestRepos(t)

	ex, err := repos.Exercises.Create(ctx, newTestExerciseFor(t))
	if err != nil {
		t.Fatalf("create exercise: %v", err)
	}

	date := time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC) // a Wednesday
	sess := domain.Session{                             //nolint:exhaustruct // StartedAt/CompletedAt zero on insert.
		Date:              date,
		PeriodizationType: domain.PeriodizationStrength,
		IsDeload:          false,
		ExerciseSets: []domain.ExerciseSet{
			{ //nolint:exhaustruct // ID assigned on insert, WarmupCompletedAt nil.
				Exercise: ex,
				Sets: []domain.Set{
					{TargetValue: 5}, //nolint:exhaustruct // all completion fields nil.
				},
			},
		},
	}

	if err = repos.Sessions.Create(ctx, sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repos.Sessions.Get(ctx, date)
	if err != nil {
		t.Fatalf("Get after Create: %v", err)
	}
	if len(got.ExerciseSets) != 1 || got.ExerciseSets[0].Exercise.ID != ex.ID {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if got.PeriodizationType != domain.PeriodizationStrength {
		t.Errorf("PeriodizationType = %s, want strength", got.PeriodizationType)
	}
}

func TestSessionRepository_Create_ConflictReturnsErrAlreadyExists(t *testing.T) {
	t.Parallel()

	ctx, repos := setupTestRepos(t)

	ex, err := repos.Exercises.Create(ctx, newTestExerciseFor(t))
	if err != nil {
		t.Fatalf("create exercise: %v", err)
	}

	date := time.Date(2026, 1, 8, 0, 0, 0, 0, time.UTC) // a Thursday — distinct from the round-trip test
	sess := domain.Session{                             //nolint:exhaustruct // StartedAt/CompletedAt zero on insert.
		Date:              date,
		PeriodizationType: domain.PeriodizationStrength,
		ExerciseSets: []domain.ExerciseSet{
			{ //nolint:exhaustruct // ID assigned on insert, WarmupCompletedAt nil.
				Exercise: ex,
				Sets:     []domain.Set{{TargetValue: 5}}, //nolint:exhaustruct // completion fields nil.
			},
		},
	}
	if err = repos.Sessions.Create(ctx, sess); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	err = repos.Sessions.Create(ctx, sess)
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Errorf("second Create err = %v, want wraps domain.ErrAlreadyExists", err)
	}
}

func TestSessionRepository_CreateBatch_ConflictReturnsErrAlreadyExists(t *testing.T) {
	t.Parallel()

	ctx, repos := setupTestRepos(t)

	ex, err := repos.Exercises.Create(ctx, newTestExerciseFor(t))
	if err != nil {
		t.Fatalf("create exercise: %v", err)
	}

	date := time.Date(2026, 1, 9, 0, 0, 0, 0, time.UTC) // a Friday, distinct from other Create tests
	sess := domain.Session{                             //nolint:exhaustruct // completion fields nil on insert.
		Date:              date,
		PeriodizationType: domain.PeriodizationStrength,
		ExerciseSets: []domain.ExerciseSet{
			{ //nolint:exhaustruct // ID assigned on insert, WarmupCompletedAt nil.
				Exercise: ex,
				Sets:     []domain.Set{{TargetValue: 5}}, //nolint:exhaustruct // completion fields nil.
			},
		},
	}

	if err = repos.Sessions.CreateBatch(ctx, []domain.Session{sess}); err != nil {
		t.Fatalf("first CreateBatch: %v", err)
	}

	err = repos.Sessions.CreateBatch(ctx, []domain.Session{sess})
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Errorf("second CreateBatch err = %v, want wraps domain.ErrAlreadyExists", err)
	}
}

func TestSessionRepository_DeleteWeek(t *testing.T) {
	t.Parallel()

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

func TestGetLatestSuccessfulSecondsBefore_NoRows_ReturnsNotFound(t *testing.T) {
	t.Parallel()
	ctx, repos := setupTestRepos(t)

	// No exercise_sets seeded for exercise 99999 → must surface ErrNotFound.
	_, err := repos.Sessions.GetLatestSuccessfulSecondsBefore(ctx, 99999, time.Now())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("err = %v, want domain.ErrNotFound", err)
	}
}
