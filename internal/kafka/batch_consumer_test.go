package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/couchcryptid/storm-data-api/internal/model"
	"github.com/couchcryptid/storm-data-api/internal/observability"
	kafkago "github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- batch consumer helpers ---

func newTestBatchConsumer(reader *mockReader, store *mockStore) *BatchConsumer {
	return &BatchConsumer{
		reader:        reader,
		store:         store,
		topic:         "test-topic",
		batchSize:     50,
		flushInterval: 500 * time.Millisecond,
		logger:        slog.Default(),
		metrics:       observability.NewTestMetrics(),
	}
}

// --- fetchBatch tests ---

func TestFetchBatch_FullBatch(t *testing.T) {
	data := validMessageBytes(t)
	reader := &mockReader{
		msgs: []kafkago.Message{
			kafkaMsg(data, 0),
			kafkaMsg(data, 1),
			kafkaMsg(data, 2),
		},
	}
	store := &mockStore{}
	bc := newTestBatchConsumer(reader, store)
	bc.batchSize = 3

	items, err := bc.fetchBatch(context.Background())
	require.NoError(t, err)
	assert.Len(t, items, 3)
	for _, item := range items {
		require.NoError(t, item.err)
		assert.NotNil(t, item.report)
	}
}

func TestFetchBatch_FlushTimeout(t *testing.T) {
	// Only one message available, should return partial batch after flush interval.
	data := validMessageBytes(t)
	reader := &mockReader{
		msgs: []kafkago.Message{kafkaMsg(data, 0)},
	}
	store := &mockStore{}
	bc := newTestBatchConsumer(reader, store)
	bc.batchSize = 50
	bc.flushInterval = 200 * time.Millisecond

	items, err := bc.fetchBatch(context.Background())
	require.NoError(t, err)
	assert.Len(t, items, 1)
}

func TestFetchBatch_PoisonPill(t *testing.T) {
	reader := &mockReader{
		msgs: []kafkago.Message{
			kafkaMsg([]byte(`{not valid json`), 0),
			kafkaMsg(validMessageBytes(t), 1),
		},
	}
	store := &mockStore{}
	bc := newTestBatchConsumer(reader, store)
	bc.batchSize = 2

	items, err := bc.fetchBatch(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Error(t, items[0].err, "first item should have unmarshal error")
	assert.NoError(t, items[1].err, "second item should be valid")
}

func TestFetchBatch_FetchError(t *testing.T) {
	reader := &mockReader{fetchErr: errors.New("connection refused")}
	store := &mockStore{}
	bc := newTestBatchConsumer(reader, store)

	items, err := bc.fetchBatch(context.Background())
	require.Error(t, err)
	assert.Nil(t, items)
}

// --- processBatch tests ---

func TestProcessBatch_HappyPath(t *testing.T) {
	data := validMessageBytes(t)
	reader := &mockReader{}
	store := &mockStore{}
	bc := newTestBatchConsumer(reader, store)

	var report model.StormReport
	require.NoError(t, json.Unmarshal(data, &report))

	items := []batchItem{
		{msg: kafkaMsg(data, 0), report: &report},
		{msg: kafkaMsg(data, 1), report: &report},
	}

	bc.processBatch(context.Background(), items)

	store.mu.Lock()
	defer store.mu.Unlock()
	assert.Len(t, store.batchInserted, 2)

	reader.mu.Lock()
	defer reader.mu.Unlock()
	assert.Len(t, reader.committed, 2)
}

func TestProcessBatch_PoisonPillsCommitted(t *testing.T) {
	data := validMessageBytes(t)
	reader := &mockReader{}
	store := &mockStore{}
	bc := newTestBatchConsumer(reader, store)

	var report model.StormReport
	require.NoError(t, json.Unmarshal(data, &report))

	items := []batchItem{
		{msg: kafkaMsg([]byte("bad"), 0), err: errors.New("unmarshal error")},
		{msg: kafkaMsg(data, 1), report: &report},
	}

	bc.processBatch(context.Background(), items)

	reader.mu.Lock()
	defer reader.mu.Unlock()
	// Both poison pill and valid message should be committed.
	assert.Len(t, reader.committed, 2)

	store.mu.Lock()
	defer store.mu.Unlock()
	assert.Len(t, store.batchInserted, 1)
}

func TestProcessBatch_InsertError(t *testing.T) {
	data := validMessageBytes(t)
	reader := &mockReader{}
	store := &mockStore{batchInsertErr: errors.New("db connection lost")}
	bc := newTestBatchConsumer(reader, store)

	var report model.StormReport
	require.NoError(t, json.Unmarshal(data, &report))

	items := []batchItem{
		{msg: kafkaMsg(data, 0), report: &report},
	}

	bc.processBatch(context.Background(), items)

	reader.mu.Lock()
	defer reader.mu.Unlock()
	// No commits when insert fails.
	assert.Empty(t, reader.committed)
}

// --- Run tests ---

func TestBatchRun_ContextCancelled(t *testing.T) {
	reader := &mockReader{} // No messages, will block.
	store := &mockStore{}
	bc := newTestBatchConsumer(reader, store)
	bc.flushInterval = 100 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := bc.Run(ctx)
	assert.NoError(t, err)
}

func TestBatchRun_ProcessesAndStops(t *testing.T) {
	data := validMessageBytes(t)
	reader := &mockReader{
		msgs: []kafkago.Message{
			kafkaMsg(data, 0),
			kafkaMsg(data, 1),
		},
	}
	store := &mockStore{}
	bc := newTestBatchConsumer(reader, store)
	bc.batchSize = 2
	bc.flushInterval = 200 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := bc.Run(ctx)
	require.NoError(t, err)

	store.mu.Lock()
	defer store.mu.Unlock()
	assert.Len(t, store.batchInserted, 2)
}

// --- Close test ---

func TestBatchClose(t *testing.T) {
	reader := &mockReader{}
	store := &mockStore{}
	bc := newTestBatchConsumer(reader, store)

	err := bc.Close()
	require.NoError(t, err)

	reader.mu.Lock()
	defer reader.mu.Unlock()
	assert.True(t, reader.closeCalled)
}

// --- mockStore batch method (struct defined in consumer_test.go) ---

var _ StoreInserter = (*mockStore)(nil)

func (m *mockStore) InsertStormReports(_ context.Context, reports []*model.StormReport) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.batchInsertErr != nil {
		return m.batchInsertErr
	}
	m.batchInserted = append(m.batchInserted, reports...)
	return nil
}
