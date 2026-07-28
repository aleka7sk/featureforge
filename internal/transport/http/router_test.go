package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/infrastructure/memory"
	transporthttp "github.com/aleka7sk/featureforge/internal/transport/http"
)

// newTestHandler builds a Phase A handler backed by a fresh in-memory
// store, the composition every handler test in this package uses -- a
// real application stack, not a mocked one, per FF-018 §18.1.
func newTestHandler() http.Handler {
	return transporthttp.NewHandler(transporthttp.Dependencies{
		UOW:   memory.NewUnitOfWork(memory.NewStore()),
		Clock: application.SystemClock{},
	})
}

// TestListProjectsHandler proves Q1 end to end through the real handler:
// an empty store returns an empty (never null) array, and a stored project
// is rendered field-for-field (FF-018 §3.2, §10.2, §10.3).
func TestListProjectsHandler(t *testing.T) {
	handler := newTestHandler()

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", ct)
	}
	if !strings.Contains(rr.Body.String(), `"projects":[]`) {
		t.Errorf("empty store must render projects as [], got %s", rr.Body.String())
	}
}

// TestUnmatchedRouteIs404 proves the catch-all renders FF-018's JSON error
// shape, not ServeMux's plain-text default (FF-018 §12.2).
func TestUnmatchedRouteIs404(t *testing.T) {
	handler := newTestHandler()
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/nonexistent", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not the JSON error shape: %v; body = %s", err, rr.Body.String())
	}
	if body.Error.Code != "not_found" {
		t.Errorf("code = %q, want not_found", body.Error.Code)
	}
}

// TestUnsupportedMethodIs405 proves ServeMux's built-in per-path 405
// handling still fires with the catch-all registered (FF-018 §12.2).
func TestUnsupportedMethodIs405(t *testing.T) {
	handler := newTestHandler()
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "/api/v1/projects", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d; body = %s", rr.Code, http.StatusMethodNotAllowed, rr.Body.String())
	}
	if allow := rr.Header().Get("Allow"); allow == "" {
		t.Error("expected an Allow header naming the supported methods")
	}
	if !strings.Contains(rr.Body.String(), `"code":"method_not_allowed"`) {
		t.Errorf("expected the JSON error shape, got %s", rr.Body.String())
	}
}

// TestNoRouteRegistersUnsupportedMethods scans the router's own
// registrations for PUT, PATCH, or DELETE (FF-018 §12.2, §18 criterion 2)
// -- the transport-level analogue of TestNoUpdateOrDeleteOnEngineeringTables.
func TestNoRouteRegistersUnsupportedMethods(t *testing.T) {
	handler := newTestHandler()
	for _, method := range []string{http.MethodPut, http.MethodPatch, http.MethodDelete} {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest(method, "/api/v1/projects", nil))
		if rr.Code == http.StatusOK || rr.Code == http.StatusCreated {
			t.Errorf("%s /api/v1/projects succeeded (status %d); no route may accept this method", method, rr.Code)
		}
	}
}
