package service

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/myrjola/petrapp/internal/contexthelpers"
)

// ExportUserData creates an SQLite database export containing all data for the authenticated user.
// This method is intended for GDPR compliance and allows users to download their complete data.
func (s *Service) ExportUserData(ctx context.Context) (string, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	if userID == 0 {
		return "", errors.New("no authenticated user found in context")
	}

	tempDir := os.TempDir()

	exportPath, err := s.db.CreateUserDB(ctx, userID, tempDir)
	if err != nil {
		return "", fmt.Errorf("create user database: %w", err)
	}

	return exportPath, nil
}
