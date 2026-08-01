package application

import (
	"context"
	"fmt"
	"sort"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

// ConsideredRevision is one entry in a ResolutionRationale's Considered list.
type ConsideredRevision struct {
	Key             engineering.RevisionKey
	Sequence        int
	AcceptanceState engineering.AcceptanceState
}

// RejectedRevision is one entry in a ResolutionRationale's Rejected list.
type RejectedRevision struct {
	Key    engineering.RevisionKey
	Reason string
}

// ResolutionRationale explains how a current-revision resolution reached
// its answer (FF-010 §4).
type ResolutionRationale struct {
	Rule             string
	SelectedKey      engineering.RevisionKey
	SelectedSequence int
	Considered       []ConsideredRevision
	Rejected         []RejectedRevision
	Warnings         []string
}

// CurrentRevisionResult is the outcome of resolving an Artifact's current
// revision. Found is false, with no error, when no revision is currently
// accepted (FF-010 §4 step 10) -- a legitimate state, not a failure.
type CurrentRevisionResult struct {
	Found     bool
	Revision  engineering.RevisionEnvelope
	Sequence  int
	Rationale ResolutionRationale
}

// ResolveCurrentRevision implements the FF-010 §4 algorithm exactly:
// the current revision is the accepted revision with the greatest sequence.
// Insertion order, RecordedAt, and revision-ID lexical order are never
// consulted. Ambiguity fails explicitly rather than picking arbitrarily.
func ResolveCurrentRevision(ctx context.Context, repos Repositories, artifactID string) (CurrentRevisionResult, error) {
	envelopes, err := repos.Revisions.ListByArtifact(ctx, artifactID)
	if err != nil {
		return CurrentRevisionResult{}, err
	}
	order, err := repos.RevisionOrder.ListByArtifact(ctx, artifactID)
	if err != nil {
		return CurrentRevisionResult{}, err
	}
	journal, err := repos.RevisionAcceptance.ListByArtifact(ctx, artifactID)
	if err != nil {
		return CurrentRevisionResult{}, err
	}

	orderByKey := make(map[engineering.RevisionKey]engineering.RevisionOrderMetadata, len(order))
	for _, o := range order {
		if _, dup := orderByKey[o.Key]; dup {
			return CurrentRevisionResult{}, fmt.Errorf("%w: revision %s has more than one order entry", ErrRevisionSequenceConflict, o.Key)
		}
		orderByKey[o.Key] = o
	}

	envelopeByKey := make(map[engineering.RevisionKey]engineering.RevisionEnvelope, len(envelopes))
	for _, e := range envelopes {
		if e.Key.ArtifactID != artifactID {
			return CurrentRevisionResult{}, fmt.Errorf("%w: revision %s was returned for artifact %s", ErrRevisionReferenceMismatch, e.Key, artifactID)
		}
		if _, dup := envelopeByKey[e.Key]; dup {
			return CurrentRevisionResult{}, fmt.Errorf("%w: revision %s was returned more than once", ErrStoredStateIntegrity, e.Key)
		}
		envelopeByKey[e.Key] = e
	}

	// Step 4: every envelope must have exactly one order entry.
	for _, e := range envelopes {
		if _, ok := orderByKey[e.Key]; !ok {
			return CurrentRevisionResult{}, fmt.Errorf("%w: revision %s", ErrRevisionOrderMissing, e.Key)
		}
	}
	// Step 5: every order entry must name a stored envelope.
	for _, o := range order {
		if _, ok := envelopeByKey[o.Key]; !ok {
			return CurrentRevisionResult{}, fmt.Errorf("%w: order entry names revision %s, which is not stored", ErrRevisionReferenceMismatch, o.Key)
		}
	}
	// Step 6: every sequence must be positive.
	for _, o := range order {
		if o.Sequence < 1 {
			return CurrentRevisionResult{}, fmt.Errorf("%w: revision %s has sequence %d", ErrRevisionSequenceInvalid, o.Key, o.Sequence)
		}
	}
	// Step 7: sequence values must be unique within the artifact.
	bySequence := make(map[int]engineering.RevisionKey, len(order))
	for _, o := range order {
		if other, dup := bySequence[o.Sequence]; dup {
			return CurrentRevisionResult{}, fmt.Errorf("%w: sequence %d is shared by %s and %s", ErrRevisionSequenceConflict, o.Sequence, other, o.Key)
		}
		bySequence[o.Sequence] = o.Key
	}
	// The governed sequence is dense, not merely positive and unique:
	// N stored revisions must occupy exactly 1..N.  Otherwise a missing
	// predecessor could make the same persisted history resolve differently
	// after an unrelated repair.
	for expected := 1; expected <= len(order); expected++ {
		if _, ok := bySequence[expected]; !ok {
			return CurrentRevisionResult{}, fmt.Errorf("%w: artifact %s has no revision at sequence %d in the required dense range 1..%d",
				ErrRevisionSequenceInvalid, artifactID, expected, len(order))
		}
	}

	// Step 8: validate the complete journal before using any entry to derive
	// current state.  Draft remains represented by absence; an explicit
	// draft entry is not a permitted transition.
	acceptanceByKey, err := validateAcceptanceJournal(ctx, repos.RevisionAcceptance, artifactID, envelopeByKey, journal)
	if err != nil {
		return CurrentRevisionResult{}, err
	}

	rationale := ResolutionRationale{Rule: "greatest sequence among accepted revisions"}
	for _, e := range envelopes {
		rationale.Considered = append(rationale.Considered, ConsideredRevision{
			Key: e.Key, Sequence: orderByKey[e.Key].Sequence, AcceptanceState: acceptanceByKey[e.Key],
		})
	}
	sort.Slice(rationale.Considered, func(i, j int) bool {
		return rationale.Considered[i].Sequence < rationale.Considered[j].Sequence
	})

	// Step 9: filter to accepted revisions.
	var accepted []engineering.RevisionEnvelope
	for _, e := range envelopes {
		if acceptanceByKey[e.Key] == engineering.AcceptanceStateAccepted {
			accepted = append(accepted, e)
		}
	}

	// Step 10: no accepted revision is a legitimate non-error result.
	if len(accepted) == 0 {
		for _, e := range envelopes {
			reason := "not accepted"
			if acceptanceByKey[e.Key] == engineering.AcceptanceStateWithdrawn {
				reason = "withdrawn"
			}
			rationale.Rejected = append(rationale.Rejected, RejectedRevision{Key: e.Key, Reason: reason})
		}
		rationale.Warnings = append(rationale.Warnings, "no accepted revision")
		return CurrentRevisionResult{Found: false, Rationale: rationale}, nil
	}

	// Step 11: find the greatest sequence among accepted revisions.
	top := orderByKey[accepted[0].Key].Sequence
	for _, e := range accepted[1:] {
		if s := orderByKey[e.Key].Sequence; s > top {
			top = s
		}
	}

	// Step 12: candidates at the top sequence.
	var candidates []engineering.RevisionEnvelope
	for _, e := range accepted {
		if orderByKey[e.Key].Sequence == top {
			candidates = append(candidates, e)
		}
	}

	// Reject every non-winning revision with exactly one reason.
	for _, e := range envelopes {
		switch acceptanceByKey[e.Key] {
		case engineering.AcceptanceStateDraft:
			rationale.Rejected = append(rationale.Rejected, RejectedRevision{Key: e.Key, Reason: "not accepted"})
		case engineering.AcceptanceStateWithdrawn:
			rationale.Rejected = append(rationale.Rejected, RejectedRevision{Key: e.Key, Reason: "withdrawn"})
		case engineering.AcceptanceStateAccepted:
			if orderByKey[e.Key].Sequence != top {
				rationale.Rejected = append(rationale.Rejected, RejectedRevision{Key: e.Key, Reason: "lower sequence"})
			}
		}
	}

	// Step 13: defensive assertion, unreachable given step 7's uniqueness
	// check -- kept so a future branching extension cannot silently return
	// an arbitrary revision.
	if len(candidates) != 1 {
		return CurrentRevisionResult{}, fmt.Errorf("%w: sequence %d has %d accepted candidates", ErrCurrentRevisionAmbiguous, top, len(candidates))
	}

	// Step 14.
	winner := candidates[0]
	rationale.SelectedKey = winner.Key
	rationale.SelectedSequence = top
	return CurrentRevisionResult{
		Found:     true,
		Revision:  winner,
		Sequence:  top,
		Rationale: rationale,
	}, nil
}

func validateAcceptanceJournal(
	ctx context.Context,
	repo RevisionAcceptanceRepository,
	artifactID string,
	envelopeByKey map[engineering.RevisionKey]engineering.RevisionEnvelope,
	journal []engineering.RevisionAcceptanceRecord,
) (map[engineering.RevisionKey]engineering.AcceptanceState, error) {
	historyByKey := make(map[engineering.RevisionKey][]engineering.RevisionAcceptanceRecord, len(envelopeByKey))
	seenRecordIDs := make(map[string]engineering.RevisionKey, len(journal))

	for _, entry := range journal {
		if entry.EffectiveAt.IsZero() {
			return nil, fmt.Errorf("%w: acceptance record %q has no effective time", ErrStoredStateIntegrity, entry.RecordID)
		}
		if _, err := engineering.NewRevisionAcceptanceRecord(
			entry.RecordID, entry.Key, entry.State, entry.EffectiveAt, entry.Actor, entry.Reason,
		); err != nil {
			return nil, fmt.Errorf("%w: invalid acceptance record %q: %v", ErrStoredStateIntegrity, entry.RecordID, err)
		}
		if entry.Key.ArtifactID != artifactID {
			return nil, fmt.Errorf("%w: acceptance record %s targets artifact %s, want %s",
				ErrRevisionReferenceMismatch, entry.RecordID, entry.Key.ArtifactID, artifactID)
		}
		if _, ok := envelopeByKey[entry.Key]; !ok {
			return nil, fmt.Errorf("%w: acceptance record %s names revision %s, which is not stored",
				ErrRevisionReferenceMismatch, entry.RecordID, entry.Key)
		}
		if other, dup := seenRecordIDs[entry.RecordID]; dup {
			return nil, fmt.Errorf("%w: acceptance record id %q is shared by revisions %s and %s",
				ErrStoredStateIntegrity, entry.RecordID, other, entry.Key)
		}
		seenRecordIDs[entry.RecordID] = entry.Key

		// RecordID is globally unique, not merely unique within this artifact.
		// The identity lookup also detects an adapter whose artifact listing and
		// global identity index disagree.
		stored, found, err := repo.GetByRecordID(ctx, entry.RecordID)
		if err != nil {
			return nil, err
		}
		if !found || !sameAcceptanceRecord(stored, entry) {
			return nil, fmt.Errorf("%w: acceptance record id %q does not resolve to its listed journal entry",
				ErrStoredStateIntegrity, entry.RecordID)
		}

		historyByKey[entry.Key] = append(historyByKey[entry.Key], entry)
	}

	states := make(map[engineering.RevisionKey]engineering.AcceptanceState, len(envelopeByKey))
	for key := range envelopeByKey {
		entries := historyByKey[key]
		sort.Slice(entries, func(i, j int) bool {
			if !entries[i].EffectiveAt.Equal(entries[j].EffectiveAt) {
				return entries[i].EffectiveAt.Before(entries[j].EffectiveAt)
			}
			return entries[i].RecordID < entries[j].RecordID
		})

		from := engineering.AcceptanceState("")
		for _, entry := range entries {
			if !engineering.ValidTransition(from, entry.State) {
				if from == "" {
					from = engineering.AcceptanceStateDraft
				}
				return nil, fmt.Errorf("%w: invalid persisted acceptance transition %s -> %s for revision %s at record %s",
					ErrStoredStateIntegrity, from, entry.State, key, entry.RecordID)
			}
			from = entry.State
		}
		if from == "" {
			from = engineering.AcceptanceStateDraft
		}
		states[key] = from
	}
	return states, nil
}

func sameAcceptanceRecord(a, b engineering.RevisionAcceptanceRecord) bool {
	return a.RecordID == b.RecordID && a.Key == b.Key && a.State == b.State &&
		a.EffectiveAt.Equal(b.EffectiveAt) && a.Actor == b.Actor && a.Reason == b.Reason
}

// resolveAcceptanceState returns the state of the latest entry for key in
// journal, ordered by (EffectiveAt, RecordID) -- a total order. A revision
// with no entry is draft by absence (FF-009 §4.3).
func resolveAcceptanceState(journal []engineering.RevisionAcceptanceRecord, key engineering.RevisionKey) engineering.AcceptanceState {
	var latest *engineering.RevisionAcceptanceRecord
	for i := range journal {
		entry := &journal[i]
		if entry.Key != key {
			continue
		}
		if latest == nil || isLaterAcceptance(*entry, *latest) {
			latest = entry
		}
	}
	if latest == nil {
		return engineering.AcceptanceStateDraft
	}
	return latest.State
}

func isLaterAcceptance(a, b engineering.RevisionAcceptanceRecord) bool {
	if !a.EffectiveAt.Equal(b.EffectiveAt) {
		return a.EffectiveAt.After(b.EffectiveAt)
	}
	return a.RecordID > b.RecordID
}

// ValidateAcceptanceTransition reports whether moving revision key from its
// current journal-resolved state to `to` is permitted (FF-009 §4.3):
// draft->accepted, draft->withdrawn, accepted->withdrawn. Returns
// ErrAcceptanceTransitionInvalid otherwise.
func ValidateAcceptanceTransition(journal []engineering.RevisionAcceptanceRecord, key engineering.RevisionKey, to engineering.AcceptanceState) error {
	from := resolveAcceptanceState(journal, key)
	if !engineering.ValidTransition(from, to) {
		return fmt.Errorf("%w: %s -> %s for revision %s", ErrAcceptanceTransitionInvalid, from, to, key)
	}
	return nil
}
