package graph

import (
	"testing"

	"github.com/couchcryptid/storm-data-graphql-api/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestNewComplexityRoot_QueryStormReports(t *testing.T) {
	c := NewComplexityRoot()
	// 1 + child
	assert.Equal(t, 101, c.Query.StormReports(100, model.StormReportFilter{}))
	assert.Equal(t, 1, c.Query.StormReports(0, model.StormReportFilter{}))
}

func TestNewComplexityRoot_ReportsMultiplier(t *testing.T) {
	c := NewComplexityRoot()
	// MaxPageSize × child
	assert.Equal(t, MaxPageSize*16, c.StormReportsResult.Reports(16))
	assert.Equal(t, 0, c.StormReportsResult.Reports(0))
}

func TestNewComplexityRoot_AggregationMultipliers(t *testing.T) {
	c := NewComplexityRoot()
	// Each aggregation list: 10 × child
	assert.Equal(t, 30, c.StormAggregations.ByEventType(3))
	assert.Equal(t, 30, c.StormAggregations.ByState(3))
	assert.Equal(t, 30, c.StormAggregations.ByHour(3))
}

func TestNewComplexityRoot_StateGroupCounties(t *testing.T) {
	c := NewComplexityRoot()
	// 5 × child
	assert.Equal(t, 10, c.StateGroup.Counties(2))
	assert.Equal(t, 0, c.StateGroup.Counties(0))
}

func TestNewComplexityRoot_NilForUnsetFields(t *testing.T) {
	c := NewComplexityRoot()
	// Fields without custom multipliers should be nil (gqlgen uses default of 1)
	assert.Nil(t, c.StormReportsResult.TotalCount)
	assert.Nil(t, c.StormReportsResult.HasMore)
	assert.Nil(t, c.StormReportsResult.Meta)
	assert.Nil(t, c.StormReportsResult.Aggregations)
	assert.Nil(t, c.StormAggregations.TotalCount)
	assert.Nil(t, c.StateGroup.Count)
	assert.Nil(t, c.StateGroup.State)
}

func TestNewComplexityRoot_WorstCase(t *testing.T) {
	// Verify the budget calculation:
	//   stormReports = 1 + child
	//   reports = MaxPageSize(20) × reportChild
	//   reportChild = id(1) + eventType(1) + geo(2) + measurement(3) + beginTime(1) +
	//     endTime(1) + source(1) + sourceOffice(1) + location(6) + comments(1) +
	//     timeBucket(1) + processedAt(1) + formattedAddress(1) + placeName(1) +
	//     geoConfidence(1) + geoSource(1) = 24
	//   reports = 20 × 24 = 480
	//   byEventType = 10 × (eventType(1) + count(1) + maxMeasurement(3)) = 50
	//   byState = 10 × (state(1) + count(1) + counties(5×2)) = 120
	//   byHour = 10 × (bucket(1) + count(1)) = 20
	//   aggregations = totalCount(1) + byEventType(50) + byState(120) + byHour(20) = 191
	//   meta = lastUpdated(1) + dataLagMinutes(1) = 2
	//   total = 1 + totalCount(1) + hasMore(1) + reports(480) + aggregations(1+191) + meta(1+2) = 678
	// Note: This exceeds 600, so a client requesting ALL fields at max depth would be
	// rejected. This is by design — typical queries request a subset.

	c := NewComplexityRoot()

	reportLeafFields := 24
	reports := c.StormReportsResult.Reports(reportLeafFields) // 20 × 24 = 480
	assert.Equal(t, 480, reports)

	byEventType := c.StormAggregations.ByEventType(5) // 10 × 5 = 50
	assert.Equal(t, 50, byEventType)

	counties := c.StateGroup.Counties(2)                 // 5 × 2 = 10
	byState := c.StormAggregations.ByState(2 + counties) // 10 × 12 = 120
	assert.Equal(t, 120, byState)

	byHour := c.StormAggregations.ByHour(2) // 10 × 2 = 20
	assert.Equal(t, 20, byHour)

	// A realistic query selects reports + one aggregation type + meta
	// reports(20×24=480) + byEventType(50) + meta(2) + topLevelScalars(3) + 1 = 536
	realisticChild := 3 + reports + 1 + byEventType + 1 + 2
	total := c.Query.StormReports(realisticChild, model.StormReportFilter{})
	assert.Equal(t, 538, total)
	assert.LessOrEqual(t, total, 600, "realistic worst-case should fit within 600 budget")
}
