package application

import (
	"context"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

// LifecycleRationale explains how a lifecycle-state resolution reached its
// answer.
type LifecycleRationale struct {
	Rule      string
	Total     int
	Duplicate bool
}

// LifecycleStateResult is the outcome of resolving a capability's current
// lifecycle state. Found is false, with no error, when no assignment has
// ever been recorded -- a legitimate state, not a failure.
type LifecycleStateResult struct {
	Found               bool
	Assignment          engineering.RecordEnvelope
	DefinitionID        string
	DefinitionVersionID string
	EstablishedBy       engineering.RevisionKey
	Rationale           LifecycleRationale
}

// ResolveLifecycleState returns the unique head of the complete validated
// predecessor chain. Timestamp maxima and tie-breaking are not authority.
func ResolveLifecycleState(ctx context.Context, repos Repositories, inspector EngineeringReplayInspector, capabilityArtifactID string) (LifecycleStateResult, error) {
	history, err := ResolveLifecycleHistory(ctx, repos, inspector, capabilityArtifactID)
	if err != nil {
		return LifecycleStateResult{}, err
	}
	if !history.Found {
		return LifecycleStateResult{Found: false, Rationale: LifecycleRationale{Rule: "no state assignment recorded"}}, nil
	}
	return LifecycleStateResult{
		Found:               true,
		Assignment:          history.Head.Assignment,
		DefinitionID:        history.Policy.DefinitionID,
		DefinitionVersionID: history.Policy.VersionID,
		EstablishedBy:       history.Head.Detail.EstablishedBy,
		Rationale: LifecycleRationale{
			Rule:  "unique head of validated lifecycle predecessor chain",
			Total: len(history.Assignments),
		},
	}, nil
}
