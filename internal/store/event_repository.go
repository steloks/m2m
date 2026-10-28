package store

import (
	"context"
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

type EventRepository struct {
	db DBTX
}

func NewEventRepository(db DBTX) *EventRepository {
	return &EventRepository{db: db}
}

// Create stores the event and reports whether it was new. An event_id the
// device already sent is left unchanged. The device must already exist.
func (r *EventRepository) Create(ctx context.Context, e Event) (bool, error) {
	tag, err := r.db.Exec(ctx,
		`INSERT INTO events (device_id, event_id, type, occurred_at, payload, received_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (device_id, event_id) DO NOTHING`,
		e.DeviceID, e.EventID, e.Type, e.OccurredAt, e.Payload, e.ReceivedAt)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
