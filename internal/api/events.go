package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/steloks/m2m/internal/event"
)

const maxBodyBytes = 64 << 10

type eventRequest struct {
	EventID    string          `json:"event_id"`
	Type       string          `json:"type"`
	OccurredAt string          `json:"occurred_at"`
	Payload    json.RawMessage `json:"payload"`
}

type eventResponse struct {
	DeviceID   string          `json:"device_id"`
	EventID    string          `json:"event_id"`
	Type       string          `json:"type"`
	OccurredAt string          `json:"occurred_at"`
	Payload    json.RawMessage `json:"payload"`
}

func (server *Server) handleRecordEvent(writer http.ResponseWriter, request *http.Request) {
	deviceID := request.PathValue("id")

	var req eventRequest
	if err := json.NewDecoder(http.MaxBytesReader(writer, request.Body, maxBodyBytes)).Decode(&req); err != nil {
		writeError(writer, http.StatusBadRequest, "body must be a JSON object with fields of the right type")
		return
	}
	if err := req.validate(); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	occurredAt, _ := time.Parse(time.RFC3339Nano, req.OccurredAt) // already checked by validate

	stored, created, err := server.store.RecordEvent(request.Context(), event.Event{
		DeviceID:   deviceID,
		EventID:    req.EventID,
		Type:       req.Type,
		OccurredAt: occurredAt,
		Payload:    req.Payload,
		ReceivedAt: server.now(),
	})
	if err != nil {
		slog.Error("record event", "device_id", deviceID, "event_id", req.EventID, "err", err)
		writeError(writer, http.StatusInternalServerError, "internal error")
		return
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(writer, status, eventResponse{
		DeviceID:   stored.DeviceID,
		EventID:    stored.EventID,
		Type:       stored.Type,
		OccurredAt: stored.OccurredAt.UTC().Format(time.RFC3339Nano),
		Payload:    stored.Payload,
	})
}
