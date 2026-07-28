package ui

// revisionsPageData is templates/revisions.html's shape (FF-001 §3.3).
// AcceptanceState per revision comes from Q6's rationale.considered[] --
// revisionDTO itself carries no acceptance field -- joined here by
// RevisionID; ResolveCurrentRevision's own rationale names every stored
// revision in Considered, accepted or not, so the join is total.
type revisionsPageData struct {
	PageTitle       string
	FeatureCardID   string
	ArtifactID      string
	CurrentRevision string
	CurrentReason   string
	Revisions       []revisionRow
	FormError       string
	FormValues      map[string]string
}

func mapRevisionsPageData(featureCardID, artifactID string, revisions []apiRevisionDTO, current apiCurrentRevisionDTO, rationale apiResolutionRationaleDTO) revisionsPageData {
	acceptance := make(map[string]string, len(rationale.Considered))
	for _, c := range rationale.Considered {
		acceptance[c.RevisionID] = c.AcceptanceState
	}
	currentRevisionID := ""
	if current.Found && current.Revision != nil {
		currentRevisionID = current.Revision.RevisionID
	}

	rows := make([]revisionRow, 0, len(revisions))
	for _, r := range revisions {
		row := mapRevisionRow(r, r.RevisionID == currentRevisionID)
		row.AcceptanceState = acceptance[r.RevisionID]
		rows = append(rows, row)
	}

	return revisionsPageData{
		PageTitle: "Revisions", FeatureCardID: featureCardID, ArtifactID: artifactID,
		CurrentRevision: currentRevisionID, CurrentReason: rationale.Rule, Revisions: rows,
	}
}
