package application

import (
	"context"
	"sort"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

// EngineeringStateInput names the records GetFeatureEngineeringState draws
// from. RequirementArtifactIDs and DecisionIDs are caller-supplied, not
// derived by this query. A caller that does not already know the
// requirement population can now obtain it via
// DiscoverRequirementArtifactIDs (AD-025, FF-016 §5) rather than being
// unable to find it at all, which is what this comment described before
// FF-016 projected a subject onto RevisionEnvelope.
type EngineeringStateInput struct {
	CapabilityArtifactID   string
	RequirementArtifactIDs []string
	DecisionIDs            []string
}

// discoverArtifactIDsBySubject collects the distinct artifact IDs of every
// revision of family whose projected subject is capabilityArtifactID,
// ascending -- never map iteration order (FF-009 §5). It is the shared
// mechanism behind DiscoverRequirementArtifactIDs and
// DiscoverValidationPlanArtifactIDs (AD-025, FF-016 §9). Completeness comes
// from ListByFamilyAndSubject (FF-016 §4); this function adds only artifact
// ID deduplication and ordering.
func discoverArtifactIDsBySubject(ctx context.Context, repos Repositories, family engineering.RevisionFamily, capabilityArtifactID string) ([]string, error) {
	revisions, err := repos.Revisions.ListByFamilyAndSubject(ctx, family, engineering.ArtifactSubjectKey(capabilityArtifactID))
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(revisions))
	for _, rev := range revisions {
		if seen[rev.Key.ArtifactID] {
			continue
		}
		seen[rev.Key.ArtifactID] = true
		out = append(out, rev.Key.ArtifactID)
	}
	sort.Strings(out)
	return out, nil
}

// DiscoverRequirementArtifactIDs finds every requirement whose projected
// subject is capabilityArtifactID, returning their artifact IDs. This is
// what clears the M.5 blocker the contract investigation identified: a
// caller holding only a capability artifact ID, with no prior knowledge of
// which requirements govern it, can obtain the complete population
// (AD-025, FF-016 §9) -- including a requirement with no plan activity and
// no claim, which a claim-derived or plan-derived population would silently
// omit (FF-011 REQ-4, the counterexample AD-025 records).
func DiscoverRequirementArtifactIDs(ctx context.Context, repos Repositories, capabilityArtifactID string) ([]string, error) {
	return discoverArtifactIDsBySubject(ctx, repos, engineering.RevisionFamilyRequirement, capabilityArtifactID)
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
