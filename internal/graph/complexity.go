package graph

import "github.com/couchcryptid/storm-data-graphql-api/internal/model"

// NewComplexityRoot returns complexity estimators for expensive fields.
// List fields use a multiplier so that requesting large result sets
// costs proportionally more against the overall complexity budget.
func NewComplexityRoot() ComplexityRoot {
	return ComplexityRoot{
		Query: struct {
			StormReports func(childComplexity int, filter model.StormReportFilter) int
		}{
			StormReports: func(childComplexity int, filter model.StormReportFilter) int {
				return 1 + childComplexity
			},
		},

		StormReportResult: struct {
			ByHour         func(childComplexity int) int
			ByState        func(childComplexity int) int
			ByType         func(childComplexity int) int
			DataLagMinutes func(childComplexity int) int
			LastUpdated    func(childComplexity int) int
			Reports        func(childComplexity int) int
			TotalCount     func(childComplexity int) int
		}{
			// reports list: assume up to 50 items returned
			Reports: func(childComplexity int) int {
				return 50 * childComplexity
			},
			ByType: func(childComplexity int) int {
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
