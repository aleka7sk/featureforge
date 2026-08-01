package proposal

import (
	"strings"
)

// Generator has no authority other than its immutable ContextPack input.
type Generator interface {
	Generate(ContextPack) (Proposal, error)
}

// DeterministicGenerator is M.6's local, I/O-free authority-boundary
// demonstration.
type DeterministicGenerator struct{}

// NewDeterministicGenerator returns the stateless M.6 generator.
func NewDeterministicGenerator() DeterministicGenerator { return DeterministicGenerator{} }

// Generate conservatively proposes the current capability content, emits a
// deterministic rationale naming every gap/finding, and cites the complete
// canonical source set.
func (DeterministicGenerator) Generate(pack ContextPack) (Proposal, error) {
	if err := pack.Validate(); err != nil {
		return Proposal{}, err
	}
	var rationale strings.Builder
	rationale.WriteString("Deterministic proposal from the exact current capability content.")
	if len(pack.uncoveredCriteria) > 0 {
		rationale.WriteString(" Uncovered criteria: ")
		for index, criterion := range pack.uncoveredCriteria {
			if index > 0 {
				rationale.WriteString(", ")
			}
			rationale.WriteString(criterion.criterionKey)
		}
		rationale.WriteString(".")
	}
	if len(pack.findings) > 0 {
		rationale.WriteString(" Validation findings: ")
		for index, finding := range pack.findings {
			if index > 0 {
				rationale.WriteString(", ")
			}
			rationale.WriteString(finding.criterionKey)
			rationale.WriteString("=")
			rationale.WriteString(string(finding.outcome))
		}
		rationale.WriteString(".")
	}
	return NewProposal(pack, pack.capability.content, rationale.String(), pack.sources)
}
