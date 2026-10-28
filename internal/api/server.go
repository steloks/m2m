package api

import (
	"net/http"

	"github.com/steloks/m2m/internal/store"
)

type Server struct {
	mux     *http.ServeMux
	handler http.Handler
	store   *store.Store
}

func NewServer(deviceToken string, st *store.Store) *Server {
	s := &Server{mux: http.NewServeMux(), store: st}
	s.routes()
	s.handler = requireToken(deviceToken, s.mux)
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /test", s.handleTest)
	s.mux.HandleFunc("POST /devices/{id}/events", s.handleRecordEvent)
	s.mux.HandleFunc("/", s.handleNotFound)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

func (s *Server) handleTest(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleNotFound(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotFound, "not found")
}
