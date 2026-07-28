// Package ui is the Phase B minimal browser UI (FF-021, AD-028). It speaks
// to the engineering model only through the existing HTTP API, in-process
// over net/http -- never through internal/application, internal/engineering,
// a repository, or UnitOfWork directly (TestUIDoesNotImportApplicationOrInfrastructure).
// A page handler decodes a request, calls the API through callAPI, maps the
// API's JSON into a view model, and renders a template -- nothing else. A
// form handler additionally translates form fields into the API's JSON
// request shape and 303-redirects on success.
package ui

import (
	"log/slog"
	"net/http"
)

// Dependencies are injected rather than constructed inside this package --
// cmd/featureforge is the sole composition root (AD-024), exactly as
// internal/transport/http.Dependencies already established. API is the
// existing, unmodified API handler (internal/transport/http.NewHandler's
// return value); this package never builds its own.
type Dependencies struct {
	API    http.Handler
	Logger *slog.Logger
}

// logger returns deps.Logger, or the default logger if none was supplied,
// so handlers never need a nil check.
func (d Dependencies) logger() *slog.Logger {
	if d.Logger != nil {
		return d.Logger
	}
	return slog.Default()
}

// NewHandler builds the complete Phase B UI handler from deps. It returns
// http.Handler, not *http.Server: cmd/featureforge owns the server
// lifecycle, exactly as internal/transport/http.NewHandler does.
func NewHandler(deps Dependencies) http.Handler {
	return withMiddleware(newRouter(deps), deps.logger())
}
