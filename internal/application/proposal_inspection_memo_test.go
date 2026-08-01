package application

import (
	"errors"
	"testing"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

type proposalMemoInspectorSpy struct {
	ProposalReplayInspector
	recordCalls int
	recordErr   error
	aiCalls     int
	aiSources   []string
}

func (s *proposalMemoInspectorSpy) ValidateRecord(engineering.RecordEnvelope) error {
	s.recordCalls++
	return s.recordErr
}

func (s *proposalMemoInspectorSpy) InspectAIAssistedCapabilityRevision(
	engineering.RevisionEnvelope,
) (engineering.Digest, engineering.Digest, []string, bool, error) {
	s.aiCalls++
	return engineering.Digest{}, engineering.Digest{}, append([]string(nil), s.aiSources...), true, nil
}

func TestProposalInspectionMemoCachesOnlyExactSuccessfulEnvelopes(t *testing.T) {
	spy := &proposalMemoInspectorSpy{}
	memo := newProposalInspectionMemo(spy)
	envelope := engineering.RecordEnvelope{
		Key:  engineering.RecordKey{Kind: engineering.RecordKindDecision, ID: "DEC-MEMO"},
		Kind: engineering.RecordKindDecision, SubjectKey: "artifact:CAP-1", Payload: []byte("payload"),
	}
	if err := memo.ValidateRecord(envelope); err != nil {
		t.Fatal(err)
	}
	if err := memo.ValidateRecord(envelope); err != nil {
		t.Fatal(err)
	}
	if spy.recordCalls != 1 {
		t.Fatalf("exact successful envelope calls = %d, want 1", spy.recordCalls)
	}

	contradictory := envelope
	contradictory.SubjectKey = "artifact:CAP-2"
	if err := memo.ValidateRecord(contradictory); err != nil {
		t.Fatal(err)
	}
	if spy.recordCalls != 2 {
		t.Fatalf("same-key different-envelope calls = %d, want 2", spy.recordCalls)
	}
}

func TestProposalInspectionMemoPreservesNilInspector(t *testing.T) {
	if memo := newProposalInspectionMemo(nil); memo != nil {
		t.Fatalf("newProposalInspectionMemo(nil) = %#v, want nil", memo)
	}
}

func TestProposalInspectionMemoNeverCachesFailures(t *testing.T) {
	want := errors.New("inspection failed")
	spy := &proposalMemoInspectorSpy{recordErr: want}
	memo := newProposalInspectionMemo(spy)
	envelope := engineering.RecordEnvelope{
		Key:  engineering.RecordKey{Kind: engineering.RecordKindDecision, ID: "DEC-FAIL"},
		Kind: engineering.RecordKindDecision, SubjectKey: "artifact:CAP-1", Payload: []byte("payload"),
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := memo.ValidateRecord(envelope); !errors.Is(err, want) {
			t.Fatalf("attempt %d err = %v, want %v", attempt+1, err, want)
		}
	}
	if spy.recordCalls != 2 {
		t.Fatalf("failed inspection calls = %d, want 2", spy.recordCalls)
	}

	spy.recordErr = nil
	if err := memo.ValidateRecord(envelope); err != nil {
		t.Fatal(err)
	}
	if err := memo.ValidateRecord(envelope); err != nil {
		t.Fatal(err)
	}
	if spy.recordCalls != 3 {
		t.Fatalf("calls after first success and cached replay = %d, want 3", spy.recordCalls)
	}
}

func TestProposalInspectionMemoReturnsDefensiveSourceCopies(t *testing.T) {
	spy := &proposalMemoInspectorSpy{aiSources: []string{"revision:CAP-1/CAP-1-REV-1"}}
	memo := newProposalInspectionMemo(spy)
	envelope := engineering.RevisionEnvelope{
		Key:            engineering.RevisionKey{ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-2"},
		RevisionFamily: engineering.RevisionFamilyCapability, Payload: []byte("payload"),
	}
	_, _, first, found, err := memo.InspectAIAssistedCapabilityRevision(envelope)
	if err != nil || !found {
		t.Fatalf("first inspection found/err = %t/%v", found, err)
	}
	first[0] = "artifact:TAMPERED"
	_, _, second, found, err := memo.InspectAIAssistedCapabilityRevision(envelope)
	if err != nil || !found {
		t.Fatalf("cached inspection found/err = %t/%v", found, err)
	}
	if second[0] != spy.aiSources[0] || spy.aiCalls != 1 {
		t.Fatalf("cached sources/calls = %v/%d, want %v/1", second, spy.aiCalls, spy.aiSources)
	}
}
