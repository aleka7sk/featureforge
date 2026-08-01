package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering"
	peos "github.com/aleka7sk/featureforge/internal/engineering/peos"
)

type lifecycleGraphNode struct {
	assignmentID string
	state        string
	revisionID   string
	effectiveAt  time.Time
}

// putLifecycleGraph records a complete PEOS lifecycle predecessor chain. It
// deliberately bypasses C6 product preconditions so read-side graph
// invariants can be tested independently from command admission rules.
func putLifecycleGraph(t *testing.T, uow application.UnitOfWork, recorder peos.Recorder, subjectID, rootID string, states ...string) []lifecycleGraphNode {
	t.Helper()
	if len(states) == 0 {
		t.Fatal("lifecycle graph requires at least one state")
	}
	transitionFor := map[string]string{
		"specified":        "specify",
		"under-validation": "begin-validation",
		"assessed":         "assess",
	}
	base := fixedTime()
	nodes := make([]lifecycleGraphNode, 0, len(states))
	for i, state := range states {
		node := lifecycleGraphNode{
			assignmentID: "SA-GRAPH-" + string(rune('1'+i)),
			state:        state,
			revisionID:   rootID + "-REV-" + string(rune('0'+i)),
			effectiveAt:  base.Add(time.Duration(i) * time.Hour),
		}
		var artifact engineering.ArtifactEnvelope
		var revision engineering.RevisionEnvelope
		var assignment engineering.RecordEnvelope
		var err error
		if i == 0 {
			artifact, revision, assignment, err = recorder.RecordEntryAssignment(engineering.EntryAssignmentInput{
				AssignmentID: node.assignmentID, SubjectArtifactID: subjectID, State: state,
				EffectiveAt: node.effectiveAt, TransitionRecordArtifactID: rootID,
				TransitionRecordRevisionID: node.revisionID, RecordedAt: node.effectiveAt,
			})
		} else {
			transitionID, ok := transitionFor[state]
			if !ok {
				t.Fatalf("no configured transition for test state %q", state)
			}
			predecessor := nodes[i-1]
			artifact, revision, assignment, err = recorder.RecordTransition(engineering.TransitionInput{
				AssignmentID: node.assignmentID, SubjectArtifactID: subjectID, State: state,
				EffectiveAt: node.effectiveAt, TransitionRecordArtifactID: rootID,
				TransitionRecordRevisionID: node.revisionID, TransitionKey: transitionID,
				FromAssignmentID: predecessor.assignmentID, AttemptedAt: predecessor.effectiveAt,
				CompletedAt: node.effectiveAt, RecordedAt: node.effectiveAt,
			})
		}
		if err != nil {
			t.Fatalf("record lifecycle node %d: %v", i, err)
		}
		if err := uow.Do(context.Background(), func(r application.Repositories) error {
			if err := r.Artifacts.Put(context.Background(), artifact); err != nil {
				return err
			}
			if err := r.Revisions.Put(context.Background(), revision); err != nil {
				return err
			}
			return r.Records.Put(context.Background(), assignment)
		}); err != nil {
			t.Fatalf("persist lifecycle node %d: %v", i, err)
		}
		nodes = append(nodes, node)
	}
	return nodes
}

func resolveLifecycle(t *testing.T, uow application.UnitOfWork) (application.LifecycleStateResult, error) {
	t.Helper()
	var result application.LifecycleStateResult
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		var err error
		result, err = application.ResolveLifecycleState(context.Background(), r, newLenientEnvelopeInspector(), "CAP-1")
		return err
	})
	return result, err
}

func newLifecycleStore(t *testing.T) application.UnitOfWork {
	t.Helper()
	f := newCommandFixture()
	seedCapability(t, f)
	return f.uow
}

func seedLifecycleReadinessScenario(t *testing.T) (application.UnitOfWork, engineering.RevisionEnvelope) {
	t.Helper()
	f := newCommandFixture()
	seedCapability(t, f)
	f.clock.Advance(time.Hour)
	if _, err := (application.ReviseCapabilitySpecificationCommand{
		ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-2", Content: mustContent(t, "Homework revision 2"),
	}).Execute(context.Background(), f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	if _, err := (application.AcceptCapabilityRevisionCommand{
		RecordID: "ACC-LIFECYCLE-CAP-2", ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-2",
		State: engineering.AcceptanceStateAccepted,
	}).Execute(context.Background(), f.uow, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	var current application.CurrentRevisionResult
	if err := f.uow.Do(context.Background(), func(r application.Repositories) error {
		var err error
		current, err = application.ResolveCurrentRevision(context.Background(), r, "CAP-1")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !current.Found {
		t.Fatal("expected a current capability revision")
	}
	return f.uow, current.Revision
}

func TestNoAssignmentsReturnsNoneWithInitializedConfiguration(t *testing.T) {
	uow := newLifecycleStore(t)
	result, err := resolveLifecycle(t, uow)
	if err != nil {
		t.Fatal(err)
	}
	if result.Found {
		t.Error("expected Found = false")
	}
	if result.Rationale.Rule != "no state assignment recorded" {
		t.Fatalf("rationale = %+v", result.Rationale)
	}
}

func TestLifecycleStateIsUniqueHeadOfValidatedGraph(t *testing.T) {
	uow := newLifecycleStore(t)
	nodes := putLifecycleGraph(t, uow, peos.NewRecorder(), "CAP-1", "TR-GRAPH", "drafting", "specified", "under-validation")
	result, err := resolveLifecycle(t, uow)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Found || result.Assignment.Key.ID != nodes[2].assignmentID || result.Assignment.StateID != "featureforge:under-validation" {
		t.Fatalf("result = %+v, want the under-validation graph head", result)
	}
	if result.Rationale.Rule != "unique head of validated lifecycle predecessor chain" || result.Rationale.Total != 3 {
		t.Fatalf("rationale = %+v", result.Rationale)
	}
	if result.DefinitionID != "LCD-1" || result.DefinitionVersionID != "LCDV-1" {
		t.Fatalf("configuration = %s/%s, want LCD-1/LCDV-1", result.DefinitionID, result.DefinitionVersionID)
	}
	if result.EstablishedBy.ArtifactID != "TR-GRAPH" || result.EstablishedBy.RevisionID != nodes[2].revisionID {
		t.Fatalf("EstablishedBy = %+v, want the exact head transition revision", result.EstablishedBy)
	}
}

func TestLifecycleStateRejectsBranchedHistory(t *testing.T) {
	uow := newLifecycleStore(t)
	recorder := peos.NewRecorder()
	nodes := putLifecycleGraph(t, uow, recorder, "CAP-1", "TR-BRANCH", "drafting", "specified")
	branchTime := fixedTime().Add(2 * time.Hour)
	artifact, revision, assignment, err := recorder.RecordTransition(engineering.TransitionInput{
		AssignmentID: "SA-GRAPH-BRANCH", SubjectArtifactID: "CAP-1", State: "specified",
		EffectiveAt: branchTime, TransitionRecordArtifactID: "TR-BRANCH",
		TransitionRecordRevisionID: "TR-BRANCH-REV-B", TransitionKey: "specify",
		FromAssignmentID: nodes[0].assignmentID, AttemptedAt: nodes[0].effectiveAt,
		CompletedAt: branchTime, RecordedAt: branchTime,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := uow.Do(context.Background(), func(r application.Repositories) error {
		if err := r.Artifacts.Put(context.Background(), artifact); err != nil {
			return err
		}
		if err := r.Revisions.Put(context.Background(), revision); err != nil {
			return err
		}
		return r.Records.Put(context.Background(), assignment)
	}); err != nil {
		t.Fatal(err)
	}
	_, err = resolveLifecycle(t, uow)
	if !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("err = %v, want ErrStoredStateIntegrity for a branched predecessor graph", err)
	}
}

func TestLifecycleStateRejectsMixedFamilyRevisionUnderSelectedRoot(t *testing.T) {
	uow := newLifecycleStore(t)
	recorder := peos.NewRecorder()
	putLifecycleGraph(t, uow, recorder, "CAP-1", "TR-MIXED-FAMILY", "drafting")
	_, foreignRevision, err := recorder.RecordEvidence(engineering.EvidenceInput{
		ArtifactID: "TR-MIXED-FAMILY", RevisionID: "TR-MIXED-FAMILY-EV-1",
		Locator: "https://evidence.example/TR-MIXED-FAMILY", RecordedAt: fixedTime().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := uow.Do(context.Background(), func(r application.Repositories) error {
		return r.Revisions.Put(context.Background(), foreignRevision)
	}); err != nil {
		t.Fatal(err)
	}

	_, err = resolveLifecycle(t, uow)
	if !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("err = %v, want ErrStoredStateIntegrity for a mixed-family root revision", err)
	}
}

func TestLifecycleStateRejectsMixedSubjectSiblingUnderSelectedRoot(t *testing.T) {
	uow := newLifecycleStore(t)
	recorder := peos.NewRecorder()
	putLifecycleGraph(t, uow, recorder, "CAP-1", "TR-MIXED-SUBJECT", "drafting")
	_, siblingRevision, siblingAssignment, err := recorder.RecordEntryAssignment(engineering.EntryAssignmentInput{
		AssignmentID: "SA-MIXED-SUBJECT", SubjectArtifactID: "CAP-2", State: "drafting",
		EffectiveAt: fixedTime().Add(time.Hour), TransitionRecordArtifactID: "TR-MIXED-SUBJECT",
		TransitionRecordRevisionID: "TR-MIXED-SUBJECT-REV-OTHER", RecordedAt: fixedTime().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := uow.Do(context.Background(), func(r application.Repositories) error {
		if err := r.Artifacts.Put(context.Background(), mustArtEnv(t, "CAP-2")); err != nil {
			return err
		}
		if err := r.Revisions.Put(context.Background(), siblingRevision); err != nil {
			return err
		}
		return r.Records.Put(context.Background(), siblingAssignment)
	}); err != nil {
		t.Fatal(err)
	}

	_, err = resolveLifecycle(t, uow)
	if !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("err = %v, want ErrStoredStateIntegrity for a mixed-subject root sibling", err)
	}
}

func seedCommandLifecycleThroughSpecified(t *testing.T, f commandFixture) (application.AssignLifecycleStateCommand, application.AssignLifecycleStateCommand) {
	t.Helper()
	ctx := context.Background()
	seedCapability(t, f)
	if _, err := (application.EstablishRequirementCommand{
		ArtifactID: "REQ-LIFECYCLE-CMD", RevisionID: "REQ-LIFECYCLE-CMD-REV-1",
		Statement: "The system SHALL make the lifecycle command history complete.", SubjectArtifactID: "CAP-1",
		SourceCapabilityRevisionID: "CAP-1-REV-1", SourceAcceptanceCriterionKey: "AC-1",
		AcceptanceRecordID: memberID("MEM-REQ-LIFECYCLE-CMD"),
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	entry := application.AssignLifecycleStateCommand{
		AssignmentID: "SA-CMD-ENTRY", SubjectArtifactID: "CAP-1", State: "drafting", IsEntry: true,
		TransitionRecordArtifactID: "TR-CMD", TransitionRecordRevisionID: "TR-CMD-REV-0",
	}
	if _, err := entry.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Hour)
	specified := application.AssignLifecycleStateCommand{
		AssignmentID: "SA-CMD-SPECIFIED", SubjectArtifactID: "CAP-1", State: "specified",
		TransitionRecordArtifactID: "TR-CMD", TransitionRecordRevisionID: "TR-CMD-REV-1",
		TransitionKey: "specify", FromAssignmentID: entry.AssignmentID,
	}
	if _, err := specified.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	return entry, specified
}

func TestLifecycleOldNonHeadReplayValidatesWholeHistoryAndWritesNothing(t *testing.T) {
	f := newCommandFixture()
	entry, _ := seedCommandLifecycleThroughSpecified(t, f)
	f.clock.Advance(time.Hour)
	release := forbidPersistenceWrites(f)
	defer release()
	if _, err := entry.Execute(context.Background(), f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("exact replay of a non-head lifecycle member: %v", err)
	}
}

func TestLifecycleNewTransitionMustExtendUniqueHead(t *testing.T) {
	f := newCommandFixture()
	entry, _ := seedCommandLifecycleThroughSpecified(t, f)
	f.clock.Advance(time.Hour)
	stale := application.AssignLifecycleStateCommand{
		AssignmentID: "SA-CMD-STALE", SubjectArtifactID: "CAP-1", State: "specified",
		TransitionRecordArtifactID: "TR-CMD", TransitionRecordRevisionID: "TR-CMD-REV-STALE",
		TransitionKey: "specify", FromAssignmentID: entry.AssignmentID,
	}
	release := forbidPersistenceWrites(f)
	defer release()
	if _, err := stale.Execute(context.Background(), f.uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrLifecycleHeadConflict) {
		t.Fatalf("err = %v, want ErrLifecycleHeadConflict for a stale predecessor", err)
	}
}

func establishLifecycleValidationPlan(t *testing.T, f commandFixture) application.EstablishValidationPlanCommand {
	t.Helper()
	plan := application.EstablishValidationPlanCommand{
		ArtifactID: "VP-LIFECYCLE", RevisionID: "VP-LIFECYCLE-REV-1", ScopeArtifactID: "CAP-1",
		AcceptanceRecordID: memberID("MEM-VP-LIFECYCLE"),
		Activities: []application.PlanActivityCommandInput{{
			Key: "ACT-LIFECYCLE", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
			Method: "manual-review", OutcomeInterpretation: "Lifecycle evidence is complete.",
			RequirementArtifactID: "REQ-LIFECYCLE-CMD", RequirementRevisionID: "REQ-LIFECYCLE-CMD-REV-1",
			ExpectedEvidence: []string{"lifecycle validation report"},
		}},
	}
	if _, err := plan.Execute(context.Background(), f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("establish lifecycle validation plan: %v", err)
	}
	return plan
}

func lifecycleValidationRun(t *testing.T, f commandFixture, id, outcome string) application.RecordValidationRunCommand {
	t.Helper()
	run := application.RecordValidationRunCommand{
		ExecutionID: id, PlanArtifactID: "VP-LIFECYCLE", PlanRevisionID: "VP-LIFECYCLE-REV-1",
		ActivityKey: "ACT-LIFECYCLE", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		Method: "manual-review", Outcome: outcome,
		EvidenceArtifactID: "EV-" + id, EvidenceRevisionID: "EV-" + id + "-REV-1",
		EvidenceLocator: "https://evidence.example/" + id,
	}
	if _, err := run.Execute(context.Background(), f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("record lifecycle validation run: %v", err)
	}
	return run
}

func TestBeginValidationRequiresCompletedExecutionWitness(t *testing.T) {
	f := newCommandFixture()
	_, specified := seedCommandLifecycleThroughSpecified(t, f)
	f.clock.Advance(time.Hour)
	establishLifecycleValidationPlan(t, f)
	f.clock.Advance(time.Hour)
	lifecycleValidationRun(t, f, "ER-LIFECYCLE-INTERRUPTED", "interrupted")
	f.clock.Advance(time.Hour)
	begin := application.AssignLifecycleStateCommand{
		AssignmentID: "SA-CMD-UNDER-VALIDATION", SubjectArtifactID: "CAP-1", State: "under-validation",
		TransitionRecordArtifactID: "TR-CMD", TransitionRecordRevisionID: "TR-CMD-REV-2",
		TransitionKey: "begin-validation", FromAssignmentID: specified.AssignmentID,
	}
	release := forbidPersistenceWrites(f)
	if _, err := begin.Execute(context.Background(), f.uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrLifecycleTransitionInvalid) {
		release()
		t.Fatalf("interrupted execution err = %v, want ErrLifecycleTransitionInvalid", err)
	}
	release()

	lifecycleValidationRun(t, f, "ER-LIFECYCLE-COMPLETED", "completed")
	f.clock.Advance(time.Hour)
	if _, err := begin.Execute(context.Background(), f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("completed execution must permit begin-validation: %v", err)
	}
}

func TestAssessDoesNotReevaluateCurrentPlanMilestone(t *testing.T) {
	f := newCommandFixture()
	_, specified := seedCommandLifecycleThroughSpecified(t, f)
	f.clock.Advance(time.Hour)
	plan := establishLifecycleValidationPlan(t, f)
	f.clock.Advance(time.Hour)
	run := lifecycleValidationRun(t, f, "ER-LIFECYCLE-ASSESS", "completed")
	f.clock.Advance(time.Hour)
	begin := application.AssignLifecycleStateCommand{
		AssignmentID: "SA-CMD-UNDER-ASSESS", SubjectArtifactID: "CAP-1", State: "under-validation",
		TransitionRecordArtifactID: "TR-CMD", TransitionRecordRevisionID: "TR-CMD-REV-2-ASSESS",
		TransitionKey: "begin-validation", FromAssignmentID: specified.AssignmentID,
	}
	if _, err := begin.Execute(context.Background(), f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("begin-validation: %v", err)
	}
	f.clock.Advance(time.Hour)
	claim := application.RecordValidationClaimCommand{
		ClaimID: "CLM-LIFECYCLE-ASSESS", ScopeArtifactID: "CAP-1",
		SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		RequirementArtifactID: "REQ-LIFECYCLE-CMD", RequirementRevisionID: "REQ-LIFECYCLE-CMD-REV-1",
		Outcome: "not-satisfied", Method: "manual-review",
		EvidenceArtifactID: run.EvidenceArtifactID, EvidenceRevisionID: run.EvidenceRevisionID,
		ExecutionID: run.ExecutionID, Reasoning: "The assessment remains valid after the plan milestone is complete.",
	}
	if _, err := claim.Execute(context.Background(), f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("record assessment claim: %v", err)
	}
	f.clock.Advance(time.Hour)
	if _, err := (application.AcceptCapabilityRevisionCommand{
		RecordID: "WITHDRAW-VP-LIFECYCLE", ArtifactID: plan.ArtifactID, RevisionID: plan.RevisionID,
		State: engineering.AcceptanceStateWithdrawn,
	}).Execute(context.Background(), f.uow, f.rec, f.clock); err != nil {
		t.Fatalf("withdraw plan after begin-validation: %v", err)
	}
	f.clock.Advance(time.Hour)
	assess := application.AssignLifecycleStateCommand{
		AssignmentID: "SA-CMD-ASSESSED", SubjectArtifactID: "CAP-1", State: "assessed",
		TransitionRecordArtifactID: "TR-CMD", TransitionRecordRevisionID: "TR-CMD-REV-3",
		TransitionKey: "assess", FromAssignmentID: begin.AssignmentID,
	}
	if _, err := assess.Execute(context.Background(), f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("assess must rely on each claim's complete support, not a newly current plan: %v", err)
	}
}

func TestAssessedAndNotReadyCoexist(t *testing.T) {
	// AD-018: lifecycle is an entry milestone, not a continuously derived
	// alias for readiness. A capability can remain assessed and later resolve
	// as not-ready.
	uow, current := seedLifecycleReadinessScenario(t)
	recorder := peos.NewRecorder()
	if err := application.EnsureLifecycleConfiguration(context.Background(), uow, recorder, recorder); err != nil {
		t.Fatal(err)
	}
	putExecution(t, uow, "ER-1", "peos:completed")
	putReadinessClaim(t, uow, "CLM-1", "CAP-1-REV-2", "REQ-1", "peos:not-satisfied", "ER-1")
	readiness := resolveReadiness(t, uow, current, []application.EffectiveRequirement{requirement(t, "REQ-1", 1)})
	if readiness.Status != application.ReadinessNotReady {
		t.Fatalf("precondition failed: readiness = %v, want not-ready", readiness.Status)
	}

	putLifecycleGraph(t, uow, recorder, "CAP-1", "TR-ASSESS", "drafting", "specified", "under-validation", "assessed")
	lifecycle, err := resolveLifecycle(t, uow)
	if err != nil {
		t.Fatal(err)
	}
	if !lifecycle.Found || lifecycle.Assignment.StateID != "featureforge:assessed" {
		t.Fatalf("lifecycle = %+v, want assessed", lifecycle)
	}
	if readiness.Status != application.ReadinessNotReady {
		t.Fatal("assessed and not-ready must be able to coexist")
	}
}

func TestLifecycleStateNotDerivedFromLaterClaims(t *testing.T) {
	uow, current := seedLifecycleReadinessScenario(t)
	recorder := peos.NewRecorder()
	if err := application.EnsureLifecycleConfiguration(context.Background(), uow, recorder, recorder); err != nil {
		t.Fatal(err)
	}
	putLifecycleGraph(t, uow, recorder, "CAP-1", "TR-STABLE", "drafting", "specified", "under-validation")
	before, err := resolveLifecycle(t, uow)
	if err != nil {
		t.Fatal(err)
	}
	putExecution(t, uow, "ER-1", "peos:completed")
	putReadinessClaim(t, uow, "CLM-1", "CAP-1-REV-2", "REQ-1", "peos:not-satisfied", "ER-1")
	_ = resolveReadiness(t, uow, current, []application.EffectiveRequirement{requirement(t, "REQ-1", 1)})
	after, err := resolveLifecycle(t, uow)
	if err != nil {
		t.Fatal(err)
	}
	if before.Assignment.Key != after.Assignment.Key || before.Assignment.StateID != after.Assignment.StateID {
		t.Fatal("recording claims must never change the persisted lifecycle head")
	}
}
