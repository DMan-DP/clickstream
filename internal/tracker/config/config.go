// Package config reads the tracker-api settings from environment variables.
package config

import (
	"fmt"
	"time"

	"github.com/DMan-DP/clickstream/internal/envconfig"
)

// Config holds the tracker-api settings: the shared ones from
// envconfig.Base, the HTTP address and the delay between SIGTERM and the
// server shutdown.
type Config struct {
	envconfig.Base
	HTTPAddr      string
	ShutdownDelay time.Duration
}

// GetConfig reads the settings from environment variables and validates them.
func GetConfig() (*Config, error) {
	base, err := envconfig.LoadBase()
	if err != nil {
		return nil, err
	}

	key := "SHUTDOWN_DELAY"
	s := envconfig.OrDefault(key, "0s")
	shutdownDelay, err := time.ParseDuration(s)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}

	if shutdownDelay < 0 {
		return nil, fmt.Errorf("%s: invalid value %q, want a non-negative duration", key, s)
	}

	return &Config{
		Base:          base,
		HTTPAddr:      envconfig.OrDefault("HTTP_ADDR", ":8080"),
		ShutdownDelay: shutdownDelay,
	}, nil
}
