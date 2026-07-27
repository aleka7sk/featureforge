package application

import (
	"context"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

// EngineeringStateInput names the records GetFeatureEngineeringState draws
// from, for the same reason TimelineInput does (FF-009 §5 defines no
// requirement-to-capability index).
type EngineeringStateInput struct {
	CapabilityArtifactID   string
	RequirementArtifactIDs []string
	DecisionIDs            []string
}

// ApplicableDecision is one decision found to name the capability as a
// subject, with the rationale for how it was matched (FF-004 §3.3).
type ApplicableDecision struct {
	DecisionID string
	Decision   engineering.RecordEnvelope
}

// EngineeringStateResult bundles every current-state answer for one
// feature, each with its own rationale (FF-010 §10).
type EngineeringStateResult struct {
	CurrentRevision       CurrentRevisionResult
	EffectiveRequirements []EffectiveRequirement
	ApplicableDecisions   []ApplicableDecision
	Readiness             ReadinessResult
	Lifecycle             LifecycleStateResult
}

// GetFeatureEngineeringState composes every current-state query into one
// result (FF-010 §10). It is read-only and deterministic.
func GetFeatureEngineeringState(ctx context.Context, repos Repositories, in EngineeringStateInput) (EngineeringStateResult, error) {
	var result EngineeringStateResult

	currentRevision, err := ResolveCurrentRevision(ctx, repos, in.CapabilityArtifactID)
	if err != nil {
		return EngineeringStateResult{}, err
	}
	result.CurrentRevision = currentRevision

	effective, err := ResolveEffectiveRequirements(ctx, repos, in.RequirementArtifactIDs)
	if err != nil {
		return EngineeringStateResult{}, err
	}
	result.EffectiveRequirements = effective

	for _, decID := range in.DecisionIDs {
		key, err := engineering.NewRecordKey(engineering.RecordKindDecision, decID)
		if err != nil {
			return EngineeringStateResult{}, err
		}
		rec, found, err := repos.Records.Get(ctx, key)
		if err != nil {
			return EngineeringStateResult{}, err
		}
		if found {
			result.ApplicableDecisions = append(result.ApplicableDecisions, ApplicableDecision{DecisionID: decID, Decision: rec})
		}
	}

	if currentRevision.Found {
		readiness, err := ResolveReadiness(ctx, repos, currentRevision.Revision, effective)
		if err != nil {
			return EngineeringStateResult{}, err
		}
		result.Readiness = readiness
	} else {
		result.Readiness = ReadinessResult{Status: ReadinessIncomplete}
	}

	lifecycle, err := ResolveLifecycleState(ctx, repos, in.CapabilityArtifactID)
	if err != nil {
		return EngineeringStateResult{}, err
	}
	result.Lifecycle = lifecycle

	return result, nil
}
