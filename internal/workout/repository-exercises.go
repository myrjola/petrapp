package workout

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/myrjola/petrapp/internal/sqlite"
)

// sqliteExerciseRepository implements exerciseRepository.
type sqliteExerciseRepository struct {
	baseRepository
}

// newSQLiteExerciseRepository creates a new SQLite exercise repository.
func newSQLiteExerciseRepository(db *sqlite.Database) *sqliteExerciseRepository {
	return &sqliteExerciseRepository{
		baseRepository: newBaseRepository(db),
	}
}

// Get retrieves a single exercise by ID.
func (r *sqliteExerciseRepository) Get(ctx context.Context, id int) (Exercise, error) {
	var exercise Exercise

	err := r.db.ReadOnly.QueryRowContext(ctx, `
		SELECT id, name, category, exercise_type, description_markdown
		FROM exercises
		WHERE id = ?`, id).Scan(
		&exercise.ID,
		&exercise.Name,
		&exercise.Category,
		&exercise.ExerciseType,
		&exercise.DescriptionMarkdown,
	)
	if err != nil {
		return Exercise{}, fmt.Errorf("query exercise: %w", err)
	}

	// Fetch muscle groups
	primaryMuscleGroups, secondaryMuscleGroups, err := r.fetchMuscleGroups(ctx, exercise.ID)
	if err != nil {
		return Exercise{}, fmt.Errorf("fetch muscle groups for exercise %d: %w", exercise.ID, err)
	}

	exercise.PrimaryMuscleGroups = primaryMuscleGroups
	exercise.SecondaryMuscleGroups = secondaryMuscleGroups

	return exercise, nil
}

// ListExercises returns all available exercises with their muscle groups.
func (r *sqliteExerciseRepository) List(ctx context.Context) (_ []Exercise, err error) {
	// First, get all exercises
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT id, name, category, exercise_type, description_markdown
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

	var exercises []Exercise
	for rows.Next() {
		var exercise Exercise
		if err = rows.Scan(&exercise.ID, &exercise.Name, &exercise.Category, &exercise.ExerciseType, &exercise.DescriptionMarkdown); err != nil {
			return nil, fmt.Errorf("scan exercise: %w", err)
		}
		exercises = append(exercises, exercise)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	// Now fetch muscle groups for each exercise
	for i, exercise := range exercises {
		var (
			primaryMuscleGroups   []string
			secondaryMuscleGroups []string
		)
		primaryMuscleGroups, secondaryMuscleGroups, err = r.fetchMuscleGroups(ctx, exercise.ID)
		if err != nil {
			return nil, fmt.Errorf("fetch muscle groups for exercise %d: %w", exercise.ID, err)
		}
		exercises[i].PrimaryMuscleGroups = primaryMuscleGroups
		exercises[i].SecondaryMuscleGroups = secondaryMuscleGroups
	}

	return exercises, nil
}

// fetchMuscleGroups retrieves the muscle groups for an exercise.
func (r *sqliteExerciseRepository) fetchMuscleGroups(
	ctx context.Context,
	exerciseID int,
) (_ []string, _ []string, err error) {
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
        SELECT mg.name, emg.is_primary
        FROM exercise_muscle_groups emg
        JOIN muscle_groups mg ON emg.muscle_group_name = mg.name
        WHERE emg.exercise_id = ?
    `, exerciseID)
	if err != nil {
		return nil, nil, fmt.Errorf("query muscle groups: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close rows: %w", closeErr))
		}
	}()

	var primaryMuscleGroups []string
	var secondaryMuscleGroups []string

	for rows.Next() {
		var (
			name      string
			isPrimary bool
		)

		if err = rows.Scan(&name, &isPrimary); err != nil {
			return nil, nil, fmt.Errorf("scan muscle group row: %w", err)
		}

		if isPrimary {
			primaryMuscleGroups = append(primaryMuscleGroups, name)
		} else {
			secondaryMuscleGroups = append(secondaryMuscleGroups, name)
		}
	}

	if err = rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate muscle group rows: %w", err)
	}

	return primaryMuscleGroups, secondaryMuscleGroups, nil
}

// Create adds a new exercise to the repository.
func (r *sqliteExerciseRepository) Create(ctx context.Context, ex Exercise) (Exercise, error) {
	var err error
	if ex, err = r.set(ctx, ex, false); err != nil {
		return ex, fmt.Errorf("create exercise: %w", err)
	}
	return ex, nil
}

// Update modifies an existing exercise.
func (r *sqliteExerciseRepository) Update(
	ctx context.Context,
	exerciseID int,
	updateFn func(ex *Exercise) (bool, error),
) error {
	// Get current exercise
	exercise, err := r.Get(ctx, exerciseID)
	if err != nil {
		return fmt.Errorf("get exercise for update: %w", err)
	}

	// Apply updates
	updated, err := updateFn(&exercise)
	if err != nil {
		return fmt.Errorf("update function: %w", err)
	}

	// Skip if no changes were made
	if !updated {
		return nil
	}

	// Upsert
	if _, err = r.set(ctx, exercise, true); err != nil {
		return fmt.Errorf("save updated exercise: %w", err)
	}

	return nil
}

// set creates or updates an exercise with optional upsert.
func (r *sqliteExerciseRepository) set(ctx context.Context, ex Exercise, upsert bool) (_ Exercise, err error) {
	// Begin transaction
	tx, err := r.db.ReadWrite.BeginTx(ctx, nil)
	if err != nil {
		return ex, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("rollback transaction: %w", rollbackErr))
		}
	}()

	// For upsert, first delete the existing exercise
	if upsert {
		_, err = tx.ExecContext(ctx, `
			DELETE FROM exercises 
			WHERE id = ?`,
			ex.ID)
		if err != nil {
			return ex, fmt.Errorf("delete exercise: %w", err)
		}
	}

	// Insert or reinsert the exercise
	var result sql.Result
	if upsert {
		// When upserting, use the existing ID
		result, err = tx.ExecContext(ctx, `
			INSERT INTO exercises (id, name, category, exercise_type, description_markdown)
			VALUES (?, ?, ?, ?, ?)`,
			ex.ID, ex.Name, ex.Category, ex.ExerciseType, ex.DescriptionMarkdown)
	} else {
		// When creating new, let SQLite assign the ID
		result, err = tx.ExecContext(ctx, `
			INSERT INTO exercises (name, category, exercise_type, description_markdown)
			VALUES (?, ?, ?, ?)`,
			ex.Name, ex.Category, ex.ExerciseType, ex.DescriptionMarkdown)
	}

	if err != nil {
		return ex, fmt.Errorf("insert exercise: %w", err)
	}

	// Get the inserted ID for new exercises
	if !upsert {
		var id int64
		id, err = result.LastInsertId()
		if err != nil {
			return ex, fmt.Errorf("get last insert ID: %w", err)
		}
		ex.ID = int(id)
	}

	// Insert primary muscle groups
	if err = r.insertMuscleGroups(ctx, tx, ex.ID, ex.PrimaryMuscleGroups, true); err != nil {
		return ex, fmt.Errorf("insert primary muscle groups: %w", err)
	}

	// Insert secondary muscle groups
	if err = r.insertMuscleGroups(ctx, tx, ex.ID, ex.SecondaryMuscleGroups, false); err != nil {
		return ex, fmt.Errorf("insert secondary muscle groups: %w", err)
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return ex, fmt.Errorf("commit transaction: %w", err)
	}

	return ex, nil
}

// insertMuscleGroups inserts muscle groups for an exercise.
func (r *sqliteExerciseRepository) insertMuscleGroups(
	ctx context.Context,
	tx *sql.Tx,
	exerciseID int,
	muscleGroups []string,
	isPrimary bool,
) error {
	for _, muscleGroup := range muscleGroups {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO exercise_muscle_groups (exercise_id, muscle_group_name, is_primary)
			VALUES (?, ?, ?)`,
			exerciseID, muscleGroup, isPrimary)
		if err != nil {
			return fmt.Errorf("insert muscle group %s: %w", muscleGroup, err)
		}
	}
	return nil
}

// ListMuscleGroups retrieves all available muscle groups.
func (r *sqliteExerciseRepository) ListMuscleGroups(ctx context.Context) ([]string, error) {
	var muscleGroups []string
	var err error

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
