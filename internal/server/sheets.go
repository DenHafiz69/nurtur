package server

import (
	"net/http"

	"nurtur/internal/view"
)

func (s *Server) handleNursingSheet(w http.ResponseWriter, r *http.Request) {
	data, err := s.buildPage(r.Context(), nonceFrom(r.Context()), nil)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	sheet := &view.NursingSheetView{}
	if data.Active != nil {
		sheet.Active = true
		sheet.ID = data.Active.ID
		sheet.SideTitle = data.Active.SideTitle
		sheet.StartedAt = data.Active.StartedAt
		sheet.ServerNow = data.Active.ServerNow
		sheet.Elapsed = data.Active.Elapsed
	}
	if data.Last != nil {
		sheet.HasLast = true
		sheet.LastSide = data.Last.SideTitle
		sheet.LastAgo = view.FormatAgo(data.Now, data.Last.Since)
	}
	data.NursingSheet = sheet
	s.renderSheet(w, r, "nursing-sheet", data)
}
