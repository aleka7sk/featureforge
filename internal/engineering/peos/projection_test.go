package peos

import (
	"testing"

	"github.com/aleka7sk/PEOS/peos/core"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

// TestProjectionFidelity_* (FF-012 §4.2): decode each envelope's payload
// back into its exact PEOS type and assert every projected field equals the
// value read from the decoded type. Projections are convenience; this is
// what makes trusting them in application-layer queries safe.

func TestProjectionFidelity_CapabilityRevision(t *testing.T) {
	f := buildFixtures(t)
	rev, err := DecodeArtifactRevision(f.capabilityRevision.Payload)
	if err != nil {
		t.Fatal(err)
	}
	assertIntegrityProjection(t, f.capabilityRevision, rev)
	assertProvenanceProjection(t, f.capabilityRevision, rev)

	if f.capabilityRevision.RevisionFamily != "capability" {
		t.Errorf("RevisionFamily = %q, want capability", f.capabilityRevision.RevisionFamily)
	}
	if f.capabilityRevision.ArtifactType != "featureforge:product-capability" {
		t.Errorf("ArtifactType = %q, want featureforge:product-capability", f.capabilityRevision.ArtifactType)
	}
	wantContentDigest := "sha256:" + f.contentDigest.Hex()
	if f.capabilityRevision.IntegrityValue != wantContentDigest {
		t.Errorf("IntegrityValue = %q, want %q", f.capabilityRevision.IntegrityValue, wantContentDigest)
	}
	if !f.capabilityRevision.ContentDigest.Equal(f.contentDigest) {
		t.Errorf("ContentDigest = %v, want %v", f.capabilityRevision.ContentDigest, f.contentDigest)
	}
}

func TestProjectionFidelity_RequirementRevision(t *testing.T) {
	f := buildFixtures(t)
	rev, err := DecodeRequirementRevision(f.requirementRevision.Payload)
	if err != nil {
		t.Fatal(err)
	}
	assertIntegrityProjection(t, f.requirementRevision, rev.Core())
	assertProvenanceProjection(t, f.requirementRevision, rev.Core())
	if f.requirementRevision.RevisionFamily != "requirement" {
		t.Errorf("RevisionFamily = %q, want requirement", f.requirementRevision.RevisionFamily)
	}
}

func TestProjectionFidelity_PlanRevision(t *testing.T) {
	f := buildFixtures(t)
	rev, err := DecodePlanRevision(f.planRevision.Payload)
	if err != nil {
		t.Fatal(err)
	}
	assertIntegrityProjection(t, f.planRevision, rev.Core())
	assertProvenanceProjection(t, f.planRevision, rev.Core())
	if f.planRevision.RevisionFamily != "validation-plan" {
		t.Errorf("RevisionFamily = %q, want validation-plan", f.planRevision.RevisionFamily)
	}
}

func TestProjectionFidelity_EvidenceRevision(t *testing.T) {
	f := buildFixtures(t)
	rev, err := DecodeArtifactRevision(f.evidenceRevision.Payload)
	if err != nil {
		t.Fatal(err)
	}
	assertIntegrityProjection(t, f.evidenceRevision, rev)
	assertProvenanceProjection(t, f.evidenceRevision, rev)
	if f.evidenceRevision.RevisionFamily != "evidence" {
		t.Errorf("RevisionFamily = %q, want evidence", f.evidenceRevision.RevisionFamily)
	}
}

func TestProjectionFidelity_TransitionRevision(t *testing.T) {
	f := buildFixtures(t)
	rev, err := DecodeTransitionRecordRevision(f.transitionRevision.Payload)
	if err != nil {
		t.Fatal(err)
	}
	assertIntegrityProjection(t, f.transitionRevision, rev.Core())
	assertProvenanceProjection(t, f.transitionRevision, rev.Core())
	if f.transitionRevision.RevisionFamily != "transition-record" {
		t.Errorf("RevisionFamily = %q, want transition-record", f.transitionRevision.RevisionFamily)
	}
	// AD-026 (FF-017): BuildTransition's revision carries its subject inside
	// its own payload (lifecycle.NewTransitionRecordContent), so this
	// projection is redundant with the payload -- the case AD-026 leaves
	// unaffected. Still asserted so a regression removing the projection is
	// caught here rather than only inferred from the canonical scenario.
	wantSubjectKey := engineering.ArtifactSubjectKey("CAP-1")
	if f.transitionRevision.SubjectKey != wantSubjectKey {
		t.Errorf("SubjectKey = %q, want %q", f.transitionRevision.SubjectKey, wantSubjectKey)
	}
}

// TestProjectionFidelity_EntryTransitionRevision covers the one codec path
// that made AD-026 necessary: BuildEntryAssignment's revision is content-free
// under AD-014, so its projected SubjectKey cannot be reconstructed from its
// own payload (FF-016 architecture review, finding F-001; AD-026). Nothing
// previously asserted this projection at all (finding F-003).
func TestProjectionFidelity_EntryTransitionRevision(t *testing.T) {
	f := buildFixtures(t)
	rev, err := DecodeArtifactRevision(f.entryTransitionRevision.Payload)
	if err != nil {
		t.Fatal(err)
	}
	assertIntegrityProjection(t, f.entryTransitionRevision, rev)
	assertProvenanceProjection(t, f.entryTransitionRevision, rev)
	if f.entryTransitionRevision.RevisionFamily != "transition-record" {
		t.Errorf("RevisionFamily = %q, want transition-record", f.entryTransitionRevision.RevisionFamily)
	}
	wantSubjectKey := engineering.ArtifactSubjectKey("CAP-1")
	if f.entryTransitionRevision.SubjectKey != wantSubjectKey {
		t.Errorf("SubjectKey = %q, want %q", f.entryTransitionRevision.SubjectKey, wantSubjectKey)
	}
}

func TestProjectionFidelity_Decision(t *testing.T) {
	f := buildFixtures(t)
	dec, err := DecodeDecision(f.decision.Payload)
	if err != nil {
		t.Fatal(err)
	}
	subjects := dec.Subjects()
	if len(subjects) != 1 {
		t.Fatalf("expected exactly one subject, got %d", len(subjects))
	}
	wantSubjectKey := engineering.ArtifactRevisionSubjectKey("CAP-1", "CAP-1-REV-1")
	if f.decision.SubjectKey != wantSubjectKey {
		t.Errorf("SubjectKey = %s, want %s", f.decision.SubjectKey, wantSubjectKey)
	}
	if f.decision.Scope != projectScope(dec.Applicability()) {
		t.Errorf("Scope = %q, want %q", f.decision.Scope, projectScope(dec.Applicability()))
	}
	prov, ok := dec.Provenance()
	if !ok {
		t.Fatal("expected decision provenance to be set")
	}
	ts, hasTS := prov.RecordedAt()
	if !hasTS {
		t.Fatal("expected decision provenance to carry a recorded-at time")
	}
	if !f.decision.OccurredAt.Equal(ts.Time()) {
		t.Errorf("OccurredAt = %v, want %v", f.decision.OccurredAt, ts.Time())
	}
}

func TestProjectionFidelity_ExecutionRecord(t *testing.T) {
	f := buildFixtures(t)
	er, err := DecodeExecution(f.execution.Payload)
	if err != nil {
		t.Fatal(err)
	}
	wantSubjectKey := engineering.ArtifactRevisionSubjectKey("CAP-1", "CAP-1-REV-1")
	if f.execution.SubjectKey != wantSubjectKey {
		t.Errorf("SubjectKey = %s, want %s", f.execution.SubjectKey, wantSubjectKey)
	}
	if f.execution.Outcome != er.Outcome().String() {
		t.Errorf("Outcome = %q, want %q", f.execution.Outcome, er.Outcome().String())
	}
	if !f.execution.OccurredAt.Equal(er.CompletedAt().Time()) {
		t.Errorf("OccurredAt = %v, want %v", f.execution.OccurredAt, er.CompletedAt().Time())
	}
	produced := er.ProducedEvidence()
	if len(produced) != 1 {
		t.Fatalf("expected exactly one produced evidence reference, got %d", len(produced))
	}
	wantEvidenceKey := engineering.EvidenceKey("EV-1", "EV-1-REV-1")
	if len(f.execution.EvidenceKeys) != 1 || f.execution.EvidenceKeys[0] != wantEvidenceKey {
		t.Errorf("EvidenceKeys = %v, want [%s]", f.execution.EvidenceKeys, wantEvidenceKey)
	}
}

func TestProjectionFidelity_Claim(t *testing.T) {
	f := buildFixtures(t)
	claim, err := DecodeClaim(f.claim.Payload)
	if err != nil {
		t.Fatal(err)
	}
	wantSubjectKey := engineering.ArtifactRevisionSubjectKey("CAP-1", "CAP-1-REV-1")
	if f.claim.SubjectKey != wantSubjectKey {
		t.Errorf("SubjectKey = %s, want %s", f.claim.SubjectKey, wantSubjectKey)
	}
	if f.claim.Outcome != claim.Outcome().String() {
		t.Errorf("Outcome = %q, want %q", f.claim.Outcome, claim.Outcome().String())
	}
	if f.claim.Scope != projectScope(claim.Scope()) {
		t.Errorf("Scope = %q, want %q", f.claim.Scope, projectScope(claim.Scope()))
	}
	criteria := claim.Criteria()
	if len(criteria) != 1 {
		t.Fatalf("expected exactly one criterion, got %d", len(criteria))
	}
	reqRevKey, err := engineering.NewRevisionKey("REQ-1", "REQ-1-REV-1")
	if err != nil {
		t.Fatal(err)
	}
	wantCriterionKey, err := engineering.RequirementCriterionKey(reqRevKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.claim.CriterionKeys) != 1 || f.claim.CriterionKeys[0] != wantCriterionKey {
		t.Errorf("CriterionKeys = %v, want [%s]", f.claim.CriterionKeys, wantCriterionKey)
	}
	if _, hasCorrection := claim.Correction(); hasCorrection {
		t.Error("the uncorrected fixture claim must not carry a correction reference")
	}
	if f.claim.HasCorrection() {
		t.Error("projected envelope must not report a correction for the uncorrected claim")
	}
}

func TestProjectionFidelity_ClaimWithCorrection(t *testing.T) {
	f := buildFixtures(t)
	claim, err := DecodeClaim(f.correctedClaim.Payload)
	if err != nil {
		t.Fatal(err)
	}
	corr, ok := claim.Correction()
	if !ok {
		t.Fatal("expected the corrected fixture claim to carry a correction reference")
	}
	if !f.correctedClaim.HasCorrection() {
		t.Fatal("projected envelope must report a correction for the corrected claim")
	}
	if f.correctedClaim.CorrectionKind != corr.Kind().String() {
		t.Errorf("CorrectionKind = %q, want %q", f.correctedClaim.CorrectionKind, corr.Kind().String())
	}
	if f.correctedClaim.CorrectionTargetID != "CLM-1" {
		t.Errorf("CorrectionTargetID = %q, want CLM-1", f.correctedClaim.CorrectionTargetID)
	}
}

func TestProjectionFidelity_StateAssignment(t *testing.T) {
	f := buildFixtures(t)
	assignment, err := DecodeStateAssignment(f.resultingAssignment.Payload)
	if err != nil {
		t.Fatal(err)
	}
	wantSubjectKey := engineering.ArtifactSubjectKey("CAP-1")
	if f.resultingAssignment.SubjectKey != wantSubjectKey {
		t.Errorf("SubjectKey = %s, want %s", f.resultingAssignment.SubjectKey, wantSubjectKey)
	}
	if f.resultingAssignment.StateID != assignment.State().String() {
		t.Errorf("StateID = %q, want %q", f.resultingAssignment.StateID, assignment.State().String())
	}
	if !f.resultingAssignment.OccurredAt.Equal(assignment.EffectiveAt().Time()) {
		t.Errorf("OccurredAt = %v, want %v", f.resultingAssignment.OccurredAt, assignment.EffectiveAt().Time())
	}
}

// assertIntegrityProjection asserts env's IntegrityValue matches the
// decoded core.ArtifactRevision's own Integrity().Value().
func assertIntegrityProjection(t *testing.T, env engineering.RevisionEnvelope, rev core.ArtifactRevision) {
	t.Helper()
	if env.IntegrityValue != rev.Integrity().Value() {
		t.Errorf("IntegrityValue = %q, want %q (from decoded revision)", env.IntegrityValue, rev.Integrity().Value())
	}
}

// assertProvenanceProjection asserts env's ProvenanceActor and
// ProvenanceRecordedAt match the decoded core.ArtifactRevision's own
// Provenance.
func assertProvenanceProjection(t *testing.T, env engineering.RevisionEnvelope, rev core.ArtifactRevision) {
	t.Helper()
	actor, hasActor := rev.Provenance().Actor()
	if env.HasProvenanceActor != hasActor {
		t.Fatalf("HasProvenanceActor = %v, want %v", env.HasProvenanceActor, hasActor)
	}
	if hasActor {
		wantActor := actor.Namespace() + ":" + actor.Identifier()
		if env.ProvenanceActor != wantActor {
			t.Errorf("ProvenanceActor = %q, want %q", env.ProvenanceActor, wantActor)
		}
	}
	ts, hasTS := rev.Provenance().RecordedAt()
	if env.HasProvenanceTime != hasTS {
		t.Fatalf("HasProvenanceTime = %v, want %v", env.HasProvenanceTime, hasTS)
	}
	if hasTS && !env.ProvenanceRecordedAt.Equal(ts.Time()) {
		t.Errorf("ProvenanceRecordedAt = %v, want %v", env.ProvenanceRecordedAt, ts.Time())
	}
}
