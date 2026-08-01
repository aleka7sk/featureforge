package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

const (
	testSubjectKey = "artifact-revision:CAP-1/CAP-1-REV-2"
	testScope      = "featureforge:capability|CAP-1"
)

// recordPassInspector keeps the correction/readiness algorithm tests focused
// on graph and verdict semantics. Dedicated integrity suites exercise the
// production PEOS inspector against complete stored envelopes.
type recordPassInspector struct {
	application.EngineeringReplayInspector
}

func (recordPassInspector) ValidateRecord(engineering.RecordEnvelope) error { return nil }

func mustClaimEnv(t *testing.T, claimID, outcome string, correction *engineering.RecordEnvelope, correctionKind string) engineering.RecordEnvelope {
	t.Helper()
	key, err := engineering.NewRecordKey(engineering.RecordKindClaim, claimID)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"claim_id":"` + claimID + `"}`)
	in := engineering.RecordEnvelopeInput{
		Key: key, SubjectKey: testSubjectKey, Scope: testScope,
		OccurredAt: fixedTime(), HasOccurredAt: true, Outcome: outcome,
		CriterionKeys: []string{"requirement-revision:REQ-1/REQ-1-REV-1"},
		Payload:       payload, PayloadDigest: engineering.ComputeDigest(payload), RecordedAt: fixedTime(),
	}
	if correction != nil {
		in.CorrectionKind = correctionKind
		in.CorrectionTargetID = correction.Key.ID
	}
	env, err := engineering.NewRecordEnvelope(in)
	if err != nil {
		t.Fatal(err)
	}
	return env
}

func putClaim(t *testing.T, uow application.UnitOfWork, env engineering.RecordEnvelope) {
	t.Helper()
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		return r.Records.Put(context.Background(), env)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func resolveClaim(t *testing.T, uow application.UnitOfWork) (application.CurrentClaimResult, error) {
	t.Helper()
	var result application.CurrentClaimResult
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		var err error
		result, err = application.ResolveCurrentClaim(context.Background(), r, recordPassInspector{}, testSubjectKey, testScope, []string{"requirement-revision:REQ-1/REQ-1-REV-1"})
		return err
	})
	return result, err
}

func TestSingleClaimIsHead(t *testing.T) {
	uow := newStoreWithRecordSubject(t)
	claim := mustClaimEnv(t, "CLM-1", "peos:satisfied", nil, "")
	putClaim(t, uow, claim)
	result, err := resolveClaim(t, uow)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Found || result.Claim.Key.ID != "CLM-1" {
		t.Errorf("result = %+v, want CLM-1", result)
	}
}

func TestChainOfTwo(t *testing.T) {
	uow := newStoreWithRecordSubject(t)
	c2 := mustClaimEnv(t, "CLM-2", "peos:satisfied", nil, "")
	putClaim(t, uow, c2)
	c4 := mustClaimEnv(t, "CLM-4", "peos:not-satisfied", &c2, "peos:correct")
	putClaim(t, uow, c4)

	result, err := resolveClaim(t, uow)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Found || result.Claim.Key.ID != "CLM-4" {
		t.Errorf("result = %+v, want CLM-4", result)
	}
}

func TestChainOfThree(t *testing.T) {
	uow := newStoreWithRecordSubject(t)
	a := mustClaimEnv(t, "CLM-A", "peos:satisfied", nil, "")
	putClaim(t, uow, a)
	b := mustClaimEnv(t, "CLM-B", "peos:not-satisfied", &a, "peos:correct")
	putClaim(t, uow, b)
	c := mustClaimEnv(t, "CLM-C", "peos:satisfied", &b, "peos:correct")
	putClaim(t, uow, c)

	result, err := resolveClaim(t, uow)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Found || result.Claim.Key.ID != "CLM-C" {
		t.Errorf("result = %+v, want CLM-C", result)
	}
}

func TestOriginalRemainsReadable(t *testing.T) {
	uow := newStoreWithRecordSubject(t)
	c2 := mustClaimEnv(t, "CLM-2", "peos:satisfied", nil, "")
	putClaim(t, uow, c2)
	c4 := mustClaimEnv(t, "CLM-4", "peos:not-satisfied", &c2, "peos:correct")
	putClaim(t, uow, c4)

	err := uow.Do(context.Background(), func(r application.Repositories) error {
		key, _ := engineering.NewRecordKey(engineering.RecordKindClaim, "CLM-2")
		got, found, err := r.Records.Get(context.Background(), key)
		if err != nil {
			return err
		}
		if !found {
			t.Fatal("CLM-2 must remain readable after correction")
		}
		if got.Outcome != "peos:satisfied" {
			t.Errorf("CLM-2 outcome = %q, want unchanged peos:satisfied", got.Outcome)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSelectionIgnoresTimestamps(t *testing.T) {
	uow := newStoreWithRecordSubject(t)
	c2 := mustClaimEnv(t, "CLM-2", "peos:satisfied", nil, "")
	putClaim(t, uow, c2)
	// The correcting claim is backdated to BEFORE its target's timestamp.
	c4 := mustClaimEnv(t, "CLM-4", "peos:not-satisfied", &c2, "peos:correct")
	c4.OccurredAt = fixedTime().Add(-24 * time.Hour)
	putClaim(t, uow, c4)

	result, err := resolveClaim(t, uow)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Found || result.Claim.Key.ID != "CLM-4" {
		t.Errorf("a backdated but unique head must still be selected: result = %+v", result)
	}
}

// TestZeroHeadsIsGraphTheoreticallyUnreachable documents why FF-010 §6 step
// 7 ("if heads = 0, return None") has no reachable test case in M.3's data
// model: a Claim carries at most one outgoing correction reference (PEOS-006
// -- Claim.Correction() is singular), so the claim graph is a functional
// graph with out-degree <= 1 per node. In such a graph, every acyclic
// (non-erroring) configuration has at least one node with in-degree 0 -- a
// head always exists. Zero heads would require every claim to be someone's
// target while every claim also has an outgoing edge, which is only
// possible via a cycle, and a cycle is rejected earlier by
// ErrCorrectionCycle before heads is ever computed. The code path is kept
// (see ResolveCurrentClaim step 7) as a documented defensive branch, exactly
// as ordering's step 13 is kept for the same reason -- not because either is
// reachable with today's data model, but so a future extension (multiple
// correction targets, for instance) cannot silently regress into picking an
// arbitrary claim instead of reporting ambiguity or absence.
func TestZeroHeadsIsGraphTheoreticallyUnreachable(t *testing.T) {
	uow := newStoreWithRecordSubject(t)
	c1 := mustClaimEnv(t, "CLM-1", "peos:satisfied", nil, "")
	putClaim(t, uow, c1)
	c2 := mustClaimEnv(t, "CLM-2", "peos:inconclusive", &c1, "peos:invalidate")
	putClaim(t, uow, c2)

	result, err := resolveClaim(t, uow)
	if err != nil {
		t.Fatal(err)
	}
	// CLM-2 has no incoming edge, so it is the head, per
	// TestInvalidatorEvaluatedOnOwnMerits. This is correct: the invalidating
	// claim IS the applicable one, evaluated on its own merits.
	if !result.Found || result.Claim.Key.ID != "CLM-2" {
		t.Errorf("result = %+v, want CLM-2 as the unique head", result)
	}
}

func TestInvalidatorEvaluatedOnOwnMerits(t *testing.T) {
	uow := newStoreWithRecordSubject(t)
	c1 := mustClaimEnv(t, "CLM-1", "peos:satisfied", nil, "")
	putClaim(t, uow, c1)
	invalidator := mustClaimEnv(t, "CLM-2", "peos:not-satisfied", &c1, "peos:invalidate")
	putClaim(t, uow, invalidator)

	result, err := resolveClaim(t, uow)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Found || result.Claim.Key.ID != "CLM-2" {
		t.Errorf("the invalidating claim should itself be a head candidate: result = %+v", result)
	}
}

func TestMissingTarget(t *testing.T) {
	uow := newStoreWithRecordSubject(t)
	ghost := engineering.RecordEnvelope{Key: engineering.RecordKey{Kind: engineering.RecordKindClaim, ID: "CLM-GHOST"}}
	c := mustClaimEnv(t, "CLM-1", "peos:not-satisfied", &ghost, "peos:correct")
	putClaim(t, uow, c)

	_, err := resolveClaim(t, uow)
	if !errors.Is(err, application.ErrCorrectionTargetMissing) {
		t.Errorf("err = %v, want ErrCorrectionTargetMissing", err)
	}
}

func TestSelfCorrection(t *testing.T) {
	uow := newStoreWithRecordSubject(t)
	self := engineering.RecordEnvelope{Key: engineering.RecordKey{Kind: engineering.RecordKindClaim, ID: "CLM-1"}}
	c := mustClaimEnv(t, "CLM-1", "peos:not-satisfied", &self, "peos:correct")
	putClaim(t, uow, c)

	_, err := resolveClaim(t, uow)
	if !errors.Is(err, application.ErrCorrectionSelfReference) {
		t.Errorf("err = %v, want ErrCorrectionSelfReference", err)
	}
}

func TestCycle(t *testing.T) {
	uow := newStoreWithRecordSubject(t)
	a := engineering.RecordKey{Kind: engineering.RecordKindClaim, ID: "CLM-A"}
	b := engineering.RecordKey{Kind: engineering.RecordKindClaim, ID: "CLM-B"}
	claimA := mustClaimEnv(t, "CLM-A", "peos:satisfied", &engineering.RecordEnvelope{Key: b}, "peos:correct")
	claimB := mustClaimEnv(t, "CLM-B", "peos:not-satisfied", &engineering.RecordEnvelope{Key: a}, "peos:correct")
	putClaim(t, uow, claimA)
	putClaim(t, uow, claimB)

	_, err := resolveClaim(t, uow)
	if !errors.Is(err, application.ErrCorrectionCycle) {
		t.Errorf("err = %v, want ErrCorrectionCycle", err)
	}
}

func TestCompetingHeads(t *testing.T) {
	uow := newStoreWithRecordSubject(t)
	target := mustClaimEnv(t, "CLM-1", "peos:satisfied", nil, "")
	putClaim(t, uow, target)
	c2 := mustClaimEnv(t, "CLM-2", "peos:not-satisfied", &target, "peos:correct")
	putClaim(t, uow, c2)
	c3 := mustClaimEnv(t, "CLM-3", "peos:satisfied", &target, "peos:correct")
	putClaim(t, uow, c3)

	_, err := resolveClaim(t, uow)
	if !errors.Is(err, application.ErrCorrectionAmbiguous) {
		t.Errorf("err = %v, want ErrCorrectionAmbiguous", err)
	}
}

func TestScopeAndCriteriaPartitioning(t *testing.T) {
	uow := newStoreWithRecordSubject(t)
	key, err := engineering.NewRecordKey(engineering.RecordKindClaim, "CLM-OTHER")
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"claim_id":"CLM-OTHER"}`)
	other, err := engineering.NewRecordEnvelope(engineering.RecordEnvelopeInput{
		Key: key, SubjectKey: testSubjectKey, Scope: testScope, OccurredAt: fixedTime(), HasOccurredAt: true,
		Outcome: "peos:satisfied", CriterionKeys: []string{"requirement-revision:REQ-2/REQ-2-REV-1"},
		Payload: payload, PayloadDigest: engineering.ComputeDigest(payload), RecordedAt: fixedTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	putClaim(t, uow, other)
	target := mustClaimEnv(t, "CLM-1", "peos:satisfied", nil, "")
	putClaim(t, uow, target)

	result, err := resolveClaim(t, uow)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Found || result.Claim.Key.ID != "CLM-1" {
		t.Errorf("a claim for a different criterion must not interfere: result = %+v", result)
	}
}
