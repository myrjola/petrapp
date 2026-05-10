// Package workout is a backward-compat shim for cmd/web through Phase 4
// of the workout-service rearchitecture. The Service type and the
// NewService constructor live in internal/service; this package
// re-exports them so handlers can keep importing "workout" without
// edits. The type aliases in models.go cover the rest of the public
// surface (domain types, sentinel errors, helper functions).
package workout

import (
	"log/slog"

	"github.com/myrjola/petrapp/internal/service"
	"github.com/myrjola/petrapp/internal/sqlite"
)

// Service is the workout orchestration entry point. The implementation
// lives in internal/service; this alias exists so that
// cmd/web/main.go's `workout.Service` field type keeps resolving. Phase
// 4 will rename the field to reference internal/service directly and
// delete this package.
type Service = service.Service

// NewService creates a new workout service. It delegates to
// service.NewService.
func NewService(db *sqlite.Database, logger *slog.Logger, openaiAPIKey string) *Service {
	return service.NewService(db, logger, openaiAPIKey)
}
