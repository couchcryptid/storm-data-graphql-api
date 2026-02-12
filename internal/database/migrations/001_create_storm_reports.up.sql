CREATE TABLE IF NOT EXISTS storm_reports (
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

-- Covers the most common query patterns
CREATE INDEX idx_event_time ON storm_reports (event_time);
CREATE INDEX idx_event_type ON storm_reports (event_type);
CREATE INDEX idx_state ON storm_reports (location_state);
CREATE INDEX idx_severity ON storm_reports (measurement_severity);

-- Composite for the typical "event_type + state + time" filter
CREATE INDEX idx_event_type_state_time ON storm_reports (event_type, location_state, event_time);

-- Supports bounding-box pre-filter for radius queries
CREATE INDEX idx_geo ON storm_reports (geo_lat, geo_lon);
