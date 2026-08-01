package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

func assertAdvancingClockReplay[T comparable](t *testing.T, f commandFixture, execute func() (T, error)) T {
	t.Helper()
	first, err := execute()
	if err != nil {
		t.Fatalf("first execution: %v", err)
	}
	f.clock.Advance(73 * time.Minute)
	releaseWriteGuard := forbidPersistenceWrites(f)
	defer releaseWriteGuard()
	second, err := execute()
	if err != nil {
		t.Fatalf("replay after advancing clock: %v", err)
	}
	if second != first {
		t.Fatalf("replay result = %+v, want original %+v", second, first)
	}
	return first
}

func forbidPersistenceWrites(f commandFixture) func() {
	f.store.SetFailureHook(func(string, int) error {
		return errors.New("test: zero-write branch attempted a persistence write")
	})
	return func() {
		f.store.SetFailureHook(nil)
	}
}

func seedRequirementForReplay(t *testing.T, f commandFixture) {
	t.Helper()
	cmd := application.EstablishRequirementCommand{
		ArtifactID: "REQ-REPLAY", RevisionID: "REQ-REPLAY-REV-1",
		Statement: "The system SHALL replay validation acts.", SubjectArtifactID: "CAP-1",
		AcceptanceRecordID: memberID("REQ-MEMBER-SEED"),
	}
	if _, err := cmd.Execute(context.Background(), f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("seed requirement: %v", err)
	}
}

func seedValidationFoundationForReplay(t *testing.T, f commandFixture) {
	t.Helper()
	seedCapability(t, f)
	seedRequirementForReplay(t, f)
	cmd := application.EstablishValidationPlanCommand{
		ArtifactID: "VP-REPLAY", RevisionID: "VP-REPLAY-REV-1", ScopeArtifactID: "CAP-1",
		AcceptanceRecordID: memberID("PLAN-MEMBER-SEED"),
		Activities: []application.PlanActivityCommandInput{{
			Key: "A-REPLAY", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
			Method: "manual-review", OutcomeInterpretation: "Satisfied when replay is stable.",
			RequirementArtifactID: "REQ-REPLAY", RequirementRevisionID: "REQ-REPLAY-REV-1",
			ExpectedEvidence: []string{"replay report"},
		}},
	}
	if _, err := cmd.Execute(context.Background(), f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("seed validation plan: %v", err)
	}
}

func TestC12CompetingCorrectionsRemainWritableAndReplayable(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedValidationFoundationForReplay(t, f)
	run := validationRunForReplay("ER-AMBIGUOUS-CORRECTIONS")
	if _, err := run.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("seed validation run: %v", err)
	}
	original := validationClaimForReplay("CLM-AMBIGUOUS-0", run.ExecutionID)
	if _, err := original.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("seed original claim: %v", err)
	}
	correction := func(id, outcome string) application.CorrectValidationClaimCommand {
		return application.CorrectValidationClaimCommand{
			ClaimID: id, CorrectionTarget: original.ClaimID, CorrectionKind: "correct",
			ScopeArtifactID: "CAP-1", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
			RequirementArtifactID: "REQ-REPLAY", RequirementRevisionID: "REQ-REPLAY-REV-1",
			Outcome: outcome, Method: "manual-review",
			EvidenceArtifactID: run.EvidenceArtifactID, EvidenceRevisionID: run.EvidenceRevisionID,
			ExecutionID: run.ExecutionID, Reasoning: "An independent reviewer recorded a competing correction.",
		}
	}
	first := correction("CLM-AMBIGUOUS-1", "not-satisfied")
	second := correction("CLM-AMBIGUOUS-2", "satisfied")
	if _, err := first.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("first correction: %v", err)
	}
	if _, err := second.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("second competing correction: %v", err)
	}

	err := f.uow.Do(ctx, func(r application.Repositories) error {
		_, err := application.ResolveCurrentClaim(
			ctx, r, "artifact-revision:CAP-1/CAP-1-REV-1", "featureforge:capability|CAP-1",
			[]string{"requirement-revision:REQ-REPLAY/REQ-REPLAY-REV-1"},
		)
		return err
	})
	if !errors.Is(err, application.ErrCorrectionAmbiguous) {
		t.Fatalf("ResolveCurrentClaim err = %v, want ErrCorrectionAmbiguous", err)
	}

	f.clock.Advance(time.Hour)
	release := forbidPersistenceWrites(f)
	defer release()
	if _, err := first.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("first correction replay in ambiguous graph: %v", err)
	}
	if _, err := second.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("second correction replay in ambiguous graph: %v", err)
	}
}

func validationRunForReplay(id string) application.RecordValidationRunCommand {
	return application.RecordValidationRunCommand{
		ExecutionID: id, PlanArtifactID: "VP-REPLAY", PlanRevisionID: "VP-REPLAY-REV-1", ActivityKey: "A-REPLAY",
		SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		Method: "manual-review", Outcome: "completed",
		EvidenceArtifactID: "EV-" + id, EvidenceRevisionID: "EV-" + id + "-REV-1",
		EvidenceLocator: "https://evidence.example/" + id,
	}
}

func validationClaimForReplay(id, executionID string) application.RecordValidationClaimCommand {
	return application.RecordValidationClaimCommand{
		ClaimID: id, ScopeArtifactID: "CAP-1",
		SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		RequirementArtifactID: "REQ-REPLAY", RequirementRevisionID: "REQ-REPLAY-REV-1",
		Outcome: "satisfied", Method: "manual-review",
		EvidenceArtifactID: "EV-" + executionID, EvidenceRevisionID: "EV-" + executionID + "-REV-1",
		ExecutionID: executionID, Reasoning: "The replay evidence is stable.",
	}
}

func seedDecisionEvidence(t *testing.T, f commandFixture) {
	t.Helper()
	seedEvidenceRevision(t, f, "EV-DECISION", "EV-DECISION-REV-1")
}

func TestC1ThroughC12ReplayAfterClockAdvance(t *testing.T) {
	ctx := context.Background()

	t.Run("C1 CreateProject", func(t *testing.T) {
		f := newCommandFixture()
		cmd := application.CreateProjectCommand{ProjectID: "PRJ-REPLAY", Name: "Replay project"}
		assertAdvancingClockReplay(t, f, func() (application.CreateProjectResult, error) {
			return cmd.Execute(ctx, f.uow, f.clock)
		})
	})

	t.Run("C2 CreateFeature", func(t *testing.T) {
		f := newCommandFixture()
		if _, err := (application.CreateProjectCommand{ProjectID: "PRJ-1", Name: "Pilot"}).Execute(ctx, f.uow, f.clock); err != nil {
			t.Fatal(err)
		}
		cmd := application.CreateFeatureCommand{FeatureCardID: "FC-REPLAY", ProjectID: "PRJ-1", Title: "Replay", Description: "Stable"}
		assertAdvancingClockReplay(t, f, func() (application.CreateFeatureResult, error) {
			return cmd.Execute(ctx, f.uow, f.clock)
		})
	})

	t.Run("C3 EstablishCapabilitySpecification", func(t *testing.T) {
		f := newCommandFixture()
		setupProjectAndFeature(t, f)
		cmd := application.EstablishCapabilitySpecificationCommand{
			FeatureCardID: "FC-1", ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-1",
			Content: mustContent(t, "Replay capability"),
		}
		assertAdvancingClockReplay(t, f, func() (application.EstablishCapabilitySpecificationResult, error) {
			return cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
	})

	t.Run("C4 ReviseCapabilitySpecification", func(t *testing.T) {
		f := newCommandFixture()
		setupProjectAndFeature(t, f)
		establishCapability(t, f)
		cmd := application.ReviseCapabilitySpecificationCommand{
			ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-2", Content: mustContent(t, "Replay revision"),
		}
		result := assertAdvancingClockReplay(t, f, func() (application.ReviseCapabilitySpecificationResult, error) {
			return cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
		if result.Sequence != 2 {
			t.Fatalf("sequence = %d, want original sequence 2", result.Sequence)
		}
	})

	t.Run("C5 AcceptCapabilityRevision", func(t *testing.T) {
		f := newCommandFixture()
		setupProjectAndFeature(t, f)
		establishCapability(t, f)
		cmd := application.AcceptCapabilityRevisionCommand{
			RecordID: "CAP-MEMBER-REPLAY", ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-1",
			State: engineering.AcceptanceStateAccepted, Reason: "approved",
		}
		assertAdvancingClockReplay(t, f, func() (application.AcceptCapabilityRevisionResult, error) {
			return cmd.Execute(ctx, f.uow, f.rec, f.clock)
		})
	})

	t.Run("C6 AssignLifecycleState", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		cmd := application.AssignLifecycleStateCommand{
			AssignmentID: "SA-REPLAY", SubjectArtifactID: "CAP-1", State: "drafting", IsEntry: true,
			TransitionRecordArtifactID: "TR-REPLAY", TransitionRecordRevisionID: "TR-REPLAY-REV-0",
		}
		assertAdvancingClockReplay(t, f, func() (application.AssignLifecycleStateResult, error) {
			return cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
	})

	t.Run("C7 EstablishRequirement", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		cmd := application.EstablishRequirementCommand{
			ArtifactID: "REQ-C7", RevisionID: "REQ-C7-REV-1", Statement: "The system SHALL replay C7.",
			SubjectArtifactID: "CAP-1", AcceptanceRecordID: memberID("REQ-MEMBER-REPLAY"),
		}
		assertAdvancingClockReplay(t, f, func() (application.EstablishRequirementResult, error) {
			return cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
	})

	t.Run("C8 RecordArchitectureDecision", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		seedDecisionEvidence(t, f)
		cmd := application.RecordArchitectureDecisionCommand{
			DecisionID: "DEC-REPLAY", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
			Question: "Should replay be semantic?", OutcomeStatement: "Yes.",
			Alternatives: []string{"rebuild values"}, EvidenceArtifactID: "EV-DECISION", EvidenceRevisionID: "EV-DECISION-REV-1",
			Rationale: "Generated representation details change.",
		}
		assertAdvancingClockReplay(t, f, func() (application.RecordArchitectureDecisionResult, error) {
			return cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
	})

	t.Run("C9 EstablishValidationPlan", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		seedRequirementForReplay(t, f)
		cmd := application.EstablishValidationPlanCommand{
			ArtifactID: "VP-C9", RevisionID: "VP-C9-REV-1", ScopeArtifactID: "CAP-1",
			AcceptanceRecordID: memberID("PLAN-MEMBER-REPLAY"),
			Activities: []application.PlanActivityCommandInput{{
				Key: "A-C9", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
				Method: "manual-review", OutcomeInterpretation: "Stable replay.",
				RequirementArtifactID: "REQ-REPLAY", RequirementRevisionID: "REQ-REPLAY-REV-1",
			}},
		}
		assertAdvancingClockReplay(t, f, func() (application.EstablishValidationPlanResult, error) {
			return cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
	})

	t.Run("C10 RecordValidationRun", func(t *testing.T) {
		f := newCommandFixture()
		seedValidationFoundationForReplay(t, f)
		cmd := validationRunForReplay("ER-REPLAY")
		assertAdvancingClockReplay(t, f, func() (application.RecordValidationRunResult, error) {
			return cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
	})

	t.Run("C11 RecordValidationClaim", func(t *testing.T) {
		f := newCommandFixture()
		seedValidationFoundationForReplay(t, f)
		run := validationRunForReplay("ER-C11")
		if _, err := run.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		cmd := validationClaimForReplay("CLM-REPLAY", "ER-C11")
		assertAdvancingClockReplay(t, f, func() (application.RecordValidationClaimResult, error) {
			return cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
	})

	t.Run("C12 CorrectValidationClaim", func(t *testing.T) {
		f := newCommandFixture()
		seedValidationFoundationForReplay(t, f)
		run := validationRunForReplay("ER-C12")
		if _, err := run.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		target := validationClaimForReplay("CLM-TARGET", "ER-C12")
		if _, err := target.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		cmd := application.CorrectValidationClaimCommand{
			ClaimID: "CLM-CORRECTION", CorrectionTarget: "CLM-TARGET", CorrectionKind: "correct",
			ScopeArtifactID: "CAP-1", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
			RequirementArtifactID: "REQ-REPLAY", RequirementRevisionID: "REQ-REPLAY-REV-1",
			Outcome: "not-satisfied", Method: "manual-review",
			EvidenceArtifactID: "EV-ER-C12", EvidenceRevisionID: "EV-ER-C12-REV-1", ExecutionID: "ER-C12",
			Reasoning: "Correction remains stable on replay.",
		}
		assertAdvancingClockReplay(t, f, func() (application.CorrectValidationClaimResult, error) {
			return cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
	})
}

func TestExplicitCallerTimesReplayAndConflict(t *testing.T) {
	ctx := context.Background()
	explicit := time.Date(2026, 3, 2, 10, 11, 12, 345678000, time.UTC)

	t.Run("C5 effective_at", func(t *testing.T) {
		f := newCommandFixture()
		setupProjectAndFeature(t, f)
		establishCapability(t, f)
		cmd := application.AcceptCapabilityRevisionCommand{
			RecordID: "CAP-MEMBER-TIME", ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-1",
			State: engineering.AcceptanceStateAccepted, Reason: "explicit time",
			EffectiveAt: explicit, HasEffectiveAt: true,
		}
		assertAdvancingClockReplay(t, f, func() (application.AcceptCapabilityRevisionResult, error) {
			return cmd.Execute(ctx, f.uow, f.rec, f.clock)
		})
		conflict := cmd
		conflict.EffectiveAt = explicit.Add(time.Second)
		if _, err := conflict.Execute(ctx, f.uow, f.rec, f.clock); !errors.Is(err, application.ErrImmutableValueConflict) {
			t.Fatalf("changed explicit effective_at err = %v, want immutable conflict", err)
		}
	})

	t.Run("C6 effective attempted and completed times", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		entry := application.AssignLifecycleStateCommand{
			AssignmentID: "SA-TIME-ENTRY", SubjectArtifactID: "CAP-1", State: "drafting", IsEntry: true,
			TransitionRecordArtifactID: "TR-TIME", TransitionRecordRevisionID: "TR-TIME-REV-0",
			EffectiveAt: explicit, HasEffectiveAt: true,
		}
		if _, err := entry.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		cmd := application.AssignLifecycleStateCommand{
			AssignmentID: "SA-TIME-TRANSITION", SubjectArtifactID: "CAP-1", State: "under-validation",
			TransitionRecordArtifactID: "TR-TIME", TransitionRecordRevisionID: "TR-TIME-REV-1",
			TransitionKey: "begin-validation", FromAssignmentID: "SA-TIME-ENTRY",
			EffectiveAt: explicit.Add(time.Minute), HasEffectiveAt: true,
			AttemptedAt: explicit.Add(2 * time.Minute), HasAttemptedAt: true,
			CompletedAt: explicit.Add(3 * time.Minute), HasCompletedAt: true,
		}
		assertAdvancingClockReplay(t, f, func() (application.AssignLifecycleStateResult, error) {
			return cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
		conflict := cmd
		conflict.CompletedAt = cmd.CompletedAt.Add(time.Second)
		if _, err := conflict.Execute(ctx, f.uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrImmutableValueConflict) {
			t.Fatalf("changed explicit completed_at err = %v, want immutable conflict", err)
		}
	})

	t.Run("C10 completed_at", func(t *testing.T) {
		f := newCommandFixture()
		seedValidationFoundationForReplay(t, f)
		cmd := validationRunForReplay("ER-TIME")
		cmd.CompletedAt, cmd.HasCompletedAt = explicit, true
		assertAdvancingClockReplay(t, f, func() (application.RecordValidationRunResult, error) {
			return cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
		conflict := cmd
		conflict.CompletedAt = explicit.Add(time.Second)
		if _, err := conflict.Execute(ctx, f.uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrImmutableValueConflict) {
			t.Fatalf("changed explicit completed_at err = %v, want immutable conflict", err)
		}
	})

	t.Run("C11 timestamp", func(t *testing.T) {
		f := newCommandFixture()
		seedValidationFoundationForReplay(t, f)
		run := validationRunForReplay("ER-TIME-CLAIM")
		if _, err := run.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		cmd := validationClaimForReplay("CLM-TIME", "ER-TIME-CLAIM")
		cmd.Timestamp, cmd.HasTimestamp = explicit, true
		assertAdvancingClockReplay(t, f, func() (application.RecordValidationClaimResult, error) {
			return cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
		conflict := cmd
		conflict.Timestamp = explicit.Add(time.Second)
		if _, err := conflict.Execute(ctx, f.uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrImmutableValueConflict) {
			t.Fatalf("changed explicit timestamp err = %v, want immutable conflict", err)
		}
	})

	t.Run("C12 timestamp", func(t *testing.T) {
		f := newCommandFixture()
		seedValidationFoundationForReplay(t, f)
		run := validationRunForReplay("ER-TIME-CORRECTION")
		if _, err := run.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		target := validationClaimForReplay("CLM-TIME-TARGET", "ER-TIME-CORRECTION")
		if _, err := target.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		cmd := application.CorrectValidationClaimCommand{
			ClaimID: "CLM-TIME-CORRECTION", CorrectionTarget: "CLM-TIME-TARGET", CorrectionKind: "correct",
			ScopeArtifactID: "CAP-1", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
			RequirementArtifactID: "REQ-REPLAY", RequirementRevisionID: "REQ-REPLAY-REV-1",
			Outcome: "not-satisfied", Method: "manual-review",
			EvidenceArtifactID: "EV-ER-TIME-CORRECTION", EvidenceRevisionID: "EV-ER-TIME-CORRECTION-REV-1",
			ExecutionID: "ER-TIME-CORRECTION", Reasoning: "Explicit correction time.",
			Timestamp: explicit, HasTimestamp: true,
		}
		assertAdvancingClockReplay(t, f, func() (application.CorrectValidationClaimResult, error) {
			return cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
		conflict := cmd
		conflict.Timestamp = explicit.Add(time.Second)
		if _, err := conflict.Execute(ctx, f.uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrImmutableValueConflict) {
			t.Fatalf("changed explicit timestamp err = %v, want immutable conflict", err)
		}
	})
}

func TestC7CallerMemberIdentityAndReplayOnlyOmission(t *testing.T) {
	f := newCommandFixture()
	seedCapability(t, f)
	ctx := context.Background()
	base := application.EstablishRequirementCommand{
		ArtifactID: "REQ-IDENTITY", RevisionID: "REQ-IDENTITY-REV-1",
		Statement: "The system SHALL preserve caller identity.", SubjectArtifactID: "CAP-1",
	}

	releaseWriteGuard := forbidPersistenceWrites(f)
	_, err := base.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
	releaseWriteGuard()
	if !errors.Is(err, application.ErrInvalidCommand) {
		t.Fatalf("new C7 without member ID err = %v, want invalid command", err)
	}

	base.AcceptanceRecordID = memberID("REQ-MEMBER-CALLER")
	if _, err := base.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("new C7 with caller member: %v", err)
	}
	assertAcceptanceMember(t, f, "REQ-IDENTITY", "REQ-IDENTITY-REV-1", "REQ-MEMBER-CALLER")

	f.clock.Advance(time.Hour)
	releaseWriteGuard = forbidPersistenceWrites(f)
	defer releaseWriteGuard()
	if _, err := base.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("C7 exact member replay: %v", err)
	}
	omittedReplay := base
	omittedReplay.AcceptanceRecordID = nil
	if _, err := omittedReplay.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("C7 replay-only omission: %v", err)
	}
	differentMember := base
	differentMember.AcceptanceRecordID = memberID("REQ-MEMBER-DIFFERENT")
	if _, err := differentMember.Execute(ctx, f.uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrImmutableValueConflict) {
		t.Fatalf("C7 different member err = %v, want immutable conflict", err)
	}
	malformedMember := base
	malformedMember.AcceptanceRecordID = memberID("not valid")
	if _, err := malformedMember.Execute(ctx, f.uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrInvalidCommand) {
		t.Fatalf("C7 malformed non-matching member err = %v, want invalid command", err)
	}
}

func TestC9CallerMemberIdentityAndRequiredReplayPresence(t *testing.T) {
	f := newCommandFixture()
	seedCapability(t, f)
	seedRequirementForReplay(t, f)
	ctx := context.Background()
	base := application.EstablishValidationPlanCommand{
		ArtifactID: "VP-IDENTITY", RevisionID: "VP-IDENTITY-REV-1", ScopeArtifactID: "CAP-1",
		Activities: []application.PlanActivityCommandInput{{
			Key: "A-IDENTITY", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
			Method: "manual-review", OutcomeInterpretation: "Member identity is caller-owned.",
			RequirementArtifactID: "REQ-REPLAY", RequirementRevisionID: "REQ-REPLAY-REV-1",
		}},
	}

	releaseWriteGuard := forbidPersistenceWrites(f)
	_, err := base.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
	releaseWriteGuard()
	if !errors.Is(err, application.ErrInvalidCommand) {
		t.Fatalf("new C9 without member ID err = %v, want invalid command", err)
	}
	base.AcceptanceRecordID = memberID("PLAN-MEMBER-CALLER")
	if _, err := base.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("new C9 with caller member: %v", err)
	}
	assertAcceptanceMember(t, f, "VP-IDENTITY", "VP-IDENTITY-REV-1", "PLAN-MEMBER-CALLER")

	f.clock.Advance(time.Hour)
	releaseWriteGuard = forbidPersistenceWrites(f)
	defer releaseWriteGuard()
	if _, err := base.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("C9 exact member replay: %v", err)
	}
	omittedReplay := base
	omittedReplay.AcceptanceRecordID = nil
	if _, err := omittedReplay.Execute(ctx, f.uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrInvalidCommand) {
		t.Fatalf("C9 replay without required member ID err = %v, want invalid command", err)
	}
	differentMember := base
	differentMember.AcceptanceRecordID = memberID("PLAN-MEMBER-DIFFERENT")
	if _, err := differentMember.Execute(ctx, f.uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrImmutableValueConflict) {
		t.Fatalf("C9 different member err = %v, want immutable conflict", err)
	}

	var current application.CurrentRevisionResult
	if err := f.uow.Do(ctx, func(r application.Repositories) error {
		var err error
		current, err = application.ResolveCurrentRevision(ctx, r, "VP-IDENTITY")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !current.Found || current.Revision.Key.RevisionID != "VP-IDENTITY-REV-1" || current.Sequence != 1 {
		t.Fatalf("current plan revision = %+v, want accepted VP-IDENTITY-REV-1 sequence 1", current)
	}
}

func TestC6CorruptDifferentAssignmentPrecedesPrimaryConflict(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)

	primary := application.AssignLifecycleStateCommand{
		AssignmentID: "SA-PRIMARY", SubjectArtifactID: "CAP-1", State: "drafting", IsEntry: true,
		TransitionRecordArtifactID: "TR-PRIMARY", TransitionRecordRevisionID: "TR-PRIMARY-REV-0",
	}
	if _, err := primary.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("seed primary lifecycle act: %v", err)
	}

	if _, err := (application.CreateFeatureCommand{
		FeatureCardID: "FC-CANDIDATE", ProjectID: "PRJ-1", Title: "Candidate subject",
	}).Execute(ctx, f.uow, f.clock); err != nil {
		t.Fatalf("seed candidate feature: %v", err)
	}
	if _, err := (application.EstablishCapabilitySpecificationCommand{
		FeatureCardID: "FC-CANDIDATE", ArtifactID: "CAP-CANDIDATE", RevisionID: "CAP-CANDIDATE-REV-1",
		Content: mustContent(t, "Candidate capability"),
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("seed candidate capability: %v", err)
	}
	candidate := application.AssignLifecycleStateCommand{
		AssignmentID: "SA-CANDIDATE", SubjectArtifactID: "CAP-CANDIDATE", State: "drafting", IsEntry: true,
		TransitionRecordArtifactID: "TR-CANDIDATE", TransitionRecordRevisionID: "TR-CANDIDATE-REV-0",
	}
	if _, err := candidate.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("seed candidate lifecycle act: %v", err)
	}

	uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
		r.Artifacts = missingArtifactRepository{
			ArtifactEnvelopeRepository: r.Artifacts,
			missing:                    engineering.ArtifactKey{ArtifactID: "TR-CANDIDATE"},
		}
	}}
	release := forbidPersistenceWrites(f)
	defer release()

	conflicting := primary
	conflicting.AssignmentID = candidate.AssignmentID
	if _, err := conflicting.Execute(ctx, uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("err = %v, want corrupt candidate ErrStoredStateIntegrity before primary conflict", err)
	}
}

func TestC6CorruptPrimaryOwnerPrecedesDifferentAssignmentConflict(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)
	primary := application.AssignLifecycleStateCommand{
		AssignmentID: "SA-PRIMARY", SubjectArtifactID: "CAP-1", State: "drafting", IsEntry: true,
		TransitionRecordArtifactID: "TR-PRIMARY", TransitionRecordRevisionID: "TR-PRIMARY-REV-0",
	}
	if _, err := primary.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("seed primary lifecycle act: %v", err)
	}
	uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
		r.Artifacts = missingArtifactRepository{
			ArtifactEnvelopeRepository: r.Artifacts,
			missing:                    engineering.ArtifactKey{ArtifactID: "TR-PRIMARY"},
		}
	}}
	release := forbidPersistenceWrites(f)
	defer release()
	conflicting := primary
	conflicting.AssignmentID = "SA-FREE"
	if _, err := conflicting.Execute(ctx, uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("err = %v, want primary ErrStoredStateIntegrity before assignment conflict", err)
	}
}

func TestC6EntryRejectsTransitionOnlyFields(t *testing.T) {
	ctx := context.Background()
	for name, mutate := range map[string]func(*application.AssignLifecycleStateCommand){
		"transition key": func(c *application.AssignLifecycleStateCommand) { c.TransitionKey = "draft-to-active" },
		"predecessor":    func(c *application.AssignLifecycleStateCommand) { c.FromAssignmentID = "SA-OLD" },
		"attempted at": func(c *application.AssignLifecycleStateCommand) {
			c.AttemptedAt = time.Date(2026, 3, 1, 1, 0, 0, 0, time.UTC)
			c.HasAttemptedAt = true
		},
		"completed at": func(c *application.AssignLifecycleStateCommand) {
			c.CompletedAt = time.Date(2026, 3, 1, 1, 1, 0, 0, time.UTC)
			c.HasCompletedAt = true
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newCommandFixture()
			cmd := application.AssignLifecycleStateCommand{
				AssignmentID: "SA-ENTRY", SubjectArtifactID: "CAP-1", State: "drafting", IsEntry: true,
				TransitionRecordArtifactID: "TR-ENTRY", TransitionRecordRevisionID: "TR-ENTRY-REV-0",
			}
			mutate(&cmd)
			release := forbidPersistenceWrites(f)
			defer release()
			if _, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrInvalidCommand) {
				t.Fatalf("err = %v, want ErrInvalidCommand", err)
			}
		})
	}
}

func TestC6SharedTransitionArtifactMustHaveCompleteHistory(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)
	artifact, _, _, err := f.rec.RecordEntryAssignment(engineering.EntryAssignmentInput{
		AssignmentID: "SA-UNWRITTEN", SubjectArtifactID: "CAP-1", State: "drafting",
		EffectiveAt: f.clock.Now(), TransitionRecordArtifactID: "TR-PARTIAL",
		TransitionRecordRevisionID: "TR-PARTIAL-REV-0", RecordedAt: f.clock.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.uow.Do(ctx, func(r application.Repositories) error {
		return r.Artifacts.Put(ctx, artifact)
	}); err != nil {
		t.Fatalf("seed partial transition artifact: %v", err)
	}
	cmd := application.AssignLifecycleStateCommand{
		AssignmentID: "SA-NEW", SubjectArtifactID: "CAP-1", State: "drafting", IsEntry: true,
		TransitionRecordArtifactID: "TR-PARTIAL", TransitionRecordRevisionID: "TR-PARTIAL-REV-1",
	}
	release := forbidPersistenceWrites(f)
	defer release()
	if _, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("err = %v, want partial shared-root ErrStoredStateIntegrity", err)
	}
}

type appendedStateAssignmentRepository struct {
	application.RecordEnvelopeRepository
	extra engineering.RecordEnvelope
}

type appendedRecordRepository struct {
	application.RecordEnvelopeRepository
	kind  engineering.RecordKind
	extra engineering.RecordEnvelope
}

func (r appendedRecordRepository) ListByKind(ctx context.Context, kind engineering.RecordKind) ([]engineering.RecordEnvelope, error) {
	stored, err := r.RecordEnvelopeRepository.ListByKind(ctx, kind)
	if err != nil || kind != r.kind {
		return stored, err
	}
	return append(stored, r.extra), nil
}

func (r appendedStateAssignmentRepository) ListByKind(ctx context.Context, kind engineering.RecordKind) ([]engineering.RecordEnvelope, error) {
	stored, err := r.RecordEnvelopeRepository.ListByKind(ctx, kind)
	if err != nil || kind != engineering.RecordKindStateAssignment {
		return stored, err
	}
	return append(stored, r.extra), nil
}

func TestC6SharedTransitionArtifactRejectsOrphanReverseAssignment(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)
	first := application.AssignLifecycleStateCommand{
		AssignmentID: "SA-FIRST", SubjectArtifactID: "CAP-1", State: "drafting", IsEntry: true,
		TransitionRecordArtifactID: "TR-SHARED", TransitionRecordRevisionID: "TR-SHARED-REV-0",
	}
	if _, err := first.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("seed first lifecycle act: %v", err)
	}
	_, _, phantom, err := f.rec.RecordEntryAssignment(engineering.EntryAssignmentInput{
		AssignmentID: "SA-PHANTOM", SubjectArtifactID: "CAP-1", State: "drafting",
		EffectiveAt: f.clock.Now(), TransitionRecordArtifactID: "TR-SHARED",
		TransitionRecordRevisionID: "TR-SHARED-GHOST", RecordedAt: f.clock.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
		r.Records = appendedStateAssignmentRepository{RecordEnvelopeRepository: r.Records, extra: phantom}
	}}
	next := application.AssignLifecycleStateCommand{
		AssignmentID: "SA-NEXT", SubjectArtifactID: "CAP-1", State: "specified",
		TransitionRecordArtifactID: "TR-SHARED", TransitionRecordRevisionID: "TR-SHARED-REV-1",
		TransitionKey: "specify", FromAssignmentID: "SA-FIRST",
	}
	release := forbidPersistenceWrites(f)
	defer release()
	if _, err := next.Execute(ctx, uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("err = %v, want orphan reverse assignment ErrStoredStateIntegrity", err)
	}
}

func TestC6ReplayRejectsSiblingTransitionWithoutAssignment(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)
	first := application.AssignLifecycleStateCommand{
		AssignmentID: "SA-FIRST-REPLAY", SubjectArtifactID: "CAP-1", State: "drafting", IsEntry: true,
		TransitionRecordArtifactID: "TR-SHARED-REPLAY", TransitionRecordRevisionID: "TR-SHARED-REPLAY-REV-0",
	}
	if _, err := first.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("seed first lifecycle act: %v", err)
	}
	_, sibling, _, err := f.rec.RecordTransition(engineering.TransitionInput{
		AssignmentID: "SA-UNWRITTEN-SIBLING", SubjectArtifactID: "CAP-1", State: "specified",
		EffectiveAt: f.clock.Now(), TransitionRecordArtifactID: "TR-SHARED-REPLAY",
		TransitionRecordRevisionID: "TR-SHARED-REPLAY-REV-1", TransitionKey: "specify",
		FromAssignmentID: "SA-FIRST-REPLAY", AttemptedAt: f.clock.Now(), CompletedAt: f.clock.Now(), RecordedAt: f.clock.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.uow.Do(ctx, func(r application.Repositories) error {
		return r.Revisions.Put(ctx, sibling)
	}); err != nil {
		t.Fatalf("seed ownerless sibling transition: %v", err)
	}
	release := forbidPersistenceWrites(f)
	defer release()
	if _, err := first.Execute(ctx, f.uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("err = %v, want corrupt shared-history ErrStoredStateIntegrity", err)
	}
}

func TestC6MissingPrimaryRevisionPrecedesOccupiedAssignmentConflict(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)
	if _, err := (application.CreateFeatureCommand{
		FeatureCardID: "FC-CANDIDATE-2", ProjectID: "PRJ-1", Title: "Candidate subject two",
	}).Execute(ctx, f.uow, f.clock); err != nil {
		t.Fatal(err)
	}
	if _, err := (application.EstablishCapabilitySpecificationCommand{
		FeatureCardID: "FC-CANDIDATE-2", ArtifactID: "CAP-CANDIDATE-2", RevisionID: "CAP-CANDIDATE-2-REV-1",
		Content: mustContent(t, "Candidate capability two"),
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	candidate := application.AssignLifecycleStateCommand{
		AssignmentID: "SA-CANDIDATE-2", SubjectArtifactID: "CAP-CANDIDATE-2", State: "drafting", IsEntry: true,
		TransitionRecordArtifactID: "TR-CANDIDATE-2", TransitionRecordRevisionID: "TR-CANDIDATE-2-REV-0",
	}
	if _, err := candidate.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	_, _, danglingOwner, err := f.rec.RecordEntryAssignment(engineering.EntryAssignmentInput{
		AssignmentID: "SA-DANGLING-OWNER", SubjectArtifactID: "CAP-1", State: "drafting",
		EffectiveAt: f.clock.Now(), TransitionRecordArtifactID: "TR-MISSING-PRIMARY",
		TransitionRecordRevisionID: "TR-MISSING-PRIMARY-REV-0", RecordedAt: f.clock.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
		r.Records = appendedStateAssignmentRepository{RecordEnvelopeRepository: r.Records, extra: danglingOwner}
	}}
	request := application.AssignLifecycleStateCommand{
		AssignmentID: candidate.AssignmentID, SubjectArtifactID: "CAP-1", State: "drafting", IsEntry: true,
		TransitionRecordArtifactID: "TR-MISSING-PRIMARY", TransitionRecordRevisionID: "TR-MISSING-PRIMARY-REV-0",
	}
	release := forbidPersistenceWrites(f)
	defer release()
	if _, err := request.Execute(ctx, uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("err = %v, want partial primary ErrStoredStateIntegrity before assignment conflict", err)
	}
}

func TestC10ExecutionOwnerWithoutEvidenceIsPartialOccupancy(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedValidationFoundationForReplay(t, f)
	phantom, err := f.rec.RecordExecution(engineering.ExecutionInput{
		ExecutionID: "ER-DANGLING", PlanArtifactID: "VP-REPLAY", PlanRevisionID: "VP-REPLAY-REV-1",
		ActivityKey: "A-REPLAY", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		Method: "manual-review", Outcome: "completed", CompletedAt: f.clock.Now(),
		EvidenceArtifactID: "EV-DANGLING", EvidenceRevisionID: "EV-DANGLING-REV-1", RecordedAt: f.clock.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
		r.Records = appendedRecordRepository{
			RecordEnvelopeRepository: r.Records, kind: engineering.RecordKindExecution, extra: phantom,
		}
	}}
	request := validationRunForReplay("ER-NEW-ON-DANGLING")
	request.EvidenceArtifactID = "EV-DANGLING"
	request.EvidenceRevisionID = "EV-DANGLING-REV-1"
	release := forbidPersistenceWrites(f)
	defer release()
	if _, err := request.Execute(ctx, uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("err = %v, want execution-only ErrStoredStateIntegrity", err)
	}
}

func TestC10RejectsExecutionForMissingSiblingEvidenceRevision(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedValidationFoundationForReplay(t, f)
	cmd := validationRunForReplay("ER-OWNER")
	if _, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("seed validation run: %v", err)
	}
	phantom, err := f.rec.RecordExecution(engineering.ExecutionInput{
		ExecutionID: "ER-GHOST", PlanArtifactID: "VP-REPLAY", PlanRevisionID: "VP-REPLAY-REV-1",
		ActivityKey: "A-REPLAY", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		Method: "manual-review", Outcome: "completed", CompletedAt: f.clock.Now(),
		EvidenceArtifactID: cmd.EvidenceArtifactID, EvidenceRevisionID: "EV-GHOST-REVISION", RecordedAt: f.clock.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
		r.Records = appendedRecordRepository{
			RecordEnvelopeRepository: r.Records, kind: engineering.RecordKindExecution, extra: phantom,
		}
	}}
	release := forbidPersistenceWrites(f)
	defer release()
	if _, err := cmd.Execute(ctx, uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("err = %v, want sibling-execution ErrStoredStateIntegrity", err)
	}
}

func TestC6RequiresCompleteCapabilitySubject(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	artifact, err := f.rec.RecordCapabilityArtifact("CAP-PARTIAL-SUBJECT", f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := f.uow.Do(ctx, func(r application.Repositories) error {
		return r.Artifacts.Put(ctx, artifact)
	}); err != nil {
		t.Fatalf("seed partial capability subject: %v", err)
	}
	cmd := application.AssignLifecycleStateCommand{
		AssignmentID: "SA-PARTIAL-SUBJECT", SubjectArtifactID: "CAP-PARTIAL-SUBJECT", State: "drafting", IsEntry: true,
		TransitionRecordArtifactID: "TR-PARTIAL-SUBJECT", TransitionRecordRevisionID: "TR-PARTIAL-SUBJECT-REV-0",
	}
	release := forbidPersistenceWrites(f)
	defer release()
	if _, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("err = %v, want partial capability ErrStoredStateIntegrity", err)
	}
}

func TestC10ForeignPartialArtifactAndExtraRevisionAreIntegrityFailures(t *testing.T) {
	ctx := context.Background()

	t.Run("foreign artifact-only occupancy", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		artifact, _, err := f.rec.RecordRequirement(engineering.RequirementInput{
			ArtifactID: "EV-FOREIGN-PARTIAL", RevisionID: "REQ-UNWRITTEN",
			Statement: "The system SHALL expose partial foreign occupancy.", SubjectArtifactID: "CAP-1",
			RecordedAt: f.clock.Now(),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := f.uow.Do(ctx, func(r application.Repositories) error {
			return r.Artifacts.Put(ctx, artifact)
		}); err != nil {
			t.Fatalf("seed partial foreign artifact: %v", err)
		}
		cmd := validationRunForReplay("ER-FOREIGN-PARTIAL")
		cmd.EvidenceArtifactID = artifact.Key.ArtifactID
		cmd.EvidenceRevisionID = "EV-FOREIGN-PARTIAL-REV-1"
		release := forbidPersistenceWrites(f)
		defer release()
		if _, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrStoredStateIntegrity) {
			t.Fatalf("err = %v, want partial foreign ErrStoredStateIntegrity", err)
		}
	})

	t.Run("complete run with an extra evidence revision", func(t *testing.T) {
		f := newCommandFixture()
		seedValidationFoundationForReplay(t, f)
		cmd := validationRunForReplay("ER-EXTRA-REVISION")
		if _, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatalf("seed validation run: %v", err)
		}
		_, extra, err := f.rec.RecordEvidence(engineering.EvidenceInput{
			ArtifactID: cmd.EvidenceArtifactID, RevisionID: cmd.EvidenceRevisionID + "-EXTRA",
			Locator: cmd.EvidenceLocator, RecordedAt: f.clock.Now(),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := f.uow.Do(ctx, func(r application.Repositories) error {
			return r.Revisions.Put(ctx, extra)
		}); err != nil {
			t.Fatalf("seed extra evidence revision: %v", err)
		}
		release := forbidPersistenceWrites(f)
		defer release()
		if _, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrStoredStateIntegrity) {
			t.Fatalf("err = %v, want extra-revision ErrStoredStateIntegrity", err)
		}
	})
}

func TestC9RequiresCompleteActivityRevisionActs(t *testing.T) {
	ctx := context.Background()

	t.Run("new plan rejects orphan requirement revision", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		if _, err := (application.EstablishRequirementCommand{
			ArtifactID: "REQ-ORPHAN", RevisionID: "REQ-ORPHAN-REV-1",
			Statement: "The system SHALL reject orphan criteria.", SubjectArtifactID: "CAP-1",
			AcceptanceRecordID: memberID("MEM-REQ-ORPHAN"),
		}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatalf("seed requirement: %v", err)
		}
		cmd := application.EstablishValidationPlanCommand{
			ArtifactID: "VP-ORPHAN", RevisionID: "VP-ORPHAN-REV-1", ScopeArtifactID: "CAP-1",
			AcceptanceRecordID: memberID("MEM-VP-ORPHAN"),
			Activities: []application.PlanActivityCommandInput{{
				Key: "A-ORPHAN", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
				Method: "manual-review", OutcomeInterpretation: "Reject incomplete references.",
				RequirementArtifactID: "REQ-ORPHAN", RequirementRevisionID: "REQ-ORPHAN-REV-1",
			}},
		}
		uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
			r.Artifacts = missingArtifactRepository{
				ArtifactEnvelopeRepository: r.Artifacts,
				missing:                    engineering.ArtifactKey{ArtifactID: "REQ-ORPHAN"},
			}
		}}
		release := forbidPersistenceWrites(f)
		defer release()
		if _, err := cmd.Execute(ctx, uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrStoredStateIntegrity) {
			t.Fatalf("err = %v, want orphan requirement ErrStoredStateIntegrity", err)
		}
	})

	t.Run("replay rejects a now-dangling requirement act", func(t *testing.T) {
		f := newCommandFixture()
		seedValidationFoundationForReplay(t, f)
		cmd := application.EstablishValidationPlanCommand{
			ArtifactID: "VP-REPLAY", RevisionID: "VP-REPLAY-REV-1", ScopeArtifactID: "CAP-1",
			AcceptanceRecordID: memberID("PLAN-MEMBER-SEED"),
			Activities: []application.PlanActivityCommandInput{{
				Key: "A-REPLAY", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
				Method: "manual-review", OutcomeInterpretation: "Satisfied when replay is stable.",
				RequirementArtifactID: "REQ-REPLAY", RequirementRevisionID: "REQ-REPLAY-REV-1",
				ExpectedEvidence: []string{"replay report"},
			}},
		}
		uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
			r.Artifacts = missingArtifactRepository{
				ArtifactEnvelopeRepository: r.Artifacts,
				missing:                    engineering.ArtifactKey{ArtifactID: "REQ-REPLAY"},
			}
		}}
		release := forbidPersistenceWrites(f)
		defer release()
		if _, err := cmd.Execute(ctx, uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrStoredStateIntegrity) {
			t.Fatalf("err = %v, want dangling requirement ErrStoredStateIntegrity", err)
		}
	})
}

func assertAcceptanceMember(t *testing.T, f commandFixture, artifactID, revisionID, recordID string) {
	t.Helper()
	ctx := context.Background()
	key := mustRevKey(t, artifactID, revisionID)
	var journal []engineering.RevisionAcceptanceRecord
	var order engineering.RevisionOrderMetadata
	var orderFound bool
	if err := f.uow.Do(ctx, func(r application.Repositories) error {
		var err error
		journal, err = r.RevisionAcceptance.ListByRevision(ctx, key)
		if err != nil {
			return err
		}
		order, orderFound, err = r.RevisionOrder.Get(ctx, key)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(journal) != 1 {
		t.Fatalf("acceptance journal = %+v, want exactly one member", journal)
	}
	if journal[0].RecordID != recordID || journal[0].State != engineering.AcceptanceStateAccepted {
		t.Fatalf("acceptance member = %+v, want accepted %s", journal[0], recordID)
	}
	if !orderFound || order.Sequence != 1 {
		t.Fatalf("order = %+v, found=%v, want sequence 1", order, orderFound)
	}
}
