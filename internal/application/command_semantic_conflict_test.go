package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

func assertImmutableConflictWithoutWrites[T any](t *testing.T, f commandFixture, execute func() (T, error)) {
	t.Helper()
	release := forbidPersistenceWrites(f)
	defer release()
	if _, err := execute(); !errors.Is(err, application.ErrImmutableValueConflict) {
		t.Fatalf("err = %v, want ErrImmutableValueConflict", err)
	}
}

func seedSemanticValidationFoundation(t *testing.T, f commandFixture) {
	t.Helper()
	ctx := context.Background()
	seedCapability(t, f)
	requirement := application.EstablishRequirementCommand{
		ArtifactID: "REQ-SEMANTIC", RevisionID: "REQ-SEMANTIC-REV-1",
		Statement: "The system SHALL reject changed immutable command semantics.", SubjectArtifactID: "CAP-1",
		SourceCapabilityRevisionID: "CAP-1-REV-1", SourceAcceptanceCriterionKey: "AC-1",
		AcceptanceRecordID: memberID("MEM-REQ-SEMANTIC"),
	}
	if _, err := requirement.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("seed semantic requirement: %v", err)
	}
	plan := application.EstablishValidationPlanCommand{
		ArtifactID: "VP-SEMANTIC", RevisionID: "VP-SEMANTIC-REV-1", ScopeArtifactID: "CAP-1",
		AcceptanceRecordID: memberID("MEM-VP-SEMANTIC"),
		Activities: []application.PlanActivityCommandInput{{
			Key: "ACT-SEMANTIC", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
			Method: "manual-review", OutcomeInterpretation: "Changed semantics conflict.",
			RequirementArtifactID: "REQ-SEMANTIC", RequirementRevisionID: "REQ-SEMANTIC-REV-1",
			ExpectedEvidence: []string{"semantic conflict report"},
		}},
	}
	if _, err := plan.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("seed semantic validation plan: %v", err)
	}
}

func semanticValidationRun(executionID string) application.RecordValidationRunCommand {
	return application.RecordValidationRunCommand{
		ExecutionID:    executionID,
		PlanArtifactID: "VP-SEMANTIC", PlanRevisionID: "VP-SEMANTIC-REV-1", ActivityKey: "ACT-SEMANTIC",
		SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		Method: "manual-review", Outcome: "completed",
		EvidenceArtifactID: "EV-" + executionID, EvidenceRevisionID: "EV-" + executionID + "-REV-1",
		EvidenceLocator: "https://evidence.example/" + executionID,
	}
}

func semanticValidationClaim(claimID, executionID string) application.RecordValidationClaimCommand {
	return application.RecordValidationClaimCommand{
		ClaimID: claimID, ScopeArtifactID: "CAP-1",
		SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		RequirementArtifactID: "REQ-SEMANTIC", RequirementRevisionID: "REQ-SEMANTIC-REV-1",
		Outcome: "satisfied", Method: "manual-review",
		EvidenceArtifactID: "EV-" + executionID, EvidenceRevisionID: "EV-" + executionID + "-REV-1",
		ExecutionID: executionID, Reasoning: "The recorded evidence satisfies the requirement.",
	}
}

func semanticCorrection(claimID, targetID, executionID string) application.CorrectValidationClaimCommand {
	return application.CorrectValidationClaimCommand{
		ClaimID: claimID, CorrectionTarget: targetID, CorrectionKind: "correct", ScopeArtifactID: "CAP-1",
		SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		RequirementArtifactID: "REQ-SEMANTIC", RequirementRevisionID: "REQ-SEMANTIC-REV-1",
		Outcome: "not-satisfied", Method: "manual-review",
		EvidenceArtifactID: "EV-" + executionID, EvidenceRevisionID: "EV-" + executionID + "-REV-1",
		ExecutionID: executionID, Reasoning: "The original assessment requires correction.",
	}
}

func TestC1ThroughC12ChangedCallerSemanticsConflictWithoutWrites(t *testing.T) {
	ctx := context.Background()

	t.Run("C1 project name", func(t *testing.T) {
		f := newCommandFixture()
		cmd := application.CreateProjectCommand{ProjectID: "PRJ-C1-CONFLICT", Name: "Original project"}
		if _, err := cmd.Execute(ctx, f.uow, f.clock); err != nil {
			t.Fatal(err)
		}
		changed := cmd
		changed.Name = "Changed project"
		assertImmutableConflictWithoutWrites(t, f, func() (application.CreateProjectResult, error) {
			return changed.Execute(ctx, f.uow, f.clock)
		})
	})

	t.Run("C2 feature title", func(t *testing.T) {
		f := newCommandFixture()
		if _, err := (application.CreateProjectCommand{ProjectID: "PRJ-C2-CONFLICT", Name: "C2"}).Execute(ctx, f.uow, f.clock); err != nil {
			t.Fatal(err)
		}
		cmd := application.CreateFeatureCommand{
			FeatureCardID: "FC-C2-CONFLICT", ProjectID: "PRJ-C2-CONFLICT", Title: "Original feature", Description: "Stable description",
		}
		if _, err := cmd.Execute(ctx, f.uow, f.clock); err != nil {
			t.Fatal(err)
		}
		changed := cmd
		changed.Title = "Changed feature"
		assertImmutableConflictWithoutWrites(t, f, func() (application.CreateFeatureResult, error) {
			return changed.Execute(ctx, f.uow, f.clock)
		})
	})

	t.Run("C3 capability content", func(t *testing.T) {
		f := newCommandFixture()
		setupProjectAndFeature(t, f)
		cmd := application.EstablishCapabilitySpecificationCommand{
			FeatureCardID: "FC-1", ArtifactID: "CAP-C3-CONFLICT", RevisionID: "CAP-C3-CONFLICT-REV-1",
			Content: mustContent(t, "Original capability"),
		}
		if _, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		changed := cmd
		changed.Content = mustContent(t, "Changed capability")
		assertImmutableConflictWithoutWrites(t, f, func() (application.EstablishCapabilitySpecificationResult, error) {
			return changed.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
	})

	t.Run("C4 capability revision content", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		cmd := application.ReviseCapabilitySpecificationCommand{
			ArtifactID: "CAP-1", RevisionID: "CAP-C4-CONFLICT-REV-2", Content: mustContent(t, "Original revision"),
		}
		if _, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		changed := cmd
		changed.Content = mustContent(t, "Changed revision")
		assertImmutableConflictWithoutWrites(t, f, func() (application.ReviseCapabilitySpecificationResult, error) {
			return changed.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
	})

	t.Run("C5 acceptance reason", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		cmd := application.AcceptCapabilityRevisionCommand{
			RecordID: "ACC-C5-CONFLICT", ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-1",
			State: engineering.AcceptanceStateWithdrawn, Reason: "original reason",
		}
		if _, err := cmd.Execute(ctx, f.uow, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		changed := cmd
		changed.Reason = "changed reason"
		assertImmutableConflictWithoutWrites(t, f, func() (application.AcceptCapabilityRevisionResult, error) {
			return changed.Execute(ctx, f.uow, f.rec, f.clock)
		})
	})

	t.Run("C6 lifecycle effective time", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		cmd := application.AssignLifecycleStateCommand{
			AssignmentID: "SA-C6-CONFLICT", SubjectArtifactID: "CAP-1", State: "drafting", IsEntry: true,
			TransitionRecordArtifactID: "TR-C6-CONFLICT", TransitionRecordRevisionID: "TR-C6-CONFLICT-REV-0",
		}
		if _, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		changed := cmd
		changed.EffectiveAt = time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
		changed.HasEffectiveAt = true
		assertImmutableConflictWithoutWrites(t, f, func() (application.AssignLifecycleStateResult, error) {
			return changed.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
	})

	t.Run("C7 requirement statement", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		cmd := application.EstablishRequirementCommand{
			ArtifactID: "REQ-C7-CONFLICT", RevisionID: "REQ-C7-CONFLICT-REV-1",
			Statement: "The system SHALL retain the original statement.", SubjectArtifactID: "CAP-1",
			SourceCapabilityRevisionID: "CAP-1-REV-1", SourceAcceptanceCriterionKey: "AC-1",
			AcceptanceRecordID: memberID("MEM-C7-CONFLICT"),
		}
		if _, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		changed := cmd
		changed.Statement = "The system SHALL use a changed statement."
		assertImmutableConflictWithoutWrites(t, f, func() (application.EstablishRequirementResult, error) {
			return changed.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
	})

	t.Run("C8 decision rationale", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		seedEvidenceRevision(t, f, "EV-C8-CONFLICT", "EV-C8-CONFLICT-REV-1")
		cmd := application.RecordArchitectureDecisionCommand{
			DecisionID: "DEC-C8-CONFLICT", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
			Question: "Should immutable semantics be preserved?", OutcomeStatement: "They are preserved.",
			EvidenceArtifactID: "EV-C8-CONFLICT", EvidenceRevisionID: "EV-C8-CONFLICT-REV-1",
			Rationale: "Original rationale.",
		}
		if _, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		changed := cmd
		changed.Rationale = "Changed rationale."
		assertImmutableConflictWithoutWrites(t, f, func() (application.RecordArchitectureDecisionResult, error) {
			return changed.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
	})

	t.Run("C9 activity interpretation", func(t *testing.T) {
		f := newCommandFixture()
		seedSemanticValidationFoundation(t, f)
		cmd := application.EstablishValidationPlanCommand{
			ArtifactID: "VP-C9-CONFLICT", RevisionID: "VP-C9-CONFLICT-REV-1", ScopeArtifactID: "CAP-1",
			AcceptanceRecordID: memberID("MEM-VP-C9-CONFLICT"),
			Activities: []application.PlanActivityCommandInput{{
				Key: "ACT-C9-CONFLICT", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
				Method: "manual-review", OutcomeInterpretation: "Original interpretation.",
				RequirementArtifactID: "REQ-SEMANTIC", RequirementRevisionID: "REQ-SEMANTIC-REV-1",
			}},
		}
		if _, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		changed := cmd
		changed.Activities = append([]application.PlanActivityCommandInput(nil), cmd.Activities...)
		changed.Activities[0].OutcomeInterpretation = "Changed interpretation."
		assertImmutableConflictWithoutWrites(t, f, func() (application.EstablishValidationPlanResult, error) {
			return changed.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
	})

	t.Run("C10 evidence locator", func(t *testing.T) {
		f := newCommandFixture()
		seedSemanticValidationFoundation(t, f)
		cmd := semanticValidationRun("ER-C10-CONFLICT")
		if _, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		changed := cmd
		changed.EvidenceLocator = "https://evidence.example/changed-location"
		assertImmutableConflictWithoutWrites(t, f, func() (application.RecordValidationRunResult, error) {
			return changed.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
	})

	t.Run("C11 claim reasoning", func(t *testing.T) {
		f := newCommandFixture()
		seedSemanticValidationFoundation(t, f)
		run := semanticValidationRun("ER-C11-CONFLICT")
		if _, err := run.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		cmd := semanticValidationClaim("CLM-C11-CONFLICT", run.ExecutionID)
		if _, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		changed := cmd
		changed.Reasoning = "Changed claim reasoning."
		assertImmutableConflictWithoutWrites(t, f, func() (application.RecordValidationClaimResult, error) {
			return changed.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
	})

	t.Run("C12 correction reasoning", func(t *testing.T) {
		f := newCommandFixture()
		seedSemanticValidationFoundation(t, f)
		run := semanticValidationRun("ER-C12-CONFLICT")
		if _, err := run.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		target := semanticValidationClaim("CLM-C12-TARGET", run.ExecutionID)
		if _, err := target.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		cmd := semanticCorrection("CLM-C12-CONFLICT", target.ClaimID, run.ExecutionID)
		if _, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		changed := cmd
		changed.Reasoning = "Changed correction reasoning."
		assertImmutableConflictWithoutWrites(t, f, func() (application.CorrectValidationClaimResult, error) {
			return changed.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
	})
}

func TestC11AndC12ShareTheClaimIdentityNamespace(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedSemanticValidationFoundation(t, f)
	run := semanticValidationRun("ER-CLAIM-NAMESPACE")
	if _, err := run.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	target := semanticValidationClaim("CLM-NAMESPACE-TARGET", run.ExecutionID)
	if _, err := target.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	occupied := semanticValidationClaim("CLM-NAMESPACE-SHARED", run.ExecutionID)
	if _, err := occupied.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	crossCommand := semanticCorrection(occupied.ClaimID, target.ClaimID, run.ExecutionID)
	assertImmutableConflictWithoutWrites(t, f, func() (application.CorrectValidationClaimResult, error) {
		return crossCommand.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
	})
}
