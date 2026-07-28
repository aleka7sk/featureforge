package scenario_test

import (
	"context"
	"testing"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/domain"
	"github.com/aleka7sk/featureforge/internal/engineering"
	peos "github.com/aleka7sk/featureforge/internal/engineering/peos"
	"github.com/aleka7sk/featureforge/internal/infrastructure/memory"
	"github.com/aleka7sk/featureforge/internal/scenario"
)

// newFixture wires a fresh Store, UnitOfWork, PEOS-backed Recorder, and a
// FixedClock starting at the scenario's reference time -- the same
// composition root the application command tests use.
func newFixture() (application.UnitOfWork, peos.Recorder, *application.FixedClock) {
	uow := memory.NewUnitOfWork(memory.NewStore())
	return uow, peos.NewRecorder(), application.NewFixedClock(scenario.FixedStart)
}

// doQuery runs one read-only query inside a transaction, per the pattern
// every application query test uses (Repositories are reachable only inside
// UnitOfWork.Do, FF-009 §6).
func doQuery[T any](t *testing.T, uow application.UnitOfWork, fn func(application.Repositories) (T, error)) T {
	t.Helper()
	var result T
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		var err error
		result, err = fn(r)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// TestCanonicalScenario runs the full FF-011 canonical scenario end to end
// through the real application commands, a real PEOS-backed Recorder, and a
// real in-memory Store, and asserts every FF-011 §9 / FF-012 §13 expected
// end state.
func TestCanonicalScenario(t *testing.T) {
	ctx := context.Background()
	uow, rec, clock := newFixture()

	result, err := scenario.Run(ctx, uow, rec, clock)
	if err != nil {
		t.Fatalf("scenario.Run: %v", err)
	}
	assertCanonicalEndState(t, ctx, uow, rec, result)
}

// assertCanonicalEndState checks every FF-011 §9 expected end state against
// whatever adapter backs uow. It is shared by the in-memory and PostgreSQL
// scenario tests so that "both adapters produce the same engineering answers"
// is one assertion body run twice, not two bodies that could drift apart.
func assertCanonicalEndState(
	t *testing.T,
	ctx context.Context,
	uow application.UnitOfWork,
	rec peos.Recorder,
	result scenario.Result,
) {
	t.Helper()

	// 1. Project and feature card exist.
	project := doQuery(t, uow, func(r application.Repositories) (bool, error) {
		_, found, err := r.Projects.Get(ctx, mustProjectID(t, result.ProjectID))
		return found, err
	})
	if !project {
		t.Error("project not found")
	}
	card := doQuery(t, uow, func(r application.Repositories) (bool, error) {
		_, found, err := r.FeatureCards.Get(ctx, mustFeatureCardID(t, result.FeatureCardID))
		return found, err
	})
	if !card {
		t.Error("feature card not found")
	}

	// 2. Current capability revision is CAP-1-REV-2, sequence 2.
	currentRevision := doQuery(t, uow, func(r application.Repositories) (application.CurrentRevisionResult, error) {
		return application.ResolveCurrentRevision(ctx, r, scenario.CapabilityArtifactID)
	})
	if !currentRevision.Found {
		t.Fatal("expected a current capability revision")
	}
	if currentRevision.Revision.Key.RevisionID != scenario.CapabilityRevision2 {
		t.Errorf("current revision = %s, want %s", currentRevision.Revision.Key.RevisionID, scenario.CapabilityRevision2)
	}
	if currentRevision.Sequence != 2 {
		t.Errorf("current sequence = %d, want 2", currentRevision.Sequence)
	}

	// 3. Content is readable and its digest matches the revision's integrity.
	content := doQuery(t, uow, func(r application.Repositories) (engineering.CapabilitySpecificationContent, error) {
		c, found, err := r.StructuredContent.Get(ctx, currentRevision.Revision.Key)
		if err != nil {
			return engineering.CapabilitySpecificationContent{}, err
		}
		if !found {
			t.Fatal("current revision content not found")
		}
		return c, nil
	})
	if content.Title() != "Homework after a lesson" {
		t.Errorf("content title = %q, want %q", content.Title(), "Homework after a lesson")
	}
	if err := rec.VerifyContentDigest(currentRevision.Revision, content); err != nil {
		t.Errorf("VerifyContentDigest: %v", err)
	}

	// 4. Revision 1's content also remains fully readable, unmutated.
	rev1Key := mustRevisionKey(t, scenario.CapabilityArtifactID, scenario.CapabilityRevision1)
	rev1 := doQuery(t, uow, func(r application.Repositories) (engineering.RevisionEnvelope, error) {
		e, found, err := r.Revisions.Get(ctx, rev1Key)
		if err != nil {
			return engineering.RevisionEnvelope{}, err
		}
		if !found {
			t.Fatal("revision 1 not found")
		}
		return e, nil
	})
	rev1Content := doQuery(t, uow, func(r application.Repositories) (engineering.CapabilitySpecificationContent, error) {
		c, found, err := r.StructuredContent.Get(ctx, rev1Key)
		if err != nil {
			return engineering.CapabilitySpecificationContent{}, err
		}
		if !found {
			t.Fatal("revision 1 content not found")
		}
		return c, nil
	})
	if err := rec.VerifyContentDigest(rev1, rev1Content); err != nil {
		t.Errorf("VerifyContentDigest (revision 1): %v", err)
	}

	// 5. All four requirements exist.
	effective := doQuery(t, uow, func(r application.Repositories) ([]application.EffectiveRequirement, error) {
		return application.ResolveEffectiveRequirements(ctx, r, scenario.RequirementArtifactIDs)
	})
	if len(effective) != 4 {
		t.Fatalf("effective requirements = %d, want 4", len(effective))
	}

	// 6. The decision and its basis exist.
	decisionFound := doQuery(t, uow, func(r application.Repositories) (bool, error) {
		key, err := engineering.NewRecordKey(engineering.RecordKindDecision, scenario.DecisionID)
		if err != nil {
			return false, err
		}
		_, found, err := r.Records.Get(ctx, key)
		return found, err
	})
	if !decisionFound {
		t.Error("decision not found")
	}

	// 7. Plan, executions, and evidence exist.
	planRevisions := doQuery(t, uow, func(r application.Repositories) (int, error) {
		revs, err := r.Revisions.ListByArtifact(ctx, scenario.PlanArtifactID)
		return len(revs), err
	})
	if planRevisions != 1 {
		t.Errorf("plan revisions = %d, want 1", planRevisions)
	}
	for _, execID := range result.ExecutionIDs {
		found := doQuery(t, uow, func(r application.Repositories) (bool, error) {
			key, err := engineering.NewRecordKey(engineering.RecordKindExecution, execID)
			if err != nil {
				return false, err
			}
			_, found, err := r.Records.Get(ctx, key)
			return found, err
		})
		if !found {
			t.Errorf("execution %s not found", execID)
		}
	}
	for _, evID := range result.EvidenceArtifactIDs {
		found := doQuery(t, uow, func(r application.Repositories) (bool, error) {
			revs, err := r.Revisions.ListByArtifact(ctx, evID)
			return len(revs) == 1, err
		})
		if !found {
			t.Errorf("evidence %s not found", evID)
		}
	}

	// 8. Both the incorrect and the corrected claim are readable; CLM-2's
	// own record is byte-identical to what was originally written -- it
	// still reads "satisfied" when fetched directly. Nothing is mutated.
	clm2 := doQuery(t, uow, func(r application.Repositories) (engineering.RecordEnvelope, error) {
		key, err := engineering.NewRecordKey(engineering.RecordKindClaim, scenario.ClaimIncorrect)
		if err != nil {
			return engineering.RecordEnvelope{}, err
		}
		env, found, err := r.Records.Get(ctx, key)
		if err != nil {
			return engineering.RecordEnvelope{}, err
		}
		if !found {
			t.Fatal("CLM-2 not found")
		}
		return env, nil
	})
	if clm2.Outcome != "peos:satisfied" {
		t.Errorf("CLM-2 outcome = %q, want peos:satisfied (must remain unchanged)", clm2.Outcome)
	}
	clm4 := doQuery(t, uow, func(r application.Repositories) (engineering.RecordEnvelope, error) {
		key, err := engineering.NewRecordKey(engineering.RecordKindClaim, scenario.ClaimCorrecting)
		if err != nil {
			return engineering.RecordEnvelope{}, err
		}
		env, found, err := r.Records.Get(ctx, key)
		if err != nil {
			return engineering.RecordEnvelope{}, err
		}
		if !found {
			t.Fatal("CLM-4 not found")
		}
		return env, nil
	})
	if !clm4.HasCorrection() {
		t.Error("CLM-4 does not carry a correction reference")
	}
	if clm4.CorrectionTargetID != scenario.ClaimIncorrect {
		t.Errorf("CLM-4 correction target = %q, want %q", clm4.CorrectionTargetID, scenario.ClaimIncorrect)
	}
	if clm4.CorrectionKind != engineering.CorrectionKindCorrect {
		t.Errorf("CLM-4 correction kind = %q, want %q", clm4.CorrectionKind, engineering.CorrectionKindCorrect)
	}
	if clm4.Outcome != "peos:not-satisfied" {
		t.Errorf("CLM-4 outcome = %q, want peos:not-satisfied", clm4.Outcome)
	}

	// 9. Current-claim resolution selects the correction head for each
	// requirement, exactly per FF-011 §9.
	subjectKey := engineering.ArtifactRevisionSubjectKey(scenario.CapabilityArtifactID, scenario.CapabilityRevision2)
	claimScope := "featureforge:capability|" + scenario.CapabilityArtifactID
	assertCurrentClaim(t, ctx, uow, subjectKey, claimScope, "REQ-1", scenario.ClaimForR1)
	assertCurrentClaim(t, ctx, uow, subjectKey, claimScope, "REQ-2", scenario.ClaimCorrecting)
	assertCurrentClaim(t, ctx, uow, subjectKey, claimScope, "REQ-3", scenario.ClaimForR3)

	// REQ-4 has no claim at all -- it was never validated.
	req4Key := mustRevisionKey(t, "REQ-4", "REQ-4-REV-1")
	req4Criterion := mustCriterionKey(t, req4Key)
	req4Result := doQuery(t, uow, func(r application.Repositories) (application.CurrentClaimResult, error) {
		return application.ResolveCurrentClaim(ctx, r, subjectKey, claimScope, []string{req4Criterion})
	})
	if req4Result.Found {
		t.Errorf("REQ-4 unexpectedly has a current claim: %s", req4Result.Claim.Key)
	}

	// 10. Release readiness is not-ready (REQ-2 not satisfied; REQ-4 also
	// reported uncovered).
	readiness := doQuery(t, uow, func(r application.Repositories) (application.ReadinessResult, error) {
		return application.ResolveReadiness(ctx, r, currentRevision.Revision, effective)
	})
	if readiness.Status != application.ReadinessNotReady {
		t.Errorf("readiness = %s, want %s", readiness.Status, application.ReadinessNotReady)
	}
	sawReq4Incomplete := false
	for _, per := range readiness.PerRequirement {
		if per.RequirementArtifactID == "REQ-4" && !per.HasClaim {
			sawReq4Incomplete = true
		}
	}
	if !sawReq4Incomplete {
		t.Error("expected REQ-4 to be reported with no applicable claim")
	}

	// 11. Lifecycle state is under-validation, not assessed -- REQ-4 was
	// never validated, so the assessment is not complete (FF-011 §8).
	lifecycle := doQuery(t, uow, func(r application.Repositories) (application.LifecycleStateResult, error) {
		return application.ResolveLifecycleState(ctx, r, scenario.CapabilityArtifactID)
	})
	if !lifecycle.Found {
		t.Fatal("expected a resolved lifecycle state")
	}
	if lifecycle.Assignment.StateID != "featureforge:under-validation" {
		t.Errorf("lifecycle state = %q, want featureforge:under-validation", lifecycle.Assignment.StateID)
	}

	// 12. Timeline is complete and links CLM-4 to CLM-2.
	timeline := doQuery(t, uow, func(r application.Repositories) (application.TimelineResult, error) {
		return application.GetFeatureTimeline(ctx, r, timelineInput(t, ctx, r, result))
	})
	if len(timeline.Dated) == 0 {
		t.Error("timeline has no dated events")
	}
	sawCorrection := false
	for _, ev := range timeline.Dated {
		if ev.Kind == application.EventClaimCorrected && ev.Corrected == scenario.ClaimIncorrect {
			sawCorrection = true
		}
	}
	if !sawCorrection {
		t.Error("timeline does not link CLM-4's correction back to CLM-2")
	}
}

// TestCanonicalScenarioInsertionOrderIndependence runs the scenario twice,
// on two independent stores, with the independent parts (which requirement
// is created first, which validation activity runs first) in a different
// order each time -- every genuine dependency (a plan before its
// executions, an execution before its claim, CLM-2 before CLM-4) is
// preserved either way. FF-010 §4 and §6 both state that resolution never
// consults insertion order; this proves it for the full scenario, not just
// the unit-level orderings tests.
func TestCanonicalScenarioInsertionOrderIndependence(t *testing.T) {
	ctx := context.Background()

	uowA, recA, clockA := newFixture()
	if _, err := scenario.Run(ctx, uowA, recA, clockA); err != nil {
		t.Fatalf("scenario.Run (default order): %v", err)
	}

	uowB, recB, clockB := newFixture()
	if _, err := scenario.RunPermuted(ctx, uowB, recB, clockB); err != nil {
		t.Fatalf("scenario.RunPermuted: %v", err)
	}

	assertSameResolvedState(t, ctx, uowA, uowB)
}

// assertSameResolvedState compares every resolved answer between two stores
// that received the same engineering acts in different orders. Shared by the
// in-memory and PostgreSQL insertion-order tests.
func assertSameResolvedState(t *testing.T, ctx context.Context, uowA, uowB application.UnitOfWork) {
	t.Helper()

	currentA := doQuery(t, uowA, func(r application.Repositories) (application.CurrentRevisionResult, error) {
		return application.ResolveCurrentRevision(ctx, r, scenario.CapabilityArtifactID)
	})
	currentB := doQuery(t, uowB, func(r application.Repositories) (application.CurrentRevisionResult, error) {
		return application.ResolveCurrentRevision(ctx, r, scenario.CapabilityArtifactID)
	})
	if currentA.Revision.Key != currentB.Revision.Key || currentA.Sequence != currentB.Sequence {
		t.Errorf("current revision differs by insertion order: %v/%d vs %v/%d",
			currentA.Revision.Key, currentA.Sequence, currentB.Revision.Key, currentB.Sequence)
	}

	subjectKey := engineering.ArtifactRevisionSubjectKey(scenario.CapabilityArtifactID, scenario.CapabilityRevision2)
	claimScope := "featureforge:capability|" + scenario.CapabilityArtifactID
	for _, reqArtifact := range []string{"REQ-1", "REQ-2", "REQ-3"} {
		claimA := doQuery(t, uowA, func(r application.Repositories) (application.CurrentClaimResult, error) {
			revKey := mustRevisionKey(t, reqArtifact, reqArtifact+"-REV-1")
			criterion := mustCriterionKey(t, revKey)
			return application.ResolveCurrentClaim(ctx, r, subjectKey, claimScope, []string{criterion})
		})
		claimB := doQuery(t, uowB, func(r application.Repositories) (application.CurrentClaimResult, error) {
			revKey := mustRevisionKey(t, reqArtifact, reqArtifact+"-REV-1")
			criterion := mustCriterionKey(t, revKey)
			return application.ResolveCurrentClaim(ctx, r, subjectKey, claimScope, []string{criterion})
		})
		if claimA.Found != claimB.Found || claimA.Claim.Key != claimB.Claim.Key {
			t.Errorf("%s current claim differs by insertion order: %v vs %v", reqArtifact, claimA.Claim.Key, claimB.Claim.Key)
		}
	}

	lifecycleA := doQuery(t, uowA, func(r application.Repositories) (application.LifecycleStateResult, error) {
		return application.ResolveLifecycleState(ctx, r, scenario.CapabilityArtifactID)
	})
	lifecycleB := doQuery(t, uowB, func(r application.Repositories) (application.LifecycleStateResult, error) {
		return application.ResolveLifecycleState(ctx, r, scenario.CapabilityArtifactID)
	})
	if lifecycleA.Assignment.StateID != lifecycleB.Assignment.StateID {
		t.Errorf("lifecycle state differs by insertion order: %q vs %q", lifecycleA.Assignment.StateID, lifecycleB.Assignment.StateID)
	}

	effectiveA := doQuery(t, uowA, func(r application.Repositories) ([]application.EffectiveRequirement, error) {
		return application.ResolveEffectiveRequirements(ctx, r, scenario.RequirementArtifactIDs)
	})
	readinessA := doQuery(t, uowA, func(r application.Repositories) (application.ReadinessResult, error) {
		return application.ResolveReadiness(ctx, r, currentA.Revision, effectiveA)
	})
	effectiveB := doQuery(t, uowB, func(r application.Repositories) ([]application.EffectiveRequirement, error) {
		return application.ResolveEffectiveRequirements(ctx, r, scenario.RequirementArtifactIDs)
	})
	readinessB := doQuery(t, uowB, func(r application.Repositories) (application.ReadinessResult, error) {
		return application.ResolveReadiness(ctx, r, currentB.Revision, effectiveB)
	})
	if readinessA.Status != readinessB.Status {
		t.Errorf("readiness differs by insertion order: %s vs %s", readinessA.Status, readinessB.Status)
	}
	if readinessA.Status != application.ReadinessNotReady {
		t.Errorf("readiness = %s, want %s", readinessA.Status, application.ReadinessNotReady)
	}
}

func assertCurrentClaim(t *testing.T, ctx context.Context, uow application.UnitOfWork, subjectKey, scope, reqArtifact, wantClaimID string) {
	t.Helper()
	revKey := mustRevisionKey(t, reqArtifact, reqArtifact+"-REV-1")
	criterion := mustCriterionKey(t, revKey)
	result := doQuery(t, uow, func(r application.Repositories) (application.CurrentClaimResult, error) {
		return application.ResolveCurrentClaim(ctx, r, subjectKey, scope, []string{criterion})
	})
	if !result.Found {
		t.Fatalf("%s: expected a current claim", reqArtifact)
	}
	if result.Claim.Key.ID != wantClaimID {
		t.Errorf("%s current claim = %s, want %s", reqArtifact, result.Claim.Key.ID, wantClaimID)
	}
}

func timelineInput(t *testing.T, ctx context.Context, r application.Repositories, result scenario.Result) application.TimelineInput {
	t.Helper()
	project, found, err := r.Projects.Get(ctx, mustProjectID(t, result.ProjectID))
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("project not found for timeline")
	}
	card, found, err := r.FeatureCards.Get(ctx, mustFeatureCardID(t, result.FeatureCardID))
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("feature card not found for timeline")
	}
	return application.TimelineInput{
		Project: project, FeatureCard: card, CapabilityArtifactID: result.CapabilityArtifactID,
		RequirementArtifactIDs: result.RequirementArtifactIDs, DecisionIDs: []string{result.DecisionID},
		PlanArtifactID: result.PlanArtifactID, ExecutionIDs: result.ExecutionIDs,
		EvidenceArtifactIDs: result.EvidenceArtifactIDs, ClaimIDs: result.ClaimIDs,
	}
}

func mustCriterionKey(t *testing.T, revKey engineering.RevisionKey) string {
	t.Helper()
	key, err := engineering.RequirementCriterionKey(revKey)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func mustRevisionKey(t *testing.T, artifactID, revisionID string) engineering.RevisionKey {
	t.Helper()
	key, err := engineering.NewRevisionKey(artifactID, revisionID)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func mustProjectID(t *testing.T, id string) domain.ProjectID {
	t.Helper()
	v, err := domain.NewProjectID(id)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func mustFeatureCardID(t *testing.T, id string) domain.FeatureCardID {
	t.Helper()
	v, err := domain.NewFeatureCardID(id)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
