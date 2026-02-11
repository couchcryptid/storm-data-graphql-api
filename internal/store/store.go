package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/couchcryptid/storm-data-api/internal/model"
	"github.com/couchcryptid/storm-data-api/internal/observability"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const columns = `id, event_type, geo_lat, geo_lon, measurement_magnitude, measurement_unit,
	event_time,
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
// IDs are deterministic SHA-256 hashes (event_type+state+coords+time+magnitude), so identical
// events always produce the same ID. ON CONFLICT DO NOTHING makes inserts
// idempotent, which is safe for Kafka's at-least-once delivery.
func (s *Store) InsertStormReport(ctx context.Context, report *model.StormReport) error {
	defer s.observeQuery("insert", time.Now())
	_, err := s.pool.Exec(ctx, `
		INSERT INTO storm_reports (`+columns+`)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)
		ON CONFLICT (id) DO NOTHING`,
		report.ID, report.EventType, report.Geo.Lat, report.Geo.Lon,
		report.Measurement.Magnitude, report.Measurement.Unit,
		report.EventTime,
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
	VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)
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
			r.ID, r.EventType, r.Geo.Lat, r.Geo.Lon,
			r.Measurement.Magnitude, r.Measurement.Unit,
			r.EventTime,
			r.Location.Raw, r.Location.Name,
			r.Location.Distance, r.Location.Direction,
			r.Location.State, r.Location.County,
			r.Comments, r.Measurement.Severity, r.SourceOffice,
			r.TimeBucket, r.ProcessedAt,
			r.Geocoding.FormattedAddress, r.Geocoding.PlaceName,
			r.Geocoding.Confidence, r.Geocoding.Source,
		)
	}

	batchResults := s.pool.SendBatch(ctx, batch)
	defer batchResults.Close()

	for range reports {
		if _, err := batchResults.Exec(); err != nil {
			return fmt.Errorf("batch insert: %w", err)
		}
	}

	return nil
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
	orderCol := "event_time"
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
		&r.ID, &r.EventType, &r.Geo.Lat, &r.Geo.Lon,
		&r.Measurement.Magnitude, &r.Measurement.Unit,
		&r.EventTime,
		&r.Location.Raw, &r.Location.Name,
		&r.Location.Distance, &r.Location.Direction,
		&r.Location.State, &r.Location.County,
		&r.Comments, &r.Measurement.Severity, &r.SourceOffice,
		&r.TimeBucket, &r.ProcessedAt,
		&r.Geocoding.FormattedAddress, &r.Geocoding.PlaceName,
		&r.Geocoding.Confidence, &r.Geocoding.Source,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan storm report: %w", err)
	}
	return &r, nil
}
