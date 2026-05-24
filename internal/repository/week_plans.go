package repository

import (
	"context"
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
	sunday := monday.AddDate(0, 0, 6)

	sessionRows, err := r.listSessionRowsBetween(ctx, userID, monday, sunday)
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
	setsByDate, err := r.loadExerciseSetsSince(ctx, r.db.ReadOnly, userID, monday)
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
