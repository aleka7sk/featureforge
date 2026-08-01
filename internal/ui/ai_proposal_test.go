package ui

import (
	"encoding/json"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

const aiProposalFixture = `{"content":{"schema_version":1,"title":"Proposed <audio> & review","problem_statement":"P > Q","user_outcome":"Reviewed output","functional_behaviours":["Upload audio"],"constraints":["No hidden writes"],"acceptance_criteria":[{"key":"AC-1","text":"Audio is stored"}],"dependencies":[],"open_questions":["Retention?"]},"rationale":"Use <exact> & persisted sources.","sources":["record:decision/DEC-1","revision:CAP-1/CAP-1-REV-1","revision:CAP-1/CAP-1-REV-2"],"context_digest":"context-digest","proposal_digest":"proposal-digest"}`

const aiContextFixture = `{"capability":{"revision":{"artifact_id":"CAP-1","revision_id":"CAP-1-REV-2"},"sequence":2,"content":{"schema_version":1,"title":"Current audio","problem_statement":"Current problem","user_outcome":"Current outcome","functional_behaviours":["Upload audio"],"constraints":[],"acceptance_criteria":[{"key":"AC-1","text":"Audio is stored"}],"dependencies":[],"open_questions":["Retention?"]},"content_digest":"content-digest"},"requirements":[{"revision":{"artifact_id":"REQ-1","revision_id":"REQ-1-REV-1"},"sequence":1,"statement":"Store audio exactly.","source_capability_revision":{"artifact_id":"CAP-1","revision_id":"CAP-1-REV-2"},"source_criterion_key":"AC-1"}],"claims":[{"record":{"kind":"claim","record_id":"CLM-1"},"requirement_revision":{"artifact_id":"REQ-1","revision_id":"REQ-1-REV-1"},"capability_revision":{"artifact_id":"CAP-1","revision_id":"CAP-1-REV-2"},"scope_artifact_id":"CAP-1","criterion_keys":["requirement-revision:REQ-1/REQ-1-REV-1"],"outcome":"not-satisfied","reasoning":"Digest mismatch.","execution_references":[{"kind":"execution","record_id":"EXEC-1"}],"evidence_references":[{"artifact_id":"EVID-1","revision_id":"EVID-1-REV-1"}]}],"decisions":[{"decision_id":"DEC-1","subject":{"kind":"artifact-revision","artifact_id":"CAP-1","revision_id":"CAP-1-REV-1"},"outcome_statement":"Use content addresses."}],"open_questions":[{"capability_revision":{"artifact_id":"CAP-1","revision_id":"CAP-1-REV-2"},"ordinal":1,"text":"Retention?"}],"uncovered_criteria":[{"capability_revision":{"artifact_id":"CAP-1","revision_id":"CAP-1-REV-2"},"criterion_key":"AC-4","criterion_text":"Notify learner.","reason":"missing_current_claim","requirement_revisions":[{"artifact_id":"REQ-4","revision_id":"REQ-4-REV-1"}]}],"findings":[{"capability_revision":{"artifact_id":"CAP-1","revision_id":"CAP-1-REV-2"},"criterion_key":"AC-1","requirement_revision":{"artifact_id":"REQ-1","revision_id":"REQ-1-REV-1"},"claim_record":{"kind":"claim","record_id":"CLM-1"},"outcome":"not-satisfied","reasoning":"Digest mismatch."}],"sources":["record:claim/CLM-1","record:decision/DEC-1","revision:CAP-1/CAP-1-REV-1","revision:CAP-1/CAP-1-REV-2"],"context_digest":"context-digest"}`

type aiAPICall struct {
	method      string
	path        string
	contentType string
	body        []byte
}

type aiProposalAPIFake struct {
	calls    []aiAPICall
	accepted bool
}

func (f *aiProposalAPIFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.calls = append(f.calls, aiAPICall{method: r.Method, path: r.URL.Path, contentType: r.Header.Get("Content-Type"), body: body})
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/features/FC-1":
		_, _ = io.WriteString(w, `{"data":{"feature":{"feature_card_id":"FC-1","capability_artifact_id":"CAP-1"},"state":{"current_revision":{"found":true,"revision":{"artifact_id":"CAP-1","revision_id":"CAP-1-REV-2"}}}}}`)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/capabilities/CAP-1/revisions":
		if f.accepted {
			_, _ = io.WriteString(w, `{"data":{"revisions":[{"artifact_id":"CAP-1","revision_id":"CAP-1-REV-3","sequence":3,"revision_family":"capability","integrity_value":"sha256:test","recorded_at":"2026-08-01T12:00:00Z","provenance_actor":"featureforge:local-user","provenance_method":"featureforge:ai-assisted","content":{"schema_version":1,"title":"Proposed <audio> & review","problem_statement":"P > Q","user_outcome":"Reviewed output","functional_behaviours":[],"constraints":[],"acceptance_criteria":[],"dependencies":[],"open_questions":[]}}],"current":{"found":false}},"rationale":{"rule":"draft by absence","considered":[{"artifact_id":"CAP-1","revision_id":"CAP-1-REV-3","sequence":3,"acceptance_state":"draft"}],"rejected":[],"warnings":[]}}`)
			break
		}
		_, _ = io.WriteString(w, `{"data":{"revisions":[],"current":{"found":false}},"rationale":{"rule":"","considered":[],"rejected":[],"warnings":[]}}`)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/capabilities/CAP-1/ai-proposals":
		_, _ = io.WriteString(w, `{"data":{"context_pack":`+aiContextFixture+`,"proposal":`+aiProposalFixture+`}}`)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/capabilities/CAP-1/ai-proposals/accept":
		f.accepted = true
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"data":{"artifact_id":"CAP-1","revision_id":"CAP-1-REV-3","sequence":3}}`)
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":{"code":"not_found","message":"not found"}}`)
	}
}

func TestAIProposalNoJavaScriptGenerateDiscardAccept(t *testing.T) {
	fake := &aiProposalAPIFake{}
	handler := NewHandler(Dependencies{API: fake})

	generateRequest := httptest.NewRequest(http.MethodPost, "/features/FC-1/ai-proposals", strings.NewReader(""))
	generateRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	generated := httptest.NewRecorder()
	handler.ServeHTTP(generated, generateRequest)
	if generated.Code != http.StatusOK {
		t.Fatalf("generate status = %d, want 200; body = %s", generated.Code, generated.Body.String())
	}
	if got := generated.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("generate Cache-Control = %q, want no-store", got)
	}
	page := generated.Body.String()
	for _, witness := range []string{
		"Current capability content", "Proposed capability content", "REQ-1/REQ-1-REV-1", "CLM-1",
		"DEC-1", "Retention?", "AC-4", "missing_current_claim", "not-satisfied",
		"record:decision/DEC-1", "Use &lt;exact&gt; &amp; persisted sources.", "context-digest", "proposal-digest",
	} {
		if !strings.Contains(page, witness) {
			t.Errorf("review page does not contain %q", witness)
		}
	}
	if strings.Contains(strings.ToLower(page), "<script") {
		t.Fatal("review page unexpectedly requires JavaScript")
	}
	if !strings.Contains(page, `action="/features/FC-1/ai-proposals/accept"`) {
		t.Fatal("review page does not contain the exact accept form action")
	}
	if !strings.Contains(page, `href="/features/FC-1/revisions">Discard proposal</a>`) {
		t.Fatal("review page does not contain the navigation-only discard link")
	}
	if strings.Contains(page, "/reject") {
		t.Fatal("review page exposes a reject route")
	}
	if len(fake.calls) != 2 || fake.calls[0].method != http.MethodGet || fake.calls[1].method != http.MethodPost ||
		fake.calls[1].path != "/api/v1/capabilities/CAP-1/ai-proposals" || len(fake.calls[1].body) != 0 {
		t.Fatalf("generate delegation calls = %+v", fake.calls)
	}

	hiddenPattern := regexp.MustCompile(`<input type="hidden" name="proposal" value="([^"]*)">`)
	hidden := hiddenPattern.FindStringSubmatch(page)
	if hidden == nil {
		t.Fatal("review page has no canonical Proposal hidden field")
	}
	canonicalProposal := html.UnescapeString(hidden[1])
	if canonicalProposal != aiProposalFixture {
		t.Fatalf("hidden Proposal changed\n got: %s\nwant: %s", canonicalProposal, aiProposalFixture)
	}

	beforeDiscard := len(fake.calls)
	discarded := httptest.NewRecorder()
	handler.ServeHTTP(discarded, httptest.NewRequest(http.MethodGet, "/features/FC-1/revisions", nil))
	if discarded.Code != http.StatusOK {
		t.Fatalf("discard navigation status = %d, want 200; body = %s", discarded.Code, discarded.Body.String())
	}
	for _, call := range fake.calls[beforeDiscard:] {
		if call.method != http.MethodGet {
			t.Fatalf("discard caused non-read API call: %+v", call)
		}
	}
	if !strings.Contains(discarded.Body.String(), `action="/features/FC-1/ai-proposals"`) {
		t.Fatal("revisions page has no proposal generation form")
	}

	secondGenerateRequest := httptest.NewRequest(http.MethodPost, "/features/FC-1/ai-proposals", strings.NewReader(""))
	secondGenerateRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	secondGenerated := httptest.NewRecorder()
	handler.ServeHTTP(secondGenerated, secondGenerateRequest)
	if secondGenerated.Code != http.StatusOK {
		t.Fatalf("second generate status = %d, want 200; body = %s", secondGenerated.Code, secondGenerated.Body.String())
	}
	if got := secondGenerated.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("second generate Cache-Control = %q, want no-store", got)
	}
	secondHidden := hiddenPattern.FindStringSubmatch(secondGenerated.Body.String())
	if secondHidden == nil {
		t.Fatal("second review page has no canonical Proposal hidden field")
	}
	secondCanonicalProposal := html.UnescapeString(secondHidden[1])
	if secondCanonicalProposal != aiProposalFixture || secondCanonicalProposal != canonicalProposal {
		t.Fatalf("second generated Proposal changed\n first: %s\nsecond: %s\n  want: %s", canonicalProposal, secondCanonicalProposal, aiProposalFixture)
	}
	secondGenerateCall := fake.calls[len(fake.calls)-1]
	if secondGenerateCall.method != http.MethodPost || secondGenerateCall.path != "/api/v1/capabilities/CAP-1/ai-proposals" || len(secondGenerateCall.body) != 0 {
		t.Fatalf("second generate delegation = %+v", secondGenerateCall)
	}

	acceptValues := url.Values{"revision_id": {"CAP-1-REV-3"}, "proposal": {secondCanonicalProposal}}
	acceptRequest := httptest.NewRequest(http.MethodPost, "/features/FC-1/ai-proposals/accept", strings.NewReader(acceptValues.Encode()))
	acceptRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	accepted := httptest.NewRecorder()
	handler.ServeHTTP(accepted, acceptRequest)
	if accepted.Code != http.StatusSeeOther {
		t.Fatalf("accept status = %d, want 303; body = %s", accepted.Code, accepted.Body.String())
	}
	if got := accepted.Header().Get("Location"); got != "/features/FC-1/revisions" {
		t.Errorf("accept Location = %q, want revisions page", got)
	}
	if got := accepted.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("accept Cache-Control = %q, want no-store", got)
	}
	acceptCall := fake.calls[len(fake.calls)-1]
	if acceptCall.method != http.MethodPost || acceptCall.path != "/api/v1/capabilities/CAP-1/ai-proposals/accept" {
		t.Fatalf("accept delegation = %+v", acceptCall)
	}
	if strings.Contains(string(acceptCall.body), `\u003c`) || strings.Contains(string(acceptCall.body), `\u003e`) || strings.Contains(string(acceptCall.body), `\u0026`) {
		t.Fatalf("callAPI HTML-escaped nested canonical Proposal: %s", acceptCall.body)
	}
	var delegated apiAcceptCapabilityProposalDTO
	if err := json.Unmarshal(acceptCall.body, &delegated); err != nil {
		t.Fatalf("decode delegated accept body: %v; body = %s", err, acceptCall.body)
	}
	if delegated.RevisionID != "CAP-1-REV-3" || string(delegated.Proposal) != aiProposalFixture {
		t.Fatalf("delegated accept body changed reviewed values: revision=%q proposal=%s", delegated.RevisionID, delegated.Proposal)
	}
	history := httptest.NewRecorder()
	handler.ServeHTTP(history, httptest.NewRequest(http.MethodGet, accepted.Header().Get("Location"), nil))
	if history.Code != http.StatusOK {
		t.Fatalf("accepted revision history status = %d, want 200; body = %s", history.Code, history.Body.String())
	}
	if !strings.Contains(history.Body.String(), "featureforge:ai-assisted") {
		t.Fatalf("accepted revision history does not show provenance method: %s", history.Body.String())
	}
}
