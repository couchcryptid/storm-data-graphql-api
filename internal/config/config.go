package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds application settings loaded from environment variables.
type Config struct {
	Port               string
	DatabaseURL        string
	KafkaBrokers       []string
	KafkaTopic         string
	KafkaGroupID       string
	LogLevel           string
	LogFormat          string
	ShutdownTimeout    time.Duration
	BatchSize          int
	BatchFlushInterval time.Duration
}

// Load reads configuration from environment variables and returns it,
// or an error if required values are missing or invalid.
func Load() (*Config, error) {
	shutdownStr := envOrDefault("SHUTDOWN_TIMEOUT", "10s")
	shutdownTimeout, err := time.ParseDuration(shutdownStr)
	if err != nil || shutdownTimeout <= 0 {
		return nil, errors.New("invalid SHUTDOWN_TIMEOUT")
	}

	batchSize, err := parseBatchSize()
	if err != nil {
		return nil, err
	}

	flushStr := envOrDefault("BATCH_FLUSH_INTERVAL", "500ms")
	flushInterval, err := time.ParseDuration(flushStr)
	if err != nil || flushInterval <= 0 {
		return nil, errors.New("invalid BATCH_FLUSH_INTERVAL")
	}

	cfg := &Config{
		Port:               envOrDefault("PORT", "8080"),
		DatabaseURL:        envOrDefault("DATABASE_URL", "postgres://storm:storm@localhost:5432/stormdata?sslmode=disable"),
		KafkaBrokers:       parseBrokers(envOrDefault("KAFKA_BROKERS", "localhost:29092")),
		KafkaTopic:         envOrDefault("KAFKA_TOPIC", "transformed-weather-data"),
		KafkaGroupID:       envOrDefault("KAFKA_GROUP_ID", "storm-data-api"),
		LogLevel:           envOrDefault("LOG_LEVEL", "info"),
		LogFormat:          envOrDefault("LOG_FORMAT", "json"),
		ShutdownTimeout:    shutdownTimeout,
		BatchSize:          batchSize,
		BatchFlushInterval: flushInterval,
	}

	if len(cfg.KafkaBrokers) == 0 {
		return nil, errors.New("KAFKA_BROKERS is required")
	}
	if cfg.KafkaTopic == "" {
		return nil, errors.New("KAFKA_TOPIC is required")
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseBatchSize() (int, error) {
	s := os.Getenv("BATCH_SIZE")
	if s == "" {
		return 50, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 1000 {
		return 0, errors.New("invalid BATCH_SIZE: must be 1-1000")
	}
	return n, nil
}

func parseBrokers(value string) []string {
	parts := strings.Split(value, ",")
	brokers := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			brokers = append(brokers, trimmed)
		}
	}
	return brokers
}
