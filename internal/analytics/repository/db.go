// Package repository stores the aggregated window statistics in
// PostgreSQL.
package repository

import (
	"context"
	"fmt"
	"time"

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
