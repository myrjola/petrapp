# Muscle Group Load Visualization on Home Page

## Context

The home page shows a weekly workout schedule (Mon–Sun). Users need a way to verify that their planned exercises are balanced across muscle groups. Each `Exercise` already carries `PrimaryMuscleGroups []string` and `SecondaryMuscleGroups []string`, so no database or service changes are needed — all data is available from the existing `ResolveWeeklySchedule` call.

**Load counts all planned sets for the week** (both completed and upcoming), so users can verify balance before the week is done.

## Approach

Add a "Weekly Muscle Load" section below the day cards. Compute load in the Go handler (primary muscles count 1.0 per set, secondary 0.5 per set), pass it to the template as a new field, and render a horizontal bar chart grouped by body area. All 17 muscle groups are shown (zeros included) so users can spot gaps at a glance.

---

## File Changes

### Modify: `cmd/web/handler-home.go`

1. Add two new structs after the existing `workoutAction` struct:

```go
type muscleGroupLoad struct {
    Name            string
    Load            float64
    BarWidthPercent int // 0–100, scaled to week's max loaded muscle
}

type muscleGroupArea struct {
    Name         string
    MuscleGroups []muscleGroupLoad
}
```

2. Add a package-level ordering variable (mirrors `fixtures.sql` order):

```go
var muscleGroupOrder = [][2]string{
    {"Upper Body", "Chest"}, {"Upper Body", "Shoulders"}, {"Upper Body", "Triceps"},
    {"Upper Body", "Biceps"}, {"Upper Body", "Upper Back"}, {"Upper Body", "Lats"},
    {"Upper Body", "Traps"}, {"Upper Body", "Forearms"},
    {"Core", "Abs"}, {"Core", "Obliques"}, {"Core", "Lower Back"},
    {"Lower Body", "Quads"}, {"Lower Body", "Hamstrings"}, {"Lower Body", "Glutes"},
    {"Lower Body", "Calves"}, {"Lower Body", "Hip Flexors"}, {"Lower Body", "Adductors"},
}
```

3. Add function `calculateMuscleGroupLoads(sessions []workout.Session) []muscleGroupArea`:
   - Build `loads map[string]float64` pre-seeded with all 17 muscle group names at 0.
   - For each session → each ExerciseSet → count `setCount := len(exerciseSet.Sets)`.
   - `loads[primary] += float64(setCount) * 1.0` and `loads[secondary] += float64(setCount) * 0.5`.
   - Find `maxLoad` across all entries.
   - Assemble `[]muscleGroupArea` in `muscleGroupOrder` sequence; compute `BarWidthPercent = int(load / maxLoad * 100)` (0 when `maxLoad == 0`).

4. Extend `homeTemplateData`:

```go
type homeTemplateData struct {
    BaseTemplateData
    Days             []dayView
    MuscleGroupAreas []muscleGroupArea
}
```

5. In `home()`, after `toDays(...)`:

```go
data.MuscleGroupAreas = calculateMuscleGroupLoads(sessions)
```

---

### Create: `ui/templates/pages/home/muscle-groups.gohtml`

A new scoped template following the exact same `@scope` + `{{ nonce }}` pattern as `day-cards.gohtml`. Inline `style="width: X%"` is safe because the CSP includes `'unsafe-inline'` in `style-src`.

```gohtml
{{- /*gotype: github.com/myrjola/petrapp/cmd/web.homeTemplateData*/ -}}

{{ define "muscle-groups" }}
    <div data-component="muscle-groups">
        <style {{ nonce }}>
            @scope {
                :scope {
                    display: flex;
                    flex-direction: column;
                    gap: var(--size-4);

                    .section-title {
                        font-size: var(--font-size-2);
                        font-weight: var(--font-weight-6);
                        color: var(--gray-8);
                    }

                    .area-block {
                        display: flex;
                        flex-direction: column;
                        gap: var(--size-2);
                    }

                    .area-title {
                        font-size: var(--font-size-1);
                        font-weight: var(--font-weight-6);
                        color: var(--gray-6);
                        text-transform: uppercase;
                        letter-spacing: 0.08em;
                    }

                    .muscle-row {
                        display: grid;
                        grid-template-columns: 9rem 1fr 3rem;
                        align-items: center;
                        gap: var(--size-2);
                    }

                    .muscle-name {
                        font-size: var(--font-size-0);
                        color: var(--gray-7);
                        text-align: right;
                    }

                    .bar-track {
                        height: 10px;
                        background: var(--gray-2);
                        border-radius: var(--radius-round);
                        overflow: hidden;
                    }

                    .bar-fill {
                        height: 100%;
                        border-radius: var(--radius-round);
                        background: var(--sky-5);
                    }

                    .load-value {
                        font-size: var(--font-size-0);
                        color: var(--gray-5);
                        text-align: right;
                    }
                }
            }
        </style>

        <div class="section-title">Weekly Muscle Load</div>

        {{ range .MuscleGroupAreas }}
            <div class="area-block">
                <div class="area-title">{{ .Name }}</div>
                {{ range .MuscleGroups }}
                    <div class="muscle-row">
                        <div class="muscle-name">{{ .Name }}</div>
                        <div class="bar-track">
                            <div class="bar-fill" style="width: {{ .BarWidthPercent }}%"></div>
                        </div>
                        <div class="load-value">{{ printf "%.1f" .Load }}</div>
                    </div>
                {{ end }}
            </div>
        {{ end }}
    </div>
{{ end }}
```

---

### Modify: `ui/templates/pages/home/schedule.gohtml`

Add `{{ template "muscle-groups" . }}` after the `.weekly-schedule` div, inside `<main>`:

```gohtml
        <div class="weekly-schedule">
            {{ template "day-cards" . }}
        </div>

        {{ template "muscle-groups" . }}
    </main>
```

---

## Acceptance Criteria

- Home page (authenticated) shows a "Weekly Muscle Load" section with three body-area headings: "Upper Body", "Core", "Lower Body".
- All 17 muscle groups are rendered (one row each), including those with 0 load.
- Bars are proportional: the muscle group with the highest load has a bar at 100%; others are scaled accordingly.
- When no sessions have any sets, all bars render at 0% width without errors.
- Primary muscle group sets count fully (1.0); secondary count at half (0.5).
- The load value displayed matches the calculation.
- No CSP violations (style uses `unsafe-inline`-covered inline width, `<style>` uses `{{ nonce }}`).
- Existing `Test_application_home` tests continue to pass.

---

## Verification

1. Run `make test` — all existing tests must pass.
2. Run `make build` — template must compile without errors.
3. Start the server, register a user, open `/`, verify the "Weekly Muscle Load" section appears below the day cards.
4. Navigate to a workout day that has exercises planned; verify the bars for those exercises' primary/secondary muscles are non-zero.
5. Confirm relative scaling: if Chest has 6 sets and Shoulders 3, Chest bar fills 100%, Shoulders fills 50%.

---

## Risks & Mitigations

| Risk | Mitigation |
|---|---|
| Muscle group names in exercises don't match hardcoded keys | Names are seeded from `fixtures.sql` and Exercise data comes from the same DB; the mapping will be consistent. |
| `printf "%.1f"` not available in templates | `printf` is a standard Go template function available by default. |
| `@scope` CSS not applying per-row widths | The `width` is set via inline `style` attribute, not via scoped CSS, so per-row variation works correctly regardless of scope rules. |
