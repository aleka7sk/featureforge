package peos

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/aleka7sk/PEOS/peos/core"
	"github.com/aleka7sk/PEOS/peos/lifecycle"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

// The lifecycle Definition and Definition Version are fixed FeatureForge
// configuration, established once in code (FF-003 §4), not persisted
// through any repository -- FF-009 §5 defines none for this family. They
// exist purely to supply the core.LifecycleDefinitionVersionRef every State
// Assignment and Transition Record Content requires.
var definitionVersionRef = mustDefinitionVersion()

func mustDefinitionVersion() core.LifecycleDefinitionVersionRef {
	defID, err := core.NewLifecycleDefinitionID("LCD-1")
	if err != nil {
		panic(fmt.Sprintf("peos: lifecycle definition id: %v", err))
	}
	defRef, err := core.NewLifecycleDefinitionRef(defID)
	if err != nil {
		panic(fmt.Sprintf("peos: lifecycle definition ref: %v", err))
	}
	verID, err := core.NewLifecycleDefinitionVersionID("LCDV-1")
	if err != nil {
		panic(fmt.Sprintf("peos: lifecycle definition version id: %v", err))
	}

	states := []lifecycle.State{
		mustState(StateDrafting, "specification work is under way"),
		mustState(StateSpecified, "an accepted revision and at least one requirement exist"),
		mustState(StateUnderValidation, "a validation plan exists and execution has begun"),
		mustState(StateAssessed, "validation has been executed and assessed"),
	}

	// The entry transition is modeled as reflexive on the sole initial
	// state (sourceStates = targetStates = {drafting}), verified against
	// v1.0.0: NewTransitionDefinition requires both non-empty, and PEOS-003
	// defines no pseudo-state for "unassigned" a TransitionDefinition could
	// otherwise name as its source.
	transitions := []lifecycle.TransitionDefinition{
		mustTransitionDefinition(TransitionEnter, []lifecycle.StateID{StateDrafting}, []lifecycle.StateID{StateDrafting}),
		mustTransitionDefinition(TransitionSpecify, []lifecycle.StateID{StateDrafting}, []lifecycle.StateID{StateSpecified}),
		mustTransitionDefinition(TransitionBeginValidation, []lifecycle.StateID{StateSpecified}, []lifecycle.StateID{StateUnderValidation}),
		mustTransitionDefinition(TransitionAssess, []lifecycle.StateID{StateUnderValidation}, []lifecycle.StateID{StateAssessed}),
	}

	scope, err := core.NewScope(CapabilityScopeKind, "*")
	if err != nil {
		panic(fmt.Sprintf("peos: lifecycle definition version scope: %v", err))
	}
	provenance, err := provenanceFor(configurationRecordedAt)
	if err != nil {
		panic(fmt.Sprintf("peos: lifecycle definition version provenance: %v", err))
	}

	dv, err := lifecycle.NewDefinitionVersion(
		verID, defRef, scope,
		[]core.VocabularyValue{LifecycleSubjectType},
		states,
		[]lifecycle.StateID{StateDrafting},
		transitions,
		TransitionEnter,
		provenance,
	)
	if err != nil {
		panic(fmt.Sprintf("peos: lifecycle definition version: %v", err))
	}
	ref, err := dv.Ref()
	if err != nil {
		panic(fmt.Sprintf("peos: lifecycle definition version ref: %v", err))
	}
	return ref
}

// configurationRecordedAt is the fixed provenance time for the one-time
// Lifecycle Definition Version configuration. It is a compile-time literal,
// not a call to time.Now, and it is not part of any engineering act's
// timeline -- the Definition Version itself is never persisted or surfaced
// to callers (FF-003 §4).
var configurationRecordedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func mustState(id lifecycle.StateID, meaning string) lifecycle.State {
	s, err := lifecycle.NewState(id, meaning)
	if err != nil {
		panic(fmt.Sprintf("peos: lifecycle state: %v", err))
	}
	return s
}

func mustTransitionDefinition(id lifecycle.TransitionID, sources, targets []lifecycle.StateID) lifecycle.TransitionDefinition {
	td, err := lifecycle.NewTransitionDefinition(id, sources, targets)
	if err != nil {
		panic(fmt.Sprintf("peos: lifecycle transition definition: %v", err))
	}
	return td
}

// lifecycleSubject converts a capability artifact identity into a
// core.LifecycleSubjectRef -- the crossing point into peos/lifecycle.
func lifecycleSubject(artifactID string) (core.LifecycleSubjectRef, error) {
	id, err := core.NewArtifactID(artifactID)
	if err != nil {
		return core.LifecycleSubjectRef{}, wrapPEOS("lifecycle subject artifact id", err)
	}
	ref, err := core.NewArtifactRef(id)
	if err != nil {
		return core.LifecycleSubjectRef{}, wrapPEOS("lifecycle subject artifact ref", err)
	}
	subject, err := core.NewLifecycleSubjectRefFromArtifact(ref)
	if err != nil {
		return core.LifecycleSubjectRef{}, wrapPEOS("lifecycle subject", err)
	}
	return subject, nil
}

// BuildEntryAssignment constructs the lifecycle entry State Assignment,
// established by a content-free Transition Record Revision (AD-014):
// PEOS v1.0.0's NewTransitionRecordContent rejects a zero fromAssignment, so
// no entry Transition Record can carry TransitionRecordContent. Returns the
// content-free revision's envelope and the entry assignment's envelope.
func BuildEntryAssignment(in EntryAssignmentInput) (engineering.RevisionEnvelope, engineering.RecordEnvelope, error) {
	trArtifactID, err := core.NewArtifactID(in.TransitionRecordArtifactID)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, wrapPEOS("entry transition record artifact id", err)
	}
	trArtifact, err := core.NewArtifact(trArtifactID, lifecycle.ArtifactTypeTransitionRecord)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, wrapPEOS("entry transition record artifact", err)
	}
	if _, err := lifecycle.NewTransitionRecord(trArtifact); err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, wrapPEOS("entry transition record", err)
	}
	trRevisionID, err := core.NewArtifactRevisionID(in.TransitionRecordRevisionID)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, wrapPEOS("entry transition record revision id", err)
	}
	origin, err := core.NewOrigin(core.OriginKindKnown, "")
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, wrapPEOS("entry origin", err)
	}
	provenance, err := provenanceFor(in.RecordedAt)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, err
	}
	// No PEOS Content object exists to hash for a content-free revision;
	// its integrity protects the revision's own recorded identity.
	integrity, err := core.NewIntegrityIdentity(
		core.IntegrityMechanismImmutableVersionIdentifier,
		in.TransitionRecordArtifactID+"/"+in.TransitionRecordRevisionID,
		core.IntegrityProtectedScopeMetadata,
	)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, wrapPEOS("entry integrity", err)
	}
	entryRev, err := core.NewArtifactRevision(trArtifactID, trRevisionID, origin, provenance, integrity)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, wrapPEOS("entry revision", err)
	}
	trRevKey, err := engineering.NewRevisionKey(in.TransitionRecordArtifactID, in.TransitionRecordRevisionID)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, err
	}
	revEnv, err := buildRevisionEnvelope(revisionEnvelopeInput{
		Key:          trRevKey,
		Family:       engineering.RevisionFamilyTransitionRecord,
		ArtifactType: lifecycle.ArtifactTypeTransitionRecord,
		Core:         entryRev,
		Payload:      entryRev,
		RecordedAt:   in.RecordedAt,
	})
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, err
	}

	assignEnv, err := buildStateAssignment(
		in.AssignmentID, in.SubjectArtifactID, in.State, in.EffectiveAt,
		in.TransitionRecordArtifactID, in.TransitionRecordRevisionID, in.RecordedAt,
	)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, err
	}
	return revEnv, assignEnv, nil
}

// BuildTransition constructs a full Transition Record Revision -- carrying
// TransitionRecordContent that names the source State Assignment it
// departed from -- and the resulting State Assignment it establishes
// (FF-011 §4.9).
func BuildTransition(in TransitionInput) (engineering.RevisionEnvelope, engineering.RecordEnvelope, error) {
	subject, err := lifecycleSubject(in.SubjectArtifactID)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, err
	}
	fromAssignmentID, err := core.NewStateAssignmentID(in.FromAssignmentID)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, wrapPEOS("from assignment id", err)
	}
	fromRef, err := core.NewStateAssignmentRef(fromAssignmentID)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, wrapPEOS("from assignment ref", err)
	}

	trArtifactID, err := core.NewArtifactID(in.TransitionRecordArtifactID)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, wrapPEOS("transition record artifact id", err)
	}
	trRevisionID, err := core.NewArtifactRevisionID(in.TransitionRecordRevisionID)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, wrapPEOS("transition record revision id", err)
	}

	// The resulting State Assignment is built first, citing the not-yet-
	// constructed revision by a bare reference -- NewStateAssignment
	// performs no lookup on establishedBy (verified), exactly as the PEOS
	// lifecycle example itself does.
	resultingEstablishedBy, err := core.NewArtifactRevisionRef(trArtifactID, trRevisionID)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, wrapPEOS("resulting established-by ref", err)
	}
	resultingAssignmentID, err := core.NewStateAssignmentID(in.AssignmentID)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, wrapPEOS("resulting assignment id", err)
	}
	stateID, err := lifecycle.NewStateID(Namespace + ":" + in.State)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, wrapPEOS("resulting state id", err)
	}
	effectiveAt, err := core.NewTimestamp(in.EffectiveAt.UTC())
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, wrapPEOS("resulting effective at", err)
	}
	assignmentProvenance, err := provenanceFor(in.RecordedAt)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, err
	}
	resultingAssignment, err := lifecycle.NewStateAssignment(
		resultingAssignmentID, subject, definitionVersionRef, stateID, effectiveAt, assignmentProvenance, resultingEstablishedBy,
	)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, wrapPEOS("resulting state assignment", err)
	}
	resultingRef, err := resultingAssignment.Ref()
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, wrapPEOS("resulting assignment ref", err)
	}

	transitionID, err := lifecycle.NewTransitionID(Namespace + ":" + in.TransitionKey)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, wrapPEOS("transition id", err)
	}
	attemptedAt, err := core.NewTimestamp(in.AttemptedAt.UTC())
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, wrapPEOS("attempted at", err)
	}
	content, err := lifecycle.NewTransitionRecordContent(
		subject, definitionVersionRef, transitionID, fromRef, attemptedAt, lifecycle.TransitionOutcomeSucceeded,
	)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, wrapPEOS("transition record content", err)
	}
	content, err = content.WithToState(stateID)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, wrapPEOS("transition content with to-state", err)
	}
	completedAt, err := core.NewTimestamp(in.CompletedAt.UTC())
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, wrapPEOS("transition completed at", err)
	}
	content, err = content.WithCompletedAt(completedAt)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, wrapPEOS("transition content with completed-at", err)
	}
	content, err = content.WithResultingAssignment(resultingRef)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, wrapPEOS("transition content with resulting assignment", err)
	}
	content, err = content.WithAuthority(LocalAuthorityRef)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, wrapPEOS("transition content with authority", err)
	}

	contentPayload, err := json.Marshal(content)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, wrapPEOS("transition content marshal", err)
	}
	contentDigest := engineering.ComputeDigest(contentPayload)
	integrity, err := contentAddressedIntegrity(contentDigest)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, wrapPEOS("transition integrity", err)
	}
	origin, err := core.NewOrigin(core.OriginKindKnown, "")
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, wrapPEOS("transition origin", err)
	}
	revisionProvenance, err := provenanceFor(in.RecordedAt)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, err
	}
	coreRev, err := core.NewArtifactRevision(trArtifactID, trRevisionID, origin, revisionProvenance, integrity)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, wrapPEOS("transition core revision", err)
	}
	trArtifact, err := core.NewArtifact(trArtifactID, lifecycle.ArtifactTypeTransitionRecord)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, wrapPEOS("transition record artifact", err)
	}
	transitionRecord, err := lifecycle.NewTransitionRecord(trArtifact)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, wrapPEOS("transition record", err)
	}
	trRevision, err := lifecycle.NewTransitionRecordRevision(transitionRecord, coreRev, content)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, wrapPEOS("transition record revision", err)
	}

	revKey, err := engineering.NewRevisionKey(in.TransitionRecordArtifactID, in.TransitionRecordRevisionID)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, err
	}
	revEnv, err := buildRevisionEnvelope(revisionEnvelopeInput{
		Key:           revKey,
		Family:        engineering.RevisionFamilyTransitionRecord,
		ArtifactType:  lifecycle.ArtifactTypeTransitionRecord,
		Core:          coreRev,
		Payload:       trRevision,
		ContentDigest: contentDigest,
		RecordedAt:    in.RecordedAt,
	})
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, err
	}

	assignEnv, err := recordEnvelopeFromStateAssignment(resultingAssignment, in.AssignmentID, in.SubjectArtifactID, in.RecordedAt)
	if err != nil {
		return engineering.RevisionEnvelope{}, engineering.RecordEnvelope{}, err
	}
	return revEnv, assignEnv, nil
}

// buildStateAssignment constructs a State Assignment established by a bare
// Artifact Revision reference (no lookup performed by the SDK) and returns
// its persistence envelope.
func buildStateAssignment(assignmentID, subjectArtifactID, state string, effectiveAt time.Time, trArtifactID, trRevisionID string, recordedAt time.Time) (engineering.RecordEnvelope, error) {
	subject, err := lifecycleSubject(subjectArtifactID)
	if err != nil {
		return engineering.RecordEnvelope{}, err
	}
	id, err := core.NewStateAssignmentID(assignmentID)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("state assignment id", err)
	}
	stateID, err := lifecycle.NewStateID(Namespace + ":" + state)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("state id", err)
	}
	ts, err := core.NewTimestamp(effectiveAt.UTC())
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("effective at", err)
	}
	provenance, err := provenanceFor(recordedAt)
	if err != nil {
		return engineering.RecordEnvelope{}, err
	}
	establishedBy, err := core.NewArtifactRevisionRef(mustArtifactID(trArtifactID), mustArtifactRevisionID(trRevisionID))
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("established by ref", err)
	}
	assignment, err := lifecycle.NewStateAssignment(id, subject, definitionVersionRef, stateID, ts, provenance, establishedBy)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("state assignment", err)
	}
	return recordEnvelopeFromStateAssignment(assignment, assignmentID, subjectArtifactID, recordedAt)
}

// recordEnvelopeFromStateAssignment projects assignment's key and subject
// from the plain input strings the caller already has -- assignmentID and
// subjectArtifactID -- rather than from assignment.Ref() or
// assignment.Subject(), whose PEOS MarshalJSON forms internal/application
// could never independently reconstruct (it cannot import PEOS). Both
// engineering/peos (here, at write time) and internal/application (via
// engineering.ArtifactSubjectKey, at query time) derive the same key from
// the same plain data.
func recordEnvelopeFromStateAssignment(assignment lifecycle.StateAssignment, assignmentID, subjectArtifactID string, recordedAt time.Time) (engineering.RecordEnvelope, error) {
	payload, err := json.Marshal(assignment)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("state assignment marshal", err)
	}
	key, err := engineering.NewRecordKey(engineering.RecordKindStateAssignment, assignmentID)
	if err != nil {
		return engineering.RecordEnvelope{}, err
	}
	effectiveAt, hasEffectiveAt := projectTimestamp(assignment.EffectiveAt(), true)
	return engineering.NewRecordEnvelope(engineering.RecordEnvelopeInput{
		Key:           key,
		SubjectKey:    engineering.ArtifactSubjectKey(subjectArtifactID),
		OccurredAt:    effectiveAt,
		HasOccurredAt: hasEffectiveAt,
		StateID:       assignment.State().String(),
		Payload:       payload,
		PayloadDigest: engineering.ComputeDigest(payload),
		RecordedAt:    recordedAt,
	})
}

func mustArtifactID(value string) core.ArtifactID {
	id, err := core.NewArtifactID(value)
	if err != nil {
		panic(err)
	}
	return id
}

func mustArtifactRevisionID(value string) core.ArtifactRevisionID {
	id, err := core.NewArtifactRevisionID(value)
	if err != nil {
		panic(err)
	}
	return id
}

// DecodeTransitionRecordRevision decodes a stored payload back into a
// lifecycle.TransitionRecordRevision. Used only by this package's own tests
// and by projection-fidelity checks. Entry revisions (content-free) are not
// decodable through this function -- use DecodeArtifactRevision instead.
func DecodeTransitionRecordRevision(payload []byte) (lifecycle.TransitionRecordRevision, error) {
	var r lifecycle.TransitionRecordRevision
	if err := json.Unmarshal(payload, &r); err != nil {
		return lifecycle.TransitionRecordRevision{}, fmt.Errorf("%w: %w", ErrStoredPayloadInvalid, err)
	}
	return r, nil
}

// DecodeStateAssignment decodes a stored payload back into a
// lifecycle.StateAssignment. Used only by this package's own tests and by
// projection-fidelity checks.
func DecodeStateAssignment(payload []byte) (lifecycle.StateAssignment, error) {
	var a lifecycle.StateAssignment
	if err := json.Unmarshal(payload, &a); err != nil {
		return lifecycle.StateAssignment{}, fmt.Errorf("%w: %w", ErrStoredPayloadInvalid, err)
	}
	return a, nil
}
