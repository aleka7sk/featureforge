package peos

import (
	"fmt"
	"time"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

// Recorder implements internal/application's EngineeringRecorder port. It
// is the only concrete type in the module with authority to construct PEOS
// values; every method's parameters and return values are domain/engineering
// types or engineering.*Envelope values, never a PEOS type -- which is what
// lets application hold a Recorder behind an interface it declares itself,
// with no import of this package (FF-008 §5.4).
//
// Recorder is stateless and safe for concurrent use; every method is a pure
// function of its arguments plus the fixed vocabulary and lifecycle
// configuration in this package.
type Recorder struct{}

// NewRecorder returns a Recorder.
func NewRecorder() Recorder { return Recorder{} }

// RecordCapabilityArtifact constructs the capability specification Artifact.
func (Recorder) RecordCapabilityArtifact(artifactID string, recordedAt time.Time) (engineering.ArtifactEnvelope, error) {
	return BuildArtifact(artifactID, ArtifactTypeProductCapability, nil, recordedAt)
}

// RecordCapabilityRevision constructs a capability specification revision.
func (Recorder) RecordCapabilityRevision(in engineering.CapabilityRevisionInput) (engineering.RevisionEnvelope, error) {
	return BuildCapabilityRevision(in)
}

// RecordAIAssistedCapabilityRevision constructs the only AI-assisted
// capability-revision representation admitted by AD-034/FF-024. Actor and
// method remain fixed by this adapter; the caller supplies only the immutable
// PEOS-free witness values.
func (Recorder) RecordAIAssistedCapabilityRevision(in engineering.CapabilityRevisionInput, proposalDigest, contextDigest engineering.Digest, sources []string) (engineering.RevisionEnvelope, error) {
	witness, err := engineering.NewAIAssistanceWitness(proposalDigest, contextDigest, sources)
	if err != nil {
		return engineering.RevisionEnvelope{}, err
	}
	in.AIAssistance = witness
	return BuildCapabilityRevision(in)
}

// InspectAIAssistedCapabilityRevision validates the complete authoritative
// payload before classifying its provenance form. found is false only for the
// valid ordinary capability-revision representation; malformed AI metadata and
// non-capability revisions return an error.
func (Recorder) InspectAIAssistedCapabilityRevision(env engineering.RevisionEnvelope) (proposalDigest, contextDigest engineering.Digest, sources []string, found bool, err error) {
	if err := (Recorder{}).ValidateRevision(env); err != nil {
		return engineering.Digest{}, engineering.Digest{}, nil, false, err
	}
	if env.RevisionFamily != engineering.RevisionFamilyCapability {
		return engineering.Digest{}, engineering.Digest{}, nil, false, fmt.Errorf("revision is not a capability")
	}
	revision, err := DecodeArtifactRevision(env.Payload)
	if err != nil {
		return engineering.Digest{}, engineering.Digest{}, nil, false, err
	}
	witness, found, err := inspectCapabilityRevisionAssistance(revision)
	if err != nil {
		return engineering.Digest{}, engineering.Digest{}, nil, false, err
	}
	if !found {
		return engineering.Digest{}, engineering.Digest{}, nil, false, nil
	}
	return witness.ProposalDigest(), witness.ContextDigest(), witness.Sources(), true, nil
}

// RecordEvidence constructs the evidence Artifact and its one revision.
func (Recorder) RecordEvidence(in engineering.EvidenceInput) (engineering.ArtifactEnvelope, engineering.RevisionEnvelope, error) {
	return BuildEvidenceArtifactAndRevision(in)
}

// RecordRequirement constructs the Requirement Artifact and its founding
// revision.
func (Recorder) RecordRequirement(in engineering.RequirementInput) (engineering.ArtifactEnvelope, engineering.RevisionEnvelope, error) {
	return BuildRequirement(in)
}

// RecordDecision constructs a Decision with its Basis.
func (Recorder) RecordDecision(in engineering.DecisionInput) (engineering.RecordEnvelope, error) {
	return BuildDecision(in)
}

// RecordValidationPlan constructs the Validation Plan Artifact and its
// founding revision.
func (Recorder) RecordValidationPlan(in engineering.PlanInput) (engineering.ArtifactEnvelope, engineering.RevisionEnvelope, error) {
	return BuildValidationPlan(in)
}

// RecordExecution constructs a Validation Execution Record.
func (Recorder) RecordExecution(in engineering.ExecutionInput) (engineering.RecordEnvelope, error) {
	return BuildExecution(in)
}

// RecordClaim constructs a Validation Claim, optionally carrying a
// correction reference. It does not itself reject a self-correcting input;
// the caller (internal/application's CorrectValidationClaim command) is
// responsible for that check before calling this method (AD-017).
func (Recorder) RecordClaim(in engineering.ClaimInput) (engineering.RecordEnvelope, error) {
	return BuildClaim(in)
}

// RecordEntryAssignment constructs the lifecycle entry State Assignment,
// established by a content-free Transition Record Revision (AD-014).
func (Recorder) RecordEntryAssignment(in engineering.EntryAssignmentInput) (engineering.ArtifactEnvelope, engineering.RevisionEnvelope, engineering.RecordEnvelope, error) {
	return BuildEntryAssignment(in)
}

// RecordTransition constructs a full Transition Record Revision and the
// State Assignment it establishes.
func (Recorder) RecordTransition(in engineering.TransitionInput) (engineering.ArtifactEnvelope, engineering.RevisionEnvelope, engineering.RecordEnvelope, error) {
	return BuildTransition(in)
}

// VerifyContentDigest recomputes content's digest and confirms it matches
// both rev's projected ContentDigest and the integrity value recorded in
// the immutable PEOS revision itself.
func (Recorder) VerifyContentDigest(rev engineering.RevisionEnvelope, content engineering.CapabilitySpecificationContent) error {
	return VerifyContentDigest(rev, content)
}

// The four methods below implement internal/application's EngineeringProjector
// port (FF-020 §4): the sibling read seam to EngineeringRecorder, decoding a
// stored Payload back into the PEOS-free projections engineering/projections.go
// defines.

// ProjectRequirementStatement decodes a stored requirement revision
// payload and returns its statement text.
func (Recorder) ProjectRequirementStatement(payload []byte) (string, error) {
	return ProjectRequirementStatement(payload)
}

// ProjectDecisionDetail decodes a stored decision payload into its full
// basis.
func (Recorder) ProjectDecisionDetail(payload []byte) (engineering.DecisionDetail, error) {
	return ProjectDecisionDetail(payload)
}

// ProjectPlanActivities decodes a stored validation plan revision payload
// into its planned activities.
func (Recorder) ProjectPlanActivities(payload []byte) ([]engineering.PlanActivityDetail, error) {
	return ProjectPlanActivities(payload)
}

// ProjectClaimReasoning decodes a stored claim payload and returns its
// reasoning text, if any.
func (Recorder) ProjectClaimReasoning(payload []byte) (string, error) {
	return ProjectClaimReasoning(payload)
}
