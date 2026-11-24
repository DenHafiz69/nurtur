package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nurtur/internal/growth"
	"nurtur/internal/timeline"
	"nurtur/internal/view"
)

const settingGrowthUnit = "growth_unit"

func (s *Server) growthUnit(ctx context.Context) growth.Unit {
	raw, ok, err := s.store.Setting(ctx, settingGrowthUnit)
	if err != nil || !ok {
		return growth.UnitMetric
	}
	return growth.ParseUnit(raw)
}

func (s *Server) handleGrowthSheet(w http.ResponseWriter, r *http.Request) {
	data, err := s.buildPage(r.Context(), nonceFrom(r.Context()), nil)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	unit := s.growthUnit(r.Context())
	if q := strings.TrimSpace(r.URL.Query().Get("unit")); q != "" {
		unit = growth.ParseUnit(q)
	}
	data.GrowthSheet = &view.GrowthSheetView{
		Unit:      string(unit),
		TimeInput: s.clock.Now().In(s.loc).Format("2006-01-02T15:04"),
	}
	s.renderSheet(w, r, "growth-sheet", data)
}

func (s *Server) handleCreateGrowth(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, "Couldn't read that form. Try again.")
		return
	}
	now := s.clock.Now().UTC()
	unit := growth.ParseUnit(r.FormValue("unit"))
	if rawUnit := strings.TrimSpace(r.FormValue("unit")); rawUnit != "" {
		_ = s.store.SetSettings(r.Context(), map[string]string{settingGrowthUnit: string(unit)})
	}

	weightInput := strings.TrimSpace(r.FormValue("weight"))
	heightInput := strings.TrimSpace(r.FormValue("height"))
	headInput := strings.TrimSpace(r.FormValue("head"))
	timeInput := strings.TrimSpace(r.FormValue("logged_at"))
	notes := strings.TrimSpace(r.FormValue("notes"))

	loggedAt, timeErr := s.parseIntakeTime(timeInput, now)
	if timeErr != nil {
		s.respondGrowthSheetError(w, r, string(unit), weightInput, heightInput, headInput, timeInput, notes, timeErr)
		return
	}

	weightG, err := growth.ParseWeight(weightInput, unit)
	if err != nil {
		s.respondGrowthSheetError(w, r, string(unit), weightInput, heightInput, headInput, timeInput, notes, err)
		return
	}
	heightMM, err := growth.ParseLength(heightInput, "height", unit)
	if err != nil {
		s.respondGrowthSheetError(w, r, string(unit), weightInput, heightInput, headInput, timeInput, notes, err)
		return
	}
	headMM, err := growth.ParseLength(headInput, "head", unit)
	if err != nil {
		s.respondGrowthSheetError(w, r, string(unit), weightInput, heightInput, headInput, timeInput, notes, err)
		return
	}

	m, err := s.store.CreateGrowth(r.Context(), growth.Measurement{
		LoggedAt: loggedAt,
		WeightG:  weightG,
		HeightMM: heightMM,
		HeadMM:   headMM,
		Notes:    notes,
	}, now)
	if err != nil {
		var ve *growth.ValidationError
		if errors.As(err, &ve) {
			s.respondGrowthSheetError(w, r, string(unit), weightInput, heightInput, headInput, timeInput, notes, err)
			return
		}
		s.serverError(w, r, err)
		return
	}

	opts := mutateResponse{
		Primary: "status-strip",
		Toast: &view.Toast{
			Kind:         "success",
			Message:      "Growth logged.",
			ActionLabel:  "Edit",
			ActionURL:    fmt.Sprintf("/growth/%d/sheet", m.ID),
			ActionMethod: "get",
			ActionTarget: "#action-dialog",
		},
		Trigger:          "sheet-saved",
		TriggerAfterSwap: true,
	}
	if onTimeline(r) {
		if row, rowErr := s.timelineRowByID(r.Context(), timeline.Growth, m.ID); rowErr == nil {
			opts.Row = &row
			opts.RowMode = "insert"
		}
	}
	s.respond(w, r, opts)
}

func (s *Server) handleGrowthEditPage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		s.notFound(w)
		return
	}
	m, err := s.store.GetGrowth(r.Context(), id)
	if errors.Is(err, growth.ErrNotFound) {
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
	unit := s.growthUnit(r.Context())
	if q := strings.TrimSpace(r.URL.Query().Get("unit")); q != "" {
		unit = growth.ParseUnit(q)
	}
	data.GrowthEdit = &view.GrowthEditView{
		ID:          m.ID,
		Unit:        string(unit),
		WeightInput: growth.FormatWeightInput(m.WeightG, unit),
		HeightInput: growth.FormatLengthInput(m.HeightMM, unit),
		HeadInput:   growth.FormatLengthInput(m.HeadMM, unit),
		TimeInput:   m.LoggedAt.In(s.loc).Format("2006-01-02T15:04"),
		Notes:       m.Notes,
	}
	s.renderSheet(w, r, "growth-edit", data)
}

func (s *Server) handleUpdateGrowth(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		s.notFound(w)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, "Couldn't read that form. Try again.")
		return
	}
	now := s.clock.Now().UTC()
	unit := growth.ParseUnit(r.FormValue("unit"))
	if rawUnit := strings.TrimSpace(r.FormValue("unit")); rawUnit != "" {
		_ = s.store.SetSettings(r.Context(), map[string]string{settingGrowthUnit: string(unit)})
	}

	weightInput := strings.TrimSpace(r.FormValue("weight"))
	heightInput := strings.TrimSpace(r.FormValue("height"))
	headInput := strings.TrimSpace(r.FormValue("head"))
	timeInput := strings.TrimSpace(r.FormValue("logged_at"))
	notes := strings.TrimSpace(r.FormValue("notes"))

	loggedAt, timeErr := s.parseIntakeTime(timeInput, now)
	if timeErr != nil {
		s.respondGrowthEditError(w, r, id, string(unit), weightInput, heightInput, headInput, timeInput, notes, timeErr)
		return
	}

	weightG, err := growth.ParseWeight(weightInput, unit)
	if err != nil {
		s.respondGrowthEditError(w, r, id, string(unit), weightInput, heightInput, headInput, timeInput, notes, err)
		return
	}
	heightMM, err := growth.ParseLength(heightInput, "height", unit)
	if err != nil {
		s.respondGrowthEditError(w, r, id, string(unit), weightInput, heightInput, headInput, timeInput, notes, err)
		return
	}
	headMM, err := growth.ParseLength(headInput, "head", unit)
	if err != nil {
		s.respondGrowthEditError(w, r, id, string(unit), weightInput, heightInput, headInput, timeInput, notes, err)
		return
	}

	m, err := s.store.UpdateGrowth(r.Context(), id, loggedAt, weightG, heightMM, headMM, notes, now)
	if errors.Is(err, growth.ErrNotFound) {
		s.notFound(w)
		return
	}
	if err != nil {
		var ve *growth.ValidationError
		if errors.As(err, &ve) {
			s.respondGrowthEditError(w, r, id, string(unit), weightInput, heightInput, headInput, timeInput, notes, err)
			return
		}
		s.serverError(w, r, err)
		return
	}

	opts := mutateResponse{
		Primary: "status-strip",
		Toast: &view.Toast{
			Kind:    "success",
			Message: "Growth updated.",
		},
		Trigger:          "sheet-saved",
		TriggerAfterSwap: true,
	}
	if onTimeline(r) {
		if row, rowErr := s.timelineRowByID(r.Context(), timeline.Growth, m.ID); rowErr == nil {
			opts.Row = &row
			opts.RowMode = "replace"
		}
	}
	s.respond(w, r, opts)
}

func (s *Server) respondGrowthSheetError(w http.ResponseWriter, r *http.Request, unit, weightInput, heightInput, headInput, timeInput, notes string, err error) {
	sheet := &view.GrowthSheetView{
		Unit:        unit,
		WeightInput: weightInput,
		HeightInput: heightInput,
		HeadInput:   headInput,
		TimeInput:   timeInput,
		Notes:       notes,
		Error:       err.Error(),
		ErrorField:  growthErrorField(err),
	}
	s.respond(w, r, mutateResponse{
		Status:   http.StatusUnprocessableEntity,
		Primary:  "growth-sheet",
		Retarget: "#action-dialog",
		Reswap:   "innerHTML",
		Apply:    func(data *view.PageData) { data.GrowthSheet = sheet },
	})
}

func (s *Server) respondGrowthEditError(w http.ResponseWriter, r *http.Request, id int64, unit, weightInput, heightInput, headInput, timeInput, notes string, err error) {
	edit := &view.GrowthEditView{
		ID:          id,
		Unit:        unit,
		WeightInput: weightInput,
		HeightInput: heightInput,
		HeadInput:   headInput,
		TimeInput:   timeInput,
		Notes:       notes,
		Error:       err.Error(),
		ErrorField:  growthErrorField(err),
	}
	s.respond(w, r, mutateResponse{
		Status:   http.StatusUnprocessableEntity,
		Primary:  "growth-edit",
		Retarget: "#action-dialog",
		Reswap:   "innerHTML",
		Apply:    func(data *view.PageData) { data.GrowthEdit = edit },
	})
}

func growthErrorField(err error) string {
	var ve *growth.ValidationError
	if errors.As(err, &ve) {
		return ve.Field
	}
	return "logged_at"
}

func formatGrowthEventRow(m growth.Measurement, unit growth.Unit, loc *time.Location) view.EventRow {
	clockLabel := view.FormatClock(m.LoggedAt, loc)
	return view.EventRow{
		Kind:         string(timeline.Growth),
		ID:           m.ID,
		Anchor:       m.LoggedAt,
		TimeLabel:    clockLabel,
		Title:        "Growth",
		Detail:       m.Summary(unit),
		Icon:         "growth",
		EditURL:      fmt.Sprintf("/growth/%d/sheet", m.ID),
		EditTarget:   "#action-dialog",
		DeleteURL:    fmt.Sprintf("/timeline/growth/%d", m.ID),
		DeleteTarget: "#status-strip",
		ActionLabel:  "growth at " + clockLabel,
		KindClass:    "event-growth",
		DeleteKey:    fmt.Sprintf("growth-%d", m.ID),
		DayKey:       view.DateKey(m.LoggedAt, loc),
	}
}
