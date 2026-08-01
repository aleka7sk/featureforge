package http_test

import (
	"net/http"
	"testing"

	transporthttp "github.com/aleka7sk/featureforge/internal/transport/http"
)

// assertPriorRevisionValidationSurvives is the M-1 regression
// (docs/reports/m5-publication-remediation.md, D1/D2), run through the real
// HTTP timeline endpoint rather than internal/application directly, so the
// fix is proven at the layer a client actually observes. The fixture is
// deliberately small, not the full FF-011 canonical scenario: that scenario
// never records an execution or claim against a capability revision that
// later becomes superseded (every claim in it targets CAP-1-REV-2, which
// stays current for the rest of the scenario), which is exactly why this
// defect survived every existing test. This fixture creates that condition
// on purpose, then asserts GET .../timeline still reports the prior
// revision's execution, evidence, and claim once a second revision becomes
// current.
func assertPriorRevisionValidationSurvives(t *testing.T, handler http.Handler) {
	t.Helper()
	mustPost(t, handler, "/api/v1/projects", map[string]any{"project_id": "PRJ-H1", "name": "History regression"})
	mustPost(t, handler, "/api/v1/features", map[string]any{"feature_card_id": "FC-H1", "project_id": "PRJ-H1", "title": "Feature"})
	mustPost(t, handler, "/api/v1/capabilities", map[string]any{
		"feature_card_id": "FC-H1", "artifact_id": "CAP-H1", "revision_id": "CAP-H1-REV-1",
		"content": map[string]any{"schema_version": 1, "title": "T", "problem_statement": "P",
			"acceptance_criteria": []map[string]any{{"key": "AC-1", "text": "History remains visible."}}},
	})
	mustPost(t, handler, "/api/v1/capabilities/CAP-H1/acceptances", map[string]any{
		"record_id": "ACC-H1", "revision_id": "CAP-H1-REV-1", "state": "accepted",
	})
	mustPost(t, handler, "/api/v1/requirements", map[string]any{
		"artifact_id": "REQ-H1", "revision_id": "REQ-H1-REV-1", "acceptance_record_id": "ACC-REQ-H1",
		"source_capability_revision_id": "CAP-H1-REV-1", "source_acceptance_criterion_key": "AC-1",
		"statement": "S", "subject_artifact_id": "CAP-H1",
	})
	mustPost(t, handler, "/api/v1/validation/plans", map[string]any{
		"artifact_id": "VP-H1", "revision_id": "VP-H1-REV-1", "scope_artifact_id": "CAP-H1",
		"acceptance_record_id": "ACC-VP-H1",
		"activities": []map[string]any{{
			"key": "A-1", "subject_artifact_id": "CAP-H1", "subject_revision_id": "CAP-H1-REV-1",
			"method": "manual-review", "outcome_interpretation": "Satisfied when reviewed.",
			"requirement_artifact_id": "REQ-H1", "requirement_revision_id": "REQ-H1-REV-1",
			"expected_evidence": []string{"reviewer note"},
		}},
	})
	mustPost(t, handler, "/api/v1/validation/runs", map[string]any{
		"execution_id": "ER-H1", "plan_artifact_id": "VP-H1", "plan_revision_id": "VP-H1-REV-1", "activity_key": "A-1",
		"subject_artifact_id": "CAP-H1", "subject_revision_id": "CAP-H1-REV-1", "method": "manual-review", "outcome": "completed",
		"evidence_artifact_id": "EV-H1", "evidence_revision_id": "EV-H1-REV-1", "evidence_locator": "https://evidence.example/EV-H1",
	})
	mustPost(t, handler, "/api/v1/validation/claims", map[string]any{
		"claim_id": "CLM-H1", "scope_artifact_id": "CAP-H1", "subject_artifact_id": "CAP-H1", "subject_revision_id": "CAP-H1-REV-1",
		"requirement_artifact_id": "REQ-H1", "requirement_revision_id": "REQ-H1-REV-1", "outcome": "satisfied", "method": "manual-review",
		"evidence_artifact_id": "EV-H1", "evidence_revision_id": "EV-H1-REV-1", "execution_id": "ER-H1",
	})

	// The second revision, which is what pushed CAP-H1-REV-1's validation
	// activity out of Q5's population before the fix.
	mustPost(t, handler, "/api/v1/capabilities/CAP-H1/revisions", map[string]any{
		"revision_id": "CAP-H1-REV-2",
		"content":     map[string]any{"schema_version": 1, "title": "T2", "problem_statement": "P2"},
	})
	mustPost(t, handler, "/api/v1/capabilities/CAP-H1/acceptances", map[string]any{
		"record_id": "ACC-H2", "revision_id": "CAP-H1-REV-2", "state": "accepted",
	})

	var resp struct {
		Data struct {
			Dated []struct {
				Kind string `json:"kind"`
			} `json:"dated"`
		} `json:"data"`
	}
	rr := getJSON(t, handler, "/api/v1/features/FC-H1/timeline", &resp)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	counts := map[string]int{}
	for _, e := range resp.Data.Dated {
		counts[e.Kind]++
	}
	for _, kind := range []string{"execution.recorded", "evidence.recorded", "claim.recorded"} {
		if counts[kind] != 1 {
			t.Errorf("%s count = %d, want exactly 1 (CAP-H1-REV-1's validation activity must remain visible after CAP-H1-REV-2 became current)", kind, counts[kind])
		}
	}
}

// TestTimelinePreservesPriorRevisionValidation runs the M-1 regression
// against the in-memory adapter.
func TestTimelinePreservesPriorRevisionValidation(t *testing.T) {
	deps := newTestDeps()
	handler := transporthttp.NewHandler(deps)
	assertPriorRevisionValidationSurvives(t, handler)
}

// TestTimelinePreservesPriorRevisionValidationPostgres runs the identical
// regression against PostgreSQL, reusing newPostgresFixtureHTTP exactly as
// TestCanonicalScenarioThroughHTTPPostgres does. Skips cleanly when
// FEATUREFORGE_POSTGRES_TEST_DSN is unset.
func TestTimelinePreservesPriorRevisionValidationPostgres(t *testing.T) {
	uow, rec, clock := newPostgresFixtureHTTP(t)
	handler := transporthttp.NewHandler(transporthttp.Dependencies{
		UOW: uow, Recorder: rec, Inspector: rec, Projector: rec, Clock: clock,
	})
	assertPriorRevisionValidationSurvives(t, handler)
}
