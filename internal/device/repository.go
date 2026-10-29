package device

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type Repository struct {
	db DBTX
}

func NewRepository(db DBTX) *Repository {
	return &Repository{db: db}
}

// Create adds the device and reports whether it was new. A device that already
// exists is left unchanged.
func (r *Repository) Create(ctx context.Context, d Device) (bool, error) {
	tag, err := r.db.Exec(ctx,
		`INSERT INTO devices (device_id, last_seen_at) VALUES ($1, $2)
		 ON CONFLICT (device_id) DO NOTHING`,
		d.ID, d.LastSeenAt)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

const selectDevice = `SELECT last_seen_at, arm_state, arm_mode, open_sensors FROM devices WHERE device_id = $1`

// Get returns ErrNotFound for a device that has never sent an event.
func (r *Repository) Get(ctx context.Context, deviceID string) (Device, error) {
	return r.get(ctx, selectDevice, deviceID)
}

// GetForUpdate reads the device and locks its row until the transaction ends,
// so concurrent events for the same device are applied one at a time.
func (r *Repository) GetForUpdate(ctx context.Context, deviceID string) (Device, error) {
	return r.get(ctx, selectDevice+` FOR UPDATE`, deviceID)
}

func (r *Repository) get(ctx context.Context, query, deviceID string) (Device, error) {
	device := Device{ID: deviceID}
	err := r.db.QueryRow(ctx, query, deviceID).
		Scan(&device.LastSeenAt, &device.ArmState, &device.ArmMode, &device.OpenSensors)
	if errors.Is(err, pgx.ErrNoRows) {
		return Device{}, ErrNotFound
	}
	return device, err
}

func (r *Repository) Update(ctx context.Context, device Device) error {
	openSensors := device.OpenSensors
	if openSensors == nil {
		openSensors = []int{}
	}
	_, err := r.db.Exec(ctx,
		`UPDATE devices SET last_seen_at = $2, arm_state = $3, arm_mode = $4, open_sensors = $5
		 WHERE device_id = $1`,
		device.ID, device.LastSeenAt, device.ArmState, device.ArmMode, openSensors)
	return err
}
