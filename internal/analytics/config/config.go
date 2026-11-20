// Package config reads the analytics-processor settings from environment
// variables.
package config

import (
	"fmt"
	"time"

	"github.com/DMan-DP/clickstream/internal/envconfig"
)

// Config holds the analytics-processor settings: the shared ones from
// envconfig.Base, the consumer group id and the aggregation window size.
type Config struct {
	envconfig.Base
	KafkaGroupID string
	WindowSize   time.Duration
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

	if ws <= 0 {
		return nil, fmt.Errorf("WINDOW_SIZE: invalid value %q, want a positive duration", s)
	}

	return &Config{
		Base:         base,
		KafkaGroupID: envconfig.OrDefault("KAFKA_GROUP_ID", "analytics-processor"),
		WindowSize:   ws,
	}, nil
}
