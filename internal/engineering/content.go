package engineering

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const (
	// SupportedSchemaVersion is the only schema_version M.3 accepts.
	SupportedSchemaVersion = 1

	maxStringBytes = 4000
	maxListItems   = 64
)

var acceptanceCriterionKeyPattern = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

// AcceptanceCriterion is one keyed acceptance criterion within a capability
// specification's content (FF-009 §4.1).
type AcceptanceCriterion struct {
	key  string
	text string
}

// NewAcceptanceCriterion validates and returns an AcceptanceCriterion.
func NewAcceptanceCriterion(key, text string) (AcceptanceCriterion, error) {
	if !acceptanceCriterionKeyPattern.MatchString(key) {
		return AcceptanceCriterion{}, fmt.Errorf("%w: acceptance criterion key %q must match [A-Za-z0-9-]+", ErrInvalidContent, key)
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return AcceptanceCriterion{}, fmt.Errorf("%w: acceptance criterion %q text must not be empty", ErrInvalidContent, key)
	}
	if err := checkStringBound("acceptance criterion text", trimmed); err != nil {
		return AcceptanceCriterion{}, err
	}
	return AcceptanceCriterion{key: key, text: trimmed}, nil
}

// Key returns the criterion's stable, revision-local key.
func (a AcceptanceCriterion) Key() string { return a.key }

// Text returns the criterion's text.
func (a AcceptanceCriterion) Text() string { return a.text }

// acceptanceCriterionWire is the canonical JSON shape of AcceptanceCriterion.
type acceptanceCriterionWire struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

func checkStringBound(field, value string) error {
	if len(value) > maxStringBytes {
		return fmt.Errorf("%w: %s exceeds %d bytes", ErrContentTooLarge, field, maxStringBytes)
	}
	return nil
}

func checkListBound(field string, n int) error {
	if n > maxListItems {
		return fmt.Errorf("%w: %s has %d elements, exceeding %d", ErrContentTooLarge, field, n, maxListItems)
	}
	return nil
}

func trimNonEmptyList(field string, items []string) ([]string, error) {
	if err := checkListBound(field, len(items)); err != nil {
		return nil, err
	}
	out := make([]string, len(items))
	for i, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			return nil, fmt.Errorf("%w: %s[%d] must not be empty", ErrInvalidContent, field, i)
		}
		if err := checkStringBound(fmt.Sprintf("%s[%d]", field, i), trimmed); err != nil {
			return nil, err
		}
		out[i] = trimmed
	}
	return out, nil
}

// CapabilitySpecificationContent is the FeatureForge-owned structured content
// of one capability revision (FF-009 §4.1). It is immutable; every optional
// field is added through a copy-return With* modifier.
type CapabilitySpecificationContent struct {
	schemaVersion        int
	title                string
	problemStatement     string
	userOutcome          string
	functionalBehaviours []string
	constraints          []string
	acceptanceCriteria   []AcceptanceCriterion
	dependencies         []string
	openQuestions        []string
}

// NewCapabilitySpecificationContent validates schemaVersion, title, and
// problemStatement -- the three mandatory fields -- and returns a content
// value with every optional field empty. Use the With* methods to add them.
func NewCapabilitySpecificationContent(schemaVersion int, title, problemStatement string) (CapabilitySpecificationContent, error) {
	if schemaVersion != SupportedSchemaVersion {
		return CapabilitySpecificationContent{}, fmt.Errorf("%w: schema_version %d is not supported, only %d", ErrUnsupportedSchemaVersion, schemaVersion, SupportedSchemaVersion)
	}
	trimmedTitle := strings.TrimSpace(title)
	if trimmedTitle == "" {
		return CapabilitySpecificationContent{}, fmt.Errorf("%w: title must not be empty", ErrInvalidContent)
	}
	if err := checkStringBound("title", trimmedTitle); err != nil {
		return CapabilitySpecificationContent{}, err
	}
	trimmedProblem := strings.TrimSpace(problemStatement)
	if trimmedProblem == "" {
		return CapabilitySpecificationContent{}, fmt.Errorf("%w: problem_statement must not be empty", ErrInvalidContent)
	}
	if err := checkStringBound("problem_statement", trimmedProblem); err != nil {
		return CapabilitySpecificationContent{}, err
	}
	return CapabilitySpecificationContent{
		schemaVersion:    schemaVersion,
		title:            trimmedTitle,
		problemStatement: trimmedProblem,
	}, nil
}

// WithUserOutcome returns a copy of c with its user outcome set.
func (c CapabilitySpecificationContent) WithUserOutcome(userOutcome string) (CapabilitySpecificationContent, error) {
	trimmed := strings.TrimSpace(userOutcome)
	if err := checkStringBound("user_outcome", trimmed); err != nil {
		return CapabilitySpecificationContent{}, err
	}
	c.userOutcome = trimmed
	return c, nil
}

// WithFunctionalBehaviours returns a copy of c with its functional
// behaviours set.
func (c CapabilitySpecificationContent) WithFunctionalBehaviours(items []string) (CapabilitySpecificationContent, error) {
	trimmed, err := trimNonEmptyList("functional_behaviours", items)
	if err != nil {
		return CapabilitySpecificationContent{}, err
	}
	c.functionalBehaviours = trimmed
	return c, nil
}

// WithConstraints returns a copy of c with its constraints set.
func (c CapabilitySpecificationContent) WithConstraints(items []string) (CapabilitySpecificationContent, error) {
	trimmed, err := trimNonEmptyList("constraints", items)
	if err != nil {
		return CapabilitySpecificationContent{}, err
	}
	c.constraints = trimmed
	return c, nil
}

// WithAcceptanceCriteria returns a copy of c with its acceptance criteria
// set. Keys must be unique within the content.
func (c CapabilitySpecificationContent) WithAcceptanceCriteria(items []AcceptanceCriterion) (CapabilitySpecificationContent, error) {
	if err := checkListBound("acceptance_criteria", len(items)); err != nil {
		return CapabilitySpecificationContent{}, err
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]AcceptanceCriterion, len(items))
	for i, item := range items {
		if item.Key() == "" {
			return CapabilitySpecificationContent{}, fmt.Errorf("%w: acceptance_criteria[%d] has an empty key", ErrInvalidContent, i)
		}
		if _, dup := seen[item.Key()]; dup {
			return CapabilitySpecificationContent{}, fmt.Errorf("%w: %q", ErrDuplicateAcceptanceCriterionKey, item.Key())
		}
		seen[item.Key()] = struct{}{}
		out[i] = item
	}
	c.acceptanceCriteria = out
	return c, nil
}

// WithDependencies returns a copy of c with its dependencies set.
func (c CapabilitySpecificationContent) WithDependencies(items []string) (CapabilitySpecificationContent, error) {
	trimmed, err := trimNonEmptyList("dependencies", items)
	if err != nil {
		return CapabilitySpecificationContent{}, err
	}
	c.dependencies = trimmed
	return c, nil
}

// WithOpenQuestions returns a copy of c with its open questions set.
func (c CapabilitySpecificationContent) WithOpenQuestions(items []string) (CapabilitySpecificationContent, error) {
	trimmed, err := trimNonEmptyList("open_questions", items)
	if err != nil {
		return CapabilitySpecificationContent{}, err
	}
	c.openQuestions = trimmed
	return c, nil
}

// SchemaVersion returns the content's schema version.
func (c CapabilitySpecificationContent) SchemaVersion() int { return c.schemaVersion }

// Title returns the content's title.
func (c CapabilitySpecificationContent) Title() string { return c.title }

// ProblemStatement returns the content's problem statement.
func (c CapabilitySpecificationContent) ProblemStatement() string { return c.problemStatement }

// UserOutcome returns the content's user outcome.
func (c CapabilitySpecificationContent) UserOutcome() string { return c.userOutcome }

// FunctionalBehaviours returns the content's functional behaviours, in order.
func (c CapabilitySpecificationContent) FunctionalBehaviours() []string {
	return append([]string(nil), c.functionalBehaviours...)
}

// Constraints returns the content's constraints, in order.
func (c CapabilitySpecificationContent) Constraints() []string {
	return append([]string(nil), c.constraints...)
}

// AcceptanceCriteria returns the content's acceptance criteria, in order.
func (c CapabilitySpecificationContent) AcceptanceCriteria() []AcceptanceCriterion {
	return append([]AcceptanceCriterion(nil), c.acceptanceCriteria...)
}

// Dependencies returns the content's dependencies, in order.
func (c CapabilitySpecificationContent) Dependencies() []string {
	return append([]string(nil), c.dependencies...)
}

// OpenQuestions returns the content's open questions, in order.
func (c CapabilitySpecificationContent) OpenQuestions() []string {
	return append([]string(nil), c.openQuestions...)
}

// IsZero reports whether c is the zero value.
func (c CapabilitySpecificationContent) IsZero() bool { return c.schemaVersion == 0 }

// contentWire is the canonical JSON shape of CapabilitySpecificationContent.
// Field declaration order IS the canonical field order (FF-009 §4.1 rule 1);
// no field uses omitempty, and every slice is normalized to non-nil before
// marshaling so empty lists encode as [] rather than null (rule 4).
type contentWire struct {
	SchemaVersion        int                       `json:"schema_version"`
	Title                string                    `json:"title"`
	ProblemStatement     string                    `json:"problem_statement"`
	UserOutcome          string                    `json:"user_outcome"`
	FunctionalBehaviours []string                  `json:"functional_behaviours"`
	Constraints          []string                  `json:"constraints"`
	AcceptanceCriteria   []acceptanceCriterionWire `json:"acceptance_criteria"`
	Dependencies         []string                  `json:"dependencies"`
	OpenQuestions        []string                  `json:"open_questions"`
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func (c CapabilitySpecificationContent) wire() contentWire {
	criteria := make([]acceptanceCriterionWire, len(c.acceptanceCriteria))
	for i, ac := range c.acceptanceCriteria {
		criteria[i] = acceptanceCriterionWire{Key: ac.key, Text: ac.text}
	}
	return contentWire{
		SchemaVersion:        c.schemaVersion,
		Title:                c.title,
		ProblemStatement:     c.problemStatement,
		UserOutcome:          c.userOutcome,
		FunctionalBehaviours: nonNil(c.functionalBehaviours),
		Constraints:          nonNil(c.constraints),
		AcceptanceCriteria:   criteria,
		Dependencies:         nonNil(c.dependencies),
		OpenQuestions:        nonNil(c.openQuestions),
	}
}

// CanonicalJSON returns the content's deterministic canonical JSON encoding
// (FF-009 §4.1): declared field order, no insignificant whitespace, HTML
// escaping disabled, empty lists as [].
func (c CapabilitySpecificationContent) CanonicalJSON() ([]byte, error) {
	return canonicalEncode(c.wire())
}

// Digest returns the SHA-256 digest of the content's canonical JSON.
func (c CapabilitySpecificationContent) Digest() (Digest, error) {
	data, err := c.CanonicalJSON()
	if err != nil {
		return Digest{}, err
	}
	return ComputeDigest(data), nil
}

// Equal reports whether c and other have byte-identical canonical JSON.
func (c CapabilitySpecificationContent) Equal(other CapabilitySpecificationContent) bool {
	a, errA := c.CanonicalJSON()
	b, errB := other.CanonicalJSON()
	if errA != nil || errB != nil {
		return false
	}
	return string(a) == string(b)
}

// ParseCapabilitySpecificationContent decodes canonical JSON produced by
// CanonicalJSON back into a validated CapabilitySpecificationContent.
func ParseCapabilitySpecificationContent(data []byte) (CapabilitySpecificationContent, error) {
	var w contentWire
	if err := json.Unmarshal(data, &w); err != nil {
		return CapabilitySpecificationContent{}, fmt.Errorf("%w: %w", ErrInvalidContent, err)
	}
	c, err := NewCapabilitySpecificationContent(w.SchemaVersion, w.Title, w.ProblemStatement)
	if err != nil {
		return CapabilitySpecificationContent{}, err
	}
	if w.UserOutcome != "" {
		if c, err = c.WithUserOutcome(w.UserOutcome); err != nil {
			return CapabilitySpecificationContent{}, err
		}
	}
	if len(w.FunctionalBehaviours) > 0 {
		if c, err = c.WithFunctionalBehaviours(w.FunctionalBehaviours); err != nil {
			return CapabilitySpecificationContent{}, err
		}
	}
	if len(w.Constraints) > 0 {
		if c, err = c.WithConstraints(w.Constraints); err != nil {
			return CapabilitySpecificationContent{}, err
		}
	}
	if len(w.AcceptanceCriteria) > 0 {
		criteria := make([]AcceptanceCriterion, len(w.AcceptanceCriteria))
		for i, ac := range w.AcceptanceCriteria {
			criteria[i], err = NewAcceptanceCriterion(ac.Key, ac.Text)
			if err != nil {
				return CapabilitySpecificationContent{}, err
			}
		}
		if c, err = c.WithAcceptanceCriteria(criteria); err != nil {
			return CapabilitySpecificationContent{}, err
		}
	}
	if len(w.Dependencies) > 0 {
		if c, err = c.WithDependencies(w.Dependencies); err != nil {
			return CapabilitySpecificationContent{}, err
		}
	}
	if len(w.OpenQuestions) > 0 {
		if c, err = c.WithOpenQuestions(w.OpenQuestions); err != nil {
			return CapabilitySpecificationContent{}, err
		}
	}
	return c, nil
}
