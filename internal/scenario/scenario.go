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
// recorder and replay inspector. clock is advanced by the driver between acts
// so provenance and timeline timestamps are strictly increasing, matching a
// real sequence of engineering acts.
func Run(ctx context.Context, uow application.UnitOfWork, recorder application.EngineeringRecorder, inspector application.EngineeringReplayInspector, clock *application.FixedClock) (Result, error) {
	return run(ctx, uow, recorder, inspector, clock, defaultOrder())
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
func RunPermuted(ctx context.Context, uow application.UnitOfWork, recorder application.EngineeringRecorder, inspector application.EngineeringReplayInspector, clock *application.FixedClock) (Result, error) {
	return run(ctx, uow, recorder, inspector, clock, order{
		requirements: []string{"REQ-2", "REQ-1"},
		activities:   [][]string{{"A-3"}, {"A-1"}, {"A-2"}},
	})
}

func run(ctx context.Context, uow application.UnitOfWork, recorder application.EngineeringRecorder, inspector application.EngineeringReplayInspector, clock *application.FixedClock, ord order) (Result, error) {
	tick := func() { clock.Advance(time.Hour) }
	if err := application.EnsureLifecycleConfiguration(ctx, uow, recorder, inspector); err != nil {
		return Result{}, fmt.Errorf("initialize lifecycle configuration: %w", err)
	}

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
	}).Execute(ctx, uow, recorder, inspector, clock); err != nil {
		return Result{}, fmt.Errorf("establish capability specification: %w", err)
	}
	tick()

	// 4. Accept Revision 1.
	if _, err := (application.AcceptCapabilityRevisionCommand{
		RecordID: "ACC-1", ArtifactID: CapabilityArtifactID, RevisionID: CapabilityRevision1, State: engineering.AcceptanceStateAccepted,
	}).Execute(ctx, uow, inspector, clock); err != nil {
		return Result{}, fmt.Errorf("accept revision 1: %w", err)
	}
	tick()

	// 5. Lifecycle entry: drafting.
	if _, err := (application.AssignLifecycleStateCommand{
		AssignmentID: EntryAssignmentID, SubjectArtifactID: CapabilityArtifactID, State: "drafting", IsEntry: true,
		TransitionRecordArtifactID: TransitionRecordArtifactID, TransitionRecordRevisionID: EntryTransitionRevisionID,
	}).Execute(ctx, uow, recorder, inspector, clock); err != nil {
		return Result{}, fmt.Errorf("assign entry lifecycle state: %w", err)
	}
	tick()

	// 6. Decision evidence (pilot-teacher interview notes), then the
	// decision itself, resolving Revision 1's open questions. Recording
	// evidence has no execution to pair it with here -- unlike A-1..A-3's
	// evidence, which RecordValidationRunCommand bundles with an execution
	// record -- so the scenario driver records it directly through the
	// recorder, exactly as RecordValidationRunCommand does internally,
	// without introducing an eleventh command beyond FF-010 §3's ten.
	if err := recordEvidenceOnly(ctx, uow, recorder, inspector, clock, DecisionEvidenceID, "https://evidence.example/"+DecisionEvidenceID); err != nil {
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
	}).Execute(ctx, uow, recorder, inspector, clock); err != nil {
		return Result{}, fmt.Errorf("record decision: %w", err)
	}
	tick()

	// 7. Capability Revision 2, then accept it.
	rev2Content, err := capabilityRevision2Content()
	if err != nil {
		return Result{}, err
	}
	if _, err := (application.ReviseCapabilitySpecificationCommand{
		ArtifactID: CapabilityArtifactID, RevisionID: CapabilityRevision2, Content: rev2Content,
	}).Execute(ctx, uow, recorder, inspector, clock); err != nil {
		return Result{}, fmt.Errorf("revise capability specification: %w", err)
	}
	tick()
	if _, err := (application.AcceptCapabilityRevisionCommand{
		RecordID: "ACC-2", ArtifactID: CapabilityArtifactID, RevisionID: CapabilityRevision2, State: engineering.AcceptanceStateAccepted,
	}).Execute(ctx, uow, inspector, clock); err != nil {
		return Result{}, fmt.Errorf("accept revision 2: %w", err)
	}
	tick()

	// 8. Requirements REQ-1, REQ-2 (permutable), then REQ-3, REQ-4.
	// AD-033 makes their exact source CAP-1-REV-2 / AC-1..AC-4 part of
	// persisted C7 state, so they are established only after that revision
	// has become the accepted current capability revision.
	for _, artifactID := range ord.requirements {
		if err := establishRequirement(ctx, uow, recorder, inspector, clock, artifactID); err != nil {
			return Result{}, err
		}
		tick()
	}
	for _, artifactID := range []string{"REQ-3", "REQ-4"} {
		if err := establishRequirement(ctx, uow, recorder, inspector, clock, artifactID); err != nil {
			return Result{}, err
		}
		tick()
	}

	// 9. Enter the specified milestone after the accepted current capability
	// revision and its traced effective Requirements exist.
	if _, err := (application.AssignLifecycleStateCommand{
		AssignmentID: SpecifiedAssignmentID, SubjectArtifactID: CapabilityArtifactID, State: "specified",
		TransitionRecordArtifactID: TransitionRecordArtifactID, TransitionRecordRevisionID: SpecifyTransitionRevisionID,
		TransitionKey: "specify", FromAssignmentID: EntryAssignmentID,
	}).Execute(ctx, uow, recorder, inspector, clock); err != nil {
		return Result{}, fmt.Errorf("assign specified lifecycle state: %w", err)
	}
	tick()

	// 10. Validation plan: activities A-1, A-2, A-3 (REQ-4 has none).
	if _, err := (application.EstablishValidationPlanCommand{
		ArtifactID: PlanArtifactID, RevisionID: PlanRevisionID, ScopeArtifactID: CapabilityArtifactID,
		AcceptanceRecordID: stringPointer("ACC-VP-1"),
		Activities: []application.PlanActivityCommandInput{
			planActivity("A-1", "REQ-1", "manual-review", "Satisfied when the reviewer confirms student visibility is specified and traceable.", "Reviewer note confirming student visibility is specified"),
			planActivity("A-2", "REQ-2", "manual-review", "Satisfied when the reviewer confirms non-student access is excluded.", "Reviewer note confirming non-student access is excluded"),
			planActivity("A-3", "REQ-3", "manual-inspection", "Satisfied when the inspector confirms the attachment representation is specified as resolvable.", "Inspection note confirming the attachment representation is resolvable"),
		},
	}).Execute(ctx, uow, recorder, inspector, clock); err != nil {
		return Result{}, fmt.Errorf("establish validation plan: %w", err)
	}
	tick()

	// 11. Execute the first permuted activity and record its evidence. That
	// completed execution is the begin-validation milestone's support; the
	// claim is deliberately recorded only after the lifecycle transition.
	claimByRequirement := map[string]string{"A-1": ClaimForR1, "A-3": ClaimForR3}
	firstActivity := true
	for _, group := range ord.activities {
		for _, key := range group {
			claimID := claimByRequirement[key]
			if key == "A-2" {
				claimID = ClaimIncorrect
			}
			if firstActivity {
				if err := recordActivityRun(ctx, uow, recorder, inspector, clock, key); err != nil {
					return Result{}, err
				}
				tick()
				if _, err := (application.AssignLifecycleStateCommand{
					AssignmentID: UnderValidationAssignmentID, SubjectArtifactID: CapabilityArtifactID, State: "under-validation",
					TransitionRecordArtifactID: TransitionRecordArtifactID, TransitionRecordRevisionID: BeginValidationTransitionRevisionID,
					TransitionKey: "begin-validation", FromAssignmentID: SpecifiedAssignmentID,
				}).Execute(ctx, uow, recorder, inspector, clock); err != nil {
					return Result{}, fmt.Errorf("assign under-validation lifecycle state: %w", err)
				}
				tick()
				if err := recordActivityClaim(ctx, uow, recorder, inspector, clock, key, claimID, "satisfied"); err != nil {
					return Result{}, err
				}
				tick()
				firstActivity = false
				continue
			}
			if key == "A-2" {
				if err := runActivity(ctx, uow, recorder, inspector, clock, key, ClaimIncorrect, "satisfied"); err != nil {
					return Result{}, err
				}
				continue
			}
			if err := runActivity(ctx, uow, recorder, inspector, clock, key, claimByRequirement[key], "satisfied"); err != nil {
				return Result{}, err
			}
		}
	}

	// 12. Correct CLM-2.
	if err := runCorrection(ctx, uow, recorder, inspector, clock); err != nil {
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

func recordEvidenceOnly(ctx context.Context, uow application.UnitOfWork, recorder application.EngineeringRecorder, inspector application.EngineeringReplayInspector, clock *application.FixedClock, evidenceID, locator string) error {
	now := clock.Now()
	return uow.Do(ctx, func(r application.Repositories) error {
		artifactKey := engineering.ArtifactKey{ArtifactID: evidenceID}
		revisionKey := engineering.RevisionKey{ArtifactID: evidenceID, RevisionID: evidenceRevisionID(evidenceID)}
		storedArtifact, artifactFound, err := r.Artifacts.Get(ctx, artifactKey)
		if err != nil {
			return err
		}
		storedRevision, revisionFound, err := r.Revisions.Get(ctx, revisionKey)
		if err != nil {
			return err
		}
		if artifactFound != revisionFound {
			return fmt.Errorf("%w: direct decision evidence is only partially persisted", application.ErrStoredStateIntegrity)
		}
		if artifactFound {
			if inspector == nil {
				return fmt.Errorf("%w: replay inspector is unavailable", application.ErrStoredStateIntegrity)
			}
			if err := inspector.ValidateEvidenceArtifact(storedArtifact); err != nil {
				return fmt.Errorf("%w: invalid direct decision-evidence artifact: %v", application.ErrStoredStateIntegrity, err)
			}
			if err := inspector.ValidateRevision(storedRevision); err != nil {
				return fmt.Errorf("%w: invalid direct decision-evidence revision: %v", application.ErrStoredStateIntegrity, err)
			}
			if storedRevision.RevisionFamily != engineering.RevisionFamilyEvidence || storedArtifact.ArtifactType != storedRevision.ArtifactType {
				return fmt.Errorf("%w: direct decision-evidence members disagree on family", application.ErrStoredStateIntegrity)
			}
			if !storedArtifact.RecordedAt.Equal(storedRevision.RecordedAt) {
				return fmt.Errorf("%w: direct decision-evidence members disagree on recorded time", application.ErrStoredStateIntegrity)
			}
			revisions, err := r.Revisions.ListByArtifact(ctx, evidenceID)
			if err != nil {
				return err
			}
			if len(revisions) != 1 || revisions[0].Key != revisionKey {
				return fmt.Errorf("%w: direct decision evidence must own exactly one revision", application.ErrStoredStateIntegrity)
			}
			if _, found, err := r.StructuredContent.Get(ctx, revisionKey); err != nil {
				return err
			} else if found {
				return fmt.Errorf("%w: direct decision evidence carries capability content", application.ErrStoredStateIntegrity)
			}
			orders, err := r.RevisionOrder.ListByArtifact(ctx, evidenceID)
			if err != nil {
				return err
			}
			acceptance, err := r.RevisionAcceptance.ListByArtifact(ctx, evidenceID)
			if err != nil {
				return err
			}
			if len(orders) != 0 || len(acceptance) != 0 {
				return fmt.Errorf("%w: direct decision evidence carries managed revision metadata", application.ErrStoredStateIntegrity)
			}
			executions, err := r.Records.ListByKind(ctx, engineering.RecordKindExecution)
			if err != nil {
				return err
			}
			for _, execution := range executions {
				if err := inspector.ValidateRecord(execution); err != nil {
					return fmt.Errorf("%w: invalid execution while checking direct decision evidence: %v", application.ErrStoredStateIntegrity, err)
				}
				for _, rawEvidenceKey := range execution.EvidenceKeys {
					artifactID, revisionID, err := engineering.ParseEvidenceKey(rawEvidenceKey)
					if err != nil {
						return fmt.Errorf("%w: malformed execution evidence key: %v", application.ErrStoredStateIntegrity, err)
					}
					if artifactID == evidenceID && revisionID == revisionKey.RevisionID {
						return fmt.Errorf("%w: direct decision evidence is also owned by a validation execution", application.ErrStoredStateIntegrity)
					}
				}
			}
			expectedArtifact, expectedRevision, err := recorder.RecordEvidence(engineering.EvidenceInput{
				ArtifactID: evidenceID, RevisionID: revisionKey.RevisionID, Locator: locator, RecordedAt: storedRevision.RecordedAt,
			})
			if err != nil {
				return fmt.Errorf("%w: rebuild direct decision evidence: %v", application.ErrInvalidCommand, err)
			}
			if !storedArtifact.Equal(expectedArtifact) || !storedRevision.Equal(expectedRevision) {
				return fmt.Errorf("%w: direct decision-evidence identity has different immutable semantics", application.ErrImmutableValueConflict)
			}
			return nil
		}

		artEnv, revEnv, err := recorder.RecordEvidence(engineering.EvidenceInput{
			ArtifactID: evidenceID, RevisionID: revisionKey.RevisionID, Locator: locator, RecordedAt: now,
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

func establishRequirement(ctx context.Context, uow application.UnitOfWork, recorder application.EngineeringRecorder, inspector application.EngineeringReplayInspector, clock *application.FixedClock, artifactID string) error {
	_, err := (application.EstablishRequirementCommand{
		ArtifactID: artifactID, RevisionID: requirementRevisionID(artifactID),
		Statement: requirementStatements[artifactID], SubjectArtifactID: CapabilityArtifactID,
		SourceCapabilityRevisionID: CapabilityRevision2, SourceAcceptanceCriterionKey: requirementCriterionKeys[artifactID],
		AcceptanceRecordID: stringPointer("ACC-" + artifactID),
	}).Execute(ctx, uow, recorder, inspector, clock)
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

func runActivity(ctx context.Context, uow application.UnitOfWork, recorder application.EngineeringRecorder, inspector application.EngineeringReplayInspector, clock *application.FixedClock, activityKey, claimID, outcome string) error {
	if err := recordActivityRun(ctx, uow, recorder, inspector, clock, activityKey); err != nil {
		return err
	}
	clock.Advance(time.Hour)
	if err := recordActivityClaim(ctx, uow, recorder, inspector, clock, activityKey, claimID, outcome); err != nil {
		return err
	}
	clock.Advance(time.Hour)
	return nil
}

func recordActivityRun(ctx context.Context, uow application.UnitOfWork, recorder application.EngineeringRecorder, inspector application.EngineeringReplayInspector, clock *application.FixedClock, activityKey string) error {
	method := activityMethod(activityKey)
	executionID := ExecutionIDs[activityKey]
	evidenceID := EvidenceIDs[activityKey]

	if _, err := (application.RecordValidationRunCommand{
		ExecutionID: executionID, PlanArtifactID: PlanArtifactID, PlanRevisionID: PlanRevisionID, ActivityKey: activityKey,
		SubjectArtifactID: CapabilityArtifactID, SubjectRevisionID: CapabilityRevision2,
		Method: method, Outcome: "completed",
		EvidenceArtifactID: evidenceID, EvidenceRevisionID: evidenceRevisionID(evidenceID),
		EvidenceLocator: "https://evidence.example/" + evidenceID,
	}).Execute(ctx, uow, recorder, inspector, clock); err != nil {
		return fmt.Errorf("record validation run %s: %w", activityKey, err)
	}
	return nil
}

func recordActivityClaim(ctx context.Context, uow application.UnitOfWork, recorder application.EngineeringRecorder, inspector application.EngineeringReplayInspector, clock *application.FixedClock, activityKey, claimID, outcome string) error {
	requirementID := activityRequirement(activityKey)
	method := activityMethod(activityKey)
	executionID := ExecutionIDs[activityKey]
	evidenceID := EvidenceIDs[activityKey]
	if _, err := (application.RecordValidationClaimCommand{
		ClaimID: claimID, ScopeArtifactID: CapabilityArtifactID,
		SubjectArtifactID: CapabilityArtifactID, SubjectRevisionID: CapabilityRevision2,
		RequirementArtifactID: requirementID, RequirementRevisionID: requirementRevisionID(requirementID),
		Outcome: outcome, Method: method,
		EvidenceArtifactID: evidenceID, EvidenceRevisionID: evidenceRevisionID(evidenceID), ExecutionID: executionID,
		Reasoning: claimReasoning(activityKey),
	}).Execute(ctx, uow, recorder, inspector, clock); err != nil {
		return fmt.Errorf("record claim for %s: %w", activityKey, err)
	}
	return nil
}

func runCorrection(ctx context.Context, uow application.UnitOfWork, recorder application.EngineeringRecorder, inspector application.EngineeringReplayInspector, clock *application.FixedClock) error {
	rerunEvidenceID := EvidenceIDs["A-2-rerun"]
	rerunExecutionID := ExecutionIDs["A-2-rerun"]
	if _, err := (application.RecordValidationRunCommand{
		ExecutionID: rerunExecutionID, PlanArtifactID: PlanArtifactID, PlanRevisionID: PlanRevisionID, ActivityKey: "A-2",
		SubjectArtifactID: CapabilityArtifactID, SubjectRevisionID: CapabilityRevision2,
		Method: "manual-review", Outcome: "completed",
		EvidenceArtifactID: rerunEvidenceID, EvidenceRevisionID: evidenceRevisionID(rerunEvidenceID),
		EvidenceLocator: "https://evidence.example/" + rerunEvidenceID,
	}).Execute(ctx, uow, recorder, inspector, clock); err != nil {
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
	}).Execute(ctx, uow, recorder, inspector, clock); err != nil {
		return fmt.Errorf("correct claim: %w", err)
	}
	clock.Advance(time.Hour)
	return nil
}

func stringPointer(value string) *string { return &value }

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
