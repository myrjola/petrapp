-- The auth tables (sessions, users, credentials) live in
-- internal/platform/auth as auth.SchemaSQL and are concatenated ahead of this
-- product schema at database-construction time, so the tables below may FK to
-- users.

----------------------
-- Workout planning --
----------------------

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
    session_goal TEXT    NOT NULL DEFAULT 'strength'
        CHECK (session_goal IN ('strength', 'hypertrophy')),
    is_deload          INTEGER NOT NULL DEFAULT 0 CHECK (is_deload IN (0, 1)),

    PRIMARY KEY (user_id, workout_date)
) WITHOUT ROWID, STRICT;

CREATE TABLE exercise_slots
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

-- Supports lookups of an exercise's history across workouts, e.g. latest starting weight.
CREATE INDEX exercise_slots_user_exercise_date_idx
    ON exercise_slots (workout_user_id, exercise_id, workout_date);

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
        REFERENCES exercise_slots (workout_user_id, workout_date, position) ON DELETE CASCADE
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

-------------------
-- Feature flags --
-------------------

CREATE TABLE feature_flags
(
    name    TEXT PRIMARY KEY CHECK (LENGTH(name) < 256),
    enabled INTEGER NOT NULL DEFAULT 0 CHECK (enabled IN (0, 1))
) WITHOUT ROWID, STRICT;

-------------------
-- Push delivery --
-------------------

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
        REFERENCES exercise_slots (workout_user_id, workout_date, position) ON DELETE CASCADE
) STRICT;

CREATE UNIQUE INDEX scheduled_pushes_slot_uidx
    ON scheduled_pushes (workout_user_id, workout_date, position);
CREATE INDEX scheduled_pushes_fire_at ON scheduled_pushes (fire_at);
