package engineering

import (
	"fmt"
	"strings"
)

// The functions below define the plain, PEOS-independent string form used
// for every projected reference key on an envelope: SubjectKey,
// CriterionKeys, EvidenceKeys, ExecutionKeys, CorrectionTargetID.
//
// They exist because a projected key must be reconstructible by
// internal/application, which cannot import PEOS and therefore cannot call
// core.EngineeringSubjectRef.MarshalJSON() (or any other PEOS reference
// type's own wire form) to recompute the same string a query needs to
// search or compare against. Both internal/engineering/peos (at write time,
// projecting a value it just built) and internal/application (at query
// time, deriving the same key from plain identity strings it already has)
// call these identical functions, so the two sides always agree.
//
// Every FeatureForge reference in the canonical scenario is one of: an
// Artifact-level subject, an Artifact-Revision-level subject, a Requirement
// Revision criterion, an Evidence citation, or an Execution Record
// citation -- the five forms below.

// ArtifactSubjectKey is the key for an EngineeringSubjectRef or
// LifecycleSubjectRef naming an Artifact at the identity level.
func ArtifactSubjectKey(artifactID string) string {
	return "artifact:" + artifactID
}

// ArtifactRevisionSubjectKey is the key for an EngineeringSubjectRef or
// LifecycleSubjectRef naming an exact Artifact Revision.
func ArtifactRevisionSubjectKey(artifactID, revisionID string) string {
	return "artifact-revision:" + artifactID + "/" + revisionID
}

// SubjectKind names which of the two subject forms a SubjectKey carries.
type SubjectKind string

const (
	// SubjectKindArtifact names an Artifact at the identity level.
	SubjectKindArtifact SubjectKind = "artifact"
	// SubjectKindArtifactRevision names an exact Artifact Revision.
	SubjectKindArtifactRevision SubjectKind = "artifact-revision"
)

// ParseSubjectKey is the inverse of ArtifactSubjectKey and
// ArtifactRevisionSubjectKey. It exists so that both persistence adapters
// resolve a RecordEnvelope's SubjectKey to the value it references using one
// shared definition of the key's shape, rather than each re-deriving the
// prefix convention independently (AD-021).
//
// For SubjectKindArtifact the returned revisionID is empty. A key that names
// neither form, or whose identity part is malformed, is ErrInvalidEnvelope.
func ParseSubjectKey(key string) (SubjectKind, string, string, error) {
	if after, found := strings.CutPrefix(key, "artifact-revision:"); found {
		artifactID, revisionID, split := strings.Cut(after, "/")
		if !split || artifactID == "" || revisionID == "" {
			return "", "", "", fmt.Errorf("%w: subject key %q must name artifact-revision:<artifact>/<revision>", ErrInvalidEnvelope, key)
		}
		return SubjectKindArtifactRevision, artifactID, revisionID, nil
	}
	if after, found := strings.CutPrefix(key, "artifact:"); found {
		if after == "" {
			return "", "", "", fmt.Errorf("%w: subject key %q must name artifact:<artifact>", ErrInvalidEnvelope, key)
		}
		return SubjectKindArtifact, after, "", nil
	}
	return "", "", "", fmt.Errorf("%w: subject key %q names neither an artifact nor an artifact revision", ErrInvalidEnvelope, key)
}

// RequirementCriterionKey is the key for a CriterionRef citing a
// Requirement's exact revision.
func RequirementCriterionKey(key RevisionKey) (string, error) {
	if key.IsZero() {
		return "", ErrInvalidEnvelope
	}
	return "requirement-revision:" + key.ArtifactID + "/" + key.RevisionID, nil
}

// ProductRuleCriterionKey is the key for a CriterionRef citing a Product
// Rule by its featureforge-namespace vocabulary value.
func ProductRuleCriterionKey(value string) string {
	return "product-rule:featureforge:" + value
}

// EvidenceKey is the key for an EvidenceArtifactRevisionRef.
func EvidenceKey(artifactID, revisionID string) string {
	return "evidence:" + artifactID + "/" + revisionID
}

// ParseEvidenceKey is the inverse of EvidenceKey. It exists so a caller
// holding only a claim's or an execution's projected EvidenceKeys can
// recover the evidence artifact ID using the one shared definition of the
// key's shape, rather than re-deriving the prefix convention independently
// at the call site (FF-018 §7, mirroring ParseSubjectKey under AD-021).
//
// A key that does not name evidence:<artifact>/<revision>, or whose
// identity part is malformed, is ErrInvalidEnvelope.
func ParseEvidenceKey(key string) (artifactID, revisionID string, err error) {
	after, found := strings.CutPrefix(key, "evidence:")
	if !found {
		return "", "", fmt.Errorf("%w: evidence key %q must name evidence:<artifact>/<revision>", ErrInvalidEnvelope, key)
	}
	artifactID, revisionID, split := strings.Cut(after, "/")
	if !split || artifactID == "" || revisionID == "" {
		return "", "", fmt.Errorf("%w: evidence key %q must name evidence:<artifact>/<revision>", ErrInvalidEnvelope, key)
	}
	return artifactID, revisionID, nil
}

// ExecutionKey is the key for a ValidationExecutionRecordRef.
func ExecutionKey(executionID string) string {
	return "execution:" + executionID
}

// ClaimKey is the key for a ValidationClaimRef.
func ClaimKey(claimID string) string {
	return "claim:" + claimID
}

// StateAssignmentKey is the key for a StateAssignmentRef.
func StateAssignmentKey(assignmentID string) string {
	return "state-assignment:" + assignmentID
}
