package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/service"
	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

func Test_GetStartingWeight(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("sw-user"), "SW User").Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO exercises (name, category, description_markdown, rep_min, rep_max) VALUES (?, ?, ?, ?, ?)",
		"Squat", "lower", "desc", 5, 8)
	if err != nil {
		t.Fatalf("insert exercise: %v", err)
	}
	var exerciseID int
	err = db.ReadOnly.QueryRowContext(ctx, "SELECT id FROM exercises WHERE name = 'Squat'").Scan(&exerciseID)
	if err != nil {
		t.Fatalf("get exercise id: %v", err)
	}

	svc := service.NewService(db, logger, "")

	today := time.Now()

	// No history: expect 0.
	got, err := svc.GetStartingWeight(ctx, exerciseID, today, domain.PeriodizationStrength)
	if err != nil {
		t.Fatalf("GetStartingWeight no history: %v", err)
	}
	if got != 0 {
		t.Errorf("no history: want 0, got %v", got)
	}

	// Insert a completed strength session 7 days ago. Set 1 ramps up from 95kg
	// (too_light), set 2 lands on 100kg (on_target), set 3 fails at 105kg
	// (too_heavy). The latest *successful* set is set 2 at 100kg.
	dateStr := today.AddDate(0, 0, -7).Format("2006-01-02")
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_sessions (user_id, workout_date, completed_at, periodization_type)
		 VALUES (?, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ'), 'strength')`,
		userID, dateStr)
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_exercises (workout_user_id, workout_date, position, exercise_id) VALUES (?, ?, 0, ?)`,
		userID, dateStr, exerciseID)
	if err != nil {
		t.Fatalf("insert workout_exercises: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_user_id, workout_date, position, set_number,
		 weight_kg, target_value, completed_value, signal)
		 VALUES (?, ?, 0, 1, 95.0, 5, 5, 'too_light'),
		        (?, ?, 0, 2, 100.0, 5, 5, 'on_target'),
		        (?, ?, 0, 3, 105.0, 5, 3, 'too_heavy')`,
		userID, dateStr, userID, dateStr, userID, dateStr)
	if err != nil {
		t.Fatalf("insert sets: %v", err)
	}

	// Same periodization (strength → strength): the latest successful set (set 2 at
	// 100kg) carries over unchanged, ignoring the failed set 3.
	got, err = svc.GetStartingWeight(ctx, exerciseID, today, domain.PeriodizationStrength)
	if err != nil {
		t.Fatalf("GetStartingWeight with history: %v", err)
	}
	if got != 100.0 {
		t.Errorf("strength → strength: want 100.0, got %v", got)
	}

	// Cross-periodization (strength 5 reps → hypertrophy 8 reps): Epley conversion
	// 100 * (1 + 5/30) / (1 + 8/30) ≈ 92.1, rounded to 0.5 = 92.0.
	got, err = svc.GetStartingWeight(ctx, exerciseID, today, domain.PeriodizationHypertrophy)
	if err != nil {
		t.Fatalf("GetStartingWeight cross-periodization: %v", err)
	}
	if got != 92.0 {
		t.Errorf("strength → hypertrophy: want 92.0, got %v", got)
	}

	// Insert today's session with different set weights. The starting weight must
	// remain anchored to the historical session, regardless of today's sets.
	todayStr := today.Format("2006-01-02")
	_, err = db.ReadWrite.ExecContext(
		ctx,
		"INSERT INTO workout_sessions (user_id, workout_date, started_at) VALUES (?, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ'))",
		userID,
		todayStr,
	)
	if err != nil {
		t.Fatalf("insert today's session: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_exercises (workout_user_id, workout_date, position, exercise_id) VALUES (?, ?, 0, ?)`,
		userID, todayStr, exerciseID)
	if err != nil {
		t.Fatalf("insert today's workout_exercises: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_user_id, workout_date, position, set_number,
		 weight_kg, target_value, completed_value, signal)
		 VALUES (?, ?, 0, 1, 75.0, 5, 5, 'too_light'),
		        (?, ?, 0, 2, 80.0, 5, 5, 'on_target')`,
		userID, todayStr, userID, todayStr)
	if err != nil {
		t.Fatalf("insert today's sets: %v", err)
	}

	got, err = svc.GetStartingWeight(ctx, exerciseID, today, domain.PeriodizationStrength)
	if err != nil {
		t.Fatalf("GetStartingWeight ignoring today: %v", err)
	}
	if got != 100.0 {
		t.Errorf("today ignored: want 100.0, got %v", got)
	}

	// Insert a more recent strength session 3 days ago where every set was
	// too_heavy. GetStartingWeight must skip it and fall back to the 7-days-ago
	// session's latest successful set (100kg).
	failDateStr := today.AddDate(0, 0, -3).Format("2006-01-02")
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_sessions (user_id, workout_date, completed_at, periodization_type)
		 VALUES (?, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ'), 'strength')`,
		userID, failDateStr)
	if err != nil {
		t.Fatalf("insert fail session: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_exercises (workout_user_id, workout_date, position, exercise_id) VALUES (?, ?, 0, ?)`,
		userID, failDateStr, exerciseID)
	if err != nil {
		t.Fatalf("insert fail workout_exercises: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_user_id, workout_date, position, set_number,
		 weight_kg, target_value, completed_value, signal)
		 VALUES (?, ?, 0, 1, 110.0, 5, 3, 'too_heavy'),
		        (?, ?, 0, 2, 110.0, 5, 2, 'too_heavy')`,
		userID, failDateStr, userID, failDateStr)
	if err != nil {
		t.Fatalf("insert fail sets: %v", err)
	}

	got, err = svc.GetStartingWeight(ctx, exerciseID, today, domain.PeriodizationStrength)
	if err != nil {
		t.Fatalf("GetStartingWeight fallback: %v", err)
	}
	if got != 100.0 {
		t.Errorf("fallback past too_heavy session: want 100.0, got %v", got)
	}
}

// Test_GetStartingWeight_Assisted covers the assisted-exercise (negative weight)
// flow across periodization changes: an on-target -50 kg x5 strength set must
// translate into a more negative weight when the next session is hypertrophy
// (8 reps), since more reps require more machine assistance for the same
// relative intensity.
func Test_GetStartingWeight_Assisted(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("sw-assisted-user"), "SW Assisted User").Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO exercises (name, category, description_markdown, rep_min, rep_max) VALUES (?, ?, ?, ?, ?)",
		"Assisted Test Exercise", "upper", "desc", 5, 8)
	if err != nil {
		t.Fatalf("insert exercise: %v", err)
	}
	var exerciseID int
	err = db.ReadOnly.QueryRowContext(ctx,
		"SELECT id FROM exercises WHERE name = 'Assisted Test Exercise'").Scan(&exerciseID)
	if err != nil {
		t.Fatalf("get exercise id: %v", err)
	}

	svc := service.NewService(db, logger, "")

	today := time.Now()

	// Insert a completed strength session 7 days ago at -50 kg x5, on target.
	dateStr := today.AddDate(0, 0, -7).Format("2006-01-02")
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_sessions (user_id, workout_date, completed_at, periodization_type)
		 VALUES (?, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ'), 'strength')`,
		userID, dateStr)
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_exercises (workout_user_id, workout_date, position, exercise_id) VALUES (?, ?, 0, ?)`,
		userID, dateStr, exerciseID)
	if err != nil {
		t.Fatalf("insert workout_exercises: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_user_id, workout_date, position, set_number,
		 weight_kg, target_value, completed_value, signal)
		 VALUES (?, ?, 0, 1, -50.0, 5, 5, 'on_target')`,
		userID, dateStr)
	if err != nil {
		t.Fatalf("insert sets: %v", err)
	}

	// Same periodization: -50 kg carries over unchanged.
	got, err := svc.GetStartingWeight(ctx, exerciseID, today, domain.PeriodizationStrength)
	if err != nil {
		t.Fatalf("GetStartingWeight strength→strength: %v", err)
	}
	if got != -50.0 {
		t.Errorf("assisted strength → strength: want -50.0, got %v", got)
	}

	// Cross-periodization (strength 5 reps → hypertrophy 8 reps): more reps
	// require more assistance, so the recommendation must be more negative.
	// -50 * (1 + 8/30) / (1 + 5/30) ≈ -54.29 → snaps to -54.5.
	got, err = svc.GetStartingWeight(ctx, exerciseID, today, domain.PeriodizationHypertrophy)
	if err != nil {
		t.Fatalf("GetStartingWeight strength→hypertrophy: %v", err)
	}
	if got != -54.5 {
		t.Errorf("assisted strength → hypertrophy: want -54.5, got %v", got)
	}
}

func Test_GetStartingSeconds(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var userID int
	if err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("ts-user"), "TS User").Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	// Insert a time_based exercise with default 30s.
	if _, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO exercises (name, category, exercise_type, default_starting_seconds, description_markdown)
		VALUES (?, ?, ?, ?, ?)`,
		"Test Plank", "upper", "time_based", 30, ""); err != nil {
		t.Fatalf("insert exercise: %v", err)
	}
	var exerciseID int
	if err = db.ReadOnly.QueryRowContext(ctx,
		"SELECT id FROM exercises WHERE name = 'Test Plank'").Scan(&exerciseID); err != nil {
		t.Fatalf("get exercise id: %v", err)
	}

	svc := service.NewService(db, logger, "")
	today := time.Now()

	// Case 1: no history → fallback to default_starting_seconds.
	got, err := svc.GetStartingSeconds(ctx, exerciseID, today)
	if err != nil {
		t.Fatalf("no history: %v", err)
	}
	if got != 30 {
		t.Errorf("no history: want 30, got %d", got)
	}

	// Case 2: seed a successful session 2 days ago.
	twoDaysAgo := today.AddDate(0, 0, -2).Format("2006-01-02")
	if _, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_sessions (user_id, workout_date, periodization_type)
		VALUES (?, ?, 'strength')`, userID, twoDaysAgo); err != nil {
		t.Fatalf("insert session 1: %v", err)
	}
	if _, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_exercises (workout_user_id, workout_date, position, exercise_id)
		VALUES (?, ?, 0, ?)`,
		userID, twoDaysAgo, exerciseID); err != nil {
		t.Fatalf("insert workout_exercises 1: %v", err)
	}
	if _, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO exercise_sets
			(workout_user_id, workout_date, position, set_number, target_value, completed_value, completed_at, signal)
		VALUES (?, ?, 0, 1, 40, 40, '2026-05-05T12:00:00.000Z', 'on_target')`,
		userID, twoDaysAgo); err != nil {
		t.Fatalf("insert set 1: %v", err)
	}

	got, err = svc.GetStartingSeconds(ctx, exerciseID, today)
	if err != nil {
		t.Fatalf("with history: %v", err)
	}
	if got != 40 {
		t.Errorf("with history: want 40, got %d", got)
	}

	// Case 3: more recent too_heavy session should be skipped.
	oneDayAgo := today.AddDate(0, 0, -1).Format("2006-01-02")
	if _, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_sessions (user_id, workout_date, periodization_type)
		VALUES (?, ?, 'strength')`, userID, oneDayAgo); err != nil {
		t.Fatalf("insert session 2: %v", err)
	}
	if _, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_exercises (workout_user_id, workout_date, position, exercise_id)
		VALUES (?, ?, 0, ?)`,
		userID, oneDayAgo, exerciseID); err != nil {
		t.Fatalf("insert workout_exercises 2: %v", err)
	}
	if _, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO exercise_sets
			(workout_user_id, workout_date, position, set_number, target_value, completed_value, completed_at, signal)
		VALUES (?, ?, 0, 1, 50, 50, '2026-05-06T12:00:00.000Z', 'too_heavy')`,
		userID, oneDayAgo); err != nil {
		t.Fatalf("insert set 2: %v", err)
	}

	got, err = svc.GetStartingSeconds(ctx, exerciseID, today)
	if err != nil {
		t.Fatalf("skip too_heavy: %v", err)
	}
	if got != 40 {
		t.Errorf("skip too_heavy: want 40 (older successful), got %d", got)
	}
}

func Test_BuildTimedProgression(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var userID int
	if err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("btp-user"), "BTP User").Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	if _, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO exercises (name, category, exercise_type, default_starting_seconds, description_markdown)
		VALUES (?, ?, ?, ?, ?)`,
		"Test Plank BTP", "upper", "time_based", 30, ""); err != nil {
		t.Fatalf("insert exercise: %v", err)
	}
	var exerciseID int
	if err = db.ReadOnly.QueryRowContext(ctx,
		"SELECT id FROM exercises WHERE name = 'Test Plank BTP'").Scan(&exerciseID); err != nil {
		t.Fatalf("get exercise id: %v", err)
	}

	svc := service.NewService(db, logger, "")

	today := time.Now().Format("2006-01-02")
	todayTime, _ := time.Parse("2006-01-02", today)

	// Seed today's session with the exercise but no completed sets yet.
	if _, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_sessions (user_id, workout_date, periodization_type)
		VALUES (?, ?, 'strength')`, userID, today); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	const pos = 0
	if _, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_exercises (workout_user_id, workout_date, position, exercise_id)
		VALUES (?, ?, ?, ?)`,
		userID, today, pos, exerciseID); err != nil {
		t.Fatalf("insert workout_exercises: %v", err)
	}
	// Seed three planned sets with target_value=30, no completion yet.
	for i := 1; i <= 3; i++ {
		if _, err = db.ReadWrite.ExecContext(ctx, `
			INSERT INTO exercise_sets (workout_user_id, workout_date, position, set_number, target_value)
			VALUES (?, ?, ?, ?, 30)`, userID, today, pos, i); err != nil {
			t.Fatalf("insert set %d: %v", i, err)
		}
	}

	// Case 1: no completed sets in this session → first set returns starting seconds (default 30).
	progression, err := svc.BuildTimedProgression(ctx, todayTime, exerciseID)
	if err != nil {
		t.Fatalf("BuildTimedProgression no completion: %v", err)
	}
	if got := progression.CurrentSet().TargetSeconds; got != 30 {
		t.Errorf("first set: got %d, want 30 (default)", got)
	}
	if got := progression.SetsCompleted(); got != 0 {
		t.Errorf("first set: SetsCompleted = %d, want 0", got)
	}

	// Case 2: complete set 1 with too_light → second set should be 35s.
	if _, err = db.ReadWrite.ExecContext(ctx, `
		UPDATE exercise_sets
		SET completed_value = 30, signal = 'too_light'
		WHERE workout_user_id = ? AND workout_date = ? AND position = ? AND set_number = 1`,
		userID, today, pos); err != nil {
		t.Fatalf("update set 1: %v", err)
	}

	progression, err = svc.BuildTimedProgression(ctx, todayTime, exerciseID)
	if err != nil {
		t.Fatalf("BuildTimedProgression after set 1: %v", err)
	}
	if got := progression.CurrentSet().TargetSeconds; got != 35 {
		t.Errorf("after too_light: got %d, want 35", got)
	}
	if got := progression.SetsCompleted(); got != 1 {
		t.Errorf("after set 1: SetsCompleted = %d, want 1", got)
	}
}

func Test_BuildProgression(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("bp-user"), "BP User").Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO exercises (name, category, description_markdown, rep_min, rep_max) VALUES (?, ?, ?, ?, ?)",
		"OHP", "upper", "desc", 5, 8)
	if err != nil {
		t.Fatalf("insert exercise: %v", err)
	}
	var exerciseID int
	err = db.ReadOnly.QueryRowContext(ctx, "SELECT id FROM exercises WHERE name = 'OHP'").Scan(&exerciseID)
	if err != nil {
		t.Fatalf("get exercise id: %v", err)
	}

	today := time.Now().Format("2006-01-02")
	// Hypertrophy session (1 completed before this one).
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_sessions (user_id, workout_date, started_at, periodization_type)
		 VALUES (?, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ'), 'hypertrophy')`,
		userID, today)
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}
	const pos = 0
	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO workout_exercises (workout_user_id, workout_date, position, exercise_id) VALUES (?, ?, ?, ?)",
		userID, today, pos, exerciseID)
	if err != nil {
		t.Fatalf("insert workout_exercises: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_user_id, workout_date, position, set_number, weight_kg, target_value)
		 VALUES (?, ?, ?, 1, 40.0, 8), (?, ?, ?, 2, 40.0, 8), (?, ?, ?, 3, 40.0, 8)`,
		userID, today, pos, userID, today, pos, userID, today, pos)
	if err != nil {
		t.Fatalf("insert sets: %v", err)
	}

	svc := service.NewService(db, logger, "")
	date, _ := time.Parse("2006-01-02", today)

	// No history: starting weight 0, target 8 reps (hypertrophy).
	prog, err := svc.BuildProgression(ctx, date, exerciseID)
	if err != nil {
		t.Fatalf("BuildProgression: %v", err)
	}
	target := prog.CurrentSet()
	if target.WeightKg != 0 {
		t.Errorf("first set weight: want 0, got %v", target.WeightKg)
	}
	if target.TargetReps != 8 {
		t.Errorf("first set reps: want 8, got %v", target.TargetReps)
	}

	// Record set 0 as TooLight at 0kg.
	weight := 0.0
	sig := domain.SignalTooLight
	if err = svc.RecordSet(ctx, date, pos, 0, &sig, &weight, 8); err != nil {
		t.Fatalf("RecordSet: %v", err)
	}

	// Rebuild: next set should be 0 + 1 = 1 kg (1kg increment in dumbbell range).
	prog, err = svc.BuildProgression(ctx, date, exerciseID)
	if err != nil {
		t.Fatalf("BuildProgression after set 1: %v", err)
	}
	target = prog.CurrentSet()
	if target.WeightKg != 1.0 {
		t.Errorf("second set weight: want 1.0, got %v", target.WeightKg)
	}
}

func Test_BuildProgression_CrossPeriodizationConversion(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("bp-x-user"), "BPX User").Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO exercises (name, category, description_markdown, rep_min, rep_max) VALUES (?, ?, ?, ?, ?)",
		"Squat", "lower", "desc", 5, 8)
	if err != nil {
		t.Fatalf("insert exercise: %v", err)
	}
	var exerciseID int
	err = db.ReadOnly.QueryRowContext(ctx, "SELECT id FROM exercises WHERE name = 'Squat'").Scan(&exerciseID)
	if err != nil {
		t.Fatalf("get exercise id: %v", err)
	}

	// Prior strength session 7 days ago: completed first set 100 kg x 5 on target.
	prevStr := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_sessions (user_id, workout_date, completed_at, periodization_type)
		 VALUES (?, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ'), 'strength')`,
		userID, prevStr)
	if err != nil {
		t.Fatalf("insert prev session: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO workout_exercises (workout_user_id, workout_date, position, exercise_id) VALUES (?, ?, 0, ?)",
		userID, prevStr, exerciseID)
	if err != nil {
		t.Fatalf("insert prev workout_exercises: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_user_id, workout_date, position, set_number,
		 weight_kg, target_value, completed_value, signal)
		 VALUES (?, ?, 0, 1, 100.0, 5, 5, 'on_target')`,
		userID, prevStr)
	if err != nil {
		t.Fatalf("insert prev set: %v", err)
	}

	// New hypertrophy session today.
	todayStr := time.Now().Format("2006-01-02")
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_sessions (user_id, workout_date, started_at, periodization_type)
		 VALUES (?, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ'), 'hypertrophy')`,
		userID, todayStr)
	if err != nil {
		t.Fatalf("insert today session: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO workout_exercises (workout_user_id, workout_date, position, exercise_id) VALUES (?, ?, 0, ?)",
		userID, todayStr, exerciseID)
	if err != nil {
		t.Fatalf("insert workout_exercises: %v", err)
	}

	svc := service.NewService(db, logger, "")
	date, _ := time.Parse("2006-01-02", todayStr)

	prog, err := svc.BuildProgression(ctx, date, exerciseID)
	if err != nil {
		t.Fatalf("BuildProgression: %v", err)
	}
	target := prog.CurrentSet()
	// Strength 100kg x5 → Hypertrophy 8 reps via Epley:
	// 100 * (1 + 5/30) / (1 + 8/30) ≈ 92.105, rounded to 0.5 = 92.0.
	if target.WeightKg != 92.0 {
		t.Errorf("first set weight: want 92.0, got %v", target.WeightKg)
	}
	if target.TargetReps != 8 {
		t.Errorf("first set reps: want 8, got %v", target.TargetReps)
	}
}

// Test_BuildProgression_CurrentSetUsesDeriveScheme is a regression test for the bug
// where Progression.CurrentSet() returned TargetReps from the legacy TargetReps()
// function (hardcoded 5/8/15) rather than from DeriveScheme on the exercise's
// per-session rep window. A Deadlift (rep_min=3, rep_max=6) on a hypertrophy
// session must produce CurrentSet().TargetReps == 6 (repMax), not 8 (the old
// hypertrophy constant). Before this fix the workout UI displayed "8 reps" even
// though the planner had persisted target_value=6.
func Test_GetStartingWeight_DeloadAppliesNinetyPercent(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("deload-user"), "Deload User").Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO exercises (name, category, description_markdown, rep_min, rep_max) VALUES (?, ?, ?, ?, ?)",
		"Deload Test Press", "upper", "desc", 5, 8)
	if err != nil {
		t.Fatalf("insert exercise: %v", err)
	}
	var exerciseID int
	err = db.ReadOnly.QueryRowContext(ctx, "SELECT id FROM exercises WHERE name = 'Deload Test Press'").
		Scan(&exerciseID)
	if err != nil {
		t.Fatalf("get exercise id: %v", err)
	}

	svc := service.NewService(db, logger, "")

	monday := time.Date(2026, time.April, 27, 0, 0, 0, 0, time.UTC)
	deloadMonday := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	mondayStr := monday.Format("2006-01-02")

	// Insert a completed hypertrophy session on monday with 80 kg on_target.
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_sessions (user_id, workout_date, completed_at, periodization_type)
		 VALUES (?, ?, '2026-04-27T10:00:00.000Z', 'hypertrophy')`,
		userID, mondayStr)
	if err != nil {
		t.Fatalf("insert hypertrophy session: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_exercises (workout_user_id, workout_date, position, exercise_id) VALUES (?, ?, 0, ?)`,
		userID, mondayStr, exerciseID)
	if err != nil {
		t.Fatalf("insert workout_exercises: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_user_id, workout_date, position, set_number,
		 weight_kg, target_value, completed_value, completed_at, signal)
		 VALUES (?, ?, 0, 1, 80.0, 8, 8, '2026-04-27T10:00:00.000Z', 'on_target')`,
		userID, mondayStr)
	if err != nil {
		t.Fatalf("insert sets: %v", err)
	}

	// GetDeloadStartingWeight returns 80 × 0.9 = 72 (snapped to 0.5).
	got, err := svc.GetDeloadStartingWeight(ctx, exerciseID, deloadMonday)
	if err != nil {
		t.Fatalf("GetDeloadStartingWeight: %v", err)
	}
	if got != 72.0 {
		t.Errorf("got %v, want 72.0 (= 80 * 0.9, snapped)", got)
	}
}

func Test_BuildProgression_CurrentSetUsesDeriveScheme(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("ds-user"), "DS User").Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	// Deadlift-like exercise: rep_min=3, rep_max=6.
	var exerciseID int
	err = db.ReadWrite.QueryRowContext(ctx,
		`INSERT INTO exercises (name, category, description_markdown, rep_min, rep_max)
		 VALUES (?, 'lower', '', 3, 6) RETURNING id`,
		"Test Deadlift DS").Scan(&exerciseID)
	if err != nil {
		t.Fatalf("insert exercise: %v", err)
	}

	svc := service.NewService(db, logger, "")

	today := time.Now().Format("2006-01-02")

	// Hypertrophy session today.
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_sessions (user_id, workout_date, started_at, periodization_type)
		 VALUES (?, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ'), 'hypertrophy')`,
		userID, today)
	if err != nil {
		t.Fatalf("insert hypertrophy session: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO workout_exercises (workout_user_id, workout_date, position, exercise_id) VALUES (?, ?, 0, ?)",
		userID, today, exerciseID)
	if err != nil {
		t.Fatalf("insert workout_exercises: %v", err)
	}

	date, _ := time.Parse("2006-01-02", today)

	// Hypertrophy: DeriveScheme(3, 6, Hypertrophy).TargetReps == 6 (repMax).
	// Before the fix this returned 8 (the legacy TargetReps hypertrophy constant).
	prog, err := svc.BuildProgression(ctx, date, exerciseID)
	if err != nil {
		t.Fatalf("BuildProgression hypertrophy: %v", err)
	}
	if got := prog.CurrentSet().TargetReps; got != 6 {
		t.Errorf("hypertrophy CurrentSet().TargetReps: want 6, got %d (legacy bug returned 8)", got)
	}

	// Strength session: DeriveScheme(3, 6, Strength).TargetReps == 3 (repMin).
	strengthDay := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_sessions (user_id, workout_date, started_at, periodization_type)
		 VALUES (?, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ'), 'strength')`,
		userID, strengthDay)
	if err != nil {
		t.Fatalf("insert strength session: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO workout_exercises (workout_user_id, workout_date, position, exercise_id) VALUES (?, ?, 0, ?)",
		userID, strengthDay, exerciseID)
	if err != nil {
		t.Fatalf("insert strength workout_exercises: %v", err)
	}

	strengthDate, _ := time.Parse("2006-01-02", strengthDay)
	prog, err = svc.BuildProgression(ctx, strengthDate, exerciseID)
	if err != nil {
		t.Fatalf("BuildProgression strength: %v", err)
	}
	if got := prog.CurrentSet().TargetReps; got != 3 {
		t.Errorf("strength CurrentSet().TargetReps: want 3, got %d", got)
	}
}
