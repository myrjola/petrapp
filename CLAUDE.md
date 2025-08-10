# PetrApp - Go Web Application

A fitness tracking web application built with Go, SQLite, and server-side rendered templates.

## Development Workflow

When implementing features, follow this architectural flow:

1. **Database First** - Start with schema changes in `internal/sqlite/` (see [Database Guidelines](internal/sqlite/CLAUDE.md))
2. **Domain Models** - Update business logic in `internal/workout/` (see [Domain Guidelines](internal/workout/CLAUDE.md))  
3. **HTTP Layer** - Add handlers and routing in `cmd/web/` (see [Web Guidelines](cmd/web/CLAUDE.md))
4. **Templates & UI** - Build frontend in `ui/templates/` (see [Template Guidelines](ui/templates/CLAUDE.md))

## Build & Run Commands

```bash
make build        # Build binary and tools - use after significant code changes
make lint         # Run golangci-lint checks - use before committing changes  
make test         # Run all tests with race detection - use after functionality changes
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
- Use specific selectors instead of generic ones (e.g., `.Find("form").FilterFunction()` instead of `.Find("form").First()`)
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
- NEVER assume that a given library is available - check neighboring files or go.mod first
- When creating new components, examine existing ones for patterns and conventions
- Always look at surrounding context (especially imports) to understand framework choices
- Make changes in the most idiomatic way for the existing codebase

## Specialized Guidelines

For detailed guidance on specific areas:

- **[Database Schema](internal/sqlite/CLAUDE.md)** - SQLite schema evolution and migration patterns
- **[Domain Models](internal/workout/CLAUDE.md)** - Business logic, models, and service layer patterns  
- **[Web Handlers](cmd/web/CLAUDE.md)** - HTTP handlers, routing, and middleware patterns
- **[Templates & UI](ui/templates/CLAUDE.md)** - Go templates, CSS architecture, and design system

## Debugging Test Failures

- For template-related errors, check for missing functions, syntax errors, or data structure mismatches
- When tests fail after UI changes, update DOM selectors to be more specific and resilient