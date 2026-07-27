package engineering

import "fmt"

// RecordKind is the closed set of immutable-record and envelope families
// FeatureForge persists (FF-009 §3).
type RecordKind string

const (
	RecordKindArtifact        RecordKind = "artifact"
	RecordKindRevision        RecordKind = "revision"
	RecordKindDecision        RecordKind = "decision"
	RecordKindExecution       RecordKind = "execution"
	RecordKindClaim           RecordKind = "claim"
	RecordKindStateAssignment RecordKind = "state-assignment"
)

// RevisionFamily distinguishes which specialization a RevisionEnvelope's
// payload decodes into (FF-009 §3.2).
type RevisionFamily string

const (
	RevisionFamilyCapability       RevisionFamily = "capability"
	RevisionFamilyRequirement      RevisionFamily = "requirement"
	RevisionFamilyValidationPlan   RevisionFamily = "validation-plan"
	RevisionFamilyTransitionRecord RevisionFamily = "transition-record"
	RevisionFamilyEvidence         RevisionFamily = "evidence"
)

// ArtifactKey identifies an ArtifactEnvelope: the owning Artifact only.
type ArtifactKey struct {
	ArtifactID string
}

// NewArtifactKey validates and returns an ArtifactKey.
func NewArtifactKey(artifactID string) (ArtifactKey, error) {
	if artifactID == "" {
		return ArtifactKey{}, fmt.Errorf("%w: artifact key requires a non-empty artifact id", ErrInvalidEnvelope)
	}
	return ArtifactKey{ArtifactID: artifactID}, nil
}

// IsZero reports whether k is the zero value.
func (k ArtifactKey) IsZero() bool { return k.ArtifactID == "" }

// String renders a stable, comparable form of the key.
func (k ArtifactKey) String() string { return k.ArtifactID }

// RevisionKey identifies a RevisionEnvelope: the exact (Artifact, Revision) pair.
type RevisionKey struct {
	ArtifactID string
	RevisionID string
}

// NewRevisionKey validates and returns a RevisionKey.
func NewRevisionKey(artifactID, revisionID string) (RevisionKey, error) {
	if artifactID == "" || revisionID == "" {
		return RevisionKey{}, fmt.Errorf("%w: revision key requires a non-empty artifact id and revision id", ErrInvalidEnvelope)
	}
	return RevisionKey{ArtifactID: artifactID, RevisionID: revisionID}, nil
}

// IsZero reports whether k is the zero value.
func (k RevisionKey) IsZero() bool { return k.ArtifactID == "" || k.RevisionID == "" }

// String renders a stable, comparable form of the key.
func (k RevisionKey) String() string { return k.ArtifactID + "/" + k.RevisionID }

// RecordKey identifies a RecordEnvelope: the record's family plus its own
// family-specific identity.
type RecordKey struct {
	Kind RecordKind
	ID   string
}

// NewRecordKey validates and returns a RecordKey.
func NewRecordKey(kind RecordKind, id string) (RecordKey, error) {
	if id == "" {
		return RecordKey{}, fmt.Errorf("%w: record key requires a non-empty id", ErrInvalidEnvelope)
	}
	switch kind {
	case RecordKindDecision, RecordKindExecution, RecordKindClaim, RecordKindStateAssignment:
	default:
		return RecordKey{}, fmt.Errorf("%w: unsupported record kind %q", ErrUnsupportedPayloadKind, kind)
	}
	return RecordKey{Kind: kind, ID: id}, nil
}

// IsZero reports whether k is the zero value.
func (k RecordKey) IsZero() bool { return k.ID == "" }

// String renders a stable, comparable form of the key.
func (k RecordKey) String() string { return string(k.Kind) + ":" + k.ID }
