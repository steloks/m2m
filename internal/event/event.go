package event

import (
	"encoding/json"
	"time"
)

type Event struct {
	Seq        int64
	DeviceID   string
	EventID    string
	Type       string
	OccurredAt time.Time
	Payload    json.RawMessage
	ReceivedAt time.Time
}

// Position is a place in a device's history, which is ordered by OccurredAt
// and then by Seq, the order the events were stored in.
type Position struct {
	OccurredAt time.Time
	Seq        int64
}

func Since(t time.Time) Position {
	return Position{OccurredAt: t}
}

func (stored Event) Position() Position {
	return Position{OccurredAt: stored.OccurredAt, Seq: stored.Seq}
}
