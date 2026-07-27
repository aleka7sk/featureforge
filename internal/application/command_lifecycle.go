package application

import (
	"context"
	"fmt"
	"time"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

// AssignLifecycleStateCommand records a lifecycle transition (FF-010 §3):
// either the entry assignment (IsEntry, established by a content-free
// Transition Record Revision, AD-014) or a full transition naming the
// State Assignment it departs from.
type AssignLifecycleStateCommand struct {
	AssignmentID               string
	SubjectArtifactID          string
	State                      string
	EffectiveAt                time.Time
	TransitionRecordArtifactID string
	TransitionRecordRevisionID string

	IsEntry bool

	TransitionKey    string
	FromAssignmentID string
	AttemptedAt      time.Time
	CompletedAt      time.Time
}

// AssignLifecycleStateResult names the records created.
type AssignLifecycleStateResult struct {
	TransitionRevisionKey engineering.RevisionKey
	AssignmentKey         engineering.RecordKey
}

// Execute validates the command and writes the Transition Record Revision
// and the State Assignment it establishes in one transaction.
func (c AssignLifecycleStateCommand) Execute(ctx context.Context, uow UnitOfWork, recorder EngineeringRecorder, clock Clock) (AssignLifecycleStateResult, error) {
	for field, value := range map[string]string{
		"assignment id": c.AssignmentID, "subject artifact id": c.SubjectArtifactID, "state": c.State,
		"transition record artifact id": c.TransitionRecordArtifactID, "transition record revision id": c.TransitionRecordRevisionID,
	} {
		if err := requireNonEmpty(field, value); err != nil {
			return AssignLifecycleStateResult{}, err
		}
	}
	if !c.IsEntry {
		for field, value := range map[string]string{
			"transition key": c.TransitionKey, "from assignment id": c.FromAssignmentID,
		} {
			if err := requireNonEmpty(field, value); err != nil {
				return AssignLifecycleStateResult{}, err
			}
		}
	}
	now := clock.Now()
	effectiveAt := requireTimeOrClock(c.EffectiveAt, clock)

	var result AssignLifecycleStateResult
	err := uow.Do(ctx, func(r Repositories) error {
		var artEnv engineering.ArtifactEnvelope
		var revEnv engineering.RevisionEnvelope
		var recEnv engineering.RecordEnvelope
		var err error

		if c.IsEntry {
			artEnv, revEnv, recEnv, err = recorder.RecordEntryAssignment(engineering.EntryAssignmentInput{
				AssignmentID: c.AssignmentID, SubjectArtifactID: c.SubjectArtifactID, State: c.State,
				EffectiveAt: effectiveAt, TransitionRecordArtifactID: c.TransitionRecordArtifactID,
				TransitionRecordRevisionID: c.TransitionRecordRevisionID, RecordedAt: now,
			})
		} else {
			fromKey, kerr := engineering.NewRecordKey(engineering.RecordKindStateAssignment, c.FromAssignmentID)
			if kerr != nil {
				return kerr
			}
			if _, found, gerr := r.Records.Get(ctx, fromKey); gerr != nil {
				return gerr
			} else if !found {
				return fmt.Errorf("%w: state assignment %s", ErrReferencedValueMissing, c.FromAssignmentID)
			}
			attemptedAt := requireTimeOrClock(c.AttemptedAt, clock)
			completedAt := requireTimeOrClock(c.CompletedAt, clock)
			artEnv, revEnv, recEnv, err = recorder.RecordTransition(engineering.TransitionInput{
				AssignmentID: c.AssignmentID, SubjectArtifactID: c.SubjectArtifactID, State: c.State,
				EffectiveAt: effectiveAt, TransitionRecordArtifactID: c.TransitionRecordArtifactID,
				TransitionRecordRevisionID: c.TransitionRecordRevisionID, TransitionKey: c.TransitionKey,
				FromAssignmentID: c.FromAssignmentID, AttemptedAt: attemptedAt, CompletedAt: completedAt, RecordedAt: now,
			})
		}
		if err != nil {
			return err
		}
		// The Transition Record Artifact is shared across every
		// transition for this subject; Put is idempotent once the first
		// transition (entry or otherwise) has registered it.
		if err := r.Artifacts.Put(ctx, artEnv); err != nil {
			return err
		}
		if err := r.Revisions.Put(ctx, revEnv); err != nil {
			return err
		}
		if err := r.Records.Put(ctx, recEnv); err != nil {
			return err
		}
		result = AssignLifecycleStateResult{TransitionRevisionKey: revEnv.Key, AssignmentKey: recEnv.Key}
		return nil
	})
	if err != nil {
		return AssignLifecycleStateResult{}, err
	}
	return result, nil
}
