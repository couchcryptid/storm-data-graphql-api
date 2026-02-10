# Architecture

![Architecture](architecture.excalidraw.svg)

## Project Structure

```
cmd/server/main.go              Entry point, wires all components
internal/
  config/config.go              Environment-based configuration
  model/storm_report.go         Domain types (StormReport, Geo, Location, Filter)
  database/
    db.go                       pgx connection pool + migration runner
    readiness.go                Readiness checker wrapping pgxpool.Ping()
    migrations/                 Embedded SQL migration files
  store/store.go                PostgreSQL data access (insert, filtered list, aggregations)
  graph/
    schema.graphqls             GraphQL schema definition (source of truth for API)
    resolver.go                 Resolver struct with store dependency
    schema.resolvers.go         Query resolver implementations
    generated.go                gqlgen generated execution engine (DO NOT EDIT)
    models_gen.go               gqlgen generated models (DO NOT EDIT)
  kafka/consumer.go             Kafka consumer with manual offset commit
  integration/                  Integration tests using testcontainers-go
  observability/
    metrics.go                  Prometheus metric definitions
    middleware.go               Chi HTTP metrics middleware
    health.go                   Liveness and readiness HTTP handlers
data/mock/
  storm_reports_240526_transformed.json   Mock data (source of truth for message shape)
```

## Layer Responsibilities

### Model (`internal/model`)

Defines the domain types that all layers share. The struct shape is derived directly from the mock JSON file in `data/mock/`. These same types are bound to gqlgen's GraphQL types via `gqlgen.yml`, so there are no separate graph models.

### Store (`internal/store`)

Handles all PostgreSQL interactions. The database schema flattens the nested JSON structure — `geo.lat`/`geo.lon` become `geo_lat`/`geo_lon` columns, `location.*` fields become `location_*` columns, `measurement.*` fields become `measurement_*` columns, and `geocoding.*` fields become `geocoding_*` columns.

The `ListStormReports` method builds dynamic WHERE clauses from the filter struct, with support for array filters (using PostgreSQL `ANY()`), sorting, and pagination. Aggregation methods (`CountByType`, `CountByState`, `CountByHour`, `LastUpdated`) provide grouped summaries. Geographic radius filtering uses the Haversine formula in SQL with a bounding box pre-filter for index efficiency.

### Graph (`internal/graph`)

Schema-first GraphQL layer using gqlgen. The schema is defined in `schema.graphqls`, and resolvers are thin — they delegate directly to the store layer with no business logic.

To regenerate after schema changes:

```bash
make generate
```

### Kafka Consumer (`internal/kafka`)

Consumes from the `transformed-weather-data` topic using `segmentio/kafka-go`. Uses manual offset commit (`FetchMessage`/`CommitMessages`) — offsets are only committed after successful database insertion. If a DB insert fails, the message is not committed and will be redelivered on restart.

### Observability (`internal/observability`)

Prometheus metrics, HTTP middleware, and health endpoints. `NewMetrics()` registers all application metrics (HTTP, Kafka, database) with the default Prometheus registry. `NewTestMetrics()` uses a throwaway registry for test isolation. The Chi middleware records request duration and count using route patterns (not raw paths) to prevent label cardinality explosion.

Endpoints:

- `GET /healthz` — liveness probe (always 200)
- `GET /readyz` — readiness probe (pings the database pool)
- `GET /metrics` — Prometheus scrape endpoint

### Database (`internal/database`)

Manages the pgx connection pool, runs embedded SQL migrations on startup, and provides a `PoolReadiness` checker for the readiness probe. Migrations are embedded into the binary using `//go:embed`.

## Database Schema

```sql
CREATE TABLE storm_reports (
    id                          TEXT PRIMARY KEY,
    type                        TEXT NOT NULL,
    geo_lat                     DOUBLE PRECISION NOT NULL,
    geo_lon                     DOUBLE PRECISION NOT NULL,
    measurement_magnitude       DOUBLE PRECISION NOT NULL,
    measurement_unit            TEXT NOT NULL,
    begin_time                  TIMESTAMPTZ NOT NULL,
    end_time                    TIMESTAMPTZ NOT NULL,
    source                      TEXT NOT NULL,
    location_raw                TEXT NOT NULL,
    location_name               TEXT NOT NULL,
    location_distance           DOUBLE PRECISION,
    location_direction          TEXT,
    location_state              TEXT NOT NULL,
    location_county             TEXT NOT NULL,
    comments                    TEXT NOT NULL,
    measurement_severity        TEXT,
    source_office               TEXT NOT NULL,
    time_bucket                 TIMESTAMPTZ NOT NULL,
    processed_at                TIMESTAMPTZ NOT NULL,
    geocoding_formatted_address TEXT NOT NULL DEFAULT '',
    geocoding_place_name        TEXT NOT NULL DEFAULT '',
    geocoding_confidence        DOUBLE PRECISION NOT NULL DEFAULT 0,
    geocoding_source            TEXT NOT NULL DEFAULT '',
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### Indexes

| Index | Columns | Purpose |
|-------|---------|---------|
| `idx_begin_time` | `begin_time` | Date range queries, ORDER BY |
| `idx_type` | `type` | Filter by event type (hail, tornado, wind) |
| `idx_state` | `location_state` | Filter by state |
| `idx_severity` | `measurement_severity` | Filter by severity level |
| `idx_type_state_time` | `type, location_state, begin_time` | Composite for the typical "type + state + time" filter |
| `idx_geo` | `geo_lat, geo_lon` | Bounding box pre-filter for radius queries |

## Kafka Consumer Offset Strategy

![Kafka Offset Strategy](kafka-offset-strategy.excalidraw.svg)
