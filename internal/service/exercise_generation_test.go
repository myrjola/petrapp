package service_test

import (
	"testing"

	"github.com/myrjola/petrapp/internal/domain"
)

// Test_GenerateExercise_FallsBackWithoutAPIKey asserts that when the service
// is built without an OpenAI API key (setupTestService passes ""), the
// minimal-exercise fallback path runs and the result is persisted with a
// real ID. This guards the "no AI, still works" promise the GenerateExercise
// doc comment makes.
func Test_GenerateExercise_FallsBackWithoutAPIKey(t *testing.T) {
	ctx, svc := setupTestService(t)

	got, err := svc.GenerateExercise(ctx, "Cossack Squat")
	if err != nil {
		t.Fatalf("GenerateExercise: %v", err)
	}

	if got.ID <= 0 {
		t.Errorf("ID = %d, want a positive persisted ID", got.ID)
	}
	if got.Name != "Cossack Squat" {
		t.Errorf("Name = %q, want %q", got.Name, "Cossack Squat")
	}
	if got.Category != domain.CategoryFullBody {
		t.Errorf("Category = %q, want %q (minimal fallback default)",
			got.Category, domain.CategoryFullBody)
	}
	if got.ExerciseType != domain.ExerciseTypeWeighted {
		t.Errorf("ExerciseType = %q, want %q (minimal fallback default)",
			got.ExerciseType, domain.ExerciseTypeWeighted)
	}

	// Confirm the row actually round-trips through the repository.
	round, err := svc.GetExercise(ctx, got.ID)
	if err != nil {
		t.Fatalf("GetExercise after fallback create: %v", err)
	}
	if round.Name != "Cossack Squat" {
		t.Errorf("round-trip Name = %q, want %q", round.Name, "Cossack Squat")
	}
}
