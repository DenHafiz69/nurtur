package server

import "net/http"

// handleAlbumPage renders the fourth tab. The album is a page shell for now;
// capture, storage, and browsing arrive in later phases.
func (s *Server) handleAlbumPage(w http.ResponseWriter, r *http.Request) {
	data, err := s.buildPage(r.Context(), nonceFrom(r.Context()), nil)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	data.View = "album"
	s.renderPage(w, r, http.StatusOK, data)
}
