package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Account is the single local login created during onboarding.
type Account struct {
	ID           int64
	Username     string
	PasswordHash string
	CreatedAt    time.Time
}

// ErrAccountNotFound means no account has the requested username.
var ErrAccountNotFound = errors.New("store: account not found")

// ErrAccountExists means the single account slot is already taken.
var ErrAccountExists = errors.New("store: account already exists")

// CountAccounts reports how many accounts exist. Nurtur allows at most one.
func (s *Store) CountAccounts(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM accounts`).Scan(&count); err != nil {
		return 0, fmt.Errorf("store: count accounts: %w", err)
	}
	return count, nil
}

// AccountByUsername returns the account for username (already normalized).
func (s *Store) AccountByUsername(ctx context.Context, username string) (Account, error) {
	var (
		acct    Account
		created string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, created_at FROM accounts WHERE username = ?`,
		username,
	).Scan(&acct.ID, &acct.Username, &acct.PasswordHash, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrAccountNotFound
	}
	if err != nil {
		return Account{}, fmt.Errorf("store: get account: %w", err)
	}
	acct.CreatedAt, err = decodeTime(created)
	if err != nil {
		return Account{}, err
	}
	return acct, nil
}

// CreateAccount inserts the only account. It refuses when one already exists,
// including when two onboarding requests race.
func (s *Store) CreateAccount(ctx context.Context, username, passwordHash string, now time.Time) (Account, error) {
	return s.CreateAccountWithSettings(ctx, username, passwordHash, nil, now)
}

// CreateAccountWithSettings creates the account and its onboarding settings in
// one transaction, so a baby name can never be lost after the account exists.
func (s *Store) CreateAccountWithSettings(ctx context.Context, username, passwordHash string, settings map[string]string, now time.Time) (Account, error) {
	now = normalizeTime(now)
	acct := Account{Username: username, PasswordHash: passwordHash, CreatedAt: now}

	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM accounts`).Scan(&count); err != nil {
			return fmt.Errorf("store: count accounts: %w", err)
		}
		if count > 0 {
			return ErrAccountExists
		}
		result, err := tx.ExecContext(ctx,
			`INSERT INTO accounts (username, password_hash, created_at) VALUES (?, ?, ?)`,
			username, passwordHash, encodeTime(now))
		if err != nil {
			if isUniqueViolation(err) {
				return ErrAccountExists
			}
			return fmt.Errorf("store: create account: %w", err)
		}
		acct.ID, _ = result.LastInsertId()
		return setSettingsTx(ctx, tx, settings)
	})
	if err != nil {
		return Account{}, err
	}
	return acct, nil
}
