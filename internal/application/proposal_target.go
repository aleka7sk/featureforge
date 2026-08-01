package application

import (
	"context"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

// proposalTargetOccupancy is the fully inspected, P-local view used by the
// reviewed-proposal command before it reads any mutable proposal dependency.
// A false occupied value is a positive absence witness across every member of
// FF-024's target set, not merely an absent RevisionEnvelope.
type proposalTargetOccupancy struct {
	occupied bool
	foreign  bool
	revision engineering.RevisionEnvelope
	content  engineering.CapabilitySpecificationContent
	order    engineering.RevisionOrderMetadata

	proposalDigest engineering.Digest
	contextDigest  engineering.Digest
	sources        []string
	aiAssisted     bool
}

// inspectProposalTarget implements FF-024's occupied-target precedence. The
// requested Revision, capability content, order metadata, acceptance journal,
// and Requirement trace are all queried before absence is concluded. Any
// locally partial target is therefore corruption even when its Revision row is
// missing.
func inspectProposalTarget(
	ctx context.Context,
	repos Repositories,
	inspector ProposalReplayInspector,
	key engineering.RevisionKey,
) (proposalTargetOccupancy, error) {
	revision, revisionFound, err := repos.Revisions.Get(ctx, key)
	if err != nil {
		return proposalTargetOccupancy{}, err
	}
	content, contentFound, err := repos.StructuredContent.Get(ctx, key)
	if err != nil {
		return proposalTargetOccupancy{}, err
	}
	order, orderFound, err := repos.RevisionOrder.Get(ctx, key)
	if err != nil {
		return proposalTargetOccupancy{}, err
	}
	journal, err := repos.RevisionAcceptance.ListByRevision(ctx, key)
	if err != nil {
		return proposalTargetOccupancy{}, err
	}
	trace, traceFound, err := repos.RequirementTraces.Get(ctx, key)
	if err != nil {
		return proposalTargetOccupancy{}, err
	}
	if contentFound && content.IsZero() || !contentFound && !content.IsZero() {
		return proposalTargetOccupancy{}, integrityError("proposal target content lookup has contradictory presence", nil)
	}
	if orderFound && order.Key.IsZero() || !orderFound && !order.Key.IsZero() {
		return proposalTargetOccupancy{}, integrityError("proposal target order lookup has contradictory presence", nil)
	}
	if traceFound && trace.IsZero() || !traceFound && !trace.IsZero() {
		return proposalTargetOccupancy{}, integrityError("proposal target trace lookup has contradictory presence", nil)
	}

	occupied := revisionFound || !revision.Key.IsZero() || contentFound || !content.IsZero() ||
		orderFound || !order.Key.IsZero() || len(journal) != 0 || traceFound || !trace.IsZero()
	if !occupied {
		return proposalTargetOccupancy{}, nil
	}
	if !revisionFound {
		return proposalTargetOccupancy{}, integrityError("proposal target is locally partial", nil)
	}
	if revision.Key != key {
		return proposalTargetOccupancy{}, integrityError("proposal target Revision lookup returned another identity", nil)
	}
	if orderFound && order.Key != key {
		return proposalTargetOccupancy{}, integrityError("proposal target order lookup returned another identity", nil)
	}
	if traceFound && trace.RequirementRevision != key {
		return proposalTargetOccupancy{}, integrityError("proposal target trace lookup returned another identity", nil)
	}
	if err := inspectRevision(inspector, revision); err != nil {
		return proposalTargetOccupancy{}, err
	}

	artifact, artifactFound, err := repos.Artifacts.Get(ctx, engineering.ArtifactKey{ArtifactID: key.ArtifactID})
	if err != nil {
		return proposalTargetOccupancy{}, err
	}
	if !artifactFound && !artifact.Key.IsZero() {
		return proposalTargetOccupancy{}, integrityError("proposal target Artifact lookup has contradictory presence", nil)
	}
	if !artifactFound {
		return proposalTargetOccupancy{}, integrityError("occupied proposal target has no owning artifact", nil)
	}
	if artifact.Key.ArtifactID != key.ArtifactID {
		return proposalTargetOccupancy{}, integrityError("proposal target Artifact lookup returned another identity", nil)
	}
	if err := inspectArtifact(inspector, artifact); err != nil {
		return proposalTargetOccupancy{}, err
	}

	if revision.RevisionFamily != engineering.RevisionFamilyCapability {
		if err := validateForeignCapabilityCommandOccupant(ctx, repos, inspector, artifact, revision, contentFound, orderFound); err != nil {
			return proposalTargetOccupancy{}, err
		}
		return proposalTargetOccupancy{occupied: true, foreign: true, revision: revision}, nil
	}

	if !contentFound || !orderFound {
		return proposalTargetOccupancy{}, integrityError("AI-assisted capability target is locally partial", nil)
	}
	if traceFound || !trace.IsZero() {
		return proposalTargetOccupancy{}, integrityError("capability target has an unexpected requirement criterion trace", nil)
	}
	if err := inspector.ValidateCapabilityContent(revision, content); err != nil {
		return proposalTargetOccupancy{}, integrityError("proposal target content disagrees with its revision", err)
	}
	if order.Key != key || order.Sequence < 1 || order.RecordedAt.IsZero() ||
		!canonicalTimeEqual(order.RecordedAt, revision.RecordedAt) {
		return proposalTargetOccupancy{}, integrityError("proposal target order metadata is contradictory", nil)
	}
	if _, err := validateManagedHistory(ctx, repos, inspector, key.ArtifactID, engineering.RevisionFamilyCapability, false); err != nil {
		return proposalTargetOccupancy{}, err
	}
	if err := validatePersistedProposalHistorySources(ctx, repos, inspector, key.ArtifactID); err != nil {
		return proposalTargetOccupancy{}, err
	}

	proposalDigest, contextDigest, sources, found, err := inspector.InspectAIAssistedCapabilityRevision(revision)
	if err != nil {
		return proposalTargetOccupancy{}, integrityError("proposal target AI-assistance witness is unreadable", err)
	}
	return proposalTargetOccupancy{
		occupied:       true,
		revision:       revision,
		content:        content,
		order:          order,
		proposalDigest: proposalDigest,
		contextDigest:  contextDigest,
		sources:        append([]string(nil), sources...),
		aiAssisted:     found,
	}, nil
}
