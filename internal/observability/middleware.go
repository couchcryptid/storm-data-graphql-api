package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// MetricsMiddleware records HTTP request duration and count.
func MetricsMiddleware(m *Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(ww, r)

			// Use chi's route pattern to avoid unbounded label cardinality
			// from dynamic path parameters.
			path := r.URL.Path
			if rctx := chi.RouteContext(r.Context()); rctx != nil && rctx.RoutePattern() != "" {
				path = rctx.RoutePattern()
			}
			method := r.Method
			status := strconv.Itoa(ww.statusCode)

			m.HTTPRequestDuration.WithLabelValues(method, path).Observe(time.Since(start).Seconds())
			m.HTTPRequestsTotal.WithLabelValues(method, path, status).Inc()
		})
	}
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Flush implements http.Flusher for streaming responses (e.g., GraphQL SSE).
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
