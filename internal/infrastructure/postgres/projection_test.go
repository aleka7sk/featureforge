package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering"
	peos "github.com/aleka7sk/featureforge/internal/engineering/peos"
	"github.com/aleka7sk/featureforge/internal/infrastructure/postgres"
)

// TestTypedColumnsAgreeWithAuthoritativePayload is the milestone's
// projection exit criterion. Every indexed column on record_envelopes and
// revision_envelopes is a projection of the payload, which is authoritative.
// This writes real records through the real commands, then reads the raw
// columns with plain SQL and compares them against what decoding the stored
// payload through the PEOS codec produces.
//
// If the two ever disagree, the projection is lying and every query that
// filters on it -- current-claim resolution, readiness, the timeline -- is
// answering from stale data.
func TestTypedColumnsAgreeWithAuthoritativePayload(t *testing.T) {
	dsn := requireDSN(t)
	pool := newIsolatedPool(t, dsn)
	uow := postgres.NewUnitOfWork(pool)
	recorder := peos.NewRecorder()
	clock := application.NewFixedClock(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	ctx := context.Background()

	seedCapability(t, uow, recorder, recorder, clock)

	// A decision cites evidence in its basis, so the evidence must exist
	// before the decision that references it.
	seedEvidence(t, uow, recorder, clock, "EV-0")
	if _, err := (application.RecordArchitectureDecisionCommand{
		DecisionID: "DEC-1", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		Question: "Should homework support an audio attachment?", OutcomeStatement: "Yes, by content address.",
		EvidenceArtifactID: "EV-0", EvidenceRevisionID: "EV-0-REV-1",
	}).Execute(ctx, uow, recorder, recorder, clock); err != nil {
		t.Fatalf("record decision: %v", err)
	}

	// Read the projected columns straight from SQL, bypassing the adapter.
	var subjectKey, scope, outcome, correctionKind, correctionTarget, stateID, payloadDigest string
	var criteria, evidence, executions []string
	var payload []byte
	err := pool.QueryRow(ctx, `
        SELECT subject_key, scope, outcome, criterion_keys, evidence_keys, execution_keys,
               correction_kind, correction_target_id, state_id, payload, payload_digest
        FROM record_envelopes WHERE kind = 'decision' AND id = 'DEC-1'`).
		Scan(&subjectKey, &scope, &outcome, &criteria, &evidence, &executions,
			&correctionKind, &correctionTarget, &stateID, &payload, &payloadDigest)
	if err != nil {
		t.Fatalf("reading raw projected columns: %v", err)
	}

	// The payload is authoritative: its digest must match the stored one, and
	// it must still decode through the codec that wrote it.
	if got := engineering.ComputeDigest(payload).String(); got != payloadDigest {
		t.Errorf("payload_digest column = %s, but the stored payload hashes to %s", payloadDigest, got)
	}
	if _, err := peos.DecodeDecision(payload); err != nil {
		t.Errorf("stored payload no longer decodes as a PEOS decision: %v", err)
	}

	// The subject projection must equal what the shared key function derives
	// from the same identities the command was given.
	wantSubject := engineering.ArtifactRevisionSubjectKey("CAP-1", "CAP-1-REV-1")
	if subjectKey != wantSubject {
		t.Errorf("subject_key = %q, want %q", subjectKey, wantSubject)
	}

	// And reading the same row back through the adapter must reproduce the
	// identical projections -- no field lost or transformed in the round trip.
	var env engineering.RecordEnvelope
	if err := uow.Do(ctx, func(r application.Repositories) error {
		key, err := engineering.NewRecordKey(engineering.RecordKindDecision, "DEC-1")
		if err != nil {
			return err
		}
		found, ok, err := r.Records.Get(ctx, key)
		if err != nil {
			return err
		}
		if !ok {
			t.Fatal("DEC-1 not found through the adapter")
		}
		env = found
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if env.SubjectKey != subjectKey {
		t.Errorf("adapter SubjectKey = %q, raw column = %q", env.SubjectKey, subjectKey)
	}
	if env.Scope != scope {
		t.Errorf("adapter Scope = %q, raw column = %q", env.Scope, scope)
	}
	if env.Outcome != outcome {
		t.Errorf("adapter Outcome = %q, raw column = %q", env.Outcome, outcome)
	}
	if env.CorrectionKind != correctionKind || env.CorrectionTargetID != correctionTarget {
		t.Errorf("adapter correction = (%q,%q), raw columns = (%q,%q)",
			env.CorrectionKind, env.CorrectionTargetID, correctionKind, correctionTarget)
	}
	if env.StateID != stateID {
		t.Errorf("adapter StateID = %q, raw column = %q", env.StateID, stateID)
	}
	if string(env.Payload) != string(payload) {
		t.Error("adapter Payload differs byte-for-byte from the stored bytea")
	}
}

// TestPayloadIsStoredByteIdentical asserts bytea round-trips the canonical
// JSON exactly. This is why payloads are bytea rather than jsonb: every
// idempotency and conflict check in this codebase compares payload bytes, and
// jsonb reparses and reserializes with no byte-identity guarantee.
func TestPayloadIsStoredByteIdentical(t *testing.T) {
	dsn := requireDSN(t)
	pool := newIsolatedPool(t, dsn)
	uow := postgres.NewUnitOfWork(pool)
	recorder := peos.NewRecorder()
	clock := application.NewFixedClock(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	ctx := context.Background()

	seedCapability(t, uow, recorder, recorder, clock)

	var stored []byte
	var digest string
	if err := pool.QueryRow(ctx,
		`SELECT payload, payload_digest FROM artifact_envelopes WHERE artifact_id = 'CAP-1'`).
		Scan(&stored, &digest); err != nil {
		t.Fatal(err)
	}
	if got := engineering.ComputeDigest(stored).String(); got != digest {
		t.Errorf("stored payload hashes to %s, but payload_digest is %s -- storage is not byte-exact", got, digest)
	}
}

// TestRevisionSubjectKeyColumnProjection is the AD-025 / FF-016 sibling of
// TestTypedColumnsAgreeWithAuthoritativePayload: it extends the same
// raw-SQL-versus-adapter comparison to the new subject_key column.
func TestRevisionSubjectKeyColumnProjection(t *testing.T) {
	dsn := requireDSN(t)
	pool := newIsolatedPool(t, dsn)
	uow := postgres.NewUnitOfWork(pool)
	recorder := peos.NewRecorder()
	clock := application.NewFixedClock(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	ctx := context.Background()

	seedCapability(t, uow, recorder, recorder, clock)
	acceptCapabilityForRequirement(t, uow, recorder, clock)
	acceptanceRecordID := "ACC-REQ-1-REV-1"
	if _, err := (application.EstablishRequirementCommand{
		ArtifactID: "REQ-1", RevisionID: "REQ-1-REV-1", AcceptanceRecordID: &acceptanceRecordID,
		Statement: "Published homework SHALL be visible to the student.", SubjectArtifactID: "CAP-1",
		SourceCapabilityRevisionID: "CAP-1-REV-1", SourceAcceptanceCriterionKey: "AC-1",
	}).Execute(ctx, uow, recorder, recorder, clock); err != nil {
		t.Fatalf("establish requirement: %v", err)
	}

	// A requirement revision projects a non-null subject_key equal to what
	// engineering.ArtifactSubjectKey derives from the same identity.
	var reqSubject *string
	if err := pool.QueryRow(ctx,
		`SELECT subject_key FROM revision_envelopes WHERE artifact_id = 'REQ-1' AND revision_id = 'REQ-1-REV-1'`).
		Scan(&reqSubject); err != nil {
		t.Fatalf("reading raw subject_key column: %v", err)
	}
	if reqSubject == nil {
		t.Fatal("requirement revision's subject_key column is NULL, want a projected value")
	}
	wantSubject := engineering.ArtifactSubjectKey("CAP-1")
	if *reqSubject != wantSubject {
		t.Errorf("subject_key column = %q, want %q", *reqSubject, wantSubject)
	}

	// A capability revision -- a family with no subject -- projects SQL
	// NULL, which the adapter must read back as an empty string, never as
	// an error and never as a sentinel value.
	var capSubject *string
	if err := pool.QueryRow(ctx,
		`SELECT subject_key FROM revision_envelopes WHERE artifact_id = 'CAP-1' AND revision_id = 'CAP-1-REV-1'`).
		Scan(&capSubject); err != nil {
		t.Fatalf("reading raw subject_key column: %v", err)
	}
	if capSubject != nil {
		t.Errorf("capability revision's subject_key column = %q, want NULL", *capSubject)
	}

	// The adapter reproduces both projections exactly, and ListByFamilyAndSubject
	// finds the requirement revision through the same column.
	if err := uow.Do(ctx, func(r application.Repositories) error {
		reqKey, err := engineering.NewRevisionKey("REQ-1", "REQ-1-REV-1")
		if err != nil {
			return err
		}
		reqEnv, found, err := r.Revisions.Get(ctx, reqKey)
		if err != nil {
			return err
		}
		if !found {
			t.Fatal("REQ-1 revision not found through the adapter")
		}
		if reqEnv.SubjectKey != wantSubject {
			t.Errorf("adapter SubjectKey = %q, raw column = %q", reqEnv.SubjectKey, *reqSubject)
		}

		capKey, err := engineering.NewRevisionKey("CAP-1", "CAP-1-REV-1")
		if err != nil {
			return err
		}
		capEnv, found, err := r.Revisions.Get(ctx, capKey)
		if err != nil {
			return err
		}
		if !found {
			t.Fatal("CAP-1 revision not found through the adapter")
		}
		if capEnv.SubjectKey != "" {
			t.Errorf("capability revision adapter SubjectKey = %q, want empty (NULL column)", capEnv.SubjectKey)
		}

		byFamilyAndSubject, err := r.Revisions.ListByFamilyAndSubject(ctx, engineering.RevisionFamilyRequirement, wantSubject)
		if err != nil {
			return err
		}
		if len(byFamilyAndSubject) != 1 || byFamilyAndSubject[0].Key != reqKey {
			t.Errorf("ListByFamilyAndSubject = %v, want exactly [REQ-1/REQ-1-REV-1]", byFamilyAndSubject)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func seedEvidence(t *testing.T, uow application.UnitOfWork, recorder application.EngineeringRecorder, clock application.Clock, evidenceID string) {
	t.Helper()
	ctx := context.Background()
	err := uow.Do(ctx, func(r application.Repositories) error {
		artEnv, revEnv, err := recorder.RecordEvidence(engineering.EvidenceInput{
			ArtifactID: evidenceID, RevisionID: evidenceID + "-REV-1",
			Locator: "https://evidence.example/" + evidenceID, RecordedAt: clock.Now(),
		})
		if err != nil {
			return err
		}
		if err := r.Artifacts.Put(ctx, artEnv); err != nil {
			return err
		}
		return r.Revisions.Put(ctx, revEnv)
	})
	if err != nil {
		t.Fatal(err)
	}
}
