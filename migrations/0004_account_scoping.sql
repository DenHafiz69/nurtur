-- Nurtur phase 3: per-account data.
--
-- Each account owns a private baby profile and event log. account_id 0 is the
-- legacy namespace used by passcode-only or auth-disabled installs. Existing
-- rows are assigned to the oldest account when one exists; otherwise they stay
-- at 0 and are claimed by the first account created.

ALTER TABLE feeds ADD COLUMN account_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE feed_segments ADD COLUMN account_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE diapers ADD COLUMN account_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sleeps ADD COLUMN account_id INTEGER NOT NULL DEFAULT 0;

UPDATE feeds SET account_id = COALESCE((SELECT MIN(id) FROM accounts), 0);
UPDATE feed_segments SET account_id = COALESCE(
    (SELECT f.account_id FROM feeds f WHERE f.id = feed_segments.feed_id), 0);
UPDATE diapers SET account_id = COALESCE((SELECT MIN(id) FROM accounts), 0);
UPDATE sleeps SET account_id = COALESCE((SELECT MIN(id) FROM accounts), 0);

CREATE TABLE settings_scoped (
    account_id INTEGER NOT NULL DEFAULT 0,
    key        TEXT NOT NULL,
    value      TEXT NOT NULL,
    PRIMARY KEY (account_id, key)
);

INSERT INTO settings_scoped (account_id, key, value)
    SELECT COALESCE((SELECT MIN(id) FROM accounts), 0), key, value FROM settings;

DROP TABLE settings;
ALTER TABLE settings_scoped RENAME TO settings;

-- Active-row uniqueness is now per account rather than per server.
DROP INDEX one_active_feed;
CREATE UNIQUE INDEX one_active_feed
    ON feeds (account_id)
    WHERE ended_at IS NULL AND deleted_at IS NULL;

DROP INDEX one_open_feed_segment;
CREATE UNIQUE INDEX one_open_feed_segment
    ON feed_segments (account_id)
    WHERE ended_at IS NULL;

DROP INDEX one_open_sleep;
CREATE UNIQUE INDEX one_open_sleep
    ON sleeps (account_id)
    WHERE ended_at IS NULL AND deleted_at IS NULL;

CREATE INDEX idx_feeds_account_started
    ON feeds (account_id, started_at DESC);

CREATE INDEX idx_diapers_account_logged
    ON diapers (account_id, logged_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_sleeps_account_started
    ON sleeps (account_id, started_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_feed_segments_account_feed
    ON feed_segments (account_id, feed_id);
