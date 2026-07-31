package application_test

import (
	"context"
	"testing"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

// TestGetFeatureTimelineForCard_PreservesPriorRevisionActivity is the M-1
// regression (docs/reports/m5-publication-remediation.md, D1/D2): an
// execution, its evidence, and a claim recorded against a capability's
// first revision must remain on the feature timeline after a second
// revision is created and becomes current. FF-006 §1 defines the timeline
// as computed over every immutable record, and FF-007's M.5 exit criterion
// requires "superseded claims and prior revisions are visible, not
// hidden." Before this fix, discoverEngineeringStateComponents scoped
// execution/claim/evidence discovery to the current revision alone, so all
// three vanished from Q5 the moment CAP-1-REV-2 became current, though
// ER-1/EV-1/CLM-1 were never corrected or withdrawn.
func TestGetFeatureTimelineForCard_PreservesPriorRevisionActivity(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f) // PRJ-1/FC-1/CAP-1, CAP-1-REV-1 accepted (sequence 1)

	if _, err := (application.EstablishRequirementCommand{
		ArtifactID: "REQ-1", RevisionID: "REQ-1-REV-1",
		Statement: "Students see homework.", SubjectArtifactID: "CAP-1",
	}).Execute(ctx, f.uow, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	establishPlan(t, f, "VP-1", "VP-1-REV-1")

	if _, err := (application.RecordValidationRunCommand{
		ExecutionID: "ER-1", PlanArtifactID: "VP-1", PlanRevisionID: "VP-1-REV-1", ActivityKey: "A-1",
		SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1", Method: "manual-review", Outcome: "completed",
		EvidenceArtifactID: "EV-1", EvidenceRevisionID: "EV-1-REV-1", EvidenceLocator: "https://evidence.example/EV-1",
	}).Execute(ctx, f.uow, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	if _, err := (application.RecordValidationClaimCommand{
		ClaimID: "CLM-1", ScopeArtifactID: "CAP-1", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		RequirementArtifactID: "REQ-1", RequirementRevisionID: "REQ-1-REV-1", Outcome: "satisfied", Method: "manual-review",
		EvidenceArtifactID: "EV-1", EvidenceRevisionID: "EV-1-REV-1", ExecutionID: "ER-1",
	}).Execute(ctx, f.uow, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}

	before := timelineEventKindCounts(t, f.uow, "FC-1")
	if before["execution.recorded"] != 1 || before["evidence.recorded"] != 1 || before["claim.recorded"] != 1 {
		t.Fatalf("precondition failed before revising: got %v, want exactly one of each", before)
	}

	if _, err := (application.ReviseCapabilitySpecificationCommand{
		ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-2",
		Content: mustContent(t, "Homework after a lesson, revised"),
	}).Execute(ctx, f.uow, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	if _, err := (application.AcceptCapabilityRevisionCommand{
		RecordID: "ACC-2", ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-2",
		State: engineering.AcceptanceStateAccepted,
	}).Execute(ctx, f.uow, f.clock); err != nil {
		t.Fatal(err)
	}

	after := timelineEventKindCounts(t, f.uow, "FC-1")
	for _, kind := range []string{"execution.recorded", "evidence.recorded", "claim.recorded"} {
		if after[kind] != 1 {
			t.Errorf("%s count after CAP-1-REV-2 became current = %d, want exactly 1 (ER-1/EV-1/CLM-1 must remain, undisturbed and not duplicated)", kind, after[kind])
		}
	}
}

// TestGetFeatureEngineeringStateForCard_PriorRevisionClaimStaysStale proves
// D1's other half: the M-1 fix must not broaden Q3/Q4. A claim recorded
// against a superseded revision must not satisfy the new current revision's
// readiness -- it is reported stale, exactly as before this change
// (query_readiness.go's findStaleClaimSequence is untouched by the fix).
func TestGetFeatureEngineeringStateForCard_PriorRevisionClaimStaysStale(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)

	if _, err := (application.EstablishRequirementCommand{
		ArtifactID: "REQ-1", RevisionID: "REQ-1-REV-1",
		Statement: "Students see homework.", SubjectArtifactID: "CAP-1",
	}).Execute(ctx, f.uow, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	establishPlan(t, f, "VP-1", "VP-1-REV-1")
	if _, err := (application.RecordValidationRunCommand{
		ExecutionID: "ER-1", PlanArtifactID: "VP-1", PlanRevisionID: "VP-1-REV-1", ActivityKey: "A-1",
		SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1", Method: "manual-review", Outcome: "completed",
		EvidenceArtifactID: "EV-1", EvidenceRevisionID: "EV-1-REV-1", EvidenceLocator: "https://evidence.example/EV-1",
	}).Execute(ctx, f.uow, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	if _, err := (application.RecordValidationClaimCommand{
		ClaimID: "CLM-1", ScopeArtifactID: "CAP-1", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		RequirementArtifactID: "REQ-1", RequirementRevisionID: "REQ-1-REV-1", Outcome: "satisfied", Method: "manual-review",
		EvidenceArtifactID: "EV-1", EvidenceRevisionID: "EV-1-REV-1", ExecutionID: "ER-1",
	}).Execute(ctx, f.uow, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}

	if _, err := (application.ReviseCapabilitySpecificationCommand{
		ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-2",
		Content: mustContent(t, "Homework after a lesson, revised"),
	}).Execute(ctx, f.uow, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	if _, err := (application.AcceptCapabilityRevisionCommand{
		RecordID: "ACC-2", ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-2",
		State: engineering.AcceptanceStateAccepted,
	}).Execute(ctx, f.uow, f.clock); err != nil {
		t.Fatal(err)
	}

	state, err := application.GetFeatureEngineeringStateForCard(ctx, f.uow, f.rec, mustFeatureCardID(t, "FC-1"))
	if err != nil {
		t.Fatal(err)
	}
	if state.CurrentRevision.Revision.Key.RevisionID != "CAP-1-REV-2" {
		t.Fatalf("current revision = %s, want CAP-1-REV-2", state.CurrentRevision.Revision.Key.RevisionID)
	}
	if state.Readiness.Status == application.ReadinessReady {
		t.Fatal("readiness = ready; a claim recorded against the superseded CAP-1-REV-1 must not satisfy CAP-1-REV-2")
	}
	if len(state.Readiness.PerRequirement) != 1 || !state.Readiness.PerRequirement[0].Stale {
		t.Fatalf("PerRequirement = %+v, want exactly REQ-1 marked stale", state.Readiness.PerRequirement)
	}
}

func timelineEventKindCounts(t *testing.T, uow application.UnitOfWork, cardID string) map[string]int {
	t.Helper()
	result, err := application.GetFeatureTimelineForCard(context.Background(), uow, mustFeatureCardID(t, cardID))
	if err != nil {
		t.Fatalf("GetFeatureTimelineForCard: %v", err)
	}
	counts := map[string]int{}
	for _, e := range result.Dated {
		counts[string(e.Kind)]++
	}
	for _, e := range result.Undated {
		counts[string(e.Kind)]++
	}
	return counts
}
