package event

import (
	"encoding/json"
	"time"
)

type Event struct {
	DeviceID   string
	EventID    string
	Type       string
	OccurredAt time.Time
	Payload    json.RawMessage
	ReceivedAt time.Time
}
