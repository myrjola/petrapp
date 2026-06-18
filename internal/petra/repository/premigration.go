package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/myrjola/petrapp/internal/petra/domain"
	"github.com/myrjola/petrapp/internal/platform/sqlitekit"
)

// PreMigrateExerciseContent rewrites the legacy exercises.description_markdown
// column into the structured content JSON column. It runs before the
// declarative migrator (which only reconciles structure, never data): it adds
// the content column and backfills it from parsed Markdown, so content is a
// column common to both the live and target schemas when the migrator rebuilds
// the table — otherwise the migrator would leave content at its DEFAULT and the
// old prose would be lost. The migrator then drops description_markdown.
//
// Fixture-seeded exercises are re-seeded with structured content on every boot,
// so this only matters for AI-generated rows whose content lives solely in the
// database. Idempotent and safe on fresh databases; delete it (and its wiring in
// the two main.go files) once prod has booted past it.
func PreMigrateExerciseContent(ctx context.Context, db *sqlitekit.Database) (err error) {
	hasLegacy, err := columnExists(ctx, db.ReadWrite, "exercises", "description_markdown")
	if err != nil {
		return fmt.Errorf("detect description_markdown column: %w", err)
	}
	hasContent, err := columnExists(ctx, db.ReadWrite, "exercises", "content")
	if err != nil {
		return fmt.Errorf("detect content column: %w", err)
	}
	// Fresh database (no legacy column) or already migrated (content present):
	// nothing to do.
	if !hasLegacy || hasContent {
		return nil
	}

	tx, err := db.ReadWrite.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin premigration transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("rollback premigration: %w", rollbackErr))
		}
	}()

	if _, err = tx.ExecContext(ctx,
		`ALTER TABLE exercises ADD COLUMN content TEXT NOT NULL DEFAULT '{}'`); err != nil {
		return fmt.Errorf("add content column: %w", err)
	}

	rewrites, err := readLegacyContent(ctx, tx)
	if err != nil {
		return err
	}
	for _, rw := range rewrites {
		if _, err = tx.ExecContext(ctx,
			`UPDATE exercises SET content = ? WHERE id = ?`, rw.content, rw.id); err != nil {
			return fmt.Errorf("update exercise %d content: %w", rw.id, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit premigration: %w", err)
	}
	return nil
}

// legacyRewrite pairs an exercise id with its parsed structured content JSON.
type legacyRewrite struct {
	id      int
	content string
}

// readLegacyContent reads every exercise's legacy Markdown description and
// parses it into structured content JSON, ready to write back.
func readLegacyContent(ctx context.Context, tx *sql.Tx) (_ []legacyRewrite, err error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, description_markdown FROM exercises`)
	if err != nil {
		return nil, fmt.Errorf("query legacy exercises: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close legacy exercises: %w", closeErr))
		}
	}()

	var rewrites []legacyRewrite
	for rows.Next() {
		var id int
		var markdown string
		if err = rows.Scan(&id, &markdown); err != nil {
			return nil, fmt.Errorf("scan legacy exercise: %w", err)
		}
		var content string
		if content, err = marshalExerciseContent(parseLegacyDescription(markdown)); err != nil {
			return nil, err
		}
		rewrites = append(rewrites, legacyRewrite{id: id, content: content})
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate legacy exercises: %w", err)
	}
	return rewrites, nil
}

// columnExists reports whether table has a column named column.
func columnExists(ctx context.Context, q queryer, table, column string) (bool, error) {
	var count int
	if err := q.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column,
	).Scan(&count); err != nil {
		return false, fmt.Errorf("query pragma_table_info: %w", err)
	}
	return count > 0, nil
}

// parseLegacyDescription parses the legacy three-section Markdown description
// (## Instructions numbered list, ## Common Mistakes bullets, ## Resources
// links) into a domain.Exercise carrying only the structured content fields.
// Common-mistake bold lead-in markers are stripped because mistakes are now
// flat one-line strings.
func parseLegacyDescription(markdown string) domain.Exercise {
	var ex domain.Exercise
	section := ""
	for raw := range strings.SplitSeq(markdown, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(line, "## "):
			section = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "## ")))
		case line == "":
			continue
		case section == "instructions":
			if item := stripListMarker(line); item != "" {
				ex.Instructions = append(ex.Instructions, item)
			}
		case section == "common mistakes":
			if item := stripListMarker(line); item != "" {
				ex.CommonMistakes = append(ex.CommonMistakes, strings.ReplaceAll(item, "**", ""))
			}
		case section == "resources":
			if res, ok := parseResourceLine(line); ok {
				ex.Resources = append(ex.Resources, res)
			}
		}
	}
	return ex
}

// stripListMarker removes a leading bullet ("- ") or ordered-list ("1. ")
// marker from a Markdown list line, returning the trimmed item text.
func stripListMarker(line string) string {
	if rest, ok := strings.CutPrefix(line, "- "); ok {
		return strings.TrimSpace(rest)
	}
	// Ordered list: "<digits>. text".
	if dot := strings.IndexByte(line, '.'); dot > 0 {
		if num := strings.TrimSpace(line[:dot]); num != "" && isDigits(num) {
			return strings.TrimSpace(line[dot+1:])
		}
	}
	return strings.TrimSpace(line)
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// parseResourceLine parses a "- [Title](URL)" Markdown link bullet into a
// Resource. It reports false when the line is not a well-formed link.
func parseResourceLine(line string) (domain.Resource, bool) {
	item := stripListMarker(line)
	open := strings.IndexByte(item, '[')
	mid := strings.Index(item, "](")
	if open != 0 || mid < 0 || !strings.HasSuffix(item, ")") {
		return domain.Resource{}, false
	}
	title := item[1:mid]
	url := item[mid+2 : len(item)-1]
	if title == "" || url == "" {
		return domain.Resource{}, false
	}
	return domain.Resource{Title: title, URL: url}, true
}
