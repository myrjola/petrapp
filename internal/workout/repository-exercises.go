package workout

import (
	"context"
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
		SELECT id, name, category, description_markdown
		FROM exercises
		WHERE id = ?`, id).Scan(
		&exercise.ID,
		&exercise.Name,
		&exercise.Category,
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
		SELECT id, name, category, description_markdown
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
		if err = rows.Scan(&exercise.ID, &exercise.Name, &exercise.Category, &exercise.DescriptionMarkdown); err != nil {
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
