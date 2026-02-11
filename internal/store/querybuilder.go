package store

import (
	"fmt"
	"math"
	"strings"

	"github.com/couchcryptid/storm-data-api/internal/model"
)

const (
	// earthRadiusMiles is used by the haversine formula for great-circle distance.
	earthRadiusMiles = 3959.0

	// milesPerDegreeLat approximates the miles-per-degree latitude (~69 mi).
	// Used by bounding-box pre-filtering for B-tree index utilization.
	milesPerDegreeLat = 69.0
)

// buildWhereSQL joins the clauses into a WHERE fragment (empty string if no clauses).
func buildWhereSQL(clauses []string) string {
	if len(clauses) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(clauses, " AND ")
}

// buildWhereClause constructs the WHERE clause and args from a filter.
// Returns the clauses, args, and the next parameter index.
// idx tracks the PostgreSQL positional parameter number ($1, $2, …).
func buildWhereClause(filter *model.StormReportFilter) ([]string, []any, int) {
	var where []string
	var args []any
	idx := 1

	// Time bounds (always present — required by schema)
	where = append(where, fmt.Sprintf("event_time >= $%d", idx))
	args = append(args, filter.TimeRange.From)
	idx++

	where = append(where, fmt.Sprintf("event_time <= $%d", idx))
	args = append(args, filter.TimeRange.To)
	idx++

	// Administrative location filters
	if len(filter.States) > 0 {
		where = append(where, fmt.Sprintf("location_state = ANY($%d)", idx))
		args = append(args, filter.States)
		idx++
	}
	if len(filter.Counties) > 0 {
		where = append(where, fmt.Sprintf("location_county = ANY($%d)", idx))
		args = append(args, filter.Counties)
		idx++
	}

	if len(filter.EventTypeFilters) > 0 {
		// Per-type OR filtering: each event type can have its own severity/magnitude/radius
		clause, newArgs, newIdx := buildEventTypeConditions(filter, args, idx)
		where = append(where, clause...)
		args = newArgs
		idx = newIdx
	} else {
		// Simple AND filtering: global filters apply uniformly to all event types
		if len(filter.EventTypes) > 0 {
			where = append(where, fmt.Sprintf("event_type = ANY($%d)", idx))
			args = append(args, eventTypeDBValues(filter.EventTypes))
			idx++
		}
		if len(filter.Severity) > 0 {
			where = append(where, fmt.Sprintf("measurement_severity = ANY($%d)", idx))
			args = append(args, severityDBValues(filter.Severity))
			idx++
		}
		if filter.MinMagnitude != nil {
			where = append(where, fmt.Sprintf("measurement_magnitude >= $%d", idx))
			args = append(args, *filter.MinMagnitude)
			idx++
		}
		if filter.Near != nil {
			geoWhere, geoArgs, geoIdx := buildGeoClause(filter.Near.Lat, filter.Near.Lon, filter.Near.RadiusMiles, idx)
			where = append(where, geoWhere...)
			args = append(args, geoArgs...)
			idx = geoIdx
		}
	}

	return where, args, idx
}

type typeCondition struct {
	eventType   model.EventType
	severity    []model.Severity
	minMag      *float64
	radiusMiles *float64
}

// collectTypeConditions merges explicit per-type overrides with unoverridden eventTypes.
// For example, given eventTypes=[HAIL, WIND], severity=[SEVERE], and
// eventTypeFilters=[{eventType: HAIL, severity: [MODERATE]}], this returns:
//   - HAIL with severity=[MODERATE] (overridden)
//   - WIND with severity=[SEVERE] (global default, not overridden)
func collectTypeConditions(filter *model.StormReportFilter) []typeCondition {
	overrideSet := make(map[model.EventType]bool)
	conditions := make([]typeCondition, 0, len(filter.EventTypeFilters)+len(filter.EventTypes))

	for _, typeFilter := range filter.EventTypeFilters {
		overrideSet[typeFilter.EventType] = true
		tc := typeCondition{eventType: typeFilter.EventType}
		if len(typeFilter.Severity) > 0 {
			tc.severity = typeFilter.Severity
		} else {
			tc.severity = filter.Severity
		}
		if typeFilter.MinMagnitude != nil {
			tc.minMag = typeFilter.MinMagnitude
		} else {
			tc.minMag = filter.MinMagnitude
		}
		if typeFilter.RadiusMiles != nil {
			tc.radiusMiles = typeFilter.RadiusMiles
		} else if filter.Near != nil {
			tc.radiusMiles = filter.Near.RadiusMiles
		}
		conditions = append(conditions, tc)
	}

	for _, et := range filter.EventTypes {
		if !overrideSet[et] {
			tc := typeCondition{
				eventType: et,
				severity:  filter.Severity,
				minMag:    filter.MinMagnitude,
			}
			if filter.Near != nil {
				tc.radiusMiles = filter.Near.RadiusMiles
			}
			conditions = append(conditions, tc)
		}
	}

	return conditions
}

// maxRadius returns the largest radius across all type conditions, or 0 if none.
func maxRadius(conditions []typeCondition) float64 {
	var largest float64
	for _, tc := range conditions {
		if tc.radiusMiles != nil && *tc.radiusMiles > largest {
			largest = *tc.radiusMiles
		}
	}
	return largest
}

// buildSingleTypeCondition builds the AND-joined predicate for one event type condition.
// Returns the parenthesized clause, its args, and the next parameter index.
func buildSingleTypeCondition(tc typeCondition, near *model.GeoRadiusFilter, idx int) (string, []any, int) {
	var parts []string
	var args []any

	parts = append(parts, fmt.Sprintf("event_type = $%d", idx))
	args = append(args, tc.eventType.DBValue())
	idx++

	if len(tc.severity) > 0 {
		parts = append(parts, fmt.Sprintf("measurement_severity = ANY($%d)", idx))
		args = append(args, severityDBValues(tc.severity))
		idx++
	}
	if tc.minMag != nil {
		parts = append(parts, fmt.Sprintf("measurement_magnitude >= $%d", idx))
		args = append(args, *tc.minMag)
		idx++
	}
	if near != nil && tc.radiusMiles != nil {
		hav := buildHaversine(near.Lat, near.Lon, *tc.radiusMiles, idx)
		parts = append(parts, hav.clause)
		args = append(args, hav.args...)
		idx = hav.nextIdx
	}

	return "(" + strings.Join(parts, " AND ") + ")", args, idx
}

// buildEventTypeConditions builds bounding-box and per-type OR clauses for eventTypeFilters.
// Returns additional WHERE clauses, updated args, and the next parameter index.
func buildEventTypeConditions(filter *model.StormReportFilter, args []any, idx int) ([]string, []any, int) {
	conditions := collectTypeConditions(filter)
	var clauses []string

	// Bounding box using the max radius across all conditions (for index usage)
	if filter.Near != nil {
		if r := maxRadius(conditions); r > 0 {
			bbWhere, bbArgs, bbIdx := buildBoundingBox(filter.Near.Lat, filter.Near.Lon, r, idx)
			clauses = append(clauses, bbWhere...)
			args = append(args, bbArgs...)
			idx = bbIdx
		}
	}

	// Per-type OR clauses
	orParts := make([]string, 0, len(conditions))
	for _, tc := range conditions {
		clause, tcArgs, nextIdx := buildSingleTypeCondition(tc, filter.Near, idx)
		orParts = append(orParts, clause)
		args = append(args, tcArgs...)
		idx = nextIdx
	}
	clauses = append(clauses, "("+strings.Join(orParts, " OR ")+")")

	return clauses, args, idx
}

// buildGeoClause builds bounding-box + haversine clauses for a single radius filter.
func buildGeoClause(lat, lon float64, radiusMiles *float64, idx int) ([]string, []any, int) {
	if radiusMiles == nil {
		return nil, nil, idx
	}
	bbWhere, bbArgs, bbIdx := buildBoundingBox(lat, lon, *radiusMiles, idx)
	hav := buildHaversine(lat, lon, *radiusMiles, bbIdx)

	clauses := make([]string, 0, len(bbWhere)+1)
	clauses = append(clauses, bbWhere...)
	clauses = append(clauses, hav.clause)

	args := make([]any, 0, len(bbArgs)+len(hav.args))
	args = append(args, bbArgs...)
	args = append(args, hav.args...)

	return clauses, args, hav.nextIdx
}

// buildBoundingBox builds lat/lon bounding box clauses for index pre-filtering
// before applying the precise haversine distance calculation. Uses approximate
// degrees-per-mile conversions: ~69 miles/degree latitude (constant globally),
// ~69*cos(lat) miles/degree longitude (varies by latitude). The coarse bounding
// box leverages the (geo_lat, geo_lon) B-tree index to quickly eliminate rows
// outside the search area before the expensive haversine runs on the remainder.
func buildBoundingBox(lat, lon, radiusMiles float64, idx int) ([]string, []any, int) {
	latDelta := radiusMiles / milesPerDegreeLat
	lonDelta := radiusMiles / (milesPerDegreeLat * math.Cos(lat*math.Pi/180.0))
	clause := fmt.Sprintf(
		"geo_lat BETWEEN $%d AND $%d AND geo_lon BETWEEN $%d AND $%d",
		idx, idx+1, idx+2, idx+3)
	args := []any{lat - latDelta, lat + latDelta, lon - lonDelta, lon + lonDelta}
	return []string{clause}, args, idx + 4
}

type haversineResult struct {
	clause  string
	args    []any
	nextIdx int
}

// buildHaversine builds a haversine great-circle distance clause.
func buildHaversine(lat, lon, radiusMiles float64, idx int) haversineResult {
	clause := fmt.Sprintf(`(
		%v * acos(
			cos(radians($%d)) * cos(radians(geo_lat)) *
			cos(radians(geo_lon) - radians($%d)) +
			sin(radians($%d)) * sin(radians(geo_lat))
		)
	) <= $%d`, earthRadiusMiles, idx, idx+1, idx+2, idx+3)
	return haversineResult{
		clause:  clause,
		args:    []any{lat, lon, lat, radiusMiles},
		nextIdx: idx + 4,
	}
}

// eventTypeDBValues converts a slice of EventType enums to their lowercase DB values.
func eventTypeDBValues(types []model.EventType) []string {
	vals := make([]string, len(types))
	for i, t := range types {
		vals[i] = t.DBValue()
	}
	return vals
}

// severityDBValues converts a slice of Severity enums to their lowercase DB values.
func severityDBValues(sevs []model.Severity) []string {
	vals := make([]string, len(sevs))
	for i, s := range sevs {
		vals[i] = s.DBValue()
	}
	return vals
}

// sortColumn maps validated SortField enum values to SQL column names.
func sortColumn(sf model.SortField) string {
	switch sf {
	case model.SortFieldEventTime:
		return "event_time"
	case model.SortFieldMagnitude:
		return "measurement_magnitude"
	case model.SortFieldLocationState:
		return "location_state"
	case model.SortFieldEventType:
		return "event_type"
	default:
		return "event_time"
	}
}
