package graph

import (
	"testing"

	"github.com/couchcryptid/storm-data-api/internal/model"
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
	// Verify the budget calculation. gqlgen computes each field as 1 + childComplexity
	// (custom multipliers replace the default). Object fields add +1 for themselves.
	//
	//   reportChild = id(1) + eventType(1) + geo(1+2=3) + measurement(1+3=4) +
	//     eventTime(1) + sourceOffice(1) + location(1+6=7) + comments(1) +
	//     timeBucket(1) + processedAt(1) = 21
	//   reports = MaxPageSize(20) × 21 = 420
	//   byEventType = 10 × (eventType(1) + count(1) + maxMeasurement(1+3=4)) = 60
	//   byState = 10 × (state(1) + count(1) + counties(5×2=10)) = 120
	//   byHour = 10 × (bucket(1) + count(1)) = 20
	//   aggregations = 1 + totalCount(1) + byEventType(60) + byState(120) + byHour(20) = 202
	//   meta = 1 + lastUpdated(1) + dataLagMinutes(1) = 3
	//   total = 1 + totalCount(1) + hasMore(1) + reports(420) + aggregations(202) + meta(3) = 628
	// Note: This exceeds 600, so a client requesting ALL fields at max depth would be
	// rejected. This is by design — typical queries request a subset.

	c := NewComplexityRoot()

	reportChildComplexity := 21
	reports := c.StormReportsResult.Reports(reportChildComplexity) // 20 × 21 = 420
	assert.Equal(t, 420, reports)

	byEventType := c.StormAggregations.ByEventType(6) // 10 × 6 = 60
	assert.Equal(t, 60, byEventType)

	counties := c.StateGroup.Counties(2)                 // 5 × 2 = 10
	byState := c.StormAggregations.ByState(2 + counties) // 10 × 12 = 120
	assert.Equal(t, 120, byState)

	byHour := c.StormAggregations.ByHour(2) // 10 × 2 = 20
	assert.Equal(t, 20, byHour)

	// A realistic worst-case: reports (all fields) + one aggregation type + meta
	//   totalCount(1) + hasMore(1) + reports(420) + aggregations(1+1+60) + meta(1+2) = 487
	realisticChild := 2 + reports + (1 + 1 + byEventType) + (1 + 2)
	total := c.Query.StormReports(realisticChild, model.StormReportFilter{})
	assert.Equal(t, 488, total)
	assert.LessOrEqual(t, total, 600, "realistic worst-case should fit within 600 budget")
}
