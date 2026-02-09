package kafka

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/couchcryptid/storm-data-graphql-api/internal/model"
	"github.com/couchcryptid/storm-data-graphql-api/internal/observability"
	kafkago "github.com/segmentio/kafka-go"
)

// MessageReader abstracts the kafka reader for testability.
type MessageReader interface {
	FetchMessage(ctx context.Context) (kafkago.Message, error)
	CommitMessages(ctx context.Context, msgs ...kafkago.Message) error
	Close() error
}

// StoreInserter abstracts the store dependency for testability.
type StoreInserter interface {
	InsertStormReport(ctx context.Context, report *model.StormReport) error
}

// Consumer reads storm reports from a Kafka topic and persists them to the store.
type Consumer struct {
	reader  MessageReader
	store   StoreInserter
	topic   string
	logger  *slog.Logger
	metrics *observability.Metrics
}

// NewConsumer creates a consumer that reads from the given topic and inserts into the store.
func NewConsumer(brokers []string, topic, groupID string, s StoreInserter, m *observability.Metrics, logger *slog.Logger) *Consumer {
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:  brokers,
		Topic:    topic,
		GroupID:  groupID,
		MinBytes: 1,
		MaxBytes: 10e6, // 10 MB
	})
	return &Consumer{
		reader:  reader,
		store:   s,
		topic:   topic,
		logger:  logger,
		metrics: m,
	}
}

// Run consumes messages until the context is cancelled.
func (c *Consumer) Run(ctx context.Context) error {
	c.logger.Info("kafka consumer started", "topic", c.topic)
	c.metrics.KafkaConsumerRunning.WithLabelValues(c.topic).Set(1)
	defer c.metrics.KafkaConsumerRunning.WithLabelValues(c.topic).Set(0)

	backoff := 200 * time.Millisecond
	maxBackoff := 5 * time.Second

	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			c.metrics.KafkaConsumerErrors.WithLabelValues(c.topic, "fetch").Inc()
			c.logger.Error("fetch kafka message", "error", err, "retry_in", backoff)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, maxBackoff)
			continue
		}
		backoff = 200 * time.Millisecond

		var report model.StormReport
		if err := json.Unmarshal(msg.Value, &report); err != nil {
			c.logger.Error("unmarshal kafka message", "error", err, "offset", msg.Offset)
			c.metrics.KafkaConsumerErrors.WithLabelValues(c.topic, "unmarshal").Inc()
			// Commit bad messages to avoid reprocessing poison pills
			if err := c.reader.CommitMessages(ctx, msg); err != nil {
				c.logger.Error("commit offset after unmarshal error", "error", err)
			}
			continue
		}

		if ctx.Err() != nil {
			return nil
		}

		if err := c.store.InsertStormReport(ctx, &report); err != nil {
			c.logger.Error("insert storm report", "error", err, "id", report.ID)
			c.metrics.KafkaConsumerErrors.WithLabelValues(c.topic, "insert").Inc()
			if ctx.Err() != nil {
				return nil
			}
			// Don't commit — message will be retried on next startup
			continue
		}

		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			c.logger.Error("commit offset", "error", err, "id", report.ID)
		}

		c.metrics.KafkaMessagesConsumed.WithLabelValues(c.topic).Inc()
		c.logger.Debug("consumed storm report", "id", report.ID, "type", report.Type)
	}
}

// Close shuts down the underlying Kafka reader.
func (c *Consumer) Close() error {
	return c.reader.Close()
}
