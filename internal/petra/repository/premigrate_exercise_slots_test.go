package repository_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/myrjola/petrapp/internal/petra/repository"
	"github.com/myrjola/petrapp/internal/platform/auth"
	"github.com/myrjola/petrapp/internal/platform/sqlitekit"
	"github.com/myrjola/petrapp/internal/platform/testkit"
)

// legacyProductSchemaSQL is the petra product schema as it stood before the
// workout_exercises -> exercise_slots rename. It lets the premigration tests
// build a database in the legacy shape. Delete it together with
// PreMigrateExerciseSlots once production has booted past the rename.
const legacyProductSchemaSQL = `
CREATE TABLE workout_preferences
(
    user_id                    INTEGER PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    monday_minutes             INTEGER NOT NULL DEFAULT 0 CHECK (monday_minutes IN (0, 45, 60, 90)),
    tuesday_minutes            INTEGER NOT NULL DEFAULT 0 CHECK (tuesday_minutes IN (0, 45, 60, 90)),
    wednesday_minutes          INTEGER NOT NULL DEFAULT 0 CHECK (wednesday_minutes IN (0, 45, 60, 90)),
    thursday_minutes           INTEGER NOT NULL DEFAULT 0 CHECK (thursday_minutes IN (0, 45, 60, 90)),
    friday_minutes             INTEGER NOT NULL DEFAULT 0 CHECK (friday_minutes IN (0, 45, 60, 90)),
    saturday_minutes           INTEGER NOT NULL DEFAULT 0 CHECK (saturday_minutes IN (0, 45, 60, 90)),
    sunday_minutes             INTEGER NOT NULL DEFAULT 0 CHECK (sunday_minutes IN (0, 45, 60, 90)),
    rest_notifications_enabled INTEGER NOT NULL DEFAULT 1 CHECK (rest_notifications_enabled IN (0, 1)),
    deload_enabled             INTEGER NOT NULL DEFAULT 0 CHECK (deload_enabled IN (0, 1)),
    mesocycle_length           INTEGER NOT NULL DEFAULT 5 CHECK (mesocycle_length BETWEEN 4 AND 7),
    mesocycle_anchor           TEXT CHECK (mesocycle_anchor IS NULL
                                           OR STRFTIME('%Y-%m-%d', mesocycle_anchor) = mesocycle_anchor)
) STRICT;

CREATE TABLE exercises
(
    id                       INTEGER PRIMARY KEY,
    name                     TEXT    NOT NULL UNIQUE CHECK (LENGTH(name) < 124),
    category                 TEXT    NOT NULL CHECK (category IN ('full_body', 'upper', 'lower')),
    exercise_type            TEXT    NOT NULL DEFAULT 'weighted'
                             CHECK (exercise_type IN ('weighted', 'bodyweight', 'assisted', 'time_based')),
    description_markdown     TEXT    NOT NULL DEFAULT '' CHECK (LENGTH(description_markdown) < 20000),
    default_starting_seconds INTEGER CHECK (default_starting_seconds IS NULL OR default_starting_seconds > 0),
    rep_min                  INTEGER CHECK (rep_min IS NULL OR (rep_min >= 1 AND rep_min <= 50)),
    rep_max                  INTEGER CHECK (rep_max IS NULL OR (rep_max >= 1 AND rep_max <= 50)),
    CHECK (exercise_type <> 'time_based' OR default_starting_seconds IS NOT NULL),
    CHECK (exercise_type =  'time_based' OR (rep_min IS NOT NULL AND rep_max IS NOT NULL)),
    CHECK (rep_min IS NULL OR rep_max IS NULL OR rep_min <= rep_max)
) STRICT;

CREATE TABLE workout_sessions
(
    user_id            INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    workout_date       TEXT    NOT NULL CHECK (STRFTIME('%Y-%m-%d', workout_date) = workout_date),
    difficulty_rating  INTEGER CHECK (difficulty_rating BETWEEN 1 AND 5),
    started_at         TEXT CHECK (started_at IS NULL OR STRFTIME('%Y-%m-%dT%H:%M:%fZ', started_at) = started_at),
    completed_at       TEXT CHECK (completed_at IS NULL OR STRFTIME('%Y-%m-%dT%H:%M:%fZ', completed_at) = completed_at),
    periodization_type TEXT    NOT NULL DEFAULT 'strength'
        CHECK (periodization_type IN ('strength', 'hypertrophy')),
    is_deload          INTEGER NOT NULL DEFAULT 0 CHECK (is_deload IN (0, 1)),

    PRIMARY KEY (user_id, workout_date)
) WITHOUT ROWID, STRICT;

CREATE TABLE workout_exercises
(
    workout_user_id     INTEGER NOT NULL,
    workout_date        TEXT    NOT NULL CHECK (STRFTIME('%Y-%m-%d', workout_date) = workout_date),
    position            INTEGER NOT NULL CHECK (position >= 0),
    exercise_id         INTEGER NOT NULL,
    warmup_completed_at TEXT CHECK (warmup_completed_at IS NULL OR
                                    STRFTIME('%Y-%m-%dT%H:%M:%fZ', warmup_completed_at) = warmup_completed_at),

    PRIMARY KEY (workout_user_id, workout_date, position),
    UNIQUE (workout_user_id, workout_date, exercise_id),
    FOREIGN KEY (workout_user_id, workout_date) REFERENCES workout_sessions (user_id, workout_date) ON DELETE CASCADE,
    FOREIGN KEY (exercise_id) REFERENCES exercises (id) DEFERRABLE INITIALLY DEFERRED
) WITHOUT ROWID, STRICT;

CREATE INDEX workout_exercises_user_exercise_date_idx
    ON workout_exercises (workout_user_id, exercise_id, workout_date);

CREATE TABLE exercise_sets
(
    workout_user_id INTEGER NOT NULL,
    workout_date    TEXT    NOT NULL CHECK (STRFTIME('%Y-%m-%d', workout_date) = workout_date),
    position        INTEGER NOT NULL,
    set_number      INTEGER NOT NULL CHECK (set_number > 0),
    weight_kg       REAL,
    target_value    INTEGER NOT NULL CHECK (target_value > 0),
    completed_value INTEGER CHECK (completed_value IS NULL OR completed_value >= 0),
    completed_at    TEXT CHECK (completed_at IS NULL OR
                                STRFTIME('%Y-%m-%dT%H:%M:%fZ', completed_at) = completed_at),
    signal          TEXT CHECK (signal IS NULL OR signal IN ('too_heavy', 'on_target', 'too_light')),

    PRIMARY KEY (workout_user_id, workout_date, position, set_number),
    FOREIGN KEY (workout_user_id, workout_date, position)
        REFERENCES workout_exercises (workout_user_id, workout_date, position) ON DELETE CASCADE
) WITHOUT ROWID, STRICT;

CREATE TABLE muscle_groups
(
    name TEXT NOT NULL PRIMARY KEY CHECK (LENGTH(name) < 64)
) WITHOUT ROWID, STRICT;

CREATE TABLE exercise_muscle_groups
(
    exercise_id       INTEGER NOT NULL REFERENCES exercises (id) ON DELETE CASCADE,
    muscle_group_name TEXT    NOT NULL REFERENCES muscle_groups (name) ON DELETE CASCADE,
    is_primary        INTEGER NOT NULL DEFAULT 0 CHECK (is_primary IN (0, 1)),

    PRIMARY KEY (exercise_id, muscle_group_name)
) WITHOUT ROWID, STRICT;

CREATE TABLE muscle_group_weekly_targets
(
    muscle_group_name   TEXT    PRIMARY KEY REFERENCES muscle_groups (name) ON DELETE CASCADE,
    min_sets            INTEGER NOT NULL CHECK (min_sets > 0),
    max_sets            INTEGER NOT NULL CHECK (max_sets >= min_sets)
) WITHOUT ROWID, STRICT;

CREATE TABLE feature_flags
(
    name    TEXT PRIMARY KEY CHECK (LENGTH(name) < 256),
    enabled INTEGER NOT NULL DEFAULT 0 CHECK (enabled IN (0, 1))
) WITHOUT ROWID, STRICT;

CREATE TABLE push_subscriptions
(
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    endpoint   TEXT    NOT NULL UNIQUE CHECK (LENGTH(endpoint) < 1024),
    p256dh     TEXT    NOT NULL CHECK (LENGTH(p256dh) < 256),
    auth       TEXT    NOT NULL CHECK (LENGTH(auth) < 256),
    created_at TEXT    NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ'))
        CHECK (STRFTIME('%Y-%m-%dT%H:%M:%fZ', created_at) = created_at)
) STRICT;

CREATE INDEX push_subscriptions_user_id ON push_subscriptions (user_id);

CREATE TABLE scheduled_pushes
(
    id              INTEGER PRIMARY KEY,
    workout_user_id INTEGER NOT NULL,
    workout_date    TEXT    NOT NULL CHECK (STRFTIME('%Y-%m-%d', workout_date) = workout_date),
    position        INTEGER NOT NULL,
    fire_at         TEXT    NOT NULL CHECK (STRFTIME('%Y-%m-%dT%H:%M:%fZ', fire_at) = fire_at),
    payload         TEXT    NOT NULL CHECK (LENGTH(payload) < 2048),
    created_at      TEXT    NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ'))
        CHECK (STRFTIME('%Y-%m-%dT%H:%M:%fZ', created_at) = created_at),

    FOREIGN KEY (workout_user_id, workout_date, position)
        REFERENCES workout_exercises (workout_user_id, workout_date, position) ON DELETE CASCADE
) STRICT;

CREATE UNIQUE INDEX scheduled_pushes_slot_uidx
    ON scheduled_pushes (workout_user_id, workout_date, position);
CREATE INDEX scheduled_pushes_fire_at ON scheduled_pushes (fire_at);
`

// newLegacyDB builds an in-memory database in the pre-rename shape (with the
// workout_exercises table) and returns it. cache=shared (used for :memory:)
// means the premigration's writes are visible to all pooled connections.
func newLegacyDB(t *testing.T) (context.Context, *sqlitekit.Database) {
	t.Helper()
	ctx := t.Context()
	db, err := sqlitekit.NewDatabase(ctx, sqlitekit.Config{
		URL:          ":memory:",
		Schema:       auth.SchemaSQL + "\n" + legacyProductSchemaSQL,
		Fixtures:     "",
		Logger:       testkit.NewLogger(testkit.NewWriter(t)),
		Premigration: nil,
	})
	if err != nil {
		t.Fatalf("create legacy database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return ctx, db
}

// seedLegacySlot inserts a user, session, exercise, a workout_exercises slot,
// one exercise_sets row, and one scheduled_pushes row, all wired by foreign
// key, so the premigration can be observed to preserve child relationships.
func seedLegacySlot(ctx context.Context, t *testing.T, db *sqlitekit.Database) {
	t.Helper()
	stmts := []string{
		`INSERT INTO users (webauthn_user_id, display_name) VALUES (X'01', 'Test User')`,
		`INSERT INTO exercises (id, name, category, rep_min, rep_max) VALUES (9999, 'Test Lift', 'lower', 5, 10)`,
		`INSERT INTO workout_sessions (user_id, workout_date) VALUES (1, '2026-06-10')`,
		`INSERT INTO workout_exercises (workout_user_id, workout_date, position, exercise_id)
		 VALUES (1, '2026-06-10', 0, 9999)`,
		`INSERT INTO exercise_sets (workout_user_id, workout_date, position, set_number, target_value)
		 VALUES (1, '2026-06-10', 0, 1, 8)`,
		`INSERT INTO scheduled_pushes (workout_user_id, workout_date, position, fire_at, payload)
		 VALUES (1, '2026-06-10', 0, '2026-06-10T10:00:00.000Z', '{}')`,
	}
	for _, stmt := range stmts {
		if _, err := db.ReadWrite.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed legacy slot (%q): %v", stmt, err)
		}
	}
}

func TestPreMigrateExerciseSlots_renamesAndPreservesData(t *testing.T) {
	t.Parallel()
	ctx, db := newLegacyDB(t)
	seedLegacySlot(ctx, t, db)

	if err := repository.PreMigrateExerciseSlots(ctx, db); err != nil {
		t.Fatalf("premigration: %v", err)
	}

	// The old table is gone and the new one carries the row.
	var legacyCount int
	if err := db.ReadOnly.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'workout_exercises'`,
	).Scan(&legacyCount); err != nil {
		t.Fatalf("count legacy table: %v", err)
	}
	if legacyCount != 0 {
		t.Errorf("workout_exercises still present after rename")
	}
	var slotCount int
	if err := db.ReadOnly.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM exercise_slots`,
	).Scan(&slotCount); err != nil {
		t.Fatalf("count exercise_slots: %v", err)
	}
	if slotCount != 1 {
		t.Errorf("exercise_slots row count = %d, want 1", slotCount)
	}

	// The child foreign key was auto-rewritten to point at exercise_slots.
	var ref string
	if err := db.ReadOnly.QueryRowContext(ctx,
		`SELECT "table" FROM pragma_foreign_key_list('exercise_sets')`,
	).Scan(&ref); err != nil {
		t.Fatalf("read exercise_sets foreign key: %v", err)
	}
	if ref != "exercise_slots" {
		t.Errorf("exercise_sets foreign key references %q, want exercise_slots", ref)
	}

	// The second child table's foreign key was rewritten too.
	var pushRef string
	if err := db.ReadOnly.QueryRowContext(ctx,
		`SELECT "table" FROM pragma_foreign_key_list('scheduled_pushes')`,
	).Scan(&pushRef); err != nil {
		t.Fatalf("read scheduled_pushes foreign key: %v", err)
	}
	if pushRef != "exercise_slots" {
		t.Errorf("scheduled_pushes foreign key references %q, want exercise_slots", pushRef)
	}

	// Idempotent: a second run is a no-op (the guard short-circuits).
	if err := repository.PreMigrateExerciseSlots(ctx, db); err != nil {
		t.Fatalf("second premigration run: %v", err)
	}
}

func TestPreMigrateExerciseSlots_fullMigratePathPreservesData(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	dbPath := filepath.Join(t.TempDir(), "petra.db")
	logger := testkit.NewLogger(testkit.NewWriter(t))

	// Build the database in the legacy shape and seed a slot with children.
	legacy, err := sqlitekit.NewDatabase(ctx, sqlitekit.Config{
		URL:          dbPath,
		Schema:       auth.SchemaSQL + "\n" + legacyProductSchemaSQL,
		Fixtures:     "",
		Logger:       logger,
		Premigration: nil,
	})
	if err != nil {
		t.Fatalf("create legacy file database: %v", err)
	}
	t.Cleanup(func() { _ = legacy.Close() })
	seedLegacySlot(ctx, t, legacy)
	if err = legacy.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	// Reopen with the real schema + premigration: the production boot path.
	migrated, err := sqlitekit.NewDatabase(ctx, sqlitekit.Config{
		URL:          dbPath,
		Schema:       auth.SchemaSQL + "\n" + repository.SchemaSQL,
		Fixtures:     repository.FixturesSQL,
		Logger:       logger,
		Premigration: repository.PreMigrateExerciseSlots,
	})
	if err != nil {
		t.Fatalf("reopen with real schema + premigration: %v", err)
	}
	t.Cleanup(func() { _ = migrated.Close() })

	// The seeded slot and its child set survived the rename + migrate.
	var joined int
	if err = migrated.ReadOnly.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM exercise_slots s
		   JOIN exercise_sets es
		     ON es.workout_user_id = s.workout_user_id
		    AND es.workout_date = s.workout_date
		    AND es.position = s.position`,
	).Scan(&joined); err != nil {
		t.Fatalf("join exercise_slots/exercise_sets: %v", err)
	}
	if joined != 1 {
		t.Errorf("joined slot+set rows = %d, want 1", joined)
	}

	// The third child table's row survived the rename + migrate too.
	var pushCount int
	if err = migrated.ReadOnly.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM scheduled_pushes`,
	).Scan(&pushCount); err != nil {
		t.Fatalf("count scheduled_pushes: %v", err)
	}
	if pushCount != 1 {
		t.Errorf("scheduled_pushes row count = %d, want 1", pushCount)
	}

	// The index was reconciled to the new name by the declarative migrator.
	var idxCount int
	if err = migrated.ReadOnly.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master
		  WHERE type = 'index' AND name = 'exercise_slots_user_exercise_date_idx'`,
	).Scan(&idxCount); err != nil {
		t.Fatalf("count renamed index: %v", err)
	}
	if idxCount != 1 {
		t.Errorf("renamed index present = %d, want 1", idxCount)
	}
}

func TestPreMigrateExerciseSlots_freshDatabaseIsNoOp(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	// A database already on the real schema has exercise_slots and no
	// workout_exercises; the premigration must be a clean no-op.
	db, err := sqlitekit.NewDatabase(ctx, sqlitekit.Config{
		URL:          ":memory:",
		Schema:       auth.SchemaSQL + "\n" + repository.SchemaSQL,
		Fixtures:     repository.FixturesSQL,
		Logger:       testkit.NewLogger(testkit.NewWriter(t)),
		Premigration: nil,
	})
	if err != nil {
		t.Fatalf("create fresh database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err = repository.PreMigrateExerciseSlots(ctx, db); err != nil {
		t.Fatalf("premigration on fresh database: %v", err)
	}

	var slotTableCount int
	if err = db.ReadOnly.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'exercise_slots'`,
	).Scan(&slotTableCount); err != nil {
		t.Fatalf("count exercise_slots table: %v", err)
	}
	if slotTableCount != 1 {
		t.Errorf("exercise_slots table present = %d, want 1", slotTableCount)
	}

	// The no-op leaves the table empty — it writes no spurious rows.
	var slotRowCount int
	if err = db.ReadOnly.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM exercise_slots`,
	).Scan(&slotRowCount); err != nil {
		t.Fatalf("count exercise_slots rows: %v", err)
	}
	if slotRowCount != 0 {
		t.Errorf("exercise_slots row count = %d, want 0", slotRowCount)
	}
}
