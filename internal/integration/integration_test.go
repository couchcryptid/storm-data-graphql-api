//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/couchcryptid/storm-data-graphql-api/internal/database"
	"github.com/couchcryptid/storm-data-graphql-api/internal/model"
	"github.com/couchcryptid/storm-data-graphql-api/internal/observability"
	"github.com/couchcryptid/storm-data-graphql-api/internal/store"

	kafkago "github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcKafka "github.com/testcontainers/testcontainers-go/modules/kafka"
	"github.com/testcontainers/testcontainers-go/wait"
)

// discardLogger returns a logger that discards all output.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// wideFilter returns a filter with a time window that covers all mock data.
func wideFilter() *model.StormReportFilter {
	return &model.StormReportFilter{
		BeginTimeAfter:  time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		BeginTimeBefore: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func startPostgres(ctx context.Context, t *testing.T) (string, testcontainers.Container) {
	t.Helper()
	req := testcontainers.ContainerRequest{
		Image:        "postgres:16",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "test",
			"POSTGRES_PASSWORD": "test",
			"POSTGRES_DB":       "testdb",
		},
		WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(30 * time.Second),
	}
	pg, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "start postgres")

	host, _ := pg.Host(ctx)
	port, _ := pg.MappedPort(ctx, "5432")
	dsn := fmt.Sprintf("postgres://test:test@%s:%s/testdb?sslmode=disable", host, port.Port())
	return dsn, pg
}

func startKafka(ctx context.Context, t *testing.T) (string, *tcKafka.KafkaContainer) {
	t.Helper()
	kc, err := tcKafka.Run(ctx, "confluentinc/confluent-local:7.6.0")
	require.NoError(t, err, "start kafka")

	brokers, err := kc.Brokers(ctx)
	require.NoError(t, err, "get brokers")
	return brokers[0], kc
}

func loadMockReports(t *testing.T) []model.StormReport {
	t.Helper()
	data, err := os.ReadFile("../../data/mock/storm_reports_240526_transformed.json")
	require.NoError(t, err, "read mock data")
	var reports []model.StormReport
	require.NoError(t, json.Unmarshal(data, &reports), "unmarshal mock data")
	return reports
}

func TestStoreInsertAndQuery(t *testing.T) {
	ctx := context.Background()

	dsn, pg := startPostgres(ctx, t)
	defer func() { _ = pg.Terminate(ctx) }()

	require.NoError(t, database.RunMigrations(dsn))

	pool, err := database.NewPool(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()

	s := store.New(pool, observability.NewTestMetrics())
	reports := loadMockReports(t)

	// Insert all reports
	for i := range reports {
		require.NoError(t, s.InsertStormReport(ctx, &reports[i]), "insert report %s", reports[i].ID)
	}

	// List all
	all, totalCount, err := s.ListStormReports(ctx, wideFilter())
	require.NoError(t, err)
	assert.Len(t, all, 30)
	assert.Equal(t, 30, totalCount)

	// Filter by type
	f := wideFilter()
	f.Types = []string{"hail"}
	hailReports, hailCount, err := s.ListStormReports(ctx, f)
	require.NoError(t, err)
	assert.Len(t, hailReports, 10)
	assert.Equal(t, 10, hailCount)

	// Filter by state
	f = wideFilter()
	f.States = []string{"TX"}
	txReports, _, err := s.ListStormReports(ctx, f)
	require.NoError(t, err)
	for _, r := range txReports {
		assert.Equal(t, "TX", r.Location.State, "report %s", r.ID)
	}

	// Filter by geo radius (around Fort Worth, TX area)
	f = wideFilter()
	lat := 32.75
	lon := -97.15
	radius := 20.0
	f.NearLat = &lat
	f.NearLon = &lon
	f.RadiusMiles = &radius
	geoReports, _, err := s.ListStormReports(ctx, f)
	require.NoError(t, err)
	assert.NotEmpty(t, geoReports, "expected reports near Fort Worth")
	for _, r := range geoReports {
		assert.InDelta(t, 32.75, r.Geo.Lat, 0.75, "report %s lat outside expected range", r.ID)
	}
}

func TestStoreAggregations(t *testing.T) {
	ctx := context.Background()
	s := setupStoreWithData(ctx, t)

	t.Run("CountByType", func(t *testing.T) {
		groups, err := s.CountByType(ctx, wideFilter())
		require.NoError(t, err)
		require.Len(t, groups, 3)
		counts := map[string]int{}
		for _, g := range groups {
			counts[g.Type] = g.Count
		}
		assert.Equal(t, 10, counts["hail"])
		assert.Equal(t, 10, counts["tornado"])
		assert.Equal(t, 10, counts["wind"])

		for _, g := range groups {
			if g.Type == "hail" {
				require.NotNil(t, g.MaxMagnitude)
				assert.Equal(t, 1.75, *g.MaxMagnitude)
			}
		}
	})

	t.Run("CountByState", func(t *testing.T) {
		groups, err := s.CountByState(ctx, wideFilter())
		require.NoError(t, err)
		stateCount := map[string]int{}
		for _, g := range groups {
			stateCount[g.State] = g.Count
		}
		assert.NotZero(t, stateCount["TX"])
		assert.NotZero(t, stateCount["NE"])

		for _, g := range groups {
			if g.State == "TX" {
				assert.NotEmpty(t, g.Counties, "TX should have counties")
				countyTotal := 0
				for _, c := range g.Counties {
					countyTotal += c.Count
				}
				assert.Equal(t, g.Count, countyTotal, "TX county sum should equal state count")
			}
		}
	})

	t.Run("CountByHour", func(t *testing.T) {
		groups, err := s.CountByHour(ctx, wideFilter())
		require.NoError(t, err)
		require.NotEmpty(t, groups)
		total := 0
		for _, g := range groups {
			total += g.Count
			assert.False(t, g.Bucket.IsZero(), "bucket should not be zero")
		}
		assert.Equal(t, 30, total)
	})

	t.Run("LastUpdated", func(t *testing.T) {
		ts, err := s.LastUpdated(ctx)
		require.NoError(t, err)
		require.NotNil(t, ts)
		assert.False(t, ts.IsZero())
	})

	t.Run("CountByType with filter", func(t *testing.T) {
		f := wideFilter()
		f.Types = []string{"hail"}
		groups, err := s.CountByType(ctx, f)
		require.NoError(t, err)
		require.Len(t, groups, 1)
		assert.Equal(t, "hail", groups[0].Type)
		assert.Equal(t, 10, groups[0].Count)
	})
}

func TestStoreFilters(t *testing.T) {
	ctx := context.Background()
	s := setupStoreWithData(ctx, t)

	t.Run("severity filter", func(t *testing.T) {
		f := wideFilter()
		f.Severity = []string{"severe"}
		reports, count, err := s.ListStormReports(ctx, f)
		require.NoError(t, err)
		assert.Equal(t, 4, count)
		for _, r := range reports {
			require.NotNil(t, r.Severity, "report %s", r.ID)
			assert.Equal(t, "severe", *r.Severity, "report %s", r.ID)
		}
	})

	t.Run("multiple severities", func(t *testing.T) {
		f := wideFilter()
		f.Severity = []string{"severe", "moderate"}
		_, count, err := s.ListStormReports(ctx, f)
		require.NoError(t, err)
		assert.Equal(t, 13, count)
	})

	t.Run("counties filter", func(t *testing.T) {
		f := wideFilter()
		f.Counties = []string{"Tarrant"}
		reports, count, err := s.ListStormReports(ctx, f)
		require.NoError(t, err)
		assert.Equal(t, 4, count)
		for _, r := range reports {
			assert.Equal(t, "Tarrant", r.Location.County, "report %s", r.ID)
		}
	})

	t.Run("minMagnitude filter", func(t *testing.T) {
		f := wideFilter()
		min := 1.75
		f.MinMagnitude = &min
		reports, count, err := s.ListStormReports(ctx, f)
		require.NoError(t, err)
		assert.Equal(t, 6, count)
		for _, r := range reports {
			assert.GreaterOrEqual(t, r.Magnitude, 1.75, "report %s", r.ID)
		}
	})

	t.Run("combined filters", func(t *testing.T) {
		f := wideFilter()
		f.Types = []string{"hail"}
		f.States = []string{"TX"}
		f.Severity = []string{"severe"}
		reports, count, err := s.ListStormReports(ctx, f)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
		for _, r := range reports {
			assert.Equal(t, "hail", r.Type, "report %s", r.ID)
			assert.Equal(t, "TX", r.Location.State, "report %s", r.ID)
			require.NotNil(t, r.Severity, "report %s", r.ID)
			assert.Equal(t, "severe", *r.Severity, "report %s", r.ID)
		}
	})

	t.Run("empty result", func(t *testing.T) {
		f := wideFilter()
		f.Types = []string{"blizzard"}
		reports, count, err := s.ListStormReports(ctx, f)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
		assert.Empty(t, reports)
	})

	t.Run("multiple types filter", func(t *testing.T) {
		f := wideFilter()
		f.Types = []string{"hail", "tornado"}
		_, count, err := s.ListStormReports(ctx, f)
		require.NoError(t, err)
		assert.Equal(t, 20, count)
	})
}

func TestStoreSortingAndPagination(t *testing.T) {
	ctx := context.Background()
	s := setupStoreWithData(ctx, t)

	t.Run("sort by magnitude DESC", func(t *testing.T) {
		f := wideFilter()
		sortBy := model.SortFieldMagnitude
		sortOrder := model.SortOrderDesc
		f.SortBy = &sortBy
		f.SortOrder = &sortOrder
		reports, _, err := s.ListStormReports(ctx, f)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(reports), 2)
		for i := 1; i < len(reports); i++ {
			assert.GreaterOrEqual(t, reports[i-1].Magnitude, reports[i].Magnitude,
				"reports[%d] > reports[%d]", i-1, i)
		}
	})

	t.Run("sort by magnitude ASC", func(t *testing.T) {
		f := wideFilter()
		sortBy := model.SortFieldMagnitude
		sortOrder := model.SortOrderAsc
		f.SortBy = &sortBy
		f.SortOrder = &sortOrder
		reports, _, err := s.ListStormReports(ctx, f)
		require.NoError(t, err)
		for i := 1; i < len(reports); i++ {
			assert.LessOrEqual(t, reports[i-1].Magnitude, reports[i].Magnitude,
				"reports[%d] < reports[%d]", i-1, i)
		}
	})

	t.Run("sort by state ASC", func(t *testing.T) {
		f := wideFilter()
		sortBy := model.SortFieldState
		sortOrder := model.SortOrderAsc
		f.SortBy = &sortBy
		f.SortOrder = &sortOrder
		reports, _, err := s.ListStormReports(ctx, f)
		require.NoError(t, err)
		for i := 1; i < len(reports); i++ {
			assert.LessOrEqual(t, reports[i-1].Location.State, reports[i].Location.State,
				"reports[%d] < reports[%d]", i-1, i)
		}
	})

	t.Run("pagination limit", func(t *testing.T) {
		f := wideFilter()
		limit := 5
		f.Limit = &limit
		reports, totalCount, err := s.ListStormReports(ctx, f)
		require.NoError(t, err)
		assert.Len(t, reports, 5)
		assert.Equal(t, 30, totalCount, "totalCount should ignore limit")
	})

	t.Run("pagination offset", func(t *testing.T) {
		f := wideFilter()
		limit := 5
		f.Limit = &limit

		page1, _, err := s.ListStormReports(ctx, f)
		require.NoError(t, err)

		offset := 5
		f.Offset = &offset
		page2, _, err := s.ListStormReports(ctx, f)
		require.NoError(t, err)
		assert.Len(t, page2, 5)

		for _, r1 := range page1 {
			for _, r2 := range page2 {
				assert.NotEqual(t, r1.ID, r2.ID, "report should not appear on both pages")
			}
		}
	})

	t.Run("offset beyond total", func(t *testing.T) {
		f := wideFilter()
		offset := 100
		f.Offset = &offset
		reports, totalCount, err := s.ListStormReports(ctx, f)
		require.NoError(t, err)
		assert.Empty(t, reports)
		assert.Equal(t, 30, totalCount, "totalCount should still be 30")
	})
}

func TestGraphQLAggregations(t *testing.T) {
	ctx := context.Background()
	s := setupStoreWithData(ctx, t)
	srv := startGraphQLServer(t, s)
	defer srv.Close()

	body := `{"query":"{ stormReports(filter: { beginTimeAfter: \"2020-01-01T00:00:00Z\", beginTimeBefore: \"2030-01-01T00:00:00Z\" }) { totalCount byType { type count maxMagnitude } byState { state count counties { county count } } byHour { bucket count } lastUpdated dataLagMinutes } }"}`

	resp, err := http.Post(srv.URL+"/query", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	var result struct {
		Data struct {
			StormReports struct {
				TotalCount int `json:"totalCount"`
				ByType     []struct {
					Type         string   `json:"type"`
					Count        int      `json:"count"`
					MaxMagnitude *float64 `json:"maxMagnitude"`
				} `json:"byType"`
				ByState []struct {
					State    string `json:"state"`
					Count    int    `json:"count"`
					Counties []struct {
						County string `json:"county"`
						Count  int    `json:"count"`
					} `json:"counties"`
				} `json:"byState"`
				ByHour []struct {
					Bucket string `json:"bucket"`
					Count  int    `json:"count"`
				} `json:"byHour"`
				LastUpdated    *string `json:"lastUpdated"`
				DataLagMinutes *int    `json:"dataLagMinutes"`
			} `json:"stormReports"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	require.Empty(t, result.Errors)

	sr := result.Data.StormReports

	assert.Equal(t, 30, sr.TotalCount)
	assert.Len(t, sr.ByType, 3)
	typeMap := map[string]int{}
	for _, g := range sr.ByType {
		typeMap[g.Type] = g.Count
	}
	assert.Equal(t, 10, typeMap["hail"])
	assert.Equal(t, 10, typeMap["tornado"])
	assert.Equal(t, 10, typeMap["wind"])

	assert.NotEmpty(t, sr.ByState)
	for _, sg := range sr.ByState {
		assert.NotEmpty(t, sg.Counties, "byState %s", sg.State)
	}

	assert.NotEmpty(t, sr.ByHour)
	hourTotal := 0
	for _, g := range sr.ByHour {
		hourTotal += g.Count
	}
	assert.Equal(t, 30, hourTotal)

	assert.NotNil(t, sr.LastUpdated)
	assert.NotNil(t, sr.DataLagMinutes)
}

func TestKafkaConsumerIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	dsn, pg := startPostgres(ctx, t)
	defer func() { _ = pg.Terminate(ctx) }()

	broker, kc := startKafka(ctx, t)
	defer func() { _ = kc.Terminate(ctx) }()

	require.NoError(t, database.RunMigrations(dsn))

	pool, err := database.NewPool(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()

	s := store.New(pool, observability.NewTestMetrics())

	// Create topic
	conn, err := kafkago.Dial("tcp", broker)
	require.NoError(t, err, "dial kafka")
	err = conn.CreateTopics(kafkago.TopicConfig{
		Topic:             "transformed-weather-data",
		NumPartitions:     1,
		ReplicationFactor: 1,
	})
	conn.Close()
	require.NoError(t, err, "create topic")

	// Produce mock messages
	reports := loadMockReports(t)
	writer := &kafkago.Writer{
		Addr:  kafkago.TCP(broker),
		Topic: "transformed-weather-data",
	}
	defer writer.Close()

	var msgs []kafkago.Message
	for i := range reports {
		data, _ := json.Marshal(reports[i])
		msgs = append(msgs, kafkago.Message{Value: data})
	}
	require.NoError(t, writer.WriteMessages(ctx, msgs...))

	// Start consumer
	consumer := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers: []string{broker},
		Topic:   "transformed-weather-data",
		GroupID: "test-group",
	})
	defer consumer.Close()

	consumed := 0
	for consumed < len(reports) {
		msg, err := consumer.ReadMessage(ctx)
		require.NoError(t, err, "read message %d", consumed)
		var report model.StormReport
		require.NoError(t, json.Unmarshal(msg.Value, &report))
		require.NoError(t, s.InsertStormReport(ctx, &report))
		consumed++
	}

	// Verify all records in database
	all, totalCount, err := s.ListStormReports(ctx, wideFilter())
	require.NoError(t, err)
	assert.Len(t, all, 30)
	assert.Equal(t, 30, totalCount)
}

func TestGraphQLEndpoint(t *testing.T) {
	ctx := context.Background()

	dsn, pg := startPostgres(ctx, t)
	defer func() { _ = pg.Terminate(ctx) }()

	require.NoError(t, database.RunMigrations(dsn))

	pool, err := database.NewPool(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()

	s := store.New(pool, observability.NewTestMetrics())

	reports := loadMockReports(t)
	for i := range reports {
		require.NoError(t, s.InsertStormReport(ctx, &reports[i]))
	}

	srv := startGraphQLServer(t, s)
	defer srv.Close()

	body := `{"query":"{ stormReports(filter: { beginTimeAfter: \"2020-01-01T00:00:00Z\", beginTimeBefore: \"2030-01-01T00:00:00Z\" }) { reports { id type magnitude } totalCount } }"}`
	resp, err := http.Post(srv.URL+"/query", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		Data struct {
			StormReports struct {
				Reports []struct {
					ID        string  `json:"id"`
					Type      string  `json:"type"`
					Magnitude float64 `json:"magnitude"`
				} `json:"reports"`
				TotalCount int `json:"totalCount"`
			} `json:"stormReports"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	assert.Len(t, result.Data.StormReports.Reports, 30)
	assert.Equal(t, 30, result.Data.StormReports.TotalCount)

	// Query with filter (hail only)
	body = `{"query":"{ stormReports(filter: { beginTimeAfter: \"2020-01-01T00:00:00Z\", beginTimeBefore: \"2030-01-01T00:00:00Z\", types: [\"hail\"] }) { reports { id type } totalCount } }"}`
	resp2, err := http.Post(srv.URL+"/query", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp2.Body.Close()

	var filtered struct {
		Data struct {
			StormReports struct {
				Reports []struct {
					ID   string `json:"id"`
					Type string `json:"type"`
				} `json:"reports"`
				TotalCount int `json:"totalCount"`
			} `json:"stormReports"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&filtered))
	assert.Len(t, filtered.Data.StormReports.Reports, 10)
	assert.Equal(t, 10, filtered.Data.StormReports.TotalCount)
}
