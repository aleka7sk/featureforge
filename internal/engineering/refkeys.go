package engineering

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
