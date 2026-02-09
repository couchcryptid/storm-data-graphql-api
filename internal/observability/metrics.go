package observability

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const namespace = "api"

// ReadinessChecker reports whether a dependency is ready to serve traffic.
type ReadinessChecker interface {
	CheckReadiness(ctx context.Context) error
}

// Metrics holds all Prometheus collectors for the application.
type Metrics struct {
	// HTTP
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec

	// Kafka
	KafkaMessagesConsumed *prometheus.CounterVec
	KafkaConsumerErrors   *prometheus.CounterVec
	KafkaConsumerRunning  *prometheus.GaugeVec

	// Database
	DBQueryDuration   *prometheus.HistogramVec
	DBPoolConnections *prometheus.GaugeVec
}

// NewMetrics creates and registers all application metrics with the default registry.
func NewMetrics() *Metrics {
	return newMetrics(promauto.With(prometheus.DefaultRegisterer))
}

// NewTestMetrics creates metrics backed by a throw-away registry.
// Safe to call from multiple tests without duplicate-registration panics.
func NewTestMetrics() *Metrics {
	return newMetrics(promauto.With(prometheus.NewRegistry()))
}

func newMetrics(factory promauto.Factory) *Metrics {
	return &Metrics{
		HTTPRequestsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "http_requests_total",
			Help:      "Total HTTP requests processed.",
		}, []string{"method", "path", "status"}),

		HTTPRequestDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request duration in seconds.",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5},
		}, []string{"method", "path"}),

		KafkaMessagesConsumed: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "kafka_messages_consumed_total",
			Help:      "Total Kafka messages consumed.",
		}, []string{"topic"}),

		KafkaConsumerErrors: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "kafka_consumer_errors_total",
			Help:      "Total Kafka consumer errors.",
		}, []string{"topic", "error_type"}),

		KafkaConsumerRunning: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "kafka_consumer_running",
			Help:      "Whether the Kafka consumer is running (1) or stopped (0).",
		}, []string{"topic"}),

		DBQueryDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "db_query_duration_seconds",
			Help:      "Database query duration in seconds.",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5},
		}, []string{"operation"}),

		DBPoolConnections: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "db_pool_connections",
			Help:      "Database connection pool statistics.",
		}, []string{"state"}),
	}
}
