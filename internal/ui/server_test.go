package ui_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering/peos"
	"github.com/aleka7sk/featureforge/internal/infrastructure/memory"
	transporthttp "github.com/aleka7sk/featureforge/internal/transport/http"
	"github.com/aleka7sk/featureforge/internal/ui"
)

// newTestHandler builds a UI handler backed by a real, in-process API
// handler over a fresh in-memory store -- proving AD-028's bridge against
// the real API, not a mock (mirrors internal/transport/http's own
// newTestHandler convention).
func newTestHandler() http.Handler {
	_, _, handler := newTestStack()
	return handler
}

func newTestStack() (application.UnitOfWork, peos.Recorder, http.Handler) {
	uow := memory.NewUnitOfWork(memory.NewStore())
	recorder := peos.NewRecorder()
	if err := application.EnsureLifecycleConfiguration(context.Background(), uow, recorder, recorder); err != nil {
		panic(err)
	}
	api := transporthttp.NewHandler(transporthttp.Dependencies{
		UOW:       uow,
		Recorder:  recorder,
		Inspector: recorder,
		Projector: recorder,
		Clock:     application.SystemClock{},
	})
	return uow, recorder, ui.NewHandler(ui.Dependencies{API: api})
}

// TestProjectsPageRendersFromAPI proves the whole AD-028 pipeline end to
// end: a GET to the UI's root route reaches the real API in-process, and
// an empty store renders the explicit empty state, not a blank panel.
func TestProjectsPageRendersFromAPI(t *testing.T) {
	handler := newTestHandler()
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	if !strings.Contains(rr.Body.String(), "No projects yet.") {
		t.Errorf("expected the empty state, got %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "<h1>Projects</h1>") {
		t.Errorf("expected the Projects heading, got %s", rr.Body.String())
	}
}

// TestStaticStylesheetServed proves the embedded asset is reachable and
// carries a deterministic content type (FF-021 §11).
func TestStaticStylesheetServed(t *testing.T) {
	handler := newTestHandler()
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/static/style.css", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Errorf("Content-Type = %q, want text/css", ct)
	}
	if rr.Body.Len() == 0 {
		t.Error("expected non-empty stylesheet")
	}
}

// TestUnknownRouteRendersUIOwnNotFound proves the UI's own 404 page
// renders, not ServeMux's plain-text default and not a route matching
// unrelated paths under the root pattern (FF-021 §4).
func TestUnknownRouteRendersUIOwnNotFound(t *testing.T) {
	handler := newTestHandler()
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/nonexistent", nil))

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Page not found") {
		t.Errorf("expected the UI's own 404 page, got %s", rr.Body.String())
	}
}

// TestProjectsPageListsRealProject proves the pipeline renders real data,
// not just the empty state -- a project created through the real API
// (never through internal/application directly, which this package cannot
// import) appears on the rendered page.
func TestProjectsPageListsRealProject(t *testing.T) {
	api := transporthttp.NewHandler(transporthttp.Dependencies{
		UOW:       memory.NewUnitOfWork(memory.NewStore()),
		Recorder:  peos.NewRecorder(),
		Inspector: peos.NewRecorder(),
		Projector: peos.NewRecorder(),
		Clock:     application.SystemClock{},
	})
	handler := ui.NewHandler(ui.Dependencies{API: api})

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(`{"project_id":"PRJ-1","name":"Pilot"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRR := httptest.NewRecorder()
	api.ServeHTTP(createRR, createReq)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("seeding PRJ-1: status = %d, want 201; body = %s", createRR.Code, createRR.Body.String())
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Pilot") {
		t.Errorf("expected the project name Pilot in the rendered page, got %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "/projects/PRJ-1") {
		t.Errorf("expected a link to /projects/PRJ-1, got %s", rr.Body.String())
	}
}
