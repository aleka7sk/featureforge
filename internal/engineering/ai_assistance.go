package engineering

import (
	"fmt"
	"regexp"
	"strings"
)

// AIAssistedMethod is the one governed provenance method for capability
// revisions created through the reviewed AI-assisted proposal workflow
// (AD-034, FF-024 §7.4). It is deliberately not a caller-selected value.
const AIAssistedMethod = "featureforge:ai-assisted"

const aiAssistanceOriginPrefix = AIAssistedMethod + ";proposal="

var (
	aiAssistanceIdentityPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	aiAssistanceCriterionPattern = regexp.MustCompile(`^[A-Za-z0-9-]+$`)
)

// AIAssistanceWitness is the PEOS-independent immutable value carried by the
// known-Origin note of an AI-assisted capability revision. Its fields are
// private so the only non-zero values are canonical values returned by the
// constructor or parser.
type AIAssistanceWitness struct {
	proposalDigest Digest
	contextDigest  Digest
	sources        []string
}

// NewAIAssistanceWitness validates the immutable proposal/context witness.
// Sources must already be in strictly increasing bytewise order; silently
// reordering them here would hide a non-canonical proposal at the persistence
// boundary.
func NewAIAssistanceWitness(proposalDigest, contextDigest Digest, sources []string) (AIAssistanceWitness, error) {
	if proposalDigest.IsZero() || contextDigest.IsZero() {
		return AIAssistanceWitness{}, fmt.Errorf("%w: AI assistance witness requires proposal and context digests", ErrInvalidContent)
	}
	if len(sources) == 0 {
		return AIAssistanceWitness{}, fmt.Errorf("%w: AI assistance witness requires at least one source", ErrInvalidContent)
	}

	canonical := append([]string(nil), sources...)
	for idx, source := range canonical {
		if err := validateAIAssistanceSourceReference(source); err != nil {
			return AIAssistanceWitness{}, err
		}
		if idx > 0 && canonical[idx-1] >= source {
			return AIAssistanceWitness{}, fmt.Errorf("%w: AI assistance sources must be sorted bytewise and unique", ErrInvalidContent)
		}
	}

	return AIAssistanceWitness{
		proposalDigest: proposalDigest,
		contextDigest:  contextDigest,
		sources:        canonical,
	}, nil
}

// ParseAIAssistanceOriginNote parses only the exact one-line FF-024 §7.4
// representation. Alternate whitespace, ordering, delimiters, digest encodings,
// duplicate sources, and malformed SourceReference values are rejected.
func ParseAIAssistanceOriginNote(note string) (AIAssistanceWitness, error) {
	afterProposal, ok := strings.CutPrefix(note, aiAssistanceOriginPrefix)
	if !ok {
		return AIAssistanceWitness{}, fmt.Errorf("%w: AI assistance origin note has the wrong prefix", ErrInvalidContent)
	}
	proposalHex, afterContext, ok := strings.Cut(afterProposal, ";context=")
	if !ok {
		return AIAssistanceWitness{}, fmt.Errorf("%w: AI assistance origin note has no context digest", ErrInvalidContent)
	}
	contextHex, sourceSection, ok := strings.Cut(afterContext, ";sources=[")
	if !ok || !strings.HasSuffix(sourceSection, "]") {
		return AIAssistanceWitness{}, fmt.Errorf("%w: AI assistance origin note has an invalid sources section", ErrInvalidContent)
	}
	sourceSection = strings.TrimSuffix(sourceSection, "]")
	if sourceSection == "" {
		return AIAssistanceWitness{}, fmt.Errorf("%w: AI assistance origin note has no sources", ErrInvalidContent)
	}

	proposalDigest, err := NewDigest(proposalHex)
	if err != nil {
		return AIAssistanceWitness{}, fmt.Errorf("%w: invalid proposal digest", err)
	}
	contextDigest, err := NewDigest(contextHex)
	if err != nil {
		return AIAssistanceWitness{}, fmt.Errorf("%w: invalid context digest", err)
	}
	witness, err := NewAIAssistanceWitness(proposalDigest, contextDigest, strings.Split(sourceSection, ","))
	if err != nil {
		return AIAssistanceWitness{}, err
	}
	if witness.OriginNote() != note {
		return AIAssistanceWitness{}, fmt.Errorf("%w: AI assistance origin note is not canonical", ErrInvalidContent)
	}
	return witness, nil
}

// ProposalDigest returns the digest of the exact accepted transient proposal.
func (w AIAssistanceWitness) ProposalDigest() Digest { return w.proposalDigest }

// ContextDigest returns the digest of the authoritative context pack from
// which the proposal was produced.
func (w AIAssistanceWitness) ContextDigest() Digest { return w.contextDigest }

// Sources returns a defensive copy of the canonical exact SourceReferences.
func (w AIAssistanceWitness) Sources() []string {
	return append([]string(nil), w.sources...)
}

// OriginNote returns the exact deterministic known-Origin note governed by
// FF-024 §7.4. The zero value returns an empty string.
func (w AIAssistanceWitness) OriginNote() string {
	if w.IsZero() {
		return ""
	}
	return aiAssistanceOriginPrefix + w.proposalDigest.Hex() +
		";context=" + w.contextDigest.Hex() +
		";sources=[" + strings.Join(w.sources, ",") + "]"
}

// IsZero reports whether w is the ordinary, non-AI capability-revision form.
func (w AIAssistanceWitness) IsZero() bool {
	return w.proposalDigest.IsZero() && w.contextDigest.IsZero() && len(w.sources) == 0
}

// Equal reports whether both witnesses bind the exact same proposal, context,
// and ordered source set.
func (w AIAssistanceWitness) Equal(other AIAssistanceWitness) bool {
	if !w.proposalDigest.Equal(other.proposalDigest) || !w.contextDigest.Equal(other.contextDigest) || len(w.sources) != len(other.sources) {
		return false
	}
	for idx := range w.sources {
		if w.sources[idx] != other.sources[idx] {
			return false
		}
	}
	return true
}

func validateAIAssistanceSourceReference(source string) error {
	invalid := func() error {
		return fmt.Errorf("%w: invalid AI assistance source reference %q", ErrInvalidContent, source)
	}
	validIdentity := func(value string) bool {
		return aiAssistanceIdentityPattern.MatchString(value)
	}
	validRevisionPair := func(value string) bool {
		artifactID, revisionID, ok := strings.Cut(value, "/")
		return ok && !strings.Contains(revisionID, "/") && validIdentity(artifactID) && validIdentity(revisionID)
	}

	if value, ok := strings.CutPrefix(source, "artifact:"); ok {
		if validIdentity(value) {
			return nil
		}
		return invalid()
	}
	if value, ok := strings.CutPrefix(source, "revision:"); ok {
		if validRevisionPair(value) {
			return nil
		}
		return invalid()
	}
	if value, ok := strings.CutPrefix(source, "requirement-trace:"); ok {
		if validRevisionPair(value) {
			return nil
		}
		return invalid()
	}
	if value, ok := strings.CutPrefix(source, "criterion:"); ok {
		artifactID, remainder, hasRevision := strings.Cut(value, "/")
		revisionID, criterionKey, hasCriterion := strings.Cut(remainder, "#")
		if hasRevision && hasCriterion && !strings.Contains(criterionKey, "#") && validIdentity(artifactID) && validIdentity(revisionID) && aiAssistanceCriterionPattern.MatchString(criterionKey) {
			return nil
		}
		return invalid()
	}
	if value, ok := strings.CutPrefix(source, "record:"); ok {
		kind, recordID, hasID := strings.Cut(value, "/")
		if !hasID || strings.Contains(recordID, "/") || !validIdentity(recordID) {
			return invalid()
		}
		switch RecordKind(kind) {
		case RecordKindDecision, RecordKindExecution, RecordKindClaim, RecordKindStateAssignment:
			return nil
		default:
			return invalid()
		}
	}
	return invalid()
}
