package store

import (
	"context"
	"fmt"
	"time"

	"github.com/couchcryptid/storm-data-api/internal/model"
)

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
			SELECT event_type, location_state, location_county,
				   measurement_magnitude, measurement_severity, time_bucket
			FROM storm_reports` + whereSQL + `
		)
		SELECT 'type' AS agg, event_type AS key1, NULL AS key2,
			   COUNT(*) AS count, MAX(measurement_magnitude) AS max_mag, NULL AS max_sev, NULL::timestamptz AS bucket
		FROM base GROUP BY event_type
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
				EventType: stringOrEmpty(key1),
				Count:     count,
			}
			if maxMag != nil {
				etg.MaxMeasurement = &model.Measurement{
					Magnitude: *maxMag,
					Unit:      unitForEventType(stringOrEmpty(key1)),
				}
			}
			result.ByEventType = append(result.ByEventType, etg)
		case "state":
			state := stringOrEmpty(key1)
			county := stringOrEmpty(key2)
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

func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
