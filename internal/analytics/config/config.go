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
// envconfig.Base, the consumer group id, the aggregation window size, the
// interval of writes to the database and the PostgreSQL connection string
// with the pool size limit.
type Config struct {
	envconfig.Base
	KafkaGroupID  string
	WindowSize    time.Duration
	FlushInterval time.Duration
	DSN           string
	DBMaxConns    int
}

// GetConfig reads the settings from environment variables and validates them.
func GetConfig() (*Config, error) {
	base, err := envconfig.LoadBase()
	if err != nil {
		return nil, err
	}

	key := "WINDOW_SIZE"
	s := envconfig.OrDefault(key, "30s")
	windowSize, err := time.ParseDuration(s)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}

	if windowSize <= 0 || windowSize%time.Second != 0 {
		return nil, fmt.Errorf("%s: invalid value %q, want a positive whole number of seconds", key, s)
	}

	key = "FLUSH_INTERVAL"
	s = envconfig.OrDefault(key, "5s")
	flushInterval, err := time.ParseDuration(s)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}

	if flushInterval <= 0 {
		return nil, fmt.Errorf("%s: invalid value %q, want a positive whole number of seconds", key, s)
	}

	dsn, err := envconfig.Required("DATABASE_URL")
	if err != nil {
		return nil, err
	}

	key = "DB_MAX_CONNS"
	s = envconfig.OrDefault(key, "10")
	maxConns, err := strconv.Atoi(s)
	if err != nil || maxConns <= 0 {
		return nil, fmt.Errorf("%s: invalid value %q, want a positive count", key, s)
	}

	return &Config{
		Base:          base,
		KafkaGroupID:  envconfig.OrDefault("KAFKA_GROUP_ID", "analytics-processor"),
		WindowSize:    windowSize,
		FlushInterval: flushInterval,
		DSN:           dsn,
		DBMaxConns:    maxConns,
	}, nil
}
