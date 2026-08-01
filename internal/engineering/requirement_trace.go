package engineering

import (
	"fmt"
	"time"
)

// RequirementCriterionTrace is the product-owned, immutable link from one
// exact Requirement Revision to the exact capability Revision and
// revision-local acceptance criterion it derives from (AD-033, FF-023 §3).
// The RequirementRevision key is the trace identity.
type RequirementCriterionTrace struct {
	RequirementRevision    RevisionKey
	CapabilityRevision     RevisionKey
	AcceptanceCriterionKey string
	RecordedAt             time.Time
}

// NewRequirementCriterionTrace validates and returns a trace. The acceptance
// criterion grammar is shared with CapabilitySpecificationContent so a trace
// can never name a key that content itself could not carry.
func NewRequirementCriterionTrace(requirementRevision, capabilityRevision RevisionKey, criterionKey string, recordedAt time.Time) (RequirementCriterionTrace, error) {
	if requirementRevision.IsZero() {
		return RequirementCriterionTrace{}, fmt.Errorf("%w: requirement criterion trace requires a non-zero requirement revision", ErrInvalidEnvelope)
	}
	if capabilityRevision.IsZero() {
		return RequirementCriterionTrace{}, fmt.Errorf("%w: requirement criterion trace requires a non-zero capability revision", ErrInvalidEnvelope)
	}
	if err := ValidateAcceptanceCriterionKey(criterionKey); err != nil {
		return RequirementCriterionTrace{}, fmt.Errorf("%w: %v", ErrInvalidEnvelope, err)
	}
	if recordedAt.IsZero() {
		return RequirementCriterionTrace{}, fmt.Errorf("%w: requirement criterion trace requires a recorded-at time", ErrInvalidEnvelope)
	}
	return RequirementCriterionTrace{
		RequirementRevision:    requirementRevision,
		CapabilityRevision:     capabilityRevision,
		AcceptanceCriterionKey: criterionKey,
		RecordedAt:             recordedAt,
	}, nil
}

// IsZero reports whether t is the zero value.
func (t RequirementCriterionTrace) IsZero() bool {
	return t.RequirementRevision.IsZero() && t.CapabilityRevision.IsZero() &&
		t.AcceptanceCriterionKey == "" && t.RecordedAt.IsZero()
}

// Equal compares every immutable field. Times use time.Equal so adapters that
// normalize location while preserving the instant remain semantically equal.
func (t RequirementCriterionTrace) Equal(other RequirementCriterionTrace) bool {
	return t.RequirementRevision == other.RequirementRevision &&
		t.CapabilityRevision == other.CapabilityRevision &&
		t.AcceptanceCriterionKey == other.AcceptanceCriterionKey &&
		t.RecordedAt.Equal(other.RecordedAt)
}
