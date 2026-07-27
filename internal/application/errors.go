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
)

// Command validation errors.
var (
	ErrInvalidCommand          = errors.New("application: command is invalid")
	ErrCapabilityAlreadyLinked = errors.New("application: feature card already linked to a different capability")
)
