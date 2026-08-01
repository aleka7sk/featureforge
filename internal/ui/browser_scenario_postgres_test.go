package ui_test

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
	"github.com/aleka7sk/featureforge/internal/testsupport/replaygate"
)

const postgresDSNEnvVar = "FEATUREFORGE_POSTGRES_TEST_DSN"

var postgresSchemaCounter atomic.Int64

// newPostgresFixtureUI mirrors internal/transport/http's own
// newPostgresFixtureHTTP: a real PostgreSQL schema of its own, isolated
// per test, dropped on cleanup. That helper is unexported in a different
// package, so it is reproduced here rather than imported (same rationale
// as internal/transport/http/scenario_http_postgres_test.go's own doc
// comment: FF-018 does not authorize cross-package test helper sharing).
func newPostgresFixtureUI(t *testing.T) (application.UnitOfWork, peos.Recorder) {
	t.Helper()
	dsn := os.Getenv(postgresDSNEnvVar)
	if dsn == "" {
		t.Skipf("%s is not set; run `make postgres-test` to run PostgreSQL integration tests", postgresDSNEnvVar)
	}
	ctx := context.Background()
	schema := fmt.Sprintf("ff_ui_browser_%d", postgresSchemaCounter.Add(1))

	admin, err := postgres.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	defer admin.Close()
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

	return postgres.NewUnitOfWork(pool), peos.NewRecorder()
}

// TestCanonicalScenarioThroughUIBrowserPostgres runs the identical
// browser-driven scenario TestCanonicalScenarioThroughUIBrowser drives
// against memory, this time against PostgreSQL, asserting the identical
// end state (FF-021 §14, mirroring FF-018 §16 step 10's adapter-parity
// pattern). Skips cleanly when FEATUREFORGE_POSTGRES_TEST_DSN is unset.
func TestCanonicalScenarioThroughUIBrowserPostgres(t *testing.T) {
	uow, rec := newPostgresFixtureUI(t)
	clock := application.NewFixedClock(scenario.FixedStart)
	assertCanonicalScenarioUIReplay(t, replaygate.New(uow), rec, clock)
}
