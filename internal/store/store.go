package store

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/couchcryptid/storm-data-graphql-api/internal/model"
	"github.com/couchcryptid/storm-data-graphql-api/internal/observability"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const columns = `id, type, geo_lat, geo_lon, magnitude, unit,
	begin_time, end_time, source,
	location_raw, location_name, location_distance, location_direction,
	location_state, location_county,
	comments, severity, source_office, time_bucket, processed_at`

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
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
		ON CONFLICT (id) DO NOTHING`,
		report.ID, report.Type, report.Geo.Lat, report.Geo.Lon,
		report.Magnitude, report.Unit,
		report.BeginTime, report.EndTime, report.Source,
		report.Location.Raw, report.Location.Name,
		report.Location.Distance, report.Location.Direction,
		report.Location.State, report.Location.County,
		report.Comments, report.Severity, report.SourceOffice,
		report.TimeBucket, report.ProcessedAt,
	)
	return err
}

// buildWhereClause constructs the WHERE clause and args from a filter.
// Returns the clauses, args, and the next parameter index.
func buildWhereClause(filter *model.StormReportFilter) ([]string, []any, int) {
	var where []string
	var args []any
	idx := 1

	// Time bounds (always present — required by schema)
	where = append(where, fmt.Sprintf("begin_time >= $%d", idx))
	args = append(args, filter.BeginTimeAfter)
	idx++

	where = append(where, fmt.Sprintf("begin_time <= $%d", idx))
	args = append(args, filter.BeginTimeBefore)
	idx++

	// Array filters using ANY()
	if len(filter.Types) > 0 {
		where = append(where, fmt.Sprintf("type = ANY($%d)", idx))
		args = append(args, filter.Types)
		idx++
	}
	if len(filter.Severity) > 0 {
		where = append(where, fmt.Sprintf("severity = ANY($%d)", idx))
		args = append(args, filter.Severity)
		idx++
	}
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

	// Minimum magnitude
	if filter.MinMagnitude != nil {
		where = append(where, fmt.Sprintf("magnitude >= $%d", idx))
		args = append(args, *filter.MinMagnitude)
		idx++
	}

	// Radius geo filter
	if filter.NearLat != nil && filter.NearLon != nil && filter.RadiusMiles != nil {
		lat := *filter.NearLat
		lon := *filter.NearLon
		radiusMiles := *filter.RadiusMiles

		// Bounding box pre-filter for index usage
		latDelta := radiusMiles / 69.0
		lonDelta := radiusMiles / (69.0 * math.Cos(lat*math.Pi/180.0))
		where = append(where, fmt.Sprintf(
			"geo_lat BETWEEN $%d AND $%d AND geo_lon BETWEEN $%d AND $%d", idx, idx+1, idx+2, idx+3))
		args = append(args, lat-latDelta, lat+latDelta, lon-lonDelta, lon+lonDelta)
		idx += 4

		// Haversine exact distance
		where = append(where, fmt.Sprintf(`(
			3959 * acos(
				cos(radians($%d)) * cos(radians(geo_lat)) *
				cos(radians(geo_lon) - radians($%d)) +
				sin(radians($%d)) * sin(radians(geo_lat))
			)
		) <= $%d`, idx, idx+1, idx+2, idx+3))
		args = append(args, lat, lon, lat, radiusMiles)
		idx += 4
	}

	return where, args, idx
}

// sortColumn maps validated SortField enum values to SQL column names.
func sortColumn(sf model.SortField) string {
	switch sf {
	case model.SortFieldBeginTime:
		return "begin_time"
	case model.SortFieldMagnitude:
		return "magnitude"
	case model.SortFieldState:
		return "location_state"
	case model.SortFieldType:
		return "type"
	default:
		return "begin_time"
	}
}

// ListStormReports returns filtered, sorted, paginated reports and the total count.
func (s *Store) ListStormReports(ctx context.Context, filter *model.StormReportFilter) ([]*model.StormReport, int, error) {
	defer s.observeQuery("list", time.Now())
	where, baseArgs, idx := buildWhereClause(filter)

	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " WHERE " + strings.Join(where, " AND ")
	}

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

// CountByType returns reports grouped by event type.
func (s *Store) CountByType(ctx context.Context, filter *model.StormReportFilter) ([]*model.TypeGroup, error) {
	defer s.observeQuery("count_by_type", time.Now())
	where, args, _ := buildWhereClause(filter)

	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " WHERE " + strings.Join(where, " AND ")
	}

	query := `SELECT type, COUNT(*) AS cnt, MAX(magnitude) AS max_mag
		FROM storm_reports` + whereSQL + `
		GROUP BY type ORDER BY cnt DESC`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("count by type: %w", err)
	}
	defer rows.Close()

	var groups []*model.TypeGroup
	for rows.Next() {
		var g model.TypeGroup
		if err := rows.Scan(&g.Type, &g.Count, &g.MaxMagnitude); err != nil {
			return nil, fmt.Errorf("scan type group: %w", err)
		}
		groups = append(groups, &g)
	}
	return groups, rows.Err()
}

// CountByState returns reports grouped by state and county.
func (s *Store) CountByState(ctx context.Context, filter *model.StormReportFilter) ([]*model.StateGroup, error) {
	defer s.observeQuery("count_by_state", time.Now())
	where, args, _ := buildWhereClause(filter)

	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " WHERE " + strings.Join(where, " AND ")
	}

	query := `SELECT location_state, location_county, COUNT(*) AS cnt
		FROM storm_reports` + whereSQL + `
		GROUP BY location_state, location_county
		ORDER BY location_state, cnt DESC`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("count by state: %w", err)
	}
	defer rows.Close()

	stateMap := make(map[string]*model.StateGroup)
	var stateOrder []string
	for rows.Next() {
		var state, county string
		var count int
		if err := rows.Scan(&state, &county, &count); err != nil {
			return nil, fmt.Errorf("scan state group: %w", err)
		}
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
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	groups := make([]*model.StateGroup, 0, len(stateOrder))
	for _, st := range stateOrder {
		groups = append(groups, stateMap[st])
	}
	return groups, nil
}

// CountByHour returns reports grouped by time_bucket.
func (s *Store) CountByHour(ctx context.Context, filter *model.StormReportFilter) ([]*model.TimeGroup, error) {
	defer s.observeQuery("count_by_hour", time.Now())
	where, args, _ := buildWhereClause(filter)

	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " WHERE " + strings.Join(where, " AND ")
	}

	query := `SELECT time_bucket, COUNT(*) AS cnt
		FROM storm_reports` + whereSQL + `
		GROUP BY time_bucket ORDER BY time_bucket`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("count by hour: %w", err)
	}
	defer rows.Close()

	var groups []*model.TimeGroup
	for rows.Next() {
		var g model.TimeGroup
		if err := rows.Scan(&g.Bucket, &g.Count); err != nil {
			return nil, fmt.Errorf("scan time group: %w", err)
		}
		groups = append(groups, &g)
	}
	return groups, rows.Err()
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
		&r.Magnitude, &r.Unit,
		&r.BeginTime, &r.EndTime, &r.Source,
		&r.Location.Raw, &r.Location.Name,
		&r.Location.Distance, &r.Location.Direction,
		&r.Location.State, &r.Location.County,
		&r.Comments, &r.Severity, &r.SourceOffice,
		&r.TimeBucket, &r.ProcessedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan storm report: %w", err)
	}
	return &r, nil
}
