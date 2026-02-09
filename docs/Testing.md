# Testing

## Run All Tests

```bash
make test
```

## Unit Tests

```bash
make test-unit
```

Unit tests validate model deserialization and enum behavior. They do not require Docker or any external services.

| Test | What it verifies |
|------|------------------|
| `TestLoadMockData` | Mock JSON deserializes into 30 `StormReport` structs |
| `TestMockDataTypes` | 10 hail, 10 tornado, 10 wind reports |
| `TestMockDataFields` | All required fields are populated (ID, coordinates, state, begin_time, source_office) |
| `TestMockDataOptionalFields` | Optional fields (`severity`, `distance`) are present on some but not all records |
| `TestMockDataHailReport` | Specific field values for `hail-1` match expected data |
| `TestSortFieldIsValid` | `SortField` enum validates known values and rejects invalid/lowercase strings |
| `TestSortFieldString` | `SortField.String()` returns correct uppercase representation |
| `TestSortOrderIsValid` | `SortOrder` enum validates `ASC`/`DESC` and rejects invalid values |
| `TestSortOrderString` | `SortOrder.String()` returns correct uppercase representation |

## Integration Tests

```bash
make test-integration
```

Integration tests use [testcontainers-go](https://github.com/testcontainers/testcontainers-go) to spin up real PostgreSQL and Kafka containers. **Docker must be running.**

| Test | What it verifies |
|------|------------------|
| `TestStoreInsertAndQuery` | Insert all 30 mock reports, then test: get by ID, list all, filter by type, filter by state, geo radius search, get non-existent returns nil |
| `TestStoreAggregations` | `CountByType` (3 groups, max magnitude), `CountByState` (with county sub-groups, sum validation), `CountByHour` (bucket totals), `LastUpdated`, `CountByType` with type filter |
| `TestStoreFilters` | Severity filter, multiple severities, counties, `minMagnitude`, combined filters (type + state + severity), empty result, multiple types |
| `TestStoreSortingAndPagination` | Sort by magnitude DESC/ASC, sort by state, limit, offset with page comparison, offset beyond total |
| `TestGraphQLEndpoint` | Full GraphQL query: list all (30), filter by type (10 hail), get single report by ID with nested fields |
| `TestGraphQLAggregations` | Full GraphQL response: `totalCount`, `byType` with counts, `byState` with county sub-groups, `byHour` bucket totals, `lastUpdated`, `dataLagMinutes` |
| `TestKafkaConsumerIntegration` | Produce 30 mock messages to Kafka, consume them, insert to Postgres, verify all 30 are in the database |

### Container Images

| Service | Image |
|---------|-------|
| PostgreSQL | `postgres:16` |
| Kafka | `confluentinc/confluent-local:7.6.0` |

The Kafka integration tests use the Confluent image (not `apache/kafka`) because the testcontainers Kafka module requires Confluent's startup scripts.

## Linting

```bash
make lint
```

Requires [golangci-lint](https://golangci-lint.run/). Configuration is in `.golangci.yml`.

Enabled linters: `errcheck`, `govet`, `staticcheck`, `unused`, `gosimple`, `ineffassign`, `typecheck`, `bodyclose`, `gocritic`, `gosec`, `misspell`, `prealloc`, `revive`, `unconvert`, `unparam`, `errname`, `exhaustive`, `sqlclosecheck`, `noctx`.
