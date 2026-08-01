package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

func lifecycleTransitionInvalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrLifecycleTransitionInvalid, fmt.Sprintf(format, args...))
}

// validateLifecycleProductPrecondition evaluates the AD-032 entry milestone
// for a genuinely new transition. It is intentionally state-at-attempt
// validation: stored lifecycle history is never retroactively re-evaluated.
func validateLifecycleProductPrecondition(
	ctx context.Context,
	repos Repositories,
	inspector EngineeringReplayInspector,
	transitionID, capabilityArtifactID string,
	completedAt time.Time,
) error {
	current, err := lifecycleCurrentCapability(ctx, repos, inspector, capabilityArtifactID)
	if err != nil {
		return err
	}
	if current.Revision.RecordedAt.After(completedAt) {
		return lifecycleTransitionInvalid("current capability revision was recorded after transition completion")
	}
	if err := acceptanceMemberNotAfter(ctx, repos, current.Revision.Key, completedAt, "current capability revision"); err != nil {
		return err
	}

	switch transitionID {
	case "specify":
		return validateSpecifyPrecondition(ctx, repos, inspector, current, completedAt)
	case "begin-validation":
		_, err := lifecycleCurrentPlanAndExecution(ctx, repos, inspector, current, completedAt)
		return err
	case "assess":
		return validateAssessPrecondition(ctx, repos, inspector, current, completedAt)
	default:
		return lifecycleTransitionInvalid("transition %q has no governed product precondition", transitionID)
	}
}

func lifecycleCurrentCapability(ctx context.Context, repos Repositories, inspector EngineeringReplayInspector, artifactID string) (CurrentRevisionResult, error) {
	if _, err := validateManagedHistory(ctx, repos, inspector, artifactID, engineering.RevisionFamilyCapability, false); err != nil {
		return CurrentRevisionResult{}, err
	}
	current, err := ResolveCurrentRevision(ctx, repos, artifactID)
	if err != nil {
		return CurrentRevisionResult{}, err
	}
	if !current.Found {
		return CurrentRevisionResult{}, lifecycleTransitionInvalid("capability %s has no current accepted revision", artifactID)
	}
	return current, nil
}

func validateSpecifyPrecondition(ctx context.Context, repos Repositories, inspector EngineeringReplayInspector, current CurrentRevisionResult, completedAt time.Time) error {
	requirementIDs, err := DiscoverRequirementArtifactIDs(ctx, repos, inspector, current.Revision.Key.ArtifactID)
	if err != nil {
		return err
	}
	effective, err := ResolveEffectiveRequirements(ctx, repos, inspector, requirementIDs)
	if err != nil {
		return err
	}
	for _, requirement := range effective {
		if requirement.SourceCapabilityRevision != current.Revision.Key {
			continue
		}
		if err := requirementSupportNotAfter(ctx, repos, requirement, completedAt); err != nil {
			if isLifecycleTimingFailure(err) {
				continue
			}
			return err
		}
		return nil
	}
	return lifecycleTransitionInvalid("specify requires an effective Requirement traced to the current capability revision by completion time")
}

func lifecycleCurrentPlanAndExecution(ctx context.Context, repos Repositories, inspector EngineeringReplayInspector, current CurrentRevisionResult, completedAt time.Time) (CurrentRevisionResult, error) {
	planID, err := ResolveApplicableValidationPlanID(ctx, repos, inspector, current.Revision.Key.ArtifactID)
	if err != nil {
		return CurrentRevisionResult{}, err
	}
	if planID == "" {
		return CurrentRevisionResult{}, lifecycleTransitionInvalid("begin-validation requires one applicable validation plan")
	}
	currentPlan, err := ResolveCurrentRevision(ctx, repos, planID)
	if err != nil {
		return CurrentRevisionResult{}, err
	}
	if !currentPlan.Found {
		return CurrentRevisionResult{}, lifecycleTransitionInvalid("begin-validation requires a current accepted validation plan")
	}
	if currentPlan.Revision.RecordedAt.After(completedAt) {
		return CurrentRevisionResult{}, lifecycleTransitionInvalid("current validation plan was recorded after transition completion")
	}
	if err := acceptanceMemberNotAfter(ctx, repos, currentPlan.Revision.Key, completedAt, "current validation plan"); err != nil {
		return CurrentRevisionResult{}, err
	}

	executions, err := listValidatedRecordsByKindAndSubject(ctx, repos, inspector, engineering.RecordKindExecution,
		engineering.ArtifactRevisionSubjectKey(current.Revision.Key.ArtifactID, current.Revision.Key.RevisionID))
	if err != nil {
		return CurrentRevisionResult{}, err
	}
	foundTimely := false
	for _, execution := range executions {
		_, evidence, _, err := validateStoredExecutionAct(ctx, repos, inspector, execution)
		if err != nil {
			return CurrentRevisionResult{}, err
		}
		planKey, _, _, err := inspector.ExecutionPlanActivity(execution)
		if err != nil {
			return CurrentRevisionResult{}, integrityError("execution plan activity is unreadable", err)
		}
		if planKey != currentPlan.Revision.Key || execution.Outcome != "peos:completed" {
			continue
		}
		if !execution.HasOccurredAt || execution.OccurredAt.After(completedAt) || execution.RecordedAt.After(completedAt) || evidence.RecordedAt.After(completedAt) {
			continue
		}
		foundTimely = true
	}
	if !foundTimely {
		return CurrentRevisionResult{}, lifecycleTransitionInvalid("begin-validation requires a completed execution of the current plan against the current capability revision by completion time")
	}
	return currentPlan, nil
}

func validateAssessPrecondition(ctx context.Context, repos Repositories, inspector EngineeringReplayInspector, current CurrentRevisionResult, completedAt time.Time) error {
	requirementIDs, err := DiscoverRequirementArtifactIDs(ctx, repos, inspector, current.Revision.Key.ArtifactID)
	if err != nil {
		return err
	}
	effective, err := ResolveEffectiveRequirements(ctx, repos, inspector, requirementIDs)
	if err != nil {
		return err
	}
	if len(effective) == 0 {
		return lifecycleTransitionInvalid("assess requires at least one effective Requirement")
	}
	for _, requirement := range effective {
		if err := requirementSupportNotAfter(ctx, repos, requirement, completedAt); err != nil {
			return err
		}
	}
	readiness, err := ResolveReadiness(ctx, repos, inspector, current.Revision, effective)
	if err != nil {
		return err
	}
	for _, requirement := range readiness.PerRequirement {
		if !requirement.HasClaim {
			return lifecycleTransitionInvalid("assess requires an applicable current Claim for Requirement %s", requirement.RequirementArtifactID)
		}
		claim := requirement.Claim
		if err := inspectRecord(inspector, claim); err != nil {
			return err
		}
		if err := validateStoredClaimReferences(ctx, repos, inspector, claim); err != nil {
			return err
		}
		if !claim.HasOccurredAt || claim.OccurredAt.After(completedAt) || claim.RecordedAt.After(completedAt) {
			return lifecycleTransitionInvalid("Claim %s was not available by transition completion", claim.Key.ID)
		}
		if len(claim.ExecutionKeys) != 1 {
			return integrityError("applicable claim has an invalid execution projection", nil)
		}
		executionID, ok := strings.CutPrefix(claim.ExecutionKeys[0], "execution:")
		if !ok || executionID == "" {
			return integrityError("applicable claim has a malformed execution identity", nil)
		}
		executionKey, _ := engineering.NewRecordKey(engineering.RecordKindExecution, executionID)
		execution, found, err := repos.Records.Get(ctx, executionKey)
		if err != nil {
			return err
		}
		if !found {
			return integrityError("applicable claim has a dangling execution", nil)
		}
		_, evidence, _, err := validateStoredExecutionAct(ctx, repos, inspector, execution)
		if err != nil {
			return err
		}
		if execution.SubjectKey != engineering.ArtifactRevisionSubjectKey(current.Revision.Key.ArtifactID, current.Revision.Key.RevisionID) {
			return lifecycleTransitionInvalid("Claim %s is not backed by the current capability revision", claim.Key.ID)
		}
		if !execution.HasOccurredAt || execution.OccurredAt.After(completedAt) || execution.RecordedAt.After(completedAt) || evidence.RecordedAt.After(completedAt) {
			return lifecycleTransitionInvalid("Claim %s support was not available by transition completion", claim.Key.ID)
		}
	}
	return nil
}

func acceptanceMemberNotAfter(ctx context.Context, repos Repositories, key engineering.RevisionKey, completedAt time.Time, label string) error {
	journal, err := repos.RevisionAcceptance.ListByRevision(ctx, key)
	if err != nil {
		return err
	}
	member, err := semanticAcceptanceMember(key, journal)
	if err != nil {
		return err
	}
	if member.EffectiveAt.After(completedAt) {
		return lifecycleTransitionInvalid("%s was accepted after transition completion", label)
	}
	return nil
}

func requirementSupportNotAfter(ctx context.Context, repos Repositories, requirement EffectiveRequirement, completedAt time.Time) error {
	revision, found, err := repos.Revisions.Get(ctx, requirement.RevisionKey)
	if err != nil {
		return err
	}
	if !found {
		return integrityError("effective requirement revision disappeared during lifecycle validation", nil)
	}
	if revision.RecordedAt.After(completedAt) {
		return lifecycleTransitionInvalid("effective Requirement %s was recorded after transition completion", requirement.ArtifactID)
	}
	if err := acceptanceMemberNotAfter(ctx, repos, requirement.RevisionKey, completedAt, "effective Requirement "+requirement.ArtifactID); err != nil {
		return err
	}
	source, found, err := repos.Revisions.Get(ctx, requirement.SourceCapabilityRevision)
	if err != nil {
		return err
	}
	if !found {
		return integrityError("effective requirement trace source disappeared during lifecycle validation", nil)
	}
	if source.RecordedAt.After(completedAt) {
		return lifecycleTransitionInvalid("Requirement %s trace source was recorded after transition completion", requirement.ArtifactID)
	}
	return acceptanceMemberNotAfter(ctx, repos, source.Key, completedAt, "Requirement "+requirement.ArtifactID+" trace source")
}

func isLifecycleTimingFailure(err error) bool {
	return errors.Is(err, ErrLifecycleTransitionInvalid)
}
