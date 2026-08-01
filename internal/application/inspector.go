package application

import (
	"time"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

// EngineeringReplayInspector is the PEOS-free read authority used by
// commands to validate an already persisted semantic act before deciding
// replay, conflict, or corruption.  It is intentionally separate from the
// display-oriented EngineeringProjector and from EngineeringRecorder's
// construction authority.
type EngineeringReplayInspector interface {
	ConfiguredLifecycleVersionKey() engineering.LifecycleDefinitionVersionKey
	InspectLifecycleConfiguration(engineering.LifecycleDefinitionEnvelope, engineering.LifecycleDefinitionVersionEnvelope) (engineering.LifecyclePolicy, error)
	InspectLifecycleAssignment(engineering.RecordEnvelope) (engineering.LifecycleAssignmentDetail, error)
	InspectLifecycleTransition(engineering.RevisionEnvelope) (engineering.LifecycleTransitionDetail, error)
	ValidateArtifact(engineering.ArtifactEnvelope) error
	ArtifactFamily(engineering.ArtifactEnvelope) (engineering.RevisionFamily, error)
	ValidateCapabilityArtifact(engineering.ArtifactEnvelope) error
	ValidateEvidenceArtifact(engineering.ArtifactEnvelope) error
	ValidateRevision(engineering.RevisionEnvelope) error
	ValidateRecord(engineering.RecordEnvelope) error
	ValidateCapabilityContent(engineering.RevisionEnvelope, engineering.CapabilitySpecificationContent) error
	ValidationPlanReferences(engineering.RevisionEnvelope) (scopeArtifactID string, activityKeys []string, methods []string, subjects []engineering.RevisionKey, requirements []engineering.RevisionKey, err error)
	TransitionTimes(engineering.RevisionEnvelope) (attemptedAt, completedAt time.Time, err error)
	TransitionPredecessor(engineering.RevisionEnvelope) (assignmentID string, present bool, err error)
	TransitionResultingAssignment(engineering.RevisionEnvelope) (assignmentID string, present bool, err error)
	TransitionTargetState(engineering.RevisionEnvelope) (stateID string, present bool, err error)
	StateAssignmentEstablishedBy(engineering.RecordEnvelope) (engineering.RevisionKey, error)
	ExecutionPlanActivity(engineering.RecordEnvelope) (plan engineering.RevisionKey, activityKey, method string, err error)
	ClaimMethod(engineering.RecordEnvelope) (string, error)
}

// ProposalReplayInspector is the M.6 read authority for the one additional
// valid capability-Revision representation. A false found value means an
// otherwise valid ordinary capability Revision. A true value returns the
// strictly decoded AI-assisted provenance/origin witness. Malformed pairings
// (method without the governed Origin, or vice versa) return an error and are
// stored-state integrity at the application boundary.
type ProposalReplayInspector interface {
	EngineeringReplayInspector
	InspectAIAssistedCapabilityRevision(
		engineering.RevisionEnvelope,
	) (proposalDigest engineering.Digest, contextDigest engineering.Digest, sources []string, found bool, err error)
}
