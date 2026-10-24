package server

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"nurtur/internal/clock"
	"nurtur/internal/diaper"
	"nurtur/internal/feed"
	"nurtur/internal/sleep"
	"nurtur/internal/status"
	"nurtur/internal/view"
)

// DayBounds returns the local day containing now as a UTC half-open interval.
func DayBounds(now time.Time, loc *time.Location) (time.Time, time.Time) {
	return clock.DayBounds(now, loc)
}

// buildPage assembles every fragment target from the same snapshot. The
// dashboard, cards, badge, summary, and list therefore always agree.
func (s *Server) buildPage(ctx context.Context, nonce string, toast *view.Toast) (*view.PageData, error) {
	now := s.clock.Now().UTC()
	profile, err := s.currentBabyProfile(ctx)
	if err != nil {
		return nil, err
	}
	p := &view.PageData{
		Nonce:           nonce,
		Version:         s.version,
		BabyName:        profile.Name,
		BabyAge:         view.FormatBabyAge(profile.BirthDate, now, s.loc),
		BabyGenderLabel: babyGenderLabel(profile.Gender),
		MascotName:      profile.Mascot,
		Now:             now,
		Location:        s.loc,
		Toast:           toast,
		CSRF:            csrfFrom(ctx),
		AuthEnabled:     s.auth.enabled(),
		SyncChip:        s.syncChip(),
	}
	p.FeedAlertMinutes = s.feedAlertMinutes(ctx)
	p.Greeting = view.Greeting(now.In(s.loc).Hour())
	p.TodayLabel = now.In(s.loc).Format("Monday, January 2")

	nightStart, nightEnd := s.nightWindow(ctx)
	snap, err := status.Build(ctx, s.store, now, s.loc, nightStart, nightEnd)
	if err != nil {
		return nil, fmt.Errorf("server: build status: %w", err)
	}
	p.Status = s.statusView(snap)
	if snap.ActiveFeed != nil {
		p.Active = s.activeView(ctx, snap.ActiveFeed, now)
	}
	if snap.ActiveSleep != nil {
		p.Sleep = view.SleepView{
			Active:    true,
			ID:        snap.ActiveSleep.ID,
			StartedAt: snap.ActiveSleep.StartedAt,
			ServerNow: now,
			Elapsed:   snap.ActiveSleep.Duration(now),
		}
	}
	if snap.ActiveFeed == nil && snap.LastFeed != nil && snap.LastFeed.EndedAt != nil {
		elapsed := now.Sub(*snap.LastFeed.EndedAt)
		if elapsed < 0 {
			elapsed = 0
		}
		p.Last = &view.LastView{
			SideTitle: snap.LastFeed.MostRecentSide().Title(),
			Since:     *snap.LastFeed.EndedAt,
			Elapsed:   elapsed,
		}
	}

	feeds, err := s.store.RecentFeeds(ctx, snap.DayStart, snap.DayEnd, 100)
	if err != nil {
		return nil, fmt.Errorf("server: load today's feeds: %w", err)
	}
	diapers, err := s.store.RecentDiapers(ctx, snap.DayStart, snap.DayEnd, 200)
	if err != nil {
		return nil, fmt.Errorf("server: load today's diapers: %w", err)
	}

	var totalLeft, totalRight time.Duration
	for _, f := range feeds {
		left, right := f.Totals(now)
		totalLeft += left
		totalRight += right
	}

	p.Events = s.eventRows(feeds, diapers, snap.SleepsToday, now)
	sleepLabel := view.SleepTotalLabel(snap.SleepToday.Total)
	if snap.ActiveSleep != nil && snap.SleepToday.Total < time.Minute {
		sleepLabel = "Sleeping now"
	}
	p.Today = view.TodaySummary{
		Feeds:       view.NewFeedSummary(len(feeds), totalLeft, totalRight),
		DiaperLabel: view.DiaperCountLabel(len(diapers)),
		SleepLabel:  sleepLabel,
		HasEvents:   len(p.Events) > 0,
	}
	return p, nil
}

func (s *Server) statusView(snap status.Snapshot) view.StatusView {
	now := snap.Now
	var out view.StatusView

	// Nursing
	out.Nursing = view.StatusCard{
		Kind:   "nursing",
		Label:  "Nursing",
		Detail: "No feeds yet",
		Tone:   "peach",
		Icon:   "drop",
		Href:   "/feeds/sheet",
	}
	if snap.ActiveFeed != nil {
		side := snap.ActiveFeed.MostRecentSide()
		since := snap.ActiveFeed.StartedAt
		if current, ok := snap.ActiveFeed.CurrentSegment(); ok {
			side = current.Side
			since = current.StartedAt
		}
		out.Nursing.Active = true
		out.Nursing.HasData = true
		out.Nursing.Value = view.FormatDuration(now.Sub(since))
		out.Nursing.ValueSince = since
		out.Nursing.ValueMode = "duration"
		out.Nursing.Detail = "Nursing " + strings.ToLower(side.Title())
		out.Nursing.Icon = "drop-" + string(side)
	} else if snap.LastFeed != nil && snap.LastFeed.EndedAt != nil {
		out.Nursing.HasData = true
		out.Nursing.Value = view.FormatAgo(now, *snap.LastFeed.EndedAt)
		out.Nursing.ValueSince = *snap.LastFeed.EndedAt
		out.Nursing.ValueMode = "ago"
		out.Nursing.Detail = snap.LastFeed.MostRecentSide().Title() + " side"
	}

	// Diaper
	out.Diaper = view.StatusCard{
		Kind:   "diaper",
		Label:  "Diaper",
		Detail: "No diapers yet",
		Tone:   "butter",
		Icon:   "diaper",
		Href:   "/diapers/sheet",
	}
	primary, secondary := latestDiapers(snap.LastWet, snap.LastDirty)
	if primary != nil {
		out.Diaper.HasData = true
		out.Diaper.Value = view.FormatAgo(now, primary.LoggedAt)
		out.Diaper.ValueSince = primary.LoggedAt
		out.Diaper.ValueMode = "ago"
		out.Diaper.Detail = primary.Kind.Title()
		if secondary != nil && secondary.ID != primary.ID && primary.Kind != diaper.Both {
			out.Diaper.Secondary = secondary.Kind.Title()
			out.Diaper.SecondaryValue = view.FormatAgo(now, secondary.LoggedAt)
			since := secondary.LoggedAt
			out.Diaper.SecondarySince = &since
			out.Diaper.SecondaryMode = "ago"
		}
	}

	// Sleep
	out.Sleep = view.StatusCard{
		Kind:   "sleep",
		Label:  "Sleep",
		Detail: "No sleep yet",
		Tone:   "sage",
		Icon:   "moon",
		Href:   "/sleeps/sheet",
	}
	switch {
	case snap.ActiveSleep != nil:
		out.Sleep.Active = true
		out.Sleep.HasData = true
		out.Sleep.Value = view.FormatDuration(snap.ActiveSleep.Duration(now))
		out.Sleep.ValueSince = snap.ActiveSleep.StartedAt
		out.Sleep.ValueMode = "duration"
		out.Sleep.Detail = "Asleep"
		out.Sleep.Icon = "moon"
	case snap.HasWakeWindow:
		out.Sleep.HasData = true
		out.Sleep.Value = view.FormatDuration(snap.WakeWindow)
		out.Sleep.ValueSince = *snap.LastWake.EndedAt
		out.Sleep.ValueMode = "duration"
		out.Sleep.Detail = "Awake"
		out.Sleep.Secondary = "since " + view.FormatClock(*snap.LastWake.EndedAt, s.loc)
		out.Sleep.Icon = "sun"
	}

	return out
}

// latestDiapers returns the more recent event first, then the other category
// if it refers to a different row.
func latestDiapers(wet, dirty *diaper.Diaper) (*diaper.Diaper, *diaper.Diaper) {
	switch {
	case wet == nil && dirty == nil:
		return nil, nil
	case wet == nil:
		return dirty, nil
	case dirty == nil:
		return wet, nil
	case wet.ID == dirty.ID:
		return wet, nil
	case dirty.LoggedAt.After(wet.LoggedAt):
		return dirty, wet
	default:
		return wet, dirty
	}
}

func (s *Server) eventRows(feeds []feed.Feed, diapers []diaper.Diaper, sleeps []sleep.Session, now time.Time) []view.EventRow {
	rows := make([]view.EventRow, 0, len(feeds)+len(diapers)+len(sleeps))

	for _, f := range feeds {
		left, right := f.Totals(now)
		active := f.EndedAt == nil
		title := "Nursed"
		detail := view.SplitLabel(left, right, f.MostRecentSide())
		if active {
			title = "Nursing"
		} else {
			detail = view.FeedDetail(left, right)
		}
		rows = append(rows, view.EventRow{
			Kind:         "feed",
			ID:           f.ID,
			Anchor:       f.StartedAt,
			TimeLabel:    view.FormatClock(f.StartedAt, s.loc),
			Title:        title,
			Detail:       detail,
			Active:       active,
			Icon:         "drop-" + string(f.MostRecentSide()),
			EditURL:      fmt.Sprintf("/feeds/%d/edit", f.ID),
			EditTarget:   "#feed-edit-dialog",
			DeleteURL:    fmt.Sprintf("/feeds/%d", f.ID),
			DeleteTarget: "#timer-card",
			ActionLabel:  "feed at " + view.FormatClock(f.StartedAt, s.loc),
			KindClass:    "event-feed",
		})
	}

	for _, d := range diapers {
		rows = append(rows, view.EventRow{
			Kind:         "diaper",
			ID:           d.ID,
			Anchor:       d.LoggedAt,
			TimeLabel:    view.FormatClock(d.LoggedAt, s.loc),
			Title:        d.Kind.Title(),
			Detail:       d.Summary(),
			Icon:         diaperIcon(d.Kind),
			EditURL:      fmt.Sprintf("/diapers/%d/sheet", d.ID),
			EditTarget:   "#action-dialog",
			DeleteURL:    fmt.Sprintf("/diapers/%d", d.ID),
			DeleteTarget: "#status-strip",
			ActionLabel:  "diaper at " + view.FormatClock(d.LoggedAt, s.loc),
			KindClass:    "event-diaper",
		})
	}

	for _, session := range sleeps {
		active := session.EndedAt == nil
		detail := view.FormatDuration(session.Duration(now))
		if active {
			detail += " so far"
		} else {
			detail += " · " + view.FormatRange(session.StartedAt, *session.EndedAt, s.loc)
		}
		title := "Slept"
		if active {
			title = "Sleeping"
		}
		rows = append(rows, view.EventRow{
			Kind:         "sleep",
			ID:           session.ID,
			Anchor:       session.StartedAt,
			TimeLabel:    view.FormatClock(session.StartedAt, s.loc),
			Title:        title,
			Detail:       detail,
			Active:       active,
			Icon:         "moon",
			EditURL:      fmt.Sprintf("/sleeps/%d/sheet", session.ID),
			EditTarget:   "#action-dialog",
			DeleteURL:    fmt.Sprintf("/sleeps/%d", session.ID),
			DeleteTarget: "#sleep-pill",
			ActionLabel:  "sleep at " + view.FormatClock(session.StartedAt, s.loc),
			KindClass:    "event-sleep",
		})
	}

	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Anchor.Equal(rows[j].Anchor) {
			return rows[i].ID > rows[j].ID
		}
		return rows[i].Anchor.After(rows[j].Anchor)
	})
	return rows
}

func diaperIcon(kind diaper.Kind) string {
	switch kind {
	case diaper.Wet:
		return "drop"
	case diaper.Dirty:
		return "dirty"
	default:
		return "diaper"
	}
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
		TotalElapsed:  left + right,
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
