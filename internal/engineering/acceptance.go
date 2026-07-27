package engineering

import (
	"fmt"
	"time"
)

// AcceptanceState is a capability revision's product-owned acceptance state
// (FF-009 §4.3). It is never stored as a field -- only as the state named by
// the latest entry in a revision's acceptance journal.
type AcceptanceState string

const (
	AcceptanceStateDraft     AcceptanceState = "draft"
	AcceptanceStateAccepted  AcceptanceState = "accepted"
	AcceptanceStateWithdrawn AcceptanceState = "withdrawn"
)

// IsValid reports whether s is one of the three declared acceptance states.
func (s AcceptanceState) IsValid() bool {
	switch s {
	case AcceptanceStateDraft, AcceptanceStateAccepted, AcceptanceStateWithdrawn:
		return true
	default:
		return false
	}
}

// RevisionAcceptanceRecord is one append-only entry in a revision's
// acceptance journal (FF-009 §4.3). A revision's current acceptance state is
// the State of its latest record ordered by (EffectiveAt, RecordID).
type RevisionAcceptanceRecord struct {
	RecordID    string
	Key         RevisionKey
	State       AcceptanceState
	EffectiveAt time.Time
	Actor       string
	Reason      string
}

// NewRevisionAcceptanceRecord validates and returns a RevisionAcceptanceRecord.
func NewRevisionAcceptanceRecord(recordID string, key RevisionKey, state AcceptanceState, effectiveAt time.Time, actor, reason string) (RevisionAcceptanceRecord, error) {
	if recordID == "" {
		return RevisionAcceptanceRecord{}, fmt.Errorf("%w: acceptance record requires a non-empty record id", ErrInvalidEnvelope)
	}
	if key.IsZero() {
		return RevisionAcceptanceRecord{}, fmt.Errorf("%w: acceptance record requires a non-zero revision key", ErrInvalidEnvelope)
	}
	if !state.IsValid() {
		return RevisionAcceptanceRecord{}, fmt.Errorf("%w: unsupported acceptance state %q", ErrInvalidEnvelope, state)
	}
	if actor == "" {
		return RevisionAcceptanceRecord{}, fmt.Errorf("%w: acceptance record requires an actor", ErrInvalidEnvelope)
	}
	return RevisionAcceptanceRecord{
		RecordID:    recordID,
		Key:         key,
		State:       state,
		EffectiveAt: effectiveAt,
		Actor:       actor,
		Reason:      reason,
	}, nil
}

// ValidTransition reports whether moving from `from` to `to` is one of the
// three permitted acceptance transitions (FF-009 §4.3): draft->accepted,
// draft->withdrawn, accepted->withdrawn. withdrawn is terminal, and
// accepted->draft is never permitted. A zero `from` (no prior record --
// draft by absence) may transition to accepted or withdrawn.
func ValidTransition(from, to AcceptanceState) bool {
	if from == "" {
		from = AcceptanceStateDraft
	}
	switch {
	case from == AcceptanceStateDraft && to == AcceptanceStateAccepted:
		return true
	case from == AcceptanceStateDraft && to == AcceptanceStateWithdrawn:
		return true
	case from == AcceptanceStateAccepted && to == AcceptanceStateWithdrawn:
		return true
	default:
		return false
	}
}
