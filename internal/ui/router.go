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
	mux.HandleFunc("GET /static/style.css", handleStaticCSS)

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
