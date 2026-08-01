package http

import (
	"time"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/domain"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

// The DTOs below mirror application result and domain types field-for-field
// (FF-018 §10.2): no transport type aliases or embeds a domain,
// application, or engineering type.

// projectDTO mirrors domain.Project (Q1, Q2).
type projectDTO struct {
	ProjectID string    `json:"project_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

func mapProjectDTO(p domain.Project) projectDTO {
	return projectDTO{ProjectID: p.ID().String(), Name: p.Name(), CreatedAt: p.CreatedAt()}
}

// listProjectsResponse is Q1's data payload (FF-018 §10.3). No rationale:
// a list of stored values is not derived.
type listProjectsResponse struct {
	Projects []projectDTO `json:"projects"`
}

// featureCardDTO mirrors domain.FeatureCard (Q2, Q3).
// CapabilityArtifactID is omitted when the card has no linked capability
// (FF-018 §10.3).
type featureCardDTO struct {
	FeatureCardID        string    `json:"feature_card_id"`
	ProjectID            string    `json:"project_id"`
	Title                string    `json:"title"`
	Description          string    `json:"description"`
	CreatedAt            time.Time `json:"created_at"`
	CapabilityArtifactID string    `json:"capability_artifact_id,omitempty"`
}

func mapFeatureCardDTO(c domain.FeatureCard) featureCardDTO {
	artifactID, _ := c.CapabilityArtifactID()
	return featureCardDTO{
		FeatureCardID: c.ID().String(), ProjectID: c.ProjectID().String(),
		Title: c.Title(), Description: c.Description(), CreatedAt: c.CreatedAt(),
		CapabilityArtifactID: artifactID,
	}
}

// listFeaturesResponse is Q2's data payload (FF-018 §10.3). No rationale.
type listFeaturesResponse struct {
	Features []featureCardDTO `json:"features"`
}

// revisionDTO mirrors engineering.RevisionEnvelope's projected fields
// (Q4, Q6, Q7). Sequence is set only where order metadata was resolved
// alongside the revision (Q6's current, and Q4's current_revision); zero
// otherwise, matching the "omitted when zero" convention that already
// applies to ContentDigest. Content is set only by mapRevisionWithContentDTO
// (Q6, Q7); mapRevisionDTO (Q4's current_revision) leaves it nil, since
// application.CurrentRevisionResult carries no content lookup -- FF-020 §5
// scopes full specification content to Q6/Q7 only (FF-020 §2 class A).
type revisionDTO struct {
	ArtifactID           string      `json:"artifact_id"`
	RevisionID           string      `json:"revision_id"`
	Sequence             int         `json:"sequence,omitempty"`
	RevisionFamily       string      `json:"revision_family"`
	ArtifactType         string      `json:"artifact_type"`
	IntegrityValue       string      `json:"integrity_value"`
	ContentDigest        string      `json:"content_digest,omitempty"`
	SubjectKey           string      `json:"subject_key,omitempty"`
	RecordedAt           time.Time   `json:"recorded_at"`
	ProvenanceActor      string      `json:"provenance_actor,omitempty"`
	ProvenanceRecordedAt *time.Time  `json:"provenance_recorded_at,omitempty"`
	Content              *contentDTO `json:"content,omitempty"`
}

func mapRevisionDTO(env engineering.RevisionEnvelope, sequence int) revisionDTO {
	dto := revisionDTO{
		ArtifactID: env.Key.ArtifactID, RevisionID: env.Key.RevisionID, Sequence: sequence,
		RevisionFamily: string(env.RevisionFamily), ArtifactType: env.ArtifactType,
		IntegrityValue: env.IntegrityValue, SubjectKey: env.SubjectKey, RecordedAt: env.RecordedAt,
	}
	if !env.ContentDigest.IsZero() {
		dto.ContentDigest = env.ContentDigest.String()
	}
	if env.HasProvenanceActor {
		dto.ProvenanceActor = env.ProvenanceActor
	}
	if env.HasProvenanceTime {
		t := env.ProvenanceRecordedAt
		dto.ProvenanceRecordedAt = &t
	}
	return dto
}

// mapContentDTOFromContent is the read-side inverse of dto_command.go's
// mapContentDTO: it projects a stored engineering.CapabilitySpecificationContent
// (FF-020 §2 class A) into the same contentDTO shape C3/C4 accept, field
// for field, using its accessor set (never invented, per §10.2).
func mapContentDTOFromContent(c engineering.CapabilitySpecificationContent) contentDTO {
	criteria := c.AcceptanceCriteria()
	dto := contentDTO{
		SchemaVersion:        c.SchemaVersion(),
		Title:                c.Title(),
		ProblemStatement:     c.ProblemStatement(),
		UserOutcome:          c.UserOutcome(),
		FunctionalBehaviours: append([]string{}, c.FunctionalBehaviours()...),
		Constraints:          append([]string{}, c.Constraints()...),
		AcceptanceCriteria:   make([]acceptanceCriterionDTO, 0, len(criteria)),
		Dependencies:         append([]string{}, c.Dependencies()...),
		OpenQuestions:        append([]string{}, c.OpenQuestions()...),
	}
	for _, ac := range criteria {
		dto.AcceptanceCriteria = append(dto.AcceptanceCriteria, acceptanceCriterionDTO{Key: ac.Key(), Text: ac.Text()})
	}
	return dto
}

// mapRevisionWithContentDTO is mapRevisionDTO plus the revision's
// structured content (FF-020 §2 class A, FF-001 §3.3: "full specification
// content per revision"), used by Q6 and Q7 only. hasContent false leaves
// Content nil -- an empty state, not an invented empty object.
func mapRevisionWithContentDTO(env engineering.RevisionEnvelope, content engineering.CapabilitySpecificationContent, hasContent bool, sequence int) revisionDTO {
	dto := mapRevisionDTO(env, sequence)
	if hasContent {
		c := mapContentDTOFromContent(content)
		dto.Content = &c
	}
	return dto
}

// currentRevisionDTO wraps revisionDTO with the Found flag
// application.CurrentRevisionResult carries (Q4, Q6).
type currentRevisionDTO struct {
	Found    bool         `json:"found"`
	Revision *revisionDTO `json:"revision,omitempty"`
}

func mapCurrentRevisionDTO(r application.CurrentRevisionResult) currentRevisionDTO {
	if !r.Found {
		return currentRevisionDTO{Found: false}
	}
	dto := mapRevisionDTO(r.Revision, r.Sequence)
	return currentRevisionDTO{Found: true, Revision: &dto}
}

// consideredRevisionDTO and rejectedRevisionDTO mirror
// application.ConsideredRevision / application.RejectedRevision.
type consideredRevisionDTO struct {
	ArtifactID      string `json:"artifact_id"`
	RevisionID      string `json:"revision_id"`
	Sequence        int    `json:"sequence"`
	AcceptanceState string `json:"acceptance_state"`
}

type rejectedRevisionDTO struct {
	ArtifactID string `json:"artifact_id"`
	RevisionID string `json:"revision_id"`
	Reason     string `json:"reason"`
}

// resolutionRationaleDTO mirrors application.ResolutionRationale.
type resolutionRationaleDTO struct {
	Rule               string                  `json:"rule"`
	SelectedArtifactID string                  `json:"selected_artifact_id,omitempty"`
	SelectedRevisionID string                  `json:"selected_revision_id,omitempty"`
	SelectedSequence   int                     `json:"selected_sequence,omitempty"`
	Considered         []consideredRevisionDTO `json:"considered"`
	Rejected           []rejectedRevisionDTO   `json:"rejected"`
	Warnings           []string                `json:"warnings"`
}

func mapResolutionRationaleDTO(r application.ResolutionRationale) resolutionRationaleDTO {
	dto := resolutionRationaleDTO{
		Rule:             r.Rule,
		SelectedSequence: r.SelectedSequence,
		Considered:       make([]consideredRevisionDTO, 0, len(r.Considered)),
		Rejected:         make([]rejectedRevisionDTO, 0, len(r.Rejected)),
		Warnings:         append([]string{}, r.Warnings...),
	}
	if !r.SelectedKey.IsZero() {
		dto.SelectedArtifactID = r.SelectedKey.ArtifactID
		dto.SelectedRevisionID = r.SelectedKey.RevisionID
	}
	for _, c := range r.Considered {
		dto.Considered = append(dto.Considered, consideredRevisionDTO{
			ArtifactID: c.Key.ArtifactID, RevisionID: c.Key.RevisionID,
			Sequence: c.Sequence, AcceptanceState: string(c.AcceptanceState),
		})
	}
	for _, rj := range r.Rejected {
		dto.Rejected = append(dto.Rejected, rejectedRevisionDTO{
			ArtifactID: rj.Key.ArtifactID, RevisionID: rj.Key.RevisionID, Reason: rj.Reason,
		})
	}
	return dto
}

// effectiveRequirementDTO mirrors application.EffectiveRequirement.
// Statement is decoded from the requirement revision's stored payload
// (FF-020 §5, FF-001 §3.4).
type effectiveRequirementDTO struct {
	ArtifactID string `json:"artifact_id"`
	RevisionID string `json:"revision_id"`
	Sequence   int    `json:"sequence"`
	Statement  string `json:"statement"`
}

// decisionBasisDTO mirrors a decision's full basis (FF-020 §5, FF-001
// §3.5: "the basis is displayed, not collapsed"). Evidence is the
// existing RecordEnvelope.EvidenceKeys projection, not a decode; every
// other field is engineering.DecisionDetail, decoded from Payload.
type decisionBasisDTO struct {
	Evidence      []string `json:"evidence"`
	Assumptions   []string `json:"assumptions"`
	Constraints   []string `json:"constraints"`
	Uncertainties []string `json:"uncertainties"`
}

// applicableDecisionDTO mirrors application.ApplicableDecision, projecting
// the cited engineering.RecordEnvelope's fields directly, plus its full
// basis (FF-020 §5).
type applicableDecisionDTO struct {
	DecisionID       string           `json:"decision_id"`
	SubjectKey       string           `json:"subject_key"`
	Scope            string           `json:"scope"`
	OccurredAt       *time.Time       `json:"occurred_at,omitempty"`
	Outcome          string           `json:"outcome"`
	Question         string           `json:"question,omitempty"`
	OutcomeStatement string           `json:"outcome_statement"`
	Rationale        string           `json:"rationale,omitempty"`
	Alternatives     []string         `json:"alternatives"`
	Basis            decisionBasisDTO `json:"basis"`
}

func mapApplicableDecisionDTO(d application.ApplicableDecision) applicableDecisionDTO {
	dto := applicableDecisionDTO{
		DecisionID: d.DecisionID, SubjectKey: d.Decision.SubjectKey, Scope: d.Decision.Scope, Outcome: d.Decision.Outcome,
		Question: d.Detail.Question, OutcomeStatement: d.Detail.OutcomeStatement, Rationale: d.Detail.Rationale,
		Alternatives: append([]string{}, d.Detail.Alternatives...),
		Basis: decisionBasisDTO{
			Evidence:      append([]string{}, d.Decision.EvidenceKeys...),
			Assumptions:   append([]string{}, d.Detail.Assumptions...),
			Constraints:   append([]string{}, d.Detail.Constraints...),
			Uncertainties: append([]string{}, d.Detail.Uncertainties...),
		},
	}
	if d.Decision.HasOccurredAt {
		t := d.Decision.OccurredAt
		dto.OccurredAt = &t
	}
	return dto
}

// rejectedClaimDTO mirrors application.RejectedClaim (FF-020 §5, FF-001
// §3.6: "superseded claims shown inline ... with their outcome intact and
// a link to the claim that corrected them" -- "shown, not hidden").
type rejectedClaimDTO struct {
	RecordKey   string `json:"record_key"`
	Reason      string `json:"reason"`
	Outcome     string `json:"outcome"`
	Reasoning   string `json:"reasoning,omitempty"`
	CorrectedBy string `json:"corrected_by,omitempty"`
}

// perRequirementReadinessDTO mirrors application.PerRequirementReadiness.
// Reasoning and CriterionKeys are the current claim's own detail (FF-020
// §5); Corrects is the existing RecordEnvelope.CorrectionTargetID
// projection, present when the current claim itself corrects an earlier
// one (e.g. FF-011's CLM-4).
type perRequirementReadinessDTO struct {
	RequirementArtifactID string             `json:"requirement_artifact_id"`
	RequirementRevisionID string             `json:"requirement_revision_id"`
	HasClaim              bool               `json:"has_claim"`
	ClaimID               string             `json:"claim_id,omitempty"`
	Outcome               string             `json:"outcome,omitempty"`
	Reasoning             string             `json:"reasoning,omitempty"`
	CriterionKeys         []string           `json:"criterion_keys,omitempty"`
	Corrects              string             `json:"corrects,omitempty"`
	ExecutionOutcome      string             `json:"execution_outcome,omitempty"`
	Stale                 bool               `json:"stale"`
	StaleSequence         int                `json:"stale_sequence,omitempty"`
	VerdictReason         string             `json:"verdict_reason"`
	Rejected              []rejectedClaimDTO `json:"rejected"`
}

func mapPerRequirementReadinessDTO(p application.PerRequirementReadiness) perRequirementReadinessDTO {
	dto := perRequirementReadinessDTO{
		RequirementArtifactID: p.RequirementArtifactID,
		RequirementRevisionID: p.RequirementRevisionKey.RevisionID,
		HasClaim:              p.HasClaim,
		Outcome:               p.Outcome,
		Reasoning:             p.Reasoning,
		ExecutionOutcome:      p.ExecutionOutcome,
		Stale:                 p.Stale,
		StaleSequence:         p.StaleSequence,
		VerdictReason:         p.VerdictReason,
		Rejected:              make([]rejectedClaimDTO, 0, len(p.Rejected)),
	}
	if p.HasClaim {
		dto.ClaimID = p.Claim.Key.ID
		dto.CriterionKeys = append([]string{}, p.Claim.CriterionKeys...)
		if p.Claim.HasCorrection() {
			dto.Corrects = p.Claim.CorrectionTargetID
		}
	}
	for _, rj := range p.Rejected {
		dto.Rejected = append(dto.Rejected, rejectedClaimDTO{
			RecordKey: rj.Key.String(), Reason: rj.Reason, Outcome: rj.Outcome,
			Reasoning: rj.Reasoning, CorrectedBy: rj.CorrectedBy,
		})
	}
	return dto
}

// readinessResultDTO mirrors application.ReadinessResult.
type readinessResultDTO struct {
	Status         string                       `json:"status"`
	PerRequirement []perRequirementReadinessDTO `json:"per_requirement"`
}

func mapReadinessResultDTO(r application.ReadinessResult) readinessResultDTO {
	dto := readinessResultDTO{Status: string(r.Status), PerRequirement: make([]perRequirementReadinessDTO, 0, len(r.PerRequirement))}
	for _, p := range r.PerRequirement {
		dto.PerRequirement = append(dto.PerRequirement, mapPerRequirementReadinessDTO(p))
	}
	return dto
}

// lifecycleRationaleDTO mirrors application.LifecycleRationale.
type lifecycleRationaleDTO struct {
	Rule      string `json:"rule"`
	Total     int    `json:"total"`
	Duplicate bool   `json:"duplicate"`
}

// lifecycleStateDTO mirrors application.LifecycleStateResult, projecting
// the cited engineering.RecordEnvelope's fields directly.
type lifecycleStateDTO struct {
	Found        bool       `json:"found"`
	StateID      string     `json:"state_id,omitempty"`
	AssignmentID string     `json:"assignment_id,omitempty"`
	SubjectKey   string     `json:"subject_key,omitempty"`
	OccurredAt   *time.Time `json:"occurred_at,omitempty"`
}

func mapLifecycleStateDTO(l application.LifecycleStateResult) lifecycleStateDTO {
	if !l.Found {
		return lifecycleStateDTO{Found: false}
	}
	dto := lifecycleStateDTO{
		Found: true, StateID: l.Assignment.StateID, AssignmentID: l.Assignment.Key.ID, SubjectKey: l.Assignment.SubjectKey,
	}
	if l.Assignment.HasOccurredAt {
		t := l.Assignment.OccurredAt
		dto.OccurredAt = &t
	}
	return dto
}

// planActivityDetailDTO mirrors engineering.PlanActivityDetail (FF-020 §5,
// FF-001 §3.6: "plan revision and its activities"). Named distinctly from
// dto_command.go's planActivityDTO (a C9 request field, different shape)
// to keep write and read DTOs unambiguous.
type planActivityDetailDTO struct {
	Key                   string   `json:"key"`
	Method                string   `json:"method"`
	OutcomeInterpretation string   `json:"outcome_interpretation"`
	ExpectedEvidence      []string `json:"expected_evidence"`
}

// validationPlanDTO mirrors application.ValidationPlanResult. Found false
// means the capability has no applicable plan yet -- an empty state, not
// an error.
type validationPlanDTO struct {
	Found      bool                    `json:"found"`
	ArtifactID string                  `json:"artifact_id,omitempty"`
	RevisionID string                  `json:"revision_id,omitempty"`
	Activities []planActivityDetailDTO `json:"activities"`
}

func mapValidationPlanDTO(v application.ValidationPlanResult) validationPlanDTO {
	dto := validationPlanDTO{Found: v.Found, Activities: make([]planActivityDetailDTO, 0, len(v.Activities))}
	if !v.Found {
		return dto
	}
	dto.ArtifactID, dto.RevisionID = v.ArtifactID, v.RevisionID
	for _, a := range v.Activities {
		dto.Activities = append(dto.Activities, planActivityDetailDTO{
			Key: a.Key, Method: a.Method, OutcomeInterpretation: a.OutcomeInterpretation,
			ExpectedEvidence: append([]string{}, a.ExpectedEvidence...),
		})
	}
	return dto
}

// engineeringStateDTO is Q4's data payload, and the "state" sub-object of
// Q3's (FF-018 §10.3).
type engineeringStateDTO struct {
	CurrentRevision       currentRevisionDTO        `json:"current_revision"`
	EffectiveRequirements []effectiveRequirementDTO `json:"effective_requirements"`
	ApplicableDecisions   []applicableDecisionDTO   `json:"applicable_decisions"`
	ValidationPlan        validationPlanDTO         `json:"validation_plan"`
	Readiness             readinessResultDTO        `json:"readiness"`
	Lifecycle             lifecycleStateDTO         `json:"lifecycle"`
}

// engineeringStateRationaleDTO is Q4's rationale payload. There is
// deliberately no "readiness" entry: application.ReadinessResult carries no
// Rule-style rationale field the way ResolutionRationale and
// LifecycleRationale do -- every other rationale in this codebase is an
// application-owned algorithm description (see e.g. query_lifecycle.go's
// literal Rule strings), never a transport-synthesized one, and inventing
// one here would violate "no derived field is invented" (FF-018 §10.2).
// The per-requirement verdict_reason values inside data already carry the
// readiness explanation FF-011 requires.
type engineeringStateRationaleDTO struct {
	CurrentRevision resolutionRationaleDTO `json:"current_revision"`
	ValidationPlan  resolutionRationaleDTO `json:"validation_plan"`
	Lifecycle       lifecycleRationaleDTO  `json:"lifecycle"`
}

func mapEngineeringStateDTO(s application.EngineeringStateResult) (engineeringStateDTO, engineeringStateRationaleDTO) {
	data := engineeringStateDTO{
		CurrentRevision:       mapCurrentRevisionDTO(s.CurrentRevision),
		EffectiveRequirements: make([]effectiveRequirementDTO, 0, len(s.EffectiveRequirements)),
		ApplicableDecisions:   make([]applicableDecisionDTO, 0, len(s.ApplicableDecisions)),
		ValidationPlan:        mapValidationPlanDTO(s.ValidationPlan),
		Readiness:             mapReadinessResultDTO(s.Readiness),
		Lifecycle:             mapLifecycleStateDTO(s.Lifecycle),
	}
	for _, req := range s.EffectiveRequirements {
		data.EffectiveRequirements = append(data.EffectiveRequirements, effectiveRequirementDTO{
			ArtifactID: req.ArtifactID, RevisionID: req.RevisionKey.RevisionID, Sequence: req.Sequence, Statement: req.Statement,
		})
	}
	for _, dec := range s.ApplicableDecisions {
		data.ApplicableDecisions = append(data.ApplicableDecisions, mapApplicableDecisionDTO(dec))
	}
	rationale := engineeringStateRationaleDTO{
		CurrentRevision: mapResolutionRationaleDTO(s.CurrentRevision.Rationale),
		ValidationPlan:  mapResolutionRationaleDTO(s.ValidationPlan.Rationale),
		Lifecycle:       lifecycleRationaleDTO{Rule: s.Lifecycle.Rationale.Rule, Total: s.Lifecycle.Rationale.Total, Duplicate: s.Lifecycle.Rationale.Duplicate},
	}
	return data, rationale
}

// featureOverviewResponse is Q3's data payload (FF-018 §10.3): the feature
// card plus the whole of Q4's data under "state".
type featureOverviewResponse struct {
	Feature featureCardDTO      `json:"feature"`
	State   engineeringStateDTO `json:"state"`
}

// timelineEventDTO mirrors application.TimelineEvent. Rationale is
// rendered inside each event, not at the response's top level: the
// application result models rationale per event
// (TimelineEvent.Rationale), and TimelineResult itself carries no
// response-level rationale to hoist there (FF-018 §10.3, deliberate
// deviation from the uniform envelope, stated explicitly).
type timelineEventDTO struct {
	EventID        string     `json:"event_id"`
	FeatureCardID  string     `json:"feature_card_id,omitempty"`
	Kind           string     `json:"kind"`
	OccurredAt     *time.Time `json:"occurred_at,omitempty"`
	Actor          string     `json:"actor,omitempty"`
	Label          string     `json:"label"`
	Summary        string     `json:"summary"`
	SourceIdentity string     `json:"source_identity"`
	References     []string   `json:"references"`
	Corrected      string     `json:"corrected,omitempty"`
	Rationale      string     `json:"rationale"`
}

func mapTimelineEventDTO(e application.TimelineEvent) timelineEventDTO {
	dto := timelineEventDTO{
		EventID: e.EventID, Kind: string(e.Kind), Actor: e.Actor, Label: e.Label, Summary: e.Summary,
		SourceIdentity: e.SourceIdentity, References: append([]string{}, e.References...),
		Corrected: e.Corrected, Rationale: e.Rationale,
	}
	if !e.FeatureCardID.IsZero() {
		dto.FeatureCardID = e.FeatureCardID.String()
	}
	if e.HasOccurredAt {
		t := e.OccurredAt
		dto.OccurredAt = &t
	}
	return dto
}

// timelineResponse is Q5's data payload (FF-018 §10.3).
type timelineResponse struct {
	Dated   []timelineEventDTO `json:"dated"`
	Undated []timelineEventDTO `json:"undated"`
}

func mapTimelineResponse(r application.TimelineResult) timelineResponse {
	resp := timelineResponse{Dated: make([]timelineEventDTO, 0, len(r.Dated)), Undated: make([]timelineEventDTO, 0, len(r.Undated))}
	for _, e := range r.Dated {
		resp.Dated = append(resp.Dated, mapTimelineEventDTO(e))
	}
	for _, e := range r.Undated {
		resp.Undated = append(resp.Undated, mapTimelineEventDTO(e))
	}
	return resp
}

// capabilityRevisionsResponse is Q6's data payload (FF-018 §10.3).
type capabilityRevisionsResponse struct {
	Revisions []revisionDTO      `json:"revisions"`
	Current   currentRevisionDTO `json:"current"`
}
