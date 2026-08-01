package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/domain"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

// seedReadSurfaceFixture establishes a capability with one requirement, one
// decision, one validation plan, and one satisfied claim -- enough to
// exercise every FF-020 read-surface projection in one pass. Mirrors
// internal/scenario's canonical identities where convenient, but is
// otherwise a minimal fixture local to this file.
func seedReadSurfaceFixture(t *testing.T, f commandFixture) {
	t.Helper()
	ctx := context.Background()
	seedCapability(t, f) // PRJ-1, FC-1, CAP-1/CAP-1-REV-1 (accepted)

	if _, err := (application.EstablishRequirementCommand{
		ArtifactID: "REQ-1", RevisionID: "REQ-1-REV-1",
		Statement: "Published homework SHALL be visible to the student.", SubjectArtifactID: "CAP-1",
		SourceCapabilityRevisionID: "CAP-1-REV-1", SourceAcceptanceCriterionKey: "AC-1",
		AcceptanceRecordID: memberID("MEM-REQ-1"),
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("EstablishRequirement: %v", err)
	}

	if err := f.uow.Do(ctx, func(r application.Repositories) error {
		artEnv, revEnv, err := f.rec.RecordEvidence(engineering.EvidenceInput{
			ArtifactID: "EV-0", RevisionID: "EV-0-REV-1", Locator: "https://evidence.example/EV-0", RecordedAt: f.clock.Now(),
		})
		if err != nil {
			return err
		}
		if err := r.Artifacts.Put(ctx, artEnv); err != nil {
			return err
		}
		return r.Revisions.Put(ctx, revEnv)
	}); err != nil {
		t.Fatalf("seeding decision evidence: %v", err)
	}

	if _, err := (application.RecordArchitectureDecisionCommand{
		DecisionID: "DEC-1", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		Question: "Should homework support audio?", OutcomeStatement: "Yes, by content address.",
		Alternatives:       []string{"store inline", "store externally"},
		EvidenceArtifactID: "EV-0", EvidenceRevisionID: "EV-0-REV-1",
		Assumptions: []string{"media hosted externally"}, Constraints: []string{"no binary storage"},
		Uncertainties: []string{"small sample"}, Rationale: "Avoids scope creep.",
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("RecordArchitectureDecision: %v", err)
	}

	if _, err := (application.EstablishValidationPlanCommand{
		ArtifactID: "VP-1", RevisionID: "VP-1-REV-1", ScopeArtifactID: "CAP-1",
		AcceptanceRecordID: memberID("MEM-PLAN-1"),
		Activities: []application.PlanActivityCommandInput{{
			Key: "A-1", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
			Method: "manual-review", OutcomeInterpretation: "Satisfied when reviewed.",
			RequirementArtifactID: "REQ-1", RequirementRevisionID: "REQ-1-REV-1",
			ExpectedEvidence: []string{"reviewer note"},
		}},
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("EstablishValidationPlan: %v", err)
	}

	if _, err := (application.RecordValidationRunCommand{
		ExecutionID: "ER-1", PlanArtifactID: "VP-1", PlanRevisionID: "VP-1-REV-1", ActivityKey: "A-1",
		SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		Method: "manual-review", Outcome: "completed",
		EvidenceArtifactID: "EV-1", EvidenceRevisionID: "EV-1-REV-1", EvidenceLocator: "https://evidence.example/EV-1",
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("RecordValidationRun: %v", err)
	}

	if _, err := (application.RecordValidationClaimCommand{
		ClaimID: "CLM-1", ScopeArtifactID: "CAP-1",
		SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		RequirementArtifactID: "REQ-1", RequirementRevisionID: "REQ-1-REV-1",
		Outcome: "satisfied", Method: "manual-review",
		EvidenceArtifactID: "EV-1", EvidenceRevisionID: "EV-1-REV-1", ExecutionID: "ER-1",
		Reasoning: "The specification states it explicitly.",
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("RecordValidationClaim: %v", err)
	}
}

// TestGetFeatureEngineeringStateForCard_RendersReadSurfaceContent proves
// every FF-020 projection reaches EngineeringStateResult through the same
// UOW-driven entry point Q3/Q4 use: requirement statement, decision basis,
// validation plan activities, and the current claim's reasoning and
// correction attribution -- none of which the pre-FF-020 result carried.
func TestGetFeatureEngineeringStateForCard_RendersReadSurfaceContent(t *testing.T) {
	f := newCommandFixture()
	seedReadSurfaceFixture(t, f)

	state, err := application.GetFeatureEngineeringStateForCard(context.Background(), f.uow, f.rec, f.rec, mustFeatureCardID(t, "FC-1"))
	if err != nil {
		t.Fatalf("GetFeatureEngineeringStateForCard: %v", err)
	}

	if len(state.EffectiveRequirements) != 1 || state.EffectiveRequirements[0].Statement != "Published homework SHALL be visible to the student." {
		t.Fatalf("EffectiveRequirements = %+v", state.EffectiveRequirements)
	}

	if len(state.ApplicableDecisions) != 1 {
		t.Fatalf("ApplicableDecisions = %+v", state.ApplicableDecisions)
	}
	detail := state.ApplicableDecisions[0].Detail
	if detail.Question != "Should homework support audio?" || detail.OutcomeStatement != "Yes, by content address." {
		t.Errorf("decision detail = %+v", detail)
	}
	if len(detail.Alternatives) != 2 || len(detail.Assumptions) != 1 || len(detail.Constraints) != 1 || len(detail.Uncertainties) != 1 {
		t.Errorf("decision basis lists = %+v", detail)
	}

	if !state.ValidationPlan.Found || state.ValidationPlan.ArtifactID != "VP-1" || state.ValidationPlan.RevisionID != "VP-1-REV-1" {
		t.Fatalf("ValidationPlan = %+v", state.ValidationPlan)
	}
	if len(state.ValidationPlan.Activities) != 1 || state.ValidationPlan.Activities[0].Key != "A-1" {
		t.Fatalf("ValidationPlan.Activities = %+v", state.ValidationPlan.Activities)
	}
	if state.ValidationPlan.Activities[0].OutcomeInterpretation != "Satisfied when reviewed." {
		t.Errorf("activity outcome interpretation = %q", state.ValidationPlan.Activities[0].OutcomeInterpretation)
	}
	if state.ValidationPlan.Rationale.Rule == "" || state.ValidationPlan.Rationale.SelectedKey.RevisionID != "VP-1-REV-1" || state.ValidationPlan.Rationale.SelectedSequence != 1 {
		t.Errorf("ValidationPlan.Rationale = %+v, want selected VP-1-REV-1 at sequence 1", state.ValidationPlan.Rationale)
	}

	if len(state.Readiness.PerRequirement) != 1 {
		t.Fatalf("PerRequirement = %+v", state.Readiness.PerRequirement)
	}
	per := state.Readiness.PerRequirement[0]
	if !per.HasClaim || per.Reasoning != "The specification states it explicitly." {
		t.Errorf("per-requirement reasoning = %+v", per)
	}
	// CriterionKeys and the "corrects" relationship are the existing
	// RecordEnvelope.CriterionKeys / CorrectionTargetID projections
	// (FF-020 §5 reuses them as-is; no application-layer field was added).
	if len(per.Claim.CriterionKeys) == 0 {
		t.Error("expected Claim.CriterionKeys to be populated from the existing RecordEnvelope projection")
	}
	if per.Claim.HasCorrection() {
		t.Errorf("CLM-1 corrects nothing, got %+v", per.Claim)
	}
}

// TestGetFeatureEngineeringStateForCard_NoCapability_EmptyReadSurface
// proves a card with no linked capability yields the same well-formed
// empty state FF-018 established, now including the FF-020 fields: no
// requirements, no decisions, and ValidationPlan.Found false -- never an
// error.
func TestGetFeatureEngineeringStateForCard_NoCapability_EmptyReadSurface(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	if _, err := (application.CreateProjectCommand{ProjectID: "PRJ-1", Name: "Pilot"}).Execute(ctx, f.uow, f.clock); err != nil {
		t.Fatal(err)
	}
	if _, err := (application.CreateFeatureCommand{FeatureCardID: "FC-1", ProjectID: "PRJ-1", Title: "Homework"}).Execute(ctx, f.uow, f.clock); err != nil {
		t.Fatal(err)
	}

	state, err := application.GetFeatureEngineeringStateForCard(ctx, f.uow, f.rec, f.rec, mustFeatureCardID(t, "FC-1"))
	if err != nil {
		t.Fatalf("GetFeatureEngineeringStateForCard: %v", err)
	}
	if state.ValidationPlan.Found {
		t.Errorf("ValidationPlan = %+v, want not found", state.ValidationPlan)
	}
	if len(state.EffectiveRequirements) != 0 || len(state.ApplicableDecisions) != 0 {
		t.Errorf("expected no requirements or decisions, got %+v / %+v", state.EffectiveRequirements, state.ApplicableDecisions)
	}
}

// TestGetCapabilityRevision_ContentPresentAndAbsent proves Q7's content
// projection (FF-020 §2 class A): a revision established through the real
// command carries its content, and a revision with no stored content
// (impossible through this module's own commands, so simulated directly
// via the repository) reports HasContent false rather than an error.
func TestGetCapabilityRevision_ContentPresentAndAbsent(t *testing.T) {
	f := newCommandFixture()
	seedCapability(t, f)

	key := mustRevKey(t, "CAP-1", "CAP-1-REV-1")
	result, found, err := application.GetCapabilityRevision(context.Background(), f.uow, key)
	if err != nil {
		t.Fatalf("GetCapabilityRevision: %v", err)
	}
	if !found || !result.HasContent {
		t.Fatalf("result = %+v, found = %v, want found with content", result, found)
	}
	if result.Content.Title() != "Homework after a lesson" {
		t.Errorf("Content.Title() = %q", result.Content.Title())
	}

	// Simulate a revision recorded with no structured content stored --
	// e.g. a family other than capability, or a defect from outside this
	// module's own write path -- and confirm the empty state, not an error.
	if err := f.uow.Do(context.Background(), func(r application.Repositories) error {
		artEnv, revEnv, err := f.rec.RecordRequirement(engineering.RequirementInput{
			ArtifactID: "REQ-9", RevisionID: "REQ-9-REV-1", Statement: "Unrelated.", SubjectArtifactID: "CAP-1", RecordedAt: f.clock.Now(),
		})
		if err != nil {
			return err
		}
		if err := r.Artifacts.Put(context.Background(), artEnv); err != nil {
			return err
		}
		return r.Revisions.Put(context.Background(), revEnv)
	}); err != nil {
		t.Fatalf("seeding a content-free revision: %v", err)
	}
	noContentResult, found, err := application.GetCapabilityRevision(context.Background(), f.uow, mustRevKey(t, "REQ-9", "REQ-9-REV-1"))
	if err != nil {
		t.Fatalf("GetCapabilityRevision (no content): %v", err)
	}
	if !found {
		t.Fatal("expected the revision itself to be found")
	}
	if noContentResult.HasContent {
		t.Errorf("expected HasContent = false for a revision with no stored structured content, got %+v", noContentResult)
	}
}

// TestGetFeatureEngineeringStateForCard_UndecodablePayload proves a
// payload that will not decode surfaces ErrStoredPayloadUnreadable rather
// than a silently empty field (FF-020 §7).
func TestGetFeatureEngineeringStateForCard_UndecodablePayload(t *testing.T) {
	f := newCommandFixture()
	seedReadSurfaceFixture(t, f)

	// A second decision, naming CAP-1/CAP-1-REV-1 as subject like DEC-1,
	// with a payload that is valid JSON (NewRecordEnvelope itself requires
	// that) but will not decode back into a decision.Decision -- built by
	// recording a real, valid decision and corrupting its payload before
	// the record's one and only Put. Corrupting DEC-1 in place is not an
	// option: a RecordEnvelope's Put is create-only, exactly like a
	// RevisionEnvelope's, so a second Put with a differing payload under
	// the same key would conflict rather than overwrite. A decision needs
	// no order or acceptance metadata the way a requirement revision does
	// (FF-020's discovery reaches it by subject alone), which keeps this
	// fixture to the one Put a corrupted-from-birth record needs.
	if err := f.uow.Do(context.Background(), func(r application.Repositories) error {
		recEnv, err := f.rec.RecordDecision(engineering.DecisionInput{
			DecisionID: "DEC-CORRUPT", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
			Question: "Placeholder.", OutcomeStatement: "Placeholder, about to be corrupted.",
			EvidenceArtifactID: "EV-0", EvidenceRevisionID: "EV-0-REV-1", RecordedAt: f.clock.Now(),
		})
		if err != nil {
			return err
		}
		recEnv.Payload = []byte(`{"unexpected": "structure, not a decision.Decision"}`)
		return r.Records.Put(context.Background(), recEnv)
	}); err != nil {
		t.Fatalf("seeding a decision with an undecodable payload: %v", err)
	}

	_, err := application.GetFeatureEngineeringStateForCard(context.Background(), f.uow, f.rec, f.rec, mustFeatureCardID(t, "FC-1"))
	if !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("err = %v, want errors.Is(err, ErrStoredStateIntegrity)", err)
	}
}

func mustFeatureCardID(t *testing.T, id string) domain.FeatureCardID {
	t.Helper()
	v, err := domain.NewFeatureCardID(id)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func mustRevKey(t *testing.T, artifactID, revisionID string) engineering.RevisionKey {
	t.Helper()
	key, err := engineering.NewRevisionKey(artifactID, revisionID)
	if err != nil {
		t.Fatal(err)
	}
	return key
}
