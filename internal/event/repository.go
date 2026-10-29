package event

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

type Repository struct {
	db DBTX
}

func NewRepository(db DBTX) *Repository {
	return &Repository{db: db}
}

// Create stores the event and reports whether it was new. An event_id the
// device already sent is left unchanged. The device must already exist.
func (r *Repository) Create(ctx context.Context, e Event) (bool, error) {
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

func (r *Repository) Get(ctx context.Context, deviceID, eventID string) (Event, error) {
	stored := Event{DeviceID: deviceID}
	err := r.db.QueryRow(ctx,
		`SELECT seq, event_id, type, occurred_at, payload, received_at
		 FROM events WHERE device_id = $1 AND event_id = $2`,
		deviceID, eventID).Scan(&stored.Seq, &stored.EventID, &stored.Type, &stored.OccurredAt, &stored.Payload, &stored.ReceivedAt)
	return stored, err
}

// List returns up to limit of the device's events that come after the given
// position, oldest first.
func (r *Repository) List(ctx context.Context, deviceID string, after Position, limit int) ([]Event, error) {
	rows, err := r.db.Query(ctx,
		`SELECT seq, event_id, type, occurred_at, payload, received_at
		 FROM events
		 WHERE device_id = $1 AND (occurred_at, seq) > ($2, $3)
		 ORDER BY occurred_at, seq
		 LIMIT $4`,
		deviceID, after.OccurredAt, after.Seq, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Event, error) {
		stored := Event{DeviceID: deviceID}
		err := row.Scan(&stored.Seq, &stored.EventID, &stored.Type, &stored.OccurredAt, &stored.Payload, &stored.ReceivedAt)
		return stored, err
	})
}
