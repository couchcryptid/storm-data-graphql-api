package graph

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"
)

// DepthLimit rejects queries that exceed a maximum selection-set nesting depth.
type DepthLimit struct {
	MaxDepth int
}

var _ interface {
	graphql.HandlerExtension
	graphql.OperationInterceptor
} = DepthLimit{}

// ExtensionName implements graphql.HandlerExtension.
func (d DepthLimit) ExtensionName() string {
	return "DepthLimit"
}

// Validate implements graphql.HandlerExtension.
func (d DepthLimit) Validate(graphql.ExecutableSchema) error {
	if d.MaxDepth < 1 {
		return fmt.Errorf("DepthLimit: MaxDepth must be >= 1")
	}
	return nil
}

// InterceptOperation implements graphql.OperationInterceptor.
func (d DepthLimit) InterceptOperation(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
	oc := graphql.GetOperationContext(ctx)
	// Skip depth check for introspection queries
	if isIntrospectionQuery(oc.Operation.SelectionSet) {
		return next(ctx)
	}
	depth := queryDepth(oc.Operation.SelectionSet)
	if depth > d.MaxDepth {
		return func(ctx context.Context) *graphql.Response {
			return graphql.ErrorResponse(ctx, "query depth %d exceeds maximum allowed depth of %d", depth, d.MaxDepth)
		}
	}
	return next(ctx)
}

// queryDepth computes the deepest nesting level in a selection set.
func queryDepth(selSet ast.SelectionSet) int {
	if len(selSet) == 0 {
		return 0
	}
	maxChild := 0
	for _, sel := range selSet {
		var childDepth int
		switch s := sel.(type) {
		case *ast.Field:
			childDepth = queryDepth(s.SelectionSet)
		case *ast.InlineFragment:
			childDepth = queryDepth(s.SelectionSet)
		case *ast.FragmentSpread:
			if s.Definition != nil {
				childDepth = queryDepth(s.Definition.SelectionSet)
			}
		}
		if childDepth > maxChild {
			maxChild = childDepth
		}
	}
	return 1 + maxChild
}

// isIntrospectionQuery checks if the query is an introspection query.
// Introspection queries access schema metadata via fields starting with "__".
func isIntrospectionQuery(selSet ast.SelectionSet) bool {
	for _, sel := range selSet {
		if field, ok := sel.(*ast.Field); ok {
			// Check if field name starts with "__" (introspection fields)
			if len(field.Name) >= 2 && field.Name[0] == '_' && field.Name[1] == '_' {
				return true
			}
		}
	}
	return false
}
