package api

import (
	"log/slog"
	"net/http"
)

// handleHealth reports whether the service can reach its database.
func (server *Server) handleHealth(writer http.ResponseWriter, request *http.Request) {
	if err := server.store.Ping(request.Context()); err != nil {
		slog.Error("health check", "err", err)
		writeError(writer, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
}
