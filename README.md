# Storm Data GraphQL API

A Go service that consumes transformed storm weather reports from a Kafka topic, persists them to PostgreSQL, and serves them through a GraphQL API. Part of the storm data pipeline. Uses the [storm-data-shared](https://github.com/couchcryptid/storm-data-shared) library for common config and observability utilities.

## How It Works

The service runs two concurrent loops:

1. **Consume** -- Reads enriched storm events from a Kafka topic and persists them to PostgreSQL
2. **Serve** -- Exposes a GraphQL API for querying storm reports with filtering, pagination, and aggregation

Supported event types: **hail**, **wind**, and **tornado**.

```
Kafka (transformed-weather-data) --> Consumer --> Store --> PostgreSQL
                                                    ^
             GraphQL (/query) --> Resolvers --------+
```

## Quick Start

### Prerequisites

- Go 1.25+
- Docker and Docker Compose

### Run locally with Docker Compose

```sh
make docker-up    # Start Postgres + Kafka
make run          # Start server (runs migrations automatically)
```

### Example Query

```graphql
{
  stormReports(filter: {
    timeRange: { from: "2024-04-26T00:00:00Z", to: "2024-04-27T00:00:00Z" }
    eventTypes: [HAIL]
    states: ["TX"]
  }) {
    totalCount
    hasMore
    reports {
      id
      eventType
      measurement { magnitude unit }
      geo { lat lon }
      location { name county state }
      comments
      eventTime
    }
    aggregations {
      byEventType { eventType count maxMeasurement { magnitude unit } }
    }
    meta { lastUpdated dataLagMinutes }
  }
}
```

## Configuration

All configuration is via environment variables with Docker Compose defaults:

| Variable           | Default                                                          | Description                                    |
| ------------------ | ---------------------------------------------------------------- | ---------------------------------------------- |
| `PORT`             | `8080`                                                           | HTTP server port                               |
| `DATABASE_URL`     | `postgres://storm:storm@postgres:5432/stormdata?sslmode=disable` | PostgreSQL connection string                  |
| `KAFKA_BROKERS`    | `kafka:9092`                                                     | Comma-separated list of Kafka broker addresses |
| `KAFKA_TOPIC`      | `transformed-weather-data`                                       | Topic to consume enriched events from          |
| `KAFKA_GROUP_ID`   | `storm-data-api`                                         | Consumer group ID                              |
| `LOG_LEVEL`        | `info`                                                           | Log level: `debug`, `info`, `warn`, `error`    |
| `LOG_FORMAT`       | `json`                                                           | Log format: `json` or `text`                   |
| `SHUTDOWN_TIMEOUT` | `10s`                                                            | Graceful shutdown deadline                     |
| `BATCH_SIZE`       | `50`                                                             | Kafka messages per batch (1--1000)             |
| `BATCH_FLUSH_INTERVAL` | `500ms`                                                      | Max wait before flushing a partial batch       |

## HTTP Endpoints

| Endpoint       | Description                                                     |
| -------------- | --------------------------------------------------------------- |
| `GET /healthz` | Liveness probe -- always returns `200`                          |
| `GET /readyz`  | Readiness probe -- returns `200` when Postgres is reachable, `503` otherwise |
| `GET /metrics` | Prometheus metrics                                              |
| `POST /query`  | GraphQL endpoint                                                |

## Prometheus Metrics

| Metric                                | Type      | Labels                       | Description                                |
| ------------------------------------- | --------- | ---------------------------- | ------------------------------------------ |
| `storm_api_http_requests_total`             | Counter   | `method`, `path`, `status`   | Total HTTP requests processed              |
| `storm_api_http_request_duration_seconds`   | Histogram | `method`, `path`             | HTTP request duration                      |
| `storm_api_kafka_messages_consumed_total`   | Counter   | `topic`                      | Total Kafka messages consumed              |
| `storm_api_kafka_consumer_errors_total`     | Counter   | `topic`, `error_type`        | Total Kafka consumer errors                |
| `storm_api_kafka_consumer_running`          | Gauge     | `topic`                      | `1` when the Kafka consumer is running     |
| `storm_api_kafka_batch_size`                | Histogram | --                           | Number of messages per batch               |
| `storm_api_kafka_batch_duration_seconds`    | Histogram | --                           | Duration of batch processing               |
| `storm_api_db_query_duration_seconds`       | Histogram | `operation`                  | Database query duration                    |
| `storm_api_db_pool_connections`             | Gauge     | `state`                      | Database connection pool statistics        |

## Development

```
make build            # Compile binary to bin/server
make run              # Run server locally
make generate         # Regenerate gqlgen GraphQL code
make test             # Run all tests
make test-unit        # Run unit tests only
make test-integration # Run integration tests (Docker required)
make lint             # Run golangci-lint
make docker-up        # Start Postgres + Kafka
make docker-down      # Stop and remove containers + volumes
```

Integration tests require Docker because they use Postgres and Kafka containers.

## Project Structure

```
cmd/server/                 Entry point
internal/
  config/                   Environment-based configuration (uses storm-data-shared/config)
  database/                 PostgreSQL connection, migrations (embedded via go:embed)
  graph/                    gqlgen GraphQL schema, resolvers, and generated code
  integration/              Integration tests (require Docker)
  kafka/                    Kafka consumer
  model/                    Domain types
  observability/            Logging and health (via storm-data-shared) + Prometheus metrics
  store/                    PostgreSQL query layer (store, querybuilder, aggregations)
data/mock/                  Sample storm report JSON for testing
```

## Documentation

See the [project wiki](../../wiki) for detailed documentation:

- [Architecture](../../wiki/Architecture) -- Project structure, layer responsibilities, database schema, and capacity
- [Configuration](../../wiki/Configuration) -- Environment variables
- [Development](../../wiki/Development) -- Build, test, lint, CI, and project conventions
- [API Reference](../../wiki/API-Reference) -- GraphQL types, queries, filter options
