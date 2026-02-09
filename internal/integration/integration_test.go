//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/couchcryptid/storm-data-graphql-api/internal/database"
	"github.com/couchcryptid/storm-data-graphql-api/internal/graph"
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

const (
	testReportMsg  = "report %s"
	testKafkaTopic = "transformed-weather-data"
	contentJSON    = "application/json"
	graphQLPath    = "/query"
)

// discardLogger returns a logger that discards all output.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// wideFilter returns a filter with a time window that covers all mock data.
func wideFilter() *model.StormReportFilter {
	return &model.StormReportFilter{
		TimeRange: model.TimeRange{
			From: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
			To:   time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
		},
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

	// Filter by event type
	f := wideFilter()
	f.EventTypes = []model.EventType{model.EventTypeHail}
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
		assert.Equal(t, "TX", r.Location.State, testReportMsg, r.ID)
	}

	// Filter by geo radius (around Fort Worth, TX area)
	f = wideFilter()
	radius := 20.0
	f.Near = &model.GeoRadiusFilter{
		Lat:         32.75,
		Lon:         -97.15,
		RadiusMiles: &radius,
	}
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

	t.Run("Aggregations CTE", func(t *testing.T) {
		agg, err := s.Aggregations(ctx, wideFilter())
		require.NoError(t, err)

		// ByEventType
		require.Len(t, agg.ByEventType, 3)
		typeMap := map[string]int{}
		for _, g := range agg.ByEventType {
			typeMap[g.EventType] = g.Count
		}
		assert.Equal(t, 10, typeMap["hail"])
		assert.Equal(t, 10, typeMap["tornado"])
		assert.Equal(t, 10, typeMap["wind"])
		assertEventTypeMaxMeasurement(t, agg.ByEventType, "hail", 1.75, "in")

		// ByState
		require.NotEmpty(t, agg.ByState)
		stateCount := map[string]int{}
		for _, g := range agg.ByState {
			stateCount[g.State] = g.Count
		}
		assert.NotZero(t, stateCount["TX"])
		assert.NotZero(t, stateCount["NE"])
		assertStateCountyTotals(t, agg.ByState, "TX")

		// ByHour
		require.NotEmpty(t, agg.ByHour)
		hourTotal := 0
		for _, g := range agg.ByHour {
			hourTotal += g.Count
			assert.False(t, g.Bucket.IsZero(), "bucket should not be zero")
		}
		assert.Equal(t, 30, hourTotal)
	})

	t.Run("LastUpdated", func(t *testing.T) {
		ts, err := s.LastUpdated(ctx)
		require.NoError(t, err)
		require.NotNil(t, ts)
		assert.False(t, ts.IsZero())
	})

	t.Run("Aggregations with event type filter", func(t *testing.T) {
		f := wideFilter()
		f.EventTypes = []model.EventType{model.EventTypeHail}
		agg, err := s.Aggregations(ctx, f)
		require.NoError(t, err)
		require.Len(t, agg.ByEventType, 1)
		assert.Equal(t, "hail", agg.ByEventType[0].EventType)
		assert.Equal(t, 10, agg.ByEventType[0].Count)
	})
}

func TestStoreFilters(t *testing.T) {
	ctx := context.Background()
	s := setupStoreWithData(ctx, t)

	t.Run("severity filter", func(t *testing.T) {
		f := wideFilter()
		f.Severity = []model.Severity{model.SeveritySevere}
		reports, count, err := s.ListStormReports(ctx, f)
		require.NoError(t, err)
		assert.Equal(t, 4, count)
		for _, r := range reports {
			require.NotNil(t, r.Severity, testReportMsg, r.ID)
			assert.Equal(t, "severe", *r.Severity, testReportMsg, r.ID)
		}
	})

	t.Run("multiple severities", func(t *testing.T) {
		f := wideFilter()
		f.Severity = []model.Severity{model.SeveritySevere, model.SeverityModerate}
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
			assert.Equal(t, "Tarrant", r.Location.County, testReportMsg, r.ID)
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
			assert.GreaterOrEqual(t, r.Magnitude, 1.75, testReportMsg, r.ID)
		}
	})

	t.Run("combined filters", func(t *testing.T) {
		f := wideFilter()
		f.EventTypes = []model.EventType{model.EventTypeHail}
		f.States = []string{"TX"}
		f.Severity = []model.Severity{model.SeveritySevere}
		reports, count, err := s.ListStormReports(ctx, f)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
		for _, r := range reports {
			assert.Equal(t, "hail", r.Type, testReportMsg, r.ID)
			assert.Equal(t, "TX", r.Location.State, testReportMsg, r.ID)
			require.NotNil(t, r.Severity, testReportMsg, r.ID)
			assert.Equal(t, "severe", *r.Severity, testReportMsg, r.ID)
		}
	})

	t.Run("empty result", func(t *testing.T) {
		f := wideFilter()
		f.EventTypes = []model.EventType{model.EventType("BLIZZARD")}
		reports, count, err := s.ListStormReports(ctx, f)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
		assert.Empty(t, reports)
	})

	t.Run("multiple event types filter", func(t *testing.T) {
		f := wideFilter()
		f.EventTypes = []model.EventType{model.EventTypeHail, model.EventTypeTornado}
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
		sortBy := model.SortFieldLocationState
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

	body := `{"query":"{ stormReports(filter: { timeRange: { from: \"2020-01-01T00:00:00Z\", to: \"2030-01-01T00:00:00Z\" } }) { totalCount aggregations { totalCount byEventType { eventType count maxMeasurement { magnitude unit } } byState { state count counties { county count } } byHour { bucket count } } meta { lastUpdated dataLagMinutes } } }"}`

	resp, err := http.Post(srv.URL+graphQLPath, contentJSON, strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	var result struct {
		Data struct {
			StormReports struct {
				TotalCount   int `json:"totalCount"`
				Aggregations struct {
					TotalCount  int `json:"totalCount"`
					ByEventType []struct {
						EventType      string `json:"eventType"`
						Count          int    `json:"count"`
						MaxMeasurement *struct {
							Magnitude float64 `json:"magnitude"`
							Unit      string  `json:"unit"`
						} `json:"maxMeasurement"`
					} `json:"byEventType"`
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
				} `json:"aggregations"`
				Meta struct {
					LastUpdated    *string `json:"lastUpdated"`
					DataLagMinutes *int    `json:"dataLagMinutes"`
				} `json:"meta"`
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

	agg := sr.Aggregations
	assert.Equal(t, 30, agg.TotalCount)
	assert.Len(t, agg.ByEventType, 3)
	typeMap := map[string]int{}
	for _, g := range agg.ByEventType {
		typeMap[g.EventType] = g.Count
	}
	assert.Equal(t, 10, typeMap["hail"])
	assert.Equal(t, 10, typeMap["tornado"])
	assert.Equal(t, 10, typeMap["wind"])

	assert.NotEmpty(t, agg.ByState)
	for _, sg := range agg.ByState {
		assert.NotEmpty(t, sg.Counties, "byState %s", sg.State)
	}

	assert.NotEmpty(t, agg.ByHour)
	hourTotal := 0
	for _, g := range agg.ByHour {
		hourTotal += g.Count
	}
	assert.Equal(t, 30, hourTotal)

	assert.NotNil(t, sr.Meta.LastUpdated)
	assert.NotNil(t, sr.Meta.DataLagMinutes)
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
		Topic:             testKafkaTopic,
		NumPartitions:     1,
		ReplicationFactor: 1,
	})
	conn.Close()
	require.NoError(t, err, "create topic")

	// Produce mock messages
	reports := loadMockReports(t)
	writer := &kafkago.Writer{
		Addr:  kafkago.TCP(broker),
		Topic: testKafkaTopic,
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
		Topic:   testKafkaTopic,
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

	body := `{"query":"{ stormReports(filter: { timeRange: { from: \"2020-01-01T00:00:00Z\", to: \"2030-01-01T00:00:00Z\" } }) { reports { id eventType measurement { magnitude } } totalCount } }"}`
	resp, err := http.Post(srv.URL+graphQLPath, contentJSON, strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		Data struct {
			StormReports struct {
				Reports []struct {
					ID          string `json:"id"`
					EventType   string `json:"eventType"`
					Measurement struct {
						Magnitude float64 `json:"magnitude"`
					} `json:"measurement"`
				} `json:"reports"`
				TotalCount int `json:"totalCount"`
			} `json:"stormReports"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	assert.Len(t, result.Data.StormReports.Reports, 20)
	assert.Equal(t, 30, result.Data.StormReports.TotalCount)

	// Query with event type filter (hail only)
	body = `{"query":"{ stormReports(filter: { timeRange: { from: \"2020-01-01T00:00:00Z\", to: \"2030-01-01T00:00:00Z\" }, eventTypes: [HAIL] }) { reports { id eventType } totalCount } }"}`
	resp2, err := http.Post(srv.URL+graphQLPath, contentJSON, strings.NewReader(body))
	require.NoError(t, err)
	defer resp2.Body.Close()

	var filtered struct {
		Data struct {
			StormReports struct {
				Reports []struct {
					ID        string `json:"id"`
					EventType string `json:"eventType"`
				} `json:"reports"`
				TotalCount int `json:"totalCount"`
			} `json:"stormReports"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&filtered))
	assert.Len(t, filtered.Data.StormReports.Reports, 10)
	assert.Equal(t, 10, filtered.Data.StormReports.TotalCount)
}

func TestGraphQLDepthExceeded(t *testing.T) {
	ctx := context.Background()
	s := setupStoreWithData(ctx, t)

	// Create server with low depth limit to test rejection.
	srv := handler.NewDefaultServer(graph.NewExecutableSchema(graph.Config{
		Resolvers:  &graph.Resolver{Store: s},
		Complexity: graph.NewComplexityRoot(),
	}))
	srv.Use(extension.FixedComplexityLimit(600))
	srv.Use(graph.DepthLimit{MaxDepth: 3})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// Depth 4: stormReports(1) > reports(2) > measurement(3) > magnitude(4)
	body := `{"query":"{ stormReports(filter: { timeRange: { from: \"2020-01-01T00:00:00Z\", to: \"2030-01-01T00:00:00Z\" } }) { reports { measurement { magnitude } } } }"}`
	resp, err := http.Post(ts.URL+graphQLPath, contentJSON, strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	var result struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	require.NotEmpty(t, result.Errors, "expected depth limit error")
	assert.Contains(t, result.Errors[0].Message, "exceeds maximum allowed depth")
}
