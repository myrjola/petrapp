# Enforce template hygiene with stdlib go tests, not a template language

Many of the frontend's most important rules are enforced only by prose and
review discipline: every page's view-model must match the fields the template
reads; every `{{template "x"}}` must resolve; every `<style>`/`<script>` must
carry its CSP nonce; `style="…"` attributes are forbidden (the CSP silently
drops them); and a literal that duplicates a design token is "a bug at any
count" ([ADR 0007](0007-inline-scoped-page-css.md), which closes by admitting
"nothing automated enforces it today"). These all share a failure mode: they
compile clean, lint clean, and only break at render time, in the browser, or
not visibly at all — the hardcoded `var(--font-serif)` chain renders
pixel-identical to the token, so no amount of running the app reveals it.

We add machine enforcement, and the decision that needed recording is *how* —
because the obvious answers are wrong for this codebase.

## Decision

Enforce template correctness and design-system discipline with **ordinary
`go test`s in `cmd/petra/`** plus **axe-core in the existing Playwright suite**.
No new language, no codegen, no non-Go toolchain. Three layers:

1. **Generic correctness** (`make test`):
   - A parse-all test parses every `pages/*` dir as `base + components + page`
     and asserts no error — catching syntax and undefined-function errors that
     today only surface when a page is first rendered (templates parse lazily
     per-page in `handlers.go`; there is no fail-fast at boot).
   - [`jba/templatecheck`](https://github.com/jba/templatecheck) type-checks each
     page against its view-model struct (`CheckHTML(tmpl.Lookup("base"),
     vm{})`). It catches field typos, type/arity mismatches, **and** unresolved
     `{{template "x"}}` targets — so no bespoke `text/template/parse` walker is
     needed. This also neutralises the gotype-comment-drift footgun: a renamed
     field fails the check instead of silently staling an IDE hint.
   - The page→view-model map is **fail-closed**: the test enumerates
     `ui/templates/pages/` and fails if any dir lacks a registry entry, so a new
     page cannot silently skip type-checking.

2. **Design-system / CSP discipline** — a text scan of the `.gohtml` files
   (not the template AST; these are HTML/CSS-text rules the AST does not model).
   `main.css` is exempt — it is the token *definition* source. The rules are
   **absolute** (no inline opt-out comment): no `style="…"` attributes; a nonce
   on every `<style>`/`<script>`; no `:hover`/`cursor:pointer`/`@media (hover)`/
   `mouseenter|mouseleave`; no `@font-face`/font-CDN; no token-duplicating
   literals (font stacks, raw hex/rgb) inside `<style>` blocks.

3. **Behavioural guarantees** — Playwright (`!testing.Short()`, runs in
   `make ci`): inject axe-core into the pages the smoke/stack-navigator suite
   already visits and assert zero WCAG AA violations, retiring the
   hand-maintained contrast matrix for those pages. CSP and Trusted-Types
   violations stay covered by the existing console-error-fails-the-test net.

## Alternatives rejected

- **`a-h/templ`** — gives true compile-time type-safety and an LSP, but it is a
  new language with a codegen step and its own non-`html/template` runtime. That
  contradicts the project's stated stance (Go stdlib focus, vanilla, **no build
  system for assets**, templates that hot-reload from disk). `templatecheck`
  recovers ~80% of the safety against typed view-models with one small MIT
  dependency and zero workflow change. Re-proposing `templ` should start here.
- **A golangci-lint plugin** — golangci-lint analyses Go AST/SSA and never parses
  `.gohtml`. A custom plugin would have to do the file-walking itself, gaining
  nothing over a `go test` while adding the brittle custom-binary/plugin
  machinery. The only payoff would be surfacing findings in the same IDE stream;
  not worth it.
- **`vnu` / `djLint`** for HTML-validity and markup hygiene — pull in a
  Java/Python toolchain for the sake of CI, against the small-footprint ethos.
  axe-core (injected into the Playwright run we already pay for) covers the
  accessibility slice that actually matters here without a new runtime.

## Consequences

- The token-literal rule's first run fails on the pre-existing
  `pages/home/unauthenticated.gohtml` font-chain literal; fixing it to
  `var(--font-serif)` lands with the rule.
- The promotion rule's token-duplication clause in [ADR 0007](0007-inline-scoped-page-css.md)
  is now machine-enforced, not a review checklist. The ≥3-template *count*
  threshold remains a human judgement.
- "Absolute, no escape hatch" means a future legitimate exception requires
  editing the checker — deliberate friction that forces the discussion into a
  diff rather than a silent inline comment.
- Coverage is split by construction: the static layers cover all pages
  unconditionally and point at file:line; axe-core covers only the pages the
  Playwright flows visit. Expanding behavioural coverage means visiting more
  pages, not changing the static layer.
- New dependency: `github.com/jba/templatecheck` (used in tests only). axe-core
  is vendored as a dev-only test asset, never shipped to clients.
</content>
</invoke>
