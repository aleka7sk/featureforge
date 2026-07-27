package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

// seedReadinessScenario writes a capability CAP-1 with an accepted revision
// CAP-1-REV-2 at sequence 2, and returns the UnitOfWork plus the resolved
// current revision, ready for per-test claim/execution seeding.
func seedReadinessScenario(t *testing.T) (application.UnitOfWork, engineering.RevisionEnvelope) {
	t.Helper()
	uow := seedRevisions(t, "CAP-1", []revisionSpec{
		{"CAP-1-REV-1", 1, engineering.AcceptanceStateAccepted},
		{"CAP-1-REV-2", 2, engineering.AcceptanceStateAccepted},
	})
	result := resolveCurrent(t, uow, "CAP-1")
	if !result.Found {
		t.Fatal("expected a current revision")
	}
	return uow, result.Revision
}

func putExecution(t *testing.T, uow application.UnitOfWork, execID, outcome string) {
	t.Helper()
	key, err := engineering.NewRecordKey(engineering.RecordKindExecution, execID)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"execution_id":"` + execID + `"}`)
	env, err := engineering.NewRecordEnvelope(engineering.RecordEnvelopeInput{
		Key: key, SubjectKey: engineering.ArtifactRevisionSubjectKey("CAP-1", "CAP-1-REV-2"),
		OccurredAt: fixedTime(), HasOccurredAt: true, Outcome: outcome,
		Payload: payload, PayloadDigest: engineering.ComputeDigest(payload), RecordedAt: fixedTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	err = uow.Do(context.Background(), func(r application.Repositories) error {
		return r.Records.Put(context.Background(), env)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func putReadinessClaim(t *testing.T, uow application.UnitOfWork, claimID, revisionID, reqArtifact, outcome, execID string) {
	t.Helper()
	reqRevKey, err := engineering.NewRevisionKey(reqArtifact, reqArtifact+"-REV-1")
	if err != nil {
		t.Fatal(err)
	}
	criterionKey, err := engineering.RequirementCriterionKey(reqRevKey)
	if err != nil {
		t.Fatal(err)
	}
	key, err := engineering.NewRecordKey(engineering.RecordKindClaim, claimID)
	if err != nil {
		t.Fatal(err)
	}
	in := engineering.RecordEnvelopeInput{
		Key:        key,
		SubjectKey: engineering.ArtifactRevisionSubjectKey("CAP-1", revisionID),
		Scope:      "featureforge:capability|CAP-1",
		OccurredAt: fixedTime(), HasOccurredAt: true, Outcome: outcome,
		CriterionKeys: []string{criterionKey},
	}
	if execID != "" {
		in.ExecutionKeys = []string{engineering.ExecutionKey(execID)}
	}
	payload := []byte(`{"claim_id":"` + claimID + `"}`)
	in.Payload = payload
	in.PayloadDigest = engineering.ComputeDigest(payload)
	in.RecordedAt = fixedTime()
	env, err := engineering.NewRecordEnvelope(in)
	if err != nil {
		t.Fatal(err)
	}
	err = uow.Do(context.Background(), func(r application.Repositories) error {
		return r.Records.Put(context.Background(), env)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func requirement(t *testing.T, artifactID string, sequence int) application.EffectiveRequirement {
	t.Helper()
	key, err := engineering.NewRevisionKey(artifactID, artifactID+"-REV-1")
	if err != nil {
		t.Fatal(err)
	}
	return application.EffectiveRequirement{ArtifactID: artifactID, RevisionKey: key, Sequence: sequence}
}

func resolveReadiness(t *testing.T, uow application.UnitOfWork, current engineering.RevisionEnvelope, reqs []application.EffectiveRequirement) application.ReadinessResult {
	t.Helper()
	var result application.ReadinessResult
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		var err error
		result, err = application.ResolveReadiness(context.Background(), r, current, reqs)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestReadyWhenAllSatisfied(t *testing.T) {
	uow, current := seedReadinessScenario(t)
	putExecution(t, uow, "ER-1", "peos:completed")
	putReadinessClaim(t, uow, "CLM-1", "CAP-1-REV-2", "REQ-1", "peos:satisfied", "ER-1")
	result := resolveReadiness(t, uow, current, []application.EffectiveRequirement{requirement(t, "REQ-1", 1)})
	if result.Status != application.ReadinessReady {
		t.Errorf("status = %v, want ready", result.Status)
	}
}

func TestNotReadyOnNegativeClaim(t *testing.T) {
	uow, current := seedReadinessScenario(t)
	putExecution(t, uow, "ER-1", "peos:completed")
	putReadinessClaim(t, uow, "CLM-1", "CAP-1-REV-2", "REQ-1", "peos:not-satisfied", "ER-1")
	result := resolveReadiness(t, uow, current, []application.EffectiveRequirement{requirement(t, "REQ-1", 1)})
	if result.Status != application.ReadinessNotReady {
		t.Errorf("status = %v, want not-ready", result.Status)
	}
}

func TestIncompleteOnMissingClaim(t *testing.T) {
	uow, current := seedReadinessScenario(t)
	result := resolveReadiness(t, uow, current, []application.EffectiveRequirement{requirement(t, "REQ-1", 1)})
	if result.Status != application.ReadinessIncomplete {
		t.Errorf("status = %v, want incomplete", result.Status)
	}
}

func TestIndeterminateOnInconclusiveClaim(t *testing.T) {
	uow, current := seedReadinessScenario(t)
	putExecution(t, uow, "ER-1", "peos:completed")
	putReadinessClaim(t, uow, "CLM-1", "CAP-1-REV-2", "REQ-1", "peos:inconclusive", "ER-1")
	result := resolveReadiness(t, uow, current, []application.EffectiveRequirement{requirement(t, "REQ-1", 1)})
	if result.Status != application.ReadinessIndeterminate {
		t.Errorf("status = %v, want indeterminate", result.Status)
	}
}

func TestIndeterminateOnInterruptedExecution(t *testing.T) {
	uow, current := seedReadinessScenario(t)
	putExecution(t, uow, "ER-1", "peos:interrupted")
	putReadinessClaim(t, uow, "CLM-1", "CAP-1-REV-2", "REQ-1", "peos:satisfied", "ER-1")
	result := resolveReadiness(t, uow, current, []application.EffectiveRequirement{requirement(t, "REQ-1", 1)})
	if result.Status != application.ReadinessIndeterminate {
		t.Errorf("status = %v, want indeterminate", result.Status)
	}
	if result.PerRequirement[0].ExecutionOutcome != "peos:interrupted" {
		t.Errorf("execution outcome not named in the per-requirement rationale: %+v", result.PerRequirement[0])
	}
}

func TestPrecedenceNotReadyBeatsIndeterminate(t *testing.T) {
	uow, current := seedReadinessScenario(t)
	putExecution(t, uow, "ER-1", "peos:completed")
	putReadinessClaim(t, uow, "CLM-1", "CAP-1-REV-2", "REQ-1", "peos:not-satisfied", "ER-1")
	putExecution(t, uow, "ER-2", "peos:completed")
	putReadinessClaim(t, uow, "CLM-2", "CAP-1-REV-2", "REQ-2", "peos:inconclusive", "ER-2")
	result := resolveReadiness(t, uow, current, []application.EffectiveRequirement{requirement(t, "REQ-1", 1), requirement(t, "REQ-2", 1)})
	if result.Status != application.ReadinessNotReady {
		t.Errorf("status = %v, want not-ready (a negative signal must never be masked by a weaker one)", result.Status)
	}
}

func TestPrecedenceIndeterminateBeatsIncomplete(t *testing.T) {
	uow, current := seedReadinessScenario(t)
	putExecution(t, uow, "ER-1", "peos:completed")
	putReadinessClaim(t, uow, "CLM-1", "CAP-1-REV-2", "REQ-1", "peos:inconclusive", "ER-1")
	result := resolveReadiness(t, uow, current, []application.EffectiveRequirement{requirement(t, "REQ-1", 1), requirement(t, "REQ-2", 1)})
	if result.Status != application.ReadinessIndeterminate {
		t.Errorf("status = %v, want indeterminate", result.Status)
	}
}

func TestPrecedenceIncompleteBeatsReady(t *testing.T) {
	uow, current := seedReadinessScenario(t)
	putExecution(t, uow, "ER-1", "peos:completed")
	putReadinessClaim(t, uow, "CLM-1", "CAP-1-REV-2", "REQ-1", "peos:satisfied", "ER-1")
	result := resolveReadiness(t, uow, current, []application.EffectiveRequirement{requirement(t, "REQ-1", 1), requirement(t, "REQ-2", 1)})
	if result.Status != application.ReadinessIncomplete {
		t.Errorf("status = %v, want incomplete", result.Status)
	}
}

func TestStaleClaimDoesNotSatisfy(t *testing.T) {
	uow, current := seedReadinessScenario(t)
	putExecution(t, uow, "ER-1", "peos:completed")
	// Claim evaluated against REV-1, not the current REV-2.
	putReadinessClaim(t, uow, "CLM-1", "CAP-1-REV-1", "REQ-1", "peos:satisfied", "ER-1")
	result := resolveReadiness(t, uow, current, []application.EffectiveRequirement{requirement(t, "REQ-1", 1)})
	if result.Status != application.ReadinessIncomplete {
		t.Errorf("status = %v, want incomplete (stale claim must not satisfy the current revision)", result.Status)
	}
	if !result.PerRequirement[0].Stale {
		t.Errorf("expected the requirement to be flagged stale: %+v", result.PerRequirement[0])
	}
}

func TestNoRequirementsIsIncomplete(t *testing.T) {
	uow, current := seedReadinessScenario(t)
	result := resolveReadiness(t, uow, current, nil)
	if result.Status != application.ReadinessIncomplete {
		t.Errorf("status = %v, want incomplete", result.Status)
	}
}

func TestReadinessRationaleIsPerRequirement(t *testing.T) {
	uow, current := seedReadinessScenario(t)
	putExecution(t, uow, "ER-1", "peos:completed")
	putReadinessClaim(t, uow, "CLM-1", "CAP-1-REV-2", "REQ-1", "peos:satisfied", "ER-1")
	result := resolveReadiness(t, uow, current, []application.EffectiveRequirement{requirement(t, "REQ-1", 1)})
	if len(result.PerRequirement) != 1 {
		t.Fatalf("expected 1 per-requirement entry, got %d", len(result.PerRequirement))
	}
	if result.PerRequirement[0].VerdictReason == "" {
		t.Error("expected a non-empty verdict reason")
	}
}

func TestReadinessIsDeterministic(t *testing.T) {
	uow, current := seedReadinessScenario(t)
	putExecution(t, uow, "ER-1", "peos:completed")
	putReadinessClaim(t, uow, "CLM-1", "CAP-1-REV-2", "REQ-1", "peos:satisfied", "ER-1")
	reqs := []application.EffectiveRequirement{requirement(t, "REQ-1", 1)}
	first := resolveReadiness(t, uow, current, reqs)
	for i := range 10 {
		got := resolveReadiness(t, uow, current, reqs)
		if got.Status != first.Status {
			t.Fatalf("iteration %d: status diverged", i)
		}
	}
}

func TestStructuralFailureIsError(t *testing.T) {
	uow := newStoreAndUOW()
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		_, err := application.ResolveEffectiveRequirements(context.Background(), r, []string{"REQ-MISSING"})
		return err
	})
	if err == nil {
		t.Fatal("expected an error resolving a requirement with no revisions at all")
	}
	if !errors.Is(err, application.ErrRevisionOrderMissing) && !errors.Is(err, application.ErrEngineeringStateIndeterminate) {
		t.Errorf("err = %v, want a structural resolution error", err)
	}
}
