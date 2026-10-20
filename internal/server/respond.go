package server

import (
	"bytes"
	"errors"
	"net/http"

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
	Apply            func(*view.PageData)
}

func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
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
		http.Redirect(w, r, "/", http.StatusSeeOther)
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
	if opts.Apply != nil {
		opts.Apply(data)
	}

	oob := mutationOOB(opts.Primary)
	if opts.Status >= 400 {
		oob = nil
	}
	s.writeFragments(w, r, opts.Status, opts.Primary, data, opts.TimerOOB, oob, opts.Trigger, opts.TriggerAfterSwap, opts.Retarget, opts.Reswap)
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
	var buf bytes.Buffer
	if err := s.renderer.Execute(&buf, "layout", data); err != nil {
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
