package store

import (
	"testing"
	"time"

	"github.com/couchcryptid/storm-data-api/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestBuildWhereClause_TimeOnly(t *testing.T) {
	filter := &model.StormReportFilter{
		TimeRange: model.TimeRange{
			From: time.Date(2024, 4, 26, 0, 0, 0, 0, time.UTC),
			To:   time.Date(2024, 4, 27, 0, 0, 0, 0, time.UTC),
		},
	}

	where, args, nextIdx := buildWhereClause(filter)

	assert.Len(t, where, 2)
	assert.Contains(t, where[0], "$1")
	assert.Contains(t, where[1], "$2")
	assert.Len(t, args, 2)
	assert.Equal(t, 3, nextIdx)
}

func TestBuildWhereClause_WithEventTypes(t *testing.T) {
	filter := &model.StormReportFilter{
		TimeRange: model.TimeRange{
			From: time.Date(2024, 4, 26, 0, 0, 0, 0, time.UTC),
			To:   time.Date(2024, 4, 27, 0, 0, 0, 0, time.UTC),
		},
		EventTypes: []model.EventType{model.EventTypeHail, model.EventTypeTornado},
	}

	where, args, nextIdx := buildWhereClause(filter)

	assert.Len(t, where, 3)
	assert.Contains(t, where[2], "event_type = ANY($3)")
	assert.Len(t, args, 3)
	// Verify enum→DB conversion
	assert.Equal(t, []string{"hail", "tornado"}, args[2])
	assert.Equal(t, 4, nextIdx)
}

func TestBuildWhereClause_AllSimpleFilters(t *testing.T) {
	mag := 1.5
	filter := &model.StormReportFilter{
		TimeRange: model.TimeRange{
			From: time.Date(2024, 4, 26, 0, 0, 0, 0, time.UTC),
			To:   time.Date(2024, 4, 27, 0, 0, 0, 0, time.UTC),
		},
		EventTypes:   []model.EventType{model.EventTypeHail},
		Severity:     []model.Severity{model.SeveritySevere},
		States:       []string{"TX", "OK"},
		Counties:     []string{"Dallas"},
		MinMagnitude: &mag,
	}

	where, args, nextIdx := buildWhereClause(filter)

	// 2 time + states + counties + eventTypes + severity + minMagnitude = 7
	assert.Len(t, where, 7)
	assert.Len(t, args, 7)
	assert.Equal(t, 8, nextIdx)
}

func TestBuildWhereClause_NearRadiusFilter(t *testing.T) {
	radius := 50.0
	filter := &model.StormReportFilter{
		TimeRange: model.TimeRange{
			From: time.Date(2024, 4, 26, 0, 0, 0, 0, time.UTC),
			To:   time.Date(2024, 4, 27, 0, 0, 0, 0, time.UTC),
		},
		Near: &model.GeoRadiusFilter{
			Lat:         32.7767,
			Lon:         -96.7970,
			RadiusMiles: &radius,
		},
	}

	where, args, nextIdx := buildWhereClause(filter)

	// 2 time + bounding box (1 clause, 4 params) + haversine (1 clause, 4 params) = 4 clauses
	assert.Len(t, where, 4)
	// 2 time args + 4 bbox args + 4 haversine args = 10
	assert.Len(t, args, 10)
	assert.Equal(t, 11, nextIdx)
}

func TestBuildWhereClause_EventTypeFilters(t *testing.T) {
	hailRadius := 20.0
	tornadoRadius := 50.0
	minMag := 1.0
	filter := &model.StormReportFilter{
		TimeRange: model.TimeRange{
			From: time.Date(2024, 4, 26, 0, 0, 0, 0, time.UTC),
			To:   time.Date(2024, 4, 27, 0, 0, 0, 0, time.UTC),
		},
		Near: &model.GeoRadiusFilter{
			Lat: 35.0,
			Lon: -97.0,
		},
		EventTypeFilters: []*model.EventTypeFilter{
			{EventType: model.EventTypeHail, MinMagnitude: &minMag, RadiusMiles: &hailRadius},
			{EventType: model.EventTypeTornado, RadiusMiles: &tornadoRadius},
		},
	}

	where, args, nextIdx := buildWhereClause(filter)

	// 2 time + bounding box (max radius 50) + OR clause = 4
	assert.Len(t, where, 4)

	// The OR clause should contain both types
	orClause := where[3]
	assert.Contains(t, orClause, "event_type =")
	assert.Contains(t, orClause, "OR")

	// Verify args count:
	// 2 time + 4 bbox + (hail: type + minMag + 4 haversine) + (tornado: type + 4 haversine) = 2+4+6+5 = 17
	assert.Len(t, args, 17)
	assert.Equal(t, 18, nextIdx)
}

func TestBuildWhereClause_EventTypeFiltersWithGlobalDefaults(t *testing.T) {
	hailRadius := 30.0
	globalMag := 0.5
	filter := &model.StormReportFilter{
		TimeRange: model.TimeRange{
			From: time.Date(2024, 4, 26, 0, 0, 0, 0, time.UTC),
			To:   time.Date(2024, 4, 27, 0, 0, 0, 0, time.UTC),
		},
		Near: &model.GeoRadiusFilter{
			Lat: 35.0,
			Lon: -97.0,
		},
		MinMagnitude: &globalMag,
		EventTypes:   []model.EventType{model.EventTypeHail, model.EventTypeWind},
		EventTypeFilters: []*model.EventTypeFilter{
			{EventType: model.EventTypeHail, RadiusMiles: &hailRadius},
		},
	}

	where, _, _ := buildWhereClause(filter)

	// The OR clause should include both hail (override) and wind (global defaults)
	orClause := where[len(where)-1]
	assert.Contains(t, orClause, "OR")
}

func TestSortColumn(t *testing.T) {
	tests := []struct {
		input model.SortField
		want  string
	}{
		{model.SortFieldEventTime, "event_time"},
		{model.SortFieldMagnitude, "measurement_magnitude"},
		{model.SortFieldLocationState, "location_state"},
		{model.SortFieldEventType, "event_type"},
		{model.SortField("UNKNOWN"), "event_time"},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			assert.Equal(t, tt.want, sortColumn(tt.input))
		})
	}
}

func TestUnitForEventType(t *testing.T) {
	assert.Equal(t, "in", unitForEventType("hail"))
	assert.Equal(t, "mph", unitForEventType("wind"))
	assert.Equal(t, "f_scale", unitForEventType("tornado"))
	assert.Empty(t, unitForEventType("unknown"))
}

func TestEventTypeDBValues(t *testing.T) {
	vals := eventTypeDBValues([]model.EventType{model.EventTypeHail, model.EventTypeWind, model.EventTypeTornado})
	assert.Equal(t, []string{"hail", "wind", "tornado"}, vals)
}

func TestSeverityDBValues(t *testing.T) {
	vals := severityDBValues([]model.Severity{model.SeverityMinor, model.SeverityExtreme})
	assert.Equal(t, []string{"minor", "extreme"}, vals)
}
