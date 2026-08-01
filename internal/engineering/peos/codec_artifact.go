package peos

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/aleka7sk/PEOS/peos/core"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

// provenanceFor builds the FeatureForge-standard Provenance: the single
// configured local actor and an explicit recorded-at time (FF-010 §2).
func provenanceFor(recordedAt time.Time) (core.Provenance, error) {
	ts, err := core.NewTimestamp(recordedAt.UTC())
	if err != nil {
		return core.Provenance{}, wrapPEOS("provenance timestamp", err)
	}
	return core.NewProvenance().WithActor(LocalActorRef).WithRecordedAt(ts), nil
}

func projectActor(actor core.ActorRef) (string, bool) {
	if actor.IsZero() {
		return "", false
	}
	return actor.Namespace() + ":" + actor.Identifier(), true
}

func projectTimestamp(ts core.Timestamp, ok bool) (time.Time, bool) {
	if !ok {
		return time.Time{}, false
	}
	return ts.Time(), true
}

// projectScope renders a core.Scope as "kind|expression" (FF-009 §3.3).
func projectScope(s core.Scope) string {
	if s.IsZero() {
		return ""
	}
	return s.Kind().String() + "|" + s.Expression()
}

// BuildArtifact constructs a core.Artifact of the given type and returns its
// persistence envelope. Used for both the capability specification Artifact
// and evidence Artifacts (FF-011 §4.1, §4.6): both are plain PEOS Artifacts
// differing only in declared type and role.
func BuildArtifact(artifactID string, artifactType core.ArtifactType, roles []core.ArtifactRole, recordedAt time.Time) (engineering.ArtifactEnvelope, error) {
	id, err := core.NewArtifactID(artifactID)
	if err != nil {
		return engineering.ArtifactEnvelope{}, wrapPEOS("artifact id", err)
	}
	artifact, err := core.NewArtifact(id, artifactType)
	if err != nil {
		return engineering.ArtifactEnvelope{}, wrapPEOS("artifact", err)
	}
	if len(roles) > 0 {
		artifact, err = artifact.WithRoles(roles...)
		if err != nil {
			return engineering.ArtifactEnvelope{}, wrapPEOS("artifact roles", err)
		}
	}
	payload, err := json.Marshal(artifact)
	if err != nil {
		return engineering.ArtifactEnvelope{}, wrapPEOS("artifact marshal", err)
	}
	key, err := engineering.NewArtifactKey(artifactID)
	if err != nil {
		return engineering.ArtifactEnvelope{}, err
	}
	digest := engineering.ComputeDigest(payload)
	return engineering.NewArtifactEnvelope(key, artifactType.String(), payload, digest, recordedAt)
}

// DecodeArtifact decodes a stored payload back into a core.Artifact. It is
// used only by this package's own tests and by projection-fidelity checks
// (FF-008 §5.3.1): no other package can name core.Artifact as a type.
func DecodeArtifact(payload []byte) (core.Artifact, error) {
	var a core.Artifact
	if err := json.Unmarshal(payload, &a); err != nil {
		return core.Artifact{}, fmt.Errorf("%w: %w", ErrStoredPayloadInvalid, err)
	}
	return a, nil
}

// revisionEnvelopeInput bundles the projected fields shared by every
// revision-producing codec. Core is the bare core.ArtifactRevision, used for
// projection (integrity, provenance); Payload is the value actually
// marshaled into the envelope -- Core itself for a bare Artifact Revision
// (capability, evidence), or the PEOS specialization wrapping it
// (requirement.Revision, validation.PlanRevision,
// lifecycle.TransitionRecordRevision) for every family PEOS defines one.
type revisionEnvelopeInput struct {
	Key           engineering.RevisionKey
	Family        engineering.RevisionFamily
	ArtifactType  core.ArtifactType
	Core          core.ArtifactRevision
	Payload       any
	ContentDigest engineering.Digest
	// SubjectKey answers "which capability is this revision about?"
	// (AD-025, FF-016 §3). Left empty for families with no subject
	// (capability, evidence); every other call site sets it via
	// engineering.ArtifactSubjectKey.
	SubjectKey string
	RecordedAt time.Time
}

// buildRevisionEnvelope marshals in.Payload and projects identity,
// integrity, and provenance fields (read from in.Core) into a
// RevisionEnvelope.
func buildRevisionEnvelope(in revisionEnvelopeInput) (engineering.RevisionEnvelope, error) {
	payload, err := json.Marshal(in.Payload)
	if err != nil {
		return engineering.RevisionEnvelope{}, wrapPEOS("revision marshal", err)
	}
	actor, hasActor := projectActor(mustProvenanceActor(in.Core.Provenance()))
	recordedTS, hasRecordedTS := in.Core.Provenance().RecordedAt()
	recordedAt, hasRecordedAt := projectTimestamp(recordedTS, hasRecordedTS)

	return engineering.NewRevisionEnvelope(engineering.RevisionEnvelopeInput{
		Key:                  in.Key,
		RevisionFamily:       in.Family,
		ArtifactType:         in.ArtifactType.String(),
		IntegrityValue:       in.Core.Integrity().Value(),
		ProvenanceActor:      actor,
		HasProvenanceActor:   hasActor,
		ProvenanceRecordedAt: recordedAt,
		HasProvenanceTime:    hasRecordedAt,
		ContentDigest:        in.ContentDigest,
		SubjectKey:           in.SubjectKey,
		Payload:              payload,
		PayloadDigest:        engineering.ComputeDigest(payload),
		RecordedAt:           in.RecordedAt,
	})
}

func mustProvenanceActor(p core.Provenance) core.ActorRef {
	actor, _ := p.Actor()
	return actor
}

func contentAddressedIntegrity(digest engineering.Digest) (core.IntegrityIdentity, error) {
	return core.NewIntegrityIdentity(
		core.IntegrityMechanismContentAddressedReference,
		"sha256:"+digest.Hex(),
		core.IntegrityProtectedScopeContent,
	)
}

// BuildCapabilityRevision constructs a core.ArtifactRevision for the
// capability specification, with an authoritative Representation and an
// IntegrityIdentity bound to the structured content's digest, and returns
// its persistence envelope (FF-011 §4.1, FF-003 §5).
func BuildCapabilityRevision(in CapabilityRevisionInput) (engineering.RevisionEnvelope, error) {
	artifactID, err := core.NewArtifactID(in.ArtifactID)
	if err != nil {
		return engineering.RevisionEnvelope{}, wrapPEOS("capability artifact id", err)
	}
	revisionID, err := core.NewArtifactRevisionID(in.RevisionID)
	if err != nil {
		return engineering.RevisionEnvelope{}, wrapPEOS("capability revision id", err)
	}
	origin, err := core.NewOrigin(core.OriginKindKnown, in.AIAssistance.OriginNote())
	if err != nil {
		return engineering.RevisionEnvelope{}, wrapPEOS("origin", err)
	}
	provenance, err := provenanceFor(in.RecordedAt)
	if err != nil {
		return engineering.RevisionEnvelope{}, err
	}
	if !in.AIAssistance.IsZero() {
		provenance = provenance.WithMethod(ProvenanceMethodAIAssisted)
	}
	integrity, err := contentAddressedIntegrity(in.ContentDigest)
	if err != nil {
		return engineering.RevisionEnvelope{}, wrapPEOS("capability integrity", err)
	}
	rep, err := core.NewRepresentationFromContentAddress(
		ContentAddressAlgorithm, in.ContentDigest.Hex(),
		MediaTypeSpecificationContent, core.RepresentationRoleAuthoritative,
	)
	if err != nil {
		return engineering.RevisionEnvelope{}, wrapPEOS("capability representation", err)
	}
	rev, err := core.NewArtifactRevision(artifactID, revisionID, origin, provenance, integrity)
	if err != nil {
		return engineering.RevisionEnvelope{}, wrapPEOS("capability revision", err)
	}
	rev, err = rev.WithRepresentations(rep)
	if err != nil {
		return engineering.RevisionEnvelope{}, wrapPEOS("capability revision representations", err)
	}
	key, err := engineering.NewRevisionKey(in.ArtifactID, in.RevisionID)
	if err != nil {
		return engineering.RevisionEnvelope{}, err
	}
	return buildRevisionEnvelope(revisionEnvelopeInput{
		Key:           key,
		Family:        engineering.RevisionFamilyCapability,
		ArtifactType:  ArtifactTypeProductCapability,
		Core:          rev,
		Payload:       rev,
		ContentDigest: in.ContentDigest,
		RecordedAt:    in.RecordedAt,
	})
}

// DecodeArtifactRevision decodes a stored payload back into a
// core.ArtifactRevision. Used only by this package's own tests and by
// projection-fidelity checks.
func DecodeArtifactRevision(payload []byte) (core.ArtifactRevision, error) {
	var rev core.ArtifactRevision
	if err := json.Unmarshal(payload, &rev); err != nil {
		return core.ArtifactRevision{}, fmt.Errorf("%w: %w", ErrStoredPayloadInvalid, err)
	}
	return rev, nil
}

// BuildEvidenceArtifactAndRevision constructs the evidence Artifact (in the
// Evidence role) and its one revision, cited by external locator
// (FF-011 §4.6). Evidence is never stored by FeatureForge; only a locator
// reference is recorded.
func BuildEvidenceArtifactAndRevision(in EvidenceInput) (engineering.ArtifactEnvelope, engineering.RevisionEnvelope, error) {
	artifactEnv, err := BuildArtifact(in.ArtifactID, ArtifactTypeValidationEvidence, []core.ArtifactRole{core.ArtifactRoleEvidence}, in.RecordedAt)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, err
	}

	artifactID, err := core.NewArtifactID(in.ArtifactID)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("evidence artifact id", err)
	}
	revisionID, err := core.NewArtifactRevisionID(in.RevisionID)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("evidence revision id", err)
	}
	origin, err := core.NewOrigin(core.OriginKindKnown, "")
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("evidence origin", err)
	}
	provenance, err := provenanceFor(in.RecordedAt)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, err
	}
	integrity, err := core.NewIntegrityIdentity(
		core.IntegrityMechanismImmutableVersionIdentifier,
		in.Locator,
		core.IntegrityProtectedScopeRepresentation,
	)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("evidence integrity", err)
	}
	rep, err := core.NewRepresentationFromExternalReference(
		in.Locator, MediaTypeValidationReport, core.RepresentationRoleAuthoritative,
	)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("evidence representation", err)
	}
	rev, err := core.NewArtifactRevision(artifactID, revisionID, origin, provenance, integrity)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("evidence revision", err)
	}
	rev, err = rev.WithRepresentations(rep)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("evidence revision representations", err)
	}
	key, err := engineering.NewRevisionKey(in.ArtifactID, in.RevisionID)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, err
	}
	revEnv, err := buildRevisionEnvelope(revisionEnvelopeInput{
		Key:          key,
		Family:       engineering.RevisionFamilyEvidence,
		ArtifactType: ArtifactTypeValidationEvidence,
		Core:         rev,
		Payload:      rev,
		RecordedAt:   in.RecordedAt,
	})
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, err
	}
	return artifactEnv, revEnv, nil
}
