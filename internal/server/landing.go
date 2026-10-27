package server

import (
	"net/http"

	"nurtur/internal/view"
)

// landingRequest reports whether the root request should show the public
// landing page instead of the signed-in dashboard. Installs without a gate keep
// their zero-friction dashboard, and signed-in browsers always go straight to
// today's page.
func (s *Server) landingRequest(r *http.Request) bool {
	return s.auth != nil && s.auth.enabled() && !s.auth.signedIn(r) && publicLanding(r)
}

func (s *Server) handleLanding(w http.ResponseWriter, r *http.Request) {
	s.renderNamed(w, r, http.StatusOK, "landing", &view.PageData{
		Nonce:      nonceFrom(r.Context()),
		Version:    s.version,
		MascotName: view.MascotPip,
		SignupOpen: s.auth.signupOpen(),
	})
}
