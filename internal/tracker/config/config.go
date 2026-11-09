package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

type Config struct {
	HTTPAddr     string
	KafkaBrokers []string
	KafkaTopic   string
	LogLevel     slog.Level
}

func getEnvOrDefault(key string, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	return value
}

func getRequiredEnv(key string) (string, error) {
	value, ok := os.LookupEnv(key)
	if !ok {
		return "", errors.New(key + " is not set")
	}

	if value == "" {
		return "", errors.New(key + " is empty")
	}

	return value, nil
}

func getKafkaBrokers() ([]string, error) {
	key := "KAFKA_BROKERS"
	kafkaAddrStr, err := getRequiredEnv(key)
	if err != nil {
		return nil, err
	}

	kafkaBrokers := make([]string, 0)
	for str := range strings.SplitSeq(kafkaAddrStr, ",") {
		trimmed := strings.TrimSpace(str)
		if trimmed != "" {
			kafkaBrokers = append(kafkaBrokers, trimmed)
		}
	}

	if len(kafkaBrokers) == 0 {
		return nil, errors.New(key + " has no addresses")
	}

	return kafkaBrokers, nil
}

func getLogLevel() (slog.Level, error) {
	var lvl slog.Level
	key := "LOG_LEVEL"

	value := getEnvOrDefault(key, "info")

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

func GetConfig() (*Config, error) {
	httpAddr := getEnvOrDefault("HTTP_ADDR", ":8080")

	kafkaBrokers, err := getKafkaBrokers()
	if err != nil {
		return nil, err
	}

	kafkaTopic, err := getRequiredEnv("KAFKA_TOPIC")
	if err != nil {
		return nil, err
	}

	lvl, err := getLogLevel()
	if err != nil {
		return nil, err
	}

	return &Config{
		HTTPAddr:     httpAddr,
		KafkaBrokers: kafkaBrokers,
		KafkaTopic:   kafkaTopic,
		LogLevel:     lvl,
	}, nil
}
