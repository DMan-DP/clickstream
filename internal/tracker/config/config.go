package config

import (
	"errors"
	"os"
	"strings"
)

type Config struct {
	HTTPAddr     string
	KafkaBrokers []string
	KafkaTopic   string
	StaticDir    string
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

	staticDir := getEnvOrDefault("STATIC_DIR", "frontend")

	return &Config{
		HTTPAddr:     httpAddr,
		KafkaBrokers: kafkaBrokers,
		KafkaTopic:   kafkaTopic,
		StaticDir:    staticDir,
	}, nil
}
