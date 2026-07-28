package http

import "time"

// The DTOs below mirror application result and domain types field-for-field
// (FF-018 §10.2): no transport type aliases or embeds a domain,
// application, or engineering type.

// projectDTO mirrors domain.Project (Q1, Q2).
type projectDTO struct {
	ProjectID string    `json:"project_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// listProjectsResponse is Q1's data payload (FF-018 §10.3). No rationale:
// a list of stored values is not derived.
type listProjectsResponse struct {
	Projects []projectDTO `json:"projects"`
}
