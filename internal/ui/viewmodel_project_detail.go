package ui

import "time"

// featureCardRow is one feature card as the project detail view renders it
// (FF-001 §3.1: "open project" shows its feature cards).
type featureCardRow struct {
	FeatureCardID string
	Title         string
	Description   string
	CreatedAt     time.Time
}

// projectDetailPageData is templates/project.html's shape.
type projectDetailPageData struct {
	PageTitle  string
	ProjectID  string
	Name       string
	Features   []featureCardRow
	FormError  string
	FormValues map[string]string
}

func mapProjectDetailPageData(project apiProjectDTO, features []apiFeatureCardDTO) projectDetailPageData {
	rows := make([]featureCardRow, 0, len(features))
	for _, f := range features {
		rows = append(rows, featureCardRow{
			FeatureCardID: f.FeatureCardID, Title: f.Title, Description: f.Description, CreatedAt: f.CreatedAt,
		})
	}
	return projectDetailPageData{PageTitle: project.Name, ProjectID: project.ProjectID, Name: project.Name, Features: rows}
}
