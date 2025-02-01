package repositories

import (
	"context"
	"database/sql"
	"github.com/myrjola/petrapp/internal/errors"
	"github.com/myrjola/petrapp/internal/models"
	"github.com/myrjola/petrapp/internal/sqlite"
	"log/slog"
)

type InvestigationRepository struct {
	database *sqlite.Database
	logger   *slog.Logger
}

func NewInvestigationRepository(dbs *sqlite.Database, logger *slog.Logger) *InvestigationRepository {
	return &InvestigationRepository{
		database: dbs,
		logger:   logger.With("source", "InvestigationRepository"),
	}
}

func (r *InvestigationRepository) Get(
	ctx context.Context,
	investigationTargetID string,
	userID []byte,
) (*models.Investigation, error) {
	var (
		investigationTarget models.InvestigationTarget
		completions         []models.Completion
		err                 error
		rows                *sql.Rows
	)

	stmt := `SELECT id, name, short_name, type, image_path FROM investigation_targets WHERE id = ?`
	if err = r.database.ReadOnly.QueryRowContext(ctx, stmt, investigationTargetID).Scan(
		&investigationTarget.ID,
		&investigationTarget.Name,
		&investigationTarget.ShortName,
		&investigationTarget.Type,
		&investigationTarget.ImagePath,
	); err != nil {
		return nil, errors.Wrap(err, "read investigation target")
	}

	stmt = `SELECT id, "order", question, answer
	FROM completions
	WHERE user_id = ? AND investigation_target_id = ?
	ORDER BY "order"`
	if rows, err = r.database.ReadOnly.QueryContext(ctx, stmt, userID, investigationTargetID); err != nil {
		return nil, errors.Wrap(err, "query completions")
	}
	defer func() {
		if err = rows.Close(); err != nil {
			err = errors.Wrap(err, "close rows")
			r.logger.Error("could not close rows", errors.SlogError(err))
		}
	}()
	for rows.Next() {
		var (
			completion models.Completion
		)
		if err = rows.Scan(&completion.ID, &completion.Order, &completion.Question, &completion.Answer); err != nil {
			return nil, errors.Wrap(err, "scan completion")
		}
		completions = append(completions, completion)
	}
	if err = rows.Err(); err != nil {
		return nil, errors.Wrap(err, "rows error")
	}

	investigation := models.Investigation{
		Target:      investigationTarget,
		Completions: completions,
	}

	return &investigation, nil
}

// FinishCompletion adds a new completion to the investigation for given investigation target and user.
//
// The completion is added to the end of the completions list. The order of the completion is determined by the previous
// completion. If no previous completion exists, set previousCompletionID to -1.
func (r *InvestigationRepository) FinishCompletion(
	ctx context.Context,
	investigationTargetID string,
	userID []byte,
	previousCompletionID int64,
	question string,
	answer string,
) error {
	stmt := `WITH new_order AS (
SELECT   
       CASE WHEN @previous_completion_id IS -1
       THEN 0
       ELSE MAX("order") + 1
       END AS "order"
	   FROM completions
	   WHERE id = @previous_completion_id
		 AND investigation_target_id = @investigation_target_id
		 AND user_id = @user_id)
INSERT
INTO completions (user_id, investigation_target_id, question, answer, "order")
VALUES (@user_id, @investigation_target_id, @question, @answer, (SELECT "order" FROM new_order));`
	params := []any{
		sql.Named("user_id", userID),
		sql.Named("investigation_target_id", investigationTargetID),
		sql.Named("question", question),
		sql.Named("answer", answer),
		sql.Named("previous_completion_id", previousCompletionID),
	}
	if _, err := r.database.ReadWrite.ExecContext(ctx, stmt, params...); err != nil {
		return errors.Wrap(err, "insert completion")
	}
	return nil
}
