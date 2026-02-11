# Code Quality

Quality philosophy, tooling, and enforcement for the GraphQL API. For pipeline-wide quality standards, see the [system Code Quality page](https://github.com/couchcryptid/storm-data-system/wiki/Code-Quality).

## Coding Philosophy

### Schema-first GraphQL

The `.graphqls` schema is the source of truth for the API contract. Frontend developers can read the schema without understanding Go. `gqlgen` generates the execution engine from the schema, ensuring the implementation always matches the contract. Domain models are bound directly via `gqlgen.yml` -- no translation layer between graph types and domain types.

### Thin resolvers

Resolvers contain no business logic. They validate input, delegate to the store, and assemble the response. This keeps the GraphQL layer as a presentation concern. All data access logic lives in the store, which is testable independently of GraphQL.

### Pure domain logic

The `model` package has no infrastructure imports. Domain types are plain Go structs derived from the mock JSON shape. This makes model validation testable without a database, Kafka, or HTTP server.

### Field-aware query execution

The resolver inspects which GraphQL fields were requested (`collectFields`) and only runs queries for those fields, using `errgroup` for parallel execution. A client that requests only `reports` never triggers aggregation or meta queries. This avoids unnecessary database work while keeping the resolver simple.

### Idempotent writes

All inserts use `ON CONFLICT (id) DO NOTHING` with deterministic SHA-256 IDs. Combined with at-least-once Kafka delivery, duplicate messages are silently deduplicated without additional infrastructure.

### Defense in depth for queries

Three layers protect against expensive or abusive queries:

1. **Complexity budget** (600) -- gqlgen estimates query cost; queries exceeding the budget are rejected before execution
2. **Depth limit** (7) -- prevents deeply nested queries
3. **Concurrency limit** (2) -- Chi middleware returns 503 when all slots are occupied

### Constructor injection

All adapters (Kafka consumer, HTTP handlers, database pool) accept `*slog.Logger` and metrics via their constructors. No global state, no service locators. Every component is testable in isolation.

## Static Analysis

### golangci-lint

15 enabled linters covering correctness, security, style, and performance:

| Category | Linters |
|----------|---------|
| Correctness | `errcheck`, `govet`, `staticcheck`, `errorlint`, `exhaustive` |
| Security | `gosec`, `noctx`, `sqlclosecheck` |
| Style | `gocritic` (diagnostic/style/performance), `revive` (exported) |
| Complexity | `gocyclo` (threshold: 15) |
| Hygiene | `misspell`, `unparam`, `errname`, `unconvert`, `prealloc` |
| Test quality | `testifylint` |

`sqlclosecheck` is unique to this service -- it verifies database rows and statements are properly closed, preventing connection pool exhaustion.

gqlgen-generated files (`generated.go`, `models_gen.go`) are excluded from analysis.

### SonarQube Cloud

Analyzed via CI on every push and pull request: [SonarCloud dashboard](https://sonarcloud.io/summary/overall?id=couchcryptid_storm-data-api)

SonarCloud configuration (`sonar-project.properties`):

- Excludes gqlgen-generated files from analysis
- Reports Go coverage via `coverage.out`
- Allows idiomatic Go test naming (`TestX_Y_Z`) on test files

## Security

| Layer | What It Catches |
|-------|----------------|
| `gosec` | SQL injection, weak crypto, hardcoded credentials |
| `sqlclosecheck` | Unclosed database rows and statements |
| `noctx` | HTTP requests without context (timeout/cancellation) |
| `gitleaks` | Secrets in source code |
| Query complexity budget | GraphQL denial-of-service via expensive queries |
| Depth limit | Deeply nested query attacks |
| Concurrency limit | Request flooding |

## Quality Gates

### Pre-commit Hooks

`.pre-commit-config.yaml` runs on every commit:

- File hygiene: trailing whitespace, end-of-file newline, YAML/JSON validation
- Formatting: `gofmt`, `goimports`
- Go checks: `go vet`, `go build`
- Linting: `golangci-lint` (5-minute timeout)
- GraphQL: schema linting via `graphql-schema-linter`
- Security: `gitleaks`, `detect-private-key`, `check-added-large-files`
- Documentation: `yamllint`

### CI Pipeline

Every push and pull request to `main` runs:

| Job | Command | What It Enforces |
|-----|---------|-----------------|
| `test-unit` | `make test-unit` | Unit tests with race detector (`-race -count=1`) |
| `lint` | `make lint` | golangci-lint with 15 linters |
| `build` | `make build` | Compile check |
| `sonarcloud` | SonarCloud scan | Coverage, bugs, vulnerabilities, code smells, security hotspots |

### SonarCloud Quality Gate

Uses the default "Sonar way" gate on new code: >= 80% coverage, <= 3% duplication, A ratings for reliability/security/maintainability, 100% security hotspots reviewed.

## Testing

Tests are organized in two tiers. See [[Development]] for commands and test inventories.

| Tier | What It Covers | Docker |
|------|---------------|--------|
| Unit | Model deserialization, enum validation, sort behavior | No |
| Integration | Store operations, GraphQL endpoint, Kafka consumer (testcontainers) | Yes |

All tests run with `-race -count=1`. Integration tests use real PostgreSQL and Kafka containers via testcontainers-go.

## Related

- [System Code Quality](https://github.com/couchcryptid/storm-data-system/wiki/Code-Quality) -- pipeline-wide quality standards
- [Shared Library Code Quality](https://github.com/couchcryptid/storm-data-shared/wiki/Code-Quality) -- shared tooling and conventions
- [[Development]] -- commands, CI pipeline, project conventions
- [[Architecture]] -- design decisions and tradeoffs
