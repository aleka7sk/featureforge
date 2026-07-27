package peos

import "github.com/aleka7sk/featureforge/internal/engineering"

// These are aliases, not new types: the canonical definitions live in
// internal/engineering (FF-008 §5.4) so that internal/application can
// declare the EngineeringRecorder port using the very same types without
// importing this package. Aliasing here keeps every codec_*.go call site
// unchanged.
type (
	CapabilityRevisionInput = engineering.CapabilityRevisionInput
	EvidenceInput           = engineering.EvidenceInput
	RequirementInput        = engineering.RequirementInput
	DecisionInput           = engineering.DecisionInput
	PlanActivityInput       = engineering.PlanActivityInput
	PlanInput               = engineering.PlanInput
	ExecutionInput          = engineering.ExecutionInput
	ClaimInput              = engineering.ClaimInput
	EntryAssignmentInput    = engineering.EntryAssignmentInput
	TransitionInput         = engineering.TransitionInput
)
