-- Nurtur phase 1 schema.
--
-- Durations are derived from feed_segments at read time; they are never stored
-- twice. Timestamps are UTC RFC3339 text, which sorts correctly and survives
-- export/import.

CREATE TABLE feeds (
    id          INTEGER PRIMARY KEY,
    started_at  TEXT NOT NULL,                 -- UTC RFC3339
    ended_at    TEXT,
    last_side   TEXT CHECK (last_side IN ('left', 'right')),
    notes       TEXT,
    created_at  TEXT NOT NULL,
    deleted_at  TEXT
);

-- At most one active feed exists at a time.
CREATE UNIQUE INDEX one_active_feed
    ON feeds ((ended_at IS NULL))
    WHERE ended_at IS NULL AND deleted_at IS NULL;

CREATE TABLE feed_segments (
    id          INTEGER PRIMARY KEY,
    feed_id     INTEGER NOT NULL REFERENCES feeds(id) ON DELETE CASCADE,
    side        TEXT NOT NULL CHECK (side IN ('left', 'right')),
    started_at  TEXT NOT NULL,
    ended_at    TEXT,
    CHECK (ended_at IS NULL OR ended_at >= started_at)
);

-- At most one open segment exists at a time, globally.
CREATE UNIQUE INDEX one_open_feed_segment
    ON feed_segments ((ended_at IS NULL))
    WHERE ended_at IS NULL;

CREATE INDEX idx_feed_segments_feed ON feed_segments (feed_id);

CREATE TABLE diapers (
    id           INTEGER PRIMARY KEY,
    logged_at    TEXT NOT NULL,
    type         TEXT NOT NULL CHECK (type IN ('wet', 'dirty', 'both')),
    color        TEXT CHECK (color IN ('yellow', 'mustard', 'green', 'dark')),
    consistency  TEXT CHECK (consistency IN ('seedy', 'liquid', 'hard')),
    notes        TEXT,
    created_at   TEXT NOT NULL,
    deleted_at   TEXT
);

CREATE TABLE sleeps (
    id          INTEGER PRIMARY KEY,
    started_at  TEXT NOT NULL,
    ended_at    TEXT,
    notes       TEXT,
    created_at  TEXT NOT NULL,
    deleted_at  TEXT
);

-- At most one open sleep session.
CREATE UNIQUE INDEX one_open_sleep
    ON sleeps ((ended_at IS NULL))
    WHERE ended_at IS NULL AND deleted_at IS NULL;

CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE INDEX idx_feeds_started     ON feeds (started_at DESC);
CREATE INDEX idx_diapers_logged    ON diapers (logged_at DESC);
CREATE INDEX idx_sleeps_started    ON sleeps (started_at DESC);
