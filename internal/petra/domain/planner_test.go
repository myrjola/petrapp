package domain_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/petra/domain"
)

// This is the planner's behavioural contract, exercised entirely through the
// public surface (NewPlanner, Plan, PlanDay). Scoring stays a private detail
// of the package; its observable effects — under-target muscles get pulled up,
// over-saturated ones get passed over, ties break to the lowest exercise ID —
// are asserted here through PlanDay's public weekUsedExerciseIDs / weekLoad
// seam. The two pure-function invariants the planner relies on
// (segmentReward ordering, goalForWeek quantisation) are unit-tested in-package
// in planner_scoring_internal_test.go.

// --- Plan: shape and scheduling -------------------------------------------

func TestPlanner_Plan_Validation(t *testing.T) {
	t.Parallel()

	monday := monday2026Date()
	exercises := minimalExercises()
	targets := minimalTargets()

	t.Run("errors for non-Monday start date", func(t *testing.T) {
		t.Parallel()
		wp := domain.NewPlanner(prefs(time.Monday, time.Wednesday), exercises, targets)
		if _, err := wp.Plan(date(monday, 1)); err == nil { // Tuesday.
			t.Error("want error for non-Monday start date, got nil")
		}
	})

	t.Run("errors when no workout days scheduled", func(t *testing.T) {
		t.Parallel()
		wp := domain.NewPlanner(prefs(), exercises, targets)
		if _, err := wp.Plan(monday); err == nil {
			t.Error("want error when no workout days scheduled, got nil")
		}
	})

	t.Run("errors when a scheduled day has no compatible exercises", func(t *testing.T) {
		t.Parallel()
		// Mon+Tue makes Monday a Lower day (tomorrow scheduled); a pool with no
		// Lower exercises must fail.
		upperOnly := []domain.Exercise{
			{ //nolint:exhaustruct // Test exercise omits display fields.
				ID: 1, Category: domain.CategoryUpper, ExerciseType: domain.ExerciseTypeWeighted,
				PrimaryMuscleGroups: []string{"Chest"}, RepMin: new(5), RepMax: new(10)},
		}
		wp := domain.NewPlanner(prefs(time.Monday, time.Tuesday), upperOnly, targets)
		if _, err := wp.Plan(monday); err == nil {
			t.Error("want error when a scheduled day has no compatible exercises, got nil")
		}
	})
}

func TestPlanner_Plan_OneSessionPerScheduledDay(t *testing.T) {
	t.Parallel()

	wp := domain.NewPlanner(prefs(time.Monday, time.Wednesday, time.Friday), minimalExercises(), minimalTargets())
	plan, err := wp.Plan(monday2026Date())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	sessions := planSessions(plan)
	if len(sessions) != 3 {
		t.Fatalf("want 3 populated sessions, got %d", len(sessions))
	}
	want := []time.Weekday{time.Monday, time.Wednesday, time.Friday}
	for i, sess := range sessions {
		if sess.Date.Weekday() != want[i] {
			t.Errorf("session %d weekday = %s, want %s", i, sess.Date.Weekday(), want[i])
		}
	}
	// Unscheduled days carry an empty session (date only, no slots).
	for i := range plan.Sessions {
		s := plan.Sessions[i]
		scheduled := s.Date.Weekday() == time.Monday ||
			s.Date.Weekday() == time.Wednesday ||
			s.Date.Weekday() == time.Friday
		if !scheduled && len(s.Slots) != 0 {
			t.Errorf("rest day %s has %d slots, want 0", s.Date.Weekday(), len(s.Slots))
		}
	}
}

func TestPlanner_Plan_ExerciseCountForDuration(t *testing.T) {
	t.Parallel()

	// A single isolated workout day so it is always CategoryFullBody and draws
	// freely from the pool; its goal is controlled by anchoring on a
	// strength-first or hypertrophy-first week. The count is the observable
	// contract: duration × session goal → exercises per session.
	tests := []struct {
		name    string
		minutes int
		goal    domain.SessionGoal
		want    int
	}{
		{"60 min strength", 60, domain.SessionGoalStrength, 3},
		{"60 min hypertrophy", 60, domain.SessionGoalHypertrophy, 4},
		{"90 min strength", 90, domain.SessionGoalStrength, 4},
		{"90 min hypertrophy", 90, domain.SessionGoalHypertrophy, 5},
		{"45 min strength", 45, domain.SessionGoalStrength, 2},
		{"45 min hypertrophy", 45, domain.SessionGoalHypertrophy, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			monday := mondayWithFirstGoal(t, tt.goal)
			p := domain.Preferences{} //nolint:exhaustruct // Only Wednesday duration matters.
			p.Minutes[time.Wednesday] = tt.minutes
			wp := domain.NewPlanner(p, seedExercises(), seedTargets())

			plan, err := wp.Plan(monday)
			if err != nil {
				t.Fatalf("Plan: %v", err)
			}
			sessions := planSessions(plan)
			if len(sessions) != 1 {
				t.Fatalf("want 1 session, got %d", len(sessions))
			}
			if sessions[0].Goal != tt.goal {
				t.Fatalf("session goal = %s, want %s", sessions[0].Goal, tt.goal)
			}
			if got := len(sessions[0].Slots); got != tt.want {
				t.Errorf("exercise count = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestPlanner_Plan_ConsecutiveSessionsAlternateGoal(t *testing.T) {
	t.Parallel()

	// Mon/Wed/Fri are all isolated FullBody days, so the rich seed pool fills
	// every session and none degrades out of the alternation sequence.
	wp := domain.NewPlanner(prefs(time.Monday, time.Wednesday, time.Friday), seedExercises(), seedTargets())
	plan, err := wp.Plan(monday2026Date())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	sessions := planSessions(plan)
	if len(sessions) < 3 {
		t.Fatalf("need at least 3 sessions, got %d", len(sessions))
	}
	for i := 1; i < len(sessions); i++ {
		if sessions[i].Goal == sessions[i-1].Goal {
			t.Errorf("sessions %d and %d share goal %s; consecutive sessions must alternate",
				i-1, i, sessions[i].Goal)
		}
	}
}

func TestPlanner_Plan_StartingGoalFlipsEachWeek(t *testing.T) {
	t.Parallel()

	wp := domain.NewPlanner(prefs(time.Monday, time.Wednesday, time.Friday), minimalExercises(), minimalTargets())
	monday := monday2026Date()

	firstGoal := func(weekOffset int) domain.SessionGoal {
		plan, err := wp.Plan(date(monday, 7*weekOffset))
		if err != nil {
			t.Fatalf("Plan week %d: %v", weekOffset, err)
		}
		return planSessions(plan)[0].Goal
	}

	if g0, g1 := firstGoal(0), firstGoal(1); g0 == g1 {
		t.Errorf("consecutive weeks must start on alternating goals: both %s", g0)
	}
	if g0, g2 := firstGoal(0), firstGoal(2); g0 != g2 {
		t.Errorf("weeks two apart must start on the same goal: %s vs %s", g0, g2)
	}
}

func TestPlanner_Plan_NoExerciseRepeatsAcrossWeek(t *testing.T) {
	t.Parallel()

	wp := domain.NewPlanner(prefs(time.Monday, time.Tuesday, time.Thursday), minimalExercises(), minimalTargets())
	plan, err := wp.Plan(monday2026Date())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	seen := map[int]bool{}
	for i := range plan.Sessions {
		for _, slot := range plan.Sessions[i].Slots {
			if seen[slot.Exercise.ID] {
				t.Errorf("exercise %d appears in two sessions across the week", slot.Exercise.ID)
			}
			seen[slot.Exercise.ID] = true
		}
	}
}

func TestPlanner_Plan_NoPrimaryMuscleRepeatsWithinSession(t *testing.T) {
	t.Parallel()

	// Three Chest-primary exercises and one Triceps-only: a 3-slot upper session
	// can take at most one Chest pick, so the rest must come from other primaries.
	exercises := []domain.Exercise{
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 1, Category: domain.CategoryUpper, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Chest"}, RepMin: new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 2, Category: domain.CategoryUpper, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Chest", "Triceps"}, RepMin: new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 3, Category: domain.CategoryUpper, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Triceps"}, RepMin: new(5), RepMax: new(10)},
	}
	targets := []domain.MuscleGroupTarget{
		{MuscleGroupName: "Chest", MinSets: 10, MaxSets: 20},
		{MuscleGroupName: "Triceps", MinSets: 8, MaxSets: 16},
	}
	// Mon+Tue → Tuesday is an Upper day (yesterday scheduled).
	wp := domain.NewPlanner(prefs(time.Monday, time.Tuesday), exercises, targets)
	sess, err := wp.PlanDay(date(monday2026Date(), 1), nil, nil)
	if err != nil {
		t.Fatalf("PlanDay: %v", err)
	}
	seenPrimary := map[string]bool{}
	for _, slot := range sess.Slots {
		for _, mg := range slot.Exercise.PrimaryMuscleGroups {
			if seenPrimary[mg] {
				t.Errorf("primary muscle group %q appears in two picks in one session", mg)
			}
			seenPrimary[mg] = true
		}
	}
}

func TestPlanner_Plan_CategoryFollowsAdjacency(t *testing.T) {
	t.Parallel()

	// The adjacency rule, observed through the planned session's WorkoutType.
	// With empty targets the lowest-id tie-break picks the FullBody Plank (id 1)
	// first on FullBody days, so WorkoutType reports the day category cleanly.
	monday := monday2026Date()
	tests := []struct {
		name  string
		prefs domain.Preferences
		date  time.Time
		want  domain.Category
	}{
		{"isolated day is full body", prefs(time.Monday, time.Wednesday, time.Friday), monday, domain.CategoryFullBody},
		{"first of consecutive days is lower", prefs(time.Monday, time.Tuesday), monday, domain.CategoryLower},
		{
			"second of consecutive days is upper",
			prefs(time.Monday, time.Tuesday),
			date(monday, 1),
			domain.CategoryUpper,
		},
		{
			"week wrap: Sunday before Monday is lower",
			prefs(time.Sunday, time.Monday, time.Tuesday),
			date(monday, 6),
			domain.CategoryLower,
		},
		{"week wrap: Monday after Sunday is upper", prefs(time.Sunday, time.Monday), monday, domain.CategoryUpper},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			wp := domain.NewPlanner(tt.prefs, planDayExercises(), nil)
			sess, err := wp.PlanDay(tt.date, nil, nil)
			if err != nil {
				t.Fatalf("PlanDay(%s): %v", tt.date.Weekday(), err)
			}
			if got := sess.WorkoutType(); got != tt.want {
				t.Errorf("WorkoutType(%s) = %s, want %s", tt.date.Weekday(), got, tt.want)
			}
		})
	}
}

func TestPlanner_Plan_SchemeRepsFollowSessionGoal(t *testing.T) {
	t.Parallel()

	// Strength takes the low end of the rep range, hypertrophy the high end.
	bench := domain.Exercise{ //nolint:exhaustruct // Test exercise omits display fields.
		ID: 1, Category: domain.CategoryFullBody, ExerciseType: domain.ExerciseTypeWeighted,
		PrimaryMuscleGroups: []string{"Chest"}, RepMin: new(5), RepMax: new(10)}
	targets := []domain.MuscleGroupTarget{{MuscleGroupName: "Chest", MinSets: 10, MaxSets: 20}}

	for _, tt := range []struct {
		goal     domain.SessionGoal
		wantReps int
	}{
		{domain.SessionGoalStrength, 5},
		{domain.SessionGoalHypertrophy, 10},
	} {
		t.Run(string(tt.goal), func(t *testing.T) {
			t.Parallel()
			monday := mondayWithFirstGoal(t, tt.goal)
			p := domain.Preferences{} //nolint:exhaustruct // Only Wednesday duration matters.
			p.Minutes[time.Wednesday] = 60
			wp := domain.NewPlanner(p, []domain.Exercise{bench}, targets)

			plan, err := wp.Plan(monday)
			if err != nil {
				t.Fatalf("Plan: %v", err)
			}
			sess := planSessions(plan)[0]
			if sess.Goal != tt.goal {
				t.Fatalf("session goal = %s, want %s", sess.Goal, tt.goal)
			}
			for _, set := range sess.Slots[0].Sets {
				if set.TargetValue != tt.wantReps {
					t.Errorf("%s set target reps = %d, want %d", tt.goal, set.TargetValue, tt.wantReps)
				}
			}
		})
	}
}

func TestPlanner_Plan_TimeBasedExerciseGetsDefaultSets(t *testing.T) {
	t.Parallel()

	plank := domain.Exercise{ //nolint:exhaustruct // Test exercise omits display fields.
		ID: 1, Category: domain.CategoryFullBody, ExerciseType: domain.ExerciseTypeTime,
		PrimaryMuscleGroups: []string{"Abs"}, DefaultStartingSeconds: new(30)}
	wp := domain.NewPlanner(prefs(time.Wednesday), []domain.Exercise{plank}, []domain.MuscleGroupTarget{
		{MuscleGroupName: "Abs", MinSets: 4, MaxSets: 8},
	})

	sess, err := wp.PlanDay(date(monday2026Date(), 2), nil, nil)
	if err != nil {
		t.Fatalf("PlanDay: %v", err)
	}
	if len(sess.Slots) != 1 {
		t.Fatalf("want 1 slot, got %d", len(sess.Slots))
	}
	if got := len(sess.Slots[0].Sets); got != 3 {
		t.Errorf("time-based slot has %d sets, want 3", got)
	}
	for _, set := range sess.Slots[0].Sets {
		if set.TargetValue != 30 {
			t.Errorf("target seconds = %d, want 30", set.TargetValue)
		}
	}
}

func TestPlanner_Plan_Deterministic(t *testing.T) {
	t.Parallel()

	// Same inputs twice → identical selections. Guards against map iteration
	// order leaking into selection.
	wp := domain.NewPlanner(prefs90(time.Tuesday, time.Thursday, time.Saturday), seedExercises(), seedTargets())
	monday := monday2026Date()

	planA, err := wp.Plan(monday)
	if err != nil {
		t.Fatalf("Plan A: %v", err)
	}
	planB, err := wp.Plan(monday)
	if err != nil {
		t.Fatalf("Plan B: %v", err)
	}
	for i := range 7 {
		if a, b := slotIDs(planA.Sessions[i]), slotIDs(planB.Sessions[i]); !reflect.DeepEqual(a, b) {
			t.Errorf("day %d differs across runs: A=%v B=%v", i, a, b)
		}
	}
}

func TestPlanner_Plan_HypertrophyDaysGetExtraExercise(t *testing.T) {
	t.Parallel()

	// Two isolated 60-min FullBody days. Strength-first alternation →
	// [strength=3, hypertrophy=4] exercises under the hypertrophy bump rule.
	monday := mondayWithFirstGoal(t, domain.SessionGoalStrength)
	p := domain.Preferences{} //nolint:exhaustruct // Only Monday/Thursday durations matter.
	p.Minutes[time.Monday] = 60
	p.Minutes[time.Thursday] = 60
	wp := domain.NewPlanner(p, minimalExercises(), minimalTargets())

	plan, err := wp.Plan(monday)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	sessions := planSessions(plan)
	if len(sessions) != 2 {
		t.Fatalf("want 2 sessions, got %d", len(sessions))
	}
	wantGoal := []domain.SessionGoal{domain.SessionGoalStrength, domain.SessionGoalHypertrophy}
	wantCount := []int{3, 4}
	for i, sess := range sessions {
		if sess.Goal != wantGoal[i] {
			t.Errorf("session %d goal = %s, want %s", i, sess.Goal, wantGoal[i])
		}
		if got := len(sess.Slots); got != wantCount[i] {
			t.Errorf("session %d (%s) exercise count = %d, want %d", i, sess.Goal, got, wantCount[i])
		}
	}
}

// --- Plan: mesocycle (deload and set-count ramp) --------------------------

func TestPlanner_Plan_DeloadForcesHypertrophyAndReducesSets(t *testing.T) {
	t.Parallel()

	anchor := time.Date(2026, time.April, 6, 0, 0, 0, 0, time.UTC) // Monday, week 0.
	planMonday := anchor.AddDate(0, 0, 21)                         // week 3 of 4 → deload.

	p := domain.Preferences{ //nolint:exhaustruct // RestNotificationsEnabled irrelevant.
		Minutes:         [7]int{time.Monday: 60, time.Tuesday: 60},
		DeloadEnabled:   true,
		MesocycleLength: 4,
		MesocycleAnchor: anchor,
	}
	exercises := []domain.Exercise{
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 1, Name: "Bench Press", Category: domain.CategoryUpper, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"chest"}, RepMin: new(8), RepMax: new(12)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 2, Name: "Squat", Category: domain.CategoryLower, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"quads"}, RepMin: new(8), RepMax: new(12)},
	}
	targets := []domain.MuscleGroupTarget{
		{MuscleGroupName: "chest", MinSets: 6, MaxSets: 12},
		{MuscleGroupName: "quads", MinSets: 6, MaxSets: 12},
	}
	wp := domain.NewPlanner(p, exercises, targets)
	plan, err := wp.Plan(planMonday)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	sessions := planSessions(plan)
	if len(sessions) == 0 {
		t.Fatal("expected at least one session")
	}
	for _, s := range sessions {
		if !s.IsDeload {
			t.Errorf("session %s IsDeload = false, want true", s.Date.Format("2006-01-02"))
		}
		if s.Goal != domain.SessionGoalHypertrophy {
			t.Errorf("session %s goal = %s, want hypertrophy", s.Date.Format("2006-01-02"), s.Goal)
		}
		for _, slot := range s.Slots {
			// Normal high-rep band is 3 sets; deload drops to 2.
			if len(slot.Sets) != 2 {
				t.Errorf("session %s exercise %s: %d sets, want 2 (deload drops one set)",
					s.Date.Format("2006-01-02"), slot.Exercise.Name, len(slot.Sets))
			}
			for _, set := range slot.Sets {
				if set.TargetValue != 12 {
					t.Errorf("set target reps = %d, want 12 (repMax for hypertrophy)", set.TargetValue)
				}
			}
		}
	}
}

func TestPlanner_Plan_NonDeloadWeekIsNotDeload(t *testing.T) {
	t.Parallel()

	anchor := time.Date(2026, time.April, 6, 0, 0, 0, 0, time.UTC)
	planMonday := anchor.AddDate(0, 0, 7) // week 1 → not a deload.

	p := domain.Preferences{ //nolint:exhaustruct // RestNotificationsEnabled irrelevant.
		Minutes:         [7]int{time.Monday: 60},
		DeloadEnabled:   true,
		MesocycleLength: 4,
		MesocycleAnchor: anchor,
	}
	exercises := []domain.Exercise{
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 1, Name: "Bench", Category: domain.CategoryFullBody, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"chest"}, RepMin: new(8), RepMax: new(12)},
	}
	wp := domain.NewPlanner(
		p,
		exercises,
		[]domain.MuscleGroupTarget{{MuscleGroupName: "chest", MinSets: 3, MaxSets: 6}},
	)
	plan, err := wp.Plan(planMonday)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	for _, s := range planSessions(plan) {
		if s.IsDeload {
			t.Errorf("session %s IsDeload = true, want false (week 1 is not a deload)", s.Date.Format("2006-01-02"))
		}
	}
}

func TestPlanner_Plan_SetCountRampsAcrossBlockAndDropsOnDeload(t *testing.T) {
	t.Parallel()

	anchor := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC) // Monday, week 0.
	p := prefs(time.Monday, time.Wednesday, time.Friday)
	p.DeloadEnabled = true
	p.MesocycleLength = 4
	p.MesocycleAnchor = anchor

	// A single rep-based exercise so every planned slot's set count is week-driven.
	bench := domain.Exercise{ //nolint:exhaustruct // Test exercise omits display fields.
		ID: 1, Name: "Bench", Category: domain.CategoryFullBody, ExerciseType: domain.ExerciseTypeWeighted,
		PrimaryMuscleGroups: []string{"Chest"}, RepMin: new(8), RepMax: new(12)}
	wp := domain.NewPlanner(p, []domain.Exercise{bench}, []domain.MuscleGroupTarget{
		{MuscleGroupName: "Chest", MinSets: 10, MaxSets: 20},
	})

	setsOnMonday := func(weekOffset int) int {
		plan, err := wp.Plan(anchor.AddDate(0, 0, 7*weekOffset))
		if err != nil {
			t.Fatalf("Plan week %d: %v", weekOffset, err)
		}
		sess := plan.Sessions[0] // Monday slot.
		if len(sess.Slots) == 0 {
			t.Fatalf("week %d Monday has no slots", weekOffset)
		}
		return len(sess.Slots[0].Sets)
	}

	week0, week2, week3 := setsOnMonday(0), setsOnMonday(2), setsOnMonday(3)
	if week0 != 3 {
		t.Errorf("week 0 set count = %d, want base 3", week0)
	}
	if week2 != 4 {
		t.Errorf("week 2 (peak) set count = %d, want peak 4", week2)
	}
	if week3 >= week2 {
		t.Errorf("deload week set count (%d) should drop below peak (%d)", week3, week2)
	}
}

// --- Scoring-driven selection (via PlanDay's public weekLoad / used seam) ---

func TestPlanner_PlanDay_PrefersUnderTargetMuscle(t *testing.T) {
	t.Parallel()

	// Two equally-eligible FullBody exercises. Whichever muscle is already loaded
	// loses to the muscle still under its floor.
	exercises := []domain.Exercise{
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 1, Category: domain.CategoryFullBody, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Shoulders"}, RepMin: new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 2, Category: domain.CategoryFullBody, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Chest"}, RepMin: new(5), RepMax: new(10)},
	}
	targets := []domain.MuscleGroupTarget{
		{MuscleGroupName: "Shoulders", MinSets: 10, MaxSets: 20},
		{MuscleGroupName: "Chest", MinSets: 10, MaxSets: 20},
	}
	wp := domain.NewPlanner(prefs(time.Tuesday), exercises, targets)
	tue := date(monday2026Date(), 1)

	// Shoulders already at floor → the Chest exercise (id 2) must lead.
	sess, err := wp.PlanDay(tue, nil, map[string]float64{"Shoulders": 10})
	if err != nil {
		t.Fatalf("PlanDay: %v", err)
	}
	if len(sess.Slots) == 0 {
		t.Fatal("no slots picked")
	}
	if sess.Slots[0].Exercise.ID != 2 {
		t.Errorf("first pick is exercise %d; want exercise 2 (Chest, under target)", sess.Slots[0].Exercise.ID)
	}
}

func TestPlanner_PlanDay_OverSaturatedMuscleLosesToFreshMuscle(t *testing.T) {
	t.Parallel()

	exercises := []domain.Exercise{
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 1, Category: domain.CategoryFullBody, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Biceps"}, RepMin: new(8), RepMax: new(12)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 2, Category: domain.CategoryFullBody, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Chest"}, RepMin: new(8), RepMax: new(12)},
	}
	targets := []domain.MuscleGroupTarget{
		{MuscleGroupName: "Biceps", MinSets: 8, MaxSets: 16},
		{MuscleGroupName: "Chest", MinSets: 10, MaxSets: 20},
	}
	wp := domain.NewPlanner(prefs(time.Tuesday), exercises, targets)
	tue := date(monday2026Date(), 1)

	// Biceps already well past its ceiling; Chest fresh. Chest (id 2) must lead.
	sess, err := wp.PlanDay(tue, nil, map[string]float64{"Biceps": 30})
	if err != nil {
		t.Fatalf("PlanDay: %v", err)
	}
	if len(sess.Slots) == 0 {
		t.Fatal("no slots picked")
	}
	if sess.Slots[0].Exercise.ID != 2 {
		t.Errorf("first pick is exercise %d; want exercise 2 (Chest, fresh muscle beats over-saturated Biceps)",
			sess.Slots[0].Exercise.ID)
	}
}

func TestPlanner_PlanDay_AvoidsUsedExercises(t *testing.T) {
	t.Parallel()

	// Mon alone makes Tuesday an Upper day (yesterday scheduled, tomorrow not);
	// mark two of the three upper picks as used and the session must avoid them.
	wp := domain.NewPlanner(prefs(time.Monday), planDayExercises(), nil)
	tue := date(monday2026Date(), 1)
	used := map[int]bool{2: true, 3: true}
	preUsed := map[int]bool{2: true, 3: true} // PlanDay mutates used; snapshot first.

	sess, err := wp.PlanDay(tue, used, nil)
	if err != nil {
		t.Fatalf("PlanDay: %v", err)
	}
	for _, slot := range sess.Slots {
		if preUsed[slot.Exercise.ID] {
			t.Errorf("PlanDay returned already-used exercise id=%d", slot.Exercise.ID)
		}
	}
}

func TestPlanner_PlanDay_TieBreakPicksLowestID(t *testing.T) {
	t.Parallel()

	// Empty targets → every candidate scores 0 → deterministic lowest-id pick.
	exercises := []domain.Exercise{
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 7, Category: domain.CategoryUpper, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Chest"}, RepMin: new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 3, Category: domain.CategoryUpper, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Shoulders"}, RepMin: new(5), RepMax: new(10)},
	}
	// Mon alone → Tuesday is Upper (yesterday scheduled, tomorrow not).
	wp := domain.NewPlanner(prefs(time.Monday), exercises, nil)
	sess, err := wp.PlanDay(date(monday2026Date(), 1), nil, nil)
	if err != nil {
		t.Fatalf("PlanDay: %v", err)
	}
	if len(sess.Slots) == 0 {
		t.Fatal("no slots picked")
	}
	if sess.Slots[0].Exercise.ID != 3 {
		t.Errorf("first pick is exercise %d; want exercise 3 (lowest id among ties)", sess.Slots[0].Exercise.ID)
	}
}

func TestPlanner_PlanDay_GracefulDegradationWhenPrimariesExhausted(t *testing.T) {
	t.Parallel()

	// All three exercises are Chest-primary; only one can be picked without a
	// primary-overlap, so the session degrades to a single slot rather than
	// looping forever.
	exercises := []domain.Exercise{
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 1, Category: domain.CategoryUpper, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Chest"}, RepMin: new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 2, Category: domain.CategoryUpper, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Chest"}, RepMin: new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 3, Category: domain.CategoryUpper, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Chest"}, RepMin: new(5), RepMax: new(10)},
	}
	// Mon alone → Tuesday is Upper (yesterday scheduled, tomorrow not).
	wp := domain.NewPlanner(prefs(time.Monday), exercises, []domain.MuscleGroupTarget{
		{MuscleGroupName: "Chest", MinSets: 10, MaxSets: 20},
	})
	sess, err := wp.PlanDay(date(monday2026Date(), 1), nil, nil) // Upper day, wants 3.
	if err != nil {
		t.Fatalf("PlanDay: %v", err)
	}
	if len(sess.Slots) != 1 {
		t.Errorf("want 1 slot (graceful degradation under primary-overlap exhaustion), got %d", len(sess.Slots))
	}
}

func TestPlanner_Plan_BalancesMuscleGroupVolumeTowardTargets(t *testing.T) {
	t.Parallel()

	// Tue/Thu/Sat 90 min, all FullBody — the seed user's schedule. Every targeted
	// muscle should reach at least 0.7× its floor (regression: Chest used to sit
	// starved) and stay within a small slack of its ceiling.
	wp := domain.NewPlanner(prefs90(time.Tuesday, time.Thursday, time.Saturday), seedExercises(), seedTargets())
	plan, err := wp.Plan(monday2026Date())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	volume := domain.WeeklyPlannedVolume(planSessions(plan))
	const ceilingSlack = 4.0
	for _, target := range seedTargets() {
		got := volume[target.MuscleGroupName]
		if lower := 0.7 * float64(target.MinSets); got < lower {
			t.Errorf("%s planned %.1f is below 0.7×floor (%.1f)", target.MuscleGroupName, got, lower)
		}
		if got > float64(target.MaxSets)+ceilingSlack {
			t.Errorf("%s planned %.1f exceeds ceiling %d + slack %.0f",
				target.MuscleGroupName, got, target.MaxSets, ceilingSlack)
		}
	}
}

// --- PlanDay: parity with Plan and error surface --------------------------

func TestPlanner_PlanDay_IsolatedDateDefaultsToFullBodyAndMediumCount(t *testing.T) {
	t.Parallel()

	p := domain.Preferences{} //nolint:exhaustruct // empty prefs on purpose — isolated day.
	wp := domain.NewPlanner(p, planDayExercises(), nil)
	wed := date(monday2026Date(), 2)

	sess, err := wp.PlanDay(wed, nil, nil)
	if err != nil {
		t.Fatalf("PlanDay: %v", err)
	}
	if sess.WorkoutType() != domain.CategoryFullBody {
		t.Errorf("WorkoutType = %s, want full body", sess.WorkoutType())
	}
	if len(sess.Slots) != 3 { // default count for an unscheduled day.
		t.Errorf("slots = %d, want 3", len(sess.Slots))
	}
}

func TestPlanner_PlanDay_UsesPrefsExerciseCountWhenScheduled(t *testing.T) {
	t.Parallel()

	p := domain.Preferences{} //nolint:exhaustruct // only Wednesday duration is relevant.
	p.Minutes[time.Wednesday] = 90
	wp := domain.NewPlanner(p, planDayExercises(), nil)

	sess, err := wp.PlanDay(date(monday2026Date(), 2), nil, nil)
	if err != nil {
		t.Fatalf("PlanDay: %v", err)
	}
	if len(sess.Slots) != 4 { // 90-min strength → exercisesLong.
		t.Errorf("slots = %d, want 4 (long day)", len(sess.Slots))
	}
}

func TestPlanner_PlanDay_SessionGoalMatchesWeeklyPlanner(t *testing.T) {
	t.Parallel()

	// PlanDay must reproduce Plan's per-day goal for every scheduled day,
	// including Sunday (regression for the Sunday=0 weekday-arithmetic bug).
	for _, days := range [][]time.Weekday{
		{time.Monday, time.Wednesday, time.Friday},
		{time.Monday, time.Sunday},
	} {
		wp := domain.NewPlanner(prefs(days...), planDayExercises(), nil)
		weekly, err := wp.Plan(monday2026Date())
		if err != nil {
			t.Fatalf("Plan: %v", err)
		}
		for i := range weekly.Sessions {
			want := weekly.Sessions[i]
			if len(want.Slots) == 0 {
				continue
			}
			var got domain.Session
			got, err = wp.PlanDay(want.Date, nil, nil)
			if err != nil {
				t.Fatalf("PlanDay(%s): %v", want.Date.Weekday(), err)
			}
			if got.Goal != want.Goal {
				t.Errorf("PlanDay(%s) goal = %s, want %s (matches weekly planner)",
					want.Date.Weekday(), got.Goal, want.Goal)
			}
		}
	}
}

func TestPlanner_PlanDay_EmptyCategoryPoolReturnsError(t *testing.T) {
	t.Parallel()

	// Mon+Tue makes Monday a Lower day; a pool with no Lower exercises must error.
	all := planDayExercises()
	noLower := make([]domain.Exercise, 0, len(all))
	for _, ex := range all {
		if ex.Category != domain.CategoryLower {
			noLower = append(noLower, ex)
		}
	}
	wp := domain.NewPlanner(prefs(time.Monday, time.Tuesday), noLower, nil)

	// The sentinel (errNoExercisesForCategory) is unexported and purely internal,
	// so the public contract is simply: an empty category pool yields an error.
	if _, err := wp.PlanDay(monday2026Date(), nil, nil); err == nil {
		t.Fatal("PlanDay must error when the category pool is empty")
	}
}

// --- Exported date helpers (live in planner.go) ---------------------------

func TestMondayOf_UsesLocalCalendarAnchoredToUTC(t *testing.T) {
	t.Parallel()

	helsinki, err := time.LoadLocation("Europe/Helsinki")
	if err != nil {
		t.Fatalf("load Europe/Helsinki: %v", err)
	}
	tests := []struct {
		name string
		in   time.Time
		want time.Time
	}{
		{
			name: "early-morning Sunday in EEST returns previous Monday",
			in:   time.Date(2026, 5, 3, 0, 32, 41, 0, helsinki),
			want: time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "Monday after midnight EEST returns same Monday",
			in:   time.Date(2026, 5, 4, 0, 32, 0, 0, helsinki),
			want: time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "Monday afternoon UTC returns same Monday",
			in:   time.Date(2026, 4, 27, 14, 0, 0, 0, time.UTC),
			want: time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "Tuesday 02:00 EEST returns same week's Monday",
			in:   time.Date(2026, 4, 28, 2, 0, 0, 0, helsinki),
			want: time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "Saturday late evening UTC returns same week's Monday",
			in:   time.Date(2026, 5, 2, 23, 59, 0, 0, time.UTC),
			want: time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := domain.MondayOf(tt.in)
			if !got.Equal(tt.want) {
				t.Errorf("MondayOf(%s) = %s, want %s", tt.in, got, tt.want)
			}
			if got.Weekday() != time.Monday {
				t.Errorf("MondayOf(%s).Weekday() = %s, want Monday", tt.in, got.Weekday())
			}
		})
	}
}

func TestStartOfDay_TruncatesToUTCMidnight(t *testing.T) {
	t.Parallel()

	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	cases := []struct {
		name string
		in   time.Time
		want time.Time
	}{
		{
			name: "UTC noon collapses to UTC midnight",
			in:   time.Date(2026, 5, 24, 12, 30, 0, 0, time.UTC),
			want: time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "UTC midnight is fixed point",
			in:   time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC),
			want: time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "local late-evening uses local calendar date",
			in:   time.Date(2026, 5, 24, 23, 30, 0, 0, loc),
			want: time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := domain.StartOfDay(tc.in)
			if !got.Equal(tc.want) {
				t.Errorf("StartOfDay(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// mondayWithFirstGoal returns a Monday in 2026 whose week starts on the given
// session goal, so a single-day Plan deterministically gets that goal. The
// starting goal flips each calendar week, so at most one step is needed.
func mondayWithFirstGoal(t *testing.T, goal domain.SessionGoal) time.Time {
	t.Helper()
	monday := monday2026Date()
	// One isolated day; read back its goal and step a week if it's the other one.
	p := domain.Preferences{} //nolint:exhaustruct // only Wednesday matters.
	p.Minutes[time.Wednesday] = 60
	wp := domain.NewPlanner(p, minimalExercises(), minimalTargets())
	for range 2 {
		plan, err := wp.Plan(monday)
		if err != nil {
			t.Fatalf("mondayWithFirstGoal Plan: %v", err)
		}
		if planSessions(plan)[0].Goal == goal {
			return monday
		}
		monday = monday.AddDate(0, 0, 7)
	}
	t.Fatalf("could not find a Monday with first goal %s", goal)
	return time.Time{}
}
