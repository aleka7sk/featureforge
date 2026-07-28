package peos

import (
	"reflect"
	"testing"
)

// TestProjectRequirementStatement proves the requirement projection
// reproduces the exact statement text BuildRequirement wrote (FF-020).
func TestProjectRequirementStatement(t *testing.T) {
	f := buildFixtures(t)
	statement, err := ProjectRequirementStatement(f.requirementRevision.Payload)
	if err != nil {
		t.Fatalf("ProjectRequirementStatement: %v", err)
	}
	want := "Published homework SHALL be visible to the student of the lesson."
	if statement != want {
		t.Errorf("statement = %q, want %q", statement, want)
	}
}

// TestProjectDecisionDetail proves the decision projection reproduces
// every field of the basis BuildDecision wrote, verbatim (FF-020,
// FF-001 §3.5: "the basis is displayed, not collapsed").
func TestProjectDecisionDetail(t *testing.T) {
	f := buildFixtures(t)
	detail, err := ProjectDecisionDetail(f.decision.Payload)
	if err != nil {
		t.Fatalf("ProjectDecisionDetail: %v", err)
	}
	if detail.Question != "Should homework support optional audio, and at what latency?" {
		t.Errorf("Question = %q", detail.Question)
	}
	if detail.OutcomeStatement != "Audio stored externally, referenced by content address." {
		t.Errorf("OutcomeStatement = %q", detail.OutcomeStatement)
	}
	if detail.Rationale != "External storage avoids scope creep." {
		t.Errorf("Rationale = %q", detail.Rationale)
	}
	if !reflect.DeepEqual(detail.Alternatives, []string{"store inline", "store externally", "defer entirely"}) {
		t.Errorf("Alternatives = %v", detail.Alternatives)
	}
	if !reflect.DeepEqual(detail.Assumptions, []string{"media hosted externally"}) {
		t.Errorf("Assumptions = %v", detail.Assumptions)
	}
	if !reflect.DeepEqual(detail.Constraints, []string{"no binary storage"}) {
		t.Errorf("Constraints = %v", detail.Constraints)
	}
	if !reflect.DeepEqual(detail.Uncertainties, []string{"small interview sample"}) {
		t.Errorf("Uncertainties = %v", detail.Uncertainties)
	}
}

// TestProjectPlanActivities proves the plan projection reproduces every
// activity BuildValidationPlan wrote (FF-020, FF-001 §3.6).
func TestProjectPlanActivities(t *testing.T) {
	f := buildFixtures(t)
	activities, err := ProjectPlanActivities(f.planRevision.Payload)
	if err != nil {
		t.Fatalf("ProjectPlanActivities: %v", err)
	}
	if len(activities) != 1 {
		t.Fatalf("activities = %d, want 1", len(activities))
	}
	a := activities[0]
	if a.Key != "A-1" {
		t.Errorf("Key = %q, want A-1", a.Key)
	}
	if a.OutcomeInterpretation != "Satisfied when the reviewer confirms student visibility." {
		t.Errorf("OutcomeInterpretation = %q", a.OutcomeInterpretation)
	}
	if !reflect.DeepEqual(a.ExpectedEvidence, []string{"reviewer note"}) {
		t.Errorf("ExpectedEvidence = %v", a.ExpectedEvidence)
	}
	if a.Method == "" {
		t.Error("Method must not be empty")
	}
}

// TestProjectClaimReasoning proves the claim projection reproduces the
// reasoning text BuildClaim wrote, for both an uncorrected and a
// correcting claim (FF-020, FF-001 §3.6).
func TestProjectClaimReasoning(t *testing.T) {
	f := buildFixtures(t)

	reasoning, err := ProjectClaimReasoning(f.claim.Payload)
	if err != nil {
		t.Fatalf("ProjectClaimReasoning (claim): %v", err)
	}
	if reasoning != "The specification states student visibility explicitly." {
		t.Errorf("reasoning = %q", reasoning)
	}

	correctedReasoning, err := ProjectClaimReasoning(f.correctedClaim.Payload)
	if err != nil {
		t.Fatalf("ProjectClaimReasoning (corrected): %v", err)
	}
	if correctedReasoning != "The original review was mistaken." {
		t.Errorf("corrected reasoning = %q", correctedReasoning)
	}
}

// TestProjectionsRejectCorruptPayload proves every projection function
// surfaces a decode error rather than a zero-value success on a payload
// that will not decode -- the same discipline
// TestDecodeRejectsCorruptDiscriminator already proves for the underlying
// Decode* functions (FF-020 §7).
func TestProjectionsRejectCorruptPayload(t *testing.T) {
	corrupt := []byte(`{"not":"valid"`)

	if _, err := ProjectRequirementStatement(corrupt); err == nil {
		t.Error("ProjectRequirementStatement: expected an error on corrupt payload")
	}
	if _, err := ProjectDecisionDetail(corrupt); err == nil {
		t.Error("ProjectDecisionDetail: expected an error on corrupt payload")
	}
	if _, err := ProjectPlanActivities(corrupt); err == nil {
		t.Error("ProjectPlanActivities: expected an error on corrupt payload")
	}
	if _, err := ProjectClaimReasoning(corrupt); err == nil {
		t.Error("ProjectClaimReasoning: expected an error on corrupt payload")
	}
}
