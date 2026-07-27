package engineering

import (
	"fmt"
	"time"
)

// RevisionOrderMetadata carries a revision's product-owned sequence only
// (FF-009 §4.2). It is insert-only. Acceptance is a separate, append-only
// journal (RevisionAcceptanceRecord) -- never stored here.
type RevisionOrderMetadata struct {
	Key        RevisionKey
	Sequence   int
	RecordedAt time.Time
}

// NewRevisionOrderMetadata validates and returns a RevisionOrderMetadata.
// Sequence must be a positive integer.
func NewRevisionOrderMetadata(key RevisionKey, sequence int, recordedAt time.Time) (RevisionOrderMetadata, error) {
	if key.IsZero() {
		return RevisionOrderMetadata{}, fmt.Errorf("%w: revision order metadata requires a non-zero key", ErrInvalidEnvelope)
	}
	if sequence < 1 {
		return RevisionOrderMetadata{}, fmt.Errorf("%w: sequence must be positive, got %d", ErrInvalidEnvelope, sequence)
	}
	return RevisionOrderMetadata{Key: key, Sequence: sequence, RecordedAt: recordedAt}, nil
}
