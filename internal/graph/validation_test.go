package graph

import (
	"testing"
	"time"

	"github.com/couchcryptid/storm-data-api/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validFilter() *model.StormReportFilter {
	return &model.StormReportFilter{
		TimeRange: model.TimeRange{
			From: time.Date(2024, 4, 26, 0, 0, 0, 0, time.UTC),
			To:   time.Date(2024, 4, 27, 0, 0, 0, 0, time.UTC),
		},
	}
}

func TestValidateFilter_Valid(t *testing.T) {
	f := validFilter()
	require.NoError(t, ValidateFilter(f))

	// Limit should be defaulted
	require.NotNil(t, f.Limit)
	assert.Equal(t, MaxPageSize, *f.Limit)
}

func TestValidateFilter_TimeRangeInvalid(t *testing.T) {
	f := validFilter()
	f.TimeRange.From, f.TimeRange.To = f.TimeRange.To, f.TimeRange.From

	err := ValidateFilter(f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeRange.to must be after timeRange.from")
}

func TestValidateFilter_TimeRangeEqual(t *testing.T) {
	f := validFilter()
	f.TimeRange.To = f.TimeRange.From

	err := ValidateFilter(f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeRange.to must be after timeRange.from")
}

func TestValidateFilter_NearDefaultsRadius(t *testing.T) {
	f := validFilter()
	f.Near = &model.GeoRadiusFilter{Lat: 32.0, Lon: -97.0}

	require.NoError(t, ValidateFilter(f))
	require.NotNil(t, f.Near.RadiusMiles)
	assert.InDelta(t, DefaultRadiusMiles, *f.Near.RadiusMiles, 0.0001)
}

func TestValidateFilter_NearRadiusExceedsMax(t *testing.T) {
	f := validFilter()
	radius := 250.0
	f.Near = &model.GeoRadiusFilter{Lat: 32.0, Lon: -97.0, RadiusMiles: &radius}

	err := ValidateFilter(f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "near.radiusMiles exceeds maximum of 200")
}

func TestValidateFilter_NearRadiusAtMax(t *testing.T) {
	f := validFilter()
	radius := MaxRadiusMiles
	f.Near = &model.GeoRadiusFilter{Lat: 32.0, Lon: -97.0, RadiusMiles: &radius}

	require.NoError(t, ValidateFilter(f))
}

func TestValidateFilter_EventTypeFiltersTooMany(t *testing.T) {
	f := validFilter()
	f.EventTypeFilters = []*model.EventTypeFilter{
		{EventType: model.EventTypeHail},
		{EventType: model.EventTypeWind},
		{EventType: model.EventTypeTornado},
		{EventType: model.EventTypeHail}, // 4th — exceeds max
	}

	err := ValidateFilter(f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at most 3 eventTypeFilters")
}

func TestValidateFilter_EventTypeFiltersDuplicate(t *testing.T) {
	f := validFilter()
	f.EventTypeFilters = []*model.EventTypeFilter{
		{EventType: model.EventTypeHail},
		{EventType: model.EventTypeHail}, // duplicate
	}

	err := ValidateFilter(f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate eventType HAIL")
}

func TestValidateFilter_EventTypeFilterPerTypeRadiusCap(t *testing.T) {
	f := validFilter()
	bigRadius := 300.0
	f.EventTypeFilters = []*model.EventTypeFilter{
		{EventType: model.EventTypeHail, RadiusMiles: &bigRadius},
	}

	err := ValidateFilter(f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "eventTypeFilters[0]: radiusMiles exceeds maximum of 200")
}

func TestValidateFilter_EventTypeFiltersValid(t *testing.T) {
	f := validFilter()
	r1 := 50.0
	r2 := 100.0
	f.EventTypeFilters = []*model.EventTypeFilter{
		{EventType: model.EventTypeHail, RadiusMiles: &r1},
		{EventType: model.EventTypeTornado, RadiusMiles: &r2},
	}

	require.NoError(t, ValidateFilter(f))
}

func TestValidateFilter_LimitExceedsMax(t *testing.T) {
	f := validFilter()
	limit := 100
	f.Limit = &limit

	err := ValidateFilter(f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "limit exceeds maximum of 20")
}

func TestValidateFilter_LimitPreservedWhenValid(t *testing.T) {
	f := validFilter()
	limit := 10
	f.Limit = &limit

	require.NoError(t, ValidateFilter(f))
	assert.Equal(t, 10, *f.Limit)
}
