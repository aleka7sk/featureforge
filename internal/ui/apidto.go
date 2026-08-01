package ui

import (
	"encoding/json"
	"time"
)

// The types below are UI-owned JSON decode targets, mirroring
// internal/transport/http's response DTOs field for field where this
// package renders them. They are not imports of that package's types --
// this package cannot import internal/transport/http (FF-021 §2) -- and
// they are not domain/application/engineering types either; they exist
// only to give json.Unmarshal somewhere typed to land, reused across the
// handlers that share a response shape (Q3 embeds Q4's).

type apiAcceptanceCriterionDTO struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

type apiContentDTO struct {
	SchemaVersion        int                         `json:"schema_version"`
	Title                string                      `json:"title"`
	ProblemStatement     string                      `json:"problem_statement"`
	UserOutcome          string                      `json:"user_outcome"`
	FunctionalBehaviours []string                    `json:"functional_behaviours"`
	Constraints          []string                    `json:"constraints"`
	AcceptanceCriteria   []apiAcceptanceCriterionDTO `json:"acceptance_criteria"`
	Dependencies         []string                    `json:"dependencies"`
	OpenQuestions        []string                    `json:"open_questions"`
}

type apiRevisionDTO struct {
	ArtifactID           string         `json:"artifact_id"`
	RevisionID           string         `json:"revision_id"`
	Sequence             int            `json:"sequence"`
	RevisionFamily       string         `json:"revision_family"`
	IntegrityValue       string         `json:"integrity_value"`
	SubjectKey           string         `json:"subject_key"`
	RecordedAt           time.Time      `json:"recorded_at"`
	ProvenanceActor      string         `json:"provenance_actor"`
	ProvenanceMethod     string         `json:"provenance_method"`
	ProvenanceRecordedAt *time.Time     `json:"provenance_recorded_at"`
	Content              *apiContentDTO `json:"content"`
}

type apiCurrentRevisionDTO struct {
	Found    bool            `json:"found"`
	Revision *apiRevisionDTO `json:"revision"`
}

type apiConsideredRevisionDTO struct {
	ArtifactID      string `json:"artifact_id"`
	RevisionID      string `json:"revision_id"`
	Sequence        int    `json:"sequence"`
	AcceptanceState string `json:"acceptance_state"`
}

type apiRejectedRevisionDTO struct {
	ArtifactID string `json:"artifact_id"`
	RevisionID string `json:"revision_id"`
	Reason     string `json:"reason"`
}

type apiResolutionRationaleDTO struct {
	Rule               string                     `json:"rule"`
	SelectedArtifactID string                     `json:"selected_artifact_id"`
	SelectedRevisionID string                     `json:"selected_revision_id"`
	Considered         []apiConsideredRevisionDTO `json:"considered"`
	Rejected           []apiRejectedRevisionDTO   `json:"rejected"`
	Warnings           []string                   `json:"warnings"`
}

type apiEffectiveRequirementDTO struct {
	ArtifactID                   string `json:"artifact_id"`
	RevisionID                   string `json:"revision_id"`
	Statement                    string `json:"statement"`
	SourceCapabilityArtifactID   string `json:"source_capability_artifact_id"`
	SourceCapabilityRevisionID   string `json:"source_capability_revision_id"`
	SourceAcceptanceCriterionKey string `json:"source_acceptance_criterion_key"`
}

type apiRequirementRevisionHistoryDTO struct {
	ArtifactID                   string `json:"artifact_id"`
	RevisionID                   string `json:"revision_id"`
	Sequence                     int    `json:"sequence"`
	AcceptanceState              string `json:"acceptance_state"`
	Statement                    string `json:"statement"`
	SourceCapabilityArtifactID   string `json:"source_capability_artifact_id"`
	SourceCapabilityRevisionID   string `json:"source_capability_revision_id"`
	SourceAcceptanceCriterionKey string `json:"source_acceptance_criterion_key"`
}

type apiDecisionBasisDTO struct {
	Evidence      []string `json:"evidence"`
	Assumptions   []string `json:"assumptions"`
	Constraints   []string `json:"constraints"`
	Uncertainties []string `json:"uncertainties"`
}

type apiApplicableDecisionDTO struct {
	DecisionID       string              `json:"decision_id"`
	SubjectKey       string              `json:"subject_key"`
	OccurredAt       *time.Time          `json:"occurred_at"`
	Outcome          string              `json:"outcome"`
	Question         string              `json:"question"`
	OutcomeStatement string              `json:"outcome_statement"`
	Rationale        string              `json:"rationale"`
	Alternatives     []string            `json:"alternatives"`
	Basis            apiDecisionBasisDTO `json:"basis"`
}

type apiRejectedClaimDTO struct {
	RecordKey   string `json:"record_key"`
	Reason      string `json:"reason"`
	Outcome     string `json:"outcome"`
	Reasoning   string `json:"reasoning"`
	CorrectedBy string `json:"corrected_by"`
}

type apiPerRequirementReadinessDTO struct {
	RequirementArtifactID string                `json:"requirement_artifact_id"`
	HasClaim              bool                  `json:"has_claim"`
	ClaimID               string                `json:"claim_id"`
	Outcome               string                `json:"outcome"`
	Reasoning             string                `json:"reasoning"`
	CriterionKeys         []string              `json:"criterion_keys"`
	Corrects              string                `json:"corrects"`
	ExecutionOutcome      string                `json:"execution_outcome"`
	Stale                 bool                  `json:"stale"`
	VerdictReason         string                `json:"verdict_reason"`
	Rejected              []apiRejectedClaimDTO `json:"rejected"`
}

type apiReadinessResultDTO struct {
	Status         string                          `json:"status"`
	PerRequirement []apiPerRequirementReadinessDTO `json:"per_requirement"`
}

type apiLifecycleRationaleDTO struct {
	Rule string `json:"rule"`
}

type apiLifecycleStateDTO struct {
	Found                 bool       `json:"found"`
	StateID               string     `json:"state_id"`
	DefinitionID          string     `json:"definition_id"`
	DefinitionVersionID   string     `json:"definition_version_id"`
	EstablishedByArtifact string     `json:"established_by_artifact_id"`
	EstablishedByRevision string     `json:"established_by_revision_id"`
	OccurredAt            *time.Time `json:"occurred_at"`
}

type apiPlanActivityDetailDTO struct {
	Key                   string   `json:"key"`
	Method                string   `json:"method"`
	OutcomeInterpretation string   `json:"outcome_interpretation"`
	ExpectedEvidence      []string `json:"expected_evidence"`
}

type apiValidationPlanDTO struct {
	Found      bool                       `json:"found"`
	ArtifactID string                     `json:"artifact_id"`
	RevisionID string                     `json:"revision_id"`
	Activities []apiPlanActivityDetailDTO `json:"activities"`
}

type apiEngineeringStateDTO struct {
	CurrentRevision       apiCurrentRevisionDTO              `json:"current_revision"`
	EffectiveRequirements []apiEffectiveRequirementDTO       `json:"effective_requirements"`
	RequirementHistory    []apiRequirementRevisionHistoryDTO `json:"requirement_history"`
	ApplicableDecisions   []apiApplicableDecisionDTO         `json:"applicable_decisions"`
	ValidationPlan        apiValidationPlanDTO               `json:"validation_plan"`
	Readiness             apiReadinessResultDTO              `json:"readiness"`
	Lifecycle             apiLifecycleStateDTO               `json:"lifecycle"`
}

type apiEngineeringStateRationaleDTO struct {
	CurrentRevision apiResolutionRationaleDTO `json:"current_revision"`
	Lifecycle       apiLifecycleRationaleDTO  `json:"lifecycle"`
}

type apiFeatureCardDTO struct {
	FeatureCardID        string    `json:"feature_card_id"`
	ProjectID            string    `json:"project_id"`
	Title                string    `json:"title"`
	Description          string    `json:"description"`
	CreatedAt            time.Time `json:"created_at"`
	CapabilityArtifactID string    `json:"capability_artifact_id"`
}

type apiTimelineEventDTO struct {
	Kind           string     `json:"kind"`
	OccurredAt     *time.Time `json:"occurred_at"`
	Actor          string     `json:"actor"`
	Label          string     `json:"label"`
	Summary        string     `json:"summary"`
	SourceIdentity string     `json:"source_identity"`
	References     []string   `json:"references"`
	Corrected      string     `json:"corrected"`
	Rationale      string     `json:"rationale"`
}

// The proposal DTOs below are UI-owned render/delegation shapes for FF-024.
// They deliberately contain only JSON primitives and other UI DTOs: the UI
// does not import proposal, application, engineering, or PEOS values.

type apiProposalRevisionKeyDTO struct {
	ArtifactID string `json:"artifact_id"`
	RevisionID string `json:"revision_id"`
}

type apiProposalRecordKeyDTO struct {
	Kind     string `json:"kind"`
	RecordID string `json:"record_id"`
}

type apiProposalCapabilityDTO struct {
	Revision      apiProposalRevisionKeyDTO `json:"revision"`
	Sequence      int                       `json:"sequence"`
	Content       apiContentDTO             `json:"content"`
	ContentDigest string                    `json:"content_digest"`
}

type apiProposalRequirementDTO struct {
	Revision                 apiProposalRevisionKeyDTO `json:"revision"`
	Sequence                 int                       `json:"sequence"`
	Statement                string                    `json:"statement"`
	SourceCapabilityRevision apiProposalRevisionKeyDTO `json:"source_capability_revision"`
	SourceCriterionKey       string                    `json:"source_criterion_key"`
}

type apiProposalClaimDTO struct {
	Record              apiProposalRecordKeyDTO     `json:"record"`
	RequirementRevision apiProposalRevisionKeyDTO   `json:"requirement_revision"`
	CapabilityRevision  apiProposalRevisionKeyDTO   `json:"capability_revision"`
	ScopeArtifactID     string                      `json:"scope_artifact_id"`
	CriterionKeys       []string                    `json:"criterion_keys"`
	Outcome             string                      `json:"outcome"`
	Reasoning           string                      `json:"reasoning"`
	ExecutionReferences []apiProposalRecordKeyDTO   `json:"execution_references"`
	EvidenceReferences  []apiProposalRevisionKeyDTO `json:"evidence_references"`
}

type apiProposalDecisionSubjectDTO struct {
	Kind       string `json:"kind"`
	ArtifactID string `json:"artifact_id"`
	RevisionID string `json:"revision_id"`
}

type apiProposalDecisionDTO struct {
	DecisionID       string                        `json:"decision_id"`
	Subject          apiProposalDecisionSubjectDTO `json:"subject"`
	OutcomeStatement string                        `json:"outcome_statement"`
}

type apiProposalOpenQuestionDTO struct {
	CapabilityRevision apiProposalRevisionKeyDTO `json:"capability_revision"`
	Ordinal            int                       `json:"ordinal"`
	Text               string                    `json:"text"`
}

type apiProposalUncoveredCriterionDTO struct {
	CapabilityRevision   apiProposalRevisionKeyDTO   `json:"capability_revision"`
	CriterionKey         string                      `json:"criterion_key"`
	CriterionText        string                      `json:"criterion_text"`
	Reason               string                      `json:"reason"`
	RequirementRevisions []apiProposalRevisionKeyDTO `json:"requirement_revisions"`
}

type apiProposalFindingDTO struct {
	CapabilityRevision  apiProposalRevisionKeyDTO `json:"capability_revision"`
	CriterionKey        string                    `json:"criterion_key"`
	RequirementRevision apiProposalRevisionKeyDTO `json:"requirement_revision"`
	ClaimRecord         apiProposalRecordKeyDTO   `json:"claim_record"`
	Outcome             string                    `json:"outcome"`
	Reasoning           string                    `json:"reasoning"`
}

type apiProposalContextPackDTO struct {
	Capability        apiProposalCapabilityDTO           `json:"capability"`
	Requirements      []apiProposalRequirementDTO        `json:"requirements"`
	Claims            []apiProposalClaimDTO              `json:"claims"`
	Decisions         []apiProposalDecisionDTO           `json:"decisions"`
	OpenQuestions     []apiProposalOpenQuestionDTO       `json:"open_questions"`
	UncoveredCriteria []apiProposalUncoveredCriterionDTO `json:"uncovered_criteria"`
	Findings          []apiProposalFindingDTO            `json:"findings"`
	Sources           []string                           `json:"sources"`
	ContextDigest     string                             `json:"context_digest"`
}

type apiCapabilityProposalDTO struct {
	Content        apiContentDTO `json:"content"`
	Rationale      string        `json:"rationale"`
	Sources        []string      `json:"sources"`
	ContextDigest  string        `json:"context_digest"`
	ProposalDigest string        `json:"proposal_digest"`
}

type apiGenerateCapabilityProposalDTO struct {
	ContextPack json.RawMessage `json:"context_pack"`
	Proposal    json.RawMessage `json:"proposal"`
}

type apiAcceptCapabilityProposalDTO struct {
	RevisionID string          `json:"revision_id"`
	Proposal   json.RawMessage `json:"proposal"`
}
