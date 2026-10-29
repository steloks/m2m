package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/steloks/m2m/internal/device"
	"github.com/steloks/m2m/internal/event"
)

type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) GetDevice(ctx context.Context, deviceID string) (device.Device, error) {
	return device.NewRepository(s.pool).Get(ctx, deviceID)
}

// RecordEvent creates the device if needed, stores the event and, if the event
// is new, applies it to the device's state, in one transaction. It returns the
// stored event and reports whether it was new.
func (s *Store) RecordEvent(ctx context.Context, received event.Event) (event.Event, bool, error) {
	var stored event.Event
	var created bool
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		devices := device.NewRepository(tx)
		if _, err := devices.Create(ctx, device.Device{ID: received.DeviceID, LastSeenAt: received.ReceivedAt}); err != nil {
			return err
		}
		current, err := devices.GetForUpdate(ctx, received.DeviceID)
		if err != nil {
			return err
		}

		events := event.NewRepository(tx)
		created, err = events.Create(ctx, received)
		if err != nil {
			return err
		}
		stored, err = events.Get(ctx, received.DeviceID, received.EventID)
		if err != nil || !created {
			return err
		}
		if err := current.Apply(received); err != nil {
			return err
		}
		return devices.Update(ctx, current)
	})
	return stored, created, err
}
