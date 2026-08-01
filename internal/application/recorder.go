package application

import (
	"time"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

// EngineeringRecorder is the port through which application causes PEOS
// values to be constructed, without importing the PEOS SDK itself
// (FF-008 §5.4). It is declared here, in domain/engineering types only, and
// implemented by internal/engineering/peos.Recorder -- structurally, via Go's
// implicit interface satisfaction, so neither package imports the other.
type EngineeringRecorder interface {
	RecordLifecycleConfiguration() (engineering.LifecycleDefinitionEnvelope, engineering.LifecycleDefinitionVersionEnvelope, error)
	RecordCapabilityArtifact(artifactID string, recordedAt time.Time) (engineering.ArtifactEnvelope, error)
	RecordCapabilityRevision(in engineering.CapabilityRevisionInput) (engineering.RevisionEnvelope, error)
	RecordEvidence(in engineering.EvidenceInput) (engineering.ArtifactEnvelope, engineering.RevisionEnvelope, error)
	RecordRequirement(in engineering.RequirementInput) (engineering.ArtifactEnvelope, engineering.RevisionEnvelope, error)
	RecordDecision(in engineering.DecisionInput) (engineering.RecordEnvelope, error)
	RecordValidationPlan(in engineering.PlanInput) (engineering.ArtifactEnvelope, engineering.RevisionEnvelope, error)
	RecordExecution(in engineering.ExecutionInput) (engineering.RecordEnvelope, error)
	RecordClaim(in engineering.ClaimInput) (engineering.RecordEnvelope, error)
	RecordEntryAssignment(in engineering.EntryAssignmentInput) (engineering.ArtifactEnvelope, engineering.RevisionEnvelope, engineering.RecordEnvelope, error)
	RecordTransition(in engineering.TransitionInput) (engineering.ArtifactEnvelope, engineering.RevisionEnvelope, engineering.RecordEnvelope, error)
	VerifyContentDigest(rev engineering.RevisionEnvelope, content engineering.CapabilitySpecificationContent) error
}

// ProposalEngineeringRecorder is the narrow M.6 construction extension used
// only after a transient proposal has been reviewed against a fresh ContextPack
// (AD-034, FF-024). The concrete PEOS adapter owns provenance/origin encoding;
// application supplies only PEOS-free content-address witnesses.
type ProposalEngineeringRecorder interface {
	EngineeringRecorder
	RecordAIAssistedCapabilityRevision(
		in engineering.CapabilityRevisionInput,
		proposalDigest engineering.Digest,
		contextDigest engineering.Digest,
		sources []string,
	) (engineering.RevisionEnvelope, error)
}
