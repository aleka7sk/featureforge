package engineering

import (
	"encoding/json"
	"fmt"
	"time"
)

func validatePayload(payload []byte, digest Digest) error {
	if len(payload) == 0 {
		return fmt.Errorf("%w: payload must not be empty", ErrInvalidEnvelope)
	}
	if !json.Valid(payload) {
		return fmt.Errorf("%w: payload is not valid JSON", ErrInvalidEnvelope)
	}
	if digest.IsZero() {
		return fmt.Errorf("%w: payload digest must not be zero", ErrInvalidEnvelope)
	}
	computed := ComputeDigest(payload)
	if !computed.Equal(digest) {
		return fmt.Errorf("%w: payload digest mismatch: computed %s, given %s", ErrInvalidEnvelope, computed, digest)
	}
	return nil
}

// samePayload reports whether two envelope payloads are byte-identical --
// the sole conflict/idempotency test for immutable records (FF-009 §3.4).
func samePayload(a, b []byte) bool { return string(a) == string(b) }

// ArtifactEnvelope is the persistence and projection carrier for a PEOS
// core.Artifact (FF-009 §3.1). It is a carrier, not a domain model: Payload
// is authoritative and every other field is a derived projection.
type ArtifactEnvelope struct {
	Key           ArtifactKey
	Kind          RecordKind
	ArtifactType  string
	Payload       []byte
	PayloadDigest Digest
	RecordedAt    time.Time
}

// NewArtifactEnvelope validates and returns an ArtifactEnvelope.
func NewArtifactEnvelope(key ArtifactKey, artifactType string, payload []byte, digest Digest, recordedAt time.Time) (ArtifactEnvelope, error) {
	if key.IsZero() {
		return ArtifactEnvelope{}, fmt.Errorf("%w: artifact envelope requires a non-zero key", ErrInvalidEnvelope)
	}
	if artifactType == "" {
		return ArtifactEnvelope{}, fmt.Errorf("%w: artifact envelope requires an artifact type", ErrInvalidEnvelope)
	}
	if err := validatePayload(payload, digest); err != nil {
		return ArtifactEnvelope{}, err
	}
	return ArtifactEnvelope{
		Key:           key,
		Kind:          RecordKindArtifact,
		ArtifactType:  artifactType,
		Payload:       append([]byte(nil), payload...),
		PayloadDigest: digest,
		RecordedAt:    recordedAt,
	}, nil
}

// Equal reports whether e and other have equal keys and byte-identical
// payloads. Projections are not compared.
func (e ArtifactEnvelope) Equal(other ArtifactEnvelope) bool {
	return e.Key == other.Key && samePayload(e.Payload, other.Payload)
}

// RevisionEnvelope is the persistence and projection carrier for a PEOS
// Artifact Revision specialization (FF-009 §3.2).
type RevisionEnvelope struct {
	Key                  RevisionKey
	Kind                 RecordKind
	RevisionFamily       RevisionFamily
	ArtifactType         string
	IntegrityValue       string
	ProvenanceActor      string
	ProvenanceRecordedAt time.Time
	HasProvenanceActor   bool
	HasProvenanceTime    bool
	ContentDigest        Digest
	Payload              []byte
	PayloadDigest        Digest
	RecordedAt           time.Time
}

// RevisionEnvelopeInput carries the projected fields for NewRevisionEnvelope.
type RevisionEnvelopeInput struct {
	Key                  RevisionKey
	RevisionFamily       RevisionFamily
	ArtifactType         string
	IntegrityValue       string
	ProvenanceActor      string
	HasProvenanceActor   bool
	ProvenanceRecordedAt time.Time
	HasProvenanceTime    bool
	ContentDigest        Digest
	Payload              []byte
	PayloadDigest        Digest
	RecordedAt           time.Time
}

// NewRevisionEnvelope validates and returns a RevisionEnvelope.
func NewRevisionEnvelope(in RevisionEnvelopeInput) (RevisionEnvelope, error) {
	if in.Key.IsZero() {
		return RevisionEnvelope{}, fmt.Errorf("%w: revision envelope requires a non-zero key", ErrInvalidEnvelope)
	}
	switch in.RevisionFamily {
	case RevisionFamilyCapability, RevisionFamilyRequirement, RevisionFamilyValidationPlan,
		RevisionFamilyTransitionRecord, RevisionFamilyEvidence:
	default:
		return RevisionEnvelope{}, fmt.Errorf("%w: unsupported revision family %q", ErrUnsupportedPayloadKind, in.RevisionFamily)
	}
	if in.ArtifactType == "" {
		return RevisionEnvelope{}, fmt.Errorf("%w: revision envelope requires an artifact type", ErrInvalidEnvelope)
	}
	if in.IntegrityValue == "" {
		return RevisionEnvelope{}, fmt.Errorf("%w: revision envelope requires an integrity value", ErrInvalidEnvelope)
	}
	if err := validatePayload(in.Payload, in.PayloadDigest); err != nil {
		return RevisionEnvelope{}, err
	}
	return RevisionEnvelope{
		Key:                  in.Key,
		Kind:                 RecordKindRevision,
		RevisionFamily:       in.RevisionFamily,
		ArtifactType:         in.ArtifactType,
		IntegrityValue:       in.IntegrityValue,
		ProvenanceActor:      in.ProvenanceActor,
		HasProvenanceActor:   in.HasProvenanceActor,
		ProvenanceRecordedAt: in.ProvenanceRecordedAt,
		HasProvenanceTime:    in.HasProvenanceTime,
		ContentDigest:        in.ContentDigest,
		Payload:              append([]byte(nil), in.Payload...),
		PayloadDigest:        in.PayloadDigest,
		RecordedAt:           in.RecordedAt,
	}, nil
}

// Equal reports whether e and other have equal keys and byte-identical
// payloads. Projections are not compared.
func (e RevisionEnvelope) Equal(other RevisionEnvelope) bool {
	return e.Key == other.Key && samePayload(e.Payload, other.Payload)
}

// RecordEnvelope is the persistence and projection carrier for an immutable
// record with family-specific identity: an execution record, a claim, a
// decision, or a state assignment (FF-009 §3.3).
type RecordEnvelope struct {
	Key                RecordKey
	Kind               RecordKind
	SubjectKey         string
	Scope              string
	OccurredAt         time.Time
	HasOccurredAt      bool
	Outcome            string
	CriterionKeys      []string
	EvidenceKeys       []string
	ExecutionKeys      []string
	CorrectionKind     string
	CorrectionTargetID string
	StateID            string
	Payload            []byte
	PayloadDigest      Digest
	RecordedAt         time.Time
}

// RecordEnvelopeInput carries the projected fields for NewRecordEnvelope.
type RecordEnvelopeInput struct {
	Key                RecordKey
	SubjectKey         string
	Scope              string
	OccurredAt         time.Time
	HasOccurredAt      bool
	Outcome            string
	CriterionKeys      []string
	EvidenceKeys       []string
	ExecutionKeys      []string
	CorrectionKind     string
	CorrectionTargetID string
	StateID            string
	Payload            []byte
	PayloadDigest      Digest
	RecordedAt         time.Time
}

// NewRecordEnvelope validates and returns a RecordEnvelope.
func NewRecordEnvelope(in RecordEnvelopeInput) (RecordEnvelope, error) {
	if in.Key.IsZero() {
		return RecordEnvelope{}, fmt.Errorf("%w: record envelope requires a non-zero key", ErrInvalidEnvelope)
	}
	if in.SubjectKey == "" {
		return RecordEnvelope{}, fmt.Errorf("%w: record envelope requires a subject key", ErrInvalidEnvelope)
	}
	if err := validatePayload(in.Payload, in.PayloadDigest); err != nil {
		return RecordEnvelope{}, err
	}
	return RecordEnvelope{
		Key:                in.Key,
		Kind:               in.Key.Kind,
		SubjectKey:         in.SubjectKey,
		Scope:              in.Scope,
		OccurredAt:         in.OccurredAt,
		HasOccurredAt:      in.HasOccurredAt,
		Outcome:            in.Outcome,
		CriterionKeys:      append([]string(nil), in.CriterionKeys...),
		EvidenceKeys:       append([]string(nil), in.EvidenceKeys...),
		ExecutionKeys:      append([]string(nil), in.ExecutionKeys...),
		CorrectionKind:     in.CorrectionKind,
		CorrectionTargetID: in.CorrectionTargetID,
		StateID:            in.StateID,
		Payload:            append([]byte(nil), in.Payload...),
		PayloadDigest:      in.PayloadDigest,
		RecordedAt:         in.RecordedAt,
	}, nil
}

// Equal reports whether e and other have equal keys and byte-identical
// payloads. Projections are not compared.
func (e RecordEnvelope) Equal(other RecordEnvelope) bool {
	return e.Key == other.Key && samePayload(e.Payload, other.Payload)
}

// HasCorrection reports whether e carries a correction reference.
func (e RecordEnvelope) HasCorrection() bool { return e.CorrectionKind != "" }
