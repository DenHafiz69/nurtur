-- Soft delete for local accounts. Deleted accounts are locked out immediately,
-- recoverable from /admin for 30 days, and permanently purged after that.

ALTER TABLE accounts ADD COLUMN deleted_at TEXT;

CREATE INDEX idx_accounts_deleted_at
    ON accounts (deleted_at)
    WHERE deleted_at IS NOT NULL;
