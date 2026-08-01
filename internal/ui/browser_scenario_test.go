package ui_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering"
	"github.com/aleka7sk/featureforge/internal/engineering/peos"
	"github.com/aleka7sk/featureforge/internal/infrastructure/memory"
	"github.com/aleka7sk/featureforge/internal/scenario"
	"github.com/aleka7sk/featureforge/internal/testsupport/replaygate"
	transporthttp "github.com/aleka7sk/featureforge/internal/transport/http"
	"github.com/aleka7sk/featureforge/internal/ui"
)

// This file drives the FF-011 canonical "Homework after a lesson" scenario
// through the UI's rendered pages, not through hand-typed UI routes: every
// form is submitted at the action attribute scraped from the page that
// actually rendered it, and every inter-screen move follows an href
// scraped from a real link -- proof the screens are navigable the way a
// browser experiences them, not just that the handlers exist. No headless
// browser and no new dependency: navigation uses net/http (in-process,
// through the real handler graph) and this file's own small regexp-based
// extraction, matching FF-021 §14's "no new dependency" constraint.

var formActionRe = regexp.MustCompile(`<form[^>]*\baction="([^"]+)"`)

func inputValue(t *testing.T, html, name string) string {
	t.Helper()
	re := regexp.MustCompile(`<input[^>]*\bname="` + regexp.QuoteMeta(name) + `"[^>]*\bvalue="([^"]*)"`)
	match := re.FindStringSubmatch(html)
	if match == nil {
		t.Fatalf("input %q not found in page:\n%s", name, html)
	}
	return match[1]
}

// formActionAfter returns the action of the first <form> appearing after
// marker in html -- marker is the section heading immediately preceding
// the form the caller wants, which is enough to disambiguate a page with
// several forms (e.g. templates/validation.html's four) without a full DOM
// parser.
func formActionAfter(t *testing.T, html, marker string) string {
	t.Helper()
	idx := strings.Index(html, marker)
	if idx < 0 {
		t.Fatalf("marker %q not found in page:\n%s", marker, html)
	}
	m := formActionRe.FindStringSubmatch(html[idx:])
	if m == nil {
		t.Fatalf("no form action found after marker %q", marker)
	}
	return m[1]
}

// linkHrefFor returns the href of the first <a> whose visible text starts
// with text -- how this test finds a navigation target instead of
// constructing the URL itself.
func linkHrefFor(t *testing.T, html, text string) string {
	t.Helper()
	re := regexp.MustCompile(`<a href="([^"]+)"[^>]*>\s*` + regexp.QuoteMeta(text))
	m := re.FindStringSubmatch(html)
	if m == nil {
		t.Fatalf("link %q not found in page:\n%s", text, html)
	}
	return m[1]
}

// browserSession is the smallest thing that behaves like a browser against
// an in-process handler: it can GET a page and submit a form -- both
// exactly as net/http itself would send them -- without following
// redirects automatically, so the test sees and asserts on the same
// Location a real browser's address bar would move to.
type browserSession struct {
	t        *testing.T
	handler  http.Handler
	captured *[]capturedUIResponse
}

type capturedUIResponse struct {
	Action       string
	RequestBody  string
	Status       int
	Location     string
	ResponseBody string
}

func (b *browserSession) get(path string) string {
	b.t.Helper()
	rr := httptest.NewRecorder()
	b.handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
	if rr.Code != http.StatusOK {
		b.t.Fatalf("GET %s: status = %d, want 200; body = %s", path, rr.Code, rr.Body.String())
	}
	return rr.Body.String()
}

// submit POSTs form-encoded values to action and returns the redirect
// target, failing the test on anything but the 303 AD-028 promises after a
// successful command.
func (b *browserSession) submit(action string, values url.Values) string {
	b.t.Helper()
	requestBody := values.Encode()
	req := httptest.NewRequest(http.MethodPost, action, strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	b.handler.ServeHTTP(rr, req)
	if b.captured != nil {
		*b.captured = append(*b.captured, capturedUIResponse{
			Action: action, RequestBody: requestBody, Status: rr.Code,
			Location: rr.Header().Get("Location"), ResponseBody: rr.Body.String(),
		})
	}
	if rr.Code != http.StatusSeeOther {
		b.t.Fatalf("POST %s: status = %d, want 303; body = %s", action, rr.Code, rr.Body.String())
	}
	return rr.Header().Get("Location")
}

// recordDecisionEvidenceForUI records the one canonical act that has no UI or
// HTTP command of its own. Every other Evidence Artifact is created together
// with a validation run; the Decision's EV-0 must still exist so Q5 can follow
// the Decision's authoritative EvidenceKeys projection instead of silently
// dropping that timeline event.
func recordDecisionEvidenceForUI(t *testing.T, uow application.UnitOfWork, rec peos.Recorder, now time.Time) {
	t.Helper()
	ctx := context.Background()
	err := uow.Do(ctx, func(r application.Repositories) error {
		artifact, revision, err := rec.RecordEvidence(engineering.EvidenceInput{
			ArtifactID: scenario.DecisionEvidenceID, RevisionID: scenario.DecisionEvidenceID + "-REV-1",
			Locator: "https://evidence.example/" + scenario.DecisionEvidenceID, RecordedAt: now,
		})
		if err != nil {
			return err
		}
		if err := r.Artifacts.Put(ctx, artifact); err != nil {
			return err
		}
		return r.Revisions.Put(ctx, revision)
	})
	if err != nil {
		t.Fatalf("record decision evidence: %v", err)
	}
}

// runCanonicalScenarioThroughUIBrowser drives the full FF-011 lifecycle by
// reading each page it lands on and acting on what that page actually
// offers -- the same identities and order internal/scenario.Run and
// internal/transport/http's runScenarioThroughHTTP use, this time as a
// person clicking links and filling forms would experience it.
func runCanonicalScenarioThroughUIBrowser(t *testing.T, uow application.UnitOfWork, rec peos.Recorder, clock *application.FixedClock, captures ...*[]capturedUIResponse) http.Handler {
	t.Helper()
	api := transporthttp.NewHandler(transporthttp.Dependencies{UOW: uow, Recorder: rec, Inspector: rec, Projector: rec, Clock: clock})
	handler := ui.NewHandler(ui.Dependencies{API: api})
	b := &browserSession{t: t, handler: handler}
	tick := func() { clock.Advance(time.Hour) }
	if len(captures) > 0 {
		b.captured = captures[0]
	}

	// 1. Projects screen: create the project.
	home := b.get("/")
	loc := b.submit(formActionAfter(t, home, "Create a project"), url.Values{
		"project_id": {scenario.ProjectID}, "name": {"Belcanto Pilot"},
	})
	tick()

	// 2. Project detail: create the feature card.
	projectPage := b.get(loc)
	if !strings.Contains(projectPage, "Belcanto Pilot") {
		t.Fatalf("expected the project name on its own page, got %s", projectPage)
	}
	loc = b.submit(formActionAfter(t, projectPage, "Create a feature card"), url.Values{
		"feature_card_id": {scenario.FeatureCardID}, "title": {"Homework after a lesson"}, "description": {""},
	})
	tick()

	// 3. Feature overview: establish the capability specification.
	overview := b.get(loc)
	loc = b.submit(formActionAfter(t, overview, "Establish the capability specification"), url.Values{
		"artifact_id": {scenario.CapabilityArtifactID}, "revision_id": {scenario.CapabilityRevision1}, "title": {"Homework after a lesson"},
		"problem_statement":     {"After a lesson ends, a teacher has no way to give the student follow-up work."},
		"user_outcome":          {"A student can see the homework their teacher set."},
		"functional_behaviours": {"A teacher can attach homework to a completed lesson.\nA teacher can publish homework."},
		"constraints":           {"Homework is visible only to the student of that lesson."},
		"acceptance_criteria":   {"AC-1: Published homework is visible to the intended student.\nAC-2: Homework is not visible to any unrelated user."},
	})
	tick()

	// 4. Accept Revision 1 before assigning the lifecycle entry.
	overview = b.get(loc)
	revisionsHref := linkHrefFor(t, overview, "Revisions")
	revisions := b.get(revisionsHref)
	loc = b.submit(formActionAfter(t, revisions, scenario.CapabilityRevision1), url.Values{
		"record_id": {"ACC-1"}, "state": {"accepted"}, "reason": {""},
	})
	tick()

	// 5. Assign the drafting entry after the accepted founding revision.
	overview = b.get("/features/" + scenario.FeatureCardID)
	loc = b.submit(formActionAfter(t, overview, "Assign a lifecycle state"), url.Values{
		"assignment_id": {scenario.EntryAssignmentID}, "state": {"drafting"}, "is_entry": {"true"},
		"transition_record_artifact_id": {scenario.TransitionRecordArtifactID},
		"transition_record_revision_id": {scenario.EntryTransitionRevisionID},
	})
	tick()

	// 6. Record the Decision's cited Evidence, then follow "Decisions" to
	// record DEC-1. No standalone Evidence form exists.
	recordDecisionEvidenceForUI(t, uow, rec, clock.Now())
	tick()
	overview = b.get(loc)
	decisionsHref := linkHrefFor(t, overview, "Decisions")
	decisions := b.get(decisionsHref)
	loc = b.submit(formActionAfter(t, decisions, "Record a decision"), url.Values{
		"decision_id": {scenario.DecisionID}, "subject_revision_id": {inputValue(t, decisions, "subject_revision_id")},
		"question":             {"Should homework support an optional audio attachment?"},
		"outcome_statement":    {"Homework supports at most one optional audio attachment."},
		"alternatives":         {"Store audio inline.\nStore audio externally."},
		"evidence_artifact_id": {scenario.DecisionEvidenceID}, "evidence_revision_id": {scenario.DecisionEvidenceID + "-REV-1"},
		"assumptions": {"Audio files are hosted by an existing media service."},
		"constraints": {"No binary storage in the first release."},
		"rationale":   {"Referencing by content address avoids introducing binary storage."},
	})
	tick()
	decisions = b.get(loc)

	// 7. Back to Revisions to add and accept Revision 2.
	revisionsHref = linkHrefFor(t, decisions, "Revisions")
	revisions = b.get(revisionsHref)
	loc = b.submit(formActionAfter(t, revisions, "Add a revision"), url.Values{
		"revision_id": {scenario.CapabilityRevision2}, "title": {"Homework after a lesson"},
		"problem_statement":     {"After a lesson ends, a teacher has no way to give the student follow-up work."},
		"user_outcome":          {"A student can see the homework, including any audio attachment."},
		"functional_behaviours": {"A teacher can attach homework to a completed lesson.\nA teacher may attach one optional audio file."},
		"constraints":           {"Homework is visible only to the student of that lesson.\nPublication completes within 5 seconds of the teacher's action."},
		"acceptance_criteria":   {"AC-1: Published homework is visible to the intended student.\nAC-2: Homework is not visible to any unrelated user.\nAC-3: An optional audio attachment has a resolvable representation.\nAC-4: Publication is observable to the student within 5 seconds."},
	})
	tick()
	revisions = b.get(loc)
	loc = b.submit(formActionAfter(t, revisions, scenario.CapabilityRevision2), url.Values{
		"record_id": {"ACC-2"}, "state": {"accepted"},
	})
	tick()

	// 8. Establish all four Requirements against exact criteria on the
	// accepted current capability revision.
	revisions = b.get(loc)
	requirementsHref := linkHrefFor(t, revisions, "Requirements")
	requirements := b.get(requirementsHref)
	requirementStatements := map[string]string{
		"REQ-1": "Published homework SHALL be visible to the student of the lesson it belongs to.",
		"REQ-2": "Published homework SHALL NOT be visible to any user who is not the student of that lesson.",
		"REQ-3": "Where homework has an audio attachment, that attachment SHALL have a representation the student can resolve.",
		"REQ-4": "Published homework SHALL become observable to the student within 5 seconds of publication.",
	}
	requirementCriteria := map[string]string{"REQ-1": "AC-1", "REQ-2": "AC-2", "REQ-3": "AC-3", "REQ-4": "AC-4"}
	for _, requirementID := range scenario.RequirementArtifactIDs {
		loc = b.submit(formActionAfter(t, requirements, "Add a requirement"), url.Values{
			"artifact_id": {requirementID}, "revision_id": {requirementID + "-REV-1"},
			"acceptance_record_id":            {"ACC-" + requirementID},
			"source_capability_revision_id":   {scenario.CapabilityRevision2},
			"source_acceptance_criterion_key": {requirementCriteria[requirementID]},
			"statement":                       {requirementStatements[requirementID]},
		})
		tick()
		requirements = b.get(loc)
	}

	// 9. The traced Requirements support the drafting -> specified milestone.
	overviewHref := linkHrefFor(t, requirements, "Overview")
	overview = b.get(overviewHref)
	loc = b.submit(formActionAfter(t, overview, "Assign a lifecycle state"), url.Values{
		"assignment_id": {scenario.SpecifiedAssignmentID}, "state": {"specified"},
		"transition_record_artifact_id": {scenario.TransitionRecordArtifactID},
		"transition_record_revision_id": {scenario.SpecifyTransitionRevisionID},
		"transition_key":                {"specify"}, "from_assignment_id": {scenario.EntryAssignmentID},
	})
	tick()

	// 10. Establish the validation plan after specified.
	overview = b.get(loc)
	validationHref := linkHrefFor(t, overview, "Validation")
	validation := b.get(validationHref)
	loc = b.submit(formActionAfter(t, validation, "Establish a validation plan"), url.Values{
		"artifact_id": {scenario.PlanArtifactID}, "revision_id": {scenario.PlanRevisionID},
		"acceptance_record_id": {"ACC-VP-1"},
		"activities": {"A-1|manual-review|Satisfied when the reviewer confirms student visibility is specified.|REQ-1|REQ-1-REV-1|Reviewer note\n" +
			"A-2|manual-review|Satisfied when the reviewer confirms non-student access is excluded.|REQ-2|REQ-2-REV-1|Reviewer note\n" +
			"A-3|manual-inspection|Satisfied when the inspector confirms the attachment representation is resolvable.|REQ-3|REQ-3-REV-1|Inspection note"},
	})
	tick()
	validation = b.get(loc)

	// 11. Complete the first execution and evidence before entering
	// under-validation; record its claim only after the transition.
	loc = b.submit(formActionAfter(t, validation, "Record a validation run"), url.Values{
		"execution_id": {"ER-1"}, "activity_key": {"A-1"}, "method": {"manual-review"}, "outcome": {"completed"},
		"evidence_artifact_id": {"EV-1"}, "evidence_revision_id": {"EV-1-REV-1"}, "evidence_locator": {"https://evidence.example/EV-1"},
	})
	tick()
	validation = b.get(loc)
	overviewHref = linkHrefFor(t, validation, "Overview")
	overview = b.get(overviewHref)
	loc = b.submit(formActionAfter(t, overview, "Assign a lifecycle state"), url.Values{
		"assignment_id": {scenario.UnderValidationAssignmentID}, "state": {"under-validation"},
		"transition_record_artifact_id": {scenario.TransitionRecordArtifactID},
		"transition_record_revision_id": {scenario.BeginValidationTransitionRevisionID},
		"transition_key":                {"begin-validation"}, "from_assignment_id": {scenario.SpecifiedAssignmentID},
	})
	tick()
	overview = b.get(loc)
	validationHref = linkHrefFor(t, overview, "Validation")
	validation = b.get(validationHref)
	loc = b.submit(formActionAfter(t, validation, "Record a claim"), url.Values{
		"claim_id": {scenario.ClaimForR1}, "requirement_artifact_id": {"REQ-1"}, "requirement_revision_id": {"REQ-1-REV-1"},
		"outcome": {"satisfied"}, "method": {"manual-review"},
		"evidence_artifact_id": {"EV-1"}, "evidence_revision_id": {"EV-1-REV-1"}, "execution_id": {"ER-1"},
		"reasoning": {"The specification states student visibility explicitly."},
	})
	tick()
	validation = b.get(loc)

	// 12. Record the remaining validation acts, then correct CLM-2.
	loc = b.submit(formActionAfter(t, validation, "Record a validation run"), url.Values{
		"execution_id": {"ER-2"}, "activity_key": {"A-2"}, "method": {"manual-review"}, "outcome": {"completed"},
		"evidence_artifact_id": {"EV-2"}, "evidence_revision_id": {"EV-2-REV-1"}, "evidence_locator": {"https://evidence.example/EV-2"},
	})
	tick()
	validation = b.get(loc)
	loc = b.submit(formActionAfter(t, validation, "Record a claim"), url.Values{
		"claim_id": {scenario.ClaimIncorrect}, "requirement_artifact_id": {"REQ-2"}, "requirement_revision_id": {"REQ-2-REV-1"},
		"outcome": {"satisfied"}, "method": {"manual-review"},
		"evidence_artifact_id": {"EV-2"}, "evidence_revision_id": {"EV-2-REV-1"}, "execution_id": {"ER-2"},
		"reasoning": {"The specification states who may view homework."},
	})
	tick()
	validation = b.get(loc)
	loc = b.submit(formActionAfter(t, validation, "Record a validation run"), url.Values{
		"execution_id": {"ER-3"}, "activity_key": {"A-3"}, "method": {"manual-inspection"}, "outcome": {"completed"},
		"evidence_artifact_id": {"EV-3"}, "evidence_revision_id": {"EV-3-REV-1"}, "evidence_locator": {"https://evidence.example/EV-3"},
	})
	tick()
	validation = b.get(loc)
	loc = b.submit(formActionAfter(t, validation, "Record a claim"), url.Values{
		"claim_id": {scenario.ClaimForR3}, "requirement_artifact_id": {"REQ-3"}, "requirement_revision_id": {"REQ-3-REV-1"},
		"outcome": {"satisfied"}, "method": {"manual-inspection"},
		"evidence_artifact_id": {"EV-3"}, "evidence_revision_id": {"EV-3-REV-1"}, "execution_id": {"ER-3"},
		"reasoning": {"The specification names a resolvable representation for the attachment."},
	})
	tick()
	validation = b.get(loc)
	loc = b.submit(formActionAfter(t, validation, "Record a validation run"), url.Values{
		"execution_id": {"ER-4"}, "activity_key": {"A-2"}, "method": {"manual-review"}, "outcome": {"completed"},
		"evidence_artifact_id": {"EV-4"}, "evidence_revision_id": {"EV-4-REV-1"}, "evidence_locator": {"https://evidence.example/EV-4"},
	})
	tick()
	validation = b.get(loc)
	loc = b.submit(formActionAfter(t, validation, "Correct a claim"), url.Values{
		"claim_id": {scenario.ClaimCorrecting}, "correction_target": {scenario.ClaimIncorrect}, "correction_kind": {"correct"},
		"requirement_artifact_id": {"REQ-2"}, "requirement_revision_id": {"REQ-2-REV-1"},
		"outcome": {"not-satisfied"}, "method": {"manual-review"},
		"evidence_artifact_id": {"EV-4"}, "evidence_revision_id": {"EV-4-REV-1"}, "execution_id": {"ER-4"},
		"reasoning": {"The original review treated a positive visibility statement as excluding other users; it does not."},
	})
	tick()
	b.get(loc)

	return handler
}

// assertCanonicalEndStateThroughUIBrowser reads back every screen through
// GET and checks the facts FF-011 §9 fixes, mirroring
// internal/transport/http's assertCanonicalEndStateThroughHTTP but read
// from rendered HTML rather than a JSON body.
func assertCanonicalEndStateThroughUIBrowser(t *testing.T, handler http.Handler) {
	t.Helper()
	b := &browserSession{t: t, handler: handler}

	overview := b.get("/features/" + scenario.FeatureCardID)
	if !strings.Contains(overview, scenario.CapabilityRevision2) {
		t.Errorf("expected the current revision CAP-1-REV-2 on the overview, got %s", overview)
	}
	if !strings.Contains(overview, "status-under-validation") {
		t.Errorf("expected the under-validation lifecycle state, got %s", overview)
	}

	revisions := b.get("/features/" + scenario.FeatureCardID + "/revisions")
	if !strings.Contains(revisions, scenario.CapabilityRevision1) || !strings.Contains(revisions, scenario.CapabilityRevision2) {
		t.Errorf("expected both revisions independently readable, got %s", revisions)
	}
	if strings.Count(revisions, "status-accepted") < 2 {
		t.Errorf("expected both revisions accepted, got %s", revisions)
	}

	requirements := b.get("/features/" + scenario.FeatureCardID + "/requirements")
	for _, requirementID := range scenario.RequirementArtifactIDs {
		if !strings.Contains(requirements, requirementID) {
			t.Errorf("expected requirement %s, got %s", requirementID, requirements)
		}
	}

	decisions := b.get("/features/" + scenario.FeatureCardID + "/decisions")
	if !strings.Contains(decisions, scenario.DecisionID) || !strings.Contains(decisions, "audio attachment") {
		t.Errorf("expected the decision and its outcome, got %s", decisions)
	}

	validation := b.get("/features/" + scenario.FeatureCardID + "/validation")
	if !strings.Contains(validation, "Current claim:</strong> "+scenario.ClaimForR1) || !strings.Contains(validation, "Current claim:</strong> "+scenario.ClaimForR3) {
		t.Errorf("expected the satisfied REQ-1 and REQ-3 claims, got %s", validation)
	}
	if !strings.Contains(validation, "claim:"+scenario.ClaimIncorrect) {
		t.Errorf("expected the superseded claim CLM-2 shown, not hidden, got %s", validation)
	}
	if !strings.Contains(validation, "corrected by "+scenario.ClaimCorrecting) {
		t.Errorf("expected CLM-2 to link to its corrector CLM-4, got %s", validation)
	}
	if !strings.Contains(validation, "status-not-satisfied") {
		t.Errorf("expected REQ-1's current outcome (not-satisfied, from CLM-2), got %s", validation)
	}

	timeline := b.get("/features/" + scenario.FeatureCardID + "/timeline")
	if !strings.Contains(timeline, "Decision recorded") || !strings.Contains(timeline, "evidence:"+scenario.DecisionEvidenceID+"/"+scenario.DecisionEvidenceID+"-REV-1") {
		t.Errorf("expected the decision and its evidence reference on the unfiltered timeline, got %s", timeline)
	}
	if count := strings.Count(timeline, `<li class="timeline-item">`); count != 29 {
		t.Errorf("timeline renders %d events, want 29", count)
	}
}

// TestCanonicalScenarioThroughUIBrowser is FF-021's highest-value test
// (FF-021 §14): the full lifecycle, driven through rendered links and
// forms on a memory-backed store.
func TestCanonicalScenarioThroughUIBrowser(t *testing.T) {
	gate := replaygate.New(memory.NewUnitOfWork(memory.NewStore()))
	rec := peos.NewRecorder()
	clock := application.NewFixedClock(scenario.FixedStart)
	assertCanonicalScenarioUIReplay(t, gate, rec, clock)
}

func assertCanonicalScenarioUIReplay(t *testing.T, gate *replaygate.Gate, rec peos.Recorder, clock *application.FixedClock) {
	t.Helper()
	if err := application.EnsureLifecycleConfiguration(context.Background(), gate, rec, rec); err != nil {
		t.Fatalf("initializing lifecycle configuration: %v", err)
	}
	var first []capturedUIResponse
	handler := runCanonicalScenarioThroughUIBrowser(t, gate, rec, clock, &first)
	assertCanonicalEndStateThroughUIBrowser(t, handler)

	clock.Advance(24 * time.Hour)
	gate.RejectWrites()
	replay := replayCapturedUISubmissions(t, handler, first)
	if gate.WriteAttempts() != 0 {
		t.Fatalf("UI scenario replay attempted %d repository mutations", gate.WriteAttempts())
	}
	if !reflect.DeepEqual(replay, first) {
		limit := len(first)
		if len(replay) < limit {
			limit = len(replay)
		}
		for i := 0; i < limit; i++ {
			if replay[i] != first[i] {
				t.Fatalf("UI replay response %d differs:\nfirst=%+v\nreplay=%+v", i, first[i], replay[i])
			}
		}
		t.Fatalf("UI replay response count = %d, want %d", len(replay), len(first))
	}
	if len(first) != 23 {
		t.Fatalf("canonical UI trace contains %d form submissions, want 23", len(first))
	}
	for i, response := range replay {
		if response.Status != http.StatusSeeOther || response.Location == "" {
			t.Fatalf("UI replay response %d (%s) = status %d, Location %q; want 303 with redirect", i, response.Action, response.Status, response.Location)
		}
	}
	assertCanonicalEndStateThroughUIBrowser(t, handler)
}

// replayCapturedUISubmissions repeats the exact form submissions discovered
// from rendered pages during the first browser journey. Create forms are
// intentionally hidden once their aggregate exists, so replay must use the
// already-observed action and encoded form body rather than pretending those
// forms remain visible in completed-state pages.
func replayCapturedUISubmissions(t *testing.T, handler http.Handler, first []capturedUIResponse) []capturedUIResponse {
	t.Helper()
	replay := make([]capturedUIResponse, 0, len(first))
	for _, request := range first {
		req := httptest.NewRequest(http.MethodPost, request.Action, strings.NewReader(request.RequestBody))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		replay = append(replay, capturedUIResponse{
			Action: request.Action, RequestBody: request.RequestBody, Status: rr.Code,
			Location: rr.Header().Get("Location"), ResponseBody: rr.Body.String(),
		})
	}
	return replay
}
