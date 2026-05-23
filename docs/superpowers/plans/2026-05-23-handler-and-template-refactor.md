# Handler & Template Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Four behavior-preserving cleanups in `internal/service`, `cmd/web`, and `ui/templates` to retire the larger-surface smells from the 2026-05-23 audit.

**Architecture:** Three small refactors plus one decision (delete or wire up the unused `internal/tools/query_tool`). No new features.

**Tech Stack:** Go 1.24 stdlib, SQLite (no schema changes), `html/template`.

**Audit notes carried into this plan:**
- **#6 (race in `RegenerateWeeklyPlanIfUnstarted`)**: the existing comment says the design is self-healing via `ResolveWeeklySchedule`. Fly config (`min_machines_running = 0`, no horizontal scaling configured) confirms single-process deployment, so an in-process per-user mutex is a sufficient correctness guard without a heavyweight repo refactor.
- **#7 (handler doing domain work)**: move the swap candidate filter + sort into a `Service.ListSwapCandidates` method. Handlers stay HTTP-shaped.
- **#10 (template status conditionals)**: flatten into precomputed booleans on `dayView` and `workoutAction`. The `dayView` struct already has `ShouldShowProgress` precomputed; do the same for the ribbon attribute and the CTA branching.
- **#13 (`SecureQueryTool`)**: delete. It has tests but zero production callers — no constructor is invoked anywhere outside its own test file. Recover from git if the future need materializes.

---

## Task 1: Per-user serialization for `RegenerateWeeklyPlanIfUnstarted` (#6)

**Files:**
- Modify: `internal/service/service.go` (add a mutex map field + accessor)
- Modify: `internal/service/sessions.go` (acquire the per-user mutex at function entry)
- Test: `internal/service/sessions_test.go` (add a concurrent-call test that asserts no duplicate-session inserts)

The current method:
1. Lists this-week sessions (read).
2. Returns early if any has `StartedAt != zero`.
3. `DeleteWeek` + `generateWeeklyPlan` (write).

If two concurrent calls both pass step 2, both proceed to delete+generate. Outcomes range from "duplicate-row error on the second `CreateBatch`" to "the second caller deletes the first caller's freshly-generated rows." The existing comment claims the design is self-healing via `ResolveWeeklySchedule`. Make it actually correct by serializing per-user; the existing self-heal stays as a belt-and-braces for multi-process deployment if that ever happens.

- [ ] **Step 1: Add the mutex map to `Service`**

In `internal/service/service.go`, modify the `Service` struct (currently lines 31–38) and `NewService` (lines 41–50). Insert the new field and initializer:

```go
// Service coordinates workout-domain operations across the repository
// layer and external integrations. One instance per process; safe for
// concurrent use because each method opens its own DB transaction.
type Service struct {
	repos            *repository.Repositories
	db               *sqlite.Database
	logger           *slog.Logger
	openaiAPIKey     string
	scheduler        PushScheduler // nil-safe; methods no-op when nil.
	maintenanceCache *maintenanceCache
	// userLocks serializes operations that must not race per-user (today:
	// RegenerateWeeklyPlanIfUnstarted). Keyed by user ID; entries are never
	// evicted — the working set is the active-user count, which is small.
	userLocks sync.Map // map[int]*sync.Mutex
}

// NewService creates a new workout service.
func NewService(db *sqlite.Database, logger *slog.Logger, openaiAPIKey string) *Service {
	return &Service{
		repos:            repository.New(db),
		db:               db,
		logger:           logger,
		openaiAPIKey:     openaiAPIKey,
		scheduler:        nil,
		maintenanceCache: newMaintenanceCache(),
		userLocks:        sync.Map{},
	}
}
```

Add `"sync"` to the import block at the top of `internal/service/service.go`.

Add this helper below `NewService`:

```go
// userMutex returns the per-user mutex, creating it on first access.
func (s *Service) userMutex(userID int) *sync.Mutex {
	if m, ok := s.userLocks.Load(userID); ok {
		return m.(*sync.Mutex) //nolint:forcetypeassert // value is always *sync.Mutex.
	}
	m, _ := s.userLocks.LoadOrStore(userID, &sync.Mutex{})
	return m.(*sync.Mutex) //nolint:forcetypeassert // value is always *sync.Mutex.
}
```

Also update `WithScheduler` (line 63–67) so the shallow copy keeps sharing the `userLocks` map. The current `cp := *s` copies the `sync.Map` value — `sync.Map`'s zero value is usable and copying via assignment is safe but copies the internal state, defeating sharing. Replace with field-by-field copy:

```go
// WithScheduler returns a copy of the service wired to a push scheduler.
// Called from main.go after the notification package is initialised. Tests
// that need scheduling behaviour call this with a fake; tests that don't
// leave it nil.
func (s *Service) WithScheduler(scheduler PushScheduler) *Service {
	cp := *s
	cp.scheduler = scheduler
	return &cp
}
```

Wait — `sync.Map` is documented as safe for concurrent use but **must not be copied after first use**. The existing `WithScheduler` does `cp := *s`. If `WithMaintenanceCacheTTL` is called *after* a key has been written, the copy is corrupt. The existing code already has this latent bug — the maintenanceCache field is a pointer, so copying a `*maintenanceCache` is fine; but if we add a `sync.Map` *value*, the copy breaks the new field.

**Decision:** store `*sync.Map` instead of `sync.Map` so the pointer is shared across `With*` copies:

Replace the field and the initializer:

```go
// Service struct field:
	userLocks *sync.Map // map[int]*sync.Mutex; shared across With* copies via pointer.
```

```go
// NewService:
		userLocks:        &sync.Map{},
```

```go
// userMutex helper (unchanged from above except for the dot access):
func (s *Service) userMutex(userID int) *sync.Mutex {
	if m, ok := s.userLocks.Load(userID); ok {
		return m.(*sync.Mutex) //nolint:forcetypeassert // value is always *sync.Mutex.
	}
	m, _ := s.userLocks.LoadOrStore(userID, &sync.Mutex{})
	return m.(*sync.Mutex) //nolint:forcetypeassert // value is always *sync.Mutex.
}
```

`WithScheduler` (and `WithMaintenanceCacheTTL` in `feature_flags.go:69`) need no change — `cp := *s` now copies a pointer.

- [ ] **Step 2: Acquire the per-user mutex in `RegenerateWeeklyPlanIfUnstarted`**

In `internal/service/sessions.go`, replace `RegenerateWeeklyPlanIfUnstarted` (lines 14–44) with:

```go
// RegenerateWeeklyPlanIfUnstarted replaces the current week's generated plan with one
// that reflects the latest preferences, but only when no session has been started yet.
// If any workout this week has a non-zero StartedAt the existing plan is left intact.
//
// The delete and generate steps are NOT wrapped in a single transaction. To
// prevent two concurrent callers from both passing the no-started-session
// check and racing on delete+generate, we serialize per-user via an
// in-process mutex. Multi-process deployments would need a different
// scheme (advisory lock or single-row sentinel); today's deployment is
// single-machine (see fly.toml `min_machines_running = 0`, no horizontal
// scaling configured).
//
// The self-heal via ResolveWeeklySchedule remains as defense-in-depth.
func (s *Service) RegenerateWeeklyPlanIfUnstarted(ctx context.Context) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	mu := s.userMutex(userID)
	mu.Lock()
	defer mu.Unlock()

	monday := domain.MondayOf(time.Now())
	sunday := monday.AddDate(0, 0, 6)

	existing, err := s.repos.Sessions.List(ctx, monday)
	if err != nil {
		return fmt.Errorf("list sessions for week: %w", err)
	}

	for _, sess := range existing {
		if !sess.Date.After(sunday) && !sess.StartedAt.IsZero() {
			return nil
		}
	}

	if err = s.repos.Sessions.DeleteWeek(ctx, monday); err != nil {
		return fmt.Errorf("delete current week: %w", err)
	}
	if err = s.generateWeeklyPlan(ctx, monday); err != nil {
		return fmt.Errorf("generate weekly plan: %w", err)
	}
	return nil
}
```

- [ ] **Step 3: Write the concurrent-call test**

Append to `internal/service/sessions_test.go`:

```go
func Test_RegenerateWeeklyPlanIfUnstarted_ConcurrentCallsSerialized(t *testing.T) {
	ctx, svc := setupTestService(t)

	// 8 concurrent regenerate calls should produce a deterministic result —
	// exactly one set of week sessions, no duplicate-key errors.
	const goroutines = 8
	var (
		wg   sync.WaitGroup
		errs = make(chan error, goroutines)
	)
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			errs <- svc.RegenerateWeeklyPlanIfUnstarted(ctx)
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("RegenerateWeeklyPlanIfUnstarted: %v", err)
		}
	}

	// Verify exactly one week's worth of sessions was created (3 days: Mon/Wed/Fri per setupTestService).
	monday := domain.MondayOf(time.Now())
	sessions, err := svc.ListSessions(ctx, monday)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 3 {
		t.Errorf("after concurrent regenerate: got %d sessions, want 3", len(sessions))
	}
}
```

Imports: `"sync"`, plus existing `"github.com/myrjola/petrapp/internal/domain"` (already in the file).

Note: this test assumes `svc.ListSessions(ctx, monday) ([]domain.Session, error)` exists. If the public surface differs (e.g. the method is called `GetWeekSessions`), check `internal/service/sessions.go` and substitute the right name. The repo-level `s.repos.Sessions.List(ctx, monday)` is the underlying call — if no public service method wraps it, this test instead asserts directly via `svc.Repos().Sessions.List(ctx, monday)`.

- [ ] **Step 4: Run the test**

Run: `go test -race -v ./internal/service -run Test_RegenerateWeeklyPlanIfUnstarted_ConcurrentCallsSerialized`

Expected: PASS. Run with `-race` so any latent data race surfaces.

- [ ] **Step 5: Run the full test suite**

Run: `make test`

Expected: PASS. The existing three `RegenerateWeeklyPlanIfUnstarted` tests in `sessions_test.go` should be unaffected.

- [ ] **Step 6: Run lint-fix**

Run: `make lint-fix`

Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add internal/service/service.go internal/service/sessions.go internal/service/sessions_test.go
git commit -m "service: serialize per-user RegenerateWeeklyPlanIfUnstarted

Two concurrent callers could both pass the no-started-session check
and race on delete+generate, sometimes producing duplicate-key
errors or wiping a freshly-generated week. Add a per-user in-process
mutex map on Service; acquire it for the read-then-write sequence.
Multi-process scaling would need a different scheme, but today's
deployment is single-machine.
"
```

---

## Task 2: Move swap candidate ranking to the service layer (#7)

**Files:**
- Modify: `internal/service/exercises.go` (add `ListSwapCandidates`)
- Modify: `cmd/web/handler-workout.go` (replace the inline filter+sort loop with the service call)
- Test: `internal/service/exercises_test.go` (add a test that exercises the filter + sort)
- Existing handler tests in `cmd/web/handler-workout_swap_test.go` (or similar) should keep passing without modification.

`workoutSwapExerciseGET` (lines 316–397) does three things that belong in the service: filter the exercise list by the existing-in-session set + query string, then sort by `domain.SwapSimilarityScore`. The handler should receive an already-ranked list and only build view cards.

- [ ] **Step 1: Read the handler-side code to confirm the seam**

```bash
sed -n '316,397p' cmd/web/handler-workout.go
```

Confirm the body matches what's in the audit (filter loop at lines 343–364, sort at 366–373, card build at 376–384). Adjust the new service method below if the seam differs.

- [ ] **Step 2: Write the failing service tests**

Append two tests to `internal/service/exercises_test.go`. They drive the seed-exercise pool (loaded by `fixtures.sql` at DB init) rather than seeding new rows, so they don't depend on bespoke fixture helpers.

```go
func Test_ListSwapCandidates_ExcludesSessionExercises(t *testing.T) {
	ctx, svc := setupTestService(t)

	// Trigger plan generation; pick the first day in the week that has any planned exercises.
	if _, err := svc.ResolveWeeklySchedule(ctx); err != nil {
		t.Fatalf("ResolveWeeklySchedule: %v", err)
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	var (
		session     domain.Session
		workoutDate time.Time
		found       bool
	)
	for i := range 7 {
		d := today.AddDate(0, 0, i)
		s, err := svc.GetSession(ctx, d)
		if err == nil && len(s.ExerciseSets) > 0 {
			session, workoutDate, found = s, d, true
			break
		}
	}
	if !found {
		t.Fatal("no workout day with exercises found in this week")
	}
	weID := session.ExerciseSets[0].ID

	current, candidates, err := svc.ListSwapCandidates(ctx, workoutDate, weID, "")
	if err != nil {
		t.Fatalf("ListSwapCandidates: %v", err)
	}
	if current.ID != session.ExerciseSets[0].Exercise.ID {
		t.Errorf("current.ID = %d, want %d", current.ID, session.ExerciseSets[0].Exercise.ID)
	}

	sessionIDs := make(map[int]bool, len(session.ExerciseSets))
	for _, es := range session.ExerciseSets {
		sessionIDs[es.Exercise.ID] = true
	}
	for _, c := range candidates {
		if sessionIDs[c.ID] {
			t.Errorf("candidate %q (id=%d) is already used by the session", c.Name, c.ID)
		}
	}
	if len(candidates) == 0 {
		t.Error("got 0 candidates; seed pool should leave at least one swap option after exclusions")
	}

	// Sort assertion: scores must be monotonically non-increasing.
	for i := 1; i < len(candidates); i++ {
		prev := domain.SwapSimilarityScore(current, candidates[i-1])
		cur := domain.SwapSimilarityScore(current, candidates[i])
		if cur > prev {
			t.Errorf("candidates not sorted by similarity desc at index %d: prev=%d cur=%d", i, prev, cur)
			break
		}
	}
}

func Test_ListSwapCandidates_FiltersByQuery(t *testing.T) {
	ctx, svc := setupTestService(t)
	if _, err := svc.ResolveWeeklySchedule(ctx); err != nil {
		t.Fatalf("ResolveWeeklySchedule: %v", err)
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	var (
		session     domain.Session
		workoutDate time.Time
		found       bool
	)
	for i := range 7 {
		d := today.AddDate(0, 0, i)
		s, err := svc.GetSession(ctx, d)
		if err == nil && len(s.ExerciseSets) > 0 {
			session, workoutDate, found = s, d, true
			break
		}
	}
	if !found {
		t.Fatal("no workout day with exercises found")
	}
	weID := session.ExerciseSets[0].ID

	// Use a substring no plausible exercise contains, so the result must be empty by filter alone.
	_, candidates, err := svc.ListSwapCandidates(ctx, workoutDate, weID, "zzzzzzz")
	if err != nil {
		t.Fatalf("ListSwapCandidates(no-match): %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("query 'zzzzzzz' returned %d candidates; want 0", len(candidates))
	}

	// Sanity case: a query that matches every candidate ('e' is in nearly every English exercise name)
	// returns the same count as the unfiltered call. If the seed pool ever lacks 'e', this asserts the
	// invariant "filter shrinks-or-keeps" instead of exact counts.
	_, all, err := svc.ListSwapCandidates(ctx, workoutDate, weID, "")
	if err != nil {
		t.Fatalf("ListSwapCandidates(unfiltered): %v", err)
	}
	_, eFiltered, err := svc.ListSwapCandidates(ctx, workoutDate, weID, "e")
	if err != nil {
		t.Fatalf("ListSwapCandidates('e'): %v", err)
	}
	if len(eFiltered) > len(all) {
		t.Errorf("'e'-filtered = %d, unfiltered = %d — filter cannot grow the set", len(eFiltered), len(all))
	}
	for _, c := range eFiltered {
		if !strings.Contains(strings.ToLower(c.Name), "e") {
			t.Errorf("'e'-filtered candidate %q does not contain 'e'", c.Name)
		}
	}
}
```

Imports for `exercises_test.go` that may need adding: `"strings"`, `"time"`, `"github.com/myrjola/petrapp/internal/domain"`. Check the existing import block first.

- [ ] **Step 3: Implement `ListSwapCandidates` in the service**

Add to `internal/service/exercises.go`:

```go
// ListSwapCandidates returns the exercises eligible to replace the given
// slot in the given session, filtered by an optional case-insensitive query
// substring and sorted by similarity to the current exercise (descending),
// then by name (ascending). Excludes the current exercise and any exercise
// already used in the same session — those would collide with the UNIQUE
// constraint on workout_exercise.
func (s *Service) ListSwapCandidates(
	ctx context.Context,
	date time.Time,
	workoutExerciseID int,
	query string,
) (domain.Exercise, []domain.Exercise, error) {
	session, err := s.GetSession(ctx, date)
	if err != nil {
		return domain.Exercise{}, nil, fmt.Errorf("get session: %w", err)
	}

	var current domain.Exercise
	currentFound := false
	existing := make(map[int]bool, len(session.ExerciseSets))
	for _, es := range session.ExerciseSets {
		existing[es.Exercise.ID] = true
		if es.ID == workoutExerciseID {
			current = es.Exercise
			currentFound = true
		}
	}
	if !currentFound {
		return domain.Exercise{}, nil, fmt.Errorf("slot %d not in session: %w", workoutExerciseID, domain.ErrSlotNotFound)
	}

	all, err := s.ListExercises(ctx)
	if err != nil {
		return domain.Exercise{}, nil, fmt.Errorf("list exercises: %w", err)
	}

	queryLower := strings.ToLower(query)
	candidates := make([]domain.Exercise, 0, len(all))
	for _, ex := range all {
		if ex.ID == current.ID || existing[ex.ID] {
			continue
		}
		if queryLower != "" && !strings.Contains(strings.ToLower(ex.Name), queryLower) {
			continue
		}
		candidates = append(candidates, ex)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		si := domain.SwapSimilarityScore(current, candidates[i])
		sj := domain.SwapSimilarityScore(current, candidates[j])
		if si != sj {
			return si > sj
		}
		return candidates[i].Name < candidates[j].Name
	})

	return current, candidates, nil
}
```

Add `"sort"`, `"strings"` to the import block in `internal/service/exercises.go` if not already present.

- [ ] **Step 4: Replace the handler-side loop**

In `cmd/web/handler-workout.go`, replace `workoutSwapExerciseGET` (lines 316–397) with:

```go
// workoutSwapExerciseGET handles GET requests to show available exercises for swapping.
func (app *application) workoutSwapExerciseGET(w http.ResponseWriter, r *http.Request) {
	date, ok := app.parseDateParam(w, r)
	if !ok {
		return
	}

	workoutExerciseID, ok := app.parseWorkoutExerciseIDParam(w, r)
	if !ok {
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))

	current, candidates, err := app.service.ListSwapCandidates(r.Context(), date, workoutExerciseID, query)
	if err != nil {
		if errors.Is(err, domain.ErrSlotNotFound) {
			app.notFound(w, r)
			return
		}
		app.serverError(w, r, err)
		return
	}

	dateStr := date.Format("2006-01-02")
	cards := make([]ExerciseResultCardData, 0, len(candidates))
	for _, ex := range candidates {
		cards = append(cards, ExerciseResultCardData{
			Exercise:    ex,
			FormAction:  fmt.Sprintf("/workouts/%s/exercises/%d/swap", dateStr, workoutExerciseID),
			FieldName:   "new_exercise_id",
			ButtonLabel: "Swap to this exercise",
		})
	}

	data := exerciseSwapTemplateData{
		BaseTemplateData:  newBaseTemplateData(r),
		Date:              date,
		Header:            PageHeaderData{Title: "Swap Exercise", Subtitle: ""},
		WorkoutExerciseID: workoutExerciseID,
		CurrentExercise:   current,
		Cards:             cards,
		Query:             query,
	}

	app.render(w, r, http.StatusOK, "exercise-swap", data)
}
```

Drop the now-unused `findExerciseSetInSession` import if this was its only caller — `grep -rn "findExerciseSetInSession" cmd/web` to verify before deleting.

If `sort` was imported only for this handler, drop it from `cmd/web/handler-workout.go`'s import block too. Same for any other now-orphaned imports.

- [ ] **Step 5: Run the new service test and the existing handler tests**

Run: `go test ./internal/service ./cmd/web -run Swap`

Expected: PASS. The handler-level swap tests still drive the same `/swap` endpoint, so they exercise the new path.

- [ ] **Step 6: Run lint-fix**

Run: `make lint-fix`

Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add internal/service/exercises.go internal/service/exercises_test.go cmd/web/handler-workout.go
git commit -m "service: move swap candidate ranking out of the handler

workoutSwapExerciseGET computed the candidate set and similarity rank
inline — exactly the pattern cmd/web/CLAUDE.md prohibits ('don't
recompute domain rules'). Move the filter + sort into a new
Service.ListSwapCandidates; handler only builds view cards.
"
```

---

## Task 3: Flatten day-card status conditionals (#10)

**Files:**
- Modify: `cmd/web/handler-home.go` (`dayView` struct, `workoutAction` struct, `toDays`, `calculateWorkoutAction`)
- Modify: `ui/templates/pages/home/day-cards.gohtml` (the two inline conditionals)
- Existing test: `cmd/web/handler-home_status_test.go` already exercises `StartWorkout` — add assertions for `IsCTA` and the ribbon flag.

The template has two remaining status-comparison chains:

1. Line 241: `{{ if or (eq .Status "today") (eq .Status "in_progress") (eq .Status "completed") (eq .Status "past-incomplete") }}data-ribbon{{ end }}`
2. Lines 253–267: `{{ if eq .Status "today" }}` ... `{{ else if .Action.StartWorkout }}` ... `{{ else }}` ...

Both branch on the `Status` string. Precompute both in `toDays` / `calculateWorkoutAction` as named booleans.

- [ ] **Step 1: Add the new fields to `dayView` and `workoutAction`**

In `cmd/web/handler-home.go`, add `ShouldShowRibbon` to `dayView` (struct currently at lines 128–157):

```go
// dayView represents a single day's view data.
type dayView struct {
	Date               time.Time
	Name               string
	IsToday            bool
	IsPast             bool
	IsScheduled        bool
	Status             string
	StatusLabel        string
	CompletedSets      int
	TotalSets          int
	ProgressPercent    int
	ShouldShowProgress bool
	// ShouldShowRibbon is true when the day-card has a visual ribbon edge
	// (today, in progress, completed, or missed). Precomputed so the
	// template doesn't compare against four status strings.
	ShouldShowRibbon bool
	DifficultyRating *int
	DifficultyStars  []bool
	Action           *workoutAction
}
```

And add `IsCTA` to `workoutAction` (struct currently at lines 159–165):

```go
// workoutAction represents an action that can be taken on a workout day.
type workoutAction struct {
	// StartWorkout indicates whether the action submits to /start (form) vs
	// navigates to the workout page (anchor).
	StartWorkout bool
	// IsCTA renders the primary call-to-action style (large button) used for
	// today's "Start Workout". When false but StartWorkout is true, the
	// action renders as a text-form-button.
	IsCTA bool
	// Label is the button/link text.
	Label string
}
```

- [ ] **Step 2: Populate the new fields**

Modify `calculateWorkoutAction` (lines 240–284) so the `statusToday` arm sets `IsCTA: true`:

```go
func calculateWorkoutAction(status string, isToday bool) *workoutAction {
	switch status {
	case statusUnscheduled:
		if isToday {
			return &workoutAction{StartWorkout: true, IsCTA: false, Label: "Start Extra Workout"}
		}
		return nil
	case statusToday:
		return &workoutAction{StartWorkout: true, IsCTA: true, Label: "Start Workout"}
	case statusInProgress:
		return &workoutAction{StartWorkout: false, IsCTA: false, Label: "Continue Workout"}
	case statusCompleted:
		if isToday {
			return &workoutAction{StartWorkout: false, IsCTA: false, Label: "Review Workout"}
		}
		return &workoutAction{StartWorkout: false, IsCTA: false, Label: "View Details"}
	case statusUpcoming:
		return &workoutAction{StartWorkout: true, IsCTA: false, Label: "Start Early"}
	case statusPastIncomplete:
		return &workoutAction{StartWorkout: true, IsCTA: false, Label: "Start Late"}
	default:
		return nil
	}
}
```

Modify `toDays` (line 286 onward) to compute `ShouldShowRibbon` once. Add this helper above `toDays`:

```go
// shouldShowRibbon returns whether the day-card carries the side-ribbon
// emphasis. Today, in-progress, completed, and missed-past all get the
// ribbon; upcoming and unscheduled don't.
func shouldShowRibbon(status string) bool {
	switch status {
	case statusToday, statusInProgress, statusCompleted, statusPastIncomplete:
		return true
	default:
		return false
	}
}
```

And in the `toDays` loop, populate the new field:

```go
		days[i] = dayView{
			Date:               date,
			Name:               date.Format("Monday"),
			IsToday:            isToday,
			IsPast:             isPast,
			IsScheduled:        isScheduled,
			Status:             status,
			StatusLabel:        statusLabel,
			CompletedSets:      completedSets,
			TotalSets:          totalSets,
			ProgressPercent:    progressPercent,
			ShouldShowProgress: totalSets > 0 && status != statusUnscheduled,
			ShouldShowRibbon:   shouldShowRibbon(status),
			DifficultyRating:   session.DifficultyRating,
			DifficultyStars:    difficultyStars,
			Action:             action,
		}
```

- [ ] **Step 3: Update the template**

In `ui/templates/pages/home/day-cards.gohtml`, replace line 241:

```gohtml
                {{ if .ShouldShowRibbon }}data-ribbon{{ end }}>
```

And replace lines 251–269 (the entire `{{ if .Action }}` block content) with:

```gohtml
                    {{ if .Action }}
                        <div class="day-action">
                            {{ if .Action.IsCTA }}
                                <form method="post" action="/workouts/{{ .Date.Format "2006-01-02" }}/start">
                                    <button type="submit" class="btn day-cta">{{ .Action.Label }}</button>
                                </form>
                            {{ else if .Action.StartWorkout }}
                                <form method="post" action="/workouts/{{ .Date.Format "2006-01-02" }}/start">
                                    <button type="submit" class="day-text-action">
                                        {{ .Action.Label }}<span class="arrow" aria-hidden="true">→</span>
                                    </button>
                                </form>
                            {{ else }}
                                <a href="/workouts/{{ .Date.Format "2006-01-02" }}" class="day-text-action">
                                    {{ .Action.Label }}<span class="arrow" aria-hidden="true">→</span>
                                </a>
                            {{ end }}
                        </div>
                    {{ end }}
```

- [ ] **Step 4: Update the existing status test**

In `cmd/web/handler-home_status_test.go`, the existing table-driven test already asserts on `StartWorkout`. Extend the table struct and the assertions to also cover `IsCTA` and (for `dayView` level) `ShouldShowRibbon`.

Read the file first:

```bash
sed -n '100,200p' cmd/web/handler-home_status_test.go
```

Add `wantIsCTA bool` to the table struct, set it per case (true only for the `statusToday` case), and assert `got.IsCTA == tt.wantIsCTA` alongside the existing `StartWorkout` assertion. If the test exercises `calculateWorkoutAction` directly, no need to test `ShouldShowRibbon` here — it's a one-line wrapper around `shouldShowRibbon`. Add a separate small table test for `shouldShowRibbon`:

```go
func TestShouldShowRibbon(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		status string
		want   bool
	}{
		{statusToday, true},
		{statusInProgress, true},
		{statusCompleted, true},
		{statusPastIncomplete, true},
		{statusUpcoming, false},
		{statusUnscheduled, false},
		{statusNotStarted, false},
	} {
		t.Run(tc.status, func(t *testing.T) {
			t.Parallel()
			if got := shouldShowRibbon(tc.status); got != tc.want {
				t.Errorf("shouldShowRibbon(%q) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 5: Run the home-page tests + render the page to spot-check**

Run: `go test ./cmd/web -run HomeStatus && go test ./cmd/web -run Home`

Expected: PASS. Then manually start the dev server (`make run` or your existing dev command) and load `/` to confirm the ribbon + CTA rendering matches what shipped — this is a UI change and the type/test layer can't catch a visual regression.

- [ ] **Step 6: Run lint-fix and full tests**

Run: `make lint-fix && make test`

Expected: clean + PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/web/handler-home.go cmd/web/handler-home_status_test.go \
        ui/templates/pages/home/day-cards.gohtml
git commit -m "web: flatten day-card status conditionals into precomputed booleans

The template had two status-string comparison chains (4-status OR for
the ribbon attribute, 3-way if/else for the action shape). Move both
into the handler: dayView.ShouldShowRibbon and workoutAction.IsCTA.
Matches the pattern already used by ShouldShowProgress.
"
```

---

## Task 4: Decide the fate of `internal/tools/query_tool.go` (#13)

**Files:**
- Delete: `internal/tools/query_tool.go`
- Delete: `internal/tools/query_tool_test.go`
- Delete: `internal/tools/` (the directory becomes empty after the two files are removed)

`SecureQueryTool` has a complete test suite and zero production callers. It looks like an early prototype that never got wired. Carrying tested-but-unused code costs: it shows up in `make lint-fix` runs, slows tests slightly, and confuses future readers. If the future need materializes, `git log -- internal/tools/` recovers it in one command.

Two-option decision:

**Option A (recommended): delete it.**

**Option B: wire it up.** This means deciding *where* — an admin-only endpoint? A CLI in `cmd/`? Without a use case, deletion is the cheaper bet.

If Option A:

- [ ] **Step 1: Confirm there are no production callers**

```bash
grep -rn "tools.SecureQueryTool\|tools.NewSecureQueryTool" --include="*.go" .
```

Expected: no hits in any file outside `internal/tools/`. The audit confirmed this on 2026-05-23; re-confirm before deletion.

- [ ] **Step 2: Confirm no other file in `internal/tools/` is in use**

```bash
ls internal/tools/
```

Expected: exactly two files — `query_tool.go` and `query_tool_test.go`. If anything else is in the directory, stop and reassess.

- [ ] **Step 3: Delete the files and the directory**

```bash
rm internal/tools/query_tool.go internal/tools/query_tool_test.go
rmdir internal/tools
```

- [ ] **Step 4: Confirm the build and tests still pass**

Run: `go build ./... && make test`

Expected: PASS — nothing depended on this package.

- [ ] **Step 5: Run lint-fix**

Run: `make lint-fix`

Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "tools: delete unused SecureQueryTool

Defined, tested, never wired into any production path. Carrying
tested-but-unused code costs more than it saves. git log restores
the file if the use case ever materializes.
"
```

If Option B is chosen instead: stop this plan and open a separate plan that includes the use case (where it's exposed, who's authorized, what request shape it accepts) — wiring a SQL execution endpoint without that design is a security risk worth a real plan.

---

## Final verification

- [ ] **Step 1: Run the full validation pipeline**

Run: `make ci`

Expected: PASS on init, build, lint-fix (no diff), test, and sec.

- [ ] **Step 2: Confirm clean working tree**

Run: `git status`

Expected: clean.

---

## Out of scope (intentionally deferred)

- All findings handled by sibling plans `2026-05-23-push-correctness-and-observability.md` and `2026-05-23-repository-and-domain-hygiene.md`.
- **Finding #4 (planner uses `time.Now()`)**: dropped — `internal/domain/CLAUDE.md` does not ban `time`.
- **Finding #12 (`selectExercisesForDay` dead)**: false positive — called 5x from `planner_internal_test.go`.
- **Finding #17 (dynamic table names in `userdb.go`)**: deferred per the user's request.
