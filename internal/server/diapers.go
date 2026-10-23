package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nurtur/internal/diaper"
	"nurtur/internal/timeline"
	"nurtur/internal/view"
)

func (s *Server) handleCreateDiaper(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, "Couldn't read that form. Try again.")
		return
	}
	kind, err := diaper.ParseKind(r.FormValue("type"))
	if err != nil {
		s.badRequest(w, "That diaper type isn't valid.")
		return
	}

	d, created, err := s.store.CreateDiaper(r.Context(), kind, s.clock.Now())
	if err != nil {
		if errors.Is(err, diaper.ErrInvalidKind) {
			s.badRequest(w, "That diaper type isn't valid.")
			return
		}
		s.serverError(w, r, err)
		return
	}

	toast := &view.Toast{
		Kind:         "success",
		ActionLabel:  "Add details",
		ActionURL:    fmt.Sprintf("/diapers/%d/sheet", d.ID),
		ActionMethod: "get",
		ActionTarget: "#action-dialog",
	}
	if created {
		toast.Message = d.Kind.Noun() + " logged."
	} else {
		toast.Message = d.Kind.Noun() + " was just logged."
		toast.Kind = "info"
	}
	opts := mutateResponse{
		Primary:          "status-strip",
		Toast:            toast,
		Trigger:          "sheet-saved",
		TriggerAfterSwap: true,
	}
	if created && onTimeline(r) {
		if row, rowErr := s.timelineRowByID(r.Context(), timeline.Diaper, d.ID); rowErr == nil {
			opts.Row = &row
			opts.RowMode = "insert"
		}
	}
	s.respond(w, r, opts)
}

func (s *Server) handleDiaperSheet(w http.ResponseWriter, r *http.Request) {
	data, err := s.buildPage(r.Context(), nonceFrom(r.Context()), nil)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	now := s.clock.Now().UTC()
	sheet, err := s.buildDiaperSheet(r, now)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	data.DiaperSheet = sheet
	s.renderSheet(w, r, "diaper-sheet", data)
}

func (s *Server) handleDiaperEditPage(w http.ResponseWriter, r *http.Request) {
	id, ok := s.diaperID(w, r)
	if !ok {
		return
	}
	d, err := s.store.GetDiaper(r.Context(), id)
	if errors.Is(err, diaper.ErrNotFound) {
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
	data.DiaperEdit = s.buildDiaperEdit(d)
	s.renderSheet(w, r, "diaper-edit", data)
}

func (s *Server) handleUpdateDiaper(w http.ResponseWriter, r *http.Request) {
	id, ok := s.diaperID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, "Couldn't read that form. Try again.")
		return
	}
	now := s.clock.Now().UTC()

	d, err := s.store.GetDiaper(r.Context(), id)
	if errors.Is(err, diaper.ErrNotFound) {
		s.notFound(w)
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}

	editView := s.buildDiaperEdit(d)
	color, colorErr := diaper.ParseColor(r.FormValue("color"))
	consistency, consistencyErr := diaper.ParseConsistency(r.FormValue("consistency"))
	editView.Color = string(color)
	editView.Consistency = string(consistency)
	editView.Notes = strings.TrimSpace(r.FormValue("notes"))
	editView.TimeInput = strings.TrimSpace(r.FormValue("logged_at"))

	start, timeErr := parseLocalDateTime(editView.TimeInput, s.loc)
	hasError := false
	if timeErr != nil {
		editView.TimeErr = "Use a valid date and time."
		hasError = true
	}
	if colorErr != nil {
		editView.ColorErr = "Pick a colour from the list."
		hasError = true
	}
	if consistencyErr != nil {
		editView.ConsistErr = "Pick a consistency from the list."
		hasError = true
	}
	if hasError {
		s.respondDiaperEditError(w, r, editView)
		return
	}

	correction := diaper.Edit{
		LoggedAt:    start,
		Color:       color,
		Consistency: consistency,
		Notes:       editView.Notes,
	}
	if err := correction.Validate(now); err != nil {
		applyDiaperValidation(editView, err)
		s.respondDiaperEditError(w, r, editView)
		return
	}
	if _, err := s.store.UpdateDiaper(r.Context(), id, correction, now); err != nil {
		var validation *diaper.ValidationError
		if errors.As(err, &validation) {
			applyDiaperValidation(editView, err)
			s.respondDiaperEditError(w, r, editView)
			return
		}
		if errors.Is(err, diaper.ErrAlreadyDeleted) || errors.Is(err, diaper.ErrNotFound) {
			s.notFound(w)
			return
		}
		s.serverError(w, r, err)
		return
	}

	if s.respondTimelineRow(w, r, timeline.Diaper, id, &view.Toast{Message: "Diaper details saved.", Kind: "success"}) {
		return
	}
	s.respond(w, r, mutateResponse{
		Primary:          "status-strip",
		Toast:            &view.Toast{Message: "Diaper details saved.", Kind: "success"},
		Trigger:          "sheet-saved",
		TriggerAfterSwap: true,
		Retarget:         "#status-strip",
		Reswap:           "outerHTML",
	})
}

func (s *Server) handleDeleteDiaper(w http.ResponseWriter, r *http.Request) {
	id, ok := s.diaperID(w, r)
	if !ok {
		return
	}
	err := s.store.DeleteDiaper(r.Context(), id, s.clock.Now())
	toast := &view.Toast{
		Message:      "Diaper deleted.",
		Kind:         "success",
		ActionLabel:  "Undo",
		ActionURL:    fmt.Sprintf("/diapers/%d/restore", id),
		ActionMethod: "post",
		ActionTarget: "#status-strip",
		DurationMS:   8000,
	}
	switch {
	case err == nil:
	case errors.Is(err, diaper.ErrAlreadyDeleted):
		toast = &view.Toast{Message: "That diaper was already deleted.", Kind: "info"}
	case errors.Is(err, diaper.ErrNotFound):
		s.notFound(w)
		return
	default:
		s.serverError(w, r, err)
		return
	}
	s.renderMutation(w, r, "status-strip", toast)
}

func (s *Server) handleRestoreDiaper(w http.ResponseWriter, r *http.Request) {
	id, ok := s.diaperID(w, r)
	if !ok {
		return
	}
	_, restored, err := s.store.RestoreDiaper(r.Context(), id)
	if err != nil {
		if errors.Is(err, diaper.ErrNotFound) {
			s.notFound(w)
			return
		}
		s.serverError(w, r, err)
		return
	}
	toast := &view.Toast{Message: "Diaper restored.", Kind: "success"}
	if !restored {
		toast = &view.Toast{Message: "That diaper is already back.", Kind: "info"}
	}
	s.renderMutation(w, r, "status-strip", toast)
}

func (s *Server) buildDiaperSheet(r *http.Request, now time.Time) (*view.DiaperSheetView, error) {
	from, to := DayBounds(now, s.loc)
	diapers, err := s.store.RecentDiapers(r.Context(), from, to, 20)
	if err != nil {
		return nil, err
	}
	lastWet, err := s.store.LastDiaper(r.Context(), diaper.Wet)
	if err != nil {
		return nil, err
	}
	lastDirty, err := s.store.LastDiaper(r.Context(), diaper.Dirty)
	if err != nil {
		return nil, err
	}

	sheet := &view.DiaperSheetView{ServerNow: now}
	for _, d := range diapers {
		detail := d.Summary()
		if detail == "" {
			detail = "No details yet"
		}
		sheet.Recent = append(sheet.Recent, view.DiaperRow{
			ID:        d.ID,
			TimeLabel: view.FormatClock(d.LoggedAt, s.loc),
			Title:     d.Kind.Title(),
			Detail:    detail,
			EditURL:   fmt.Sprintf("/diapers/%d/sheet", d.ID),
		})
	}
	if lastWet != nil {
		sheet.HasWet = true
		sheet.WetSince = lastWet.LoggedAt
		sheet.LastWet = view.FormatAgo(now, lastWet.LoggedAt)
	}
	if lastDirty != nil {
		sheet.HasDirty = true
		sheet.DirtySince = lastDirty.LoggedAt
		sheet.LastDirty = view.FormatAgo(now, lastDirty.LoggedAt)
	}
	return sheet, nil
}

func (s *Server) buildDiaperEdit(d diaper.Diaper) *view.DiaperEditView {
	edit := &view.DiaperEditView{
		ID:          d.ID,
		KindTitle:   d.Kind.Title(),
		TimeInput:   (&view.PageData{Location: s.loc}).DateInput(d.LoggedAt),
		Color:       string(d.Color),
		Consistency: string(d.Consistency),
		Notes:       d.Notes,
	}
	edit.Colors = []view.ChipOption{
		{Value: "", Label: "Not sure", Selected: d.Color == ""},
		{Value: string(diaper.ColorYellow), Label: "Yellow", Selected: d.Color == diaper.ColorYellow},
		{Value: string(diaper.ColorMustard), Label: "Mustard", Selected: d.Color == diaper.ColorMustard},
		{Value: string(diaper.ColorGreen), Label: "Green", Selected: d.Color == diaper.ColorGreen},
		{Value: string(diaper.ColorDark), Label: "Dark", Selected: d.Color == diaper.ColorDark},
	}
	edit.Consistencies = []view.ChipOption{
		{Value: "", Label: "Not sure", Selected: d.Consistency == ""},
		{Value: string(diaper.Seedy), Label: "Seedy", Selected: d.Consistency == diaper.Seedy},
		{Value: string(diaper.Liquid), Label: "Liquid", Selected: d.Consistency == diaper.Liquid},
		{Value: string(diaper.Hard), Label: "Hard", Selected: d.Consistency == diaper.Hard},
	}
	return edit
}

func (s *Server) respondDiaperEditError(w http.ResponseWriter, r *http.Request, edit *view.DiaperEditView) {
	s.respond(w, r, mutateResponse{
		Status:   http.StatusUnprocessableEntity,
		Primary:  "diaper-edit",
		Retarget: "#action-dialog",
		Reswap:   "innerHTML",
		Apply:    func(data *view.PageData) { data.DiaperEdit = edit },
	})
}

func applyDiaperValidation(edit *view.DiaperEditView, err error) {
	var validation *diaper.ValidationError
	if !errors.As(err, &validation) {
		edit.NotesErr = "Couldn't save that. Check the details and try again?"
		return
	}
	switch validation.Field {
	case "logged_at":
		edit.TimeErr = validation.Message
	case "color":
		edit.ColorErr = validation.Message
	case "consistency":
		edit.ConsistErr = validation.Message
	case "notes":
		edit.NotesErr = validation.Message
	default:
		edit.NotesErr = validation.Message
	}
}

func (s *Server) diaperID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		s.notFound(w)
		return 0, false
	}
	return id, true
}
