package model

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// ─── Core types ─────────────────────────────────────────────

// StormReport represents a single severe weather event consumed from Kafka and
// persisted in the database.
//
// Nesting strategy (mirrors the ETL domain model):
//   - Geo, Location, and Measurement are nested structs matching the Kafka wire
//     format. This allows json.Unmarshal to deserialize enriched events
//     automatically, and gqlgen auto-resolves the corresponding GraphQL types
//     without field resolvers. The store layer flattens these to prefixed DB
//     columns (geo_lat, measurement_magnitude, etc.) for relational storage
//     and indexing.
//   - EventType maps to the "event_type" column in the database and the
//     "eventType" field in the GraphQL schema.
type StormReport struct {
	ID           string      `json:"id"`
	EventType    string      `json:"event_type"`
	Geo          Geo         `json:"geo"`
	Measurement  Measurement `json:"measurement"`
	EventTime    time.Time   `json:"event_time"`
	Location     Location    `json:"location"`
	Comments     string      `json:"comments"`
	SourceOffice string      `json:"source_office"`
	TimeBucket   time.Time   `json:"time_bucket"`
	ProcessedAt  time.Time   `json:"processed_at"`
}

// Geo holds latitude and longitude coordinates. Nested as a struct because
// lat/lon are always used together and map directly to the GraphQL Geo type.
// Flattened to geo_lat/geo_lon columns in the database for spatial indexing.
type Geo struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// Location describes where a storm event occurred. Nested as a struct because
// these fields are tightly coupled: the ETL parses raw NWS format into components,
// and they travel together through the pipeline. Flattened to location_* columns
// in the database.
type Location struct {
	Raw       string   `json:"raw"`
	Name      string   `json:"name"`
	Distance  *float64 `json:"distance,omitempty"`
	Direction *string  `json:"direction,omitempty"`
	State     string   `json:"state"`
	County    string   `json:"county"`
}

// Measurement groups magnitude, unit, and severity for a storm report.
// Nested on StormReport to match the Kafka wire format; gqlgen auto-resolves
// the GraphQL Measurement type. Flattened to measurement_* DB columns.
type Measurement struct {
	Magnitude float64 `json:"magnitude"`
	Unit      string  `json:"unit"`
	Severity  *string `json:"severity,omitempty"`
}

// ─── Enums ──────────────────────────────────────────────────

// EventType enumerates the types of severe weather events.
type EventType string

// EventType enum values.
const (
	EventTypeHail    EventType = "HAIL"
	EventTypeWind    EventType = "WIND"
	EventTypeTornado EventType = "TORNADO"
)

// IsValid returns true if the event type is a known value.
func (e EventType) IsValid() bool {
	switch e {
	case EventTypeHail, EventTypeWind, EventTypeTornado:
		return true
	}
	return false
}

func (e EventType) String() string { return string(e) }

// DBValue returns the lowercase DB representation of the event type.
func (e EventType) DBValue() string { return strings.ToLower(string(e)) }

// UnmarshalGQL implements the graphql.Unmarshaler interface.
func (e *EventType) UnmarshalGQL(v any) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("EventType must be a string")
	}
	*e = EventType(str)
	if !e.IsValid() {
		return fmt.Errorf("invalid EventType %q", str)
	}
	return nil
}

// MarshalGQL implements the graphql.Marshaler interface.
func (e EventType) MarshalGQL(w io.Writer) {
	_, _ = fmt.Fprintf(w, "%q", string(e))
}

// Severity enumerates the severity levels of storm events.
type Severity string

// Severity enum values.
const (
	SeverityMinor    Severity = "MINOR"
	SeverityModerate Severity = "MODERATE"
	SeveritySevere   Severity = "SEVERE"
	SeverityExtreme  Severity = "EXTREME"
)

// IsValid returns true if the severity is a known value.
func (e Severity) IsValid() bool {
	switch e {
	case SeverityMinor, SeverityModerate, SeveritySevere, SeverityExtreme:
		return true
	}
	return false
}

func (e Severity) String() string { return string(e) }

// DBValue returns the lowercase DB representation of the severity.
func (e Severity) DBValue() string { return strings.ToLower(string(e)) }

// UnmarshalGQL implements the graphql.Unmarshaler interface.
func (e *Severity) UnmarshalGQL(v any) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("Severity must be a string")
	}
	*e = Severity(str)
	if !e.IsValid() {
		return fmt.Errorf("invalid Severity %q", str)
	}
	return nil
}

// MarshalGQL implements the graphql.Marshaler interface.
func (e Severity) MarshalGQL(w io.Writer) {
	_, _ = fmt.Fprintf(w, "%q", string(e))
}

// SortField enumerates the columns available for sorting storm reports.
type SortField string

// SortField enum values.
const (
	SortFieldEventTime     SortField = "EVENT_TIME"
	SortFieldMagnitude     SortField = "MAGNITUDE"
	SortFieldLocationState SortField = "LOCATION_STATE"
	SortFieldEventType     SortField = "EVENT_TYPE"
)

// IsValid returns true if the sort field is a known value.
func (e SortField) IsValid() bool {
	switch e {
	case SortFieldEventTime, SortFieldMagnitude, SortFieldLocationState, SortFieldEventType:
		return true
	}
	return false
}

func (e SortField) String() string { return string(e) }

// SortOrder specifies ascending or descending sort direction.
type SortOrder string

// SortOrder enum values.
const (
	SortOrderAsc  SortOrder = "ASC"
	SortOrderDesc SortOrder = "DESC"
)

// IsValid returns true if the sort order is a known value.
func (e SortOrder) IsValid() bool {
	switch e {
	case SortOrderAsc, SortOrderDesc:
		return true
	}
	return false
}

func (e SortOrder) String() string { return string(e) }

// ─── Filter inputs ──────────────────────────────────────────

// TimeRange specifies a time window for filtering.
type TimeRange struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// GeoRadiusFilter specifies a geographic radius filter.
type GeoRadiusFilter struct {
	Lat         float64  `json:"lat"`
	Lon         float64  `json:"lon"`
	RadiusMiles *float64 `json:"radiusMiles,omitempty"`
}

// EventTypeFilter allows per-type overrides for severity, magnitude, and radius.
type EventTypeFilter struct {
	EventType    EventType  `json:"eventType"`
	Severity     []Severity `json:"severity,omitempty"`
	MinMagnitude *float64   `json:"minMagnitude,omitempty"`
	RadiusMiles  *float64   `json:"radiusMiles,omitempty"`
}

// StormReportFilter specifies time range, event, location, sorting, and pagination criteria.
type StormReportFilter struct {
	TimeRange TimeRange        `json:"timeRange"`
	Near      *GeoRadiusFilter `json:"near,omitempty"`
	States    []string         `json:"states,omitempty"`
	Counties  []string         `json:"counties,omitempty"`

	// Global defaults — apply to any type not overridden.
	EventTypes   []EventType `json:"eventTypes,omitempty"`
	Severity     []Severity  `json:"severity,omitempty"`
	MinMagnitude *float64    `json:"minMagnitude,omitempty"`

	// Per-type overrides (max 3).
	EventTypeFilters []*EventTypeFilter `json:"eventTypeFilters,omitempty"`

	// Sorting & pagination.
	SortBy    *SortField `json:"sortBy,omitempty"`
	SortOrder *SortOrder `json:"sortOrder,omitempty"`
	Limit     *int       `json:"limit,omitempty"`
	Offset    *int       `json:"offset,omitempty"`
}

// ─── Result envelope ────────────────────────────────────────

// StormReportsResult is the top-level GraphQL response.
type StormReportsResult struct {
	TotalCount   int                `json:"totalCount"`
	HasMore      bool               `json:"hasMore"`
	Reports      []*StormReport     `json:"reports"`
	Aggregations *StormAggregations `json:"aggregations"`
	Meta         *QueryMeta         `json:"meta"`
}

// StormAggregations groups aggregation results by event type, state, and hour.
type StormAggregations struct {
	TotalCount  int               `json:"totalCount"`
	ByEventType []*EventTypeGroup `json:"byEventType"`
	ByState     []*StateGroup     `json:"byState"`
	ByHour      []*TimeGroup      `json:"byHour"`
}

// QueryMeta provides metadata about the query result.
type QueryMeta struct {
	LastUpdated    *time.Time `json:"lastUpdated,omitempty"`
	DataLagMinutes *int       `json:"dataLagMinutes,omitempty"`
}

// ─── Aggregation types ──────────────────────────────────────

// EventTypeGroup aggregates storm reports by event type.
type EventTypeGroup struct {
	EventType      string       `json:"eventType"`
	Count          int          `json:"count"`
	MaxMeasurement *Measurement `json:"maxMeasurement,omitempty"`
}

// StateGroup aggregates storm reports by state, with county breakdowns.
type StateGroup struct {
	State    string         `json:"state"`
	Count    int            `json:"count"`
	Counties []*CountyGroup `json:"counties"`
}

// CountyGroup aggregates storm reports by county within a state.
type CountyGroup struct {
	County string `json:"county"`
	Count  int    `json:"count"`
}

// TimeGroup aggregates storm reports by hourly time bucket.
type TimeGroup struct {
	Bucket time.Time `json:"bucket"`
	Count  int       `json:"count"`
}
