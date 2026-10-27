package server

import (
	"bytes"
	"errors"
	"net/http"
	"time"

	"nurtur/internal/feed"
	"nurtur/internal/view"
)

// allOOB is every companion fragment a dashboard mutation may refresh.
var allOOB = []string{
	"last-nursed-oob",
	"status-strip-oob",
	"timeline-today-oob",
	"sleep-pill-oob",
	"toast-oob",
}

var reconcileOOB = []string{
	"last-nursed-oob",
	"status-strip-oob",
	"timeline-today-oob",
	"sleep-pill-oob",
}

// mutationOOB returns the companion fragments for a primary target. The
// primary is never also sent out-of-band.
func mutationOOB(primary string) []string {
	switch primary {
	case "timer":
		return allOOB
	case "status-strip":
		return []string{"last-nursed-oob", "timeline-today-oob", "sleep-pill-oob", "toast-oob"}
	case "sleep-pill":
		return []string{"last-nursed-oob", "status-strip-oob", "timeline-today-oob", "toast-oob"}
	default:
		return allOOB
	}
}

// mutateResponse describes one HTMX (or no-JS) response.
type mutateResponse struct {
	Status           int
	Primary          string
	Toast            *view.Toast
	TimerOOB         bool
	Trigger          string
	TriggerAfterSwap bool
	Retarget         string
	Reswap           string
	Edit             *view.EditView
	Row              *view.EventRow
	RowMode          string // insert or update a history row after a quick log
	Redirect         string
	ExtraOOB         []string
	Apply            func(*view.PageData)
}

func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// wantsMainFragment reports a boosted tab navigation: htmx asks for #main and
// the rest of the shell (header, dock, tab bar) stays on the page. A history
// restore refetches without an HX-Target, but still only needs the page body.
func wantsMainFragment(r *http.Request) bool {
	if !isHTMX(r) {
		return false
	}
	return r.Header.Get("HX-Target") == "main" || r.Header.Get("HX-History-Restore-Request") == "true"
}

func (s *Server) respond(w http.ResponseWriter, r *http.Request, opts mutateResponse) {
	if opts.Status == 0 {
		opts.Status = http.StatusOK
	}
	if opts.Primary == "" {
		opts.Primary = "timer"
	}

	if !isHTMX(r) {
		if opts.Status >= 400 && opts.Edit != nil {
			data, err := s.buildPage(r.Context(), nonceFrom(r.Context()), opts.Toast)
			if err != nil {
				s.serverError(w, r, err)
				return
			}
			data.Edit = opts.Edit
			data.EditOpen = true
			s.renderPage(w, r, opts.Status, data)
			return
		}
		redirect := opts.Redirect
		if redirect == "" {
			redirect = "/"
		}
		http.Redirect(w, r, redirect, http.StatusSeeOther)
		return
	}

	data, err := s.buildPage(r.Context(), nonceFrom(r.Context()), opts.Toast)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if opts.Edit != nil {
		data.Edit = opts.Edit
	}
	if opts.Row != nil {
		data.Row = opts.Row
	}
	if opts.Apply != nil {
		opts.Apply(data)
	}

	// Diaper forms target the always-present toast region so the request can
	// be sent from either page. Point dashboard responses back at the status
	// strip; the history page rewrites the response below.
	onHistory := onTimeline(r)
	if opts.Primary == "status-strip" && !onHistory {
		opts.Retarget = "#status-strip"
		opts.Reswap = "outerHTML"
	}

	// A quick log on the history page swaps the new or changed row directly
	// into today's list. The dock's original primary fragment becomes an OOB
	// companion so the timer and sleep pill still refresh in place.
	primary := opts.Primary
	oob := mutationOOB(primary)
	if opts.Status >= 400 {
		oob = nil
	} else {
		timelineHandled := false
		if onHistory && opts.Row != nil && opts.RowMode != "" {
			row := *opts.Row
			if day, err := s.timelineDayView(r.Context(), row.Anchor); err == nil {
				data.TimelineDay = day
				data.Row = &row
				switch opts.RowMode {
				case "insert":
					if r.PostFormValue("timeline_today") == "1" {
						opts.Primary = "timeline-row-fragment"
						opts.Retarget = "#timeline-events-" + row.DayKey
						opts.Reswap = "afterbegin"
						oob = append(oob, "timeline-day-oob")
						timelineHandled = true
					} else if page, err := s.buildTimeline(r.Context(), time.Time{}, defaultTimelineLimit); err == nil {
						// This page had no today group (empty history or a
						// paginated older day), so refresh the first page.
						opts.Primary = "timeline-days"
						opts.Retarget = "#timeline-list"
						opts.Reswap = "innerHTML"
						data.Timeline = page
						data.TimelineDay = nil
						timelineHandled = true
					}
				case "update":
					opts.Primary = "timeline-row-fragment"
					opts.Retarget = "#" + row.RowID
					opts.Reswap = "outerHTML"
					oob = append(oob, "timeline-day-oob")
					timelineHandled = true
				}
				if timelineHandled {
					switch primary {
					case "timer":
						opts.TimerOOB = true
					case "sleep-pill":
						oob = append(oob, "sleep-pill-oob")
					}
				}
			}
		}
		// Timer and sheet mutations can happen from the history page too. Keep
		// today's day header in sync with whatever just changed.
		if !timelineHandled && data.TimelineDay == nil && onHistory {
			if day, err := s.timelineDayView(r.Context(), s.clock.Now().UTC()); err == nil {
				data.TimelineDay = day
				opts.ExtraOOB = append(opts.ExtraOOB, "timeline-day-oob")
			}
		}
		oob = append(oob, opts.ExtraOOB...)
		if onHistory {
			if opts.Primary == "status-strip" {
				opts.Primary = "toast-primary"
				opts.Retarget = "#toast-region"
				opts.Reswap = "innerHTML"
			}
			oob = timelineOOB(oob, opts.Primary)
		}
	}
	s.writeFragments(w, r, opts.Status, opts.Primary, data, opts.TimerOOB, oob, opts.Trigger, opts.TriggerAfterSwap, opts.Retarget, opts.Reswap)
}

// timelineOOB drops companions that only exist on the dashboard.
func timelineOOB(oob []string, primary string) []string {
	if len(oob) == 0 {
		return oob
	}
	out := make([]string, 0, len(oob))
	for _, name := range oob {
		switch name {
		case "status-strip-oob", "timeline-today-oob":
			continue
		case "toast-oob":
			if primary == "toast" || primary == "toast-primary" {
				continue
			}
		}
		out = append(out, name)
	}
	return out
}

// renderMutation is the standard successful mutation response: a primary
// fragment plus the OOB companions for that target.
func (s *Server) renderMutation(w http.ResponseWriter, r *http.Request, primary string, toast *view.Toast) {
	s.respond(w, r, mutateResponse{
		Primary:          primary,
		Toast:            toast,
		Trigger:          "sheet-saved",
		TriggerAfterSwap: true,
	})
}

func (s *Server) respondEditError(w http.ResponseWriter, r *http.Request, edit *view.EditView) {
	s.respond(w, r, mutateResponse{
		Status:   http.StatusUnprocessableEntity,
		Primary:  "feed-edit",
		Edit:     edit,
		Retarget: "#feed-edit-dialog",
		Reswap:   "innerHTML",
	})
}

func (s *Server) respondEditConflict(w http.ResponseWriter, r *http.Request, message string) {
	s.respond(w, r, mutateResponse{
		Toast:            &view.Toast{Message: message, Kind: "info"},
		Trigger:          "feed-saved",
		TriggerAfterSwap: true,
		Retarget:         "#timer-card",
		Reswap:           "outerHTML",
	})
}

// renderSheet renders one dialog fragment. htmx swaps it into #action-dialog;
// a no-JS request gets the full dashboard with the sheet open.
func (s *Server) renderSheet(w http.ResponseWriter, r *http.Request, name string, data *view.PageData) {
	if isHTMX(r) {
		s.writeFragments(w, r, http.StatusOK, name, data, false, nil, "", false, "", "")
		return
	}
	data.Sheet = name
	data.SheetOpen = true
	s.renderPage(w, r, http.StatusOK, data)
}

func (s *Server) writeFragments(
	w http.ResponseWriter,
	r *http.Request,
	status int,
	primary string,
	data *view.PageData,
	timerOOB bool,
	oob []string,
	trigger string,
	triggerAfterSwap bool,
	retarget, reswap string,
) {
	var buf bytes.Buffer
	if primary != "" {
		if err := s.renderer.Execute(&buf, primary, data); err != nil {
			s.serverError(w, r, err)
			return
		}
	}
	if timerOOB {
		data.TimerOOB = true
		err := s.renderer.Execute(&buf, "timer", data)
		data.TimerOOB = false
		if err != nil {
			s.serverError(w, r, err)
			return
		}
	}
	for _, name := range oob {
		if err := s.renderer.Execute(&buf, name, data); err != nil {
			s.serverError(w, r, err)
			return
		}
	}

	header := w.Header()
	header.Set("Content-Type", "text/html; charset=utf-8")
	header.Set("Cache-Control", "no-store")
	if trigger != "" {
		if triggerAfterSwap {
			header.Set("HX-Trigger-After-Swap", trigger)
		} else {
			header.Set("HX-Trigger", trigger)
		}
	}
	if retarget != "" {
		header.Set("HX-Retarget", retarget)
	}
	if reswap != "" {
		header.Set("HX-Reswap", reswap)
	}
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) renderPage(w http.ResponseWriter, r *http.Request, status int, data *view.PageData) {
	data.Path = r.URL.RequestURI()
	s.attachChangelog(r.Context(), data)
	templateName := "layout"
	if wantsMainFragment(r) {
		// A boosted tab only needs the page body; skip the shell it already has.
		templateName = "page-main"
	}
	var buf bytes.Buffer
	if err := s.renderer.Execute(&buf, templateName, data); err != nil {
		s.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) serverError(w http.ResponseWriter, r *http.Request, err error) {
	s.log.Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	http.Error(w, "Something went wrong. Try again.", http.StatusInternalServerError)
}

func (s *Server) badRequest(w http.ResponseWriter, message string) {
	http.Error(w, message, http.StatusBadRequest)
}

func (s *Server) notFound(w http.ResponseWriter) {
	http.Error(w, "Not found.", http.StatusNotFound)
}

// applyValidation copies a feed.ValidationError onto the right field.
func applyValidation(edit *view.EditView, err error) {
	var ve *feed.ValidationError
	if !errors.As(err, &ve) {
		edit.TotalsErr = "Couldn't save that. Check the times and try again?"
		return
	}
	switch ve.Field {
	case "started_at":
		edit.StartErr = ve.Message
	case "ended_at":
		edit.EndErr = ve.Message
	case "left_minutes":
		edit.LeftErr = ve.Message
	case "right_minutes":
		edit.RightErr = ve.Message
	case "segment_started_at":
		edit.StartErr = ve.Message
	default:
		edit.TotalsErr = ve.Message
	}
}
