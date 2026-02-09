CREATE TABLE IF NOT EXISTS storm_reports (
    id              TEXT PRIMARY KEY,
    type            TEXT NOT NULL,
    geo_lat         DOUBLE PRECISION NOT NULL,
    geo_lon         DOUBLE PRECISION NOT NULL,
    magnitude       DOUBLE PRECISION NOT NULL,
    unit            TEXT NOT NULL,
    begin_time      TIMESTAMPTZ NOT NULL,
    end_time        TIMESTAMPTZ NOT NULL,
    source          TEXT NOT NULL,
    location_raw    TEXT NOT NULL,
    location_name   TEXT NOT NULL,
    location_distance   DOUBLE PRECISION,
    location_direction  TEXT,
    location_state  TEXT NOT NULL,
    location_county TEXT NOT NULL,
    comments        TEXT NOT NULL,
    severity        TEXT,
    source_office   TEXT NOT NULL,
    time_bucket     TIMESTAMPTZ NOT NULL,
    processed_at    TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Covers the most common query patterns
CREATE INDEX idx_begin_time ON storm_reports (begin_time);
CREATE INDEX idx_type ON storm_reports (type);
CREATE INDEX idx_state ON storm_reports (location_state);
CREATE INDEX idx_severity ON storm_reports (severity);

-- Composite for the typical "type + state + time" filter
CREATE INDEX idx_type_state_time ON storm_reports (type, location_state, begin_time);

-- Supports bounding-box pre-filter for radius queries
CREATE INDEX idx_geo ON storm_reports (geo_lat, geo_lon);
