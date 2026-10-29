package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/steloks/m2m/internal/device"
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

const (
	defaultHistoryLimit = 50
	maxHistoryLimit     = 500
)

type historyEvent struct {
	EventID    string          `json:"event_id"`
	Type       string          `json:"type"`
	OccurredAt string          `json:"occurred_at"`
	Payload    json.RawMessage `json:"payload"`
}

type historyResponse struct {
	Events []historyEvent `json:"events"`
	Next   *string        `json:"next"`
}

func (server *Server) handleListEvents(writer http.ResponseWriter, request *http.Request) {
	deviceID := request.PathValue("id")
	after, limit, err := parseHistoryQuery(request.URL.Query())
	if err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}

	// One extra event tells whether there is a next page.
	events, err := server.store.ListEvents(request.Context(), deviceID, after, limit+1)
	if errors.Is(err, device.ErrNotFound) {
		writeError(writer, http.StatusNotFound, "device not found")
		return
	}
	if err != nil {
		slog.Error("list events", "device_id", deviceID, "err", err)
		writeError(writer, http.StatusInternalServerError, "internal error")
		return
	}

	response := historyResponse{Events: make([]historyEvent, 0, limit)}
	if len(events) > limit {
		events = events[:limit]
		next := nextPageURL(deviceID, events[len(events)-1].Position(), limit)
		response.Next = &next
	}
	for _, stored := range events {
		response.Events = append(response.Events, historyEvent{
			EventID:    stored.EventID,
			Type:       stored.Type,
			OccurredAt: stored.OccurredAt.UTC().Format(time.RFC3339Nano),
			Payload:    stored.Payload,
		})
	}
	writeJSON(writer, http.StatusOK, response)
}

func parseHistoryQuery(query url.Values) (event.Position, int, error) {
	limit := defaultHistoryLimit
	if query.Has("limit") {
		parsed, err := strconv.Atoi(query.Get("limit"))
		if err != nil || parsed < 1 || parsed > maxHistoryLimit {
			return event.Position{}, 0, errors.New("limit must be an integer between 1 and 500")
		}
		limit = parsed
	}

	if query.Has("since") && query.Has("cursor") {
		return event.Position{}, 0, errors.New("since and cursor cannot be used together")
	}
	var after event.Position
	if query.Has("since") {
		since, err := time.Parse(time.RFC3339Nano, query.Get("since"))
		if err != nil {
			return event.Position{}, 0, errors.New("since must be an ISO 8601 timestamp with a time zone, e.g. 2026-09-29T14:00:00Z")
		}
		after = event.Since(since)
	}
	if query.Has("cursor") {
		position, err := decodeCursor(query.Get("cursor"))
		if err != nil {
			return event.Position{}, 0, errors.New("cursor is invalid")
		}
		after = position
	}
	return after, limit, nil
}

func nextPageURL(deviceID string, after event.Position, limit int) string {
	query := url.Values{}
	query.Set("cursor", encodeCursor(after))
	query.Set("limit", strconv.Itoa(limit))
	return "/devices/" + url.PathEscape(deviceID) + "/events?" + query.Encode()
}

// A cursor is the position of the last event on a page, as
// base64("<occurred_at>,<seq>"). Clients should treat it as opaque.
func encodeCursor(position event.Position) string {
	raw := position.OccurredAt.UTC().Format(time.RFC3339Nano) + "," + strconv.FormatInt(position.Seq, 10)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(cursor string) (event.Position, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return event.Position{}, err
	}
	occurredAtText, seqText, found := strings.Cut(string(raw), ",")
	if !found {
		return event.Position{}, errors.New("missing seq")
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, occurredAtText)
	if err != nil {
		return event.Position{}, err
	}
	seq, err := strconv.ParseInt(seqText, 10, 64)
	if err != nil {
		return event.Position{}, err
	}
	return event.Position{OccurredAt: occurredAt, Seq: seq}, nil
}
