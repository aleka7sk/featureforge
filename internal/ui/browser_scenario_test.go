package ui_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering/peos"
	"github.com/aleka7sk/featureforge/internal/infrastructure/memory"
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
	t       *testing.T
	handler http.Handler
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
	req := httptest.NewRequest(http.MethodPost, action, strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	b.handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther {
		b.t.Fatalf("POST %s: status = %d, want 303; body = %s", action, rr.Code, rr.Body.String())
	}
	return rr.Header().Get("Location")
}

// runCanonicalScenarioThroughUIBrowser drives the full FF-011 lifecycle by
// reading each page it lands on and acting on what that page actually
// offers -- the same identities and order internal/scenario.Run and
// internal/transport/http's runScenarioThroughHTTP use, this time as a
// person clicking links and filling forms would experience it.
func runCanonicalScenarioThroughUIBrowser(t *testing.T, uow application.UnitOfWork, rec peos.Recorder, clock application.Clock) http.Handler {
	t.Helper()
	api := transporthttp.NewHandler(transporthttp.Dependencies{UOW: uow, Recorder: rec, Projector: rec, Clock: clock})
	handler := ui.NewHandler(ui.Dependencies{API: api})
	b := &browserSession{t: t, handler: handler}

	// 1. Projects screen: create the project.
	home := b.get("/")
	loc := b.submit(formActionAfter(t, home, "Create a project"), url.Values{
		"project_id": {"PRJ-1"}, "name": {"Belcanto Pilot"},
	})

	// 2. Project detail: create the feature card.
	projectPage := b.get(loc)
	if !strings.Contains(projectPage, "Belcanto Pilot") {
		t.Fatalf("expected the project name on its own page, got %s", projectPage)
	}
	loc = b.submit(formActionAfter(t, projectPage, "Create a feature card"), url.Values{
		"feature_card_id": {"FC-1"}, "title": {"Homework after a lesson"}, "description": {""},
	})

	// 3. Feature overview: establish the capability specification.
	overview := b.get(loc)
	loc = b.submit(formActionAfter(t, overview, "Establish the capability specification"), url.Values{
		"artifact_id": {"CAP-1"}, "revision_id": {"CAP-1-REV-1"}, "title": {"Homework after a lesson"},
		"problem_statement":     {"After a lesson ends, a teacher has no way to give the student follow-up work."},
		"user_outcome":          {"A student can see the homework their teacher set."},
		"functional_behaviours": {"A teacher can attach homework to a completed lesson.\nA teacher can publish homework."},
		"constraints":           {"Homework is visible only to the student of that lesson."},
		"acceptance_criteria":   {"AC-1: Published homework is visible to the intended student.\nAC-2: Homework is not visible to any unrelated user."},
	})

	// Still on the overview once redirected back: assign the entry
	// lifecycle state.
	overview = b.get(loc)
	loc = b.submit(formActionAfter(t, overview, "Assign a lifecycle state"), url.Values{
		"assignment_id": {"LC-1"}, "state": {"drafting"}, "is_entry": {"true"},
		"transition_record_artifact_id": {"TR-1"}, "transition_record_revision_id": {"TR-1-REV-1"},
	})
	overview = b.get(loc)

	// 4. Follow the "Revisions" nav link to accept Revision 1.
	revisionsHref := linkHrefFor(t, overview, "Revisions")
	revisions := b.get(revisionsHref)
	loc = b.submit(formActionAfter(t, revisions, "CAP-1-REV-1"), url.Values{
		"record_id": {"ACC-1"}, "state": {"accepted"}, "reason": {""},
	})
	revisions = b.get(loc)

	// 5. Follow "Requirements" from the overview to establish REQ-1 and
	// REQ-2.
	overview = b.get("/features/FC-1")
	requirementsHref := linkHrefFor(t, overview, "Requirements")
	requirements := b.get(requirementsHref)
	loc = b.submit(formActionAfter(t, requirements, "Add a requirement"), url.Values{
		"artifact_id": {"REQ-1"}, "revision_id": {"REQ-1-REV-1"},
		"statement": {"Published homework SHALL be visible to the student of the lesson it belongs to."},
	})
	requirements = b.get(loc)
	loc = b.submit(formActionAfter(t, requirements, "Add a requirement"), url.Values{
		"artifact_id": {"REQ-2"}, "revision_id": {"REQ-2-REV-1"},
		"statement": {"Published homework SHALL NOT be visible to any user who is not the student of that lesson."},
	})
	requirements = b.get(loc)

	// 6. Follow "Decisions" to record DEC-1.
	decisionsHref := linkHrefFor(t, requirements, "Decisions")
	decisions := b.get(decisionsHref)
	loc = b.submit(formActionAfter(t, decisions, "Record a decision"), url.Values{
		"decision_id": {"DEC-1"}, "question": {"Should homework support an optional audio attachment?"},
		"outcome_statement":    {"Homework supports at most one optional audio attachment."},
		"alternatives":         {"Store audio inline.\nStore audio externally."},
		"evidence_artifact_id": {"EV-DEC-1"}, "evidence_revision_id": {"EV-DEC-1-REV-1"},
		"assumptions": {"Audio files are hosted by an existing media service."},
		"constraints": {"No binary storage in the first release."},
		"rationale":   {"Referencing by content address avoids introducing binary storage."},
	})
	decisions = b.get(loc)

	// 7. Back to Revisions to add and accept Revision 2.
	revisionsHref = linkHrefFor(t, decisions, "Revisions")
	revisions = b.get(revisionsHref)
	loc = b.submit(formActionAfter(t, revisions, "Add a revision"), url.Values{
		"revision_id": {"CAP-1-REV-2"}, "title": {"Homework after a lesson"},
		"problem_statement":     {"After a lesson ends, a teacher has no way to give the student follow-up work."},
		"user_outcome":          {"A student can see the homework, including any audio attachment."},
		"functional_behaviours": {"A teacher can attach homework to a completed lesson.\nA teacher may attach one optional audio file."},
		"acceptance_criteria":   {"AC-1: Published homework is visible to the intended student.\nAC-3: An optional audio attachment has a resolvable representation."},
	})
	revisions = b.get(loc)
	loc = b.submit(formActionAfter(t, revisions, "CAP-1-REV-2"), url.Values{
		"record_id": {"ACC-2"}, "state": {"accepted"},
	})
	revisions = b.get(loc)

	// 8. Follow "Validation" to establish the plan, run its one activity
	// twice, and correct the first run's claim.
	validationHref := linkHrefFor(t, revisions, "Validation")
	validation := b.get(validationHref)
	loc = b.submit(formActionAfter(t, validation, "Establish a validation plan"), url.Values{
		"artifact_id": {"VP-1"}, "revision_id": {"VP-1-REV-1"},
		"activities": {"A-1|manual-review|Satisfied when the reviewer confirms student visibility is specified.|REQ-1|REQ-1-REV-1|Reviewer note"},
	})
	validation = b.get(loc)

	loc = b.submit(formActionAfter(t, validation, "Record a validation run"), url.Values{
		"execution_id": {"ER-1"}, "activity_key": {"A-1"}, "method": {"manual-review"}, "outcome": {"completed"},
		"evidence_artifact_id": {"EV-1"}, "evidence_revision_id": {"EV-1-REV-1"}, "evidence_locator": {"https://evidence.example/EV-1"},
	})
	validation = b.get(loc)
	loc = b.submit(formActionAfter(t, validation, "Record a claim"), url.Values{
		"claim_id": {"CLM-1"}, "requirement_artifact_id": {"REQ-1"}, "requirement_revision_id": {"REQ-1-REV-1"},
		"outcome": {"satisfied"}, "method": {"manual-review"},
		"evidence_artifact_id": {"EV-1"}, "evidence_revision_id": {"EV-1-REV-1"}, "execution_id": {"ER-1"},
		"reasoning": {"The specification states student visibility explicitly."},
	})
	validation = b.get(loc)

	loc = b.submit(formActionAfter(t, validation, "Record a validation run"), url.Values{
		"execution_id": {"ER-2"}, "activity_key": {"A-1"}, "method": {"manual-review"}, "outcome": {"completed"},
		"evidence_artifact_id": {"EV-2"}, "evidence_revision_id": {"EV-2-REV-1"}, "evidence_locator": {"https://evidence.example/EV-2"},
	})
	validation = b.get(loc)
	loc = b.submit(formActionAfter(t, validation, "Correct a claim"), url.Values{
		"claim_id": {"CLM-2"}, "correction_target": {"CLM-1"}, "correction_kind": {"correct"},
		"requirement_artifact_id": {"REQ-1"}, "requirement_revision_id": {"REQ-1-REV-1"},
		"outcome": {"not-satisfied"}, "method": {"manual-review"},
		"evidence_artifact_id": {"EV-2"}, "evidence_revision_id": {"EV-2-REV-1"}, "execution_id": {"ER-2"},
		"reasoning": {"The original review missed that the specification does not exclude other users."},
	})
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

	overview := b.get("/features/FC-1")
	if !strings.Contains(overview, "CAP-1-REV-2") {
		t.Errorf("expected the current revision CAP-1-REV-2 on the overview, got %s", overview)
	}
	if !strings.Contains(overview, "status-drafting") {
		t.Errorf("expected the drafting lifecycle state, got %s", overview)
	}

	revisions := b.get("/features/FC-1/revisions")
	if !strings.Contains(revisions, "CAP-1-REV-1") || !strings.Contains(revisions, "CAP-1-REV-2") {
		t.Errorf("expected both revisions independently readable, got %s", revisions)
	}
	if strings.Count(revisions, "status-accepted") < 2 {
		t.Errorf("expected both revisions accepted, got %s", revisions)
	}

	requirements := b.get("/features/FC-1/requirements")
	if !strings.Contains(requirements, "REQ-1") || !strings.Contains(requirements, "REQ-2") {
		t.Errorf("expected both requirements, got %s", requirements)
	}

	decisions := b.get("/features/FC-1/decisions")
	if !strings.Contains(decisions, "DEC-1") || !strings.Contains(decisions, "audio attachment") {
		t.Errorf("expected the decision and its outcome, got %s", decisions)
	}

	validation := b.get("/features/FC-1/validation")
	if !strings.Contains(validation, "claim:CLM-1") {
		t.Errorf("expected the superseded claim CLM-1 shown, not hidden, got %s", validation)
	}
	if !strings.Contains(validation, "corrected by CLM-2") {
		t.Errorf("expected CLM-1 to link to its corrector CLM-2, got %s", validation)
	}
	if !strings.Contains(validation, "status-not-satisfied") {
		t.Errorf("expected REQ-1's current outcome (not-satisfied, from CLM-2), got %s", validation)
	}

	timeline := b.get("/features/FC-1/timeline")
	if !strings.Contains(timeline, "DEC-1") {
		t.Errorf("expected the decision on the unfiltered timeline, got %s", timeline)
	}
}

// TestCanonicalScenarioThroughUIBrowser is FF-021's highest-value test
// (FF-021 §14): the full lifecycle, driven through rendered links and
// forms on a memory-backed store.
func TestCanonicalScenarioThroughUIBrowser(t *testing.T) {
	uow := memory.NewUnitOfWork(memory.NewStore())
	rec := peos.NewRecorder()
	handler := runCanonicalScenarioThroughUIBrowser(t, uow, rec, application.SystemClock{})
	assertCanonicalEndStateThroughUIBrowser(t, handler)
}
