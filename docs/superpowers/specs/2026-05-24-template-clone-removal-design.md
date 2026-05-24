# Eliminate per-render template clone

## Problem

`(*application).pageTemplate` (`cmd/web/handlers.go:78`) clones the cached
parsed template on every render. The clone exists only so the per-request
`(*Template).Funcs(...)` call in `renderToBuf` can rebind `nonce` and
`mdToHTML` without racing concurrent `Execute` calls — those two functions
are the only ones in `contextTemplateFuncs` that depend on request state.

The 2026-05-24 staging stresstest (20 users, 2 m, ~200 req/s) showed the
cost. CPU profile (`pprof/cpu-20260524T163824Z.pb.gz`):

- `app.render` pipeline — 28 % cum across `home` + `preferencesGET`.
- `html/template.escape*` — 5–9 % CPU per handler. The escaper walks every
  action node on first execute; `Clone()` resets that state, so the walk
  reruns on every request instead of once per page.

Heap profile (`pprof/heap-20260524T164026Z.pb.gz`):

- 3.80 GB allocated to `bytes.growSlice` over 2 m (~32 MB/s).
- 1.64 GB to `text/template/parse.Tree.Copy`, 0.89 GB to `TextNode.Copy`,
  0.27 GB to `html/template.editActionNode` — all driven by `Clone()`.

p99 on `GET /`, `GET /preferences`, and `POST /preferences` reached
700–900 ms with max 2–3 s, enough to trip the slow-request flight
recorder (`pprof/traces/slow-20260524-163959.trace`). The trace itself
isn't load-bearing for the diagnosis — the heap profile is the smoking
gun — but it confirms the spikes were real, not measurement noise.

## Goal

Make the parsed template tree concurrency-safe so `pageTemplate` returns
the cached `*template.Template` directly. The escape state is computed
once per page name, ever; no per-request copies of the parse tree.

The expected effect is a single-digit-percent CPU drop and a large
reduction in GC pressure, which should flatten p99 on the affected
handlers from 700–900 ms toward the p95 baseline (~150 ms).

## Non-goals

- No embed.FS migration; templates continue to load via `os.DirFS` at
  runtime (the dev-loop affordance documented in the top-level
  `CLAUDE.md`).
- No rework of the dev/prod cache split. Dev mode keeps re-parsing on
  every call so template edits show on refresh.
- No feature flag. The behaviour change is internal and covered by
  existing handler e2e tests.
- No markdown-rendering rework beyond moving the call out of the render
  hot path.

## Design

### Per-request state moves onto the data struct

`nonce` and `mdToHTML` are the only request-scoped template funcs.
Eliminate the rebind by passing the value through data instead of through
`Funcs`.

#### `BaseTemplateData` carries the nonce

`cmd/web/templates.go`:

```go
type BaseTemplateData struct {
    Authenticated     bool
    IsAdmin           bool
    InvalidationToken string
    Nonce             template.HTMLAttr
}

func newBaseTemplateData(r *http.Request) BaseTemplateData {
    // …existing fields…
    return BaseTemplateData{
        // …
        Nonce: template.HTMLAttr(
            fmt.Sprintf("nonce=%q", contexthelpers.CSPNonce(r.Context())),
        ),
    }
}
```

The string format (`nonce="<value>"`) exactly mirrors what
`contextTemplateFuncs.nonce` returned. Templates change every
`{{ nonce }}` to `{{ .Nonce }}`. Every page-data struct already embeds
`BaseTemplateData`, so the field promotes through.

#### Markdown is pre-rendered in the handler

`renderMarkdownToHTML` loses its `*application` receiver and becomes a
package-level helper:

```go
func markdownToHTML(ctx context.Context, logger *slog.Logger, md string) template.HTML
```

It keeps the existing log-on-error branch verbatim (goldmark's `Convert`
into a `bytes.Buffer` does not currently error in practice, but the
defensive log is cheap and matches today's behaviour).

The two consumers are the only places markdown reaches a template:

- `cmd/web/handler-exercise-info.go` — `exerciseInfoTemplateData` gets a
  `DescriptionHTML template.HTML` sibling field; populated in
  `exerciseInfoGET` before `app.render`.
- `cmd/web/components.go` — `ExerciseResultCardData` gets the same field.
  Both build sites in `cmd/web/handler-workout.go` (lines 342, 467)
  populate it.

Templates change:

- `ui/templates/pages/exercise-info/exercise-info.gohtml:365`:
  `{{ mdToHTML .Exercise.DescriptionMarkdown }}` → `{{ .DescriptionHTML }}`
- `ui/templates/components/exercise-result-card.gohtml:29`: same change.

### `pageTemplate` returns the cached template directly

`cmd/web/handlers.go`:

```go
func (app *application) pageTemplate(pageName string) (*template.Template, error) {
    if app.devMode {
        return app.parsePageTemplate(pageName)
    }
    if cached := app.parsedTemplates.get(pageName); cached != nil {
        return cached, nil
    }
    parsed, err := app.parsePageTemplate(pageName)
    if err != nil {
        return nil, err
    }
    app.parsedTemplates.set(pageName, parsed)
    return parsed, nil
}
```

No `Clone()`, no per-request `Funcs(...)`. `renderToBuf` drops the
`t.Funcs(app.contextTemplateFuncs(ctx))` call.

`contextTemplateFuncs` is deleted. `baseTemplateFuncs` is renamed to
`templateFuncs` and keeps only the stateless `formatFloat` and `sub`. It
is bound once at parse time inside `parsePageTemplate`, exactly as today.

The `templateCache` keeps its `sync.RWMutex` — two requests racing to
populate the same cache slot can both call `parsePageTemplate`; the
second one's `set` overwrites the first. That's harmless because the
parsed trees are observationally identical, and after one of them wins
every subsequent request reads the same `*template.Template`.

### Why this is safe for concurrent Execute

The stdlib documents `(*Template).Execute` as safe for concurrent use as
long as the template is not being modified. After
`parsePageTemplate`, nothing mutates the tree:

- No `Funcs` call ever runs on the cached template again. The cache
  setter writes the tree once.
- `html/template`'s lazy escaper mutates the tree on first `Execute`.
  Subsequent executes see escaped action nodes and skip the walk. The
  first-execute mutation is protected by `html/template`'s own internal
  `sync.Mutex` on the namespace, so two goroutines racing on a
  not-yet-escaped template will serialise on it; only one walks. (See
  `escapeTemplate` in `src/html/template/escape.go` upstream.)

Net: at most one escape walk per page name across the lifetime of the
process.

## Testing

### Replace the existing cache test

`cmd/web/handlers_test.go:Test_pageTemplate_cachesAndReturnsCloneInProdMode`
locks in today's clone behaviour. Rewrite as
`Test_pageTemplate_cachesAndReturnsSameInstanceInProdMode`: first call
parses and caches; second call returns the same pointer; concurrent calls
return the same pointer and do not race (`go test -race`).

`Test_pageTemplate_skipsCacheInDevMode` is unchanged.

### Render benchmark

Add `cmd/web/handlers_bench_test.go` with `BenchmarkRenderHome` that
constructs an `application` with the prod-mode cache populated, then
renders the `home` template into a discard writer in a tight loop. Used
as the local before/after measurement and as a regression guard in CI
runs of `go test -bench=.`.

### End-to-end coverage

Existing handler tests in `cmd/web/*_test.go` already render real HTML
through `e2etest`. A missing-nonce or broken-markdown-pipeline regression
fails them immediately (`function "nonce" not defined` at execute time).
A post-edit grep is the belt-and-suspenders check:

```sh
grep -rn '{{ *nonce\b\|mdToHTML\|contextTemplateFuncs' cmd/web ui/templates
# expected: zero matches
```

### Re-run the stresstest post-merge

After the PR lands on staging via the standard CD path
(`docs/2026-05-02-…` style is not required for this; the existing
`make fly-*` flow is enough), re-run:

```sh
./bin/stresstest --users 20 --duration 2m \
  --pprof-url http://localhost:6060 --out pprof \
  petra-staging.fly.dev
```

against `petra-staging.fly.dev` and compare the latency table and the
heap profile against the 2026-05-24 baseline. Expected: p99 on `/`,
`/preferences`, `POST /preferences` drops from ~800 ms to ≤200 ms;
`bytes.growSlice` allocations collapse; `escapeTree` falls out of the
top-30 CPU consumers.

## Risk surface

- **Forgotten `{{ nonce }}`**: surfaces as `function "nonce" not defined`
  at execute time. Caught by both the e2e tests and the post-edit grep.
- **Component dot not carrying nonce**: the four current Go partials
  (`field`, `banner`, `page-header`, `back-link`) are reviewed during
  implementation. Any that emit `<style {{ nonce }}>` get a `Nonce`
  field on their dot struct, populated by callers from their embedded
  `BaseTemplateData.Nonce`. (Current state: a quick read suggests none
  of the four emit their own nonce'd block, but the implementation step
  verifies.)
- **`formatFloat` / `sub` behaviour change**: today's per-request
  `Funcs` override replaces the parse-time stubs with identical
  implementations. Removing the override is a no-op for them.
- **Dev-mode parity**: dev mode still re-parses on every call, so a
  template edit referencing `.Nonce` is reflected immediately.

## Docs to update

- `cmd/web/CLAUDE.md` — "BaseTemplateData" section: drop the paragraph
  saying nonce does not travel on the struct; add a sentence saying it
  does.
- `ui/templates/CLAUDE.md` — "Available Template Functions": remove
  `{{ nonce }}` and `{{ mdToHTML }}` entries. Replace with: nonce is on
  `BaseTemplateData` as `Nonce` and used as `{{ .Nonce }}`; markdown is
  pre-rendered in handlers and exposed as `template.HTML` data fields.
  Update the colocated-style example and the script example to use
  `{{ .Nonce }}`. Update the "Styling Components" line that mentions
  `<style {{ nonce }}>` to use `<style {{ .Nonce }}>`.
- `cmd/web/handlers.go` — `pageTemplate` docstring: drop the "clone per
  render" rationale; state the new invariant (cached template is
  immutable after first execute, returned directly).

## Rollout

Standard CD path: PR → review app for manual smoke → merge to `main` →
staging → prod. No flag, no migration step. Roll back by reverting the
commit.

## Out of scope (followups, if useful)

- Switching template loading to `embed.FS` for prod (the runtime-load
  affordance was deliberate; revisit only if a separate motivation
  appears).
- Moving `formatFloat` / `sub` from FuncMap entries to methods on data
  structs for symmetry with the new pattern.
- Per-handler timing histograms to feed a future SLO dashboard.
