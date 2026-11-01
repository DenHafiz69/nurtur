-- Nurtur album metadata.
--
-- Photo bytes live in private object storage; this table is the only index.
-- upload_id is generated once in the browser and reused on retries, so a
-- network failure cannot create two rows for one photo. taken_at is UTC; the
-- UI groups it into local days using NURTUR_TZ.

CREATE TABLE album_photos (
    id              INTEGER PRIMARY KEY,
    account_id      INTEGER NOT NULL DEFAULT 0,
    upload_id       TEXT NOT NULL,
    taken_at        TEXT NOT NULL,
    taken_at_source TEXT NOT NULL
        CHECK (taken_at_source IN ('exif', 'file', 'upload', 'manual')),
    caption         TEXT,
    width           INTEGER NOT NULL,
    height          INTEGER NOT NULL,
    bytes           INTEGER NOT NULL,
    thumb_bytes     INTEGER NOT NULL,
    content_type    TEXT NOT NULL,
    full_key        TEXT NOT NULL,
    thumb_key       TEXT NOT NULL,
    created_at      TEXT NOT NULL,
    deleted_at      TEXT
);

-- Retrying a failed upload must return the existing row, not insert another.
CREATE UNIQUE INDEX one_album_upload
    ON album_photos (account_id, upload_id);

CREATE INDEX idx_album_account_taken
    ON album_photos (account_id, taken_at DESC, id DESC)
    WHERE deleted_at IS NULL;
