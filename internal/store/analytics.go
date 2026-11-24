package store

import (
	"context"
	"fmt"
	"time"

	"nurtur/internal/diaper"
	"nurtur/internal/feed"
	"nurtur/internal/growth"
	"nurtur/internal/intake"
	"nurtur/internal/sleep"
)

// FeedsInRange returns all live feeds started in [from, to), oldest first,
// with their nursing segments hydrated.
func (s *Store) FeedsInRange(ctx context.Context, from, to time.Time) ([]feed.Feed, error) {
	if !to.After(from) {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+feedColumns+`
		FROM feeds
		WHERE account_id = ?
		  AND deleted_at IS NULL
		  AND started_at >= ?
		  AND started_at < ?
		ORDER BY started_at ASC, id ASC`,
		AccountID(ctx), encodeTime(from), encodeTime(to))
	if err != nil {
		return nil, fmt.Errorf("store: feeds in range: %w", err)
	}
	defer rows.Close()

	var feeds []feed.Feed
	for rows.Next() {
		f, err := scanFeed(rows)
		if err != nil {
			return nil, err
		}
		feeds = append(feeds, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: feeds in range: %w", err)
	}
	if len(feeds) == 0 {
		return feeds, nil
	}

	ids := make([]int64, len(feeds))
	index := make(map[int64]int, len(feeds))
	for i := range feeds {
		ids[i] = feeds[i].ID
		index[feeds[i].ID] = i
	}
	segments, err := s.segmentsForFeeds(ctx, s.db, ids)
	if err != nil {
		return nil, err
	}
	for feedID, segs := range segments {
		if i, ok := index[feedID]; ok {
			feeds[i].Segments = segs
		}
	}
	return feeds, nil
}

// DiapersInRange returns all live diapers logged in [from, to), oldest first.
func (s *Store) DiapersInRange(ctx context.Context, from, to time.Time) ([]diaper.Diaper, error) {
	if !to.After(from) {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+diaperColumns+`
		FROM diapers
		WHERE account_id = ?
		  AND deleted_at IS NULL
		  AND logged_at >= ?
		  AND logged_at < ?
		ORDER BY logged_at ASC, id ASC`,
		AccountID(ctx), encodeTime(from), encodeTime(to))
	if err != nil {
		return nil, fmt.Errorf("store: diapers in range: %w", err)
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
	return out, rows.Err()
}

// SleepsInRange returns all live sleep sessions overlapping [from, to), oldest first.
func (s *Store) SleepsInRange(ctx context.Context, from, to, now time.Time) ([]sleep.Session, error) {
	if !to.After(from) {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+sleepColumns+`
		FROM sleeps
		WHERE account_id = ?
		  AND deleted_at IS NULL
		  AND started_at < ?
		  AND COALESCE(ended_at, ?) > ?
		ORDER BY started_at ASC, id ASC`,
		AccountID(ctx), encodeTime(to), encodeTime(now), encodeTime(from))
	if err != nil {
		return nil, fmt.Errorf("store: sleeps in range: %w", err)
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
	return out, rows.Err()
}

// BottlesInRange returns all live bottle feeds logged in [from, to), oldest first.
func (s *Store) BottlesInRange(ctx context.Context, from, to time.Time) ([]intake.Bottle, error) {
	if !to.After(from) {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+bottleColumns+`
		FROM bottles
		WHERE account_id = ?
		  AND deleted_at IS NULL
		  AND logged_at >= ?
		  AND logged_at < ?
		ORDER BY logged_at ASC, id ASC`,
		AccountID(ctx), encodeTime(from), encodeTime(to))
	if err != nil {
		return nil, fmt.Errorf("store: bottles in range: %w", err)
	}
	defer rows.Close()

	var out []intake.Bottle
	for rows.Next() {
		b, err := scanBottle(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// PumpsInRange returns all live pump sessions logged in [from, to), oldest first.
func (s *Store) PumpsInRange(ctx context.Context, from, to time.Time) ([]intake.Pump, error) {
	if !to.After(from) {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+pumpColumns+`
		FROM pumps
		WHERE account_id = ?
		  AND deleted_at IS NULL
		  AND logged_at >= ?
		  AND logged_at < ?
		ORDER BY logged_at ASC, id ASC`,
		AccountID(ctx), encodeTime(from), encodeTime(to))
	if err != nil {
		return nil, fmt.Errorf("store: pumps in range: %w", err)
	}
	defer rows.Close()

	var out []intake.Pump
	for rows.Next() {
		p, err := scanPump(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SolidsInRange returns all live solid meals logged in [from, to), oldest first.
func (s *Store) SolidsInRange(ctx context.Context, from, to time.Time) ([]intake.Solid, error) {
	if !to.After(from) {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+solidColumns+`
		FROM solids
		WHERE account_id = ?
		  AND deleted_at IS NULL
		  AND logged_at >= ?
		  AND logged_at < ?
		ORDER BY logged_at ASC, id ASC`,
		AccountID(ctx), encodeTime(from), encodeTime(to))
	if err != nil {
		return nil, fmt.Errorf("store: solids in range: %w", err)
	}
	defer rows.Close()

	var out []intake.Solid
	for rows.Next() {
		sol, err := scanSolid(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sol)
	}
	return out, rows.Err()
}

// LiveGrowth returns all non-deleted growth measurements for the current account, oldest first.
func (s *Store) LiveGrowth(ctx context.Context) ([]growth.Measurement, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+growthColumns+`
		FROM growth
		WHERE account_id = ? AND deleted_at IS NULL
		ORDER BY logged_at ASC, id ASC`,
		AccountID(ctx))
	if err != nil {
		return nil, fmt.Errorf("store: live growth: %w", err)
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
