// Package config reads the analytics-processor settings from environment
// variables.
package config

import "github.com/DMan-DP/clickstream/internal/envconfig"

// Config holds the analytics-processor settings: the shared ones from
// envconfig.Base and the consumer group id.
type Config struct {
	envconfig.Base
	KafkaGroupID string
}

// GetConfig reads the settings from environment variables and validates them.
func GetConfig() (*Config, error) {
	base, err := envconfig.LoadBase()
	if err != nil {
		return nil, err
	}

	return &Config{
		Base:         base,
		KafkaGroupID: envconfig.OrDefault("KAFKA_GROUP_ID", "analytics-processor"),
	}, nil
}
