-- Nurtur local accounts.
--
-- Nurtur deliberately has no external identity provider. Anyone with the
-- owner-supplied signup key can create an account. Accounts are independent:
-- each one owns its own baby profile and care log. Passwords are PBKDF2-SHA256
-- hashes, never plain text.

CREATE TABLE accounts (
    id            INTEGER PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at    TEXT NOT NULL
);
