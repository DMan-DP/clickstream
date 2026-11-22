// Package repository stores the aggregated window statistics in
// PostgreSQL.
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/DMan-DP/clickstream/internal/analytics/domain"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

// Connection lifetime limits. Idle connections are kept well above the
// flush interval, so a connection survives between ticks, but below the
// usual NAT and firewall idle timeouts. Lifetime makes the pool replace
// old connections gradually, for example after a database failover.
const (
	connMaxLifetime = 30 * time.Minute
	connMaxIdleTime = 5 * time.Minute
)

const saveCounterQuery = `
			INSERT INTO event_stats AS t (window_sec, window_start, event_type, element_id, count)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (window_sec, window_start, event_type, element_id)
			DO UPDATE SET count = t.count + EXCLUDED.count,
			updated_at = now()
			`
const saveChoiceQuery = `
			INSERT INTO value_stats AS t (window_sec, window_start, element_id, value, count)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (window_sec, window_start, element_id, value)
			DO UPDATE SET count      = t.count + EXCLUDED.count,
            updated_at = now()
			`

// Postgres writes window snapshots to PostgreSQL through a connection
// pool. It is safe for concurrent use by several partitions.
type Postgres struct {
	db *sqlx.DB
}

// NewPostgres connects to PostgreSQL using dsn, checks the connection
// within ctx and sets up the pool with at most maxConns connections.
func NewPostgres(ctx context.Context, dsn string, maxConns int) (*Postgres, error) {
	db, err := sqlx.ConnectContext(ctx, "pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("db connect: %w", err)
	}

	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(maxConns)
	db.SetConnMaxLifetime(connMaxLifetime)
	db.SetConnMaxIdleTime(connMaxIdleTime)
	return &Postgres{db: db}, nil
}

// Close closes the connection pool. Call it after the last Save.
func (r *Postgres) Close() error {
	return r.db.Close()
}

// Save adds the counters of snapshot to the stored ones in one
// transaction: a row with the same window and key gets its count
// increased, a missing row is inserted. On error the transaction is
// rolled back and the caller may pass the snapshot to Save again. The
// only exception is a lost reply to COMMIT: the data may be stored
// anyway, and a retry counts it twice.
func (r *Postgres) Save(ctx context.Context, snapshot domain.Snapshot) error {
	windowSec := int64(snapshot.WindowSize.Seconds())

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db begin tx: %w", err)
	}
	defer tx.Rollback()

	for _, row := range snapshot.Counters {
		_, err := tx.ExecContext(ctx,
			saveCounterQuery,
			windowSec,
			row.WindowStart,
			row.Type,
			row.ElementID,
			row.Count,
		)
		if err != nil {
			return fmt.Errorf("db save counter: %w", err)
		}
	}

	for _, row := range snapshot.Choices {
		_, err := tx.ExecContext(ctx,
			saveChoiceQuery,
			windowSec,
			row.WindowStart,
			row.ElementID,
			row.Value,
			row.Count,
		)
		if err != nil {
			return fmt.Errorf("db save choice: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("db commit: %w", err)
	}

	return nil
}
