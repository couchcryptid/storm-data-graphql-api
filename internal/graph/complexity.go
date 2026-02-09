package graph

import "github.com/couchcryptid/storm-data-graphql-api/internal/model"

// NewComplexityRoot returns complexity estimators for expensive fields.
// Budget: 600 (worst-case ~537 per recommendations doc).
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
