-- Nurtur local accounts.
--
-- Nurtur is a single-household app and deliberately has no external identity
-- provider. At most one account is created, using the owner-supplied signup
-- key; everyone else signs in with the username and password stored here.
-- Passwords are PBKDF2-SHA256 hashes, never plain text.

CREATE TABLE accounts (
    id            INTEGER PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at    TEXT NOT NULL
);
