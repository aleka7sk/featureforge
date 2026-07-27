package application

import (
	"context"
	"fmt"
	"sort"

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
	Found      bool
	Assignment engineering.RecordEnvelope
	Rationale  LifecycleRationale
}

// ResolveLifecycleState implements the FF-010 §8 algorithm: the current
// state is the assignment with the greatest EffectiveAt, ties broken by the
// lowest record ID when the tied assignments name the same state, or
// rejected as ambiguous when they name different states.
func ResolveLifecycleState(ctx context.Context, repos Repositories, capabilityArtifactID string) (LifecycleStateResult, error) {
	subjectKey := engineering.ArtifactSubjectKey(capabilityArtifactID)
	assignments, err := repos.Records.ListByKindAndSubject(ctx, engineering.RecordKindStateAssignment, subjectKey)
	if err != nil {
		return LifecycleStateResult{}, err
	}
	if len(assignments) == 0 {
		return LifecycleStateResult{Found: false, Rationale: LifecycleRationale{Rule: "no state assignment recorded"}}, nil
	}

	sort.Slice(assignments, func(i, j int) bool {
		if !assignments[i].OccurredAt.Equal(assignments[j].OccurredAt) {
			return assignments[i].OccurredAt.After(assignments[j].OccurredAt)
		}
		return assignments[i].Key.ID < assignments[j].Key.ID
	})

	top := assignments[0].OccurredAt
	var candidates []engineering.RecordEnvelope
	for _, a := range assignments {
		if a.OccurredAt.Equal(top) {
			candidates = append(candidates, a)
		}
	}

	if len(candidates) == 1 {
		return LifecycleStateResult{
			Found:      true,
			Assignment: candidates[0],
			Rationale:  LifecycleRationale{Rule: "greatest EffectiveAt", Total: len(assignments)},
		}, nil
	}

	firstState := candidates[0].StateID
	for _, c := range candidates[1:] {
		if c.StateID != firstState {
			return LifecycleStateResult{}, fmt.Errorf("%w: %d assignments at %v name different states", ErrAmbiguousLifecycleState, len(candidates), top)
		}
	}
	// All candidates name the same state: a harmless duplicate. The sort
	// above already placed the lowest record ID first.
	return LifecycleStateResult{
		Found:      true,
		Assignment: candidates[0],
		Rationale:  LifecycleRationale{Rule: "greatest EffectiveAt, tie-broken by lowest record ID", Total: len(assignments), Duplicate: true},
	}, nil
}
