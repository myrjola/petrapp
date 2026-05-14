# Migrate `home` and `exerciseset` onto the Frontend Foundation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring the last two unmigrated page templates — `home` and `exerciseset` — onto the frontend foundation, adopting `page-header`/`back-link`/`.card`/`.stack`/`.cluster` where they genuinely fit and fixing the one template-logic offense in `sets-container`.

**Architecture:** Pure structural migration — no aesthetic redesign. Foundation primitives replace duplicated inline layout/surface CSS; genuinely page-specific visualizations (`muscle-balance`, `progress-bar`, the landing hero, `sets-container`'s set forms) stay page-scoped. One Go change moves `sets-container`'s in-template `$isActive` computation into the handler. The safety net is the existing Playwright + e2e handler test suite, which must stay green; every test selector is preserved.

**Tech Stack:** Go `html/template` (no-build, filesystem-loaded), CSS `@layer` cascade + `@scope`, strict CSP with nonces, goquery/Playwright tests.

**Spec:** `docs/superpowers/specs/2026-05-14-migrate-home-exerciseset-design.md`

---

## Background every task needs

- **No-build frontend.** Editing a `.gohtml` and re-running the test is the loop; `make build` recompiles the Go binary (needed after handler changes).
- **CSP.** Every `<style>`/`<script>` keeps `{{ nonce }}`. Never introduce `style=""` attributes. Dynamic CSS values stay inside nonced `<style>` blocks.
- **`@layer` cascade is `reset, props, layout, components`.** `.card` lives in `@layer components`; `.stack`/`.cluster` in `@layer layout`. **An unlayered scoped `<style>` rule beats any layered rule.** So a scoped `:scope { display: flex }` still wins over `.card`'s layered `display: block` — this is why several elements below get `.card` *and* keep a scoped `display:flex`. **Do not put `.card` and `.stack` on the same element** — `.card`'s `display:block` (components layer) would beat `.stack`'s `display:flex` (layout layer).
- **Foundation primitive definitions** (`ui/static/main.css`): `.stack` = `display:flex; flex-direction:column; gap:var(--size-4)`. `.cluster` = `display:flex; flex-wrap:wrap; gap:var(--size-2); align-items:center`. `.card` = `display:block; padding:var(--size-3); background:var(--color-surface-elevated); border:var(--border-size-1) solid var(--color-border); border-radius:var(--radius-3); box-shadow:var(--shadow-1)`.
- **Components** (`ui/templates/components/`): `back-link` — dot is an href string; renders `<a href data-back-button class="back-link"><span aria-hidden>←</span><span>Back</span></a>` plus its own scoped `<style>`. `page-header` — dot is `cmd/web.PageHeaderData{Title, Subtitle}`; renders `<header class="page-header"><h1>…</h1></header>` plus scoped `<style>`.
- **Commit style:** sentence describing the *why*, ending with the `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>` trailer (HEREDOC). Never `--no-verify`.
- **Baseline before every template edit:** read the test files named in the task's "Test selectors to preserve" list and confirm you keep every class/attribute/text they query.

---

### Task 1: `home` — authenticated view (`schedule.gohtml` + handler header)

Adds a `page-header` ("This Week") to the authenticated home page, which currently has **no `<h1>`** at all, and swaps the `<main>`'s inline flex-column for `.stack`.

**Files:**
- Modify: `cmd/web/handler-home.go` (`homeTemplateData` struct ~lines 49-64; `home()` struct literal ~lines 393-402)
- Modify: `ui/templates/pages/home/schedule.gohtml`

**Test selectors to preserve** (read `cmd/web/handler-home_test.go`, `cmd/web/playwright_test.go`): `doc.Find("main").First()`, `.weekly-schedule` as a direct child of `main`, the relative ordering `.weekly-schedule` before `.muscle-balance`. Adding a `page-header` `<header>` child of `main` is fine — the test compares *relative* positions of those two classes only.

- [ ] **Step 1: Baseline — confirm the home tests are green**

Run: `go test ./cmd/web/ -run Test_application_home -v`
Expected: PASS (`Initial state`, `After registration`, `After logout`, `After login`, `Muscle balance section renders after login`).

- [ ] **Step 2: Add the `Header` field to `homeTemplateData`**

In `cmd/web/handler-home.go`, add `Header PageHeaderData` as the first field after the embed:

```go
type homeTemplateData struct {
	BaseTemplateData
	// Header is the page-header dot for the authenticated view.
	Header PageHeaderData
	// Days contains the workout sessions for the current week.
	Days []dayView
	// MuscleBalance summarises weekly volume per muscle group, grouped by region.
	// Empty for unauthenticated users; Regions is empty when the week has no exercises.
	MuscleBalance muscleBalanceView
	// WeekInBlock is the 1-based week index within the current mesocycle.
	WeekInBlock int
	// MesocycleLength is the total number of weeks in the mesocycle block.
	MesocycleLength int
	// IsDeloadWeek reports whether the current week is the last (deload) week.
	IsDeloadWeek bool
	// DeloadEnabled reports whether the deload feature is enabled for this user.
	DeloadEnabled bool
}
```

- [ ] **Step 3: Populate `Header` in `home()`**

In `cmd/web/handler-home.go`, update the struct literal in `home()`:

```go
	data := homeTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
		Header:           PageHeaderData{Title: "This Week", Subtitle: ""},
		Days:             nil,
		MuscleBalance:    muscleBalanceView{Regions: nil},
		WeekInBlock:      0,
		MesocycleLength:  0,
		IsDeloadWeek:     false,
		DeloadEnabled:    false,
	}
```

(`unauthenticated.gohtml` never references `.Header`, so the unauthenticated path is unaffected.)

- [ ] **Step 4: Rebuild and confirm compilation**

Run: `make build`
Expected: builds with no errors.

- [ ] **Step 5: Migrate `schedule.gohtml`**

Replace the whole `{{ define "schedule" }}` body. The `<main>` gets `class="stack"` (its `.stack` gap of `var(--size-4)` matches the old inline gap, so no override needed); the scoped style keeps only `padding`; the `header` element selector becomes `.menu-button` (so it no longer also matches the `page-header`'s `<header>`); `.weekly-schedule` keeps its gap override; the `page-header` renders as the first child:

```gohtml
{{- /*gotype: github.com/myrjola/petrapp/cmd/web.homeTemplateData*/ -}}

{{ define "schedule" }}
    <main class="stack">
        <style {{ nonce }}>
            @scope {
                :scope {
                    padding: var(--size-4);
                }

                .menu-button {
                    margin-left: auto;
                }

                .menu-button a {
                    display: inline-flex;
                    align-items: center;
                    padding: var(--size-2) var(--size-3);
                    background: var(--white);
                    color: var(--gray-9);
                    text-decoration: none;
                    border-radius: var(--radius-2);
                    border: 1px solid var(--gray-3);
                    font-weight: var(--font-weight-5);
                    font-size: var(--font-size-1);
                    transition: all 0.2s ease;
                    box-shadow: var(--shadow-1);
                }

                .menu-button a:hover {
                    background: var(--gray-0);
                    border-color: var(--gray-4);
                    box-shadow: var(--shadow-3);
                    transform: translateY(-1px);
                }

                .menu-button a:active {
                    transform: translateY(0);
                    box-shadow: var(--shadow-1);
                }

                .weekly-schedule {
                    gap: var(--size-3);
                }
            }
        </style>

        {{ template "page-header" .Header }}

        <header class="menu-button">
            <a href="/preferences">
                Menu
            </a>
        </header>

        {{ if .DeloadEnabled }}
            <style {{ nonce }}>
                @scope (.week-chip) {
                    :scope {
                        display: inline-block;
                        padding: var(--size-1) var(--size-2);
                        border-radius: var(--radius-2);
                        background: var(--gray-1);
                        color: var(--gray-9);
                        font-size: var(--font-size-0);

                        &.week-chip--deload {
                            background: var(--sky-2);
                            color: var(--sky-10);
                            font-weight: var(--font-weight-7);
                        }
                    }
                }
            </style>
            <span class="week-chip{{ if .IsDeloadWeek }} week-chip--deload{{ end }}">
                Week {{ .WeekInBlock }} of {{ .MesocycleLength }}{{ if .IsDeloadWeek }} · Deload{{ end }}
            </span>
        {{ end }}

        <div class="weekly-schedule stack">
            {{ template "day-cards" . }}
        </div>

        {{ template "muscle-balance" . }}
    </main>
{{ end }}
```

Note: `.weekly-schedule stack` is safe — `.weekly-schedule` has no `.card`, so `.stack`'s `display:flex` applies and the scoped `gap: var(--size-3)` overrides `.stack`'s default gap.

- [ ] **Step 6: Rebuild and run the home tests**

Run: `make build && go test ./cmd/web/ -run Test_application_home -v`
Expected: PASS. The `Muscle balance section renders after login` subtest still finds `.weekly-schedule` before `.muscle-balance` as `main` children.

- [ ] **Step 7: Commit**

```bash
git add cmd/web/handler-home.go ui/templates/pages/home/schedule.gohtml
git commit -m "$(cat <<'EOF'
Migrate authenticated home view to the frontend foundation

Adds a page-header — the authenticated home page had no <h1> at all,
so the heading hierarchy jumped straight to the muscle-balance <h2>.
The <main> inline flex-column becomes the .stack primitive.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: `home` — `day-cards.gohtml` (`.card`, `.cluster`, dead-class cleanup)

`.day-card` adopts `.card` for its surface; the per-`data-status` border/background overrides stay scoped. `.workout-actions` becomes `.cluster`. The undefined `btn-primary`/`btn-secondary` classes (no rule exists in `main.css`; the `<button>` element is already styled) are dropped — pure dead-code removal, zero visual change.

**Files:**
- Modify: `ui/templates/pages/home/day-cards.gohtml`

**Also reviewed — no change:** `ui/templates/pages/home/progress-bar.gohtml`. Its wrapper is `display:flex; align-items:center; gap:var(--size-3)`; `.cluster` would add `flex-wrap:wrap`, which is wrong for a single-line bar+label (the label could wrap below the bar). No foundation primitive fits — it stays page-specific.

**Test selectors to preserve** (`cmd/web/handler-home_test.go`, `cmd/web/playwright_test.go`): `.day-card` keeps its class and `data-status` attribute. Playwright finds the start button by role+name (`"Start Workout"`, `"Start Extra Workout"`, `"Continue Workout"`, etc.) — the button text is unchanged, so removing the dead class attribute is safe.

- [ ] **Step 1: Baseline — confirm home tests green**

Run: `go test ./cmd/web/ -run Test_application_home -v`
Expected: PASS.

- [ ] **Step 2: Migrate `day-cards.gohtml`**

Replace the whole `{{ define "day-cards" }}` body. The scoped `.day-card` rule loses `border`, `border-radius`, `padding`, `background` (now from `.card`) and keeps `display:flex; flex-direction:column; gap` plus every `&[data-status]` override. `.workout-actions` is removed from the scoped style entirely (now `.cluster`). The element gets `class="day-card card"`; the actions wrapper gets `class="workout-actions cluster"`; the start `<button>` loses its dead class attribute:

```gohtml
{{- /*gotype: github.com/myrjola/petrapp/cmd/web.dayData*/ -}}

{{ define "day-cards" }}
    <style {{ nonce }}>
        @scope {
            .day-card {
                display: flex;
                flex-direction: column;
                gap: var(--size-2);
                transition: all 0.2s ease;

                &[data-status="today"] {
                    border-color: var(--sky-6);
                    background: var(--sky-0);
                    box-shadow: 0 2px 8px var(--sky-2);
                }

                &[data-status="completed"] {
                    border-color: var(--lime-6);
                    background: var(--lime-0);
                }

                &[data-status="in_progress"] {
                    border-color: var(--yellow-6);
                    background: var(--yellow-0);
                }

                &[data-status="past-incomplete"] {
                    border-color: var(--red-6);
                    background: var(--red-1);
                }

                &[data-status="unscheduled"] {
                    border-color: var(--gray-4);
                    background: var(--gray-1);
                    border-style: dashed;
                    border-width: 1px;
                    opacity: 0.7;
                }

                &[data-status="upcoming"] {
                    border-color: var(--gray-3);
                    background: var(--gray-0);
                }
            }

            .day-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
            }

            .day-title {
                font-weight: var(--font-weight-6);
                font-size: var(--font-size-4);
            }

            .day-date {
                color: var(--gray-6);
                font-size: var(--font-size-0);
            }

            .status-indicator {
                display: inline-flex;
                align-items: center;
                gap: var(--size-1);
                padding: var(--size-1) var(--size-2);
                border-radius: var(--radius-2);
                font-size: var(--font-size-1);
                font-weight: var(--font-weight-5);
                text-transform: uppercase;
                letter-spacing: 0.05em;

                &[data-status="completed"] {
                    background: var(--lime-2);
                    color: var(--lime-9);
                }

                &[data-status="in_progress"] {
                    background: var(--yellow-2);
                    color: var(--yellow-11);
                }

                &[data-status="upcoming"] {
                    background: var(--gray-3);
                    color: var(--gray-9);
                }

                &[data-status="past-incomplete"] {
                    background: var(--red-2);
                    color: var(--red-9);
                }

                &[data-status="unscheduled"] {
                    background: var(--gray-2);
                    color: var(--gray-9);
                    border: 1px dashed var(--gray-5);
                }

                &[data-status="today"] {
                    background: var(--sky-2);
                    color: var(--sky-9);
                    font-weight: var(--font-weight-6);
                }
            }
        }
    </style>

    {{ range .Days }}
        <div class="day-card card" data-status="{{ .Status }}">
            <div class="day-header">
                <div>
                    <div class="day-title">
                        {{ .Name }}
                    </div>
                    <div class="day-date">{{ .Date.Format "Jan 2, 2006" }}</div>
                </div>
                <div class="status-indicator" data-status="{{ .Status }}">
                    {{ .StatusLabel }}
                </div>
            </div>

            {{ template "progress-bar" . }}

            <div class="workout-actions cluster">
                {{ if .Action }}
                    {{ if .Action.StartWorkout }}
                        <form method="post" action="/workouts/{{ .Date.Format "2006-01-02" }}/start">
                            <button type="submit">
                                {{ .Action.Label }}
                            </button>
                        </form>
                    {{ else }}
                        <a href="/workouts/{{ .Date.Format "2006-01-02" }}" class="btn">
                            {{ .Action.Label }}
                        </a>
                    {{ end }}
                {{ end }}
            </div>
        </div>
    {{ end }}
{{ end }}
```

Note: `.day-card card` keeps its scoped `display:flex` (unlayered scoped rule beats `.card`'s layered `display:block`). `.card` supplies `border`, `border-radius`, `padding`, `box-shadow`; the `&[data-status]` rules override `border-color`/`background`/`border-style` as before.

- [ ] **Step 3: Rebuild and run the home tests**

Run: `make build && go test ./cmd/web/ -run Test_application_home -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add ui/templates/pages/home/day-cards.gohtml
git commit -m "$(cat <<'EOF'
Migrate day-cards to .card and .cluster primitives

The .day-card surface (border, radius, padding, shadow) now comes from
.card; per-status colour overrides stay scoped. .workout-actions uses
.cluster. Drops the dead btn-primary/btn-secondary classes — no such
rules exist in main.css, so the change is visually inert cleanup.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: `home` — `muscle-balance.gohtml` (`.card` surface, `.stack`)

The `<section class="muscle-balance">` adopts `.card` for its surface; the inner `.region` divs adopt `.stack`. **Every visualization rule stays** — bars, legend, rows, per-`data-status` colours, and the dynamic per-`data-slug` widths emitted from the nonced `<style>` (CSP requires those to stay in a `<style>` block).

**Files:**
- Modify: `ui/templates/pages/home/muscle-balance.gohtml`

**Test selectors to preserve** (`cmd/web/handler-home_test.go`): `section.muscle-balance` (keeps the class — `.card` is *added*), `.row`, `.row[data-slug="chest"]`, `.row[data-slug="calves"]`, `.row[data-status="…"]`, `.target-mark`, the `aria-labelledby="muscle-balance-heading"` / `#muscle-balance-heading` pairing, and `.muscle-balance` as a direct `main` child after `.weekly-schedule`.

- [ ] **Step 1: Baseline — confirm the muscle-balance subtest is green**

Run: `go test ./cmd/web/ -run Test_application_home/Muscle_balance_section_renders_after_login -v`
Expected: PASS.

- [ ] **Step 2: Migrate the `:scope` and `.region` rules in `muscle-balance.gohtml`**

In `ui/templates/pages/home/muscle-balance.gohtml`:

1. Change the section opening tag from
   `<section class="muscle-balance" aria-labelledby="muscle-balance-heading">`
   to
   `<section class="muscle-balance card" aria-labelledby="muscle-balance-heading">`.

2. In the scoped `<style>`, change the `:scope` rule from:

```css
                    :scope {
                        display: flex;
                        flex-direction: column;
                        gap: var(--size-4);
                        padding: var(--size-4);
                        background: var(--gray-0);
                        border: 1px solid var(--gray-3);
                        border-radius: var(--radius-3);
                    }
```

   to (drop `padding`/`background`/`border`/`border-radius` — now from `.card`; keep the flex column):

```css
                    :scope {
                        display: flex;
                        flex-direction: column;
                        gap: var(--size-4);
                    }
```

3. Change the `.region` rule from:

```css
                    .region {
                        display: flex;
                        flex-direction: column;
                        gap: var(--size-2);
                    }
```

   to (the flex column now comes from `.stack`; keep only the gap override):

```css
                    .region {
                        gap: var(--size-2);
                    }
```

4. Change the region div from `<div class="region">` to `<div class="region stack">`.

Leave the entire rest of the file unchanged — the `h2`, `.row`, `.bar*`, `.target-mark`, `.legend*`, `.counts`, the `{{ range }}`-generated per-`data-slug` width rules, and all markup.

- [ ] **Step 3: Rebuild and run the muscle-balance subtest**

Run: `make build && go test ./cmd/web/ -run Test_application_home -v`
Expected: PASS — 17 `.row`s, `chest` row shows `target 10`, `calves` row has no `.target-mark`, `.muscle-balance` renders after `.weekly-schedule`.

- [ ] **Step 4: Commit**

```bash
git add ui/templates/pages/home/muscle-balance.gohtml
git commit -m "$(cat <<'EOF'
Migrate muscle-balance surface to .card and .stack

The section surface (padding, background, border, radius) now comes
from .card and the .region columns from .stack. Every visualization
rule — bars, legend, per-data-slug widths, status colours — is
unchanged; this is a hard component that stays page-specific.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: `home` — `unauthenticated.gohtml` (`.stack`, `.cluster`)

A light touch on the landing hero: the centered content column adopts `.stack`, the Sign in/Register button row adopts `.cluster`. The hero `<h1>`/logo/tagline markup, the page-wrapper scaffolding, the footer, the forms, the WebAuthn scripts, and `backdrops.gohtml` all stay page-specific — this is a full-viewport hero, the same treatment the `not-found`/`error`/`maintenance` heroes got in commit 567f424.

**Files:**
- Modify: `ui/templates/pages/home/unauthenticated.gohtml`

**Test selectors to preserve** (`cmd/web/handler-home_test.go`, `cmd/web/playwright_test.go`): `button:contains('Sign in')` and `button:contains('Register')` (the `<button>` elements and their text are untouched); Playwright's `GetByRole("button", Name: "Register"/"Sign in")`.

- [ ] **Step 1: Baseline — confirm the unauthenticated-state assertions pass**

Run: `go test ./cmd/web/ -run Test_application_home -v`
Expected: PASS (`Initial state` finds one `Sign in` + one `Register` button).

- [ ] **Step 2: Migrate the content-column `<div>` to `.stack`**

In `ui/templates/pages/home/unauthenticated.gohtml`, find the inner content `<div>` (the one whose scoped `<style>` block contains `gap: var(--size-5)`, `text-align: center`, and the `> img` / `> h1` / `> p` child rules). Change that `<div>` to `<div class="stack">` and change its scoped `:scope` rule from:

```css
                            :scope {
                                display: flex;
                                flex-direction: column;
                                gap: var(--size-5);
                                text-align: center;

                                > img {
```

to (drop `display`/`flex-direction` — now from `.stack`; keep the gap override, `text-align`, and all child rules):

```css
                            :scope {
                                gap: var(--size-5);
                                text-align: center;

                                > img {
```

Leave the `> img`, `> h1`, `> p` rules and everything else inside that `<style>` block unchanged.

- [ ] **Step 3: Migrate the button-row `<div>` to `.cluster`**

In the same file, find the innermost `<div>` that wraps the two `<form>`s (Sign in / Register) — its scoped `<style>` is `:scope { display: flex; justify-content: center; gap: var(--size-4); }`. Change that `<div>` to `<div class="cluster">` and change its scoped rule from:

```css
                            @scope {
                                :scope {
                                    display: flex;
                                    justify-content: center;
                                    gap: var(--size-4);
                                }
                            }
```

to (drop `display` — now from `.cluster`; keep `justify-content` and the gap override, which differ from `.cluster`'s defaults):

```css
                            @scope {
                                :scope {
                                    justify-content: center;
                                    gap: var(--size-4);
                                }
                            }
```

Leave the two `<form>`s, their `<button>`s, and the WebAuthn `<script>` blocks exactly as they are.

- [ ] **Step 4: Rebuild and run the home tests**

Run: `make build && go test ./cmd/web/ -run Test_application_home -v`
Expected: PASS — `Sign in` and `Register` buttons still found in the unauthenticated state.

- [ ] **Step 5: Commit**

```bash
git add ui/templates/pages/home/unauthenticated.gohtml
git commit -m "$(cat <<'EOF'
Migrate unauthenticated hero layout to .stack and .cluster

The centered content column and the Sign in/Register button row use
the layout primitives instead of inline flex CSS. The hero heading,
logo, tagline, forms, and backdrops stay page-specific — this is a
full-viewport hero, kept page-scoped like the other hero pages.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: `exerciseset` — `exercise-header.gohtml` + `exerciseset.gohtml` shell

`exercise-header` replaces its hand-rolled back link with the `back-link` component (real dedup — and drops the duplicated `.back-link` scoped CSS), and `.header-actions` adopts `.cluster`. `exerciseset.gohtml`'s `<main>` adopts `.stack`.

**Files:**
- Modify: `ui/templates/pages/exerciseset/exercise-header.gohtml`
- Modify: `ui/templates/pages/exerciseset/exerciseset.gohtml`

**Also reviewed — no change:** `ui/templates/pages/exerciseset/warmup.gohtml`. `.warmup-banner` is a page-specific info-bordered call-to-action (heading + description + form) and `.warmup-status` a custom success pill; neither maps onto a foundation primitive without override-churn that would violate YAGNI. It stays page-specific (the spec's "light touch" — reviewed, no change warranted).

**Test selectors to preserve** (read `cmd/web/handler-exerciseset_test.go`, `cmd/web/playwright_test.go`): `h1` must still exist on the page (the `<h1 class="exercise-title">` is untouched); the back link keeps `data-back-button` (consumed by `main.js`) — the `back-link` component provides it. No test queries `aria-label="Back to workout"`; the component's accessible name becomes "Back" (the `←` span is `aria-hidden`), which is acceptable.

- [ ] **Step 1: Baseline — confirm the exerciseset tests are green**

Run: `go test ./cmd/web/ -run Test_application_exerciseSet -v`
Expected: PASS.

- [ ] **Step 2: Replace the hand-rolled back link in `exercise-header.gohtml`**

In `ui/templates/pages/exerciseset/exercise-header.gohtml`:

1. In the scoped `<style>`, **delete** the entire `.back-link { … }` rule block (the one with `color`, `text-decoration`, `display: inline-flex`, the `&:hover` and `&:focus-visible` nested rules) — the `back-link` component carries its own scoped styles.

2. Replace the back-link anchor markup:

```gohtml
        <a href="/workouts/{{ .Date.Format "2006-01-02" }}" data-back-button class="back-link"
           aria-label="Back to workout">
            <span aria-hidden="true">←</span>
            <span>Back</span>
        </a>
```

   with:

```gohtml
        {{ template "back-link" (printf "/workouts/%s" (.Date.Format "2006-01-02")) }}
```

The component renders a `<style>` sibling followed by the `<a class="back-link" data-back-button>`. The `<style>` is `display:none` and therefore not a grid item, so the parent's `grid-template-columns: auto 1fr auto` still places the `<a>` as the first column.

- [ ] **Step 3: Migrate `.header-actions` to `.cluster` in `exercise-header.gohtml`**

1. Change `<div class="header-actions">` to `<div class="header-actions cluster">`.

2. In the scoped `<style>`, change the `.header-actions` rule from:

```css
                .header-actions {
                    display: flex;
                    gap: var(--size-2);
                    justify-content: flex-end;
                }
```

   to (drop `display`/`gap` — now from `.cluster`; keep `justify-content`, which `.cluster` does not set):

```css
                .header-actions {
                    justify-content: flex-end;
                }
```

Leave the `@media (max-width: 480px)` block's `.header-actions { grid-column: 1 / -1; justify-content: center; }` rule unchanged.

- [ ] **Step 4: Migrate the `<main>` in `exerciseset.gohtml` to `.stack`**

In `ui/templates/pages/exerciseset/exerciseset.gohtml`:

1. Change `<main class="page-container" role="main" aria-label="Exercise Set Workout">` to `<main class="page-container stack" role="main" aria-label="Exercise Set Workout">`.

2. In the scoped `<style>`, change the `:scope` rule from:

```css
                :scope {
                    display: flex;
                    flex-direction: column;
                    gap: var(--size-5);
                    max-width: 600px;
                    margin: var(--size-4) auto;
                    padding: var(--size-4);

                    @media (max-width: 768px) {
                        margin: var(--size-2) auto;
                        padding: var(--size-3);
                        gap: var(--size-4);
                    }
                }
```

   to (drop `display`/`flex-direction` — now from `.stack`; keep the `gap` override and the page-specific `max-width`/`margin`/`padding` and media query):

```css
                :scope {
                    gap: var(--size-5);
                    max-width: 600px;
                    margin: var(--size-4) auto;
                    padding: var(--size-4);

                    @media (max-width: 768px) {
                        margin: var(--size-2) auto;
                        padding: var(--size-3);
                        gap: var(--size-4);
                    }
                }
```

Leave the `.timer` rule, both `<script>` blocks, and the `{{ template … }}` calls unchanged.

- [ ] **Step 5: Rebuild and run the exerciseset tests**

Run: `make build && go test ./cmd/web/ -run 'Test_application_exerciseSet|TestExerciseSetGET_DeloadHidesSignalButtons|Test_ExerciseSet_RestChipAfterCompletedSet' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add ui/templates/pages/exerciseset/exercise-header.gohtml ui/templates/pages/exerciseset/exerciseset.gohtml
git commit -m "$(cat <<'EOF'
Migrate exerciseset header and shell to the frontend foundation

exercise-header uses the shared back-link component (dropping its
duplicated scoped CSS) and .cluster for the action row; the page
<main> uses .stack. The page-specific max-width/margin and the fixed
timer stay scoped.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: `exerciseset` — move `sets-container`'s `$isActive` into the handler

`sets-container.gohtml` computes per-set "is this the active row" state in-template from five fields — the exact template-logic offense the foundation spec calls out. This task adds a tested pure helper and an `IsActive` field on `setDisplay`, computed in the handler. The template is **not** touched here (it still computes its own `$isActive`); Task 7 switches it over. This keeps every task independently green.

**Files:**
- Modify: `cmd/web/handler-exerciseset.go` (`setDisplay` struct ~lines 15-21; `exerciseSetGET` ~lines 79-168)
- Test: `cmd/web/handler-exerciseset_test.go` (add a unit test; package `main`)

- [ ] **Step 1: Write the failing unit test**

Append to `cmd/web/handler-exerciseset_test.go`:

```go
func Test_computeSetActive(t *testing.T) {
	tests := []struct {
		name                 string
		warmupComplete       bool
		completed            bool
		index                int
		firstIncompleteIndex int
		editingIndex         int
		isEditing            bool
		want                 bool
	}{
		{
			name:                 "warmup not complete is never active",
			warmupComplete:       false,
			completed:            false,
			index:                0,
			firstIncompleteIndex: 0,
			editingIndex:         -1,
			isEditing:            false,
			want:                 false,
		},
		{
			name:                 "first incomplete set is active",
			warmupComplete:       true,
			completed:            false,
			index:                2,
			firstIncompleteIndex: 2,
			editingIndex:         -1,
			isEditing:            false,
			want:                 true,
		},
		{
			name:                 "a later incomplete set is not active",
			warmupComplete:       true,
			completed:            false,
			index:                3,
			firstIncompleteIndex: 2,
			editingIndex:         -1,
			isEditing:            false,
			want:                 false,
		},
		{
			name:                 "completed set at the first-incomplete index is not active",
			warmupComplete:       true,
			completed:            true,
			index:                2,
			firstIncompleteIndex: 2,
			editingIndex:         -1,
			isEditing:            false,
			want:                 false,
		},
		{
			name:                 "the set being edited is active even though completed",
			warmupComplete:       true,
			completed:            true,
			index:                1,
			firstIncompleteIndex: 4,
			editingIndex:         1,
			isEditing:            true,
			want:                 true,
		},
		{
			name:                 "editing mode does not activate a non-edited set",
			warmupComplete:       true,
			completed:            true,
			index:                0,
			firstIncompleteIndex: 4,
			editingIndex:         1,
			isEditing:            true,
			want:                 false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeSetActive(
				tt.warmupComplete, tt.completed, tt.index, tt.firstIncompleteIndex, tt.editingIndex, tt.isEditing)
			if got != tt.want {
				t.Errorf("computeSetActive() = %v, want %v", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/web/ -run Test_computeSetActive -v`
Expected: FAIL — `undefined: computeSetActive`.

- [ ] **Step 3: Add the `IsActive` field and the `computeSetActive` helper**

In `cmd/web/handler-exerciseset.go`, add `IsActive` to the `setDisplay` struct:

```go
type setDisplay struct {
	Set          domain.Set
	TargetStr    string // Pre-formatted target string (e.g. "5", "30s").
	CompletedStr string // Pre-formatted completed string, same unit as TargetStr.
	Unit         string // "reps" or "seconds" — for input labels.
	Number       int    // 1-based set number for display.
	IsActive     bool   // Whether this row renders the completion form.
}
```

Add the helper (place it just below `getLastCompletedAt`):

```go
// computeSetActive reports whether a set row should render its completion form.
// A row is active when the warmup is done and it is either the first incomplete
// set or the set explicitly being edited. This is a multi-field derived value, so
// it lives in the handler rather than the template.
func computeSetActive(
	warmupComplete, completed bool,
	index, firstIncompleteIndex, editingIndex int,
	isEditing bool,
) bool {
	if !warmupComplete {
		return false
	}
	isCurrentTarget := !completed && firstIncompleteIndex == index
	isEditingThis := isEditing && editingIndex == index
	return isCurrentTarget || isEditingThis
}
```

- [ ] **Step 4: Run the unit test to verify it passes**

Run: `go test ./cmd/web/ -run Test_computeSetActive -v`
Expected: PASS (all six sub-cases).

- [ ] **Step 5: Populate `IsActive` in `exerciseSetGET`**

In `cmd/web/handler-exerciseset.go`, the `prepareSetsDisplay` call sits inside the `data := exerciseSetTemplateData{…}` literal. After that literal is constructed (after the closing `}` of the literal, before `app.render(...)`), add a loop that fills in `IsActive` — it needs `FirstIncompleteIndex`, `IsEditing`, `EditingIndex`, and `exerciseSet.WarmupCompletedAt`, all already in scope:

```go
	for i := range data.SetsDisplay {
		data.SetsDisplay[i].IsActive = computeSetActive(
			exerciseSet.WarmupCompletedAt != nil,
			data.SetsDisplay[i].Set.CompletedValue != nil,
			i,
			data.FirstIncompleteIndex,
			data.EditingIndex,
			data.IsEditing,
		)
	}

	app.render(w, r, http.StatusOK, "exerciseset", data)
```

(Replace the existing bare `app.render(w, r, http.StatusOK, "exerciseset", data)` line with the loop + that same render call.)

Note on the `completed` argument: the template's original `$isActive` used `(not $set.CompletedValue)` — `CompletedValue` is a `*int`, truthy when non-nil. So `completed` here is `data.SetsDisplay[i].Set.CompletedValue != nil`, matching the template's existing semantics exactly.

- [ ] **Step 6: Rebuild and run the full exerciseset suite**

Run: `make build && go test ./cmd/web/ -run 'Test_computeSetActive|Test_application_exerciseSet|TestExerciseSetGET_DeloadHidesSignalButtons|Test_ExerciseSet_RestChipAfterCompletedSet' -v`
Expected: PASS. The template still computes its own `$isActive`, so behavior is unchanged; the new field is populated but not yet consumed.

- [ ] **Step 7: Commit**

```bash
git add cmd/web/handler-exerciseset.go cmd/web/handler-exerciseset_test.go
git commit -m "$(cat <<'EOF'
Compute exercise-set active state in the handler

The sets-container template derived per-set active state from five
fields inline — the template-logic offense the frontend foundation
spec calls out. Adds a tested computeSetActive helper and a
setDisplay.IsActive field; the template switch follows next.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: `exerciseset` — `sets-container.gohtml` (`.card` on `.exercise-set`, consume `.IsActive`)

`.exercise-set` adopts `.card` for its surface; the `.completed`/`.active` variant overrides stay scoped. The in-template `$isActive` computation is replaced by a read of the handler-prepared `$setDisplay.IsActive` from Task 6 — the `$isActive` variable name is kept so the rest of the template is untouched. The set forms, inputs, signal buttons, deload banner, and rest chip all stay exactly as they are.

**Files:**
- Modify: `ui/templates/pages/exerciseset/sets-container.gohtml`

**Test selectors to preserve** (`cmd/web/handler-exerciseset_test.go`, `cmd/web/playwright_test.go`): `.exercise-set` (keeps the class — `.card` is *added*), `.exercise-set.completed`, `.exercise-set.active`, `.weight`, `.reps`, `.edit-button`, `.rest-chip[data-rest-end-at-ms]`, `.deload-banner`, `button[name='signal']`, `button.signal-btn`, `button:contains('Done!')`, `input[name='weight']`, the form `action` attributes.

- [ ] **Step 1: Baseline — confirm the exerciseset tests are green**

Run: `go test ./cmd/web/ -run 'Test_application_exerciseSet|TestExerciseSetGET_DeloadHidesSignalButtons|Test_ExerciseSet_RestChipAfterCompletedSet' -v`
Expected: PASS.

- [ ] **Step 2: Add `.card` to `.exercise-set` and trim its scoped surface rules**

In `ui/templates/pages/exerciseset/sets-container.gohtml`:

1. In the scoped `<style>`, change the `.exercise-set` rule from:

```css
                .exercise-set {
                    display: flex;
                    flex-direction: column;
                    gap: var(--size-3);
                    padding: var(--size-4);
                    background: var(--color-surface);
                    border: 1px solid var(--color-border);
                    border-radius: var(--radius-3);
                    transition: background-color 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease;
                    position: relative;
```

   to (drop `padding`/`background`/`border`/`border-radius` — now from `.card`; keep the flex column, transition, and `position`):

```css
                .exercise-set {
                    display: flex;
                    flex-direction: column;
                    gap: var(--size-3);
                    transition: background-color 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease;
                    position: relative;
```

   Leave the rest of the `.exercise-set` block — the `&.completed`, `&.active`, and all nested `.set-info` / `.set-form` / `.input-field` / `.assisted-field` / `.signal-group` / `.signal-buttons` / `.signal-btn` / `.submit-button` / `.bodyweight-form` rules — exactly as they are. The `&.completed` and `&.active` rules override `background`/`border-color`/`box-shadow` and still win as unlayered scoped rules.

2. Change the set element opening tag from:

```gohtml
            <div class="exercise-set{{ if $set.CompletedValue }} completed{{ end }}{{ if $isActive }} active{{ end }}"
```

   to (insert `card` right after `exercise-set`):

```gohtml
            <div class="exercise-set card{{ if $set.CompletedValue }} completed{{ end }}{{ if $isActive }} active{{ end }}"
```

   `.exercise-set card` keeps its scoped `display:flex` (unlayered beats `.card`'s layered `display:block`).

- [ ] **Step 3: Replace the in-template `$isActive` computation**

In the same file, in the `{{ range $index, $setDisplay := .SetsDisplay }}` loop, change:

```gohtml
            {{ $isActive := and $.ExerciseSet.WarmupCompletedAt (or (and (not $set.CompletedValue) (eq $.FirstIncompleteIndex $index)) (and $.IsEditing (eq $index $.EditingIndex))) }}
```

   to:

```gohtml
            {{ $isActive := $setDisplay.IsActive }}
```

The `$isActive` variable name is unchanged, so the downstream `{{ if $isActive }}` blocks and the `{{ if $isActive }}aria-current="step"{{ end }}` attribute need no edits.

- [ ] **Step 4: Rebuild and run the full exerciseset suite**

Run: `make build && go test ./cmd/web/ -run 'Test_computeSetActive|Test_application_exerciseSet|TestExerciseSetGET_DeloadHidesSignalButtons|Test_ExerciseSet_RestChipAfterCompletedSet' -v`
Expected: PASS — `.exercise-set.completed` and `.exercise-set.active` still match, the warmup→set→edit flow still works, the deload session still hides signal buttons.

- [ ] **Step 5: Commit**

```bash
git add ui/templates/pages/exerciseset/sets-container.gohtml
git commit -m "$(cat <<'EOF'
Migrate sets-container to .card and handler-derived active state

The .exercise-set surface comes from .card; .completed/.active
overrides stay scoped. The template now reads setDisplay.IsActive
instead of deriving active state inline. The set forms, inputs,
signal buttons, deload banner, and rest chip are unchanged — this
hard component stays page-specific.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 8: Full verification

Runs the complete CI gate over both migrated pages.

**Files:** none — verification only.

- [ ] **Step 1: Run the full focused page suite**

Run: `go test ./cmd/web/ -run 'Test_application_home|Test_application_exerciseSet|Test_application_workoutSwapExercise|TestExerciseSetGET_DeloadHidesSignalButtons|Test_ExerciseSet_RestChipAfterCompletedSet|Test_computeSetActive|Test_application_exerciseSet_assisted_storage|Test_application_exerciseSet_nonexistent_exercise_returns_custom_404|Test_application_exerciseSet_swap_preserves_url_and_drops_completed_sets' -v`
Expected: PASS.

- [ ] **Step 2: Run `make ci`**

Run: `make ci`
Expected: `init`, `build`, `lint`, `test`, `sec` all pass. This includes the Playwright suite (`playwright_test.go`), which drives both pages end-to-end.

- [ ] **Step 3: If `make ci` fails for a reason unrelated to this work**

STOP and report. Per the plan's guardrails: a test failing for a reason unrelated to the current task, or `make ci` failing after a clean implementation, is a stop-and-ask condition — do not paper over it.

- [ ] **Step 4: No commit**

Task 8 produces no code changes. If `make ci` is green, the branch is ready for `superpowers:finishing-a-development-branch`.

---

## Self-Review Notes

- **Spec coverage:** Task 1 → authenticated home `page-header`/`.stack`; Task 2 → `day-cards` `.card`/`.cluster` + dead-class cleanup (+ `progress-bar` reviewed-no-change); Task 3 → `muscle-balance` `.card`/`.stack`; Task 4 → `unauthenticated` `.stack`/`.cluster`; Task 5 → `exercise-header` `back-link` component + `.cluster`, `exerciseset.gohtml` `.stack` (+ `warmup` reviewed-no-change); Tasks 6–7 → `sets-container` `.card` + the `$isActive` → `setDisplay.IsActive` template-logic fix; Task 8 → `make ci`. All spec deliverables map to a task.
- **Decisions honored:** no new shared component, no new `.badge` variant, `.warmup-banner`/`.deload-banner` left page-specific, `field` not forced into `sets-container`, no styleguide/`CLAUDE.md` change.
- **Type consistency:** `computeSetActive(warmupComplete, completed bool, index, firstIncompleteIndex, editingIndex int, isEditing bool) bool` and `setDisplay.IsActive bool` are defined in Task 6 and consumed identically in Task 6 Step 5 and Task 7 Step 3. `PageHeaderData{Title, Subtitle}` matches `cmd/web/components.go`.
- **Independently green:** Task 6 adds the handler field without touching the template (template still self-computes), so the build stays green; Task 7 then switches the template over. No task leaves the tree red.
