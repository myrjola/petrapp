// Package repository contains SQLite-backed implementations of the workout
// domain's data-access contracts. Repositories return domain.* types directly;
// no persistence-shaped intermediate aggregate is exposed to callers. Update
// closures operate on *domain.Session — invariants are enforced via the
// aggregate methods on domain.Session, not by the repository.
package repository

import (
	"context"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/sqlite"
)

// Repositories bundles the per-aggregate repository handles wired together by
// New. Its fields are interface-typed so callers depend on the contract, not
// the SQLite implementation.
type Repositories struct {
	Sessions          SessionRepository
	Exercises         ExerciseRepository
	Preferences       PreferencesRepository
	FeatureFlags      FeatureFlagRepository
	MuscleTargets     MuscleGroupTargetRepository
	PushSubscriptions PushSubscriptionRepository
	ScheduledPushes   ScheduledPushRepository
}

// New constructs all seven SQLite-backed repositories. SessionRepository
// hydrates ExerciseSet.Exercise inline by joining `exercises` and batching
// muscle-group lookups, so it no longer depends on ExerciseRepository.
func New(db *sqlite.Database) *Repositories {
	prefs := newSQLitePreferencesRepository(db)
	muscleTargets := newSQLiteMuscleGroupTargetRepository(db)
	featureFlags := newSQLiteFeatureFlagRepository(db)
	exercises := newSQLiteExerciseRepository(db)
	sessions := newSQLiteSessionRepository(db)
	pushSubs := newSQLitePushSubscriptionRepository(db)
	scheduledPushes := newSQLiteScheduledPushRepository(db)
	return &Repositories{
		Preferences:       prefs,
		MuscleTargets:     muscleTargets,
		FeatureFlags:      featureFlags,
		Exercises:         exercises,
		Sessions:          sessions,
		PushSubscriptions: pushSubs,
		ScheduledPushes:   scheduledPushes,
	}
}

// SessionRepository persists workout sessions and their exercise slots.
type SessionRepository interface {
	Get(ctx context.Context, date time.Time) (domain.Session, error)
	List(ctx context.Context, sinceDate time.Time) ([]domain.Session, error)
	CreateBatch(ctx context.Context, sessions []domain.Session) error

	// Create inserts a single session and its exercise slots. Returns
	// domain.ErrAlreadyExists (wrapped) if a session already exists for the
	// date — callers use errors.Is to recover from concurrent insert races.
	Create(ctx context.Context, sess domain.Session) error

	// Update loads the session inside a single transaction, runs fn against
	// the hydrated *domain.Session, and persists the result. Returning nil
	// from fn commits; returning an error rolls back. Sentinel errors from
	// domain (e.g. ErrAlreadyStarted) propagate so callers can detect no-op
	// cases via errors.Is.
	Update(ctx context.Context, date time.Time, fn func(*domain.Session) error) error

	DeleteWeek(ctx context.Context, monday time.Time) error

	// Read-only specialised queries.
	ListSetsForExerciseSince(
		ctx context.Context, exerciseID int, sinceDate time.Time,
	) ([]domain.ExerciseSetHistory, error)
	GetLatestStartingWeightBefore(
		ctx context.Context, exerciseID int, beforeDate time.Time,
	) (domain.LatestStartingSet, error)
	GetLatestSuccessfulSecondsBefore(
		ctx context.Context, exerciseID int, beforeDate time.Time,
	) (int, error)
	CountCompleted(ctx context.Context) (int, error)
}

// ExerciseRepository persists exercise definitions and their muscle-group
// associations.
type ExerciseRepository interface {
	Get(ctx context.Context, id int) (domain.Exercise, error)
	List(ctx context.Context) ([]domain.Exercise, error)
	Create(ctx context.Context, ex domain.Exercise) (domain.Exercise, error)

	// Update reads the exercise, runs fn, and persists the result if fn
	// returned nil. fn returning an error rolls back without writing.
	Update(ctx context.Context, exerciseID int, fn func(*domain.Exercise) error) error

	ListMuscleGroups(ctx context.Context) ([]string, error)
}

// PreferencesRepository persists per-user weekly schedule preferences.
type PreferencesRepository interface {
	Get(ctx context.Context) (domain.Preferences, error)
	Set(ctx context.Context, prefs domain.Preferences) error
}

// FeatureFlagRepository persists boolean feature toggles by name.
type FeatureFlagRepository interface {
	Get(ctx context.Context, name string) (domain.FeatureFlag, error)
	Set(ctx context.Context, flag domain.FeatureFlag) error
	List(ctx context.Context) ([]domain.FeatureFlag, error)
}

// MuscleGroupTargetRepository serves the static muscle-group weekly volume
// targets used by the planner.
type MuscleGroupTargetRepository interface {
	List(ctx context.Context) ([]domain.MuscleGroupTarget, error)
}

// PushSubscriptionRepository persists per-device Web Push subscriptions.
type PushSubscriptionRepository interface {
	Insert(ctx context.Context, sub domain.PushSubscription) (domain.PushSubscription, error)
	DeleteByEndpoint(ctx context.Context, endpoint string) error
	DeleteByID(ctx context.Context, id int) error
	// DeleteAllByUser removes every subscription for the authenticated user in
	// a single statement. Used by the service layer when the caller asks to
	// delete all of their own devices (e.g. logout).
	DeleteAllByUser(ctx context.Context) error
	ListByUser(ctx context.Context) ([]domain.PushSubscription, error)
	CountByUser(ctx context.Context) (int, error)
}

// ScheduledPushRepository persists pending push fires so they survive
// process restarts. One row per workout_exercise_id (enforced by UNIQUE
// index).
type ScheduledPushRepository interface {
	Replace(ctx context.Context, push domain.ScheduledPush) (domain.ScheduledPush, error)
	Delete(ctx context.Context, id int) error
	DeleteByWorkoutExercise(ctx context.Context, workoutExerciseID int) error
	DeleteByWorkoutSession(ctx context.Context, userID int, date time.Time) error
	Get(ctx context.Context, workoutExerciseID int) (domain.ScheduledPush, error)
	ListAll(ctx context.Context) ([]domain.ScheduledPush, error)
}
