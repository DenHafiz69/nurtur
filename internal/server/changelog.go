package server

import (
	"context"
	"net/http"
	"net/url"

	"nurtur/internal/changelog"
	"nurtur/internal/view"
)

// settingChangelogSeen stores the last version whose release notes this
// account has read. It is deliberately account-scoped so every phone in the
// household dismisses the sheet together.
const settingChangelogSeen = "changelog_seen_version"

// attachChangelog opens the update sheet on full page renders when the running
// version is newer than the one this account has seen. Fragments and the
// changelog page itself never trigger it.
func (s *Server) attachChangelog(ctx context.Context, data *view.PageData) {
	if data.ChangelogOpen || data.View == "changelog" || data.SheetOpen || data.EditOpen {
		return
	}
	seen, _, err := s.store.Setting(ctx, settingChangelogSeen)
	if err != nil {
		// Release notes are a nicety. Never let them block the dashboard.
		s.log.Warn("could not read changelog setting", "error", err)
		return
	}
	releases := changelog.Unseen(s.version, seen)
	if len(releases) == 0 {
		return
	}
	data.Releases = releases
	data.ChangelogOpen = true
}

func (s *Server) handleChangelogPage(w http.ResponseWriter, r *http.Request) {
	data, err := s.buildPage(r.Context(), nonceFrom(r.Context()), nil)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	data.View = "changelog"
	data.ChangelogOpen = false
	data.Releases = changelog.All()
	s.renderPage(w, r, http.StatusOK, data)
}

// handleDismissChangelog records the running version as read. HTMX requests get
// a 204 plus an event that closes the sheet; a plain form post falls back to a
// redirect so the no-JS path keeps working.
func (s *Server) handleDismissChangelog(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()

	if seen := changelog.NormalizeVersion(s.version); seen != "" {
		if err := s.store.SetSettings(r.Context(), map[string]string{
			settingChangelogSeen: seen,
		}); err != nil {
			s.serverError(w, r, err)
			return
		}
	}

	if isHTMX(r) {
		w.Header().Set("HX-Trigger", "changelog-dismissed")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	target := safeNext(r.FormValue("return_to"))
	if target == "/" {
		// Referer is a reasonable fallback when the form was posted by the
		// browser rather than filled in by a full page render.
		if ref := r.Referer(); ref != "" {
			if u, err := url.Parse(ref); err == nil && u.Host == r.Host {
				target = safeNext(u.RequestURI())
			}
		}
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// changelogSeenSetting returns the value new accounts should start with so
// onboarding does not immediately show them the current release notes.
func (s *Server) changelogSeenSetting() (string, bool) {
	seen := changelog.NormalizeVersion(s.version)
	return seen, seen != ""
}
