package ui

import "net/http"

// newRouter registers every Phase B page and form route (FF-021 §4). Page
// routes are GET; form routes are POST, one per command, redirecting back
// to the screen that shows the result (AD-028). Route ownership is
// disjoint from internal/transport/http's: the UI never registers a
// pattern under /api/v1, and the API handler this package calls never
// registers a browser route -- each package owns its own ServeMux;
// cmd/featureforge composes the two under separate prefixes (FF-021 §4).
func newRouter(deps Dependencies) http.Handler {
	mux := http.NewServeMux()

	// "/{$}" matches only the exact root path -- a bare "/" would match
	// every unmatched path as a fallback (the same net/http.ServeMux
	// shadowing FF-018 §16 step 4 already found and solved), which would
	// make every genuinely unmatched route look like the Projects screen
	// instead of 404ing.
	mux.HandleFunc("GET /{$}", handleProjects(deps))
	mux.HandleFunc("GET /projects/{projectID}", handleProjectDetail(deps))
	mux.HandleFunc("GET /features/{featureCardID}", handleFeatureOverview(deps))
	mux.HandleFunc("GET /features/{featureCardID}/revisions", handleRevisions(deps))
	mux.HandleFunc("GET /features/{featureCardID}/requirements", handleRequirements(deps))
	mux.HandleFunc("GET /features/{featureCardID}/decisions", handleDecisions(deps))
	mux.HandleFunc("GET /features/{featureCardID}/validation", handleValidation(deps))
	mux.HandleFunc("GET /features/{featureCardID}/timeline", handleTimeline(deps))
	mux.HandleFunc("GET /static/style.css", handleStaticCSS)

	// Command forms (AD-028), one per command, in FF-011 canonical order.
	mux.HandleFunc("POST /projects", handleCreateProject(deps))
	mux.HandleFunc("POST /projects/{projectID}/features", handleCreateFeature(deps))
	mux.HandleFunc("POST /features/{featureCardID}/capability", handleEstablishCapability(deps))
	mux.HandleFunc("POST /features/{featureCardID}/lifecycle", handleAssignLifecycle(deps))
	mux.HandleFunc("POST /features/{featureCardID}/revisions", handleReviseCapability(deps))
	mux.HandleFunc("POST /features/{featureCardID}/revisions/{revisionID}/acceptance", handleAcceptRevision(deps))
	mux.HandleFunc("POST /features/{featureCardID}/requirements", handleEstablishRequirement(deps))
	mux.HandleFunc("POST /features/{featureCardID}/decisions", handleRecordDecision(deps))
	mux.HandleFunc("POST /features/{featureCardID}/validation-plan", handleEstablishPlan(deps))
	mux.HandleFunc("POST /features/{featureCardID}/validation-runs", handleRecordRun(deps))
	mux.HandleFunc("POST /features/{featureCardID}/claims", handleRecordClaim(deps))
	mux.HandleFunc("POST /features/{featureCardID}/claims/corrections", handleCorrectClaim(deps))

	return withNotFound(mux)
}

// withNotFound renders the UI's own 404 page in place of ServeMux's
// plain-text default, mirroring internal/transport/http's
// withJSONNotFoundAndMethodNotAllowed technique but for HTML instead of
// JSON.
func withNotFound(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nw := &notFoundInterceptor{ResponseWriter: w}
		mux.ServeHTTP(nw, r)
		if nw.status == http.StatusNotFound {
			notFoundPage(w, r)
		}
	})
}

type notFoundInterceptor struct {
	http.ResponseWriter
	status int
}

func (w *notFoundInterceptor) WriteHeader(status int) {
	w.status = status
	if status != http.StatusNotFound {
		w.ResponseWriter.WriteHeader(status)
	}
}

func (w *notFoundInterceptor) Write(b []byte) (int, error) {
	if w.status == http.StatusNotFound {
		return len(b), nil
	}
	return w.ResponseWriter.Write(b)
}
