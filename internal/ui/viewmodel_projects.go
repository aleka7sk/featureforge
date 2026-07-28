package ui

import "time"

// projectRow is one project as the Projects screen renders it (FF-001
// §3.1). Built from Q1's JSON shape by projectsViewModel -- never from a
// domain or application type, which this package cannot import.
type projectRow struct {
	ProjectID string
	Name      string
	CreatedAt time.Time
}

// projectsPageData is templates/projects.html's shape.
type projectsPageData struct {
	PageTitle  string
	Projects   []projectRow
	FormError  string
	FormValues map[string]string
}

// apiProjectDTO mirrors internal/transport/http's projectDTO field for
// field, decoded from Q1's response. A UI-owned copy, not an import: this
// package cannot reach the transport package's unexported type, and would
// not import it even if exported (FF-021 §2).
type apiProjectDTO struct {
	ProjectID string    `json:"project_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

func mapProjectsPageData(projects []apiProjectDTO) projectsPageData {
	data := projectsPageData{PageTitle: "Projects", Projects: make([]projectRow, 0, len(projects))}
	for _, p := range projects {
		data.Projects = append(data.Projects, projectRow{ProjectID: p.ProjectID, Name: p.Name, CreatedAt: p.CreatedAt})
	}
	return data
}
