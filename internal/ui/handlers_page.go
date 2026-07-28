package ui

import (
	"net/http"
	"net/url"
)

// handleProjects renders screen 1 (FF-001 §3.1) from Q1.
func handleProjects(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := callAPI(r.Context(), deps.API, http.MethodGet, "/api/v1/projects", nil)
		if err != nil {
			writeInternalErrorPage(w)
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
			writeInternalErrorPage(w)
			return
		}
		render(w, http.StatusOK, "projects", mapProjectsPageData(body.Projects))
	}
}

// handleProjectDetail renders screen 1's "open project" view (FF-001
// §3.1): the project's own identity plus its feature cards (Q2). There is
// no single-project query (Q1 lists every project; FF-020's
// endpoint-minimization review found no case where the existing 19
// operations are insufficient), so the project is located by filtering
// Q1's list -- the same "reuse the existing broad query" pattern the API
// itself uses for discovery (AD-025). A projectID absent from that list
// renders the UI's own not-found page.
func handleProjectDetail(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID := r.PathValue("projectID")

		listResult, err := callAPI(r.Context(), deps.API, http.MethodGet, "/api/v1/projects", nil)
		if err != nil {
			writeInternalErrorPage(w)
			return
		}
		if !listResult.OK {
			writeAPIErrorPage(w, listResult)
			return
		}
		var listBody struct {
			Projects []apiProjectDTO `json:"projects"`
		}
		if err := decodeInto(listResult, &listBody); err != nil {
			writeInternalErrorPage(w)
			return
		}
		var project apiProjectDTO
		found := false
		for _, p := range listBody.Projects {
			if p.ProjectID == projectID {
				project = p
				found = true
				break
			}
		}
		if !found {
			notFoundPage(w, r)
			return
		}

		featuresResult, err := callAPI(r.Context(), deps.API, http.MethodGet, "/api/v1/projects/"+url.PathEscape(projectID)+"/features", nil)
		if err != nil {
			writeInternalErrorPage(w)
			return
		}
		if !featuresResult.OK {
			writeAPIErrorPage(w, featuresResult)
			return
		}
		var featuresBody struct {
			Features []apiFeatureCardDTO `json:"features"`
		}
		if err := decodeInto(featuresResult, &featuresBody); err != nil {
			writeInternalErrorPage(w)
			return
		}
		render(w, http.StatusOK, "project_detail", mapProjectDetailPageData(project, featuresBody.Features))
	}
}

// handleFeatureOverview renders screen 2 (FF-001 §3.2) from Q3 (feature
// identity, engineering state, its rationale), Q7 for the current
// revision's title (Q4's embedded current_revision carries no content --
// FF-020 §5 scopes that to Q6/Q7), and Q5 for recent history.
func handleFeatureOverview(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		featureCardID := r.PathValue("featureCardID")

		overviewResult, err := callAPI(r.Context(), deps.API, http.MethodGet, "/api/v1/features/"+url.PathEscape(featureCardID), nil)
		if err != nil {
			writeInternalErrorPage(w)
			return
		}
		if !overviewResult.OK {
			writeAPIErrorPage(w, overviewResult)
			return
		}
		var overviewBody struct {
			Feature apiFeatureCardDTO      `json:"feature"`
			State   apiEngineeringStateDTO `json:"state"`
		}
		if err := decodeInto(overviewResult, &overviewBody); err != nil {
			writeInternalErrorPage(w)
			return
		}
		var rationale apiEngineeringStateRationaleDTO
		if err := decodeRationale(overviewResult, &rationale); err != nil {
			writeInternalErrorPage(w)
			return
		}

		var currentRevisionTitle string
		if cur := overviewBody.State.CurrentRevision; cur.Found && cur.Revision != nil {
			revResult, err := callAPI(r.Context(), deps.API, http.MethodGet,
				"/api/v1/capabilities/"+url.PathEscape(cur.Revision.ArtifactID)+"/revisions/"+url.PathEscape(cur.Revision.RevisionID), nil)
			if err != nil {
				writeInternalErrorPage(w)
				return
			}
			if revResult.OK {
				var revBody apiRevisionDTO
				if err := decodeInto(revResult, &revBody); err != nil {
					writeInternalErrorPage(w)
					return
				}
				if revBody.Content != nil {
					currentRevisionTitle = revBody.Content.Title
				}
			}
		}

		timelineResult, err := callAPI(r.Context(), deps.API, http.MethodGet, "/api/v1/features/"+url.PathEscape(featureCardID)+"/timeline", nil)
		if err != nil {
			writeInternalErrorPage(w)
			return
		}
		if !timelineResult.OK {
			writeAPIErrorPage(w, timelineResult)
			return
		}
		var timelineBody struct {
			Dated []apiTimelineEventDTO `json:"dated"`
		}
		if err := decodeInto(timelineResult, &timelineBody); err != nil {
			writeInternalErrorPage(w)
			return
		}
		recent := timelineBody.Dated
		if len(recent) > 5 {
			recent = recent[len(recent)-5:]
		}

		render(w, http.StatusOK, "feature_overview", mapFeatureOverviewPageData(overviewBody.Feature, overviewBody.State, rationale, currentRevisionTitle, recent))
	}
}

// handleRevisions renders screen 3 (FF-001 §3.3): Q3 for the feature's
// capability artifact ID, then Q6 for every revision and its acceptance
// rationale. A feature with no capability yet renders the screen's own
// empty state rather than a Q6 call with an empty artifact ID.
func handleRevisions(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		featureCardID := r.PathValue("featureCardID")

		overviewResult, err := callAPI(r.Context(), deps.API, http.MethodGet, "/api/v1/features/"+url.PathEscape(featureCardID), nil)
		if err != nil {
			writeInternalErrorPage(w)
			return
		}
		if !overviewResult.OK {
			writeAPIErrorPage(w, overviewResult)
			return
		}
		var overviewBody struct {
			Feature apiFeatureCardDTO `json:"feature"`
		}
		if err := decodeInto(overviewResult, &overviewBody); err != nil {
			writeInternalErrorPage(w)
			return
		}

		if overviewBody.Feature.CapabilityArtifactID == "" {
			render(w, http.StatusOK, "revisions", mapRevisionsPageData(featureCardID, "", nil, apiCurrentRevisionDTO{}, apiResolutionRationaleDTO{}))
			return
		}

		revisionsResult, err := callAPI(r.Context(), deps.API, http.MethodGet,
			"/api/v1/capabilities/"+url.PathEscape(overviewBody.Feature.CapabilityArtifactID)+"/revisions", nil)
		if err != nil {
			writeInternalErrorPage(w)
			return
		}
		if !revisionsResult.OK {
			writeAPIErrorPage(w, revisionsResult)
			return
		}
		var revisionsBody struct {
			Revisions []apiRevisionDTO      `json:"revisions"`
			Current   apiCurrentRevisionDTO `json:"current"`
		}
		if err := decodeInto(revisionsResult, &revisionsBody); err != nil {
			writeInternalErrorPage(w)
			return
		}
		var rationale apiResolutionRationaleDTO
		if err := decodeRationale(revisionsResult, &rationale); err != nil {
			writeInternalErrorPage(w)
			return
		}

		render(w, http.StatusOK, "revisions", mapRevisionsPageData(featureCardID, overviewBody.Feature.CapabilityArtifactID, revisionsBody.Revisions, revisionsBody.Current, rationale))
	}
}

// handleRequirements renders screen 4 (FF-001 §3.4) from Q4 alone --
// effective requirements and per-requirement readiness are both already
// keyed by featureCardID, with no need for Q3 first.
func handleRequirements(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		featureCardID := r.PathValue("featureCardID")

		result, err := callAPI(r.Context(), deps.API, http.MethodGet, "/api/v1/features/"+url.PathEscape(featureCardID)+"/state", nil)
		if err != nil {
			writeInternalErrorPage(w)
			return
		}
		if !result.OK {
			writeAPIErrorPage(w, result)
			return
		}
		var state apiEngineeringStateDTO
		if err := decodeInto(result, &state); err != nil {
			writeInternalErrorPage(w)
			return
		}

		render(w, http.StatusOK, "requirements", mapRequirementsPageData(featureCardID, capabilityIDFromState(state), state.EffectiveRequirements, state.Readiness.PerRequirement))
	}
}

// handleDecisions renders screen 5 (FF-001 §3.5) from Q4 alone.
func handleDecisions(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		featureCardID := r.PathValue("featureCardID")

		result, err := callAPI(r.Context(), deps.API, http.MethodGet, "/api/v1/features/"+url.PathEscape(featureCardID)+"/state", nil)
		if err != nil {
			writeInternalErrorPage(w)
			return
		}
		if !result.OK {
			writeAPIErrorPage(w, result)
			return
		}
		var state apiEngineeringStateDTO
		if err := decodeInto(result, &state); err != nil {
			writeInternalErrorPage(w)
			return
		}

		render(w, http.StatusOK, "decisions", mapDecisionsPageData(featureCardID, capabilityIDFromState(state), state.ApplicableDecisions))
	}
}

// handleValidation renders screen 6 (FF-001 §3.6), composing Q4 (plan
// activities, per-requirement readiness and claims) with Q5 (filtered to
// execution.recorded events by the view model) -- the endpoint-minimization
// review found no dedicated endpoint owns this dataset; Q4 and Q5 already
// do (docs/spec/020-read-surface-extension.md §7).
func handleValidation(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		featureCardID := r.PathValue("featureCardID")

		stateResult, err := callAPI(r.Context(), deps.API, http.MethodGet, "/api/v1/features/"+url.PathEscape(featureCardID)+"/state", nil)
		if err != nil {
			writeInternalErrorPage(w)
			return
		}
		if !stateResult.OK {
			writeAPIErrorPage(w, stateResult)
			return
		}
		var state apiEngineeringStateDTO
		if err := decodeInto(stateResult, &state); err != nil {
			writeInternalErrorPage(w)
			return
		}

		events, ok := fetchAllTimelineEvents(w, r, deps, featureCardID)
		if !ok {
			return
		}

		render(w, http.StatusOK, "validation", mapValidationPageData(featureCardID, capabilityIDFromState(state), state.ValidationPlan, state.Readiness.PerRequirement, events))
	}
}

// handleTimeline renders screen 7 (FF-001 §3.7) from Q5. The kind filter
// is a query parameter read here and applied at render time by the view
// model (open question N3) -- Q5 itself takes no filter parameter.
func handleTimeline(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		featureCardID := r.PathValue("featureCardID")
		selectedKind := r.URL.Query().Get("kind")

		result, err := callAPI(r.Context(), deps.API, http.MethodGet, "/api/v1/features/"+url.PathEscape(featureCardID)+"/timeline", nil)
		if err != nil {
			writeInternalErrorPage(w)
			return
		}
		if !result.OK {
			writeAPIErrorPage(w, result)
			return
		}
		var body struct {
			Dated   []apiTimelineEventDTO `json:"dated"`
			Undated []apiTimelineEventDTO `json:"undated"`
		}
		if err := decodeInto(result, &body); err != nil {
			writeInternalErrorPage(w)
			return
		}

		render(w, http.StatusOK, "timeline", mapTimelinePageData(featureCardID, selectedKind, body.Dated, body.Undated))
	}
}

// capabilityIDFromState extracts the capability artifact ID Q4 itself
// never returns directly, from the one field of its response that already
// carries it -- the current revision's key -- rather than issuing a
// separate Q3 call every requirements/decisions/validation screen would
// otherwise need. Empty when no revision has ever been accepted.
func capabilityIDFromState(state apiEngineeringStateDTO) string {
	if state.CurrentRevision.Found && state.CurrentRevision.Revision != nil {
		return state.CurrentRevision.Revision.ArtifactID
	}
	return ""
}

// fetchAllTimelineEvents calls Q5 and returns dated and undated events
// concatenated, for a screen (Validation) that filters by kind rather than
// by date-presence. ok is false when a response was already written.
func fetchAllTimelineEvents(w http.ResponseWriter, r *http.Request, deps Dependencies, featureCardID string) ([]apiTimelineEventDTO, bool) {
	result, err := callAPI(r.Context(), deps.API, http.MethodGet, "/api/v1/features/"+url.PathEscape(featureCardID)+"/timeline", nil)
	if err != nil {
		writeInternalErrorPage(w)
		return nil, false
	}
	if !result.OK {
		writeAPIErrorPage(w, result)
		return nil, false
	}
	var body struct {
		Dated   []apiTimelineEventDTO `json:"dated"`
		Undated []apiTimelineEventDTO `json:"undated"`
	}
	if err := decodeInto(result, &body); err != nil {
		writeInternalErrorPage(w)
		return nil, false
	}
	events := make([]apiTimelineEventDTO, 0, len(body.Dated)+len(body.Undated))
	events = append(events, body.Dated...)
	events = append(events, body.Undated...)
	return events, true
}
