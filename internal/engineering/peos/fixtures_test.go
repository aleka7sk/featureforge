package peos

import (
	"testing"
	"time"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

// testFixtures builds the full canonical scenario chain (FF-011) once, in
// dependency order, so codec_test.go and projection_test.go can exercise
// every family without re-deriving the same setup.
type testFixtures struct {
	t time.Time

	content       engineering.CapabilitySpecificationContent
	contentDigest engineering.Digest

	capabilityArtifact engineering.ArtifactEnvelope
	capabilityRevision engineering.RevisionEnvelope

	requirementArtifact engineering.ArtifactEnvelope
	requirementRevision engineering.RevisionEnvelope

	decision engineering.RecordEnvelope

	planArtifact engineering.ArtifactEnvelope
	planRevision engineering.RevisionEnvelope

	evidenceArtifact engineering.ArtifactEnvelope
	evidenceRevision engineering.RevisionEnvelope

	execution engineering.RecordEnvelope

	claim          engineering.RecordEnvelope
	correctedClaim engineering.RecordEnvelope

	transitionRecordArtifact engineering.ArtifactEnvelope
	entryTransitionRevision  engineering.RevisionEnvelope
	entryAssignment          engineering.RecordEnvelope

	transitionRevision  engineering.RevisionEnvelope
	resultingAssignment engineering.RecordEnvelope
}

func buildFixtures(t *testing.T) testFixtures {
	t.Helper()
	when := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	f := testFixtures{t: when}

	content, err := engineering.NewCapabilitySpecificationContent(1, "Homework after a lesson", "No follow-up work today.")
	if err != nil {
		t.Fatalf("content: %v", err)
	}
	f.content = content
	digest, err := content.Digest()
	if err != nil {
		t.Fatalf("content digest: %v", err)
	}
	f.contentDigest = digest

	capArtEnv, err := BuildArtifact("CAP-1", ArtifactTypeProductCapability, nil, when)
	if err != nil {
		t.Fatalf("BuildArtifact capability: %v", err)
	}
	f.capabilityArtifact = capArtEnv

	capRevEnv, err := BuildCapabilityRevision(CapabilityRevisionInput{
		ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-1", ContentDigest: digest, RecordedAt: when,
	})
	if err != nil {
		t.Fatalf("BuildCapabilityRevision: %v", err)
	}
	f.capabilityRevision = capRevEnv

	reqArtEnv, reqRevEnv, err := BuildRequirement(RequirementInput{
		ArtifactID: "REQ-1", RevisionID: "REQ-1-REV-1",
		Statement:         "Published homework SHALL be visible to the student of the lesson.",
		SubjectArtifactID: "CAP-1", RecordedAt: when,
	})
	if err != nil {
		t.Fatalf("BuildRequirement: %v", err)
	}
	f.requirementArtifact, f.requirementRevision = reqArtEnv, reqRevEnv

	evArtEnv, evRevEnv, err := BuildEvidenceArtifactAndRevision(EvidenceInput{
		ArtifactID: "EV-1", RevisionID: "EV-1-REV-1", Locator: "https://evidence.example/EV-1", RecordedAt: when,
	})
	if err != nil {
		t.Fatalf("BuildEvidenceArtifactAndRevision: %v", err)
	}
	f.evidenceArtifact, f.evidenceRevision = evArtEnv, evRevEnv

	decEnv, err := BuildDecision(DecisionInput{
		DecisionID: "DEC-1", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		Question:           "Should homework support optional audio, and at what latency?",
		OutcomeStatement:   "Audio stored externally, referenced by content address.",
		Alternatives:       []string{"store inline", "store externally", "defer entirely"},
		EvidenceArtifactID: "EV-1", EvidenceRevisionID: "EV-1-REV-1",
		Assumptions: []string{"media hosted externally"}, Constraints: []string{"no binary storage"},
		Uncertainties: []string{"small interview sample"}, Rationale: "External storage avoids scope creep.",
		RecordedAt: when,
	})
	if err != nil {
		t.Fatalf("BuildDecision: %v", err)
	}
	f.decision = decEnv

	planArtEnv, planRevEnv, err := BuildValidationPlan(PlanInput{
		ArtifactID: "VP-1", RevisionID: "VP-1-REV-1", ScopeArtifactID: "CAP-1",
		Activities: []PlanActivityInput{{
			Key: "A-1", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
			Method: "manual-review", OutcomeInterpretation: "Satisfied when the reviewer confirms student visibility.",
			RequirementArtifactID: "REQ-1", RequirementRevisionID: "REQ-1-REV-1",
			ExpectedEvidence: []string{"reviewer note"},
		}},
		RecordedAt: when,
	})
	if err != nil {
		t.Fatalf("BuildValidationPlan: %v", err)
	}
	f.planArtifact, f.planRevision = planArtEnv, planRevEnv

	execEnv, err := BuildExecution(ExecutionInput{
		ExecutionID: "ER-1", PlanArtifactID: "VP-1", PlanRevisionID: "VP-1-REV-1", ActivityKey: "A-1",
		SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		Method: "manual-review", Outcome: "completed", CompletedAt: when,
		EvidenceArtifactID: "EV-1", EvidenceRevisionID: "EV-1-REV-1", RecordedAt: when,
	})
	if err != nil {
		t.Fatalf("BuildExecution: %v", err)
	}
	f.execution = execEnv

	claimEnv, err := BuildClaim(ClaimInput{
		ClaimID: "CLM-1", ScopeArtifactID: "CAP-1",
		SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		RequirementArtifactID: "REQ-1", RequirementRevisionID: "REQ-1-REV-1",
		Outcome: "satisfied", Method: "manual-review",
		EvidenceArtifactID: "EV-1", EvidenceRevisionID: "EV-1-REV-1", ExecutionID: "ER-1",
		Reasoning: "The specification states student visibility explicitly.",
		Timestamp: when, RecordedAt: when,
	})
	if err != nil {
		t.Fatalf("BuildClaim: %v", err)
	}
	f.claim = claimEnv

	correctedEnv, err := BuildClaim(ClaimInput{
		ClaimID: "CLM-4", ScopeArtifactID: "CAP-1",
		SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		RequirementArtifactID: "REQ-1", RequirementRevisionID: "REQ-1-REV-1",
		Outcome: "not-satisfied", Method: "manual-review",
		EvidenceArtifactID: "EV-1", EvidenceRevisionID: "EV-1-REV-1", ExecutionID: "ER-1",
		Reasoning: "The original review was mistaken.",
		Timestamp: when, RecordedAt: when,
		HasCorrection: true, CorrectionKind: "correct", CorrectionTarget: "CLM-1",
	})
	if err != nil {
		t.Fatalf("BuildClaim (corrected): %v", err)
	}
	f.correctedClaim = correctedEnv

	trArtEnv, entryRevEnv, entryAssignEnv, err := BuildEntryAssignment(EntryAssignmentInput{
		AssignmentID: "SA-1", SubjectArtifactID: "CAP-1", State: "drafting", EffectiveAt: when,
		TransitionRecordArtifactID: "TR-1", TransitionRecordRevisionID: "TR-1-REV-0", RecordedAt: when,
	})
	if err != nil {
		t.Fatalf("BuildEntryAssignment: %v", err)
	}
	f.transitionRecordArtifact = trArtEnv
	f.entryTransitionRevision, f.entryAssignment = entryRevEnv, entryAssignEnv

	_, transRevEnv, resultingEnv, err := BuildTransition(TransitionInput{
		AssignmentID: "SA-2", SubjectArtifactID: "CAP-1", State: "under-validation", EffectiveAt: when,
		TransitionRecordArtifactID: "TR-1", TransitionRecordRevisionID: "TR-1-REV-1",
		TransitionKey: "begin-validation", FromAssignmentID: "SA-1",
		AttemptedAt: when, CompletedAt: when, RecordedAt: when,
	})
	if err != nil {
		t.Fatalf("BuildTransition: %v", err)
	}
	f.transitionRevision, f.resultingAssignment = transRevEnv, resultingEnv

	return f
}
