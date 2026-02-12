# Storm Data API

A Go service that consumes transformed storm weather reports from a Kafka topic, persists them to PostgreSQL, and serves them through a GraphQL API. Part of the storm data pipeline.

## Pipeline Position

```
Collector --> Kafka --> ETL --> Kafka --> [API] --> PostgreSQL + GraphQL
```

**Upstream**: The [ETL service](https://github.com/couchcryptid/storm-data-etl/wiki) publishes enriched events to the `transformed-weather-data` topic.

Uses the [storm-data-shared](https://github.com/couchcryptid/storm-data-shared/wiki) library for logging, health endpoints, and config parsing. For the full pipeline architecture, see the [system wiki](https://github.com/couchcryptid/storm-data-system/wiki).

## Pages

- [[Architecture]] -- Project structure, layer responsibilities, database schema, and capacity
- [[Configuration]] -- Environment variables
- [[Development]] -- Build, test, lint, CI, and project conventions
- [[API Reference]] -- GraphQL types, queries, filter options
