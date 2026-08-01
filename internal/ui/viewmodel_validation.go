package ui

// validationPageData is templates/validation.html's shape (FF-001 §3.6:
// "what was planned, what ran, what it produced, and what is claimed").
// This screen is a composition of Q4 (plan activities, current claims,
// readiness) and Q5 (execution.recorded events, filtered by kind) --
// FF-020's endpoint-minimization review found no dedicated endpoint owns
// this dataset; Q4 and Q5 already do (docs/spec/020-read-surface-extension.md
// §7).
type validationPageData struct {
	PageTitle      string
	FeatureCardID  string
	CapabilityID   string
	PlanFound      bool
	PlanArtifactID string
	Activities     []activityRow
	Executions     []timelineRow
	Readiness      []readinessRow
	FormError      string
	FormValues     map[string]string
	PlanFormFailed bool
}

func mapValidationPageData(featureCardID, capabilityID string, plan apiValidationPlanDTO, per []apiPerRequirementReadinessDTO, events []apiTimelineEventDTO) validationPageData {
	executions := make([]apiTimelineEventDTO, 0, len(events))
	for _, e := range events {
		if e.Kind == "execution.recorded" {
			executions = append(executions, e)
		}
	}
	return validationPageData{
		PageTitle: "Validation", FeatureCardID: featureCardID, CapabilityID: capabilityID,
		PlanFound: plan.Found, PlanArtifactID: plan.ArtifactID, Activities: mapActivityRows(plan.Activities),
		Executions: mapTimelineRows(executions), Readiness: mapReadinessRows(per),
	}
}
