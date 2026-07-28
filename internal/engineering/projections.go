package engineering

// The types below are PEOS-free read-side projections of engineering
// content that lives only inside a stored Payload (FF-020 §2 class B):
// decision detail, and planned validation activities. They are decoded by
// internal/engineering/peos (the sole PEOS importer, AD-005) and returned
// through the EngineeringProjector port (internal/application), the same
// seam EngineeringRecorder already uses on the write side. Requirement
// statement and claim reasoning need no dedicated type -- each is a single
// string -- so the projector returns those directly.
//
// Field names deliberately avoid TestNoShadowStructNames's forbidden set
// (Decision, Claim, Requirement, ...) and TestNoPEOSTypeIsCopied's
// suspicious field-set check: these carry only what a screen renders, not a
// mirror of the PEOS object.

// DecisionDetail is a decision's full basis, decoded from its stored
// payload (FF-001 §3.5: "the basis is displayed, not collapsed").
type DecisionDetail struct {
	Question         string
	OutcomeStatement string
	Rationale        string
	Alternatives     []string
	Assumptions      []string
	Constraints      []string
	Uncertainties    []string
}

// PlanActivityDetail is one planned validation activity, decoded from a
// validation plan revision's stored payload (FF-001 §3.6: "plan revision
// and its activities").
type PlanActivityDetail struct {
	Key                   string
	Method                string
	OutcomeInterpretation string
	ExpectedEvidence      []string
}
