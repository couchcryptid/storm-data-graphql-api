//go:build integration

package integration_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/couchcryptid/storm-data-graphql-api/internal/database"
	"github.com/couchcryptid/storm-data-graphql-api/internal/graph"
	"github.com/couchcryptid/storm-data-graphql-api/internal/model"
	"github.com/couchcryptid/storm-data-graphql-api/internal/observability"
	"github.com/couchcryptid/storm-data-graphql-api/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func startGraphQLServer(t *testing.T, s *store.Store) *httptest.Server {
	t.Helper()
	srv := handler.NewDefaultServer(graph.NewExecutableSchema(graph.Config{
		Resolvers:  &graph.Resolver{Store: s},
		Complexity: graph.NewComplexityRoot(),
	}))
	srv.Use(extension.FixedComplexityLimit(500))
	srv.Use(graph.DepthLimit{MaxDepth: 7})
	return httptest.NewServer(srv)
}

// assertTypeMaxMagnitude finds the given type in groups and asserts its max magnitude.
func assertTypeMaxMagnitude(t *testing.T, groups []*model.TypeGroup, typ string, expected float64) {
	t.Helper()
	for _, g := range groups {
		if g.Type == typ {
			require.NotNil(t, g.MaxMagnitude)
			assert.Equal(t, expected, *g.MaxMagnitude)
			return
		}
	}
	t.Fatalf("type %s not found in groups", typ)
}

// assertStateCountyTotals finds the given state in groups and asserts its county counts sum correctly.
func assertStateCountyTotals(t *testing.T, groups []*model.StateGroup, state string) {
	t.Helper()
	for _, g := range groups {
		if g.State == state {
			assert.NotEmpty(t, g.Counties, "%s should have counties", state)
			countyTotal := 0
			for _, c := range g.Counties {
				countyTotal += c.Count
			}
			assert.Equal(t, g.Count, countyTotal, "%s county sum should equal state count", state)
			return
		}
	}
	t.Fatalf("state %s not found in groups", state)
}

// setupStoreWithData starts Postgres, runs migrations, inserts all mock data,
// and registers cleanup. Returns the populated store.
func setupStoreWithData(ctx context.Context, t *testing.T) *store.Store {
	t.Helper()
	dsn, pg := startPostgres(ctx, t)
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	require.NoError(t, database.RunMigrations(dsn))

	pool, err := database.NewPool(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	s := store.New(pool, observability.NewTestMetrics())
	reports := loadMockReports(t)
	for i := range reports {
		require.NoError(t, s.InsertStormReport(ctx, &reports[i]), "insert report %s", reports[i].ID)
	}
	return s
}
