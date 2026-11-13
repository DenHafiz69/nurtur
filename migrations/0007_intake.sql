-- Nurtur growing-baby intake tracking: bottles, pumping sessions, and solids.

CREATE TABLE bottles (
    id         INTEGER PRIMARY KEY,
    account_id INTEGER NOT NULL DEFAULT 0,
    logged_at  TEXT NOT NULL,
    milk_type  TEXT NOT NULL CHECK (milk_type IN ('breast_milk', 'formula')),
    amount_ml  INTEGER NOT NULL CHECK (amount_ml > 0),
    notes      TEXT,
    created_at TEXT NOT NULL,
    deleted_at TEXT
);

CREATE INDEX idx_bottles_account_logged
    ON bottles (account_id, logged_at DESC, id DESC)
    WHERE deleted_at IS NULL;

CREATE TABLE pumps (
    id               INTEGER PRIMARY KEY,
    account_id       INTEGER NOT NULL DEFAULT 0,
    logged_at        TEXT NOT NULL,
    side             TEXT NOT NULL CHECK (side IN ('left', 'right', 'both')),
    amount_ml        INTEGER NOT NULL DEFAULT 0 CHECK (amount_ml >= 0),
    duration_minutes INTEGER NOT NULL DEFAULT 0 CHECK (duration_minutes >= 0),
    notes            TEXT,
    created_at       TEXT NOT NULL,
    deleted_at       TEXT
);

CREATE INDEX idx_pumps_account_logged
    ON pumps (account_id, logged_at DESC, id DESC)
    WHERE deleted_at IS NULL;

CREATE TABLE solids (
    id         INTEGER PRIMARY KEY,
    account_id INTEGER NOT NULL DEFAULT 0,
    logged_at  TEXT NOT NULL,
    food       TEXT NOT NULL,
    portion    TEXT NOT NULL DEFAULT '' CHECK (portion IN ('', 'taste', 'some', 'most')),
    reaction   TEXT NOT NULL DEFAULT '' CHECK (reaction IN ('', 'none', 'mild', 'allergic')),
    notes      TEXT,
    created_at TEXT NOT NULL,
    deleted_at TEXT
);

CREATE INDEX idx_solids_account_logged
    ON solids (account_id, logged_at DESC, id DESC)
    WHERE deleted_at IS NULL;
