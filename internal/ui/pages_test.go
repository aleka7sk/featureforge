package ui_test

import (
	"context"
	"encoding/json"
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

// pageFixture is a real API handler (in-memory store) plus the UI handler
// built on top of it (AD-028), seeded with one small but non-trivial
// engineering history: a project, a feature card, two capability
// revisions (one accepted, superseding the first), a lifecycle entry, two
// requirements, a decision with a full basis, a validation plan, an
// executed run, a satisfied claim, and a rejected/correcting claim pair --
// enough to exercise every FF-001 §3 screen's non-empty rendering path,
// not just its empty state (TestProjectsPageListsRealProject already
// covers the empty case).
type pageFixture struct {
	api http.Handler
	ui  http.Handler
}

func seedPageFixture(t *testing.T) pageFixture {
	t.Helper()
	uow := memory.NewUnitOfWork(memory.NewStore())
	recorder := peos.NewRecorder()
	if err := application.EnsureLifecycleConfiguration(context.Background(), uow, recorder, recorder); err != nil {
		t.Fatal(err)
	}
	api := transporthttp.NewHandler(transporthttp.Dependencies{
		UOW:       uow,
		Recorder:  recorder,
		Inspector: recorder,
		Projector: recorder,
		Clock:     application.SystemClock{},
	})
	handler := ui.NewHandler(ui.Dependencies{API: api})

	post(t, api, "/api/v1/projects", map[string]any{"project_id": "PRJ-1", "name": "Pilot"})
	post(t, api, "/api/v1/features", map[string]any{
		"feature_card_id": "FC-1", "project_id": "PRJ-1", "title": "Homework", "description": "After a lesson.",
	})
	post(t, api, "/api/v1/capabilities", map[string]any{
		"feature_card_id": "FC-1", "artifact_id": "CAP-1", "revision_id": "CAP-1-REV-1",
		"content": capabilityContentJSON("No follow-up.", ""),
	})
	post(t, api, "/api/v1/capabilities/CAP-1/acceptances", map[string]any{
		"record_id": "ACC-1", "revision_id": "CAP-1-REV-1", "state": "accepted",
	})
	post(t, api, "/api/v1/capabilities/CAP-1/lifecycle", map[string]any{
		"assignment_id": "LC-1", "state": "drafting", "is_entry": true,
		"transition_record_artifact_id": "TR-1", "transition_record_revision_id": "TR-1-REV-1",
	})
	post(t, api, "/api/v1/requirements", map[string]any{
		"artifact_id": "REQ-1", "revision_id": "REQ-1-REV-1",
		"acceptance_record_id":          "ACC-REQ-1",
		"source_capability_revision_id": "CAP-1-REV-1", "source_acceptance_criterion_key": "AC-1",
		"statement": "Published homework SHALL be visible to the student.", "subject_artifact_id": "CAP-1",
	})
	post(t, api, "/api/v1/requirements", map[string]any{
		"artifact_id": "REQ-2", "revision_id": "REQ-2-REV-1",
		"acceptance_record_id":          "ACC-REQ-2",
		"source_capability_revision_id": "CAP-1-REV-1", "source_acceptance_criterion_key": "AC-2",
		"statement": "Published homework SHALL NOT be visible to other users.", "subject_artifact_id": "CAP-1",
	})
	post(t, api, "/api/v1/decisions", map[string]any{
		"decision_id": "DEC-1", "subject_artifact_id": "CAP-1", "subject_revision_id": "CAP-1-REV-1",
		"question":             "Should homework support an audio attachment?",
		"outcome_statement":    "Homework supports at most one optional audio attachment.",
		"alternatives":         []string{"Store audio inline.", "Store audio externally."},
		"evidence_artifact_id": "EV-1", "evidence_revision_id": "EV-1-REV-1",
		"assumptions":   []string{"Audio files are hosted externally."},
		"constraints":   []string{"No binary storage in the first release."},
		"uncertainties": []string{"Interview sample was small."},
		"rationale":     "Referencing by content address avoids adding binary storage.",
	})
	post(t, api, "/api/v1/capabilities/CAP-1/revisions", map[string]any{
		"revision_id": "CAP-1-REV-2", "content": capabilityContentJSON("No follow-up.", "Now with audio."),
	})
	post(t, api, "/api/v1/capabilities/CAP-1/acceptances", map[string]any{
		"record_id": "ACC-2", "revision_id": "CAP-1-REV-2", "state": "accepted",
	})
	post(t, api, "/api/v1/validation/plans", map[string]any{
		"artifact_id": "VP-1", "revision_id": "VP-1-REV-1", "scope_artifact_id": "CAP-1",
		"acceptance_record_id": "ACC-VP-1",
		"activities": []map[string]any{
			{
				"key": "A-1", "subject_artifact_id": "CAP-1", "subject_revision_id": "CAP-1-REV-2",
				"method": "manual-review", "outcome_interpretation": "Satisfied when visibility is confirmed.",
				"requirement_artifact_id": "REQ-1", "requirement_revision_id": "REQ-1-REV-1",
				"expected_evidence": []string{"Reviewer note"},
			},
			{
				"key": "A-2", "subject_artifact_id": "CAP-1", "subject_revision_id": "CAP-1-REV-2",
				"method": "manual-review", "outcome_interpretation": "Satisfied when exclusion is confirmed.",
				"requirement_artifact_id": "REQ-2", "requirement_revision_id": "REQ-2-REV-1",
				"expected_evidence": []string{"Access review note"},
			},
		},
	})
	post(t, api, "/api/v1/validation/runs", map[string]any{
		"execution_id": "ER-1", "plan_artifact_id": "VP-1", "plan_revision_id": "VP-1-REV-1",
		"activity_key": "A-1", "subject_artifact_id": "CAP-1", "subject_revision_id": "CAP-1-REV-2",
		"method": "manual-review", "outcome": "completed",
		"evidence_artifact_id": "EV-1", "evidence_revision_id": "EV-1-REV-1", "evidence_locator": "https://evidence.example/EV-1",
	})
	post(t, api, "/api/v1/validation/claims", map[string]any{
		"claim_id": "CLM-1", "scope_artifact_id": "CAP-1",
		"subject_artifact_id": "CAP-1", "subject_revision_id": "CAP-1-REV-2",
		"requirement_artifact_id": "REQ-1", "requirement_revision_id": "REQ-1-REV-1",
		"outcome": "satisfied", "method": "manual-review",
		"evidence_artifact_id": "EV-1", "evidence_revision_id": "EV-1-REV-1", "execution_id": "ER-1",
		"reasoning": "The specification states student visibility explicitly.",
	})
	post(t, api, "/api/v1/validation/runs", map[string]any{
		"execution_id": "ER-2", "plan_artifact_id": "VP-1", "plan_revision_id": "VP-1-REV-1",
		"activity_key": "A-2", "subject_artifact_id": "CAP-1", "subject_revision_id": "CAP-1-REV-2",
		"method": "manual-review", "outcome": "completed",
		"evidence_artifact_id": "EV-2", "evidence_revision_id": "EV-2-REV-1", "evidence_locator": "https://evidence.example/EV-2",
	})
	post(t, api, "/api/v1/validation/claims", map[string]any{
		"claim_id": "CLM-2", "scope_artifact_id": "CAP-1",
		"subject_artifact_id": "CAP-1", "subject_revision_id": "CAP-1-REV-2",
		"requirement_artifact_id": "REQ-2", "requirement_revision_id": "REQ-2-REV-1",
		"outcome": "satisfied", "method": "manual-review",
		"evidence_artifact_id": "EV-2", "evidence_revision_id": "EV-2-REV-1", "execution_id": "ER-2",
		"reasoning": "The specification states who may view homework.",
	})
	post(t, api, "/api/v1/validation/runs", map[string]any{
		"execution_id": "ER-3", "plan_artifact_id": "VP-1", "plan_revision_id": "VP-1-REV-1",
		"activity_key": "A-2", "subject_artifact_id": "CAP-1", "subject_revision_id": "CAP-1-REV-2",
		"method": "manual-review", "outcome": "completed",
		"evidence_artifact_id": "EV-3", "evidence_revision_id": "EV-3-REV-1", "evidence_locator": "https://evidence.example/EV-3",
	})
	post(t, api, "/api/v1/validation/claims/corrections", map[string]any{
		"claim_id": "CLM-3", "correction_target": "CLM-2", "correction_kind": "correct",
		"scope_artifact_id": "CAP-1", "subject_artifact_id": "CAP-1", "subject_revision_id": "CAP-1-REV-2",
		"requirement_artifact_id": "REQ-2", "requirement_revision_id": "REQ-2-REV-1",
		"outcome": "not-satisfied", "method": "manual-review",
		"evidence_artifact_id": "EV-3", "evidence_revision_id": "EV-3-REV-1", "execution_id": "ER-3",
		"reasoning": "The original review treated the positive statement as implying exclusion. It does not.",
	})

	return pageFixture{api: api, ui: handler}
}

func post(t *testing.T, handler http.Handler, path string, body map[string]any) {
	t.Helper()
	rr := postJSON(t, handler, path, body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("POST %s: status = %d, want 201; body = %s", path, rr.Code, rr.Body.String())
	}
}

func postJSON(t *testing.T, handler http.Handler, path string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshalling request body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(encoded)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func capabilityContentJSON(problemStatement, userOutcome string) map[string]any {
	return map[string]any{
		"schema_version": 1, "title": "Homework", "problem_statement": problemStatement, "user_outcome": userOutcome,
		"functional_behaviours": []string{"A teacher can publish homework."},
		"constraints":           []string{"Homework is visible only to its own student."},
		"acceptance_criteria": []map[string]any{
			{"key": "AC-1", "text": "Published homework is visible to the intended student."},
			{"key": "AC-2", "text": "Homework is not visible to unrelated users."},
		},
		"dependencies":   []string{},
		"open_questions": []string{},
	}
}

func getPage(t *testing.T, handler http.Handler, path string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
	return rr, rr.Body.String()
}

func TestProjectsPageShowsFeatureCount(t *testing.T) {
	fx := seedPageFixture(t)
	rr, body := getPage(t, fx.ui, "/")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, body)
	}
	if !strings.Contains(body, "<th scope=\"col\">Features</th>") || !strings.Contains(body, "<td>1</td>") {
		t.Errorf("expected the authoritative feature count for PRJ-1, got %s", body)
	}
}

// TestProjectDetailPageListsFeatures proves screen 1's "open project" view
// (Q1 filtered by ID, plus Q2) renders the project's own feature cards.
func TestProjectDetailPageListsFeatures(t *testing.T) {
	fx := seedPageFixture(t)
	rr, body := getPage(t, fx.ui, "/projects/PRJ-1")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, body)
	}
	if !strings.Contains(body, "Pilot") {
		t.Error("expected the project name")
	}
	if !strings.Contains(body, "Homework") || !strings.Contains(body, "/features/FC-1") {
		t.Errorf("expected a link to the feature card, got %s", body)
	}
}

// TestProjectDetailPageUnknownProjectRendersNotFound proves a projectID
// absent from Q1's list renders the UI's own 404, not a blank or panicking
// page.
func TestProjectDetailPageUnknownProjectRendersNotFound(t *testing.T) {
	fx := seedPageFixture(t)
	rr, body := getPage(t, fx.ui, "/projects/PRJ-404")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", rr.Code, body)
	}
}

// TestFeatureOverviewPageRendersState proves screen 2 composes Q3 (feature
// identity, readiness, lifecycle, its rationale) with Q7 (current
// revision's title) and Q5 (recent history) into one page.
func TestFeatureOverviewPageRendersState(t *testing.T) {
	fx := seedPageFixture(t)
	rr, body := getPage(t, fx.ui, "/features/FC-1")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, body)
	}
	if !strings.Contains(body, "CAP-1-REV-2") {
		t.Errorf("expected the current revision id CAP-1-REV-2, got %s", body)
	}
	if !strings.Contains(body, "Homework") {
		t.Error("expected the current revision's title (from Q7)")
	}
	if !strings.Contains(body, "status-drafting") {
		t.Errorf("expected the lifecycle status badge, got %s", body)
	}
	if !strings.Contains(body, "2 requirement") {
		t.Errorf("expected the requirement count, got %s", body)
	}
}

// TestRevisionsPageShowsBothRevisionsIndependently proves screen 3's core
// FF-001 §3.3 property against real data: Revision 1 is as readable as
// Revision 2, and each carries its own acceptance state (joined from Q6's
// rationale.considered[]).
func TestRevisionsPageShowsBothRevisionsIndependently(t *testing.T) {
	fx := seedPageFixture(t)
	rr, body := getPage(t, fx.ui, "/features/FC-1/revisions")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, body)
	}
	if !strings.Contains(body, "CAP-1-REV-1") || !strings.Contains(body, "CAP-1-REV-2") {
		t.Errorf("expected both revisions, got %s", body)
	}
	if !strings.Contains(body, "Now with audio.") {
		t.Error("expected revision 2's user outcome")
	}
	if strings.Count(body, "status-accepted") < 2 {
		t.Errorf("expected both revisions accepted, got %s", body)
	}
}

// TestRequirementsPageShowsStatementsAndClaims proves screen 4 (Q4 alone)
// renders each requirement's statement and its current readiness.
func TestRequirementsPageShowsStatementsAndClaims(t *testing.T) {
	fx := seedPageFixture(t)
	rr, body := getPage(t, fx.ui, "/features/FC-1/requirements")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, body)
	}
	if !strings.Contains(body, "SHALL be visible to the student") {
		t.Error("expected REQ-1's statement")
	}
	if !strings.Contains(body, "CLM-1") {
		t.Errorf("expected REQ-1's current claim CLM-1, got %s", body)
	}
}

func TestRequirementsPageShowsEveryPriorRevision(t *testing.T) {
	fx := seedPageFixture(t)
	post(t, fx.api, "/api/v1/requirements", map[string]any{
		"artifact_id": "REQ-1", "revision_id": "REQ-1-REV-2", "acceptance_record_id": "ACC-REQ-1-REV-2",
		"source_capability_revision_id": "CAP-1-REV-2", "source_acceptance_criterion_key": "AC-1",
		"statement": "Published homework SHALL remain visible after revision.", "subject_artifact_id": "CAP-1",
	})
	rr, body := getPage(t, fx.ui, "/features/FC-1/requirements")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, body)
	}
	for _, want := range []string{
		"REQ-1/REQ-1-REV-1", "Published homework SHALL be visible to the student.",
		"REQ-1/REQ-1-REV-2", "Published homework SHALL remain visible after revision.",
		"Historical immutable revision", "Sequence 1", "Sequence 2",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %q in Requirement history, got %s", want, body)
		}
	}
}

// TestDecisionsPageShowsFullBasis proves screen 5 renders a decision's
// complete basis -- "displayed, not collapsed" (FF-001 §3.5).
func TestDecisionsPageShowsFullBasis(t *testing.T) {
	fx := seedPageFixture(t)
	rr, body := getPage(t, fx.ui, "/features/FC-1/decisions")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, body)
	}
	for _, want := range []string{"DEC-1", "artifact-revision:CAP-1/CAP-1-REV-1", "audio attachment", "EV-1", "hosted externally", "No binary storage", "Interview sample was small"} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %q in the rendered page, got %s", want, body)
		}
	}
}

// TestValidationPageShowsSupersededClaim proves screen 6 shows CLM-2 as
// superseded, with its original outcome intact and a link to CLM-3, which
// corrected it -- FF-001 §3.6: "shown, not hidden."
func TestValidationPageShowsSupersededClaim(t *testing.T) {
	fx := seedPageFixture(t)
	rr, body := getPage(t, fx.ui, "/features/FC-1/validation")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, body)
	}
	if !strings.Contains(body, "VP-1/VP-1-REV-1") {
		t.Error("expected the validation plan artifact and revision ids")
	}
	if !strings.Contains(body, "Reviewer note") || !strings.Contains(body, "Access review note") {
		t.Error("expected each plan activity's evidence expectation")
	}
	if !strings.Contains(body, "claim:CLM-2") {
		t.Errorf("expected the superseded claim CLM-2 to be shown, got %s", body)
	}
	if !strings.Contains(body, `href="#claim:CLM-3"`) || !strings.Contains(body, "corrected by CLM-3") {
		t.Error("expected a real hyperlink to the correcting claim")
	}
	if !strings.Contains(body, "requirement-revision:REQ-2/REQ-2-REV-1") {
		t.Error("expected the claim criterion identity")
	}
}

// TestTimelinePageFiltersByKind proves screen 7's render-time kind filter
// narrows real history without a second query.
func TestTimelinePageFiltersByKind(t *testing.T) {
	fx := seedPageFixture(t)
	rr, body := getPage(t, fx.ui, "/features/FC-1/timeline")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, body)
	}
	if !strings.Contains(body, "capability.accepted") && !strings.Contains(body, "Capability") {
		t.Errorf("expected unfiltered history to include capability events, got %s", body)
	}
	for _, want := range []string{
		"by featureforge:local-user",
		"<strong>Source:</strong>",
		`href="/features/FC-1/timeline#timeline-source-`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected complete, navigable timeline field %q, got %s", want, body)
		}
	}

	rr, filtered := getPage(t, fx.ui, "/features/FC-1/timeline?kind=decision.recorded")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, filtered)
	}
	if !strings.Contains(filtered, "DEC-1") {
		t.Errorf("expected the decision event to survive the filter, got %s", filtered)
	}
	if strings.Contains(filtered, "REQ-1-REV-1") {
		t.Errorf("expected requirement events to be filtered out, got %s", filtered)
	}
}

func TestTimelineReferencePageNavigatesLifecyclePolicyAndRejectsUnknownIdentity(t *testing.T) {
	fx := seedPageFixture(t)
	timelineResponse, timeline := getPage(t, fx.ui, "/features/FC-1/timeline")
	if timelineResponse.Code != http.StatusOK {
		t.Fatalf("timeline status = %d, want 200; body = %s", timelineResponse.Code, timeline)
	}
	policyPath := "/features/FC-1/timeline/reference?identity=LCD-1%2FLCDV-1"
	if !strings.Contains(timeline, `href="`+policyPath+`"`) {
		t.Fatalf("timeline has no navigable lifecycle policy reference %q; body = %s", policyPath, timeline)
	}

	rr, body := getPage(t, fx.ui, policyPath)
	if rr.Code != http.StatusOK {
		t.Fatalf("reference status = %d, want 200; body = %s", rr.Code, body)
	}
	for _, want := range []string{"LCD-1/LCDV-1", "Lifecycle state -&gt; drafting", "validated lifecycle predecessor-chain order", "authoritative source representation", "entry transition", "transitions"} {
		if !strings.Contains(body, want) {
			t.Errorf("lifecycle reference detail missing %q; body = %s", want, body)
		}
	}

	rr, body = getPage(t, fx.ui, "/features/FC-1/timeline/reference?identity=LCD-404%2FLCDV-404")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("unknown reference status = %d, want 404; body = %s", rr.Code, body)
	}
	if !strings.Contains(body, "The feature timeline does not contain this reference.") {
		t.Errorf("unknown reference page lacks an honest not-found explanation: %s", body)
	}
}
