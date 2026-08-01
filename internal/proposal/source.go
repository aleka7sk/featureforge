package proposal

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

// SourceKind is the closed set of FF-024 source-reference forms.
type SourceKind string

const (
	SourceKindArtifact         SourceKind = "artifact"
	SourceKindRevision         SourceKind = "revision"
	SourceKindRequirementTrace SourceKind = "requirement-trace"
	SourceKindCriterion        SourceKind = "criterion"
	SourceKindRecord           SourceKind = "record"
)

var governedIdentityPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

// SourceReference is one immutable canonical witness to persisted input.
// Its zero value is invalid.
type SourceReference struct {
	canonical string
	kind      SourceKind
}

// ParseSourceReference validates and returns one canonical source reference.
func ParseSourceReference(value string) (SourceReference, error) {
	kind, tail, found := strings.Cut(value, ":")
	if !found || tail == "" {
		return SourceReference{}, invalidSource(value, "missing kind or identity")
	}
	switch SourceKind(kind) {
	case SourceKindArtifact:
		if !validIdentity(tail) {
			return SourceReference{}, invalidSource(value, "invalid artifact identity")
		}
	case SourceKindRevision, SourceKindRequirementTrace:
		artifactID, revisionID, ok := splitRevision(tail)
		if !ok || !validIdentity(artifactID) || !validIdentity(revisionID) {
			return SourceReference{}, invalidSource(value, "invalid revision identity")
		}
	case SourceKindCriterion:
		revision, criterionKey, ok := strings.Cut(tail, "#")
		if !ok || strings.Contains(criterionKey, "#") {
			return SourceReference{}, invalidSource(value, "criterion reference must contain one #")
		}
		artifactID, revisionID, ok := splitRevision(revision)
		if !ok || !validIdentity(artifactID) || !validIdentity(revisionID) || engineering.ValidateAcceptanceCriterionKey(criterionKey) != nil {
			return SourceReference{}, invalidSource(value, "invalid criterion identity")
		}
	case SourceKindRecord:
		recordKind, recordID, ok := strings.Cut(tail, "/")
		if !ok || strings.Contains(recordID, "/") || !validIdentity(recordID) {
			return SourceReference{}, invalidSource(value, "invalid record identity")
		}
		if _, err := engineering.NewRecordKey(engineering.RecordKind(recordKind), recordID); err != nil {
			return SourceReference{}, invalidSource(value, "unsupported record kind")
		}
	default:
		return SourceReference{}, invalidSource(value, "unsupported source kind")
	}
	return SourceReference{canonical: value, kind: SourceKind(kind)}, nil
}

func invalidSource(value, reason string) error {
	return fmt.Errorf("%w: %q: %s", ErrInvalidSourceReference, value, reason)
}

func validIdentity(value string) bool { return governedIdentityPattern.MatchString(value) }

func splitRevision(value string) (string, string, bool) {
	artifactID, revisionID, found := strings.Cut(value, "/")
	return artifactID, revisionID, found && !strings.Contains(revisionID, "/")
}

func artifactSource(artifactID string) SourceReference {
	return SourceReference{canonical: "artifact:" + artifactID, kind: SourceKindArtifact}
}

func revisionSource(key engineering.RevisionKey) SourceReference {
	return SourceReference{canonical: "revision:" + key.ArtifactID + "/" + key.RevisionID, kind: SourceKindRevision}
}

func requirementTraceSource(key engineering.RevisionKey) SourceReference {
	return SourceReference{canonical: "requirement-trace:" + key.ArtifactID + "/" + key.RevisionID, kind: SourceKindRequirementTrace}
}

func criterionSource(key engineering.RevisionKey, criterionKey string) SourceReference {
	return SourceReference{canonical: "criterion:" + key.ArtifactID + "/" + key.RevisionID + "#" + criterionKey, kind: SourceKindCriterion}
}

func recordSource(key engineering.RecordKey) SourceReference {
	return SourceReference{canonical: "record:" + string(key.Kind) + "/" + key.ID, kind: SourceKindRecord}
}

// String returns the complete canonical source string.
func (r SourceReference) String() string { return r.canonical }

// Kind returns the source-reference form.
func (r SourceReference) Kind() SourceKind { return r.kind }

// IsZero reports whether r is the invalid zero value.
func (r SourceReference) IsZero() bool { return r.canonical == "" }
