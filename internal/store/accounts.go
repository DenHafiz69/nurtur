package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Account is one local login created during onboarding.
type Account struct {
	ID           int64
	Username     string
	PasswordHash string
	CreatedAt    time.Time
}

// ErrAccountNotFound means no account has the requested username.
var ErrAccountNotFound = errors.New("store: account not found")

// ErrAccountExists means the requested username is already registered.
var ErrAccountExists = errors.New("store: account already exists")

// CountAccounts reports how many accounts exist.
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

// ListAccounts returns every account, oldest first. It backs the read-only
// admin screen.
func (s *Store) ListAccounts(ctx context.Context) ([]Account, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, username, created_at FROM accounts ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("store: list accounts: %w", err)
	}
	defer rows.Close()

	accounts := make([]Account, 0, 8)
	for rows.Next() {
		var (
			acct    Account
			created string
		)
		if err := rows.Scan(&acct.ID, &acct.Username, &created); err != nil {
			return nil, fmt.Errorf("store: scan account: %w", err)
		}
		acct.CreatedAt, err = decodeTime(created)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, acct)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list accounts: %w", err)
	}
	return accounts, nil
}

// CreateAccount inserts a new account. It refuses a username that is already
// registered, including when two signup requests race.
func (s *Store) CreateAccount(ctx context.Context, username, passwordHash string, now time.Time) (Account, error) {
	return s.CreateAccountWithSettings(ctx, username, passwordHash, nil, now)
}

// CreateAccountWithSettings creates the account and its onboarding settings in
// one transaction, so a baby name can never be lost after the account exists.
func (s *Store) CreateAccountWithSettings(ctx context.Context, username, passwordHash string, settings map[string]string, now time.Time) (Account, error) {
	return s.createAccount(ctx, username, passwordHash, settings, 0, now)
}

// CreateAccountWithInvite creates the account and consumes the invite link in
// the same transaction. A link that was used or revoked in the meantime fails
// the whole write with ErrInviteUnavailable, so no account is created twice for
// one invitation.
func (s *Store) CreateAccountWithInvite(ctx context.Context, username, passwordHash string, settings map[string]string, inviteID int64, now time.Time) (Account, error) {
	return s.createAccount(ctx, username, passwordHash, settings, inviteID, now)
}

func (s *Store) createAccount(ctx context.Context, username, passwordHash string, settings map[string]string, inviteID int64, now time.Time) (Account, error) {
	now = normalizeTime(now)
	acct := Account{Username: username, PasswordHash: passwordHash, CreatedAt: now}

	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		var existing int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM accounts`).Scan(&existing); err != nil {
			return fmt.Errorf("store: count accounts: %w", err)
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

		if inviteID != 0 {
			if err := claimInviteTx(ctx, tx, inviteID, acct.ID, now); err != nil {
				return err
			}
		}

		// Data created before accounts existed lives in the account 0 namespace.
		// The first real account adopts it; later accounts start empty.
		if existing == 0 {
			if err := claimLegacyDataTx(ctx, tx, acct.ID); err != nil {
				return err
			}
		}
		return setSettingsTx(ctx, tx, acct.ID, settings)
	})
	if err != nil {
		return Account{}, err
	}
	return acct, nil
}

// claimLegacyDataTx moves the pre-account namespace (account 0) into the first
// account so an existing install keeps its history after enabling signups.
func claimLegacyDataTx(ctx context.Context, tx *sql.Tx, accountID int64) error {
	statements := []string{
		`UPDATE feeds SET account_id = ? WHERE account_id = 0`,
		`UPDATE feed_segments SET account_id = ? WHERE account_id = 0`,
		`UPDATE diapers SET account_id = ? WHERE account_id = 0`,
		`UPDATE sleeps SET account_id = ? WHERE account_id = 0`,
		`UPDATE settings SET account_id = ? WHERE account_id = 0`,
	}
	for _, stmt := range statements {
		if _, err := tx.ExecContext(ctx, stmt, accountID); err != nil {
			return fmt.Errorf("store: claim legacy data: %w", err)
		}
	}
	return nil
}
