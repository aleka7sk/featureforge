package peos

import (
	"encoding/json"
	"fmt"

	"github.com/aleka7sk/PEOS/peos/core"
	"github.com/aleka7sk/PEOS/peos/requirement"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

// BuildRequirement constructs the Requirement Artifact and its founding
// Revision (FF-011 §4.2). The requirement's subject is the capability
// Artifact at the identity level, converted through
// core.EngineeringSubjectRefFromArtifact -- the crossing point between
// requirement and validation this package alone is permitted to make
// (FF-003 §7).
func BuildRequirement(in RequirementInput) (engineering.ArtifactEnvelope, engineering.RevisionEnvelope, error) {
	artifactID, err := core.NewArtifactID(in.ArtifactID)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("requirement artifact id", err)
	}
	artifact, err := core.NewArtifact(artifactID, requirement.ArtifactTypeRequirement)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("requirement artifact", err)
	}
	req, err := requirement.New(artifact)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("requirement", err)
	}
	artifactPayload, err := json.Marshal(artifact)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("requirement artifact marshal", err)
	}
	artifactKey, err := engineering.NewArtifactKey(in.ArtifactID)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, err
	}
	artifactEnv, err := engineering.NewArtifactEnvelope(
		artifactKey, requirement.ArtifactTypeRequirement.String(), artifactPayload,
		engineering.ComputeDigest(artifactPayload), in.RecordedAt,
	)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, err
	}

	stmt, err := requirement.NewStatement(in.Statement)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("requirement statement", err)
	}
	subjectArtifactID, err := core.NewArtifactID(in.SubjectArtifactID)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("requirement subject artifact id", err)
	}
	subjectRef, err := core.NewArtifactRef(subjectArtifactID)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("requirement subject ref", err)
	}
	subject, err := core.EngineeringSubjectRefFromArtifact(subjectRef)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("requirement subject", err)
	}
	content, err := requirement.NewContent(
		[]requirement.Statement{stmt},
		[]core.EngineeringSubjectRef{subject},
		requirement.SubjectCombinationIndependent,
		requirement.NewUnrestrictedApplicability(),
	)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("requirement content", err)
	}
	contentPayload, err := json.Marshal(content)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("requirement content marshal", err)
	}
	contentDigest := engineering.ComputeDigest(contentPayload)

	revisionID, err := core.NewArtifactRevisionID(in.RevisionID)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("requirement revision id", err)
	}
	origin, err := core.NewOrigin(core.OriginKindKnown, "")
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("requirement origin", err)
	}
	provenance, err := provenanceFor(in.RecordedAt)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, err
	}
	integrity, err := contentAddressedIntegrity(contentDigest)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("requirement integrity", err)
	}
	coreRev, err := core.NewArtifactRevision(artifactID, revisionID, origin, provenance, integrity)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("requirement core revision", err)
	}
	reqRev, err := requirement.NewRevision(req, coreRev, content)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("requirement revision", err)
	}
	revKey, err := engineering.NewRevisionKey(in.ArtifactID, in.RevisionID)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, err
	}
	revEnv, err := buildRevisionEnvelope(revisionEnvelopeInput{
		Key:           revKey,
		Family:        engineering.RevisionFamilyRequirement,
		ArtifactType:  requirement.ArtifactTypeRequirement,
		Core:          coreRev,
		Payload:       reqRev,
		ContentDigest: contentDigest,
		RecordedAt:    in.RecordedAt,
	})
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, err
	}
	return artifactEnv, revEnv, nil
}

// DecodeRequirementRevision decodes a stored payload back into a
// requirement.Revision. Used only by this package's own tests and by
// projection-fidelity checks.
func DecodeRequirementRevision(payload []byte) (requirement.Revision, error) {
	var r requirement.Revision
	if err := json.Unmarshal(payload, &r); err != nil {
		return requirement.Revision{}, fmt.Errorf("%w: %w", ErrStoredPayloadInvalid, err)
	}
	return r, nil
}

// DecodeRequirement decodes a stored Requirement artifact payload back into
// a requirement.Requirement. Used only by this package's own tests.
func DecodeRequirement(payload []byte) (requirement.Requirement, error) {
	var r requirement.Requirement
	if err := json.Unmarshal(payload, &r); err != nil {
		return requirement.Requirement{}, fmt.Errorf("%w: %w", ErrStoredPayloadInvalid, err)
	}
	return r, nil
}
