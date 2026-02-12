# Configuration

All configuration is via environment variables. Every variable has a default suitable for local development with the Docker Compose stack.

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP server port |
| `DATABASE_URL` | `postgres://storm:storm@localhost:5432/stormdata?sslmode=disable` | PostgreSQL connection string |
| `KAFKA_BROKERS` | `localhost:29092` | Kafka broker address |
| `KAFKA_TOPIC` | `transformed-weather-data` | Kafka topic to consume |
| `KAFKA_GROUP_ID` | `storm-data-api` | Kafka consumer group ID |
| `LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | `json` | Log format: `json` or `text` |
| `SHUTDOWN_TIMEOUT` | `10s` | Graceful shutdown deadline (Go duration) |
| `BATCH_SIZE` | `50` | Kafka messages per batch (1--1000) |
| `BATCH_FLUSH_INTERVAL` | `500ms` | Max wait before flushing a partial batch (Go duration) |

## Shared Parsers

Several environment variables use parsers from the [storm-data-shared](https://github.com/couchcryptid/storm-data-shared/wiki/Configuration) library:

| Variable | Shared Parser |
|----------|--------------|
| `KAFKA_BROKERS` | `config.ParseBrokers()` |
| `BATCH_SIZE` | `config.ParseBatchSize()` |
| `BATCH_FLUSH_INTERVAL` | `config.ParseBatchFlushInterval()` |
| `SHUTDOWN_TIMEOUT` | `config.ParseShutdownTimeout()` |
| `LOG_LEVEL`, `LOG_FORMAT` | `observability.NewLogger()` |

API-specific variables (`PORT`, `DATABASE_URL`, `KAFKA_TOPIC`, `KAFKA_GROUP_ID`) are parsed in `internal/config/config.go`.

## Docker Compose Environment Files

The Compose stack uses per-service env files to keep credentials out of `compose.yml`:

| File | Service | Contents |
|------|---------|----------|
| `.env.postgres` | `postgres` | `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` |
| `.env.kafka` | `kafka` | KRaft configuration variables |

These files are referenced via `env_file:` in `compose.yml`.

## Local Development

No `.env` file is needed for local development. The defaults connect to the Docker Compose services:

```bash
make docker-up   # Start Postgres + Kafka
make run         # Start server with defaults
```

## Custom Configuration

Override any variable as needed:

```bash
PORT=3000 DATABASE_URL=postgres://user:pass@db:5432/mydb make run
```

## Operational Endpoints

These endpoints are always available and do not require configuration:

| Endpoint | Description |
|----------|-------------|
| `GET /healthz` | Liveness probe — always returns 200 |
| `GET /readyz` | Readiness probe — returns 200 if Postgres is reachable, 503 otherwise |
| `GET /metrics` | Prometheus scrape endpoint (all `storm_api_*` metrics) |

## Docker

When running the server in Docker, pass environment variables to configure external service connections:

```bash
docker run -e DATABASE_URL=postgres://user:pass@host:5432/db \
           -e KAFKA_BROKERS=kafka:9092 \
           -p 8080:8080 \
           storm-data-api
```

## Related

- [Shared Configuration](https://github.com/couchcryptid/storm-data-shared/wiki/Configuration) -- shared parsers for Kafka, batch, and shutdown settings
- [ETL Configuration](https://github.com/couchcryptid/storm-data-etl/wiki/Configuration) -- upstream Kafka topic and enrichment settings
- [[Architecture]] -- design decisions and query protection layers
- [[Development]] -- build, test, lint, CI, and project conventions
