# Aesthetics Uplift — Increment 1: Token & Component Foundation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Repaint the design-token layer and shared components to the warm "Stone" visual direction so the entire app shifts palette coherently in one increment, without touching any individual page's scoped styles yet.

**Architecture:** All colour/shadow changes land in `ui/static/main.css` `@layer props` (new Stone/Clay/Ember ramps, retuned `--gray-*` aliases, warm-tinted shadows, re-pointed semantic tokens). The reset switches the body font off the serif face and the app background onto a warm surface. Shared components (`button`/`.btn`, `.badge` in `main.css`; `banner` and `back-link` partials) are re-pointed at the new tokens. The `page-header` and `field` partials and `.card` already use only semantic tokens, so they shift automatically. `/dev/styleguide` gains Stone and Clay swatch sections, asserted by `handler-styleguide_test.go`.

**Tech Stack:** CSS (custom properties, `@layer`, `@scope`), Go `html/template`, Go (`cmd/web` handler + `goquery`-based e2e test).

**Spec:** `docs/superpowers/specs/2026-05-14-aesthetics-uplift-design.md` (Increment 1).

---

## File Structure

### Modified files

| File | Change |
|---|---|
| `ui/static/main.css` | `@layer props`: add Stone/Clay/Ember ramps, retune `--gray-*` as aliases onto Stone, warm-tint `--shadow-*`, add `--color-warning*`/`--color-error*`, re-point all semantic colour tokens. `@layer reset`: body font → `--font-sans`, app background → `--color-surface`. `@layer components`: `button`/`.btn` → clay + softer radius; `.badge` variants → warmed functional tokens. |
| `ui/templates/base.gohtml` | `theme-color` meta → warm near-black `#1c1813`. |
| `ui/templates/components/banner.gohtml` | `--red-*`/`--lime-*`/`--sky-*` → `--color-error*`/`--color-success*`/`--color-info*`. |
| `ui/templates/components/back-link.gohtml` | Hover background `--gray-1` → `--stone-2`. |
| `cmd/web/handler-styleguide.go` | Add `scaleStoneMax`/`scaleClayMax` consts, `Stones`/`Clays` slices, new semantic token names, extend `colorTokens` union. |
| `ui/templates/pages/styleguide/styleguide.gohtml` | Add `<h3>Stone</h3>` and `<h3>Clay</h3>` swatch grids to the Colors section. |
| `cmd/web/handler-styleguide_test.go` | Assert the Stone and Clay sections render with swatches. |

### Untouched (shift automatically via re-pointed semantic tokens)

- `ui/templates/components/page-header.gohtml`, `ui/templates/components/field.gohtml` — already use only semantic tokens.
- `.card` in `main.css` `@layer components` — already uses `--color-surface-elevated`, `--color-border`, `--shadow-1`.
- Every `pages/**` template — their scoped styles are polished in Increments 2-5. During this increment they still render coherently because `--gray-*` is aliased onto Stone and `--sky/lime/yellow/red-*` raw scales are left intact.

---

## Sequencing rationale

Task 1 lands the whole token layer first — nothing references the new ramps yet, so it is a safe, build-green base. Tasks 2-5 then re-point consumers (reset, buttons, badges, partials) onto those tokens, each an isolated visual change verifiable on `/dev/styleguide`. Tasks 6-7 are a real TDD loop for the only Go change in the increment (the styleguide handler/template). Task 8 is the full-suite gate.

**A note on verification:** CSS token/colour changes have no unit test — there is no compiler or linter for `main.css`. Tasks 1-5 are verified by (a) the styleguide e2e test still passing, which renders `base.gohtml` plus every component and would catch template breakage, and (b) a described visual check on `/dev/styleguide`, which displays every token and component on one page. Task 8 runs the full `make ci`. This is the appropriate verification shape for a palette repaint; do not invent fake unit tests for hex values.

---

## Task 1: Stone/Clay/Ember ramps, warm shadows, re-pointed semantic tokens

**Files:**
- Modify: `ui/static/main.css` (`@layer props`, lines ~86-213)

- [ ] **Step 1: Warm-tint the shadow tokens**

In `ui/static/main.css`, replace:

```css
        /* Shadows (elevation) */
        --shadow-1: 0 1px 2px 0 rgb(0 0 0 / 0.05);
        --shadow-2: 0 1px 3px 0 rgb(0 0 0 / 0.1), 0 1px 2px -1px rgb(0 0 0 / 0.1);
        --shadow-3: 0 2px 4px 0 rgb(0 0 0 / 0.1);
```

with:

```css
        /* Shadows (elevation) — warm-tinted for the Stone palette */
        --shadow-1: 0 1px 2px 0 rgb(120 90 60 / 0.10);
        --shadow-2: 0 2px 8px 0 rgb(120 90 60 / 0.12), 0 1px 2px -1px rgb(120 90 60 / 0.10);
        --shadow-3: 0 4px 14px 0 rgb(120 90 60 / 0.14);
```

- [ ] **Step 2: Replace the gray ramp with the Stone ramp plus transitional gray aliases**

Replace:

```css
        --gray-0: #f9fafb;
        --gray-1: #f3f4f6;
        --gray-2: #e5e7eb;
        --gray-3: #d1d5db;
        --gray-4: #9ca3af;
        --gray-5: #6b7280;
        --gray-6: #4b5563;
        --gray-7: #374151;
        --gray-8: #1f2937;
        --gray-9: #111827;
        --gray-10: #030712;
```

with:

```css
        /* Stone — warm neutral ramp (surfaces, borders, text) */
        --stone-0: #faf7f2;
        --stone-1: #f4efe6;
        --stone-2: #ece4d9;
        --stone-3: #ddd2c1;
        --stone-4: #c3b5a0;
        --stone-5: #9b8e7d;
        --stone-6: #766b5c;
        --stone-7: #574e42;
        --stone-8: #3d342b;
        --stone-9: #2e2620;
        --stone-10: #1c1813;
        /* Gray — transitional aliases onto Stone. Per-page references are
           renamed to --stone-* as pages are polished; the --gray-* names are
           removed in the final increment of this redesign. */
        --gray-0: var(--stone-0);
        --gray-1: var(--stone-1);
        --gray-2: var(--stone-2);
        --gray-3: var(--stone-3);
        --gray-4: var(--stone-4);
        --gray-5: var(--stone-5);
        --gray-6: var(--stone-6);
        --gray-7: var(--stone-7);
        --gray-8: var(--stone-8);
        --gray-9: var(--stone-9);
        --gray-10: var(--stone-10);
```

- [ ] **Step 3: Add the Clay and Ember ramps after the yellow ramp**

Replace:

```css
        --yellow-12: #663500;

        /* Semantic colors for exercise sets */
```

with:

```css
        --yellow-12: #663500;

        /* Clay — primary accent (buttons, links, active state) */
        --clay-0: #fbf1e8;
        --clay-1: #f0d9c4;
        --clay-2: #e0b48a;
        --clay-3: #c98a55;
        --clay-4: #a8643c;
        --clay-5: #8a4f2e;
        --clay-6: #6b3c22;

        /* Ember — Focus-mode highlight accent */
        --ember: #e08a4c;

        /* Semantic colors */
```

- [ ] **Step 4: Re-point the semantic colour tokens onto Stone/Clay and add warning/error tokens**

Replace:

```css
        --color-surface: var(--gray-0);
        --color-surface-elevated: var(--white);
        --color-surface-active: var(--sky-0);
        --color-surface-completed: var(--lime-0);
        --color-border: var(--gray-3);
        --color-border-focus: var(--sky-4);
        --color-text-primary: var(--gray-9);
        --color-text-secondary: var(--gray-6);
        --color-text-muted: var(--gray-5);
        --color-success: var(--lime-7);
        --color-success-bg: var(--lime-0);
        --color-info: var(--sky-7);
        --color-info-bg: var(--sky-0);
```

with:

```css
        --color-surface: var(--stone-1);
        --color-surface-elevated: var(--stone-0);
        --color-surface-active: var(--clay-0);
        --color-surface-completed: #eef1e2;
        --color-border: var(--stone-3);
        --color-border-focus: var(--clay-3);
        --color-text-primary: var(--stone-9);
        --color-text-secondary: var(--stone-6);
        --color-text-muted: var(--stone-5);
        --color-success: #5c6b38;
        --color-success-bg: #e8ecd9;
        --color-warning: #8a5a17;
        --color-warning-bg: #f6e6c8;
        --color-error: #8a2f1c;
        --color-error-bg: #f6dcd4;
        --color-info: #3f5a68;
        --color-info-bg: #dde8ec;
```

- [ ] **Step 5: Verify the file still parses and the tree builds**

Run: `make build`
Expected: builds with no errors (this confirms the templates that embed `main.css` references still compile; `main.css` itself is served from disk).

- [ ] **Step 6: Commit**

```bash
git add ui/static/main.css
git commit -m "$(cat <<'EOF'
Add the Stone palette token layer

Introduces the warm Stone/Clay/Ember ramps, retunes --gray-* as
transitional aliases onto Stone, warm-tints the shadow tokens, and
re-points the semantic colour tokens. Nothing consumes the new accents
yet — this is the foundation the component and page passes build on.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Body font and app background (reset) + theme-color

**Files:**
- Modify: `ui/static/main.css` (`@layer reset`, lines ~14-24)
- Modify: `ui/templates/base.gohtml:18`

- [ ] **Step 1: Switch the app background to the warm surface and the body font off the serif face**

In `ui/static/main.css`, replace:

```css
    html, body {
        height: 100%;
        color: var(--gray-9);
        background: var(--white)
    }

    body {
        line-height: 1.5;
        -webkit-font-smoothing: antialiased;
        font-family: var(--font-serif);
    }
```

with:

```css
    html, body {
        height: 100%;
        color: var(--color-text-primary);
        background: var(--color-surface);
    }

    body {
        line-height: 1.5;
        -webkit-font-smoothing: antialiased;
        font-family: var(--font-sans);
    }
```

- [ ] **Step 2: Warm the PWA theme-color**

In `ui/templates/base.gohtml`, replace:

```html
        <meta name="theme-color" content="#000"/>
```

with:

```html
        <meta name="theme-color" content="#1c1813"/>
```

- [ ] **Step 3: Verify templates still render**

Run: `go test ./cmd/web/ -run Test_application_styleguide -v`
Expected: PASS (this renders `base.gohtml` end-to-end; a template typo would fail here).

- [ ] **Step 4: Visual check**

Run: `make dev`, open `http://localhost:<port>/dev/styleguide` (port is printed by `make dev`).
Expected: the page background is a warm off-white (not pure white or cool gray), body text is warm near-black, and headings/body now share the system sans face (no serif body text). Stop the dev server when done.

- [ ] **Step 5: Commit**

```bash
git add ui/static/main.css ui/templates/base.gohtml
git commit -m "$(cat <<'EOF'
Move the app shell onto the Stone surface and system sans

Switches the body background to the warm --color-surface, drops the
serif body face for system sans, and warms the PWA theme-color.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Clay buttons with a softer radius

**Files:**
- Modify: `ui/static/main.css` (`@layer components`, `button, .btn` rule, lines ~243-267)

- [ ] **Step 1: Re-point the button colours onto Clay and soften the corner radius**

In `ui/static/main.css`, replace the entire `button, .btn` rule:

```css
    button, .btn {
        position: relative;
        display: inline-flex;
        border-radius: var(--radius-2);
        background-color: var(--sky-10);
        color: var(--white);
        font-family: var(--font-sans);
        font-size: var(--font-size-1);
        font-weight: var(--font-weight-7);
        letter-spacing: var(--font-letterspacing-3);
        --vertical-padding: 2.5em;
        padding: var(--size-2) var(--vertical-padding);
        border: none;
        white-space: nowrap;
        text-decoration: none;

        &:hover {
            cursor: pointer;
            background-color: var(--sky-7);
        }

        &:focus-visible {
            outline: var(--sky-4) solid 2px;
        }
    }
```

with:

```css
    button, .btn {
        position: relative;
        display: inline-flex;
        border-radius: var(--radius-3);
        background-color: var(--clay-4);
        color: var(--white);
        font-family: var(--font-sans);
        font-size: var(--font-size-1);
        font-weight: var(--font-weight-7);
        letter-spacing: var(--font-letterspacing-3);
        --vertical-padding: 2.5em;
        padding: var(--size-2) var(--vertical-padding);
        border: none;
        white-space: nowrap;
        text-decoration: none;

        &:hover {
            cursor: pointer;
            background-color: var(--clay-5);
        }

        &:focus-visible {
            outline: var(--clay-3) solid 2px;
        }
    }
```

- [ ] **Step 2: Verify templates still render**

Run: `go test ./cmd/web/ -run Test_application_styleguide -v`
Expected: PASS.

- [ ] **Step 3: Visual check**

Run: `make dev`, open `/dev/styleguide`, scroll to the "Components → Buttons" section.
Expected: both the `<button>` and the `.btn` anchor are terracotta/clay, with visibly rounder corners than before; hover darkens the clay. Stop the dev server when done.

- [ ] **Step 4: Commit**

```bash
git add ui/static/main.css
git commit -m "$(cat <<'EOF'
Restyle the button component to clay with a softer radius

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Warm the badge variants

**Files:**
- Modify: `ui/static/main.css` (`@layer components`, `.badge*` rules, lines ~283-316)

- [ ] **Step 1: Re-point the badge base and variants onto Stone and the warmed functional tokens**

In `ui/static/main.css`, replace:

```css
    .badge {
        display: inline-flex;
        align-items: center;
        gap: var(--size-1);
        padding: var(--size-1) var(--size-2);
        border-radius: var(--radius-2);
        font-family: var(--font-sans);
        font-size: var(--font-size-0);
        font-weight: var(--font-weight-6);
        text-transform: uppercase;
        letter-spacing: var(--font-letterspacing-2);
        background: var(--gray-2);
        color: var(--gray-9);
    }

    .badge--success {
        background: var(--lime-2);
        color: var(--lime-9);
    }

    .badge--warning {
        background: var(--yellow-2);
        color: var(--yellow-11);
    }

    .badge--neutral {
        background: var(--gray-2);
        color: var(--gray-9);
    }

    .badge--info {
        background: var(--sky-1);
        color: var(--sky-9);
    }
```

with:

```css
    .badge {
        display: inline-flex;
        align-items: center;
        gap: var(--size-1);
        padding: var(--size-1) var(--size-2);
        border-radius: var(--radius-2);
        font-family: var(--font-sans);
        font-size: var(--font-size-0);
        font-weight: var(--font-weight-6);
        text-transform: uppercase;
        letter-spacing: var(--font-letterspacing-2);
        background: var(--stone-2);
        color: var(--stone-9);
    }

    .badge--success {
        background: var(--color-success-bg);
        color: var(--color-success);
    }

    .badge--warning {
        background: var(--color-warning-bg);
        color: var(--color-warning);
    }

    .badge--neutral {
        background: var(--stone-2);
        color: var(--stone-8);
    }

    .badge--info {
        background: var(--color-info-bg);
        color: var(--color-info);
    }
```

- [ ] **Step 2: Verify templates still render**

Run: `go test ./cmd/web/ -run Test_application_styleguide -v`
Expected: PASS.

- [ ] **Step 3: Visual check**

Run: `make dev`, open `/dev/styleguide`, scroll to the "Badges" section.
Expected: all four badges (success, warning, neutral, info) sit in the warm palette — muted sage, amber, stone, and dusty-blue rather than the previous bright Tailwind hues. Stop the dev server when done.

- [ ] **Step 4: Commit**

```bash
git add ui/static/main.css
git commit -m "$(cat <<'EOF'
Warm the badge variants onto the Stone functional tokens

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Warm the banner and back-link partials

**Files:**
- Modify: `ui/templates/components/banner.gohtml:20-33`
- Modify: `ui/templates/components/back-link.gohtml:26`

- [ ] **Step 1: Re-point the banner variant colours onto the semantic functional tokens**

In `ui/templates/components/banner.gohtml`, replace:

```css
                :scope.banner--error {
                    background: var(--red-1);
                    color: var(--red-12);
                }

                :scope.banner--success {
                    background: var(--lime-1);
                    color: var(--lime-9);
                }

                :scope.banner--info {
                    background: var(--sky-1);
                    color: var(--sky-9);
                }
```

with:

```css
                :scope.banner--error {
                    background: var(--color-error-bg);
                    color: var(--color-error);
                }

                :scope.banner--success {
                    background: var(--color-success-bg);
                    color: var(--color-success);
                }

                :scope.banner--info {
                    background: var(--color-info-bg);
                    color: var(--color-info);
                }
```

- [ ] **Step 2: Re-point the back-link hover background onto Stone**

In `ui/templates/components/back-link.gohtml`, in the `&:hover` block, replace:

```css
                &:hover {
                    background: var(--gray-1);
                    color: var(--color-text-primary);
                }
```

with:

```css
                &:hover {
                    background: var(--stone-2);
                    color: var(--color-text-primary);
                }
```

- [ ] **Step 3: Verify templates still render**

Run: `go test ./cmd/web/ -run Test_application_styleguide -v`
Expected: PASS (the styleguide renders all three banner variants and the back-link).

- [ ] **Step 4: Visual check**

Run: `make dev`, open `/dev/styleguide`, scroll to the "Banner" and "Back link" sections.
Expected: the error/success/info banners use the warm functional pairs (dusty red-brown, sage, dusty blue); hovering the back link gives a warm sand background. Stop the dev server when done.

- [ ] **Step 5: Commit**

```bash
git add ui/templates/components/banner.gohtml ui/templates/components/back-link.gohtml
git commit -m "$(cat <<'EOF'
Warm the banner and back-link partials onto Stone tokens

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Failing styleguide test for the Stone and Clay sections

**Files:**
- Modify: `cmd/web/handler-styleguide_test.go`

- [ ] **Step 1: Add assertions for the Stone and Clay colour sections**

In `cmd/web/handler-styleguide_test.go`, immediately after the `client := server.Client()` block and the `doc, err := client.GetDoc(...)` block (i.e. right before the `// Layout primitives.` comment), insert:

```go
	// Stone and Clay ramps — the core of the Stone design direction.
	if doc.Find("h3:contains('Stone')").Length() == 0 {
		t.Error("expected a 'Stone' colour section on the styleguide")
	}
	if doc.Find(".bg-stone-5").Length() == 0 {
		t.Error("expected a --stone-5 swatch on the styleguide")
	}
	if doc.Find("h3:contains('Clay')").Length() == 0 {
		t.Error("expected a 'Clay' colour section on the styleguide")
	}
	if doc.Find(".bg-clay-4").Length() == 0 {
		t.Error("expected a --clay-4 swatch on the styleguide")
	}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/web/ -run Test_application_styleguide -v`
Expected: FAIL — `expected a 'Stone' colour section on the styleguide`, `expected a --stone-5 swatch ...`, `expected a 'Clay' colour section ...`, `expected a --clay-4 swatch ...` (the styleguide does not render these sections yet).

- [ ] **Step 3: Commit**

```bash
git add cmd/web/handler-styleguide_test.go
git commit -m "$(cat <<'EOF'
Add failing styleguide assertions for the Stone and Clay ramps

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Render the Stone and Clay sections on the styleguide

**Files:**
- Modify: `cmd/web/handler-styleguide.go`
- Modify: `ui/templates/pages/styleguide/styleguide.gohtml`

- [ ] **Step 1: Add the Stone and Clay scale-extent constants**

In `cmd/web/handler-styleguide.go`, in the `const (...)` block, add two entries (keep the existing ones):

```go
const (
	scaleGrayMax       = 10
	scaleStoneMax      = 10
	scaleClayMax       = 6
	scaleSkyMax        = 10
	scaleLimeMax       = 10
	scaleRedMax        = 12
	scaleYellowMax     = 12
	scaleSizeMax       = 15
	scaleFontSizeMax   = 8
	scaleFluidFontMax  = 3
	scaleFontWeightMax = 9
	scaleRadiusMax     = 6
)
```

- [ ] **Step 2: Add the `Stones` and `Clays` fields to the template-data struct**

In `cmd/web/handler-styleguide.go`, in `type styleguideTemplateData struct`, add the two fields after `Grays`:

```go
type styleguideTemplateData struct {
	BaseTemplateData
	Grays          []string
	Stones         []string
	Clays          []string
	Skies          []string
	Limes          []string
	Reds           []string
	Yellows        []string
	SemanticColors []string
	// ColorTokens is the union of all color slices above. The template iterates over
	// it once to emit a `.bg-{name}` utility rule per token, avoiding inline `style=`
	// attributes (blocked by the `style-src 'nonce-...'` CSP).
	ColorTokens    []string
	Sizes          []string
	FontSizes      []string
	FluidFontSizes []string
	FontWeights    []string
	Radii          []string
	// BannerExamples drives the Banner section of the styleguide.
	BannerExamples []BannerData
	// PageHeaderExample drives the Page header section of the styleguide.
	PageHeaderExample PageHeaderData
	// FieldExamples drives the Field section of the styleguide.
	FieldExamples []FieldData
}
```

- [ ] **Step 3: Build the Stone/Clay slices, extend the semantic list and the union, and populate the struct**

In `cmd/web/handler-styleguide.go`, in `styleguideGET`, replace this block:

```go
	grays := rangeNames("gray", 0, scaleGrayMax)
	skies := rangeNames("sky", 0, scaleSkyMax)
	limes := rangeNames("lime", 0, scaleLimeMax)
	reds := rangeNames("red", 0, scaleRedMax)
	yellows := rangeNames("yellow", 0, scaleYellowMax)
	semantics := []string{
		"color-surface",
		"color-surface-elevated",
		"color-surface-active",
		"color-surface-completed",
		"color-border",
		"color-border-focus",
		"color-text-primary",
		"color-text-secondary",
		"color-text-muted",
		"color-success",
		"color-success-bg",
		"color-info",
		"color-info-bg",
	}
	colorTokens := make([]string, 0, len(grays)+len(skies)+len(limes)+len(reds)+len(yellows)+len(semantics))
	colorTokens = append(colorTokens, grays...)
	colorTokens = append(colorTokens, skies...)
	colorTokens = append(colorTokens, limes...)
	colorTokens = append(colorTokens, reds...)
	colorTokens = append(colorTokens, yellows...)
	colorTokens = append(colorTokens, semantics...)
```

with:

```go
	grays := rangeNames("gray", 0, scaleGrayMax)
	stones := rangeNames("stone", 0, scaleStoneMax)
	clays := rangeNames("clay", 0, scaleClayMax)
	skies := rangeNames("sky", 0, scaleSkyMax)
	limes := rangeNames("lime", 0, scaleLimeMax)
	reds := rangeNames("red", 0, scaleRedMax)
	yellows := rangeNames("yellow", 0, scaleYellowMax)
	semantics := []string{
		"color-surface",
		"color-surface-elevated",
		"color-surface-active",
		"color-surface-completed",
		"color-border",
		"color-border-focus",
		"color-text-primary",
		"color-text-secondary",
		"color-text-muted",
		"color-success",
		"color-success-bg",
		"color-warning",
		"color-warning-bg",
		"color-error",
		"color-error-bg",
		"color-info",
		"color-info-bg",
		"ember",
	}
	colorTokens := make([]string, 0,
		len(grays)+len(stones)+len(clays)+len(skies)+len(limes)+len(reds)+len(yellows)+len(semantics))
	colorTokens = append(colorTokens, grays...)
	colorTokens = append(colorTokens, stones...)
	colorTokens = append(colorTokens, clays...)
	colorTokens = append(colorTokens, skies...)
	colorTokens = append(colorTokens, limes...)
	colorTokens = append(colorTokens, reds...)
	colorTokens = append(colorTokens, yellows...)
	colorTokens = append(colorTokens, semantics...)
```

Then, in the `data := styleguideTemplateData{...}` literal, add the two fields right after `Grays: grays,`:

```go
		Grays:            grays,
		Stones:           stones,
		Clays:            clays,
		Skies:            skies,
```

- [ ] **Step 4: Add the Stone and Clay swatch grids to the styleguide template**

In `ui/templates/pages/styleguide/styleguide.gohtml`, in the `<section>` whose heading is `<h2>Colors</h2>`, replace:

```gohtml
        <section>
            <h2>Colors</h2>

            <h3>Gray</h3>
```

with:

```gohtml
        <section>
            <h2>Colors</h2>

            <h3>Stone</h3>
            <div class="color-grid">
                {{ range $name := .Stones }}
                    <div class="swatch">
                        <div class="swatch-color bg-{{ $name }}"></div>
                        <div class="swatch-label">--{{ $name }}</div>
                    </div>
                {{ end }}
            </div>

            <h3>Clay (primary accent)</h3>
            <div class="color-grid">
                {{ range $name := .Clays }}
                    <div class="swatch">
                        <div class="swatch-color bg-{{ $name }}"></div>
                        <div class="swatch-label">--{{ $name }}</div>
                    </div>
                {{ end }}
            </div>

            <h3>Gray (transitional — aliased onto Stone, removed in a later increment)</h3>
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./cmd/web/ -run Test_application_styleguide -v`
Expected: PASS — the Stone and Clay sections now render with `.bg-stone-5` and `.bg-clay-4` swatches.

- [ ] **Step 6: Visual check**

Run: `make dev`, open `/dev/styleguide`, look at the top of the "Colors" section.
Expected: a "Stone" 11-swatch ramp and a "Clay (primary accent)" 7-swatch ramp appear above the (now warm-aliased) "Gray" ramp. The Semantic grid further down shows the new `--color-warning*`, `--color-error*`, and `--ember` swatches. Stop the dev server when done.

- [ ] **Step 7: Commit**

```bash
git add cmd/web/handler-styleguide.go ui/templates/pages/styleguide/styleguide.gohtml
git commit -m "$(cat <<'EOF'
Add Stone and Clay swatch sections to the styleguide

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Full-suite verification and visual QA pass

**Files:** none (verification only)

- [ ] **Step 1: Run the full CI suite**

Run: `make ci`
Expected: `init`, `build`, `lint-fix`, `test`, `sec` all pass. If `lint-fix` makes formatting changes, review them, `git add` them, and commit with message `Apply lint-fix formatting`.

- [ ] **Step 2: Visual QA across the real pages**

Run: `make dev`, then open and eyeball each of:
- `/` (authenticated home / schedule) — confirm the page background is warm, day-cards still legible, the "Menu" button and any `.btn` anchors are clay.
- `/dev/styleguide` — confirm Stone/Clay/Semantic sections, warm shadows on the elevation samples, clay buttons, warm badges and banners.
- One workout page (`/workouts/<date>`) — confirm the status badge, exercise cards, and the sticky "Complete workout" button read in the warm palette with no broken contrast.

Expected: every page reads as warm "Stone"; no element is left on a cool gray/blue surface that looks out of place. The raw `--sky/lime/yellow/red-*` page-level usages (e.g. day-card status tints) will still be their original hues — that is expected and is cleaned up in Increments 2-5. Stop the dev server when done.

- [ ] **Step 3: Final confirmation**

Confirm `git status` is clean and `git log --oneline` shows the Task 1-7 commits (plus any lint-fix commit). Increment 1 is complete: the app shell, tokens, and shared components are on the Stone system.

---

## Self-review notes

- **Spec coverage:** Increment 1 of the spec calls for — retune `--gray-*` to Stone (Task 1.2), add Clay + Ember (Task 1.3), re-point semantic tokens (Task 1.4), soften radii (Task 3, button → `--radius-3`), warm shadows (Task 1.1), drop the serif body face (Task 2.1), restyle `button`/`.btn` (Task 3), `.card` (automatic — uses semantic tokens only, verified in Task 8.2), `.badge` (Task 4), the partials (Task 5 for banner/back-link; page-header/field automatic), update `/dev/styleguide` (Tasks 6-7), keep `handler-styleguide_test.go` green (Tasks 6-7). All covered.
- **Functional colour values** (`--color-success` etc.) are the spec's "target direction, may be tuned" set, pinned to concrete hexes here so the increment is unambiguous.
- **`.card` radius:** the spec's "cards a touch rounder" is implemented as the button radius bump (`--radius-2` → `--radius-3`); `.card` is already at `--radius-3`. No new radius token is introduced — the existing scale is sufficient.
- **Out of scope for this increment (per the spec's foundation-first staging):** all `pages/**` scoped styles, the raw `--sky/lime/yellow/red-*` call sites, Focus mode, and the motion pass. Those are Increments 2-6, each with its own plan.
