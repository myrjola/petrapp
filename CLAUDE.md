# PetrApp Commands and Guidelines

## Build & Run Commands
```
make build        # Build binary and tools
make dev          # Run development server
make lint         # Run golangci-lint checks
make test         # Run all tests with race detection
make ci           # Run init, build, lint, test, sec
```

## Testing
- Run single test: `go test -v ./path/to/package -run TestName`
- Table-driven tests with clear assertions

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

## Following conventions

When making changes to files, first understand the file's code conventions. Mimic code style, use existing libraries and utilities, and follow existing patterns.
- NEVER assume that a given library is available, even if it is well known. Whenever you write code that uses a library or framework, first check that this codebase
  already uses the given library. For example, you might look at neighboring files, or check the package.json (or cargo.toml, and so on depending on the language).
- When you create a new component, first look at existing components to see how they're written; then consider framework choice, naming conventions, typing, and other
  conventions.
- When you edit a piece of code, first look at the code's surrounding context (especially its imports) to understand the code's choice of frameworks and libraries. Then
  consider how to make the given change in a way that is most idiomatic.
- Always follow security best practices. Never introduce code that exposes or logs secrets and keys. Never commit secrets or keys to the repository.

## CSS Architecture

- When working with CSS, first examine existing templates to understand the scoping strategy
- Use @scope at-rules for page-specific component styles rather than global CSS classes
- Only add styles to main.css if they are truly global (design tokens, utilities, base components)
- Page-specific styles should be scoped within their respective template files

## Template Functions and Data Preparation

- Check cmd/web/handlers.go for available custom template functions (like nonce, csrf, mdToHTML)
- Built-in Go template functions (add, sub, mul, div) may not always be available - verify before use
- STRONGLY PREFER preparing data in Go code rather than adding complexity to templates
- When templates need computed values (like human-readable indices), transform the data in the handler before passing to template
- Use the Task tool to search for existing template function usage patterns
- If a template function is missing, modify the backend data structure instead of adding new template functions

## Design System Architecture

- Examine the existing CSS layer structure (@layer reset, props, layout, components) before making changes
- Global components in main.css should be truly reusable across multiple pages (buttons, forms, utilities)
- Page-specific styling should use scoped @scope at-rules within the template file
- When improving UI/UX, maintain the existing design token system and CSS custom properties
- Prefer enhancing existing design tokens over creating new CSS variables

## Error Recovery and Data-First Approach

- When template rendering fails, prioritize fixing data preparation over modifying templates
- If missing template functions cause errors, transform data in Go handlers instead of registering new functions
- When encountering "function not defined" errors, check if the logic belongs in the backend data layer
- For display formatting (like converting 0-based to 1-based indexing), modify the data structure before template rendering
- Prefer simple, logic-free templates over complex template expressions

important-instruction-reminders

Do what has been asked; nothing more, nothing less.
NEVER create files unless they're absolutely necessary for achieving your goal.
ALWAYS prefer editing an existing file to creating a new one.
NEVER proactively create documentation files (*.md) or README files. Only create documentation files if explicitly requested by the User.

## Debugging Test Failures

- When tests fail with generic errors (like "unexpected status code: 500"), temporarily replace io.Discard with os.Stdout in test setup to see detailed error output
- After identifying the issue, revert back to io.Discard to keep test output clean
- For template-related errors, check for missing functions, syntax errors, or data structure mismatches
- Use verbose test output (go test -v) to get more detailed failure information
