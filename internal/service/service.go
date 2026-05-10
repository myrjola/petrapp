// Package service holds workout orchestration: cross-aggregate coordination,
// external integrations (OpenAI, GDPR export), and the methods called by
// HTTP handlers. Pure rules live in internal/domain; persistence lives in
// internal/repository. This package depends on both.
package service

import (
	"log/slog"

	"github.com/myrjola/petrapp/internal/repository"
	"github.com/myrjola/petrapp/internal/sqlite"
)

// Service coordinates workout-domain operations across the repository
// layer and external integrations. One instance per process; safe for
// concurrent use because each method opens its own DB transaction.
type Service struct {
	repos        *repository.Repositories
	db           *sqlite.Database
	logger       *slog.Logger
	openaiAPIKey string
}

// NewService creates a new workout service.
func NewService(db *sqlite.Database, logger *slog.Logger, openaiAPIKey string) *Service {
	return &Service{
		repos:        repository.New(db, logger),
		db:           db,
		logger:       logger,
		openaiAPIKey: openaiAPIKey,
	}
}
