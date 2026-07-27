package scenario

import (
	"context"
	"fmt"
	"time"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

// Result names every identity the scenario created, for assertions and for
// building queries (TimelineInput, EngineeringStateInput) after the run.
type Result struct {
	ProjectID              string
	FeatureCardID          string
	CapabilityArtifactID   string
	RequirementArtifactIDs []string
	DecisionID             string
	PlanArtifactID         string
	ExecutionIDs           []string
	EvidenceArtifactIDs    []string
	ClaimIDs               []string
}

// Run executes the canonical "Homework after a lesson" scenario (FF-011)
// through application commands, in the FF-011 §9 order, against uow and
// recorder. clock is advanced by the driver between acts so provenance and
// timeline timestamps are strictly increasing, matching a real sequence of
// engineering acts.
func Run(ctx context.Context, uow application.UnitOfWork, recorder application.EngineeringRecorder, clock *application.FixedClock) (Result, error) {
	return run(ctx, uow, recorder, clock, defaultOrder())
}

// order controls the sequencing of the independent parts of the scenario
// (which requirement is created first, which validation activity runs
// first) while preserving every genuine dependency (revision 1 before the
// decision, the decision before revision 2, an activity's plan before its
// execution, an execution before its claim, CLM-2 before CLM-4). Used by
// RunPermuted to prove the resolved end state does not depend on it.
type order struct {
	requirements []string   // artifact IDs, in creation order
	activities   [][]string // groups of [activityKey] executed together, in order
}

func defaultOrder() order {
	return order{
		requirements: []string{"REQ-1", "REQ-2"},
		activities:   [][]string{{"A-1"}, {"A-2"}, {"A-3"}},
	}
}

// RunPermuted runs the identical scenario with the requirement-creation
// order and the per-activity validation order permuted, and REQ-3/REQ-4
// (which must exist before the plan cites REQ-3, so they are appended
// after the permutable pair but before the plan either way) folded in
// consistently. It exists solely for
// TestCanonicalScenarioInsertionOrderIndependence.
func RunPermuted(ctx context.Context, uow application.UnitOfWork, recorder application.EngineeringRecorder, clock *application.FixedClock) (Result, error) {
	return run(ctx, uow, recorder, clock, order{
		requirements: []string{"REQ-2", "REQ-1"},
		activities:   [][]string{{"A-3"}, {"A-1"}, {"A-2"}},
	})
}

func run(ctx context.Context, uow application.UnitOfWork, recorder application.EngineeringRecorder, clock *application.FixedClock, ord order) (Result, error) {
	tick := func() { clock.Advance(time.Hour) }

	// 1. Project.
	if _, err := (application.CreateProjectCommand{ProjectID: ProjectID, Name: "Belcanto Pilot"}).
		Execute(ctx, uow, clock); err != nil {
		return Result{}, fmt.Errorf("create project: %w", err)
	}
	tick()

	// 2. Feature card.
	if _, err := (application.CreateFeatureCommand{
		FeatureCardID: FeatureCardID, ProjectID: ProjectID, Title: "Homework after a lesson",
	}).Execute(ctx, uow, clock); err != nil {
		return Result{}, fmt.Errorf("create feature: %w", err)
	}
	tick()

	// 3. Capability specification + Revision 1.
	rev1Content, err := capabilityRevision1Content()
	if err != nil {
		return Result{}, err
	}
	if _, err := (application.EstablishCapabilitySpecificationCommand{
		FeatureCardID: FeatureCardID, ArtifactID: CapabilityArtifactID, RevisionID: CapabilityRevision1, Content: rev1Content,
	}).Execute(ctx, uow, recorder, clock); err != nil {
		return Result{}, fmt.Errorf("establish capability specification: %w", err)
	}
	tick()

	// 4. Accept Revision 1.
	if _, err := (application.AcceptCapabilityRevisionCommand{
		RecordID: "ACC-1", ArtifactID: CapabilityArtifactID, RevisionID: CapabilityRevision1, State: engineering.AcceptanceStateAccepted,
	}).Execute(ctx, uow, clock); err != nil {
		return Result{}, fmt.Errorf("accept revision 1: %w", err)
	}
	tick()

	// 5. Lifecycle entry: drafting.
	if _, err := (application.AssignLifecycleStateCommand{
		AssignmentID: EntryAssignmentID, SubjectArtifactID: CapabilityArtifactID, State: "drafting", IsEntry: true,
		TransitionRecordArtifactID: TransitionRecordArtifactID, TransitionRecordRevisionID: EntryTransitionRevisionID,
	}).Execute(ctx, uow, recorder, clock); err != nil {
		return Result{}, fmt.Errorf("assign entry lifecycle state: %w", err)
	}
	tick()

	// 6. Requirements REQ-1, REQ-2 (permutable), then REQ-3, REQ-4.
	for _, artifactID := range ord.requirements {
		if err := establishRequirement(ctx, uow, recorder, clock, artifactID); err != nil {
			return Result{}, err
		}
		tick()
	}
	for _, artifactID := range []string{"REQ-3", "REQ-4"} {
		if err := establishRequirement(ctx, uow, recorder, clock, artifactID); err != nil {
			return Result{}, err
		}
		tick()
	}

	// 7. Decision evidence (pilot-teacher interview notes), then the
	// decision itself, resolving Revision 1's open questions. Recording
	// evidence has no execution to pair it with here -- unlike A-1..A-3's
	// evidence, which RecordValidationRunCommand bundles with an execution
	// record -- so the scenario driver records it directly through the
	// recorder, exactly as RecordValidationRunCommand does internally,
	// without introducing an eleventh command beyond FF-010 §3's ten.
	if err := recordEvidenceOnly(ctx, uow, recorder, clock, DecisionEvidenceID, "https://evidence.example/"+DecisionEvidenceID); err != nil {
		return Result{}, fmt.Errorf("record decision evidence: %w", err)
	}
	tick()

	if _, err := (application.RecordArchitectureDecisionCommand{
		DecisionID: DecisionID, SubjectArtifactID: CapabilityArtifactID, SubjectRevisionID: CapabilityRevision1,
		Question:         "Should homework support an optional audio attachment, and what publication latency is acceptable?",
		OutcomeStatement: "Homework supports at most one optional audio attachment, stored outside the capability record and retained as a content-addressed representation reference; publication must be observable to the student within 5 seconds.",
		Alternatives: []string{
			"Store audio inline in the capability record.",
			"Store audio externally and retain a content-addressed representation reference.",
			"Defer audio entirely.",
		},
		EvidenceArtifactID: DecisionEvidenceID, EvidenceRevisionID: evidenceRevisionID(DecisionEvidenceID),
		Assumptions:   []string{"Audio files are hosted by an existing media service."},
		Constraints:   []string{"No binary storage in the first release."},
		Uncertainties: []string{"Interview sample was 4 teachers."},
		Rationale:     "Referencing by content address avoids introducing binary storage into the first release; 5 seconds is the longest delay the pilot teachers described as acceptable.",
	}).Execute(ctx, uow, recorder, clock); err != nil {
		return Result{}, fmt.Errorf("record decision: %w", err)
	}
	tick()

	// 8. Capability Revision 2, then accept it.
	rev2Content, err := capabilityRevision2Content()
	if err != nil {
		return Result{}, err
	}
	if _, err := (application.ReviseCapabilitySpecificationCommand{
		ArtifactID: CapabilityArtifactID, RevisionID: CapabilityRevision2, Content: rev2Content,
	}).Execute(ctx, uow, recorder, clock); err != nil {
		return Result{}, fmt.Errorf("revise capability specification: %w", err)
	}
	tick()
	if _, err := (application.AcceptCapabilityRevisionCommand{
		RecordID: "ACC-2", ArtifactID: CapabilityArtifactID, RevisionID: CapabilityRevision2, State: engineering.AcceptanceStateAccepted,
	}).Execute(ctx, uow, clock); err != nil {
		return Result{}, fmt.Errorf("accept revision 2: %w", err)
	}
	tick()

	// 9. Validation plan: activities A-1, A-2, A-3 (REQ-4 has none).
	if _, err := (application.EstablishValidationPlanCommand{
		ArtifactID: PlanArtifactID, RevisionID: PlanRevisionID, ScopeArtifactID: CapabilityArtifactID,
		Activities: []application.PlanActivityCommandInput{
			planActivity("A-1", "REQ-1", "manual-review", "Satisfied when the reviewer confirms student visibility is specified and traceable.", "Reviewer note confirming student visibility is specified"),
			planActivity("A-2", "REQ-2", "manual-review", "Satisfied when the reviewer confirms non-student access is excluded.", "Reviewer note confirming non-student access is excluded"),
			planActivity("A-3", "REQ-3", "manual-inspection", "Satisfied when the inspector confirms the attachment representation is specified as resolvable.", "Inspection note confirming the attachment representation is resolvable"),
		},
	}).Execute(ctx, uow, recorder, clock); err != nil {
		return Result{}, fmt.Errorf("establish validation plan: %w", err)
	}
	tick()

	// 10. Lifecycle: begin validation.
	if _, err := (application.AssignLifecycleStateCommand{
		AssignmentID: FirstAssignmentID, SubjectArtifactID: CapabilityArtifactID, State: "under-validation",
		TransitionRecordArtifactID: TransitionRecordArtifactID, TransitionRecordRevisionID: FirstTransitionRevisionID,
		TransitionKey: "begin-validation", FromAssignmentID: EntryAssignmentID,
	}).Execute(ctx, uow, recorder, clock); err != nil {
		return Result{}, fmt.Errorf("assign under-validation lifecycle state: %w", err)
	}
	tick()

	// 11. Execute activities A-1..A-3 (permutable order), each producing
	// evidence and a satisfied claim, then re-run A-2 and correct CLM-2.
	claimByRequirement := map[string]string{"A-1": ClaimForR1, "A-3": ClaimForR3}
	for _, group := range ord.activities {
		for _, key := range group {
			if key == "A-2" {
				if err := runActivity(ctx, uow, recorder, clock, key, ClaimIncorrect, "satisfied"); err != nil {
					return Result{}, err
				}
				continue
			}
			if err := runActivity(ctx, uow, recorder, clock, key, claimByRequirement[key], "satisfied"); err != nil {
				return Result{}, err
			}
		}
	}

	// 12. Correct CLM-2.
	if err := runCorrection(ctx, uow, recorder, clock); err != nil {
		return Result{}, err
	}

	claimIDs := []string{ClaimForR1, ClaimIncorrect, ClaimForR3, ClaimCorrecting}
	evidenceIDs := []string{DecisionEvidenceID, "EV-1", "EV-2", "EV-3", "EV-4"}
	executionIDs := []string{"ER-1", "ER-2", "ER-3", "ER-4"}

	return Result{
		ProjectID: ProjectID, FeatureCardID: FeatureCardID, CapabilityArtifactID: CapabilityArtifactID,
		RequirementArtifactIDs: RequirementArtifactIDs, DecisionID: DecisionID, PlanArtifactID: PlanArtifactID,
		ExecutionIDs: executionIDs, EvidenceArtifactIDs: evidenceIDs, ClaimIDs: claimIDs,
	}, nil
}

func recordEvidenceOnly(ctx context.Context, uow application.UnitOfWork, recorder application.EngineeringRecorder, clock *application.FixedClock, evidenceID, locator string) error {
	now := clock.Now()
	return uow.Do(ctx, func(r application.Repositories) error {
		artEnv, revEnv, err := recorder.RecordEvidence(engineering.EvidenceInput{
			ArtifactID: evidenceID, RevisionID: evidenceRevisionID(evidenceID), Locator: locator, RecordedAt: now,
		})
		if err != nil {
			return err
		}
		if err := r.Artifacts.Put(ctx, artEnv); err != nil {
			return err
		}
		return r.Revisions.Put(ctx, revEnv)
	})
}

func establishRequirement(ctx context.Context, uow application.UnitOfWork, recorder application.EngineeringRecorder, clock *application.FixedClock, artifactID string) error {
	_, err := (application.EstablishRequirementCommand{
		ArtifactID: artifactID, RevisionID: requirementRevisionID(artifactID),
		Statement: requirementStatements[artifactID], SubjectArtifactID: CapabilityArtifactID,
	}).Execute(ctx, uow, recorder, clock)
	if err != nil {
		return fmt.Errorf("establish requirement %s: %w", artifactID, err)
	}
	return nil
}

func planActivity(key, requirementArtifactID, method, interpretation, expectedEvidence string) application.PlanActivityCommandInput {
	return application.PlanActivityCommandInput{
		Key: key, SubjectArtifactID: CapabilityArtifactID, SubjectRevisionID: CapabilityRevision2,
		Method: method, OutcomeInterpretation: interpretation,
		RequirementArtifactID: requirementArtifactID, RequirementRevisionID: requirementRevisionID(requirementArtifactID),
		ExpectedEvidence: []string{expectedEvidence},
	}
}

func runActivity(ctx context.Context, uow application.UnitOfWork, recorder application.EngineeringRecorder, clock *application.FixedClock, activityKey, claimID, outcome string) error {
	requirementID := activityRequirement(activityKey)
	method := activityMethod(activityKey)
	executionID := ExecutionIDs[activityKey]
	evidenceID := EvidenceIDs[activityKey]

	if _, err := (application.RecordValidationRunCommand{
		ExecutionID: executionID, PlanArtifactID: PlanArtifactID, PlanRevisionID: PlanRevisionID, ActivityKey: activityKey,
		SubjectArtifactID: CapabilityArtifactID, SubjectRevisionID: CapabilityRevision2,
		Method: method, Outcome: "completed",
		EvidenceArtifactID: evidenceID, EvidenceRevisionID: evidenceRevisionID(evidenceID),
		EvidenceLocator: "https://evidence.example/" + evidenceID,
	}).Execute(ctx, uow, recorder, clock); err != nil {
		return fmt.Errorf("record validation run %s: %w", activityKey, err)
	}
	clock.Advance(time.Hour)

	if _, err := (application.RecordValidationClaimCommand{
		ClaimID: claimID, ScopeArtifactID: CapabilityArtifactID,
		SubjectArtifactID: CapabilityArtifactID, SubjectRevisionID: CapabilityRevision2,
		RequirementArtifactID: requirementID, RequirementRevisionID: requirementRevisionID(requirementID),
		Outcome: outcome, Method: method,
		EvidenceArtifactID: evidenceID, EvidenceRevisionID: evidenceRevisionID(evidenceID), ExecutionID: executionID,
		Reasoning: claimReasoning(activityKey),
	}).Execute(ctx, uow, recorder, clock); err != nil {
		return fmt.Errorf("record claim for %s: %w", activityKey, err)
	}
	clock.Advance(time.Hour)
	return nil
}

func runCorrection(ctx context.Context, uow application.UnitOfWork, recorder application.EngineeringRecorder, clock *application.FixedClock) error {
	rerunEvidenceID := EvidenceIDs["A-2-rerun"]
	rerunExecutionID := ExecutionIDs["A-2-rerun"]
	if _, err := (application.RecordValidationRunCommand{
		ExecutionID: rerunExecutionID, PlanArtifactID: PlanArtifactID, PlanRevisionID: PlanRevisionID, ActivityKey: "A-2",
		SubjectArtifactID: CapabilityArtifactID, SubjectRevisionID: CapabilityRevision2,
		Method: "manual-review", Outcome: "completed",
		EvidenceArtifactID: rerunEvidenceID, EvidenceRevisionID: evidenceRevisionID(rerunEvidenceID),
		EvidenceLocator: "https://evidence.example/" + rerunEvidenceID,
	}).Execute(ctx, uow, recorder, clock); err != nil {
		return fmt.Errorf("record re-run validation run: %w", err)
	}
	clock.Advance(time.Hour)

	if _, err := (application.CorrectValidationClaimCommand{
		ClaimID: ClaimCorrecting, CorrectionTarget: ClaimIncorrect, CorrectionKind: "correct",
		ScopeArtifactID: CapabilityArtifactID, SubjectArtifactID: CapabilityArtifactID, SubjectRevisionID: CapabilityRevision2,
		RequirementArtifactID: "REQ-2", RequirementRevisionID: requirementRevisionID("REQ-2"),
		Outcome: "not-satisfied", Method: "manual-review",
		EvidenceArtifactID: rerunEvidenceID, EvidenceRevisionID: evidenceRevisionID(rerunEvidenceID), ExecutionID: rerunExecutionID,
		Reasoning: "Revision 2 specifies who may view homework but does not state that other users are excluded. The original review treated the positive statement as implying the exclusion. It does not.",
	}).Execute(ctx, uow, recorder, clock); err != nil {
		return fmt.Errorf("correct claim: %w", err)
	}
	clock.Advance(time.Hour)
	return nil
}

func activityRequirement(activityKey string) string {
	switch activityKey {
	case "A-1":
		return "REQ-1"
	case "A-2":
		return "REQ-2"
	case "A-3":
		return "REQ-3"
	default:
		return ""
	}
}

func activityMethod(activityKey string) string {
	if activityKey == "A-3" {
		return "manual-inspection"
	}
	return "manual-review"
}

func claimReasoning(activityKey string) string {
	switch activityKey {
	case "A-1":
		return "The specification states student visibility explicitly."
	case "A-2":
		return "The specification states who may view homework."
	case "A-3":
		return "The specification names a resolvable representation for the attachment."
	default:
		return ""
	}
}
