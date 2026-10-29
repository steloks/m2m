package api

import (
	"context"
	"net/http"
	"time"

	"github.com/steloks/m2m/internal/device"
	"github.com/steloks/m2m/internal/event"
)

type Store interface {
	RecordEvent(ctx context.Context, received event.Event) (event.Event, bool, error)
	GetDevice(ctx context.Context, deviceID string) (device.Device, error)
}

type Server struct {
	mux              *http.ServeMux
	handler          http.Handler
	store            Store
	now              func() time.Time
	offlineThreshold time.Duration
}

func NewServer(deviceToken string, offlineThreshold time.Duration, st Store) *Server {
	s := &Server{mux: http.NewServeMux(), store: st, now: time.Now, offlineThreshold: offlineThreshold}
	s.routes()
	s.handler = requireToken(deviceToken, s.mux)
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /test", s.handleTest)
	s.mux.HandleFunc("GET /devices/{id}", s.handleGetDevice)
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
