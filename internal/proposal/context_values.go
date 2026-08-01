package proposal

import (
	"fmt"
	"strings"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

func invalidContext(field, reason string) error {
	return fmt.Errorf("%w: %s %s", ErrInvalidContextPack, field, reason)
}

func validateRevisionKey(field string, key engineering.RevisionKey) error {
	if key.IsZero() || !validIdentity(key.ArtifactID) || !validIdentity(key.RevisionID) {
		return invalidContext(field, "must contain governed artifact and revision identities")
	}
	return nil
}

func validateRecordKey(field string, key engineering.RecordKey, kind engineering.RecordKind) error {
	if key.IsZero() || key.Kind != kind || !validIdentity(key.ID) {
		return invalidContext(field, "has the wrong kind or an invalid identity")
	}
	return nil
}

func requireText(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return invalidContext(field, "must not be empty")
	}
	return nil
}

func copyRevisionKeys(values []engineering.RevisionKey) []engineering.RevisionKey {
	return cloneValues(values)
}

func copyRecordKeys(values []engineering.RecordKey) []engineering.RecordKey {
	return cloneValues(values)
}

// CapabilityContext is the exact current capability revision and decoded
// specification at the root of a ContextPack.
type CapabilityContext struct {
	revision      engineering.RevisionKey
	sequence      int
	content       engineering.CapabilitySpecificationContent
	contentDigest engineering.Digest
}

// NewCapabilityContext validates and captures the current capability value.
func NewCapabilityContext(revision engineering.RevisionKey, sequence int, content engineering.CapabilitySpecificationContent) (CapabilityContext, error) {
	if err := validateRevisionKey("capability revision", revision); err != nil {
		return CapabilityContext{}, err
	}
	if sequence <= 0 {
		return CapabilityContext{}, invalidContext("capability sequence", "must be positive")
	}
	if content.IsZero() {
		return CapabilityContext{}, invalidContext("capability content", "must not be zero")
	}
	digest, err := content.Digest()
	if err != nil {
		return CapabilityContext{}, invalidContext("capability content", err.Error())
	}
	return CapabilityContext{revision: revision, sequence: sequence, content: content, contentDigest: digest}, nil
}

func (c CapabilityContext) isZero() bool { return c.revision.IsZero() || c.content.IsZero() }

// Revision returns the exact current revision key.
func (c CapabilityContext) Revision() engineering.RevisionKey { return c.revision }

// Sequence returns the positive managed revision sequence.
func (c CapabilityContext) Sequence() int { return c.sequence }

// Content returns the immutable capability content.
func (c CapabilityContext) Content() engineering.CapabilitySpecificationContent { return c.content }

// ContentDigest returns the recomputed capability-content digest.
func (c CapabilityContext) ContentDigest() engineering.Digest { return c.contentDigest }

// RequirementContext is one effective Requirement revision and its exact
// AD-033 trace.
type RequirementContext struct {
	revision                 engineering.RevisionKey
	sequence                 int
	statement                string
	sourceCapabilityRevision engineering.RevisionKey
	sourceCriterionKey       string
}

// NewRequirementContext validates and captures one effective Requirement.
func NewRequirementContext(revision engineering.RevisionKey, sequence int, statement string, sourceCapabilityRevision engineering.RevisionKey, sourceCriterionKey string) (RequirementContext, error) {
	if err := validateRevisionKey("requirement revision", revision); err != nil {
		return RequirementContext{}, err
	}
	if sequence <= 0 {
		return RequirementContext{}, invalidContext("requirement sequence", "must be positive")
	}
	if err := requireText("requirement statement", statement); err != nil {
		return RequirementContext{}, err
	}
	if err := validateRevisionKey("requirement source capability revision", sourceCapabilityRevision); err != nil {
		return RequirementContext{}, err
	}
	if err := engineering.ValidateAcceptanceCriterionKey(sourceCriterionKey); err != nil {
		return RequirementContext{}, invalidContext("requirement source criterion", err.Error())
	}
	return RequirementContext{
		revision: revision, sequence: sequence, statement: statement,
		sourceCapabilityRevision: sourceCapabilityRevision, sourceCriterionKey: sourceCriterionKey,
	}, nil
}

func (r RequirementContext) isZero() bool { return r.revision.IsZero() }

func (r RequirementContext) Revision() engineering.RevisionKey { return r.revision }

func (r RequirementContext) Sequence() int { return r.sequence }

func (r RequirementContext) Statement() string { return r.statement }

func (r RequirementContext) SourceCapabilityRevision() engineering.RevisionKey {
	return r.sourceCapabilityRevision
}

func (r RequirementContext) SourceCriterionKey() string { return r.sourceCriterionKey }

// ClaimContext is one unique current validation Claim for an effective
// Requirement against the exact current capability revision.
type ClaimContext struct {
	record              engineering.RecordKey
	requirementRevision engineering.RevisionKey
	capabilityRevision  engineering.RevisionKey
	scopeArtifactID     string
	criterionKeys       []string
	outcome             string
	reasoning           string
	executionReferences []engineering.RecordKey
	evidenceReferences  []engineering.RevisionKey
}

// NewClaimContext validates and defensively captures one current Claim.
func NewClaimContext(record engineering.RecordKey, requirementRevision, capabilityRevision engineering.RevisionKey, scopeArtifactID string, criterionKeys []string, outcome, reasoning string, executionReferences []engineering.RecordKey, evidenceReferences []engineering.RevisionKey) (ClaimContext, error) {
	if err := validateRecordKey("claim record", record, engineering.RecordKindClaim); err != nil {
		return ClaimContext{}, err
	}
	if err := validateRevisionKey("claim requirement revision", requirementRevision); err != nil {
		return ClaimContext{}, err
	}
	if err := validateRevisionKey("claim capability revision", capabilityRevision); err != nil {
		return ClaimContext{}, err
	}
	if !validIdentity(scopeArtifactID) {
		return ClaimContext{}, invalidContext("claim scope artifact id", "must match the governed identity grammar")
	}
	criteria, err := validateUniqueTextList("claim criterion keys", criterionKeys, true)
	if err != nil {
		return ClaimContext{}, err
	}
	switch outcome {
	case "satisfied", "not-satisfied", "inconclusive":
	default:
		return ClaimContext{}, invalidContext("claim outcome", "must be satisfied, not-satisfied, or inconclusive")
	}
	executions := copyRecordKeys(executionReferences)
	seenExecutions := make(map[engineering.RecordKey]struct{}, len(executions))
	for index, reference := range executions {
		if err := validateRecordKey(fmt.Sprintf("claim execution reference[%d]", index), reference, engineering.RecordKindExecution); err != nil {
			return ClaimContext{}, err
		}
		if _, duplicate := seenExecutions[reference]; duplicate {
			return ClaimContext{}, invalidContext("claim execution references", "must not contain duplicates")
		}
		seenExecutions[reference] = struct{}{}
	}
	evidence := copyRevisionKeys(evidenceReferences)
	seenEvidence := make(map[engineering.RevisionKey]struct{}, len(evidence))
	for index, reference := range evidence {
		if err := validateRevisionKey(fmt.Sprintf("claim evidence reference[%d]", index), reference); err != nil {
			return ClaimContext{}, err
		}
		if _, duplicate := seenEvidence[reference]; duplicate {
			return ClaimContext{}, invalidContext("claim evidence references", "must not contain duplicates")
		}
		seenEvidence[reference] = struct{}{}
	}
	return ClaimContext{
		record: record, requirementRevision: requirementRevision, capabilityRevision: capabilityRevision,
		scopeArtifactID: scopeArtifactID, criterionKeys: criteria, outcome: outcome, reasoning: reasoning,
		executionReferences: executions, evidenceReferences: evidence,
	}, nil
}

func validateUniqueTextList(field string, values []string, requireNonEmpty bool) ([]string, error) {
	if requireNonEmpty && len(values) == 0 {
		return nil, invalidContext(field, "must not be empty")
	}
	result := append([]string(nil), values...)
	if result == nil {
		result = []string{}
	}
	seen := make(map[string]struct{}, len(result))
	for index, value := range result {
		if strings.TrimSpace(value) == "" {
			return nil, invalidContext(fmt.Sprintf("%s[%d]", field, index), "must not be empty")
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, invalidContext(field, "must not contain duplicates")
		}
		seen[value] = struct{}{}
	}
	return result, nil
}

func (c ClaimContext) isZero() bool { return c.record.IsZero() }

func (c ClaimContext) Record() engineering.RecordKey { return c.record }

func (c ClaimContext) RequirementRevision() engineering.RevisionKey { return c.requirementRevision }

func (c ClaimContext) CapabilityRevision() engineering.RevisionKey { return c.capabilityRevision }

func (c ClaimContext) ScopeArtifactID() string { return c.scopeArtifactID }

func (c ClaimContext) CriterionKeys() []string { return cloneValues(c.criterionKeys) }

func (c ClaimContext) Outcome() string { return c.outcome }

func (c ClaimContext) Reasoning() string { return c.reasoning }

func (c ClaimContext) ExecutionReferences() []engineering.RecordKey {
	return copyRecordKeys(c.executionReferences)
}

func (c ClaimContext) EvidenceReferences() []engineering.RevisionKey {
	return copyRevisionKeys(c.evidenceReferences)
}

// DecisionSubject is the exact authoritative Artifact or Revision subject of
// an applicable Decision.
type DecisionSubject struct {
	kind       engineering.SubjectKind
	artifactID string
	revisionID string
}

// NewArtifactDecisionSubject returns an Artifact-level Decision subject.
func NewArtifactDecisionSubject(artifactID string) (DecisionSubject, error) {
	if !validIdentity(artifactID) {
		return DecisionSubject{}, invalidContext("decision subject artifact id", "must match the governed identity grammar")
	}
	return DecisionSubject{kind: engineering.SubjectKindArtifact, artifactID: artifactID}, nil
}

// NewRevisionDecisionSubject returns an exact-Revision Decision subject.
func NewRevisionDecisionSubject(revision engineering.RevisionKey) (DecisionSubject, error) {
	if err := validateRevisionKey("decision subject revision", revision); err != nil {
		return DecisionSubject{}, err
	}
	return DecisionSubject{kind: engineering.SubjectKindArtifactRevision, artifactID: revision.ArtifactID, revisionID: revision.RevisionID}, nil
}

func (s DecisionSubject) isZero() bool { return s.artifactID == "" }

func (s DecisionSubject) Kind() engineering.SubjectKind { return s.kind }

func (s DecisionSubject) ArtifactID() string { return s.artifactID }

func (s DecisionSubject) RevisionID() string { return s.revisionID }

func (s DecisionSubject) source() SourceReference {
	if s.kind == engineering.SubjectKindArtifact {
		return artifactSource(s.artifactID)
	}
	return revisionSource(engineering.RevisionKey{ArtifactID: s.artifactID, RevisionID: s.revisionID})
}

// DecisionContext is one applicable Decision. The constructor ordinal is a
// sorting witness assigned after the application's governed RecordedAt/ID
// ordering; it is not itself part of the ContextPack wire value.
type DecisionContext struct {
	decisionID       string
	subject          DecisionSubject
	outcomeStatement string
	ordinal          int
}

// NewDecisionContext validates and captures one applicable Decision.
func NewDecisionContext(decisionID string, subject DecisionSubject, outcomeStatement string, canonicalOrdinal int) (DecisionContext, error) {
	if !validIdentity(decisionID) {
		return DecisionContext{}, invalidContext("decision id", "must match the governed identity grammar")
	}
	if subject.isZero() {
		return DecisionContext{}, invalidContext("decision subject", "must not be zero")
	}
	if err := requireText("decision outcome statement", outcomeStatement); err != nil {
		return DecisionContext{}, err
	}
	if canonicalOrdinal <= 0 {
		return DecisionContext{}, invalidContext("decision canonical ordinal", "must be positive")
	}
	return DecisionContext{decisionID: decisionID, subject: subject, outcomeStatement: outcomeStatement, ordinal: canonicalOrdinal}, nil
}

func (d DecisionContext) isZero() bool { return d.decisionID == "" }

func (d DecisionContext) DecisionID() string { return d.decisionID }

func (d DecisionContext) Subject() DecisionSubject { return d.subject }

func (d DecisionContext) OutcomeStatement() string { return d.outcomeStatement }

// OpenQuestionContext is one exact current-content question and its one-based
// ordinal.
type OpenQuestionContext struct {
	capabilityRevision engineering.RevisionKey
	ordinal            int
	text               string
}

func NewOpenQuestionContext(capabilityRevision engineering.RevisionKey, ordinal int, text string) (OpenQuestionContext, error) {
	if err := validateRevisionKey("open-question capability revision", capabilityRevision); err != nil {
		return OpenQuestionContext{}, err
	}
	if ordinal <= 0 {
		return OpenQuestionContext{}, invalidContext("open-question ordinal", "must be positive")
	}
	if err := requireText("open-question text", text); err != nil {
		return OpenQuestionContext{}, err
	}
	return OpenQuestionContext{capabilityRevision: capabilityRevision, ordinal: ordinal, text: text}, nil
}

func (q OpenQuestionContext) isZero() bool { return q.capabilityRevision.IsZero() }

func (q OpenQuestionContext) CapabilityRevision() engineering.RevisionKey {
	return q.capabilityRevision
}

func (q OpenQuestionContext) Ordinal() int { return q.ordinal }

func (q OpenQuestionContext) Text() string { return q.text }

// UncoveredReason is the closed FF-024 coverage-gap classification.
type UncoveredReason string

const (
	UncoveredNoRequirementTrace  UncoveredReason = "no_requirement_trace"
	UncoveredMissingCurrentClaim UncoveredReason = "missing_current_claim"
)

// UncoveredCriterion is one current criterion missing trace or Claim coverage.
type UncoveredCriterion struct {
	capabilityRevision   engineering.RevisionKey
	criterionKey         string
	criterionText        string
	reason               UncoveredReason
	requirementRevisions []engineering.RevisionKey
}

func NewUncoveredCriterion(capabilityRevision engineering.RevisionKey, criterionKey, criterionText string, reason UncoveredReason, requirementRevisions []engineering.RevisionKey) (UncoveredCriterion, error) {
	if err := validateRevisionKey("uncovered capability revision", capabilityRevision); err != nil {
		return UncoveredCriterion{}, err
	}
	if err := engineering.ValidateAcceptanceCriterionKey(criterionKey); err != nil {
		return UncoveredCriterion{}, invalidContext("uncovered criterion key", err.Error())
	}
	if err := requireText("uncovered criterion text", criterionText); err != nil {
		return UncoveredCriterion{}, err
	}
	revisions := copyRevisionKeys(requirementRevisions)
	seen := make(map[engineering.RevisionKey]struct{}, len(revisions))
	for index, revision := range revisions {
		if err := validateRevisionKey(fmt.Sprintf("uncovered requirement revision[%d]", index), revision); err != nil {
			return UncoveredCriterion{}, err
		}
		if _, duplicate := seen[revision]; duplicate {
			return UncoveredCriterion{}, invalidContext("uncovered requirement revisions", "must not contain duplicates")
		}
		seen[revision] = struct{}{}
	}
	switch reason {
	case UncoveredNoRequirementTrace:
		if len(revisions) != 0 {
			return UncoveredCriterion{}, invalidContext("uncovered requirement revisions", "must be empty for no_requirement_trace")
		}
	case UncoveredMissingCurrentClaim:
		if len(revisions) == 0 {
			return UncoveredCriterion{}, invalidContext("uncovered requirement revisions", "must not be empty for missing_current_claim")
		}
	default:
		return UncoveredCriterion{}, invalidContext("uncovered reason", "is unsupported")
	}
	return UncoveredCriterion{
		capabilityRevision: capabilityRevision, criterionKey: criterionKey, criterionText: criterionText,
		reason: reason, requirementRevisions: revisions,
	}, nil
}

func (u UncoveredCriterion) isZero() bool { return u.capabilityRevision.IsZero() }

func (u UncoveredCriterion) CapabilityRevision() engineering.RevisionKey { return u.capabilityRevision }

func (u UncoveredCriterion) CriterionKey() string { return u.criterionKey }

func (u UncoveredCriterion) CriterionText() string { return u.criterionText }

func (u UncoveredCriterion) Reason() UncoveredReason { return u.reason }

func (u UncoveredCriterion) RequirementRevisions() []engineering.RevisionKey {
	return copyRevisionKeys(u.requirementRevisions)
}

// FindingOutcome is the closed set surfaced as a validation finding.
type FindingOutcome string

const (
	FindingNotSatisfied FindingOutcome = "not-satisfied"
	FindingInconclusive FindingOutcome = "inconclusive"
)

// ValidationFinding surfaces one negative or inconclusive current Claim.
type ValidationFinding struct {
	capabilityRevision  engineering.RevisionKey
	criterionKey        string
	requirementRevision engineering.RevisionKey
	claimRecord         engineering.RecordKey
	outcome             FindingOutcome
	reasoning           string
}

func NewValidationFinding(capabilityRevision engineering.RevisionKey, criterionKey string, requirementRevision engineering.RevisionKey, claimRecord engineering.RecordKey, outcome FindingOutcome, reasoning string) (ValidationFinding, error) {
	if err := validateRevisionKey("finding capability revision", capabilityRevision); err != nil {
		return ValidationFinding{}, err
	}
	if err := engineering.ValidateAcceptanceCriterionKey(criterionKey); err != nil {
		return ValidationFinding{}, invalidContext("finding criterion key", err.Error())
	}
	if err := validateRevisionKey("finding requirement revision", requirementRevision); err != nil {
		return ValidationFinding{}, err
	}
	if err := validateRecordKey("finding claim record", claimRecord, engineering.RecordKindClaim); err != nil {
		return ValidationFinding{}, err
	}
	if outcome != FindingNotSatisfied && outcome != FindingInconclusive {
		return ValidationFinding{}, invalidContext("finding outcome", "must be not-satisfied or inconclusive")
	}
	return ValidationFinding{
		capabilityRevision: capabilityRevision, criterionKey: criterionKey,
		requirementRevision: requirementRevision, claimRecord: claimRecord, outcome: outcome, reasoning: reasoning,
	}, nil
}

func (f ValidationFinding) isZero() bool { return f.capabilityRevision.IsZero() }

func (f ValidationFinding) CapabilityRevision() engineering.RevisionKey { return f.capabilityRevision }

func (f ValidationFinding) CriterionKey() string { return f.criterionKey }

func (f ValidationFinding) RequirementRevision() engineering.RevisionKey {
	return f.requirementRevision
}

func (f ValidationFinding) ClaimRecord() engineering.RecordKey { return f.claimRecord }

func (f ValidationFinding) Outcome() FindingOutcome { return f.outcome }

func (f ValidationFinding) Reasoning() string { return f.reasoning }
