package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	sqlite3 "github.com/mattn/go-sqlite3"
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
// range. Slot IDs are preserved via INSERT ... RETURNING id, with a two-pass
// reinsert (explicit-ID slots before auto-ID slots) so SQLite's rowid
// assignment never collides with a preserved workout_exercise.id. Domain
// sentinels returned by fn propagate unchanged so callers can errors.Is
// against them.
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
	if err = r.reinsertWeekInTx(ctx, tx, wp); err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit week: %w", err)
	}
	return nil
}

// Create persists a freshly-planned WeekPlan in a single transaction. Returns
// domain.ErrAlreadyExists (wrapped) when any session row already exists for the
// week — callers use errors.Is to fall through to a re-read recovery path.
// Rest-day placeholders (no slots, no lifecycle state) are skipped.
func (r *sqliteWeekPlanRepository) Create(ctx context.Context, plan domain.WeekPlan) (err error) {
	tx, err := r.db.ReadWrite.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("rollback transaction: %w", rollbackErr))
		}
	}()
	for i := range plan.Sessions {
		sess := plan.Sessions[i]
		if isRestDayPlaceholder(sess) {
			continue
		}
		if err = r.insertSessionInTx(ctx, tx, sess); err != nil {
			// Only the workout_sessions PK conflict (duplicate date for this user) maps to
			// ErrAlreadyExists. UNIQUE violations from saveExerciseSets propagate as-is —
			// those are programming errors, not concurrent-insert races.
			var sqliteErr sqlite3.Error
			if errors.As(err, &sqliteErr) && sqliteErr.ExtendedCode == sqlite3.ErrConstraintPrimaryKey {
				return fmt.Errorf("create week starting %s: %w", formatDate(plan.Monday), domain.ErrAlreadyExists)
			}
			return fmt.Errorf("insert session %s: %w", formatDate(sess.Date), err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit create week: %w", err)
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

// reinsertWeekInTx persists wp's sessions in three passes so SQLite's
// INTEGER PRIMARY KEY auto-assignment never collides with a preserved slot ID.
// Pass 1 inserts every workout_sessions row. Pass 2 inserts all explicit-ID
// slots across the week, claiming their workout_exercise.id values. Pass 3
// inserts NULL-ID slots, which SQLite assigns rowids around the now-claimed
// IDs. Mixing pre-existing-ID and new (ID==0) slots across sessions would
// otherwise hit "UNIQUE constraint failed: workout_exercise.id".
func (r *sqliteWeekPlanRepository) reinsertWeekInTx(
	ctx context.Context, tx *sql.Tx, wp domain.WeekPlan,
) error {
	for i := range wp.Sessions {
		sess := wp.Sessions[i]
		if isRestDayPlaceholder(sess) {
			continue
		}
		if err := r.insertSessionRowInTx(ctx, tx, sess); err != nil {
			return fmt.Errorf("insert session row %s: %w", formatDate(sess.Date), err)
		}
	}
	if err := r.reinsertSlotsForWeek(ctx, tx, wp, true); err != nil {
		return err
	}
	return r.reinsertSlotsForWeek(ctx, tx, wp, false)
}

// reinsertSlotsForWeek inserts one ID class of slots across every scheduled
// session in wp. When explicitOnly is true, only slots with a pre-existing
// ID (>0) are inserted; when false, only auto-assign slots (ID==0). Splitting
// into two single-class passes is what avoids the workout_exercise.id rowid
// collision described on reinsertWeekInTx.
func (r *sqliteWeekPlanRepository) reinsertSlotsForWeek(
	ctx context.Context, tx *sql.Tx, wp domain.WeekPlan, explicitOnly bool,
) error {
	label := "new-id"
	if explicitOnly {
		label = "explicit-id"
	}
	for i := range wp.Sessions {
		sess := wp.Sessions[i]
		if isRestDayPlaceholder(sess) {
			continue
		}
		for _, slot := range sess.ExerciseSets {
			if explicitOnly == (slot.ID == 0) {
				continue
			}
			if err := r.saveOneSlotInTx(ctx, tx, sess.Date, slot); err != nil {
				return fmt.Errorf("save %s slot for %s: %w", label, formatDate(sess.Date), err)
			}
		}
	}
	return nil
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
