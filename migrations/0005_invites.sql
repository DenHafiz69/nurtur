-- Nurtur admin invites.
--
-- The admin generates magic links that let one person create an account
-- without the shared signup key. Only the SHA-256 hash of a token is stored,
-- so a copy of this table cannot be turned back into working links. A token is
-- single-use: used_at is written in the same transaction as the new account.

CREATE TABLE invites (
    id              INTEGER PRIMARY KEY,
    token_hash      TEXT NOT NULL UNIQUE,
    label           TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL,
    expires_at      TEXT NOT NULL,
    used_at         TEXT,
    used_account_id INTEGER,
    revoked_at      TEXT
);

CREATE INDEX idx_invites_created ON invites (created_at DESC);
