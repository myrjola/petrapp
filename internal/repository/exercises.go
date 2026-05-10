package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/sqlite"
)

type sqliteExerciseRepository struct {
	baseRepository
}

func newSQLiteExerciseRepository(db *sqlite.Database) *sqliteExerciseRepository {
	return &sqliteExerciseRepository{baseRepository: newBaseRepository(db)}
}

func (r *sqliteExerciseRepository) Get(ctx context.Context, id int) (domain.Exercise, error) {
	var exercise domain.Exercise
	var defaultStartingSeconds, repMin, repMax sql.NullInt64

	err := r.db.ReadOnly.QueryRowContext(ctx, `
		SELECT id, name, category, exercise_type, description_markdown,
		       default_starting_seconds, rep_min, rep_max
		FROM exercises
		WHERE id = ?`, id).Scan(
		&exercise.ID,
		&exercise.Name,
		&exercise.Category,
		&exercise.ExerciseType,
		&exercise.DescriptionMarkdown,
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

	primaryMuscleGroups, secondaryMuscleGroups, err := r.fetchMuscleGroups(ctx, exercise.ID)
	if err != nil {
		return domain.Exercise{}, fmt.Errorf("fetch muscle groups for exercise %d: %w", exercise.ID, err)
	}
	exercise.PrimaryMuscleGroups = primaryMuscleGroups
	exercise.SecondaryMuscleGroups = secondaryMuscleGroups

	return exercise, nil
}

func (r *sqliteExerciseRepository) List(ctx context.Context) (_ []domain.Exercise, err error) {
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT id, name, category, exercise_type, description_markdown,
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
		var defaultStartingSeconds, repMin, repMax sql.NullInt64
		if err = rows.Scan(
			&exercise.ID, &exercise.Name, &exercise.Category, &exercise.ExerciseType,
			&exercise.DescriptionMarkdown, &defaultStartingSeconds, &repMin, &repMax,
		); err != nil {
			return nil, fmt.Errorf("scan exercise: %w", err)
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

	for i, exercise := range exercises {
		var primary, secondary []string
		primary, secondary, err = r.fetchMuscleGroups(ctx, exercise.ID)
		if err != nil {
			return nil, fmt.Errorf("fetch muscle groups for exercise %d: %w", exercise.ID, err)
		}
		exercises[i].PrimaryMuscleGroups = primary
		exercises[i].SecondaryMuscleGroups = secondary
	}
	return exercises, nil
}

func (r *sqliteExerciseRepository) fetchMuscleGroups(
	ctx context.Context,
	exerciseID int,
) (_ []string, _ []string, err error) {
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT mg.name, emg.is_primary
		FROM exercise_muscle_groups emg
		JOIN muscle_groups mg ON emg.muscle_group_name = mg.name
		WHERE emg.exercise_id = ?`, exerciseID)
	if err != nil {
		return nil, nil, fmt.Errorf("query muscle groups: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close rows: %w", closeErr))
		}
	}()

	var primary, secondary []string
	for rows.Next() {
		var (
			name      string
			isPrimary bool
		)
		if err = rows.Scan(&name, &isPrimary); err != nil {
			return nil, nil, fmt.Errorf("scan muscle group row: %w", err)
		}
		if isPrimary {
			primary = append(primary, name)
		} else {
			secondary = append(secondary, name)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate muscle group rows: %w", err)
	}
	return primary, secondary, nil
}

func (r *sqliteExerciseRepository) Create(ctx context.Context, ex domain.Exercise) (domain.Exercise, error) {
	created, err := r.set(ctx, ex, false)
	if err != nil {
		return domain.Exercise{}, fmt.Errorf("create exercise: %w", err)
	}
	return created, nil
}

// Update reads the exercise, runs fn, and persists the result if fn returned
// nil. fn returning an error rolls back without writing.
func (r *sqliteExerciseRepository) Update(
	ctx context.Context,
	exerciseID int,
	fn func(*domain.Exercise) error,
) error {
	exercise, err := r.Get(ctx, exerciseID)
	if err != nil {
		return fmt.Errorf("get exercise for update: %w", err)
	}
	if err = fn(&exercise); err != nil {
		return err
	}
	if _, err = r.set(ctx, exercise, true); err != nil {
		return fmt.Errorf("save updated exercise: %w", err)
	}
	return nil
}

func (r *sqliteExerciseRepository) set(
	ctx context.Context,
	ex domain.Exercise,
	upsert bool,
) (_ domain.Exercise, err error) {
	tx, err := r.db.ReadWrite.BeginTx(ctx, nil)
	if err != nil {
		return ex, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("rollback transaction: %w", rollbackErr))
		}
	}()

	if upsert {
		if _, err = tx.ExecContext(ctx, `DELETE FROM exercises WHERE id = ?`, ex.ID); err != nil {
			return ex, fmt.Errorf("delete exercise: %w", err)
		}
	}

	var result sql.Result
	if upsert {
		result, err = tx.ExecContext(ctx, `
			INSERT INTO exercises (id, name, category, exercise_type, description_markdown,
			                       default_starting_seconds, rep_min, rep_max)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			ex.ID, ex.Name, ex.Category, ex.ExerciseType, ex.DescriptionMarkdown,
			ex.DefaultStartingSeconds, ex.RepMin, ex.RepMax)
	} else {
		result, err = tx.ExecContext(ctx, `
			INSERT INTO exercises (name, category, exercise_type, description_markdown,
			                       default_starting_seconds, rep_min, rep_max)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			ex.Name, ex.Category, ex.ExerciseType, ex.DescriptionMarkdown,
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

	if err = tx.Commit(); err != nil {
		return ex, fmt.Errorf("commit transaction: %w", err)
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
	for _, muscleGroup := range muscleGroups {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO exercise_muscle_groups (exercise_id, muscle_group_name, is_primary)
			VALUES (?, ?, ?)`,
			exerciseID, muscleGroup, isPrimary); err != nil {
			return fmt.Errorf("insert muscle group %s: %w", muscleGroup, err)
		}
	}
	return nil
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
