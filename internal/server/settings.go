package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"nurtur/internal/view"
)

const (
	settingNightStart       = "night_start"
	settingNightEnd         = "night_end"
	settingBabyName         = "baby_name"
	settingBabyBirthDate    = "baby_birth_date"
	settingBabyGender       = "baby_gender"
	settingMascot           = "mascot"
	settingFeedAlertMinutes = "feed_alert_minutes"

	babyGenderGirl = "girl"
	babyGenderBoy  = "boy"

	// defaultFeedAlertMinutes is the nursing ping threshold when an account has
	// never saved one. Zero stored minutes turns the ping off.
	defaultFeedAlertMinutes = 10
	maxFeedAlertMinutes     = 120

	// maxBabyAgeYears keeps an obvious typo from saving a birth date that would
	// render a five-year-old as a "newborn" or an adult as a baby.
	maxBabyAgeYears = 5
)

var (
	errBabyNameRequired  = errors.New("baby name is required")
	errBabyNameTooLong   = errors.New("baby name must be 40 characters or fewer")
	errBirthDateRequired = errors.New("baby birth date is required")
	errBirthDateInvalid  = errors.New("baby birth date must be a valid date")
	errBirthDateFuture   = errors.New("baby birth date cannot be in the future")
	errBirthDateTooOld   = errors.New("baby birth date must be within the last 5 years")
	errGenderRequired    = errors.New("baby gender is required")
	errGenderInvalid     = errors.New("baby gender must be girl or boy")
	errMascotInvalid     = errors.New("mascot must be pip or cat")
	errFeedAlertInvalid  = errors.New("feed alert minutes must be a whole number from 0 to 120")
)

// babyProfile is the account's one baby. BirthDate and Gender are written once
// during onboarding and never edited.
type babyProfile struct {
	Name      string
	BirthDate time.Time
	Gender    string
	Mascot    string
	Complete  bool
}

// currentBabyName returns the name saved for the account bound to ctx,
// falling back to the environment value for installs that predate onboarding.
// The environment is never the primary source once a name is stored.
func (s *Server) currentBabyName(ctx context.Context) string {
	name := strings.TrimSpace(s.defaultBabyName)
	if raw, ok, err := s.store.Setting(ctx, settingBabyName); err == nil && ok {
		if stored := strings.TrimSpace(raw); stored != "" {
			name = stored
		}
	}
	return name
}

// currentBabyProfile loads the account's baby details. Complete is false when
// either the birth date or gender is missing, which sends existing accounts to
// the one-time welcome page.
func (s *Server) currentBabyProfile(ctx context.Context) (babyProfile, error) {
	values, err := s.store.Settings(ctx)
	if err != nil {
		return babyProfile{}, fmt.Errorf("server: load baby profile: %w", err)
	}

	profile := babyProfile{Name: strings.TrimSpace(s.defaultBabyName)}
	if stored := strings.TrimSpace(values[settingBabyName]); stored != "" {
		profile.Name = stored
	}
	if raw := strings.TrimSpace(values[settingBabyBirthDate]); raw != "" {
		if born, err := time.ParseInLocation("2006-01-02", raw, s.loc); err == nil {
			profile.BirthDate = born
		}
	}
	switch values[settingBabyGender] {
	case babyGenderGirl, babyGenderBoy:
		profile.Gender = values[settingBabyGender]
	}
	if mascot, err := parseMascot(values[settingMascot]); err == nil {
		profile.Mascot = mascot
	} else {
		profile.Mascot = view.MascotPip
	}
	profile.Complete = !profile.BirthDate.IsZero() && profile.Gender != ""
	return profile, nil
}

// todayISO is the local calendar date used as the max value on date inputs.
func (s *Server) todayISO() string {
	return s.clock.Now().In(s.loc).Format("2006-01-02")
}

// parseBirthDate accepts a calendar date and rejects missing, malformed,
// future, and implausibly old values.
func parseBirthDate(raw string, now time.Time, loc *time.Location) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, errBirthDateRequired
	}
	parsed, err := time.ParseInLocation("2006-01-02", raw, loc)
	if err != nil {
		return time.Time{}, errBirthDateInvalid
	}
	localNow := now.In(loc)
	today := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc)
	born := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, loc)
	if born.After(today) {
		return time.Time{}, errBirthDateFuture
	}
	if born.Before(today.AddDate(-maxBabyAgeYears, 0, 0)) {
		return time.Time{}, errBirthDateTooOld
	}
	return born, nil
}

// parseBabyGender normalizes the limited set of choices the form offers.
func parseBabyGender(raw string) (string, error) {
	gender := strings.ToLower(strings.TrimSpace(raw))
	switch gender {
	case "":
		return "", errGenderRequired
	case babyGenderGirl, babyGenderBoy:
		return gender, nil
	default:
		return "", errGenderInvalid
	}
}

// babyGenderLabel renders the stored value for display.
func babyGenderLabel(gender string) string {
	switch gender {
	case babyGenderGirl:
		return "Girl"
	case babyGenderBoy:
		return "Boy"
	default:
		return ""
	}
}

// birthDateMessage renders a validation error as copy for the signup and
// welcome forms.
func birthDateMessage(err error) string {
	switch {
	case errors.Is(err, errBirthDateRequired):
		return "Baby's date of birth is required."
	case errors.Is(err, errBirthDateFuture):
		return "The date of birth can't be in the future."
	case errors.Is(err, errBirthDateTooOld):
		return "That date is more than 5 years ago. Check the year?"
	default:
		return "That date doesn't look right. Try again?"
	}
}

// genderMessage renders a validation error as form copy.
func genderMessage(err error) string {
	switch {
	case errors.Is(err, errGenderRequired):
		return "Choose the baby's gender."
	default:
		return "Choose Girl or Boy."
	}
}

// parseMascot normalizes the mascot choice. An empty value keeps the original
// Pip default so older installs and forms without a picker still work.
func parseMascot(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", view.MascotPip:
		return view.MascotPip, nil
	case view.MascotCat:
		return view.MascotCat, nil
	default:
		return "", errMascotInvalid
	}
}

// mascotMessage renders a parseMascot error as copy for the forms.
func mascotMessage(err error) string {
	if errors.Is(err, errMascotInvalid) {
		return "Choose " + view.MascotLabel(view.MascotPip) + " or " + view.MascotLabel(view.MascotCat) + "."
	}
	return "That mascot doesn't look right. Try again?"
}

// currentMascot returns the account's mascot, falling back to Pip when the
// stored value is missing or unreadable.
func (s *Server) currentMascot(ctx context.Context) string {
	raw, ok, err := s.store.Setting(ctx, settingMascot)
	if err != nil || !ok {
		return view.MascotPip
	}
	mascot, err := parseMascot(raw)
	if err != nil {
		return view.MascotPip
	}
	return mascot
}

// parseBabyName trims and collapses whitespace so a name typed with stray
// spaces is stored cleanly.
func parseBabyName(raw string) (string, error) {
	name := strings.Join(strings.Fields(raw), " ")
	if name == "" {
		return "", errBabyNameRequired
	}
	if utf8.RuneCountInString(name) > 40 {
		return "", errBabyNameTooLong
	}
	return name, nil
}

// babyNameMessage renders a parseBabyName error as copy for the signup and
// settings forms. Error values themselves stay lower-case for linting.
func babyNameMessage(err error) string {
	switch {
	case errors.Is(err, errBabyNameRequired):
		return "Baby's name is required."
	case errors.Is(err, errBabyNameTooLong):
		return "Keep the baby's name under 40 characters."
	default:
		return "That name doesn't look right. Try again?"
	}
}

// nightWindow returns the account's configured night window, preferring stored
// settings over the environment defaults.
func (s *Server) nightWindow(ctx context.Context) (time.Duration, time.Duration) {
	start, end := s.nightStart, s.nightEnd
	values, err := s.store.Settings(ctx)
	if err != nil {
		return start, end
	}
	if raw := values[settingNightStart]; raw != "" {
		if d, err := parseClockValue(raw); err == nil {
			start = d
		}
	}
	if raw := values[settingNightEnd]; raw != "" {
		if d, err := parseClockValue(raw); err == nil {
			end = d
		}
	}
	return start, end
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

// feedAlertMinutes returns the account's nursing ping threshold in minutes.
// Zero disables the ping. A missing or unreadable value falls back to the
// default so an older install still gets a sensible alert.
func (s *Server) feedAlertMinutes(ctx context.Context) int {
	raw, ok, err := s.store.Setting(ctx, settingFeedAlertMinutes)
	if err != nil || !ok {
		return defaultFeedAlertMinutes
	}
	minutes, err := parseFeedAlertMinutes(raw)
	if err != nil {
		return defaultFeedAlertMinutes
	}
	return minutes
}

// parseFeedAlertMinutes accepts zero (off) through maxFeedAlertMinutes.
func parseFeedAlertMinutes(raw string) (int, error) {
	minutes, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || minutes < 0 || minutes > maxFeedAlertMinutes {
		return 0, errFeedAlertInvalid
	}
	return minutes, nil
}

// feedAlertMessage renders a parse error as form copy.
func feedAlertMessage(err error) string {
	if errors.Is(err, errFeedAlertInvalid) {
		return "Enter 0 to turn alerts off, or a whole number from 1 to 120."
	}
	return "That alert setting doesn't look right. Try again?"
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
		settings.BabyErr = babyNameMessage(nameErr)
		s.respondSettings(w, r, http.StatusUnprocessableEntity, settings, nil)
		return
	}
	if err := s.store.SetSettings(r.Context(), map[string]string{settingBabyName: name}); err != nil {
		s.serverError(w, r, err)
		return
	}
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

func (s *Server) handleSaveFeedAlert(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, "Couldn't read that form. Try again.")
		return
	}
	minutes, parseErr := parseFeedAlertMinutes(r.FormValue("feed_alert_minutes"))
	settings, err := s.buildSettings(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if parseErr != nil {
		settings.FeedAlertErr = feedAlertMessage(parseErr)
		s.respondSettings(w, r, http.StatusUnprocessableEntity, settings, nil)
		return
	}
	if err := s.store.SetSettings(r.Context(), map[string]string{
		settingFeedAlertMinutes: strconv.Itoa(minutes),
	}); err != nil {
		s.serverError(w, r, err)
		return
	}
	settings, err = s.buildSettings(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.respondSettings(w, r, http.StatusOK, settings, &view.Toast{
		Message: "Nursing alert saved.",
		Kind:    "success",
	})
}

func (s *Server) handleSaveMascot(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, "Couldn't read that form. Try again.")
		return
	}
	mascot, parseErr := parseMascot(r.FormValue("mascot"))
	settings, err := s.buildSettings(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if parseErr != nil {
		settings.MascotErr = mascotMessage(parseErr)
		s.respondSettings(w, r, http.StatusUnprocessableEntity, settings, nil)
		return
	}
	if err := s.store.SetSettings(r.Context(), map[string]string{settingMascot: mascot}); err != nil {
		s.serverError(w, r, err)
		return
	}
	settings, err = s.buildSettings(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.respondSettings(w, r, http.StatusOK, settings, &view.Toast{
		Message: "Mascot saved.",
		Kind:    "success",
	}, "header-mascot-oob")
}

func (s *Server) respondSettings(w http.ResponseWriter, r *http.Request, status int, settings *view.SettingsView, toast *view.Toast, extraOOB ...string) {
	oob := append([]string{"baby-heading-oob"}, extraOOB...)
	s.respond(w, r, mutateResponse{
		Status:   status,
		Primary:  "settings-panel",
		Toast:    toast,
		Retarget: "#settings-panel",
		Reswap:   "outerHTML",
		Redirect: "/settings",
		// The header's mascot and name/age live outside #settings-panel, so
		// refresh them out-of-band instead of waiting for the next full page.
		ExtraOOB: oob,
		Apply: func(data *view.PageData) {
			data.View = "settings"
			data.Settings = settings
			data.MascotName = settings.Mascot
		},
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
		Mascot:      s.currentMascot(ctx),

		FeedAlertMinutes: s.feedAlertMinutes(ctx),

		SyncMode:   "Local only",
		SyncStatus: "No cloud copy configured",
		LastPush:   "—",
		LastPull:   "—",
		Pending:    "—",
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
