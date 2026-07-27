package application

import (
	"context"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

// RecordArchitectureDecisionCommand records a Decision with its Basis
// (FF-010 §3).
type RecordArchitectureDecisionCommand struct {
	DecisionID         string
	SubjectArtifactID  string
	SubjectRevisionID  string
	Question           string
	OutcomeStatement   string
	Alternatives       []string
	EvidenceArtifactID string
	EvidenceRevisionID string
	Assumptions        []string
	Constraints        []string
	Uncertainties      []string
	Rationale          string
}

// RecordArchitectureDecisionResult names the record created.
type RecordArchitectureDecisionResult struct {
	Key engineering.RecordKey
}

// Execute validates the command and writes the Decision in one transaction.
func (c RecordArchitectureDecisionCommand) Execute(ctx context.Context, uow UnitOfWork, recorder EngineeringRecorder, clock Clock) (RecordArchitectureDecisionResult, error) {
	if err := requireNonEmpty("decision id", c.DecisionID); err != nil {
		return RecordArchitectureDecisionResult{}, err
	}
	if err := requireNonEmpty("subject artifact id", c.SubjectArtifactID); err != nil {
		return RecordArchitectureDecisionResult{}, err
	}
	if err := requireNonEmpty("subject revision id", c.SubjectRevisionID); err != nil {
		return RecordArchitectureDecisionResult{}, err
	}
	if err := requireNonEmpty("question", c.Question); err != nil {
		return RecordArchitectureDecisionResult{}, err
	}
	if err := requireNonEmpty("outcome statement", c.OutcomeStatement); err != nil {
		return RecordArchitectureDecisionResult{}, err
	}
	if err := requireNonEmpty("evidence artifact id", c.EvidenceArtifactID); err != nil {
		return RecordArchitectureDecisionResult{}, err
	}
	if err := requireNonEmpty("evidence revision id", c.EvidenceRevisionID); err != nil {
		return RecordArchitectureDecisionResult{}, err
	}
	now := clock.Now()

	var result RecordArchitectureDecisionResult
	err := uow.Do(ctx, func(r Repositories) error {
		env, err := recorder.RecordDecision(engineering.DecisionInput{
			DecisionID: c.DecisionID, SubjectArtifactID: c.SubjectArtifactID, SubjectRevisionID: c.SubjectRevisionID,
			Question: c.Question, OutcomeStatement: c.OutcomeStatement, Alternatives: c.Alternatives,
			EvidenceArtifactID: c.EvidenceArtifactID, EvidenceRevisionID: c.EvidenceRevisionID,
			Assumptions: c.Assumptions, Constraints: c.Constraints, Uncertainties: c.Uncertainties,
			Rationale: c.Rationale, RecordedAt: now,
		})
		if err != nil {
			return err
		}
		if err := r.Records.Put(ctx, env); err != nil {
			return err
		}
		result = RecordArchitectureDecisionResult{Key: env.Key}
		return nil
	})
	if err != nil {
		return RecordArchitectureDecisionResult{}, err
	}
	return result, nil
}
