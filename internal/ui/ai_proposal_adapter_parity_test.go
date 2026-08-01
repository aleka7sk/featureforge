package ui_test

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering"
	"github.com/aleka7sk/featureforge/internal/engineering/peos"
	"github.com/aleka7sk/featureforge/internal/infrastructure/memory"
	"github.com/aleka7sk/featureforge/internal/proposal"
	"github.com/aleka7sk/featureforge/internal/scenario"
	"github.com/aleka7sk/featureforge/internal/testsupport/replaygate"
	transporthttp "github.com/aleka7sk/featureforge/internal/transport/http"
	"github.com/aleka7sk/featureforge/internal/ui"
)

const aiProposalTargetRevision = "CAP-1-REV-3"

var canonicalAIProposalSources = []string{
	"criterion:CAP-1/CAP-1-REV-2#AC-1",
	"criterion:CAP-1/CAP-1-REV-2#AC-2",
	"criterion:CAP-1/CAP-1-REV-2#AC-3",
	"criterion:CAP-1/CAP-1-REV-2#AC-4",
	"record:claim/CLM-1",
	"record:claim/CLM-3",
	"record:claim/CLM-4",
	"record:decision/DEC-1",
	"record:execution/ER-1",
	"record:execution/ER-3",
	"record:execution/ER-4",
	"requirement-trace:REQ-1/REQ-1-REV-1",
	"requirement-trace:REQ-2/REQ-2-REV-1",
	"requirement-trace:REQ-3/REQ-3-REV-1",
	"requirement-trace:REQ-4/REQ-4-REV-1",
	"revision:CAP-1/CAP-1-REV-1",
	"revision:CAP-1/CAP-1-REV-2",
	"revision:EV-1/EV-1-REV-1",
	"revision:EV-3/EV-3-REV-1",
	"revision:EV-4/EV-4-REV-1",
	"revision:REQ-1/REQ-1-REV-1",
	"revision:REQ-2/REQ-2-REV-1",
	"revision:REQ-3/REQ-3-REV-1",
	"revision:REQ-4/REQ-4-REV-1",
}

func postUIForm(t *testing.T, handler http.Handler, path string, values url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func proposalSourceListAfter(t *testing.T, page, heading string) []string {
	t.Helper()
	marker := "<h3>" + heading + "</h3>"
	start := strings.Index(page, marker)
	if start < 0 {
		t.Fatalf("proposal review has no %q source heading", heading)
	}
	list := page[start+len(marker):]
	open := strings.Index(list, "<ul>")
	close := strings.Index(list, "</ul>")
	if open < 0 || close < 0 || close < open {
		t.Fatalf("proposal review has no complete source list after %q", heading)
	}
	rows := regexp.MustCompile(`<li><code>([^<]+)</code></li>`).FindAllStringSubmatch(list[open:close], -1)
	values := make([]string, len(rows))
	for index, row := range rows {
		values[index] = html.UnescapeString(row[1])
	}
	return values
}

func proposalSection(t *testing.T, page, headingID string) string {
	t.Helper()
	start := strings.Index(page, `id="`+headingID+`"`)
	if start < 0 {
		t.Fatalf("proposal review has no section heading %q", headingID)
	}
	section := page[start:]
	if end := strings.Index(section, "</section>"); end >= 0 {
		return section[:end]
	}
	return section
}

type proposalTargetState struct {
	revisionFound  bool
	contentFound   bool
	orderFound     bool
	orderSequence  int
	traceFound     bool
	acceptanceRows int
}

func readProposalTargetState(t *testing.T, uow application.UnitOfWork) proposalTargetState {
	t.Helper()
	ctx := context.Background()
	key := engineering.RevisionKey{ArtifactID: scenario.CapabilityArtifactID, RevisionID: aiProposalTargetRevision}
	var state proposalTargetState
	err := uow.Do(ctx, func(repos application.Repositories) error {
		var err error
		_, state.revisionFound, err = repos.Revisions.Get(ctx, key)
		if err != nil {
			return fmt.Errorf("get target revision: %w", err)
		}
		_, state.contentFound, err = repos.StructuredContent.Get(ctx, key)
		if err != nil {
			return fmt.Errorf("get target content: %w", err)
		}
		order, found, err := repos.RevisionOrder.Get(ctx, key)
		if err != nil {
			return fmt.Errorf("get target order: %w", err)
		}
		state.orderFound = found
		state.orderSequence = order.Sequence
		_, state.traceFound, err = repos.RequirementTraces.Get(ctx, key)
		if err != nil {
			return fmt.Errorf("get target trace: %w", err)
		}
		journal, err := repos.RevisionAcceptance.ListByRevision(ctx, key)
		if err != nil {
			return fmt.Errorf("list target acceptance: %w", err)
		}
		state.acceptanceRows = len(journal)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func assertAIProposalUIAdapterParity(t *testing.T, base application.UnitOfWork, recorder peos.Recorder) {
	t.Helper()
	gate := replaygate.New(base)
	clock := application.NewFixedClock(scenario.FixedStart)
	if err := application.EnsureLifecycleConfiguration(context.Background(), gate, recorder, recorder); err != nil {
		t.Fatalf("initialize lifecycle configuration: %v", err)
	}
	// Seed the exact FF-011 scenario through the real rendered browser flow.
	runCanonicalScenarioThroughUIBrowser(t, gate, recorder, clock)

	api := transporthttp.NewHandler(transporthttp.Dependencies{
		UOW: gate, Recorder: recorder, Inspector: recorder, Projector: recorder,
		Generator: proposal.NewDeterministicGenerator(), Clock: clock,
	})
	handler := ui.NewHandler(ui.Dependencies{API: api})
	browser := &browserSession{t: t, handler: handler}

	revisionsPath := "/features/" + scenario.FeatureCardID + "/revisions"
	revisions := browser.get(revisionsPath)
	generateAction := formActionAfter(t, revisions, "AI-assisted proposal")
	writeCountBeforeReview := gate.WriteAttempts()

	first := postUIForm(t, handler, generateAction, nil)
	if first.Code != http.StatusOK {
		t.Fatalf("generate review status = %d, want 200; body = %s", first.Code, first.Body.String())
	}
	if got := first.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("generate Cache-Control = %q, want no-store", got)
	}
	page := first.Body.String()
	if strings.Contains(strings.ToLower(page), "<script") {
		t.Fatal("real-stack proposal review requires JavaScript")
	}
	for _, requirementID := range scenario.RequirementArtifactIDs {
		if !strings.Contains(page, requirementID+"/"+requirementID+"-REV-1") {
			t.Errorf("review page omits effective Requirement %s", requirementID)
		}
	}
	uncovered := proposalSection(t, page, "proposal-uncovered-heading")
	if !strings.Contains(uncovered, "AC-4") || !strings.Contains(uncovered, "missing_current_claim") || !strings.Contains(uncovered, "REQ-4/REQ-4-REV-1") {
		t.Errorf("review page does not show the canonical AC-4 uncovered result: %s", uncovered)
	}
	finding := proposalSection(t, page, "proposal-findings-heading")
	if !strings.Contains(finding, "AC-2") || !strings.Contains(finding, "CLM-4") || !strings.Contains(finding, "not-satisfied") {
		t.Errorf("review page does not show the corrected AC-2 finding: %s", finding)
	}
	if got := proposalSourceListAfter(t, page, "Context sources"); !slices.Equal(got, canonicalAIProposalSources) {
		t.Fatalf("context sources =\n%v\nwant exact canonical set\n%v", got, canonicalAIProposalSources)
	}
	if got := proposalSourceListAfter(t, page, "Proposal sources"); !slices.Equal(got, canonicalAIProposalSources) {
		t.Fatalf("proposal sources =\n%v\nwant exact canonical set\n%v", got, canonicalAIProposalSources)
	}

	canonicalProposal := html.UnescapeString(inputValue(t, page, "proposal"))
	parsed, err := proposal.ParseProposal([]byte(canonicalProposal))
	if err != nil {
		t.Fatalf("hidden reviewed Proposal is not canonical: %v", err)
	}
	parsedSources := make([]string, len(parsed.Sources()))
	for index, source := range parsed.Sources() {
		parsedSources[index] = source.String()
	}
	if !slices.Equal(parsedSources, canonicalAIProposalSources) {
		t.Fatalf("hidden Proposal sources =\n%v\nwant\n%v", parsedSources, canonicalAIProposalSources)
	}

	discardLink := regexp.MustCompile(`<a[^>]*href="([^"]+)"[^>]*>\s*Discard proposal`).FindStringSubmatch(page)
	if discardLink == nil {
		t.Fatal("proposal review has no navigation-only discard link")
	}
	discarded := browser.get(discardLink[1])
	if !strings.Contains(discarded, `action="`+generateAction+`"`) {
		t.Fatal("discard did not return to the revisions screen with proposal generation available")
	}
	if got := readProposalTargetState(t, gate); got != (proposalTargetState{}) {
		t.Fatalf("discard or generation persisted target state: %+v", got)
	}
	if got := gate.WriteAttempts(); got != writeCountBeforeReview {
		t.Fatalf("generate/discard attempted %d engineering writes", got-writeCountBeforeReview)
	}

	second := postUIForm(t, handler, generateAction, nil)
	if second.Code != http.StatusOK || second.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("second generate = %d Cache-Control %q; want 200/no-store", second.Code, second.Header().Get("Cache-Control"))
	}
	if !bytes.Equal(first.Body.Bytes(), second.Body.Bytes()) {
		t.Fatal("equal persisted context did not regenerate a byte-identical review page")
	}
	secondProposal := html.UnescapeString(inputValue(t, second.Body.String(), "proposal"))
	if secondProposal != canonicalProposal {
		t.Fatal("equal persisted context did not regenerate byte-identical canonical Proposal JSON")
	}
	if got := readProposalTargetState(t, gate); got != (proposalTargetState{}) {
		t.Fatalf("second generation persisted target state: %+v", got)
	}

	acceptAction := formActionAfter(t, second.Body.String(), "Accept as a draft revision")
	acceptValues := url.Values{"revision_id": {aiProposalTargetRevision}, "proposal": {secondProposal}}
	accepted := postUIForm(t, handler, acceptAction, acceptValues)
	if accepted.Code != http.StatusSeeOther || accepted.Header().Get("Location") != revisionsPath {
		t.Fatalf("accept = status %d Location %q, want 303 %q; body = %s", accepted.Code, accepted.Header().Get("Location"), revisionsPath, accepted.Body.String())
	}
	if got := accepted.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("accept Cache-Control = %q, want no-store", got)
	}
	state := readProposalTargetState(t, gate)
	if !state.revisionFound || !state.contentFound || !state.orderFound || state.orderSequence != 3 || state.traceFound || state.acceptanceRows != 0 {
		t.Fatalf("accepted proposal target state = %+v, want complete sequence-3 draft with no trace or acceptance row", state)
	}

	history := browser.get(revisionsPath)
	if !strings.Contains(history, "<strong>"+scenario.CapabilityRevision2+"</strong> is current.") {
		t.Fatalf("accepting a draft changed the current accepted revision: %s", history)
	}
	target := proposalSection(t, history, "rev-"+aiProposalTargetRevision)
	for _, witness := range []string{aiProposalTargetRevision, "status-draft", "featureforge:local-user", "featureforge:ai-assisted"} {
		if !strings.Contains(target, witness) {
			t.Errorf("draft revision panel omits %q: %s", witness, target)
		}
	}

	// Occupied replay is recognized before freshness and performs no writes,
	// even after the candidate clock has changed.
	clock.Advance(24 * time.Hour)
	gate.RejectWrites()
	replayed := postUIForm(t, handler, acceptAction, acceptValues)
	if replayed.Code != http.StatusSeeOther || replayed.Header().Get("Location") != revisionsPath {
		t.Fatalf("exact accept replay = status %d Location %q, want 303 %q; body = %s", replayed.Code, replayed.Header().Get("Location"), revisionsPath, replayed.Body.String())
	}
	if gate.WriteAttempts() != 0 {
		t.Fatalf("exact UI proposal replay attempted %d repository writes", gate.WriteAttempts())
	}
}

func TestAIProposalCanonicalUIAdapterParityMemory(t *testing.T) {
	assertAIProposalUIAdapterParity(t, memory.NewUnitOfWork(memory.NewStore()), peos.NewRecorder())
}

func TestAIProposalCanonicalUIAdapterParityPostgres(t *testing.T) {
	uow, recorder := newPostgresFixtureUI(t)
	assertAIProposalUIAdapterParity(t, uow, recorder)
}
