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
// range. The reinsert is a single pass — slot identity is the array index in
// Session.Slots, written into the row's position column, so SQLite
// never auto-assigns a rowid. Domain sentinels returned by fn propagate
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
			// ErrAlreadyExists. UNIQUE violations from saveExerciseSetsInTx propagate as-is —
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
		sess.Slots = setsByDate[formatDate(sess.Date)]
		wp.Sessions[offset] = sess
	}
	return wp, nil
}

// reinsertWeekInTx persists wp's sessions in a single pass. Slot positions
// come from the in-memory array index; the row's position column captures
// it, so SQLite never auto-assigns a rowid and there is nothing to collide.
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
		for pos, slot := range sess.Slots {
			if err := r.saveOneSlotInTx(ctx, tx, sess.Date, pos, slot); err != nil {
				return fmt.Errorf("save slot %d for %s: %w", pos, formatDate(sess.Date), err)
			}
		}
	}
	return nil
}

// isRestDayPlaceholder reports whether sess has no persistent state worth
// writing. Used by Create and reinsertWeekInTx to skip pure rest-day
// placeholders. A non-empty PeriodizationType marks a planner-scheduled day
// even when its exercise pool was exhausted (zero slots): those rows must
// persist so the day round-trips through Get and a later StartSession can
// re-insert without losing the planner's periodization choice (which would
// trip the workout_sessions.periodization_type CHECK constraint).
func isRestDayPlaceholder(sess domain.Session) bool {
	return sess.PeriodizationType == "" &&
		len(sess.Slots) == 0 &&
		sess.StartedAt.IsZero() &&
		sess.CompletedAt.IsZero() &&
		sess.DifficultyRating == nil &&
		!sess.IsDeload
}
