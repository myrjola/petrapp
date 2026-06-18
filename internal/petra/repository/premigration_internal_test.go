package repository

import (
	"slices"
	"testing"

	"github.com/myrjola/petrapp/internal/petra/domain"
	"github.com/myrjola/petrapp/internal/platform/sqlitekit"
	"github.com/myrjola/petrapp/internal/platform/testkit"
)

func TestParseLegacyDescription(t *testing.T) {
	t.Parallel()

	md := "## Instructions\n" +
		"1. Stand tall.\n" +
		"2. Press the bar up.\n\n" +
		"## Common Mistakes\n" +
		"- **Flaring elbows**: keep them tucked.\n" +
		"- Bouncing the bar.\n\n" +
		"## Resources\n" +
		"- [Video tutorial](https://example.com/v)\n"

	got := parseLegacyDescription(md)

	if !slices.Equal(got.Instructions, []string{"Stand tall.", "Press the bar up."}) {
		t.Errorf("Instructions = %#v", got.Instructions)
	}
	// Bold lead-in markers are stripped; mistakes are flat.
	if !slices.Equal(got.CommonMistakes, []string{"Flaring elbows: keep them tucked.", "Bouncing the bar."}) {
		t.Errorf("CommonMistakes = %#v", got.CommonMistakes)
	}
	if len(got.Resources) != 1 ||
		got.Resources[0] != (domain.Resource{Title: "Video tutorial", URL: "https://example.com/v"}) {
		t.Errorf("Resources = %#v", got.Resources)
	}
}

func TestParseLegacyDescription_Empty(t *testing.T) {
	t.Parallel()

	got := parseLegacyDescription("")
	if len(got.Instructions) != 0 || len(got.CommonMistakes) != 0 || len(got.Resources) != 0 {
		t.Errorf("expected empty content, got %#v", got)
	}
}

// TestPreMigrateExerciseContent boots a database on the legacy schema (with a
// description_markdown column), then runs the premigration directly and asserts
// it adds the content column, backfills it from parsed Markdown, and is a no-op
// on a second run.
func TestPreMigrateExerciseContent(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	const legacySchema = `CREATE TABLE exercises (
		id                   INTEGER PRIMARY KEY,
		name                 TEXT NOT NULL,
		description_markdown TEXT NOT NULL DEFAULT ''
	) STRICT;`

	db, err := sqlitekit.NewDatabase(ctx, sqlitekit.Config{
		URL:          ":memory:",
		Schema:       legacySchema,
		Fixtures:     "",
		Logger:       testkit.NewLogger(testkit.NewWriter(t)),
		Premigration: nil,
	})
	if err != nil {
		t.Fatalf("create legacy database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	const md = "## Instructions\n1. Step one\n\n## Resources\n- [Guide](https://example.com/g)\n"
	if _, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercises (id, name, description_markdown) VALUES (1, 'Legacy', ?)`, md); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}

	if err = PreMigrateExerciseContent(ctx, db); err != nil {
		t.Fatalf("premigration: %v", err)
	}

	hasContent, err := columnExists(ctx, db.ReadOnly, "exercises", "content")
	if err != nil {
		t.Fatalf("columnExists: %v", err)
	}
	if !hasContent {
		t.Fatal("premigration did not add the content column")
	}

	var raw string
	if err = db.ReadOnly.QueryRowContext(ctx,
		`SELECT content FROM exercises WHERE id = 1`).Scan(&raw); err != nil {
		t.Fatalf("read content: %v", err)
	}
	var ex domain.Exercise
	if err = unmarshalExerciseContent(raw, &ex); err != nil {
		t.Fatalf("unmarshal backfilled content: %v", err)
	}
	if !slices.Equal(ex.Instructions, []string{"Step one"}) {
		t.Errorf("backfilled Instructions = %#v", ex.Instructions)
	}
	if len(ex.Resources) != 1 || ex.Resources[0].URL != "https://example.com/g" {
		t.Errorf("backfilled Resources = %#v", ex.Resources)
	}

	// Idempotent: a second run short-circuits without error.
	if err = PreMigrateExerciseContent(ctx, db); err != nil {
		t.Fatalf("second premigration run: %v", err)
	}
}
