package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/sqlite"
)

const (
	timestampFormat = "2006-01-02T15:04:05.000Z"
	dateFormat      = time.DateOnly
)

// queryer is satisfied by both *sql.DB and *sql.Tx, so read helpers can run
// either standalone or inside an open transaction.
type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// muscleGroups holds the primary/secondary muscle-group names for one exercise.
type muscleGroups struct {
	primary   []string
	secondary []string
}

// fetchMuscleGroupsByExerciseID loads primary/secondary muscle groups for every
// given exercise ID in a single query, keyed by exercise ID. Returns an empty
// map when ids is empty. Shared by the exercise and session repositories so
// neither issues a per-exercise follow-up query.
func fetchMuscleGroupsByExerciseID(
	ctx context.Context,
	q queryer,
	ids []int,
) (_ map[int]muscleGroups, err error) {
	if len(ids) == 0 {
		return map[int]muscleGroups{}, nil
	}

	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1] // trim trailing comma
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	query := `
		SELECT emg.exercise_id, mg.name, emg.is_primary
		FROM exercise_muscle_groups emg
		JOIN muscle_groups mg ON emg.muscle_group_name = mg.name
		WHERE emg.exercise_id IN (` + placeholders + `)`

	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query muscle groups: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close muscle group rows: %w", closeErr))
		}
	}()

	byExercise := make(map[int]muscleGroups, len(ids))
	for rows.Next() {
		var (
			exerciseID int
			name       string
			isPrimary  bool
		)
		if err = rows.Scan(&exerciseID, &name, &isPrimary); err != nil {
			return nil, fmt.Errorf("scan muscle group row: %w", err)
		}
		g := byExercise[exerciseID]
		if isPrimary {
			g.primary = append(g.primary, name)
		} else {
			g.secondary = append(g.secondary, name)
		}
		byExercise[exerciseID] = g
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate muscle group rows: %w", err)
	}
	return byExercise, nil
}

// baseRepository contains common functionality for all SQLite repositories.
type baseRepository struct {
	db *sqlite.Database
}

func newBaseRepository(db *sqlite.Database) baseRepository {
	return baseRepository{db: db}
}

// parseTimestamp parses a timestamp from a nullable database string.
func parseTimestamp(timestampStr sql.NullString) (time.Time, error) {
	if !timestampStr.Valid {
		return time.Time{}, nil
	}
	parsedTime, err := time.Parse(timestampFormat, timestampStr.String)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse timestamp format: %w", err)
	}
	return parsedTime, nil
}

// formatDate formats a time.Time to the canonical YYYY-MM-DD string.
func formatDate(date time.Time) string {
	return date.Format(dateFormat)
}

// formatTimestamp formats a time.Time to the canonical UTC ISO-8601 string.
func formatTimestamp(t time.Time) string {
	return t.UTC().Format(timestampFormat)
}

// insertSessionInTx inserts a workout_sessions row and its child workout_exercise
// + exercise_sets rows for the authenticated user. Shared by SessionRepository
// and WeekPlanRepository so both repos persist with identical SQL shape.
func (r baseRepository) insertSessionInTx(ctx context.Context, tx *sql.Tx, sess domain.Session) error {
	if err := r.insertSessionRowInTx(ctx, tx, sess); err != nil {
		return err
	}
	if err := r.saveExerciseSetsInTx(ctx, tx, sess.Date, sess.ExerciseSets); err != nil {
		return fmt.Errorf("save exercise sets: %w", err)
	}
	return nil
}

// insertSessionRowInTx inserts only the workout_sessions row for sess, without
// touching workout_exercise or exercise_sets. Used by WeekPlanRepository.Update
// to stage every session row before any slot is inserted — required so the
// three-pass slot insert can claim pre-existing IDs across all sessions before
// SQLite auto-assigns rowids for new (ID==0) slots.
func (r baseRepository) insertSessionRowInTx(ctx context.Context, tx *sql.Tx, sess domain.Session) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	dateStr := formatDate(sess.Date)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO workout_sessions (
			user_id, workout_date, difficulty_rating, started_at, completed_at, periodization_type, is_deload
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID, dateStr, sess.DifficultyRating,
		formatTimestamp(sess.StartedAt), formatTimestamp(sess.CompletedAt),
		sess.PeriodizationType, sess.IsDeload); err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

// saveExerciseSetsInTx writes the workout_exercise rows and their child
// exercise_sets for a session in two passes: explicit-ID slots first (so they
// claim their rowids), then ID==0 slots (so SQLite auto-assigns rowids around
// the already-claimed IDs). Without the two-pass split, a new slot could
// auto-assign a rowid that a later explicit-ID slot would then collide with.
func (r baseRepository) saveExerciseSetsInTx(
	ctx context.Context,
	tx *sql.Tx,
	date time.Time,
	exerciseSets []domain.ExerciseSet,
) error {
	for _, slot := range exerciseSets {
		if slot.ID == 0 {
			continue
		}
		if err := r.saveOneSlotInTx(ctx, tx, date, slot); err != nil {
			return err
		}
	}
	for _, slot := range exerciseSets {
		if slot.ID > 0 {
			continue
		}
		if err := r.saveOneSlotInTx(ctx, tx, date, slot); err != nil {
			return err
		}
	}
	return nil
}

// saveOneSlotInTx inserts a single workout_exercise row (preserving slot.ID
// when > 0, auto-assigning when 0) and its child exercise_sets rows. The
// set_number sequence is per-slot and starts at 1; reordering slots at the
// caller does not affect that numbering.
func (r baseRepository) saveOneSlotInTx(
	ctx context.Context,
	tx *sql.Tx,
	date time.Time,
	slot domain.ExerciseSet,
) error {
	dateStr := formatDate(date)
	userID := contexthelpers.AuthenticatedUserID(ctx)

	var idArg any
	if slot.ID > 0 {
		idArg = slot.ID
	}
	var warmupArg any
	if slot.WarmupCompletedAt != nil {
		warmupArg = formatTimestamp(*slot.WarmupCompletedAt)
	}
	var weID int
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO workout_exercise (
			id, workout_user_id, workout_date, exercise_id, warmup_completed_at
		) VALUES (?, ?, ?, ?, ?)
		RETURNING id`,
		idArg, userID, dateStr, slot.Exercise.ID, warmupArg).Scan(&weID); err != nil {
		return fmt.Errorf("insert workout exercise: %w", err)
	}
	for i, set := range slot.Sets {
		var completedAtStr any
		if set.CompletedAt != nil {
			completedAtStr = formatTimestamp(*set.CompletedAt)
		}
		var signalValue any
		if set.Signal != nil {
			signalValue = string(*set.Signal)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO exercise_sets (
				workout_exercise_id, set_number,
				weight_kg, target_value, completed_value, completed_at, signal
			) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			weID, i+1,
			set.WeightKg, set.TargetValue, set.CompletedValue, completedAtStr, signalValue); err != nil {
			return fmt.Errorf("insert exercise set: %w", err)
		}
	}
	return nil
}

// deleteWeekInTx removes all workout_sessions for the user between [monday,
// monday+6] inside tx. CASCADE clears child workout_exercise and exercise_sets
// rows. Called from WeekPlanRepository.Update before the three-pass reinsert.
func (r baseRepository) deleteWeekInTx(
	ctx context.Context, tx *sql.Tx, userID int, monday time.Time,
) error {
	sunday := monday.AddDate(0, 0, 6)
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM workout_sessions
		WHERE user_id = ? AND workout_date BETWEEN ? AND ?`,
		userID, formatDate(monday), formatDate(sunday)); err != nil {
		return fmt.Errorf("delete week sessions: %w", err)
	}
	return nil
}
