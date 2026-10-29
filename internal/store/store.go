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

// RecordEvent creates the device if needed, stores the event and, if the event
// is new, applies it to the device's state, in one transaction. It reports
// whether the event was new.
func (s *Store) RecordEvent(ctx context.Context, received event.Event) (bool, error) {
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

		created, err = event.NewRepository(tx).Create(ctx, received)
		if err != nil || !created {
			return err
		}
		if err := current.Apply(received); err != nil {
			return err
		}
		return devices.Update(ctx, current)
	})
	return created, err
}
