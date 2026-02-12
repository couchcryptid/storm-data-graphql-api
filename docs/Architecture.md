# Architecture

```
Kafka (enriched) ──> Consumer ──> Store ──> PostgreSQL
                                    ^
       GraphQL /query ──> Resolvers─┘
```

## Project Structure

```
cmd/server/main.go              Entry point, wires all components
internal/
  config/config.go              Environment-based configuration (uses storm-data-shared/config)
  model/storm_report.go         Domain types (StormReport, Geo, Location, Filter)
  database/
    db.go                       pgx connection pool + migration runner
    readiness.go                Readiness checker (implements storm-data-shared ReadinessChecker)
    migrations/                 Embedded SQL migration files
  store/
    store.go                    Store type, CRUD operations, row scanning
    querybuilder.go             WHERE clause construction, geo/haversine, sorting
    aggregations.go             CTE aggregation queries and result types
  graph/
    schema.graphqls             GraphQL schema definition (source of truth for API)
    resolver.go                 Resolver struct with store dependency
    schema.resolvers.go         Query resolver implementations
    generated.go                gqlgen generated execution engine (DO NOT EDIT)
    models_gen.go               gqlgen generated models (DO NOT EDIT)
  kafka/consumer.go             Kafka consumer with manual offset commit
  integration/                  Integration tests using testcontainers-go
  observability/
    logging.go                  Delegates to storm-data-shared for slog logger creation
    health.go                   Delegates to storm-data-shared for health endpoints
    metrics.go                  Prometheus metric definitions (service-specific)
    middleware.go               Chi HTTP metrics middleware
data/mock/
  storm_reports_240426_transformed.json   Mock data (source of truth for message shape)
```

## Layer Responsibilities

### Model (`internal/model`)

Defines the domain types that all layers share. The struct shape is derived directly from the mock JSON file in `data/mock/`. These same types are bound to gqlgen's GraphQL types via `gqlgen.yml`, so there are no separate graph models.

### Store (`internal/store`)

Handles all PostgreSQL interactions, split into three focused files:

- **`store.go`** -- Store type, `InsertStormReport(s)`, `ListStormReports`, `LastUpdated`, and row scanning
- **`querybuilder.go`** -- Dynamic WHERE clause construction from filter structs, geo/haversine calculations, bounding box pre-filters, sorting helpers
- **`aggregations.go`** -- CTE-based aggregation query (`Aggregations`), result types (`AggResult`, `EventTypeGroup`, `StateGroup`, `TimeGroup`)

The database schema flattens the nested JSON structure — `geo.lat`/`geo.lon` become `geo_lat`/`geo_lon` columns, `location.*` fields become `location_*` columns, and `measurement.*` fields become `measurement_*` columns.

`ListStormReports` builds dynamic WHERE clauses from the filter struct, with support for array filters (using PostgreSQL `ANY()`), sorting, and pagination. Geographic radius filtering uses the Haversine formula in SQL with a bounding box pre-filter for index efficiency.

### Graph (`internal/graph`)

Schema-first GraphQL layer using gqlgen. The schema is defined in `schema.graphqls`, and resolvers are thin — they delegate directly to the store layer with no business logic.

To regenerate after schema changes:

```bash
make generate
```

### Kafka Consumer (`internal/kafka`)

Consumes from the `transformed-weather-data` topic using `segmentio/kafka-go`. Uses manual offset commit (`FetchMessage`/`CommitMessages`) — offsets are only committed after successful database insertion. If a DB insert fails, the message is not committed and will be redelivered on restart.

### Observability (`internal/observability`)

Prometheus metrics, HTTP middleware, and health endpoints. Logging and health endpoint handlers delegate to the [storm-data-shared](https://github.com/couchcryptid/storm-data-shared) `observability` package. `NewMetrics()` registers all application metrics (HTTP, Kafka, database) with the default Prometheus registry. `NewTestMetrics()` uses a throwaway registry for test isolation. The Chi middleware records request duration and count using route patterns (not raw paths) to prevent label cardinality explosion.

Endpoints:

- `GET /healthz` — liveness probe (always 200, via shared `LivenessHandler`)
- `GET /readyz` — readiness probe (pings the database pool, via shared `ReadinessHandler`)
- `GET /metrics` — Prometheus scrape endpoint

### Database (`internal/database`)

Manages the pgx connection pool, runs embedded SQL migrations on startup, and provides a `PoolReadiness` checker for the readiness probe. Migrations are embedded into the binary using `//go:embed`.

## Database Schema

```sql
CREATE TABLE storm_reports (
    id                          TEXT PRIMARY KEY,
    event_type                  TEXT NOT NULL,
    geo_lat                     DOUBLE PRECISION NOT NULL,
    geo_lon                     DOUBLE PRECISION NOT NULL,
    measurement_magnitude       DOUBLE PRECISION NOT NULL,
    measurement_unit            TEXT NOT NULL,
    event_time                  TIMESTAMPTZ NOT NULL,
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
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### Indexes

| Index | Columns | Purpose |
|-------|---------|---------|
| `idx_event_time` | `event_time` | Date range queries, ORDER BY |
| `idx_event_type` | `event_type` | Filter by event type (hail, tornado, wind) |
| `idx_state` | `location_state` | Filter by state |
| `idx_severity` | `measurement_severity` | Filter by severity level |
| `idx_event_type_state_time` | `event_type, location_state, event_time` | Composite for the typical "type + state + time" filter |
| `idx_geo` | `geo_lat, geo_lon` | Bounding box pre-filter for radius queries |

## Design Decisions

### Schema-First GraphQL with Direct Model Binding

The GraphQL schema is defined in `.graphqls` files. gqlgen generates the execution engine, but domain models (`internal/model`) are bound directly via `gqlgen.yml` rather than using generated model types.

**Why**: The schema is the API contract — frontend developers can read it without knowing Go. Direct model binding eliminates a translation layer between graph types and domain types. Only one field resolver (`eventType` → `type`) is needed to bridge a naming difference.

### Thin Resolvers

Resolvers contain no business logic. They validate input, delegate to the store, and assemble the response.

**Why**: Keeps the GraphQL layer as a presentation concern. All data access logic lives in the store, which is testable independently of GraphQL.

### Field-Aware Parallel Query Execution

The resolver inspects which GraphQL fields were requested (`collectFields`) and only runs queries for those fields, using `errgroup` for parallel execution.

**Why**: A typical `stormReports` query runs up to 3 parallel database calls (reports, aggregations, meta). If the client only requests `reports`, the aggregation and meta queries never execute. This avoids unnecessary database work while keeping the resolver simple.

### Dynamic WHERE Clause Building

`buildWhereClause` constructs parameterized SQL from the filter struct, using positional `$N` parameters with an incrementing index.

**Why**: The GraphQL filter has many optional fields (time range, states, types, severity, radius). Building WHERE clauses dynamically avoids maintaining dozens of static query variants. Parameterized queries prevent SQL injection.

### Haversine with Bounding Box Pre-filter

Radius queries first apply a rectangular lat/lon bounding box (uses the `idx_geo` B-tree index), then apply the precise haversine great-circle distance formula to the remaining rows.

**Why**: The haversine formula is expensive to compute across every row. The bounding box eliminates most rows cheaply via index scan, limiting haversine computation to a small candidate set. The approximation (`~69 miles/degree`) is sufficient for the pre-filter since haversine corrects the final result.

**Scaling note**: For the current dataset size (~300 events/day), a composite B-tree index on `(geo_lat, geo_lon)` with bounding-box pre-filter is sufficient and avoids adding PostGIS as a dependency. At significantly larger scale, a PostGIS `geography` column with GIST index would enable native spatial operators (`ST_DWithin`) with better performance characteristics for dense datasets and would support dynamic vector tile rendering via `ST_AsMVT`.

### Embedded SQL Migrations

Database migrations are embedded into the binary via `//go:embed` and run automatically on startup using `golang-migrate`.

**Why**: The binary is self-contained — no external migration files to deploy or keep in sync. Migrations run before the server accepts traffic, ensuring the schema is always up to date.

### Idempotent Writes

All inserts use `ON CONFLICT (id) DO NOTHING` with deterministic SHA-256 IDs.

**Why**: Combined with at-least-once Kafka delivery, this makes the write path naturally idempotent. Duplicate messages (from consumer restarts or rebalances) are silently deduplicated. No additional deduplication infrastructure needed.

### Query Protection Layers

Three layers protect against expensive or abusive queries:

1. **Complexity budget** (600) — gqlgen estimates query cost based on field weights; queries exceeding the budget are rejected before execution
2. **Depth limit** (7) — prevents deeply nested queries
3. **Concurrency limit** (2) — a channel-based semaphore in Chi middleware returns 503 when all slots are occupied

**Why**: GraphQL's flexibility makes it easy for clients to construct queries that are expensive to resolve. These limits bound the worst case without restricting normal usage patterns.

### Batch Kafka Consumer

The consumer fetches messages in time-bounded batches (configurable via `BATCH_SIZE` and `BATCH_FLUSH_INTERVAL`), inserts them in a single `pgx.Batch` call, and commits offsets only after successful insertion.

**Why**: Batch database writes amortize connection overhead and reduce round trips. Time-bounded fetching ensures partial batches are flushed promptly rather than waiting indefinitely for a full batch.

## Capacity

SPC data volumes are small (~1,000--5,000 records/day during storm season). The Kafka consumer processes an entire day's data in under 1 minute. The GraphQL read path executes 5 parallel database queries per filter via `errgroup`, typically completing in 2--50 ms. Six indexes cover the primary query patterns (see above).

The 256 MB container memory limit provides 4--12x headroom over the ~20--60 MB steady-state footprint. The write path is over-provisioned for expected load; read path performance depends on dataset size and query complexity.

For horizontal scaling on the write path, deploy multiple instances with Kafka consumer groups (`KAFKA_GROUP_ID`). Read path scaling is handled by adding API replicas behind a load balancer.

## Related

- [System Architecture](https://github.com/couchcryptid/storm-data-system/wiki/Architecture) -- full pipeline design, deployment topology, and improvement roadmap
- [ETL Architecture](https://github.com/couchcryptid/storm-data-etl/wiki/Architecture) -- upstream service that publishes enriched events to Kafka
- [Shared Architecture](https://github.com/couchcryptid/storm-data-shared/wiki/Architecture) -- shared library packages used by the API
- [[API Reference]] -- GraphQL schema, types, filters, and example queries
- [[Configuration]] -- environment variables and operational endpoints
- [[Development]] -- build, test, lint, CI, and project conventions
