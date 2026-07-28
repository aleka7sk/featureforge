package ui

import "net/http"

// handleProjects renders screen 1 (FF-001 §3.1) from Q1.
func handleProjects(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := callAPI(r.Context(), deps.API, http.MethodGet, "/api/v1/projects", nil)
		if err != nil {
			writeErrorPage(w, http.StatusInternalServerError, "Something went wrong", "An unexpected error occurred.")
			return
		}
		if !result.OK {
			writeAPIErrorPage(w, result)
			return
		}
		var body struct {
			Projects []apiProjectDTO `json:"projects"`
		}
		if err := decodeInto(result, &body); err != nil {
			writeErrorPage(w, http.StatusInternalServerError, "Something went wrong", "An unexpected error occurred.")
			return
		}
		render(w, http.StatusOK, "projects", mapProjectsPageData(body.Projects))
	}
}
