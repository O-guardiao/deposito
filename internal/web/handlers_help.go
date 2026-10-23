package web

import "net/http"

// handleHelp renders the static "how it works" page.
func (s *Server) handleHelp(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, "help", "", nil)
}
