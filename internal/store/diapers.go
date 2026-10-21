package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"nurtur/internal/diaper"
)

const diaperColumns = `id, logged_at, type, COALESCE(color, ''), COALESCE(consistency, ''), COALESCE(notes, ''), created_at, deleted_at`

// CreateDiaper logs a diaper immediately. A matching event within
// diaper.DuplicateWindow is treated as a double tap: the existing row is
// returned with created=false and no second row is written.
func (s *Store) CreateDiaper(ctx context.Context, kind diaper.Kind, now time.Time) (diaper.Diaper, bool, error) {
	if !kind.Valid() {
		return diaper.Diaper{}, false, diaper.ErrInvalidKind
	}
	now = normalizeTime(now)
	var (
		out     diaper.Diaper
		created bool
	)

	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		duplicate, err := s.recentDuplicateDiaper(ctx, tx, kind, now)
		if err != nil {
			return err
		}
		if duplicate != nil {
			out = *duplicate
			return nil
		}

		res, err := tx.ExecContext(ctx,
			`INSERT INTO diapers (account_id, logged_at, type, created_at) VALUES (?, ?, ?, ?)`,
			AccountID(ctx), encodeTime(now), kind, encodeTime(now))
		if err != nil {
			return fmt.Errorf("store: insert diaper: %w", err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("store: diaper id: %w", err)
		}
		out = diaper.Diaper{ID: id, LoggedAt: now, Kind: kind, CreatedAt: now}
		created = true
		return nil
	})
	return out, created, err
}

// GetDiaper returns a non-deleted diaper.
func (s *Store) GetDiaper(ctx context.Context, id int64) (diaper.Diaper, error) {
	return s.getDiaperAt(ctx, s.db, id, false)
}

// LastDiaper returns the most recent event that counts as kind. A "both"
// event satisfies both wet and dirty.
func (s *Store) LastDiaper(ctx context.Context, kind diaper.Kind) (*diaper.Diaper, error) {
	if kind != diaper.Wet && kind != diaper.Dirty {
		return nil, diaper.ErrInvalidKind
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT `+diaperColumns+`
		FROM diapers
		WHERE account_id = ? AND deleted_at IS NULL AND (type = ? OR type = 'both')
		ORDER BY logged_at DESC, id DESC
		LIMIT 1`, AccountID(ctx), kind)
	d, err := scanDiaper(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// RecentDiapers lists events logged in [from, to), newest first.
func (s *Store) RecentDiapers(ctx context.Context, from, to time.Time, limit int) ([]diaper.Diaper, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+diaperColumns+`
		FROM diapers
		WHERE account_id = ? AND deleted_at IS NULL AND logged_at >= ? AND logged_at < ?
		ORDER BY logged_at DESC, id DESC
		LIMIT ?`,
		AccountID(ctx), encodeTime(from), encodeTime(to), limit)
	if err != nil {
		return nil, fmt.Errorf("store: list diapers: %w", err)
	}
	defer rows.Close()

	var out []diaper.Diaper
	for rows.Next() {
		d, err := scanDiaper(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list diapers: %w", err)
	}
	return out, nil
}

// UpdateDiaper corrects the details or logged time of an existing event.
func (s *Store) UpdateDiaper(ctx context.Context, id int64, edit diaper.Edit, now time.Time) (diaper.Diaper, error) {
	now = normalizeTime(now)
	edit.LoggedAt = normalizeTime(edit.LoggedAt)
	if err := edit.Validate(now); err != nil {
		return diaper.Diaper{}, err
	}
	var out diaper.Diaper

	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		d, err := s.getDiaperAt(ctx, tx, id, false)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE diapers SET logged_at = ?, color = ?, consistency = ?, notes = ? WHERE id = ? AND account_id = ? AND deleted_at IS NULL`,
			encodeTime(edit.LoggedAt), nullableString(string(edit.Color)), nullableString(string(edit.Consistency)), edit.Notes, id, AccountID(ctx)); err != nil {
			return fmt.Errorf("store: update diaper: %w", err)
		}
		d.LoggedAt = edit.LoggedAt
		d.Color = edit.Color
		d.Consistency = edit.Consistency
		d.Notes = edit.Notes
		out = d
		return nil
	})
	return out, err
}

// DeleteDiaper soft-deletes an event.
func (s *Store) DeleteDiaper(ctx context.Context, id int64, now time.Time) error {
	now = normalizeTime(now)
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE diapers SET deleted_at = ? WHERE id = ? AND account_id = ? AND deleted_at IS NULL`,
			encodeTime(now), id, AccountID(ctx))
		if err != nil {
			return fmt.Errorf("store: soft delete diaper: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("store: soft delete rows: %w", err)
		}
		if n != 1 {
			if _, err := s.getDiaperAt(ctx, tx, id, true); errors.Is(err, diaper.ErrNotFound) {
				return diaper.ErrNotFound
			} else if err != nil {
				return err
			}
			return diaper.ErrAlreadyDeleted
		}
		return nil
	})
}

// RestoreDiaper clears deleted_at. restored=false means it was already
// visible and nothing changed.
func (s *Store) RestoreDiaper(ctx context.Context, id int64) (diaper.Diaper, bool, error) {
	var (
		out      diaper.Diaper
		restored bool
	)
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		d, err := s.getDiaperAt(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if d.DeletedAt == nil {
			out = d
			return nil
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE diapers SET deleted_at = NULL WHERE id = ? AND account_id = ?`, id, AccountID(ctx)); err != nil {
			return fmt.Errorf("store: restore diaper: %w", err)
		}
		d.DeletedAt = nil
		out = d
		restored = true
		return nil
	})
	if err != nil {
		return diaper.Diaper{}, false, err
	}
	return out, restored, nil
}

func (s *Store) getDiaperAt(ctx context.Context, q queryer, id int64, includeDeleted bool) (diaper.Diaper, error) {
	query := `SELECT ` + diaperColumns + ` FROM diapers WHERE id = ? AND account_id = ?`
	args := []any{id, AccountID(ctx)}
	if !includeDeleted {
		query += ` AND deleted_at IS NULL`
	}
	d, err := scanDiaper(q.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return diaper.Diaper{}, diaper.ErrNotFound
	}
	if err != nil {
		return diaper.Diaper{}, err
	}
	return d, nil
}

func (s *Store) recentDuplicateDiaper(ctx context.Context, q queryer, kind diaper.Kind, now time.Time) (*diaper.Diaper, error) {
	from := now.Add(-diaper.DuplicateWindow)
	row := q.QueryRowContext(ctx, `
		SELECT `+diaperColumns+`
		FROM diapers
		WHERE account_id = ? AND deleted_at IS NULL AND type = ? AND logged_at >= ? AND logged_at <= ?
		ORDER BY logged_at DESC, id DESC
		LIMIT 1`,
		AccountID(ctx), kind, encodeTime(from), encodeTime(now))
	d, err := scanDiaper(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func scanDiaper(s scanner) (diaper.Diaper, error) {
	var (
		d           diaper.Diaper
		logged      string
		kind        string
		color       string
		consistency string
		notes       string
		created     string
		deleted     sql.NullString
	)
	if err := s.Scan(&d.ID, &logged, &kind, &color, &consistency, &notes, &created, &deleted); err != nil {
		return diaper.Diaper{}, err
	}
	var err error
	if d.LoggedAt, err = decodeTime(logged); err != nil {
		return diaper.Diaper{}, err
	}
	if d.CreatedAt, err = decodeTime(created); err != nil {
		return diaper.Diaper{}, err
	}
	if d.DeletedAt, err = decodeNullTime(deleted); err != nil {
		return diaper.Diaper{}, err
	}
	d.Kind = diaper.Kind(kind)
	d.Color = diaper.Color(color)
	d.Consistency = diaper.Consistency(consistency)
	d.Notes = notes
	return d, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
