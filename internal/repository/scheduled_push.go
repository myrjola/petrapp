package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/sqlite"
)

type sqliteScheduledPushRepository struct {
	baseRepository
}

func newSQLiteScheduledPushRepository(db *sqlite.Database) *sqliteScheduledPushRepository {
	return &sqliteScheduledPushRepository{baseRepository: newBaseRepository(db)}
}

// Replace upserts the row for the given workout_exercise_id. The UNIQUE index
// on workout_exercise_id enforces the one-pending-push-per-slot invariant.
func (r *sqliteScheduledPushRepository) Replace(
	ctx context.Context, push domain.ScheduledPush,
) (domain.ScheduledPush, error) {
	var createdAt sql.NullString
	err := r.db.ReadWrite.QueryRowContext(ctx, `
		INSERT INTO scheduled_pushes (user_id, workout_exercise_id, fire_at, payload)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (workout_exercise_id) DO UPDATE SET
		    user_id = excluded.user_id,
		    fire_at = excluded.fire_at,
		    payload = excluded.payload
		RETURNING id, created_at`,
		push.UserID, push.WorkoutExerciseID, formatTimestamp(push.FireAt), push.Payload,
	).Scan(&push.ID, &createdAt)
	if err != nil {
		return domain.ScheduledPush{}, fmt.Errorf("upsert scheduled push: %w", err)
	}
	if push.CreatedAt, err = parseTimestamp(createdAt); err != nil {
		return domain.ScheduledPush{}, fmt.Errorf("parse created_at: %w", err)
	}
	return push, nil
}

func (r *sqliteScheduledPushRepository) Delete(ctx context.Context, id int) error {
	if _, err := r.db.ReadWrite.ExecContext(ctx,
		`DELETE FROM scheduled_pushes WHERE id = ?`, id,
	); err != nil {
		return fmt.Errorf("delete scheduled push: %w", err)
	}
	return nil
}

func (r *sqliteScheduledPushRepository) DeleteByWorkoutExercise(ctx context.Context, workoutExerciseID int) error {
	if _, err := r.db.ReadWrite.ExecContext(ctx,
		`DELETE FROM scheduled_pushes WHERE workout_exercise_id = ?`, workoutExerciseID,
	); err != nil {
		return fmt.Errorf("delete scheduled push by workout_exercise: %w", err)
	}
	return nil
}

func (r *sqliteScheduledPushRepository) DeleteByWorkoutSession(
	ctx context.Context, userID int, date time.Time,
) error {
	if _, err := r.db.ReadWrite.ExecContext(ctx, `
		DELETE FROM scheduled_pushes
		WHERE workout_exercise_id IN (
		    SELECT id FROM workout_exercise
		    WHERE workout_user_id = ? AND workout_date = ?
		)`, userID, formatDate(date),
	); err != nil {
		return fmt.Errorf("delete scheduled pushes by session: %w", err)
	}
	return nil
}

func (r *sqliteScheduledPushRepository) Get(
	ctx context.Context, workoutExerciseID int,
) (domain.ScheduledPush, error) {
	var (
		push      domain.ScheduledPush
		fireAt    sql.NullString
		createdAt sql.NullString
	)
	err := r.db.ReadOnly.QueryRowContext(ctx, `
		SELECT id, user_id, workout_exercise_id, fire_at, payload, created_at
		FROM scheduled_pushes
		WHERE workout_exercise_id = ?`, workoutExerciseID,
	).Scan(&push.ID, &push.UserID, &push.WorkoutExerciseID, &fireAt, &push.Payload, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ScheduledPush{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.ScheduledPush{}, fmt.Errorf("query scheduled push: %w", err)
	}
	if push.FireAt, err = parseTimestamp(fireAt); err != nil {
		return domain.ScheduledPush{}, fmt.Errorf("parse fire_at: %w", err)
	}
	if push.CreatedAt, err = parseTimestamp(createdAt); err != nil {
		return domain.ScheduledPush{}, fmt.Errorf("parse created_at: %w", err)
	}
	return push, nil
}

func (r *sqliteScheduledPushRepository) ListAll(ctx context.Context) (_ []domain.ScheduledPush, err error) {
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT id, user_id, workout_exercise_id, fire_at, payload, created_at
		FROM scheduled_pushes
		ORDER BY fire_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("query scheduled pushes: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close rows: %w", closeErr))
		}
	}()

	var pushes []domain.ScheduledPush
	for rows.Next() {
		var (
			push      domain.ScheduledPush
			fireAt    sql.NullString
			createdAt sql.NullString
		)
		if err = rows.Scan(
			&push.ID, &push.UserID, &push.WorkoutExerciseID, &fireAt, &push.Payload, &createdAt,
		); err != nil {
			return nil, fmt.Errorf("scan scheduled push: %w", err)
		}
		if push.FireAt, err = parseTimestamp(fireAt); err != nil {
			return nil, fmt.Errorf("parse fire_at: %w", err)
		}
		if push.CreatedAt, err = parseTimestamp(createdAt); err != nil {
			return nil, fmt.Errorf("parse created_at: %w", err)
		}
		pushes = append(pushes, push)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return pushes, nil
}
