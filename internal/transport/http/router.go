package http

import "net/http"

// newRouter registers every Phase A route (FF-018 §3): GET and POST only,
// all under /api/v1, in the §3 matrix's order. No PUT, PATCH, or DELETE
// pattern is ever registered (FF-018 §12.2, §18 criterion 2).
func newRouter(deps Dependencies) http.Handler {
	mux := http.NewServeMux()

	// Query endpoints (FF-018 §3.2).
	mux.HandleFunc("GET /api/v1/projects", handleListProjects(deps))
	mux.HandleFunc("GET /api/v1/projects/{projectID}/features", handleListFeatures(deps))
	mux.HandleFunc("GET /api/v1/features/{featureCardID}", handleGetFeature(deps))
	mux.HandleFunc("GET /api/v1/features/{featureCardID}/state", handleGetFeatureState(deps))
	mux.HandleFunc("GET /api/v1/features/{featureCardID}/timeline", handleGetFeatureTimeline(deps))
	mux.HandleFunc("GET /api/v1/capabilities/{artifactID}/revisions", handleListCapabilityRevisions(deps))
	mux.HandleFunc("GET /api/v1/capabilities/{artifactID}/revisions/{revisionID}", handleGetCapabilityRevision(deps))

	// Command endpoints, in canonical-scenario order (FF-018 §16 step 6):
	// project -> feature -> capability -> requirement -> decision -> plan
	// -> run -> claim -> correction -> lifecycle.
	mux.HandleFunc("POST /api/v1/projects", handleCreateProject(deps))
	mux.HandleFunc("POST /api/v1/features", handleCreateFeature(deps))
	mux.HandleFunc("POST /api/v1/capabilities", handleEstablishCapability(deps))
	mux.HandleFunc("POST /api/v1/capabilities/{artifactID}/revisions", handleReviseCapability(deps))
	mux.HandleFunc("POST /api/v1/capabilities/{artifactID}/acceptances", handleAcceptRevision(deps))
	mux.HandleFunc("POST /api/v1/requirements", handleEstablishRequirement(deps))
	mux.HandleFunc("POST /api/v1/decisions", handleRecordDecision(deps))
	mux.HandleFunc("POST /api/v1/validation/plans", handleEstablishPlan(deps))
	mux.HandleFunc("POST /api/v1/validation/runs", handleRecordRun(deps))
	mux.HandleFunc("POST /api/v1/validation/claims", handleRecordClaim(deps))
	mux.HandleFunc("POST /api/v1/validation/claims/corrections", handleCorrectClaim(deps))
	mux.HandleFunc("POST /api/v1/capabilities/{artifactID}/lifecycle", handleAssignLifecycle(deps))

	return withJSONNotFoundAndMethodNotAllowed(mux)
}

// withJSONNotFoundAndMethodNotAllowed rewrites ServeMux's plain-text 404
// and 405 defaults into FF-018's JSON error shape (§8.3, §12.2), without
// registering a route to do it. A bare "/" pattern would be simpler but is
// wrong: ServeMux treats a method-less pattern as matching every method,
// which pre-empts its own automatic 405 for a path registered under a
// different method -- the exact shadowing §12.2 forbids. mux.Handler
// cannot distinguish "truly unmatched" from "method mismatch" either; both
// report an empty pattern. So this runs the real mux and inspects the
// status it actually wrote, which is the only place the two differ.
func withJSONNotFoundAndMethodNotAllowed(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nw := &notFoundInterceptor{ResponseWriter: w}
		mux.ServeHTTP(nw, r)
		switch nw.status {
		case http.StatusNotFound:
			writeError(w, http.StatusNotFound, "not_found", "no route matches "+r.Method+" "+r.URL.Path)
		case http.StatusMethodNotAllowed:
			// ServeMux already set the Allow header directly on w -- Header()
			// is never intercepted below, only WriteHeader/Write are.
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method "+r.Method+" is not allowed for "+r.URL.Path)
		}
	})
}

// notFoundInterceptor lets ServeMux's own routing decision run to
// completion, discarding its plain-text 404/405 body so the caller can
// substitute JSON, while passing every other response straight through
// unmodified.
type notFoundInterceptor struct {
	http.ResponseWriter
	status int
}

func (w *notFoundInterceptor) WriteHeader(status int) {
	w.status = status
	if status != http.StatusNotFound && status != http.StatusMethodNotAllowed {
		w.ResponseWriter.WriteHeader(status)
	}
}

func (w *notFoundInterceptor) Write(b []byte) (int, error) {
	if w.status == http.StatusNotFound || w.status == http.StatusMethodNotAllowed {
		return len(b), nil // discard ServeMux's plain-text body
	}
	return w.ResponseWriter.Write(b)
}
