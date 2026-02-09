package database

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolReadiness wraps a pgxpool.Pool and implements observability.ReadinessChecker.
type PoolReadiness struct {
	pool *pgxpool.Pool
}

// NewPoolReadiness returns a readiness checker backed by the given pool.
func NewPoolReadiness(pool *pgxpool.Pool) *PoolReadiness {
	return &PoolReadiness{pool: pool}
}

// CheckReadiness pings the database to verify connectivity.
func (p *PoolReadiness) CheckReadiness(ctx context.Context) error {
	return p.pool.Ping(ctx)
}
