package engineering

import (
	"errors"
	"testing"
	"time"
)

func TestNewRequirementCriterionTrace(t *testing.T) {
	requirement, _ := NewRevisionKey("REQ-1", "REQ-1-REV-1")
	capability, _ := NewRevisionKey("CAP-1", "CAP-1-REV-2")
	recordedAt := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	trace, err := NewRequirementCriterionTrace(requirement, capability, "AC-3", recordedAt)
	if err != nil {
		t.Fatal(err)
	}
	if trace.RequirementRevision != requirement || trace.CapabilityRevision != capability ||
		trace.AcceptanceCriterionKey != "AC-3" || !trace.RecordedAt.Equal(recordedAt) {
		t.Fatalf("trace = %+v, want the exact constructor input", trace)
	}
	if trace.IsZero() {
		t.Fatal("constructed trace reports itself as zero")
	}
	if !trace.Equal(trace) {
		t.Fatal("trace must equal itself")
	}
}

func TestRequirementCriterionTraceValidation(t *testing.T) {
	requirement, _ := NewRevisionKey("REQ-1", "REQ-1-REV-1")
	capability, _ := NewRevisionKey("CAP-1", "CAP-1-REV-2")
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	for _, tc := range []struct {
		name        string
		requirement RevisionKey
		capability  RevisionKey
		criterion   string
		recordedAt  time.Time
	}{
		{name: "missing requirement", capability: capability, criterion: "AC-1", recordedAt: now},
		{name: "missing capability", requirement: requirement, criterion: "AC-1", recordedAt: now},
		{name: "invalid criterion", requirement: requirement, capability: capability, criterion: "AC 1", recordedAt: now},
		{name: "missing time", requirement: requirement, capability: capability, criterion: "AC-1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewRequirementCriterionTrace(tc.requirement, tc.capability, tc.criterion, tc.recordedAt)
			if !errors.Is(err, ErrInvalidEnvelope) {
				t.Fatalf("err = %v, want ErrInvalidEnvelope", err)
			}
		})
	}
}
