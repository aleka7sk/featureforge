// Package peos is the single boundary through which FeatureForge constructs,
// serializes, and projects PEOS v1.0.0 values (AD-005). No other package in
// this module imports the PEOS SDK.
package peos

import (
	"errors"
	"fmt"

	"github.com/aleka7sk/PEOS/peos/core"
	"github.com/aleka7sk/PEOS/peos/lifecycle"
)

// ErrUnknownVocabularyString is returned when a plain string crossing the
// application/engineering boundary does not name a value in the closed
// vocabulary it was expected to select from.
var ErrUnknownVocabularyString = errors.New("peos: unknown vocabulary string")

// Namespace is the only vocabulary namespace FeatureForge constructs values
// in. FeatureForge never constructs a value in PEOS's own "peos" namespace;
// every PEOS-namespace value used here is an SDK-exported constant
// (FF-011 §3).
const Namespace = "featureforge"

func mustVocabularyValue(value string) core.VocabularyValue {
	v, err := core.NewVocabularyValue(Namespace, value)
	if err != nil {
		panic(fmt.Sprintf("peos: invalid featureforge vocabulary value %q: %v", value, err))
	}
	return v
}

func mustStateID(value string) lifecycle.StateID {
	id, err := lifecycle.NewStateID(Namespace + ":" + value)
	if err != nil {
		panic(fmt.Sprintf("peos: invalid state id %q: %v", value, err))
	}
	return id
}

func mustTransitionID(value string) lifecycle.TransitionID {
	id, err := lifecycle.NewTransitionID(Namespace + ":" + value)
	if err != nil {
		panic(fmt.Sprintf("peos: invalid transition id %q: %v", value, err))
	}
	return id
}

// Artifact Types (FF-011 §3).
var (
	ArtifactTypeProductCapability  = core.NewArtifactType(mustVocabularyValue("product-capability"))
	ArtifactTypeValidationEvidence = core.NewArtifactType(mustVocabularyValue("validation-evidence"))
)

// Representation media types (FF-011 §3).
var (
	MediaTypeSpecificationContent = mustVocabularyValue("specification-content")
	MediaTypeValidationReport     = mustVocabularyValue("validation-report")
)

// ContentAddressAlgorithm is the vocabulary value identifying the digest
// algorithm used for content-addressed representations and integrity values.
var ContentAddressAlgorithm = mustVocabularyValue("sha256")

// Validation methods (FF-011 §3).
var (
	ValidationMethodManualReview     = core.NewValidationMethod(mustVocabularyValue("manual-review"))
	ValidationMethodManualInspection = core.NewValidationMethod(mustVocabularyValue("manual-inspection"))
)

// CapabilityScopeKind is the Scope kind used for every Plan, Claim, Decision,
// and Lifecycle Definition Version scope in the canonical scenario.
var CapabilityScopeKind = mustVocabularyValue("capability")

// LifecycleSubjectType is the Lifecycle Definition Version's declared subject
// type.
var LifecycleSubjectType = mustVocabularyValue("capability-lifecycle")

// Lifecycle states (FF-010 §8). StateID wraps a validated core.LocalKey, not
// a core.VocabularyValue -- PEOS does not require State identifiers to be
// namespaced -- but the featureforge: prefix is used here for readability
// and to keep every FeatureForge-declared identifier visually consistent.
var (
	StateDrafting        = mustStateID("drafting")
	StateSpecified       = mustStateID("specified")
	StateUnderValidation = mustStateID("under-validation")
	StateAssessed        = mustStateID("assessed")
)

// Lifecycle transitions (FF-011 §4.9).
var (
	TransitionEnter           = mustTransitionID("enter")
	TransitionSpecify         = mustTransitionID("specify")
	TransitionBeginValidation = mustTransitionID("begin-validation")
	TransitionAssess          = mustTransitionID("assess")
)

// LocalActorRef is the single configured actor for the entire POC
// (FF-002 §2).
var LocalActorRef = mustActorRef()

func mustActorRef() core.ActorRef {
	ref, err := core.NewActorRef(Namespace, "local-user")
	if err != nil {
		panic(fmt.Sprintf("peos: invalid local actor ref: %v", err))
	}
	return ref
}

// LocalAuthorityRef is the single configured authority for the entire POC.
var LocalAuthorityRef = mustAuthorityRef()

func mustAuthorityRef() core.AuthorityRef {
	ref, err := core.NewAuthorityRef(Namespace, "local-user")
	if err != nil {
		panic(fmt.Sprintf("peos: invalid local authority ref: %v", err))
	}
	return ref
}

// parseValidationMethod parses a validation-method vocabulary suffix
// ("manual-review", "manual-inspection") into its PEOS constant. This is
// the boundary conversion that keeps core.ValidationMethod out of every
// input struct application populates (FF-008 §5.4).
func parseValidationMethod(s string) (core.ValidationMethod, error) {
	switch s {
	case "manual-review":
		return ValidationMethodManualReview, nil
	case "manual-inspection":
		return ValidationMethodManualInspection, nil
	default:
		return core.ValidationMethod{}, fmt.Errorf("%w: validation method %q", ErrUnknownVocabularyString, s)
	}
}

// parseExecutionOutcome parses "completed", "failed", "interrupted", or
// "indeterminate" into its PEOS constant.
func parseExecutionOutcome(s string) (core.ExecutionOutcome, error) {
	switch s {
	case "completed":
		return core.ExecutionOutcomeCompleted, nil
	case "failed":
		return core.ExecutionOutcomeFailed, nil
	case "interrupted":
		return core.ExecutionOutcomeInterrupted, nil
	case "indeterminate":
		return core.ExecutionOutcomeIndeterminate, nil
	default:
		return core.ExecutionOutcome{}, fmt.Errorf("%w: execution outcome %q", ErrUnknownVocabularyString, s)
	}
}

// parseClaimOutcome parses "satisfied", "not-satisfied", or "inconclusive"
// into its PEOS constant.
func parseClaimOutcome(s string) (core.ClaimOutcome, error) {
	switch s {
	case "satisfied":
		return core.ClaimOutcomeSatisfied, nil
	case "not-satisfied":
		return core.ClaimOutcomeNotSatisfied, nil
	case "inconclusive":
		return core.ClaimOutcomeInconclusive, nil
	default:
		return core.ClaimOutcome{}, fmt.Errorf("%w: claim outcome %q", ErrUnknownVocabularyString, s)
	}
}

// parseCorrectionKind parses "correct", "replace", or "invalidate" into its
// PEOS constant.
func parseCorrectionKind(s string) (core.CorrectionKind, error) {
	switch s {
	case "correct":
		return core.CorrectionKindCorrect, nil
	case "replace":
		return core.CorrectionKindReplace, nil
	case "invalidate":
		return core.CorrectionKindInvalidate, nil
	default:
		return core.CorrectionKind{}, fmt.Errorf("%w: correction kind %q", ErrUnknownVocabularyString, s)
	}
}
