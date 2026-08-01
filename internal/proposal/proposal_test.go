package proposal

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

func TestCanonicalContextRoundTripAndGoldenDigest(t *testing.T) {
	pack := canonicalFixture(t, false)
	encoded, err := pack.CanonicalJSON()
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	if bytes.Contains(encoded, []byte(":null")) {
		t.Fatalf("canonical context contains null slice: %s", encoded)
	}
	const wantDigest = "b823a25345870b54f39bc450443f47e86a18b71d8e605551fd3b5eb8fb52797b"
	if got := pack.ContextDigest().Hex(); got != wantDigest {
		t.Fatalf("ContextDigest = %q, want %q", got, wantDigest)
	}
	parsed, err := ParseContextPack(encoded)
	if err != nil {
		t.Fatalf("ParseContextPack: %v", err)
	}
	roundTrip, err := parsed.CanonicalJSON()
	if err != nil {
		t.Fatalf("round-trip CanonicalJSON: %v", err)
	}
	if !bytes.Equal(encoded, roundTrip) {
		t.Fatalf("round trip changed canonical bytes\nfirst: %s\nagain: %s", encoded, roundTrip)
	}
	assertSourcePresent(t, parsed.Sources(), "record:decision/DEC-OLD")
	assertSourcePresent(t, parsed.Sources(), "revision:CAP-1/CAP-1-REV-1")
}

func TestContextEmptySlicesAreArrays(t *testing.T) {
	content := mustContent(t, nil, nil)
	capability, err := NewCapabilityContext(revision("CAP-EMPTY", "REV-1"), 1, content)
	if err != nil {
		t.Fatal(err)
	}
	pack, err := NewContextPack(capability, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := pack.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"requirements":[]`, `"claims":[]`, `"decisions":[]`, `"open_questions":[]`, `"uncovered_criteria":[]`, `"findings":[]`} {
		if !bytes.Contains(encoded, []byte(field)) {
			t.Errorf("canonical JSON missing %s: %s", field, encoded)
		}
	}
	if pack.Requirements() == nil || pack.Claims() == nil || pack.Decisions() == nil ||
		pack.OpenQuestions() == nil || pack.UncoveredCriteria() == nil || pack.Findings() == nil {
		t.Fatal("an empty ContextPack accessor returned nil instead of an empty slice")
	}
}

func TestContextOrderingPermutationNormalizes(t *testing.T) {
	ordered := canonicalFixture(t, false)
	permuted := canonicalFixture(t, true)
	orderedJSON, err := ordered.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	permutedJSON, err := permuted.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(orderedJSON, permutedJSON) {
		t.Fatalf("equivalent input permutations differ\nordered:  %s\npermuted: %s", orderedJSON, permutedJSON)
	}
	if !ordered.ContextDigest().Equal(permuted.ContextDigest()) {
		t.Fatal("equivalent input permutations produced different context digests")
	}
}

func TestSourceReferenceRejectsInvalidForms(t *testing.T) {
	for _, value := range []string{
		"", "unknown:CAP-1", "artifact:", "artifact:bad/value",
		"revision:CAP-1", "revision:CAP-1/REV-1/extra",
		"criterion:CAP-1/REV-1", "criterion:CAP-1/REV-1#bad_key",
		"record:artifact/A-1", "record:claim/bad/value",
	} {
		t.Run(value, func(t *testing.T) {
			if _, err := ParseSourceReference(value); !errors.Is(err, ErrInvalidSourceReference) {
				t.Fatalf("ParseSourceReference(%q) error = %v", value, err)
			}
		})
	}
}

func TestContextAndProposalDefensiveCopies(t *testing.T) {
	pack := canonicalFixture(t, false)
	before, err := pack.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	requirements := pack.Requirements()
	requirements[0] = RequirementContext{}
	claims := pack.Claims()
	criteria := claims[0].CriterionKeys()
	criteria[0] = "mutated"
	uncovered := pack.UncoveredCriteria()
	affected := uncovered[0].RequirementRevisions()
	if len(affected) > 0 {
		affected[0] = revision("MUTATED", "MUTATED")
	}
	sources := pack.Sources()
	sources[0] = SourceReference{}
	after, err := pack.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("mutation through ContextPack accessors changed canonical bytes")
	}

	proposalInputSources := pack.Sources()
	proposal, err := NewProposal(pack, pack.Capability().Content(), "Reviewed exact context.", proposalInputSources)
	if err != nil {
		t.Fatal(err)
	}
	proposalBefore, _ := proposal.CanonicalJSON()
	proposalInputSources[0] = SourceReference{}
	proposalSources := proposal.Sources()
	proposalSources[0] = SourceReference{}
	proposalAfter, _ := proposal.CanonicalJSON()
	if !bytes.Equal(proposalBefore, proposalAfter) {
		t.Fatal("mutation through Proposal.Sources changed canonical bytes")
	}
}

func TestEmptyClaimReasoningIsPreserved(t *testing.T) {
	content := mustContent(t, []engineering.AcceptanceCriterion{mustCriterion(t, "AC-1", "Exact criterion.")}, nil)
	current := revision("CAP-EMPTY-REASON", "REV-1")
	capability, err := NewCapabilityContext(current, 1, content)
	if err != nil {
		t.Fatal(err)
	}
	requirement := mustRequirement(t, "REQ-EMPTY-REASON", "REV-1", 1, "Exact requirement.", current, "AC-1")
	requirementCriterion, err := engineering.RequirementCriterionKey(requirement.Revision())
	if err != nil {
		t.Fatal(err)
	}
	claim, err := NewClaimContext(
		engineering.RecordKey{Kind: engineering.RecordKindClaim, ID: "CLM-EMPTY-REASON"},
		requirement.Revision(), current, current.ArtifactID, []string{requirementCriterion},
		"not-satisfied", "", nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if claim.ExecutionReferences() == nil || claim.EvidenceReferences() == nil {
		t.Fatal("an empty ClaimContext accessor returned nil instead of an empty slice")
	}
	finding := mustFinding(t, current, "AC-1", requirement.Revision(), claim.Record(), FindingNotSatisfied, "")
	pack, err := NewContextPack(capability, []RequirementContext{requirement}, []ClaimContext{claim}, nil, nil, nil, []ValidationFinding{finding})
	if err != nil {
		t.Fatalf("NewContextPack: %v", err)
	}
	encoded, err := pack.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if got := bytes.Count(encoded, []byte(`"reasoning":""`)); got != 2 {
		t.Fatalf("empty reasoning occurrences = %d, want 2: %s", got, encoded)
	}
	if _, err := ParseContextPack(encoded); err != nil {
		t.Fatalf("ParseContextPack: %v", err)
	}
}

func TestProposalRoundTripGoldenAndContextBinding(t *testing.T) {
	pack := canonicalFixture(t, false)
	proposal, err := NewProposal(pack, pack.Capability().Content(), "Reviewed exact context.", pack.Sources())
	if err != nil {
		t.Fatalf("NewProposal: %v", err)
	}
	const wantDigest = "ee59b09faad689ce1f6229d1769a50cedf3edfc49baef0463495e7a5ca8a3698"
	if got := proposal.ProposalDigest().Hex(); got != wantDigest {
		t.Fatalf("ProposalDigest = %q, want %q", got, wantDigest)
	}
	encoded, err := proposal.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseProposal(encoded)
	if err != nil {
		t.Fatalf("ParseProposal: %v", err)
	}
	if err := parsed.ValidateAgainst(pack); err != nil {
		t.Fatalf("ValidateAgainst: %v", err)
	}
	roundTrip, _ := parsed.CanonicalJSON()
	if !bytes.Equal(encoded, roundTrip) {
		t.Fatal("proposal round trip changed canonical bytes")
	}
}

func TestProposalMutationChangesDigest(t *testing.T) {
	pack := canonicalFixture(t, false)
	base, err := NewProposal(pack, pack.Capability().Content(), "Reviewed exact context.", pack.Sources())
	if err != nil {
		t.Fatal(err)
	}
	changedContent, err := pack.Capability().Content().WithUserOutcome("A different reviewed outcome.")
	if err != nil {
		t.Fatal(err)
	}
	withoutCriterion := removeSource(pack.Sources(), "criterion:CAP-1/CAP-1-REV-2#AC-5")
	mutations := map[string]func() (Proposal, error){
		"content": func() (Proposal, error) {
			return NewProposal(pack, changedContent, base.Rationale(), base.Sources())
		},
		"rationale": func() (Proposal, error) {
			return NewProposal(pack, base.Content(), "A different rationale.", base.Sources())
		},
		"sources": func() (Proposal, error) {
			return NewProposal(pack, base.Content(), base.Rationale(), withoutCriterion)
		},
		"context_digest": func() (Proposal, error) {
			return newProposal(base.Content(), base.Rationale(), base.Sources(), engineering.ComputeDigest([]byte("different context")), true)
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changed, err := mutate()
			if err != nil {
				t.Fatal(err)
			}
			if changed.ProposalDigest().Equal(base.ProposalDigest()) {
				t.Fatal("bound-field mutation did not change ProposalDigest")
			}
		})
	}
}

func TestProposalRejectsDuplicateUnknownMissingAndUnpairedSources(t *testing.T) {
	pack := canonicalFixture(t, false)
	content := pack.Capability().Content()
	all := pack.Sources()
	duplicate := append(append([]SourceReference(nil), all...), all[0])
	if _, err := NewProposal(pack, content, "rationale", duplicate); !errors.Is(err, ErrInvalidProposal) {
		t.Fatalf("duplicate sources error = %v", err)
	}
	unknown := append([]SourceReference(nil), all...)
	unknownReference, err := ParseSourceReference("revision:CAP-OTHER/REV-1")
	if err != nil {
		t.Fatal(err)
	}
	unknown = append(unknown, unknownReference)
	if _, err := NewProposal(pack, content, "rationale", unknown); !errors.Is(err, ErrInvalidProposal) {
		t.Fatalf("unknown source error = %v", err)
	}
	missingCurrent := removeSource(all, "revision:CAP-1/CAP-1-REV-2")
	if _, err := NewProposal(pack, content, "rationale", missingCurrent); !errors.Is(err, ErrInvalidProposal) {
		t.Fatalf("missing current source error = %v", err)
	}
	unpaired := removeSource(all, "revision:CAP-1/CAP-1-REV-1")
	if _, err := NewProposal(pack, content, "rationale", unpaired); !errors.Is(err, ErrInvalidProposal) {
		t.Fatalf("unpaired Decision source error = %v", err)
	}
}

func TestDeterministicGeneratorIsByteStableAndNamesGaps(t *testing.T) {
	pack := canonicalFixture(t, false)
	generator := NewDeterministicGenerator()
	first, err := generator.Generate(pack)
	if err != nil {
		t.Fatal(err)
	}
	second, err := generator.Generate(pack)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, _ := first.CanonicalJSON()
	secondJSON, _ := second.CanonicalJSON()
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatal("equal context generated different proposal bytes")
	}
	for _, witness := range []string{"AC-4", "AC-5", "AC-2=not-satisfied", "AC-3=inconclusive"} {
		if !strings.Contains(first.Rationale(), witness) {
			t.Errorf("rationale %q does not name %q", first.Rationale(), witness)
		}
	}
	if !slices.Equal(first.Sources(), pack.Sources()) {
		t.Fatal("deterministic generator did not cite the complete ContextPack source set")
	}
	if _, err := generator.Generate(ContextPack{}); !errors.Is(err, ErrInvalidContextPack) {
		t.Fatalf("zero pack error = %v", err)
	}
}

func TestDeterministicGeneratorHandlesEveryValidCriterionAtMaximumCardinality(t *testing.T) {
	criteria := make([]engineering.AcceptanceCriterion, 64)
	uncovered := make([]UncoveredCriterion, 64)
	current := revision("CAP-MAX-CONTEXT", "CAP-MAX-CONTEXT-REV-1")
	for index := range criteria {
		key := fmt.Sprintf("AC-%02d-%s", index+1, strings.Repeat("X", 96))
		criteria[index] = mustCriterion(t, key, "A valid criterion at the governed list boundary.")
		uncovered[index] = mustUncovered(
			t,
			current,
			key,
			criteria[index].Text(),
			UncoveredNoRequirementTrace,
			nil,
		)
	}
	content := mustContent(t, criteria, nil)
	capability, err := NewCapabilityContext(current, 1, content)
	if err != nil {
		t.Fatal(err)
	}
	pack, err := NewContextPack(capability, nil, nil, nil, nil, uncovered, nil)
	if err != nil {
		t.Fatalf("NewContextPack(maximum valid criteria): %v", err)
	}

	generated, err := NewDeterministicGenerator().Generate(pack)
	if err != nil {
		t.Fatalf("Generate(maximum valid criteria): %v", err)
	}
	if err := generated.ValidateAgainst(pack); err != nil {
		t.Fatalf("generated Proposal.ValidateAgainst: %v", err)
	}
	if len(generated.Rationale()) <= 4000 {
		t.Fatalf("boundary fixture did not exceed the former invalid 4000-byte limit: %d", len(generated.Rationale()))
	}
	for _, criterion := range []engineering.AcceptanceCriterion{criteria[0], criteria[len(criteria)-1]} {
		if !strings.Contains(generated.Rationale(), criterion.Key()) {
			t.Fatalf("rationale omitted boundary criterion %q", criterion.Key())
		}
	}
}

func canonicalFixture(t *testing.T, permuted bool) ContextPack {
	t.Helper()
	criteria := []engineering.AcceptanceCriterion{
		mustCriterion(t, "AC-1", "Upload audio."),
		mustCriterion(t, "AC-2", "Store by content address."),
		mustCriterion(t, "AC-3", "Expose validation evidence."),
		mustCriterion(t, "AC-4", "Notify the learner."),
		mustCriterion(t, "AC-5", "Retain an audit witness."),
	}
	content := mustContent(t, criteria, []string{"How long is audio retained?"})
	current := revision("CAP-1", "CAP-1-REV-2")
	capability, err := NewCapabilityContext(current, 2, content)
	if err != nil {
		t.Fatal(err)
	}
	requirements := []RequirementContext{
		mustRequirement(t, "REQ-1", "REQ-1-REV-1", 1, "Audio can be uploaded.", current, "AC-1"),
		mustRequirement(t, "REQ-2", "REQ-2-REV-1", 1, "Audio uses content addressing.", current, "AC-2"),
		mustRequirement(t, "REQ-3", "REQ-3-REV-1", 1, "Evidence is visible.", current, "AC-3"),
		mustRequirement(t, "REQ-4", "REQ-4-REV-1", 1, "Learners are notified.", current, "AC-4"),
	}
	claims := []ClaimContext{
		mustClaim(t, "CLM-1", requirements[0].Revision(), current, "satisfied", "Upload passed.", "EXEC-1", "EVID-1"),
		mustClaim(t, "CLM-2", requirements[1].Revision(), current, "not-satisfied", "Digest mismatch.", "EXEC-2", "EVID-2"),
		mustClaim(t, "CLM-3", requirements[2].Revision(), current, "inconclusive", "Evidence was incomplete.", "EXEC-3", "EVID-3"),
	}
	oldSubject, err := NewRevisionDecisionSubject(revision("CAP-1", "CAP-1-REV-1"))
	if err != nil {
		t.Fatal(err)
	}
	artifactSubject, err := NewArtifactDecisionSubject("CAP-1")
	if err != nil {
		t.Fatal(err)
	}
	decisions := []DecisionContext{
		mustDecision(t, "DEC-OLD", oldSubject, "Use content addressing.", 1),
		mustDecision(t, "DEC-ART", artifactSubject, "Retain exact engineering sources.", 2),
	}
	questions := []OpenQuestionContext{mustQuestion(t, current, 1, "How long is audio retained?")}
	uncovered := []UncoveredCriterion{
		mustUncovered(t, current, "AC-4", "Notify the learner.", UncoveredMissingCurrentClaim, []engineering.RevisionKey{requirements[3].Revision()}),
		mustUncovered(t, current, "AC-5", "Retain an audit witness.", UncoveredNoRequirementTrace, nil),
	}
	findings := []ValidationFinding{
		mustFinding(t, current, "AC-2", requirements[1].Revision(), claims[1].Record(), FindingNotSatisfied, claims[1].Reasoning()),
		mustFinding(t, current, "AC-3", requirements[2].Revision(), claims[2].Record(), FindingInconclusive, claims[2].Reasoning()),
	}
	if permuted {
		requirements = []RequirementContext{requirements[3], requirements[1], requirements[0], requirements[2]}
		claims = []ClaimContext{claims[2], claims[0], claims[1]}
		decisions = []DecisionContext{decisions[1], decisions[0]}
		uncovered = []UncoveredCriterion{uncovered[1], uncovered[0]}
		findings = []ValidationFinding{findings[1], findings[0]}
	}
	pack, err := NewContextPack(capability, requirements, claims, decisions, questions, uncovered, findings)
	if err != nil {
		t.Fatal(err)
	}
	return pack
}

func mustContent(t *testing.T, criteria []engineering.AcceptanceCriterion, questions []string) engineering.CapabilitySpecificationContent {
	t.Helper()
	content, err := engineering.NewCapabilitySpecificationContent(1, "Audio homework", "Learners need durable audio submission.")
	if err != nil {
		t.Fatal(err)
	}
	content, err = content.WithUserOutcome("Teachers can review audio.")
	if err != nil {
		t.Fatal(err)
	}
	if criteria != nil {
		content, err = content.WithAcceptanceCriteria(criteria)
		if err != nil {
			t.Fatal(err)
		}
	}
	if questions != nil {
		content, err = content.WithOpenQuestions(questions)
		if err != nil {
			t.Fatal(err)
		}
	}
	return content
}

func mustCriterion(t *testing.T, key, text string) engineering.AcceptanceCriterion {
	t.Helper()
	criterion, err := engineering.NewAcceptanceCriterion(key, text)
	if err != nil {
		t.Fatal(err)
	}
	return criterion
}

func revision(artifactID, revisionID string) engineering.RevisionKey {
	return engineering.RevisionKey{ArtifactID: artifactID, RevisionID: revisionID}
}

func mustRequirement(t *testing.T, artifactID, revisionID string, sequence int, statement string, source engineering.RevisionKey, criterion string) RequirementContext {
	t.Helper()
	requirement, err := NewRequirementContext(revision(artifactID, revisionID), sequence, statement, source, criterion)
	if err != nil {
		t.Fatal(err)
	}
	return requirement
}

func mustClaim(t *testing.T, id string, requirement, capability engineering.RevisionKey, outcome, reasoning, executionID, evidenceID string) ClaimContext {
	t.Helper()
	criterion, err := engineering.RequirementCriterionKey(requirement)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := NewClaimContext(
		engineering.RecordKey{Kind: engineering.RecordKindClaim, ID: id}, requirement, capability, capability.ArtifactID,
		[]string{criterion}, outcome, reasoning,
		[]engineering.RecordKey{{Kind: engineering.RecordKindExecution, ID: executionID}},
		[]engineering.RevisionKey{revision(evidenceID, evidenceID+"-REV-1")},
	)
	if err != nil {
		t.Fatal(err)
	}
	return claim
}

func mustDecision(t *testing.T, id string, subject DecisionSubject, outcome string, ordinal int) DecisionContext {
	t.Helper()
	decision, err := NewDecisionContext(id, subject, outcome, ordinal)
	if err != nil {
		t.Fatal(err)
	}
	return decision
}

func mustQuestion(t *testing.T, capability engineering.RevisionKey, ordinal int, text string) OpenQuestionContext {
	t.Helper()
	question, err := NewOpenQuestionContext(capability, ordinal, text)
	if err != nil {
		t.Fatal(err)
	}
	return question
}

func mustUncovered(t *testing.T, capability engineering.RevisionKey, key, text string, reason UncoveredReason, requirements []engineering.RevisionKey) UncoveredCriterion {
	t.Helper()
	uncovered, err := NewUncoveredCriterion(capability, key, text, reason, requirements)
	if err != nil {
		t.Fatal(err)
	}
	return uncovered
}

func mustFinding(t *testing.T, capability engineering.RevisionKey, criterion string, requirement engineering.RevisionKey, claim engineering.RecordKey, outcome FindingOutcome, reasoning string) ValidationFinding {
	t.Helper()
	finding, err := NewValidationFinding(capability, criterion, requirement, claim, outcome, reasoning)
	if err != nil {
		t.Fatal(err)
	}
	return finding
}

func assertSourcePresent(t *testing.T, sources []SourceReference, want string) {
	t.Helper()
	for _, source := range sources {
		if source.String() == want {
			return
		}
	}
	t.Errorf("sources do not contain %q", want)
}

func removeSource(sources []SourceReference, value string) []SourceReference {
	result := make([]SourceReference, 0, len(sources))
	for _, source := range sources {
		if source.String() != value {
			result = append(result, source)
		}
	}
	return result
}
