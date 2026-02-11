package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "8080", cfg.Port)
	assert.Equal(t, "postgres://storm:storm@localhost:5432/stormdata?sslmode=disable", cfg.DatabaseURL)
	assert.Equal(t, []string{"localhost:29092"}, cfg.KafkaBrokers)
	assert.Equal(t, "transformed-weather-data", cfg.KafkaTopic)
	assert.Equal(t, "storm-data-api", cfg.KafkaGroupID)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, "json", cfg.LogFormat)
	assert.Equal(t, 10*time.Second, cfg.ShutdownTimeout)
	assert.Equal(t, 50, cfg.BatchSize)
	assert.Equal(t, 500*time.Millisecond, cfg.BatchFlushInterval)
}

func TestLoad_CustomEnv(t *testing.T) {
	t.Setenv("PORT", "3000")
	t.Setenv("DATABASE_URL", "postgres://user:pass@db:5432/mydb")
	t.Setenv("KAFKA_BROKERS", "broker1:9092,broker2:9092")
	t.Setenv("KAFKA_TOPIC", "custom-topic")
	t.Setenv("KAFKA_GROUP_ID", "custom-group")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("LOG_FORMAT", "text")
	t.Setenv("SHUTDOWN_TIMEOUT", "30s")
	t.Setenv("BATCH_SIZE", "100")
	t.Setenv("BATCH_FLUSH_INTERVAL", "1s")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "3000", cfg.Port)
	assert.Equal(t, "postgres://user:pass@db:5432/mydb", cfg.DatabaseURL)
	assert.Equal(t, []string{"broker1:9092", "broker2:9092"}, cfg.KafkaBrokers)
	assert.Equal(t, "custom-topic", cfg.KafkaTopic)
	assert.Equal(t, "custom-group", cfg.KafkaGroupID)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, "text", cfg.LogFormat)
	assert.Equal(t, 30*time.Second, cfg.ShutdownTimeout)
	assert.Equal(t, 100, cfg.BatchSize)
	assert.Equal(t, 1*time.Second, cfg.BatchFlushInterval)
}

func TestLoad_InvalidShutdownTimeout(t *testing.T) {
	t.Setenv("SHUTDOWN_TIMEOUT", "not-a-duration")
	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SHUTDOWN_TIMEOUT")
}

func TestLoad_NegativeShutdownTimeout(t *testing.T) {
	t.Setenv("SHUTDOWN_TIMEOUT", "-1s")
	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SHUTDOWN_TIMEOUT")
}

func TestLoad_InvalidBatchSize(t *testing.T) {
	t.Setenv("BATCH_SIZE", "0")
	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "BATCH_SIZE")
}

func TestLoad_InvalidBatchFlushInterval(t *testing.T) {
	t.Setenv("BATCH_FLUSH_INTERVAL", "bad")
	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "BATCH_FLUSH_INTERVAL")
}

func TestParseBrokers(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"single broker", "localhost:9092", []string{"localhost:9092"}},
		{"multiple brokers", "a:1,b:2", []string{"a:1", "b:2"}},
		{"trims whitespace", " a , , b ", []string{"a", "b"}},
		{"empty string", "", []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseBrokers(tt.input))
		})
	}
}

func TestEnvOrDefault(t *testing.T) {
	t.Run("returns env value when set", func(t *testing.T) {
		t.Setenv("TEST_CONFIG_KEY", "custom")
		assert.Equal(t, "custom", envOrDefault("TEST_CONFIG_KEY", "default"))
	})

	t.Run("returns fallback when unset", func(t *testing.T) {
		assert.Equal(t, "fallback", envOrDefault("NONEXISTENT_KEY_FOR_TEST", "fallback"))
	})
}
