package kafka

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/couchcryptid/storm-data-api/internal/model"
	"github.com/couchcryptid/storm-data-api/internal/observability"
	"github.com/couchcryptid/storm-data-shared/retry"
	kafkago "github.com/segmentio/kafka-go"
)

// batchItem holds a fetched Kafka message and its unmarshalled result.
type batchItem struct {
	msg    kafkago.Message
	report *model.StormReport
	err    error // non-nil if unmarshal failed (poison pill)
}

// BatchConsumer reads storm reports from Kafka in batches and persists them to the store.
type BatchConsumer struct {
	reader        MessageReader
	store         StoreInserter
	topic         string
	batchSize     int
	flushInterval time.Duration
	logger        *slog.Logger
	metrics       *observability.Metrics
}

// NewBatchConsumer creates a batch consumer with time-bounded fetching.
func NewBatchConsumer(
	brokers []string,
	topic, groupID string,
	batchSize int,
	flushInterval time.Duration,
	s StoreInserter,
	m *observability.Metrics,
	logger *slog.Logger,
) *BatchConsumer {
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:     brokers,
		Topic:       topic,
		GroupID:     groupID,
		StartOffset: kafkago.FirstOffset,
		MinBytes:    1,
		MaxBytes:    10e6, // 10 MB
	})
	return &BatchConsumer{
		reader:        reader,
		store:         s,
		topic:         topic,
		batchSize:     batchSize,
		flushInterval: flushInterval,
		logger:        logger,
		metrics:       m,
	}
}

// Run consumes messages in batches until the context is cancelled.
func (bc *BatchConsumer) Run(ctx context.Context) error {
	bc.logger.Info("kafka batch consumer started",
		"topic", bc.topic, "batch_size", bc.batchSize, "flush_interval", bc.flushInterval)
	bc.metrics.KafkaConsumerRunning.WithLabelValues(bc.topic).Set(1)
	defer bc.metrics.KafkaConsumerRunning.WithLabelValues(bc.topic).Set(0)

	// Exponential backoff: start at 200ms, double each retry, cap at 5s.
	// Keeps retry storms short while avoiding tight loops during Kafka outages.
	backoff := 200 * time.Millisecond
	maxBackoff := 5 * time.Second

	for {
		items, err := bc.fetchBatch(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			bc.metrics.KafkaConsumerErrors.WithLabelValues(bc.topic, "fetch_batch").Inc()
			bc.logger.Error("fetch batch", "error", err, "retry_in", backoff)
			if !retry.SleepWithContext(ctx, backoff) {
				return nil
			}
			backoff = retry.NextBackoff(backoff, maxBackoff)
			continue
		}
		backoff = 200 * time.Millisecond

		if len(items) == 0 {
			if ctx.Err() != nil {
				return nil
			}
			continue
		}

		bc.processBatch(ctx, items)
	}
}

// fetchBatch collects up to batchSize messages or until flushInterval elapses.
func (bc *BatchConsumer) fetchBatch(ctx context.Context) ([]batchItem, error) {
	start := time.Now()
	defer func() {
		bc.metrics.KafkaBatchDuration.WithLabelValues(bc.topic, "fetch").Observe(time.Since(start).Seconds())
	}()

	items := make([]batchItem, 0, bc.batchSize)
	deadline := time.Now().Add(bc.flushInterval)

	for len(items) < bc.batchSize {
		timeout := time.Until(deadline)
		if timeout <= 0 {
			break
		}

		fetchCtx, cancel := context.WithTimeout(ctx, timeout)
		msg, err := bc.reader.FetchMessage(fetchCtx)
		cancel()

		if err != nil {
			if ctx.Err() != nil {
				// Parent context cancelled — return what we have.
				break
			}
			if fetchCtx.Err() == context.DeadlineExceeded {
				// Flush interval expired — return partial batch.
				break
			}
			return nil, err
		}

		var report model.StormReport
		if unmarshalErr := json.Unmarshal(msg.Value, &report); unmarshalErr != nil {
			items = append(items, batchItem{msg: msg, err: unmarshalErr})
		} else {
			items = append(items, batchItem{msg: msg, report: &report})
		}
	}

	bc.metrics.KafkaBatchSize.WithLabelValues(bc.topic).Observe(float64(len(items)))
	return items, nil
}

// processBatch inserts valid reports and commits all offsets.
func (bc *BatchConsumer) processBatch(ctx context.Context, items []batchItem) {
	start := time.Now()
	defer func() {
		bc.metrics.KafkaBatchDuration.WithLabelValues(bc.topic, "process").Observe(time.Since(start).Seconds())
	}()

	var validReports []*model.StormReport
	var validMsgs []kafkago.Message
	var poisonMsgs []kafkago.Message

	for i := range items {
		if items[i].err != nil {
			bc.logger.Error("unmarshal in batch", "error", items[i].err, "offset", items[i].msg.Offset)
			bc.metrics.KafkaConsumerErrors.WithLabelValues(bc.topic, "unmarshal").Inc()
			poisonMsgs = append(poisonMsgs, items[i].msg)
		} else {
			validReports = append(validReports, items[i].report)
			validMsgs = append(validMsgs, items[i].msg)
		}
	}

	// Commit poison pills so Kafka doesn't re-deliver them in an infinite loop.
	// Bad messages are logged above for manual investigation; skipping them is
	// preferable to blocking the entire consumer on unrecoverable parse errors.
	if len(poisonMsgs) > 0 {
		if err := bc.reader.CommitMessages(ctx, poisonMsgs...); err != nil {
			bc.logger.Error("commit poison pills", "error", err, "count", len(poisonMsgs))
		}
	}

	if len(validReports) == 0 {
		return
	}

	if err := bc.store.InsertStormReports(ctx, validReports); err != nil {
		bc.logger.Error("batch insert storm reports", "error", err, "count", len(validReports))
		bc.metrics.KafkaConsumerErrors.WithLabelValues(bc.topic, "batch_insert").Inc()
		return
	}

	if err := bc.reader.CommitMessages(ctx, validMsgs...); err != nil {
		bc.logger.Error("commit batch offsets", "error", err, "count", len(validMsgs))
	}

	bc.metrics.KafkaMessagesConsumed.WithLabelValues(bc.topic).Add(float64(len(validReports)))
	bc.logger.Debug("consumed batch", "count", len(validReports))
}

// Close shuts down the underlying Kafka reader.
func (bc *BatchConsumer) Close() error {
	return bc.reader.Close()
}
