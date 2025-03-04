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
