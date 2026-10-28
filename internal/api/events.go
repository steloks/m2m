package api

import "net/http"

func (s *Server) handleRecordEvent(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "not implemented")
}
