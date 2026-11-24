package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"nurtur/internal/growth"
)

const growthColumns = `id, account_id, logged_at, weight_g, height_mm, head_mm, COALESCE(notes, ''), created_at, deleted_at`

// CreateGrowth logs a growth measurement for the current account.
func (s *Store) CreateGrowth(ctx context.Context, m growth.Measurement, now time.Time) (growth.Measurement, error) {
	m.AccountID = AccountID(ctx)
	m.LoggedAt = normalizeTime(m.LoggedAt)
	m.CreatedAt = normalizeTime(now)
	m.Notes = strings.TrimSpace(m.Notes)
	if err := m.Validate(m.CreatedAt); err != nil {
		return growth.Measurement{}, err
	}

	res, err := s.db.ExecContext(ctx, `
		INSERT INTO growth (account_id, logged_at, weight_g, height_mm, head_mm, notes, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		m.AccountID,
		encodeTime(m.LoggedAt),
		m.WeightG,
		m.HeightMM,
		m.HeadMM,
		nullableString(m.Notes),
		encodeTime(m.CreatedAt),
	)
	if err != nil {
		return growth.Measurement{}, fmt.Errorf("store: create growth: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return growth.Measurement{}, fmt.Errorf("store: growth id: %w", err)
	}
	m.ID = id
	s.notifyChange()
	return m, nil
}

// GetGrowth returns one live growth measurement in the current account.
func (s *Store) GetGrowth(ctx context.Context, id int64) (growth.Measurement, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+growthColumns+` FROM growth
		WHERE id = ? AND account_id = ? AND deleted_at IS NULL`,
		id, AccountID(ctx),
	)
	m, err := scanGrowth(row)
	if errors.Is(err, sql.ErrNoRows) {
		return growth.Measurement{}, growth.ErrNotFound
	}
	if err != nil {
		return growth.Measurement{}, fmt.Errorf("store: get growth: %w", err)
	}
	return m, nil
}

// UpdateGrowth updates an existing live growth measurement.
func (s *Store) UpdateGrowth(ctx context.Context, id int64, loggedAt time.Time, weightG, heightMM, headMM int, notes string, now time.Time) (growth.Measurement, error) {
	m := growth.Measurement{
		LoggedAt: normalizeTime(loggedAt),
		WeightG:  weightG,
		HeightMM: heightMM,
		HeadMM:   headMM,
		Notes:    strings.TrimSpace(notes),
	}
	if err := m.Validate(normalizeTime(now)); err != nil {
		return growth.Measurement{}, err
	}

	res, err := s.db.ExecContext(ctx, `
		UPDATE growth
		SET logged_at = ?, weight_g = ?, height_mm = ?, head_mm = ?, notes = ?
		WHERE id = ? AND account_id = ? AND deleted_at IS NULL`,
		encodeTime(m.LoggedAt), m.WeightG, m.HeightMM, m.HeadMM, nullableString(m.Notes),
		id, AccountID(ctx),
	)
	if err != nil {
		return growth.Measurement{}, fmt.Errorf("store: update growth: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return growth.Measurement{}, fmt.Errorf("store: update growth rows: %w", err)
	}
	if n == 0 {
		return growth.Measurement{}, growth.ErrNotFound
	}
	updated, err := s.GetGrowth(ctx, id)
	if err != nil {
		return growth.Measurement{}, err
	}
	s.notifyChange()
	return updated, nil
}

// DeleteGrowth soft-deletes a growth measurement.
func (s *Store) DeleteGrowth(ctx context.Context, id int64, deletedAt time.Time) (growth.Measurement, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE growth SET deleted_at = ?
		WHERE id = ? AND account_id = ? AND deleted_at IS NULL`,
		encodeTime(normalizeTime(deletedAt)), id, AccountID(ctx),
	)
	if err != nil {
		return growth.Measurement{}, fmt.Errorf("store: delete growth: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return growth.Measurement{}, fmt.Errorf("store: delete growth rows: %w", err)
	}
	if n == 0 {
		var exists int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(1) FROM growth WHERE id = ? AND account_id = ?`,
			id, AccountID(ctx),
		).Scan(&exists); err != nil {
			return growth.Measurement{}, fmt.Errorf("store: check growth: %w", err)
		}
		if exists == 0 {
			return growth.Measurement{}, growth.ErrNotFound
		}
		return growth.Measurement{}, growth.ErrAlreadyDeleted
	}
	s.notifyChange()
	row := s.db.QueryRowContext(ctx,
		`SELECT `+growthColumns+` FROM growth WHERE id = ? AND account_id = ?`,
		id, AccountID(ctx),
	)
	m, err := scanGrowth(row)
	if err != nil {
		return growth.Measurement{}, fmt.Errorf("store: reload deleted growth: %w", err)
	}
	return m, nil
}

// RestoreGrowth clears deleted_at for an undone delete.
func (s *Store) RestoreGrowth(ctx context.Context, id int64) (growth.Measurement, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE growth SET deleted_at = NULL
		WHERE id = ? AND account_id = ? AND deleted_at IS NOT NULL`,
		id, AccountID(ctx),
	)
	if err != nil {
		return growth.Measurement{}, fmt.Errorf("store: restore growth: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return growth.Measurement{}, fmt.Errorf("store: restore growth rows: %w", err)
	}
	if n == 0 {
		m, getErr := s.GetGrowth(ctx, id)
		if getErr != nil {
			return growth.Measurement{}, getErr
		}
		return m, nil
	}
	s.notifyChange()
	return s.GetGrowth(ctx, id)
}

// LatestGrowth returns the most recent live growth measurement, if any.
func (s *Store) LatestGrowth(ctx context.Context) (*growth.Measurement, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+growthColumns+` FROM growth
		WHERE account_id = ? AND deleted_at IS NULL
		ORDER BY logged_at DESC, id DESC LIMIT 1`,
		AccountID(ctx),
	)
	m, err := scanGrowth(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: latest growth: %w", err)
	}
	return &m, nil
}

// AllGrowth returns every growth row (including soft-deleted) for export.
func (s *Store) AllGrowth(ctx context.Context) ([]growth.Measurement, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+growthColumns+` FROM growth
		WHERE account_id = ?
		ORDER BY logged_at ASC, id ASC`,
		AccountID(ctx),
	)
	if err != nil {
		return nil, fmt.Errorf("store: export growth: %w", err)
	}
	defer rows.Close()

	var out []growth.Measurement
	for rows.Next() {
		m, err := scanGrowth(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) growthByIDs(ctx context.Context, ids []int64) (map[int64]growth.Measurement, error) {
	out := make(map[int64]growth.Measurement, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	placeholders, args := idPlaceholders(AccountID(ctx), ids)
	// #nosec G202 -- columns are compile-time constants and placeholders are generated '?' tokens.
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+growthColumns+` FROM growth
		WHERE account_id = ? AND deleted_at IS NULL AND id IN (`+placeholders+`)`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("store: growth by ids: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		m, err := scanGrowth(rows)
		if err != nil {
			return nil, err
		}
		out[m.ID] = m
	}
	return out, rows.Err()
}

func scanGrowth(sc scanner) (growth.Measurement, error) {
	var (
		m         growth.Measurement
		loggedRaw string
		created   string
		deleted   sql.NullString
	)
	if err := sc.Scan(&m.ID, &m.AccountID, &loggedRaw, &m.WeightG, &m.HeightMM, &m.HeadMM, &m.Notes, &created, &deleted); err != nil {
		return growth.Measurement{}, err
	}
	var err error
	if m.LoggedAt, err = decodeTime(loggedRaw); err != nil {
		return growth.Measurement{}, err
	}
	if m.CreatedAt, err = decodeTime(created); err != nil {
		return growth.Measurement{}, err
	}
	if m.DeletedAt, err = decodeNullTime(deleted); err != nil {
		return growth.Measurement{}, err
	}
	return m, nil
}
