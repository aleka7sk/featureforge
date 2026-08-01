package ui

// requirementRow is one requirement, joining Q4's effective_requirements[]
// (revision + statement) with its readiness row (current claim, outcome,
// criterion key) by artifact ID (FF-001 §3.4).
type requirementRow struct {
	ArtifactID                   string
	RevisionID                   string
	Sequence                     int
	AcceptanceState              string
	IsCurrent                    bool
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

func mapRequirementsPageData(featureCardID, capabilityID string, effective []apiEffectiveRequirementDTO, history []apiRequirementRevisionHistoryDTO, per []apiPerRequirementReadinessDTO) requirementsPageData {
	readinessByID := make(map[string]readinessRow, len(per))
	for _, row := range mapReadinessRows(per) {
		readinessByID[row.RequirementArtifactID] = row
	}

	effectiveRevisionByID := make(map[string]string, len(effective))
	for _, req := range effective {
		effectiveRevisionByID[req.ArtifactID] = req.RevisionID
	}

	rows := make([]requirementRow, 0, len(history))
	for _, req := range history {
		isCurrent := effectiveRevisionByID[req.ArtifactID] == req.RevisionID
		var readiness readinessRow
		if isCurrent {
			readiness = readinessByID[req.ArtifactID]
		}
		rows = append(rows, requirementRow{
			ArtifactID: req.ArtifactID, RevisionID: req.RevisionID, Sequence: req.Sequence,
			AcceptanceState: req.AcceptanceState, IsCurrent: isCurrent, Statement: req.Statement,
			SourceCapabilityArtifactID:   req.SourceCapabilityArtifactID,
			SourceCapabilityRevisionID:   req.SourceCapabilityRevisionID,
			SourceAcceptanceCriterionKey: req.SourceAcceptanceCriterionKey,
			Readiness:                    readiness,
		})
	}

	return requirementsPageData{
		PageTitle: "Requirements", FeatureCardID: featureCardID, CapabilityID: capabilityID, Requirements: rows,
	}
}
