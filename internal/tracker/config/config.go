// Package config reads the tracker-api settings from environment variables.
package config

import "github.com/DMan-DP/clickstream/internal/envconfig"

// Config holds the tracker-api settings: the shared ones from
// envconfig.Base and the HTTP address.
type Config struct {
	envconfig.Base
	HTTPAddr string
}

// GetConfig reads the settings from environment variables and validates them.
func GetConfig() (*Config, error) {
	base, err := envconfig.LoadBase()
	if err != nil {
		return nil, err
	}

	return &Config{
		Base:     base,
		HTTPAddr: envconfig.OrDefault("HTTP_ADDR", ":8080"),
	}, nil
}
