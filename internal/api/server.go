package api

import (
	"net/http"
)

type Server struct {
	mux     *http.ServeMux
	handler http.Handler
}

func NewServer(deviceToken string) *Server {
	s := &Server{mux: http.NewServeMux()}
	s.routes()
	s.handler = requireToken(deviceToken, s.mux)
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /test", s.handleTest)
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
