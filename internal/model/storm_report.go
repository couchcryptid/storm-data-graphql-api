package model

import "time"

// ─── Core types ─────────────────────────────────────────────

// StormReport represents a single severe weather event persisted in the database.
type StormReport struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	Geo          Geo       `json:"geo"`
	Magnitude    float64   `json:"magnitude"`
	Unit         string    `json:"unit"`
	BeginTime    time.Time `json:"begin_time"`
	EndTime      time.Time `json:"end_time"`
	Source       string    `json:"source"`
	Location     Location  `json:"location"`
	Comments     string    `json:"comments"`
	Severity     *string   `json:"severity,omitempty"`
	SourceOffice string    `json:"source_office"`
	TimeBucket   time.Time `json:"time_bucket"`
	ProcessedAt  time.Time `json:"processed_at"`
}

// Geo holds latitude and longitude coordinates.
type Geo struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// Location describes where a storm event occurred.
type Location struct {
	Raw       string   `json:"raw"`
	Name      string   `json:"name"`
	Distance  *float64 `json:"distance,omitempty"`
	Direction *string  `json:"direction,omitempty"`
	State     string   `json:"state"`
	County    string   `json:"county"`
}

// ─── Enums ──────────────────────────────────────────────────

// SortField enumerates the columns available for sorting storm reports.
type SortField string

// Allowed SortField values.
const (
	SortFieldBeginTime SortField = "BEGIN_TIME"
	SortFieldMagnitude SortField = "MAGNITUDE"
	SortFieldState     SortField = "STATE"
	SortFieldType      SortField = "TYPE"
)

// IsValid returns true if the SortField is one of the known enum values.
func (e SortField) IsValid() bool {
	switch e {
	case SortFieldBeginTime, SortFieldMagnitude, SortFieldState, SortFieldType:
		return true
	}
	return false
}

func (e SortField) String() string { return string(e) }

// SortOrder specifies ascending or descending sort direction.
type SortOrder string

// Allowed SortOrder values.
const (
	SortOrderAsc  SortOrder = "ASC"
	SortOrderDesc SortOrder = "DESC"
)

// IsValid returns true if the SortOrder is one of the known enum values.
func (e SortOrder) IsValid() bool {
	switch e {
	case SortOrderAsc, SortOrderDesc:
		return true
	}
	return false
}

func (e SortOrder) String() string { return string(e) }

// ─── Filter ─────────────────────────────────────────────────

// StormReportFilter specifies time range, event, location, sorting, and pagination criteria.
type StormReportFilter struct {
	// Time (required)
	BeginTimeAfter  time.Time `json:"beginTimeAfter"`
	BeginTimeBefore time.Time `json:"beginTimeBefore"`

	// Event
	Types        []string `json:"types,omitempty"`
	Severity     []string `json:"severity,omitempty"`
	MinMagnitude *float64 `json:"minMagnitude,omitempty"`

	// Location - administrative
	States   []string `json:"states,omitempty"`
	Counties []string `json:"counties,omitempty"`

	// Location - radius
	NearLat     *float64 `json:"nearLat,omitempty"`
	NearLon     *float64 `json:"nearLon,omitempty"`
	RadiusMiles *float64 `json:"radiusMiles,omitempty"`

	// Sorting & pagination
	SortBy    *SortField `json:"sortBy,omitempty"`
	SortOrder *SortOrder `json:"sortOrder,omitempty"`
	Limit     *int       `json:"limit,omitempty"`
	Offset    *int       `json:"offset,omitempty"`
}

// ─── Result envelope ────────────────────────────────────────

// StormReportResult is the top-level GraphQL response containing reports and aggregations.
type StormReportResult struct {
	Reports        []*StormReport `json:"reports"`
	TotalCount     int            `json:"totalCount"`
	ByType         []*TypeGroup   `json:"byType"`
	ByState        []*StateGroup  `json:"byState"`
	ByHour         []*TimeGroup   `json:"byHour"`
	LastUpdated    *time.Time     `json:"lastUpdated,omitempty"`
	DataLagMinutes *int           `json:"dataLagMinutes,omitempty"`
}

// ─── Aggregation types ──────────────────────────────────────

// TypeGroup aggregates storm reports by event type.
type TypeGroup struct {
	Type         string   `json:"type"`
	Count        int      `json:"count"`
	MaxMagnitude *float64 `json:"maxMagnitude,omitempty"`
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
