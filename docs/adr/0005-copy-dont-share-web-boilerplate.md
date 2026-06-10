# Copy, don't share, the web boilerplate across apps

`cmd/example` deliberately copies Petra's web middleware/render/flash/fileserver
boilerplate instead of importing a shared web kit. When `internal/platform/` was
carved out (2026-06), only `sqlitekit` and `auth` were genuinely decoupled and
shared — they are expensive to reimplement and dangerous to let diverge. The web
layer was duplicated on the rule-of-three / "a little copying beats a little
dependency" principle: designing a `web.Kit` against one real app plus a toy
todo would encode the wrong joints. The diff between two *real* apps reveals the
true seam.

## Consequences

- Drift between the copies is expected and acceptable until a third consumer
  appears; do not "fix" the duplication by extracting prematurely.
- `internal/platform/` must stay product-agnostic — enforced by depguard rules
  in `.golangci.yml` (platform cannot import `internal/petra` or `cmd/`).
- Revisit when a second real app (not `cmd/example`) needs the web layer.
