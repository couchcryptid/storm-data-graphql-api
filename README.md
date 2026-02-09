# Storm Data GraphQL API

Go service that consumes storm weather reports from a Kafka topic, persists them to PostgreSQL, and serves them through a GraphQL API.

```
Kafka (transformed-weather-data) --> Consumer --> Store --> PostgreSQL
                                                    ^
             GraphQL (/query) --> Resolvers --------+
```

## Quick Start

```bash
make docker-up    # Start Postgres + Kafka
make run          # Start server (runs migrations automatically)
open http://localhost:8080   # GraphQL Playground
```

## Prerequisites

- Go 1.25.6+
- Docker and Docker Compose

## Example Query

```graphql
{
  stormReports(filter: {
    beginTimeAfter: "2024-04-26T00:00:00Z"
    beginTimeBefore: "2024-04-27T00:00:00Z"
    types: ["hail"]
    states: ["TX"]
  }) {
    totalCount
    reports {
      id
      magnitude
      geo { lat lon }
      location { name county state }
      comments
      beginTime
    }
    byType { type count maxMagnitude }
  }
}
```

## Make Targets

| Target | Description |
|--------|-------------|
| `make docker-up` | Start Postgres + Kafka |
| `make docker-down` | Stop and remove containers + volumes |
| `make run` | Run server locally |
| `make build` | Compile binary to `bin/server` |
| `make generate` | Regenerate gqlgen GraphQL code |
| `make test` | Run all tests |
| `make test-unit` | Run unit tests only |
| `make test-integration` | Run integration tests (requires Docker) |
| `make lint` | Run golangci-lint |

## Operational Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /healthz` | Liveness probe (always 200) |
| `GET /readyz` | Readiness probe (pings Postgres) |
| `GET /metrics` | Prometheus metrics |

## Configuration

All via environment variables with local dev defaults:

| Variable | Default |
|----------|---------|
| `PORT` | `8080` |
| `DATABASE_URL` | `postgres://storm:storm@localhost:5432/stormdata?sslmode=disable` |
| `KAFKA_BROKERS` | `localhost:29092` |
| `KAFKA_TOPIC` | `transformed-weather-data` |
| `KAFKA_GROUP_ID` | `storm-data-graphql-api` |

## Documentation

See the [Wiki](../../wiki) for detailed documentation:

- [Architecture](../../wiki/Architecture) — project structure, layer responsibilities, database schema
- [Data Model](../../wiki/Data-Model) — Kafka message shape, event types, field mapping
- [API Reference](../../wiki/API-Reference) — GraphQL types, queries, filter options
- [Configuration](../../wiki/Configuration) — environment variables
- [Testing](../../wiki/Testing) — unit and integration test details
