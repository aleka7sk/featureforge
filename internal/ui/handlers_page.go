package ui

import (
	"context"
	"net/http"
	"net/url"
)

// Every screen's data is assembled by a load*PageData function taking only
// a context and the identifiers its route carries -- never an
// *http.Request or http.ResponseWriter. A GET handler calls its loader and
// renders on success or writes the problem on failure; a command form
// handler (handlers_form.go) calls the same loader to re-render the
// originating screen, with FormError/FormValues set, after a correctable
// failure (FF-021 §12). This is the one seam that keeps read and
// write-error-recovery from duplicating the Q1-Q7 composition logic.

// handleProjects renders screen 1 (FF-001 §3.1) from Q1.
func handleProjects(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, problem := loadProjectsPageData(r.Context(), deps)
		if problem != nil {
			problem.write(w)
			return
		}
		render(w, http.StatusOK, "projects", data)
	}
}

func loadProjectsPageData(ctx context.Context, deps Dependencies) (projectsPageData, *pageProblem) {
	result, err := callAPI(ctx, deps.API, http.MethodGet, "/api/v1/projects", nil)
	if err != nil {
		return projectsPageData{}, &pageProblem{internal: true}
	}
	if !result.OK {
		return projectsPageData{}, &pageProblem{api: result}
	}
	var body struct {
		Projects []apiProjectDTO `json:"projects"`
	}
	if err := decodeInto(result, &body); err != nil {
		return projectsPageData{}, &pageProblem{internal: true}
	}
	return mapProjectsPageData(body.Projects), nil
}

// handleProjectDetail renders screen 1's "open project" view (FF-001
// §3.1): the project's own identity plus its feature cards (Q2). There is
// no single-project query (Q1 lists every project; FF-020's
// endpoint-minimization review found no case where the existing 19
// operations are insufficient), so the project is located by filtering
// Q1's list -- the same "reuse the existing broad query" pattern the API
// itself uses for discovery (AD-025). A projectID absent from that list is
// reported through found=false, not a *pageProblem, because "no such
// project" is not itself an API or plumbing failure -- the caller decides
// what that means (404 for a GET, a form error for a POST that raced a
// deletion, though nothing in this domain is ever deleted).
func handleProjectDetail(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID := r.PathValue("projectID")
		data, found, problem := loadProjectDetailPageData(r.Context(), deps, projectID)
		if problem != nil {
			problem.write(w)
			return
		}
		if !found {
			notFoundPage(w, r)
			return
		}
		render(w, http.StatusOK, "project_detail", data)
	}
}

func loadProjectDetailPageData(ctx context.Context, deps Dependencies, projectID string) (projectDetailPageData, bool, *pageProblem) {
	listResult, err := callAPI(ctx, deps.API, http.MethodGet, "/api/v1/projects", nil)
	if err != nil {
		return projectDetailPageData{}, false, &pageProblem{internal: true}
	}
	if !listResult.OK {
		return projectDetailPageData{}, false, &pageProblem{api: listResult}
	}
	var listBody struct {
		Projects []apiProjectDTO `json:"projects"`
	}
	if err := decodeInto(listResult, &listBody); err != nil {
		return projectDetailPageData{}, false, &pageProblem{internal: true}
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
		return projectDetailPageData{}, false, nil
	}

	featuresResult, err := callAPI(ctx, deps.API, http.MethodGet, "/api/v1/projects/"+url.PathEscape(projectID)+"/features", nil)
	if err != nil {
		return projectDetailPageData{}, false, &pageProblem{internal: true}
	}
	if !featuresResult.OK {
		return projectDetailPageData{}, false, &pageProblem{api: featuresResult}
	}
	var featuresBody struct {
		Features []apiFeatureCardDTO `json:"features"`
	}
	if err := decodeInto(featuresResult, &featuresBody); err != nil {
		return projectDetailPageData{}, false, &pageProblem{internal: true}
	}
	return mapProjectDetailPageData(project, featuresBody.Features), true, nil
}

// handleFeatureOverview renders screen 2 (FF-001 §3.2) from Q3 (feature
// identity, engineering state, its rationale), Q7 for the current
// revision's title (Q4's embedded current_revision carries no content --
// FF-020 §5 scopes that to Q6/Q7), and Q5 for recent history.
func handleFeatureOverview(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		featureCardID := r.PathValue("featureCardID")
		data, problem := loadFeatureOverviewPageData(r.Context(), deps, featureCardID)
		if problem != nil {
			problem.write(w)
			return
		}
		render(w, http.StatusOK, "feature_overview", data)
	}
}

func loadFeatureOverviewPageData(ctx context.Context, deps Dependencies, featureCardID string) (featureOverviewPageData, *pageProblem) {
	overviewResult, err := callAPI(ctx, deps.API, http.MethodGet, "/api/v1/features/"+url.PathEscape(featureCardID), nil)
	if err != nil {
		return featureOverviewPageData{}, &pageProblem{internal: true}
	}
	if !overviewResult.OK {
		return featureOverviewPageData{}, &pageProblem{api: overviewResult}
	}
	var overviewBody struct {
		Feature apiFeatureCardDTO      `json:"feature"`
		State   apiEngineeringStateDTO `json:"state"`
	}
	if err := decodeInto(overviewResult, &overviewBody); err != nil {
		return featureOverviewPageData{}, &pageProblem{internal: true}
	}
	var rationale apiEngineeringStateRationaleDTO
	if err := decodeRationale(overviewResult, &rationale); err != nil {
		return featureOverviewPageData{}, &pageProblem{internal: true}
	}

	var currentRevisionTitle string
	if cur := overviewBody.State.CurrentRevision; cur.Found && cur.Revision != nil {
		revResult, err := callAPI(ctx, deps.API, http.MethodGet,
			"/api/v1/capabilities/"+url.PathEscape(cur.Revision.ArtifactID)+"/revisions/"+url.PathEscape(cur.Revision.RevisionID), nil)
		if err != nil {
			return featureOverviewPageData{}, &pageProblem{internal: true}
		}
		if revResult.OK {
			var revBody apiRevisionDTO
			if err := decodeInto(revResult, &revBody); err != nil {
				return featureOverviewPageData{}, &pageProblem{internal: true}
			}
			if revBody.Content != nil {
				currentRevisionTitle = revBody.Content.Title
			}
		}
	}

	timelineResult, err := callAPI(ctx, deps.API, http.MethodGet, "/api/v1/features/"+url.PathEscape(featureCardID)+"/timeline", nil)
	if err != nil {
		return featureOverviewPageData{}, &pageProblem{internal: true}
	}
	if !timelineResult.OK {
		return featureOverviewPageData{}, &pageProblem{api: timelineResult}
	}
	var timelineBody struct {
		Dated []apiTimelineEventDTO `json:"dated"`
	}
	if err := decodeInto(timelineResult, &timelineBody); err != nil {
		return featureOverviewPageData{}, &pageProblem{internal: true}
	}
	recent := timelineBody.Dated
	if len(recent) > 5 {
		recent = recent[len(recent)-5:]
	}

	return mapFeatureOverviewPageData(overviewBody.Feature, overviewBody.State, rationale, currentRevisionTitle, recent), nil
}

// handleRevisions renders screen 3 (FF-001 §3.3): Q3 for the feature's
// capability artifact ID, then Q6 for every revision and its acceptance
// rationale. A feature with no capability yet renders the screen's own
// empty state rather than a Q6 call with an empty artifact ID.
func handleRevisions(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		featureCardID := r.PathValue("featureCardID")
		data, problem := loadRevisionsPageData(r.Context(), deps, featureCardID)
		if problem != nil {
			problem.write(w)
			return
		}
		render(w, http.StatusOK, "revisions", data)
	}
}

func loadRevisionsPageData(ctx context.Context, deps Dependencies, featureCardID string) (revisionsPageData, *pageProblem) {
	overviewResult, err := callAPI(ctx, deps.API, http.MethodGet, "/api/v1/features/"+url.PathEscape(featureCardID), nil)
	if err != nil {
		return revisionsPageData{}, &pageProblem{internal: true}
	}
	if !overviewResult.OK {
		return revisionsPageData{}, &pageProblem{api: overviewResult}
	}
	var overviewBody struct {
		Feature apiFeatureCardDTO `json:"feature"`
	}
	if err := decodeInto(overviewResult, &overviewBody); err != nil {
		return revisionsPageData{}, &pageProblem{internal: true}
	}

	if overviewBody.Feature.CapabilityArtifactID == "" {
		return mapRevisionsPageData(featureCardID, "", nil, apiCurrentRevisionDTO{}, apiResolutionRationaleDTO{}), nil
	}

	revisionsResult, err := callAPI(ctx, deps.API, http.MethodGet,
		"/api/v1/capabilities/"+url.PathEscape(overviewBody.Feature.CapabilityArtifactID)+"/revisions", nil)
	if err != nil {
		return revisionsPageData{}, &pageProblem{internal: true}
	}
	if !revisionsResult.OK {
		return revisionsPageData{}, &pageProblem{api: revisionsResult}
	}
	var revisionsBody struct {
		Revisions []apiRevisionDTO      `json:"revisions"`
		Current   apiCurrentRevisionDTO `json:"current"`
	}
	if err := decodeInto(revisionsResult, &revisionsBody); err != nil {
		return revisionsPageData{}, &pageProblem{internal: true}
	}
	var rationale apiResolutionRationaleDTO
	if err := decodeRationale(revisionsResult, &rationale); err != nil {
		return revisionsPageData{}, &pageProblem{internal: true}
	}

	return mapRevisionsPageData(featureCardID, overviewBody.Feature.CapabilityArtifactID, revisionsBody.Revisions, revisionsBody.Current, rationale), nil
}

// handleRequirements renders screen 4 (FF-001 §3.4) from Q4 alone --
// effective requirements and per-requirement readiness are both already
// keyed by featureCardID, with no need for Q3 first.
func handleRequirements(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		featureCardID := r.PathValue("featureCardID")
		data, problem := loadRequirementsPageData(r.Context(), deps, featureCardID)
		if problem != nil {
			problem.write(w)
			return
		}
		render(w, http.StatusOK, "requirements", data)
	}
}

func loadRequirementsPageData(ctx context.Context, deps Dependencies, featureCardID string) (requirementsPageData, *pageProblem) {
	state, problem := loadEngineeringState(ctx, deps, featureCardID)
	if problem != nil {
		return requirementsPageData{}, problem
	}
	return mapRequirementsPageData(featureCardID, capabilityIDFromState(state), state.EffectiveRequirements, state.Readiness.PerRequirement), nil
}

// handleDecisions renders screen 5 (FF-001 §3.5) from Q4 alone.
func handleDecisions(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		featureCardID := r.PathValue("featureCardID")
		data, problem := loadDecisionsPageData(r.Context(), deps, featureCardID)
		if problem != nil {
			problem.write(w)
			return
		}
		render(w, http.StatusOK, "decisions", data)
	}
}

func loadDecisionsPageData(ctx context.Context, deps Dependencies, featureCardID string) (decisionsPageData, *pageProblem) {
	state, problem := loadEngineeringState(ctx, deps, featureCardID)
	if problem != nil {
		return decisionsPageData{}, problem
	}
	data := mapDecisionsPageData(featureCardID, capabilityIDFromState(state), state.ApplicableDecisions)
	if state.CurrentRevision.Found && state.CurrentRevision.Revision != nil {
		data.SubjectRevisionID = state.CurrentRevision.Revision.RevisionID
	}
	return data, nil
}

// handleValidation renders screen 6 (FF-001 §3.6), composing Q4 (plan
// activities, per-requirement readiness and claims) with Q5 (filtered to
// execution.recorded events by the view model) -- the endpoint-minimization
// review found no dedicated endpoint owns this dataset; Q4 and Q5 already
// do (docs/spec/020-read-surface-extension.md §7).
func handleValidation(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		featureCardID := r.PathValue("featureCardID")
		data, problem := loadValidationPageData(r.Context(), deps, featureCardID)
		if problem != nil {
			problem.write(w)
			return
		}
		render(w, http.StatusOK, "validation", data)
	}
}

func loadValidationPageData(ctx context.Context, deps Dependencies, featureCardID string) (validationPageData, *pageProblem) {
	state, problem := loadEngineeringState(ctx, deps, featureCardID)
	if problem != nil {
		return validationPageData{}, problem
	}
	events, problem := loadAllTimelineEvents(ctx, deps, featureCardID)
	if problem != nil {
		return validationPageData{}, problem
	}
	return mapValidationPageData(featureCardID, capabilityIDFromState(state), state.ValidationPlan, state.Readiness.PerRequirement, events), nil
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

// loadEngineeringState is Q4 alone, shared by the Requirements, Decisions,
// and Validation loaders.
func loadEngineeringState(ctx context.Context, deps Dependencies, featureCardID string) (apiEngineeringStateDTO, *pageProblem) {
	result, err := callAPI(ctx, deps.API, http.MethodGet, "/api/v1/features/"+url.PathEscape(featureCardID)+"/state", nil)
	if err != nil {
		return apiEngineeringStateDTO{}, &pageProblem{internal: true}
	}
	if !result.OK {
		return apiEngineeringStateDTO{}, &pageProblem{api: result}
	}
	var state apiEngineeringStateDTO
	if err := decodeInto(result, &state); err != nil {
		return apiEngineeringStateDTO{}, &pageProblem{internal: true}
	}
	return state, nil
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

// loadAllTimelineEvents calls Q5 and returns dated and undated events
// concatenated, for a screen (Validation) that filters by kind rather than
// by date-presence.
func loadAllTimelineEvents(ctx context.Context, deps Dependencies, featureCardID string) ([]apiTimelineEventDTO, *pageProblem) {
	result, err := callAPI(ctx, deps.API, http.MethodGet, "/api/v1/features/"+url.PathEscape(featureCardID)+"/timeline", nil)
	if err != nil {
		return nil, &pageProblem{internal: true}
	}
	if !result.OK {
		return nil, &pageProblem{api: result}
	}
	var body struct {
		Dated   []apiTimelineEventDTO `json:"dated"`
		Undated []apiTimelineEventDTO `json:"undated"`
	}
	if err := decodeInto(result, &body); err != nil {
		return nil, &pageProblem{internal: true}
	}
	events := make([]apiTimelineEventDTO, 0, len(body.Dated)+len(body.Undated))
	events = append(events, body.Dated...)
	events = append(events, body.Undated...)
	return events, nil
}
