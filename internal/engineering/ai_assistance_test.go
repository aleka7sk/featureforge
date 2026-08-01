package engineering

import (
	"errors"
	"testing"
)

func TestAIAssistanceWitnessCanonicalRoundTrip(t *testing.T) {
	proposal := ComputeDigest([]byte("proposal"))
	context := ComputeDigest([]byte("context"))
	sources := []string{
		"artifact:CAP-1",
		"criterion:CAP-1/CAP-1-REV-1#AC-1",
		"record:claim/CLM-1",
		"record:decision/DEC-1",
		"record:execution/EX-1",
		"requirement-trace:REQ-1/REQ-1-REV-1",
		"revision:CAP-1/CAP-1-REV-1",
	}
	witness, err := NewAIAssistanceWitness(proposal, context, sources)
	if err != nil {
		t.Fatal(err)
	}
	wantNote := AIAssistedMethod + ";proposal=" + proposal.Hex() +
		";context=" + context.Hex() +
		";sources=[" + sources[0] + "," + sources[1] + "," + sources[2] + "," + sources[3] + "," + sources[4] + "," + sources[5] + "," + sources[6] + "]"
	if got := witness.OriginNote(); got != wantNote {
		t.Fatalf("OriginNote() = %q, want %q", got, wantNote)
	}

	parsed, err := ParseAIAssistanceOriginNote(wantNote)
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.Equal(witness) {
		t.Fatal("parsed witness differs from constructed witness")
	}

	returnedSources := parsed.Sources()
	returnedSources[0] = "artifact:MUTATED"
	if parsed.Sources()[0] != sources[0] {
		t.Fatal("Sources did not return a defensive copy")
	}
}

func TestAIAssistanceWitnessRejectsNonCanonicalValues(t *testing.T) {
	proposal := ComputeDigest([]byte("proposal"))
	context := ComputeDigest([]byte("context"))

	tests := []struct {
		name     string
		proposal Digest
		context  Digest
		sources  []string
	}{
		{name: "missing proposal digest", context: context, sources: []string{"artifact:CAP-1"}},
		{name: "missing context digest", proposal: proposal, sources: []string{"artifact:CAP-1"}},
		{name: "missing sources", proposal: proposal, context: context},
		{name: "unsorted", proposal: proposal, context: context, sources: []string{"revision:CAP-1/REV-1", "artifact:CAP-1"}},
		{name: "duplicate", proposal: proposal, context: context, sources: []string{"artifact:CAP-1", "artifact:CAP-1"}},
		{name: "unknown grammar", proposal: proposal, context: context, sources: []string{"evidence:EV-1/REV-1"}},
		{name: "identity delimiter", proposal: proposal, context: context, sources: []string{"artifact:CAP/1"}},
		{name: "invalid criterion", proposal: proposal, context: context, sources: []string{"criterion:CAP-1/REV-1#criterion_with_underscore"}},
		{name: "unknown record kind", proposal: proposal, context: context, sources: []string{"record:proposal/PROP-1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewAIAssistanceWitness(tt.proposal, tt.context, tt.sources); !errors.Is(err, ErrInvalidContent) {
				t.Fatalf("error = %v, want ErrInvalidContent", err)
			}
		})
	}
}

func TestParseAIAssistanceOriginNoteRejectsAlternateForms(t *testing.T) {
	proposal := ComputeDigest([]byte("proposal"))
	context := ComputeDigest([]byte("context"))
	base := AIAssistedMethod + ";proposal=" + proposal.Hex() + ";context=" + context.Hex()
	tests := []string{
		base + ";sources=[]",
		base + ";sources=[revision:CAP-1/REV-1,artifact:CAP-1]",
		base + ";sources=[artifact:CAP-1,artifact:CAP-1]",
		base + ";sources=[artifact:CAP-1] ",
		" " + base + ";sources=[artifact:CAP-1]",
		AIAssistedMethod + ";proposal=" + proposal.Hex() + ";sources=[artifact:CAP-1];context=" + context.Hex(),
	}
	for _, note := range tests {
		if _, err := ParseAIAssistanceOriginNote(note); !errors.Is(err, ErrInvalidContent) {
			t.Errorf("ParseAIAssistanceOriginNote(%q) error = %v, want ErrInvalidContent", note, err)
		}
	}
}
