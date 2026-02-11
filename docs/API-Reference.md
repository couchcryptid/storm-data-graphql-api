# API Reference

The GraphQL API is served at `/query`. A GraphQL Playground is available at `/` for interactive exploration.

## Query

### stormReports

Fetch storm reports with filtering, sorting, pagination, and aggregations. Accepts a single filter with required time bounds.

```graphql
query {
  stormReports(filter: {
    timeRange: { from: "2024-04-26T00:00:00Z", to: "2024-04-27T00:00:00Z" }
    eventTypes: [HAIL]
    states: ["TX"]
  }) {
    totalCount
    hasMore
    reports {
      id
      eventType
      measurement { magnitude unit severity }
      geo { lat lon }
      location { name state county }
      eventTime
      geocoding { formattedAddress placeName confidence source }
    }
    aggregations {
      byEventType { eventType count maxMeasurement { magnitude unit } }
      byState { state count counties { county count } }
      byHour { bucket count }
    }
    meta { lastUpdated dataLagMinutes }
  }
}
```

## Types

### StormReportsResult

The top-level result returned by `stormReports`.

| Field | Type | Description |
|-------|------|-------------|
| `totalCount` | `Int!` | Total matching reports (ignores `limit`/`offset`) |
| `hasMore` | `Boolean!` | Whether more results exist beyond the current page |
| `reports` | `[StormReport!]!` | Matching reports (respects sorting and pagination) |
| `aggregations` | `StormAggregations!` | Aggregated statistics for the matching reports |
| `meta` | `QueryMeta!` | Metadata about data freshness |

### StormAggregations

| Field | Type | Description |
|-------|------|-------------|
| `totalCount` | `Int!` | Total matching reports |
| `byEventType` | `[EventTypeGroup!]!` | Report counts grouped by event type |
| `byState` | `[StateGroup!]!` | Report counts grouped by state and county |
| `byHour` | `[TimeGroup!]!` | Report counts grouped by time bucket |

### QueryMeta

| Field | Type | Description |
|-------|------|-------------|
| `lastUpdated` | `DateTime` | Most recent `processedAt` timestamp in the database |
| `dataLagMinutes` | `Int` | Minutes since `lastUpdated` |

### StormReport

| Field | Type | Description |
|-------|------|-------------|
| `id` | `ID!` | Unique identifier (deterministic SHA-256 hash) |
| `eventType` | `String!` | Event type: `hail`, `tornado`, or `wind` |
| `geo` | `Geo!` | Geographic coordinates |
| `measurement` | `Measurement!` | Magnitude, unit, and severity |
| `eventTime` | `DateTime!` | When the event occurred (RFC 3339) |
| `sourceOffice` | `String!` | NWS office code (e.g., `FWD`, `OAX`, `TSA`) |
| `location` | `Location!` | Location details |
| `comments` | `String!` | Free-text description of the event |
| `timeBucket` | `DateTime!` | Hourly time bucket for aggregation |
| `processedAt` | `DateTime!` | When the record was processed |
| `geocoding` | `Geocoding!` | Geocoding enrichment results (empty when geocoding disabled) |

### Measurement

| Field | Type | Description |
|-------|------|-------------|
| `magnitude` | `Float!` | Event magnitude (interpretation depends on `unit`) |
| `unit` | `String!` | Magnitude unit: `in` (hail inches), `mph` (wind speed), `f_scale` (tornado) |
| `severity` | `String` | Severity level (nullable; e.g., `moderate`, `severe`) |

### Geo

| Field | Type | Description |
|-------|------|-------------|
| `lat` | `Float!` | Latitude |
| `lon` | `Float!` | Longitude |

### Location

| Field | Type | Description |
|-------|------|-------------|
| `raw` | `String!` | Raw location string (e.g., `8 ESE Chappel`) |
| `name` | `String!` | City/place name |
| `distance` | `Float` | Distance from named location in miles (nullable) |
| `direction` | `String` | Cardinal direction from named location (nullable) |
| `state` | `String!` | Two-letter state code |
| `county` | `String!` | County name |

### Geocoding

| Field | Type | Description |
|-------|------|-------------|
| `formattedAddress` | `String!` | Full address from geocoding (empty if geocoding disabled) |
| `placeName` | `String!` | Short place name from geocoding (empty if geocoding disabled) |
| `confidence` | `Float!` | Geocoding confidence score 0-1 (0 if geocoding disabled) |
| `source` | `String!` | Geocoding method: `forward`, `reverse`, `original`, `failed`, or empty |

### Aggregation Types

#### EventTypeGroup

| Field | Type | Description |
|-------|------|-------------|
| `eventType` | `String!` | Event type |
| `count` | `Int!` | Number of reports |
| `maxMeasurement` | `Measurement` | Highest magnitude measurement in this group |

#### StateGroup

| Field | Type | Description |
|-------|------|-------------|
| `state` | `String!` | Two-letter state code |
| `count` | `Int!` | Number of reports |
| `counties` | `[CountyGroup!]!` | Breakdown by county |

#### CountyGroup

| Field | Type | Description |
|-------|------|-------------|
| `county` | `String!` | County name |
| `count` | `Int!` | Number of reports |

#### TimeGroup

| Field | Type | Description |
|-------|------|-------------|
| `bucket` | `DateTime!` | Hourly time bucket |
| `count` | `Int!` | Number of reports |

## Enums

### EventType

`HAIL`, `WIND`, `TORNADO`

### Severity

`MINOR`, `MODERATE`, `SEVERE`, `EXTREME`

### SortField

`EVENT_TIME`, `MAGNITUDE`, `LOCATION_STATE`, `EVENT_TYPE`

### SortOrder

`ASC`, `DESC` (default: `DESC`)

## Filter Options

### StormReportFilter

| Field | Type | Description |
|-------|------|-------------|
| `timeRange` | `TimeRange!` | Time bounds (required) |
| `near` | `GeoRadiusFilter` | Center point and radius for geographic search |
| `states` | `[String!]` | Match any of the listed state codes |
| `counties` | `[String!]` | Match any of the listed county names |
| `eventTypes` | `[EventType!]` | Global event type filter (enum values) |
| `severity` | `[Severity!]` | Global severity filter (enum values) |
| `minMagnitude` | `Float` | Global minimum magnitude threshold |
| `eventTypeFilters` | `[EventTypeFilter!]` | Per-type overrides (max 3, see below) |
| `sortBy` | `SortField` | Sort field |
| `sortOrder` | `SortOrder` | Sort direction (default: `DESC`) |
| `limit` | `Int` | Maximum reports to return (max 20, default 20) |
| `offset` | `Int` | Number of reports to skip (for pagination) |

### TimeRange

| Field | Type | Description |
|-------|------|-------------|
| `from` | `DateTime!` | Events starting at or after this time |
| `to` | `DateTime!` | Events starting before this time (`to` must be after `from`) |

### GeoRadiusFilter

| Field | Type | Description |
|-------|------|-------------|
| `lat` | `Float!` | Center latitude |
| `lon` | `Float!` | Center longitude |
| `radiusMiles` | `Float` | Search radius in miles (default: 20, max: 200) |

### EventTypeFilter

Per-type override that takes precedence over global filter fields for a specific event type. At most 3, no duplicate event types.

| Field | Type | Description |
|-------|------|-------------|
| `eventType` | `EventType!` | Which event type this override applies to |
| `severity` | `[Severity!]` | Override severity filter for this type |
| `minMagnitude` | `Float` | Override minimum magnitude for this type |
| `radiusMiles` | `Float` | Override search radius for this type (max: 200) |

## Example Queries

### Geographic Radius Search

```graphql
query {
  stormReports(filter: {
    timeRange: { from: "2024-04-26T00:00:00Z", to: "2024-04-27T00:00:00Z" }
    near: { lat: 32.75, lon: -97.15, radiusMiles: 20.0 }
  }) {
    totalCount
    reports {
      id
      eventType
      geo { lat lon }
      location { name state }
    }
  }
}
```

### Sorted and Paginated Query

```graphql
query {
  stormReports(filter: {
    timeRange: { from: "2024-04-26T00:00:00Z", to: "2024-04-27T00:00:00Z" }
    eventTypes: [HAIL]
    sortBy: MAGNITUDE
    sortOrder: DESC
    limit: 10
    offset: 0
  }) {
    totalCount
    hasMore
    reports {
      id
      measurement { magnitude unit }
      location { name county state }
    }
  }
}
```

### Per-Type Overrides

Search for hail within 50 miles and tornadoes within 100 miles simultaneously:

```graphql
query {
  stormReports(filter: {
    timeRange: { from: "2024-04-26T15:00:00Z", to: "2024-04-26T20:00:00Z" }
    near: { lat: 32.75, lon: -97.15 }
    eventTypeFilters: [
      { eventType: HAIL, severity: [SEVERE, MODERATE], minMagnitude: 1.0, radiusMiles: 50.0 }
      { eventType: TORNADO, radiusMiles: 100.0 }
    ]
  }) {
    totalCount
    reports {
      id
      eventType
      measurement { magnitude unit severity }
      location { name county }
      comments
    }
    aggregations {
      byEventType { eventType count maxMeasurement { magnitude unit } }
    }
  }
}
```
