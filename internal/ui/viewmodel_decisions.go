package ui

// decisionsPageData is templates/decisions.html's shape (FF-001 §3.5).
type decisionsPageData struct {
	PageTitle         string
	FeatureCardID     string
	CapabilityID      string
	SubjectRevisionID string
	Decisions         []decisionRow
	FormError         string
	FormValues        map[string]string
}

func mapDecisionsPageData(featureCardID, capabilityID string, decisions []apiApplicableDecisionDTO) decisionsPageData {
	return decisionsPageData{
		PageTitle: "Decisions", FeatureCardID: featureCardID, CapabilityID: capabilityID,
		Decisions: mapDecisionRows(decisions),
	}
}
