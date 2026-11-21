// Package envconfig reads settings from environment variables. It holds the
// helpers and the settings shared by tracker-api and analytics-processor;
// each service embeds Base into its own Config and adds its own fields.
package envconfig

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// Base holds the settings both services need.
type Base struct {
	KafkaBrokers []string
	KafkaTopic   string
	LogLevel     slog.Level
}

// LoadBase reads KAFKA_BROKERS, KAFKA_TOPIC and LOG_LEVEL.
func LoadBase() (Base, error) {
	kafkaBrokers, err := List("KAFKA_BROKERS")
	if err != nil {
		return Base{}, err
	}

	kafkaTopic, err := Required("KAFKA_TOPIC")
	if err != nil {
		return Base{}, err
	}

	lvl, err := logLevel("LOG_LEVEL")
	if err != nil {
		return Base{}, err
	}

	return Base{
		KafkaBrokers: kafkaBrokers,
		KafkaTopic:   kafkaTopic,
		LogLevel:     lvl,
	}, nil
}

// OrDefault returns the value of the variable, or defaultValue if it is
// unset or empty.
func OrDefault(key string, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	return value
}

// Required returns the value of the variable, or an error if it is unset
// or empty.
func Required(key string) (string, error) {
	value, ok := os.LookupEnv(key)
	if !ok {
		return "", errors.New(key + " is not set")
	}

	if value == "" {
		return "", errors.New(key + " is empty")
	}

	return value, nil
}

// List reads a required comma-separated variable. Spaces around items are
// trimmed and empty items are dropped; at least one item must remain.
func List(key string) ([]string, error) {
	str, err := Required(key)
	if err != nil {
		return nil, err
	}

	items := make([]string, 0)
	for item := range strings.SplitSeq(str, ",") {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			items = append(items, trimmed)
		}
	}

	if len(items) == 0 {
		return nil, errors.New(key + " has no values")
	}

	return items, nil
}

// logLevel reads a log level: debug, info, warn or error, exactly in lower
// case and without spaces. An unset or empty variable means info; any
// other value is an error.
func logLevel(key string) (slog.Level, error) {
	var lvl slog.Level

	value := OrDefault(key, "info")

	switch value {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		return lvl, fmt.Errorf("%s: unsupported value %q, want debug, info, warn or error", key, value)
	}

	return lvl, nil
}
