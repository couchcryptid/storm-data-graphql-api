package graph

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConcurrencyLimit_AllowsWithinLimit(t *testing.T) {
	handler := ConcurrencyLimit(2)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/query", nil))

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestConcurrencyLimit_RejectsBeyondLimit(t *testing.T) {
	block := make(chan struct{})
	var inFlight atomic.Int32

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		inFlight.Add(1)
		<-block
		w.WriteHeader(http.StatusOK)
	})

	handler := ConcurrencyLimit(1)(inner)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	ctx := context.Background()

	// First request fills the slot
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, nil)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}
	}()

	// Wait for first request to be in flight
	for inFlight.Load() < 1 {
		time.Sleep(5 * time.Millisecond)
	}

	// Second request should be rejected
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	_ = resp.Body.Close()

	// Release blocked request
	close(block)
	wg.Wait()
}
