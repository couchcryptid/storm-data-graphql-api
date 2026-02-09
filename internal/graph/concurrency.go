package graph

import "net/http"

// ConcurrencyLimit restricts the number of concurrent GraphQL requests
// to prevent pgx connection pool exhaustion. On a 4-connection pool
// with 1 reserved for Kafka, limit should be 2.
func ConcurrencyLimit(limit int) func(http.Handler) http.Handler {
	sem := make(chan struct{}, limit)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
				next.ServeHTTP(w, r)
			default:
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"errors":[{"message":"server busy, try again"}]}`))
			}
		})
	}
}
