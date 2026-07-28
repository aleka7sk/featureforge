package http

import (
	"net/http"

	"github.com/aleka7sk/featureforge/internal/application"
)

// handleListProjects implements Q1 (FF-018 §3.2, §10.3).
func handleListProjects(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projects, err := application.ListProjects(r.Context(), deps.UOW)
		if err != nil {
			writeAppError(w, r, deps, err)
			return
		}
		dtos := make([]projectDTO, 0, len(projects))
		for _, p := range projects {
			dtos = append(dtos, projectDTO{ProjectID: p.ID().String(), Name: p.Name(), CreatedAt: p.CreatedAt()})
		}
		writeJSON(w, http.StatusOK, listProjectsResponse{Projects: dtos}, nil)
	}
}
