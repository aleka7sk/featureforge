package http

import (
	"log/slog"
	"net/http"
	"time"
)

// withMiddleware wraps mux with panic recovery and request logging
// (FF-018 §14.3). Recovery is the outermost layer: a panic inside mux
// unwinds past logMiddleware's post-call log line straight to
// recoverMiddleware's deferred recover, so it is never left uncaught.
// This does not interfere with UnitOfWork.Do's own panic path -- Do rolls
// back and re-panics before this middleware ever sees anything, so
// rollback always happens first (FF-018 §14.3).
func withMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	return recoverMiddleware(logMiddleware(next, logger), logger)
}

// statusWriter captures the status code a handler wrote, so logMiddleware
// can log it after the fact without every handler reporting it directly.
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
		logger.Info("request", "method", r.Method, "path", r.URL.Path, "status", sw.status, "duration", time.Since(start))
	})
}

func recoverMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Error("panic recovered", "method", r.Method, "path", r.URL.Path, "panic", rec)
				writeError(w, http.StatusInternalServerError, "internal_error", internalErrorMessage)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
