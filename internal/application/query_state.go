package application

import (
	"context"
	"fmt"
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
//
// PlanArtifactID is the applicable validation plan (FF-018 §6.6's
// exactly-one contract, ResolveApplicableValidationPlanID), empty when
// none exists yet. It was already discovered by every caller before
// FF-020 and discarded; FF-020 renders it as ValidationPlanResult rather
// than re-deriving it (FF-020 §5).
type EngineeringStateInput struct {
	CapabilityArtifactID   string
	RequirementArtifactIDs []string
	DecisionIDs            []string
	PlanArtifactID         string
}

// discoverArtifactIDsBySubject collects the distinct artifact IDs of every
// revision of family whose projected subject is capabilityArtifactID,
// ascending -- never map iteration order (FF-009 §5). It is the shared
// mechanism behind DiscoverRequirementArtifactIDs and
// DiscoverValidationPlanArtifactIDs (AD-025, FF-016 §9). Completeness comes
// from ListByFamilyAndSubject (FF-016 §4); this function adds only artifact
// ID deduplication and ordering.
func discoverArtifactIDsBySubject(ctx context.Context, repos Repositories, inspector EngineeringReplayInspector, family engineering.RevisionFamily, capabilityArtifactID string) ([]string, error) {
	subjectKey := engineering.ArtifactSubjectKey(capabilityArtifactID)
	revisions, err := repos.Revisions.ListByFamilyAndSubject(ctx, family, subjectKey)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(revisions))
	for _, rev := range revisions {
		if rev.RevisionFamily != family || rev.SubjectKey != subjectKey {
			return nil, integrityError("subject discovery returned a revision outside its requested projection", nil)
		}
		if seen[rev.Key.ArtifactID] {
			continue
		}
		if err := validateStableSubjectProjection(ctx, repos, rev.Key.ArtifactID, family, subjectKey); err != nil {
			return nil, err
		}
		if _, err := validateManagedHistory(ctx, repos, inspector, rev.Key.ArtifactID, family, true); err != nil {
			return nil, err
		}
		seen[rev.Key.ArtifactID] = true
		out = append(out, rev.Key.ArtifactID)
	}
	sort.Strings(out)
	return out, nil
}

// validateStableSubjectProjection prevents a single Requirement or
// Validation Plan Artifact from being discovered for one capability through
// an older revision while its current revision has silently retargeted the
// shared Artifact to another capability. Command paths additionally decode
// and inspect every envelope; read discovery can enforce this invariant from
// the complete projected history it is already required to enumerate.
func validateStableSubjectProjection(ctx context.Context, repos Repositories, artifactID string, family engineering.RevisionFamily, subjectKey string) error {
	revisions, err := repos.Revisions.ListByArtifact(ctx, artifactID)
	if err != nil {
		return err
	}
	if len(revisions) == 0 {
		return integrityError("subject discovery resolved an artifact with no revisions", nil)
	}
	seen := make(map[engineering.RevisionKey]struct{}, len(revisions))
	for _, revision := range revisions {
		if revision.Key.ArtifactID != artifactID || revision.RevisionFamily != family || revision.SubjectKey != subjectKey {
			return integrityError("managed artifact history disagrees with its stable discovered subject", nil)
		}
		if _, duplicate := seen[revision.Key]; duplicate {
			return integrityError("managed artifact history contains a duplicate revision", nil)
		}
		seen[revision.Key] = struct{}{}
	}
	return nil
}

// DiscoverRequirementArtifactIDs finds every requirement whose projected
// subject is capabilityArtifactID, returning their artifact IDs. This is
// what clears the M.5 blocker the contract investigation identified: a
// caller holding only a capability artifact ID, with no prior knowledge of
// which requirements govern it, can obtain the complete population
// (AD-025, FF-016 §9) -- including a requirement with no plan activity and
// no claim, which a claim-derived or plan-derived population would silently
// omit (FF-011 REQ-4, the counterexample AD-025 records).
func DiscoverRequirementArtifactIDs(ctx context.Context, repos Repositories, inspector EngineeringReplayInspector, capabilityArtifactID string) ([]string, error) {
	return discoverArtifactIDsBySubject(ctx, repos, inspector, engineering.RevisionFamilyRequirement, capabilityArtifactID)
}

// DiscoverDecisionIDs finds every decision naming any revision of
// capabilityArtifactID as its subject, returning their decision IDs
// (FF-018 §6.3). A decision's subject is a capability *revision*
// (RecordEnvelope.SubjectKey uses ArtifactRevisionSubjectKey), so every
// revision of the artifact is consulted, not only the current one -- a
// decision recorded against an earlier revision remains part of the
// feature's history after a later revision exists. Deduplicated and sorted
// ascending, matching discoverArtifactIDsBySubject's determinism guarantee.
func DiscoverDecisionIDs(ctx context.Context, repos Repositories, capabilityArtifactID string) ([]string, error) {
	revisions, err := repos.Revisions.ListByArtifact(ctx, capabilityArtifactID)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(revisions))
	for _, rev := range revisions {
		subjectKey := engineering.ArtifactRevisionSubjectKey(rev.Key.ArtifactID, rev.Key.RevisionID)
		decisions, err := repos.Records.ListByKindAndSubject(ctx, engineering.RecordKindDecision, subjectKey)
		if err != nil {
			return nil, err
		}
		for _, dec := range decisions {
			if seen[dec.Key.ID] {
				continue
			}
			seen[dec.Key.ID] = true
			out = append(out, dec.Key.ID)
		}
	}
	sort.Strings(out)
	return out, nil
}

// ApplicableDecision is one decision found to name the capability as a
// subject, with the rationale for how it was matched (FF-004 §3.3).
// Detail is the decision's full basis, decoded from Decision.Payload
// (FF-020 §5, FF-001 §3.5: "the basis is displayed, not collapsed").
type ApplicableDecision struct {
	DecisionID string
	Decision   engineering.RecordEnvelope
	Detail     engineering.DecisionDetail
}

// ValidationPlanResult names the applicable validation plan and its
// activities (FF-020 §5, FF-001 §3.6: "plan revision and its activities").
// Found is false, with no error, when the capability has no applicable
// plan yet -- the same well-formed-empty convention every other
// EngineeringStateResult field already uses. Validation-plan revisions use
// the same governed order and acceptance resolution as capability and
// requirement revisions.
type ValidationPlanResult struct {
	Found      bool
	ArtifactID string
	RevisionID string
	Activities []engineering.PlanActivityDetail
	Rationale  ResolutionRationale
}

// EngineeringStateResult bundles every current-state answer for one
// feature, each with its own rationale (FF-010 §10).
type EngineeringStateResult struct {
	CurrentRevision       CurrentRevisionResult
	EffectiveRequirements []EffectiveRequirement
	ApplicableDecisions   []ApplicableDecision
	ValidationPlan        ValidationPlanResult
	Readiness             ReadinessResult
	Lifecycle             LifecycleStateResult
}

// GetFeatureEngineeringState composes every current-state query into one
// result (FF-010 §10). It is read-only and deterministic. projector decodes
// the display content FF-020 adds -- requirement statements, decision
// bases, and plan activities -- from the payloads repos already returns;
// a stored payload that will not decode is ErrStoredPayloadUnreadable
// (FF-020 §7), never a silently empty field.
func GetFeatureEngineeringState(ctx context.Context, repos Repositories, projector EngineeringProjector, inspector EngineeringReplayInspector, in EngineeringStateInput) (EngineeringStateResult, error) {
	var result EngineeringStateResult
	if in.CapabilityArtifactID != "" {
		subjectKey := engineering.ArtifactSubjectKey(in.CapabilityArtifactID)
		for _, artifactID := range in.RequirementArtifactIDs {
			if err := validateStableSubjectProjection(ctx, repos, artifactID, engineering.RevisionFamilyRequirement, subjectKey); err != nil {
				return EngineeringStateResult{}, err
			}
			if _, err := validateManagedHistory(ctx, repos, inspector, artifactID, engineering.RevisionFamilyRequirement, true); err != nil {
				return EngineeringStateResult{}, err
			}
		}
		if in.PlanArtifactID != "" {
			if err := validateStableSubjectProjection(ctx, repos, in.PlanArtifactID, engineering.RevisionFamilyValidationPlan, subjectKey); err != nil {
				return EngineeringStateResult{}, err
			}
			if _, err := validateManagedHistory(ctx, repos, inspector, in.PlanArtifactID, engineering.RevisionFamilyValidationPlan, true); err != nil {
				return EngineeringStateResult{}, err
			}
		}
	}

	currentRevision, err := ResolveCurrentRevision(ctx, repos, in.CapabilityArtifactID)
	if err != nil {
		return EngineeringStateResult{}, err
	}
	result.CurrentRevision = currentRevision

	effective, err := ResolveEffectiveRequirements(ctx, repos, in.RequirementArtifactIDs)
	if err != nil {
		return EngineeringStateResult{}, err
	}
	for i, req := range effective {
		env, found, err := repos.Revisions.Get(ctx, req.RevisionKey)
		if err != nil {
			return EngineeringStateResult{}, err
		}
		if !found {
			continue
		}
		statement, err := projector.ProjectRequirementStatement(env.Payload)
		if err != nil {
			return EngineeringStateResult{}, fmt.Errorf("%w: requirement %s: %w", ErrStoredPayloadUnreadable, req.ArtifactID, err)
		}
		effective[i].Statement = statement
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
			detail, err := projector.ProjectDecisionDetail(rec.Payload)
			if err != nil {
				return EngineeringStateResult{}, fmt.Errorf("%w: decision %s: %w", ErrStoredPayloadUnreadable, decID, err)
			}
			result.ApplicableDecisions = append(result.ApplicableDecisions, ApplicableDecision{DecisionID: decID, Decision: rec, Detail: detail})
		}
	}

	if in.PlanArtifactID != "" {
		currentPlan, err := ResolveCurrentRevision(ctx, repos, in.PlanArtifactID)
		if err != nil {
			return EngineeringStateResult{}, err
		}
		result.ValidationPlan.Rationale = currentPlan.Rationale
		if currentPlan.Found {
			rev := currentPlan.Revision
			activities, err := projector.ProjectPlanActivities(rev.Payload)
			if err != nil {
				return EngineeringStateResult{}, fmt.Errorf("%w: validation plan %s: %w", ErrStoredPayloadUnreadable, in.PlanArtifactID, err)
			}
			result.ValidationPlan = ValidationPlanResult{
				Found: true, ArtifactID: rev.Key.ArtifactID, RevisionID: rev.Key.RevisionID, Activities: activities,
				Rationale: currentPlan.Rationale,
			}
		}
	}

	if currentRevision.Found {
		readiness, err := ResolveReadiness(ctx, repos, currentRevision.Revision, effective)
		if err != nil {
			return EngineeringStateResult{}, err
		}
		if err := decorateReadinessReasoning(ctx, repos, readiness, projector); err != nil {
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

// decorateReadinessReasoning fills in the free-text reasoning
// GetFeatureEngineeringState adds to a readiness result -- the current
// claim's own reasoning, and each superseded or invalidated claim's, all
// decoded from their stored payloads (FF-020 §5, FF-001 §3.6: "claims
// with outcomes, criteria, reasoning, and correction links" and
// "superseded claims shown inline ... not hidden"). ResolveReadiness
// itself has no projector, so this is a second pass over its result
// rather than something ResolveReadiness does inline; readiness.PerRequirement
// is a slice, so mutating its elements by index here is visible to the
// caller without a pointer receiver.
func decorateReadinessReasoning(ctx context.Context, repos Repositories, readiness ReadinessResult, projector EngineeringProjector) error {
	for i := range readiness.PerRequirement {
		p := &readiness.PerRequirement[i]
		if p.HasClaim {
			reasoning, err := projector.ProjectClaimReasoning(p.Claim.Payload)
			if err != nil {
				return fmt.Errorf("%w: claim %s: %w", ErrStoredPayloadUnreadable, p.Claim.Key, err)
			}
			p.Reasoning = reasoning
		}
		for j := range p.Rejected {
			rej := &p.Rejected[j]
			env, found, err := repos.Records.Get(ctx, rej.Key)
			if err != nil {
				return err
			}
			if !found {
				continue
			}
			reasoning, err := projector.ProjectClaimReasoning(env.Payload)
			if err != nil {
				return fmt.Errorf("%w: claim %s: %w", ErrStoredPayloadUnreadable, rej.Key, err)
			}
			rej.Reasoning = reasoning
		}
	}
	return nil
}
