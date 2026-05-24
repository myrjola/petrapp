package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/sqlite"
)

type sqliteWeekPlanRepository struct {
	baseRepository
}

func newSQLiteWeekPlanRepository(db *sqlite.Database) *sqliteWeekPlanRepository {
	return &sqliteWeekPlanRepository{baseRepository: newBaseRepository(db)}
}

// Get loads the WeekPlan for the week beginning on monday. Sessions is always
// length 7; non-scheduled dates carry an empty Session{Date: ...}. Returns
// domain.ErrNotFound when no workout_sessions row exists for the week.
func (r *sqliteWeekPlanRepository) Get(ctx context.Context, monday time.Time) (domain.WeekPlan, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	return r.getInTx(ctx, r.db.ReadOnly, userID, monday)
}

// Update loads the WeekPlan for monday inside a single transaction, runs fn,
// then persists the result via delete-then-reinsert across the week's date
// range. Slot IDs are preserved via INSERT ... RETURNING id (same trick as
// SessionRepository.Update). Domain sentinels returned by fn propagate
// unchanged so callers can errors.Is against them.
func (r *sqliteWeekPlanRepository) Update(
	ctx context.Context, monday time.Time, fn func(*domain.WeekPlan) error,
) (err error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)

	tx, err := r.db.ReadWrite.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("rollback transaction: %w", rollbackErr))
		}
	}()

	wp, err := r.getInTx(ctx, tx, userID, monday)
	if err != nil {
		return fmt.Errorf("get week for update: %w", err)
	}

	if err = fn(&wp); err != nil {
		return err
	}

	if err = r.deleteWeekInTx(ctx, tx, userID, monday); err != nil {
		return fmt.Errorf("delete week for rewrite: %w", err)
	}
	for i := range wp.Sessions {
		sess := wp.Sessions[i]
		if isRestDayPlaceholder(sess) {
			continue
		}
		if err = r.insertSessionInTx(ctx, tx, sess); err != nil {
			return fmt.Errorf("insert session %s: %w", formatDate(sess.Date), err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit week: %w", err)
	}
	return nil
}

// getInTx loads the WeekPlan using q as the queryer. When q is a *sql.Tx the
// read sees a consistent snapshot under SQLite's BEGIN IMMEDIATE locking,
// which is what Update relies on for atomic read-modify-write.
func (r *sqliteWeekPlanRepository) getInTx(
	ctx context.Context, q queryer, userID int, monday time.Time,
) (domain.WeekPlan, error) {
	sunday := monday.AddDate(0, 0, 6)

	sessionRows, err := r.listSessionRowsBetween(ctx, q, userID, monday, sunday)
	if err != nil {
		return domain.WeekPlan{}, fmt.Errorf("list session rows for week: %w", err)
	}
	if len(sessionRows) == 0 {
		return domain.WeekPlan{}, fmt.Errorf(
			"week starting %s: %w", monday.Format(time.DateOnly), domain.ErrNotFound,
		)
	}

	// loadExerciseSetsSince scans WHERE workout_date >= monday with no upper
	// bound, so it may pull in slots from later weeks. That's wasted I/O but
	// functionally fine: we only key into the resulting map by dates that fall
	// inside this week, so out-of-range entries are ignored.
	setsByDate, err := r.loadExerciseSetsSince(ctx, q, userID, monday)
	if err != nil {
		return domain.WeekPlan{}, fmt.Errorf("load exercise sets for week: %w", err)
	}

	wp := domain.WeekPlan{Monday: monday} //nolint:exhaustruct // Sessions initialised below.
	for i := range 7 {
		//nolint:exhaustruct // rest-day placeholder; only Date is meaningful.
		wp.Sessions[i] = domain.Session{Date: monday.AddDate(0, 0, i)}
	}
	for _, sess := range sessionRows {
		offset := int(sess.Date.Sub(monday).Hours() / 24)
		if offset < 0 || offset > 6 {
			continue
		}
		sess.ExerciseSets = setsByDate[formatDate(sess.Date)]
		wp.Sessions[offset] = sess
	}
	return wp, nil
}

// isRestDayPlaceholder reports whether sess has no persistent state worth
// writing (no slots, no lifecycle, no deload flag). Used by Update to skip
// pure rest-day placeholders during the reinsert pass.
func isRestDayPlaceholder(sess domain.Session) bool {
	return len(sess.ExerciseSets) == 0 &&
		sess.StartedAt.IsZero() &&
		sess.CompletedAt.IsZero() &&
		sess.DifficultyRating == nil &&
		!sess.IsDeload
}
