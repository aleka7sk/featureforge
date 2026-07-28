package engineering

import (
	"errors"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixedRecordedAt() time.Time { return time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC) }

func validPayload(t *testing.T) ([]byte, Digest) {
	t.Helper()
	payload := []byte(`{"hello":"world"}`)
	return payload, ComputeDigest(payload)
}

func TestEnvelopeValidation(t *testing.T) {
	key, _ := NewArtifactKey("CAP-1")
	payload, digest := validPayload(t)

	t.Run("empty key", func(t *testing.T) {
		_, err := NewArtifactEnvelope(ArtifactKey{}, "featureforge:product-capability", payload, digest, fixedRecordedAt())
		if !errors.Is(err, ErrInvalidEnvelope) {
			t.Errorf("err = %v, want ErrInvalidEnvelope", err)
		}
	})
	t.Run("empty payload", func(t *testing.T) {
		_, err := NewArtifactEnvelope(key, "featureforge:product-capability", nil, digest, fixedRecordedAt())
		if !errors.Is(err, ErrInvalidEnvelope) {
			t.Errorf("err = %v, want ErrInvalidEnvelope", err)
		}
	})
	t.Run("non-JSON payload", func(t *testing.T) {
		bad := []byte("not json")
		_, err := NewArtifactEnvelope(key, "featureforge:product-capability", bad, ComputeDigest(bad), fixedRecordedAt())
		if !errors.Is(err, ErrInvalidEnvelope) {
			t.Errorf("err = %v, want ErrInvalidEnvelope", err)
		}
	})
	t.Run("digest mismatch", func(t *testing.T) {
		otherPayload := []byte(`{"other":"payload"}`)
		wrongDigest := ComputeDigest(otherPayload)
		_, err := NewArtifactEnvelope(key, "featureforge:product-capability", payload, wrongDigest, fixedRecordedAt())
		if !errors.Is(err, ErrInvalidEnvelope) {
			t.Errorf("err = %v, want ErrInvalidEnvelope", err)
		}
	})
	t.Run("unknown record kind", func(t *testing.T) {
		_, err := NewRecordKey(RecordKind("bogus"), "X-1")
		if !errors.Is(err, ErrUnsupportedPayloadKind) {
			t.Errorf("err = %v, want ErrUnsupportedPayloadKind", err)
		}
	})
	t.Run("unknown revision family", func(t *testing.T) {
		revKey, _ := NewRevisionKey("CAP-1", "REV-1")
		_, err := NewRevisionEnvelope(RevisionEnvelopeInput{
			Key: revKey, RevisionFamily: RevisionFamily("bogus"),
			ArtifactType: "featureforge:product-capability", IntegrityValue: "sha256:x",
			Payload: payload, PayloadDigest: digest, RecordedAt: fixedRecordedAt(),
		})
		if !errors.Is(err, ErrUnsupportedPayloadKind) {
			t.Errorf("err = %v, want ErrUnsupportedPayloadKind", err)
		}
	})
	t.Run("empty subject key accepted for every family", func(t *testing.T) {
		for _, family := range []RevisionFamily{
			RevisionFamilyCapability, RevisionFamilyRequirement, RevisionFamilyValidationPlan,
			RevisionFamilyTransitionRecord, RevisionFamilyEvidence,
		} {
			revKey, _ := NewRevisionKey("CAP-1", "REV-1")
			_, err := NewRevisionEnvelope(RevisionEnvelopeInput{
				Key: revKey, RevisionFamily: family,
				ArtifactType: "featureforge:product-capability", IntegrityValue: "sha256:x",
				Payload: payload, PayloadDigest: digest, RecordedAt: fixedRecordedAt(),
			})
			if err != nil {
				t.Errorf("family %s: unexpected error with empty subject key: %v", family, err)
			}
		}
	})
	t.Run("valid subject key accepted for families that define one", func(t *testing.T) {
		for _, family := range []RevisionFamily{
			RevisionFamilyRequirement, RevisionFamilyValidationPlan, RevisionFamilyTransitionRecord,
		} {
			revKey, _ := NewRevisionKey("REQ-1", "REV-1")
			env, err := NewRevisionEnvelope(RevisionEnvelopeInput{
				Key: revKey, RevisionFamily: family,
				ArtifactType: "featureforge:requirement", IntegrityValue: "sha256:x",
				SubjectKey: ArtifactSubjectKey("CAP-1"),
				Payload:    payload, PayloadDigest: digest, RecordedAt: fixedRecordedAt(),
			})
			if err != nil {
				t.Errorf("family %s: unexpected error with a valid subject key: %v", family, err)
			}
			if env.SubjectKey != ArtifactSubjectKey("CAP-1") {
				t.Errorf("family %s: SubjectKey = %q, want %q", family, env.SubjectKey, ArtifactSubjectKey("CAP-1"))
			}
		}
	})
	t.Run("subject key rejected for capability and evidence", func(t *testing.T) {
		for _, family := range []RevisionFamily{RevisionFamilyCapability, RevisionFamilyEvidence} {
			revKey, _ := NewRevisionKey("CAP-1", "REV-1")
			_, err := NewRevisionEnvelope(RevisionEnvelopeInput{
				Key: revKey, RevisionFamily: family,
				ArtifactType: "featureforge:product-capability", IntegrityValue: "sha256:x",
				SubjectKey: ArtifactSubjectKey("CAP-1"),
				Payload:    payload, PayloadDigest: digest, RecordedAt: fixedRecordedAt(),
			})
			if !errors.Is(err, ErrInvalidEnvelope) {
				t.Errorf("family %s: err = %v, want ErrInvalidEnvelope", family, err)
			}
		}
	})
	t.Run("malformed subject key rejected", func(t *testing.T) {
		revKey, _ := NewRevisionKey("REQ-1", "REV-1")
		_, err := NewRevisionEnvelope(RevisionEnvelopeInput{
			Key: revKey, RevisionFamily: RevisionFamilyRequirement,
			ArtifactType: "featureforge:requirement", IntegrityValue: "sha256:x",
			SubjectKey: "nonsense:CAP-1",
			Payload:    payload, PayloadDigest: digest, RecordedAt: fixedRecordedAt(),
		})
		if !errors.Is(err, ErrInvalidEnvelope) {
			t.Errorf("err = %v, want ErrInvalidEnvelope", err)
		}
	})
}

func TestEnvelopeEqualityIsKeyPlusPayload(t *testing.T) {
	key, _ := NewArtifactKey("CAP-1")
	payload, digest := validPayload(t)

	a, err := NewArtifactEnvelope(key, "featureforge:product-capability", payload, digest, fixedRecordedAt())
	if err != nil {
		t.Fatal(err)
	}
	// A second envelope with a different RecordedAt projection (not part of
	// equality) but identical key and payload must still be Equal.
	b, err := NewArtifactEnvelope(key, "featureforge:product-capability", payload, digest, fixedRecordedAt().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !a.Equal(b) {
		t.Error("envelopes with equal keys and identical payloads must be Equal regardless of differing projections")
	}

	otherKey, _ := NewArtifactKey("CAP-2")
	c, err := NewArtifactEnvelope(otherKey, "featureforge:product-capability", payload, digest, fixedRecordedAt())
	if err != nil {
		t.Fatal(err)
	}
	if a.Equal(c) {
		t.Error("envelopes with different keys must not be Equal")
	}
}

// TestRevisionEnvelopeEqualityComparesSubjectKey asserts SubjectKey
// participates in Equal (AD-026): two revision envelopes with equal keys and
// identical payloads are Equal only when SubjectKey also matches, and a
// differing SubjectKey makes them unequal. Other projections (RecordedAt)
// remain excluded, as TestEnvelopeEqualityIsKeyPlusPayload already covers for
// ArtifactEnvelope.
func TestRevisionEnvelopeEqualityComparesSubjectKey(t *testing.T) {
	revKey, _ := NewRevisionKey("REQ-1", "REV-1")
	payload, digest := validPayload(t)

	a, err := NewRevisionEnvelope(RevisionEnvelopeInput{
		Key: revKey, RevisionFamily: RevisionFamilyRequirement,
		ArtifactType: "featureforge:requirement", IntegrityValue: "sha256:x",
		SubjectKey: ArtifactSubjectKey("CAP-1"),
		Payload:    payload, PayloadDigest: digest, RecordedAt: fixedRecordedAt(),
	})
	if err != nil {
		t.Fatal(err)
	}
	// Same key, payload, and SubjectKey, but a differing RecordedAt
	// projection: still Equal, since RecordedAt is not compared.
	sameSubject, err := NewRevisionEnvelope(RevisionEnvelopeInput{
		Key: revKey, RevisionFamily: RevisionFamilyRequirement,
		ArtifactType: "featureforge:requirement", IntegrityValue: "sha256:x",
		SubjectKey: ArtifactSubjectKey("CAP-1"),
		Payload:    payload, PayloadDigest: digest, RecordedAt: fixedRecordedAt().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !a.Equal(sameSubject) {
		t.Error("revision envelopes with equal keys, payloads, and SubjectKey must be Equal regardless of differing RecordedAt")
	}

	differentSubject, err := NewRevisionEnvelope(RevisionEnvelopeInput{
		Key: revKey, RevisionFamily: RevisionFamilyRequirement,
		ArtifactType: "featureforge:requirement", IntegrityValue: "sha256:x",
		SubjectKey: ArtifactSubjectKey("CAP-2"),
		Payload:    payload, PayloadDigest: digest, RecordedAt: fixedRecordedAt(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.Equal(differentSubject) {
		t.Error("revision envelopes with equal keys and identical payloads but differing SubjectKey must not be Equal (AD-026)")
	}
}

func TestEnvelopeDigestMatchesPayload(t *testing.T) {
	payload, digest := validPayload(t)
	if !ComputeDigest(payload).Equal(digest) {
		t.Error("ComputeDigest(payload) must equal the digest passed to the constructor")
	}
}

func TestRecordKeyDistinguishesKinds(t *testing.T) {
	claimKey, err := NewRecordKey(RecordKindClaim, "X-1")
	if err != nil {
		t.Fatal(err)
	}
	execKey, err := NewRecordKey(RecordKindExecution, "X-1")
	if err != nil {
		t.Fatal(err)
	}
	if claimKey == execKey {
		t.Error("the same ID string under two different kinds must not collide")
	}
	if claimKey.String() == execKey.String() {
		t.Error("String() must distinguish kinds too")
	}
}

// TestEngineeringPackageCompilesWithoutPEOS parses every non-test .go file in
// this package's directory and asserts none imports the PEOS SDK. This is
// the FF-013 step-3 gate: internal/engineering must build and pass its tests
// with no PEOS package anywhere in its import graph (AD-005). The
// system-wide "only one package imports PEOS" rule is asserted for every
// package by internal/architecture.
func TestEngineeringPackageCompilesWithoutPEOS(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	found := false
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		found = true
		f, err := parser.ParseFile(fset, filepath.Join(".", name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if strings.Contains(path, "aleka7sk/PEOS") {
				t.Errorf("%s imports %s -- internal/engineering must not import PEOS", name, path)
			}
			if strings.HasSuffix(path, "/engineering/peos") {
				t.Errorf("%s imports its own peos child package %s -- forbidden (FF-008 §7)", name, path)
			}
		}
	}
	if !found {
		t.Fatal("no non-test .go files found in internal/engineering; test setup is broken")
	}
}
