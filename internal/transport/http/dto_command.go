package http

import (
	"time"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

// The request DTOs below mirror their command struct field-for-field
// (FF-018 §3.1, §7): no transport type aliases or embeds an application,
// domain, or engineering type. For C4, C5, C6 the {artifactID} path value
// is authoritative and the DTO omits an artifact ID field entirely
// (FF-018 §3.1 "Path/body precedence").

// --- C1 CreateProject ---

type createProjectRequest struct {
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
}

type createProjectResponse struct {
	ProjectID string `json:"project_id"`
}

// --- C2 CreateFeature ---

type createFeatureRequest struct {
	FeatureCardID string `json:"feature_card_id"`
	ProjectID     string `json:"project_id"`
	Title         string `json:"title"`
	Description   string `json:"description"`
}

type createFeatureResponse struct {
	FeatureCardID string `json:"feature_card_id"`
}

// --- capability content, shared by C3 and C4 ---

type acceptanceCriterionDTO struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

type contentDTO struct {
	SchemaVersion        int                      `json:"schema_version"`
	Title                string                   `json:"title"`
	ProblemStatement     string                   `json:"problem_statement"`
	UserOutcome          string                   `json:"user_outcome"`
	FunctionalBehaviours []string                 `json:"functional_behaviours"`
	Constraints          []string                 `json:"constraints"`
	AcceptanceCriteria   []acceptanceCriterionDTO `json:"acceptance_criteria"`
	Dependencies         []string                 `json:"dependencies"`
	OpenQuestions        []string                 `json:"open_questions"`
}

// mapContentDTO calls engineering.CapabilitySpecificationContent's builder
// chain in order, returning the first builder error (FF-018 §3.1): the
// builders carry validation the transport must not duplicate (§9.1), so
// this function performs no validation of its own beyond the mapping
// itself.
func mapContentDTO(dto contentDTO) (engineering.CapabilitySpecificationContent, error) {
	c, err := engineering.NewCapabilitySpecificationContent(dto.SchemaVersion, dto.Title, dto.ProblemStatement)
	if err != nil {
		return engineering.CapabilitySpecificationContent{}, err
	}
	if c, err = c.WithUserOutcome(dto.UserOutcome); err != nil {
		return engineering.CapabilitySpecificationContent{}, err
	}
	if c, err = c.WithFunctionalBehaviours(dto.FunctionalBehaviours); err != nil {
		return engineering.CapabilitySpecificationContent{}, err
	}
	if c, err = c.WithConstraints(dto.Constraints); err != nil {
		return engineering.CapabilitySpecificationContent{}, err
	}
	criteria := make([]engineering.AcceptanceCriterion, len(dto.AcceptanceCriteria))
	for i, ac := range dto.AcceptanceCriteria {
		crit, err := engineering.NewAcceptanceCriterion(ac.Key, ac.Text)
		if err != nil {
			return engineering.CapabilitySpecificationContent{}, err
		}
		criteria[i] = crit
	}
	if c, err = c.WithAcceptanceCriteria(criteria); err != nil {
		return engineering.CapabilitySpecificationContent{}, err
	}
	if c, err = c.WithDependencies(dto.Dependencies); err != nil {
		return engineering.CapabilitySpecificationContent{}, err
	}
	if c, err = c.WithOpenQuestions(dto.OpenQuestions); err != nil {
		return engineering.CapabilitySpecificationContent{}, err
	}
	return c, nil
}

// --- C3 EstablishCapabilitySpecification ---

type establishCapabilityRequest struct {
	FeatureCardID string     `json:"feature_card_id"`
	ArtifactID    string     `json:"artifact_id"`
	RevisionID    string     `json:"revision_id"`
	Content       contentDTO `json:"content"`
}

type establishCapabilityResponse struct {
	ArtifactID string `json:"artifact_id"`
	RevisionID string `json:"revision_id"`
	Sequence   int    `json:"sequence"`
}

// --- C4 ReviseCapabilitySpecification ---
// path: artifactID

type reviseCapabilityRequest struct {
	RevisionID string     `json:"revision_id"`
	Content    contentDTO `json:"content"`
}

type reviseCapabilityResponse struct {
	RevisionID string `json:"revision_id"`
	Sequence   int    `json:"sequence"`
}

// --- C5 AcceptCapabilityRevision ---
// path: artifactID

type acceptRevisionRequest struct {
	RecordID    string    `json:"record_id"`
	RevisionID  string    `json:"revision_id"`
	State       string    `json:"state"`
	Reason      string    `json:"reason"`
	EffectiveAt time.Time `json:"effective_at"`
}

type acceptRevisionResponse struct {
	RecordID string `json:"record_id"`
}

// --- C6 AssignLifecycleState ---
// path: artifactID (mapped to SubjectArtifactID)

type assignLifecycleRequest struct {
	AssignmentID               string    `json:"assignment_id"`
	State                      string    `json:"state"`
	EffectiveAt                time.Time `json:"effective_at"`
	TransitionRecordArtifactID string    `json:"transition_record_artifact_id"`
	TransitionRecordRevisionID string    `json:"transition_record_revision_id"`
	IsEntry                    bool      `json:"is_entry"`
	TransitionKey              string    `json:"transition_key"`
	FromAssignmentID           string    `json:"from_assignment_id"`
	AttemptedAt                time.Time `json:"attempted_at"`
	CompletedAt                time.Time `json:"completed_at"`
}

type assignLifecycleResponse struct {
	TransitionRevisionKey string `json:"transition_revision_key"`
	AssignmentKey         string `json:"assignment_key"`
}

// --- C7 EstablishRequirement ---

type establishRequirementRequest struct {
	ArtifactID        string `json:"artifact_id"`
	RevisionID        string `json:"revision_id"`
	Statement         string `json:"statement"`
	SubjectArtifactID string `json:"subject_artifact_id"`
}

type establishRequirementResponse struct {
	ArtifactID string `json:"artifact_id"`
	RevisionID string `json:"revision_id"`
}

// --- C8 RecordArchitectureDecision ---

type recordDecisionRequest struct {
	DecisionID         string   `json:"decision_id"`
	SubjectArtifactID  string   `json:"subject_artifact_id"`
	SubjectRevisionID  string   `json:"subject_revision_id"`
	Question           string   `json:"question"`
	OutcomeStatement   string   `json:"outcome_statement"`
	Alternatives       []string `json:"alternatives"`
	EvidenceArtifactID string   `json:"evidence_artifact_id"`
	EvidenceRevisionID string   `json:"evidence_revision_id"`
	Assumptions        []string `json:"assumptions"`
	Constraints        []string `json:"constraints"`
	Uncertainties      []string `json:"uncertainties"`
	Rationale          string   `json:"rationale"`
}

type recordDecisionResponse struct {
	DecisionKey string `json:"decision_key"`
}

// --- C9 EstablishValidationPlan ---

type planActivityDTO struct {
	Key                   string   `json:"key"`
	SubjectArtifactID     string   `json:"subject_artifact_id"`
	SubjectRevisionID     string   `json:"subject_revision_id"`
	Method                string   `json:"method"`
	OutcomeInterpretation string   `json:"outcome_interpretation"`
	RequirementArtifactID string   `json:"requirement_artifact_id"`
	RequirementRevisionID string   `json:"requirement_revision_id"`
	ExpectedEvidence      []string `json:"expected_evidence"`
}

type establishPlanRequest struct {
	ArtifactID      string            `json:"artifact_id"`
	RevisionID      string            `json:"revision_id"`
	ScopeArtifactID string            `json:"scope_artifact_id"`
	Activities      []planActivityDTO `json:"activities"`
}

type establishPlanResponse struct {
	ArtifactID string `json:"artifact_id"`
	RevisionID string `json:"revision_id"`
}

func mapPlanActivities(dtos []planActivityDTO) []application.PlanActivityCommandInput {
	out := make([]application.PlanActivityCommandInput, len(dtos))
	for i, a := range dtos {
		out[i] = application.PlanActivityCommandInput{
			Key: a.Key, SubjectArtifactID: a.SubjectArtifactID, SubjectRevisionID: a.SubjectRevisionID,
			Method: a.Method, OutcomeInterpretation: a.OutcomeInterpretation,
			RequirementArtifactID: a.RequirementArtifactID, RequirementRevisionID: a.RequirementRevisionID,
			ExpectedEvidence: a.ExpectedEvidence,
		}
	}
	return out
}

// --- C10 RecordValidationRun ---

type recordRunRequest struct {
	ExecutionID        string    `json:"execution_id"`
	PlanArtifactID     string    `json:"plan_artifact_id"`
	PlanRevisionID     string    `json:"plan_revision_id"`
	ActivityKey        string    `json:"activity_key"`
	SubjectArtifactID  string    `json:"subject_artifact_id"`
	SubjectRevisionID  string    `json:"subject_revision_id"`
	Method             string    `json:"method"`
	Outcome            string    `json:"outcome"`
	CompletedAt        time.Time `json:"completed_at"`
	EvidenceArtifactID string    `json:"evidence_artifact_id"`
	EvidenceRevisionID string    `json:"evidence_revision_id"`
	EvidenceLocator    string    `json:"evidence_locator"`
}

type recordRunResponse struct {
	EvidenceArtifactID string `json:"evidence_artifact_id"`
	EvidenceRevisionID string `json:"evidence_revision_id"`
	ExecutionKey       string `json:"execution_key"`
}

// --- C11 RecordValidationClaim ---

type recordClaimRequest struct {
	ClaimID               string    `json:"claim_id"`
	ScopeArtifactID       string    `json:"scope_artifact_id"`
	SubjectArtifactID     string    `json:"subject_artifact_id"`
	SubjectRevisionID     string    `json:"subject_revision_id"`
	RequirementArtifactID string    `json:"requirement_artifact_id"`
	RequirementRevisionID string    `json:"requirement_revision_id"`
	Outcome               string    `json:"outcome"`
	Method                string    `json:"method"`
	EvidenceArtifactID    string    `json:"evidence_artifact_id"`
	EvidenceRevisionID    string    `json:"evidence_revision_id"`
	ExecutionID           string    `json:"execution_id"`
	Reasoning             string    `json:"reasoning"`
	Timestamp             time.Time `json:"timestamp"`
}

type recordClaimResponse struct {
	ClaimKey string `json:"claim_key"`
}

// --- C12 CorrectValidationClaim ---

type correctClaimRequest struct {
	ClaimID               string    `json:"claim_id"`
	CorrectionTarget      string    `json:"correction_target"`
	CorrectionKind        string    `json:"correction_kind"`
	ScopeArtifactID       string    `json:"scope_artifact_id"`
	SubjectArtifactID     string    `json:"subject_artifact_id"`
	SubjectRevisionID     string    `json:"subject_revision_id"`
	RequirementArtifactID string    `json:"requirement_artifact_id"`
	RequirementRevisionID string    `json:"requirement_revision_id"`
	Outcome               string    `json:"outcome"`
	Method                string    `json:"method"`
	EvidenceArtifactID    string    `json:"evidence_artifact_id"`
	EvidenceRevisionID    string    `json:"evidence_revision_id"`
	ExecutionID           string    `json:"execution_id"`
	Reasoning             string    `json:"reasoning"`
	Timestamp             time.Time `json:"timestamp"`
}

type correctClaimResponse struct {
	ClaimKey string `json:"claim_key"`
}
