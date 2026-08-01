package engineering

import (
	"fmt"
	"sort"
	"time"
)

// LifecycleDefinitionEnvelope is the PEOS-free persistence carrier for the
// one configured Lifecycle Definition. Payload is authoritative; DefinitionID
// is a checked lookup projection.
type LifecycleDefinitionEnvelope struct {
	DefinitionID  string
	Payload       []byte
	PayloadDigest Digest
}

// NewLifecycleDefinitionEnvelope validates and defensively copies a
// Lifecycle Definition payload.
func NewLifecycleDefinitionEnvelope(definitionID string, payload []byte, digest Digest) (LifecycleDefinitionEnvelope, error) {
	if definitionID == "" {
		return LifecycleDefinitionEnvelope{}, fmt.Errorf("%w: lifecycle definition id must not be empty", ErrInvalidEnvelope)
	}
	if err := validatePayload(payload, digest); err != nil {
		return LifecycleDefinitionEnvelope{}, err
	}
	return LifecycleDefinitionEnvelope{
		DefinitionID:  definitionID,
		Payload:       append([]byte(nil), payload...),
		PayloadDigest: digest,
	}, nil
}

// Equal applies insert-only identity and byte equality. Projections never
// override the authoritative payload.
func (e LifecycleDefinitionEnvelope) Equal(other LifecycleDefinitionEnvelope) bool {
	return e.DefinitionID == other.DefinitionID && samePayload(e.Payload, other.Payload)
}

// LifecycleDefinitionVersionKey identifies one immutable configured version.
type LifecycleDefinitionVersionKey struct {
	DefinitionID string
	VersionID    string
}

// NewLifecycleDefinitionVersionKey validates a lifecycle configuration key.
func NewLifecycleDefinitionVersionKey(definitionID, versionID string) (LifecycleDefinitionVersionKey, error) {
	if definitionID == "" || versionID == "" {
		return LifecycleDefinitionVersionKey{}, fmt.Errorf("%w: lifecycle definition and version ids must not be empty", ErrInvalidEnvelope)
	}
	return LifecycleDefinitionVersionKey{DefinitionID: definitionID, VersionID: versionID}, nil
}

// IsZero reports whether either identity component is absent.
func (k LifecycleDefinitionVersionKey) IsZero() bool {
	return k.DefinitionID == "" || k.VersionID == ""
}

// String renders the stable compound identity.
func (k LifecycleDefinitionVersionKey) String() string {
	return k.DefinitionID + "/" + k.VersionID
}

// LifecycleDefinitionVersionEnvelope is the PEOS-free persistence carrier
// for one immutable Lifecycle Definition Version.
type LifecycleDefinitionVersionEnvelope struct {
	Key           LifecycleDefinitionVersionKey
	Payload       []byte
	PayloadDigest Digest
	RecordedAt    time.Time
}

// NewLifecycleDefinitionVersionEnvelope validates and defensively copies a
// Lifecycle Definition Version payload.
func NewLifecycleDefinitionVersionEnvelope(key LifecycleDefinitionVersionKey, payload []byte, digest Digest, recordedAt time.Time) (LifecycleDefinitionVersionEnvelope, error) {
	if key.IsZero() {
		return LifecycleDefinitionVersionEnvelope{}, fmt.Errorf("%w: lifecycle definition version key must not be zero", ErrInvalidEnvelope)
	}
	if recordedAt.IsZero() {
		return LifecycleDefinitionVersionEnvelope{}, fmt.Errorf("%w: lifecycle definition version recorded-at must not be zero", ErrInvalidEnvelope)
	}
	if err := validatePayload(payload, digest); err != nil {
		return LifecycleDefinitionVersionEnvelope{}, err
	}
	return LifecycleDefinitionVersionEnvelope{
		Key:           key,
		Payload:       append([]byte(nil), payload...),
		PayloadDigest: digest,
		RecordedAt:    recordedAt,
	}, nil
}

// Equal applies insert-only identity and byte equality.
func (e LifecycleDefinitionVersionEnvelope) Equal(other LifecycleDefinitionVersionEnvelope) bool {
	return e.Key == other.Key && samePayload(e.Payload, other.Payload)
}

// LifecycleTransitionPolicy is the minimal PEOS-free projection application
// needs to validate one requested or stored lifecycle edge.
type LifecycleTransitionPolicy struct {
	TransitionID string
	SourceStates []string
	TargetStates []string
}

// LifecyclePolicy is a validated projection of the configured Definition and
// Definition Version. Slice-returning methods below preserve its immutability.
type LifecyclePolicy struct {
	DefinitionID      string
	VersionID         string
	EntryTransitionID string
	initialStates     []string
	transitions       []LifecycleTransitionPolicy
}

// NewLifecyclePolicy constructs a deterministic, defensive policy projection.
func NewLifecyclePolicy(definitionID, versionID, entryTransitionID string, initialStates []string, transitions []LifecycleTransitionPolicy) (LifecyclePolicy, error) {
	if definitionID == "" || versionID == "" || entryTransitionID == "" || len(initialStates) == 0 || len(transitions) == 0 {
		return LifecyclePolicy{}, fmt.Errorf("%w: incomplete lifecycle policy", ErrInvalidEnvelope)
	}
	initialCopy := append([]string(nil), initialStates...)
	for _, state := range initialCopy {
		if state == "" {
			return LifecyclePolicy{}, fmt.Errorf("%w: lifecycle policy contains an empty initial state", ErrInvalidEnvelope)
		}
	}
	transitionCopy := make([]LifecycleTransitionPolicy, len(transitions))
	seen := make(map[string]bool, len(transitions))
	for i, transition := range transitions {
		if transition.TransitionID == "" || len(transition.SourceStates) == 0 || len(transition.TargetStates) == 0 || seen[transition.TransitionID] {
			return LifecyclePolicy{}, fmt.Errorf("%w: lifecycle policy contains an invalid transition", ErrInvalidEnvelope)
		}
		seen[transition.TransitionID] = true
		transitionCopy[i] = LifecycleTransitionPolicy{
			TransitionID: transition.TransitionID,
			SourceStates: append([]string(nil), transition.SourceStates...),
			TargetStates: append([]string(nil), transition.TargetStates...),
		}
	}
	sort.Strings(initialCopy)
	sort.Slice(transitionCopy, func(i, j int) bool { return transitionCopy[i].TransitionID < transitionCopy[j].TransitionID })
	return LifecyclePolicy{
		DefinitionID:      definitionID,
		VersionID:         versionID,
		EntryTransitionID: entryTransitionID,
		initialStates:     initialCopy,
		transitions:       transitionCopy,
	}, nil
}

// InitialStates returns the configured initial state identities.
func (p LifecyclePolicy) InitialStates() []string {
	return append([]string(nil), p.initialStates...)
}

// Transitions returns a deep copy of the configured transition rules.
func (p LifecyclePolicy) Transitions() []LifecycleTransitionPolicy {
	out := make([]LifecycleTransitionPolicy, len(p.transitions))
	for i, transition := range p.transitions {
		out[i] = LifecycleTransitionPolicy{
			TransitionID: transition.TransitionID,
			SourceStates: append([]string(nil), transition.SourceStates...),
			TargetStates: append([]string(nil), transition.TargetStates...),
		}
	}
	return out
}

// Permits reports whether one exact transition permits source -> target.
func (p LifecyclePolicy) Permits(transitionID, sourceState, targetState string) bool {
	for _, transition := range p.transitions {
		if transition.TransitionID != transitionID {
			continue
		}
		sourceAllowed, targetAllowed := false, false
		for _, source := range transition.SourceStates {
			sourceAllowed = sourceAllowed || source == sourceState
		}
		for _, target := range transition.TargetStates {
			targetAllowed = targetAllowed || target == targetState
		}
		return sourceAllowed && targetAllowed
	}
	return false
}

// HasInitialState reports membership in the configured initial-state set.
func (p LifecyclePolicy) HasInitialState(state string) bool {
	for _, initial := range p.initialStates {
		if initial == state {
			return true
		}
	}
	return false
}

// LifecycleAssignmentDetail is the validated application projection of one
// stored State Assignment.
type LifecycleAssignmentDetail struct {
	AssignmentID      string
	SubjectArtifactID string
	DefinitionVersion LifecycleDefinitionVersionKey
	StateID           string
	EffectiveAt       time.Time
	RecordedAt        time.Time
	Actor             string
	EstablishedBy     RevisionKey
}

// LifecycleTransitionDetail is the validated application projection of an
// establishing Transition Record Revision. Entry revisions are content-free;
// their definition version is proven through the assignment they establish.
type LifecycleTransitionDetail struct {
	RevisionKey           RevisionKey
	Entry                 bool
	SubjectArtifactID     string
	DefinitionVersion     LifecycleDefinitionVersionKey
	TransitionID          string
	FromAssignmentID      string
	ResultingAssignmentID string
	TargetStateID         string
	AttemptedAt           time.Time
	CompletedAt           time.Time
	RecordedAt            time.Time
	Actor                 string
}
