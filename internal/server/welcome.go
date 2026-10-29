package server

import (
	"net/http"
	"net/url"
	"strings"

	"nurtur/internal/view"
)

// requireBabyProfile sends people with an incomplete baby profile to the
// one-time welcome page. It only guards the full-page screens, so HTMX
// fragments and exports keep working while the prompt is pending. This also
// covers sessions that were already signed in before the profile existed.
func (s *Server) requireBabyProfile(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A signed-out visitor on the landing page has no baby profile yet.
		// Sending them to onboarding would be both confusing and wrong.
		if s.landingRequest(r) {
			next.ServeHTTP(w, r)
			return
		}
		if !needsBabyProfilePrompt(r) {
			next.ServeHTTP(w, r)
			return
		}
		profile, err := s.currentBabyProfile(r.Context())
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		if profile.Complete {
			next.ServeHTTP(w, r)
			return
		}
		target := "/welcome?next=" + url.QueryEscape(r.URL.RequestURI())
		http.Redirect(w, r, target, http.StatusSeeOther)
	})
}

// needsBabyProfilePrompt limits the prompt to top-level page loads. A browser
// navigating to the dashboard, history, or settings is redirected; background
// fragments and form posts are not.
func needsBabyProfilePrompt(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if isHTMX(r) {
		return false
	}
	switch r.URL.Path {
	case "/", "/timeline", "/album", "/settings":
		return true
	default:
		return false
	}
}

func (s *Server) welcomePageData(r *http.Request, next string) (*view.PageData, error) {
	profile, err := s.currentBabyProfile(r.Context())
	if err != nil {
		return nil, err
	}
	birthDate := ""
	if !profile.BirthDate.IsZero() {
		birthDate = profile.BirthDate.Format("2006-01-02")
	}
	return &view.PageData{
		Nonce:         nonceFrom(r.Context()),
		Version:       s.version,
		BabyName:      profile.Name,
		BabyBirthDate: birthDate,
		BabyGender:    profile.Gender,
		MascotName:    profile.Mascot,
		MaxBirthDate:  s.todayISO(),
		Next:          next,
	}, nil
}

func (s *Server) handleWelcomePage(w http.ResponseWriter, r *http.Request) {
	next := welcomeNext(r.URL.Query().Get("next"))
	profile, err := s.currentBabyProfile(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if profile.Complete {
		// #nosec G710 -- next has passed through safeNext, which allows only
		// same-origin paths and rejects absolute or backslash-normalised targets.
		http.Redirect(w, r, next, http.StatusSeeOther)
		return
	}
	data, err := s.welcomePageData(r, next)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.renderNamed(w, r, http.StatusOK, "welcome", data)
}

func (s *Server) handleSaveWelcome(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, "Couldn't read that form. Try again.")
		return
	}
	next := welcomeNext(r.FormValue("next"))

	profile, err := s.currentBabyProfile(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	// The profile is written once. A second submission, or a replay from a
	// stale tab, must not rewrite the birth date or gender.
	if profile.Complete {
		// #nosec G710 -- next has passed through safeNext, which allows only
		// same-origin paths and rejects absolute or backslash-normalised targets.
		http.Redirect(w, r, next, http.StatusSeeOther)
		return
	}

	rawBirthDate := strings.TrimSpace(r.FormValue("baby_birth_date"))
	rawGender := strings.TrimSpace(r.FormValue("baby_gender"))
	birthDate, birthErr := parseBirthDate(rawBirthDate, s.clock.Now(), s.loc)
	gender, genderErr := parseBabyGender(rawGender)
	mascot, mascotErr := parseMascot(r.FormValue("mascot"))

	data, err := s.welcomePageData(r, next)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	data.BabyBirthDate = rawBirthDate
	data.BabyGender = rawGender
	data.MascotName = mascot
	if mascotErr != nil {
		data.MascotName = ""
	}
	switch {
	case birthErr != nil:
		data.ProfileError = birthDateMessage(birthErr)
		s.renderNamed(w, r, http.StatusUnprocessableEntity, "welcome", data)
		return
	case genderErr != nil:
		data.ProfileError = genderMessage(genderErr)
		s.renderNamed(w, r, http.StatusUnprocessableEntity, "welcome", data)
		return
	case mascotErr != nil:
		data.ProfileError = mascotMessage(mascotErr)
		s.renderNamed(w, r, http.StatusUnprocessableEntity, "welcome", data)
		return
	}

	if err := s.store.SetSettingsOnce(r.Context(), map[string]string{
		settingBabyBirthDate: birthDate.Format("2006-01-02"),
		settingBabyGender:    gender,
		settingMascot:        mascot,
	}); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.log.Info("baby profile completed")
	// #nosec G710 -- next has passed through safeNext, which allows only
	// same-origin paths and rejects absolute or backslash-normalised targets.
	http.Redirect(w, r, next, http.StatusSeeOther)
}

// welcomeNext keeps a missing or self-referential next target from bouncing
// back to the welcome page after the profile is already complete.
func welcomeNext(raw string) string {
	next := safeNext(raw)
	if next == "/welcome" || strings.HasPrefix(next, "/welcome?") || strings.HasPrefix(next, "/welcome/") {
		return "/"
	}
	return next
}
