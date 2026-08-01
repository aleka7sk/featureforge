package ui_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering/peos"
	"github.com/aleka7sk/featureforge/internal/infrastructure/memory"
	"github.com/aleka7sk/featureforge/internal/scenario"
	transporthttp "github.com/aleka7sk/featureforge/internal/transport/http"
	"github.com/aleka7sk/featureforge/internal/ui"
)

// postForm submits a UI command form exactly as a browser would --
// application/x-www-form-urlencoded, no JSON -- and returns the recorded
// response, following redirects manually (the test asserts on Location,
// not by chasing it, since chasing would hide a wrong redirect target).
func postForm(t *testing.T, handler http.Handler, path string, values url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func mustPostForm(t *testing.T, handler http.Handler, path string, values url.Values, wantRedirect string) {
	t.Helper()
	rr := postForm(t, handler, path, values)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("POST %s: status = %d, want 303; body = %s", path, rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Location"); got != wantRedirect {
		t.Fatalf("POST %s: Location = %q, want %q", path, got, wantRedirect)
	}
}

// TestCanonicalScenarioThroughUIForms drives the FF-011 canonical
// "Homework after a lesson" lifecycle entirely through the UI's own POST
// routes -- form-encoded, never JSON -- proving AD-028's bridge for every
// one of the twelve commands, not just the three apiclient_test.go
// exercises against a fake handler. Every step 303s to the screen that
// shows its result (POST-Redirect-GET); the final GETs then prove the
// committed state is visible exactly as internal/transport/http's own
// canonical scenario test proves it through the API directly.
func TestCanonicalScenarioThroughUIForms(t *testing.T) {
	uow, recorder, handler := newTestStack()

	// C1 create project.
	mustPostForm(t, handler, "/projects", url.Values{
		"project_id": {"PRJ-1"}, "name": {"Pilot"},
	}, "/projects/PRJ-1")

	// C2 create feature.
	mustPostForm(t, handler, "/projects/PRJ-1/features", url.Values{
		"feature_card_id": {"FC-1"}, "title": {"Homework"}, "description": {"After a lesson."},
	}, "/features/FC-1")

	// C3 establish capability.
	mustPostForm(t, handler, "/features/FC-1/capability", url.Values{
		"artifact_id": {"CAP-1"}, "revision_id": {"CAP-1-REV-1"}, "title": {"Homework"},
		"problem_statement": {"No follow-up."}, "user_outcome": {"Visible homework."},
		"functional_behaviours": {"A teacher can publish homework."},
		"constraints":           {"Visible only to its own student."},
		"acceptance_criteria":   {"AC-1: Published homework is visible to the intended student."},
	}, "/features/FC-1")

	// C5 accept Revision 1 (before it can be the subject of a requirement
	// or decision, and before C6 may establish the lifecycle entry).
	mustPostForm(t, handler, "/features/FC-1/revisions/CAP-1-REV-1/acceptance", url.Values{
		"record_id": {"ACC-1"}, "state": {"accepted"}, "reason": {""},
	}, "/features/FC-1/revisions")

	// C6 establish the drafting lifecycle entry.
	mustPostForm(t, handler, "/features/FC-1/lifecycle", url.Values{
		"assignment_id": {"SA-1"}, "state": {"drafting"}, "is_entry": {"true"},
		"transition_record_artifact_id": {"TR-1"}, "transition_record_revision_id": {"TR-1-REV-0"},
	}, "/features/FC-1")

	// C8 cites an Evidence act. There is intentionally no standalone
	// Evidence form, so seed that prerequisite through the same real
	// engineering repository used by the in-process API.
	recordDecisionEvidenceForUI(t, uow, recorder, time.Now().UTC())
	mustPostForm(t, handler, "/features/FC-1/decisions", url.Values{
		"decision_id": {"DEC-1"}, "question": {"Should homework support audio?"},
		"outcome_statement":    {"Homework supports one optional audio attachment."},
		"alternatives":         {"Store inline.\nStore externally."},
		"evidence_artifact_id": {scenario.DecisionEvidenceID}, "evidence_revision_id": {scenario.DecisionEvidenceID + "-REV-1"},
		"assumptions": {"Audio is hosted externally."}, "constraints": {"No binary storage."},
		"rationale": {"Avoids adding binary storage."},
	}, "/features/FC-1/decisions")

	// C4 revise the capability to Revision 2, then C5 accept it.
	mustPostForm(t, handler, "/features/FC-1/revisions", url.Values{
		"revision_id": {"CAP-1-REV-2"}, "title": {"Homework"},
		"problem_statement": {"No follow-up."}, "user_outcome": {"Now with audio."},
		"functional_behaviours": {"A teacher can publish homework."},
		"acceptance_criteria":   {"AC-1: Published homework is visible to the intended student."},
	}, "/features/FC-1/revisions")
	mustPostForm(t, handler, "/features/FC-1/revisions/CAP-1-REV-2/acceptance", url.Values{
		"record_id": {"ACC-2"}, "state": {"accepted"},
	}, "/features/FC-1/revisions")

	// C7 establishes Requirements against the exact current accepted
	// capability revision and criterion.
	mustPostForm(t, handler, "/features/FC-1/requirements", url.Values{
		"artifact_id": {"REQ-1"}, "revision_id": {"REQ-1-REV-1"},
		"acceptance_record_id":          {"ACC-REQ-1"},
		"source_capability_revision_id": {"CAP-1-REV-2"}, "source_acceptance_criterion_key": {"AC-1"},
		"statement": {"Published homework SHALL be visible to the student."},
	}, "/features/FC-1/requirements")
	mustPostForm(t, handler, "/features/FC-1/requirements", url.Values{
		"artifact_id": {"REQ-2"}, "revision_id": {"REQ-2-REV-1"},
		"acceptance_record_id":          {"ACC-REQ-2"},
		"source_capability_revision_id": {"CAP-1-REV-2"}, "source_acceptance_criterion_key": {"AC-1"},
		"statement": {"Published homework SHALL NOT be visible to other users."},
	}, "/features/FC-1/requirements")

	// The traced Requirements satisfy the drafting -> specified product
	// precondition.
	mustPostForm(t, handler, "/features/FC-1/lifecycle", url.Values{
		"assignment_id": {"SA-2"}, "state": {"specified"},
		"transition_record_artifact_id": {"TR-1"}, "transition_record_revision_id": {"TR-1-REV-1"},
		"transition_key": {"specify"}, "from_assignment_id": {"SA-1"},
	}, "/features/FC-1")

	// C9 establish the validation plan (one activity, against REQ-1).
	mustPostForm(t, handler, "/features/FC-1/validation-plan", url.Values{
		"artifact_id": {"VP-1"}, "revision_id": {"VP-1-REV-1"},
		"acceptance_record_id": {"ACC-VP-1"},
		"activities":           {"A-1|manual-review|Satisfied when visibility is confirmed.|REQ-1|REQ-1-REV-1|Reviewer note"},
	}, "/features/FC-1/validation")

	// C10 records the completed current-plan execution required by the
	// specified -> under-validation transition.
	mustPostForm(t, handler, "/features/FC-1/validation-runs", url.Values{
		"execution_id": {"ER-1"}, "activity_key": {"A-1"}, "method": {"manual-review"}, "outcome": {"completed"},
		"evidence_artifact_id": {"EV-1"}, "evidence_revision_id": {"EV-1-REV-1"}, "evidence_locator": {"https://evidence.example/EV-1"},
	}, "/features/FC-1/validation")
	mustPostForm(t, handler, "/features/FC-1/lifecycle", url.Values{
		"assignment_id": {"SA-3"}, "state": {"under-validation"},
		"transition_record_artifact_id": {"TR-1"}, "transition_record_revision_id": {"TR-1-REV-2"},
		"transition_key": {"begin-validation"}, "from_assignment_id": {"SA-2"},
	}, "/features/FC-1")

	// C11 records a satisfied claim against REQ-1 after validation begins.
	mustPostForm(t, handler, "/features/FC-1/claims", url.Values{
		"claim_id": {"CLM-1"}, "requirement_artifact_id": {"REQ-1"}, "requirement_revision_id": {"REQ-1-REV-1"},
		"outcome": {"satisfied"}, "method": {"manual-review"},
		"evidence_artifact_id": {"EV-1"}, "evidence_revision_id": {"EV-1-REV-1"}, "execution_id": {"ER-1"},
		"reasoning": {"The specification states student visibility explicitly."},
	}, "/features/FC-1/validation")

	// A second run and a corrected claim, proving C12 through the UI.
	mustPostForm(t, handler, "/features/FC-1/validation-runs", url.Values{
		"execution_id": {"ER-2"}, "activity_key": {"A-1"}, "method": {"manual-review"}, "outcome": {"completed"},
		"evidence_artifact_id": {"EV-2"}, "evidence_revision_id": {"EV-2-REV-1"}, "evidence_locator": {"https://evidence.example/EV-2"},
	}, "/features/FC-1/validation")
	mustPostForm(t, handler, "/features/FC-1/claims/corrections", url.Values{
		"claim_id": {"CLM-2"}, "correction_target": {"CLM-1"}, "correction_kind": {"correct"},
		"requirement_artifact_id": {"REQ-1"}, "requirement_revision_id": {"REQ-1-REV-1"},
		"outcome": {"not-satisfied"}, "method": {"manual-review"},
		"evidence_artifact_id": {"EV-2"}, "evidence_revision_id": {"EV-2-REV-1"}, "execution_id": {"ER-2"},
		"reasoning": {"The original review missed an edge case."},
	}, "/features/FC-1/validation")

	// The whole history is now readable through the GET screens, exactly
	// as if it had been entered through JSON -- AD-028 preserved every API
	// semantic (FF-021 §2).
	_, overview := getPage(t, handler, "/features/FC-1")
	if !strings.Contains(overview, "CAP-1-REV-2") || !strings.Contains(overview, "status-under-validation") {
		t.Errorf("expected the current revision and lifecycle state on the overview, got %s", overview)
	}

	_, revisions := getPage(t, handler, "/features/FC-1/revisions")
	if !strings.Contains(revisions, "CAP-1-REV-1") || !strings.Contains(revisions, "CAP-1-REV-2") {
		t.Errorf("expected both revisions, got %s", revisions)
	}
	if strings.Count(revisions, "status-accepted") < 2 {
		t.Errorf("expected both revisions accepted, got %s", revisions)
	}

	_, decisions := getPage(t, handler, "/features/FC-1/decisions")
	if !strings.Contains(decisions, "DEC-1") || !strings.Contains(decisions, scenario.DecisionEvidenceID) {
		t.Errorf("expected the decision and its evidence, got %s", decisions)
	}

	_, validation := getPage(t, handler, "/features/FC-1/validation")
	if !strings.Contains(validation, "claim:CLM-1") {
		t.Errorf("expected the superseded claim CLM-1 shown, not hidden, got %s", validation)
	}
	if !strings.Contains(validation, "corrected by CLM-2") {
		t.Errorf("expected a link from CLM-1 to its corrector CLM-2, got %s", validation)
	}
	if !strings.Contains(validation, "status-not-satisfied") {
		t.Errorf("expected REQ-1's current outcome (not-satisfied, from CLM-2), got %s", validation)
	}
}

// TestCreateProjectForm_CorrectableFailurePreservesInput proves a
// correctable API failure (here: a project_id conflict) re-renders the
// Projects screen with the submitted values echoed back and a client-safe
// message, rather than a blank error page (FF-021 §12).
func TestCreateProjectForm_CorrectableFailurePreservesInput(t *testing.T) {
	api := transporthttp.NewHandler(transporthttp.Dependencies{
		UOW:       memory.NewUnitOfWork(memory.NewStore()),
		Recorder:  peos.NewRecorder(),
		Inspector: peos.NewRecorder(),
		Projector: peos.NewRecorder(),
		Clock:     application.SystemClock{},
	})
	handler := ui.NewHandler(ui.Dependencies{API: api})

	mustPostForm(t, handler, "/projects", url.Values{"project_id": {"PRJ-1"}, "name": {"Pilot"}}, "/projects/PRJ-1")

	rr := postForm(t, handler, "/projects", url.Values{"project_id": {"PRJ-1"}, "name": {"A different name"}})
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body = %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `value="PRJ-1"`) {
		t.Errorf("expected the submitted project_id preserved in the form, got %s", body)
	}
	if !strings.Contains(body, `value="A different name"`) {
		t.Errorf("expected the submitted name preserved in the form, got %s", body)
	}
	if !strings.Contains(body, `role="alert"`) {
		t.Errorf("expected an accessible error notice, got %s", body)
	}
	// The prior project (from before the failed submission) must still be
	// visible -- the current read state stays on screen through a
	// correctable failure, not just a blank form (FF-021 §12).
	if !strings.Contains(body, "Pilot") {
		t.Errorf("expected the existing project list still rendered, got %s", body)
	}
}

func TestEstablishPlanForm_ConflictPreservesMemberIdentity(t *testing.T) {
	handler := newTestHandler()

	mustPostForm(t, handler, "/projects", url.Values{"project_id": {"PRJ-PLAN-PRESERVE"}, "name": {"Pilot"}}, "/projects/PRJ-PLAN-PRESERVE")
	mustPostForm(t, handler, "/projects/PRJ-PLAN-PRESERVE/features", url.Values{
		"feature_card_id": {"FC-PLAN-PRESERVE"}, "title": {"Homework"},
	}, "/features/FC-PLAN-PRESERVE")
	mustPostForm(t, handler, "/features/FC-PLAN-PRESERVE/capability", url.Values{
		"artifact_id": {"CAP-PLAN-PRESERVE"}, "revision_id": {"CAP-PLAN-PRESERVE-REV-1"},
		"title": {"Homework"}, "problem_statement": {"No follow-up."},
		"acceptance_criteria": {"AC-1: Homework is visible."},
	}, "/features/FC-PLAN-PRESERVE")
	mustPostForm(t, handler, "/features/FC-PLAN-PRESERVE/revisions/CAP-PLAN-PRESERVE-REV-1/acceptance", url.Values{
		"record_id": {"ACC-CAP-PLAN-PRESERVE"}, "state": {"accepted"},
	}, "/features/FC-PLAN-PRESERVE/revisions")
	mustPostForm(t, handler, "/features/FC-PLAN-PRESERVE/requirements", url.Values{
		"artifact_id": {"REQ-PLAN-PRESERVE"}, "revision_id": {"REQ-PLAN-PRESERVE-REV-1"},
		"source_capability_revision_id": {"CAP-PLAN-PRESERVE-REV-1"}, "source_acceptance_criterion_key": {"AC-1"},
		"acceptance_record_id": {"ACC-REQ-PLAN-PRESERVE"}, "statement": {"Homework SHALL be visible."},
	}, "/features/FC-PLAN-PRESERVE/requirements")

	activities := "A-PLAN|manual-review|Satisfied when visible|REQ-PLAN-PRESERVE|REQ-PLAN-PRESERVE-REV-1|review note"
	path := "/features/FC-PLAN-PRESERVE/validation-plan"
	mustPostForm(t, handler, path, url.Values{
		"artifact_id": {"VP-PLAN-PRESERVE"}, "revision_id": {"VP-PLAN-PRESERVE-REV-1"},
		"acceptance_record_id": {"ACC-VP-PLAN-PRESERVE"}, "activities": {activities},
	}, "/features/FC-PLAN-PRESERVE/validation")

	rr := postForm(t, handler, path, url.Values{
		"artifact_id": {"VP-PLAN-PRESERVE"}, "revision_id": {"VP-PLAN-PRESERVE-REV-1"},
		"acceptance_record_id": {"ACC-VP-DIFFERENT"}, "activities": {activities},
	})
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body = %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `id="plan_acceptance_record_id"`) || !strings.Contains(body, `value="ACC-VP-DIFFERENT"`) {
		t.Errorf("expected the conflicting member identity preserved in the visible C9 form, got %s", body)
	}
	if !strings.Contains(body, "VP-PLAN-PRESERVE") || !strings.Contains(body, `role="alert"`) {
		t.Errorf("expected committed plan state and accessible conflict notice, got %s", body)
	}
}

func TestEstablishRequirementForm_ConflictPreservesMemberIdentity(t *testing.T) {
	handler := newTestHandler()

	mustPostForm(t, handler, "/projects", url.Values{"project_id": {"PRJ-REQ-PRESERVE"}, "name": {"Pilot"}}, "/projects/PRJ-REQ-PRESERVE")
	mustPostForm(t, handler, "/projects/PRJ-REQ-PRESERVE/features", url.Values{
		"feature_card_id": {"FC-REQ-PRESERVE"}, "title": {"Homework"},
	}, "/features/FC-REQ-PRESERVE")
	mustPostForm(t, handler, "/features/FC-REQ-PRESERVE/capability", url.Values{
		"artifact_id": {"CAP-REQ-PRESERVE"}, "revision_id": {"CAP-REQ-PRESERVE-REV-1"},
		"title": {"Homework"}, "problem_statement": {"No follow-up."},
		"acceptance_criteria": {"AC-1: Homework is visible."},
	}, "/features/FC-REQ-PRESERVE")
	mustPostForm(t, handler, "/features/FC-REQ-PRESERVE/revisions/CAP-REQ-PRESERVE-REV-1/acceptance", url.Values{
		"record_id": {"ACC-CAP-REQ-PRESERVE"}, "state": {"accepted"},
	}, "/features/FC-REQ-PRESERVE/revisions")

	path := "/features/FC-REQ-PRESERVE/requirements"
	mustPostForm(t, handler, path, url.Values{
		"artifact_id": {"REQ-PRESERVE"}, "revision_id": {"REQ-PRESERVE-REV-1"},
		"source_capability_revision_id": {"CAP-REQ-PRESERVE-REV-1"}, "source_acceptance_criterion_key": {"AC-1"},
		"acceptance_record_id": {"MEM-REQ-PRESERVE"}, "statement": {"Homework SHALL be visible."},
	}, path)

	rr := postForm(t, handler, path, url.Values{
		"artifact_id": {"REQ-PRESERVE"}, "revision_id": {"REQ-PRESERVE-REV-1"},
		"source_capability_revision_id": {"CAP-REQ-PRESERVE-REV-1"}, "source_acceptance_criterion_key": {"AC-1"},
		"acceptance_record_id": {"MEM-REQ-PRESERVE-DIFFERENT"}, "statement": {"Homework SHALL be visible."},
	})
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body = %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `id="acceptance_record_id"`) || !strings.Contains(body, `value="MEM-REQ-PRESERVE-DIFFERENT"`) {
		t.Errorf("expected the conflicting member identity preserved in the visible C7 form, got %s", body)
	}
	if !strings.Contains(body, "REQ-PRESERVE") || !strings.Contains(body, `role="alert"`) {
		t.Errorf("expected committed requirement state and accessible conflict notice, got %s", body)
	}
}

// TestAssignLifecycleForm_TransitionKeyRequiredForNonEntry proves the API's
// own validation (not a UI-side rule) is what the form surfaces: a
// non-entry assignment missing transition_key is rejected by the command,
// and the UI shows exactly that rejection.
func TestAssignLifecycleForm_TransitionKeyRequiredForNonEntry(t *testing.T) {
	handler := newTestHandler()

	mustPostForm(t, handler, "/projects", url.Values{"project_id": {"PRJ-1"}, "name": {"Pilot"}}, "/projects/PRJ-1")
	mustPostForm(t, handler, "/projects/PRJ-1/features", url.Values{"feature_card_id": {"FC-1"}, "title": {"Homework"}}, "/features/FC-1")
	mustPostForm(t, handler, "/features/FC-1/capability", url.Values{
		"artifact_id": {"CAP-1"}, "revision_id": {"CAP-1-REV-1"}, "title": {"Homework"}, "problem_statement": {"No follow-up."},
	}, "/features/FC-1")

	// from_assignment_id is deliberately supplied so transition_key is the
	// only missing field -- the command validates both through a map whose
	// iteration order is unspecified, so leaving both empty would make the
	// error message (though not the rejection itself) flaky.
	rr := postForm(t, handler, "/features/FC-1/lifecycle", url.Values{
		"assignment_id": {"LC-1"}, "state": {"under-validation"},
		"transition_record_artifact_id": {"TR-1"}, "transition_record_revision_id": {"TR-1-REV-2"},
		"from_assignment_id": {"LC-0"},
	})
	if rr.Code == http.StatusSeeOther {
		t.Fatalf("expected the missing transition_key to be rejected, got a redirect to %q", rr.Header().Get("Location"))
	}
	if !strings.Contains(rr.Body.String(), "transition key") {
		t.Errorf("expected the API's own validation message about transition key, got %s", rr.Body.String())
	}
}
