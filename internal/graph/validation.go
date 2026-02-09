package graph

import (
	"fmt"

	"github.com/couchcryptid/storm-data-graphql-api/internal/model"
)

// Query protection limits.
const (
	MaxEventTypeFilters = 3
	MaxPageSize         = 20
	MaxRadiusMiles      = 200.0
	DefaultRadiusMiles  = 20.0
)

// ValidateFilter validates a single filter, enforcing limits and applying defaults.
func ValidateFilter(filter *model.StormReportFilter) error {
	// Time range: to must be after from
	if !filter.TimeRange.To.After(filter.TimeRange.From) {
		return fmt.Errorf("timeRange.to must be after timeRange.from")
	}

	// Geo radius: default and cap
	if filter.Near != nil {
		if filter.Near.RadiusMiles == nil {
			d := DefaultRadiusMiles
			filter.Near.RadiusMiles = &d
		}
		if *filter.Near.RadiusMiles > MaxRadiusMiles {
			return fmt.Errorf("near.radiusMiles exceeds maximum of %.0f", MaxRadiusMiles)
		}
	}

	// EventTypeFilters: max 3, no duplicate types
	if len(filter.EventTypeFilters) > MaxEventTypeFilters {
		return fmt.Errorf("at most %d eventTypeFilters allowed", MaxEventTypeFilters)
	}
	seen := make(map[model.EventType]bool)
	for i, etf := range filter.EventTypeFilters {
		if seen[etf.EventType] {
			return fmt.Errorf("eventTypeFilters[%d]: duplicate eventType %s", i, etf.EventType)
		}
		seen[etf.EventType] = true

		// Per-type radius cap
		if etf.RadiusMiles != nil && *etf.RadiusMiles > MaxRadiusMiles {
			return fmt.Errorf("eventTypeFilters[%d]: radiusMiles exceeds maximum of %.0f", i, MaxRadiusMiles)
		}
	}

	// Pagination defaults and caps
	if filter.Limit == nil {
		d := MaxPageSize
		filter.Limit = &d
	} else if *filter.Limit > MaxPageSize {
		return fmt.Errorf("limit exceeds maximum of %d", MaxPageSize)
	}

	return nil
}
