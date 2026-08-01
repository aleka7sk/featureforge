package ui

// requirementRow is one requirement, joining Q4's effective_requirements[]
// (revision + statement) with its readiness row (current claim, outcome,
// criterion key) by artifact ID (FF-001 §3.4).
type requirementRow struct {
	ArtifactID                   string
	RevisionID                   string
	Statement                    string
	SourceCapabilityArtifactID   string
	SourceCapabilityRevisionID   string
	SourceAcceptanceCriterionKey string
	Readiness                    readinessRow
}

// requirementsPageData is templates/requirements.html's shape.
type requirementsPageData struct {
	PageTitle                  string
	FeatureCardID              string
	CapabilityID               string
	SourceCapabilityRevisionID string
	Requirements               []requirementRow
	FormError                  string
	FormValues                 map[string]string
}

func mapRequirementsPageData(featureCardID, capabilityID string, effective []apiEffectiveRequirementDTO, per []apiPerRequirementReadinessDTO) requirementsPageData {
	readinessByID := make(map[string]readinessRow, len(per))
	for _, row := range mapReadinessRows(per) {
		readinessByID[row.RequirementArtifactID] = row
	}

	rows := make([]requirementRow, 0, len(effective))
	for _, req := range effective {
		rows = append(rows, requirementRow{
			ArtifactID: req.ArtifactID, RevisionID: req.RevisionID, Statement: req.Statement,
			SourceCapabilityArtifactID:   req.SourceCapabilityArtifactID,
			SourceCapabilityRevisionID:   req.SourceCapabilityRevisionID,
			SourceAcceptanceCriterionKey: req.SourceAcceptanceCriterionKey,
			Readiness:                    readinessByID[req.ArtifactID],
		})
	}

	return requirementsPageData{
		PageTitle: "Requirements", FeatureCardID: featureCardID, CapabilityID: capabilityID, Requirements: rows,
	}
}
