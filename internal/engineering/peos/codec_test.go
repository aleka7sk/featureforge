package peos

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/aleka7sk/PEOS/peos/core"
	"github.com/aleka7sk/PEOS/peos/lifecycle"
	"github.com/aleka7sk/PEOS/peos/validation"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

var referenceTestTime = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

// TestRoundTrip_* covers FF-012 §4.1: for every PEOS family the canonical
// scenario uses, encode -> decode -> re-encode must be byte-identical.

func TestRoundTrip_Artifact(t *testing.T) {
	f := buildFixtures(t)
	roundTrip(t, f.capabilityArtifact.Payload, DecodeArtifact)
}

func TestRoundTrip_ArtifactRevision(t *testing.T) {
	f := buildFixtures(t)
	roundTrip(t, f.capabilityRevision.Payload, DecodeArtifactRevision)
}

func TestRoundTrip_Requirement(t *testing.T) {
	f := buildFixtures(t)
	roundTrip(t, f.requirementArtifact.Payload, DecodeRequirement)
}

func TestRoundTrip_RequirementRevision(t *testing.T) {
	f := buildFixtures(t)
	roundTrip(t, f.requirementRevision.Payload, DecodeRequirementRevision)
}

func TestRoundTrip_Decision(t *testing.T) {
	f := buildFixtures(t)
	roundTrip(t, f.decision.Payload, DecodeDecision)
}

func TestRoundTrip_Plan(t *testing.T) {
	f := buildFixtures(t)
	roundTrip(t, f.planArtifact.Payload, DecodePlan)
}

func TestRoundTrip_PlanRevision(t *testing.T) {
	f := buildFixtures(t)
	roundTrip(t, f.planRevision.Payload, DecodePlanRevision)
}

func TestRoundTrip_ExecutionRecord(t *testing.T) {
	f := buildFixtures(t)
	roundTrip(t, f.execution.Payload, DecodeExecution)
}

func TestRoundTrip_Claim(t *testing.T) {
	f := buildFixtures(t)
	roundTrip(t, f.claim.Payload, DecodeClaim)
}

func TestRoundTrip_ClaimWithCorrection(t *testing.T) {
	f := buildFixtures(t)
	decoded := roundTrip(t, f.correctedClaim.Payload, DecodeClaim)
	corr, ok := decoded.Correction()
	if !ok {
		t.Fatal("expected the decoded claim to carry a correction reference")
	}
	if corr.Kind().String() != "peos:correct" {
		t.Errorf("correction kind = %s, want peos:correct", corr.Kind())
	}
}

func TestRoundTrip_StateAssignment(t *testing.T) {
	f := buildFixtures(t)
	roundTrip(t, f.entryAssignment.Payload, DecodeStateAssignment)
	roundTrip(t, f.resultingAssignment.Payload, DecodeStateAssignment)
}

func TestRoundTrip_TransitionRecordRevision(t *testing.T) {
	f := buildFixtures(t)
	roundTrip(t, f.transitionRevision.Payload, DecodeTransitionRecordRevision)
}

func TestRoundTrip_EntryRevisionIsBareArtifactRevision(t *testing.T) {
	f := buildFixtures(t)
	roundTrip(t, f.entryTransitionRevision.Payload, DecodeArtifactRevision)
}

// roundTrip decodes payload with decode, re-encodes the result, and asserts
// byte-identical output. It returns the decoded value for further
// assertions.
func roundTrip[T any](t *testing.T, payload []byte, decode func([]byte) (T, error)) T {
	t.Helper()
	decoded, err := decode(payload)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	reencoded, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("re-encode: %v", err)
	}
	if string(reencoded) != string(payload) {
		t.Errorf("round trip not byte-identical:\ngot:  %s\nwant: %s", reencoded, payload)
	}
	return decoded
}

func TestDecodeRejectsInvalidJSON(t *testing.T) {
	_, err := DecodeArtifact([]byte("not json"))
	if !errors.Is(err, ErrStoredPayloadInvalid) {
		t.Errorf("err = %v, want ErrStoredPayloadInvalid", err)
	}
}

func TestDecodePreservesReceiverOnFailure(t *testing.T) {
	f := buildFixtures(t)
	valid, err := DecodeArtifact(f.capabilityArtifact.Payload)
	if err != nil {
		t.Fatal(err)
	}
	before := valid
	if err := json.Unmarshal([]byte("not json"), &valid); err == nil {
		t.Fatal("expected an error decoding invalid JSON")
	}
	beforeBytes, _ := json.Marshal(before)
	afterBytes, _ := json.Marshal(valid)
	if string(beforeBytes) != string(afterBytes) {
		t.Error("a failed decode must leave the receiver untouched")
	}
}

func TestDecodeToleratesUnknownFields(t *testing.T) {
	f := buildFixtures(t)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(f.capabilityArtifact.Payload, &raw); err != nil {
		t.Fatal(err)
	}
	raw["future_field_from_a_later_peos_version"] = json.RawMessage(`"anything"`)
	withExtra, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeArtifact(withExtra); err != nil {
		t.Errorf("unknown fields must be tolerated: %v", err)
	}
}

func TestDecodeRejectsCorruptDiscriminator(t *testing.T) {
	f := buildFixtures(t)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(f.evidenceRevision.Payload, &raw); err != nil {
		t.Fatal(err)
	}
	reps, ok := raw["representations"]
	if !ok {
		t.Skip("payload has no representations field to corrupt")
	}
	var repList []map[string]json.RawMessage
	if err := json.Unmarshal(reps, &repList); err != nil {
		t.Fatal(err)
	}
	if len(repList) == 0 {
		t.Skip("no representation to corrupt")
	}
	if content, ok := repList[0]["content"]; ok {
		var contentMap map[string]json.RawMessage
		if err := json.Unmarshal(content, &contentMap); err == nil {
			contentMap["kind"] = json.RawMessage(`"not-a-real-kind"`)
			corrupted, _ := json.Marshal(contentMap)
			repList[0]["content"] = corrupted
			newReps, _ := json.Marshal(repList)
			raw["representations"] = newReps
			corruptedPayload, _ := json.Marshal(raw)
			if _, err := DecodeArtifactRevision(corruptedPayload); err == nil {
				t.Error("expected a corrupted discriminator to be rejected")
			}
		}
	}
}

// TestPEOSSentinelsRemainMatchable and TestPEOSNestedSentinelsPreserved
// (FF-012 §4.3): a rejected value must match both the specific PEOS cause
// and the general PEOS sentinel via errors.Is.
func TestPEOSSentinelsRemainMatchable(t *testing.T) {
	_, err := BuildClaim(ClaimInput{
		ClaimID: "CLM-X", ScopeArtifactID: "CAP-1",
		SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		RequirementArtifactID: "REQ-1", RequirementRevisionID: "REQ-1-REV-1",
		Outcome: "satisfied", Method: "manual-review",
		EvidenceArtifactID: "", EvidenceRevisionID: "",
		ExecutionID: "ER-1", Timestamp: fixedTestTime(), RecordedAt: fixedTestTime(),
	})
	if err == nil {
		t.Fatal("expected an error constructing a claim with an empty evidence reference")
	}
	if !errors.Is(err, core.ErrEmptyIdentity) {
		t.Errorf("err = %v, want it to match core.ErrEmptyIdentity", err)
	}
}

func TestBuildDecisionSupportsCapabilityArtifactSubject(t *testing.T) {
	when := fixedTestTime()
	envelope, err := BuildDecision(DecisionInput{
		DecisionID:         "DEC-ARTIFACT",
		SubjectArtifactID:  "CAP-1",
		Question:           "Which rule governs every revision?",
		OutcomeStatement:   "Use one capability-wide rule.",
		EvidenceArtifactID: "EV-1",
		EvidenceRevisionID: "EV-1-REV-1",
		RecordedAt:         when,
	})
	if err != nil {
		t.Fatalf("BuildDecision: %v", err)
	}
	if envelope.SubjectKey != engineering.ArtifactSubjectKey("CAP-1") {
		t.Fatalf("SubjectKey = %q, want Artifact subject", envelope.SubjectKey)
	}
	if err := (Recorder{}).ValidateRecord(envelope); err != nil {
		t.Fatalf("ValidateRecord: %v", err)
	}
	decoded, err := DecodeDecision(envelope.Payload)
	if err != nil {
		t.Fatalf("DecodeDecision: %v", err)
	}
	subjects := decoded.Subjects()
	if len(subjects) != 1 {
		t.Fatalf("subjects = %d, want 1", len(subjects))
	}
	artifact, ok := subjects[0].AsArtifact()
	if !ok || artifact.ArtifactID().String() != "CAP-1" {
		t.Fatalf("subject = %#v, want Artifact CAP-1", subjects[0])
	}
}

// TestPEOSNestedSentinelsPreserved exercises PEOS's own documented
// nested-sentinel example (consumer guide §5): a succeeded Transition
// Record Revision whose enclosing Provenance carries no Actor fails with
// both the general lifecycle.ErrInvalidTransitionRecordRevision and the
// specific lifecycle.ErrMissingResponsibleActor, and both must remain
// errors.Is-matchable after this package wraps the error.
func TestPEOSNestedSentinelsPreserved(t *testing.T) {
	subject, err := lifecycleSubject("CAP-1")
	if err != nil {
		t.Fatal(err)
	}
	ts, err := core.NewTimestamp(referenceTestTime)
	if err != nil {
		t.Fatal(err)
	}
	transitionID, err := lifecycle.NewTransitionID("featureforge:enter")
	if err != nil {
		t.Fatal(err)
	}
	fromID, err := core.NewStateAssignmentID("SA-1")
	if err != nil {
		t.Fatal(err)
	}
	fromRef, err := core.NewStateAssignmentRef(fromID)
	if err != nil {
		t.Fatal(err)
	}
	content, err := lifecycle.NewTransitionRecordContent(subject, definitionVersionRef, transitionID, fromRef, ts, lifecycle.TransitionOutcomeSucceeded)
	if err != nil {
		t.Fatal(err)
	}
	content, err = content.WithToState(StateDrafting)
	if err != nil {
		t.Fatal(err)
	}
	content, err = content.WithCompletedAt(ts)
	if err != nil {
		t.Fatal(err)
	}
	resultID, err := core.NewStateAssignmentID("SA-2")
	if err != nil {
		t.Fatal(err)
	}
	resultRef, err := core.NewStateAssignmentRef(resultID)
	if err != nil {
		t.Fatal(err)
	}
	content, err = content.WithResultingAssignment(resultRef)
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately no WithAuthority call, and a Provenance with no Actor
	// (built directly here, bypassing provenanceFor, which always sets
	// LocalActorRef) -- both are required for a succeeded transition.
	trArtifact, err := core.NewArtifact(mustTestArtifactID(t, "TR-BAD"), lifecycle.ArtifactTypeTransitionRecord)
	if err != nil {
		t.Fatal(err)
	}
	tr, err := lifecycle.NewTransitionRecord(trArtifact)
	if err != nil {
		t.Fatal(err)
	}
	origin, err := core.NewOrigin(core.OriginKindKnown, "")
	if err != nil {
		t.Fatal(err)
	}
	provenanceWithNoActor := core.NewProvenance().WithRecordedAt(ts)
	integrity, err := core.NewIntegrityIdentity(core.IntegrityMechanismContentAddressedReference, "sha256:abc", core.IntegrityProtectedScopeContent)
	if err != nil {
		t.Fatal(err)
	}
	rev, err := core.NewArtifactRevision(mustTestArtifactID(t, "TR-BAD"), mustTestRevisionID(t, "REV-1"), origin, provenanceWithNoActor, integrity)
	if err != nil {
		t.Fatal(err)
	}

	_, sdkErr := lifecycle.NewTransitionRecordRevision(tr, rev, content)
	if sdkErr == nil {
		t.Fatal("expected an error for a succeeded transition with no responsible actor")
	}
	wrapped := wrapPEOS("test nested sentinel", sdkErr)
	if !errors.Is(wrapped, lifecycle.ErrInvalidTransitionRecordRevision) {
		t.Errorf("wrapped err = %v, want it to match the general lifecycle.ErrInvalidTransitionRecordRevision", wrapped)
	}
	if !errors.Is(wrapped, lifecycle.ErrMissingResponsibleActor) {
		t.Errorf("wrapped err = %v, want it to match the specific lifecycle.ErrMissingResponsibleActor", wrapped)
	}
}

// TestNoGenericInternalError (FF-012 §4.3): every error this package
// returns is a named sentinel or a wrapped PEOS error, never a bare
// unwrapped string.
func TestNoGenericInternalError(t *testing.T) {
	_, err := BuildArtifact("", ArtifactTypeProductCapability, nil, fixedTestTime())
	if err == nil {
		t.Fatal("expected an error for an empty artifact id")
	}
	if !errors.Is(err, core.ErrEmptyIdentity) {
		t.Errorf("err = %v, want it to wrap core.ErrEmptyIdentity", err)
	}
}

// Constructor-flow tests (FF-012 §4.4).

func TestSatisfactionClaimRequiresRequirementCriterion(t *testing.T) {
	// BuildClaim always cites a Requirement-revision criterion by
	// construction; this test documents that the underlying SDK rule is
	// real by exercising the verified failure directly.
	subjRevRef, err := core.NewArtifactRevisionRef(mustTestArtifactID(t, "CAP-1"), mustTestRevisionID(t, "CAP-1-REV-1"))
	if err != nil {
		t.Fatal(err)
	}
	subject, err := core.EngineeringSubjectRefFromArtifactRevision(subjRevRef)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := core.NewScope(CapabilityScopeKind, "CAP-1")
	if err != nil {
		t.Fatal(err)
	}
	pr, err := core.NewProductRuleRef(mustVocabularyValueForTest(t, "release-readiness"))
	if err != nil {
		t.Fatal(err)
	}
	crit, err := core.CriterionRefFromProductRule(pr)
	if err != nil {
		t.Fatal(err)
	}
	evRef, err := core.NewEvidenceArtifactRevisionRef(mustTestArtifactID(t, "EV-1"), mustTestRevisionID(t, "EV-1-REV-1"))
	if err != nil {
		t.Fatal(err)
	}
	ts, err := core.NewTimestamp(fixedTestTime())
	if err != nil {
		t.Fatal(err)
	}
	prov, err := provenanceFor(fixedTestTime())
	if err != nil {
		t.Fatal(err)
	}
	claimID, err := core.NewValidationClaimID("CLM-BAD")
	if err != nil {
		t.Fatal(err)
	}
	_, err = validation.NewClaim(claimID, core.ClaimTypeSatisfaction, subject, scope,
		core.ClaimOutcomeSatisfied, ValidationMethodManualReview,
		[]core.CriterionRef{crit}, []core.EvidenceArtifactRevisionRef{evRef}, ts, prov)
	if !errors.Is(err, validation.ErrInvalidSatisfactionClaim) {
		t.Errorf("err = %v, want ErrInvalidSatisfactionClaim", err)
	}
}

func TestClaimRequiresEvidence(t *testing.T) {
	_, err := BuildClaim(ClaimInput{
		ClaimID: "CLM-NOEV", ScopeArtifactID: "CAP-1",
		SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		RequirementArtifactID: "REQ-1", RequirementRevisionID: "REQ-1-REV-1",
		Outcome: "satisfied", Method: "manual-review",
		EvidenceArtifactID: "", EvidenceRevisionID: "",
		ExecutionID: "ER-1", Timestamp: fixedTestTime(), RecordedAt: fixedTestTime(),
	})
	if err == nil {
		t.Fatal("expected an error for a claim with no evidence reference")
	}
}

func TestPlannedActivityRequiresOutcomeInterpretation(t *testing.T) {
	_, _, err := BuildValidationPlan(PlanInput{
		ArtifactID: "VP-X", RevisionID: "VP-X-REV-1", ScopeArtifactID: "CAP-1",
		Activities: []PlanActivityInput{{
			Key: "A-1", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
			Method: "manual-review", OutcomeInterpretation: "",
			RequirementArtifactID: "REQ-1", RequirementRevisionID: "REQ-1-REV-1",
		}},
		RecordedAt: fixedTestTime(),
	})
	if err == nil {
		t.Fatal("expected an error for an empty outcome interpretation")
	}
}

func TestEntryStateAssignmentAcceptsBareEstablishedBy(t *testing.T) {
	f := buildFixtures(t)
	if f.entryAssignment.StateID == "" {
		t.Error("entry assignment must have a projected state id")
	}
}

// TestTransitionContentRejectsZeroFromAssignment pins the SDK limitation
// AD-014 works around: lifecycle.NewTransitionRecordContent rejects a zero
// fromAssignment, so no entry Transition Record can carry
// TransitionRecordContent. If this test ever starts failing because the SDK
// began accepting a zero fromAssignment, AD-014 should be revisited.
func TestTransitionContentRejectsZeroFromAssignment(t *testing.T) {
	subject, err := lifecycleSubject("CAP-1")
	if err != nil {
		t.Fatal(err)
	}
	ts, err := core.NewTimestamp(referenceTestTime)
	if err != nil {
		t.Fatal(err)
	}
	transitionID, err := lifecycle.NewTransitionID("featureforge:enter")
	if err != nil {
		t.Fatal(err)
	}
	_, err = lifecycle.NewTransitionRecordContent(
		subject, definitionVersionRef, transitionID, core.StateAssignmentRef{}, ts, lifecycle.TransitionOutcomeSucceeded,
	)
	if err == nil {
		t.Fatal("expected NewTransitionRecordContent to reject a zero fromAssignment -- if this now succeeds, AD-014 can be revisited")
	}
}

func fixedTestTime() time.Time { return referenceTestTime }

func mustTestArtifactID(t *testing.T, s string) core.ArtifactID {
	t.Helper()
	id, err := core.NewArtifactID(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustTestRevisionID(t *testing.T, s string) core.ArtifactRevisionID {
	t.Helper()
	id, err := core.NewArtifactRevisionID(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustVocabularyValueForTest(t *testing.T, value string) core.VocabularyValue {
	t.Helper()
	vv, err := core.NewVocabularyValue(Namespace, value)
	if err != nil {
		t.Fatal(err)
	}
	return vv
}
