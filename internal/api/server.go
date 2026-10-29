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
	ListDevices(ctx context.Context, status string, now time.Time, offlineThreshold time.Duration) ([]device.Device, error)
	ListEvents(ctx context.Context, deviceID string, after event.Position, limit int) ([]event.Event, error)
	Ping(ctx context.Context) error
}

type Server struct {
	handler          http.Handler
	store            Store
	now              func() time.Time
	offlineThreshold time.Duration
	metrics          *metrics
}

func NewServer(deviceToken string, offlineThreshold time.Duration, st Store) *Server {
	s := &Server{store: st, now: time.Now, offlineThreshold: offlineThreshold, metrics: newMetrics()}
	s.handler = s.metrics.instrument(s.routes(deviceToken))
	return s
}

func (s *Server) routes(deviceToken string) http.Handler {
	// Routes that need the X-Device-Token header.
	protected := http.NewServeMux()
	protected.HandleFunc("GET /devices", s.handleListDevices)
	protected.HandleFunc("GET /devices/{id}", s.handleGetDevice)
	protected.HandleFunc("GET /devices/{id}/events", s.handleListEvents)
	protected.HandleFunc("POST /devices/{id}/events", s.handleRecordEvent)
	protected.HandleFunc("/", s.handleNotFound)

	// Routes open to everyone, for health checks and metric scrapers. Every
	// other path goes on to the protected routes, through the token check.
	public := http.NewServeMux()
	public.HandleFunc("GET /health", s.handleHealth)
	public.Handle("GET /metrics", s.metrics.handler())
	public.Handle("/", requireToken(deviceToken, protected))
	return public
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

func (s *Server) handleNotFound(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotFound, "not found")
}
