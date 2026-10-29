package api

import (
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"time"
)

var (
	uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	eventTypes  = []string{"armed", "disarmed", "sensor_opened", "sensor_closed", "heartbeat"}
)

type eventPayload struct {
	Mode     *string `json:"mode"`
	SensorID *int    `json:"sensor_id"`
}

func (req eventRequest) validate() error {
	if req.EventID == "" {
		return errors.New("event_id is required")
	}
	if !uuidPattern.MatchString(req.EventID) {
		return errors.New("event_id must be a UUID")
	}

	if req.Type == "" {
		return errors.New("type is required")
	}
	if !slices.Contains(eventTypes, req.Type) {
		return errors.New("type must be one of armed, disarmed, sensor_opened, sensor_closed, heartbeat")
	}

	if req.OccurredAt == "" {
		return errors.New("occurred_at is required")
	}
	if _, err := time.Parse(time.RFC3339Nano, req.OccurredAt); err != nil {
		return errors.New("occurred_at must be an ISO 8601 timestamp with a time zone, e.g. 2026-09-29T14:02:00Z")
	}

	if len(req.Payload) == 0 || string(req.Payload) == "null" {
		return errors.New("payload is required")
	}
	var payload eventPayload
	if err := json.Unmarshal(req.Payload, &payload); err != nil {
		return errors.New("payload must be a JSON object with fields of the right type")
	}

	switch req.Type {
	case "armed":
		if payload.Mode == nil {
			return errors.New("payload.mode is required")
		}
		if *payload.Mode != "away" && *payload.Mode != "stay" {
			return errors.New("payload.mode must be one of away, stay")
		}
	case "sensor_opened", "sensor_closed":
		if payload.SensorID == nil {
			return errors.New("payload.sensor_id is required")
		}
		if *payload.SensorID < 1 || *payload.SensorID > 64 {
			return errors.New("payload.sensor_id must be between 1 and 64")
		}
	}
	return nil
}
