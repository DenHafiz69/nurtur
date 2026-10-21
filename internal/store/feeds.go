package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"nurtur/internal/feed"
)

const feedColumns = `id, started_at, ended_at, COALESCE(last_side, ''), COALESCE(notes, ''), created_at, deleted_at`

const segmentColumns = `id, feed_id, side, started_at, ended_at`

// ActiveFeed returns the running feed with its segments, or nil when idle.
func (s *Store) ActiveFeed(ctx context.Context) (*feed.Feed, error) {
	f, err := s.loadFeedAt(ctx, s.db, 0, false, true)
	if errors.Is(err, feed.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return f, nil
}

// GetFeed returns a non-deleted feed by id.
func (s *Store) GetFeed(ctx context.Context, id int64) (*feed.Feed, error) {
	return s.loadFeedAt(ctx, s.db, id, false, false)
}

// LastFeed returns the most recent non-deleted feed that has at least one
// segment. It may be the active feed; callers decide what to show.
func (s *Store) LastFeed(ctx context.Context) (*feed.Feed, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+feedColumns+`
		FROM feeds f
		WHERE f.account_id = ?
		  AND f.deleted_at IS NULL
		  AND EXISTS (SELECT 1 FROM feed_segments s WHERE s.feed_id = f.id)
		ORDER BY COALESCE(f.ended_at, f.started_at) DESC, f.id DESC
		LIMIT 1`, AccountID(ctx))
	f, err := scanFeed(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := s.attachSegments(ctx, s.db, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// RecentFeeds lists feeds that started in [from, to), plus any active feed, in
// reverse chronological order. Segments are loaded in one follow-up query.
func (s *Store) RecentFeeds(ctx context.Context, from, to time.Time, limit int) ([]feed.Feed, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+feedColumns+`
		FROM feeds
		WHERE account_id = ?
		  AND deleted_at IS NULL
		  AND ((started_at >= ? AND started_at < ?) OR ended_at IS NULL)
		ORDER BY started_at DESC, id DESC
		LIMIT ?`,
		AccountID(ctx), encodeTime(from), encodeTime(to), limit)
	if err != nil {
		return nil, fmt.Errorf("store: list feeds: %w", err)
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
		return nil, fmt.Errorf("store: list feeds: %w", err)
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

// CreateFeed starts a feed on side and opens its first segment. The partial
// unique index guarantees at most one active feed even if two requests race.
func (s *Store) CreateFeed(ctx context.Context, side feed.Side, now time.Time) (feed.Feed, error) {
	if !side.Valid() {
		return feed.Feed{}, feed.ErrInvalidSide
	}
	now = normalizeTime(now)
	account := AccountID(ctx)
	var out feed.Feed

	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		var active int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(1) FROM feeds WHERE account_id = ? AND ended_at IS NULL AND deleted_at IS NULL`,
			account,
		).Scan(&active); err != nil {
			return fmt.Errorf("store: check active feed: %w", err)
		}
		if active > 0 {
			return feed.ErrAlreadyActive
		}

		res, err := tx.ExecContext(ctx,
			`INSERT INTO feeds (account_id, started_at, created_at) VALUES (?, ?, ?)`,
			account, encodeTime(now), encodeTime(now))
		if err != nil {
			if isUniqueViolation(err) {
				return feed.ErrAlreadyActive
			}
			return fmt.Errorf("store: insert feed: %w", err)
		}
		feedID, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("store: feed id: %w", err)
		}

		res, err = tx.ExecContext(ctx,
			`INSERT INTO feed_segments (account_id, feed_id, side, started_at) VALUES (?, ?, ?, ?)`,
			account, feedID, side, encodeTime(now))
		if err != nil {
			if isUniqueViolation(err) {
				return feed.ErrAlreadyActive
			}
			return fmt.Errorf("store: insert segment: %w", err)
		}
		segmentID, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("store: segment id: %w", err)
		}

		out = feed.Feed{
			ID:        feedID,
			StartedAt: now,
			CreatedAt: now,
			Segments: []feed.Segment{{
				ID:        segmentID,
				FeedID:    feedID,
				Side:      side,
				StartedAt: now,
			}},
		}
		return nil
	})
	return out, err
}

// SwitchFeed closes the open segment and opens one on the requested side.
// Tapping the already-active side is an explicit no-op error.
func (s *Store) SwitchFeed(ctx context.Context, id int64, side feed.Side, now time.Time) (feed.Feed, error) {
	if !side.Valid() {
		return feed.Feed{}, feed.ErrInvalidSide
	}
	now = normalizeTime(now)
	var out feed.Feed

	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		f, err := s.loadFeedAt(ctx, tx, id, true, false)
		if err != nil {
			return err
		}
		if f.DeletedAt != nil {
			return feed.ErrAlreadyDeleted
		}
		if f.EndedAt != nil {
			return feed.ErrNoActiveFeed
		}
		current, ok := f.CurrentSegment()
		if !ok {
			return feed.ErrNoActiveFeed
		}
		if current.Side == side {
			return feed.ErrSameSide
		}

		end := now
		if end.Before(current.StartedAt) {
			end = current.StartedAt
		}
		res, err := tx.ExecContext(ctx,
			`UPDATE feed_segments SET ended_at = ? WHERE id = ? AND account_id = ? AND ended_at IS NULL`,
			encodeTime(end), current.ID, AccountID(ctx))
		if err != nil {
			return fmt.Errorf("store: close segment: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("store: close segment rows: %w", err)
		}
		if n != 1 {
			return feed.ErrNoActiveFeed
		}

		res, err = tx.ExecContext(ctx,
			`INSERT INTO feed_segments (account_id, feed_id, side, started_at) VALUES (?, ?, ?, ?)`,
			AccountID(ctx), id, side, encodeTime(end))
		if err != nil {
			if isUniqueViolation(err) {
				return feed.ErrAlreadyActive
			}
			return fmt.Errorf("store: open segment: %w", err)
		}
		newID, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("store: segment id: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE feeds SET last_side = ? WHERE id = ? AND account_id = ?`, side, id, AccountID(ctx)); err != nil {
			return fmt.Errorf("store: update last side: %w", err)
		}

		for i := range f.Segments {
			if f.Segments[i].ID == current.ID {
				f.Segments[i].EndedAt = timePtr(end)
			}
		}
		f.Segments = append(f.Segments, feed.Segment{
			ID:        newID,
			FeedID:    id,
			Side:      side,
			StartedAt: end,
		})
		f.LastSide = side
		out = *f
		return nil
	})
	return out, err
}

// StopFeed closes the open segment and the feed.
func (s *Store) StopFeed(ctx context.Context, id int64, now time.Time) (feed.Feed, error) {
	now = normalizeTime(now)
	var out feed.Feed

	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		f, err := s.loadFeedAt(ctx, tx, id, true, false)
		if err != nil {
			return err
		}
		if f.DeletedAt != nil {
			return feed.ErrAlreadyDeleted
		}
		if f.EndedAt != nil {
			return feed.ErrNoActiveFeed
		}

		end := now
		current, ok := f.CurrentSegment()
		if ok {
			if end.Before(current.StartedAt) {
				end = current.StartedAt
			}
			res, err := tx.ExecContext(ctx,
				`UPDATE feed_segments SET ended_at = ? WHERE id = ? AND account_id = ? AND ended_at IS NULL`,
				encodeTime(end), current.ID, AccountID(ctx))
			if err != nil {
				return fmt.Errorf("store: close segment: %w", err)
			}
			n, err := res.RowsAffected()
			if err != nil {
				return fmt.Errorf("store: close segment rows: %w", err)
			}
			if n != 1 {
				return feed.ErrNoActiveFeed
			}
			for i := range f.Segments {
				if f.Segments[i].ID == current.ID {
					f.Segments[i].EndedAt = timePtr(end)
				}
			}
			f.LastSide = current.Side
		} else {
			f.LastSide = f.MostRecentSide()
		}

		res, err := tx.ExecContext(ctx,
			`UPDATE feeds SET ended_at = ?, last_side = ? WHERE id = ? AND account_id = ? AND ended_at IS NULL`,
			encodeTime(end), f.LastSide, id, AccountID(ctx))
		if err != nil {
			return fmt.Errorf("store: stop feed: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("store: stop feed rows: %w", err)
		}
		if n != 1 {
			return feed.ErrNoActiveFeed
		}

		f.EndedAt = timePtr(end)
		out = *f
		return nil
	})
	return out, err
}

// DeleteFeed soft-deletes a feed. An active feed is closed at now first, in the
// same transaction, so the timer can never point at a deleted row.
func (s *Store) DeleteFeed(ctx context.Context, id int64, now time.Time) error {
	now = normalizeTime(now)
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		f, err := s.loadFeedAt(ctx, tx, id, true, false)
		if err != nil {
			return err
		}
		if f.DeletedAt != nil {
			return feed.ErrAlreadyDeleted
		}

		if f.EndedAt == nil {
			end := now
			if current, ok := f.CurrentSegment(); ok {
				if end.Before(current.StartedAt) {
					end = current.StartedAt
				}
				if _, err := tx.ExecContext(ctx,
					`UPDATE feed_segments SET ended_at = ? WHERE id = ? AND account_id = ? AND ended_at IS NULL`,
					encodeTime(end), current.ID, AccountID(ctx)); err != nil {
					return fmt.Errorf("store: close segment before delete: %w", err)
				}
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE feeds SET ended_at = ?, last_side = ? WHERE id = ? AND account_id = ? AND ended_at IS NULL`,
				encodeTime(end), f.MostRecentSide(), id, AccountID(ctx)); err != nil {
				return fmt.Errorf("store: close feed before delete: %w", err)
			}
		}

		res, err := tx.ExecContext(ctx,
			`UPDATE feeds SET deleted_at = ? WHERE id = ? AND account_id = ? AND deleted_at IS NULL`,
			encodeTime(now), id, AccountID(ctx))
		if err != nil {
			return fmt.Errorf("store: soft delete feed: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("store: soft delete rows: %w", err)
		}
		if n != 1 {
			return feed.ErrAlreadyDeleted
		}
		return nil
	})
}

// UpdateCompletedFeed rebuilds a finished feed's segments from submitted side
// totals. The feed must not be active.
func (s *Store) UpdateCompletedFeed(ctx context.Context, id int64, edit feed.CompletedEdit) (feed.Feed, error) {
	edit.StartedAt = normalizeTime(edit.StartedAt)
	edit.EndedAt = normalizeTime(edit.EndedAt)
	if err := edit.Validate(); err != nil {
		return feed.Feed{}, err
	}
	segments := feed.BuildSegments(id, edit)
	var out feed.Feed

	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		f, err := s.loadFeedAt(ctx, tx, id, true, false)
		if err != nil {
			return err
		}
		if f.DeletedAt != nil {
			return feed.ErrAlreadyDeleted
		}
		if f.EndedAt == nil {
			return feed.ErrFeedActive
		}

		if _, err := tx.ExecContext(ctx,
			`DELETE FROM feed_segments WHERE feed_id = ? AND account_id = ?`,
			id, AccountID(ctx)); err != nil {
			return fmt.Errorf("store: clear segments: %w", err)
		}
		for i := range segments {
			res, err := tx.ExecContext(ctx,
				`INSERT INTO feed_segments (account_id, feed_id, side, started_at, ended_at) VALUES (?, ?, ?, ?, ?)`,
				AccountID(ctx), id, segments[i].Side, encodeTime(segments[i].StartedAt), encodeTime(*segments[i].EndedAt))
			if err != nil {
				return fmt.Errorf("store: insert rebuilt segment: %w", err)
			}
			segments[i].ID, err = res.LastInsertId()
			if err != nil {
				return fmt.Errorf("store: rebuilt segment id: %w", err)
			}
		}

		if _, err := tx.ExecContext(ctx,
			`UPDATE feeds SET started_at = ?, ended_at = ?, last_side = ? WHERE id = ? AND account_id = ?`,
			encodeTime(edit.StartedAt), encodeTime(edit.EndedAt), edit.LastSide(), id, AccountID(ctx)); err != nil {
			return fmt.Errorf("store: update feed times: %w", err)
		}

		f.StartedAt = edit.StartedAt
		f.EndedAt = timePtr(edit.EndedAt)
		f.LastSide = edit.LastSide()
		f.Segments = segments
		out = *f
		return nil
	})
	return out, err
}

// AdjustActiveSegmentStart moves the open segment's start by delta, clamping so
// it stays between the previous segment and now.
func (s *Store) AdjustActiveSegmentStart(ctx context.Context, id int64, delta time.Duration, now time.Time) (feed.Feed, error) {
	return s.updateActiveStart(ctx, id, time.Time{}, delta, now, true)
}

// SetActiveSegmentStart sets the open segment's start from an explicit time.
func (s *Store) SetActiveSegmentStart(ctx context.Context, id int64, start, now time.Time) (feed.Feed, error) {
	return s.updateActiveStart(ctx, id, start, 0, now, false)
}

func (s *Store) updateActiveStart(ctx context.Context, id int64, start time.Time, delta time.Duration, now time.Time, clamp bool) (feed.Feed, error) {
	now = normalizeTime(now)
	start = normalizeTime(start)
	var out feed.Feed

	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		f, err := s.loadFeedAt(ctx, tx, id, true, false)
		if err != nil {
			return err
		}
		if f.DeletedAt != nil {
			return feed.ErrAlreadyDeleted
		}
		if f.EndedAt != nil {
			return feed.ErrNoActiveFeed
		}
		current, ok := f.CurrentSegment()
		if !ok {
			return feed.ErrNoActiveFeed
		}
		index := segmentIndex(f, current.ID)
		if index < 0 {
			return feed.ErrNoSegments
		}

		proposed := start
		if clamp {
			proposed = current.StartedAt.Add(delta)
			if proposed.After(now) {
				proposed = now
			}
			if index > 0 && f.Segments[index-1].EndedAt != nil && proposed.Before(*f.Segments[index-1].EndedAt) {
				proposed = *f.Segments[index-1].EndedAt
			}
			if proposed.After(now) {
				proposed = now
			}
		} else {
			if proposed.After(now) {
				return &feed.ValidationError{
					Field:   "segment_started_at",
					Message: "That start is in the future. Try again?",
				}
			}
			if index > 0 && f.Segments[index-1].EndedAt != nil && proposed.Before(*f.Segments[index-1].EndedAt) {
				return &feed.ValidationError{
					Field:   "segment_started_at",
					Message: "That's before the previous side ended. Try again?",
				}
			}
		}
		proposed = normalizeTime(proposed)

		if _, err := tx.ExecContext(ctx,
			`UPDATE feed_segments SET started_at = ? WHERE id = ? AND account_id = ? AND ended_at IS NULL`,
			encodeTime(proposed), current.ID, AccountID(ctx)); err != nil {
			return fmt.Errorf("store: adjust segment start: %w", err)
		}
		if index == 0 {
			if _, err := tx.ExecContext(ctx,
				`UPDATE feeds SET started_at = ? WHERE id = ? AND account_id = ?`,
				encodeTime(proposed), id, AccountID(ctx)); err != nil {
				return fmt.Errorf("store: adjust feed start: %w", err)
			}
			f.StartedAt = proposed
		}

		f.Segments[index].StartedAt = proposed
		out = *f
		return nil
	})
	return out, err
}

// TotalsForFeed derives per-side totals in SQL. It is used for the live split
// shown while a feed is running.
func (s *Store) TotalsForFeed(ctx context.Context, id int64, now time.Time) (left, right time.Duration, err error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT side,
		       COALESCE(SUM(MAX(0, strftime('%s', COALESCE(ended_at, ?)) - strftime('%s', started_at))), 0)
		FROM feed_segments
		WHERE feed_id = ? AND account_id = ?
		GROUP BY side`,
		encodeTime(now), id, AccountID(ctx))
	if err != nil {
		return 0, 0, fmt.Errorf("store: feed totals: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			side    string
			seconds int64
		)
		if err := rows.Scan(&side, &seconds); err != nil {
			return 0, 0, fmt.Errorf("store: scan feed totals: %w", err)
		}
		d := time.Duration(seconds) * time.Second
		switch feed.Side(side) {
		case feed.SideLeft:
			left = d
		case feed.SideRight:
			right = d
		}
	}
	if err := rows.Err(); err != nil {
		return 0, 0, fmt.Errorf("store: feed totals: %w", err)
	}
	return left, right, nil
}

// loadFeedAt loads a feed by id (or the active feed when id is zero).
func (s *Store) loadFeedAt(ctx context.Context, q queryer, id int64, includeDeleted, activeOnly bool) (*feed.Feed, error) {
	var (
		query string
		args  []any
	)
	switch {
	case activeOnly:
		query = `SELECT ` + feedColumns + ` FROM feeds WHERE account_id = ? AND ended_at IS NULL AND deleted_at IS NULL ORDER BY id DESC LIMIT 1`
		args = []any{AccountID(ctx)}
	case includeDeleted:
		query = `SELECT ` + feedColumns + ` FROM feeds WHERE id = ? AND account_id = ?`
		args = []any{id, AccountID(ctx)}
	default:
		query = `SELECT ` + feedColumns + ` FROM feeds WHERE id = ? AND account_id = ? AND deleted_at IS NULL`
		args = []any{id, AccountID(ctx)}
	}

	f, err := scanFeed(q.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, feed.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := s.attachSegments(ctx, q, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

func (s *Store) attachSegments(ctx context.Context, q queryer, f *feed.Feed) error {
	segs, err := s.segmentsForFeed(ctx, q, f.ID)
	if err != nil {
		return err
	}
	f.Segments = segs
	return nil
}

func (s *Store) segmentsForFeed(ctx context.Context, q queryer, feedID int64) ([]feed.Segment, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT `+segmentColumns+` FROM feed_segments WHERE feed_id = ? AND account_id = ? ORDER BY started_at ASC, id ASC`,
		feedID, AccountID(ctx))
	if err != nil {
		return nil, fmt.Errorf("store: list segments: %w", err)
	}
	defer rows.Close()

	var segs []feed.Segment
	for rows.Next() {
		seg, err := scanSegment(rows)
		if err != nil {
			return nil, err
		}
		segs = append(segs, seg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list segments: %w", err)
	}
	return segs, nil
}

func (s *Store) segmentsForFeeds(ctx context.Context, q queryer, ids []int64) (map[int64][]feed.Segment, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids)+1)
	for _, id := range ids {
		args = append(args, id)
	}
	args = append(args, AccountID(ctx))
	rows, err := q.QueryContext(ctx,
		`SELECT `+segmentColumns+` FROM feed_segments WHERE feed_id IN (`+placeholders+`) AND account_id = ? ORDER BY started_at ASC, id ASC`,
		args...)
	if err != nil {
		return nil, fmt.Errorf("store: list segments: %w", err)
	}
	defer rows.Close()

	out := make(map[int64][]feed.Segment, len(ids))
	for rows.Next() {
		seg, err := scanSegment(rows)
		if err != nil {
			return nil, err
		}
		out[seg.FeedID] = append(out[seg.FeedID], seg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list segments: %w", err)
	}
	return out, nil
}

func scanFeed(s scanner) (feed.Feed, error) {
	var (
		f        feed.Feed
		started  string
		ended    sql.NullString
		lastSide string
		notes    string
		created  string
		deleted  sql.NullString
	)
	if err := s.Scan(&f.ID, &started, &ended, &lastSide, &notes, &created, &deleted); err != nil {
		return feed.Feed{}, err
	}
	var err error
	if f.StartedAt, err = decodeTime(started); err != nil {
		return feed.Feed{}, err
	}
	if f.CreatedAt, err = decodeTime(created); err != nil {
		return feed.Feed{}, err
	}
	if f.EndedAt, err = decodeNullTime(ended); err != nil {
		return feed.Feed{}, err
	}
	if f.DeletedAt, err = decodeNullTime(deleted); err != nil {
		return feed.Feed{}, err
	}
	f.LastSide = feed.Side(lastSide)
	f.Notes = notes
	return f, nil
}

func scanSegment(s scanner) (feed.Segment, error) {
	var (
		seg     feed.Segment
		side    string
		started string
		ended   sql.NullString
	)
	if err := s.Scan(&seg.ID, &seg.FeedID, &side, &started, &ended); err != nil {
		return feed.Segment{}, err
	}
	var err error
	if seg.StartedAt, err = decodeTime(started); err != nil {
		return feed.Segment{}, err
	}
	if seg.EndedAt, err = decodeNullTime(ended); err != nil {
		return feed.Segment{}, err
	}
	seg.Side = feed.Side(side)
	return seg, nil
}

func segmentIndex(f *feed.Feed, segmentID int64) int {
	for i := range f.Segments {
		if f.Segments[i].ID == segmentID {
			return i
		}
	}
	return -1
}

func timePtr(t time.Time) *time.Time { return &t }
