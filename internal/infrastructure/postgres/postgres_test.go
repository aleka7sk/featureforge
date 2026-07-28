package postgres_test

import (
	"context"
	"testing"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/infrastructure/contracttest"
	"github.com/aleka7sk/featureforge/internal/infrastructure/postgres"
)

// TestPostgresRepositoryContractSuite runs the identical suite the in-memory
// adapter runs. Both adapters passing the same test body, with no
// PostgreSQL-specific weakening, is what makes persistence-independence a
// verified claim rather than an assertion.
func TestPostgresRepositoryContractSuite(t *testing.T) {
	dsn := requireDSN(t)
	contracttest.RunRepositoryContractSuite(t, func() application.UnitOfWork {
		return newIsolatedUOW(t, dsn)
	})
}

// TestMigrateIsIdempotent asserts a second Migrate on an already-migrated
// database is a no-op, which is what lets every process start and every test
// setup call it unconditionally.
func TestMigrateIsIdempotent(t *testing.T) {
	dsn := requireDSN(t)
	pool := newIsolatedPool(t, dsn) // already migrated once
	ctx := context.Background()

	if err := postgres.Migrate(ctx, pool); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if err := postgres.Migrate(ctx, pool); err != nil {
		t.Fatalf("third Migrate: %v", err)
	}

	var applied int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if applied != 1 {
		t.Errorf("schema_migrations holds %d rows after three Migrate calls, want 1", applied)
	}
}

// TestMigrateCreatesEveryTable asserts the schema a fresh database ends up
// with is the one the adapter actually queries.
func TestMigrateCreatesEveryTable(t *testing.T) {
	dsn := requireDSN(t)
	pool := newIsolatedPool(t, dsn)
	ctx := context.Background()

	want := []string{
		"artifact_envelopes", "feature_card_capability_links", "feature_cards",
		"projects", "record_envelopes", "revision_acceptance", "revision_envelopes",
		"revision_order", "schema_migrations", "structured_content",
	}
	for _, table := range want {
		var exists bool
		err := pool.QueryRow(ctx, `
            SELECT true FROM information_schema.tables
            WHERE table_name = $1 AND table_schema = current_schema()`, table).Scan(&exists)
		if err != nil || !exists {
			t.Errorf("table %s missing after migration (err=%v)", table, err)
		}
	}
}
