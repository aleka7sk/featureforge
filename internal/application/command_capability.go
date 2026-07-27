package application

import (
	"context"
	"fmt"
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
	if err := requireNonEmpty("project id", c.ProjectID); err != nil {
		return CreateProjectResult{}, err
	}
	if err := requireNonEmpty("name", c.Name); err != nil {
		return CreateProjectResult{}, err
	}
	id, err := domain.NewProjectID(c.ProjectID)
	if err != nil {
		return CreateProjectResult{}, err
	}
	project, err := domain.NewProject(id, c.Name, clock.Now())
	if err != nil {
		return CreateProjectResult{}, err
	}
	err = uow.Do(ctx, func(r Repositories) error {
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
	if err := requireNonEmpty("feature card id", c.FeatureCardID); err != nil {
		return CreateFeatureResult{}, err
	}
	if err := requireNonEmpty("title", c.Title); err != nil {
		return CreateFeatureResult{}, err
	}
	fid, err := domain.NewFeatureCardID(c.FeatureCardID)
	if err != nil {
		return CreateFeatureResult{}, err
	}
	pid, err := domain.NewProjectID(c.ProjectID)
	if err != nil {
		return CreateFeatureResult{}, err
	}
	card, err := domain.NewFeatureCard(fid, pid, c.Title, c.Description, clock.Now())
	if err != nil {
		return CreateFeatureResult{}, err
	}
	err = uow.Do(ctx, func(r Repositories) error {
		if _, found, err := r.Projects.Get(ctx, pid); err != nil {
			return err
		} else if !found {
			return fmt.Errorf("%w: project %s", ErrNotFound, pid)
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

// Execute validates the command, confirms the FeatureCard exists, links it
// to the new capability, and writes the Artifact, founding Revision,
// structured content, and sequence-1 order metadata in one transaction.
func (c EstablishCapabilitySpecificationCommand) Execute(ctx context.Context, uow UnitOfWork, recorder EngineeringRecorder, clock Clock) (EstablishCapabilitySpecificationResult, error) {
	if err := requireNonEmpty("feature card id", c.FeatureCardID); err != nil {
		return EstablishCapabilitySpecificationResult{}, err
	}
	if err := requireNonEmpty("artifact id", c.ArtifactID); err != nil {
		return EstablishCapabilitySpecificationResult{}, err
	}
	if err := requireNonEmpty("revision id", c.RevisionID); err != nil {
		return EstablishCapabilitySpecificationResult{}, err
	}
	if c.Content.IsZero() {
		return EstablishCapabilitySpecificationResult{}, fmt.Errorf("%w: content must not be zero", ErrInvalidCommand)
	}
	fid, err := domain.NewFeatureCardID(c.FeatureCardID)
	if err != nil {
		return EstablishCapabilitySpecificationResult{}, err
	}
	digest, err := c.Content.Digest()
	if err != nil {
		return EstablishCapabilitySpecificationResult{}, err
	}
	now := clock.Now()

	var result EstablishCapabilitySpecificationResult
	err = uow.Do(ctx, func(r Repositories) error {
		if _, found, err := r.FeatureCards.Get(ctx, fid); err != nil {
			return err
		} else if !found {
			return fmt.Errorf("%w: feature card %s", ErrNotFound, fid)
		}

		artifactEnv, err := recorder.RecordCapabilityArtifact(c.ArtifactID, now)
		if err != nil {
			return err
		}
		if err := r.Artifacts.Put(ctx, artifactEnv); err != nil {
			return err
		}

		revisionEnv, err := recorder.RecordCapabilityRevision(engineering.CapabilityRevisionInput{
			ArtifactID: c.ArtifactID, RevisionID: c.RevisionID, ContentDigest: digest, RecordedAt: now,
		})
		if err != nil {
			return err
		}
		if err := r.Revisions.Put(ctx, revisionEnv); err != nil {
			return err
		}
		if err := r.StructuredContent.Put(ctx, revisionEnv.Key, c.Content); err != nil {
			return err
		}
		order, err := engineering.NewRevisionOrderMetadata(revisionEnv.Key, 1, now)
		if err != nil {
			return err
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
func (c ReviseCapabilitySpecificationCommand) Execute(ctx context.Context, uow UnitOfWork, recorder EngineeringRecorder, clock Clock) (ReviseCapabilitySpecificationResult, error) {
	if err := requireNonEmpty("artifact id", c.ArtifactID); err != nil {
		return ReviseCapabilitySpecificationResult{}, err
	}
	if err := requireNonEmpty("revision id", c.RevisionID); err != nil {
		return ReviseCapabilitySpecificationResult{}, err
	}
	if c.Content.IsZero() {
		return ReviseCapabilitySpecificationResult{}, fmt.Errorf("%w: content must not be zero", ErrInvalidCommand)
	}
	digest, err := c.Content.Digest()
	if err != nil {
		return ReviseCapabilitySpecificationResult{}, err
	}
	now := clock.Now()

	var result ReviseCapabilitySpecificationResult
	err = uow.Do(ctx, func(r Repositories) error {
		if _, found, err := r.Artifacts.Get(ctx, engineering.ArtifactKey{ArtifactID: c.ArtifactID}); err != nil {
			return err
		} else if !found {
			return fmt.Errorf("%w: capability artifact %s", ErrNotFound, c.ArtifactID)
		}
		existing, err := r.RevisionOrder.ListByArtifact(ctx, c.ArtifactID)
		if err != nil {
			return err
		}
		next := 1
		for _, o := range existing {
			if o.Sequence >= next {
				next = o.Sequence + 1
			}
		}

		revisionEnv, err := recorder.RecordCapabilityRevision(engineering.CapabilityRevisionInput{
			ArtifactID: c.ArtifactID, RevisionID: c.RevisionID, ContentDigest: digest, RecordedAt: now,
		})
		if err != nil {
			return err
		}
		if err := r.Revisions.Put(ctx, revisionEnv); err != nil {
			return err
		}
		if err := r.StructuredContent.Put(ctx, revisionEnv.Key, c.Content); err != nil {
			return err
		}
		order, err := engineering.NewRevisionOrderMetadata(revisionEnv.Key, next, now)
		if err != nil {
			return err
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
}

// AcceptCapabilityRevisionResult names the journal entry created.
type AcceptCapabilityRevisionResult struct {
	RecordID string
}

// Execute reads the acceptance journal for the revision, validates the
// transition against the journal head, and appends one
// RevisionAcceptanceRecord.
func (c AcceptCapabilityRevisionCommand) Execute(ctx context.Context, uow UnitOfWork, clock Clock) (AcceptCapabilityRevisionResult, error) {
	if err := requireNonEmpty("record id", c.RecordID); err != nil {
		return AcceptCapabilityRevisionResult{}, err
	}
	key, err := engineering.NewRevisionKey(c.ArtifactID, c.RevisionID)
	if err != nil {
		return AcceptCapabilityRevisionResult{}, err
	}
	if !c.State.IsValid() {
		return AcceptCapabilityRevisionResult{}, fmt.Errorf("%w: unsupported acceptance state %q", ErrInvalidCommand, c.State)
	}
	effectiveAt := c.EffectiveAt
	if zeroTime(effectiveAt) {
		effectiveAt = clock.Now()
	}

	err = uow.Do(ctx, func(r Repositories) error {
		if _, found, err := r.Revisions.Get(ctx, key); err != nil {
			return err
		} else if !found {
			return fmt.Errorf("%w: revision %s", ErrNotFound, key)
		}
		journal, err := r.RevisionAcceptance.ListByRevision(ctx, key)
		if err != nil {
			return err
		}
		if err := ValidateAcceptanceTransition(journal, key, c.State); err != nil {
			return err
		}
		record, err := engineering.NewRevisionAcceptanceRecord(c.RecordID, key, c.State, effectiveAt, "featureforge:local-user", c.Reason)
		if err != nil {
			return err
		}
		return r.RevisionAcceptance.Append(ctx, record)
	})
	if err != nil {
		return AcceptCapabilityRevisionResult{}, err
	}
	return AcceptCapabilityRevisionResult{RecordID: c.RecordID}, nil
}
