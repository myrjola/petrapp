-- Step 1: Disable foreign key constraints
PRAGMA foreign_keys=OFF;

-- Step 2: Start transaction
BEGIN TRANSACTION;

-- Step 3: Remember current schema version (optional but recommended)
PRAGMA schema_version;

-- Step 4: Create new users table with integer primary key
CREATE TABLE new_users
(
    id               INTEGER PRIMARY KEY,
    webauthn_user_id BLOB    NOT NULL CHECK (LENGTH(webauthn_user_id) < 256),
    display_name     TEXT    NOT NULL CHECK (LENGTH(display_name) < 64),
    created          TEXT    NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ'))
        CHECK (STRFTIME('%Y-%m-%dT%H:%M:%fZ', created) = created),
    updated          TEXT    NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ'))
        CHECK (STRFTIME('%Y-%m-%dT%H:%M:%fZ', updated) = updated),
    is_admin         INTEGER NOT NULL DEFAULT 0 CHECK (is_admin IN (0, 1))
) STRICT;

-- Create unique index on webauthn_user_id for efficient authentication lookups
CREATE UNIQUE INDEX idx_users_webauthn_user_id ON new_users(webauthn_user_id);

-- Step 5: Copy data from old users table
INSERT INTO new_users (webauthn_user_id, display_name, created, updated, is_admin)
SELECT id, display_name, created, updated, is_admin FROM users;

-- Step 6: Create new credentials table with integer user_id
CREATE TABLE new_credentials
(
    id                          BLOB PRIMARY KEY CHECK (LENGTH(id) < 256),
    public_key                  BLOB    NOT NULL CHECK (LENGTH(public_key) < 256),
    attestation_type            TEXT    NOT NULL CHECK (LENGTH(attestation_type) < 256),
    transport                   TEXT    NOT NULL CHECK (LENGTH(transport) < 256),
    flag_user_present           INTEGER NOT NULL CHECK (flag_user_present IN (0, 1)),
    flag_user_verified          INTEGER NOT NULL CHECK (flag_user_verified IN (0, 1)),
    flag_backup_eligible        INTEGER NOT NULL CHECK (flag_backup_eligible IN (0, 1)),
    flag_backup_state           INTEGER NOT NULL CHECK (flag_backup_state IN (0, 1)),
    authenticator_aaguid        BLOB    NOT NULL CHECK (LENGTH(authenticator_aaguid) < 256),
    authenticator_sign_count    INTEGER NOT NULL,
    authenticator_clone_warning INTEGER NOT NULL CHECK (authenticator_clone_warning IN (0, 1)),
    authenticator_attachment    TEXT    NOT NULL CHECK (LENGTH(authenticator_attachment) < 256),
    created                     TEXT    NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ'))
        CHECK (STRFTIME('%Y-%m-%dT%H:%M:%fZ', created) = created),
    updated                     TEXT    NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ'))
        CHECK (STRFTIME('%Y-%m-%dT%H:%M:%fZ', updated) = updated),
    user_id                     INTEGER NOT NULL REFERENCES new_users (id) ON DELETE CASCADE
) WITHOUT ROWID, STRICT;

-- Step 7: Copy credentials data, mapping old BLOB user_id to new INTEGER id
INSERT INTO new_credentials
SELECT c.id, c.public_key, c.attestation_type, c.transport,
       c.flag_user_present, c.flag_user_verified, c.flag_backup_eligible, c.flag_backup_state,
       c.authenticator_aaguid, c.authenticator_sign_count, c.authenticator_clone_warning,
       c.authenticator_attachment, c.created, c.updated, nu.id
FROM credentials c
         JOIN users ou ON c.user_id = ou.id
         JOIN new_users nu ON ou.id = nu.webauthn_user_id;

-- Step 8: Create new workout_preferences table with integer user_id
CREATE TABLE new_workout_preferences
(
    user_id           INTEGER PRIMARY KEY REFERENCES new_users (id) ON DELETE CASCADE,
    monday_minutes    INTEGER NOT NULL DEFAULT 0 CHECK (monday_minutes IN (0, 45, 60, 90)),
    tuesday_minutes   INTEGER NOT NULL DEFAULT 0 CHECK (tuesday_minutes IN (0, 45, 60, 90)),
    wednesday_minutes INTEGER NOT NULL DEFAULT 0 CHECK (wednesday_minutes IN (0, 45, 60, 90)),
    thursday_minutes  INTEGER NOT NULL DEFAULT 0 CHECK (thursday_minutes IN (0, 45, 60, 90)),
    friday_minutes    INTEGER NOT NULL DEFAULT 0 CHECK (friday_minutes IN (0, 45, 60, 90)),
    saturday_minutes  INTEGER NOT NULL DEFAULT 0 CHECK (saturday_minutes IN (0, 45, 60, 90)),
    sunday_minutes    INTEGER NOT NULL DEFAULT 0 CHECK (sunday_minutes IN (0, 45, 60, 90))
) STRICT;

-- Step 9: Copy workout_preferences data
INSERT INTO new_workout_preferences
SELECT nu.id, wp.monday_minutes, wp.tuesday_minutes, wp.wednesday_minutes,
       wp.thursday_minutes, wp.friday_minutes, wp.saturday_minutes, wp.sunday_minutes
FROM workout_preferences wp
         JOIN users ou ON wp.user_id = ou.id
         JOIN new_users nu ON ou.id = nu.webauthn_user_id;

-- Step 10: Create new workout_sessions table with integer user_id
CREATE TABLE new_workout_sessions
(
    user_id           INTEGER NOT NULL REFERENCES new_users (id) ON DELETE CASCADE,
    workout_date      TEXT NOT NULL CHECK (LENGTH(workout_date) <= 10 AND
                                           DATE(workout_date, '+0 days') IS workout_date),
    difficulty_rating INTEGER CHECK (difficulty_rating BETWEEN 1 AND 5),
    started_at        TEXT CHECK (started_at IS NULL OR STRFTIME('%Y-%m-%dT%H:%M:%fZ', started_at) = started_at),
    completed_at      TEXT CHECK (completed_at IS NULL OR STRFTIME('%Y-%m-%dT%H:%M:%fZ', completed_at) = completed_at),

    PRIMARY KEY (user_id, workout_date)
) WITHOUT ROWID, STRICT;

-- Step 11: Copy workout_sessions data
INSERT INTO new_workout_sessions
SELECT nu.id, ws.workout_date, ws.difficulty_rating, ws.started_at, ws.completed_at
FROM workout_sessions ws
         JOIN users ou ON ws.user_id = ou.id
         JOIN new_users nu ON ou.id = nu.webauthn_user_id;

-- Step 12: Create new workout_exercise table with integer workout_user_id
CREATE TABLE new_workout_exercise
(
    workout_user_id     INTEGER NOT NULL,
    workout_date        TEXT    NOT NULL CHECK (STRFTIME('%Y-%m-%d', workout_date) = workout_date),
    exercise_id         INTEGER NOT NULL,
    warmup_completed_at TEXT CHECK (warmup_completed_at IS NULL OR
                                    STRFTIME('%Y-%m-%dT%H:%M:%fZ', warmup_completed_at) = warmup_completed_at),

    PRIMARY KEY (workout_user_id, workout_date, exercise_id),
    FOREIGN KEY (workout_user_id, workout_date) REFERENCES new_workout_sessions (user_id, workout_date) ON DELETE CASCADE,
    FOREIGN KEY (exercise_id) REFERENCES exercises (id) DEFERRABLE INITIALLY DEFERRED
) WITHOUT ROWID, STRICT;

-- Step 13: Copy workout_exercise data
INSERT INTO new_workout_exercise
SELECT nu.id, we.workout_date, we.exercise_id, we.warmup_completed_at
FROM workout_exercise we
         JOIN users ou ON we.workout_user_id = ou.id
         JOIN new_users nu ON ou.id = nu.webauthn_user_id;

-- Step 14: Create new exercise_sets table with integer workout_user_id
CREATE TABLE new_exercise_sets
(
    workout_user_id INTEGER NOT NULL,
    workout_date    TEXT    NOT NULL CHECK (STRFTIME('%Y-%m-%d', workout_date) = workout_date),
    exercise_id     INTEGER NOT NULL,
    set_number      INTEGER NOT NULL CHECK (set_number > 0),
    weight_kg       REAL CHECK (weight_kg IS NULL OR weight_kg >= 0),
    min_reps        INTEGER NOT NULL CHECK (min_reps > 0),
    max_reps        INTEGER NOT NULL CHECK (max_reps >= min_reps),
    completed_reps  INTEGER,
    completed_at    TEXT CHECK (completed_at IS NULL OR STRFTIME('%Y-%m-%dT%H:%M:%fZ', completed_at) = completed_at),

    PRIMARY KEY (workout_user_id, workout_date, exercise_id, set_number),
    FOREIGN KEY (workout_user_id, workout_date) REFERENCES new_workout_sessions (user_id, workout_date) ON DELETE CASCADE,
    FOREIGN KEY (exercise_id) REFERENCES exercises (id) DEFERRABLE INITIALLY DEFERRED
) WITHOUT ROWID, STRICT;

-- Step 15: Copy exercise_sets data
INSERT INTO new_exercise_sets
SELECT nu.id, es.workout_date, es.exercise_id, es.set_number,
       es.weight_kg, es.min_reps, es.max_reps, es.completed_reps, es.completed_at
FROM exercise_sets es
         JOIN users ou ON es.workout_user_id = ou.id
         JOIN new_users nu ON ou.id = nu.webauthn_user_id;

-- Step 16: Drop old triggers
DROP TRIGGER users_updated_timestamp;
DROP TRIGGER credentials_updated_timestamp;

-- Step 17: Drop old tables in correct order (respecting foreign key dependencies)
DROP TABLE exercise_sets;
DROP TABLE workout_exercise;
DROP TABLE workout_sessions;
DROP TABLE workout_preferences;
DROP TABLE credentials;
DROP TABLE users;

-- Step 18: Rename new tables to original names
ALTER TABLE new_users RENAME TO users;
ALTER TABLE new_credentials RENAME TO credentials;
ALTER TABLE new_workout_preferences RENAME TO workout_preferences;
ALTER TABLE new_workout_sessions RENAME TO workout_sessions;
ALTER TABLE new_workout_exercise RENAME TO workout_exercise;
ALTER TABLE new_exercise_sets RENAME TO exercise_sets;

-- Step 19: Recreate triggers
CREATE TRIGGER users_updated_timestamp
    AFTER UPDATE
    ON users
BEGIN
    UPDATE users SET updated = STRFTIME('%Y-%m-%dT%H:%M:%fZ') WHERE id = old.id;
END;

CREATE TRIGGER credentials_updated_timestamp
    AFTER UPDATE
    ON credentials
BEGIN
    UPDATE credentials SET updated = STRFTIME('%Y-%m-%dT%H:%M:%fZ') WHERE id = old.id;
END;

-- Step 20: Verify foreign key constraints
PRAGMA foreign_key_check;

-- Step 21: Commit transaction
COMMIT;

-- Step 22: Re-enable foreign key constraints
PRAGMA foreign_keys=ON;

-- Step 23: Optional - Run integrity check
PRAGMA integrity_check;

-- Step 24: Optional - Analyze tables for query optimizer
ANALYZE;