-- Album enhancements: favorites and milestone tags.

ALTER TABLE album_photos ADD COLUMN favorite INTEGER NOT NULL DEFAULT 0;
ALTER TABLE album_photos ADD COLUMN tag TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_album_account_fav_taken
    ON album_photos (account_id, favorite, taken_at DESC, id DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_album_account_tag_taken
    ON album_photos (account_id, tag, taken_at DESC, id DESC)
    WHERE deleted_at IS NULL;
