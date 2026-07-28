package application

import "github.com/aleka7sk/featureforge/internal/engineering"

// EngineeringProjector is the sibling read port to EngineeringRecorder
// (FF-020 §4): the seam through which application decodes a stored PEOS
// payload back into PEOS-free display content, without importing the PEOS
// SDK. Declared here, in engineering types only, and implemented by
// internal/engineering/peos.Recorder -- structurally, via Go's implicit
// interface satisfaction, exactly as EngineeringRecorder is. Kept separate
// from EngineeringRecorder so command and query dependencies stay
// separable: a query never needs write authority, and a command never
// needs decode authority.
type EngineeringProjector interface {
	ProjectRequirementStatement(payload []byte) (string, error)
	ProjectDecisionDetail(payload []byte) (engineering.DecisionDetail, error)
	ProjectPlanActivities(payload []byte) ([]engineering.PlanActivityDetail, error)
	ProjectClaimReasoning(payload []byte) (string, error)
}
