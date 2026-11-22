// Package config reads the analytics-processor settings from environment
// variables.
package config

import (
	"fmt"
	"strconv"
	"time"

	"github.com/DMan-DP/clickstream/internal/envconfig"
)

// Config holds the analytics-processor settings: the shared ones from
// envconfig.Base, the consumer group id, the aggregation window size and
// the PostgreSQL connection string with the pool size limit.
type Config struct {
	envconfig.Base
	KafkaGroupID string
	WindowSize   time.Duration
	DSN          string
	DBMaxConns   int
}

// GetConfig reads the settings from environment variables and validates them.
func GetConfig() (*Config, error) {
	base, err := envconfig.LoadBase()
	if err != nil {
		return nil, err
	}

	s := envconfig.OrDefault("WINDOW_SIZE", "30s")
	ws, err := time.ParseDuration(s)
	if err != nil {
		return nil, fmt.Errorf("WINDOW_SIZE: %w", err)
	}

	if ws <= 0 || ws%time.Second != 0 {
		return nil, fmt.Errorf("WINDOW_SIZE: invalid value %q, want a positive duration", s)
	}

	dsn, err := envconfig.Required("DATABASE_URL")
	if err != nil {
		return nil, err
	}

	s = envconfig.OrDefault("DB_MAX_CONNS", "10")
	maxConns, err := strconv.Atoi(s)
	if err != nil || maxConns <= 0 {
		return nil, fmt.Errorf("DB_MAX_CONNS: invalid value %q, want a positive count", s)
	}

	return &Config{
		Base:         base,
		KafkaGroupID: envconfig.OrDefault("KAFKA_GROUP_ID", "analytics-processor"),
		WindowSize:   ws,
		DSN:          dsn,
		DBMaxConns:   maxConns,
	}, nil
}
