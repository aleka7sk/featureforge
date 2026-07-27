package engineering

import "time"

// These input types are the parameters of the EngineeringRecorder port
// (declared in internal/application, implemented in
// internal/engineering/peos). They live here -- not in engineering/peos --
// so that internal/application can reference them without importing the
// PEOS SDK (FF-008 §5.4). None may contain a PEOS type; Method, Outcome, and
// CorrectionKind cross as plain strings, parsed against the closed
// FeatureForge/PEOS vocabulary inside engineering/peos.

// CapabilityRevisionInput records a capability specification revision
// (FF-011 §4.1).
type CapabilityRevisionInput struct {
	ArtifactID    string
	RevisionID    string
	ContentDigest Digest
	RecordedAt    time.Time
}

// EvidenceInput records an evidence Artifact and its one revision, cited by
// external locator (FF-011 §4.6).
type EvidenceInput struct {
	ArtifactID string
	RevisionID string
	Locator    string
	RecordedAt time.Time
}

// RequirementInput records a Requirement Artifact and its founding revision
// (FF-011 §4.2). SubjectArtifactID is the capability the requirement is
// about.
type RequirementInput struct {
	ArtifactID        string
	RevisionID        string
	Statement         string
	SubjectArtifactID string
	RecordedAt        time.Time
}

// DecisionInput records a Decision with its Basis (FF-011 §4.3).
type DecisionInput struct {
	DecisionID         string
	SubjectArtifactID  string
	SubjectRevisionID  string
	Question           string
	OutcomeStatement   string
	Alternatives       []string
	EvidenceArtifactID string
	EvidenceRevisionID string
	Assumptions        []string
	Constraints        []string
	Uncertainties      []string
	Rationale          string
	RecordedAt         time.Time
}

// PlanActivityInput is one planned validation activity (FF-011 §4.4). Method
// is a validation-method vocabulary suffix, e.g. "manual-review".
type PlanActivityInput struct {
	Key                   string
	SubjectArtifactID     string
	SubjectRevisionID     string
	Method                string
	OutcomeInterpretation string
	RequirementArtifactID string
	RequirementRevisionID string
	ExpectedEvidence      []string
}

// PlanInput records a Validation Plan and its founding revision
// (FF-011 §4.4). ScopeArtifactID is the capability the plan concerns.
type PlanInput struct {
	ArtifactID      string
	RevisionID      string
	ScopeArtifactID string
	Activities      []PlanActivityInput
	RecordedAt      time.Time
}

// ExecutionInput records a Validation Execution Record (FF-011 §4.5). Method
// is a validation-method vocabulary suffix; Outcome is one of "completed",
// "failed", "interrupted", "indeterminate".
type ExecutionInput struct {
	ExecutionID        string
	PlanArtifactID     string
	PlanRevisionID     string
	ActivityKey        string
	SubjectArtifactID  string
	SubjectRevisionID  string
	Method             string
	Outcome            string
	CompletedAt        time.Time
	EvidenceArtifactID string
	EvidenceRevisionID string
	RecordedAt         time.Time
}

// ClaimInput records a Validation Claim (FF-011 §4.8). Outcome is one of
// "satisfied", "not-satisfied", "inconclusive". If HasCorrection is set, the
// claim carries a correction reference naming CorrectionTarget, and
// CorrectionKind is one of "correct", "replace", "invalidate".
// ScopeArtifactID is the capability the claim concerns.
type ClaimInput struct {
	ClaimID               string
	ScopeArtifactID       string
	SubjectArtifactID     string
	SubjectRevisionID     string
	RequirementArtifactID string
	RequirementRevisionID string
	Outcome               string
	Method                string
	EvidenceArtifactID    string
	EvidenceRevisionID    string
	ExecutionID           string
	Reasoning             string
	Timestamp             time.Time
	RecordedAt            time.Time
	HasCorrection         bool
	CorrectionKind        string
	CorrectionTarget      string
}

// EntryAssignmentInput records the lifecycle entry State Assignment,
// established by a content-free Transition Record Revision (AD-014).
type EntryAssignmentInput struct {
	AssignmentID               string
	SubjectArtifactID          string
	State                      string
	EffectiveAt                time.Time
	TransitionRecordArtifactID string
	TransitionRecordRevisionID string
	RecordedAt                 time.Time
}

// TransitionInput records a non-entry lifecycle transition: a full
// Transition Record Revision plus the State Assignment it establishes.
type TransitionInput struct {
	AssignmentID               string
	SubjectArtifactID          string
	State                      string
	EffectiveAt                time.Time
	TransitionRecordArtifactID string
	TransitionRecordRevisionID string
	TransitionKey              string
	FromAssignmentID           string
	AttemptedAt                time.Time
	CompletedAt                time.Time
	RecordedAt                 time.Time
}
