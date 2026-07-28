package ui

import "time"

// The row types below are shared render-ready shapes, built by more than
// one screen's view model (FF-021 §6): a readiness row appears on both
// Feature overview and Validation; a timeline row on both Feature overview
// (last five) and Timeline (all). No field here is invented -- each is a
// direct rendering of a field already present in the corresponding api*DTO
// (FF-021 §6: "no field is fabricated").

// readinessRow is one requirement's readiness verdict (FF-001 §3.2, §3.4,
// §3.6: "the per-requirement rationale table", never a bare badge).
type readinessRow struct {
	RequirementArtifactID string
	HasClaim              bool
	ClaimID               string
	Outcome               string
	Reasoning             string
	CriterionKeys         []string
	Corrects              string
	ExecutionOutcome      string
	Stale                 bool
	VerdictReason         string
	Rejected              []rejectedClaimRow
}

// rejectedClaimRow is one superseded or invalidated claim, "shown, not
// hidden" (FF-001 §3.6), with its outcome intact and a link to whatever
// corrected it, when there was one.
type rejectedClaimRow struct {
	RecordKey   string
	Reason      string
	Outcome     string
	Reasoning   string
	CorrectedBy string
}

func mapReadinessRows(per []apiPerRequirementReadinessDTO) []readinessRow {
	rows := make([]readinessRow, 0, len(per))
	for _, p := range per {
		row := readinessRow{
			RequirementArtifactID: p.RequirementArtifactID, HasClaim: p.HasClaim, ClaimID: p.ClaimID,
			Outcome: p.Outcome, Reasoning: p.Reasoning, CriterionKeys: p.CriterionKeys, Corrects: p.Corrects,
			ExecutionOutcome: p.ExecutionOutcome, Stale: p.Stale, VerdictReason: p.VerdictReason,
			Rejected: make([]rejectedClaimRow, 0, len(p.Rejected)),
		}
		for _, rj := range p.Rejected {
			row.Rejected = append(row.Rejected, rejectedClaimRow{
				RecordKey: rj.RecordKey, Reason: rj.Reason, Outcome: rj.Outcome,
				Reasoning: rj.Reasoning, CorrectedBy: rj.CorrectedBy,
			})
		}
		rows = append(rows, row)
	}
	return rows
}

// timelineRow is one rendered timeline event (FF-001 §3.7: "label, actor,
// timestamp, references, detail, rationale").
type timelineRow struct {
	Kind           string
	HasOccurredAt  bool
	OccurredAt     time.Time
	Actor          string
	Label          string
	Summary        string
	SourceIdentity string
	References     []string
	Corrected      string
	Rationale      string
}

func mapTimelineRow(e apiTimelineEventDTO) timelineRow {
	row := timelineRow{
		Kind: e.Kind, Actor: e.Actor, Label: e.Label, Summary: e.Summary,
		SourceIdentity: e.SourceIdentity, References: e.References, Corrected: e.Corrected, Rationale: e.Rationale,
	}
	if e.OccurredAt != nil {
		row.HasOccurredAt = true
		row.OccurredAt = *e.OccurredAt
	}
	return row
}

func mapTimelineRows(events []apiTimelineEventDTO) []timelineRow {
	rows := make([]timelineRow, 0, len(events))
	for _, e := range events {
		rows = append(rows, mapTimelineRow(e))
	}
	return rows
}

// revisionRow is one capability revision (FF-001 §3.3: "sequence,
// acceptance state, recorded-at, actor; full specification content").
type revisionRow struct {
	ArtifactID           string
	RevisionID           string
	Sequence             int
	IsCurrent            bool
	AcceptanceState      string
	RecordedAt           time.Time
	ProvenanceActor      string
	HasContent           bool
	Title                string
	ProblemStatement     string
	UserOutcome          string
	FunctionalBehaviours []string
	Constraints          []string
	AcceptanceCriteria   []apiAcceptanceCriterionDTO
	Dependencies         []string
	OpenQuestions        []string
}

func mapRevisionRow(r apiRevisionDTO, isCurrent bool) revisionRow {
	row := revisionRow{
		ArtifactID: r.ArtifactID, RevisionID: r.RevisionID, Sequence: r.Sequence, IsCurrent: isCurrent,
		RecordedAt: r.RecordedAt, ProvenanceActor: r.ProvenanceActor,
	}
	if r.Content != nil {
		row.HasContent = true
		row.Title = r.Content.Title
		row.ProblemStatement = r.Content.ProblemStatement
		row.UserOutcome = r.Content.UserOutcome
		row.FunctionalBehaviours = r.Content.FunctionalBehaviours
		row.Constraints = r.Content.Constraints
		row.AcceptanceCriteria = r.Content.AcceptanceCriteria
		row.Dependencies = r.Content.Dependencies
		row.OpenQuestions = r.Content.OpenQuestions
	}
	return row
}

// decisionRow is one decision with its full basis (FF-001 §3.5: "the
// basis is displayed, not collapsed").
type decisionRow struct {
	DecisionID       string
	HasOccurredAt    bool
	OccurredAt       time.Time
	Outcome          string
	Question         string
	OutcomeStatement string
	Rationale        string
	Alternatives     []string
	Evidence         []string
	Assumptions      []string
	Constraints      []string
	Uncertainties    []string
}

func mapDecisionRow(d apiApplicableDecisionDTO) decisionRow {
	row := decisionRow{
		DecisionID: d.DecisionID, Outcome: d.Outcome, Question: d.Question, OutcomeStatement: d.OutcomeStatement,
		Rationale: d.Rationale, Alternatives: d.Alternatives, Evidence: d.Basis.Evidence,
		Assumptions: d.Basis.Assumptions, Constraints: d.Basis.Constraints, Uncertainties: d.Basis.Uncertainties,
	}
	if d.OccurredAt != nil {
		row.HasOccurredAt = true
		row.OccurredAt = *d.OccurredAt
	}
	return row
}

func mapDecisionRows(decisions []apiApplicableDecisionDTO) []decisionRow {
	rows := make([]decisionRow, 0, len(decisions))
	for _, d := range decisions {
		rows = append(rows, mapDecisionRow(d))
	}
	return rows
}

// activityRow is one planned validation activity (FF-001 §3.6: "plan
// revision and its activities").
type activityRow struct {
	Key                   string
	Method                string
	OutcomeInterpretation string
	ExpectedEvidence      []string
}

func mapActivityRows(activities []apiPlanActivityDetailDTO) []activityRow {
	rows := make([]activityRow, 0, len(activities))
	for _, a := range activities {
		rows = append(rows, activityRow{Key: a.Key, Method: a.Method, OutcomeInterpretation: a.OutcomeInterpretation, ExpectedEvidence: a.ExpectedEvidence})
	}
	return rows
}
