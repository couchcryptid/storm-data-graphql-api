package graph

import (
	"context"
	"math"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/couchcryptid/storm-data-graphql-api/internal/model"
	"github.com/couchcryptid/storm-data-graphql-api/internal/store"
)

// collectFields returns the set of field names requested on StormReportsResult,
// including dotted paths for nested fields (e.g. "aggregations.byEventType").
func collectFields(ctx context.Context) map[string]bool {
	collected := graphql.CollectFieldsCtx(ctx, nil)
	fields := make(map[string]bool, len(collected))
	for _, f := range collected {
		fields[f.Name] = true
		for _, child := range graphql.CollectFields(graphql.GetOperationContext(ctx), f.Selections, nil) {
			fields[f.Name+"."+child.Name] = true
		}
	}
	return fields
}

// applyMeta fetches and assigns lastUpdated and dataLagMinutes to the QueryMeta.
func applyMeta(ctx context.Context, s *store.Store, meta *model.QueryMeta) error {
	lastUpdated, err := s.LastUpdated(ctx)
	if err != nil {
		return err
	}
	meta.LastUpdated = lastUpdated
	if lastUpdated != nil {
		lag := int(math.Round(time.Since(*lastUpdated).Minutes()))
		meta.DataLagMinutes = &lag
	}
	return nil
}
