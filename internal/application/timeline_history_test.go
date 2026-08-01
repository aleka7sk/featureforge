package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

type invalidCriterionRequirementTraceRepository struct {
	application.RequirementCriterionTraceRepository
	key engineering.RevisionKey
}

func (r invalidCriterionRequirementTraceRepository) Get(ctx context.Context, key engineering.RevisionKey) (engineering.RequirementCriterionTrace, bool, error) {
	trace, found, err := r.RequirementCriterionTraceRepository.Get(ctx, key)
	if err != nil || !found || key != r.key {
		return trace, found, err
	}
	trace.AcceptanceCriterionKey = "AC-MISSING"
	return trace, true, nil
}

func TestRequirementTimelineEventCarriesValidatedCriterionTrace(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)
	if _, err := (application.EstablishRequirementCommand{
		ArtifactID: "REQ-TRACE", RevisionID: "REQ-TRACE-REV-1",
		Statement: "The requirement SHALL retain its exact criterion source.", SubjectArtifactID: "CAP-1",
		SourceCapabilityRevisionID: "CAP-1-REV-1", SourceAcceptanceCriterionKey: "AC-1",
		AcceptanceRecordID: memberID("MEM-REQ-TRACE"),
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}

	timeline, err := application.GetFeatureTimelineForCard(ctx, f.uow, f.rec, f.rec, mustFeatureCardID(t, "FC-1"))
	if err != nil {
		t.Fatal(err)
	}
	wantTrace := "CAP-1/CAP-1-REV-1#AC-1"
	for _, event := range append(timeline.Dated, timeline.Undated...) {
		if event.Kind != application.EventRequirementRevised || event.SourceIdentity != "REQ-TRACE/REQ-TRACE-REV-1" {
			continue
		}
		if !strings.Contains(event.Summary, "exact trace "+wantTrace) {
			t.Errorf("requirement summary = %q, want exact trace %q", event.Summary, wantTrace)
		}
		if !containsTimelineReference(event.References, "CAP-1/CAP-1-REV-1") ||
			!containsTimelineReference(event.References, "criterion:"+wantTrace) {
			t.Errorf("requirement references = %v, want source revision and exact criterion trace", event.References)
		}
		return
	}
	t.Fatal("requirement timeline event not found")
}

func TestRequirementTimelineRejectsMissingCriterionTrace(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)
	cmd := traceRequirementCommand("REQ-TIMELINE-MISSING-TRACE")
	if _, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	key := mustRevKey(t, cmd.ArtifactID, cmd.RevisionID)
	uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
		r.RequirementTraces = missingRequirementTraceRepository{
			RequirementCriterionTraceRepository: r.RequirementTraces,
			missing:                             key,
		}
	}}

	_, err := application.GetFeatureTimelineForCard(ctx, uow, f.rec, f.rec, mustFeatureCardID(t, "FC-1"))
	if !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("err = %v, want ErrStoredStateIntegrity", err)
	}
}

func TestRequirementTimelineRejectsInvalidCriterionTrace(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)
	cmd := traceRequirementCommand("REQ-TIMELINE-INVALID-TRACE")
	if _, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	key := mustRevKey(t, cmd.ArtifactID, cmd.RevisionID)
	uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
		r.RequirementTraces = invalidCriterionRequirementTraceRepository{
			RequirementCriterionTraceRepository: r.RequirementTraces,
			key:                                 key,
		}
	}}

	_, err := application.GetFeatureTimelineForCard(ctx, uow, f.rec, f.rec, mustFeatureCardID(t, "FC-1"))
	if !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("err = %v, want ErrStoredStateIntegrity", err)
	}
}

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
		SourceCapabilityRevisionID: "CAP-1-REV-1", SourceAcceptanceCriterionKey: "AC-1",
		AcceptanceRecordID: memberID("MEM-REQ-1"),
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	establishPlan(t, f, "VP-1", "VP-1-REV-1")

	if _, err := (application.RecordValidationRunCommand{
		ExecutionID: "ER-1", PlanArtifactID: "VP-1", PlanRevisionID: "VP-1-REV-1", ActivityKey: "A-1",
		SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1", Method: "manual-review", Outcome: "completed",
		EvidenceArtifactID: "EV-1", EvidenceRevisionID: "EV-1-REV-1", EvidenceLocator: "https://evidence.example/EV-1",
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	if _, err := (application.RecordValidationClaimCommand{
		ClaimID: "CLM-1", ScopeArtifactID: "CAP-1", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		RequirementArtifactID: "REQ-1", RequirementRevisionID: "REQ-1-REV-1", Outcome: "satisfied", Method: "manual-review",
		EvidenceArtifactID: "EV-1", EvidenceRevisionID: "EV-1-REV-1", ExecutionID: "ER-1",
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}

	before := timelineEventKindCounts(t, f.uow, f.rec, "FC-1")
	if before["execution.recorded"] != 1 || before["evidence.recorded"] != 1 || before["claim.recorded"] != 1 {
		t.Fatalf("precondition failed before revising: got %v, want exactly one of each", before)
	}

	if _, err := (application.ReviseCapabilitySpecificationCommand{
		ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-2",
		Content: mustContent(t, "Homework after a lesson, revised"),
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	if _, err := (application.AcceptCapabilityRevisionCommand{
		RecordID: "ACC-2", ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-2",
		State: engineering.AcceptanceStateAccepted,
	}).Execute(ctx, f.uow, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}

	after := timelineEventKindCounts(t, f.uow, f.rec, "FC-1")
	for _, kind := range []string{"execution.recorded", "evidence.recorded", "claim.recorded"} {
		if after[kind] != 1 {
			t.Errorf("%s count after CAP-1-REV-2 became current = %d, want exactly 1 (ER-1/EV-1/CLM-1 must remain, undisturbed and not duplicated)", kind, after[kind])
		}
	}
}

func TestGetFeatureTimelineForCardIncludesValidatedDecisionEvidence(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)
	seedEvidenceRevision(t, f, "EV-DEC", "EV-DEC-REV-1")
	if _, err := (application.RecordArchitectureDecisionCommand{
		DecisionID: "DEC-1", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		Question: "What supports the decision?", OutcomeStatement: "The cited evidence does.",
		EvidenceArtifactID: "EV-DEC", EvidenceRevisionID: "EV-DEC-REV-1",
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}

	timeline, err := application.GetFeatureTimelineForCard(ctx, f.uow, f.rec, f.rec, mustFeatureCardID(t, "FC-1"))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range append(timeline.Dated, timeline.Undated...) {
		if event.Kind == application.EventEvidenceRecorded && event.SourceIdentity == "EV-DEC/EV-DEC-REV-1" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("decision evidence timeline count = %d, want exactly 1", count)
	}
}

func TestGetFeatureTimelineForCardKeepsGovernedDecisionForwardCitationReadable(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)
	if _, err := (application.RecordArchitectureDecisionCommand{
		DecisionID: "DEC-1", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		Question: "What supports the decision?", OutcomeStatement: "An unresolved citation.",
		EvidenceArtifactID: "EV-MISSING", EvidenceRevisionID: "EV-MISSING-REV-1",
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}

	timeline, err := application.GetFeatureTimelineForCard(ctx, f.uow, f.rec, f.rec, mustFeatureCardID(t, "FC-1"))
	if err != nil {
		t.Fatal(err)
	}
	wantReference := engineering.EvidenceKey("EV-MISSING", "EV-MISSING-REV-1")
	decisionFound, evidenceInvented := false, false
	for _, event := range append(timeline.Undated, timeline.Dated...) {
		if event.Kind == application.EventDecisionRecorded && event.SourceIdentity == "decision:DEC-1" {
			decisionFound = containsTimelineReference(event.References, wantReference)
		}
		if event.Kind == application.EventEvidenceRecorded && event.SourceIdentity == "EV-MISSING/EV-MISSING-REV-1" {
			evidenceInvented = true
		}
	}
	if !decisionFound || evidenceInvented {
		t.Fatalf("decision found/reference = %t, evidence event invented = %t; timeline = %+v", decisionFound, evidenceInvented, timeline)
	}
}

func TestGetFeatureTimelineForCardRejectsPartiallyMaterialisedDecisionEvidence(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)
	artifact, _, err := f.rec.RecordEvidence(engineering.EvidenceInput{
		ArtifactID: "EV-PARTIAL", RevisionID: "EV-PARTIAL-REV-1",
		Locator: "https://evidence.example/EV-PARTIAL", RecordedAt: f.clock.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.uow.Do(ctx, func(r application.Repositories) error { return r.Artifacts.Put(ctx, artifact) }); err != nil {
		t.Fatal(err)
	}
	if _, err := (application.RecordArchitectureDecisionCommand{
		DecisionID: "DEC-1", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		Question: "What supports the decision?", OutcomeStatement: "A partially occupied citation must fail closed.",
		EvidenceArtifactID: "EV-PARTIAL", EvidenceRevisionID: "EV-PARTIAL-REV-1",
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}

	_, err = application.GetFeatureTimelineForCard(ctx, f.uow, f.rec, f.rec, mustFeatureCardID(t, "FC-1"))
	if !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("err = %v, want ErrStoredStateIntegrity", err)
	}
}

func TestTimelineEventDetailNamesDecisionPlanAndExecutionSemantics(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)
	if _, err := (application.RecordArchitectureDecisionCommand{
		DecisionID: "DEC-1", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		Question: "How should media be stored?", OutcomeStatement: "Use external content-addressed media.",
		EvidenceArtifactID: "EV-PENDING", EvidenceRevisionID: "EV-PENDING-REV-1",
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	establishPlan(t, f, "VP-1", "VP-1-REV-1")
	if _, err := (application.RecordValidationRunCommand{
		ExecutionID: "ER-1", PlanArtifactID: "VP-1", PlanRevisionID: "VP-1-REV-1", ActivityKey: "A-1",
		SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1", Method: "manual-review", Outcome: "completed",
		EvidenceArtifactID: "EV-PENDING", EvidenceRevisionID: "EV-PENDING-REV-1", EvidenceLocator: "https://evidence.example/EV-PENDING",
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}

	timeline, err := application.GetFeatureTimelineForCard(ctx, f.uow, f.rec, f.rec, mustFeatureCardID(t, "FC-1"))
	if err != nil {
		t.Fatal(err)
	}
	wantSummary := map[application.EventKind][]string{
		application.EventDecisionRecorded:  {"Use external content-addressed media."},
		application.EventPlanRevised:       {"A-1", "manual-review", "Satisfied when reviewed.", "reviewer note"},
		application.EventExecutionRecorded: {"A-1", "peos:completed"},
	}
	seen := make(map[application.EventKind]bool, len(wantSummary))
	for _, event := range append(timeline.Undated, timeline.Dated...) {
		parts, relevant := wantSummary[event.Kind]
		if !relevant {
			continue
		}
		seen[event.Kind] = true
		for _, part := range parts {
			if !strings.Contains(event.Summary, part) {
				t.Errorf("%s summary = %q, want %q", event.Kind, event.Summary, part)
			}
		}
		if event.Kind == application.EventPlanRevised && !containsTimelineReference(event.References, "REQ-1/REQ-1-REV-1") {
			t.Errorf("plan references = %v, want exact Requirement revision", event.References)
		}
		if event.Kind == application.EventExecutionRecorded && !containsTimelineReference(event.References, "VP-1/VP-1-REV-1") {
			t.Errorf("execution references = %v, want exact plan revision", event.References)
		}
	}
	if len(seen) != len(wantSummary) {
		t.Fatalf("seen detail kinds = %v, want %v", seen, wantSummary)
	}
}

func TestGetFeatureTimelineForCardRejectsCorruptDecisionEvidence(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)
	if err := f.uow.Do(ctx, func(r application.Repositories) error {
		artifact, revision, err := f.rec.RecordEvidence(engineering.EvidenceInput{
			ArtifactID: "EV-CORRUPT", RevisionID: "EV-CORRUPT-REV-1",
			Locator: "https://evidence.example/EV-CORRUPT", RecordedAt: f.clock.Now(),
		})
		if err != nil {
			return err
		}
		revision.Payload = []byte(`{"corrupt":"payload"}`)
		if err := r.Artifacts.Put(ctx, artifact); err != nil {
			return err
		}
		return r.Revisions.Put(ctx, revision)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := (application.RecordArchitectureDecisionCommand{
		DecisionID: "DEC-1", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		Question: "What supports the decision?", OutcomeStatement: "A corrupt citation.",
		EvidenceArtifactID: "EV-CORRUPT", EvidenceRevisionID: "EV-CORRUPT-REV-1",
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}

	_, err := application.GetFeatureTimelineForCard(ctx, f.uow, f.rec, f.rec, mustFeatureCardID(t, "FC-1"))
	if !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("err = %v, want ErrStoredStateIntegrity", err)
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
		SourceCapabilityRevisionID: "CAP-1-REV-1", SourceAcceptanceCriterionKey: "AC-1",
		AcceptanceRecordID: memberID("MEM-REQ-1"),
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	establishPlan(t, f, "VP-1", "VP-1-REV-1")
	if _, err := (application.RecordValidationRunCommand{
		ExecutionID: "ER-1", PlanArtifactID: "VP-1", PlanRevisionID: "VP-1-REV-1", ActivityKey: "A-1",
		SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1", Method: "manual-review", Outcome: "completed",
		EvidenceArtifactID: "EV-1", EvidenceRevisionID: "EV-1-REV-1", EvidenceLocator: "https://evidence.example/EV-1",
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	if _, err := (application.RecordValidationClaimCommand{
		ClaimID: "CLM-1", ScopeArtifactID: "CAP-1", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		RequirementArtifactID: "REQ-1", RequirementRevisionID: "REQ-1-REV-1", Outcome: "satisfied", Method: "manual-review",
		EvidenceArtifactID: "EV-1", EvidenceRevisionID: "EV-1-REV-1", ExecutionID: "ER-1",
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}

	if _, err := (application.ReviseCapabilitySpecificationCommand{
		ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-2",
		Content: mustContent(t, "Homework after a lesson, revised"),
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	if _, err := (application.AcceptCapabilityRevisionCommand{
		RecordID: "ACC-2", ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-2",
		State: engineering.AcceptanceStateAccepted,
	}).Execute(ctx, f.uow, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}

	state, err := application.GetFeatureEngineeringStateForCard(ctx, f.uow, f.rec, f.rec, mustFeatureCardID(t, "FC-1"))
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

type timelineReadAuthority interface {
	application.EngineeringProjector
	application.EngineeringReplayInspector
}

func timelineEventKindCounts(t *testing.T, uow application.UnitOfWork, reader timelineReadAuthority, cardID string) map[string]int {
	t.Helper()
	result, err := application.GetFeatureTimelineForCard(context.Background(), uow, reader, reader, mustFeatureCardID(t, cardID))
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
