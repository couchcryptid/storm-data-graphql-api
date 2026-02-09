# API Reference

The GraphQL API is served at `/query`. A GraphQL Playground is available at `/` for interactive exploration.

## Query

### stormReports

Fetch storm reports with filtering, sorting, pagination, and aggregations. The filter is required and must include time bounds.

```graphql
query {
  stormReports(filter: {
    beginTimeAfter: "2024-04-26T00:00:00Z"
    beginTimeBefore: "2024-04-27T00:00:00Z"
    types: ["hail"]
    states: ["TX"]
  }) {
    totalCount
    reports {
      id
      type
      magnitude
      geo { lat lon }
      location { name state county }
      beginTime
      severity
    }
    byType { type count maxMagnitude }
    byState { state count counties { county count } }
    byHour { bucket count }
    lastUpdated
    dataLagMinutes
  }
}
```

## Types

### StormReportResult

The result envelope returned by `stormReports`.

| Field | Type | Description |
|-------|------|-------------|
| `reports` | `[StormReport!]!` | Matching reports (respects sorting and pagination) |
| `totalCount` | `Int!` | Total matching reports (ignores `limit`/`offset`) |
| `byType` | `[TypeGroup!]!` | Report counts grouped by event type |
| `byState` | `[StateGroup!]!` | Report counts grouped by state and county |
| `byHour` | `[TimeGroup!]!` | Report counts grouped by time bucket |
| `lastUpdated` | `DateTime` | Most recent `processedAt` timestamp in the database |
| `dataLagMinutes` | `Int` | Minutes since `lastUpdated` |

### StormReport

| Field | Type | Description |
|-------|------|-------------|
| `id` | `ID!` | Unique identifier (e.g., `hail-1`, `tornado-3`) |
| `type` | `String!` | Event type: `hail`, `tornado`, or `wind` |
| `geo` | `Geo!` | Geographic coordinates |
| `magnitude` | `Float!` | Event magnitude (interpretation depends on `unit`) |
| `unit` | `String!` | Magnitude unit: `in` (hail inches), `mph` (wind speed), `f_scale` (tornado) |
| `beginTime` | `DateTime!` | Event start time (RFC 3339) |
| `endTime` | `DateTime!` | Event end time (RFC 3339) |
| `source` | `String!` | Data source identifier |
| `location` | `Location!` | Location details |
| `comments` | `String!` | Free-text description of the event |
| `severity` | `String` | Severity level (nullable; e.g., `moderate`, `severe`) |
| `sourceOffice` | `String!` | NWS office code (e.g., `FWD`, `OAX`, `TSA`) |
| `timeBucket` | `DateTime!` | Hourly time bucket for aggregation |
| `processedAt` | `DateTime!` | When the record was processed |

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

### Aggregation Types

#### TypeGroup

| Field | Type | Description |
|-------|------|-------------|
| `type` | `String!` | Event type |
| `count` | `Int!` | Number of reports |
| `maxMagnitude` | `Float` | Highest magnitude in this group |
| `reports` | `[StormReport!]!` | Reports in this group |

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
| `reports` | `[StormReport!]!` | Reports in this county |

#### TimeGroup

| Field | Type | Description |
|-------|------|-------------|
| `bucket` | `DateTime!` | Hourly time bucket |
| `count` | `Int!` | Number of reports |
| `reports` | `[StormReport!]!` | Reports in this bucket |

## Filter Options

All filter fields except time bounds are optional. When multiple fields are provided, they are combined with AND logic.

### StormReportFilter

| Field | Type | Description |
|-------|------|-------------|
| `beginTimeAfter` | `DateTime!` | Events starting at or after this time (required) |
| `beginTimeBefore` | `DateTime!` | Events starting at or before this time (required) |
| `types` | `[String!]` | Match any of the listed event types |
| `severity` | `[String!]` | Match any of the listed severity levels |
| `minMagnitude` | `Float` | Minimum magnitude threshold |
| `states` | `[String!]` | Match any of the listed state codes |
| `counties` | `[String!]` | Match any of the listed county names |
| `nearLat` | `Float` | Center latitude for radius search |
| `nearLon` | `Float` | Center longitude for radius search |
| `radiusMiles` | `Float` | Search radius in miles (requires `nearLat` + `nearLon`) |
| `sortBy` | `SortField` | Sort field: `BEGIN_TIME`, `MAGNITUDE`, `STATE`, `TYPE` |
| `sortOrder` | `SortOrder` | Sort direction: `ASC`, `DESC` (default: `DESC`) |
| `limit` | `Int` | Maximum number of reports to return |
| `offset` | `Int` | Number of reports to skip (for pagination) |

### Geographic Radius Search

All three geo fields (`nearLat`, `nearLon`, `radiusMiles`) must be provided together. The query uses the Haversine formula for accurate great-circle distance calculations.

```graphql
query {
  stormReports(filter: {
    beginTimeAfter: "2024-04-26T00:00:00Z"
    beginTimeBefore: "2024-04-27T00:00:00Z"
    nearLat: 32.75
    nearLon: -97.15
    radiusMiles: 20.0
  }) {
    totalCount
    reports {
      id
      type
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
    beginTimeAfter: "2024-04-26T00:00:00Z"
    beginTimeBefore: "2024-04-27T00:00:00Z"
    types: ["hail"]
    sortBy: MAGNITUDE
    sortOrder: DESC
    limit: 10
    offset: 0
  }) {
    totalCount
    reports {
      id
      magnitude
      unit
      location { name county state }
    }
  }
}
```

### Combined Filters

```graphql
query {
  stormReports(filter: {
    beginTimeAfter: "2024-04-26T15:00:00Z"
    beginTimeBefore: "2024-04-26T20:00:00Z"
    types: ["hail"]
    states: ["TX"]
    severity: ["severe", "moderate"]
    minMagnitude: 1.0
  }) {
    totalCount
    reports {
      id
      magnitude
      unit
      location { name county }
      comments
    }
    byType { type count maxMagnitude }
  }
}
```
