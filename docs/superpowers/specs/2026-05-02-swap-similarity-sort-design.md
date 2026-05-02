# Swap Exercise — Similarity-Based Sort

## Overview

The Swap Exercise page (`/workouts/{date}/exercises/{id}/swap`) currently lists
candidate exercises in whatever order the database returns them, optionally
filtered by a name search. Users have to scan or search to find an exercise
that trains the same muscles as the one they're swapping out.

Sort the list so exercises that hit the same primary and secondary muscle
groups as the current exercise rise to the top. Same-category exercises get
a small additional bump. The full list is preserved (no filtering on score),
ordering is silent (no badges, no sections), and the existing name search
keeps working — it narrows the list, similarity orders what remains.

The intent: a user who can't perform the planned exercise (e.g. equipment
unavailable) can grab the next-best variety match without thinking.

---

## Key design decisions

| Decision | Choice | Reason |
|----------|--------|--------|
| Ranking signal | Muscle-group overlap, weighted by primary/secondary, plus same-category bonus | Matches the user's mental model of "hits the same area" |
| Score shape | Sum of integer weights | Transparent, easy to test, easy to tune by editing four constants |
| Score visibility | Silent — sort only, no badges or sections | The list ordering itself communicates the ranking; extra UI adds noise |
| Filter behavior | Show every non-conflicting exercise | A weak match is still reachable by scrolling; nothing should be hidden |
| Search interaction | Search filters by name, similarity orders the survivors | Both behaviors are useful and don't conflict |
| Tie-break | Alphabetical by name, via `sort.SliceStable` | Deterministic order for tests and a predictable user experience |
| Placement | New `internal/workout/swap.go` with a pure function | Self-contained unit, no service-layer state, natural home for table-driven tests |

---

## Scoring formula

```
SwapSimilarityScore(current, candidate) =
    4 * |current.Primary  ∩ candidate.Primary|
  + 2 * |current.Primary  ∩ candidate.Secondary|
  + 2 * |current.Secondary ∩ candidate.Primary|
  + 1 * |current.Secondary ∩ candidate.Secondary|
  + (3 if current.Category == candidate.Category else 0)
```

Worked example — current is Bench Press (primary: Chest, Triceps; secondary:
Shoulders), all candidates are upper-body:

| Candidate | Primary | Secondary | Score | Breakdown |
|-----------|---------|-----------|-------|-----------|
| Dumbbell Bench | Chest, Triceps | Shoulders | **12** | 4+4 (P∩P) + 1 (S∩S) + 3 (cat) |
| Incline Press | Chest, Shoulders | Triceps | **11** | 4 (P∩P: Chest) + 2 (P∩S: Shoulders) + 2 (S∩P: Triceps) + 3 (cat) |
| Push-Ups | Chest | Triceps, Shoulders | **10** | 4 (P∩P: Chest) + 2 (P∩S: Triceps) + 1 (S∩S: Shoulders) + 3 (cat) |
| Squat (lower) | Quads, Glutes | Hamstrings | **0** | No muscle overlap, different category |

Score is non-negative. Disjoint exercises in different categories score `0`.

The four weights (4, 2, 1, 3) are dials — change them in one place if the
ordering feels wrong in practice.

---

## Implementation plan

### `internal/workout/swap.go` (new file)

```go
package workout

// SwapSimilarityScore returns a non-negative integer where higher means
// "better candidate to swap current for". Pure function, no I/O.
//
// Weights:
//   - primary ∩ primary:     +4 per shared muscle
//   - primary ∩ secondary:   +2 per shared muscle (both directions)
//   - secondary ∩ secondary: +1 per shared muscle
//   - same category:         +3 flat bonus
func SwapSimilarityScore(current, candidate Exercise) int {
    score := 0
    score += 4 * countShared(current.PrimaryMuscleGroups, candidate.PrimaryMuscleGroups)
    score += 2 * countShared(current.PrimaryMuscleGroups, candidate.SecondaryMuscleGroups)
    score += 2 * countShared(current.SecondaryMuscleGroups, candidate.PrimaryMuscleGroups)
    score += 1 * countShared(current.SecondaryMuscleGroups, candidate.SecondaryMuscleGroups)
    if current.Category == candidate.Category {
        score += 3
    }
    return score
}

func countShared(a, b []string) int {
    if len(a) == 0 || len(b) == 0 {
        return 0
    }
    set := make(map[string]struct{}, len(a))
    for _, m := range a {
        set[m] = struct{}{}
    }
    n := 0
    for _, m := range b {
        if _, ok := set[m]; ok {
            n++
        }
    }
    return n
}
```

Symmetry: `SwapSimilarityScore(a, b) == SwapSimilarityScore(b, a)` because
the four `countShared` terms cover both directions of the primary↔secondary
cross and the category check is symmetric.

### `cmd/web/handler-workout.go` — `workoutSwapExerciseGET`

Existing filter loop stays as-is. After the loop, sort the slice:

```go
sort.SliceStable(compatibleExercises, func(i, j int) bool {
    si := workout.SwapSimilarityScore(currentSlot.Exercise, compatibleExercises[i])
    sj := workout.SwapSimilarityScore(currentSlot.Exercise, compatibleExercises[j])
    if si != sj {
        return si > sj
    }
    return compatibleExercises[i].Name < compatibleExercises[j].Name
})
```

`sort` is added to the import block. No other handler changes.

### Templates

No changes. The template ranges over `CompatibleExercises` in the order it
receives them; reordering the slice in the handler reorders the rendered list.

### Search interaction

The query filter (`strings.Contains(strings.ToLower(exercise.Name), queryLower)`)
runs before the sort, so similarity orders only the surviving name matches.
No special-casing needed.

---

## Testing

### Unit tests — `internal/workout/swap_test.go` (new file)

Table-driven, exercising every weight independently and the bonus:

| Case | Current | Candidate | Expected |
|------|---------|-----------|----------|
| Identical, same category | P:{A,B} S:{C} cat:upper | P:{A,B} S:{C} cat:upper | 4+4+1+3 = 12 |
| One primary match, same category | P:{A} S:{} cat:upper | P:{A} S:{} cat:upper | 4+3 = 7 |
| Primary↔secondary, same category | P:{A} S:{} cat:upper | P:{} S:{A} cat:upper | 2+3 = 5 |
| Secondary↔primary, same category | P:{} S:{A} cat:upper | P:{A} S:{} cat:upper | 2+3 = 5 |
| Secondary↔secondary, same category | P:{} S:{A} cat:upper | P:{} S:{A} cat:upper | 1+3 = 4 |
| Disjoint, same category | P:{A} S:{} cat:upper | P:{B} S:{} cat:upper | 3 |
| Disjoint, different category | P:{A} S:{} cat:upper | P:{B} S:{} cat:lower | 0 |
| Empty slices, different category | P:{} S:{} cat:upper | P:{} S:{} cat:lower | 0 |
| Empty slices, same category | P:{} S:{} cat:upper | P:{} S:{} cat:upper | 3 |
| Symmetry | (a couple of pairs from above) | (swapped) | unchanged |

### Handler test — `cmd/web/handler-workout_test.go` (or wherever swap GET tests live)

E2e-style. Hit `GET /workouts/{date}/exercises/{id}/swap` with a seeded
session whose current slot is a known upper-body exercise. Parse the
rendered HTML; assert that:

1. A seeded same-category, same-primary-muscle exercise appears in the list
   *before* a seeded different-category exercise with no muscle overlap.
2. The same exercise appears before a same-category exercise with no muscle
   overlap (validates that primary-muscle weight beats the category bonus).

One test, two assertions on positions in the rendered list — keeps the test
focused on ordering rather than scoring details (those are covered by the
unit test).

---

## Out of scope

- No badges, pips, sections, or muscle-group highlighting in the UI.
- No filtering on score — even zero-overlap exercises remain reachable.
- No changes to the POST handler, the swap operation itself, or historical
  set lookup.
- No changes to the template, the template data struct, or CSS.
- No tuning UI for the weight constants — they're code-level dials.
