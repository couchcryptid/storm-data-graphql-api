package graph

import "github.com/couchcryptid/storm-data-api/internal/model"

// NewComplexityRoot returns complexity estimators for expensive fields.
// gqlgen computes total query complexity bottom-up and rejects queries exceeding
// the budget (600). Multipliers estimate the maximum number of child items each
// field can return:
//   - Reports: up to MaxPageSize (20) items per query
//   - ByEventType/ByState/ByHour: up to 10 groups each
//   - Counties: up to 5 per state
//
// Cost examples (budget = 600):
//
//	Dashboard query (reports + partial aggregations):  ~458  ✓
//	Reports (all fields) + one aggregation + meta:     ~488  ✓
//	All fields on all types (intentionally rejected):  ~628  ✗
//
// See TestNewComplexityRoot_WorstCase for the exact field-by-field calculation.
func NewComplexityRoot() ComplexityRoot {
	return ComplexityRoot{
		Query: struct {
			StormReports func(childComplexity int, filter model.StormReportFilter) int
		}{
			StormReports: func(childComplexity int, _ model.StormReportFilter) int {
				return 1 + childComplexity
			},
		},

		StormReportsResult: struct {
			Aggregations func(childComplexity int) int
			HasMore      func(childComplexity int) int
			Meta         func(childComplexity int) int
			Reports      func(childComplexity int) int
			TotalCount   func(childComplexity int) int
		}{
			Reports: func(childComplexity int) int {
				return MaxPageSize * childComplexity
			},
		},

		StormAggregations: struct {
			ByEventType func(childComplexity int) int
			ByHour      func(childComplexity int) int
			ByState     func(childComplexity int) int
			TotalCount  func(childComplexity int) int
		}{
			ByEventType: func(childComplexity int) int {
				return 10 * childComplexity
			},
			ByState: func(childComplexity int) int {
				return 10 * childComplexity
			},
			ByHour: func(childComplexity int) int {
				return 10 * childComplexity
			},
		},

		StateGroup: struct {
			Count    func(childComplexity int) int
			Counties func(childComplexity int) int
			State    func(childComplexity int) int
		}{
			Counties: func(childComplexity int) int {
				return 5 * childComplexity
			},
		},
	}
}
