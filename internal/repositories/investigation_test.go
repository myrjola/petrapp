package repositories_test

import (
	"context"
	"github.com/myrjola/petrapp/internal/models"
	"github.com/myrjola/petrapp/internal/repositories"
	"github.com/myrjola/petrapp/internal/testhelpers"
	"github.com/stretchr/testify/require"
	"io"
	"os"
	"testing"
)

func TestInvestigationRepository_Get(t *testing.T) {
	t.Parallel()
	logger := testhelpers.NewLogger(io.Discard)
	dbs := newTestDB(t, logger)
	repo := repositories.NewInvestigationRepository(dbs, logger)

	tests := []struct {
		name                  string
		investigationTargetID string
		userID                []byte
		wantInvestigation     models.Investigation
		wantErr               bool
	}{
		{
			name:                  "Without completions",
			investigationTargetID: "rue-morgue",
			userID:                []byte{1},
			wantInvestigation: models.Investigation{
				Target: models.InvestigationTarget{
					ID:        "rue-morgue",
					Name:      "Rue Morgue Murder Scene",
					ShortName: "Rue Morgue",
					Type:      models.InvestigationTargetTypeScene,
					ImagePath: "https://myrjola.twic.pics/petrapp/rue-morgue.webp",
				},
				Completions: nil,
			},
			wantErr: false,
		},
		{
			name:                  "With completions",
			investigationTargetID: "le-bon",
			userID:                []byte{1},
			wantInvestigation: models.Investigation{
				Target: models.InvestigationTarget{
					ID:        "le-bon",
					Name:      "Adolphe Le Bon",
					ShortName: "Adolphe",
					Type:      models.InvestigationTargetTypePerson,
					ImagePath: "https://myrjola.twic.pics/petrapp/adolphe_le-bon.webp",
				},
				Completions: []models.Completion{
					{
						ID:       1,
						Order:    0,
						Question: "What is your name?",
						Answer:   "Adolphe Le Bon",
					},
					{
						ID:       2,
						Order:    1,
						Question: "What is your occupation?",
						Answer:   "Bank clerc",
					},
					{
						ID:       3,
						Order:    2,
						Question: "What is your address?",
						Answer:   "Rue Morgue",
					},
				},
			},
			wantErr: false,
		},
		{
			name:                  "Invalid user name returns empty completions",
			investigationTargetID: "rue-morgue",
			userID:                []byte("nonexistent"),
			wantInvestigation: models.Investigation{
				Target: models.InvestigationTarget{
					ID:        "rue-morgue",
					Name:      "Rue Morgue Murder Scene",
					ShortName: "Rue Morgue",
					Type:      models.InvestigationTargetTypeScene,
					ImagePath: "https://myrjola.twic.pics/petrapp/rue-morgue.webp",
				},
				Completions: nil,
			},
			wantErr: false,
		},
		{
			name:                  "Invalid investigation target",
			investigationTargetID: "nonexistent",
			userID:                []byte{1},
			wantErr:               true,
			wantInvestigation:     models.Investigation{}, //nolint:exhaustruct // expected to be empty
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			investigation, err := repo.Get(context.TODO(), tt.investigationTargetID, tt.userID)

			if tt.wantErr {
				require.Error(t, err, "expected error")
				require.Nil(t, investigation, "investigation should be nil")
				return
			}

			require.NoError(t, err, "failed to read investigation")
			require.NotNilf(t, investigation, "investigation not found")
			require.Equal(t, tt.wantInvestigation.Target, investigation.Target, "investigation target mismatch")
			require.Equal(t, tt.wantInvestigation.Completions, investigation.Completions, "completions mismatch")
		})
	}
}
func TestInvestigationRepository_FinishCompletion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                  string
		investigationTargetID string
		userID                []byte
		previousCompletionID  int64
		wantErr               bool
	}{
		{
			name:                  "first completion",
			investigationTargetID: "rue-morgue",
			userID:                []byte{1},
			previousCompletionID:  -1,
			wantErr:               false,
		},
		{
			name:                  "second completion for user",
			investigationTargetID: "rue-morgue",
			userID:                []byte{2},
			previousCompletionID:  4,
			wantErr:               false,
		},
		{
			name:                  "not allowed to target invalid completion",
			investigationTargetID: "rue-morgue",
			userID:                []byte{1},
			previousCompletionID:  0,
			wantErr:               true,
		},
		{
			name:                  "fourth completion",
			investigationTargetID: "le-bon",
			userID:                []byte{1},
			previousCompletionID:  3,
			wantErr:               false,
		},
		{
			name:                  "has to target last completion in investigation",
			investigationTargetID: "le-bon",
			userID:                []byte{1},
			previousCompletionID:  2,
			wantErr:               true,
		},
		{
			name:                  "invalid investigation target",
			investigationTargetID: "nonexistent",
			userID:                []byte{1},
			previousCompletionID:  -1,
			wantErr:               true,
		},
		{
			name:                  "invalid user",
			investigationTargetID: "le-bon",
			userID:                []byte("nonexistent"),
			previousCompletionID:  -1,
			wantErr:               true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			logger := testhelpers.NewLogger(os.Stdout)
			dbs := newTestDB(t, logger)
			repo := repositories.NewInvestigationRepository(dbs, logger)
			ctx := context.TODO()
			var err error
			err = repo.FinishCompletion(ctx, tt.investigationTargetID, tt.userID, tt.previousCompletionID, "question", "answer")
			if tt.wantErr {
				require.Error(t, err, "expected error")
				return
			}
			require.NoError(t, err, "failed to read completion")
			var investigation *models.Investigation
			investigation, err = repo.Get(ctx, tt.investigationTargetID, tt.userID)
			require.NoError(t, err, "failed to read investigation")
			numCompletions := len(investigation.Completions)
			lastCompletion := investigation.Completions[numCompletions-1]
			require.Equal(t, lastCompletion.Order, int64(numCompletions-1), "wrong order")
			require.Equal(t, "question", lastCompletion.Question, "question mismatch")
			require.Equal(t, "answer", lastCompletion.Answer, "answer mismatch")
		})
	}
}

func Benchmark_InvestigationRepository(b *testing.B) {
	logger := testhelpers.NewLogger(os.Stdout)
	dbs := newBenchmarkDB(b, logger)
	repo := repositories.NewInvestigationRepository(dbs, logger)
	ctx := context.Background()
	investigationTarget := "le-bon"
	userID := []byte{1, 2, 3}
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := repo.Get(ctx, investigationTarget, userID)
			require.NoError(b, err)
		}
	})
}
