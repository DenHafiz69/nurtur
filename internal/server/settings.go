package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"nurtur/internal/view"
)

const (
	settingNightStart = "night_start"
	settingNightEnd   = "night_end"
	settingBabyName   = "baby_name"
)

// currentBabyName returns the name saved during onboarding, falling back to the
// environment value for installs that predate the onboarding field. The
// environment is never the primary source once a name is stored.
func (s *Server) currentBabyName(ctx context.Context) string {
	s.babyMu.RLock()
	if s.babyLoaded {
		name := s.babyName
		s.babyMu.RUnlock()
		return name
	}
	s.babyMu.RUnlock()

	s.babyMu.Lock()
	defer s.babyMu.Unlock()
	if !s.babyLoaded {
		name := strings.TrimSpace(s.defaultBabyName)
		if raw, ok, err := s.store.Setting(ctx, settingBabyName); err == nil && ok {
			if stored := strings.TrimSpace(raw); stored != "" {
				name = stored
			}
		}
		s.babyName = name
		s.babyLoaded = true
	}
	return s.babyName
}

func (s *Server) setBabyName(name string) {
	s.babyMu.Lock()
	s.babyName = name
	s.babyLoaded = true
	s.babyMu.Unlock()
}

// parseBabyName trims and collapses whitespace so a name typed with stray
// spaces is stored cleanly.
func parseBabyName(raw string) (string, error) {
	name := strings.Join(strings.Fields(raw), " ")
	if name == "" {
		return "", errors.New("Baby's name is required.")
	}
	if utf8.RuneCountInString(name) > 40 {
		return "", errors.New("Keep the baby's name under 40 characters.")
	}
	return name, nil
}

// nightWindow returns the configured night window, preferring the settings
// table over the environment defaults. The value is cached until it changes.
func (s *Server) nightWindow(ctx context.Context) (time.Duration, time.Duration) {
	s.nightMu.RLock()
	if s.nightLoaded {
		start, end := s.nightStart, s.nightEnd
		s.nightMu.RUnlock()
		return start, end
	}
	s.nightMu.RUnlock()

	s.nightMu.Lock()
	defer s.nightMu.Unlock()
	if !s.nightLoaded {
		values, err := s.store.Settings(ctx)
		if err == nil {
			if raw := values[settingNightStart]; raw != "" {
				if d, err := parseClockValue(raw); err == nil {
					s.nightStart = d
				}
			}
			if raw := values[settingNightEnd]; raw != "" {
				if d, err := parseClockValue(raw); err == nil {
					s.nightEnd = d
				}
			}
		}
		s.nightLoaded = true
	}
	return s.nightStart, s.nightEnd
}

func (s *Server) invalidateNightWindow() {
	s.nightMu.Lock()
	s.nightLoaded = false
	s.nightMu.Unlock()
}

func parseClockValue(raw string) (time.Duration, error) {
	parsed, err := time.Parse("15:04", strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("use HH:MM")
	}
	return time.Duration(parsed.Hour())*time.Hour + time.Duration(parsed.Minute())*time.Minute, nil
}

func formatClockValue(d time.Duration) string {
	hour := int(d / time.Hour)
	minute := int((d % time.Hour) / time.Minute)
	return fmt.Sprintf("%02d:%02d", hour, minute)
}

func (s *Server) handleSettingsPage(w http.ResponseWriter, r *http.Request) {
	data, err := s.buildPage(r.Context(), nonceFrom(r.Context()), nil)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	data.View = "settings"
	settings, err := s.buildSettings(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	data.Settings = settings
	s.renderPage(w, r, http.StatusOK, data)
}

func (s *Server) handleSaveSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, "Couldn't read that form. Try again.")
		return
	}
	startRaw := strings.TrimSpace(r.FormValue("night_start"))
	endRaw := strings.TrimSpace(r.FormValue("night_end"))

	settings, err := s.buildSettings(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	settings.NightStart = startRaw
	settings.NightEnd = endRaw

	start, startErr := parseClockValue(startRaw)
	end, endErr := parseClockValue(endRaw)
	hasError := false
	if startErr != nil {
		settings.NightErr = "Use HH:MM, like 19:00."
		hasError = true
	}
	if endErr != nil {
		settings.NightErr = "Use HH:MM, like 07:00."
		hasError = true
	}
	if !hasError && start == end {
		settings.NightErr = "Night can't start and end at the same time."
		hasError = true
	}
	if hasError {
		s.respondSettings(w, r, http.StatusUnprocessableEntity, settings, nil)
		return
	}

	if err := s.store.SetSettings(r.Context(), map[string]string{
		settingNightStart: formatClockValue(start),
		settingNightEnd:   formatClockValue(end),
	}); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.invalidateNightWindow()

	settings, err = s.buildSettings(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.respondSettings(w, r, http.StatusOK, settings, &view.Toast{
		Message: "Night window saved.",
		Kind:    "success",
	})
}

func (s *Server) handleSaveBabyName(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, "Couldn't read that form. Try again.")
		return
	}
	name, nameErr := parseBabyName(r.FormValue("baby_name"))
	settings, err := s.buildSettings(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if nameErr != nil {
		settings.BabyErr = nameErr.Error()
		s.respondSettings(w, r, http.StatusUnprocessableEntity, settings, nil)
		return
	}
	if err := s.store.SetSettings(r.Context(), map[string]string{settingBabyName: name}); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.setBabyName(name)
	settings, err = s.buildSettings(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.respondSettings(w, r, http.StatusOK, settings, &view.Toast{
		Message: "Baby name saved.",
		Kind:    "success",
	})
}

func (s *Server) respondSettings(w http.ResponseWriter, r *http.Request, status int, settings *view.SettingsView, toast *view.Toast) {
	s.respond(w, r, mutateResponse{
		Status:   status,
		Primary:  "settings-panel",
		Toast:    toast,
		Retarget: "#settings-panel",
		Reswap:   "outerHTML",
		Redirect: "/settings",
		Apply:    func(data *view.PageData) { data.View = "settings"; data.Settings = settings },
	})
}

func (s *Server) buildSettings(ctx context.Context) (*view.SettingsView, error) {
	nightStart, nightEnd := s.nightWindow(ctx)
	out := &view.SettingsView{
		BabyName:    s.currentBabyName(ctx),
		Timezone:    s.loc.String(),
		DBPath:      s.store.Path(),
		Version:     s.version,
		AuthEnabled: s.auth.enabled(),
		NightStart:  formatClockValue(nightStart),
		NightEnd:    formatClockValue(nightEnd),
		SyncMode:    "Local only",
		SyncStatus:  "No cloud copy configured",
		LastPush:    "—",
		LastPull:    "—",
		Pending:     "—",
		ThemeNote: fmt.Sprintf(
			"Theme follows the time of day (night %s–%s) unless you override it from the header.",
			formatClockValue(nightStart), formatClockValue(nightEnd)),
	}
	if info, err := os.Stat(s.store.Path()); err == nil {
		out.DBSize = view.FormatBytes(info.Size())
	} else {
		out.DBSize = "—"
	}

	if s.worker == nil {
		return out, nil
	}
	out.SyncMode = "Turso Cloud sync"
	status := s.worker.Status()
	switch {
	case status.LastPushError != "" || status.LastPullError != "":
		out.SyncStatus = "Sync issue — changes stay local"
		out.Degraded = true
	case status.Pushing || status.Dirty:
		out.SyncStatus = "Syncing…"
		out.Degraded = status.PendingAgeSeconds > 300
	default:
		out.SyncStatus = "Synced"
	}
	if status.LastPush != nil {
		out.LastPush = status.LastPush.In(s.loc).Format("Jan 2, 3:04 PM")
	} else {
		out.LastPush = "Never"
	}
	if status.LastPull != nil {
		out.LastPull = status.LastPull.In(s.loc).Format("Jan 2, 3:04 PM")
	} else {
		out.LastPull = "Never"
	}
	if status.Dirty && status.PendingAgeSeconds > 0 {
		out.Pending = (time.Duration(status.PendingAgeSeconds) * time.Second).Round(time.Second).String()
	} else {
		out.Pending = "—"
	}
	out.SyncDetail = fmt.Sprintf("last push %s · last pull %s", out.LastPush, out.LastPull)
	return out, nil
}

// syncChip is the header's short sync state. It is intentionally separate from
// network connectivity: the phone can be online while Turso is catching up.
func (s *Server) syncChip() *view.SyncChipView {
	if s.worker == nil {
		return &view.SyncChipView{
			Label: "Local",
			Tone:  "local",
			Title: "Saved on this device's server. No cloud copy configured.",
		}
	}
	status := s.worker.Status()
	switch {
	case status.LastPushError != "" || status.LastPullError != "":
		return &view.SyncChipView{Label: "Local copy", Tone: "warn", Title: "Cloud sync is having trouble. Logging still works."}
	case status.Pushing || status.Dirty:
		return &view.SyncChipView{Label: "Syncing…", Tone: "pending", Title: "Changes are saved locally and being pushed to Turso Cloud."}
	case status.LastPush == nil:
		return &view.SyncChipView{Label: "Not synced yet", Tone: "pending", Title: "Waiting for the first push to Turso Cloud."}
	default:
		return &view.SyncChipView{Label: "Synced", Tone: "ok", Title: "Local and cloud copies are caught up."}
	}
}

func (s *Server) handleSyncChip(w http.ResponseWriter, r *http.Request) {
	data := &view.PageData{SyncChip: s.syncChip()}
	s.writeFragments(w, r, http.StatusOK, "sync-chip", data, false, nil, "", false, "", "")
}

func (s *Server) handleOffline(w http.ResponseWriter, r *http.Request) {
	data := &view.PageData{Nonce: nonceFrom(r.Context()), Version: s.version}
	s.renderNamed(w, r, http.StatusOK, "offline", data)
}

func (s *Server) handleServiceWorker(w http.ResponseWriter, r *http.Request) {
	body, err := fs.ReadFile(s.assets, "web/static/sw.js")
	if err != nil {
		s.notFound(w)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Service-Worker-Allowed", "/")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
