package store

import (
	"context"
	"time"
)

type Device struct {
	ID         string
	LastSeenAt time.Time
}

type DeviceRepository struct {
	db DBTX
}

func NewDeviceRepository(db DBTX) *DeviceRepository {
	return &DeviceRepository{db: db}
}

// Create adds the device and reports whether it was new. A device that already
// exists is left unchanged.
func (r *DeviceRepository) Create(ctx context.Context, d Device) (bool, error) {
	tag, err := r.db.Exec(ctx,
		`INSERT INTO devices (device_id, last_seen_at) VALUES ($1, $2)
		 ON CONFLICT (device_id) DO NOTHING`,
		d.ID, d.LastSeenAt)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
