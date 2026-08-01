package application

import (
	"context"
	"fmt"
	"slices"

	"github.com/aleka7sk/featureforge/internal/engineering"
	"github.com/aleka7sk/featureforge/internal/proposal"
)

// GenerateCapabilityProposalResult contains the complete transient values
// returned for explicit human review. Neither value is persisted by generate.
type GenerateCapabilityProposalResult struct {
	ContextPack proposal.ContextPack
	Proposal    proposal.Proposal
}

// GenerateCapabilityProposal assembles one complete context in a read-only
// transaction and invokes the authority-free generator only after that
// transaction has closed.
func GenerateCapabilityProposal(
	ctx context.Context,
	uow UnitOfWork,
	projector EngineeringProjector,
	inspector ProposalReplayInspector,
	generator proposal.Generator,
	artifactID string,
) (GenerateCapabilityProposalResult, error) {
	pack, err := AssembleProposalContext(ctx, uow, projector, inspector, artifactID)
	if err != nil {
		return GenerateCapabilityProposalResult{}, err
	}
	if generator == nil {
		return GenerateCapabilityProposalResult{}, integrityError("proposal generator is unavailable", nil)
	}
	generated, err := generator.Generate(pack)
	if err != nil {
		return GenerateCapabilityProposalResult{}, err
	}
	if err := generated.ValidateAgainst(pack); err != nil {
		return GenerateCapabilityProposalResult{}, fmt.Errorf("application: proposal generator returned an invalid proposal: %w", err)
	}
	return GenerateCapabilityProposalResult{ContextPack: pack, Proposal: generated}, nil
}

// AcceptCapabilityProposalInput is the reviewed, round-tripped proposal plus
// the route Artifact identity and caller-controlled new Revision identity.
type AcceptCapabilityProposalInput struct {
	ArtifactID string
	RevisionID string
	Proposal   proposal.Proposal
}

// AcceptCapabilityProposalResult names the ordinary draft capability Revision
// created by review acceptance, or recovered by exact occupied replay.
type AcceptCapabilityProposalResult struct {
	ArtifactKey engineering.ArtifactKey
	RevisionKey engineering.RevisionKey
	Sequence    int
}

// AcceptCapabilityProposal applies FF-024's occupied-target precedence and,
// only for a genuinely absent target, reassembles freshness and writes the
// exact AI-assisted draft act in one UnitOfWork. It never calls a Generator.
func AcceptCapabilityProposal(
	ctx context.Context,
	uow UnitOfWork,
	recorder ProposalEngineeringRecorder,
	projector EngineeringProjector,
	inspector ProposalReplayInspector,
	clock Clock,
	in AcceptCapabilityProposalInput,
) (AcceptCapabilityProposalResult, error) {
	if err := requireIdentity("artifact id", in.ArtifactID); err != nil {
		return AcceptCapabilityProposalResult{}, err
	}
	if err := requireIdentity("revision id", in.RevisionID); err != nil {
		return AcceptCapabilityProposalResult{}, err
	}
	if err := in.Proposal.Validate(); err != nil {
		return AcceptCapabilityProposalResult{}, invalidCommand(err)
	}
	key, err := engineering.NewRevisionKey(in.ArtifactID, in.RevisionID)
	if err != nil {
		return AcceptCapabilityProposalResult{}, invalidCommand(err)
	}
	now := normalizeTime(clock.Now())
	proposalSources := proposalSourceStrings(in.Proposal.Sources())

	var result AcceptCapabilityProposalResult
	err = uow.Do(ctx, func(repos Repositories) error {
		memo := newProposalInspectionMemo(inspector)
		target, err := inspectProposalTarget(ctx, repos, memo, key)
		if err != nil {
			return err
		}
		if target.occupied {
			if target.foreign {
				return immutableConflict("proposal target belongs to another revision family")
			}
			if !target.aiAssisted {
				return immutableConflict("proposal target is an ordinary capability revision")
			}
			if !target.content.Equal(in.Proposal.Content()) ||
				!target.proposalDigest.Equal(in.Proposal.ProposalDigest()) ||
				!target.contextDigest.Equal(in.Proposal.ContextDigest()) ||
				!slices.Equal(target.sources, proposalSources) {
				return immutableConflict("AI-assisted proposal target is occupied by different immutable semantics")
			}
			result = AcceptCapabilityProposalResult{
				ArtifactKey: engineering.ArtifactKey{ArtifactID: target.revision.Key.ArtifactID},
				RevisionKey: target.revision.Key,
				Sequence:    target.order.Sequence,
			}
			return nil
		}

		artifact, found, err := repos.Artifacts.Get(ctx, engineering.ArtifactKey{ArtifactID: in.ArtifactID})
		if err != nil {
			return err
		}
		if !found && !artifact.Key.IsZero() {
			return integrityError("proposal Artifact lookup has contradictory presence", nil)
		}
		if !found {
			if err := validateAbsentArtifactHistory(ctx, repos, inspector, in.ArtifactID); err != nil {
				return err
			}
			return fmt.Errorf("%w: capability artifact %s", ErrNotFound, in.ArtifactID)
		}
		if artifact.Key.ArtifactID != in.ArtifactID {
			return integrityError("proposal Artifact lookup returned another identity", nil)
		}
		if err := inspectArtifact(memo, artifact); err != nil {
			return err
		}
		family, err := memo.ArtifactFamily(artifact)
		if err != nil {
			return integrityError("proposal Artifact family is unreadable", err)
		}
		if family != engineering.RevisionFamilyCapability {
			if err := validateForeignArtifactOccupancy(ctx, repos, memo, artifact); err != nil {
				return err
			}
			return immutableConflict("proposal Artifact identity belongs to another family")
		}

		assembly, err := collectProposalAssembly(ctx, repos, projector, memo, in.ArtifactID)
		if err != nil {
			return err
		}
		fresh, err := proposalContextPackFromAssembly(assembly)
		if err != nil {
			return err
		}
		if !in.Proposal.ContextDigest().Equal(fresh.ContextDigest()) {
			return fmt.Errorf("%w: capability %s", ErrProposalContextStale, in.ArtifactID)
		}
		if err := in.Proposal.ValidateAgainst(fresh); err != nil {
			return invalidCommand(err)
		}
		if err := requireServerTime(now, "a new AI-assisted capability-revision act"); err != nil {
			return err
		}
		contentDigest, err := in.Proposal.Content().Digest()
		if err != nil {
			return invalidCommand(err)
		}
		revision, err := recorder.RecordAIAssistedCapabilityRevision(
			engineering.CapabilityRevisionInput{
				ArtifactID:    in.ArtifactID,
				RevisionID:    in.RevisionID,
				ContentDigest: contentDigest,
				RecordedAt:    now,
			},
			in.Proposal.ProposalDigest(),
			in.Proposal.ContextDigest(),
			proposalSources,
		)
		if err != nil {
			return invalidCommand(err)
		}
		if err := repos.Revisions.Put(ctx, revision); err != nil {
			return err
		}
		if err := repos.StructuredContent.Put(ctx, revision.Key, in.Proposal.Content()); err != nil {
			return err
		}
		next := assembly.historySize + 1
		order, err := engineering.NewRevisionOrderMetadata(revision.Key, next, now)
		if err != nil {
			return invalidCommand(err)
		}
		if err := repos.RevisionOrder.Put(ctx, order); err != nil {
			return err
		}
		result = AcceptCapabilityProposalResult{
			ArtifactKey: engineering.ArtifactKey{ArtifactID: in.ArtifactID},
			RevisionKey: revision.Key,
			Sequence:    next,
		}
		return nil
	})
	if err != nil {
		return AcceptCapabilityProposalResult{}, err
	}
	return result, nil
}

func proposalSourceStrings(sources []proposal.SourceReference) []string {
	result := make([]string, len(sources))
	for index, source := range sources {
		result[index] = source.String()
	}
	return result
}
