package store

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/couchcryptid/storm-data-api/internal/model"
	"github.com/couchcryptid/storm-data-api/internal/observability"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const columns = `id, type, geo_lat, geo_lon, measurement_magnitude, measurement_unit,
	begin_time, end_time, source,
	location_raw, location_name, location_distance, location_direction,
	location_state, location_county,
	comments, measurement_severity, source_office, time_bucket, processed_at,
	geocoding_formatted_address, geocoding_place_name, geocoding_confidence, geocoding_source`

// Store provides persistence operations for storm reports backed by PostgreSQL.
type Store struct {
	pool    *pgxpool.Pool
	metrics *observability.Metrics
}

// New creates a Store with the given connection pool and metrics.
func New(pool *pgxpool.Pool, m *observability.Metrics) *Store {
	return &Store{pool: pool, metrics: m}
}

func (s *Store) observeQuery(operation string, start time.Time) {
	s.metrics.DBQueryDuration.WithLabelValues(operation).Observe(time.Since(start).Seconds())
}

// InsertStormReport upserts a storm report into the database.
func (s *Store) InsertStormReport(ctx context.Context, report *model.StormReport) error {
	defer s.observeQuery("insert", time.Now())
	_, err := s.pool.Exec(ctx, `
		INSERT INTO storm_reports (`+columns+`)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)
		ON CONFLICT (id) DO NOTHING`,
		report.ID, report.Type, report.Geo.Lat, report.Geo.Lon,
		report.Measurement.Magnitude, report.Measurement.Unit,
		report.BeginTime, report.EndTime, report.Source,
		report.Location.Raw, report.Location.Name,
		report.Location.Distance, report.Location.Direction,
		report.Location.State, report.Location.County,
		report.Comments, report.Measurement.Severity, report.SourceOffice,
		report.TimeBucket, report.ProcessedAt,
		report.Geocoding.FormattedAddress, report.Geocoding.PlaceName,
		report.Geocoding.Confidence, report.Geocoding.Source,
	)
	return err
}

const insertSQL = `INSERT INTO storm_reports (` + columns + `)
	VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)
	ON CONFLICT (id) DO NOTHING`

// InsertStormReports batch-inserts multiple storm reports using pgx.Batch.
func (s *Store) InsertStormReports(ctx context.Context, reports []*model.StormReport) error {
	if len(reports) == 0 {
		return nil
	}
	defer s.observeQuery("batch_insert", time.Now())

	batch := &pgx.Batch{}
	for _, r := range reports {
		batch.Queue(insertSQL,
			r.ID, r.Type, r.Geo.Lat, r.Geo.Lon,
			r.Measurement.Magnitude, r.Measurement.Unit,
			r.BeginTime, r.EndTime, r.Source,
			r.Location.Raw, r.Location.Name,
			r.Location.Distance, r.Location.Direction,
			r.Location.State, r.Location.County,
			r.Comments, r.Measurement.Severity, r.SourceOffice,
			r.TimeBucket, r.ProcessedAt,
			r.Geocoding.FormattedAddress, r.Geocoding.PlaceName,
			r.Geocoding.Confidence, r.Geocoding.Source,
		)
	}

	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()

	for range reports {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("batch insert: %w", err)
		}
	}

	return nil
}

// buildWhereSQL joins the clauses into a WHERE fragment (empty string if no clauses).
func buildWhereSQL(clauses []string) string {
	if len(clauses) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(clauses, " AND ")
}

// buildWhereClause constructs the WHERE clause and args from a filter.
// Returns the clauses, args, and the next parameter index.
func buildWhereClause(filter *model.StormReportFilter) ([]string, []any, int) {
	var where []string
	var args []any
	idx := 1

	// Time bounds (always present — required by schema)
	where = append(where, fmt.Sprintf("begin_time >= $%d", idx))
	args = append(args, filter.TimeRange.From)
	idx++

	where = append(where, fmt.Sprintf("begin_time <= $%d", idx))
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
		// Mode 2: Per-type OR conditions with optional per-type radius
		clause, newArgs, newIdx := buildEventTypeConditions(filter, args, idx)
		where = append(where, clause...)
		args = newArgs
		idx = newIdx
	} else {
		// Mode 1: Simple AND conditions
		if len(filter.EventTypes) > 0 {
			where = append(where, fmt.Sprintf("type = ANY($%d)", idx))
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

	for _, etf := range filter.EventTypeFilters {
		overrideSet[etf.EventType] = true
		tc := typeCondition{eventType: etf.EventType}
		if len(etf.Severity) > 0 {
			tc.severity = etf.Severity
		} else {
			tc.severity = filter.Severity
		}
		if etf.MinMagnitude != nil {
			tc.minMag = etf.MinMagnitude
		} else {
			tc.minMag = filter.MinMagnitude
		}
		if etf.RadiusMiles != nil {
			tc.radiusMiles = etf.RadiusMiles
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

// buildEventTypeConditions builds bounding-box and per-type OR clauses for eventTypeFilters.
// Returns additional WHERE clauses, updated args, and the next parameter index.
func buildEventTypeConditions(filter *model.StormReportFilter, args []any, idx int) ([]string, []any, int) {
	conditions := collectTypeConditions(filter)
	var clauses []string

	// Bounding box using the max radius across all conditions (for index usage)
	if filter.Near != nil {
		var maxRadius float64
		for _, tc := range conditions {
			if tc.radiusMiles != nil && *tc.radiusMiles > maxRadius {
				maxRadius = *tc.radiusMiles
			}
		}
		if maxRadius > 0 {
			bbWhere, bbArgs, bbIdx := buildBoundingBox(filter.Near.Lat, filter.Near.Lon, maxRadius, idx)
			clauses = append(clauses, bbWhere...)
			args = append(args, bbArgs...)
			idx = bbIdx
		}
	}

	// Per-type OR clauses
	orParts := make([]string, 0, len(conditions))
	for _, tc := range conditions {
		var parts []string
		parts = append(parts, fmt.Sprintf("type = $%d", idx))
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
		if filter.Near != nil && tc.radiusMiles != nil {
			hav := buildHaversine(filter.Near.Lat, filter.Near.Lon, *tc.radiusMiles, idx)
			parts = append(parts, hav.clause)
			args = append(args, hav.args...)
			idx = hav.nextIdx
		}
		orParts = append(orParts, "("+strings.Join(parts, " AND ")+")")
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
	latDelta := radiusMiles / 69.0
	lonDelta := radiusMiles / (69.0 * math.Cos(lat*math.Pi/180.0))
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
// 3959 is the Earth's mean radius in miles.
func buildHaversine(lat, lon, radiusMiles float64, idx int) haversineResult {
	clause := fmt.Sprintf(`(
		3959 * acos(
			cos(radians($%d)) * cos(radians(geo_lat)) *
			cos(radians(geo_lon) - radians($%d)) +
			sin(radians($%d)) * sin(radians(geo_lat))
		)
	) <= $%d`, idx, idx+1, idx+2, idx+3)
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
	case model.SortFieldBeginTime:
		return "begin_time"
	case model.SortFieldMagnitude:
		return "measurement_magnitude"
	case model.SortFieldLocationState:
		return "location_state"
	case model.SortFieldEventType:
		return "type"
	default:
		return "begin_time"
	}
}

// ListStormReports returns filtered, sorted, paginated reports and the total count.
func (s *Store) ListStormReports(ctx context.Context, filter *model.StormReportFilter) ([]*model.StormReport, int, error) {
	defer s.observeQuery("list", time.Now())
	where, baseArgs, idx := buildWhereClause(filter)

	whereSQL := buildWhereSQL(where)

	// Count total matching rows
	countQuery := "SELECT COUNT(*) FROM storm_reports" + whereSQL
	var totalCount int
	if err := s.pool.QueryRow(ctx, countQuery, baseArgs...).Scan(&totalCount); err != nil {
		return nil, 0, fmt.Errorf("count storm reports: %w", err)
	}

	// Build data query with sorting and pagination
	orderCol := "begin_time"
	orderDir := "DESC"
	if filter.SortBy != nil && filter.SortBy.IsValid() {
		orderCol = sortColumn(*filter.SortBy)
	}
	if filter.SortOrder != nil && filter.SortOrder.IsValid() && *filter.SortOrder == model.SortOrderAsc {
		orderDir = "ASC"
	}

	dataArgs := make([]any, len(baseArgs))
	copy(dataArgs, baseArgs)

	query := "SELECT " + columns + " FROM storm_reports" + whereSQL +
		fmt.Sprintf(" ORDER BY %s %s", orderCol, orderDir)

	if filter.Limit != nil {
		query += fmt.Sprintf(" LIMIT $%d", idx)
		dataArgs = append(dataArgs, *filter.Limit)
		idx++
	}
	if filter.Offset != nil {
		query += fmt.Sprintf(" OFFSET $%d", idx)
		dataArgs = append(dataArgs, *filter.Offset)
	}

	rows, err := s.pool.Query(ctx, query, dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query storm reports: %w", err)
	}
	defer rows.Close()

	var reports []*model.StormReport
	for rows.Next() {
		r, err := scanStormReport(rows)
		if err != nil {
			return nil, 0, err
		}
		reports = append(reports, r)
	}
	return reports, totalCount, rows.Err()
}

// AggResult holds combined aggregation results from a single CTE query.
type AggResult struct {
	ByEventType []*model.EventTypeGroup
	ByState     []*model.StateGroup
	ByHour      []*model.TimeGroup
}

// unitForEventType returns the measurement unit for a given event type.
func unitForEventType(et string) string {
	switch et {
	case "hail":
		return "in"
	case "wind":
		return "mph"
	case "tornado":
		return "f_scale"
	default:
		return ""
	}
}

// Aggregations returns event type, state, and hourly aggregations in a single query.
// Uses a CTE with UNION ALL to compute all three aggregation types in one database
// round-trip. The "agg" discriminator column routes each row to the appropriate
// result slice during scanning.
func (s *Store) Aggregations(ctx context.Context, filter *model.StormReportFilter) (*AggResult, error) {
	defer s.observeQuery("aggregations", time.Now())
	where, args, _ := buildWhereClause(filter)
	whereSQL := buildWhereSQL(where)

	query := `WITH base AS (
			SELECT type, location_state, location_county,
				   measurement_magnitude, measurement_severity, time_bucket
			FROM storm_reports` + whereSQL + `
		)
		SELECT 'type' AS agg, type AS key1, NULL AS key2,
			   COUNT(*) AS count, MAX(measurement_magnitude) AS max_mag, NULL AS max_sev, NULL::timestamptz AS bucket
		FROM base GROUP BY type
		UNION ALL
		SELECT 'state', location_state, location_county,
			   COUNT(*), NULL, NULL, NULL
		FROM base GROUP BY location_state, location_county
		UNION ALL
		SELECT 'hour', NULL, NULL,
			   COUNT(*), NULL, NULL, time_bucket
		FROM base GROUP BY time_bucket`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("aggregations: %w", err)
	}
	defer rows.Close()

	result := &AggResult{}
	stateMap := make(map[string]*model.StateGroup)
	var stateOrder []string

	for rows.Next() {
		var agg string
		var key1, key2 *string
		var count int
		var maxMag *float64
		var maxSev *string
		var bucket *time.Time

		if err := rows.Scan(&agg, &key1, &key2, &count, &maxMag, &maxSev, &bucket); err != nil {
			return nil, fmt.Errorf("scan aggregation row: %w", err)
		}

		switch agg {
		case "type":
			etg := &model.EventTypeGroup{
				EventType: deref(key1),
				Count:     count,
			}
			if maxMag != nil {
				etg.MaxMeasurement = &model.Measurement{
					Magnitude: *maxMag,
					Unit:      unitForEventType(deref(key1)),
				}
			}
			result.ByEventType = append(result.ByEventType, etg)
		case "state":
			state := deref(key1)
			county := deref(key2)
			sg, ok := stateMap[state]
			if !ok {
				sg = &model.StateGroup{State: state}
				stateMap[state] = sg
				stateOrder = append(stateOrder, state)
			}
			sg.Count += count
			sg.Counties = append(sg.Counties, &model.CountyGroup{
				County: county,
				Count:  count,
			})
		case "hour":
			if bucket != nil {
				result.ByHour = append(result.ByHour, &model.TimeGroup{
					Bucket: *bucket,
					Count:  count,
				})
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, st := range stateOrder {
		result.ByState = append(result.ByState, stateMap[st])
	}

	return result, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// LastUpdated returns the most recent processed_at timestamp.
func (s *Store) LastUpdated(ctx context.Context) (*time.Time, error) {
	defer s.observeQuery("last_updated", time.Now())
	var t *time.Time
	err := s.pool.QueryRow(ctx, "SELECT MAX(processed_at) FROM storm_reports").Scan(&t)
	if err != nil {
		return nil, fmt.Errorf("last updated: %w", err)
	}
	return t, nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanStormReport(row scannable) (*model.StormReport, error) {
	var r model.StormReport
	err := row.Scan(
		&r.ID, &r.Type, &r.Geo.Lat, &r.Geo.Lon,
		&r.Measurement.Magnitude, &r.Measurement.Unit,
		&r.BeginTime, &r.EndTime, &r.Source,
		&r.Location.Raw, &r.Location.Name,
		&r.Location.Distance, &r.Location.Direction,
		&r.Location.State, &r.Location.County,
		&r.Comments, &r.Measurement.Severity, &r.SourceOffice,
		&r.TimeBucket, &r.ProcessedAt,
		&r.Geocoding.FormattedAddress, &r.Geocoding.PlaceName,
		&r.Geocoding.Confidence, &r.Geocoding.Source,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan storm report: %w", err)
	}
	return &r, nil
}
