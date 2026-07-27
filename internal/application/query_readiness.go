package application

import (
	"context"
	"fmt"
	"slices"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

// ReadinessStatus is FeatureForge's product-owned readiness outcome
// (FF-010 §7). It is never a PEOS value.
type ReadinessStatus string

const (
	ReadinessReady         ReadinessStatus = "ready"
	ReadinessNotReady      ReadinessStatus = "not-ready"
	ReadinessIndeterminate ReadinessStatus = "indeterminate"
	ReadinessIncomplete    ReadinessStatus = "incomplete"
)

// EffectiveRequirement is one requirement's current revision, as resolved
// by the same ordering algorithm capability revisions use (FF-004 §3.2:
// requirement revisions follow the same policy).
type EffectiveRequirement struct {
	ArtifactID  string
	RevisionKey engineering.RevisionKey
	Sequence    int
}

// ResolveEffectiveRequirements resolves the current revision of every
// requirement in requirementArtifactIDs. Any requirement failing
// resolution fails the whole query, naming it (FF-004 §3.2) -- a partial
// requirement set would silently understate what must be satisfied.
func ResolveEffectiveRequirements(ctx context.Context, repos Repositories, requirementArtifactIDs []string) ([]EffectiveRequirement, error) {
	out := make([]EffectiveRequirement, 0, len(requirementArtifactIDs))
	for _, artifactID := range requirementArtifactIDs {
		result, err := ResolveCurrentRevision(ctx, repos, artifactID)
		if err != nil {
			return nil, fmt.Errorf("resolving requirement %s: %w", artifactID, err)
		}
		if !result.Found {
			return nil, fmt.Errorf("%w: requirement %s has no accepted revision", ErrEngineeringStateIndeterminate, artifactID)
		}
		out = append(out, EffectiveRequirement{ArtifactID: artifactID, RevisionKey: result.Revision.Key, Sequence: result.Sequence})
	}
	return out, nil
}

// PerRequirementReadiness is one requirement's contribution to a
// ReadinessResult.
type PerRequirementReadiness struct {
	RequirementArtifactID  string
	RequirementRevisionKey engineering.RevisionKey
	HasClaim               bool
	Claim                  engineering.RecordEnvelope
	Outcome                string
	ExecutionOutcome       string
	Stale                  bool
	StaleSequence          int
	Rejected               []RejectedClaim
	VerdictReason          string
}

// ReadinessResult is the outcome of a release-readiness resolution
// (FF-010 §7).
type ReadinessResult struct {
	Status         ReadinessStatus
	PerRequirement []PerRequirementReadiness
}

// ResolveReadiness implements the FF-010 §7 readiness algorithm and
// precedence (not-ready > indeterminate > incomplete > ready, AD-016).
// currentRevision is the capability's current revision, already resolved
// by the caller; requirements is the effective-requirements set, already
// resolved by the caller. A structural failure (ambiguous revision, dangling
// reference) is the caller's concern via the errors those resolutions
// already returned -- this function only computes the semantic verdict.
func ResolveReadiness(ctx context.Context, repos Repositories, currentRevision engineering.RevisionEnvelope, requirements []EffectiveRequirement) (ReadinessResult, error) {
	if len(requirements) == 0 {
		return ReadinessResult{Status: ReadinessIncomplete}, nil
	}

	result := ReadinessResult{PerRequirement: make([]PerRequirementReadiness, 0, len(requirements))}
	sawNotReady := false
	sawIndeterminate := false
	sawIncomplete := false

	subjectKey := engineering.ArtifactRevisionSubjectKey(currentRevision.Key.ArtifactID, currentRevision.Key.RevisionID)
	scope := "featureforge:capability|" + currentRevision.Key.ArtifactID

	for _, req := range requirements {
		criterionKey, err := engineering.RequirementCriterionKey(req.RevisionKey)
		if err != nil {
			return ReadinessResult{}, err
		}
		claimResult, err := ResolveCurrentClaim(ctx, repos, subjectKey, scope, []string{criterionKey})
		if err != nil {
			return ReadinessResult{}, err
		}

		per := PerRequirementReadiness{RequirementArtifactID: req.ArtifactID, RequirementRevisionKey: req.RevisionKey}
		if !claimResult.Found {
			// ResolveCurrentClaim was scoped to the CURRENT revision's
			// subject, so a claim recorded against an earlier revision
			// (structurally excluded from that search) is otherwise
			// indistinguishable from "no claim at all". Search separately,
			// across every subject, so a stale claim is reported as stale
			// rather than silently read as missing (FF-010 §7).
			staleSeq, staleFound, err := findStaleClaimSequence(ctx, repos, criterionKey, currentRevision.Key.ArtifactID)
			if err != nil {
				return ReadinessResult{}, err
			}
			if staleFound {
				per.Stale = true
				per.StaleSequence = staleSeq
				per.VerdictReason = fmt.Sprintf("stale: evaluated against sequence %d, not the current revision", staleSeq)
			} else {
				per.VerdictReason = "no applicable claim"
			}
			sawIncomplete = true
			result.PerRequirement = append(result.PerRequirement, per)
			continue
		}
		per.HasClaim = true
		per.Claim = claimResult.Claim
		per.Outcome = claimResult.Claim.Outcome
		per.Rejected = claimResult.Rationale.Rejected

		execOutcome, execErr := supportingExecutionOutcome(ctx, repos, claimResult.Claim)
		if execErr != nil {
			return ReadinessResult{}, execErr
		}
		per.ExecutionOutcome = execOutcome

		switch {
		case claimResult.Claim.Outcome == "peos:not-satisfied":
			per.VerdictReason = "not satisfied"
			sawNotReady = true
		case claimResult.Claim.Outcome == "peos:inconclusive":
			per.VerdictReason = "inconclusive"
			sawIndeterminate = true
		case execOutcome == "peos:interrupted" || execOutcome == "peos:indeterminate":
			per.VerdictReason = fmt.Sprintf("supporting execution outcome is %s", execOutcome)
			sawIndeterminate = true
		case claimResult.Claim.Outcome == "peos:satisfied":
			per.VerdictReason = "satisfied"
		default:
			per.VerdictReason = fmt.Sprintf("unrecognized claim outcome %q", claimResult.Claim.Outcome)
			sawIndeterminate = true
		}
		result.PerRequirement = append(result.PerRequirement, per)
	}

	switch {
	case sawNotReady:
		result.Status = ReadinessNotReady
	case sawIndeterminate:
		result.Status = ReadinessIndeterminate
	case sawIncomplete:
		result.Status = ReadinessIncomplete
	default:
		result.Status = ReadinessReady
	}
	return result, nil
}

// supportingExecutionOutcome resolves the Outcome projected on the first
// execution record cited by claim, or "" if none is cited.
func supportingExecutionOutcome(ctx context.Context, repos Repositories, claim engineering.RecordEnvelope) (string, error) {
	if len(claim.ExecutionKeys) == 0 {
		return "", nil
	}
	executions, err := repos.Records.ListByKind(ctx, engineering.RecordKindExecution)
	if err != nil {
		return "", err
	}
	wanted := claim.ExecutionKeys[0]
	for _, e := range executions {
		if engineering.ExecutionKey(e.Key.ID) == wanted {
			return e.Outcome, nil
		}
	}
	return "", nil
}

// findStaleClaimSequence searches every claim (regardless of subject) for
// one citing criterionKey, and if found, resolves the sequence of the
// capability revision it was evaluated against -- distinguishing "a claim
// exists but is stale" from "no claim was ever recorded" (FF-010 §7).
func findStaleClaimSequence(ctx context.Context, repos Repositories, criterionKey, capabilityArtifactID string) (int, bool, error) {
	all, err := repos.Records.ListByKind(ctx, engineering.RecordKindClaim)
	if err != nil {
		return 0, false, err
	}
	for _, c := range all {
		if !containsString(c.CriterionKeys, criterionKey) {
			continue
		}
		if c.Key.ID == "" {
			continue
		}
		revisionID, ok := revisionIDFromSubjectKey(c.SubjectKey, capabilityArtifactID)
		if !ok {
			continue
		}
		order, found, err := repos.RevisionOrder.Get(ctx, mustRevisionKeyOf(capabilityArtifactID, revisionID))
		if err != nil {
			return 0, false, err
		}
		if !found {
			continue
		}
		return order.Sequence, true, nil
	}
	return 0, false, nil
}

func containsString(haystack []string, needle string) bool {
	return slices.Contains(haystack, needle)
}

// revisionIDFromSubjectKey extracts the revision ID from an
// ArtifactRevisionSubjectKey for capabilityArtifactID, or false if
// subjectKey does not name a revision of that artifact.
func revisionIDFromSubjectKey(subjectKey, capabilityArtifactID string) (string, bool) {
	prefix := "artifact-revision:" + capabilityArtifactID + "/"
	if len(subjectKey) <= len(prefix) || subjectKey[:len(prefix)] != prefix {
		return "", false
	}
	return subjectKey[len(prefix):], true
}

func mustRevisionKeyOf(artifactID, revisionID string) engineering.RevisionKey {
	key, _ := engineering.NewRevisionKey(artifactID, revisionID)
	return key
}
