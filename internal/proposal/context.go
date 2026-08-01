package proposal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"sort"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

// ContextPack is the immutable, exact, source-bearing input to a proposal
// Generator. Its zero value is invalid.
type ContextPack struct {
	capability        CapabilityContext
	requirements      []RequirementContext
	claims            []ClaimContext
	decisions         []DecisionContext
	openQuestions     []OpenQuestionContext
	uncoveredCriteria []UncoveredCriterion
	findings          []ValidationFinding
	sources           []SourceReference
	contextDigest     engineering.Digest
}

// NewContextPack validates cross-row invariants, normalizes every governed
// ordering, derives the exact source union, and computes the context digest.
func NewContextPack(capability CapabilityContext, requirements []RequirementContext, claims []ClaimContext, decisions []DecisionContext, openQuestions []OpenQuestionContext, uncoveredCriteria []UncoveredCriterion, findings []ValidationFinding) (ContextPack, error) {
	pack := ContextPack{
		capability:        capability,
		requirements:      append([]RequirementContext(nil), requirements...),
		claims:            append([]ClaimContext(nil), claims...),
		decisions:         append([]DecisionContext(nil), decisions...),
		openQuestions:     append([]OpenQuestionContext(nil), openQuestions...),
		uncoveredCriteria: cloneUncovered(uncoveredCriteria),
		findings:          append([]ValidationFinding(nil), findings...),
	}
	pack.normalizeEmptySlices()
	if err := pack.validateAndNormalize(); err != nil {
		return ContextPack{}, err
	}
	pack.sources = pack.deriveSources()
	body, err := pack.canonicalBodyJSON()
	if err != nil {
		return ContextPack{}, fmt.Errorf("%w: encode canonical body: %v", ErrInvalidContextPack, err)
	}
	pack.contextDigest = engineering.ComputeDigest(body)
	return pack, nil
}

func cloneUncovered(values []UncoveredCriterion) []UncoveredCriterion {
	if len(values) == 0 {
		return []UncoveredCriterion{}
	}
	out := cloneValues(values)
	for index := range out {
		out[index].requirementRevisions = copyRevisionKeys(out[index].requirementRevisions)
	}
	return out
}

func (p *ContextPack) normalizeEmptySlices() {
	if p.requirements == nil {
		p.requirements = []RequirementContext{}
	}
	if p.claims == nil {
		p.claims = []ClaimContext{}
	}
	if p.decisions == nil {
		p.decisions = []DecisionContext{}
	}
	if p.openQuestions == nil {
		p.openQuestions = []OpenQuestionContext{}
	}
	if p.uncoveredCriteria == nil {
		p.uncoveredCriteria = []UncoveredCriterion{}
	}
	if p.findings == nil {
		p.findings = []ValidationFinding{}
	}
}

func (p *ContextPack) validateAndNormalize() error {
	if p.capability.isZero() {
		return invalidContext("capability", "must not be zero")
	}
	root := p.capability.revision
	criteria := p.capability.content.AcceptanceCriteria()
	criterionOrder := make(map[string]int, len(criteria))
	criterionText := make(map[string]string, len(criteria))
	for index, criterion := range criteria {
		criterionOrder[criterion.Key()] = index
		criterionText[criterion.Key()] = criterion.Text()
	}

	sort.Slice(p.requirements, func(i, j int) bool {
		return compareKeys(p.requirements[i].revision, p.requirements[j].revision) < 0
	})
	requirementByKey := make(map[engineering.RevisionKey]RequirementContext, len(p.requirements))
	for index, requirement := range p.requirements {
		if requirement.isZero() {
			return invalidContext(fmt.Sprintf("requirements[%d]", index), "must not be zero")
		}
		if _, duplicate := requirementByKey[requirement.revision]; duplicate {
			return invalidContext("requirements", "must not contain duplicate revisions")
		}
		if requirement.sourceCapabilityRevision.ArtifactID != root.ArtifactID {
			return invalidContext(fmt.Sprintf("requirements[%d]", index), "trace must name a Revision of the rooted capability Artifact")
		}
		if requirement.sourceCapabilityRevision == root {
			if _, exists := criterionOrder[requirement.sourceCriterionKey]; !exists {
				return invalidContext(fmt.Sprintf("requirements[%d]", index), "names no criterion on the current capability revision")
			}
		}
		requirementByKey[requirement.revision] = requirement
	}

	sort.Slice(p.claims, func(i, j int) bool {
		if compared := compareKeys(p.claims[i].requirementRevision, p.claims[j].requirementRevision); compared != 0 {
			return compared < 0
		}
		return p.claims[i].record.ID < p.claims[j].record.ID
	})
	claimByRequirement := make(map[engineering.RevisionKey]ClaimContext, len(p.claims))
	claimByRecord := make(map[engineering.RecordKey]ClaimContext, len(p.claims))
	for index, claim := range p.claims {
		if claim.isZero() {
			return invalidContext(fmt.Sprintf("claims[%d]", index), "must not be zero")
		}
		if claim.capabilityRevision != root || claim.scopeArtifactID != root.ArtifactID {
			return invalidContext(fmt.Sprintf("claims[%d]", index), "must name the exact current capability revision and scope artifact")
		}
		if _, exists := requirementByKey[claim.requirementRevision]; !exists {
			return invalidContext(fmt.Sprintf("claims[%d]", index), "names no effective Requirement revision")
		}
		criterion, err := engineering.RequirementCriterionKey(claim.requirementRevision)
		if err != nil || len(claim.criterionKeys) != 1 || claim.criterionKeys[0] != criterion {
			return invalidContext(fmt.Sprintf("claims[%d]", index), "must carry the exact Requirement criterion key")
		}
		if _, duplicate := claimByRequirement[claim.requirementRevision]; duplicate {
			return invalidContext("claims", "must contain at most one current Claim per Requirement")
		}
		if _, duplicate := claimByRecord[claim.record]; duplicate {
			return invalidContext("claims", "must not contain duplicate Claim records")
		}
		claimByRequirement[claim.requirementRevision] = claim
		claimByRecord[claim.record] = claim
	}

	sort.SliceStable(p.decisions, func(i, j int) bool {
		if p.decisions[i].ordinal != p.decisions[j].ordinal {
			return p.decisions[i].ordinal < p.decisions[j].ordinal
		}
		return p.decisions[i].decisionID < p.decisions[j].decisionID
	})
	decisionIDs := make(map[string]struct{}, len(p.decisions))
	decisionOrdinals := make(map[int]struct{}, len(p.decisions))
	for index, decision := range p.decisions {
		if decision.isZero() {
			return invalidContext(fmt.Sprintf("decisions[%d]", index), "must not be zero")
		}
		if decision.subject.artifactID != root.ArtifactID {
			return invalidContext(fmt.Sprintf("decisions[%d]", index), "must name the capability Artifact or one of its Revisions")
		}
		if _, duplicate := decisionIDs[decision.decisionID]; duplicate {
			return invalidContext("decisions", "must not contain duplicate Decision identities")
		}
		if _, duplicate := decisionOrdinals[decision.ordinal]; duplicate {
			return invalidContext("decisions", "must not contain duplicate canonical ordinals")
		}
		decisionIDs[decision.decisionID] = struct{}{}
		decisionOrdinals[decision.ordinal] = struct{}{}
	}

	sort.Slice(p.openQuestions, func(i, j int) bool { return p.openQuestions[i].ordinal < p.openQuestions[j].ordinal })
	questions := p.capability.content.OpenQuestions()
	if len(p.openQuestions) != len(questions) {
		return invalidContext("open questions", "must exactly reproduce current capability content")
	}
	for index, question := range p.openQuestions {
		if question.isZero() || question.capabilityRevision != root || question.ordinal != index+1 || question.text != questions[index] {
			return invalidContext(fmt.Sprintf("open_questions[%d]", index), "must reproduce the exact current-content ordinal and text")
		}
	}

	for index := range p.uncoveredCriteria {
		sort.Slice(p.uncoveredCriteria[index].requirementRevisions, func(i, j int) bool {
			return compareKeys(p.uncoveredCriteria[index].requirementRevisions[i], p.uncoveredCriteria[index].requirementRevisions[j]) < 0
		})
	}
	sort.Slice(p.uncoveredCriteria, func(i, j int) bool {
		return criterionOrder[p.uncoveredCriteria[i].criterionKey] < criterionOrder[p.uncoveredCriteria[j].criterionKey]
	})
	if err := validateUncovered(p.uncoveredCriteria, root, criteria, criterionText, requirementByKey, claimByRequirement); err != nil {
		return err
	}

	sort.Slice(p.findings, func(i, j int) bool {
		leftOrder := criterionOrder[p.findings[i].criterionKey]
		rightOrder := criterionOrder[p.findings[j].criterionKey]
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		if compared := compareKeys(p.findings[i].requirementRevision, p.findings[j].requirementRevision); compared != 0 {
			return compared < 0
		}
		return p.findings[i].claimRecord.ID < p.findings[j].claimRecord.ID
	})
	if err := validateFindings(p.findings, root, criterionOrder, requirementByKey, claimByRequirement); err != nil {
		return err
	}
	return nil
}

func compareKeys(left, right engineering.RevisionKey) int {
	return compareRevisionKeys(keyWire(left), keyWire(right))
}

func validateUncovered(actual []UncoveredCriterion, root engineering.RevisionKey, criteria []engineering.AcceptanceCriterion, criterionText map[string]string, requirements map[engineering.RevisionKey]RequirementContext, claims map[engineering.RevisionKey]ClaimContext) error {
	expected := make(map[string]UncoveredCriterion)
	for _, criterion := range criteria {
		mapped := make([]engineering.RevisionKey, 0)
		missing := make([]engineering.RevisionKey, 0)
		for key, requirement := range requirements {
			if requirement.sourceCapabilityRevision != root || requirement.sourceCriterionKey != criterion.Key() {
				continue
			}
			mapped = append(mapped, key)
			if _, found := claims[key]; !found {
				missing = append(missing, key)
			}
		}
		sort.Slice(mapped, func(i, j int) bool { return compareKeys(mapped[i], mapped[j]) < 0 })
		sort.Slice(missing, func(i, j int) bool { return compareKeys(missing[i], missing[j]) < 0 })
		switch {
		case len(mapped) == 0:
			expected[criterion.Key()] = UncoveredCriterion{
				capabilityRevision: root, criterionKey: criterion.Key(), criterionText: criterion.Text(),
				reason: UncoveredNoRequirementTrace, requirementRevisions: []engineering.RevisionKey{},
			}
		case len(missing) > 0:
			expected[criterion.Key()] = UncoveredCriterion{
				capabilityRevision: root, criterionKey: criterion.Key(), criterionText: criterion.Text(),
				reason: UncoveredMissingCurrentClaim, requirementRevisions: missing,
			}
		}
	}
	if len(actual) != len(expected) {
		return invalidContext("uncovered criteria", "do not equal the computed FF-024 coverage gaps")
	}
	seen := make(map[string]struct{}, len(actual))
	for index, uncovered := range actual {
		if uncovered.isZero() || uncovered.capabilityRevision != root {
			return invalidContext(fmt.Sprintf("uncovered_criteria[%d]", index), "must name the exact current capability revision")
		}
		if text, exists := criterionText[uncovered.criterionKey]; !exists || text != uncovered.criterionText {
			return invalidContext(fmt.Sprintf("uncovered_criteria[%d]", index), "must name an exact current criterion and text")
		}
		if _, duplicate := seen[uncovered.criterionKey]; duplicate {
			return invalidContext("uncovered criteria", "must not contain duplicate criterion keys")
		}
		want, exists := expected[uncovered.criterionKey]
		if !exists || want.reason != uncovered.reason || !slices.Equal(want.requirementRevisions, uncovered.requirementRevisions) {
			return invalidContext(fmt.Sprintf("uncovered_criteria[%d]", index), "does not equal the computed FF-024 classification")
		}
		seen[uncovered.criterionKey] = struct{}{}
	}
	return nil
}

func validateFindings(actual []ValidationFinding, root engineering.RevisionKey, criterionOrder map[string]int, requirements map[engineering.RevisionKey]RequirementContext, claims map[engineering.RevisionKey]ClaimContext) error {
	type findingKey struct {
		criterion   string
		requirement engineering.RevisionKey
	}
	expected := make(map[findingKey]ValidationFinding)
	for requirementKey, claim := range claims {
		requirement := requirements[requirementKey]
		if requirement.sourceCapabilityRevision != root {
			continue
		}
		var outcome FindingOutcome
		switch claim.outcome {
		case "not-satisfied":
			outcome = FindingNotSatisfied
		case "inconclusive":
			outcome = FindingInconclusive
		default:
			continue
		}
		key := findingKey{criterion: requirement.sourceCriterionKey, requirement: requirementKey}
		expected[key] = ValidationFinding{
			capabilityRevision: root, criterionKey: requirement.sourceCriterionKey,
			requirementRevision: requirementKey, claimRecord: claim.record, outcome: outcome, reasoning: claim.reasoning,
		}
	}
	if len(actual) != len(expected) {
		return invalidContext("findings", "do not equal the computed negative and inconclusive current Claims")
	}
	seen := make(map[findingKey]struct{}, len(actual))
	for index, finding := range actual {
		if finding.isZero() || finding.capabilityRevision != root {
			return invalidContext(fmt.Sprintf("findings[%d]", index), "must name the exact current capability revision")
		}
		if _, exists := criterionOrder[finding.criterionKey]; !exists {
			return invalidContext(fmt.Sprintf("findings[%d]", index), "names no current criterion")
		}
		key := findingKey{criterion: finding.criterionKey, requirement: finding.requirementRevision}
		if _, duplicate := seen[key]; duplicate {
			return invalidContext("findings", "must not contain duplicate criterion/Requirement pairs")
		}
		want, exists := expected[key]
		if !exists || want.claimRecord != finding.claimRecord || want.outcome != finding.outcome || want.reasoning != finding.reasoning {
			return invalidContext(fmt.Sprintf("findings[%d]", index), "does not equal its current Claim")
		}
		seen[key] = struct{}{}
	}
	return nil
}

func (p ContextPack) deriveSources() []SourceReference {
	byValue := map[string]SourceReference{}
	add := func(reference SourceReference) { byValue[reference.String()] = reference }
	add(revisionSource(p.capability.revision))
	for _, criterion := range p.capability.content.AcceptanceCriteria() {
		add(criterionSource(p.capability.revision, criterion.Key()))
	}
	for _, requirement := range p.requirements {
		add(revisionSource(requirement.revision))
		add(requirementTraceSource(requirement.revision))
		add(revisionSource(requirement.sourceCapabilityRevision))
		add(criterionSource(requirement.sourceCapabilityRevision, requirement.sourceCriterionKey))
	}
	for _, claim := range p.claims {
		add(recordSource(claim.record))
		for _, execution := range claim.executionReferences {
			add(recordSource(execution))
		}
		for _, evidence := range claim.evidenceReferences {
			add(revisionSource(evidence))
		}
	}
	for _, decision := range p.decisions {
		add(recordSource(engineering.RecordKey{Kind: engineering.RecordKindDecision, ID: decision.decisionID}))
		add(decision.subject.source())
	}
	values := make([]SourceReference, 0, len(byValue))
	for _, reference := range byValue {
		values = append(values, reference)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].String() < values[j].String() })
	return values
}

// IsZero reports whether p is the invalid zero ContextPack.
func (p ContextPack) IsZero() bool { return p.capability.isZero() || p.contextDigest.IsZero() }

// Validate recomputes the body digest and checks the immutable value.
func (p ContextPack) Validate() error {
	if p.IsZero() {
		return invalidContext("context pack", "must not be zero")
	}
	body, err := p.canonicalBodyJSON()
	if err != nil {
		return invalidContext("context pack", err.Error())
	}
	if !engineering.ComputeDigest(body).Equal(p.contextDigest) {
		return invalidContext("context digest", "does not match the canonical context body")
	}
	return nil
}

func (p ContextPack) Capability() CapabilityContext { return p.capability }

func (p ContextPack) Requirements() []RequirementContext {
	return cloneValues(p.requirements)
}

func (p ContextPack) Claims() []ClaimContext {
	return cloneValues(p.claims)
}

func (p ContextPack) Decisions() []DecisionContext {
	return cloneValues(p.decisions)
}

func (p ContextPack) OpenQuestions() []OpenQuestionContext {
	return cloneValues(p.openQuestions)
}

func (p ContextPack) UncoveredCriteria() []UncoveredCriterion {
	return cloneUncovered(p.uncoveredCriteria)
}

func (p ContextPack) Findings() []ValidationFinding {
	return cloneValues(p.findings)
}

func (p ContextPack) Sources() []SourceReference {
	return cloneValues(p.sources)
}

func (p ContextPack) ContextDigest() engineering.Digest { return p.contextDigest }

type revisionKeyWire struct {
	ArtifactID string `json:"artifact_id"`
	RevisionID string `json:"revision_id"`
}

type recordKeyWire struct {
	Kind     string `json:"kind"`
	RecordID string `json:"record_id"`
}

type capabilityWire struct {
	Revision      revisionKeyWire `json:"revision"`
	Sequence      int             `json:"sequence"`
	Content       json.RawMessage `json:"content"`
	ContentDigest string          `json:"content_digest"`
}

type requirementWire struct {
	Revision                 revisionKeyWire `json:"revision"`
	Sequence                 int             `json:"sequence"`
	Statement                string          `json:"statement"`
	SourceCapabilityRevision revisionKeyWire `json:"source_capability_revision"`
	SourceCriterionKey       string          `json:"source_criterion_key"`
}

type claimWire struct {
	Record              recordKeyWire     `json:"record"`
	RequirementRevision revisionKeyWire   `json:"requirement_revision"`
	CapabilityRevision  revisionKeyWire   `json:"capability_revision"`
	ScopeArtifactID     string            `json:"scope_artifact_id"`
	CriterionKeys       []string          `json:"criterion_keys"`
	Outcome             string            `json:"outcome"`
	Reasoning           string            `json:"reasoning"`
	ExecutionReferences []recordKeyWire   `json:"execution_references"`
	EvidenceReferences  []revisionKeyWire `json:"evidence_references"`
}

type decisionSubjectWire struct {
	Kind       string `json:"kind"`
	ArtifactID string `json:"artifact_id"`
	RevisionID string `json:"revision_id"`
}

type decisionWire struct {
	DecisionID       string              `json:"decision_id"`
	Subject          decisionSubjectWire `json:"subject"`
	OutcomeStatement string              `json:"outcome_statement"`
}

type openQuestionWire struct {
	CapabilityRevision revisionKeyWire `json:"capability_revision"`
	Ordinal            int             `json:"ordinal"`
	Text               string          `json:"text"`
}

type uncoveredCriterionWire struct {
	CapabilityRevision   revisionKeyWire   `json:"capability_revision"`
	CriterionKey         string            `json:"criterion_key"`
	CriterionText        string            `json:"criterion_text"`
	Reason               string            `json:"reason"`
	RequirementRevisions []revisionKeyWire `json:"requirement_revisions"`
}

type findingWire struct {
	CapabilityRevision  revisionKeyWire `json:"capability_revision"`
	CriterionKey        string          `json:"criterion_key"`
	RequirementRevision revisionKeyWire `json:"requirement_revision"`
	ClaimRecord         recordKeyWire   `json:"claim_record"`
	Outcome             string          `json:"outcome"`
	Reasoning           string          `json:"reasoning"`
}

type contextBodyWire struct {
	Capability        capabilityWire           `json:"capability"`
	Requirements      []requirementWire        `json:"requirements"`
	Claims            []claimWire              `json:"claims"`
	Decisions         []decisionWire           `json:"decisions"`
	OpenQuestions     []openQuestionWire       `json:"open_questions"`
	UncoveredCriteria []uncoveredCriterionWire `json:"uncovered_criteria"`
	Findings          []findingWire            `json:"findings"`
	Sources           []string                 `json:"sources"`
}

type contextWire struct {
	Capability        capabilityWire           `json:"capability"`
	Requirements      []requirementWire        `json:"requirements"`
	Claims            []claimWire              `json:"claims"`
	Decisions         []decisionWire           `json:"decisions"`
	OpenQuestions     []openQuestionWire       `json:"open_questions"`
	UncoveredCriteria []uncoveredCriterionWire `json:"uncovered_criteria"`
	Findings          []findingWire            `json:"findings"`
	Sources           []string                 `json:"sources"`
	ContextDigest     string                   `json:"context_digest"`
}

func keyWire(key engineering.RevisionKey) revisionKeyWire {
	return revisionKeyWire{ArtifactID: key.ArtifactID, RevisionID: key.RevisionID}
}

func recordWire(key engineering.RecordKey) recordKeyWire {
	return recordKeyWire{Kind: string(key.Kind), RecordID: key.ID}
}

func (p ContextPack) bodyWire() (contextBodyWire, error) {
	content, err := p.capability.content.CanonicalJSON()
	if err != nil {
		return contextBodyWire{}, err
	}
	requirements := make([]requirementWire, len(p.requirements))
	for index, requirement := range p.requirements {
		requirements[index] = requirementWire{
			Revision: keyWire(requirement.revision), Sequence: requirement.sequence, Statement: requirement.statement,
			SourceCapabilityRevision: keyWire(requirement.sourceCapabilityRevision), SourceCriterionKey: requirement.sourceCriterionKey,
		}
	}
	claims := make([]claimWire, len(p.claims))
	for index, claim := range p.claims {
		executions := make([]recordKeyWire, len(claim.executionReferences))
		for i, execution := range claim.executionReferences {
			executions[i] = recordWire(execution)
		}
		evidence := make([]revisionKeyWire, len(claim.evidenceReferences))
		for i, reference := range claim.evidenceReferences {
			evidence[i] = keyWire(reference)
		}
		claims[index] = claimWire{
			Record: recordWire(claim.record), RequirementRevision: keyWire(claim.requirementRevision),
			CapabilityRevision: keyWire(claim.capabilityRevision), ScopeArtifactID: claim.scopeArtifactID,
			CriterionKeys: nonNilStrings(claim.criterionKeys), Outcome: claim.outcome, Reasoning: claim.reasoning,
			ExecutionReferences: executions, EvidenceReferences: evidence,
		}
	}
	decisions := make([]decisionWire, len(p.decisions))
	for index, decision := range p.decisions {
		decisions[index] = decisionWire{
			DecisionID:       decision.decisionID,
			Subject:          decisionSubjectWire{Kind: string(decision.subject.kind), ArtifactID: decision.subject.artifactID, RevisionID: decision.subject.revisionID},
			OutcomeStatement: decision.outcomeStatement,
		}
	}
	questions := make([]openQuestionWire, len(p.openQuestions))
	for index, question := range p.openQuestions {
		questions[index] = openQuestionWire{CapabilityRevision: keyWire(question.capabilityRevision), Ordinal: question.ordinal, Text: question.text}
	}
	uncovered := make([]uncoveredCriterionWire, len(p.uncoveredCriteria))
	for index, item := range p.uncoveredCriteria {
		revisions := make([]revisionKeyWire, len(item.requirementRevisions))
		for i, revision := range item.requirementRevisions {
			revisions[i] = keyWire(revision)
		}
		uncovered[index] = uncoveredCriterionWire{
			CapabilityRevision: keyWire(item.capabilityRevision), CriterionKey: item.criterionKey,
			CriterionText: item.criterionText, Reason: string(item.reason), RequirementRevisions: revisions,
		}
	}
	findings := make([]findingWire, len(p.findings))
	for index, finding := range p.findings {
		findings[index] = findingWire{
			CapabilityRevision: keyWire(finding.capabilityRevision), CriterionKey: finding.criterionKey,
			RequirementRevision: keyWire(finding.requirementRevision), ClaimRecord: recordWire(finding.claimRecord),
			Outcome: string(finding.outcome), Reasoning: finding.reasoning,
		}
	}
	sources := make([]string, len(p.sources))
	for index, source := range p.sources {
		sources[index] = source.String()
	}
	return contextBodyWire{
		Capability:   capabilityWire{Revision: keyWire(p.capability.revision), Sequence: p.capability.sequence, Content: content, ContentDigest: p.capability.contentDigest.Hex()},
		Requirements: requirements, Claims: claims, Decisions: decisions, OpenQuestions: questions,
		UncoveredCriteria: uncovered, Findings: findings, Sources: sources,
	}, nil
}

func (p ContextPack) canonicalBodyJSON() ([]byte, error) {
	body, err := p.bodyWire()
	if err != nil {
		return nil, err
	}
	return canonicalEncode(body)
}

// CanonicalJSON returns the deterministic context encoding including its
// digest. The digest itself is computed over the same body without the final
// context_digest field.
func (p ContextPack) CanonicalJSON() ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	body, err := p.bodyWire()
	if err != nil {
		return nil, err
	}
	return canonicalEncode(contextWire{
		Capability: body.Capability, Requirements: body.Requirements, Claims: body.Claims,
		Decisions: body.Decisions, OpenQuestions: body.OpenQuestions, UncoveredCriteria: body.UncoveredCriteria,
		Findings: body.Findings, Sources: body.Sources, ContextDigest: p.contextDigest.Hex(),
	})
}

// ParseContextPack parses one exact canonical ContextPack and verifies every
// nested invariant, derived source, and digest.
func ParseContextPack(data []byte) (ContextPack, error) {
	var wire contextWire
	if err := decodeStrict(data, &wire); err != nil {
		return ContextPack{}, fmt.Errorf("%w: decode: %v", ErrInvalidContextPack, err)
	}
	content, err := engineering.ParseCapabilitySpecificationContent(wire.Capability.Content)
	if err != nil {
		return ContextPack{}, fmt.Errorf("%w: capability content: %v", ErrInvalidContextPack, err)
	}
	canonicalContent, err := content.CanonicalJSON()
	if err != nil || !bytes.Equal(wire.Capability.Content, canonicalContent) {
		return ContextPack{}, invalidContext("capability content", "must use its exact canonical encoding")
	}
	capability, err := NewCapabilityContext(fromKeyWire(wire.Capability.Revision), wire.Capability.Sequence, content)
	if err != nil {
		return ContextPack{}, err
	}
	if wire.Capability.ContentDigest != capability.contentDigest.Hex() {
		return ContextPack{}, invalidContext("capability content digest", "does not match recomputed content")
	}
	requirements := make([]RequirementContext, len(wire.Requirements))
	for index, item := range wire.Requirements {
		requirements[index], err = NewRequirementContext(fromKeyWire(item.Revision), item.Sequence, item.Statement, fromKeyWire(item.SourceCapabilityRevision), item.SourceCriterionKey)
		if err != nil {
			return ContextPack{}, err
		}
	}
	claims := make([]ClaimContext, len(wire.Claims))
	for index, item := range wire.Claims {
		executions := make([]engineering.RecordKey, len(item.ExecutionReferences))
		for i, reference := range item.ExecutionReferences {
			executions[i] = fromRecordWire(reference)
		}
		evidence := make([]engineering.RevisionKey, len(item.EvidenceReferences))
		for i, reference := range item.EvidenceReferences {
			evidence[i] = fromKeyWire(reference)
		}
		claims[index], err = NewClaimContext(
			fromRecordWire(item.Record), fromKeyWire(item.RequirementRevision), fromKeyWire(item.CapabilityRevision),
			item.ScopeArtifactID, item.CriterionKeys, item.Outcome, item.Reasoning, executions, evidence,
		)
		if err != nil {
			return ContextPack{}, err
		}
	}
	decisions := make([]DecisionContext, len(wire.Decisions))
	for index, item := range wire.Decisions {
		subject, subjectErr := decisionSubjectFromWire(item.Subject)
		if subjectErr != nil {
			return ContextPack{}, subjectErr
		}
		decisions[index], err = NewDecisionContext(item.DecisionID, subject, item.OutcomeStatement, index+1)
		if err != nil {
			return ContextPack{}, err
		}
	}
	questions := make([]OpenQuestionContext, len(wire.OpenQuestions))
	for index, item := range wire.OpenQuestions {
		questions[index], err = NewOpenQuestionContext(fromKeyWire(item.CapabilityRevision), item.Ordinal, item.Text)
		if err != nil {
			return ContextPack{}, err
		}
	}
	uncovered := make([]UncoveredCriterion, len(wire.UncoveredCriteria))
	for index, item := range wire.UncoveredCriteria {
		revisions := make([]engineering.RevisionKey, len(item.RequirementRevisions))
		for i, revision := range item.RequirementRevisions {
			revisions[i] = fromKeyWire(revision)
		}
		uncovered[index], err = NewUncoveredCriterion(fromKeyWire(item.CapabilityRevision), item.CriterionKey, item.CriterionText, UncoveredReason(item.Reason), revisions)
		if err != nil {
			return ContextPack{}, err
		}
	}
	findings := make([]ValidationFinding, len(wire.Findings))
	for index, item := range wire.Findings {
		findings[index], err = NewValidationFinding(
			fromKeyWire(item.CapabilityRevision), item.CriterionKey, fromKeyWire(item.RequirementRevision),
			fromRecordWire(item.ClaimRecord), FindingOutcome(item.Outcome), item.Reasoning,
		)
		if err != nil {
			return ContextPack{}, err
		}
	}
	pack, err := NewContextPack(capability, requirements, claims, decisions, questions, uncovered, findings)
	if err != nil {
		return ContextPack{}, err
	}
	if len(wire.Sources) != len(pack.sources) {
		return ContextPack{}, invalidContext("sources", "must equal the exact derived source union")
	}
	for index, value := range wire.Sources {
		reference, sourceErr := ParseSourceReference(value)
		if sourceErr != nil {
			return ContextPack{}, fmt.Errorf("%w: %v", ErrInvalidContextPack, sourceErr)
		}
		if reference != pack.sources[index] {
			return ContextPack{}, invalidContext("sources", "must be sorted, duplicate-free, and equal the derived source union")
		}
	}
	digest, err := engineering.NewDigest(wire.ContextDigest)
	if err != nil || !digest.Equal(pack.contextDigest) {
		return ContextPack{}, invalidContext("context digest", "does not match the canonical context body")
	}
	canonical, err := pack.CanonicalJSON()
	if err != nil || !bytes.Equal(data, canonical) {
		return ContextPack{}, invalidContext("context JSON", "must be the exact canonical encoding")
	}
	return pack, nil
}

func fromKeyWire(wire revisionKeyWire) engineering.RevisionKey {
	return engineering.RevisionKey{ArtifactID: wire.ArtifactID, RevisionID: wire.RevisionID}
}

func fromRecordWire(wire recordKeyWire) engineering.RecordKey {
	return engineering.RecordKey{Kind: engineering.RecordKind(wire.Kind), ID: wire.RecordID}
}

func decisionSubjectFromWire(wire decisionSubjectWire) (DecisionSubject, error) {
	switch engineering.SubjectKind(wire.Kind) {
	case engineering.SubjectKindArtifact:
		if wire.RevisionID != "" {
			return DecisionSubject{}, invalidContext("decision subject", "Artifact subject must have an empty revision id")
		}
		return NewArtifactDecisionSubject(wire.ArtifactID)
	case engineering.SubjectKindArtifactRevision:
		return NewRevisionDecisionSubject(engineering.RevisionKey{ArtifactID: wire.ArtifactID, RevisionID: wire.RevisionID})
	default:
		return DecisionSubject{}, invalidContext("decision subject kind", "is unsupported")
	}
}
