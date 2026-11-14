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
	DeletedAt    *time.Time
}

// ErrAccountNotFound means no account has the requested username or id.
var ErrAccountNotFound = errors.New("store: account not found")

// ErrAccountExists means the requested username is already registered.
var ErrAccountExists = errors.New("store: account already exists")

// ErrAccountAlreadyDeleted means the account was already soft-deleted.
var ErrAccountAlreadyDeleted = errors.New("store: account already deleted")

// ErrAccountNotDeleted means the account is active and cannot be restored.
var ErrAccountNotDeleted = errors.New("store: account not deleted")

// CountAccounts reports how many active (non-deleted) accounts exist.
func (s *Store) CountAccounts(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM accounts WHERE deleted_at IS NULL`).Scan(&count); err != nil {
		return 0, fmt.Errorf("store: count accounts: %w", err)
	}
	return count, nil
}

// AccountByUsername returns the active account for username (already normalized).
func (s *Store) AccountByUsername(ctx context.Context, username string) (Account, error) {
	return s.scanSingleAccount(ctx,
		`SELECT id, username, password_hash, created_at, deleted_at FROM accounts WHERE username = ? AND deleted_at IS NULL`,
		username,
	)
}

// AccountByID returns the account for id, whether active or soft-deleted.
func (s *Store) AccountByID(ctx context.Context, id int64) (Account, error) {
	return s.scanSingleAccount(ctx,
		`SELECT id, username, password_hash, created_at, deleted_at FROM accounts WHERE id = ?`,
		id,
	)
}

// AccountActive reports whether id belongs to an existing, non-deleted account.
func (s *Store) AccountActive(ctx context.Context, id int64) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM accounts WHERE id = ? AND deleted_at IS NULL`, id,
	).Scan(&count); err != nil {
		return false, fmt.Errorf("store: check active account: %w", err)
	}
	return count > 0, nil
}

func (s *Store) scanSingleAccount(ctx context.Context, query string, arg any) (Account, error) {
	var (
		acct      Account
		created   string
		deletedAt sql.NullString
	)
	err := s.db.QueryRowContext(ctx, query, arg).Scan(
		&acct.ID, &acct.Username, &acct.PasswordHash, &created, &deletedAt,
	)
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
	acct.DeletedAt, err = decodeNullTime(deletedAt)
	if err != nil {
		return Account{}, err
	}
	return acct, nil
}

// ListAccounts returns every account (active first, then soft-deleted), oldest first.
func (s *Store) ListAccounts(ctx context.Context) ([]Account, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, username, created_at, deleted_at FROM accounts ORDER BY deleted_at IS NOT NULL ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("store: list accounts: %w", err)
	}
	defer rows.Close()

	accounts := make([]Account, 0, 8)
	for rows.Next() {
		var (
			acct      Account
			created   string
			deletedAt sql.NullString
		)
		if err := rows.Scan(&acct.ID, &acct.Username, &created, &deletedAt); err != nil {
			return nil, fmt.Errorf("store: scan account: %w", err)
		}
		acct.CreatedAt, err = decodeTime(created)
		if err != nil {
			return nil, err
		}
		acct.DeletedAt, err = decodeNullTime(deletedAt)
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

// SoftDeleteAccount marks an active account as deleted at now.
func (s *Store) SoftDeleteAccount(ctx context.Context, id int64, now time.Time) error {
	now = normalizeTime(now)
	res, err := s.db.ExecContext(ctx,
		`UPDATE accounts SET deleted_at = ? WHERE id = ? AND deleted_at IS NULL`,
		encodeTime(now), id)
	if err != nil {
		return fmt.Errorf("store: soft delete account: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: soft delete account rows: %w", err)
	}
	if n > 0 {
		return nil
	}
	var deleted sql.NullString
	err = s.db.QueryRowContext(ctx, `SELECT deleted_at FROM accounts WHERE id = ?`, id).Scan(&deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrAccountNotFound
	}
	if err != nil {
		return fmt.Errorf("store: check account delete: %w", err)
	}
	return ErrAccountAlreadyDeleted
}

// RestoreAccount clears deleted_at on a soft-deleted account so it can sign in again.
func (s *Store) RestoreAccount(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE accounts SET deleted_at = NULL WHERE id = ? AND deleted_at IS NOT NULL`,
		id)
	if err != nil {
		return fmt.Errorf("store: restore account: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: restore account rows: %w", err)
	}
	if n > 0 {
		return nil
	}
	var deleted sql.NullString
	err = s.db.QueryRowContext(ctx, `SELECT deleted_at FROM accounts WHERE id = ?`, id).Scan(&deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrAccountNotFound
	}
	if err != nil {
		return fmt.Errorf("store: check account restore: %w", err)
	}
	return ErrAccountNotDeleted
}

// ExpiredDeletedAccounts returns accounts soft-deleted at or before cutoff.
func (s *Store) ExpiredDeletedAccounts(ctx context.Context, cutoff time.Time) ([]Account, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, username, created_at, deleted_at FROM accounts
		 WHERE deleted_at IS NOT NULL AND deleted_at <= ?
		 ORDER BY deleted_at ASC, id ASC`,
		encodeTime(normalizeTime(cutoff)))
	if err != nil {
		return nil, fmt.Errorf("store: list expired accounts: %w", err)
	}
	defer rows.Close()

	accounts := make([]Account, 0, 4)
	for rows.Next() {
		var (
			acct      Account
			created   string
			deletedAt sql.NullString
		)
		if err := rows.Scan(&acct.ID, &acct.Username, &created, &deletedAt); err != nil {
			return nil, fmt.Errorf("store: scan expired account: %w", err)
		}
		acct.CreatedAt, err = decodeTime(created)
		if err != nil {
			return nil, err
		}
		acct.DeletedAt, err = decodeNullTime(deletedAt)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, acct)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list expired accounts: %w", err)
	}
	return accounts, nil
}

// HardDeleteAccount permanently removes an account and all of its scoped rows.
// Callers must remove external album objects first.
func (s *Store) HardDeleteAccount(ctx context.Context, id int64) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		scopedDeletes := []string{
			`DELETE FROM feed_segments WHERE account_id = ?`,
			`DELETE FROM feeds WHERE account_id = ?`,
			`DELETE FROM diapers WHERE account_id = ?`,
			`DELETE FROM sleeps WHERE account_id = ?`,
			`DELETE FROM bottles WHERE account_id = ?`,
			`DELETE FROM pumps WHERE account_id = ?`,
			`DELETE FROM solids WHERE account_id = ?`,
			`DELETE FROM album_photos WHERE account_id = ?`,
			`DELETE FROM settings WHERE account_id = ?`,
			`UPDATE invites SET used_account_id = NULL WHERE used_account_id = ?`,
			`DELETE FROM accounts WHERE id = ?`,
		}
		for _, stmt := range scopedDeletes {
			if _, err := tx.ExecContext(ctx, stmt, id); err != nil {
				return fmt.Errorf("store: hard delete account %d: %w", id, err)
			}
		}
		return nil
	})
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
