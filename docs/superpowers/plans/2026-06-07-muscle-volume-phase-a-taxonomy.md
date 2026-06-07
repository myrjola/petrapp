# Muscle-Volume Model — Phase A (Taxonomy) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Clean up the muscle-group taxonomy — split side/rear delts out of the lumped "Shoulders" group, demote Traps/Forearms/Adductors to tag-only (a later-phase concern, no code here), and drop Hip Flexors — without changing the volume/scoring model.

**Architecture:** Two layers change. (1) Domain constants + `RegionFor` in `internal/petra/domain/muscle_group.go`. (2) Seed data in `internal/petra/repository/fixtures.sql` (re-applied every boot; the migration vehicle for muscle-group rows), plus a one-shot in `docs/` that removes obsolete rows from the *production* database (the established pattern — fixtures create desired state and `ON CONFLICT`-update existing rows, but do not delete obsolete prod rows). No `schema.sql` change: table structure is unchanged, only rows.

**Tech Stack:** Go, SQLite (declarative migrator in `internal/platform/sqlitekit`; idempotent `fixtures.sql` seed).

> **⚠️ Scope:** This plan covers **Phase A only**. The design doc (`docs/superpowers/specs/2026-06-07-muscle-volume-model-design.md`) defines four phases: A (taxonomy), B (range targets), D (volume ramp), C (frequency floor). **Plans for B, D, and C are intentionally NOT written yet** — author them once Phase A is implemented and merged, so they can be grounded in the post-A code. See "Follow-up" at the end.

---

## Background facts (verified against current code)

- Muscle-group names are domain constants in `muscle_group.go:37-55`; the only non-test Go reference to `MuscleGroupHipFlexors` is `RegionFor` itself (`muscle_group.go:60-74`). Removing the constant only requires editing `RegionFor`.
- `cmd/petra/handler-home.go:351` calls `RegionFor(v.Name)` to bucket the dashboard; new groups must return a real region (not `RegionOther`).
- The dashboard's muscle-group list comes from `ExerciseRepository.ListMuscleGroups` (`SELECT name FROM muscle_groups ORDER BY name`), so adding/removing rows in `muscle_groups` flows automatically; there is no hardcoded Go list.
- `fixtures.sql` is executed in full on every boot (`sqlitekit.NewDatabase`), using `INSERT … ON CONFLICT … DO UPDATE` for idempotency. It does **not** delete obsolete rows — prod deletions go through a one-shot in `docs/` (see the existing precedent comments at `fixtures.sql:638-641` and `:758-760`).
- `exercise_muscle_groups.muscle_group_name` has `ON DELETE CASCADE` to `muscle_groups.name`, so deleting the `Hip Flexors` muscle-group row removes all its mappings automatically.
- Current relevant `exercise_muscle_groups` rows: `(5,'Shoulders',1)` Lateral Raise, `(9,'Shoulders',0)` Pulldown, `(34,'Shoulders',1)` Face Pull, `(15,'Hip Flexors',0)` Leg Extension, `(21,'Hip Flexors',0)` Plank, `(39,'Hip Flexors',1)` + `(39,'Abs',1)` Hanging Leg Raise (Abs already primary).
- `repos.Exercises.Get(ctx, id)` returns a fully hydrated `domain.Exercise` with `PrimaryMuscleGroups` / `SecondaryMuscleGroups`. `setupTestRepos(t)` (in `helpers_test.go`) returns a fresh, fixture-seeded DB.

## File structure

- **Modify** `internal/petra/domain/muscle_group.go` — add `MuscleGroupSideDelts`, `MuscleGroupRearDelts`; remove `MuscleGroupHipFlexors`; update `RegionFor`.
- **Modify** `internal/petra/domain/muscle_group_test.go` — add a `RegionFor` table test.
- **Modify** `internal/petra/repository/fixtures.sql` — muscle-group rows + exercise mapping remap.
- **Modify** `internal/petra/repository/exercises_test.go` — assert the seeded taxonomy.
- **Create** `docs/2026-06-07-muscle-taxonomy-delts.sql` — one-shot prod cleanup of obsolete rows.

---

## Task 1: Domain constants + RegionFor

**Files:**
- Modify: `internal/petra/domain/muscle_group.go:37-55` (constants), `:60-74` (`RegionFor`)
- Test: `internal/petra/domain/muscle_group_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/petra/domain/muscle_group_test.go`:

```go
func Test_RegionFor_DeltHeads(t *testing.T) {
	t.Parallel()

	cases := map[string]domain.MuscleGroupRegion{
		domain.MuscleGroupSideDelts: domain.RegionUpperPush,
		domain.MuscleGroupRearDelts: domain.RegionUpperPull,
		domain.MuscleGroupShoulders: domain.RegionUpperPush,
		domain.MuscleGroupChest:     domain.RegionUpperPush,
		domain.MuscleGroupLats:      domain.RegionUpperPull,
		domain.MuscleGroupQuads:     domain.RegionLegs,
		"Unknown Muscle":            domain.RegionOther,
	}
	for name, want := range cases {
		if got := domain.RegionFor(name); got != want {
			t.Errorf("RegionFor(%q) = %q, want %q", name, got, want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/petra/domain/ -run Test_RegionFor_DeltHeads`
Expected: FAIL — compile error `undefined: domain.MuscleGroupSideDelts` / `domain.MuscleGroupRearDelts`.

- [ ] **Step 3: Add the constants**

In `internal/petra/domain/muscle_group.go`, add the two constants directly after `MuscleGroupShoulders` and remove the `MuscleGroupHipFlexors` line. The block becomes:

```go
const (
	MuscleGroupChest      = "Chest"
	MuscleGroupShoulders  = "Shoulders"
	MuscleGroupSideDelts  = "Side Delts"
	MuscleGroupRearDelts  = "Rear Delts"
	MuscleGroupTriceps    = "Triceps"
	MuscleGroupUpperBack  = "Upper Back"
	MuscleGroupLats       = "Lats"
	MuscleGroupBiceps     = "Biceps"
	MuscleGroupTraps      = "Traps"
	MuscleGroupForearms   = "Forearms"
	MuscleGroupQuads      = "Quads"
	MuscleGroupHamstrings = "Hamstrings"
	MuscleGroupGlutes     = "Glutes"
	MuscleGroupCalves     = "Calves"
	MuscleGroupAdductors  = "Adductors"
	MuscleGroupAbs        = "Abs"
	MuscleGroupObliques   = "Obliques"
	MuscleGroupLowerBack  = "Lower Back"
)
```

- [ ] **Step 4: Update `RegionFor`**

Replace the `switch` body in `RegionFor` so Side Delts joins push, Rear Delts joins pull, and Hip Flexors is gone:

```go
func RegionFor(muscleGroupName string) MuscleGroupRegion {
	switch muscleGroupName {
	case MuscleGroupChest, MuscleGroupShoulders, MuscleGroupSideDelts, MuscleGroupTriceps:
		return RegionUpperPush
	case MuscleGroupUpperBack, MuscleGroupLats, MuscleGroupBiceps, MuscleGroupTraps,
		MuscleGroupForearms, MuscleGroupRearDelts:
		return RegionUpperPull
	case MuscleGroupQuads, MuscleGroupHamstrings, MuscleGroupGlutes,
		MuscleGroupCalves, MuscleGroupAdductors:
		return RegionLegs
	case MuscleGroupAbs, MuscleGroupObliques, MuscleGroupLowerBack:
		return RegionCore
	default:
		return RegionOther
	}
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/petra/domain/ -run Test_RegionFor_DeltHeads`
Expected: PASS.

- [ ] **Step 6: Confirm the package still builds (no dangling Hip Flexors constant refs)**

Run: `go build ./... && go test ./internal/petra/domain/`
Expected: builds clean; domain tests pass. (Synthetic `"Hip Flexors"` string literals in `planner_internal_test.go` are unaffected — they never used the constant.)

- [ ] **Step 7: Commit**

```bash
git add internal/petra/domain/muscle_group.go internal/petra/domain/muscle_group_test.go
git commit -m "feat(domain): add side/rear delt groups, drop hip flexors"
```

---

## Task 2: Re-seed fixtures + assert the new taxonomy

**Files:**
- Modify: `internal/petra/repository/fixtures.sql` (muscle-group rows `:8-29`; exercise mappings `:624-783`)
- Test: `internal/petra/repository/exercises_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/petra/repository/exercises_test.go`:

```go
func contains(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}

func TestExerciseRepository_DeltTaxonomySeed(t *testing.T) {
	t.Parallel()

	ctx, repos := setupTestRepos(t)

	groups, err := repos.Exercises.ListMuscleGroups(ctx)
	if err != nil {
		t.Fatalf("ListMuscleGroups: %v", err)
	}
	if !contains(groups, "Side Delts") || !contains(groups, "Rear Delts") {
		t.Errorf("muscle groups missing delt heads: %v", groups)
	}
	if contains(groups, "Hip Flexors") {
		t.Errorf("Hip Flexors should be removed: %v", groups)
	}

	// Lateral Raise (5): side-delt prime mover, no longer generic Shoulders.
	lateralRaise, err := repos.Exercises.Get(ctx, 5)
	if err != nil {
		t.Fatalf("Get(5): %v", err)
	}
	if !contains(lateralRaise.PrimaryMuscleGroups, "Side Delts") ||
		contains(lateralRaise.PrimaryMuscleGroups, "Shoulders") {
		t.Errorf("Lateral Raise primaries = %v, want Side Delts and no Shoulders",
			lateralRaise.PrimaryMuscleGroups)
	}

	// Face Pull (34): rear-delt prime mover.
	facePull, err := repos.Exercises.Get(ctx, 34)
	if err != nil {
		t.Fatalf("Get(34): %v", err)
	}
	if !contains(facePull.PrimaryMuscleGroups, "Rear Delts") ||
		contains(facePull.PrimaryMuscleGroups, "Shoulders") {
		t.Errorf("Face Pull primaries = %v, want Rear Delts and no Shoulders",
			facePull.PrimaryMuscleGroups)
	}

	// Seated Cable Row (11): rear delts as a synergist.
	row, err := repos.Exercises.Get(ctx, 11)
	if err != nil {
		t.Fatalf("Get(11): %v", err)
	}
	if !contains(row.SecondaryMuscleGroups, "Rear Delts") {
		t.Errorf("Seated Cable Row secondaries = %v, want Rear Delts",
			row.SecondaryMuscleGroups)
	}

	// Hanging Leg Raise (39): Hip Flexors gone, Abs still prime mover.
	hlr, err := repos.Exercises.Get(ctx, 39)
	if err != nil {
		t.Fatalf("Get(39): %v", err)
	}
	if contains(hlr.PrimaryMuscleGroups, "Hip Flexors") ||
		contains(hlr.SecondaryMuscleGroups, "Hip Flexors") {
		t.Errorf("Hanging Leg Raise still references Hip Flexors: P=%v S=%v",
			hlr.PrimaryMuscleGroups, hlr.SecondaryMuscleGroups)
	}
	if !contains(hlr.PrimaryMuscleGroups, "Abs") {
		t.Errorf("Hanging Leg Raise primaries = %v, want Abs", hlr.PrimaryMuscleGroups)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/petra/repository/ -run TestExerciseRepository_DeltTaxonomySeed`
Expected: FAIL — `Side Delts`/`Rear Delts` absent, Lateral Raise still maps to `Shoulders`, etc.

- [ ] **Step 3: Add the new muscle-group rows**

In `internal/petra/repository/fixtures.sql`, in the `INSERT INTO muscle_groups (name)` block (around `:8`): add `('Side Delts'),` and `('Rear Delts'),` right after `('Shoulders'),`, and delete the `('Hip Flexors'),` line from the Lower Body section. The Lower Body tail becomes:

```sql
-- Lower Body
('Quads'),
('Hamstrings'),
('Glutes'),
('Calves'),
('Adductors') ON CONFLICT(name) DO
UPDATE SET name = excluded.name;
```

- [ ] **Step 4: Remap the exercise→muscle-group rows**

In the `INSERT INTO exercise_muscle_groups …` block (`:624-783`) make exactly these edits:

Change these three existing rows:

```sql
-- was: (5, 'Shoulders', 1),
(5, 'Side Delts', 1),
-- was: (9, 'Shoulders', 0),
(9, 'Rear Delts', 0),
-- was: (34, 'Shoulders', 1),
(34, 'Rear Delts', 1),
```

Add rear-delt synergist rows for the remaining rows/pulldowns/pull-ups (place each next to that exercise's other rows):

```sql
(10, 'Rear Delts', 0),
(11, 'Rear Delts', 0),
(12, 'Rear Delts', 0),
(33, 'Rear Delts', 0),
(24, 'Rear Delts', 0),
```

Delete these three Hip Flexors rows (so fresh DBs never seed them):

```sql
-- delete: (15, 'Hip Flexors', 0),
-- delete: (21, 'Hip Flexors', 0),
-- delete: (39, 'Hip Flexors', 1),
```

Leave `(39, 'Abs', 1)` in place — Abs is already the Hanging Leg Raise prime mover.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/petra/repository/ -run TestExerciseRepository_DeltTaxonomySeed`
Expected: PASS.

- [ ] **Step 6: Run the full repository + migrator suites**

Run: `go test ./internal/petra/repository/ ./internal/platform/sqlitekit/`
Expected: PASS (fixtures still apply cleanly; FK to the new groups resolves because the `muscle_groups` insert runs before `exercise_muscle_groups`).

- [ ] **Step 7: Commit**

```bash
git add internal/petra/repository/fixtures.sql internal/petra/repository/exercises_test.go
git commit -m "feat(repo): seed side/rear delts, remap delt + hip-flexor exercises"
```

---

## Task 3: Production one-shot for obsolete rows

`fixtures.sql` creates the desired state on fresh DBs and updates existing rows, but it cannot remove rows the seed no longer lists. Production still holds `(5,'Shoulders')`, `(9,'Shoulders')`, `(34,'Shoulders')`, and the `Hip Flexors` group with its mappings. This one-shot removes them, following the precedent referenced at `fixtures.sql:638-641`.

**Files:**
- Create: `docs/2026-06-07-muscle-taxonomy-delts.sql`

- [ ] **Step 1: Write the one-shot**

Create `docs/2026-06-07-muscle-taxonomy-delts.sql`:

```sql
-- One-shot prod cleanup for the side/rear-delt taxonomy split (Phase A).
-- fixtures.sql seeds the new desired state on every boot but does not delete
-- rows it no longer lists; this removes the obsolete production rows.
-- Apply once to production via the fly-ops path (make fly-sql-write SCRIPT=...).
-- Idempotent: re-running deletes nothing further.

-- Generic-shoulder rows that moved to a specific delt head.
DELETE FROM exercise_muscle_groups WHERE exercise_id = 5  AND muscle_group_name = 'Shoulders';
DELETE FROM exercise_muscle_groups WHERE exercise_id = 9  AND muscle_group_name = 'Shoulders';
DELETE FROM exercise_muscle_groups WHERE exercise_id = 34 AND muscle_group_name = 'Shoulders';

-- Drop the Hip Flexors group; ON DELETE CASCADE clears its exercise mappings
-- (Leg Extension, Plank, Hanging Leg Raise).
DELETE FROM muscle_groups WHERE name = 'Hip Flexors';
```

- [ ] **Step 2: Sanity-check the SQL parses**

Run: `sqlite3 ":memory:" ".read docs/2026-06-07-muscle-taxonomy-delts.sql"` (expects a no-op error only if tables are absent — acceptable; this just catches syntax typos). If `sqlite3` is unavailable, skip; the statements are plain DML.

- [ ] **Step 3: Commit**

```bash
git add docs/2026-06-07-muscle-taxonomy-delts.sql
git commit -m "docs: one-shot prod cleanup for delt taxonomy split"
```

- [ ] **Step 4: Deploy note**

After this branch is deployed, apply the one-shot to production once via the fly-ops skill (`make fly-sql-write SCRIPT=docs/2026-06-07-muscle-taxonomy-delts.sql`), then verify no exercise still maps to `Hip Flexors` or has both a delt head and generic `Shoulders` where it shouldn't. This step is operational, not part of the merge.

---

## Final verification

- [ ] Run the full suite and linter:

Run: `make lint-fix && make test`
Expected: clean lint, all tests pass.

- [ ] Manual smoke (optional): launch the app and open the home/volume dashboard; confirm "Side Delts" and "Rear Delts" render under their regions and "Hip Flexors" is gone. (Use the `run` skill.)

---

## Self-review against the spec (Phase A scope)

- **Taxonomy split (spec §A):** Side/Rear Delts added (Task 1 constants + Task 2 fixtures); `RegionFor` routes Side→push, Rear→pull (Task 1). ✓
- **Hip Flexors dropped (spec §A):** constant removed (Task 1), seed rows removed (Task 2), prod rows cascade-deleted (Task 3). ✓
- **Fixtures remapping (spec §A):** lateral raise→Side Delts primary; face pull→Rear Delts primary; rows/pulldowns/pull-up→Rear Delts secondary; Hip Flexors mappings removed (Task 2). ✓
- **Tag-only demotion of Traps/Forearms/Adductors:** no code in Phase A — "tag-only" only means "no target row," which is a Phase B (targets) concern. Correctly deferred. ✓
- **No volume/scoring change:** confirmed — `scoreCandidate`, `MuscleGroupTarget`, and targets table are untouched in this phase. ✓
- **Type consistency:** test helper `contains` defined once in Task 2; constant names (`MuscleGroupSideDelts`, `MuscleGroupRearDelts`) consistent across tasks. ✓

---

## Follow-up: write the subsequent-phase plans after Phase A ships

**NOTE (per request): do not write plans for Phases B, D, and C until Phase A is implemented and merged.** Once it is, return to the writing-plans skill and author, in this order (matching the design doc's phasing):

1. **Phase B — Range targets:** `MuscleGroupTarget{MinSets, MaxSets}`, `muscle_group_weekly_targets` schema change (`weekly_sets_target`→`min_sets`, add `max_sets`), seed the full target table (incl. Side/Rear Delts, Calves, Abs, Lower Back), piecewise `scoreCandidate` against a static goal = `MinSets`.
2. **Phase D — Volume ramp:** `MesocycleWeekIndex`, decouple set count from periodization, ramping `goal()`.
3. **Phase C — Frequency floor:** days-touched tracking + scoring bonus.

Grounding each plan in the actual post-A (and post-B, post-D) code is why they're deferred rather than written upfront.
