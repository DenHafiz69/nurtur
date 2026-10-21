package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"nurtur/internal/sleep"
)

const sleepColumns = `id, started_at, ended_at, COALESCE(notes, ''), created_at, deleted_at`

// ActiveSleep returns the running sleep, or nil when awake.
func (s *Store) ActiveSleep(ctx context.Context) (*sleep.Session, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+sleepColumns+`
		FROM sleeps
		WHERE account_id = ? AND ended_at IS NULL AND deleted_at IS NULL
		ORDER BY started_at DESC, id DESC
		LIMIT 1`, AccountID(ctx))
	session, err := scanSleep(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// GetSleep returns a non-deleted sleep.
func (s *Store) GetSleep(ctx context.Context, id int64) (sleep.Session, error) {
	return s.getSleepAt(ctx, s.db, id, false)
}

// LastEndedSleep returns the most recent completed sleep.
func (s *Store) LastEndedSleep(ctx context.Context) (*sleep.Session, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+sleepColumns+`
		FROM sleeps
		WHERE account_id = ? AND deleted_at IS NULL AND ended_at IS NOT NULL
		ORDER BY ended_at DESC, id DESC
		LIMIT 1`, AccountID(ctx))
	session, err := scanSleep(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// SleepsBetween lists sleep sessions that overlap [from, to), newest first.
func (s *Store) SleepsBetween(ctx context.Context, from, to, now time.Time, limit int) ([]sleep.Session, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+sleepColumns+`
		FROM sleeps
		WHERE account_id = ?
		  AND deleted_at IS NULL
		  AND started_at < ?
		  AND COALESCE(ended_at, ?) > ?
		ORDER BY started_at DESC, id DESC
		LIMIT ?`,
		AccountID(ctx), encodeTime(to), encodeTime(now), encodeTime(from), limit)
	if err != nil {
		return nil, fmt.Errorf("store: list sleeps: %w", err)
	}
	defer rows.Close()

	var out []sleep.Session
	for rows.Next() {
		session, err := scanSleep(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list sleeps: %w", err)
	}
	return out, nil
}

// StartSleep opens a sleep session at now.
func (s *Store) StartSleep(ctx context.Context, now time.Time) (sleep.Session, error) {
	now = normalizeTime(now)
	var out sleep.Session

	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		var active int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(1) FROM sleeps WHERE account_id = ? AND ended_at IS NULL AND deleted_at IS NULL`,
			AccountID(ctx),
		).Scan(&active); err != nil {
			return fmt.Errorf("store: check active sleep: %w", err)
		}
		if active > 0 {
			return sleep.ErrAlreadyActive
		}
		overlap, err := s.sleepOverlap(ctx, tx, 0, now, nil, now)
		if err != nil {
			return err
		}
		if overlap {
			return sleep.ErrOverlap
		}

		res, err := tx.ExecContext(ctx,
			`INSERT INTO sleeps (account_id, started_at, created_at) VALUES (?, ?, ?)`,
			AccountID(ctx), encodeTime(now), encodeTime(now))
		if err != nil {
			if isUniqueViolation(err) {
				return sleep.ErrAlreadyActive
			}
			return fmt.Errorf("store: insert sleep: %w", err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("store: sleep id: %w", err)
		}
		out = sleep.Session{ID: id, StartedAt: now, CreatedAt: now}
		return nil
	})
	return out, err
}

// CreateSleep logs a completed sleep retroactively.
func (s *Store) CreateSleep(ctx context.Context, start, end time.Time, notes string, now time.Time) (sleep.Session, error) {
	now = normalizeTime(now)
	start = normalizeTime(start)
	end = normalizeTime(end)
	if err := sleep.ValidateTimes(start, &end, now, true); err != nil {
		return sleep.Session{}, err
	}
	if err := sleep.ValidateNotes(notes); err != nil {
		return sleep.Session{}, err
	}

	var out sleep.Session
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		overlap, err := s.sleepOverlap(ctx, tx, 0, start, &end, now)
		if err != nil {
			return err
		}
		if overlap {
			return sleep.ErrOverlap
		}

		res, err := tx.ExecContext(ctx,
			`INSERT INTO sleeps (account_id, started_at, ended_at, notes, created_at) VALUES (?, ?, ?, ?, ?)`,
			AccountID(ctx), encodeTime(start), encodeTime(end), notes, encodeTime(now))
		if err != nil {
			return fmt.Errorf("store: insert completed sleep: %w", err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("store: sleep id: %w", err)
		}
		out = sleep.Session{
			ID:        id,
			StartedAt: start,
			EndedAt:   timePtr(end),
			Notes:     notes,
			CreatedAt: now,
		}
		return nil
	})
	return out, err
}

// StopSleep closes the active session at now.
func (s *Store) StopSleep(ctx context.Context, id int64, now time.Time) (sleep.Session, error) {
	now = normalizeTime(now)
	var out sleep.Session

	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		session, err := s.getSleepAt(ctx, tx, id, false)
		if err != nil {
			return err
		}
		if session.EndedAt != nil {
			return sleep.ErrNoActiveSleep
		}
		end := now
		if end.Before(session.StartedAt) {
			end = session.StartedAt
		}
		res, err := tx.ExecContext(ctx,
			`UPDATE sleeps SET ended_at = ? WHERE id = ? AND account_id = ? AND ended_at IS NULL`,
			encodeTime(end), id, AccountID(ctx))
		if err != nil {
			return fmt.Errorf("store: stop sleep: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("store: stop sleep rows: %w", err)
		}
		if n != 1 {
			return sleep.ErrNoActiveSleep
		}
		session.EndedAt = timePtr(end)
		out = session
		return nil
	})
	return out, err
}

// UpdateSleep corrects the times or notes of a sleep. A nil edit.EndedAt
// keeps an active session open and is rejected for a completed one.
func (s *Store) UpdateSleep(ctx context.Context, id int64, edit sleep.Edit, now time.Time) (sleep.Session, error) {
	now = normalizeTime(now)
	edit.StartedAt = normalizeTime(edit.StartedAt)
	if edit.EndedAt != nil {
		end := normalizeTime(*edit.EndedAt)
		edit.EndedAt = &end
	}

	var out sleep.Session
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		session, err := s.getSleepAt(ctx, tx, id, false)
		if err != nil {
			return err
		}
		completed := session.EndedAt != nil
		if err := sleep.ValidateTimes(edit.StartedAt, edit.EndedAt, now, completed); err != nil {
			return err
		}
		if err := sleep.ValidateNotes(edit.Notes); err != nil {
			return err
		}
		overlap, err := s.sleepOverlap(ctx, tx, id, edit.StartedAt, edit.EndedAt, now)
		if err != nil {
			return err
		}
		if overlap {
			return sleep.ErrOverlap
		}

		var endValue any
		if edit.EndedAt != nil {
			endValue = encodeTime(*edit.EndedAt)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE sleeps SET started_at = ?, ended_at = ?, notes = ? WHERE id = ? AND account_id = ? AND deleted_at IS NULL`,
			encodeTime(edit.StartedAt), endValue, edit.Notes, id, AccountID(ctx)); err != nil {
			return fmt.Errorf("store: update sleep: %w", err)
		}
		session.StartedAt = edit.StartedAt
		session.EndedAt = edit.EndedAt
		session.Notes = edit.Notes
		out = session
		return nil
	})
	return out, err
}

// DeleteSleep soft-deletes a sleep. An active session is closed at now first,
// in the same transaction.
func (s *Store) DeleteSleep(ctx context.Context, id int64, now time.Time) error {
	now = normalizeTime(now)
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		session, err := s.getSleepAt(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if session.DeletedAt != nil {
			return sleep.ErrAlreadyDeleted
		}
		if session.EndedAt == nil {
			end := now
			if end.Before(session.StartedAt) {
				end = session.StartedAt
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE sleeps SET ended_at = ? WHERE id = ? AND account_id = ? AND ended_at IS NULL`,
				encodeTime(end), id, AccountID(ctx)); err != nil {
				return fmt.Errorf("store: close sleep before delete: %w", err)
			}
		}
		res, err := tx.ExecContext(ctx,
			`UPDATE sleeps SET deleted_at = ? WHERE id = ? AND account_id = ? AND deleted_at IS NULL`,
			encodeTime(now), id, AccountID(ctx))
		if err != nil {
			return fmt.Errorf("store: soft delete sleep: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("store: soft delete rows: %w", err)
		}
		if n != 1 {
			return sleep.ErrAlreadyDeleted
		}
		return nil
	})
}

// RestoreSleep clears deleted_at after checking that the restored interval
// still fits between the other sessions.
func (s *Store) RestoreSleep(ctx context.Context, id int64, now time.Time) (sleep.Session, bool, error) {
	now = normalizeTime(now)
	var (
		out      sleep.Session
		restored bool
	)
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		session, err := s.getSleepAt(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if session.DeletedAt == nil {
			out = session
			return nil
		}
		overlap, err := s.sleepOverlap(ctx, tx, id, session.StartedAt, session.EndedAt, now)
		if err != nil {
			return err
		}
		if overlap {
			return sleep.ErrOverlap
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE sleeps SET deleted_at = NULL WHERE id = ? AND account_id = ?`, id, AccountID(ctx)); err != nil {
			if isUniqueViolation(err) {
				return sleep.ErrAlreadyActive
			}
			return fmt.Errorf("store: restore sleep: %w", err)
		}
		session.DeletedAt = nil
		out = session
		restored = true
		return nil
	})
	if err != nil {
		return sleep.Session{}, false, err
	}
	return out, restored, nil
}

func (s *Store) getSleepAt(ctx context.Context, q queryer, id int64, includeDeleted bool) (sleep.Session, error) {
	query := `SELECT ` + sleepColumns + ` FROM sleeps WHERE id = ? AND account_id = ?`
	args := []any{id, AccountID(ctx)}
	if !includeDeleted {
		query += ` AND deleted_at IS NULL`
	}
	session, err := scanSleep(q.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return sleep.Session{}, sleep.ErrNotFound
	}
	if err != nil {
		return sleep.Session{}, err
	}
	return session, nil
}

// sleepOverlap reports whether [start, end) overlaps another live sleep.
// An open candidate ends at now.
func (s *Store) sleepOverlap(ctx context.Context, q queryer, excludeID int64, start time.Time, end *time.Time, now time.Time) (bool, error) {
	effectiveEnd := now
	if end != nil {
		effectiveEnd = *end
	}
	var count int
	if err := q.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM sleeps
		WHERE account_id = ?
		  AND deleted_at IS NULL
		  AND id != ?
		  AND started_at < ?
		  AND COALESCE(ended_at, ?) > ?`,
		AccountID(ctx), excludeID, encodeTime(effectiveEnd), encodeTime(now), encodeTime(start),
	).Scan(&count); err != nil {
		return false, fmt.Errorf("store: check sleep overlap: %w", err)
	}
	return count > 0, nil
}

func scanSleep(s scanner) (sleep.Session, error) {
	var (
		session sleep.Session
		started string
		ended   sql.NullString
		notes   string
		created string
		deleted sql.NullString
	)
	if err := s.Scan(&session.ID, &started, &ended, &notes, &created, &deleted); err != nil {
		return sleep.Session{}, err
	}
	var err error
	if session.StartedAt, err = decodeTime(started); err != nil {
		return sleep.Session{}, err
	}
	if session.CreatedAt, err = decodeTime(created); err != nil {
		return sleep.Session{}, err
	}
	if session.EndedAt, err = decodeNullTime(ended); err != nil {
		return sleep.Session{}, err
	}
	if session.DeletedAt, err = decodeNullTime(deleted); err != nil {
		return sleep.Session{}, err
	}
	session.Notes = notes
	return session, nil
}
