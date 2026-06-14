# Inline scoped page CSS; promote shared primitives at the rule of three

Shared, reusable styles live in the fingerprinted `cmd/petra/ui/static/main.css`
(design tokens + component classes). Page-specific styles live *inline* in the
page's `.gohtml`, in a nonce'd `<style>` block scoped with `@scope`. We chose
this for developer happiness on a no-CSS-build frontend: styles sit next to the
markup they style, hot-reload from disk, need no naming ceremony, and require no
extraction step. `@scope` + `@layer` give cascade isolation without global
pollution.

The cost is real and measured, not negligible: because each `<style>` carries a
per-request CSP nonce, inline CSS is never cacheable and is re-shipped on every
full-page navigation. On the hottest path — logging a set, a full-page `POST` —
the document re-ships ~22.8 KB of inline CSS, **~2.9 KB gzipped**, re-parsed in
well under a millisecond. A 25-set workout re-transfers ~72 KB gzipped of CSS a
cached file would have sent once. We accept that in exchange for the DX and the
absence of a CSS-extraction build. Revisit if the traffic shape changes (e.g. a
much larger page CSS payload, or a hot path that re-renders far more often).

The failure mode of "page-specific → inline" is that it gives no signal for when
a pattern has stopped being page-specific. Brand primitives then rot inline,
copy-pasted across templates — the exact copy-paste tax locality-of-behavior is
meant to avoid. When this ADR was written, the overline and gradient-divider
treatments were duplicated inline across 5 templates each, and the serif display
title's font stack was hardcoded in 6 templates instead of using the existing
`--font-serif` token — two sources of truth for the most important brand
decision in the design language.

## Promotion rule

- A style pattern reused in **≥3 templates** graduates to a real class/utility
  in `main.css`. (Overline, display-title, and the divider rule are due now.)
- **Any** inline literal that duplicates an existing token is a bug, regardless
  of count: use the token. The Charter stack must become `var(--font-serif)`
  everywhere it appears.
- Inline `<style>` is reserved for genuinely page-unique layout and for dynamic
  values that must be emitted from the template (e.g.
  `width: {{ .ProgressPercent }}%`), which cannot be hoisted.

## Amendment (2026-06-14): the rule covers JavaScript too

The same failure mode applies to inline scripts, so the promotion rule is not
CSS-only. `templates/README.md` ("JavaScript in Templates") is right that
**inline-first stands for page logic** — page-specific DOM wiring, `@scope`d
behaviour, and template-context scripts stay inline, and length alone is never a
promotion trigger. But a **device/capability helper used by ≥2 surfaces lives in
`main.js` (or an importmap module), not copied inline** — a primitive duplicated
across surfaces is a bug regardless of medium. Duplication is the trigger, not
size.

This was written while the screen wake-lock and `AudioContext`-unlock helpers
were re-implemented inline in the rest-timer (`sets-container.gohtml`) despite
already living in `main.js`, and the WebAuthn register/login bindings were two
near-identical inline scripts in `unauthenticated.gohtml`. Promoting those —
shared helpers to `main.js`, the auth binding to the existing `webauthn`
importmap module — is the standing follow-up this clause governs. Any new module
must respect the CSP / Trusted-Types rules in the README (build DOM with
nodes/`textContent`; reach script URLs only through the sanctioned importmap /
`sw-loader` policy, never a script-URL sink).

## Consequences

- Inline page CSS stays; we do not extract per-page stylesheets or a second
  bundle. Only `main.css` is content-fingerprinted (at startup, via the asset
  manifest in `cmd/petra/assets.go`) and cached immutably; the inline page CSS
  rides uncached inside each HTML document, which is the cost recorded above.
- The promotion rule is a review-time judgement; nothing automated enforces it
  today, so the ≥3 / token-duplication thresholds are the checklist.
- Brand primitives belong in `main.css` even when they first appear on one page;
  "first used here" is not "specific to here".
