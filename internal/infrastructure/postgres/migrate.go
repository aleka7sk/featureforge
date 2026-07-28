package postgres

import (
	"context"
	"embed"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// migrationFS holds the schema as data rather than as a script a developer
// must remember to run. No migration framework is used: one embedded
// directory and a version table are the whole mechanism (AD-020).
//
//go:embed migrations/*.sql
var migrationFS embed.FS

// migration is one versioned schema change, named NNNN_description.sql.
type migration struct {
	version int64
	name    string
	body    string
}

// Migrate applies every migration not yet recorded in schema_migrations, in
// version order. It is idempotent and creates schema_migrations itself, so it
// is safe to call on an empty database, on every process start, and at the
// top of every integration test -- a developer never creates a schema or a
// table by hand.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
        version    bigint      PRIMARY KEY,
        applied_at timestamptz NOT NULL DEFAULT now()
    )`); err != nil {
		return fmt.Errorf("postgres: creating schema_migrations: %w", err)
	}

	applied, err := appliedVersions(ctx, pool)
	if err != nil {
		return err
	}
	migrations, err := loadMigrations()
	if err != nil {
		return err
	}

	for _, m := range migrations {
		if applied[m.version] {
			continue
		}
		if err := applyMigration(ctx, pool, m); err != nil {
			return err
		}
	}
	return nil
}

// applyMigration runs one migration's statements and records its version in
// the same transaction, so a partially-applied migration can never be
// recorded as complete.
func applyMigration(ctx context.Context, pool *pgxpool.Pool, m migration) (err error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: beginning migration %d: %w", m.version, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(context.Background())
		}
	}()

	if _, err := tx.Exec(ctx, m.body); err != nil {
		return fmt.Errorf("postgres: applying migration %d (%s): %w", m.version, m.name, err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, m.version); err != nil {
		return fmt.Errorf("postgres: recording migration %d: %w", m.version, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: committing migration %d: %w", m.version, err)
	}
	committed = true
	return nil
}

// appliedVersions reads the set of versions already recorded.
func appliedVersions(ctx context.Context, pool *pgxpool.Pool) (map[int64]bool, error) {
	rows, err := pool.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("postgres: reading schema_migrations: %w", err)
	}
	defer rows.Close()

	applied := map[int64]bool{}
	for rows.Next() {
		var version int64
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("postgres: scanning schema_migrations: %w", err)
		}
		applied[version] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterating schema_migrations: %w", err)
	}
	return applied, nil
}

// loadMigrations reads every embedded migration, sorted by version. A file
// whose name does not begin with a numeric version is an error rather than a
// silently skipped file.
func loadMigrations() ([]migration, error) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("postgres: reading embedded migrations: %w", err)
	}

	out := make([]migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		version, err := versionOf(entry.Name())
		if err != nil {
			return nil, err
		}
		body, err := migrationFS.ReadFile(path.Join("migrations", entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("postgres: reading migration %s: %w", entry.Name(), err)
		}
		out = append(out, migration{version: version, name: entry.Name(), body: string(body)})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	for i := 1; i < len(out); i++ {
		if out[i].version == out[i-1].version {
			return nil, fmt.Errorf("postgres: migrations %s and %s share version %d", out[i-1].name, out[i].name, out[i].version)
		}
	}
	return out, nil
}

// versionOf extracts the leading numeric version from NNNN_description.sql.
func versionOf(filename string) (int64, error) {
	base, _, found := strings.Cut(filename, "_")
	if !found {
		return 0, fmt.Errorf("postgres: migration %s must be named NNNN_description.sql", filename)
	}
	version, err := strconv.ParseInt(base, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("postgres: migration %s has a non-numeric version prefix: %w", filename, err)
	}
	return version, nil
}
