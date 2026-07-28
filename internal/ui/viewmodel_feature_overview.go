package ui

// featureOverviewPageData is templates/feature_overview.html's shape
// (FF-001 §3.2). CurrentRevisionTitle comes from a separate Q7 call the
// handler makes for the current revision key Q3 already returned -- Q4's
// current_revision carries no content (FF-020 §5 scopes that to Q6/Q7).
type featureOverviewPageData struct {
	PageTitle             string
	FeatureCardID         string
	ProjectID             string
	Title                 string
	Description           string
	HasCapability         bool
	CapabilityArtifactID  string
	CurrentRevisionFound  bool
	CurrentRevisionID     string
	CurrentRevisionTitle  string
	CurrentRevisionSeq    int
	CurrentRevisionReason string
	RequirementCount      int
	ReadinessStatus       string
	Readiness             []readinessRow
	LifecycleFound        bool
	LifecycleStateID      string
	LifecycleReason       string
	RecentEvents          []timelineRow
	FormError             string
	FormValues            map[string]string
}

func mapFeatureOverviewPageData(
	card apiFeatureCardDTO,
	state apiEngineeringStateDTO,
	rationale apiEngineeringStateRationaleDTO,
	currentRevisionTitle string,
	recentEvents []apiTimelineEventDTO,
) featureOverviewPageData {
	data := featureOverviewPageData{
		PageTitle: card.Title, FeatureCardID: card.FeatureCardID, ProjectID: card.ProjectID,
		Title: card.Title, Description: card.Description,
		HasCapability: card.CapabilityArtifactID != "", CapabilityArtifactID: card.CapabilityArtifactID,
		RequirementCount: len(state.EffectiveRequirements),
		ReadinessStatus:  state.Readiness.Status,
		Readiness:        mapReadinessRows(state.Readiness.PerRequirement),
		LifecycleFound:   state.Lifecycle.Found, LifecycleStateID: state.Lifecycle.StateID,
		LifecycleReason: rationale.Lifecycle.Rule,
		RecentEvents:    mapTimelineRows(recentEvents),
	}
	if state.CurrentRevision.Found && state.CurrentRevision.Revision != nil {
		data.CurrentRevisionFound = true
		data.CurrentRevisionID = state.CurrentRevision.Revision.RevisionID
		data.CurrentRevisionSeq = state.CurrentRevision.Revision.Sequence
		data.CurrentRevisionTitle = currentRevisionTitle
		data.CurrentRevisionReason = rationale.CurrentRevision.Rule
	}
	return data
}
