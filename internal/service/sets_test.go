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

func Test_RecordSetCompletion(t *testing.T) {
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	defer func() { _ = db.Close() }()

	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("rsc-user"), "RSC User").Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	// Deadlift is pre-seeded by fixtures.sql; fetch its ID directly.
	var exerciseID int
	err = db.ReadOnly.QueryRowContext(ctx, "SELECT id FROM exercises WHERE name = 'Deadlift'").Scan(&exerciseID)
	if err != nil {
		t.Fatalf("get exercise id: %v", err)
	}

	today := time.Now().Format("2006-01-02")
	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO workout_sessions (user_id, workout_date, started_at) VALUES (?, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ'))",
		userID, today)
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}
	var weID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES (?, ?, ?) RETURNING id",
		userID, today, exerciseID).Scan(&weID)
	if err != nil {
		t.Fatalf("insert workout_exercise: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_exercise_id, set_number,
		 weight_kg, target_value)
		 VALUES (?, 1, 100.0, 5)`,
		weID)
	if err != nil {
		t.Fatalf("insert set: %v", err)
	}

	svc := service.NewService(db, logger, "")
	date, _ := time.Parse("2006-01-02", today)

	weight := 102.5
	if err = svc.RecordSet(ctx, date, weID, 0, domain.SignalOnTarget, &weight, 5); err != nil {
		t.Fatalf("RecordSet: %v", err)
	}

	sess, err := svc.GetSession(ctx, date)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}

	var es *domain.ExerciseSet
	for i := range sess.ExerciseSets {
		if sess.ExerciseSets[i].Exercise.ID == exerciseID {
			es = &sess.ExerciseSets[i]
			break
		}
	}
	if es == nil {
		t.Fatal("exercise not found in session")
	}

	set := es.Sets[0]
	if set.Signal == nil || *set.Signal != domain.SignalOnTarget {
		t.Errorf("signal: want on_target, got %v", set.Signal)
	}
	if set.WeightKg == nil || *set.WeightKg != 102.5 {
		t.Errorf("weight: want 102.5, got %v", set.WeightKg)
	}
	if set.CompletedValue == nil || *set.CompletedValue != 5 {
		t.Errorf("completed value: want 5, got %v", set.CompletedValue)
	}
	if set.CompletedAt == nil {
		t.Error("completed_at: want non-nil")
	}
}
