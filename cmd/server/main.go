package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/couchcryptid/storm-data-graphql-api/internal/config"
	"github.com/couchcryptid/storm-data-graphql-api/internal/database"
	"github.com/couchcryptid/storm-data-graphql-api/internal/graph"
	"github.com/couchcryptid/storm-data-graphql-api/internal/kafka"
	"github.com/couchcryptid/storm-data-graphql-api/internal/observability"
	"github.com/couchcryptid/storm-data-graphql-api/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger := observability.NewLogger(cfg)
	metrics := observability.NewMetrics()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Database
	if err := database.RunMigrations(cfg.DatabaseURL); err != nil {
		logger.Error("run migrations", "error", err)
		os.Exit(1) //nolint:gocritic // startup exits before meaningful defers
	}

	pool, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	s := store.New(pool, metrics)
	readiness := database.NewPoolReadiness(pool)

	// DB pool stats collector
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				stat := pool.Stat()
				metrics.DBPoolConnections.WithLabelValues("idle").Set(float64(stat.IdleConns()))
				metrics.DBPoolConnections.WithLabelValues("active").Set(float64(stat.AcquiredConns()))
				metrics.DBPoolConnections.WithLabelValues("total").Set(float64(stat.TotalConns()))
			}
		}
	}()

	// Kafka consumer
	consumer := kafka.NewConsumer(cfg.KafkaBrokers, cfg.KafkaTopic, cfg.KafkaGroupID, s, metrics, logger)
	defer func() {
		if err := consumer.Close(); err != nil {
			logger.Error("kafka consumer close", "error", err)
		}
	}()
	go func() {
		if err := consumer.Run(ctx); err != nil {
			logger.Error("kafka consumer", "error", err)
		}
	}()

	// GraphQL server
	srv := handler.NewDefaultServer(graph.NewExecutableSchema(graph.Config{
		Resolvers:  &graph.Resolver{Store: s},
		Complexity: graph.NewComplexityRoot(),
	}))
	srv.Use(extension.FixedComplexityLimit(600))
	srv.Use(graph.DepthLimit{MaxDepth: 7})

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.AllowAll().Handler)
	r.Use(observability.MetricsMiddleware(metrics))
	r.Use(graph.ConcurrencyLimit(2))
	r.Handle("/", playground.Handler("Storm Data API", "/query"))
	r.Handle("/query", srv)
	r.Get("/healthz", observability.LivenessHandler())
	r.Get("/readyz", observability.ReadinessHandler(readiness))
	r.Handle("/metrics", promhttp.Handler())

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           http.TimeoutHandler(r, 25*time.Second, `{"errors":[{"message":"request timeout"}]}`),
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		<-ctx.Done()
		logger.Info("shutting down")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("server shutdown", "error", err)
		}
	}()

	logger.Info("server started", "port", cfg.Port)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}

	logger.Info("shutdown complete")
}
