package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/steloks/m2m/internal/store"
)

const maxBodyBytes = 64 << 10

type eventRequest struct {
	EventID    string          `json:"event_id"`
	Type       string          `json:"type"`
	OccurredAt time.Time       `json:"occurred_at"`
	Payload    json.RawMessage `json:"payload"`
}

type eventResponse struct {
	DeviceID   string          `json:"device_id"`
	EventID    string          `json:"event_id"`
	Type       string          `json:"type"`
	OccurredAt string          `json:"occurred_at"`
	Payload    json.RawMessage `json:"payload"`
}

func (s *Server) handleRecordEvent(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("id")

	var req eventRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "body must be a valid JSON event")
		return
	}

	created, err := s.store.RecordEvent(r.Context(), store.Event{
		DeviceID:   deviceID,
		EventID:    req.EventID,
		Type:       req.Type,
		OccurredAt: req.OccurredAt,
		Payload:    req.Payload,
		ReceivedAt: time.Now(),
	})
	if err != nil {
		slog.Error("record event", "device_id", deviceID, "event_id", req.EventID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, eventResponse{
		DeviceID:   deviceID,
		EventID:    req.EventID,
		Type:       req.Type,
		OccurredAt: req.OccurredAt.UTC().Format(time.RFC3339Nano),
		Payload:    req.Payload,
	})
}
