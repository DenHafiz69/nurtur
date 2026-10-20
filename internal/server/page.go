package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	"nurtur/internal/feed"
	"nurtur/internal/view"
)

// DayBounds returns the local day containing now as a UTC half-open interval.
// Using AddDate on a local midnight keeps 23- and 25-hour DST days correct.
func DayBounds(now time.Time, loc *time.Location) (time.Time, time.Time) {
	if loc == nil {
		loc = time.Local
	}
	local := now.In(loc)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	return start.UTC(), start.AddDate(0, 0, 1).UTC()
}

// buildPage assembles every fragment target from the same snapshot. The
// dashboard, timer card, badge, summary, and list therefore always agree.
func (s *Server) buildPage(ctx context.Context, nonce string, toast *view.Toast) (*view.PageData, error) {
	now := s.clock.Now().UTC()
	p := &view.PageData{
		Nonce:    nonce,
		Version:  s.version,
		BabyName: s.babyName,
		Now:      now,
		Location: s.loc,
		Toast:    toast,
	}
	if p.BabyName == "" {
		p.BabyName = "Baby"
	}
	p.Greeting = view.Greeting(now.In(s.loc).Hour())
	p.TodayLabel = now.In(s.loc).Format("Monday, January 2")

	active, err := s.store.ActiveFeed(ctx)
	if err != nil {
		return nil, fmt.Errorf("server: load active feed: %w", err)
	}
	last, err := s.store.LastFeed(ctx)
	if err != nil {
		return nil, fmt.Errorf("server: load last feed: %w", err)
	}

	if active != nil {
		p.Active = s.activeView(ctx, active, now)
	}

	if active == nil && last != nil && last.EndedAt != nil {
		elapsed := now.Sub(*last.EndedAt)
		if elapsed < 0 {
			elapsed = 0
		}
		p.Last = &view.LastView{
			SideTitle: last.MostRecentSide().Title(),
			Since:     *last.EndedAt,
			Elapsed:   elapsed,
		}
	}

	from, to := DayBounds(now, s.loc)
	feeds, err := s.store.RecentFeeds(ctx, from, to, 100)
	if err != nil {
		return nil, fmt.Errorf("server: load today's feeds: %w", err)
	}

	var totalLeft, totalRight time.Duration
	p.Feeds = make([]view.FeedRow, 0, len(feeds))
	for _, f := range feeds {
		left, right := f.Totals(now)
		totalLeft += left
		totalRight += right
		side := f.MostRecentSide()
		p.Feeds = append(p.Feeds, view.FeedRow{
			ID:            f.ID,
			Active:        f.EndedAt == nil,
			StartLabel:    f.StartedAt.In(s.loc).Format("15:04"),
			StartedAt:     f.StartedAt,
			TotalLabel:    view.FormatDuration(left + right),
			SplitLabel:    view.SplitLabel(left, right, side),
			LastSide:      side,
			LastSideTitle: side.Title(),
		})
	}

	p.Today = view.TodaySummary{
		Count:      len(p.Feeds),
		CountLabel: view.FeedCountLabel(len(p.Feeds)),
		SplitLabel: view.TodaySplitLabel(totalLeft, totalRight),
	}
	return p, nil
}

func (s *Server) activeView(ctx context.Context, f *feed.Feed, now time.Time) *view.ActiveView {
	side := f.MostRecentSide()
	started := f.StartedAt
	elapsed := now.Sub(started)
	if current, ok := f.CurrentSegment(); ok {
		side = current.Side
		started = current.StartedAt
		elapsed = current.Duration(now)
	}
	if elapsed < 0 {
		elapsed = 0
	}

	left, right, err := s.store.TotalsForFeed(ctx, f.ID, now)
	if err != nil {
		left, right = f.Totals(now)
	}
	opposite := feed.Opposite(side)
	return &view.ActiveView{
		ID:            f.ID,
		Side:          side,
		SideTitle:     side.Title(),
		Opposite:      opposite,
		OppositeTitle: opposite.Title(),
		StartedAt:     started,
		ServerNow:     now,
		Elapsed:       elapsed,
		SplitLabel:    view.SplitLabel(left, right, side),
	}
}

func (s *Server) buildEditView(f *feed.Feed, now time.Time) *view.EditView {
	p := &view.PageData{Location: s.loc, Now: now}
	ev := &view.EditView{ID: f.ID, Active: f.EndedAt == nil}
	left, right := f.Totals(now)
	ev.LeftMinutes = view.Minutes(left)
	ev.RightMinutes = view.Minutes(right)

	if ev.Active {
		ev.Title = "Adjust this feed"
		side := f.MostRecentSide()
		start := f.StartedAt
		if current, ok := f.CurrentSegment(); ok {
			side = current.Side
			start = current.StartedAt
		}
		ev.SideTitle = side.Title()
		ev.StartInput = p.DateInput(start)
	} else {
		ev.Title = "Edit feed"
		ev.StartInput = p.DateInput(f.StartedAt)
		if f.EndedAt != nil {
			ev.EndInput = p.DateInput(*f.EndedAt)
		}
	}
	return ev
}

// parseLocalDateTime reads a datetime-local value in the configured timezone
// and returns UTC. Both minute and second precision are accepted.
func parseLocalDateTime(raw string, loc *time.Location) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}
	layouts := []string{"2006-01-02T15:04:05", "2006-01-02T15:04"}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, raw, loc); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid time %q", raw)
}
