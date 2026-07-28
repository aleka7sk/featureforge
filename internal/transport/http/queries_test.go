package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	transporthttp "github.com/aleka7sk/featureforge/internal/transport/http"
)

func getJSON(t *testing.T, handler http.Handler, path string, out any) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
	if out != nil && rr.Code == http.StatusOK {
		if err := json.Unmarshal(rr.Body.Bytes(), out); err != nil {
			t.Fatalf("decoding response: %v; body = %s", err, rr.Body.String())
		}
	}
	return rr
}

// seedForQueries drives C1-C11 through the real handler so the query tests
// below read real, engine-produced state rather than hand-built fixtures.
// It intentionally omits C12/C6, which the canonical order test already
// covers; queries only need one requirement, one decision, one plan
// activity, one execution, and one claim to exercise every field.
func seedForQueries(t *testing.T, handler http.Handler) {
	t.Helper()
	steps := []struct {
		path string
		body map[string]any
	}{
		{"/api/v1/projects", map[string]any{"project_id": "PRJ-1", "name": "Pilot"}},
		{"/api/v1/features", map[string]any{"feature_card_id": "FC-1", "project_id": "PRJ-1", "title": "Homework", "description": "d"}},
		{"/api/v1/capabilities", map[string]any{
			"feature_card_id": "FC-1", "artifact_id": "CAP-1", "revision_id": "CAP-1-REV-1",
			"content": map[string]any{"schema_version": 1, "title": "Homework", "problem_statement": "No follow-up."},
		}},
		{"/api/v1/capabilities/CAP-1/acceptances", map[string]any{"record_id": "ACC-1", "revision_id": "CAP-1-REV-1", "state": "accepted"}},
		{"/api/v1/requirements", map[string]any{
			"artifact_id": "REQ-1", "revision_id": "REQ-1-REV-1", "statement": "Statement.", "subject_artifact_id": "CAP-1",
		}},
		{"/api/v1/decisions", map[string]any{
			"decision_id": "DEC-1", "subject_artifact_id": "CAP-1", "subject_revision_id": "CAP-1-REV-1",
			"question": "Q?", "outcome_statement": "Outcome.",
			"evidence_artifact_id": "EV-0", "evidence_revision_id": "EV-0-REV-1",
		}},
		{"/api/v1/validation/plans", map[string]any{
			"artifact_id": "VP-1", "revision_id": "VP-1-REV-1", "scope_artifact_id": "CAP-1",
			"activities": []map[string]any{{
				"key": "A-1", "subject_artifact_id": "CAP-1", "subject_revision_id": "CAP-1-REV-1",
				"method": "manual-review", "outcome_interpretation": "Satisfied when reviewed.",
				"requirement_artifact_id": "REQ-1", "requirement_revision_id": "REQ-1-REV-1",
				"expected_evidence": []string{"note"},
			}},
		}},
		{"/api/v1/validation/runs", map[string]any{
			"execution_id": "ER-1", "plan_artifact_id": "VP-1", "plan_revision_id": "VP-1-REV-1", "activity_key": "A-1",
			"subject_artifact_id": "CAP-1", "subject_revision_id": "CAP-1-REV-1",
			"method": "manual-review", "outcome": "completed",
			"evidence_artifact_id": "EV-1", "evidence_revision_id": "EV-1-REV-1", "evidence_locator": "https://evidence.example/EV-1",
		}},
		{"/api/v1/validation/claims", map[string]any{
			"claim_id": "CLM-1", "scope_artifact_id": "CAP-1",
			"subject_artifact_id": "CAP-1", "subject_revision_id": "CAP-1-REV-1",
			"requirement_artifact_id": "REQ-1", "requirement_revision_id": "REQ-1-REV-1",
			"outcome": "satisfied", "method": "manual-review",
			"evidence_artifact_id": "EV-1", "evidence_revision_id": "EV-1-REV-1", "execution_id": "ER-1",
			"reasoning": "Because.",
		}},
	}
	for _, s := range steps {
		rr := postJSON(t, handler, s.path, s.body, nil)
		if rr.Code != http.StatusCreated {
			t.Fatalf("seeding %s: status = %d, want 201; body = %s", s.path, rr.Code, rr.Body.String())
		}
	}
}

func TestListFeaturesHandler(t *testing.T) {
	deps := newTestDeps()
	handler := transporthttp.NewHandler(deps)
	seedForQueries(t, handler)

	var resp struct {
		Data struct {
			Features []struct {
				FeatureCardID        string `json:"feature_card_id"`
				CapabilityArtifactID string `json:"capability_artifact_id"`
			} `json:"features"`
		} `json:"data"`
	}
	rr := getJSON(t, handler, "/api/v1/projects/PRJ-1/features", &resp)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	if len(resp.Data.Features) != 1 || resp.Data.Features[0].FeatureCardID != "FC-1" {
		t.Fatalf("features = %+v, want exactly [FC-1]", resp.Data.Features)
	}
	if resp.Data.Features[0].CapabilityArtifactID != "CAP-1" {
		t.Errorf("capability_artifact_id = %q, want CAP-1", resp.Data.Features[0].CapabilityArtifactID)
	}
}

func TestGetFeatureStateHandler(t *testing.T) {
	deps := newTestDeps()
	handler := transporthttp.NewHandler(deps)
	seedForQueries(t, handler)

	var resp struct {
		Data struct {
			CurrentRevision struct {
				Found    bool `json:"found"`
				Revision struct {
					ArtifactID string `json:"artifact_id"`
					RevisionID string `json:"revision_id"`
				} `json:"revision"`
			} `json:"current_revision"`
			EffectiveRequirements []struct {
				ArtifactID string `json:"artifact_id"`
			} `json:"effective_requirements"`
			ApplicableDecisions []struct {
				DecisionID string `json:"decision_id"`
			} `json:"applicable_decisions"`
			Readiness struct {
				Status         string `json:"status"`
				PerRequirement []struct {
					RequirementArtifactID string `json:"requirement_artifact_id"`
					HasClaim              bool   `json:"has_claim"`
				} `json:"per_requirement"`
			} `json:"readiness"`
		} `json:"data"`
		Rationale struct {
			CurrentRevision struct {
				Rule string `json:"rule"`
			} `json:"current_revision"`
		} `json:"rationale"`
	}
	rr := getJSON(t, handler, "/api/v1/features/FC-1/state", &resp)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	if !resp.Data.CurrentRevision.Found || resp.Data.CurrentRevision.Revision.RevisionID != "CAP-1-REV-1" {
		t.Errorf("current_revision = %+v, want found with CAP-1-REV-1", resp.Data.CurrentRevision)
	}
	if len(resp.Data.EffectiveRequirements) != 1 || resp.Data.EffectiveRequirements[0].ArtifactID != "REQ-1" {
		t.Errorf("effective_requirements = %+v, want exactly [REQ-1]", resp.Data.EffectiveRequirements)
	}
	if len(resp.Data.ApplicableDecisions) != 1 || resp.Data.ApplicableDecisions[0].DecisionID != "DEC-1" {
		t.Errorf("applicable_decisions = %+v, want exactly [DEC-1]", resp.Data.ApplicableDecisions)
	}
	if resp.Data.Readiness.Status != "ready" {
		t.Errorf("readiness.status = %q, want ready", resp.Data.Readiness.Status)
	}
	if len(resp.Data.Readiness.PerRequirement) != 1 || !resp.Data.Readiness.PerRequirement[0].HasClaim {
		t.Errorf("readiness.per_requirement = %+v, want exactly one requirement with a claim", resp.Data.Readiness.PerRequirement)
	}
	if resp.Rationale.CurrentRevision.Rule == "" {
		t.Error("rationale.current_revision.rule must be present for a derived answer (FF-018 §10.1)")
	}
}

// TestGetFeatureStateHandler_ReadSurfaceContent proves every FF-020 field
// reaches the wire through Q4: requirement statement text, decision
// question/outcome_statement/basis, validation_plan and its activities,
// and the current claim's reasoning and criterion_keys -- none invented,
// all reproducing exactly what seedForQueries recorded.
func TestGetFeatureStateHandler_ReadSurfaceContent(t *testing.T) {
	deps := newTestDeps()
	handler := transporthttp.NewHandler(deps)
	seedForQueries(t, handler)

	var resp struct {
		Data struct {
			EffectiveRequirements []struct {
				ArtifactID string `json:"artifact_id"`
				Statement  string `json:"statement"`
			} `json:"effective_requirements"`
			ApplicableDecisions []struct {
				DecisionID       string `json:"decision_id"`
				Question         string `json:"question"`
				OutcomeStatement string `json:"outcome_statement"`
				Basis            struct {
					Evidence []string `json:"evidence"`
				} `json:"basis"`
			} `json:"applicable_decisions"`
			ValidationPlan struct {
				Found      bool   `json:"found"`
				ArtifactID string `json:"artifact_id"`
				RevisionID string `json:"revision_id"`
				Activities []struct {
					Key                   string   `json:"key"`
					OutcomeInterpretation string   `json:"outcome_interpretation"`
					ExpectedEvidence      []string `json:"expected_evidence"`
				} `json:"activities"`
			} `json:"validation_plan"`
			Readiness struct {
				PerRequirement []struct {
					RequirementArtifactID string   `json:"requirement_artifact_id"`
					Reasoning             string   `json:"reasoning"`
					CriterionKeys         []string `json:"criterion_keys"`
					Corrects              string   `json:"corrects"`
				} `json:"per_requirement"`
			} `json:"readiness"`
		} `json:"data"`
	}
	rr := getJSON(t, handler, "/api/v1/features/FC-1/state", &resp)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}

	if len(resp.Data.EffectiveRequirements) != 1 || resp.Data.EffectiveRequirements[0].Statement != "Statement." {
		t.Errorf("effective_requirements = %+v, want REQ-1 with statement %q", resp.Data.EffectiveRequirements, "Statement.")
	}

	if len(resp.Data.ApplicableDecisions) != 1 {
		t.Fatalf("applicable_decisions = %+v, want exactly one", resp.Data.ApplicableDecisions)
	}
	dec := resp.Data.ApplicableDecisions[0]
	if dec.Question != "Q?" || dec.OutcomeStatement != "Outcome." {
		t.Errorf("decision = %+v, want question=%q outcome_statement=%q", dec, "Q?", "Outcome.")
	}
	if len(dec.Basis.Evidence) != 1 {
		t.Errorf("decision basis.evidence = %v, want exactly one entry (from the existing EvidenceKeys projection)", dec.Basis.Evidence)
	}

	if !resp.Data.ValidationPlan.Found || resp.Data.ValidationPlan.ArtifactID != "VP-1" || resp.Data.ValidationPlan.RevisionID != "VP-1-REV-1" {
		t.Fatalf("validation_plan = %+v, want found VP-1/VP-1-REV-1", resp.Data.ValidationPlan)
	}
	if len(resp.Data.ValidationPlan.Activities) != 1 || resp.Data.ValidationPlan.Activities[0].Key != "A-1" {
		t.Fatalf("validation_plan.activities = %+v, want exactly [A-1]", resp.Data.ValidationPlan.Activities)
	}
	if resp.Data.ValidationPlan.Activities[0].OutcomeInterpretation != "Satisfied when reviewed." {
		t.Errorf("activity outcome_interpretation = %q", resp.Data.ValidationPlan.Activities[0].OutcomeInterpretation)
	}

	if len(resp.Data.Readiness.PerRequirement) != 1 {
		t.Fatalf("per_requirement = %+v, want exactly one", resp.Data.Readiness.PerRequirement)
	}
	per := resp.Data.Readiness.PerRequirement[0]
	if per.Reasoning != "Because." {
		t.Errorf("per_requirement.reasoning = %q, want %q", per.Reasoning, "Because.")
	}
	if len(per.CriterionKeys) == 0 {
		t.Error("per_requirement.criterion_keys must not be empty")
	}
	if per.Corrects != "" {
		t.Errorf("CLM-1 corrects nothing, got %q", per.Corrects)
	}
}

func TestGetFeatureHandler(t *testing.T) {
	deps := newTestDeps()
	handler := transporthttp.NewHandler(deps)
	seedForQueries(t, handler)

	var resp struct {
		Data struct {
			Feature struct {
				FeatureCardID string `json:"feature_card_id"`
			} `json:"feature"`
			State struct {
				CurrentRevision struct {
					Found bool `json:"found"`
				} `json:"current_revision"`
			} `json:"state"`
		} `json:"data"`
	}
	rr := getJSON(t, handler, "/api/v1/features/FC-1", &resp)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	if resp.Data.Feature.FeatureCardID != "FC-1" {
		t.Errorf("feature.feature_card_id = %q, want FC-1", resp.Data.Feature.FeatureCardID)
	}
	if !resp.Data.State.CurrentRevision.Found {
		t.Error("state.current_revision.found = false, want true")
	}
}

func TestGetFeatureTimelineHandler(t *testing.T) {
	deps := newTestDeps()
	handler := transporthttp.NewHandler(deps)
	seedForQueries(t, handler)

	var resp struct {
		Data struct {
			Dated []struct {
				Kind       string   `json:"kind"`
				Rationale  string   `json:"rationale"`
				References []string `json:"references"`
			} `json:"dated"`
		} `json:"data"`
	}
	rr := getJSON(t, handler, "/api/v1/features/FC-1/timeline", &resp)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	if len(resp.Data.Dated) == 0 {
		t.Fatal("expected dated events")
	}
	for _, e := range resp.Data.Dated {
		if e.Rationale == "" {
			t.Errorf("event kind %q has no per-event rationale", e.Kind)
		}
	}
	if !strings.Contains(rr.Body.String(), `"rationale"`) {
		t.Error("expected rationale to be present per event")
	}
	if strings.Contains(rr.Body.String(), `"rationale":{`) {
		t.Error("Q5 must not carry a top-level rationale object (FF-018 §10.3 deliberate deviation)")
	}

	// FF-020 §5: execution.recorded and claim.recorded references are
	// enriched with the record's own EvidenceKeys/ExecutionKeys, so a
	// reader can follow what an event produced or relied on.
	sawEnrichedExecution := false
	for _, e := range resp.Data.Dated {
		if e.Kind == "execution.recorded" && len(e.References) > 1 {
			sawEnrichedExecution = true
		}
	}
	if !sawEnrichedExecution {
		t.Error("expected an execution.recorded event with more than its bare subject in references (evidence key)")
	}
}

func TestListCapabilityRevisionsHandler(t *testing.T) {
	deps := newTestDeps()
	handler := transporthttp.NewHandler(deps)
	seedForQueries(t, handler)
	postJSON(t, handler, "/api/v1/capabilities/CAP-1/revisions", map[string]any{
		"revision_id": "CAP-1-REV-2",
		"content":     map[string]any{"schema_version": 1, "title": "Homework", "problem_statement": "No follow-up.", "user_outcome": "Updated."},
	}, nil)

	var resp struct {
		Data struct {
			Revisions []struct {
				RevisionID string `json:"revision_id"`
				Sequence   int    `json:"sequence"`
				Content    *struct {
					Title       string `json:"title"`
					UserOutcome string `json:"user_outcome"`
				} `json:"content"`
			} `json:"revisions"`
			Current struct {
				Found    bool `json:"found"`
				Revision struct {
					RevisionID string `json:"revision_id"`
				} `json:"revision"`
			} `json:"current"`
		} `json:"data"`
	}
	rr := getJSON(t, handler, "/api/v1/capabilities/CAP-1/revisions", &resp)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	if len(resp.Data.Revisions) != 2 {
		t.Fatalf("revisions = %+v, want 2", resp.Data.Revisions)
	}
	if resp.Data.Revisions[0].RevisionID != "CAP-1-REV-1" || resp.Data.Revisions[1].RevisionID != "CAP-1-REV-2" {
		t.Errorf("revisions not in ascending key order: %+v", resp.Data.Revisions)
	}
	// Only revision 1 is accepted, so it -- not revision 2 -- is current.
	if !resp.Data.Current.Found || resp.Data.Current.Revision.RevisionID != "CAP-1-REV-1" {
		t.Errorf("current = %+v, want found with CAP-1-REV-1", resp.Data.Current)
	}

	// FF-020 §2 class A: every revision carries its own structured
	// content -- revision 1's content is unaffected by revision 2's,
	// exactly as FF-001 §3.3 requires ("revision 1 must be as readable
	// after revision 2 exists as it was before").
	if resp.Data.Revisions[0].Content == nil || resp.Data.Revisions[0].Content.UserOutcome != "" {
		t.Errorf("revision 1 content = %+v, want present with no user_outcome (never set)", resp.Data.Revisions[0].Content)
	}
	if resp.Data.Revisions[1].Content == nil || resp.Data.Revisions[1].Content.UserOutcome != "Updated." {
		t.Errorf("revision 2 content = %+v, want present with user_outcome %q", resp.Data.Revisions[1].Content, "Updated.")
	}
}

func TestGetCapabilityRevisionHandler(t *testing.T) {
	deps := newTestDeps()
	handler := transporthttp.NewHandler(deps)
	seedForQueries(t, handler)

	var resp struct {
		Data struct {
			ArtifactID string `json:"artifact_id"`
			RevisionID string `json:"revision_id"`
			Content    *struct {
				Title            string `json:"title"`
				ProblemStatement string `json:"problem_statement"`
			} `json:"content"`
		} `json:"data"`
	}
	rr := getJSON(t, handler, "/api/v1/capabilities/CAP-1/revisions/CAP-1-REV-1", &resp)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	if resp.Data.ArtifactID != "CAP-1" || resp.Data.RevisionID != "CAP-1-REV-1" {
		t.Errorf("got %+v, want CAP-1/CAP-1-REV-1", resp.Data)
	}
	if resp.Data.Content == nil || resp.Data.Content.Title != "Homework" || resp.Data.Content.ProblemStatement != "No follow-up." {
		t.Errorf("content = %+v, want title=Homework problem_statement=%q", resp.Data.Content, "No follow-up.")
	}

	rr = getJSON(t, handler, "/api/v1/capabilities/CAP-1/revisions/NONEXISTENT", nil)
	if rr.Code != http.StatusNotFound {
		t.Errorf("missing revision: status = %d, want 404; body = %s", rr.Code, rr.Body.String())
	}
	if code := errorCode(t, rr); code != "not_found" {
		t.Errorf("code = %q, want not_found", code)
	}
}

// TestGetFeatureStateNoCapability proves a card with no linked capability
// yields a well-formed, empty-but-Incomplete state rather than an error
// (FF-018 §3.2, §6.4 step 2).
func TestGetFeatureStateNoCapability(t *testing.T) {
	deps := newTestDeps()
	handler := transporthttp.NewHandler(deps)
	postJSON(t, handler, "/api/v1/projects", map[string]any{"project_id": "PRJ-1", "name": "Pilot"}, nil)
	postJSON(t, handler, "/api/v1/features", map[string]any{
		"feature_card_id": "FC-1", "project_id": "PRJ-1", "title": "Homework", "description": "",
	}, nil)

	var resp struct {
		Data struct {
			CurrentRevision struct {
				Found bool `json:"found"`
			} `json:"current_revision"`
			Readiness struct {
				Status string `json:"status"`
			} `json:"readiness"`
		} `json:"data"`
	}
	rr := getJSON(t, handler, "/api/v1/features/FC-1/state", &resp)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	if resp.Data.CurrentRevision.Found {
		t.Error("expected current_revision.found = false for a card with no capability")
	}
	if resp.Data.Readiness.Status != "incomplete" {
		t.Errorf("readiness.status = %q, want incomplete", resp.Data.Readiness.Status)
	}
}

// TestGetFeatureStateMissingCard proves an unknown FeatureCardID is 404
// (FF-018 §6.4 step 1).
func TestGetFeatureStateMissingCard(t *testing.T) {
	deps := newTestDeps()
	handler := transporthttp.NewHandler(deps)

	rr := getJSON(t, handler, "/api/v1/features/FC-MISSING/state", nil)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body = %s", rr.Code, rr.Body.String())
	}
}
