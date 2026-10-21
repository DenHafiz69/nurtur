package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nurtur/internal/clock"
	"nurtur/internal/diaper"
	"nurtur/internal/feed"
	"nurtur/internal/sleep"
	"nurtur/internal/timeline"
	"nurtur/internal/view"
)

const (
	defaultTimelineLimit = 50
	maxTimelineLimit     = 200
	maxTimelineDays      = 7
	timelineDayLoadCap   = 500
)

func (s *Server) handleTimelinePage(w http.ResponseWriter, r *http.Request) {
	data, err := s.buildPage(r.Context(), nonceFrom(r.Context()), nil)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	data.View = "timeline"
	cursor, err := parseTimelineCursor(r.URL.Query().Get("before"))
	if err != nil {
		s.badRequest(w, "That history link isn't valid.")
		return
	}
	page, err := s.buildTimeline(r.Context(), cursor, parseTimelineLimit(r.URL.Query().Get("limit")))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	data.Timeline = page
	s.renderPage(w, r, http.StatusOK, data)
}

func (s *Server) handleTimelinePartial(w http.ResponseWriter, r *http.Request) {
	cursor, err := parseTimelineCursor(r.URL.Query().Get("before"))
	if err != nil {
		s.badRequest(w, "That history link isn't valid.")
		return
	}
	limit := parseTimelineLimit(r.URL.Query().Get("limit"))
	page, err := s.buildTimeline(r.Context(), cursor, limit)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	data, err := s.buildPage(r.Context(), nonceFrom(r.Context()), nil)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	data.View = "timeline"
	data.Timeline = page
	s.writeFragments(w, r, http.StatusOK, "timeline-days", data, false, nil, "", false, "", "")
}

// buildTimeline loads whole local days, newest first, until the entry budget is
// reached. Paginating by day means a date header never splits across pages.
func (s *Server) buildTimeline(ctx context.Context, before time.Time, limit int) (*view.TimelinePageView, error) {
	if limit <= 0 {
		limit = defaultTimelineLimit
	}
	if limit > maxTimelineLimit {
		limit = maxTimelineLimit
	}
	now := s.clock.Now().UTC()

	var dayEnd time.Time
	if before.IsZero() {
		_, dayEnd = clock.DayBounds(now, s.loc)
	} else {
		dayEnd = before.UTC()
	}

	page := &view.TimelinePageView{}
	if dayEnd.After(now.Add(48 * time.Hour)) {
		dayEnd = now
	}

	dayStart := previousLocalDayStart(dayEnd, s.loc)
	nextBoundary := dayEnd
	days := 0
	for days < maxTimelineDays {
		entries, err := s.store.TimelineBetween(ctx, dayStart, dayEnd, timelineDayLoadCap)
		if err != nil {
			return nil, err
		}
		summary, err := s.store.TimelineDaySummary(ctx, dayStart, dayEnd, now)
		if err != nil {
			return nil, err
		}

		if len(entries) > 0 {
			group := view.TimelineDay{
				Key:   view.DateKey(dayStart, s.loc),
				Label: view.DayLabelRelative(dayStart, now, s.loc),
				Meta:  view.DayMetaLabel(summary.Diapers, summary.Sleep),
				Feeds: view.NewFeedSummary(summary.Feeds, summary.FeedLeft, summary.FeedRight),
			}
			for _, entry := range entries {
				group.Rows = append(group.Rows, s.timelineRow(entry, now))
			}
			page.Days = append(page.Days, group)
			page.Count += len(entries)
		}

		nextBoundary = dayStart
		days++
		if page.Count >= limit {
			break
		}
		dayEnd = dayStart
		dayStart = previousLocalDayStart(dayEnd, s.loc)
	}

	hasMore, err := s.store.HasTimelineBefore(ctx, nextBoundary)
	if err != nil {
		return nil, err
	}
	page.HasMore = hasMore
	page.HasDays = len(page.Days) > 0
	if hasMore {
		page.NextCursor = nextBoundary.UTC().Format(time.RFC3339)
	}
	return page, nil
}

// timelineDayView rebuilds one local day's header aggregate after a mutation,
// so a summary card never disagrees with the rows beside it.
func (s *Server) timelineDayView(ctx context.Context, anchor time.Time) (*view.TimelineDay, error) {
	now := s.clock.Now().UTC()
	start, end := clock.DayBounds(anchor, s.loc)
	summary, err := s.store.TimelineDaySummary(ctx, start, end, now)
	if err != nil {
		return nil, err
	}
	return &view.TimelineDay{
		Key:   view.DateKey(start, s.loc),
		Label: view.DayLabelRelative(start, now, s.loc),
		Meta:  view.DayMetaLabel(summary.Diapers, summary.Sleep),
		Feeds: view.NewFeedSummary(summary.Feeds, summary.FeedLeft, summary.FeedRight),
	}, nil
}

func previousLocalDayStart(boundary time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.Local
	}
	local := boundary.In(loc)
	start := time.Date(local.Year(), local.Month(), local.Day()-1, 0, 0, 0, 0, loc)
	return start.UTC()
}

func parseTimelineCursor(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

func parseTimelineLimit(raw string) int {
	if raw == "" {
		return defaultTimelineLimit
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultTimelineLimit
	}
	if n > maxTimelineLimit {
		return maxTimelineLimit
	}
	return n
}

func (s *Server) timelineRow(entry timeline.Entry, now time.Time) view.EventRow {
	switch {
	case entry.Feed != nil:
		f := entry.Feed
		left, right := f.Totals(now)
		active := f.EndedAt == nil
		title := "Nursed"
		detail := view.SplitLabel(left, right, f.MostRecentSide())
		if active {
			title = "Nursing"
		} else {
			detail = view.FeedDetail(left, right)
		}
		return view.EventRow{
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
			DeleteURL:    fmt.Sprintf("/timeline/feed/%d", f.ID),
			DeleteTarget: "closest li",
			ActionLabel:  "feed at " + view.FormatClock(f.StartedAt, s.loc),
			KindClass:    "event-feed",
			Timeline:     true,
			RowID:        fmt.Sprintf("row-feed-%d", f.ID),
			DeleteKey:    fmt.Sprintf("feed-%d", f.ID),
		}

	case entry.Diaper != nil:
		d := entry.Diaper
		return view.EventRow{
			Kind:         "diaper",
			ID:           d.ID,
			Anchor:       d.LoggedAt,
			TimeLabel:    view.FormatClock(d.LoggedAt, s.loc),
			Title:        d.Kind.Title(),
			Detail:       d.Summary(),
			Icon:         diaperIcon(d.Kind),
			EditURL:      fmt.Sprintf("/diapers/%d/sheet", d.ID),
			EditTarget:   "#action-dialog",
			DeleteURL:    fmt.Sprintf("/timeline/diaper/%d", d.ID),
			DeleteTarget: "closest li",
			ActionLabel:  "diaper at " + view.FormatClock(d.LoggedAt, s.loc),
			KindClass:    "event-diaper",
			Timeline:     true,
			RowID:        fmt.Sprintf("row-diaper-%d", d.ID),
			DeleteKey:    fmt.Sprintf("diaper-%d", d.ID),
		}

	case entry.Sleep != nil:
		session := entry.Sleep
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
		return view.EventRow{
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
			DeleteURL:    fmt.Sprintf("/timeline/sleep/%d", session.ID),
			DeleteTarget: "closest li",
			ActionLabel:  "sleep at " + view.FormatClock(session.StartedAt, s.loc),
			KindClass:    "event-sleep",
			Timeline:     true,
			RowID:        fmt.Sprintf("row-sleep-%d", session.ID),
			DeleteKey:    fmt.Sprintf("sleep-%d", session.ID),
		}
	}
	return view.EventRow{}
}

// timelineRowByID re-reads one live event and builds its row. Restore uses it
// to put a deleted event back in the exact spot it came from.
func (s *Server) timelineRowByID(ctx context.Context, kind timeline.Kind, id int64) (view.EventRow, error) {
	now := s.clock.Now().UTC()
	entry := timeline.Entry{Kind: kind, ID: id}
	switch kind {
	case timeline.Feed:
		f, err := s.store.GetFeed(ctx, id)
		if err != nil {
			return view.EventRow{}, err
		}
		entry.Feed = f
		entry.Anchor = f.StartedAt
	case timeline.Diaper:
		d, err := s.store.GetDiaper(ctx, id)
		if err != nil {
			return view.EventRow{}, err
		}
		entry.Diaper = &d
		entry.Anchor = d.LoggedAt
	case timeline.Sleep:
		session, err := s.store.GetSleep(ctx, id)
		if err != nil {
			return view.EventRow{}, err
		}
		entry.Sleep = &session
		entry.Anchor = session.StartedAt
	default:
		return view.EventRow{}, fmt.Errorf("server: unknown timeline kind %q", kind)
	}
	return s.timelineRow(entry, now), nil
}

func (s *Server) handleTimelineDelete(w http.ResponseWriter, r *http.Request) {
	kind, id, ok := s.timelineTarget(w, r)
	if !ok {
		return
	}
	// Capture the row's day before deleting so its summary card can refresh.
	var dayAnchor time.Time
	if row, err := s.timelineRowByID(r.Context(), kind, id); err == nil {
		dayAnchor = row.Anchor
	}

	now := s.clock.Now()
	var err error
	switch kind {
	case timeline.Feed:
		err = s.store.DeleteFeed(r.Context(), id, now)
	case timeline.Diaper:
		err = s.store.DeleteDiaper(r.Context(), id, now)
	case timeline.Sleep:
		err = s.store.DeleteSleep(r.Context(), id, now)
	}

	key := fmt.Sprintf("%s-%d", kind, id)
	switch {
	case err == nil:
	case errors.Is(err, feed.ErrAlreadyDeleted), errors.Is(err, diaper.ErrAlreadyDeleted), errors.Is(err, sleep.ErrAlreadyDeleted):
		s.renderMutation(w, r, "status-strip", &view.Toast{
			Message: "That " + strings.ToLower(kind.Title()) + " was already deleted.",
			Kind:    "info",
		})
		return
	case errors.Is(err, feed.ErrNotFound), errors.Is(err, diaper.ErrNotFound), errors.Is(err, sleep.ErrNotFound):
		s.notFound(w)
		return
	default:
		s.serverError(w, r, err)
		return
	}

	toast := &view.Toast{
		Message:      kind.Title() + " deleted.",
		Kind:         "success",
		ActionLabel:  "Undo",
		ActionURL:    fmt.Sprintf("/timeline/%s/%d/restore", kind, id),
		ActionMethod: "post",
		ActionTarget: "#deleted-" + key,
		DurationMS:   8000,
	}

	trigger := ""
	if kind == timeline.Feed {
		trigger = "timer-cleared"
	}
	var day *view.TimelineDay
	if !dayAnchor.IsZero() {
		day, _ = s.timelineDayView(r.Context(), dayAnchor)
	}
	s.respond(w, r, mutateResponse{
		Primary:  "timeline-placeholder",
		Toast:    toast,
		TimerOOB: kind == timeline.Feed,
		Trigger:  trigger,
		Redirect: "/timeline",
		Reswap:   "outerHTML",
		ExtraOOB: []string{"timeline-day-oob"},
		Apply: func(data *view.PageData) {
			data.DeletedKey = key
			data.TimelineDay = day
		},
	})
}

func (s *Server) handleTimelineRestore(w http.ResponseWriter, r *http.Request) {
	kind, id, ok := s.timelineTarget(w, r)
	if !ok {
		return
	}
	now := s.clock.Now()
	var (
		restored bool
		err      error
	)
	switch kind {
	case timeline.Feed:
		_, restored, err = s.store.RestoreFeed(r.Context(), id)
	case timeline.Diaper:
		_, restored, err = s.store.RestoreDiaper(r.Context(), id)
	case timeline.Sleep:
		_, restored, err = s.store.RestoreSleep(r.Context(), id, now)
	}

	switch {
	case err == nil:
	case errors.Is(err, feed.ErrAlreadyActive):
		s.renderMutation(w, r, "status-strip", &view.Toast{
			Message: "Another feed is running, so that one can't come back as active.",
			Kind:    "info",
		})
		return
	case errors.Is(err, sleep.ErrOverlap):
		s.renderMutation(w, r, "status-strip", &view.Toast{
			Message: "Another sleep overlaps that time, so it can't be restored.",
			Kind:    "info",
		})
		return
	case errors.Is(err, feed.ErrNotFound), errors.Is(err, diaper.ErrNotFound), errors.Is(err, sleep.ErrNotFound):
		s.notFound(w)
		return
	default:
		s.serverError(w, r, err)
		return
	}

	toast := &view.Toast{Message: kind.Title() + " restored.", Kind: "success"}
	if !restored {
		toast = &view.Toast{Message: "That " + strings.ToLower(kind.Title()) + " is already back.", Kind: "info"}
	}

	row, err := s.timelineRowByID(r.Context(), kind, id)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	day, _ := s.timelineDayView(r.Context(), row.Anchor)
	s.respond(w, r, mutateResponse{
		Primary:          "timeline-row-fragment",
		Row:              &row,
		Toast:            toast,
		TimerOOB:         kind == timeline.Feed,
		Trigger:          "sheet-saved",
		TriggerAfterSwap: true,
		Reswap:           "outerHTML",
		Redirect:         "/timeline",
		ExtraOOB:         []string{"timeline-day-oob"},
		Apply:            func(data *view.PageData) { data.TimelineDay = day },
	})
}

func (s *Server) timelineTarget(w http.ResponseWriter, r *http.Request) (timeline.Kind, int64, bool) {
	kind, err := timeline.ParseKind(r.PathValue("kind"))
	if err != nil {
		s.notFound(w)
		return "", 0, false
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		s.notFound(w)
		return "", 0, false
	}
	return kind, id, true
}

// onTimeline reports whether the browser is currently on the history page, so
// edit responses can update the row in place instead of the dashboard cards.
func onTimeline(r *http.Request) bool {
	current := r.Header.Get("HX-Current-URL")
	if current == "" {
		current = r.Referer()
	}
	return strings.Contains(current, "/timeline")
}

// respondTimelineRow updates a single history row in place after an edit. It
// returns false when the request did not come from the history page, letting
// the caller use its normal dashboard target.
func (s *Server) respondTimelineRow(w http.ResponseWriter, r *http.Request, kind timeline.Kind, id int64, toast *view.Toast) bool {
	if !onTimeline(r) {
		return false
	}
	row, err := s.timelineRowByID(r.Context(), kind, id)
	if err != nil {
		s.serverError(w, r, err)
		return true
	}
	day, _ := s.timelineDayView(r.Context(), row.Anchor)
	s.respond(w, r, mutateResponse{
		Primary:          "timeline-row-fragment",
		Row:              &row,
		Toast:            toast,
		TimerOOB:         kind == timeline.Feed,
		Trigger:          "sheet-saved",
		TriggerAfterSwap: true,
		Retarget:         "#" + row.RowID,
		Reswap:           "outerHTML",
		Redirect:         "/timeline",
		ExtraOOB:         []string{"timeline-day-oob"},
		Apply:            func(data *view.PageData) { data.TimelineDay = day },
	})
	return true
}
