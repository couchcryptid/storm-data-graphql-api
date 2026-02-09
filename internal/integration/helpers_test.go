//go:build integration

package integration

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/couchcryptid/storm-data-graphql-api/internal/database"
	"github.com/couchcryptid/storm-data-graphql-api/internal/graph"
	"github.com/couchcryptid/storm-data-graphql-api/internal/observability"
	"github.com/couchcryptid/storm-data-graphql-api/internal/store"
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
