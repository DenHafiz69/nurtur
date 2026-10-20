package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nurtur/internal/sleep"
	"nurtur/internal/timeline"
	"nurtur/internal/view"
)

func (s *Server) handleSleepSheet(w http.ResponseWriter, r *http.Request) {
	data, err := s.buildPage(r.Context(), nonceFrom(r.Context()), nil)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	now := s.clock.Now().UTC()
	sheet, err := s.buildSleepSheet(r.Context(), now)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	data.SleepSheet = sheet
	s.renderSheet(w, r, "sleep-sheet", data)
}

func (s *Server) handleCreateSleep(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, "Couldn't read that form. Try again.")
		return
	}
	now := s.clock.Now().UTC()
	rawStart := strings.TrimSpace(r.FormValue("started_at"))

	if rawStart == "" {
		s.startSleep(w, r, now)
		return
	}

	rawEnd := strings.TrimSpace(r.FormValue("ended_at"))
	notes := strings.TrimSpace(r.FormValue("notes"))

	sheet, err := s.buildSleepSheet(r.Context(), now)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	sheet.StartInput = rawStart
	sheet.EndInput = rawEnd

	start, startErr := parseLocalDateTime(rawStart, s.loc)
	var end time.Time
	var endErr error
	if rawEnd == "" {
		sheet.EndErr = "Pick an end time."
	} else {
		end, endErr = parseLocalDateTime(rawEnd, s.loc)
		if endErr != nil {
			sheet.EndErr = "Use a valid date and time."
		}
	}
	if startErr != nil {
		sheet.StartErr = "Use a valid date and time."
	}
	if notes != "" && len([]rune(notes)) > sleep.MaxNotesLength {
		sheet.NotesErr = "Keep the note a little shorter."
	}
	if startErr != nil || endErr != nil || sheet.EndErr != "" || sheet.NotesErr != "" {
		s.respondSleepSheetError(w, r, sheet)
		return
	}

	if err := sleep.ValidateTimes(start, &end, now, true); err != nil {
		applySleepSheetValidation(sheet, err)
		s.respondSleepSheetError(w, r, sheet)
		return
	}

	created, err := s.store.CreateSleep(r.Context(), start, end, notes, now)
	if err != nil {
		if applySleepSheetError(sheet, err) {
			s.respondSleepSheetError(w, r, sheet)
			return
		}
		s.serverError(w, r, err)
		return
	}
	s.renderMutation(w, r, "sleep-pill", &view.Toast{
		Message: "Sleep saved: " + view.FormatDuration(created.Duration(now)) + ".",
		Kind:    "success",
	})
}

func (s *Server) startSleep(w http.ResponseWriter, r *http.Request, now time.Time) {
	_, err := s.store.StartSleep(r.Context(), now)
	var toast *view.Toast
	switch {
	case err == nil:
		toast = &view.Toast{Message: "Pip's asleep. Sleep well.", Kind: "success"}
	case errors.Is(err, sleep.ErrAlreadyActive):
		toast = &view.Toast{Message: "Pip is already asleep.", Kind: "info"}
	case errors.Is(err, sleep.ErrOverlap):
		toast = &view.Toast{Message: "That would overlap another sleep. Check the times?", Kind: "info"}
	default:
		s.serverError(w, r, err)
		return
	}
	s.renderMutation(w, r, "sleep-pill", toast)
}

func (s *Server) handleStopSleep(w http.ResponseWriter, r *http.Request) {
	id, ok := s.sleepID(w, r)
	if !ok {
		return
	}
	now := s.clock.Now()
	stopped, err := s.store.StopSleep(r.Context(), id, now)
	var toast *view.Toast
	switch {
	case err == nil:
		toast = &view.Toast{
			Message: "Awake. That was " + view.FormatDuration(stopped.Duration(now)) + ".",
			Kind:    "success",
		}
	case errors.Is(err, sleep.ErrNoActiveSleep):
		toast = &view.Toast{Message: "That sleep already ended.", Kind: "info"}
	case errors.Is(err, sleep.ErrAlreadyDeleted):
		toast = &view.Toast{Message: "That sleep was deleted.", Kind: "info"}
	case errors.Is(err, sleep.ErrNotFound):
		s.notFound(w)
		return
	default:
		s.serverError(w, r, err)
		return
	}
	s.renderMutation(w, r, "sleep-pill", toast)
}

func (s *Server) handleSleepEditPage(w http.ResponseWriter, r *http.Request) {
	id, ok := s.sleepID(w, r)
	if !ok {
		return
	}
	session, err := s.store.GetSleep(r.Context(), id)
	if errors.Is(err, sleep.ErrNotFound) {
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
	data.SleepEdit = s.buildSleepEdit(session)
	s.renderSheet(w, r, "sleep-edit", data)
}

func (s *Server) handleUpdateSleep(w http.ResponseWriter, r *http.Request) {
	id, ok := s.sleepID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, "Couldn't read that form. Try again.")
		return
	}
	now := s.clock.Now().UTC()

	session, err := s.store.GetSleep(r.Context(), id)
	if errors.Is(err, sleep.ErrNotFound) {
		s.notFound(w)
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	edit := s.buildSleepEdit(session)

	rawStart := strings.TrimSpace(r.FormValue("started_at"))
	rawEnd := strings.TrimSpace(r.FormValue("ended_at"))
	edit.StartInput = rawStart
	edit.EndInput = rawEnd
	edit.Notes = strings.TrimSpace(r.FormValue("notes"))

	start, startErr := parseLocalDateTime(rawStart, s.loc)
	if startErr != nil {
		edit.StartErr = "Use a valid date and time."
	}
	var endPtr *time.Time
	endErr := false
	if rawEnd == "" {
		if !edit.Active {
			edit.EndErr = "Pick an end time."
			endErr = true
		}
	} else {
		end, err := parseLocalDateTime(rawEnd, s.loc)
		if err != nil {
			edit.EndErr = "Use a valid date and time."
			endErr = true
		} else {
			endPtr = &end
		}
	}
	if len([]rune(edit.Notes)) > sleep.MaxNotesLength {
		edit.NotesErr = "Keep the note a little shorter."
	}
	if startErr != nil || endErr || edit.NotesErr != "" {
		s.respondSleepEditError(w, r, edit)
		return
	}

	if err := sleep.ValidateTimes(start, endPtr, now, !edit.Active); err != nil {
		applySleepEditValidation(edit, err)
		s.respondSleepEditError(w, r, edit)
		return
	}

	updated, err := s.store.UpdateSleep(r.Context(), id, sleep.Edit{
		StartedAt: start,
		EndedAt:   endPtr,
		Notes:     edit.Notes,
	}, now)
	if err != nil {
		if applySleepEditError(edit, err) {
			s.respondSleepEditError(w, r, edit)
			return
		}
		s.serverError(w, r, err)
		return
	}

	toast := &view.Toast{Message: "Sleep updated.", Kind: "success"}
	if updated.EndedAt != nil && session.EndedAt == nil {
		toast.Message = "Awake. That was " + view.FormatDuration(updated.Duration(now)) + "."
	}
	if s.respondTimelineRow(w, r, timeline.Sleep, id, toast) {
		return
	}
	s.respond(w, r, mutateResponse{
		Primary:          "sleep-pill",
		Toast:            toast,
		Trigger:          "sheet-saved",
		TriggerAfterSwap: true,
		Retarget:         "#sleep-pill",
		Reswap:           "outerHTML",
	})
}

func (s *Server) handleDeleteSleep(w http.ResponseWriter, r *http.Request) {
	id, ok := s.sleepID(w, r)
	if !ok {
		return
	}
	err := s.store.DeleteSleep(r.Context(), id, s.clock.Now())
	toast := &view.Toast{
		Message:      "Sleep deleted.",
		Kind:         "success",
		ActionLabel:  "Undo",
		ActionURL:    fmt.Sprintf("/sleeps/%d/restore", id),
		ActionMethod: "post",
		ActionTarget: "#sleep-pill",
		DurationMS:   8000,
	}
	switch {
	case err == nil:
	case errors.Is(err, sleep.ErrAlreadyDeleted):
		toast = &view.Toast{Message: "That sleep was already deleted.", Kind: "info"}
	case errors.Is(err, sleep.ErrNotFound):
		s.notFound(w)
		return
	default:
		s.serverError(w, r, err)
		return
	}
	s.renderMutation(w, r, "sleep-pill", toast)
}

func (s *Server) handleRestoreSleep(w http.ResponseWriter, r *http.Request) {
	id, ok := s.sleepID(w, r)
	if !ok {
		return
	}
	_, restored, err := s.store.RestoreSleep(r.Context(), id, s.clock.Now())
	var toast *view.Toast
	switch {
	case err == nil && restored:
		toast = &view.Toast{Message: "Sleep restored.", Kind: "success"}
	case err == nil:
		toast = &view.Toast{Message: "That sleep is already back.", Kind: "info"}
	case errors.Is(err, sleep.ErrOverlap):
		toast = &view.Toast{Message: "Can't undo that — it would overlap another sleep.", Kind: "info"}
	case errors.Is(err, sleep.ErrNotFound):
		s.notFound(w)
		return
	default:
		s.serverError(w, r, err)
		return
	}
	s.renderMutation(w, r, "sleep-pill", toast)
}

func (s *Server) buildSleepSheet(ctx context.Context, now time.Time) (*view.SleepSheetView, error) {
	from, to := DayBounds(now, s.loc)
	sessions, err := s.store.SleepsBetween(ctx, from, to, now, 500)
	if err != nil {
		return nil, err
	}
	nightStart, nightEnd := s.nightWindow(ctx)
	sheet := &view.SleepSheetView{ServerNow: now}
	page := &view.PageData{Location: s.loc}
	sheet.StartInput = page.DateInput(now.Add(-time.Hour))
	sheet.EndInput = page.DateInput(now)
	split := sleep.SplitRange(sessions, from, to, now, s.loc, nightStart, nightEnd)
	sheet.TotalLabel = view.FormatDuration(split.Total)
	sheet.DayLabel = view.FormatDuration(split.Day)
	sheet.NightLabel = view.FormatDuration(split.Night)

	active, err := s.store.ActiveSleep(ctx)
	if err != nil {
		return nil, err
	}
	if active != nil {
		sheet.Active = true
		sheet.ID = active.ID
		sheet.StartedAt = active.StartedAt
		sheet.Elapsed = active.Duration(now)
	}

	if len(sessions) > 20 {
		sessions = sessions[:20]
	}
	for _, session := range sessions {
		activeSession := session.EndedAt == nil
		detail := view.FormatDuration(session.Duration(now))
		if activeSession {
			detail += " so far"
		} else {
			detail += " · " + view.FormatRange(session.StartedAt, *session.EndedAt, s.loc)
		}
		title := "Slept"
		if activeSession {
			title = "Sleeping"
		}
		sheet.Recent = append(sheet.Recent, view.SleepRow{
			ID:        session.ID,
			TimeLabel: view.FormatClock(session.StartedAt, s.loc),
			Title:     title,
			Detail:    detail,
			Active:    activeSession,
			EditURL:   fmt.Sprintf("/sleeps/%d/sheet", session.ID),
		})
	}
	sheet.HasRecent = len(sheet.Recent) > 0
	return sheet, nil
}

func (s *Server) buildSleepEdit(session sleep.Session) *view.SleepEditView {
	page := &view.PageData{Location: s.loc}
	edit := &view.SleepEditView{
		ID:         session.ID,
		Active:     session.EndedAt == nil,
		StartInput: page.DateInput(session.StartedAt),
		Notes:      session.Notes,
	}
	if session.EndedAt != nil {
		edit.EndInput = page.DateInput(*session.EndedAt)
	}
	return edit
}

func (s *Server) respondSleepSheetError(w http.ResponseWriter, r *http.Request, sheet *view.SleepSheetView) {
	s.respond(w, r, mutateResponse{
		Status:   http.StatusUnprocessableEntity,
		Primary:  "sleep-sheet",
		Retarget: "#action-dialog",
		Reswap:   "innerHTML",
		Apply:    func(data *view.PageData) { data.SleepSheet = sheet },
	})
}

func (s *Server) respondSleepEditError(w http.ResponseWriter, r *http.Request, edit *view.SleepEditView) {
	s.respond(w, r, mutateResponse{
		Status:   http.StatusUnprocessableEntity,
		Primary:  "sleep-edit",
		Retarget: "#action-dialog",
		Reswap:   "innerHTML",
		Apply:    func(data *view.PageData) { data.SleepEdit = edit },
	})
}

func applySleepSheetValidation(sheet *view.SleepSheetView, err error) {
	var validation *sleep.ValidationError
	if errors.As(err, &validation) {
		switch validation.Field {
		case "started_at":
			sheet.StartErr = validation.Message
		case "ended_at":
			sheet.EndErr = validation.Message
		case "notes":
			sheet.NotesErr = validation.Message
		}
	}
}

func applySleepSheetError(sheet *view.SleepSheetView, err error) bool {
	switch {
	case errors.Is(err, sleep.ErrOverlap):
		sheet.EndErr = "That overlaps another sleep. Adjust the times?"
		return true
	case errors.Is(err, sleep.ErrAlreadyActive):
		sheet.EndErr = "A sleep is already running. Wake up first."
		return true
	}
	var validation *sleep.ValidationError
	if errors.As(err, &validation) {
		applySleepSheetValidation(sheet, err)
		if sheet.StartErr == "" && sheet.EndErr == "" && sheet.NotesErr == "" {
			sheet.EndErr = validation.Message
		}
		return true
	}
	return false
}

func applySleepEditValidation(edit *view.SleepEditView, err error) {
	var validation *sleep.ValidationError
	if errors.As(err, &validation) {
		switch validation.Field {
		case "started_at":
			edit.StartErr = validation.Message
		case "ended_at":
			edit.EndErr = validation.Message
		case "notes":
			edit.NotesErr = validation.Message
		}
	}
}

func applySleepEditError(edit *view.SleepEditView, err error) bool {
	switch {
	case errors.Is(err, sleep.ErrOverlap):
		edit.EndErr = "That overlaps another sleep. Adjust the times?"
		return true
	case errors.Is(err, sleep.ErrAlreadyActive):
		edit.EndErr = "A sleep is already running. Wake up first."
		return true
	case errors.Is(err, sleep.ErrNoActiveSleep):
		edit.EndErr = "That sleep already ended. Reload and try again."
		return true
	}
	var validation *sleep.ValidationError
	if errors.As(err, &validation) {
		applySleepEditValidation(edit, err)
		if edit.StartErr == "" && edit.EndErr == "" && edit.NotesErr == "" {
			edit.EndErr = validation.Message
		}
		return true
	}
	return false
}

func (s *Server) sleepID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		s.notFound(w)
		return 0, false
	}
	return id, true
}
