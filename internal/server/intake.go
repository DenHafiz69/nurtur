package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nurtur/internal/intake"
	"nurtur/internal/timeline"
	"nurtur/internal/view"
)

func (s *Server) bottleEventRow(b intake.Bottle, unit intake.Unit, isTimeline bool) view.EventRow {
	detail := b.MilkType.Title()
	if b.Notes != "" {
		detail += " · " + b.Notes
	}
	deleteURL := fmt.Sprintf("/bottles/%d", b.ID)
	deleteTarget := "#status-strip"
	if isTimeline {
		deleteURL = fmt.Sprintf("/timeline/bottle/%d", b.ID)
		deleteTarget = "closest li"
	}
	return view.EventRow{
		Kind:         "bottle",
		ID:           b.ID,
		Anchor:       b.LoggedAt,
		TimeLabel:    view.FormatClock(b.LoggedAt, s.loc),
		Title:        "Bottle · " + intake.FormatVolume(b.AmountML, unit),
		Detail:       detail,
		Icon:         "bottle",
		EditURL:      fmt.Sprintf("/bottles/%d/sheet", b.ID),
		EditTarget:   "#action-dialog",
		DeleteURL:    deleteURL,
		DeleteTarget: deleteTarget,
		ActionLabel:  "bottle at " + view.FormatClock(b.LoggedAt, s.loc),
		KindClass:    "event-bottle",
		Timeline:     isTimeline,
		RowID:        fmt.Sprintf("row-bottle-%d", b.ID),
		DeleteKey:    fmt.Sprintf("bottle-%d", b.ID),
		DayKey:       view.DateKey(b.LoggedAt, s.loc),
	}
}

func (s *Server) pumpEventRow(p intake.Pump, unit intake.Unit, isTimeline bool) view.EventRow {
	var parts []string
	if p.AmountML > 0 {
		parts = append(parts, intake.FormatVolume(p.AmountML, unit))
	}
	if p.DurationMinutes > 0 {
		parts = append(parts, fmt.Sprintf("%d min", p.DurationMinutes))
	}
	if p.Notes != "" {
		parts = append(parts, p.Notes)
	}
	deleteURL := fmt.Sprintf("/pumps/%d", p.ID)
	deleteTarget := "#status-strip"
	if isTimeline {
		deleteURL = fmt.Sprintf("/timeline/pump/%d", p.ID)
		deleteTarget = "closest li"
	}
	return view.EventRow{
		Kind:         "pump",
		ID:           p.ID,
		Anchor:       p.LoggedAt,
		TimeLabel:    view.FormatClock(p.LoggedAt, s.loc),
		Title:        "Pumped · " + p.Side.Title(),
		Detail:       strings.Join(parts, " · "),
		Icon:         "pump",
		EditURL:      fmt.Sprintf("/pumps/%d/sheet", p.ID),
		EditTarget:   "#action-dialog",
		DeleteURL:    deleteURL,
		DeleteTarget: deleteTarget,
		ActionLabel:  "pump at " + view.FormatClock(p.LoggedAt, s.loc),
		KindClass:    "event-pump",
		Timeline:     isTimeline,
		RowID:        fmt.Sprintf("row-pump-%d", p.ID),
		DeleteKey:    fmt.Sprintf("pump-%d", p.ID),
		DayKey:       view.DateKey(p.LoggedAt, s.loc),
	}
}

func (s *Server) solidEventRow(sol intake.Solid, isTimeline bool) view.EventRow {
	var parts []string
	if t := sol.Portion.Title(); t != "" {
		parts = append(parts, t)
	}
	if t := sol.Reaction.Title(); t != "" && sol.Reaction != intake.ReactionNone {
		parts = append(parts, t)
	}
	if sol.Notes != "" {
		parts = append(parts, sol.Notes)
	}
	deleteURL := fmt.Sprintf("/solids/%d", sol.ID)
	deleteTarget := "#status-strip"
	if isTimeline {
		deleteURL = fmt.Sprintf("/timeline/solid/%d", sol.ID)
		deleteTarget = "closest li"
	}
	return view.EventRow{
		Kind:         "solid",
		ID:           sol.ID,
		Anchor:       sol.LoggedAt,
		TimeLabel:    view.FormatClock(sol.LoggedAt, s.loc),
		Title:        "Solids · " + sol.Food,
		Detail:       strings.Join(parts, " · "),
		Icon:         "solid",
		EditURL:      fmt.Sprintf("/solids/%d/sheet", sol.ID),
		EditTarget:   "#action-dialog",
		DeleteURL:    deleteURL,
		DeleteTarget: deleteTarget,
		ActionLabel:  "solid meal at " + view.FormatClock(sol.LoggedAt, s.loc),
		KindClass:    "event-solid",
		Timeline:     isTimeline,
		RowID:        fmt.Sprintf("row-solid-%d", sol.ID),
		DeleteKey:    fmt.Sprintf("solid-%d", sol.ID),
		DayKey:       view.DateKey(sol.LoggedAt, s.loc),
	}
}

func dayMetaWithIntake(summary timeline.DaySummary, unit intake.Unit) string {
	base := view.DayMetaLabel(summary.Diapers, summary.Sleep)
	extra := intakeSummaryLabel(summary.Bottles, summary.BottleML, summary.Pumps, summary.PumpML, summary.Solids, unit)
	if base == "" {
		return extra
	}
	if extra == "" {
		return base
	}
	return base + " · " + extra
}

func intakeSummaryLabel(bottles, bottleML, pumps, pumpML, solids int, unit intake.Unit) string {
	var parts []string
	if bottles > 0 {
		noun := "bottles"
		if bottles == 1 {
			noun = "bottle"
		}
		parts = append(parts, fmt.Sprintf("%d %s (%s)", bottles, noun, intake.FormatVolume(bottleML, unit)))
	}
	if pumps > 0 {
		noun := "pumps"
		if pumps == 1 {
			noun = "pump"
		}
		if pumpML > 0 {
			parts = append(parts, fmt.Sprintf("%d %s (%s)", pumps, noun, intake.FormatVolume(pumpML, unit)))
		} else {
			parts = append(parts, fmt.Sprintf("%d %s", pumps, noun))
		}
	}
	if solids > 0 {
		noun := "meals"
		if solids == 1 {
			noun = "meal"
		}
		parts = append(parts, fmt.Sprintf("%d %s", solids, noun))
	}
	return strings.Join(parts, " · ")
}

func (s *Server) pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		s.notFound(w)
		return 0, false
	}
	return id, true
}

func (s *Server) parseIntakeTime(raw string, now time.Time) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return now, nil
	}
	t, err := parseLocalDateTime(raw, s.loc)
	if err != nil {
		return time.Time{}, &intake.ValidationError{Field: "logged_at", Message: "Enter a valid date and time."}
	}
	return t, nil
}

// --- Bottles ---

func (s *Server) handleBottleSheet(w http.ResponseWriter, r *http.Request) {
	data, err := s.buildPage(r.Context(), nonceFrom(r.Context()), nil)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	unit := s.volumeUnit(r.Context())
	defaultAmt := "90"
	if unit == intake.UnitOZ {
		defaultAmt = "3"
	}
	data.BottleSheet = &view.BottleSheetView{
		MilkType:  string(intake.MilkBreast),
		Amount:    defaultAmt,
		Unit:      string(unit),
		TimeInput: s.clock.Now().In(s.loc).Format("2006-01-02T15:04"),
	}
	s.renderSheet(w, r, "bottle-sheet", data)
}

func (s *Server) handleCreateBottle(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, "Couldn't read that form. Try again.")
		return
	}
	now := s.clock.Now().UTC()
	unit := s.volumeUnit(r.Context())
	milk, err := intake.ParseMilkType(r.FormValue("milk_type"))
	if err != nil {
		s.badRequest(w, "That milk type isn't valid.")
		return
	}
	quickTap := r.FormValue("quick") == "1"
	var amountML int
	if rawML := strings.TrimSpace(r.FormValue("amount_ml")); rawML != "" {
		v, convErr := strconv.Atoi(rawML)
		if convErr != nil {
			s.badRequest(w, "That bottle amount isn't valid.")
			return
		}
		amountML = v
	} else {
		var parseErr error
		amountML, parseErr = intake.ParseVolumeInput(r.FormValue("amount"), unit)
		if parseErr != nil {
			s.respondBottleSheetError(w, r, milk, r.FormValue("amount"), string(unit), r.FormValue("logged_at"), r.FormValue("notes"), parseErr)
			return
		}
	}

	loggedAt, timeErr := s.parseIntakeTime(r.FormValue("logged_at"), now)
	if timeErr != nil {
		s.respondBottleSheetError(w, r, milk, r.FormValue("amount"), string(unit), r.FormValue("logged_at"), r.FormValue("notes"), timeErr)
		return
	}

	notes := strings.TrimSpace(r.FormValue("notes"))
	b, created, err := s.store.CreateBottle(r.Context(), milk, amountML, loggedAt, now, notes, quickTap)
	if err != nil {
		var ve *intake.ValidationError
		if errors.As(err, &ve) {
			s.respondBottleSheetError(w, r, milk, r.FormValue("amount"), string(unit), r.FormValue("logged_at"), notes, err)
			return
		}
		s.serverError(w, r, err)
		return
	}

	toast := &view.Toast{
		Kind:         "success",
		Message:      fmt.Sprintf("%s bottle (%s) logged.", milk.Title(), intake.FormatVolume(b.AmountML, unit)),
		ActionLabel:  "Edit",
		ActionURL:    fmt.Sprintf("/bottles/%d/sheet", b.ID),
		ActionMethod: "get",
		ActionTarget: "#action-dialog",
	}
	if !created {
		toast.Message = "Bottle was just logged."
		toast.Kind = "info"
	}
	opts := mutateResponse{
		Primary:          "status-strip",
		Toast:            toast,
		Trigger:          "sheet-saved",
		TriggerAfterSwap: true,
	}
	if created && onTimeline(r) {
		if row, rowErr := s.timelineRowByID(r.Context(), timeline.Bottle, b.ID); rowErr == nil {
			opts.Row = &row
			opts.RowMode = "insert"
		}
	}
	s.respond(w, r, opts)
}

func (s *Server) respondBottleSheetError(w http.ResponseWriter, r *http.Request, milk intake.MilkType, amount, unit, timeInput, notes string, err error) {
	sheet := &view.BottleSheetView{
		MilkType:  string(milk),
		Amount:    amount,
		Unit:      unit,
		TimeInput: timeInput,
		Notes:     notes,
		Error:     err.Error(),
	}
	var ve *intake.ValidationError
	if errors.As(err, &ve) {
		sheet.ErrorField = ve.Field
	}
	s.respond(w, r, mutateResponse{
		Status:   http.StatusUnprocessableEntity,
		Primary:  "bottle-sheet",
		Retarget: "#action-dialog",
		Reswap:   "innerHTML",
		Apply:    func(data *view.PageData) { data.BottleSheet = sheet },
	})
}

func (s *Server) handleBottleEditPage(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r)
	if !ok {
		return
	}
	b, err := s.store.GetBottle(r.Context(), id)
	if errors.Is(err, intake.ErrNotFound) {
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
	unit := s.volumeUnit(r.Context())
	data.BottleEdit = &view.BottleEditView{
		ID:        b.ID,
		MilkType:  string(b.MilkType),
		Amount:    intake.VolumeInputValue(b.AmountML, unit),
		Unit:      string(unit),
		TimeInput: b.LoggedAt.In(s.loc).Format("2006-01-02T15:04"),
		Notes:     b.Notes,
	}
	s.renderSheet(w, r, "bottle-edit", data)
}

func (s *Server) handleUpdateBottle(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, "Couldn't read that form. Try again.")
		return
	}
	now := s.clock.Now().UTC()
	unit := s.volumeUnit(r.Context())
	milk, err := intake.ParseMilkType(r.FormValue("milk_type"))
	if err != nil {
		s.badRequest(w, "That milk type isn't valid.")
		return
	}
	edit := &view.BottleEditView{
		ID:        id,
		MilkType:  string(milk),
		Amount:    strings.TrimSpace(r.FormValue("amount")),
		Unit:      string(unit),
		TimeInput: strings.TrimSpace(r.FormValue("logged_at")),
		Notes:     strings.TrimSpace(r.FormValue("notes")),
	}
	amountML, amtErr := intake.ParseVolumeInput(edit.Amount, unit)
	if amtErr != nil {
		s.respondBottleEditError(w, r, edit, amtErr)
		return
	}
	loggedAt, timeErr := s.parseIntakeTime(edit.TimeInput, now)
	if timeErr != nil {
		s.respondBottleEditError(w, r, edit, timeErr)
		return
	}
	if _, err := s.store.UpdateBottle(r.Context(), id, milk, amountML, loggedAt, now, edit.Notes); err != nil {
		if errors.Is(err, intake.ErrNotFound) {
			s.notFound(w)
			return
		}
		var ve *intake.ValidationError
		if errors.As(err, &ve) {
			s.respondBottleEditError(w, r, edit, err)
			return
		}
		s.serverError(w, r, err)
		return
	}
	toast := &view.Toast{Message: "Bottle updated.", Kind: "success"}
	if s.respondTimelineRow(w, r, timeline.Bottle, id, toast) {
		return
	}
	s.renderMutation(w, r, "status-strip", toast)
}

func (s *Server) respondBottleEditError(w http.ResponseWriter, r *http.Request, edit *view.BottleEditView, err error) {
	edit.Error = err.Error()
	var ve *intake.ValidationError
	if errors.As(err, &ve) {
		edit.ErrorField = ve.Field
	}
	s.respond(w, r, mutateResponse{
		Status:   http.StatusUnprocessableEntity,
		Primary:  "bottle-edit",
		Retarget: "#action-dialog",
		Reswap:   "innerHTML",
		Apply:    func(data *view.PageData) { data.BottleEdit = edit },
	})
}

func (s *Server) handleDeleteBottle(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r)
	if !ok {
		return
	}
	_, err := s.store.DeleteBottle(r.Context(), id, s.clock.Now())
	if errors.Is(err, intake.ErrNotFound) {
		s.notFound(w)
		return
	}
	if errors.Is(err, intake.ErrAlreadyDeleted) {
		s.renderMutation(w, r, "status-strip", &view.Toast{Message: "That bottle was already deleted.", Kind: "info"})
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.renderMutation(w, r, "status-strip", &view.Toast{
		Message:      "Bottle deleted.",
		Kind:         "success",
		ActionLabel:  "Undo",
		ActionURL:    fmt.Sprintf("/bottles/%d/restore", id),
		ActionMethod: "post",
		ActionTarget: "#status-strip",
		DurationMS:   8000,
	})
}

func (s *Server) handleRestoreBottle(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r)
	if !ok {
		return
	}
	if _, err := s.store.RestoreBottle(r.Context(), id); err != nil {
		if errors.Is(err, intake.ErrNotFound) {
			s.notFound(w)
			return
		}
		s.serverError(w, r, err)
		return
	}
	s.renderMutation(w, r, "status-strip", &view.Toast{Message: "Bottle restored.", Kind: "success"})
}

// --- Pumps ---

func (s *Server) handlePumpSheet(w http.ResponseWriter, r *http.Request) {
	data, err := s.buildPage(r.Context(), nonceFrom(r.Context()), nil)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	side, err := intake.ParseSide(r.URL.Query().Get("side"))
	if err != nil {
		side = intake.SideBoth
	}
	unit := s.volumeUnit(r.Context())
	data.PumpSheet = &view.PumpSheetView{
		Side:      string(side),
		Unit:      string(unit),
		Duration:  "15",
		TimeInput: s.clock.Now().In(s.loc).Format("2006-01-02T15:04"),
	}
	s.renderSheet(w, r, "pump-sheet", data)
}

func (s *Server) handleCreatePump(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, "Couldn't read that form. Try again.")
		return
	}
	now := s.clock.Now().UTC()
	unit := s.volumeUnit(r.Context())
	side, err := intake.ParseSide(r.FormValue("side"))
	if err != nil {
		s.badRequest(w, "That pump side isn't valid.")
		return
	}
	rawAmount := strings.TrimSpace(r.FormValue("amount"))
	rawDur := strings.TrimSpace(r.FormValue("duration_minutes"))
	timeInput := strings.TrimSpace(r.FormValue("logged_at"))
	notes := strings.TrimSpace(r.FormValue("notes"))

	var amountML int
	if rawAmount != "" {
		var amtErr error
		amountML, amtErr = intake.ParseVolumeInput(rawAmount, unit)
		if amtErr != nil {
			s.respondPumpSheetError(w, r, side, rawAmount, string(unit), rawDur, timeInput, notes, amtErr)
			return
		}
	}
	var durationMin int
	if rawDur != "" {
		d, convErr := strconv.Atoi(rawDur)
		if convErr != nil || d < 0 {
			s.respondPumpSheetError(w, r, side, rawAmount, string(unit), rawDur, timeInput, notes, &intake.ValidationError{Field: "duration_minutes", Message: "Duration must be a whole number of minutes."})
			return
		}
		durationMin = d
	}
	loggedAt, timeErr := s.parseIntakeTime(timeInput, now)
	if timeErr != nil {
		s.respondPumpSheetError(w, r, side, rawAmount, string(unit), rawDur, timeInput, notes, timeErr)
		return
	}

	p, err := s.store.CreatePump(r.Context(), side, amountML, durationMin, loggedAt, now, notes)
	if err != nil {
		var ve *intake.ValidationError
		if errors.As(err, &ve) {
			s.respondPumpSheetError(w, r, side, rawAmount, string(unit), rawDur, timeInput, notes, err)
			return
		}
		s.serverError(w, r, err)
		return
	}

	msg := fmt.Sprintf("Pump (%s) logged.", p.Side.Title())
	if p.AmountML > 0 {
		msg = fmt.Sprintf("Pumped %s (%s) logged.", intake.FormatVolume(p.AmountML, unit), p.Side.Title())
	}
	opts := mutateResponse{
		Primary: "status-strip",
		Toast: &view.Toast{
			Kind:         "success",
			Message:      msg,
			ActionLabel:  "Edit",
			ActionURL:    fmt.Sprintf("/pumps/%d/sheet", p.ID),
			ActionMethod: "get",
			ActionTarget: "#action-dialog",
		},
		Trigger:          "sheet-saved",
		TriggerAfterSwap: true,
	}
	if onTimeline(r) {
		if row, rowErr := s.timelineRowByID(r.Context(), timeline.Pump, p.ID); rowErr == nil {
			opts.Row = &row
			opts.RowMode = "insert"
		}
	}
	s.respond(w, r, opts)
}

func (s *Server) respondPumpSheetError(w http.ResponseWriter, r *http.Request, side intake.Side, amount, unit, dur, timeInput, notes string, err error) {
	sheet := &view.PumpSheetView{
		Side:      string(side),
		Amount:    amount,
		Unit:      unit,
		Duration:  dur,
		TimeInput: timeInput,
		Notes:     notes,
		Error:     err.Error(),
	}
	var ve *intake.ValidationError
	if errors.As(err, &ve) {
		sheet.ErrorField = ve.Field
	}
	s.respond(w, r, mutateResponse{
		Status:   http.StatusUnprocessableEntity,
		Primary:  "pump-sheet",
		Retarget: "#action-dialog",
		Reswap:   "innerHTML",
		Apply:    func(data *view.PageData) { data.PumpSheet = sheet },
	})
}

func (s *Server) handlePumpEditPage(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r)
	if !ok {
		return
	}
	p, err := s.store.GetPump(r.Context(), id)
	if errors.Is(err, intake.ErrNotFound) {
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
	unit := s.volumeUnit(r.Context())
	dur := ""
	if p.DurationMinutes > 0 {
		dur = strconv.Itoa(p.DurationMinutes)
	}
	data.PumpEdit = &view.PumpEditView{
		ID:        p.ID,
		Side:      string(p.Side),
		Amount:    intake.VolumeInputValue(p.AmountML, unit),
		Unit:      string(unit),
		Duration:  dur,
		TimeInput: p.LoggedAt.In(s.loc).Format("2006-01-02T15:04"),
		Notes:     p.Notes,
	}
	s.renderSheet(w, r, "pump-edit", data)
}

func (s *Server) handleUpdatePump(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, "Couldn't read that form. Try again.")
		return
	}
	now := s.clock.Now().UTC()
	unit := s.volumeUnit(r.Context())
	side, err := intake.ParseSide(r.FormValue("side"))
	if err != nil {
		s.badRequest(w, "That pump side isn't valid.")
		return
	}
	edit := &view.PumpEditView{
		ID:        id,
		Side:      string(side),
		Amount:    strings.TrimSpace(r.FormValue("amount")),
		Unit:      string(unit),
		Duration:  strings.TrimSpace(r.FormValue("duration_minutes")),
		TimeInput: strings.TrimSpace(r.FormValue("logged_at")),
		Notes:     strings.TrimSpace(r.FormValue("notes")),
	}
	var amountML int
	if edit.Amount != "" {
		var amtErr error
		amountML, amtErr = intake.ParseVolumeInput(edit.Amount, unit)
		if amtErr != nil {
			s.respondPumpEditError(w, r, edit, amtErr)
			return
		}
	}
	var durationMin int
	if edit.Duration != "" {
		d, convErr := strconv.Atoi(edit.Duration)
		if convErr != nil || d < 0 {
			s.respondPumpEditError(w, r, edit, &intake.ValidationError{Field: "duration_minutes", Message: "Duration must be a whole number of minutes."})
			return
		}
		durationMin = d
	}
	loggedAt, timeErr := s.parseIntakeTime(edit.TimeInput, now)
	if timeErr != nil {
		s.respondPumpEditError(w, r, edit, timeErr)
		return
	}
	if _, err := s.store.UpdatePump(r.Context(), id, side, amountML, durationMin, loggedAt, now, edit.Notes); err != nil {
		if errors.Is(err, intake.ErrNotFound) {
			s.notFound(w)
			return
		}
		var ve *intake.ValidationError
		if errors.As(err, &ve) {
			s.respondPumpEditError(w, r, edit, err)
			return
		}
		s.serverError(w, r, err)
		return
	}
	toast := &view.Toast{Message: "Pump updated.", Kind: "success"}
	if s.respondTimelineRow(w, r, timeline.Pump, id, toast) {
		return
	}
	s.renderMutation(w, r, "status-strip", toast)
}

func (s *Server) respondPumpEditError(w http.ResponseWriter, r *http.Request, edit *view.PumpEditView, err error) {
	edit.Error = err.Error()
	var ve *intake.ValidationError
	if errors.As(err, &ve) {
		edit.ErrorField = ve.Field
	}
	s.respond(w, r, mutateResponse{
		Status:   http.StatusUnprocessableEntity,
		Primary:  "pump-edit",
		Retarget: "#action-dialog",
		Reswap:   "innerHTML",
		Apply:    func(data *view.PageData) { data.PumpEdit = edit },
	})
}

func (s *Server) handleDeletePump(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r)
	if !ok {
		return
	}
	_, err := s.store.DeletePump(r.Context(), id, s.clock.Now())
	if errors.Is(err, intake.ErrNotFound) {
		s.notFound(w)
		return
	}
	if errors.Is(err, intake.ErrAlreadyDeleted) {
		s.renderMutation(w, r, "status-strip", &view.Toast{Message: "That pump was already deleted.", Kind: "info"})
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.renderMutation(w, r, "status-strip", &view.Toast{
		Message:      "Pump deleted.",
		Kind:         "success",
		ActionLabel:  "Undo",
		ActionURL:    fmt.Sprintf("/pumps/%d/restore", id),
		ActionMethod: "post",
		ActionTarget: "#status-strip",
		DurationMS:   8000,
	})
}

func (s *Server) handleRestorePump(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r)
	if !ok {
		return
	}
	if _, err := s.store.RestorePump(r.Context(), id); err != nil {
		if errors.Is(err, intake.ErrNotFound) {
			s.notFound(w)
			return
		}
		s.serverError(w, r, err)
		return
	}
	s.renderMutation(w, r, "status-strip", &view.Toast{Message: "Pump restored.", Kind: "success"})
}

// --- Solids ---

func (s *Server) handleSolidSheet(w http.ResponseWriter, r *http.Request) {
	data, err := s.buildPage(r.Context(), nonceFrom(r.Context()), nil)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	data.SolidSheet = &view.SolidSheetView{
		TimeInput: s.clock.Now().In(s.loc).Format("2006-01-02T15:04"),
	}
	s.renderSheet(w, r, "solid-sheet", data)
}

func (s *Server) handleCreateSolid(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, "Couldn't read that form. Try again.")
		return
	}
	now := s.clock.Now().UTC()
	food := strings.TrimSpace(r.FormValue("food"))
	portion, err := intake.ParsePortion(r.FormValue("portion"))
	if err != nil {
		s.badRequest(w, "That portion isn't valid.")
		return
	}
	reaction, err := intake.ParseReaction(r.FormValue("reaction"))
	if err != nil {
		s.badRequest(w, "That reaction isn't valid.")
		return
	}
	timeInput := strings.TrimSpace(r.FormValue("logged_at"))
	notes := strings.TrimSpace(r.FormValue("notes"))

	loggedAt, timeErr := s.parseIntakeTime(timeInput, now)
	if timeErr != nil {
		s.respondSolidSheetError(w, r, food, string(portion), string(reaction), timeInput, notes, timeErr)
		return
	}

	sol, err := s.store.CreateSolid(r.Context(), food, portion, reaction, loggedAt, now, notes)
	if err != nil {
		var ve *intake.ValidationError
		if errors.As(err, &ve) {
			s.respondSolidSheetError(w, r, food, string(portion), string(reaction), timeInput, notes, err)
			return
		}
		s.serverError(w, r, err)
		return
	}

	opts := mutateResponse{
		Primary: "status-strip",
		Toast: &view.Toast{
			Kind:         "success",
			Message:      fmt.Sprintf("%s logged.", sol.Food),
			ActionLabel:  "Edit",
			ActionURL:    fmt.Sprintf("/solids/%d/sheet", sol.ID),
			ActionMethod: "get",
			ActionTarget: "#action-dialog",
		},
		Trigger:          "sheet-saved",
		TriggerAfterSwap: true,
	}
	if onTimeline(r) {
		if row, rowErr := s.timelineRowByID(r.Context(), timeline.Solid, sol.ID); rowErr == nil {
			opts.Row = &row
			opts.RowMode = "insert"
		}
	}
	s.respond(w, r, opts)
}

func (s *Server) respondSolidSheetError(w http.ResponseWriter, r *http.Request, food, portion, reaction, timeInput, notes string, err error) {
	sheet := &view.SolidSheetView{
		Food:      food,
		Portion:   portion,
		Reaction:  reaction,
		TimeInput: timeInput,
		Notes:     notes,
		Error:     err.Error(),
	}
	var ve *intake.ValidationError
	if errors.As(err, &ve) {
		sheet.ErrorField = ve.Field
	}
	s.respond(w, r, mutateResponse{
		Status:   http.StatusUnprocessableEntity,
		Primary:  "solid-sheet",
		Retarget: "#action-dialog",
		Reswap:   "innerHTML",
		Apply:    func(data *view.PageData) { data.SolidSheet = sheet },
	})
}

func (s *Server) handleSolidEditPage(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r)
	if !ok {
		return
	}
	sol, err := s.store.GetSolid(r.Context(), id)
	if errors.Is(err, intake.ErrNotFound) {
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
	data.SolidEdit = &view.SolidEditView{
		ID:        sol.ID,
		Food:      sol.Food,
		Portion:   string(sol.Portion),
		Reaction:  string(sol.Reaction),
		TimeInput: sol.LoggedAt.In(s.loc).Format("2006-01-02T15:04"),
		Notes:     sol.Notes,
	}
	s.renderSheet(w, r, "solid-edit", data)
}

func (s *Server) handleUpdateSolid(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, "Couldn't read that form. Try again.")
		return
	}
	now := s.clock.Now().UTC()
	portion, err := intake.ParsePortion(r.FormValue("portion"))
	if err != nil {
		s.badRequest(w, "That portion isn't valid.")
		return
	}
	reaction, err := intake.ParseReaction(r.FormValue("reaction"))
	if err != nil {
		s.badRequest(w, "That reaction isn't valid.")
		return
	}
	edit := &view.SolidEditView{
		ID:        id,
		Food:      strings.TrimSpace(r.FormValue("food")),
		Portion:   string(portion),
		Reaction:  string(reaction),
		TimeInput: strings.TrimSpace(r.FormValue("logged_at")),
		Notes:     strings.TrimSpace(r.FormValue("notes")),
	}
	loggedAt, timeErr := s.parseIntakeTime(edit.TimeInput, now)
	if timeErr != nil {
		s.respondSolidEditError(w, r, edit, timeErr)
		return
	}
	if _, err := s.store.UpdateSolid(r.Context(), id, edit.Food, portion, reaction, loggedAt, now, edit.Notes); err != nil {
		if errors.Is(err, intake.ErrNotFound) {
			s.notFound(w)
			return
		}
		var ve *intake.ValidationError
		if errors.As(err, &ve) {
			s.respondSolidEditError(w, r, edit, err)
			return
		}
		s.serverError(w, r, err)
		return
	}
	toast := &view.Toast{Message: "Solid meal updated.", Kind: "success"}
	if s.respondTimelineRow(w, r, timeline.Solid, id, toast) {
		return
	}
	s.renderMutation(w, r, "status-strip", toast)
}

func (s *Server) respondSolidEditError(w http.ResponseWriter, r *http.Request, edit *view.SolidEditView, err error) {
	edit.Error = err.Error()
	var ve *intake.ValidationError
	if errors.As(err, &ve) {
		edit.ErrorField = ve.Field
	}
	s.respond(w, r, mutateResponse{
		Status:   http.StatusUnprocessableEntity,
		Primary:  "solid-edit",
		Retarget: "#action-dialog",
		Reswap:   "innerHTML",
		Apply:    func(data *view.PageData) { data.SolidEdit = edit },
	})
}

func (s *Server) handleDeleteSolid(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r)
	if !ok {
		return
	}
	_, err := s.store.DeleteSolid(r.Context(), id, s.clock.Now())
	if errors.Is(err, intake.ErrNotFound) {
		s.notFound(w)
		return
	}
	if errors.Is(err, intake.ErrAlreadyDeleted) {
		s.renderMutation(w, r, "status-strip", &view.Toast{Message: "That solid meal was already deleted.", Kind: "info"})
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.renderMutation(w, r, "status-strip", &view.Toast{
		Message:      "Solid meal deleted.",
		Kind:         "success",
		ActionLabel:  "Undo",
		ActionURL:    fmt.Sprintf("/solids/%d/restore", id),
		ActionMethod: "post",
		ActionTarget: "#status-strip",
		DurationMS:   8000,
	})
}

func (s *Server) handleRestoreSolid(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r)
	if !ok {
		return
	}
	if _, err := s.store.RestoreSolid(r.Context(), id); err != nil {
		if errors.Is(err, intake.ErrNotFound) {
			s.notFound(w)
			return
		}
		s.serverError(w, r, err)
		return
	}
	s.renderMutation(w, r, "status-strip", &view.Toast{Message: "Solid meal restored.", Kind: "success"})
}
