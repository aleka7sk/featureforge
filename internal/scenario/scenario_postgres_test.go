package scenario_test

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

// newPostgresFixture returns the same three collaborators newFixture returns,
// but backed by a real PostgreSQL schema of its own. The scenario driver and
// every fixture it uses are unchanged: only the UnitOfWork differs, which is
// the entire point of the milestone.
func newPostgresFixture(t *testing.T) (application.UnitOfWork, peos.Recorder, *application.FixedClock) {
	t.Helper()
	dsn := os.Getenv(postgresDSNEnvVar)
	if dsn == "" {
		t.Skipf("%s is not set; run `make postgres-test` to run PostgreSQL integration tests", postgresDSNEnvVar)
	}
	ctx := context.Background()
	schema := fmt.Sprintf("ff_scenario_%d", postgresSchemaCounter.Add(1))

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

// TestCanonicalScenarioPostgres runs the identical canonical scenario against
// PostgreSQL and asserts the identical end state, using the same
// scenario.Run driver and the same assertion body as the in-memory test.
func TestCanonicalScenarioPostgres(t *testing.T) {
	ctx := context.Background()
	uow, rec, clock := newPostgresFixture(t)

	result, err := scenario.Run(ctx, uow, rec, rec, clock)
	if err != nil {
		t.Fatalf("scenario.Run against PostgreSQL: %v", err)
	}
	assertCanonicalEndState(t, ctx, uow, rec, result)
}

// TestCanonicalScenarioReplayPostgres runs the exact FF-011 command stream a
// second time against the same PostgreSQL UnitOfWork and advancing clock. The
// shared write gate rejects every repository mutator during replay, while the
// shared closed-world snapshot proves every persisted byte remains unchanged.
func TestCanonicalScenarioReplayPostgres(t *testing.T) {
	uow, rec, clock := newPostgresFixture(t)
	assertCanonicalScenarioReplay(t, context.Background(), uow, rec, clock)
}

// TestCanonicalScenarioPostgresInsertionOrderIndependence proves the
// PostgreSQL adapter resolves the same answers regardless of the order the
// independent acts were recorded in -- so the determinism FF-004 §2 requires
// is a property of the resolution algorithms, not of a particular store's
// iteration order.
func TestCanonicalScenarioPostgresInsertionOrderIndependence(t *testing.T) {
	ctx := context.Background()

	uowA, recA, clockA := newPostgresFixture(t)
	if _, err := scenario.Run(ctx, uowA, recA, recA, clockA); err != nil {
		t.Fatalf("scenario.Run (default order): %v", err)
	}
	uowB, recB, clockB := newPostgresFixture(t)
	if _, err := scenario.RunPermuted(ctx, uowB, recB, recB, clockB); err != nil {
		t.Fatalf("scenario.RunPermuted: %v", err)
	}

	assertSameResolvedState(t, ctx, uowA, uowB)
}
