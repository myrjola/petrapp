# Development Guidelines for PetrApp

This document provides specific information for developers working on the PetrApp project.

## Build/Configuration Instructions

### Prerequisites

- Go
- Make

### Setup and Build

```bash
make ci
```

This builds the application and runs all tests, linters, and security checks.

### Environment Configuration

The application uses environment variables for configuration but the defaults should work out of the box.

## Testing Information

### Running Tests

1. **Run all tests**:
   ```bash
   make test
   ```
   This runs all tests with race detection and test shuffling enabled.

2. **Run specific tests**:
   ```bash
   go test ./internal/package/...
   ```
   Replace `package` with the specific package you want to test.

3. **Run tests with verbose output**:
   ```bash
   go test -v ./...
   ```

### Writing Tests

1. **Test file naming**: Test files should be named with a `_test.go` suffix and placed in the same package as the code
   being tested.

2. **Test function naming**: Test functions should be named `Test_FunctionName` or `TestStructName_MethodName`.

3. **Table-driven tests**: Use table-driven tests for testing multiple cases with the same logic.

4. **In-memory database**: For database tests, use an in-memory SQLite database:
   ```go
   db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
   ```

5. **Helper functions**: Use t.Helper() for helper functions to improve test output:
   ```go
   func helperFunction(t *testing.T) {
       t.Helper()
       // Helper code
   }
   ```
6. **No assertion libraries**: Don't use testify or other assertion libraries. Our own helpers and go-cmp for struct
   comparison are enough.
7. **Prefer end-to-end tests**: Use the internal/e2etest package to create tests for the handlers. Feel free to add more
   helpers to the package as needed.:w

### Example Test

Here's a simple example test for the `ptr.Ref` function:

```go
package ptr_test

import (
	"testing"

	"github.com/myrjola/petrapp/internal/ptr"
)

func TestRef(t *testing.T) {
	// Test with string
	t.Run("string", func(t *testing.T) {
		s := "test"
		p := ptr.Ref(s)
		
		if p == nil {
			t.Fatal("Expected pointer to be non-nil")
		}
		
		if *p != s {
			t.Errorf("Expected %q, got %q", s, *p)
		}
	})
}
```

## Additional Development Information

### Project Structure

- `cmd/`: Contains the main application entry points
    - `cmd/web/`: Main web application
    - `cmd/migratetest/`: Database migration test utility
    - `cmd/smoketest/`: Smoke test utility
- `internal/`: Internal packages not meant for external use
    - `internal/workout/`: Workout-related functionality
    - `internal/sqlite/`: Database access and migration
    - `internal/contexthelpers/`: Context-related utilities
    - `internal/webauthnhandler/`: WebAuthn authentication
    - `internal/e2etest/`: End-to-end testing utilities
- `ui/`: User interface files
    - `ui/templates/`: HTML templates (using Go's html/template)
    - `ui/static/`: Static assets (CSS, JS, images)

### Database

- SQLite is used as the database
- Schema is defined in `internal/sqlite/schema.sql`
- Fixtures are in `internal/sqlite/fixtures.sql`
- Database migrations are handled by the `sqlite` package

### Authentication

The application uses WebAuthn for passwordless authentication:

- WebAuthn handlers are in `internal/webauthnhandler/`
- Client-side WebAuthn code is in `ui/static/webauthn.js`

### Code Style

- The project uses golangci-lint for code quality checks
- Run `make lint` to check for style issues
- Git hooks are set up to run linting before commits

### Debugging

- The application includes a pprof server for debugging
- Access pprof at the address specified by `PETRAPP_PPROF_ADDR` (default: ":6060")
- For database debugging, you can connect to the SQLite database directly:
  ```bash
  make fly-sqlite3  # For the deployed database
  sqlite3 petrapp.sqlite3  # For the local database
  ```

### Backend

- Go with standard library focus and minimal dependencies
    - Notable golangci-lint rules :
        - govet shadow: declaration of "err" shadows declaration at line 392
        - godot: check if comments end in a period.
- SQLite database for simple, self-contained data storage
    - Since the data is local, n+1 query problem is not a concern. Keep the SQL queries simple.

### Frontend

- No React or other frontend frameworks
- Go templates for HTML generation (template/html standard library package)
- Vanilla JavaScript without a build system
- Minimalist styling using minimal CSS while maintaining accessibility and usability
- Scoped CSS for style isolation
- Mobile-only interface
- Progressive enhancement for JavaScript features, i.e., app will work even when JavaScript is disabled.
    - Vanilla HTML forms and anchor links for core functionality

