# Storm Data GraphQL API

A Go service that consumes transformed storm weather reports from a Kafka topic, persists them to PostgreSQL, and serves them through a GraphQL API. Part of the storm data pipeline.

## Pages

- [[Architecture]] -- Project structure, layer responsibilities, database schema
- [[Configuration]] -- Environment variables
- [[Deployment]] -- Docker Compose setup and production considerations
- [[Development]] -- Build, test, lint, CI, and project conventions
- [[Data Model]] -- Kafka message shape, event types, field mapping
- [[API Reference]] -- GraphQL types, queries, filter options
- [[Performance]] -- Query performance, scaling, and bottleneck analysis
