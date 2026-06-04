package auth

// SchemaSQL defines the tables auth owns: scs sessions, users, and webauthn
// credentials. Apps concatenate this ahead of their own product schema when
// constructing the database, so product tables may FK to users.
const SchemaSQL = `
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

-- webauthn_user_id UNIQUE constraint already creates an implicit index; no separate index needed.

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

CREATE INDEX credentials_user_id_idx ON credentials (user_id);

CREATE TRIGGER credentials_updated_timestamp
    AFTER UPDATE
    ON credentials
BEGIN
    UPDATE credentials SET updated = STRFTIME('%Y-%m-%dT%H:%M:%fZ') WHERE id = old.id;
END;
`
