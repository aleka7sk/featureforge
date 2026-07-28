package postgres_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/infrastructure/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

// dsnEnvVar names the connection string for the PostgreSQL instance the
// integration tests run against. When it is unset every PostgreSQL test skips,
// so plain `go test ./...` stays meaningful without Docker -- unlike a build
// tag, which would silently not compile these files at all.
const dsnEnvVar = "FEATUREFORGE_POSTGRES_TEST_DSN"

// schemaCounter makes each test schema name unique within a process without
// needing a clock or randomness.
var schemaCounter atomic.Int64

// requireDSN returns the configured DSN or skips the calling test. It must be
// called from the test function itself, never from inside a subtest closure
// passed to the contract suite: t.Skip on an outer, captured T from another
// goroutine is a Go testing bug.
func requireDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv(dsnEnvVar)
	if dsn == "" {
		t.Skipf("%s is not set; run `make postgres-test` to run PostgreSQL integration tests", dsnEnvVar)
	}
	return dsn
}

// newIsolatedPool creates a uniquely named schema, migrates it, and returns a
// pool whose search_path is scoped to it. Registering cleanup on t means each
// test gets a database that starts empty and is dropped afterwards, without
// depending on TRUNCATE ordering across the foreign-key graph.
//
// It panics rather than calling t.Fatal on failure, because the contract
// suite invokes its factory from inside subtest goroutines: a panic is
// attributed to the subtest that caused it, whereas t.Fatal on a captured
// outer T is not.
func newIsolatedPool(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	schema := fmt.Sprintf("ff_test_%d", schemaCounter.Add(1))

	admin, err := postgres.Connect(ctx, dsn)
	if err != nil {
		panic(fmt.Sprintf("postgres test harness: connecting: %v", err))
	}
	defer admin.Close()
	// Drop first: a previous run that panicked before registering cleanup can
	// leave its schema behind, and the counter restarts at 1 each process.
	if _, err := admin.Exec(ctx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE"); err != nil {
		panic(fmt.Sprintf("postgres test harness: dropping stale schema %s: %v", schema, err))
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		panic(fmt.Sprintf("postgres test harness: creating schema %s: %v", schema, err))
	}

	pool, err := postgres.Connect(ctx, withSearchPath(dsn, schema))
	if err != nil {
		panic(fmt.Sprintf("postgres test harness: connecting to schema %s: %v", schema, err))
	}
	if err := postgres.Migrate(ctx, pool); err != nil {
		panic(fmt.Sprintf("postgres test harness: migrating schema %s: %v", schema, err))
	}

	t.Cleanup(func() {
		pool.Close()
		cleanup, err := postgres.Connect(context.Background(), dsn)
		if err != nil {
			return
		}
		defer cleanup.Close()
		_, _ = cleanup.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
	})
	return pool
}

// newIsolatedUOW is the factory the shared contract suite calls.
func newIsolatedUOW(t *testing.T, dsn string) application.UnitOfWork {
	return postgres.NewUnitOfWork(newIsolatedPool(t, dsn))
}

// withSearchPath adds a search_path parameter to a DSN so every statement
// this pool issues resolves against the test's own schema.
func withSearchPath(dsn, schema string) string {
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	return dsn + separator + "search_path=" + schema
}
