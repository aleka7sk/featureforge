// Package http is the FeatureForge HTTP transport (FF-018 §5.1,
// AD-022/AD-023, extended by FF-024).
// It decodes requests, invokes exactly one internal/application entry
// point, encodes responses, and maps errors -- nothing else. It never
// holds application.Repositories and never calls UnitOfWork.Do (FF-015
// §4.1); every read or write reaches the application layer through an
// entry point that owns its own transaction.
package http

import (
	"log/slog"
	"net/http"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/proposal"
)

// Dependencies are injected rather than constructed inside this package --
// cmd/featureforge is the sole composition root (FF-018 §5.2, AD-024).
type Dependencies struct {
	UOW       application.UnitOfWork
	Recorder  application.ProposalEngineeringRecorder
	Inspector application.ProposalReplayInspector
	Projector application.EngineeringProjector
	Generator proposal.Generator
	Clock     application.Clock
	Logger    *slog.Logger
}

// logger returns deps.Logger, or the default logger if none was supplied,
// so handlers never need a nil check.
func (d Dependencies) logger() *slog.Logger {
	if d.Logger != nil {
		return d.Logger
	}
	return slog.Default()
}

// NewHandler builds the complete HTTP handler from deps. It
// returns http.Handler, not *http.Server: cmd/featureforge owns the
// server lifecycle (FF-018 §5.1), which is what keeps httptest usage in
// this package's own tests trivial.
func NewHandler(deps Dependencies) http.Handler {
	return withMiddleware(newRouter(deps), deps.logger())
}
