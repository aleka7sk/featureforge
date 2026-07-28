package application

import "errors"

// Persistence errors (FF-009 §8). Declared here as the ports' own
// contract; the in-memory (and later PostgreSQL) adapter returns these
// exact sentinels.
var (
	ErrNotFound               = errors.New("application: not found")
	ErrImmutableValueConflict = errors.New("application: immutable value conflict")
	ErrReferencedValueMissing = errors.New("application: referenced value missing")
	ErrTransactionAborted     = errors.New("application: transaction aborted")
	ErrNestedTransaction      = errors.New("application: nested transaction")
)

// Ordering errors (FF-010 §4).
var (
	ErrRevisionOrderMissing        = errors.New("application: revision order metadata missing")
	ErrRevisionSequenceConflict    = errors.New("application: revision sequence conflict")
	ErrRevisionSequenceInvalid     = errors.New("application: revision sequence invalid")
	ErrNoAcceptedRevision          = errors.New("application: no accepted revision")
	ErrCurrentRevisionAmbiguous    = errors.New("application: current revision ambiguous")
	ErrRevisionReferenceMismatch   = errors.New("application: revision reference mismatch")
	ErrAcceptanceTransitionInvalid = errors.New("application: acceptance transition invalid")
)

// Correction errors (FF-010 §6). ErrCorrectionSelfReference exists because
// PEOS v1.0.0 accepts a self-correcting reference; FeatureForge does not
// (AD-017, verified against the SDK).
var (
	ErrCorrectionTargetMissing  = errors.New("application: correction target missing")
	ErrCorrectionFamilyMismatch = errors.New("application: correction family mismatch")
	ErrCorrectionSelfReference  = errors.New("application: correction self-reference")
	ErrCorrectionCycle          = errors.New("application: correction cycle")
	ErrCorrectionAmbiguous      = errors.New("application: correction ambiguous")
)

// Query errors (FF-010 §7, §8).
var (
	ErrEngineeringStateIndeterminate = errors.New("application: engineering state indeterminate")
	ErrTimelineSourceInvalid         = errors.New("application: timeline source invalid")
	ErrAmbiguousLifecycleState       = errors.New("application: lifecycle state ambiguous")
	ErrUnknownDefinitionVersion      = errors.New("application: unknown lifecycle definition version")
	// ErrValidationPlanAmbiguous reports that a capability's discovered
	// validation-plan population has more than one member. The composition
	// that builds TimelineInput.PlanArtifactID (FF-018 §6.6) requires
	// exactly one applicable plan; the model defines no acceptance or
	// order metadata for plans (unlike capability and requirement
	// revisions), so there is no mechanism to rank two, and none is
	// invented here. Sort order is deterministic for discovery output but
	// is never used to select a plan.
	ErrValidationPlanAmbiguous = errors.New("application: validation plan ambiguous")
	// ErrStoredPayloadUnreadable reports that a stored PEOS payload would
	// not decode when a read-side projection (FF-020) tried to reconstruct
	// display content from it. A revision or record that was written
	// through this module's own commands always decodes; failure here
	// means the stored bytes are corrupt, which is server-side data
	// integrity, not a client mistake -- mapped to 500 with no internal
	// text (FF-020 §7).
	ErrStoredPayloadUnreadable = errors.New("application: stored payload unreadable")
)

// Command validation errors.
var (
	ErrInvalidCommand          = errors.New("application: command is invalid")
	ErrCapabilityAlreadyLinked = errors.New("application: feature card already linked to a different capability")
)
