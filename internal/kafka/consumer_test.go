package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/couchcryptid/storm-data-api/internal/model"
	"github.com/couchcryptid/storm-data-api/internal/observability"
	kafkago "github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── Mocks ──────────────────────────────────────────────────

type mockReader struct {
	mu          sync.Mutex
	msgs        []kafkago.Message
	idx         int
	fetchErr    error
	commitErr   error
	closeCalled bool
	committed   []kafkago.Message
}

func (m *mockReader) FetchMessage(ctx context.Context) (kafkago.Message, error) {
	m.mu.Lock()
	if m.fetchErr != nil {
		err := m.fetchErr
		m.mu.Unlock()
		return kafkago.Message{}, err
	}
	if m.idx < len(m.msgs) {
		msg := m.msgs[m.idx]
		m.idx++
		m.mu.Unlock()
		return msg, nil
	}
	m.mu.Unlock()
	// No more messages — block until context cancelled.
	<-ctx.Done()
	return kafkago.Message{}, ctx.Err()
}

func (m *mockReader) CommitMessages(_ context.Context, msgs ...kafkago.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.committed = append(m.committed, msgs...)
	return m.commitErr
}

func (m *mockReader) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeCalled = true
	return nil
}

type mockStore struct {
	mu             sync.Mutex
	inserted       []*model.StormReport
	insertErr      error
	batchInserted  []*model.StormReport
	batchInsertErr error
}

func (m *mockStore) InsertStormReport(_ context.Context, report *model.StormReport) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inserted = append(m.inserted, report)
	return m.insertErr
}

// ─── Helpers ────────────────────────────────────────────────

func newTestConsumer(reader *mockReader, store *mockStore) *Consumer {
	return &Consumer{
		reader:  reader,
		store:   store,
		topic:   "test-topic",
		logger:  slog.Default(),
		metrics: observability.NewTestMetrics(),
	}
}

func validReport() model.StormReport {
	return model.StormReport{
		ID:          "abc123",
		Type:        "hail",
		Measurement: model.Measurement{Magnitude: 1.75, Unit: "in"},
		Source:      "trained_spotter",
		Location: model.Location{
			Raw:    "2 NW Springfield",
			Name:   "Springfield",
			State:  "IL",
			County: "Sangamon",
		},
		Geo:         model.Geo{Lat: 39.8, Lon: -89.6},
		BeginTime:   time.Date(2025, 6, 15, 18, 30, 0, 0, time.UTC),
		EndTime:     time.Date(2025, 6, 15, 18, 45, 0, 0, time.UTC),
		Comments:    "quarter size hail",
		TimeBucket:  time.Date(2025, 6, 15, 18, 0, 0, 0, time.UTC),
		ProcessedAt: time.Date(2025, 6, 15, 19, 0, 0, 0, time.UTC),
	}
}

func validMessageBytes(t *testing.T) []byte {
	t.Helper()
	b, err := json.Marshal(validReport())
	require.NoError(t, err)
	return b
}

func kafkaMsg(value []byte, offset int64) kafkago.Message {
	return kafkago.Message{
		Topic:  "test-topic",
		Value:  value,
		Offset: offset,
	}
}

// ─── handleMessage tests ────────────────────────────────────

func TestHandleMessage_HappyPath(t *testing.T) {
	store := &mockStore{}
	reader := &mockReader{}
	c := newTestConsumer(reader, store)

	msg := kafkaMsg(validMessageBytes(t), 42)
	stop := c.handleMessage(context.Background(), msg)

	assert.False(t, stop, "handleMessage should not signal stop on success")

	// Insert was called with the correct report.
	require.Len(t, store.inserted, 1)
	assert.Equal(t, "abc123", store.inserted[0].ID)
	assert.Equal(t, "hail", store.inserted[0].Type)
	assert.InDelta(t, 1.75, store.inserted[0].Measurement.Magnitude, 0.001)
	assert.Equal(t, "IL", store.inserted[0].Location.State)

	// Message was committed.
	require.Len(t, reader.committed, 1)
	assert.Equal(t, int64(42), reader.committed[0].Offset)
}

func TestHandleMessage_UnmarshalError(t *testing.T) {
	store := &mockStore{}
	reader := &mockReader{}
	c := newTestConsumer(reader, store)

	msg := kafkaMsg([]byte(`{not valid json`), 7)
	stop := c.handleMessage(context.Background(), msg)

	assert.False(t, stop, "unmarshal errors should not stop the consumer")

	// Insert must NOT be called.
	assert.Empty(t, store.inserted, "store should not be called for invalid JSON")

	// Poison pill should still be committed.
	require.Len(t, reader.committed, 1)
	assert.Equal(t, int64(7), reader.committed[0].Offset)
}

func TestHandleMessage_InsertError(t *testing.T) {
	store := &mockStore{insertErr: errors.New("db connection lost")}
	reader := &mockReader{}
	c := newTestConsumer(reader, store)

	msg := kafkaMsg(validMessageBytes(t), 10)
	stop := c.handleMessage(context.Background(), msg)

	assert.False(t, stop, "insert error should not stop the consumer when context is active")

	// Insert was attempted.
	require.Len(t, store.inserted, 1)

	// Message should NOT be committed on insert failure.
	assert.Empty(t, reader.committed, "message must not be committed when insert fails")
}

func TestHandleMessage_CommitError(t *testing.T) {
	store := &mockStore{}
	reader := &mockReader{commitErr: errors.New("commit failed")}
	c := newTestConsumer(reader, store)

	msg := kafkaMsg(validMessageBytes(t), 15)
	stop := c.handleMessage(context.Background(), msg)

	assert.False(t, stop, "commit error should not stop the consumer")

	// Insert was called successfully.
	require.Len(t, store.inserted, 1)
	assert.Equal(t, "abc123", store.inserted[0].ID)
}

func TestHandleMessage_ContextCancelled(t *testing.T) {
	store := &mockStore{}
	reader := &mockReader{}
	c := newTestConsumer(reader, store)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before handleMessage runs.

	msg := kafkaMsg(validMessageBytes(t), 20)
	stop := c.handleMessage(ctx, msg)

	assert.True(t, stop, "cancelled context should signal the consumer to stop")

	// Insert must not be called when context is cancelled.
	assert.Empty(t, store.inserted)
}

// ─── Run tests ──────────────────────────────────────────────

func TestRun_FetchError_Backoff(t *testing.T) {
	reader := &mockReader{fetchErr: errors.New("connection refused")}
	store := &mockStore{}
	c := newTestConsumer(reader, store)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := c.Run(ctx)

	require.NoError(t, err, "Run should return nil when context is cancelled")
	assert.Empty(t, store.inserted, "no messages should be inserted when fetch fails")
}

func TestRun_ContextCancelled(t *testing.T) {
	reader := &mockReader{} // No messages, FetchMessage will block on ctx.Done().
	store := &mockStore{}
	c := newTestConsumer(reader, store)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	err := c.Run(ctx)

	assert.NoError(t, err, "Run should return nil on context cancellation")
}

func TestRun_ProcessesMessagesThenStops(t *testing.T) {
	data := validMessageBytes(t)
	reader := &mockReader{
		msgs: []kafkago.Message{
			kafkaMsg(data, 0),
			kafkaMsg(data, 1),
		},
	}
	store := &mockStore{}
	c := newTestConsumer(reader, store)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := c.Run(ctx)

	require.NoError(t, err)

	// Both messages processed and committed.
	assert.Len(t, store.inserted, 2)
	assert.Len(t, reader.committed, 2)
}

// ─── Close test ─────────────────────────────────────────────

func TestClose(t *testing.T) {
	reader := &mockReader{}
	store := &mockStore{}
	c := newTestConsumer(reader, store)

	err := c.Close()

	require.NoError(t, err)
	assert.True(t, reader.closeCalled, "Close should delegate to the reader")
}
