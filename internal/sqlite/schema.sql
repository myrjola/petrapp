---------------------------------
-- Authentication and sessions --
---------------------------------

CREATE TABLE sessions
(
    token  TEXT PRIMARY KEY CHECK (LENGTH(token) < 256),
    data   BLOB NOT NULL CHECK (LENGTH(data) < 2056),
    expiry REAL NOT NULL
) WITHOUT ROWID, STRICT;

CREATE INDEX sessions_expiry_idx ON sessions (expiry);

CREATE TABLE users
(
    id               INTEGER PRIMARY KEY,
    webauthn_user_id BLOB    NOT NULL UNIQUE CHECK (LENGTH(webauthn_user_id) < 256),
    display_name     TEXT    NOT NULL CHECK (LENGTH(display_name) < 64),
    created          TEXT    NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ'))
        CHECK (STRFTIME('%Y-%m-%dT%H:%M:%fZ', created) = created),
    updated          TEXT    NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ'))
        CHECK (STRFTIME('%Y-%m-%dT%H:%M:%fZ', updated) = updated),
    is_admin         INTEGER NOT NULL DEFAULT 0 CHECK (is_admin IN (0, 1))
) STRICT;

CREATE INDEX users_webauthn_user_id_idx ON users (webauthn_user_id);

CREATE TRIGGER users_updated_timestamp
    AFTER UPDATE
    ON users
BEGIN
    UPDATE users SET updated = STRFTIME('%Y-%m-%dT%H:%M:%fZ') WHERE id = old.id;
END;

CREATE TABLE credentials
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
    user_id                     INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE
) WITHOUT ROWID, STRICT;

CREATE TRIGGER credentials_updated_timestamp
    AFTER UPDATE
    ON credentials
BEGIN
    UPDATE credentials SET updated = STRFTIME('%Y-%m-%dT%H:%M:%fZ') WHERE id = old.id;
END;

----------------------
-- Workout planning --
----------------------

CREATE TABLE workout_preferences
(
    user_id           INTEGER PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    monday_minutes    INTEGER NOT NULL DEFAULT 0 CHECK (monday_minutes IN (0, 45, 60, 90)),
    tuesday_minutes   INTEGER NOT NULL DEFAULT 0 CHECK (tuesday_minutes IN (0, 45, 60, 90)),
    wednesday_minutes INTEGER NOT NULL DEFAULT 0 CHECK (wednesday_minutes IN (0, 45, 60, 90)),
    thursday_minutes  INTEGER NOT NULL DEFAULT 0 CHECK (thursday_minutes IN (0, 45, 60, 90)),
    friday_minutes    INTEGER NOT NULL DEFAULT 0 CHECK (friday_minutes IN (0, 45, 60, 90)),
    saturday_minutes  INTEGER NOT NULL DEFAULT 0 CHECK (saturday_minutes IN (0, 45, 60, 90)),
    sunday_minutes    INTEGER NOT NULL DEFAULT 0 CHECK (sunday_minutes IN (0, 45, 60, 90))
) WITHOUT ROWID, STRICT;

CREATE TABLE exercises
(
    id                   INTEGER PRIMARY KEY,
    name                 TEXT NOT NULL UNIQUE CHECK (LENGTH(name) < 124),
    category             TEXT NOT NULL CHECK (category IN ('full_body', 'upper', 'lower')),
    exercise_type        TEXT NOT NULL DEFAULT 'weighted' CHECK (exercise_type IN ('weighted', 'bodyweight')),
    description_markdown TEXT NOT NULL DEFAULT '' CHECK (LENGTH(description_markdown) < 20000)
) STRICT;

CREATE TABLE workout_sessions
(
    user_id           INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    workout_date      TEXT    NOT NULL CHECK (LENGTH(workout_date) <= 10 AND
                                              DATE(workout_date, '+0 days') IS workout_date),
    difficulty_rating INTEGER CHECK (difficulty_rating BETWEEN 1 AND 5),
    started_at        TEXT CHECK (started_at IS NULL OR STRFTIME('%Y-%m-%dT%H:%M:%fZ', started_at) = started_at),
    completed_at      TEXT CHECK (completed_at IS NULL OR STRFTIME('%Y-%m-%dT%H:%M:%fZ', completed_at) = completed_at),

    PRIMARY KEY (user_id, workout_date)
) WITHOUT ROWID, STRICT;

CREATE TABLE workout_exercise
(
    workout_user_id     INTEGER NOT NULL,
    workout_date        TEXT    NOT NULL CHECK (STRFTIME('%Y-%m-%d', workout_date) = workout_date),
    exercise_id         INTEGER NOT NULL,
    warmup_completed_at TEXT CHECK (warmup_completed_at IS NULL OR
                                    STRFTIME('%Y-%m-%dT%H:%M:%fZ', warmup_completed_at) = warmup_completed_at),

    PRIMARY KEY (workout_user_id, workout_date, exercise_id),
    FOREIGN KEY (workout_user_id, workout_date) REFERENCES workout_sessions (user_id, workout_date) ON DELETE CASCADE,
    FOREIGN KEY (exercise_id) REFERENCES exercises (id) DEFERRABLE INITIALLY DEFERRED
) WITHOUT ROWID, STRICT;

CREATE TABLE exercise_sets
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
    FOREIGN KEY (workout_user_id, workout_date) REFERENCES workout_sessions (user_id, workout_date) ON DELETE CASCADE,
    FOREIGN KEY (exercise_id) REFERENCES exercises (id) DEFERRABLE INITIALLY DEFERRED
) WITHOUT ROWID, STRICT;

CREATE TABLE muscle_groups
(
    name TEXT NOT NULL UNIQUE CHECK (LENGTH(name) < 64) PRIMARY KEY
) WITHOUT ROWID, STRICT;

CREATE TABLE exercise_muscle_groups
(
    exercise_id       INTEGER NOT NULL REFERENCES exercises (id) ON DELETE CASCADE,
    muscle_group_name TEXT    NOT NULL REFERENCES muscle_groups (name) ON DELETE CASCADE,
    is_primary        INTEGER NOT NULL DEFAULT 0 CHECK (is_primary IN (0, 1)),

    PRIMARY KEY (exercise_id, muscle_group_name)
) WITHOUT ROWID, STRICT;

--------------------
-- AI Trainer Chat --
--------------------

CREATE TABLE conversations
(
    id              INTEGER PRIMARY KEY,
    user_id         INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    title           TEXT CHECK (title IS NULL OR LENGTH(title) < 201),
    created_at      TEXT NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ'))
        CHECK (STRFTIME('%Y-%m-%dT%H:%M:%fZ', created_at) = created_at),
    updated_at      TEXT NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ'))
        CHECK (STRFTIME('%Y-%m-%dT%H:%M:%fZ', updated_at) = updated_at),
    is_active       INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1)),
    context_summary TEXT
) STRICT;

CREATE INDEX idx_conversations_user_id ON conversations (user_id);
CREATE INDEX idx_conversations_updated_at ON conversations (updated_at);

CREATE TRIGGER conversations_updated_timestamp
    AFTER UPDATE
    ON conversations
BEGIN
    UPDATE conversations SET updated_at = STRFTIME('%Y-%m-%dT%H:%M:%fZ') WHERE id = old.id;
END;

CREATE TABLE chat_messages
(
    id                INTEGER PRIMARY KEY,
    conversation_id   INTEGER NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    message_type      TEXT NOT NULL CHECK (message_type IN ('user', 'assistant')),
    content           TEXT NOT NULL CHECK (LENGTH(content) <= 10000),
    created_at        TEXT NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ'))
        CHECK (STRFTIME('%Y-%m-%dT%H:%M:%fZ', created_at) = created_at),
    token_count       INTEGER CHECK (token_count IS NULL OR token_count >= 0),
    error_message     TEXT,
    query_executed    TEXT,
    execution_time_ms INTEGER CHECK (execution_time_ms IS NULL OR execution_time_ms >= 0)
) STRICT;

CREATE INDEX idx_chat_messages_conversation ON chat_messages (conversation_id, created_at);
CREATE INDEX idx_chat_messages_type ON chat_messages (message_type);

CREATE TABLE message_visualizations
(
    id           INTEGER PRIMARY KEY,
    message_id   INTEGER NOT NULL REFERENCES chat_messages (id) ON DELETE CASCADE,
    chart_type   TEXT NOT NULL CHECK (chart_type IN ('line', 'bar', 'scatter', 'pie', 'heatmap')),
    chart_config TEXT NOT NULL,
    data_query   TEXT NOT NULL CHECK (LENGTH(data_query) > 0),
    created_at   TEXT NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ'))
        CHECK (STRFTIME('%Y-%m-%dT%H:%M:%fZ', created_at) = created_at)
) STRICT;

CREATE TABLE token_usage
(
    id                INTEGER PRIMARY KEY,
    user_id           INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    conversation_id   INTEGER REFERENCES conversations (id) ON DELETE CASCADE,
    message_id        INTEGER REFERENCES chat_messages (id) ON DELETE CASCADE,
    tokens_used       INTEGER NOT NULL CHECK (tokens_used >= 0),
    request_type      TEXT NOT NULL CHECK (request_type IN ('chat', 'visualization', 'analysis', 'recommendation')),
    model             TEXT NOT NULL CHECK (LENGTH(model) < 100),
    timestamp         TEXT NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ'))
        CHECK (STRFTIME('%Y-%m-%dT%H:%M:%fZ', timestamp) = timestamp),
    response_time_ms  INTEGER CHECK (response_time_ms IS NULL OR response_time_ms >= 0)
) STRICT;

CREATE INDEX idx_message_visualizations_message ON message_visualizations (message_id);

CREATE INDEX idx_token_usage_user_date ON token_usage (user_id, DATE(timestamp));
CREATE INDEX idx_token_usage_conversation ON token_usage (conversation_id);
CREATE INDEX idx_token_usage_timestamp ON token_usage (timestamp);

-------------------
-- Feature flags --
-------------------

CREATE TABLE feature_flags
(
    name    TEXT PRIMARY KEY CHECK (LENGTH(name) < 256),
    enabled INTEGER NOT NULL DEFAULT 0 CHECK (enabled IN (0, 1))
) WITHOUT ROWID, STRICT;
