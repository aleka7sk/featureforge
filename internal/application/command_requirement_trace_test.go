package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

type missingRequirementTraceRepository struct {
	application.RequirementCriterionTraceRepository
	missing engineering.RevisionKey
}

func (r missingRequirementTraceRepository) Get(ctx context.Context, key engineering.RevisionKey) (engineering.RequirementCriterionTrace, bool, error) {
	if key == r.missing {
		return engineering.RequirementCriterionTrace{}, false, nil
	}
	return r.RequirementCriterionTraceRepository.Get(ctx, key)
}

func traceRequirementCommand(artifactID string) application.EstablishRequirementCommand {
	return application.EstablishRequirementCommand{
		ArtifactID: artifactID, RevisionID: artifactID + "-REV-1",
		Statement:         "The system SHALL retain its exact capability criterion source.",
		SubjectArtifactID: "CAP-1", SourceCapabilityRevisionID: "CAP-1-REV-1",
		SourceAcceptanceCriterionKey: "AC-1", AcceptanceRecordID: memberID("MEM-" + artifactID),
	}
}

func TestC7PersistsExactRequirementCriterionTraceAtomically(t *testing.T) {
	f := newCommandFixture()
	seedCapability(t, f)
	ctx := context.Background()
	cmd := traceRequirementCommand("REQ-TRACE")
	if _, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}

	requirementKey := mustRevKey(t, cmd.ArtifactID, cmd.RevisionID)
	capabilityKey := mustRevKey(t, cmd.SubjectArtifactID, cmd.SourceCapabilityRevisionID)
	if err := f.uow.Do(ctx, func(r application.Repositories) error {
		trace, found, err := r.RequirementTraces.Get(ctx, requirementKey)
		if err != nil {
			return err
		}
		if !found {
			t.Fatal("persisted RequirementCriterionTrace not found")
		}
		if trace.RequirementRevision != requirementKey || trace.CapabilityRevision != capabilityKey ||
			trace.AcceptanceCriterionKey != "AC-1" {
			t.Fatalf("trace = %+v, want exact request source", trace)
		}
		revision, found, err := r.Revisions.Get(ctx, requirementKey)
		if err != nil {
			return err
		}
		if !found {
			t.Fatal("persisted Requirement revision not found")
		}
		order, found, err := r.RevisionOrder.Get(ctx, requirementKey)
		if err != nil {
			return err
		}
		if !found {
			t.Fatal("persisted Requirement order not found")
		}
		if !trace.RecordedAt.Equal(revision.RecordedAt) || !trace.RecordedAt.Equal(order.RecordedAt) {
			t.Fatalf("trace/revision/order times disagree: %s / %s / %s", trace.RecordedAt, revision.RecordedAt, order.RecordedAt)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestC7NewActSourceReferenceClassification(t *testing.T) {
	t.Run("missing revision", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		cmd := traceRequirementCommand("REQ-MISSING-SOURCE")
		cmd.SourceCapabilityRevisionID = "CAP-1-REV-MISSING"
		release := forbidPersistenceWrites(f)
		defer release()
		if _, err := cmd.Execute(context.Background(), f.uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrReferencedValueMissing) {
			t.Fatalf("err = %v, want ErrReferencedValueMissing", err)
		}
	})

	t.Run("coherent foreign artifact", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		seedEvidenceRevision(t, f, "EV-SOURCE", "EV-SOURCE-REV-1")
		cmd := traceRequirementCommand("REQ-FOREIGN-SOURCE")
		cmd.SubjectArtifactID = "EV-SOURCE"
		cmd.SourceCapabilityRevisionID = "EV-SOURCE-REV-1"
		release := forbidPersistenceWrites(f)
		defer release()
		if _, err := cmd.Execute(context.Background(), f.uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrReferencedValueMissing) {
			t.Fatalf("err = %v, want ErrReferencedValueMissing", err)
		}
	})

	t.Run("stale revision", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		ctx := context.Background()
		if _, err := (application.ReviseCapabilitySpecificationCommand{
			ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-2", Content: mustContent(t, "Revision 2"),
		}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		acceptCapability(t, f, "ACC-CAP-1-REV-2", "CAP-1", "CAP-1-REV-2")
		cmd := traceRequirementCommand("REQ-STALE-SOURCE")
		release := forbidPersistenceWrites(f)
		defer release()
		if _, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrReferencedValueMissing) {
			t.Fatalf("err = %v, want ErrReferencedValueMissing", err)
		}
	})

	t.Run("missing criterion", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		cmd := traceRequirementCommand("REQ-MISSING-CRITERION")
		cmd.SourceAcceptanceCriterionKey = "AC-404"
		release := forbidPersistenceWrites(f)
		defer release()
		if _, err := cmd.Execute(context.Background(), f.uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrReferencedValueMissing) {
			t.Fatalf("err = %v, want ErrReferencedValueMissing", err)
		}
	})

	t.Run("corrupt source", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		key := mustRevKey(t, "CAP-1", "CAP-1-REV-1")
		uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
			r.Revisions = corruptRevisionGetRepository{RevisionEnvelopeRepository: r.Revisions, key: key}
		}}
		cmd := traceRequirementCommand("REQ-CORRUPT-SOURCE")
		release := forbidPersistenceWrites(f)
		defer release()
		if _, err := cmd.Execute(context.Background(), uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrStoredStateIntegrity) {
			t.Fatalf("err = %v, want ErrStoredStateIntegrity", err)
		}
	})
}

func TestC7OccupiedTraceMismatchAndMissingTrace(t *testing.T) {
	f := newCommandFixture()
	seedCapability(t, f)
	ctx := context.Background()
	cmd := traceRequirementCommand("REQ-TRACE-REPLAY")
	if _, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}

	changed := cmd
	changed.SourceAcceptanceCriterionKey = "AC-2"
	release := forbidPersistenceWrites(f)
	if _, err := changed.Execute(ctx, f.uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrImmutableValueConflict) {
		release()
		t.Fatalf("changed trace err = %v, want ErrImmutableValueConflict", err)
	}
	release()

	key := mustRevKey(t, cmd.ArtifactID, cmd.RevisionID)
	uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
		r.RequirementTraces = missingRequirementTraceRepository{
			RequirementCriterionTraceRepository: r.RequirementTraces, missing: key,
		}
	}}
	release = forbidPersistenceWrites(f)
	defer release()
	if _, err := cmd.Execute(ctx, uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("missing trace err = %v, want ErrStoredStateIntegrity", err)
	}
}
