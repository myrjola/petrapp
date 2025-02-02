CREATE TABLE sessions
(
    token  TEXT PRIMARY KEY CHECK (length(token) < 256),
    data   BLOB NOT NULL CHECK (length(data) < 2056),
    expiry REAL NOT NULL
) WITHOUT ROWID, STRICT;

CREATE INDEX sessions_expiry_idx ON sessions (expiry);

CREATE TABLE users
(
    id           BLOB PRIMARY KEY CHECK (length(id) < 256),
    display_name TEXT NOT NULL CHECK (length(display_name) < 64),

    created      TEXT NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ')) CHECK (length(created) < 256),
    updated      TEXT NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ')) CHECK (length(updated) < 256)
) WITHOUT ROWID, STRICT;

CREATE TRIGGER users_updated_timestamp
    AFTER UPDATE
    ON users
BEGIN
    UPDATE users SET updated = STRFTIME('%Y-%m-%dT%H:%M:%fZ') WHERE id = old.id;
END;

CREATE TABLE credentials
(
    id                          BLOB PRIMARY KEY CHECK (length(id) < 256),
    public_key                  BLOB    NOT NULL CHECK (length(public_key) < 256),
    attestation_type            TEXT    NOT NULL CHECK (length(attestation_type) < 256),
    transport                   TEXT    NOT NULL CHECK (length(transport) < 256),
    flag_user_present           INTEGER NOT NULL CHECK (flag_user_present IN (0, 1)),
    flag_user_verified          INTEGER NOT NULL CHECK (flag_user_verified IN (0, 1)),
    flag_backup_eligible        INTEGER NOT NULL CHECK (flag_backup_eligible IN (0, 1)),
    flag_backup_state           INTEGER NOT NULL CHECK (flag_backup_state IN (0, 1)),
    authenticator_aaguid        BLOB    NOT NULL CHECK (length(authenticator_aaguid) < 256),
    authenticator_sign_count    INTEGER NOT NULL,
    authenticator_clone_warning INTEGER NOT NULL CHECK (authenticator_clone_warning IN (0, 1)),
    authenticator_attachment    TEXT    NOT NULL CHECK (length(authenticator_attachment) < 256),

    created                     TEXT    NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ')) CHECK (length(created) < 256),
    updated                     TEXT    NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ')) CHECK (length(updated) < 256),

    user_id                     BLOB    NOT NULL REFERENCES users (id) ON DELETE CASCADE
) WITHOUT ROWID, STRICT;

CREATE TRIGGER credentials_updated_timestamp
    AFTER UPDATE
    ON credentials
BEGIN
    UPDATE credentials SET updated = STRFTIME('%Y-%m-%dT%H:%M:%fZ') WHERE id = old.id;
END;
