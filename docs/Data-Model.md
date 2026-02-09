# Data Model

## Source of Truth

The Kafka message shape is defined by the mock data file:

```
data/mock/storm_reports_240526_transformed.json
```

All Go structs, the GraphQL schema, and the database schema are derived from this file.

## Message Shape

Each Kafka message on the `transformed-weather-data` topic is a single JSON object representing one storm report:

```json
{
  "id": "hail-1",
  "type": "hail",
  "geo": {
    "lat": 31.02,
    "lon": -98.44
  },
  "magnitude": 1.25,
  "unit": "in",
  "begin_time": "2024-04-26T15:10:00Z",
  "end_time": "2024-04-26T15:10:00Z",
  "source": "mock",
  "location": {
    "raw": "8 ESE Chappel",
    "name": "Chappel",
    "distance": 8,
    "direction": "ESE",
    "state": "TX",
    "county": "San Saba"
  },
  "comments": "1.25 inch hail reported at Colorado Bend State Park. (SJT)",
  "severity": "moderate",
  "source_office": "SJT",
  "time_bucket": "2024-04-26T15:00:00Z",
  "processed_at": "2024-04-26T22:00:00Z"
}
```

## Event Types

| Type | `magnitude` meaning | `unit` |
|------|---------------------|--------|
| `hail` | Hail stone diameter | `in` (inches) |
| `tornado` | F/EF scale rating | `f_scale` |
| `wind` | Wind speed | `mph` |

A magnitude of `0` means the value was not measured or not applicable.

## Optional Fields

| Field | When absent |
|-------|-------------|
| `severity` | Not assigned to all report types (absent on most tornado and some wind reports) |
| `location.distance` | Report is at the named location itself (no offset) |
| `location.direction` | Report is at the named location itself (no offset) |

## Database Column Mapping

Nested JSON fields are flattened in PostgreSQL:

| JSON Path | Database Column |
|-----------|----------------|
| `id` | `id` |
| `type` | `type` |
| `geo.lat` | `geo_lat` |
| `geo.lon` | `geo_lon` |
| `location.raw` | `location_raw` |
| `location.name` | `location_name` |
| `location.distance` | `location_distance` |
| `location.direction` | `location_direction` |
| `location.state` | `location_state` |
| `location.county` | `location_county` |
| _(all other fields)_ | _(same name, snake_case)_ |

## Mock Data Summary

The mock file contains 30 storm reports from April 26, 2024:

- **10 hail** reports across TX, IA, NE, CO
- **10 tornado** reports across OK, NE, TX
- **10 wind** reports across OK, NV, NE, TX
