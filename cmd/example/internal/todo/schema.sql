CREATE TABLE todos
(
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    title   TEXT    NOT NULL,
    notes   TEXT    NOT NULL DEFAULT '',
    done    INTEGER NOT NULL DEFAULT 0,
    created TEXT    NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ'))
);
