package peos

import (
	"fmt"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

// InspectLifecycleAssignment returns a fully validated, PEOS-free projection
// of one State Assignment.
func (Recorder) InspectLifecycleAssignment(env engineering.RecordEnvelope) (engineering.LifecycleAssignmentDetail, error) {
	if err := (Recorder{}).ValidateRecord(env); err != nil {
		return engineering.LifecycleAssignmentDetail{}, err
	}
	if env.Kind != engineering.RecordKindStateAssignment {
		return engineering.LifecycleAssignmentDetail{}, fmt.Errorf("record is not a state assignment")
	}
	assignment, err := DecodeStateAssignment(env.Payload)
	if err != nil {
		return engineering.LifecycleAssignmentDetail{}, err
	}
	subjectKind, subjectArtifactID, subjectRevisionID, err := engineering.ParseSubjectKey(env.SubjectKey)
	if err != nil || subjectKind != engineering.SubjectKindArtifact || subjectRevisionID != "" {
		return engineering.LifecycleAssignmentDetail{}, fmt.Errorf("state assignment subject is not an artifact: %w", err)
	}
	definitionVersion, err := engineering.NewLifecycleDefinitionVersionKey(
		assignment.DefinitionVersion().LifecycleDefinitionID().String(),
		assignment.DefinitionVersion().VersionID().String(),
	)
	if err != nil {
		return engineering.LifecycleAssignmentDetail{}, err
	}
	provenance := assignment.Provenance()
	actor, hasActor := provenance.Actor()
	recordedAt, hasRecordedAt := provenance.RecordedAt()
	if !hasActor || !hasRecordedAt {
		return engineering.LifecycleAssignmentDetail{}, fmt.Errorf("state assignment provenance is incomplete")
	}
	projectedActor, _ := projectActor(actor)
	establishedBy := assignment.EstablishedBy()
	establishingKey, err := engineering.NewRevisionKey(establishedBy.ArtifactID().String(), establishedBy.RevisionID().String())
	if err != nil {
		return engineering.LifecycleAssignmentDetail{}, err
	}
	return engineering.LifecycleAssignmentDetail{
		AssignmentID:      assignment.ID().String(),
		SubjectArtifactID: subjectArtifactID,
		DefinitionVersion: definitionVersion,
		StateID:           trimFeatureForgeNamespace(assignment.State().String()),
		EffectiveAt:       assignment.EffectiveAt().Time(),
		RecordedAt:        recordedAt.Time(),
		Actor:             projectedActor,
		EstablishedBy:     establishingKey,
	}, nil
}

// InspectLifecycleTransition returns a fully validated, PEOS-free projection
// of one entry or content-bearing Transition Record Revision.
func (Recorder) InspectLifecycleTransition(env engineering.RevisionEnvelope) (engineering.LifecycleTransitionDetail, error) {
	if err := (Recorder{}).ValidateRevision(env); err != nil {
		return engineering.LifecycleTransitionDetail{}, err
	}
	if env.RevisionFamily != engineering.RevisionFamilyTransitionRecord {
		return engineering.LifecycleTransitionDetail{}, fmt.Errorf("revision is not a transition record")
	}
	subjectKind, subjectArtifactID, subjectRevisionID, err := engineering.ParseSubjectKey(env.SubjectKey)
	if err != nil || subjectKind != engineering.SubjectKindArtifact || subjectRevisionID != "" {
		return engineering.LifecycleTransitionDetail{}, fmt.Errorf("transition subject is not an artifact: %w", err)
	}

	if transition, decodeErr := DecodeTransitionRecordRevision(env.Payload); decodeErr == nil {
		content := transition.Content()
		completedAt, hasCompletedAt := content.CompletedAt()
		resulting, hasResulting := content.ResultingAssignment()
		target, hasTarget := content.ToState()
		if !hasCompletedAt || !hasResulting || !hasTarget {
			return engineering.LifecycleTransitionDetail{}, fmt.Errorf("transition record content is incomplete")
		}
		definitionVersion, err := engineering.NewLifecycleDefinitionVersionKey(
			content.DefinitionVersion().LifecycleDefinitionID().String(),
			content.DefinitionVersion().VersionID().String(),
		)
		if err != nil {
			return engineering.LifecycleTransitionDetail{}, err
		}
		provenance := transition.Core().Provenance()
		actor, hasActor := provenance.Actor()
		recordedAt, hasRecordedAt := provenance.RecordedAt()
		if !hasActor || !hasRecordedAt {
			return engineering.LifecycleTransitionDetail{}, fmt.Errorf("transition revision provenance is incomplete")
		}
		projectedActor, _ := projectActor(actor)
		return engineering.LifecycleTransitionDetail{
			RevisionKey:           env.Key,
			SubjectArtifactID:     subjectArtifactID,
			DefinitionVersion:     definitionVersion,
			TransitionID:          trimFeatureForgeNamespace(content.Transition().String()),
			FromAssignmentID:      content.FromAssignment().StateAssignmentID().String(),
			ResultingAssignmentID: resulting.StateAssignmentID().String(),
			TargetStateID:         trimFeatureForgeNamespace(target.String()),
			AttemptedAt:           content.AttemptedAt().Time(),
			CompletedAt:           completedAt.Time(),
			RecordedAt:            recordedAt.Time(),
			Actor:                 projectedActor,
		}, nil
	}

	entry, err := DecodeArtifactRevision(env.Payload)
	if err != nil {
		return engineering.LifecycleTransitionDetail{}, err
	}
	provenance := entry.Provenance()
	actor, hasActor := provenance.Actor()
	recordedAt, hasRecordedAt := provenance.RecordedAt()
	if !hasActor || !hasRecordedAt {
		return engineering.LifecycleTransitionDetail{}, fmt.Errorf("entry transition revision provenance is incomplete")
	}
	projectedActor, _ := projectActor(actor)
	return engineering.LifecycleTransitionDetail{
		RevisionKey:       env.Key,
		Entry:             true,
		SubjectArtifactID: subjectArtifactID,
		RecordedAt:        recordedAt.Time(),
		Actor:             projectedActor,
	}, nil
}
