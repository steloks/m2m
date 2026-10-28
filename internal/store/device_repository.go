package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Device struct {
	ID         string
	LastSeenAt time.Time
}

type DeviceRepository struct {
	pool *pgxpool.Pool
}

func NewDeviceRepository(s *Store) *DeviceRepository {
	return &DeviceRepository{pool: s.pool}
}

// Create adds the device and reports whether it was new. A device that already
// exists is left unchanged.
func (r *DeviceRepository) Create(ctx context.Context, d Device) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`INSERT INTO devices (device_id, last_seen_at) VALUES ($1, $2)
		 ON CONFLICT (device_id) DO NOTHING`,
		d.ID, d.LastSeenAt)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
