package application

import (
	"context"
	"time"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

// EstablishRequirementCommand records a Requirement Artifact and its
// founding revision together, for the same reason capability establishment
// does (FF-010 §3, "no CRUD-oriented service decomposition": one command
// covers both a requirement's first appearance and any later revision,
// since both record a requirement.Revision the identical way).
type EstablishRequirementCommand struct {
	ArtifactID        string
	RevisionID        string
	Statement         string
	SubjectArtifactID string
}

// EstablishRequirementResult names the keys created.
type EstablishRequirementResult struct {
	ArtifactKey engineering.ArtifactKey
	RevisionKey engineering.RevisionKey
}

// Execute validates the command and writes the Requirement Artifact and
// Revision in one transaction.
func (c EstablishRequirementCommand) Execute(ctx context.Context, uow UnitOfWork, recorder EngineeringRecorder, clock Clock) (EstablishRequirementResult, error) {
	if err := requireNonEmpty("artifact id", c.ArtifactID); err != nil {
		return EstablishRequirementResult{}, err
	}
	if err := requireNonEmpty("revision id", c.RevisionID); err != nil {
		return EstablishRequirementResult{}, err
	}
	if err := requireNonEmpty("statement", c.Statement); err != nil {
		return EstablishRequirementResult{}, err
	}
	if err := requireNonEmpty("subject artifact id", c.SubjectArtifactID); err != nil {
		return EstablishRequirementResult{}, err
	}
	now := clock.Now()

	var result EstablishRequirementResult
	err := uow.Do(ctx, func(r Repositories) error {
		artifactEnv, revisionEnv, err := recorder.RecordRequirement(engineering.RequirementInput{
			ArtifactID: c.ArtifactID, RevisionID: c.RevisionID, Statement: c.Statement,
			SubjectArtifactID: c.SubjectArtifactID, RecordedAt: now,
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

		// Requirement revisions follow the same current-revision resolution
		// policy as capability revisions (FF-004 §3.2: "the same ordering
		// contract"), which requires both order metadata and an accepted
		// journal entry for every stored revision (FF-004 §2 rules 5-6).
		// FF-010 §3's command table lists EstablishRequirement's engineering
		// act as only "Requirement artifact + revision", omitting both --
		// a gap against FF-004 §3.2's own requirement, not a deliberate
		// narrowing. This command closes it the same way
		// EstablishCapabilitySpecification closes the equivalent gap for a
		// capability's founding revision: write sequence-1-or-next order
		// metadata and an immediate "accepted" journal entry, since no
		// separate accept-requirement command exists and this scenario
		// never revises or withdraws a requirement.
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
		order, err := engineering.NewRevisionOrderMetadata(revisionEnv.Key, next, now)
		if err != nil {
			return err
		}
		if err := r.RevisionOrder.Put(ctx, order); err != nil {
			return err
		}
		acceptance, err := engineering.NewRevisionAcceptanceRecord(
			"ACC-"+c.ArtifactID+"-"+c.RevisionID, revisionEnv.Key, engineering.AcceptanceStateAccepted, now,
			"featureforge:local-user", "requirement established",
		)
		if err != nil {
			return err
		}
		if err := r.RevisionAcceptance.Append(ctx, acceptance); err != nil {
			return err
		}

		result = EstablishRequirementResult{ArtifactKey: artifactEnv.Key, RevisionKey: revisionEnv.Key}
		return nil
	})
	if err != nil {
		return EstablishRequirementResult{}, err
	}
	return result, nil
}

// requireTimeOrClock returns t if set, else clock.Now().
func requireTimeOrClock(t time.Time, clock Clock) time.Time {
	if zeroTime(t) {
		return clock.Now()
	}
	return t
}
