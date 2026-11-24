-- Growth measurements: weight (grams), height/length (millimeters), and head circumference (millimeters).

CREATE TABLE growth (
    id         INTEGER PRIMARY KEY,
    account_id INTEGER NOT NULL DEFAULT 0,
    logged_at  TEXT NOT NULL,
    weight_g   INTEGER NOT NULL DEFAULT 0 CHECK (weight_g >= 0),
    height_mm  INTEGER NOT NULL DEFAULT 0 CHECK (height_mm >= 0),
    head_mm    INTEGER NOT NULL DEFAULT 0 CHECK (head_mm >= 0),
    notes      TEXT,
    created_at TEXT NOT NULL,
    deleted_at TEXT
);

CREATE INDEX idx_growth_account_logged
    ON growth (account_id, logged_at DESC, id DESC)
    WHERE deleted_at IS NULL;
