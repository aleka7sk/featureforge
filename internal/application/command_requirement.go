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
