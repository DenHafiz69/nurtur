package server

import (
	"net/http"

	"nurtur/internal/view"
)

// handleAlbumPage renders the fourth tab. The album is a page shell for now;
// capture, storage reads, and photo browsing arrive in later phases.
func (s *Server) handleAlbumPage(w http.ResponseWriter, r *http.Request) {
	data, err := s.buildPage(r.Context(), nonceFrom(r.Context()), nil)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	data.View = "album"
	data.Album = &view.AlbumPageView{}
	s.renderPage(w, r, http.StatusOK, data)
}

// handleAlbumPartial is the paging seam used by "Load earlier days". It is
// empty in phase 1 but already returns the same fragment the full page embeds.
func (s *Server) handleAlbumPartial(w http.ResponseWriter, r *http.Request) {
	data, err := s.buildPage(r.Context(), nonceFrom(r.Context()), nil)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	data.View = "album"
	data.Album = &view.AlbumPageView{}
	s.writeFragments(w, r, http.StatusOK, "album-days", data, false, nil, "", false, "", "")
}
