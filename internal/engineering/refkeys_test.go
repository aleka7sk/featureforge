package engineering

import (
	"errors"
	"testing"
)

// TestParseEvidenceKeyRoundTrips (FF-018 §7): ParseEvidenceKey is the exact
// inverse of EvidenceKey.
func TestParseEvidenceKeyRoundTrips(t *testing.T) {
	artifactID, revisionID, err := ParseEvidenceKey(EvidenceKey("EV-1", "EV-1-REV-1"))
	if err != nil {
		t.Fatal(err)
	}
	if artifactID != "EV-1" {
		t.Errorf("artifactID = %q, want %q", artifactID, "EV-1")
	}
	if revisionID != "EV-1-REV-1" {
		t.Errorf("revisionID = %q, want %q", revisionID, "EV-1-REV-1")
	}
}

// TestParseEvidenceKeyRejectsMalformedInput (FF-018 §7): every malformed
// shape is ErrInvalidEnvelope, mirroring ParseSubjectKey's conventions.
func TestParseEvidenceKeyRejectsMalformedInput(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"empty string", ""},
		{"missing prefix", "EV-1/EV-1-REV-1"},
		{"wrong prefix", "artifact:EV-1/EV-1-REV-1"},
		{"no separator", "evidence:EV-1-EV-1-REV-1"},
		{"empty artifact id", "evidence:/EV-1-REV-1"},
		{"empty revision id", "evidence:EV-1/"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := ParseEvidenceKey(tc.key)
			if !errors.Is(err, ErrInvalidEnvelope) {
				t.Errorf("err = %v, want ErrInvalidEnvelope", err)
			}
		})
	}
}
