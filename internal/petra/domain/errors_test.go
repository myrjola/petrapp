package domain_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/myrjola/petrapp/internal/petra/domain"
)

func TestValidationError(t *testing.T) {
	t.Parallel()

	err := error(domain.ValidationError{Message: "name is required"})

	if err.Error() != "name is required" {
		t.Errorf("Error() = %q, want %q", err.Error(), "name is required")
	}

	var ve domain.ValidationError
	if !errors.As(err, &ve) {
		t.Fatal("errors.As failed to match ValidationError")
	}
	if ve.Message != "name is required" {
		t.Errorf("Message = %q, want %q", ve.Message, "name is required")
	}

	wrapped := fmt.Errorf("create exercise: %w", err)
	if !errors.As(wrapped, &ve) {
		t.Fatal("errors.As failed to match wrapped ValidationError")
	}
}
