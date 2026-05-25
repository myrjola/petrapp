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
	WeekPlans         WeekPlanRepository
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
	weekPlans := newSQLiteWeekPlanRepository(db)
	pushSubs := newSQLitePushSubscriptionRepository(db)
	scheduledPushes := newSQLiteScheduledPushRepository(db)
	return &Repositories{
		Preferences:       prefs,
		MuscleTargets:     muscleTargets,
		FeatureFlags:      featureFlags,
		Exercises:         exercises,
		Sessions:          sessions,
		WeekPlans:         weekPlans,
		PushSubscriptions: pushSubs,
		ScheduledPushes:   scheduledPushes,
	}
}

// SessionRepository is the read-only view of workout sessions. Writes are
// owned by WeekPlanRepository — see internal/repository/week_plans.go. The
// reads here serve reporting and per-day handler queries.
type SessionRepository interface {
	Get(ctx context.Context, date time.Time) (domain.Session, error)
	List(ctx context.Context, sinceDate time.Time) ([]domain.Session, error)

	// Read-only specialised queries used by reporting.
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

// WeekPlanRepository persists the full week aggregate. The Update closure
// pattern loads the seven days into a domain.WeekPlan, runs fn under a single
// transaction, and persists the diff on nil via delete-then-reinsert across
// the week's date range. Domain sentinels returned by fn propagate unchanged
// so callers can errors.Is them.
type WeekPlanRepository interface {
	// Get returns the lazily-materialised week. Sessions is always length 7;
	// non-scheduled dates carry an empty Session{Date: ...}. Returns
	// domain.ErrNotFound when no workout_sessions row exists for the week.
	Get(ctx context.Context, monday time.Time) (domain.WeekPlan, error)

	// Update loads the WeekPlan for monday inside a single transaction, runs fn
	// against the hydrated *domain.WeekPlan, and persists the result via
	// delete-then-reinsert across the week's date range. Returning nil from fn
	// commits; returning an error rolls back. Slot identity is the array index
	// in Session.ExerciseSets, persisted as the workout_exercises.position
	// column, so the reinsert is a single pass and no autoincrement collisions
	// are possible. Sentinel errors from domain (e.g. ErrAlreadyStarted)
	// propagate so callers can detect no-op cases via errors.Is.
	Update(ctx context.Context, monday time.Time, fn func(*domain.WeekPlan) error) error

	// Create persists a freshly-planned WeekPlan. Returns domain.ErrAlreadyExists
	// (wrapped) when any session row already exists for the week, so callers can
	// recover from concurrent first-time generation races.
	Create(ctx context.Context, plan domain.WeekPlan) error
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
	Get(ctx context.Context, name domain.FeatureFlagName) (domain.FeatureFlag, error)
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
// process restarts. One row per slot (enforced by a UNIQUE index on
// (workout_user_id, workout_date, position)).
type ScheduledPushRepository interface {
	Replace(ctx context.Context, push domain.ScheduledPush) (domain.ScheduledPush, error)
	Delete(ctx context.Context, id int) error
	DeleteBySlot(ctx context.Context, userID int, date time.Time, pos int) error
	DeleteByWorkoutSession(ctx context.Context, userID int, date time.Time) error
	GetBySlot(ctx context.Context, userID int, date time.Time, pos int) (domain.ScheduledPush, error)
	ListAll(ctx context.Context) ([]domain.ScheduledPush, error)
}
