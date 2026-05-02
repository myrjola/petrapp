---
name: Add Exercise — Name Search
description: Port the swap page's `?q=` name filter to the add-exercise page
type: spec
---

# Add Exercise — Name Search

## Overview

The Add Exercise page (`/workouts/{date}/add-exercise`) currently lists every
exercise not already in the workout in DB order, with no way to narrow the
list. The Swap Exercise page already has a working name filter (PR #79,
commit `0c4d452`): a `?q=` GET form, case-insensitive substring match on
`Exercise.Name`, query echoed in the input, and a `.no-results` empty state
that quotes the query.

Port that filter to the add page. Same pattern, same UX, same markup.

The similarity sort that swap also gained (commit `b0777d7`) does **not**
port — add has no "current" exercise to compare against.

---

## Key design decisions

| Decision | Choice | Reason |
|----------|--------|--------|
| Filter location | In-handler `strings.Contains` on lowercased name | Matches swap; ~20 fixture rows, FTS5 not worth it |
| UI pattern | GET form, no JS | Matches swap after PR #79's second commit dropped the live-navigation enhancement |
| Empty state | `.no-results` block echoing the query | Matches swap; reload-safe |
| Shared helper / component | None | Two sites, threshold for extraction is three (per `ui/templates/CLAUDE.md`) |
| Sort order | Unchanged (DB order) | No "current" exercise to compute similarity against; out of scope |

---

## Implementation plan

### `cmd/web/handler-workout.go` — `workoutAddExerciseGET`

Mirror the swap handler's filter:

```go
query := strings.TrimSpace(r.URL.Query().Get("q"))
queryLower := strings.ToLower(query)

var availableExercises []workout.Exercise
for _, exercise := range allExercises {
    if existingExerciseIDs[exercise.ID] {
        continue
    }
    if queryLower != "" && !strings.Contains(strings.ToLower(exercise.Name), queryLower) {
        continue
    }
    availableExercises = append(availableExercises, exercise)
}
```

Add `Query string` to `exerciseAddTemplateData` and populate it from `query`.

`strings` is already imported by this file.

### `ui/templates/pages/exercise-add/exercise-add.gohtml`

Two additions, both copied verbatim from `exercise-swap.gohtml`:

1. Insert a `<section class="exercise-search">` block between the `<header>`
   and the `<section class="available-exercises">`. It contains a scoped
   `<style {{ nonce }}>` block, a `role="search"` GET form with a single
   `<input type="search" name="q" value="{{ .Query }}">` and a Search button.
2. Wrap the `{{ range .Exercises }}` body in a `{{ if .Exercises }} …
   {{ else }} <p class="no-results"> … </p> {{ end }}`. When `.Query` is
   set, echo it: `No exercises match &ldquo;{{ .Query }}&rdquo;.` Otherwise:
   `No exercises available to add.` Add the `.no-results` style rule into
   the existing scoped block on `.available-exercises` (matches swap).

CSS stays colocated and scoped per the template guidelines.

### Tests

New e2e test in `cmd/web/handler-workout_test.go`:
`Test_application_workoutAddExercise_search_filters_by_name`. Structure
mirrors `Test_application_workoutSwapExercise_search_filters_by_name` in
`cmd/web/handler-exerciseset_test.go`:

1. Register; set preferences for today's weekday; start the workout.
2. GET `/workouts/{today}/add-exercise`; collect every
   `.exercise-option .exercise-name`. Bail if fewer than 2 (need a useful
   filter target).
3. Pick the first word of the first name, uppercase it (verifies
   case-insensitivity), GET `?q={needle}`. Assert: at least one match,
   filtered count ≤ unfiltered count, every result contains the substring
   case-insensitively, search input value echoes the query.
4. GET `?q=zzznotreal`. Assert: zero `.exercise-option`, `.no-results`
   element exists and its text contains `zzznotreal`.

The existing `Test_application_addWorkout` (which submits the form without
`?q=`) keeps working unchanged.

---

## Out of scope

- No similarity sort — add has no "current exercise" to score against.
- No shared search component or helper — duplication threshold is three
  call sites, this is the second.
- No service-layer changes — filtering stays handler-side, matching swap.
- No JavaScript enhancement — pure GET form, matches swap after the live
  navigation revert in PR #79.
- No changes to `workoutAddExercisePOST` or to `workoutService.AddExercise`.
