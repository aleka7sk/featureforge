// Package postgres is the PostgreSQL persistence adapter. It implements the
// same application.Repositories and application.UnitOfWork contracts the
// in-memory adapter does, and it is the only package in this module that
// imports a database driver (AD-020, enforced by an architecture test).
//
// No PostgreSQL type crosses this package's boundary: every exported
// signature is expressed in application, domain, and engineering types.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens a connection pool against dsn and verifies it is reachable.
// The caller owns the returned pool and must Close it.
func Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: parsing dsn: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("postgres: opening pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: pinging: %w", err)
	}
	return pool, nil
}
