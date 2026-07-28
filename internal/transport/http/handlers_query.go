package http

import (
	"net/http"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/domain"
	"github.com/aleka7sk/featureforge/internal/engineering"
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
			dtos = append(dtos, mapProjectDTO(p))
		}
		writeJSON(w, http.StatusOK, listProjectsResponse{Projects: dtos}, nil)
	}
}

// handleListFeatures implements Q2 (FF-018 §3.2, §10.3).
func handleListFeatures(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rawProjectID, ok := requirePathValue(w, r, "projectID")
		if !ok {
			return
		}
		projectID, err := domain.NewProjectID(rawProjectID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		cards, err := application.ListFeaturesByProject(r.Context(), deps.UOW, projectID)
		if err != nil {
			writeAppError(w, r, deps, err)
			return
		}
		dtos := make([]featureCardDTO, 0, len(cards))
		for _, c := range cards {
			dtos = append(dtos, mapFeatureCardDTO(c))
		}
		writeJSON(w, http.StatusOK, listFeaturesResponse{Features: dtos}, nil)
	}
}

// handleGetFeature implements Q3 (FF-018 §3.2, §10.3): the feature card
// plus the whole of Q4's state, composed by application.GetFeatureOverview.
func handleGetFeature(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		featureCardID, ok := requireFeatureCardID(w, r)
		if !ok {
			return
		}
		overview, err := application.GetFeatureOverview(r.Context(), deps.UOW, deps.Projector, featureCardID)
		if err != nil {
			writeAppError(w, r, deps, err)
			return
		}
		state, rationale := mapEngineeringStateDTO(overview.State)
		writeJSON(w, http.StatusOK, featureOverviewResponse{
			Feature: mapFeatureCardDTO(overview.FeatureCard), State: state,
		}, rationale)
	}
}

// handleGetFeatureState implements Q4 (FF-018 §3.2, §10.3).
func handleGetFeatureState(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		featureCardID, ok := requireFeatureCardID(w, r)
		if !ok {
			return
		}
		state, err := application.GetFeatureEngineeringStateForCard(r.Context(), deps.UOW, deps.Projector, featureCardID)
		if err != nil {
			writeAppError(w, r, deps, err)
			return
		}
		data, rationale := mapEngineeringStateDTO(state)
		writeJSON(w, http.StatusOK, data, rationale)
	}
}

// handleGetFeatureTimeline implements Q5 (FF-018 §3.2, §10.3). Rationale
// is rendered per event inside data, not at the envelope's top level
// (dto_query.go's timelineEventDTO doc comment explains why); the
// envelope's own rationale argument is nil here deliberately.
func handleGetFeatureTimeline(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		featureCardID, ok := requireFeatureCardID(w, r)
		if !ok {
			return
		}
		timeline, err := application.GetFeatureTimelineForCard(r.Context(), deps.UOW, featureCardID)
		if err != nil {
			writeAppError(w, r, deps, err)
			return
		}
		writeJSON(w, http.StatusOK, mapTimelineResponse(timeline), nil)
	}
}

// handleListCapabilityRevisions implements Q6 (FF-018 §3.2, §10.3).
func handleListCapabilityRevisions(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		artifactID, ok := requirePathValue(w, r, "artifactID")
		if !ok {
			return
		}
		result, err := application.GetCapabilityRevisions(r.Context(), deps.UOW, artifactID)
		if err != nil {
			writeAppError(w, r, deps, err)
			return
		}
		revisions := make([]revisionDTO, 0, len(result.Revisions))
		for _, rev := range result.Revisions {
			sequence := 0
			if result.Current.Found && rev.Revision.Key == result.Current.Revision.Key {
				sequence = result.Current.Sequence
			}
			revisions = append(revisions, mapRevisionWithContentDTO(rev.Revision, rev.Content, rev.HasContent, sequence))
		}
		data := capabilityRevisionsResponse{Revisions: revisions, Current: mapCurrentRevisionDTO(result.Current)}
		writeJSON(w, http.StatusOK, data, mapResolutionRationaleDTO(result.Current.Rationale))
	}
}

// handleGetCapabilityRevision implements Q7 (FF-018 §3.2, §10.3). Found is
// false, with no error, when the revision does not exist; the handler
// maps that directly to 404 rather than manufacturing ErrNotFound (FF-018
// §3.2 "Q7 404 semantics").
func handleGetCapabilityRevision(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		artifactID, ok := requirePathValue(w, r, "artifactID")
		if !ok {
			return
		}
		revisionID, ok := requirePathValue(w, r, "revisionID")
		if !ok {
			return
		}
		key, err := engineering.NewRevisionKey(artifactID, revisionID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		result, found, err := application.GetCapabilityRevision(r.Context(), deps.UOW, key)
		if err != nil {
			writeAppError(w, r, deps, err)
			return
		}
		if !found {
			writeError(w, http.StatusNotFound, "not_found", "no revision matches "+key.String())
			return
		}
		writeJSON(w, http.StatusOK, mapRevisionWithContentDTO(result.Revision, result.Content, result.HasContent, 0), nil)
	}
}

// requireFeatureCardID reads and validates the {featureCardID} path value,
// shared by Q3, Q4, and Q5.
func requireFeatureCardID(w http.ResponseWriter, r *http.Request) (domain.FeatureCardID, bool) {
	raw, ok := requirePathValue(w, r, "featureCardID")
	if !ok {
		return domain.FeatureCardID{}, false
	}
	id, err := domain.NewFeatureCardID(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return domain.FeatureCardID{}, false
	}
	return id, true
}
