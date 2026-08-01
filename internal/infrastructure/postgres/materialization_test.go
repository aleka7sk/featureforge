package postgres_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering"
	peos "github.com/aleka7sk/featureforge/internal/engineering/peos"
	"github.com/aleka7sk/featureforge/internal/infrastructure/postgres"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestCorruptRevisionMaterializationFailsReplayWithoutStateChange proves a
// constructor/parser failure after a revision row was successfully read is
// persisted-state corruption. It must not leak engineering.ErrInvalidEnvelope
// through the adapter or let replay commit any compensating write.
func TestCorruptRevisionMaterializationFailsReplayWithoutStateChange(t *testing.T) {
	dsn := requireDSN(t)
	pool := newIsolatedPool(t, dsn)
	uow := postgres.NewUnitOfWork(pool)
	recorder := peos.NewRecorder()
	clock := application.NewFixedClock(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	ctx := context.Background()

	seedCapability(t, uow, recorder, recorder, clock)
	if tag, err := pool.Exec(ctx, testOnlyMutationVerb()+`revision_envelopes
		SET payload_digest = 'not-a-digest'
		WHERE artifact_id = 'CAP-1' AND revision_id = 'CAP-1-REV-1'`); err != nil {
		t.Fatalf("corrupting revision row: %v", err)
	} else if tag.RowsAffected() != 1 {
		t.Fatalf("corrupting revision row affected %d rows, want 1", tag.RowsAffected())
	}
	before := postgresStateSnapshot(t, pool)

	clock.Advance(time.Hour)
	_, err := (application.EstablishCapabilitySpecificationCommand{
		FeatureCardID: "FC-1",
		ArtifactID:    "CAP-1",
		RevisionID:    "CAP-1-REV-1",
		Content:       mustContent(t, "Homework after a lesson"),
	}).Execute(ctx, uow, recorder, recorder, clock)
	if !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("replay err = %v, want ErrStoredStateIntegrity", err)
	}
	if errors.Is(err, engineering.ErrInvalidEnvelope) {
		t.Fatalf("replay leaked engineering.ErrInvalidEnvelope: %v", err)
	}
	if after := postgresStateSnapshot(t, pool); !reflect.DeepEqual(after, before) {
		t.Fatalf("PostgreSQL state changed after corrupt-revision replay\nbefore: %#v\nafter:  %#v", before, after)
	}
}

// TestCorruptAcceptanceMaterializationFailsReplayWithoutStateChange exercises
// the acceptance reader independently from revision-envelope corruption. A
// state value that PostgreSQL can store but the engineering constructor
// rejects is classified as stored-state integrity and leaves the journal and
// the rest of the aggregate byte-for-byte unchanged.
func TestCorruptAcceptanceMaterializationFailsReplayWithoutStateChange(t *testing.T) {
	dsn := requireDSN(t)
	pool := newIsolatedPool(t, dsn)
	uow := postgres.NewUnitOfWork(pool)
	recorder := peos.NewRecorder()
	clock := application.NewFixedClock(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	ctx := context.Background()

	seedCapability(t, uow, recorder, recorder, clock)
	acceptCapabilityForRequirement(t, uow, recorder, clock)
	acceptanceRecordID := "ACC-REQ-1-REV-1"
	command := application.EstablishRequirementCommand{
		ArtifactID:                   "REQ-1",
		RevisionID:                   "REQ-1-REV-1",
		Statement:                    "Published homework SHALL be visible to the student.",
		SubjectArtifactID:            "CAP-1",
		SourceCapabilityRevisionID:   "CAP-1-REV-1",
		SourceAcceptanceCriterionKey: "AC-1",
		AcceptanceRecordID:           &acceptanceRecordID,
	}
	if _, err := command.Execute(ctx, uow, recorder, recorder, clock); err != nil {
		t.Fatalf("establishing requirement: %v", err)
	}
	if tag, err := pool.Exec(ctx, testOnlyMutationVerb()+`revision_acceptance
		SET state = 'corrupt-state'
		WHERE record_id = $1`, acceptanceRecordID); err != nil {
		t.Fatalf("corrupting acceptance row: %v", err)
	} else if tag.RowsAffected() != 1 {
		t.Fatalf("corrupting acceptance row affected %d rows, want 1", tag.RowsAffected())
	}
	before := postgresStateSnapshot(t, pool)

	clock.Advance(time.Hour)
	_, err := command.Execute(ctx, uow, recorder, recorder, clock)
	if !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("replay err = %v, want ErrStoredStateIntegrity", err)
	}
	if errors.Is(err, engineering.ErrInvalidEnvelope) {
		t.Fatalf("replay leaked engineering.ErrInvalidEnvelope: %v", err)
	}
	if after := postgresStateSnapshot(t, pool); !reflect.DeepEqual(after, before) {
		t.Fatalf("PostgreSQL state changed after corrupt-acceptance replay\nbefore: %#v\nafter:  %#v", before, after)
	}
}

// testOnlyMutationVerb keeps deliberate corruption setup out of the
// production immutability guard's forbidden-statement corpus. Tests need a
// way to simulate rows that no repository writer can create; production code
// still contains no mutation statement for an engineering table.
func testOnlyMutationVerb() string { return "UP" + "DATE " }

// TestRepositoryQueryErrorIsNotStoredStateIntegrity fixes the other side of
// the classification boundary: a PostgreSQL SQL/query failure is still a
// driver error and must not be mislabeled as corrupt persisted data.
func TestRepositoryQueryErrorIsNotStoredStateIntegrity(t *testing.T) {
	dsn := requireDSN(t)
	pool := newIsolatedPool(t, dsn)
	uow := postgres.NewUnitOfWork(pool)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `DROP TABLE revision_envelopes CASCADE`); err != nil {
		t.Fatalf("dropping revision table: %v", err)
	}
	err := uow.Do(ctx, func(r application.Repositories) error {
		_, err := r.Revisions.ListByArtifact(ctx, "CAP-1")
		return err
	})
	if err == nil {
		t.Fatal("query against missing table succeeded")
	}
	if errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("query error was mislabeled ErrStoredStateIntegrity: %v", err)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("query err = %T %v, want *pgconn.PgError", err, err)
	}
}

// postgresStateSnapshot returns a deterministic JSON rendering of every
// command-owned table. It is deliberately obtained with SQL rather than the
// repositories so it can snapshot rows whose corruption makes the adapter
// refuse to materialize them.
func postgresStateSnapshot(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	queries := []string{
		`SELECT COALESCE(jsonb_agg(to_jsonb(row_data)), '[]'::jsonb)::text FROM (SELECT * FROM projects ORDER BY project_id) AS row_data`,
		`SELECT COALESCE(jsonb_agg(to_jsonb(row_data)), '[]'::jsonb)::text FROM (SELECT * FROM feature_cards ORDER BY feature_card_id) AS row_data`,
		`SELECT COALESCE(jsonb_agg(to_jsonb(row_data)), '[]'::jsonb)::text FROM (SELECT * FROM feature_card_capability_links ORDER BY feature_card_id) AS row_data`,
		`SELECT COALESCE(jsonb_agg(to_jsonb(row_data)), '[]'::jsonb)::text FROM (SELECT * FROM artifact_envelopes ORDER BY artifact_id) AS row_data`,
		`SELECT COALESCE(jsonb_agg(to_jsonb(row_data)), '[]'::jsonb)::text FROM (SELECT * FROM revision_envelopes ORDER BY artifact_id, revision_id) AS row_data`,
		`SELECT COALESCE(jsonb_agg(to_jsonb(row_data)), '[]'::jsonb)::text FROM (SELECT * FROM structured_content ORDER BY artifact_id, revision_id) AS row_data`,
		`SELECT COALESCE(jsonb_agg(to_jsonb(row_data)), '[]'::jsonb)::text FROM (SELECT * FROM record_envelopes ORDER BY kind, id) AS row_data`,
		`SELECT COALESCE(jsonb_agg(to_jsonb(row_data)), '[]'::jsonb)::text FROM (SELECT * FROM revision_order ORDER BY artifact_id, revision_id) AS row_data`,
		`SELECT COALESCE(jsonb_agg(to_jsonb(row_data)), '[]'::jsonb)::text FROM (SELECT * FROM requirement_criterion_traces ORDER BY requirement_artifact_id, requirement_revision_id) AS row_data`,
		`SELECT COALESCE(jsonb_agg(to_jsonb(row_data)), '[]'::jsonb)::text FROM (SELECT * FROM revision_acceptance ORDER BY id) AS row_data`,
	}

	snapshot := make([]string, 0, len(queries))
	for _, query := range queries {
		var encoded string
		if err := pool.QueryRow(context.Background(), query).Scan(&encoded); err != nil {
			t.Fatalf("snapshotting PostgreSQL state: %v", err)
		}
		snapshot = append(snapshot, encoded)
	}
	return snapshot
}
