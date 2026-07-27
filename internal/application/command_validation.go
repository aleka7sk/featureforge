package application

import (
	"context"
	"fmt"
	"time"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

// PlanActivityCommandInput is one planned activity within
// EstablishValidationPlanCommand.
type PlanActivityCommandInput struct {
	Key                   string
	SubjectArtifactID     string
	SubjectRevisionID     string
	Method                string
	OutcomeInterpretation string
	RequirementArtifactID string
	RequirementRevisionID string
	ExpectedEvidence      []string
}

// EstablishValidationPlanCommand records the Validation Plan Artifact and
// its founding revision together, for the same reason capability and
// requirement establishment do (FF-010 §3).
type EstablishValidationPlanCommand struct {
	ArtifactID      string
	RevisionID      string
	ScopeArtifactID string
	Activities      []PlanActivityCommandInput
}

// EstablishValidationPlanResult names the keys created.
type EstablishValidationPlanResult struct {
	ArtifactKey engineering.ArtifactKey
	RevisionKey engineering.RevisionKey
}

// Execute validates the command and writes the Plan Artifact and Revision
// in one transaction.
func (c EstablishValidationPlanCommand) Execute(ctx context.Context, uow UnitOfWork, recorder EngineeringRecorder, clock Clock) (EstablishValidationPlanResult, error) {
	if err := requireNonEmpty("artifact id", c.ArtifactID); err != nil {
		return EstablishValidationPlanResult{}, err
	}
	if err := requireNonEmpty("revision id", c.RevisionID); err != nil {
		return EstablishValidationPlanResult{}, err
	}
	if err := requireNonEmpty("scope artifact id", c.ScopeArtifactID); err != nil {
		return EstablishValidationPlanResult{}, err
	}
	if len(c.Activities) == 0 {
		return EstablishValidationPlanResult{}, fmt.Errorf("%w: a validation plan requires at least one activity", ErrInvalidCommand)
	}
	activities := make([]engineering.PlanActivityInput, len(c.Activities))
	for i, a := range c.Activities {
		activities[i] = engineering.PlanActivityInput{
			Key: a.Key, SubjectArtifactID: a.SubjectArtifactID, SubjectRevisionID: a.SubjectRevisionID,
			Method: a.Method, OutcomeInterpretation: a.OutcomeInterpretation,
			RequirementArtifactID: a.RequirementArtifactID, RequirementRevisionID: a.RequirementRevisionID,
			ExpectedEvidence: a.ExpectedEvidence,
		}
	}
	now := clock.Now()

	var result EstablishValidationPlanResult
	err := uow.Do(ctx, func(r Repositories) error {
		artifactEnv, revisionEnv, err := recorder.RecordValidationPlan(engineering.PlanInput{
			ArtifactID: c.ArtifactID, RevisionID: c.RevisionID, ScopeArtifactID: c.ScopeArtifactID,
			Activities: activities, RecordedAt: now,
		})
		if err != nil {
			return err
		}
		if err := r.Artifacts.Put(ctx, artifactEnv); err != nil {
			return err
		}
		if err := r.Revisions.Put(ctx, revisionEnv); err != nil {
			return err
		}
		result = EstablishValidationPlanResult{ArtifactKey: artifactEnv.Key, RevisionKey: revisionEnv.Key}
		return nil
	})
	if err != nil {
		return EstablishValidationPlanResult{}, err
	}
	return result, nil
}

// RecordValidationRunCommand writes the evidence Artifact and revision,
// then the execution record citing that evidence (FF-010 §3.4). One
// transaction: an execution record whose evidence failed to write must not
// exist.
type RecordValidationRunCommand struct {
	ExecutionID        string
	PlanArtifactID     string
	PlanRevisionID     string
	ActivityKey        string
	SubjectArtifactID  string
	SubjectRevisionID  string
	Method             string
	Outcome            string
	CompletedAt        time.Time
	EvidenceArtifactID string
	EvidenceRevisionID string
	EvidenceLocator    string
}

// RecordValidationRunResult names the records created.
type RecordValidationRunResult struct {
	EvidenceArtifactKey engineering.ArtifactKey
	EvidenceRevisionKey engineering.RevisionKey
	ExecutionKey        engineering.RecordKey
}

// Execute validates the command and writes the evidence and execution
// record in one transaction.
func (c RecordValidationRunCommand) Execute(ctx context.Context, uow UnitOfWork, recorder EngineeringRecorder, clock Clock) (RecordValidationRunResult, error) {
	for field, value := range map[string]string{
		"execution id": c.ExecutionID, "plan artifact id": c.PlanArtifactID, "plan revision id": c.PlanRevisionID,
		"activity key": c.ActivityKey, "subject artifact id": c.SubjectArtifactID, "subject revision id": c.SubjectRevisionID,
		"method": c.Method, "outcome": c.Outcome, "evidence artifact id": c.EvidenceArtifactID,
		"evidence revision id": c.EvidenceRevisionID, "evidence locator": c.EvidenceLocator,
	} {
		if err := requireNonEmpty(field, value); err != nil {
			return RecordValidationRunResult{}, err
		}
	}
	now := clock.Now()
	completedAt := requireTimeOrClock(c.CompletedAt, clock)

	var result RecordValidationRunResult
	err := uow.Do(ctx, func(r Repositories) error {
		evArtEnv, evRevEnv, err := recorder.RecordEvidence(engineering.EvidenceInput{
			ArtifactID: c.EvidenceArtifactID, RevisionID: c.EvidenceRevisionID, Locator: c.EvidenceLocator, RecordedAt: now,
		})
		if err != nil {
			return err
		}
		if err := r.Artifacts.Put(ctx, evArtEnv); err != nil {
			return err
		}
		if err := r.Revisions.Put(ctx, evRevEnv); err != nil {
			return err
		}

		execEnv, err := recorder.RecordExecution(engineering.ExecutionInput{
			ExecutionID: c.ExecutionID, PlanArtifactID: c.PlanArtifactID, PlanRevisionID: c.PlanRevisionID,
			ActivityKey: c.ActivityKey, SubjectArtifactID: c.SubjectArtifactID, SubjectRevisionID: c.SubjectRevisionID,
			Method: c.Method, Outcome: c.Outcome, CompletedAt: completedAt,
			EvidenceArtifactID: c.EvidenceArtifactID, EvidenceRevisionID: c.EvidenceRevisionID, RecordedAt: now,
		})
		if err != nil {
			return err
		}
		if err := r.Records.Put(ctx, execEnv); err != nil {
			return err
		}

		result = RecordValidationRunResult{EvidenceArtifactKey: evArtEnv.Key, EvidenceRevisionKey: evRevEnv.Key, ExecutionKey: execEnv.Key}
		return nil
	})
	if err != nil {
		return RecordValidationRunResult{}, err
	}
	return result, nil
}

// RecordValidationClaimCommand records a Validation Claim (FF-010 §3).
type RecordValidationClaimCommand struct {
	ClaimID               string
	ScopeArtifactID       string
	SubjectArtifactID     string
	SubjectRevisionID     string
	RequirementArtifactID string
	RequirementRevisionID string
	Outcome               string
	Method                string
	EvidenceArtifactID    string
	EvidenceRevisionID    string
	ExecutionID           string
	Reasoning             string
	Timestamp             time.Time
}

// RecordValidationClaimResult names the record created.
type RecordValidationClaimResult struct {
	Key engineering.RecordKey
}

// Execute validates the command and writes the Claim in one transaction.
func (c RecordValidationClaimCommand) Execute(ctx context.Context, uow UnitOfWork, recorder EngineeringRecorder, clock Clock) (RecordValidationClaimResult, error) {
	env, err := recordClaim(ctx, uow, recorder, engineering.ClaimInput{
		ClaimID: c.ClaimID, ScopeArtifactID: c.ScopeArtifactID,
		SubjectArtifactID: c.SubjectArtifactID, SubjectRevisionID: c.SubjectRevisionID,
		RequirementArtifactID: c.RequirementArtifactID, RequirementRevisionID: c.RequirementRevisionID,
		Outcome: c.Outcome, Method: c.Method,
		EvidenceArtifactID: c.EvidenceArtifactID, EvidenceRevisionID: c.EvidenceRevisionID,
		ExecutionID: c.ExecutionID, Reasoning: c.Reasoning, Timestamp: requireTimeOrClock(c.Timestamp, clock),
		RecordedAt: clock.Now(),
	})
	if err != nil {
		return RecordValidationClaimResult{}, err
	}
	return RecordValidationClaimResult{Key: env.Key}, nil
}

// CorrectValidationClaimCommand records a new Claim carrying a correction
// reference to an earlier one (FF-010 §3.5). It has four invariants
// RecordValidationClaim does not: the target must exist, must be a claim,
// the resulting chain must have no cycle, and PEOS v1.0.0 accepts a
// self-correction (verified) so FeatureForge rejects it explicitly here
// (AD-017), before any PEOS construction.
type CorrectValidationClaimCommand struct {
	ClaimID               string
	CorrectionTarget      string
	CorrectionKind        string
	ScopeArtifactID       string
	SubjectArtifactID     string
	SubjectRevisionID     string
	RequirementArtifactID string
	RequirementRevisionID string
	Outcome               string
	Method                string
	EvidenceArtifactID    string
	EvidenceRevisionID    string
	ExecutionID           string
	Reasoning             string
	Timestamp             time.Time
}

// CorrectValidationClaimResult names the record created.
type CorrectValidationClaimResult struct {
	Key engineering.RecordKey
}

// Execute validates the correction-specific invariants, then everything
// RecordValidationClaim validates, and writes the correcting Claim in one
// transaction.
func (c CorrectValidationClaimCommand) Execute(ctx context.Context, uow UnitOfWork, recorder EngineeringRecorder, clock Clock) (CorrectValidationClaimResult, error) {
	if err := requireNonEmpty("correction target", c.CorrectionTarget); err != nil {
		return CorrectValidationClaimResult{}, err
	}
	if c.CorrectionTarget == c.ClaimID {
		return CorrectValidationClaimResult{}, fmt.Errorf("%w: claim %q cannot correct itself", ErrCorrectionSelfReference, c.ClaimID)
	}
	switch c.CorrectionKind {
	case "correct", "replace", "invalidate":
	default:
		return CorrectValidationClaimResult{}, fmt.Errorf("%w: unsupported correction kind %q", ErrInvalidCommand, c.CorrectionKind)
	}

	timestamp := requireTimeOrClock(c.Timestamp, clock)
	var env engineering.RecordEnvelope
	err := uow.Do(ctx, func(r Repositories) error {
		targetKey, err := engineering.NewRecordKey(engineering.RecordKindClaim, c.CorrectionTarget)
		if err != nil {
			return err
		}
		if _, found, err := r.Records.Get(ctx, targetKey); err != nil {
			return err
		} else if !found {
			return fmt.Errorf("%w: claim %s", ErrCorrectionTargetMissing, c.CorrectionTarget)
		}

		in := engineering.ClaimInput{
			ClaimID: c.ClaimID, ScopeArtifactID: c.ScopeArtifactID,
			SubjectArtifactID: c.SubjectArtifactID, SubjectRevisionID: c.SubjectRevisionID,
			RequirementArtifactID: c.RequirementArtifactID, RequirementRevisionID: c.RequirementRevisionID,
			Outcome: c.Outcome, Method: c.Method,
			EvidenceArtifactID: c.EvidenceArtifactID, EvidenceRevisionID: c.EvidenceRevisionID,
			ExecutionID: c.ExecutionID, Reasoning: c.Reasoning, Timestamp: timestamp, RecordedAt: clock.Now(),
			HasCorrection: true, CorrectionKind: c.CorrectionKind, CorrectionTarget: c.CorrectionTarget,
		}
		if err := validateClaimInput(in); err != nil {
			return err
		}
		built, err := recorder.RecordClaim(in)
		if err != nil {
			return err
		}
		if err := r.Records.Put(ctx, built); err != nil {
			return err
		}
		env = built

		// Confirm the resulting chain has no cycle by resolving it; a
		// cycle surfaces as ErrCorrectionCycle from ResolveCurrentClaim.
		_, err = ResolveCurrentClaim(ctx, r, env.SubjectKey, env.Scope, env.CriterionKeys)
		return err
	})
	if err != nil {
		return CorrectValidationClaimResult{}, err
	}
	return CorrectValidationClaimResult{Key: env.Key}, nil
}

// recordClaim is the shared write path for RecordValidationClaim and
// CorrectValidationClaim.
func recordClaim(ctx context.Context, uow UnitOfWork, recorder EngineeringRecorder, in engineering.ClaimInput) (engineering.RecordEnvelope, error) {
	if err := validateClaimInput(in); err != nil {
		return engineering.RecordEnvelope{}, err
	}
	var env engineering.RecordEnvelope
	err := uow.Do(ctx, func(r Repositories) error {
		built, err := recorder.RecordClaim(in)
		if err != nil {
			return err
		}
		if err := r.Records.Put(ctx, built); err != nil {
			return err
		}
		env = built
		return nil
	})
	if err != nil {
		return engineering.RecordEnvelope{}, err
	}
	return env, nil
}

func validateClaimInput(in engineering.ClaimInput) error {
	for field, value := range map[string]string{
		"claim id": in.ClaimID, "scope artifact id": in.ScopeArtifactID,
		"subject artifact id": in.SubjectArtifactID, "subject revision id": in.SubjectRevisionID,
		"requirement artifact id": in.RequirementArtifactID, "requirement revision id": in.RequirementRevisionID,
		"outcome": in.Outcome, "method": in.Method,
		"evidence artifact id": in.EvidenceArtifactID, "evidence revision id": in.EvidenceRevisionID,
		"execution id": in.ExecutionID,
	} {
		if err := requireNonEmpty(field, value); err != nil {
			return err
		}
	}
	return nil
}
