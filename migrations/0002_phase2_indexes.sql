-- Nurtur phase 2: last-event and day-range indexes.
--
-- The tables themselves shipped in 0001 with the consistency column already
-- present; this migration only adds query paths for the status cards, the
-- unified today list, and the wake-window lookup.

CREATE INDEX idx_diapers_live_logged
    ON diapers (logged_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_diapers_kind_live
    ON diapers (type, logged_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_sleeps_live_started
    ON sleeps (started_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_sleeps_live_ended
    ON sleeps (ended_at DESC)
    WHERE deleted_at IS NULL;
