# Storm Data GraphQL API

Go service that consumes storm weather reports from a Kafka topic, persists them to PostgreSQL, and serves them through a GraphQL API.

## Data Flow

```
Kafka (transformed-weather-data) --> Consumer --> Store --> PostgreSQL
                                                    ^
GraphQL queries --> chi router --> Resolvers --------+
```

Storm reports arrive as JSON messages on the `transformed-weather-data` Kafka topic. The consumer deserializes each message, inserts it into PostgreSQL, and only commits the Kafka offset after a successful write. The GraphQL API reads from the same database, supporting filtered queries by type, location, date range, severity, and geographic radius.

## Tech Stack

| Component | Technology |
|-----------|-----------|
| Language | Go 1.25.6 |
| GraphQL | gqlgen (schema-first) |
| HTTP Router | chi with logger, recoverer, CORS middleware |
| Kafka | segmentio/kafka-go |
| Database | PostgreSQL 16 via pgx |
| Migrations | golang-migrate (embedded SQL) |
| Local Dev | Docker Compose (Kafka KRaft + Postgres) |
| Integration Tests | testcontainers-go |
| Linting | golangci-lint |

## Quick Start

```bash
# Start infrastructure
make docker-up

# Run the server
make run

# Open GraphQL Playground
open http://localhost:8080
```

See [[Configuration]] for environment variables and setup details.
