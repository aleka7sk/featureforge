package ui

import (
	"log/slog"
	"net/http"
	"time"
)

// withMiddleware wraps mux with panic recovery and request logging,
// mirroring internal/transport/http/middleware.go's withMiddleware exactly
// -- recovery outermost, so a panic inside a handler unwinds past
// logMiddleware's post-call log line straight to recoverMiddleware's
// deferred recover, never left uncaught.
func withMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	return recoverMiddleware(logMiddleware(next, logger), logger)
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func logMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		logger.Info("ui request", "method", r.Method, "path", r.URL.Path, "status", sw.status, "duration", time.Since(start))
	})
}

// recoverMiddleware renders the same generic error page writeErrorPage
// would for an unexpected API failure, so a panicking handler never
// leaves the browser with a bare connection close or a leaked panic
// value (mirrors internal/transport/http's recoverMiddleware).
func recoverMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Error("ui panic recovered", "method", r.Method, "path", r.URL.Path, "panic", rec)
				writeErrorPage(w, http.StatusInternalServerError, "Something went wrong", "An unexpected error occurred.")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
