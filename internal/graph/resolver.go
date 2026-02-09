package graph

import "github.com/couchcryptid/storm-data-graphql-api/internal/store"

//go:generate go run github.com/99designs/gqlgen generate

// Resolver is the root resolver for the GraphQL schema.
type Resolver struct {
	Store *store.Store
}
