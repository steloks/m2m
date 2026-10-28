package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

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

// RecordEvent creates the device if needed and stores the event, in one
// transaction. It reports whether the event was new.
func (s *Store) RecordEvent(ctx context.Context, e Event) (bool, error) {
	var created bool
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		_, err := NewDeviceRepository(tx).Create(ctx, Device{ID: e.DeviceID, LastSeenAt: e.ReceivedAt})
		if err != nil {
			return err
		}
		created, err = NewEventRepository(tx).Create(ctx, e)
		return err
	})
	return created, err
}
