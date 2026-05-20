# Text Overflow & Localization-Readiness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the home-page day-card text bleed and document overflow-prevention practices so future components stay localization-ready.

**Architecture:** The day-card grid (`1fr auto`) is replaced by a flex column whose name+action sub-row uses `flex-wrap: wrap`, so a long action drops to its own line instead of overflowing the card. The corrected component then becomes the worked example for a new "Text overflow & localization-readiness" section in the UI guidelines.

**Tech Stack:** Go `html/template` (`.gohtml`), scoped CSS via `@scope` in nonce'd `<style>` blocks, e2etest/goquery handler tests.

**Note on TDD:** This is a CSS-layout + documentation change. CSS overflow behaviour cannot be asserted through goquery (no layout engine), so there is no failing unit test to write first. The existing `Test_application_home` handler test is the regression guard; correctness of the wrap behaviour is confirmed by a visual check at 320px width. Tasks below reflect that honestly.

**Spec:** `docs/superpowers/specs/2026-05-20-text-overflow-localization-design.md`

---

### Task 1: Fix the day-card overflow

**Files:**
- Modify: `ui/templates/pages/home/day-cards.gohtml` (scoped CSS in the `<style>` block, lines ~11–81; markup, lines ~243–263)

The card currently lays out as `grid-template-columns: 1fr auto`. A bare `1fr` is `minmax(auto, 1fr)`, so column 1 will not shrink below the min-content width of the day name and the grid overflows. The fix: make `.day` a flex column, and wrap the day name + action in a new `.day-headline` element that is a wrapping flex row.

- [ ] **Step 1: Convert `.day` from a 2-column grid to a flex column**

In `ui/templates/pages/home/day-cards.gohtml`, replace:

```css
                .day {
                    position: relative;
                    display: grid;
                    grid-template-columns: 1fr auto;
                    column-gap: var(--size-3);
                    row-gap: var(--size-2);
                    padding: var(--size-4);
```

with:

```css
                .day {
                    position: relative;
                    display: flex;
                    flex-direction: column;
                    gap: var(--size-2);
                    padding: var(--size-4);
```

(Leave the rest of the `.day` rule — `border-radius`, `background`, `border`, `box-shadow`, `transition` — unchanged.)

- [ ] **Step 2: Drop the grid-span from `.day-overline`**

Replace:

```css
                /* Top row: mono date overline + right-aligned status meta. Spans full width. */
                .day-overline {
                    grid-column: 1 / -1;
                    display: flex;
```

with:

```css
                /* Top row: mono date overline + right-aligned status meta. */
                .day-overline {
                    display: flex;
```

- [ ] **Step 3: Add the `.day-headline` wrapping row and clean up `.day-name`**

Replace:

```css
                /* Serif day name — the editorial anchor. */
                .day-name {
                    grid-column: 1;
                    align-self: center;
                    margin: 0;
```

with:

```css
                /* Day name + action share a wrapping row: side by side when they
                   fit, the action drops to its own line when the combined width
                   would bleed past the card edge. */
                .day-headline {
                    display: flex;
                    flex-wrap: wrap;
                    align-items: center;
                    justify-content: space-between;
                    gap: var(--size-2) var(--size-3);
                }

                /* Serif day name — the editorial anchor. */
                .day-name {
                    min-width: 0;
                    margin: 0;
```

(`min-width: 0` lets the name shrink/wrap instead of forcing the action off-edge. The rest of the `.day-name` rule — `font-family`, `font-weight`, `font-size`, `line-height`, `letter-spacing`, `color` — stays unchanged.)

- [ ] **Step 4: Strip grid props from `.day-action`**

Replace:

```css
                .day-action {
                    grid-column: 2;
                    align-self: center;
                    justify-self: end;
                }
```

with:

```css
                .day-action {
                    min-width: 0;
                }
```

(`flex-wrap` on `.day-headline` plus `justify-content: space-between` now handle placement; `min-width: 0` lets a long action wrap internally rather than bleed. Leave the `.day-action form { margin: 0; }` rule that follows unchanged.)

- [ ] **Step 5: Drop the grid-span from `.day-progress`**

Replace:

```css
                /* Slim sub-row: 3px rule + tiny mono caption. Spans full width. */
                .day-progress {
                    grid-column: 1 / -1;
                    display: flex;
```

with:

```css
                /* Slim sub-row: 3px rule + tiny mono caption. */
                .day-progress {
                    display: flex;
```

- [ ] **Step 6: Wrap the day name + action in `.day-headline` in the markup**

Replace:

```gohtml
                <h2 class="day-name">{{ .Name }}</h2>

                {{ if .Action }}
                    <div class="day-action">
                        {{ if eq .Status "today" }}
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

with:

```gohtml
                <div class="day-headline">
                    <h2 class="day-name">{{ .Name }}</h2>

                    {{ if .Action }}
                        <div class="day-action">
                            {{ if eq .Status "today" }}
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
                </div>
```

- [ ] **Step 7: Run the home handler test (regression guard)**

Run: `go test -v ./cmd/web -run Test_application_home`
Expected: PASS. The test asserts on `.weekly-schedule` existence and section ordering, not layout — adding the `.day-headline` wrapper inside `.day` does not affect it.

- [ ] **Step 8: Run the full test suite and linter**

Run: `make test`
Expected: all packages PASS.

Run: `make lint-fix`
Expected: no outstanding lint errors.

- [ ] **Step 9: Visual verification at narrow width**

Run: `make dev` (templates load from the filesystem at runtime — no rebuild needed).
Open the home page in a browser, set the viewport to 320px wide, and confirm for every day-card status variant — `today`, `in_progress`, `completed`, `past-incomplete`, `upcoming`, `unscheduled` — that:
- the day name and action sit side by side when they fit;
- the longest combination ("Wednesday" + "Start Extra Workout") wraps the action onto its own left-aligned line with no text bleeding past the card edge;
- the `completed` strike-through, the left ribbon, and the progress bar still render correctly.

If you cannot drive a browser, say so explicitly and ask the user to perform this check rather than claiming success.

- [ ] **Step 10: Commit**

```bash
git add ui/templates/pages/home/day-cards.gohtml
git commit -m "$(cat <<'EOF'
ui: stop day-card text bleeding past the card edge

Replace the day-card 1fr/auto grid with a flex column whose name+action
sub-row wraps, so a long action drops to its own line instead of
overflowing. A bare 1fr is minmax(auto,1fr) and refuses to shrink below
its content's min-content width.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Document overflow-prevention practices

**Files:**
- Modify: `ui/templates/CLAUDE.md` (insert a new section after "## Accessibility", before "## Template Data Preparation")

- [ ] **Step 1: Insert the new section**

In `ui/templates/CLAUDE.md`, find the end of the "## Accessibility" section — the last bullet under "### Other standing requirements" ends with:

```markdown
- **Focus rings**: prefer `outline-offset: 2px` with a high-contrast
  colour (`--stone-9` or a two-tone `outline` + `box-shadow`). Don't
  rely on a light-tone outline against a similarly-toned background.
```

Immediately after that bullet (and before the `## Template Data Preparation` heading), insert:

```markdown

## Text overflow & localization-readiness

Layouts must survive text longer than today's English strings. The app is
not localized yet, but German / Finnish / Russian routinely run 30–50%
longer — a layout validated only against compact English WILL break on
translation. Build the resilience in now.

### Let flex/grid children shrink

Flex and grid items default to `min-width: auto` (their min-content width)
and refuse to shrink below their longest word. This is the single most
common cause of text bleeding past a container.

- Use `minmax(0, 1fr)` instead of a bare `1fr` for grid tracks that should
  yield. A bare `1fr` is `minmax(auto, 1fr)` — it will not shrink.
- Put `min-width: 0` on text-bearing flex children.
- Apply `justify-self: end` / `margin-inline-start: auto` only *after* the
  element can shrink — otherwise it pushes itself off the edge.

### Give every text node one overflow strategy

Pick exactly one, deliberately:

- **Wrap** (default, preferred) — no fixed height; if the row holds
  competing items, give the row `flex-wrap: wrap` so the lower-priority
  item drops to its own line.
- **Break** unbreakable tokens (long compounds, URLs) — `overflow-wrap:
  anywhere`. Use `anywhere`, not `break-word`: only `anywhere` shrinks
  min-content, so only `anywhere` also fixes grid/flex track sizing. Add
  `hyphens: auto` for prose.
- **Truncate** — `text-overflow: ellipsis` or `-webkit-line-clamp`, only
  for secondary, repeating text whose full value is reachable elsewhere
  (a `title`, an `aria-label`, a detail view). Never silently truncate a
  primary label.

### Don't let two flexible elements fight for one row

Two elements that both want horizontal space will collide. Either let the
row wrap, or let the lower-priority element move to its own line. No fixed
`width` / `height` on text containers — `min-*` plus padding is fine; a
size fitted to an English label breaks in other languages.

### Test for growth

Budget roughly +40% length. Check new components at 320px width with a
deliberately long label, not just the English string. Keep source labels
short — but a short label is never a substitute for a layout that
survives a long one.

Worked example: `pages/home/day-cards.gohtml` — the day name and action
share a `flex-wrap: wrap` row (`.day-headline`), so a long action drops
to its own line instead of bleeding past the card.
```

- [ ] **Step 2: Verify the section reads correctly in context**

Run: `grep -n "Text overflow\|Template Data Preparation" ui/templates/CLAUDE.md`
Expected: the "## Text overflow & localization-readiness" heading appears before "## Template Data Preparation", with no duplicated or orphaned headings.

- [ ] **Step 3: Commit**

```bash
git add ui/templates/CLAUDE.md
git commit -m "$(cat <<'EOF'
docs: add text-overflow & localization-readiness UI guidelines

Document the practices that keep layouts resilient to longer strings,
pointing at the day-card fix as the worked example.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review

**Spec coverage:**
- Spec "Part 1 — Documentation" (new section, four rule groups, placement after Accessibility, worked-example pointer) → Task 2. ✓
- Spec "Part 2 — Fix `day-cards.gohtml`" (wrapping flex row, drop `justify-self: end`, defensive `min-width: 0`, markup wrapper, preserve all status variants) → Task 1, Steps 1–6 and 9. ✓
- Spec "Testing" (`make test`, `make lint-fix`, 320px visual check) → Task 1, Steps 7–9. ✓
- Spec non-goals (no i18n framework, no app-wide audit, no tooling) → respected; no task introduces them. ✓

**Placeholder scan:** No TBD/TODO; every code step shows exact before/after content. ✓

**Type consistency:** The new class `.day-headline` is defined in Task 1 Step 3 (CSS) and used in Task 1 Step 6 (markup) and referenced in Task 2's worked-example pointer — consistent spelling throughout. ✓
