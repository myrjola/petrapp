# Docs

Repo-wide documentation. Layer- and app-specific reference lives next to the
code it documents (`cmd/petra/README.md`, `internal/petra/*/README.md`,
`internal/platform/sqlitekit/README.md`, `cmd/petra/ui/design-system.md`) —
see the docs index in the [root README](../README.md).

## What lives here

| Path | Contents |
|---|---|
| [`operations.md`](operations.md) | Running, inspecting, profiling, and deploying the live Fly instances |
| [`disaster-recovery.md`](disaster-recovery.md) | The DR runbook: failure-scenario catalog and rebuild-from-nothing procedure |
| [`adr/`](adr/) | Architecture decision records — durable "why is it this way" answers |
| [`ops-log/`](ops-log/) | Dated records of manual prod surgery (see conventions below) |
| [`plans/`](plans/) | In-flight design/implementation plans only (see lifecycle below) |

## Conventions

### Ops log

Recovery SQL, one-shot prod migrations, and post-incident write-ups go in
`ops-log/` named `YYYY-MM-DD-<slug>.{md,sql}`. Keep them checked in — they are
the only durable record of manual prod surgery: production data still carries
the shape these scripts created, and nothing else explains it. The
`make migratetest` flow runs deploy migrations only, never scripts from this
folder.

### Plan lifecycle

A design spec or implementation plan lives in `plans/` only while the work is
in flight. **When the work ships, delete the plan in the same change** —
the merged code, tests, and git history supersede it, and stale plans pollute
search results. Durable rationale that outlives the work goes to an ADR in
`adr/` or into the relevant layer README, not into a kept plan.

### ADRs

One decision per file, sequentially numbered (`NNNN-slug.md`). Write one only
when the decision is hard to reverse, surprising without context, *and* the
result of a real trade-off. A paragraph is enough.
