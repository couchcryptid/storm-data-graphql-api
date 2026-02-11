# Data Model

## Source of Truth

The authoritative data model is defined by:

1. **GraphQL schema**: `internal/graph/schema.graphqls` — defines the API types (`StormReport`, `Measurement`, `Geocoding`, etc.)
2. **Database migration**: `internal/database/migrations/001_create_storm_reports.up.sql` — defines the PostgreSQL schema
3. **System Data Model**: The [storm-data-system Data Model](https://github.com/couchcryptid/storm-data-system/wiki/Data-Model) wiki page documents the full pipeline data model

The mock data file (`data/mock/storm_reports_240426_transformed.json`) provides test fixtures in this shape.

## Message Shape

Each Kafka message on the `transformed-weather-data` topic is a single JSON object representing one storm report:

```json
{
  "id": "a3f8b2c1e7d9...",
  "type": "hail",
  "geo": {
    "lat": 31.02,
    "lon": -98.44
  },
  "measurement": {
    "magnitude": 1.25,
    "unit": "in",
    "severity": "moderate"
  },
  "begin_time": "2026-01-01T15:10:00Z",
  "end_time": "2026-01-01T15:10:00Z",
  "source": "spc",
  "location": {
    "raw": "8 ESE Chappel",
    "name": "Chappel",
    "distance": 8,
    "direction": "ESE",
    "state": "TX",
    "county": "San Saba"
  },
  "comments": "1.25 inch hail reported at Colorado Bend State Park. (SJT)",
  "source_office": "SJT",
  "time_bucket": "2026-01-01T15:00:00Z",
  "processed_at": "2026-01-01T22:00:00Z",
  "geocoding": {
    "formatted_address": "",
    "place_name": "",
    "confidence": 0,
    "source": ""
  }
}
```

## Event Types

| Type | `measurement.magnitude` meaning | `measurement.unit` |
|------|---------------------|--------|
| `hail` | Hail stone diameter | `in` (inches) |
| `tornado` | F/EF scale rating | `f_scale` |
| `wind` | Wind speed | `mph` |

A magnitude of `0` means the value was not measured or not applicable.

## Optional Fields

| Field | When absent |
|-------|-------------|
| `measurement.severity` | Magnitude is 0 or unmeasured |
| `location.distance` | Report is at the named location itself (no offset) |
| `location.direction` | Report is at the named location itself (no offset) |
| `geocoding` | Geocoding disabled or all fields are zero-valued |

## Database Column Mapping

Nested JSON fields are flattened in PostgreSQL:

| JSON Path | Database Column |
|-----------|----------------|
| `id` | `id` |
| `type` | `type` |
| `geo.lat` | `geo_lat` |
| `geo.lon` | `geo_lon` |
| `measurement.magnitude` | `measurement_magnitude` |
| `measurement.unit` | `measurement_unit` |
| `measurement.severity` | `measurement_severity` |
| `location.raw` | `location_raw` |
| `location.name` | `location_name` |
| `location.distance` | `location_distance` |
| `location.direction` | `location_direction` |
| `location.state` | `location_state` |
| `location.county` | `location_county` |
| `geocoding.formatted_address` | `geocoding_formatted_address` |
| `geocoding.place_name` | `geocoding_place_name` |
| `geocoding.confidence` | `geocoding_confidence` |
| `geocoding.source` | `geocoding_source` |
| All other fields | Same name (snake_case) |

## Mock Data Summary

The mock file contains 30 storm reports from April 26, 2024:

- **10 hail** reports across TX, IA, NE, CO
- **10 tornado** reports across OK, NE, TX
- **10 wind** reports across OK, NV, NE, TX
