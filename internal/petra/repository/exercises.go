package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/myrjola/petrapp/internal/petra/domain"
	"github.com/myrjola/petrapp/internal/platform/sqlitekit"
)

type sqliteExerciseRepository struct {
	baseRepository
}

func newSQLiteExerciseRepository(db *sqlitekit.Database) *sqliteExerciseRepository {
	return &sqliteExerciseRepository{baseRepository: newBaseRepository(db)}
}

func (r *sqliteExerciseRepository) Get(ctx context.Context, id int) (domain.Exercise, error) {
	return r.get(ctx, r.db.ReadOnly, id)
}

func (r *sqliteExerciseRepository) ListMuscleGroups(ctx context.Context) (_ []string, err error) {
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT name
		FROM muscle_groups
		ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("query muscle groups: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close rows: %w", closeErr))
		}
	}()

	var muscleGroups []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan muscle group: %w", err)
		}
		muscleGroups = append(muscleGroups, name)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return muscleGroups, nil
}

func (r *sqliteExerciseRepository) List(ctx context.Context) (_ []domain.Exercise, err error) {
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT id, name, category, exercise_type, content,
		       default_starting_seconds, rep_min, rep_max
		FROM exercises
		ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("query exercises: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close rows: %w", closeErr))
		}
	}()

	var exercises []domain.Exercise
	for rows.Next() {
		var exercise domain.Exercise
		var content string
		var defaultStartingSeconds, repMin, repMax sql.NullInt64
		if err = rows.Scan(
			&exercise.ID, &exercise.Name, &exercise.Category, &exercise.ExerciseType,
			&content, &defaultStartingSeconds, &repMin, &repMax,
		); err != nil {
			return nil, fmt.Errorf("scan exercise: %w", err)
		}
		if err = unmarshalExerciseContent(content, &exercise); err != nil {
			return nil, err
		}
		if defaultStartingSeconds.Valid {
			v := int(defaultStartingSeconds.Int64)
			exercise.DefaultStartingSeconds = &v
		}
		if repMin.Valid {
			v := int(repMin.Int64)
			exercise.RepMin = &v
		}
		if repMax.Valid {
			v := int(repMax.Int64)
			exercise.RepMax = &v
		}
		exercises = append(exercises, exercise)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	ids := make([]int, len(exercises))
	for i := range exercises {
		ids[i] = exercises[i].ID
	}
	byExercise, err := fetchMuscleGroupsByExerciseID(ctx, r.db.ReadOnly, ids)
	if err != nil {
		return nil, fmt.Errorf("fetch muscle groups: %w", err)
	}
	for i := range exercises {
		g := byExercise[exercises[i].ID]
		exercises[i].PrimaryMuscleGroups = g.primary
		exercises[i].SecondaryMuscleGroups = g.secondary
	}
	return exercises, nil
}

func (r *sqliteExerciseRepository) Create(ctx context.Context, ex domain.Exercise) (_ domain.Exercise, err error) {
	tx, err := r.db.ReadWrite.BeginTx(ctx, nil)
	if err != nil {
		return domain.Exercise{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("rollback transaction: %w", rollbackErr))
		}
	}()

	created, err := r.set(ctx, tx, ex, false)
	if err != nil {
		return domain.Exercise{}, fmt.Errorf("create exercise: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return domain.Exercise{}, fmt.Errorf("commit create exercise: %w", err)
	}
	return created, nil
}

// Update loads the exercise inside a single transaction, runs fn against the
// hydrated *domain.Exercise, and persists the result. Returning nil from fn
// commits; returning an error rolls back without writing. The read and write
// share one transaction so concurrent updates cannot interleave a
// read-modify-write race.
func (r *sqliteExerciseRepository) Update(
	ctx context.Context,
	exerciseID int,
	fn func(*domain.Exercise) error,
) (err error) {
	tx, err := r.db.ReadWrite.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("rollback transaction: %w", rollbackErr))
		}
	}()

	exercise, err := r.get(ctx, tx, exerciseID)
	if err != nil {
		return fmt.Errorf("get exercise for update: %w", err)
	}
	if err = fn(&exercise); err != nil {
		return err
	}
	if _, err = r.set(ctx, tx, exercise, true); err != nil {
		return fmt.Errorf("save updated exercise: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit update exercise: %w", err)
	}
	return nil
}

// get loads a single exercise via q, which may be the read-only handle or an
// open transaction. Reading through the Update transaction is what makes the
// exercise read-modify-write atomic.
func (r *sqliteExerciseRepository) get(ctx context.Context, q queryer, id int) (domain.Exercise, error) {
	var exercise domain.Exercise
	var content string
	var defaultStartingSeconds, repMin, repMax sql.NullInt64

	err := q.QueryRowContext(ctx, `
		SELECT id, name, category, exercise_type, content,
		       default_starting_seconds, rep_min, rep_max
		FROM exercises
		WHERE id = ?`, id).Scan(
		&exercise.ID,
		&exercise.Name,
		&exercise.Category,
		&exercise.ExerciseType,
		&content,
		&defaultStartingSeconds,
		&repMin,
		&repMax,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Exercise{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Exercise{}, fmt.Errorf("query exercise: %w", err)
	}
	if err = unmarshalExerciseContent(content, &exercise); err != nil {
		return domain.Exercise{}, err
	}
	if defaultStartingSeconds.Valid {
		v := int(defaultStartingSeconds.Int64)
		exercise.DefaultStartingSeconds = &v
	}
	if repMin.Valid {
		v := int(repMin.Int64)
		exercise.RepMin = &v
	}
	if repMax.Valid {
		v := int(repMax.Int64)
		exercise.RepMax = &v
	}

	byExercise, err := fetchMuscleGroupsByExerciseID(ctx, q, []int{exercise.ID})
	if err != nil {
		return domain.Exercise{}, fmt.Errorf("fetch muscle groups for exercise %d: %w", exercise.ID, err)
	}
	g := byExercise[exercise.ID]
	exercise.PrimaryMuscleGroups = g.primary
	exercise.SecondaryMuscleGroups = g.secondary

	return exercise, nil
}

// set writes the exercise row and its muscle-group associations inside tx. When
// upsert is true the existing row (matched by ex.ID) is deleted first and the
// explicit ID is reused; otherwise a fresh ID is assigned and returned. The
// caller owns the transaction.
func (r *sqliteExerciseRepository) set(
	ctx context.Context,
	tx *sql.Tx,
	ex domain.Exercise,
	upsert bool,
) (domain.Exercise, error) {
	if upsert {
		if _, err := tx.ExecContext(ctx, `DELETE FROM exercises WHERE id = ?`, ex.ID); err != nil {
			return ex, fmt.Errorf("delete exercise: %w", err)
		}
	}

	content, err := marshalExerciseContent(ex)
	if err != nil {
		return ex, err
	}

	var result sql.Result
	if upsert {
		result, err = tx.ExecContext(ctx, `
			INSERT INTO exercises (id, name, category, exercise_type, content,
			                       default_starting_seconds, rep_min, rep_max)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			ex.ID, ex.Name, ex.Category, ex.ExerciseType, content,
			ex.DefaultStartingSeconds, ex.RepMin, ex.RepMax)
	} else {
		result, err = tx.ExecContext(ctx, `
			INSERT INTO exercises (name, category, exercise_type, content,
			                       default_starting_seconds, rep_min, rep_max)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			ex.Name, ex.Category, ex.ExerciseType, content,
			ex.DefaultStartingSeconds, ex.RepMin, ex.RepMax)
	}
	if err != nil {
		return ex, fmt.Errorf("insert exercise: %w", err)
	}

	if !upsert {
		var id int64
		if id, err = result.LastInsertId(); err != nil {
			return ex, fmt.Errorf("get last insert ID: %w", err)
		}
		ex.ID = int(id)
	}

	if err = r.insertMuscleGroups(ctx, tx, ex.ID, ex.PrimaryMuscleGroups, true); err != nil {
		return ex, fmt.Errorf("insert primary muscle groups: %w", err)
	}
	if err = r.insertMuscleGroups(ctx, tx, ex.ID, ex.SecondaryMuscleGroups, false); err != nil {
		return ex, fmt.Errorf("insert secondary muscle groups: %w", err)
	}
	return ex, nil
}

func (r *sqliteExerciseRepository) insertMuscleGroups(
	ctx context.Context,
	tx *sql.Tx,
	exerciseID int,
	muscleGroups []string,
	isPrimary bool,
) error {
	if len(muscleGroups) == 0 {
		return nil
	}
	// One statement: VALUES (?, ?, ?), (?, ?, ?), ...
	const colsPerRow = 3 // exercise_id, muscle_group_name, is_primary
	placeholders := strings.Repeat("(?, ?, ?),", len(muscleGroups))
	placeholders = placeholders[:len(placeholders)-1] // trim trailing comma
	args := make([]any, 0, len(muscleGroups)*colsPerRow)
	for _, mg := range muscleGroups {
		args = append(args, exerciseID, mg, isPrimary)
	}
	//nolint:gosec // placeholders is built from a count, not user input
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO exercise_muscle_groups (exercise_id, muscle_group_name, is_primary)
		VALUES `+placeholders, args...); err != nil {
		return fmt.Errorf("insert muscle groups: %w", err)
	}
	return nil
}
