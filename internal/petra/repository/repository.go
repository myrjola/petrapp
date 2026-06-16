// Package repository contains SQLite-backed implementations of the workout
// domain's data-access contracts. Repositories return domain.* types directly;
// no persistence-shaped intermediate aggregate is exposed to callers. Update
// closures operate on *domain.Session — invariants are enforced via the
// aggregate methods on domain.Session, not by the repository.
package repository

import (
	"github.com/myrjola/petrapp/internal/platform/sqlitekit"
)

// Repositories bundles the per-aggregate SQLite repository handles wired
// together by New.
//
// The fields hold the concrete repositories directly. There is one adapter per
// aggregate (SQLite), and tests exercise it against an in-memory database, so
// there is no second implementation a seam would select between. SQL stays
// hidden by the package boundary, not by an interface: every method takes and
// returns domain.* types and translates sql.ErrNoRows to domain.ErrNotFound, so
// callers never see a persistence type regardless of the field being concrete.
// A consumer that genuinely needs to narrow or fake one repository declares its
// own interface at the call site (see notification.ScheduledPushRepo) — the
// concrete type satisfies it structurally.
type Repositories struct {
	Sessions          *sqliteSessionRepository
	WeekPlans         *sqliteWeekPlanRepository
	Exercises         *sqliteExerciseRepository
	Preferences       *sqlitePreferencesRepository
	FeatureFlags      *sqliteFeatureFlagRepository
	MuscleTargets     *sqliteMuscleGroupTargetRepository
	PushSubscriptions *sqlitePushSubscriptionRepository
	ScheduledPushes   *sqliteScheduledPushRepository
}

// New constructs all eight SQLite-backed repositories. The session repository
// hydrates ExerciseSlot.Exercise inline by joining `exercises` and batching
// muscle-group lookups, so it does not depend on the exercise repository.
func New(db *sqlitekit.Database) *Repositories {
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
