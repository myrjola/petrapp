# PetrApp - Go Web Application

A fitness tracking web application built with Go, SQLite, and server-side rendered templates.

## Development Workflow

When a feature spans multiple layers, work outwards from the data model:

1. **Database First** - Start with schema changes in `internal/sqlite/` (
   see [Database Guidelines](internal/sqlite/CLAUDE.md))
2. **Domain Models** - Update business logic in `internal/workout/` (
   see [Domain Guidelines](internal/workout/CLAUDE.md))
3. **HTTP Layer** - Add handlers and routing in `cmd/web/` (see [Web Guidelines](cmd/web/CLAUDE.md))
4. **Templates & UI** - Build frontend in `ui/templates/` (see [Template Guidelines](ui/templates/CLAUDE.md))

If a change is scoped to one or two layers (e.g. a UI-only tweak or a handler-only bug fix), start at the lowest relevant layer — you don't have to touch the ones above.

## Runtime Layout

- Templates (`ui/templates/`) and static assets (`ui/static/`) are loaded from the filesystem at runtime (`os.DirFS`, not `//go:embed`). Editing a template and refreshing the browser is the whole dev loop — no rebuild needed.
- In the Docker image, `main.css` and `main.js` are fingerprinted with an md5 hash at build time and `base.gohtml` is rewritten to reference the hashed names (see `Dockerfile`). Other static files (`webauthn.js`, icons) are served under their original names.

## One-shot scripts and post-mortems

Recovery SQL, one-shot prod migrations, and post-incident write-ups go in `docs/` named
`YYYY-MM-DD-<slug>.{md,sql}`. Keep them checked in — they're the only durable record of
manual prod surgery, and the `make migratetest` flow runs them only via the deploy migration
path, not from `docs/`. Examples: `docs/2026-05-02-code-review-cleanup.md`,
`docs/2026-05-02-recover-incline-dumbbell-bench-press.sql`.

## Build & Run Commands

```bash
make init         # Initializes the development environment - run once after cloning
make lint-fix     # Run golangci-lint checks with automatic fixing enabled - use before committing changes
make test         # Run all tests - use after functionality changes
make ci           # Run init, build, lint, test, sec - use for comprehensive verification
```

**When to use specific commands:**

- Use `make build` after adding new files or significant code changes
- Use `make test` after implementing new features or modifying existing functionality
- Use `make lint` before committing to catch style and complexity issues
- Use `make ci` for complex changes requiring full validation (database changes, major refactoring)
- Always run `make ci` when making significant architectural changes

## Testing

- Run single test: `go test -v ./path/to/package -run TestName`
- Table-driven tests with clear assertions
- Run tests after UI changes to catch compatibility issues early

### Maintaining Test Compatibility

When making UI changes:

- Consider impact on existing tests that rely on DOM structure
- Use specific selectors instead of generic ones (e.g., `.Find("form").FilterFunction()` instead of
  `.Find("form").First()`)
- Look for unique identifiers like button text, form actions, or data attributes for reliable test selectors
- When tests break due to DOM changes, update selectors to be more specific and resilient

## Code Style

- Standard Go formatting with 100-line function limit
- Error types must be suffixed with "Error", sentinel errors with "Err" prefix
- Strongly typed with exhaustive enum checking
- No global loggers or init functions
- Comments must end with a period

## Security Guidelines

- HTTP handlers must use context
- CSP and CSRF protection enforced
- All SQL queries parameterized
- Never introduce code that exposes or logs secrets and keys
- Never commit secrets or keys to the repository

## Following Conventions

When making changes to files, first understand the file's code conventions:

- NEVER assume that a given library is available - check neighboring files or `go.mod` first
- When creating new components, examine existing ones for patterns and conventions
- Always look at the surrounding context (especially imports) to understand framework choices
- Make changes in the most idiomatic way for the existing codebase

## Fixing linter complaints

- govet shadow is often fixed by reusing the earlier `err` variable instead of using different variable name.

## Debugging Test Failures

- For template-related errors, check for missing functions, syntax errors, or data structure mismatches
- When tests fail after UI changes, update DOM selectors to be more specific and resilient
