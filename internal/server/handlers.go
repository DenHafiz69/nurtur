package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nurtur/internal/feed"
	"nurtur/internal/timeline"
	"nurtur/internal/view"
)

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if s.landingRequest(r) {
		s.handleLanding(w, r)
		return
	}
	data, err := s.buildPage(r.Context(), nonceFrom(r.Context()), nil)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.renderPage(w, r, http.StatusOK, data)
}

func (s *Server) handleTimerFragment(w http.ResponseWriter, r *http.Request) {
	data, err := s.buildPage(r.Context(), nonceFrom(r.Context()), nil)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	oob := reconcileOOB
	if onTimeline(r) {
		oob = timelineOOB(oob, "timer")
	}
	s.writeFragments(w, r, http.StatusOK, "timer", data, false, oob, "", false, "", "")
}

func (s *Server) handleStartFeed(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, "Couldn't read that form. Try again.")
		return
	}
	side, err := feed.ParseSide(r.FormValue("side"))
	if err != nil {
		s.badRequest(w, "That side isn't valid.")
		return
	}

	created, err := s.store.CreateFeed(r.Context(), side, s.clock.Now())
	toast := &view.Toast{Message: "Nursing " + strings.ToLower(side.Title()) + ".", Kind: "success"}
	switch {
	case err == nil:
	case errors.Is(err, feed.ErrAlreadyActive):
		toast = &view.Toast{Message: "A feed is already running. Switch sides instead.", Kind: "info"}
	case errors.Is(err, feed.ErrInvalidSide):
		s.badRequest(w, "That side isn't valid.")
		return
	default:
		s.serverError(w, r, err)
		return
	}
	opts := mutateResponse{Toast: toast, Trigger: "sheet-saved", TriggerAfterSwap: true}
	if err == nil && onTimeline(r) {
		if row, rowErr := s.timelineRowByID(r.Context(), timeline.Feed, created.ID); rowErr == nil {
			opts.Row = &row
			opts.RowMode = "insert"
		}
	}
	s.respond(w, r, opts)
}

func (s *Server) handleSwitchFeed(w http.ResponseWriter, r *http.Request) {
	id, ok := s.feedID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, "Couldn't read that form. Try again.")
		return
	}
	side, err := feed.ParseSide(r.FormValue("side"))
	if err != nil {
		s.badRequest(w, "That side isn't valid.")
		return
	}

	updated, err := s.store.SwitchFeed(r.Context(), id, side, s.clock.Now())
	toast := &view.Toast{Message: "Switched to " + strings.ToLower(side.Title()) + ".", Kind: "success"}
	switch {
	case err == nil:
	case errors.Is(err, feed.ErrSameSide):
		toast = &view.Toast{Message: "Already nursing " + strings.ToLower(side.Title()) + ".", Kind: "info"}
	case errors.Is(err, feed.ErrNoActiveFeed):
		toast = &view.Toast{Message: "That feed isn't running anymore.", Kind: "info"}
	case errors.Is(err, feed.ErrAlreadyDeleted):
		toast = &view.Toast{Message: "That feed was deleted.", Kind: "info"}
	case errors.Is(err, feed.ErrNotFound):
		s.notFound(w)
		return
	default:
		s.serverError(w, r, err)
		return
	}
	opts := mutateResponse{Toast: toast, Trigger: "sheet-saved", TriggerAfterSwap: true}
	if err == nil && onTimeline(r) {
		if row, rowErr := s.timelineRowByID(r.Context(), timeline.Feed, updated.ID); rowErr == nil {
			opts.Row = &row
			opts.RowMode = "update"
		}
	}
	s.respond(w, r, opts)
}

func (s *Server) handleStopFeed(w http.ResponseWriter, r *http.Request) {
	id, ok := s.feedID(w, r)
	if !ok {
		return
	}

	stopped, err := s.store.StopFeed(r.Context(), id, s.clock.Now())
	var toast *view.Toast
	switch {
	case err == nil:
		toast = &view.Toast{Message: "Saved. Last side: " + stopped.MostRecentSide().Title() + ".", Kind: "success"}
	case errors.Is(err, feed.ErrNoActiveFeed):
		toast = &view.Toast{Message: "That feed already ended.", Kind: "info"}
	case errors.Is(err, feed.ErrAlreadyDeleted):
		toast = &view.Toast{Message: "That feed was deleted.", Kind: "info"}
	case errors.Is(err, feed.ErrNotFound):
		s.notFound(w)
		return
	default:
		s.serverError(w, r, err)
		return
	}
	opts := mutateResponse{Toast: toast, Trigger: "sheet-saved", TriggerAfterSwap: true}
	if err == nil && onTimeline(r) {
		if row, rowErr := s.timelineRowByID(r.Context(), timeline.Feed, stopped.ID); rowErr == nil {
			opts.Row = &row
			opts.RowMode = "update"
		}
	}
	s.respond(w, r, opts)
}

func (s *Server) handleDeleteFeed(w http.ResponseWriter, r *http.Request) {
	id, ok := s.feedID(w, r)
	if !ok {
		return
	}

	err := s.store.DeleteFeed(r.Context(), id, s.clock.Now())
	toast := &view.Toast{
		Message:      "Feed deleted.",
		Kind:         "success",
		ActionLabel:  "Undo",
		ActionURL:    fmt.Sprintf("/feeds/%d/restore", id),
		ActionMethod: "post",
		ActionTarget: "#timer-card",
		DurationMS:   8000,
	}
	switch {
	case err == nil:
	case errors.Is(err, feed.ErrAlreadyDeleted):
		toast = &view.Toast{Message: "That feed was already deleted.", Kind: "info"}
	case errors.Is(err, feed.ErrNotFound):
		s.notFound(w)
		return
	default:
		s.serverError(w, r, err)
		return
	}
	s.respond(w, r, mutateResponse{Toast: toast, Trigger: "timer-cleared", TriggerAfterSwap: true})
}

func (s *Server) handleRestoreFeed(w http.ResponseWriter, r *http.Request) {
	id, ok := s.feedID(w, r)
	if !ok {
		return
	}
	_, restored, err := s.store.RestoreFeed(r.Context(), id)
	var toast *view.Toast
	switch {
	case err == nil && restored:
		toast = &view.Toast{Message: "Feed restored.", Kind: "success"}
	case err == nil:
		toast = &view.Toast{Message: "That feed is already back.", Kind: "info"}
	case errors.Is(err, feed.ErrAlreadyActive):
		toast = &view.Toast{Message: "Another feed is running, so that one can't come back as active.", Kind: "info"}
	case errors.Is(err, feed.ErrNotFound):
		s.notFound(w)
		return
	default:
		s.serverError(w, r, err)
		return
	}
	s.respond(w, r, mutateResponse{
		Primary:          "timer",
		Toast:            toast,
		Trigger:          "sheet-saved",
		TriggerAfterSwap: true,
		Retarget:         "#timer-card",
		Reswap:           "outerHTML",
	})
}

func (s *Server) handleEditPage(w http.ResponseWriter, r *http.Request) {
	id, ok := s.feedID(w, r)
	if !ok {
		return
	}
	now := s.clock.Now().UTC()
	f, err := s.store.GetFeed(r.Context(), id)
	if errors.Is(err, feed.ErrNotFound) {
		s.notFound(w)
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}

	data, err := s.buildPage(r.Context(), nonceFrom(r.Context()), nil)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	data.Edit = s.buildEditView(f, now)

	if isHTMX(r) {
		s.writeFragments(w, r, http.StatusOK, "feed-edit", data, false, nil, "", false, "", "")
		return
	}
	data.EditOpen = true
	s.renderPage(w, r, http.StatusOK, data)
}

func (s *Server) handleEditFeed(w http.ResponseWriter, r *http.Request) {
	id, ok := s.feedID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, "Couldn't read that form. Try again.")
		return
	}

	now := s.clock.Now().UTC()
	f, err := s.store.GetFeed(r.Context(), id)
	if errors.Is(err, feed.ErrNotFound) {
		s.notFound(w)
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}

	if f.EndedAt == nil {
		s.handleActiveEdit(w, r, f, now)
		return
	}
	s.handleCompletedEdit(w, r, f, now)
}

func (s *Server) handleActiveEdit(w http.ResponseWriter, r *http.Request, f *feed.Feed, now time.Time) {
	edit := s.buildEditView(f, now)

	if raw := strings.TrimSpace(r.FormValue("adjust")); raw != "" {
		minutes, err := strconv.Atoi(raw)
		if err != nil || minutes < -1440 || minutes > 1440 {
			s.badRequest(w, "That adjustment isn't valid.")
			return
		}
		updated, err := s.store.AdjustActiveSegmentStart(r.Context(), f.ID, time.Duration(minutes)*time.Minute, now)
		if err != nil {
			s.handleEditDomainError(w, r, err, "That feed isn't running anymore.")
			return
		}
		ev := s.buildEditView(&updated, now)
		if s.respondTimelineRow(w, r, timeline.Feed, f.ID, &view.Toast{Message: "Timer adjusted.", Kind: "success"}) {
			return
		}
		s.respond(w, r, mutateResponse{
			Primary:  "feed-edit",
			Toast:    &view.Toast{Message: "Timer adjusted.", Kind: "success"},
			TimerOOB: true,
			Edit:     ev,
		})
		return
	}

	raw := strings.TrimSpace(r.FormValue("segment_started_at"))
	edit.StartInput = raw
	start, err := parseLocalDateTime(raw, s.loc)
	if err != nil {
		edit.StartErr = "Use a valid date and time."
		s.respondEditError(w, r, edit)
		return
	}

	_, err = s.store.SetActiveSegmentStart(r.Context(), f.ID, start, now)
	if err != nil {
		var validation *feed.ValidationError
		if errors.As(err, &validation) {
			applyValidation(edit, err)
			s.respondEditError(w, r, edit)
			return
		}
		s.handleEditDomainError(w, r, err, "That feed isn't running anymore.")
		return
	}

	if s.respondTimelineRow(w, r, timeline.Feed, f.ID, &view.Toast{Message: "Start time updated.", Kind: "success"}) {
		return
	}
	s.respond(w, r, mutateResponse{
		Toast:            &view.Toast{Message: "Start time updated.", Kind: "success"},
		Trigger:          "feed-saved",
		TriggerAfterSwap: true,
		Retarget:         "#timer-card",
		Reswap:           "outerHTML",
	})
}

func (s *Server) handleCompletedEdit(w http.ResponseWriter, r *http.Request, f *feed.Feed, now time.Time) {
	edit := s.buildEditView(f, now)

	rawStart := strings.TrimSpace(r.FormValue("started_at"))
	rawEnd := strings.TrimSpace(r.FormValue("ended_at"))
	edit.StartInput = rawStart
	edit.EndInput = rawEnd

	start, startErr := parseLocalDateTime(rawStart, s.loc)
	end, endErr := parseLocalDateTime(rawEnd, s.loc)
	if startErr != nil {
		edit.StartErr = "Use a valid date and time."
	}
	if endErr != nil {
		edit.EndErr = "Use a valid date and time."
	}

	left, leftErr := parseMinutesField(r.FormValue("left_minutes"))
	right, rightErr := parseMinutesField(r.FormValue("right_minutes"))
	if leftErr != nil {
		edit.LeftErr = leftErr.Error()
	} else {
		edit.LeftMinutes = left
	}
	if rightErr != nil {
		edit.RightErr = rightErr.Error()
	} else {
		edit.RightMinutes = right
	}
	futureStart := start.After(now.Add(time.Minute))
	futureEnd := end.After(now.Add(time.Minute))
	if futureStart {
		edit.StartErr = "That start is in the future. Try again?"
	}
	if futureEnd {
		edit.EndErr = "That end time is in the future. Try again?"
	}
	if startErr != nil || endErr != nil || leftErr != nil || rightErr != nil || futureStart || futureEnd {
		s.respondEditError(w, r, edit)
		return
	}

	correction := feed.CompletedEdit{
		StartedAt: start,
		EndedAt:   end,
		Left:      time.Duration(left) * time.Minute,
		Right:     time.Duration(right) * time.Minute,
	}
	if err := correction.Validate(); err != nil {
		applyValidation(edit, err)
		s.respondEditError(w, r, edit)
		return
	}

	_, err := s.store.UpdateCompletedFeed(r.Context(), f.ID, correction)
	if err != nil {
		var validation *feed.ValidationError
		if errors.As(err, &validation) {
			applyValidation(edit, err)
			s.respondEditError(w, r, edit)
			return
		}
		s.handleEditDomainError(w, r, err, "That feed changed. Close this and try again.")
		return
	}

	if s.respondTimelineRow(w, r, timeline.Feed, f.ID, &view.Toast{Message: "Feed updated.", Kind: "success"}) {
		return
	}
	s.respond(w, r, mutateResponse{
		Toast:            &view.Toast{Message: "Feed updated.", Kind: "success"},
		Trigger:          "feed-saved",
		TriggerAfterSwap: true,
		Retarget:         "#timer-card",
		Reswap:           "outerHTML",
	})
}

func (s *Server) handleEditDomainError(w http.ResponseWriter, r *http.Request, err error, message string) {
	switch {
	case errors.Is(err, feed.ErrNotFound):
		s.notFound(w)
	case errors.Is(err, feed.ErrNoActiveFeed),
		errors.Is(err, feed.ErrFeedActive),
		errors.Is(err, feed.ErrAlreadyDeleted):
		s.respondEditConflict(w, r, message)
	default:
		s.serverError(w, r, err)
	}
}

func (s *Server) feedID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		s.notFound(w)
		return 0, false
	}
	return id, true
}

// fieldMessage is a user-facing validation message. It is a typed error so the
// sentence case and punctuation stay pleasant in the form (ST1005 applies to
// errors.New-style programmer errors, not deliberate UI copy).
type fieldMessage string

func (m fieldMessage) Error() string { return string(m) }

func parseMinutesField(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	minutes, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fieldMessage("Use whole minutes.")
	}
	if minutes < -1440 || minutes > 1440 {
		return 0, fieldMessage("Keep each side under 24 hours.")
	}
	return minutes, nil
}
