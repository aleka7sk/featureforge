package application

import (
	"context"
	"errors"
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
	HasEffectiveAt   bool
	HasAttemptedAt   bool
	HasCompletedAt   bool
}

// AssignLifecycleStateResult names the records created.
type AssignLifecycleStateResult struct {
	TransitionRevisionKey engineering.RevisionKey
	AssignmentKey         engineering.RecordKey
}

// Execute validates the command and writes the Transition Record Revision
// and the State Assignment it establishes in one transaction.
func (c AssignLifecycleStateCommand) Execute(ctx context.Context, uow UnitOfWork, recorder EngineeringRecorder, inspector EngineeringReplayInspector, clock Clock) (AssignLifecycleStateResult, error) {
	for field, value := range map[string]string{
		"assignment id": c.AssignmentID, "subject artifact id": c.SubjectArtifactID,
		"transition record artifact id": c.TransitionRecordArtifactID, "transition record revision id": c.TransitionRecordRevisionID,
	} {
		if err := requireIdentity(field, value); err != nil {
			return AssignLifecycleStateResult{}, err
		}
	}
	if err := requireIdentity("state", c.State); err != nil {
		return AssignLifecycleStateResult{}, err
	}
	if err := requireOneOf("state", c.State, "drafting", "specified", "under-validation", "assessed"); err != nil {
		return AssignLifecycleStateResult{}, err
	}
	if !c.IsEntry {
		for field, value := range map[string]string{
			"transition key": c.TransitionKey, "from assignment id": c.FromAssignmentID,
		} {
			if err := requireIdentity(field, value); err != nil {
				return AssignLifecycleStateResult{}, err
			}
		}
		transitionTargets := map[string]string{
			"specify": "specified", "begin-validation": "under-validation", "assess": "assessed",
		}
		targetState, supported := transitionTargets[c.TransitionKey]
		if !supported {
			return AssignLifecycleStateResult{}, &fieldError{field: "transition key", reason: "must name a supported content-bearing transition"}
		}
		if c.State != targetState {
			return AssignLifecycleStateResult{}, &fieldError{field: "state", reason: "must match the configured transition target"}
		}
	} else {
		if c.State != "drafting" {
			return AssignLifecycleStateResult{}, &fieldError{field: "state", reason: "must be drafting for an entry assignment"}
		}
		for field, value := range map[string]string{
			"transition key": c.TransitionKey, "from assignment id": c.FromAssignmentID,
		} {
			if value != "" {
				return AssignLifecycleStateResult{}, &fieldError{field: field, reason: "must be omitted for an entry assignment"}
			}
		}
		if c.HasAttemptedAt || !c.AttemptedAt.IsZero() {
			return AssignLifecycleStateResult{}, &fieldError{field: "attempted at", reason: "must be omitted for an entry assignment"}
		}
		if c.HasCompletedAt || !c.CompletedAt.IsZero() {
			return AssignLifecycleStateResult{}, &fieldError{field: "completed at", reason: "must be omitted for an entry assignment"}
		}
	}
	for field, presentAndZero := range map[string]bool{
		"effective at": c.HasEffectiveAt && c.EffectiveAt.IsZero(),
		"attempted at": c.HasAttemptedAt && c.AttemptedAt.IsZero(),
		"completed at": c.HasCompletedAt && c.CompletedAt.IsZero(),
	} {
		if presentAndZero {
			return AssignLifecycleStateResult{}, &fieldError{field: field, reason: "must be a valid timestamp when present"}
		}
	}
	hasEffectiveAt := c.HasEffectiveAt || !c.EffectiveAt.IsZero()
	hasAttemptedAt := c.HasAttemptedAt || !c.AttemptedAt.IsZero()
	hasCompletedAt := c.HasCompletedAt || !c.CompletedAt.IsZero()
	now := normalizeTime(clock.Now())
	effectiveAt := optionalTime(c.EffectiveAt, now)
	attemptedAt := optionalTime(c.AttemptedAt, now)
	completedAt := optionalTime(c.CompletedAt, now)
	revisionKey, err := engineering.NewRevisionKey(c.TransitionRecordArtifactID, c.TransitionRecordRevisionID)
	if err != nil {
		return AssignLifecycleStateResult{}, invalidCommand(err)
	}
	assignmentKey, err := engineering.NewRecordKey(engineering.RecordKindStateAssignment, c.AssignmentID)
	if err != nil {
		return AssignLifecycleStateResult{}, invalidCommand(err)
	}

	var result AssignLifecycleStateResult
	err = uow.Do(ctx, func(r Repositories) error {
		storedRevision, revisionFound, err := r.Revisions.Get(ctx, revisionKey)
		if err != nil {
			return err
		}
		if !revisionFound {
			if err := validateUnmanagedRevisionMetadata(ctx, r, revisionKey); err != nil {
				return err
			}
		}
		storedAssignment, assignmentFound, err := r.Records.Get(ctx, assignmentKey)
		if err != nil {
			return err
		}
		assignments, err := r.Records.ListByKind(ctx, engineering.RecordKindStateAssignment)
		if err != nil {
			return err
		}
		owners := make([]engineering.RecordEnvelope, 0, 1)
		rootOwners := make([]engineering.RecordEnvelope, 0, 1)
		for _, candidate := range assignments {
			if err := inspectRecord(inspector, candidate); err != nil {
				return err
			}
			establishedBy, inspectErr := inspector.StateAssignmentEstablishedBy(candidate)
			if inspectErr != nil {
				return integrityError("state assignment established-by inspection", inspectErr)
			}
			if establishedBy.ArtifactID == c.TransitionRecordArtifactID {
				rootOwners = append(rootOwners, candidate)
			}
			if establishedBy == revisionKey {
				owners = append(owners, candidate)
			}
		}
		// AssignmentID is a secondary candidate identity. Its complete
		// lifecycle act must be inspected before an ordinary conflict on the
		// requested transition root; otherwise a dangling candidate could be
		// hidden behind a 409 for an unrelated coherent occupant.
		if assignmentFound {
			if err := validateStateAssignmentRoot(ctx, r, inspector, storedAssignment); err != nil {
				return err
			}
		}
		storedRootArtifact, rootArtifactFound, err := r.Artifacts.Get(ctx, engineering.ArtifactKey{ArtifactID: c.TransitionRecordArtifactID})
		if err != nil {
			return err
		}
		if rootArtifactFound {
			if err := inspectArtifact(inspector, storedRootArtifact); err != nil {
				return err
			}
			if err := validateForeignArtifactOccupancy(ctx, r, inspector, storedRootArtifact); err != nil {
				return err
			}
			rootFamily, inspectErr := inspector.ArtifactFamily(storedRootArtifact)
			if inspectErr != nil {
				return integrityError("transition-record root family is unreadable", inspectErr)
			}
			if rootFamily != engineering.RevisionFamilyTransitionRecord && len(rootOwners) != 0 {
				return integrityError("foreign artifact is named by a state-assignment parent", nil)
			}
			if rootFamily != engineering.RevisionFamilyTransitionRecord {
				return immutableConflict("transition-record artifact identity belongs to another family")
			}
			rootRevisions, lookupErr := r.Revisions.ListByArtifact(ctx, c.TransitionRecordArtifactID)
			if lookupErr != nil {
				return lookupErr
			}
			if len(rootRevisions) == 0 {
				return integrityError("transition-record root has no revisions", nil)
			}
			if rootRevisions[0].SubjectKey != engineering.ArtifactSubjectKey(c.SubjectArtifactID) {
				return immutableConflict("transition-record root belongs to another lifecycle subject")
			}
		} else {
			if err := validateAbsentArtifactHistory(ctx, r, inspector, c.TransitionRecordArtifactID); err != nil {
				return err
			}
			if len(rootOwners) != 0 {
				return integrityError("absent transition-record artifact has dangling state assignments", nil)
			}
		}
		if !revisionFound && len(owners) > 0 {
			return integrityError("lifecycle act has an assignment owner without its transition revision", nil)
		}
		if assignmentFound && !revisionFound {
			if err := validateStateAssignmentRoot(ctx, r, inspector, storedAssignment); err != nil {
				return err
			}
			return immutableConflict("assignment identity already belongs to another lifecycle act")
		}
		if revisionFound && storedRevision.RevisionFamily != engineering.RevisionFamilyTransitionRecord {
			if err := inspectRevision(inspector, storedRevision); err != nil {
				return err
			}
			artifact, found, lookupErr := r.Artifacts.Get(ctx, engineering.ArtifactKey{ArtifactID: storedRevision.Key.ArtifactID})
			if lookupErr != nil {
				return lookupErr
			}
			if !found {
				return integrityError("foreign transition revision has no owning artifact", nil)
			}
			if err := inspectArtifact(inspector, artifact); err != nil {
				return err
			}
			if err := validateForeignArtifactOccupancy(ctx, r, inspector, artifact); err != nil {
				return err
			}
			if artifact.ArtifactType != storedRevision.ArtifactType {
				return integrityError("foreign transition revision disagrees with its artifact", nil)
			}
			if len(owners) != 0 {
				return integrityError("foreign revision unexpectedly establishes a state assignment", nil)
			}
			if assignmentFound {
				if err := validateStateAssignmentRoot(ctx, r, inspector, storedAssignment); err != nil {
					return err
				}
			}
			return immutableConflict("transition revision identity belongs to another revision family")
		}

		if revisionFound || assignmentFound || len(owners) > 0 {
			if !revisionFound {
				return integrityError("lifecycle act has an assignment without its transition revision", nil)
			}
			if err := inspectRevision(inspector, storedRevision); err != nil {
				return err
			}
			if len(owners) != 1 {
				return integrityError("transition revision must establish exactly one state assignment", nil)
			}
			owner := owners[0]
			if err := validateStateAssignmentAct(ctx, r, inspector, owner); err != nil {
				return err
			}
			artifact, found, err := r.Artifacts.Get(ctx, engineering.ArtifactKey{ArtifactID: c.TransitionRecordArtifactID})
			if err != nil {
				return err
			}
			if !found {
				return integrityError("lifecycle act has no transition-record artifact", nil)
			}
			if err := inspectArtifact(inspector, artifact); err != nil {
				return err
			}
			if err := validateForeignArtifactOccupancy(ctx, r, inspector, artifact); err != nil {
				return err
			}
			if artifact.ArtifactType != storedRevision.ArtifactType {
				return integrityError("transition artifact and revision families disagree", nil)
			}
			if owner.Key != assignmentKey {
				if assignmentFound {
					if err := validateStateAssignmentRoot(ctx, r, inspector, storedAssignment); err != nil {
						return err
					}
				}
				return immutableConflict("transition revision already belongs to another assignment")
			}
			if !assignmentFound || storedAssignment.Key != owner.Key {
				return integrityError("lifecycle assignment lookup disagrees with transition ownership", nil)
			}
			if !canonicalTimeEqual(storedRevision.RecordedAt, storedAssignment.RecordedAt) {
				return integrityError("transition revision and assignment disagree on recorded time", nil)
			}

			resultingID, hasResulting, inspectErr := inspector.TransitionResultingAssignment(storedRevision)
			if inspectErr != nil {
				return integrityError("transition resulting-assignment inspection", inspectErr)
			}
			if c.IsEntry == hasResulting || (hasResulting && resultingID != c.AssignmentID) {
				return immutableConflict("entry/content-bearing transition representation differs")
			}
			if hasEffectiveAt && !canonicalTimeEqual(c.EffectiveAt, storedAssignment.OccurredAt) {
				return immutableConflict("state assignment has a different effective time")
			}
			storedAttemptedAt, storedCompletedAt := time.Time{}, time.Time{}
			if !c.IsEntry {
				storedFromID, hasStoredFrom, inspectErr := inspector.TransitionPredecessor(storedRevision)
				if inspectErr != nil {
					return integrityError("transition predecessor inspection", inspectErr)
				}
				if !hasStoredFrom {
					return integrityError("content-bearing transition has no predecessor", nil)
				}
				fromKey, keyErr := engineering.NewRecordKey(engineering.RecordKindStateAssignment, storedFromID)
				if keyErr != nil {
					return integrityError("transition predecessor identity is invalid", keyErr)
				}
				from, found, lookupErr := r.Records.Get(ctx, fromKey)
				if lookupErr != nil {
					return lookupErr
				}
				if !found {
					return integrityError("transition predecessor is dangling", nil)
				}
				if err := validateStateAssignmentAct(ctx, r, inspector, from); err != nil {
					return err
				}
				if from.SubjectKey != storedAssignment.SubjectKey {
					return integrityError("transition predecessor belongs to another lifecycle subject", nil)
				}
				storedAttemptedAt, storedCompletedAt, inspectErr = inspector.TransitionTimes(storedRevision)
				if inspectErr != nil {
					return integrityError("transition time inspection", inspectErr)
				}
				if hasAttemptedAt && !canonicalTimeEqual(c.AttemptedAt, storedAttemptedAt) {
					return immutableConflict("transition has a different attempted time")
				}
				if hasCompletedAt && !canonicalTimeEqual(c.CompletedAt, storedCompletedAt) {
					return immutableConflict("transition has a different completed time")
				}
			}

			var expectedArtifact engineering.ArtifactEnvelope
			var expectedRevision engineering.RevisionEnvelope
			var expectedAssignment engineering.RecordEnvelope
			if c.IsEntry {
				expectedArtifact, expectedRevision, expectedAssignment, err = recorder.RecordEntryAssignment(engineering.EntryAssignmentInput{
					AssignmentID: c.AssignmentID, SubjectArtifactID: c.SubjectArtifactID, State: c.State,
					EffectiveAt: storedAssignment.OccurredAt, TransitionRecordArtifactID: c.TransitionRecordArtifactID,
					TransitionRecordRevisionID: c.TransitionRecordRevisionID, RecordedAt: storedRevision.RecordedAt,
				})
			} else {
				expectedArtifact, expectedRevision, expectedAssignment, err = recorder.RecordTransition(engineering.TransitionInput{
					AssignmentID: c.AssignmentID, SubjectArtifactID: c.SubjectArtifactID, State: c.State,
					EffectiveAt: storedAssignment.OccurredAt, TransitionRecordArtifactID: c.TransitionRecordArtifactID,
					TransitionRecordRevisionID: c.TransitionRecordRevisionID, TransitionKey: c.TransitionKey,
					FromAssignmentID: c.FromAssignmentID, AttemptedAt: storedAttemptedAt, CompletedAt: storedCompletedAt,
					RecordedAt: storedRevision.RecordedAt,
				})
			}
			if err != nil {
				return invalidCommand(err)
			}
			if !artifact.Equal(expectedArtifact) || !storedRevision.Equal(expectedRevision) || !storedAssignment.Equal(expectedAssignment) {
				return immutableConflict("lifecycle identities are occupied by different semantics")
			}
			result = AssignLifecycleStateResult{TransitionRevisionKey: storedRevision.Key, AssignmentKey: storedAssignment.Key}
			return nil
		}

		var artEnv engineering.ArtifactEnvelope
		var revEnv engineering.RevisionEnvelope
		var recEnv engineering.RecordEnvelope

		var ordinaryDependencyError error
		if err := validateCapabilityArtifactReference(ctx, r, recorder, inspector, c.SubjectArtifactID, "lifecycle subject", false); err != nil {
			if !errors.Is(err, ErrReferencedValueMissing) {
				return err
			}
			ordinaryDependencyError = err
		}

		if c.IsEntry {
			if ordinaryDependencyError != nil {
				return ordinaryDependencyError
			}
			if err := requireServerTime(now, "a new lifecycle-entry act"); err != nil {
				return err
			}
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
			from, found, gerr := r.Records.Get(ctx, fromKey)
			if gerr != nil {
				return gerr
			} else if !found {
				if ordinaryDependencyError == nil {
					ordinaryDependencyError = fmt.Errorf("%w: state assignment %s", ErrReferencedValueMissing, c.FromAssignmentID)
				}
			} else {
				if err := validateStateAssignmentRoot(ctx, r, inspector, from); err != nil {
					return err
				}
				if from.SubjectKey != engineering.ArtifactSubjectKey(c.SubjectArtifactID) && ordinaryDependencyError == nil {
					ordinaryDependencyError = fmt.Errorf("%w: predecessor assignment belongs to another lifecycle subject", ErrReferencedValueMissing)
				}
				predecessorParent, inspectErr := inspector.StateAssignmentEstablishedBy(from)
				if inspectErr != nil {
					return integrityError("predecessor assignment parent is unreadable", inspectErr)
				}
				if predecessorParent.ArtifactID != c.TransitionRecordArtifactID && ordinaryDependencyError == nil {
					ordinaryDependencyError = fmt.Errorf("%w: predecessor assignment belongs to another transition-record root", ErrReferencedValueMissing)
				}
			}
			if ordinaryDependencyError != nil {
				return ordinaryDependencyError
			}
			if err := requireServerTime(now, "a new lifecycle-transition act"); err != nil {
				return err
			}
			artEnv, revEnv, recEnv, err = recorder.RecordTransition(engineering.TransitionInput{
				AssignmentID: c.AssignmentID, SubjectArtifactID: c.SubjectArtifactID, State: c.State,
				EffectiveAt: effectiveAt, TransitionRecordArtifactID: c.TransitionRecordArtifactID,
				TransitionRecordRevisionID: c.TransitionRecordRevisionID, TransitionKey: c.TransitionKey,
				FromAssignmentID: c.FromAssignmentID, AttemptedAt: attemptedAt, CompletedAt: completedAt, RecordedAt: now,
			})
		}
		if err != nil {
			return invalidCommand(err)
		}
		// The Transition Record Artifact is shared across every
		// transition for this subject; Put is idempotent once the first
		// transition (entry or otherwise) has registered it.
		if rootArtifactFound {
			if !storedRootArtifact.Equal(artEnv) {
				return immutableConflict("transition-record artifact identity belongs to another family")
			}
			revisions, lookupErr := r.Revisions.ListByArtifact(ctx, storedRootArtifact.Key.ArtifactID)
			if lookupErr != nil {
				return lookupErr
			}
			for _, revision := range revisions {
				if revision.SubjectKey != recEnv.SubjectKey {
					return immutableConflict("transition-record root belongs to another lifecycle subject")
				}
			}
			if c.IsEntry {
				return immutableConflict("transition-record root already has an entry assignment")
			}
		} else if err := r.Artifacts.Put(ctx, artEnv); err != nil {
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
