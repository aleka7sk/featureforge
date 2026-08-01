package proposal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

// Proposal is transient proposed capability content bound to an exact
// ContextPack by content addresses and exact source witnesses.
type Proposal struct {
	content        engineering.CapabilitySpecificationContent
	rationale      string
	sources        []SourceReference
	contextDigest  engineering.Digest
	proposalDigest engineering.Digest
}

// NewProposal constructs a fully validated proposal bound to pack. Source
// order is normalized; duplicate, external, unpaired, or incomplete sources
// are rejected.
func NewProposal(pack ContextPack, content engineering.CapabilitySpecificationContent, rationale string, sources []SourceReference) (Proposal, error) {
	if err := pack.Validate(); err != nil {
		return Proposal{}, fmt.Errorf("%w: %v", ErrInvalidProposal, err)
	}
	proposal, err := newProposal(content, rationale, sources, pack.contextDigest, true)
	if err != nil {
		return Proposal{}, err
	}
	if err := proposal.ValidateAgainst(pack); err != nil {
		return Proposal{}, err
	}
	return proposal, nil
}

func newProposal(content engineering.CapabilitySpecificationContent, rationale string, sources []SourceReference, contextDigest engineering.Digest, normalizeSources bool) (Proposal, error) {
	if content.IsZero() {
		return Proposal{}, invalidProposal("content", "must not be zero")
	}
	if strings.TrimSpace(rationale) == "" {
		return Proposal{}, invalidProposal("rationale", "must not be empty")
	}
	if contextDigest.IsZero() {
		return Proposal{}, invalidProposal("context digest", "must not be zero")
	}
	if len(sources) == 0 {
		return Proposal{}, invalidProposal("sources", "must not be empty")
	}
	canonicalSources := append([]SourceReference(nil), sources...)
	seen := make(map[string]struct{}, len(canonicalSources))
	for index, source := range canonicalSources {
		parsed, err := ParseSourceReference(source.String())
		if err != nil || parsed != source {
			return Proposal{}, invalidProposal(fmt.Sprintf("sources[%d]", index), "is not a valid canonical source reference")
		}
		if _, duplicate := seen[source.String()]; duplicate {
			return Proposal{}, invalidProposal("sources", "must not contain duplicates")
		}
		seen[source.String()] = struct{}{}
	}
	if normalizeSources {
		sort.Slice(canonicalSources, func(i, j int) bool { return canonicalSources[i].String() < canonicalSources[j].String() })
	} else {
		for index := 1; index < len(canonicalSources); index++ {
			if canonicalSources[index-1].String() >= canonicalSources[index].String() {
				return Proposal{}, invalidProposal("sources", "must be duplicate-free and sorted bytewise")
			}
		}
	}
	proposal := Proposal{content: content, rationale: rationale, sources: canonicalSources, contextDigest: contextDigest}
	body, err := proposal.canonicalBodyJSON()
	if err != nil {
		return Proposal{}, invalidProposal("canonical body", err.Error())
	}
	proposal.proposalDigest = engineering.ComputeDigest(body)
	return proposal, nil
}

func invalidProposal(field, reason string) error {
	return fmt.Errorf("%w: %s %s", ErrInvalidProposal, field, reason)
}

// IsZero reports whether p is the invalid zero Proposal.
func (p Proposal) IsZero() bool {
	return p.content.IsZero() || p.contextDigest.IsZero() || p.proposalDigest.IsZero()
}

// Content returns the immutable proposed capability content.
func (p Proposal) Content() engineering.CapabilitySpecificationContent { return p.content }

// Rationale returns the exact rationale bound by ProposalDigest.
func (p Proposal) Rationale() string { return p.rationale }

// Sources returns a defensive copy of the canonical source subset.
func (p Proposal) Sources() []SourceReference {
	return cloneValues(p.sources)
}

// ContextDigest returns the exact context content address reviewed by the
// generator.
func (p Proposal) ContextDigest() engineering.Digest { return p.contextDigest }

// ProposalDigest returns the content address of the canonical proposal body.
func (p Proposal) ProposalDigest() engineering.Digest { return p.proposalDigest }

// Validate verifies the proposal's state-independent canonical invariants.
// Context source membership is checked by ValidateAgainst.
func (p Proposal) Validate() error {
	if p.IsZero() {
		return invalidProposal("proposal", "must not be zero")
	}
	if strings.TrimSpace(p.rationale) == "" || len(p.sources) == 0 {
		return invalidProposal("proposal", "contains invalid rationale or sources")
	}
	for index, source := range p.sources {
		parsed, err := ParseSourceReference(source.String())
		if err != nil || parsed != source {
			return invalidProposal(fmt.Sprintf("sources[%d]", index), "is invalid")
		}
		if index > 0 && p.sources[index-1].String() >= source.String() {
			return invalidProposal("sources", "must be duplicate-free and sorted bytewise")
		}
	}
	body, err := p.canonicalBodyJSON()
	if err != nil {
		return invalidProposal("canonical body", err.Error())
	}
	if !engineering.ComputeDigest(body).Equal(p.proposalDigest) {
		return invalidProposal("proposal digest", "does not match the canonical proposal body")
	}
	return nil
}

// ValidateAgainst verifies context identity, exact source membership, the
// mandatory current-revision source, and Decision source/subject pairing.
func (p Proposal) ValidateAgainst(pack ContextPack) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if err := pack.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidProposal, err)
	}
	if !p.contextDigest.Equal(pack.contextDigest) {
		return invalidProposal("context digest", "does not identify the supplied ContextPack")
	}
	allowed := make(map[string]struct{}, len(pack.sources))
	for _, source := range pack.sources {
		allowed[source.String()] = struct{}{}
	}
	selected := make(map[string]struct{}, len(p.sources))
	for _, source := range p.sources {
		if _, exists := allowed[source.String()]; !exists {
			return invalidProposal("sources", "contain a reference outside the ContextPack")
		}
		selected[source.String()] = struct{}{}
	}
	current := revisionSource(pack.capability.revision).String()
	if _, exists := selected[current]; !exists {
		return invalidProposal("sources", "must include the exact current capability revision")
	}
	for _, decision := range pack.decisions {
		record := recordSource(engineering.RecordKey{Kind: engineering.RecordKindDecision, ID: decision.decisionID}).String()
		if _, cited := selected[record]; !cited {
			continue
		}
		if _, paired := selected[decision.subject.source().String()]; !paired {
			return invalidProposal("sources", "must pair every cited Decision record with its exact authoritative subject")
		}
	}
	return nil
}

type proposalBodyWire struct {
	Content       json.RawMessage `json:"content"`
	Rationale     string          `json:"rationale"`
	Sources       []string        `json:"sources"`
	ContextDigest string          `json:"context_digest"`
}

type proposalWire struct {
	Content        json.RawMessage `json:"content"`
	Rationale      string          `json:"rationale"`
	Sources        []string        `json:"sources"`
	ContextDigest  string          `json:"context_digest"`
	ProposalDigest string          `json:"proposal_digest"`
}

func (p Proposal) bodyWire() (proposalBodyWire, error) {
	content, err := p.content.CanonicalJSON()
	if err != nil {
		return proposalBodyWire{}, err
	}
	sources := make([]string, len(p.sources))
	for index, source := range p.sources {
		sources[index] = source.String()
	}
	return proposalBodyWire{Content: content, Rationale: p.rationale, Sources: sources, ContextDigest: p.contextDigest.Hex()}, nil
}

func (p Proposal) canonicalBodyJSON() ([]byte, error) {
	body, err := p.bodyWire()
	if err != nil {
		return nil, err
	}
	return canonicalEncode(body)
}

// CanonicalJSON returns the deterministic proposal encoding including its
// digest. ProposalDigest is computed over the same body without the final
// proposal_digest field.
func (p Proposal) CanonicalJSON() ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	body, err := p.bodyWire()
	if err != nil {
		return nil, err
	}
	return canonicalEncode(proposalWire{
		Content: body.Content, Rationale: body.Rationale, Sources: body.Sources,
		ContextDigest: body.ContextDigest, ProposalDigest: p.proposalDigest.Hex(),
	})
}

// ParseProposal parses one exact canonical Proposal and verifies its full
// content and digest. Call ValidateAgainst inside the fresh acceptance
// transaction to check state-dependent source membership and pairing.
func ParseProposal(data []byte) (Proposal, error) {
	var wire proposalWire
	if err := decodeStrict(data, &wire); err != nil {
		return Proposal{}, fmt.Errorf("%w: decode: %v", ErrInvalidProposal, err)
	}
	content, err := engineering.ParseCapabilitySpecificationContent(wire.Content)
	if err != nil {
		return Proposal{}, fmt.Errorf("%w: content: %v", ErrInvalidProposal, err)
	}
	canonicalContent, err := content.CanonicalJSON()
	if err != nil || !bytes.Equal(wire.Content, canonicalContent) {
		return Proposal{}, invalidProposal("content", "must use its exact canonical encoding")
	}
	contextDigest, err := engineering.NewDigest(wire.ContextDigest)
	if err != nil {
		return Proposal{}, invalidProposal("context digest", "must be a lower-case SHA-256 digest")
	}
	sources := make([]SourceReference, len(wire.Sources))
	for index, value := range wire.Sources {
		sources[index], err = ParseSourceReference(value)
		if err != nil {
			return Proposal{}, fmt.Errorf("%w: %v", ErrInvalidProposal, err)
		}
	}
	proposal, err := newProposal(content, wire.Rationale, sources, contextDigest, false)
	if err != nil {
		return Proposal{}, err
	}
	digest, err := engineering.NewDigest(wire.ProposalDigest)
	if err != nil || !digest.Equal(proposal.proposalDigest) {
		return Proposal{}, invalidProposal("proposal digest", "does not match the canonical proposal body")
	}
	canonical, err := proposal.CanonicalJSON()
	if err != nil || !bytes.Equal(data, canonical) {
		return Proposal{}, invalidProposal("proposal JSON", "must be the exact canonical encoding")
	}
	return proposal, nil
}
