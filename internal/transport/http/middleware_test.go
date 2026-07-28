package http

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRecoverMiddlewarePreservesHTTPContract proves a panic inside a
// handler is caught by withMiddleware's recovery layer rather than
// escaping to the server, and that the response it writes is FF-018's own
// JSON error shape at 500 -- not a bare connection close, and not the
// panic value itself (FF-018 §14.3, §18 criterion 10). Exercised through
// withMiddleware, not recoverMiddleware alone, so the doc comment's
// ordering claim -- recovery outermost, so a panic unwinds past
// logMiddleware's post-call log line straight to the deferred recover --
// is what actually runs, not merely one layer in isolation.
func TestRecoverMiddlewarePreservesHTTPContract(t *testing.T) {
	panicking := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom: a detail that must never reach the client")
	})
	handler := withMiddleware(panicking, slog.Default())

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)

	func() {
		defer func() {
			if rec := recover(); rec != nil {
				t.Fatalf("panic escaped withMiddleware: %v", rec)
			}
		}()
		handler.ServeHTTP(w, r)
	}()

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", ct)
	}
	if !strings.Contains(w.Body.String(), `"code":"internal_error"`) {
		t.Errorf("expected the JSON error shape with code internal_error, got %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "boom") {
		t.Error("the panic value leaked into the response body")
	}
}

// TestNonPanickingHandlerIsUnaffectedByRecovery proves withMiddleware adds
// no overhead-visible behaviour to the ordinary path: a handler that
// writes normally is passed through untouched.
func TestNonPanickingHandlerIsUnaffectedByRecovery(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{}}`))
	})
	handler := withMiddleware(ok, slog.Default())

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if w.Body.String() != `{"data":{}}` {
		t.Errorf("body = %s, want it passed through unchanged", w.Body.String())
	}
}
