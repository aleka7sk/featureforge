package application

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aleka7sk/featureforge/internal/domain"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

// CreateProjectCommand establishes a Project (FF-010 §3).
type CreateProjectCommand struct {
	ProjectID string
	Name      string
}

// CreateProjectResult names the Project created.
type CreateProjectResult struct {
	ProjectID domain.ProjectID
}

// Execute validates the command, writes the Project in one transaction, and
// returns its identity. Re-execution with identical content is a no-op.
func (c CreateProjectCommand) Execute(ctx context.Context, uow UnitOfWork, clock Clock) (CreateProjectResult, error) {
	if err := requireIdentity("project id", c.ProjectID); err != nil {
		return CreateProjectResult{}, err
	}
	if err := requireNonEmpty("name", c.Name); err != nil {
		return CreateProjectResult{}, err
	}
	id, err := domain.NewProjectID(c.ProjectID)
	if err != nil {
		return CreateProjectResult{}, invalidCommand(err)
	}
	createdAt := normalizeTime(clock.Now())
	err = uow.Do(ctx, func(r Repositories) error {
		if stored, found, err := r.Projects.Get(ctx, id); err != nil {
			return err
		} else if found {
			if stored.CreatedAt().IsZero() {
				return integrityError("stored project has no creation time", nil)
			}
			if stored.Name() != strings.TrimSpace(c.Name) {
				return fmt.Errorf("%w: project %s already exists with different immutable values", ErrImmutableValueConflict, id)
			}
			return nil
		}
		cards, err := r.FeatureCards.ListByProject(ctx, id)
		if err != nil {
			return err
		}
		if len(cards) != 0 {
			return integrityError("absent project identity has dangling FeatureCards", nil)
		}
		if createdAt.IsZero() {
			return fmt.Errorf("application: clock returned a zero project creation time")
		}
		project, err := domain.NewProject(id, c.Name, createdAt)
		if err != nil {
			return invalidCommand(err)
		}
		return r.Projects.Put(ctx, project)
	})
	if err != nil {
		return CreateProjectResult{}, err
	}
	return CreateProjectResult{ProjectID: id}, nil
}

// CreateFeatureCommand establishes a FeatureCard (FF-010 §3).
type CreateFeatureCommand struct {
	FeatureCardID string
	ProjectID     string
	Title         string
	Description   string
}

// CreateFeatureResult names the FeatureCard created.
type CreateFeatureResult struct {
	FeatureCardID domain.FeatureCardID
}

// Execute validates the command, reads the owning Project, writes the
// FeatureCard in one transaction, and returns its identity.
func (c CreateFeatureCommand) Execute(ctx context.Context, uow UnitOfWork, clock Clock) (CreateFeatureResult, error) {
	if err := requireIdentity("feature card id", c.FeatureCardID); err != nil {
		return CreateFeatureResult{}, err
	}
	if err := requireIdentity("project id", c.ProjectID); err != nil {
		return CreateFeatureResult{}, err
	}
	if err := requireNonEmpty("title", c.Title); err != nil {
		return CreateFeatureResult{}, err
	}
	fid, err := domain.NewFeatureCardID(c.FeatureCardID)
	if err != nil {
		return CreateFeatureResult{}, invalidCommand(err)
	}
	pid, err := domain.NewProjectID(c.ProjectID)
	if err != nil {
		return CreateFeatureResult{}, invalidCommand(err)
	}
	createdAt := normalizeTime(clock.Now())
	err = uow.Do(ctx, func(r Repositories) error {
		if stored, found, err := r.FeatureCards.Get(ctx, fid); err != nil {
			return err
		} else if found {
			if stored.CreatedAt().IsZero() {
				return integrityError("stored FeatureCard has no creation time", nil)
			}
			if owner, ownerFound, ownerErr := r.Projects.Get(ctx, stored.ProjectID()); ownerErr != nil {
				return ownerErr
			} else if !ownerFound {
				return integrityError("feature card has no owning project", nil)
			} else if owner.CreatedAt().IsZero() {
				return integrityError("feature card owning project has no creation time", nil)
			}
			if stored.ProjectID() != pid || stored.Title() != strings.TrimSpace(c.Title) || stored.Description() != strings.TrimSpace(c.Description) {
				return fmt.Errorf("%w: feature card %s already exists with different immutable values", ErrImmutableValueConflict, fid)
			}
			return nil
		}
		if owner, found, err := r.Projects.Get(ctx, pid); err != nil {
			return err
		} else if !found {
			return fmt.Errorf("%w: project %s", ErrNotFound, pid)
		} else if owner.CreatedAt().IsZero() {
			return integrityError("feature card owning project has no creation time", nil)
		}
		if createdAt.IsZero() {
			return fmt.Errorf("application: clock returned a zero FeatureCard creation time")
		}
		card, err := domain.NewFeatureCard(fid, pid, c.Title, c.Description, createdAt)
		if err != nil {
			return invalidCommand(err)
		}
		return r.FeatureCards.Put(ctx, card)
	})
	if err != nil {
		return CreateFeatureResult{}, err
	}
	return CreateFeatureResult{FeatureCardID: fid}, nil
}

// EstablishCapabilitySpecificationCommand records the capability Artifact
// and its founding Revision together (FF-010 §3.1): PEOS-002 permits an
// Artifact to precede its first Revision but says such an Artifact must not
// be treated as reproducible or validated, and core.Artifact retains no
// creation provenance of its own, so the two are one engineering act.
type EstablishCapabilitySpecificationCommand struct {
	FeatureCardID string
	ArtifactID    string
	RevisionID    string
	Content       engineering.CapabilitySpecificationContent
}

// EstablishCapabilitySpecificationResult names the keys created.
type EstablishCapabilitySpecificationResult struct {
	ArtifactKey engineering.ArtifactKey
	RevisionKey engineering.RevisionKey
	Sequence    int
}

// capabilityFeatureOwners walks the product-owned Project -> FeatureCard
// collections for every card linked to one engineering Artifact identity.
// ArtifactEnvelope intentionally carries no FeatureCardID projection.
func capabilityFeatureOwners(ctx context.Context, r Repositories, artifactID string) ([]domain.FeatureCard, error) {
	projects, err := r.Projects.List(ctx)
	if err != nil {
		return nil, err
	}
	seenProjects := make(map[domain.ProjectID]struct{}, len(projects))
	owners := make([]domain.FeatureCard, 0, 1)
	for _, project := range projects {
		if project.IsZero() || project.CreatedAt().IsZero() {
			return nil, integrityError("project listing contains an invalid project", nil)
		}
		if _, duplicate := seenProjects[project.ID()]; duplicate {
			return nil, integrityError("project listing contains duplicate identities", nil)
		}
		seenProjects[project.ID()] = struct{}{}
		cards, err := r.FeatureCards.ListByProject(ctx, project.ID())
		if err != nil {
			return nil, err
		}
		for _, card := range cards {
			if card.IsZero() || card.CreatedAt().IsZero() || card.ProjectID() != project.ID() {
				return nil, integrityError("project FeatureCard listing is contradictory", nil)
			}
			linkedArtifactID, linked := card.CapabilityArtifactID()
			if linked && linkedArtifactID == artifactID {
				owners = append(owners, card)
			}
		}
	}
	return owners, nil
}

// capabilityFeatureOwner recovers and verifies the unique product-owned root
// of a persisted capability.
func capabilityFeatureOwner(ctx context.Context, r Repositories, artifactID string) (domain.FeatureCard, error) {
	owners, err := capabilityFeatureOwners(ctx, r, artifactID)
	if err != nil {
		return domain.FeatureCard{}, err
	}
	if len(owners) != 1 {
		return domain.FeatureCard{}, integrityError("capability must have exactly one FeatureCard owner", nil)
	}
	owner := owners[0]
	if project, found, err := r.Projects.Get(ctx, owner.ProjectID()); err != nil {
		return domain.FeatureCard{}, err
	} else if !found {
		return domain.FeatureCard{}, integrityError("capability FeatureCard has no owning project", nil)
	} else if project.CreatedAt().IsZero() {
		return domain.FeatureCard{}, integrityError("capability FeatureCard owner project has no creation time", nil)
	}
	stored, found, err := r.FeatureCards.Get(ctx, owner.ID())
	if err != nil {
		return domain.FeatureCard{}, err
	}
	if !found || stored.CreatedAt().IsZero() || stored.ProjectID() != owner.ProjectID() {
		return domain.FeatureCard{}, integrityError("capability FeatureCard owner lookup is contradictory", nil)
	}
	linkedArtifactID, linked := stored.CapabilityArtifactID()
	if !linked || linkedArtifactID != artifactID {
		return domain.FeatureCard{}, integrityError("capability FeatureCard owner link is contradictory", nil)
	}
	return stored, nil
}

func validateAbsentCapabilityOwner(ctx context.Context, r Repositories, artifactID string) error {
	owners, err := capabilityFeatureOwners(ctx, r, artifactID)
	if err != nil {
		return err
	}
	if len(owners) != 0 {
		return integrityError("absent capability artifact already has a FeatureCard owner", nil)
	}
	return nil
}

func validateNoCapabilityFeatureOwners(ctx context.Context, r Repositories, artifactID, reason string) error {
	owners, err := capabilityFeatureOwners(ctx, r, artifactID)
	if err != nil {
		return err
	}
	if len(owners) != 0 {
		return integrityError(reason, nil)
	}
	return nil
}

// validateForeignCapabilityCommandOccupant proves that a non-capability pair
// is a complete act of its own family before C3/C4 classify the collision as
// immutable. A readable envelope pair alone is not enough: managed revisions
// need their order/member history, evidence needs one execution owner, and a
// transition revision needs one state-assignment owner.
func validateForeignCapabilityCommandOccupant(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, artifact engineering.ArtifactEnvelope, revision engineering.RevisionEnvelope, contentFound, orderFound bool) error {
	if artifact.ArtifactType != revision.ArtifactType {
		return integrityError("foreign artifact and revision families disagree", nil)
	}
	if contentFound {
		return integrityError("foreign revision occupancy has capability structured content", nil)
	}
	switch revision.RevisionFamily {
	case engineering.RevisionFamilyRequirement, engineering.RevisionFamilyValidationPlan:
		return validateManagedForeignOccupant(ctx, r, inspector, artifact, revision, orderFound)
	case engineering.RevisionFamilyEvidence:
		if orderFound {
			return integrityError("evidence revision unexpectedly has capability order metadata", nil)
		}
		occupancy, err := classifyEvidenceOccupancy(ctx, r, inspector, revision.Key, artifact, true, revision, true)
		if err != nil {
			return err
		}
		if occupancy != evidenceOccupancyComplete && occupancy != evidenceOccupancyForeign {
			return integrityError("evidence revision is not a complete governed act", nil)
		}
		return nil
	case engineering.RevisionFamilyTransitionRecord:
		if orderFound {
			return integrityError("transition revision unexpectedly has capability order metadata", nil)
		}
		return validateForeignArtifactOccupancy(ctx, r, inspector, artifact)
	default:
		return integrityError("foreign revision occupancy is not a complete governed act", nil)
	}
}

func validateForeignCapabilityCommandArtifact(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, artifact engineering.ArtifactEnvelope) error {
	revisions, err := r.Revisions.ListByArtifact(ctx, artifact.Key.ArtifactID)
	if err != nil {
		return err
	}
	if len(revisions) == 0 {
		return integrityError("foreign artifact has no revision history", nil)
	}
	for _, revision := range revisions {
		if err := inspectRevision(inspector, revision); err != nil {
			return err
		}
		if artifact.ArtifactType != revision.ArtifactType {
			return integrityError("artifact and revision families disagree", nil)
		}
		content, contentFound, err := r.StructuredContent.Get(ctx, revision.Key)
		if err != nil {
			return err
		}
		_, orderFound, err := r.RevisionOrder.Get(ctx, revision.Key)
		if err != nil {
			return err
		}
		if revision.RevisionFamily == engineering.RevisionFamilyCapability {
			if !contentFound || !orderFound {
				return integrityError("capability revision occupancy is partial", nil)
			}
			if err := inspector.ValidateCapabilityContent(revision, content); err != nil {
				return integrityError("capability structured content disagrees with its revision", err)
			}
			if _, err := validateManagedHistory(ctx, r, inspector, revision.Key.ArtifactID, engineering.RevisionFamilyCapability, false); err != nil {
				return err
			}
			if _, err := capabilityFeatureOwner(ctx, r, revision.Key.ArtifactID); err != nil {
				return err
			}
			continue
		}
		if err := validateForeignCapabilityCommandOccupant(ctx, r, inspector, artifact, revision, contentFound, orderFound); err != nil {
			return err
		}
	}
	return nil
}

// Execute validates the command, confirms the FeatureCard exists, links it
// to the new capability, and writes the Artifact, founding Revision,
// structured content, and sequence-1 order metadata in one transaction.
func (c EstablishCapabilitySpecificationCommand) Execute(ctx context.Context, uow UnitOfWork, recorder EngineeringRecorder, inspector EngineeringReplayInspector, clock Clock) (EstablishCapabilitySpecificationResult, error) {
	if err := requireNonEmpty("feature card id", c.FeatureCardID); err != nil {
		return EstablishCapabilitySpecificationResult{}, err
	}
	if err := requireIdentity("artifact id", c.ArtifactID); err != nil {
		return EstablishCapabilitySpecificationResult{}, err
	}
	if err := requireIdentity("revision id", c.RevisionID); err != nil {
		return EstablishCapabilitySpecificationResult{}, err
	}
	if c.Content.IsZero() {
		return EstablishCapabilitySpecificationResult{}, fmt.Errorf("%w: content must not be zero", ErrInvalidCommand)
	}
	fid, err := domain.NewFeatureCardID(c.FeatureCardID)
	if err != nil {
		return EstablishCapabilitySpecificationResult{}, invalidCommand(err)
	}
	digest, err := c.Content.Digest()
	if err != nil {
		return EstablishCapabilitySpecificationResult{}, invalidCommand(err)
	}
	key, err := engineering.NewRevisionKey(c.ArtifactID, c.RevisionID)
	if err != nil {
		return EstablishCapabilitySpecificationResult{}, invalidCommand(err)
	}
	now := normalizeTime(clock.Now())

	var result EstablishCapabilitySpecificationResult
	err = uow.Do(ctx, func(r Repositories) error {
		artifactEnv, artifactFound, err := r.Artifacts.Get(ctx, engineering.ArtifactKey{ArtifactID: c.ArtifactID})
		if err != nil {
			return err
		}
		revisionEnv, revisionFound, err := r.Revisions.Get(ctx, key)
		if err != nil {
			return err
		}
		storedContent, contentFound, err := r.StructuredContent.Get(ctx, key)
		if err != nil {
			return err
		}
		order, orderFound, err := r.RevisionOrder.Get(ctx, key)
		if err != nil {
			return err
		}

		// Classify requested-pair occupancy before consulting the requested
		// FeatureCard dependency. Persisted corruption or an immutable
		// occupancy conflict must not be hidden by a missing request reference.
		if revisionFound || contentFound || orderFound {
			if !revisionFound {
				return integrityError("capability founding pair is locally partial", nil)
			}
			if err := inspectRevision(inspector, revisionEnv); err != nil {
				return err
			}
			if !artifactFound {
				return integrityError("occupied capability founding pair has no owning artifact", nil)
			}
			if err := inspectArtifact(inspector, artifactEnv); err != nil {
				return err
			}

			if revisionEnv.RevisionFamily != engineering.RevisionFamilyCapability {
				if err := validateForeignCapabilityCommandOccupant(ctx, r, inspector, artifactEnv, revisionEnv, contentFound, orderFound); err != nil {
					return err
				}
				return immutableConflict("capability founding pair belongs to another revision family")
			}

			if !contentFound || !orderFound {
				return integrityError("capability founding pair is locally partial", nil)
			}
			if err := inspector.ValidateCapabilityContent(revisionEnv, storedContent); err != nil {
				return integrityError("capability founding content disagrees with its revision", err)
			}
			if artifactEnv.ArtifactType != revisionEnv.ArtifactType || order.Key != key || order.Sequence < 1 || order.RecordedAt.IsZero() {
				return integrityError("capability founding act is contradictory", nil)
			}
			if !canonicalTimeEqual(revisionEnv.RecordedAt, order.RecordedAt) {
				return integrityError("capability revision and order metadata disagree on recorded time", nil)
			}
			expectedArtifact, buildErr := recorder.RecordCapabilityArtifact(c.ArtifactID, artifactEnv.RecordedAt)
			if buildErr != nil {
				return invalidCommand(buildErr)
			}
			if _, err := validateManagedHistory(ctx, r, inspector, c.ArtifactID, engineering.RevisionFamilyCapability, false); err != nil {
				return err
			}
			owner, err := capabilityFeatureOwner(ctx, r, c.ArtifactID)
			if err != nil {
				return err
			}
			if !artifactEnv.Equal(expectedArtifact) {
				return immutableConflict("capability artifact is occupied by different immutable semantics")
			}
			if order.Sequence != 1 {
				return immutableConflict("capability founding pair is occupied by a later capability revision")
			}
			if !canonicalTimeEqual(artifactEnv.RecordedAt, revisionEnv.RecordedAt) {
				return integrityError("capability founding act does not share one recorded moment", nil)
			}
			expectedRevision, buildErr := recorder.RecordCapabilityRevision(engineering.CapabilityRevisionInput{
				ArtifactID: c.ArtifactID, RevisionID: c.RevisionID, ContentDigest: digest, RecordedAt: revisionEnv.RecordedAt,
			})
			if buildErr != nil {
				return invalidCommand(buildErr)
			}
			if owner.ID() != fid || !revisionEnv.Equal(expectedRevision) || !storedContent.Equal(c.Content) {
				return immutableConflict("capability founding identity is occupied by different semantics")
			}
			result = EstablishCapabilitySpecificationResult{ArtifactKey: artifactEnv.Key, RevisionKey: revisionEnv.Key, Sequence: order.Sequence}
			return nil
		}

		if artifactFound {
			if err := inspectArtifact(inspector, artifactEnv); err != nil {
				return err
			}
			expectedArtifact, buildErr := recorder.RecordCapabilityArtifact(c.ArtifactID, artifactEnv.RecordedAt)
			if buildErr != nil {
				return invalidCommand(buildErr)
			}
			if !artifactEnv.Equal(expectedArtifact) {
				if err := validateForeignCapabilityCommandArtifact(ctx, r, inspector, artifactEnv); err != nil {
					return err
				}
				return immutableConflict("artifact identity belongs to another family")
			}
			if _, err := validateManagedHistory(ctx, r, inspector, c.ArtifactID, engineering.RevisionFamilyCapability, false); err != nil {
				return err
			}
			if _, err := capabilityFeatureOwner(ctx, r, c.ArtifactID); err != nil {
				return err
			}
			return immutableConflict("capability artifact already has a different founding revision")
		}
		if err := validateAbsentArtifactHistory(ctx, r, inspector, c.ArtifactID); err != nil {
			return err
		}

		card, found, err := r.FeatureCards.Get(ctx, fid)
		if err != nil {
			return err
		} else if !found {
			return fmt.Errorf("%w: feature card %s", ErrNotFound, fid)
		}
		if card.CreatedAt().IsZero() {
			return integrityError("feature card has no creation time", nil)
		}
		if project, found, err := r.Projects.Get(ctx, card.ProjectID()); err != nil {
			return err
		} else if !found {
			return integrityError("feature card has no owning project", nil)
		} else if project.CreatedAt().IsZero() {
			return integrityError("feature card owning project has no creation time", nil)
		}
		if linked, hasLink := card.CapabilityArtifactID(); hasLink {
			if linked == c.ArtifactID {
				return integrityError("feature card link names an absent capability aggregate", nil)
			}
			if _, err := validateManagedHistory(ctx, r, inspector, linked, engineering.RevisionFamilyCapability, false); err != nil {
				return err
			}
			owner, err := capabilityFeatureOwner(ctx, r, linked)
			if err != nil {
				return err
			}
			if owner.ID() != fid {
				return integrityError("feature card link and capability owner disagree", nil)
			}
			return fmt.Errorf("%w: feature card %s is already linked", ErrCapabilityAlreadyLinked, fid)
		}
		if err := requireServerTime(now, "a new capability-establishment act"); err != nil {
			return err
		}
		artifactEnv, err = recorder.RecordCapabilityArtifact(c.ArtifactID, now)
		if err != nil {
			return invalidCommand(err)
		}
		revisionEnv, err = recorder.RecordCapabilityRevision(engineering.CapabilityRevisionInput{
			ArtifactID: c.ArtifactID, RevisionID: c.RevisionID, ContentDigest: digest, RecordedAt: now,
		})
		if err != nil {
			return invalidCommand(err)
		}
		if err := r.Artifacts.Put(ctx, artifactEnv); err != nil {
			return err
		}
		if err := r.Revisions.Put(ctx, revisionEnv); err != nil {
			return err
		}
		if err := r.StructuredContent.Put(ctx, revisionEnv.Key, c.Content); err != nil {
			return err
		}
		order, err = engineering.NewRevisionOrderMetadata(revisionEnv.Key, 1, now)
		if err != nil {
			return invalidCommand(err)
		}
		if err := r.RevisionOrder.Put(ctx, order); err != nil {
			return err
		}
		if err := r.FeatureCards.LinkCapability(ctx, fid, c.ArtifactID); err != nil {
			return err
		}

		result = EstablishCapabilitySpecificationResult{ArtifactKey: artifactEnv.Key, RevisionKey: revisionEnv.Key, Sequence: 1}
		return nil
	})
	if err != nil {
		return EstablishCapabilitySpecificationResult{}, err
	}
	return result, nil
}

// ReviseCapabilitySpecificationCommand records a new capability revision at
// the next sequence (FF-010 §3.2).
type ReviseCapabilitySpecificationCommand struct {
	ArtifactID string
	RevisionID string
	Content    engineering.CapabilitySpecificationContent
}

// ReviseCapabilitySpecificationResult names the keys created.
type ReviseCapabilitySpecificationResult struct {
	RevisionKey engineering.RevisionKey
	Sequence    int
}

// Execute reads the artifact's existing order metadata, computes
// max(sequence)+1, and writes the new revision, content, and order metadata
// in one transaction. Two concurrent executions produce n and n+1, never
// two of n (the UnitOfWork's lock covers the whole read-then-write).
func (c ReviseCapabilitySpecificationCommand) Execute(ctx context.Context, uow UnitOfWork, recorder EngineeringRecorder, inspector EngineeringReplayInspector, clock Clock) (ReviseCapabilitySpecificationResult, error) {
	if err := requireIdentity("artifact id", c.ArtifactID); err != nil {
		return ReviseCapabilitySpecificationResult{}, err
	}
	if err := requireIdentity("revision id", c.RevisionID); err != nil {
		return ReviseCapabilitySpecificationResult{}, err
	}
	if c.Content.IsZero() {
		return ReviseCapabilitySpecificationResult{}, fmt.Errorf("%w: content must not be zero", ErrInvalidCommand)
	}
	digest, err := c.Content.Digest()
	if err != nil {
		return ReviseCapabilitySpecificationResult{}, invalidCommand(err)
	}
	key, err := engineering.NewRevisionKey(c.ArtifactID, c.RevisionID)
	if err != nil {
		return ReviseCapabilitySpecificationResult{}, invalidCommand(err)
	}
	now := normalizeTime(clock.Now())

	var result ReviseCapabilitySpecificationResult
	err = uow.Do(ctx, func(r Repositories) error {
		storedRevision, revisionFound, err := r.Revisions.Get(ctx, key)
		if err != nil {
			return err
		}
		storedContent, contentFound, err := r.StructuredContent.Get(ctx, key)
		if err != nil {
			return err
		}
		storedOrder, orderFound, err := r.RevisionOrder.Get(ctx, key)
		if err != nil {
			return err
		}

		// The requested revision pair is authoritative for precedence. An
		// occupied local pair is classified before the owning Artifact so a
		// dangling/partial act is an integrity failure, while a wholly absent
		// pair preserves the governed missing-Artifact outcome.
		if revisionFound || contentFound || orderFound {
			if !revisionFound {
				return integrityError("capability revision pair is locally partial", nil)
			}
			if err := inspectRevision(inspector, storedRevision); err != nil {
				return err
			}

			artifact, artifactFound, err := r.Artifacts.Get(ctx, engineering.ArtifactKey{ArtifactID: c.ArtifactID})
			if err != nil {
				return err
			}
			if !artifactFound {
				return integrityError("occupied capability revision pair has no owning artifact", nil)
			}
			if err := inspectArtifact(inspector, artifact); err != nil {
				return err
			}

			if storedRevision.RevisionFamily != engineering.RevisionFamilyCapability {
				if err := validateForeignCapabilityCommandOccupant(ctx, r, inspector, artifact, storedRevision, contentFound, orderFound); err != nil {
					return err
				}
				return immutableConflict("revision identity belongs to another revision family")
			}
			if !contentFound || !orderFound {
				return integrityError("capability revision pair is locally partial", nil)
			}
			if err := inspector.ValidateCapabilityContent(storedRevision, storedContent); err != nil {
				return integrityError("capability revision content disagrees with its revision", err)
			}
			if artifact.ArtifactType != storedRevision.ArtifactType || storedOrder.Key != key || storedOrder.Sequence < 1 || storedOrder.RecordedAt.IsZero() {
				return integrityError("capability revision act is contradictory", nil)
			}
			if !canonicalTimeEqual(storedRevision.RecordedAt, storedOrder.RecordedAt) {
				return integrityError("capability revision and order metadata disagree on recorded time", nil)
			}
			expectedArtifact, buildErr := recorder.RecordCapabilityArtifact(c.ArtifactID, artifact.RecordedAt)
			if buildErr != nil {
				return invalidCommand(buildErr)
			}
			if _, err := validateManagedHistory(ctx, r, inspector, c.ArtifactID, engineering.RevisionFamilyCapability, false); err != nil {
				return err
			}
			if !artifact.Equal(expectedArtifact) {
				return immutableConflict("capability artifact is occupied by different immutable semantics")
			}
			expected, buildErr := recorder.RecordCapabilityRevision(engineering.CapabilityRevisionInput{
				ArtifactID: c.ArtifactID, RevisionID: c.RevisionID, ContentDigest: digest, RecordedAt: storedRevision.RecordedAt,
			})
			if buildErr != nil {
				return invalidCommand(buildErr)
			}
			if !storedRevision.Equal(expected) || !storedContent.Equal(c.Content) {
				return immutableConflict("capability revision identity is occupied by different semantics")
			}
			result = ReviseCapabilitySpecificationResult{RevisionKey: storedRevision.Key, Sequence: storedOrder.Sequence}
			return nil
		}

		artifact, found, err := r.Artifacts.Get(ctx, engineering.ArtifactKey{ArtifactID: c.ArtifactID})
		if err != nil {
			return err
		} else if !found {
			if err := validateAbsentArtifactHistory(ctx, r, inspector, c.ArtifactID); err != nil {
				return err
			}
			return fmt.Errorf("%w: capability artifact %s", ErrNotFound, c.ArtifactID)
		}
		if err := inspectArtifact(inspector, artifact); err != nil {
			return err
		}
		expectedArtifact, buildErr := recorder.RecordCapabilityArtifact(c.ArtifactID, artifact.RecordedAt)
		if buildErr != nil {
			return invalidCommand(buildErr)
		}
		if !artifact.Equal(expectedArtifact) {
			if err := validateForeignCapabilityCommandArtifact(ctx, r, inspector, artifact); err != nil {
				return err
			}
			return immutableConflict("artifact identity belongs to another family")
		}
		historySize, err := validateManagedHistory(ctx, r, inspector, c.ArtifactID, engineering.RevisionFamilyCapability, false)
		if err != nil {
			return err
		}
		next := historySize + 1
		if err := requireServerTime(now, "a new capability-revision act"); err != nil {
			return err
		}

		revisionEnv, err := recorder.RecordCapabilityRevision(engineering.CapabilityRevisionInput{
			ArtifactID: c.ArtifactID, RevisionID: c.RevisionID, ContentDigest: digest, RecordedAt: now,
		})
		if err != nil {
			return invalidCommand(err)
		}
		if err := r.Revisions.Put(ctx, revisionEnv); err != nil {
			return err
		}
		if err := r.StructuredContent.Put(ctx, revisionEnv.Key, c.Content); err != nil {
			return err
		}
		order, err := engineering.NewRevisionOrderMetadata(revisionEnv.Key, next, now)
		if err != nil {
			return invalidCommand(err)
		}
		if err := r.RevisionOrder.Put(ctx, order); err != nil {
			return err
		}

		result = ReviseCapabilitySpecificationResult{RevisionKey: revisionEnv.Key, Sequence: next}
		return nil
	})
	if err != nil {
		return ReviseCapabilitySpecificationResult{}, err
	}
	return result, nil
}

// AcceptCapabilityRevisionCommand appends one acceptance journal entry
// (FF-010 §3.3). It records no PEOS value -- acceptance is product-owned.
type AcceptCapabilityRevisionCommand struct {
	RecordID    string
	ArtifactID  string
	RevisionID  string
	State       engineering.AcceptanceState
	Reason      string
	EffectiveAt time.Time
	// HasEffectiveAt distinguishes an omitted defaultable time from an
	// explicitly supplied value.  A non-zero EffectiveAt remains treated as
	// present for direct Go callers written before this flag existed.
	HasEffectiveAt bool
}

// AcceptCapabilityRevisionResult names the journal entry created.
type AcceptCapabilityRevisionResult struct {
	RecordID string
}

// Execute reads the acceptance journal for the revision, validates the
// transition against the journal head, and appends one
// RevisionAcceptanceRecord.
func (c AcceptCapabilityRevisionCommand) Execute(ctx context.Context, uow UnitOfWork, inspector EngineeringReplayInspector, clock Clock) (AcceptCapabilityRevisionResult, error) {
	if err := requireIdentity("record id", c.RecordID); err != nil {
		return AcceptCapabilityRevisionResult{}, err
	}
	if err := requireIdentity("artifact id", c.ArtifactID); err != nil {
		return AcceptCapabilityRevisionResult{}, err
	}
	if err := requireIdentity("revision id", c.RevisionID); err != nil {
		return AcceptCapabilityRevisionResult{}, err
	}
	key, err := engineering.NewRevisionKey(c.ArtifactID, c.RevisionID)
	if err != nil {
		return AcceptCapabilityRevisionResult{}, invalidCommand(err)
	}
	if !c.State.IsValid() {
		return AcceptCapabilityRevisionResult{}, fmt.Errorf("%w: unsupported acceptance state %q", ErrInvalidCommand, c.State)
	}
	hasEffectiveAt := c.HasEffectiveAt || !zeroTime(c.EffectiveAt)
	if c.HasEffectiveAt && zeroTime(c.EffectiveAt) {
		return AcceptCapabilityRevisionResult{}, &fieldError{field: "effective at", reason: "must be a valid timestamp when present"}
	}
	candidateTime := normalizeTime(clock.Now())
	effectiveAt := optionalTime(c.EffectiveAt, candidateTime)

	err = uow.Do(ctx, func(r Repositories) error {
		if stored, found, err := r.RevisionAcceptance.GetByRecordID(ctx, c.RecordID); err != nil {
			return integrityError("acceptance identity lookup", err)
		} else if found {
			if err := validateAcceptanceCandidate(ctx, r, inspector, stored); err != nil {
				return err
			}
			if stored.Actor != "featureforge:local-user" {
				return integrityError("acceptance record has an unexpected server actor", nil)
			}
			if stored.Key != key || stored.State != c.State || stored.Reason != c.Reason {
				return fmt.Errorf("%w: acceptance record %s already names another immutable act", ErrImmutableValueConflict, c.RecordID)
			}
			if hasEffectiveAt && !canonicalTimeEqual(stored.EffectiveAt, c.EffectiveAt) {
				return fmt.Errorf("%w: acceptance record %s has a different effective time", ErrImmutableValueConflict, c.RecordID)
			}
			return nil
		}
		revision, found, err := r.Revisions.Get(ctx, key)
		if err != nil {
			return err
		} else if !found {
			return fmt.Errorf("%w: revision %s", ErrNotFound, key)
		}
		if err := inspectRevision(inspector, revision); err != nil {
			return err
		}
		requireMember := false
		switch revision.RevisionFamily {
		case engineering.RevisionFamilyCapability:
		case engineering.RevisionFamilyRequirement, engineering.RevisionFamilyValidationPlan:
			requireMember = true
		default:
			return integrityError("acceptance command targets an unmanaged revision family", nil)
		}
		if _, err := validateManagedHistory(ctx, r, inspector, key.ArtifactID, revision.RevisionFamily, requireMember); err != nil {
			return err
		}
		journal, err := r.RevisionAcceptance.ListByRevision(ctx, key)
		if err != nil {
			return err
		}
		if err := ValidateAcceptanceTransition(journal, key, c.State); err != nil {
			return err
		}
		if effectiveAt.IsZero() {
			return fmt.Errorf("application: clock returned a zero acceptance effective time")
		}
		record, err := engineering.NewRevisionAcceptanceRecord(c.RecordID, key, c.State, effectiveAt, "featureforge:local-user", c.Reason)
		if err != nil {
			return invalidCommand(err)
		}
		if err := validateAcceptanceAppendTransition(journal, record); err != nil {
			return err
		}
		return r.RevisionAcceptance.Append(ctx, record)
	})
	if err != nil {
		return AcceptCapabilityRevisionResult{}, err
	}
	return AcceptCapabilityRevisionResult{RecordID: c.RecordID}, nil
}

// validateAcceptanceAppendTransition validates the journal as it will be
// observed after insertion, in its governed (EffectiveAt, RecordID) order.
// Checking only the current head would allow a backdated entry to create a
// history that the next read correctly rejects as contradictory.
func validateAcceptanceAppendTransition(journal []engineering.RevisionAcceptanceRecord, candidate engineering.RevisionAcceptanceRecord) error {
	combined := append(append([]engineering.RevisionAcceptanceRecord(nil), journal...), candidate)
	sort.Slice(combined, func(i, j int) bool {
		if combined[i].EffectiveAt.Equal(combined[j].EffectiveAt) {
			return combined[i].RecordID < combined[j].RecordID
		}
		return combined[i].EffectiveAt.Before(combined[j].EffectiveAt)
	})
	var from engineering.AcceptanceState
	for _, record := range combined {
		if !engineering.ValidTransition(from, record.State) {
			if from == "" {
				from = engineering.AcceptanceStateDraft
			}
			return fmt.Errorf("%w: %s -> %s for revision %s at record %s", ErrAcceptanceTransitionInvalid, from, record.State, candidate.Key, record.RecordID)
		}
		from = record.State
	}
	return nil
}
