# sqlitekit — agent notes

Engine reference: [README.md](README.md).

- Product-agnostic: no concrete schema or product seams here — callers pass
  everything via `Config`. Product schema guidance lives in
  `internal/petra/repository/README.md`.
- The migrator is purely structural; data-shape transforms belong in
  product-owned premigrations, never in this package.
