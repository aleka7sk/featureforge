package http_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aleka7sk/featureforge/internal/application"
	peos "github.com/aleka7sk/featureforge/internal/engineering/peos"
	"github.com/aleka7sk/featureforge/internal/infrastructure/postgres"
	"github.com/aleka7sk/featureforge/internal/scenario"
)

const postgresDSNEnvVar = "FEATUREFORGE_POSTGRES_TEST_DSN"

var postgresSchemaCounter atomic.Int64

// newPostgresFixtureHTTP mirrors internal/scenario/scenario_postgres_test.go's
// newPostgresFixture: a real PostgreSQL schema of its own, isolated per test,
// dropped on cleanup. FF-018 does not authorize sharing that unexported
// helper across packages, so it is reproduced here rather than imported.
func newPostgresFixtureHTTP(t *testing.T) (application.UnitOfWork, peos.Recorder, *application.FixedClock) {
	t.Helper()
	dsn := os.Getenv(postgresDSNEnvVar)
	if dsn == "" {
		t.Skipf("%s is not set; run `make postgres-test` to run PostgreSQL integration tests", postgresDSNEnvVar)
	}
	ctx := context.Background()
	schema := fmt.Sprintf("ff_scenario_http_%d", postgresSchemaCounter.Add(1))

	admin, err := postgres.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	defer admin.Close()
	// A previous run that failed before cleanup can leave its schema behind,
	// and the counter restarts at 1 in each process.
	if _, err := admin.Exec(ctx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE"); err != nil {
		t.Fatalf("dropping stale schema %s: %v", schema, err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("creating schema %s: %v", schema, err)
	}

	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	pool, err := postgres.Connect(ctx, dsn+separator+"search_path="+schema)
	if err != nil {
		t.Fatalf("connecting to schema %s: %v", schema, err)
	}
	if err := postgres.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrating schema %s: %v", schema, err)
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

	return postgres.NewUnitOfWork(pool), peos.NewRecorder(), application.NewFixedClock(scenario.FixedStart)
}

// TestCanonicalScenarioThroughHTTPPostgres runs the identical scenario
// runScenarioThroughHTTP drives against a memory-backed handler, this time
// against PostgreSQL, and asserts the identical end state (FF-018 §16
// step 10). Skips cleanly when FEATUREFORGE_POSTGRES_TEST_DSN is unset,
// matching the gating convention every other PostgreSQL-backed test in
// this module uses.
func TestCanonicalScenarioThroughHTTPPostgres(t *testing.T) {
	ctx := context.Background()
	uow, rec, clock := newPostgresFixtureHTTP(t)
	handler := runScenarioThroughHTTP(t, ctx, uow, rec, clock)
	assertCanonicalEndStateThroughHTTP(t, ctx, handler, uow, rec)
}
