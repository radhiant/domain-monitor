package store

// schemaSQL is applied on every start; each statement is idempotent.
//
// Timestamps are Unix seconds rather than SQLite datetimes: every query here is
// a range scan or a bucket division, and integers make both trivial.
//
// The certificate, DNS and registration records are stored as JSON payloads
// instead of exploded columns. They are display-only — nothing ever filters or
// sorts on them in SQL, because the wall reads those from the in-memory
// snapshot — so a JSON column keeps the schema from churning every time a
// field is added to the model.
const schemaSQL = `
CREATE TABLE IF NOT EXISTS targets (
    id         TEXT PRIMARY KEY,
    apex       TEXT    NOT NULL,
    host       TEXT    NOT NULL,
    url        TEXT    NOT NULL,
    label      TEXT    NOT NULL,
    is_apex    INTEGER NOT NULL DEFAULT 0,
    ord        INTEGER NOT NULL DEFAULT 0,
    archived   INTEGER NOT NULL DEFAULT 0,
    first_seen INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS checks (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    target_id   TEXT    NOT NULL,
    ts          INTEGER NOT NULL,
    up          INTEGER NOT NULL,
    status      TEXT    NOT NULL,
    status_code INTEGER NOT NULL DEFAULT 0,
    latency_ms  REAL    NOT NULL DEFAULT 0,
    error       TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_checks_target_ts ON checks(target_id, ts);
CREATE INDEX IF NOT EXISTS idx_checks_ts        ON checks(ts);

CREATE TABLE IF NOT EXISTS incidents (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    target_id  TEXT    NOT NULL,
    started_at INTEGER NOT NULL,
    ended_at   INTEGER,
    reason     TEXT    NOT NULL DEFAULT '',
    last_error TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_incidents_target ON incidents(target_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_incidents_open   ON incidents(ended_at);

CREATE TABLE IF NOT EXISTS events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    target_id   TEXT    NOT NULL,
    at          INTEGER NOT NULL,
    from_status TEXT    NOT NULL,
    to_status   TEXT    NOT NULL,
    detail      TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_events_at ON events(at DESC);

CREATE TABLE IF NOT EXISTS cert_state (
    target_id  TEXT PRIMARY KEY,
    checked_at INTEGER NOT NULL,
    payload    TEXT    NOT NULL
);

CREATE TABLE IF NOT EXISTS dns_state (
    target_id  TEXT PRIMARY KEY,
    checked_at INTEGER NOT NULL,
    payload    TEXT    NOT NULL
);

CREATE TABLE IF NOT EXISTS domain_state (
    apex       TEXT PRIMARY KEY,
    checked_at INTEGER NOT NULL,
    payload    TEXT    NOT NULL
);
`
