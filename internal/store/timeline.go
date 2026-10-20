package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"nurtur/internal/diaper"
	"nurtur/internal/feed"
	"nurtur/internal/sleep"
	"nurtur/internal/timeline"
)

// timelineLimit bounds a single day's event load. A day this busy would mean
// roughly one event every three minutes, which is far beyond a realistic day.
const timelineLimit = 500

// TimelineBetween returns every live event whose anchor timestamp falls in
// [from, to), newest first. Ordering is stable: anchor DESC, id DESC, kind DESC.
func (s *Store) TimelineBetween(ctx context.Context, from, to time.Time, limit int) ([]timeline.Entry, error) {
	if !to.After(from) {
		return nil, nil
	}
	if limit <= 0 || limit > timelineLimit {
		limit = timelineLimit
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT kind, id, anchor FROM (
			SELECT 'feed' AS kind, id, started_at AS anchor
			  FROM feeds WHERE deleted_at IS NULL AND started_at >= ? AND started_at < ?
			UNION ALL
			SELECT 'diaper', id, logged_at
			  FROM diapers WHERE deleted_at IS NULL AND logged_at >= ? AND logged_at < ?
			UNION ALL
			SELECT 'sleep', id, started_at
			  FROM sleeps WHERE deleted_at IS NULL AND started_at >= ? AND started_at < ?
		)
		ORDER BY anchor DESC, id DESC, kind DESC
		LIMIT ?`,
		encodeTime(from), encodeTime(to),
		encodeTime(from), encodeTime(to),
		encodeTime(from), encodeTime(to),
		limit)
	if err != nil {
		return nil, fmt.Errorf("store: timeline anchors: %w", err)
	}
	defer rows.Close()

	type anchor struct {
		kind   timeline.Kind
		id     int64
		anchor time.Time
	}
	var anchors []anchor
	var feedIDs, diaperIDs, sleepIDs []int64
	for rows.Next() {
		var (
			kindRaw string
			id      int64
			raw     string
		)
		if err := rows.Scan(&kindRaw, &id, &raw); err != nil {
			return nil, fmt.Errorf("store: scan timeline anchor: %w", err)
		}
		when, err := decodeTime(raw)
		if err != nil {
			return nil, err
		}
		kind := timeline.Kind(kindRaw)
		anchors = append(anchors, anchor{kind: kind, id: id, anchor: when})
		switch kind {
		case timeline.Feed:
			feedIDs = append(feedIDs, id)
		case timeline.Diaper:
			diaperIDs = append(diaperIDs, id)
		case timeline.Sleep:
			sleepIDs = append(sleepIDs, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: timeline anchors: %w", err)
	}
	if len(anchors) == 0 {
		return nil, nil
	}

	feeds, err := s.feedsByIDs(ctx, feedIDs)
	if err != nil {
		return nil, err
	}
	diapers, err := s.diapersByIDs(ctx, diaperIDs)
	if err != nil {
		return nil, err
	}
	sleeps, err := s.sleepsByIDs(ctx, sleepIDs)
	if err != nil {
		return nil, err
	}

	out := make([]timeline.Entry, 0, len(anchors))
	for _, a := range anchors {
		entry := timeline.Entry{Kind: a.kind, ID: a.id, Anchor: a.anchor}
		switch a.kind {
		case timeline.Feed:
			if f, ok := feeds[a.id]; ok {
				entry.Feed = f
			}
		case timeline.Diaper:
			if d, ok := diapers[a.id]; ok {
				entry.Diaper = d
			}
		case timeline.Sleep:
			if session, ok := sleeps[a.id]; ok {
				entry.Sleep = session
			}
		}
		// A row can disappear between the anchor query and hydration (sync pull,
		// for example). Skipping it keeps the response honest.
		if entry.Feed != nil || entry.Diaper != nil || entry.Sleep != nil {
			out = append(out, entry)
		}
	}
	return out, nil
}

// TimelineDaySummary counts feeds and diapers that started/logged in [from, to)
// and sums sleep overlap with that local day.
func (s *Store) TimelineDaySummary(ctx context.Context, from, to, now time.Time) (timeline.DaySummary, error) {
	var summary timeline.DaySummary
	if !to.After(from) {
		return summary, nil
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(1) FROM feeds   WHERE deleted_at IS NULL AND started_at >= ? AND started_at < ?),
			(SELECT COUNT(1) FROM diapers WHERE deleted_at IS NULL AND logged_at  >= ? AND logged_at  < ?)`,
		encodeTime(from), encodeTime(to),
		encodeTime(from), encodeTime(to),
	).Scan(&summary.Feeds, &summary.Diapers); err != nil {
		return timeline.DaySummary{}, fmt.Errorf("store: timeline day counts: %w", err)
	}

	sessions, err := s.SleepsBetween(ctx, from, to, now, timelineLimit)
	if err != nil {
		return timeline.DaySummary{}, err
	}
	for _, session := range sessions {
		start := session.StartedAt
		if start.Before(from) {
			start = from
		}
		end := session.EffectiveEnd(now)
		if end.After(to) {
			end = to
		}
		if end.After(start) {
			summary.Sleep += end.Sub(start)
		}
	}
	return summary, nil
}

// HasTimelineBefore reports whether any live event starts before t. It drives
// the "Load earlier" control without fetching a whole extra page.
func (s *Store) HasTimelineBefore(ctx context.Context, before time.Time) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx, `
		SELECT CASE WHEN
			EXISTS (SELECT 1 FROM feeds   WHERE deleted_at IS NULL AND started_at < ?) OR
			EXISTS (SELECT 1 FROM diapers WHERE deleted_at IS NULL AND logged_at  < ?) OR
			EXISTS (SELECT 1 FROM sleeps  WHERE deleted_at IS NULL AND started_at < ?)
		THEN 1 ELSE 0 END`,
		encodeTime(before), encodeTime(before), encodeTime(before),
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("store: timeline before: %w", err)
	}
	return exists == 1, nil
}

// AllFeeds returns every feed, including soft-deleted rows, oldest first. It is
// used by the logical export.
func (s *Store) AllFeeds(ctx context.Context) ([]feed.Feed, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+feedColumns+` FROM feeds ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("store: export feeds: %w", err)
	}
	defer rows.Close()

	var out []feed.Feed
	var ids []int64
	for rows.Next() {
		f, err := scanFeed(rows)
		if err != nil {
			return nil, err
		}
		ids = append(ids, f.ID)
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: export feeds: %w", err)
	}
	if len(ids) == 0 {
		return out, nil
	}

	segments, err := s.segmentsForFeeds(ctx, s.db, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Segments = segments[out[i].ID]
	}
	return out, nil
}

// AllDiapers returns every diaper, including soft-deleted rows, oldest first.
func (s *Store) AllDiapers(ctx context.Context) ([]diaper.Diaper, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+diaperColumns+` FROM diapers ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("store: export diapers: %w", err)
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
		return nil, fmt.Errorf("store: export diapers: %w", err)
	}
	return out, nil
}

// AllSleeps returns every sleep, including soft-deleted rows, oldest first.
func (s *Store) AllSleeps(ctx context.Context) ([]sleep.Session, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+sleepColumns+` FROM sleeps ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("store: export sleeps: %w", err)
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
		return nil, fmt.Errorf("store: export sleeps: %w", err)
	}
	return out, nil
}

// RestoreFeed clears deleted_at. restored=false means the feed was already
// visible. Restoring an open feed is rejected if another feed is running.
func (s *Store) RestoreFeed(ctx context.Context, id int64) (feed.Feed, bool, error) {
	var (
		out      feed.Feed
		restored bool
	)
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		f, err := s.loadFeedAt(ctx, tx, id, true, false)
		if err != nil {
			return err
		}
		if f.DeletedAt == nil {
			out = *f
			return nil
		}
		if f.EndedAt == nil {
			var active int
			if err := tx.QueryRowContext(ctx, `
				SELECT COUNT(1) FROM feeds
				WHERE ended_at IS NULL AND deleted_at IS NULL AND id != ?`, id).Scan(&active); err != nil {
				return fmt.Errorf("store: check active feed before restore: %w", err)
			}
			if active > 0 {
				return feed.ErrAlreadyActive
			}
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE feeds SET deleted_at = NULL WHERE id = ?`, id); err != nil {
			if isUniqueViolation(err) {
				return feed.ErrAlreadyActive
			}
			return fmt.Errorf("store: restore feed: %w", err)
		}
		f.DeletedAt = nil
		out = *f
		restored = true
		return nil
	})
	if err != nil {
		return feed.Feed{}, false, err
	}
	return out, restored, nil
}

func (s *Store) feedsByIDs(ctx context.Context, ids []int64) (map[int64]*feed.Feed, error) {
	out := make(map[int64]*feed.Feed, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	placeholders, args := placeholdersFor(ids)
	// #nosec G202 -- columns are compile-time constants and placeholders are
	// generated "?" marks; every id is still bound as a query argument.
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+feedColumns+` FROM feeds WHERE deleted_at IS NULL AND id IN (`+placeholders+`)`,
		args...)
	if err != nil {
		return nil, fmt.Errorf("store: timeline feeds: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		f, err := scanFeed(rows)
		if err != nil {
			return nil, err
		}
		item := f
		out[item.ID] = &item
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: timeline feeds: %w", err)
	}
	segments, err := s.segmentsForFeeds(ctx, s.db, ids)
	if err != nil {
		return nil, err
	}
	for id, f := range out {
		f.Segments = segments[id]
	}
	return out, nil
}

func (s *Store) diapersByIDs(ctx context.Context, ids []int64) (map[int64]*diaper.Diaper, error) {
	out := make(map[int64]*diaper.Diaper, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	placeholders, args := placeholdersFor(ids)
	// #nosec G202 -- columns are compile-time constants and placeholders are
	// generated "?" marks; every id is still bound as a query argument.
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+diaperColumns+` FROM diapers WHERE deleted_at IS NULL AND id IN (`+placeholders+`)`,
		args...)
	if err != nil {
		return nil, fmt.Errorf("store: timeline diapers: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		d, err := scanDiaper(rows)
		if err != nil {
			return nil, err
		}
		item := d
		out[item.ID] = &item
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: timeline diapers: %w", err)
	}
	return out, nil
}

func (s *Store) sleepsByIDs(ctx context.Context, ids []int64) (map[int64]*sleep.Session, error) {
	out := make(map[int64]*sleep.Session, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	placeholders, args := placeholdersFor(ids)
	// #nosec G202 -- columns are compile-time constants and placeholders are
	// generated "?" marks; every id is still bound as a query argument.
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+sleepColumns+` FROM sleeps WHERE deleted_at IS NULL AND id IN (`+placeholders+`)`,
		args...)
	if err != nil {
		return nil, fmt.Errorf("store: timeline sleeps: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		session, err := scanSleep(rows)
		if err != nil {
			return nil, err
		}
		item := session
		out[item.ID] = &item
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: timeline sleeps: %w", err)
	}
	return out, nil
}

func placeholdersFor(ids []int64) (string, []any) {
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return placeholders, args
}
