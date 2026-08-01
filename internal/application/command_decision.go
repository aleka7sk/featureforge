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
func (c RecordArchitectureDecisionCommand) Execute(ctx context.Context, uow UnitOfWork, recorder EngineeringRecorder, inspector EngineeringReplayInspector, clock Clock) (RecordArchitectureDecisionResult, error) {
	if err := requireIdentity("decision id", c.DecisionID); err != nil {
		return RecordArchitectureDecisionResult{}, err
	}
	if err := requireIdentity("subject artifact id", c.SubjectArtifactID); err != nil {
		return RecordArchitectureDecisionResult{}, err
	}
	if err := requireIdentity("subject revision id", c.SubjectRevisionID); err != nil {
		return RecordArchitectureDecisionResult{}, err
	}
	if err := requireNonEmpty("question", c.Question); err != nil {
		return RecordArchitectureDecisionResult{}, err
	}
	if err := requireNonEmpty("outcome statement", c.OutcomeStatement); err != nil {
		return RecordArchitectureDecisionResult{}, err
	}
	if err := requireIdentity("evidence artifact id", c.EvidenceArtifactID); err != nil {
		return RecordArchitectureDecisionResult{}, err
	}
	if err := requireIdentity("evidence revision id", c.EvidenceRevisionID); err != nil {
		return RecordArchitectureDecisionResult{}, err
	}
	for field, values := range map[string][]string{
		"alternative": c.Alternatives, "assumption": c.Assumptions,
		"constraint": c.Constraints, "uncertainty": c.Uncertainties,
	} {
		for _, value := range values {
			if err := requireNonEmpty(field, value); err != nil {
				return RecordArchitectureDecisionResult{}, err
			}
		}
	}
	if c.Rationale != "" {
		if err := requireNonEmpty("rationale", c.Rationale); err != nil {
			return RecordArchitectureDecisionResult{}, err
		}
	}
	key, err := engineering.NewRecordKey(engineering.RecordKindDecision, c.DecisionID)
	if err != nil {
		return RecordArchitectureDecisionResult{}, invalidCommand(err)
	}
	now := normalizeTime(clock.Now())

	var result RecordArchitectureDecisionResult
	err = uow.Do(ctx, func(r Repositories) error {
		if stored, found, err := r.Records.Get(ctx, key); err != nil {
			return err
		} else if found {
			if err := inspectRecord(inspector, stored); err != nil {
				return err
			}
			if err := validateStoredDecisionReferences(ctx, r, inspector, stored); err != nil {
				return err
			}
			expected, buildErr := recorder.RecordDecision(engineering.DecisionInput{
				DecisionID: c.DecisionID, SubjectArtifactID: c.SubjectArtifactID, SubjectRevisionID: c.SubjectRevisionID,
				Question: c.Question, OutcomeStatement: c.OutcomeStatement, Alternatives: c.Alternatives,
				EvidenceArtifactID: c.EvidenceArtifactID, EvidenceRevisionID: c.EvidenceRevisionID,
				Assumptions: c.Assumptions, Constraints: c.Constraints, Uncertainties: c.Uncertainties,
				Rationale: c.Rationale, RecordedAt: stored.RecordedAt,
			})
			if buildErr != nil {
				return invalidCommand(buildErr)
			}
			if !stored.Equal(expected) {
				return immutableConflict("decision identity is occupied by different semantics")
			}
			result = RecordArchitectureDecisionResult{Key: stored.Key}
			return nil
		}
		if err := validateDecisionInputReferences(ctx, r, inspector, engineering.DecisionInput{
			SubjectArtifactID: c.SubjectArtifactID, SubjectRevisionID: c.SubjectRevisionID,
			EvidenceArtifactID: c.EvidenceArtifactID, EvidenceRevisionID: c.EvidenceRevisionID,
		}); err != nil {
			return err
		}
		if err := requireServerTime(now, "a new architecture-decision act"); err != nil {
			return err
		}
		env, err := recorder.RecordDecision(engineering.DecisionInput{
			DecisionID: c.DecisionID, SubjectArtifactID: c.SubjectArtifactID, SubjectRevisionID: c.SubjectRevisionID,
			Question: c.Question, OutcomeStatement: c.OutcomeStatement, Alternatives: c.Alternatives,
			EvidenceArtifactID: c.EvidenceArtifactID, EvidenceRevisionID: c.EvidenceRevisionID,
			Assumptions: c.Assumptions, Constraints: c.Constraints, Uncertainties: c.Uncertainties,
			Rationale: c.Rationale, RecordedAt: now,
		})
		if err != nil {
			return invalidCommand(err)
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
