package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Invite is one admin-generated signup link. Only TokenHash is stored, so the
// original token exists only in the link shown at creation time.
type Invite struct {
	ID            int64
	TokenHash     string
	Label         string
	CreatedAt     time.Time
	ExpiresAt     time.Time
	UsedAt        *time.Time
	UsedAccountID *int64
	RevokedAt     *time.Time
}

// Active reports whether the invite can still create an account at now.
func (i Invite) Active(now time.Time) bool {
	return i.UsedAt == nil && i.RevokedAt == nil && now.Before(i.ExpiresAt)
}

// ErrInviteNotFound means no invite has the requested token hash, or that an
// invite exists but is no longer active.
var ErrInviteNotFound = errors.New("store: invite not found")

// ErrInviteUnavailable means an invite was consumed or revoked while a signup
// was in flight.
var ErrInviteUnavailable = errors.New("store: invite is no longer available")

// CreateInvite stores the hash of a new signup token and returns the record.
func (s *Store) CreateInvite(ctx context.Context, tokenHash, label string, now, expiresAt time.Time) (Invite, error) {
	now = normalizeTime(now)
	expiresAt = normalizeTime(expiresAt)
	invite := Invite{
		TokenHash: tokenHash,
		Label:     label,
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}

	result, err := s.db.ExecContext(ctx,
		`INSERT INTO invites (token_hash, label, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		tokenHash, label, encodeTime(now), encodeTime(expiresAt))
	if err != nil {
		return Invite{}, fmt.Errorf("store: create invite: %w", err)
	}
	invite.ID, _ = result.LastInsertId()
	return invite, nil
}

// InviteByTokenHash returns the invite for a token hash, active or not.
func (s *Store) InviteByTokenHash(ctx context.Context, tokenHash string) (Invite, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, token_hash, label, created_at, expires_at, used_at, used_account_id, revoked_at
		 FROM invites WHERE token_hash = ?`, tokenHash)
	invite, err := scanInvite(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Invite{}, ErrInviteNotFound
	}
	if err != nil {
		return Invite{}, fmt.Errorf("store: get invite: %w", err)
	}
	return invite, nil
}

// ListInvites returns the newest invites first. The admin page is a small
// single-household screen, so a bounded list is enough.
func (s *Store) ListInvites(ctx context.Context, limit int) ([]Invite, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, token_hash, label, created_at, expires_at, used_at, used_account_id, revoked_at
		 FROM invites ORDER BY created_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list invites: %w", err)
	}
	defer rows.Close()

	invites := make([]Invite, 0, limit)
	for rows.Next() {
		invite, err := scanInvite(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan invite: %w", err)
		}
		invites = append(invites, invite)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list invites: %w", err)
	}
	return invites, nil
}

// RevokeInvite invalidates an unused invite. Used invites and invites that were
// already revoked are reported as not found so the caller can show one calm
// message instead of an internal state.
func (s *Store) RevokeInvite(ctx context.Context, id int64, now time.Time) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE invites SET revoked_at = ?
		 WHERE id = ? AND used_at IS NULL AND revoked_at IS NULL`,
		encodeTime(normalizeTime(now)), id)
	if err != nil {
		return fmt.Errorf("store: revoke invite: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: revoke invite: %w", err)
	}
	if affected == 0 {
		return ErrInviteNotFound
	}
	return nil
}

// claimInviteTx marks an invite used inside the account-creation transaction.
// The guarded UPDATE makes two racing signups share one link safely.
func claimInviteTx(ctx context.Context, tx *sql.Tx, inviteID, accountID int64, now time.Time) error {
	result, err := tx.ExecContext(ctx,
		`UPDATE invites SET used_at = ?, used_account_id = ?
		 WHERE id = ? AND used_at IS NULL AND revoked_at IS NULL AND expires_at > ?`,
		encodeTime(normalizeTime(now)), accountID, inviteID, encodeTime(normalizeTime(now)))
	if err != nil {
		return fmt.Errorf("store: claim invite: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: claim invite: %w", err)
	}
	if affected == 0 {
		return ErrInviteUnavailable
	}
	return nil
}

type inviteScanner interface {
	Scan(dest ...any) error
}

func scanInvite(row inviteScanner) (Invite, error) {
	var (
		invite        Invite
		created       string
		expires       string
		usedAt        sql.NullString
		usedAccountID sql.NullInt64
		revokedAt     sql.NullString
	)
	if err := row.Scan(
		&invite.ID, &invite.TokenHash, &invite.Label, &created, &expires,
		&usedAt, &usedAccountID, &revokedAt,
	); err != nil {
		return Invite{}, err
	}

	parsedCreated, err := decodeTime(created)
	if err != nil {
		return Invite{}, err
	}
	invite.CreatedAt = parsedCreated
	parsedExpires, err := decodeTime(expires)
	if err != nil {
		return Invite{}, err
	}
	invite.ExpiresAt = parsedExpires
	if usedAt.Valid {
		parsed, err := decodeTime(usedAt.String)
		if err != nil {
			return Invite{}, err
		}
		invite.UsedAt = &parsed
	}
	if usedAccountID.Valid {
		value := usedAccountID.Int64
		invite.UsedAccountID = &value
	}
	if revokedAt.Valid {
		parsed, err := decodeTime(revokedAt.String)
		if err != nil {
			return Invite{}, err
		}
		invite.RevokedAt = &parsed
	}
	return invite, nil
}
