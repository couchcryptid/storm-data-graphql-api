package config

import (
	"errors"
	"time"

	sharedcfg "github.com/couchcryptid/storm-data-shared/config"
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
	shutdownTimeout, err := sharedcfg.ParseShutdownTimeout()
	if err != nil {
		return nil, err
	}

	batchSize, err := sharedcfg.ParseBatchSize()
	if err != nil {
		return nil, err
	}

	flushInterval, err := sharedcfg.ParseBatchFlushInterval()
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Port:               sharedcfg.EnvOrDefault("PORT", "8080"),
		DatabaseURL:        sharedcfg.EnvOrDefault("DATABASE_URL", "postgres://storm:storm@localhost:5432/stormdata?sslmode=disable"),
		KafkaBrokers:       sharedcfg.ParseBrokers(sharedcfg.EnvOrDefault("KAFKA_BROKERS", "localhost:29092")),
		KafkaTopic:         sharedcfg.EnvOrDefault("KAFKA_TOPIC", "transformed-weather-data"),
		KafkaGroupID:       sharedcfg.EnvOrDefault("KAFKA_GROUP_ID", "storm-data-api"),
		LogLevel:           sharedcfg.EnvOrDefault("LOG_LEVEL", "info"),
		LogFormat:          sharedcfg.EnvOrDefault("LOG_FORMAT", "json"),
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
