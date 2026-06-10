# Repository — agent notes

Package reference and schema guidance: [README.md](README.md) — read its
"Schema & migrations" section before any `schema.sql` change.

- `fixtures.sql` re-applies on **every boot** and must coexist with rows prod
  already holds — design seeds upsert-safe.
- Premigrations must be idempotent and short-circuit on both fresh and
  already-migrated databases; delete them (call site, test, fixture) once prod
  has booted past them.
- Translate `sql.ErrNoRows` → `domain.ErrNotFound` at the boundary; callers
  never see SQL errors.
