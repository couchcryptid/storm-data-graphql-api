# Deployment

## Docker Compose (Local Development)

The `compose.yml` at the repo root runs the full stack: PostgreSQL, Kafka (KRaft mode), and the API service.

### Start

```sh
make docker-up
```

### Stop

```sh
make docker-down
```

To remove volumes (Postgres data, Kafka data):

```sh
docker compose down -v
```

### Services

| Service                  | Image                | Port  | Description                            |
| ------------------------ | -------------------- | ----- | -------------------------------------- |
| `postgres`               | `postgres:16`        | 5432  | PostgreSQL database                    |
| `kafka`                  | `apache/kafka:3.7.0` | 29092 | Message broker (KRaft, no Zookeeper)   |
| `storm-data-api` | Built from `Dockerfile` | 8080 | GraphQL API service                   |

### Health Checks

All services have health checks configured:

- **Postgres**: `pg_isready -U storm -d stormdata`
- **Kafka**: `kafka-topics --list` against the internal listener
- **API**: HTTP `GET /healthz`

Services start in dependency order: Postgres + Kafka -> API (waits for both to be healthy).

### Resource Limits

| Service  | Memory Limit | Memory Reservation |
| -------- | ------------ | ------------------ |
| Postgres | 512 MB       | 256 MB             |
| Kafka    | 1 GB         | 512 MB             |
| API      | 256 MB       | 128 MB             |

## Docker Image

The service uses a multi-stage build:

1. **Build stage** (`golang:1.25-alpine`): Compiles the binary with `-ldflags="-s -w"` for a smaller output
2. **Runtime stage** (`gcr.io/distroless/static-debian12:nonroot`): Minimal image with no shell or package manager

The final image contains only the binary, CA certificates, and a static BusyBox binary (for health check `wget`).

### Build Manually

```sh
docker build -t storm-data-api .
```

### Run Standalone

```sh
docker run -p 8080:8080 \
  -e DATABASE_URL=postgres://user:pass@host:5432/stormdata?sslmode=disable \
  -e KAFKA_BROKERS=host.docker.internal:9092 \
  -e KAFKA_TOPIC=transformed-weather-data \
  storm-data-api
```

## Environment Files

| File            | Used By                    | Description                                       |
| --------------- | -------------------------- | ------------------------------------------------- |
| `.env`          | `storm-data-api`   | API service config (gitignored)                    |
| `.env.postgres` | `postgres` container       | `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` |
| `.env.kafka`    | `kafka` container          | Kafka KRaft broker settings (listeners, controller, replication) |

## Production

For cloud deployment options and cost analysis, see the [system Architecture wiki](https://github.com/couchcryptid/storm-data-system/wiki/Architecture#gcp-cloud-cost-analysis). Database migrations run automatically on startup -- ensure the database user has DDL permissions.

## Related

- [System Deployment](https://github.com/couchcryptid/storm-data-system/wiki/Deployment) -- full-stack Docker Compose with all services
- [System Architecture](https://github.com/couchcryptid/storm-data-system/wiki/Architecture) -- cloud cost analysis and deployment topology
- [[Configuration]] -- environment variables and operational endpoints
- [[Development]] -- local development setup and testing
