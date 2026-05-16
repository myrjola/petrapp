# Add/Swap Exercise card redesign

## Problem

The Add Exercise and Swap Exercise pages (`ui/templates/pages/exercise-add/`
and `ui/templates/pages/exercise-swap/`) render a list of exercise cards
whose visual hierarchy is inverted and whose metadata is presented as flat
grey text. On mobile this reads as cluttered and confusing — the secondary
"Info" button is the loudest element on the card while the primary "Add" /
"Swap" action whispers in pale green underneath it. Category and primary
muscle groups appear as `Upper Body` / `Primary: Triceps` prose instead of
using the chip/badge vocabulary the rest of the app uses (e.g. the muscle
pills on `exercise-info`).

Two further code-health issues fall out of fixing this:

- The `<dialog>` used to show exercise info carries ~50 lines of inline
  scoped CSS, emitted once per exercise in both templates (N exercises × 2
  pages of duplication).
- The clay-1/clay-6 "primary muscle" pill style is already defined locally
  in `exercise-info.gohtml`. Add/Swap will be the second and third sites to
  want it, which hits the "extract when three call sites exist" threshold
  from `ui/templates/CLAUDE.md`.

## Goals

1. Make the primary action on each card the visually dominant action, and
   the Info button clearly secondary.
2. Replace the flat grey metadata text with category + muscle-group chips
   that match the existing design language.
3. Eliminate the per-exercise duplication of the info-dialog styles.
4. Apply the same changes to both Add and Swap, so the pages share one
   card pattern.

## Non-goals

- No changes to search behaviour (the GET form + submit button stays).
- No restructuring of how Swap presents the current → new relationship.
- No changes to handlers, data-prep, or any layer below the templates.
- No changes to the Info dialog's JS open behaviour — `me().addEventListener('click', …)`
  on the per-card trigger stays.

## Design

### Card markup (applies to both pages)

```gohtml
<div class="exercise-option card stack">
    <div class="exercise-name">{{ .Name }}</div>

    <div class="meta cluster">
        <span class="badge">{{ .Category.Label }}</span>
        {{ range .PrimaryMuscleGroups }}
            <span class="muscle-chip muscle-chip--primary">{{ . }}</span>
        {{ end }}
    </div>

    <div class="actions cluster">
        <form method="post" action="…" class="add-form">
            <input type="hidden" name="exercise_id" value="{{ .ID }}">
            <button type="submit" class="btn btn--quiet">Add this exercise</button>
        </form>
        <button type="button" class="btn btn--ghost btn--sm info-trigger">
            <script {{ nonce }}>
              me().addEventListener('click', () => {
                document.getElementById('dialog-exercise-{{ .ID }}').showModal()
              })
            </script>
            Info
        </button>
    </div>

    <dialog id="dialog-exercise-{{ .ID }}" class="sheet-dialog">
        <form method="dialog" class="sheet-dialog__close">
            <button class="btn btn--ghost btn--sm">Close</button>
        </form>
        {{ mdToHTML .DescriptionMarkdown }}
    </dialog>
</div>
```

A single scoped `<style>` block at the section level handles:

- `.exercise-option.card` — base card stack gap (`--size-3`), slightly
  tighter on small screens.
- `.actions.cluster` — primary form takes `flex: 1` so the Add/Swap button
  fills the row and Info stays compact on the right. Gap `--size-2`.
- `.meta.cluster` — wraps to a second line on narrow viewports.

The Swap template uses `Swap to this exercise` and the `/swap` action URL
but otherwise renders the same card.

### `.muscle-chip` — new global component class

Added to `main.css` `@layer components`:

```css
.muscle-chip {
    display: inline-flex;
    align-items: center;
    padding: var(--size-1) var(--size-2);
    border-radius: var(--radius-2);
    font-size: var(--font-size-0);
    font-weight: var(--font-weight-6);
    background: var(--stone-2);
    color: var(--stone-8);
}

.muscle-chip--primary {
    background: var(--clay-1);
    color: var(--clay-6);
}
```

`exercise-info.gohtml` migrates from its local `.muscle-group` /
`.muscle-group.primary` rules to `.muscle-chip` / `.muscle-chip--primary`.
The local scoped rules are removed.

### `.sheet-dialog` — new global component class

Added to `main.css` `@layer components`. Moves the existing slide-up sheet
CSS verbatim out of the two page templates:

```css
.sheet-dialog {
    padding: var(--size-3);
    border: none;
    position: fixed;
    inset: 0;
    transform: translateY(100%);
    animation: sheet-dialog-slide-up 0.3s ease-in-out forwards;
    transition-behavior: allow-discrete;
}

.sheet-dialog::backdrop {
    background-color: rgba(0, 0, 0, 0.5);
    animation: sheet-dialog-fade-in 0.3s ease-out forwards;
}

.sheet-dialog__close {
    display: flex;
    justify-content: flex-end;
    margin-bottom: var(--size-3);
}

@keyframes sheet-dialog-slide-up {
    from { transform: translateY(100%); }
    to   { transform: translateY(0); }
}

@keyframes sheet-dialog-fade-in {
    from { opacity: 0; }
    to   { opacity: 1; }
}
```

The keyframe names are prefixed because `@keyframes` lives in the global
namespace, not under `@layer`.

### Styleguide

Add two entries to `ui/templates/pages/styleguide/styleguide.gohtml`:

- A `.muscle-chip` section showing the base and `--primary` variants.
- A `.sheet-dialog` section with a trigger button that opens an example
  dialog.

Add matching assertions in `cmd/web/handler-styleguide_test.go` so the
styleguide is kept honest.

### Files touched

| File | Change |
| --- | --- |
| `ui/static/main.css` | Add `.muscle-chip*` and `.sheet-dialog*` rules under `@layer components`. |
| `ui/templates/pages/exercise-add/exercise-add.gohtml` | Rewrite card markup; remove per-card dialog styles; use chips + new button hierarchy. |
| `ui/templates/pages/exercise-swap/exercise-swap.gohtml` | Same rewrite; `Swap to this exercise` label. |
| `ui/templates/pages/exercise-info/exercise-info.gohtml` | Switch `.muscle-group`/`.primary` to `.muscle-chip`/`.muscle-chip--primary`; delete local rules. |
| `ui/templates/pages/styleguide/styleguide.gohtml` | Add muscle-chip + sheet-dialog entries. |
| `cmd/web/handler-styleguide_test.go` | Assert presence of the two new styleguide entries. |
| `ui/templates/CLAUDE.md` | Add `.muscle-chip` and `.sheet-dialog` to the "Current components" list under class-components. |

No Go source under `internal/` is touched. No handler or template data
shapes change.

## Testing

- `make test` — existing handler tests for `exercise-add`, `exercise-swap`,
  and `exercise-info` must keep passing. Selectors in those tests should
  be unaffected because the page-header / form actions / hidden inputs are
  unchanged.
- New styleguide assertions cover the two new component classes.
- Manual: open `/dev/styleguide` and the Add/Swap flows in a small-viewport
  browser; verify primary action dominates, Info dialog still opens and
  dismisses, chips wrap correctly on a narrow phone.
- `make lint-fix` to keep golangci-lint clean.

## Open questions

None.
