package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering"
	"github.com/aleka7sk/featureforge/internal/engineering/peos"
	"github.com/aleka7sk/featureforge/internal/proposal"
	transporthttp "github.com/aleka7sk/featureforge/internal/transport/http"
)

type generatedProposalData struct {
	ContextPack json.RawMessage `json:"context_pack"`
	Proposal    json.RawMessage `json:"proposal"`
}

func seedProposalCapability(t *testing.T, handler http.Handler, suffix, revisionID, problem string) {
	t.Helper()
	projectID := "PRJ-" + suffix
	featureID := "FC-" + suffix
	artifactID := "CAP-" + suffix
	mustPost(t, handler, "/api/v1/projects", map[string]any{
		"project_id": projectID, "name": "AI proposal test",
	})
	mustPost(t, handler, "/api/v1/features", map[string]any{
		"feature_card_id": featureID, "project_id": projectID, "title": "AI proposal test",
	})
	mustPost(t, handler, "/api/v1/capabilities", map[string]any{
		"feature_card_id": featureID,
		"artifact_id":     artifactID,
		"revision_id":     revisionID,
		"content": map[string]any{
			"schema_version":    1,
			"title":             "AI proposal test",
			"problem_statement": problem,
			"acceptance_criteria": []map[string]any{
				{"key": "AC-1", "text": "The reviewed proposal remains source-bound."},
			},
			"open_questions": []string{"Which source should be reviewed next?"},
		},
	})
	mustPost(t, handler, "/api/v1/capabilities/"+artifactID+"/acceptances", map[string]any{
		"record_id": "ACC-" + suffix + "-1", "revision_id": revisionID, "state": "accepted",
	})
}

func generateProposal(t *testing.T, handler http.Handler, artifactID string) (*httptest.ResponseRecorder, generatedProposalData) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/capabilities/"+artifactID+"/ai-proposals", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("generate status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	var response struct {
		Data generatedProposalData `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode generate response: %v; body = %s", err, rr.Body.String())
	}
	return rr, response.Data
}

func postCanonicalProposalAcceptance(t *testing.T, handler http.Handler, path, revisionID string, proposalJSON json.RawMessage, out any) *httptest.ResponseRecorder {
	t.Helper()
	revisionJSON, err := json.Marshal(revisionID)
	if err != nil {
		t.Fatal(err)
	}
	body := make([]byte, 0, len(revisionJSON)+len(proposalJSON)+32)
	body = append(body, `{"revision_id":`...)
	body = append(body, revisionJSON...)
	body = append(body, `,"proposal":`...)
	body = append(body, proposalJSON...)
	body = append(body, '}')
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if out != nil && rr.Code < 300 {
		if err := json.Unmarshal(rr.Body.Bytes(), out); err != nil {
			t.Fatalf("decode accept response: %v; body = %s", err, rr.Body.String())
		}
	}
	return rr
}

func TestAIProposalGenerateAndAcceptHTTP(t *testing.T) {
	deps := newTestDeps()
	handler := transporthttp.NewHandler(deps)
	seedProposalCapability(t, handler, "AI-HTTP", "CAP-AI-HTTP-REV-1", "A reviewed <source> & its exact provenance must survive canonical transport.")

	generatedResponse, generated := generateProposal(t, handler, "CAP-AI-HTTP")
	if got := generatedResponse.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("generate Cache-Control = %q, want no-store", got)
	}
	if len(generated.ContextPack) == 0 || len(generated.Proposal) == 0 {
		t.Fatalf("generate response omitted complete DTOs: %+v", generated)
	}
	secondResponse, secondGenerated := generateProposal(t, handler, "CAP-AI-HTTP")
	if !bytes.Equal(generatedResponse.Body.Bytes(), secondResponse.Body.Bytes()) ||
		!bytes.Equal(generated.ContextPack, secondGenerated.ContextPack) ||
		!bytes.Equal(generated.Proposal, secondGenerated.Proposal) {
		t.Fatal("repeated generation over equal state returned different data bytes")
	}
	pack, err := proposal.ParseContextPack(generated.ContextPack)
	if err != nil {
		t.Fatalf("context_pack is not a complete canonical ContextPack: %v", err)
	}
	parsedProposal, err := proposal.ParseProposal(generated.Proposal)
	if err != nil {
		t.Fatalf("proposal is not a complete canonical Proposal: %v", err)
	}
	if err := parsedProposal.ValidateAgainst(pack); err != nil {
		t.Fatalf("generated proposal does not bind generated context: %v", err)
	}
	canonicalContext, err := pack.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	canonicalProposal, err := parsedProposal.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(generated.ContextPack, canonicalContext) || !bytes.Equal(generated.Proposal, canonicalProposal) {
		t.Fatal("generate response changed canonical ContextPack or Proposal bytes")
	}

	var accepted struct {
		Data struct {
			ArtifactID string `json:"artifact_id"`
			RevisionID string `json:"revision_id"`
			Sequence   int    `json:"sequence"`
		} `json:"data"`
	}
	acceptedResponse := postCanonicalProposalAcceptance(
		t, handler, "/api/v1/capabilities/CAP-AI-HTTP/ai-proposals/accept", "CAP-AI-HTTP-REV-2", generated.Proposal, &accepted,
	)
	if acceptedResponse.Code != http.StatusCreated {
		t.Fatalf("accept status = %d, want 201; body = %s", acceptedResponse.Code, acceptedResponse.Body.String())
	}
	if got := acceptedResponse.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("accept Cache-Control = %q, want no-store", got)
	}
	if accepted.Data.ArtifactID != "CAP-AI-HTTP" || accepted.Data.RevisionID != "CAP-AI-HTTP-REV-2" || accepted.Data.Sequence != 2 {
		t.Fatalf("accept data = %+v, want CAP-AI-HTTP/CAP-AI-HTTP-REV-2 sequence 2", accepted.Data)
	}

	// Exact occupied replay remains the same 201 result and appends nothing.
	replay := postCanonicalProposalAcceptance(
		t, handler, "/api/v1/capabilities/CAP-AI-HTTP/ai-proposals/accept", "CAP-AI-HTTP-REV-2", generated.Proposal, nil,
	)
	if replay.Code != http.StatusCreated {
		t.Fatalf("accept replay status = %d, want 201; body = %s", replay.Code, replay.Body.String())
	}
	journalSize := queryRepo(t, deps.UOW, func(repos application.Repositories) (int, error) {
		rows, err := repos.RevisionAcceptance.ListByRevision(context.Background(), engineering.RevisionKey{
			ArtifactID: "CAP-AI-HTTP", RevisionID: "CAP-AI-HTTP-REV-2",
		})
		return len(rows), err
	})
	if journalSize != 0 {
		t.Fatalf("AI-assisted draft appended %d acceptance rows, want zero", journalSize)
	}
}

func TestAIProposalHTTPRejectsTamperBodiesAndRejectRoute(t *testing.T) {
	deps := newTestDeps()
	handler := transporthttp.NewHandler(deps)
	seedProposalCapability(t, handler, "AI-TAMPER", "CAP-AI-TAMPER-REV-1", "Tampered transient values must not persist.")
	_, generated := generateProposal(t, handler, "CAP-AI-TAMPER")
	parsed, err := proposal.ParseProposal(generated.Proposal)
	if err != nil {
		t.Fatal(err)
	}
	originalDigest := parsed.ProposalDigest().Hex()
	tamperedDigest := "0" + originalDigest[1:]
	if tamperedDigest == originalDigest {
		tamperedDigest = "1" + originalDigest[1:]
	}
	tampered := bytes.Replace(generated.Proposal, []byte(originalDigest), []byte(tamperedDigest), 1)
	if bytes.Equal(tampered, generated.Proposal) {
		t.Fatal("test did not tamper proposal_digest")
	}

	rr := postCanonicalProposalAcceptance(
		t, handler, "/api/v1/capabilities/CAP-AI-TAMPER/ai-proposals/accept", "CAP-AI-TAMPER-REV-2", tampered, nil,
	)
	if rr.Code != http.StatusBadRequest || errorCode(t, rr) != "invalid_command" {
		t.Fatalf("tampered proposal status/code = %d/%s, want 400/invalid_command; body = %s", rr.Code, errorCode(t, rr), rr.Body.String())
	}

	unknownOuter := postJSON(t, handler, "/api/v1/capabilities/CAP-AI-TAMPER/ai-proposals/accept", map[string]any{
		"revision_id": "CAP-AI-TAMPER-REV-2", "proposal": generated.Proposal, "unexpected": true,
	}, nil)
	if unknownOuter.Code != http.StatusBadRequest || errorCode(t, unknownOuter) != "invalid_command" {
		t.Fatalf("unknown accept field status/code = %d/%s, want 400/invalid_command", unknownOuter.Code, errorCode(t, unknownOuter))
	}

	nonEmptyGenerate := postJSON(t, handler, "/api/v1/capabilities/CAP-AI-TAMPER/ai-proposals", map[string]any{"unexpected": true}, nil)
	if nonEmptyGenerate.Code != http.StatusBadRequest || errorCode(t, nonEmptyGenerate) != "invalid_command" {
		t.Fatalf("non-empty generate status/code = %d/%s, want 400/invalid_command", nonEmptyGenerate.Code, errorCode(t, nonEmptyGenerate))
	}

	reject := postJSON(t, handler, "/api/v1/capabilities/CAP-AI-TAMPER/ai-proposals/reject", map[string]any{}, nil)
	if reject.Code != http.StatusNotFound || errorCode(t, reject) != "not_found" {
		t.Fatalf("guessed reject route status/code = %d/%s, want 404/not_found", reject.Code, errorCode(t, reject))
	}

	targetFound := queryRepo(t, deps.UOW, func(repos application.Repositories) (bool, error) {
		_, found, err := repos.Revisions.Get(context.Background(), engineering.RevisionKey{
			ArtifactID: "CAP-AI-TAMPER", RevisionID: "CAP-AI-TAMPER-REV-2",
		})
		return found, err
	})
	if targetFound {
		t.Fatal("tampered proposal created its target revision")
	}
}

func TestAIProposalHTTPMapsStaleContext(t *testing.T) {
	deps := newTestDeps()
	handler := transporthttp.NewHandler(deps)
	seedProposalCapability(t, handler, "AI-STALE", "CAP-AI-STALE-REV-1", "The first accepted context.")
	_, generated := generateProposal(t, handler, "CAP-AI-STALE")

	mustPost(t, handler, "/api/v1/capabilities/CAP-AI-STALE/revisions", map[string]any{
		"revision_id": "CAP-AI-STALE-REV-2",
		"content": map[string]any{
			"schema_version":    1,
			"title":             "AI proposal test",
			"problem_statement": "A later accepted context makes the old proposal stale.",
			"acceptance_criteria": []map[string]any{
				{"key": "AC-1", "text": "The reviewed proposal remains source-bound."},
			},
			"open_questions": []string{"Which source should be reviewed next?"},
		},
	})
	mustPost(t, handler, "/api/v1/capabilities/CAP-AI-STALE/acceptances", map[string]any{
		"record_id": "ACC-AI-STALE-2", "revision_id": "CAP-AI-STALE-REV-2", "state": "accepted",
	})

	rr := postCanonicalProposalAcceptance(
		t, handler, "/api/v1/capabilities/CAP-AI-STALE/ai-proposals/accept", "CAP-AI-STALE-REV-3", generated.Proposal, nil,
	)
	if rr.Code != http.StatusConflict || errorCode(t, rr) != "proposal_context_stale" {
		t.Fatalf("stale proposal status/code = %d/%s, want 409/proposal_context_stale; body = %s", rr.Code, errorCode(t, rr), rr.Body.String())
	}
	if got := rr.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("stale response Cache-Control = %q, want no-store", got)
	}

	targetFound := queryRepo(t, deps.UOW, func(repos application.Repositories) (bool, error) {
		_, found, err := repos.Revisions.Get(context.Background(), engineering.RevisionKey{
			ArtifactID: "CAP-AI-STALE", RevisionID: "CAP-AI-STALE-REV-3",
		})
		return found, err
	})
	if targetFound {
		t.Fatal("stale proposal created its target revision")
	}
}

func TestCapabilityRevisionReadDTOProjectsOnlyAIProvenanceMethod(t *testing.T) {
	deps := newTestDeps()
	handler := transporthttp.NewHandler(deps)
	seedProposalCapability(t, handler, "AI-READ", "CAP-AI-READ-REV-1", "Read DTOs distinguish reviewed AI assistance without changing ordinary JSON.")
	_, generated := generateProposal(t, handler, "CAP-AI-READ")
	accepted := postCanonicalProposalAcceptance(
		t, handler, "/api/v1/capabilities/CAP-AI-READ/ai-proposals/accept", "CAP-AI-READ-REV-2", generated.Proposal, nil,
	)
	if accepted.Code != http.StatusCreated {
		t.Fatalf("accept status = %d, want 201; body = %s", accepted.Code, accepted.Body.String())
	}

	var list struct {
		Data struct {
			Revisions []json.RawMessage `json:"revisions"`
		} `json:"data"`
	}
	listResponse := getJSON(t, handler, "/api/v1/capabilities/CAP-AI-READ/revisions", &list)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("Q6 status = %d, want 200; body = %s", listResponse.Code, listResponse.Body.String())
	}
	if len(list.Data.Revisions) != 2 {
		t.Fatalf("Q6 revisions = %d, want 2", len(list.Data.Revisions))
	}
	for _, raw := range list.Data.Revisions {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		var revisionID string
		if err := json.Unmarshal(fields["revision_id"], &revisionID); err != nil {
			t.Fatal(err)
		}
		method, present := fields["provenance_method"]
		switch revisionID {
		case "CAP-AI-READ-REV-1":
			if present {
				t.Fatalf("ordinary Q6 revision changed JSON with provenance_method=%s", method)
			}
		case "CAP-AI-READ-REV-2":
			var value string
			if !present || json.Unmarshal(method, &value) != nil || value != engineering.AIAssistedMethod {
				t.Fatalf("AI Q6 provenance_method = %s (present %t), want %q", method, present, engineering.AIAssistedMethod)
			}
		default:
			t.Fatalf("unexpected Q6 revision %q", revisionID)
		}
	}

	ordinaryDetail := getJSON(t, handler, "/api/v1/capabilities/CAP-AI-READ/revisions/CAP-AI-READ-REV-1", nil)
	if ordinaryDetail.Code != http.StatusOK {
		t.Fatalf("ordinary Q7 status = %d, want 200", ordinaryDetail.Code)
	}
	if bytes.Contains(ordinaryDetail.Body.Bytes(), []byte(`"provenance_method"`)) {
		t.Fatalf("ordinary Q7 JSON gained provenance_method: %s", ordinaryDetail.Body.String())
	}

	var aiDetail struct {
		Data struct {
			ProvenanceMethod string `json:"provenance_method"`
		} `json:"data"`
	}
	aiDetailResponse := getJSON(t, handler, "/api/v1/capabilities/CAP-AI-READ/revisions/CAP-AI-READ-REV-2", &aiDetail)
	if aiDetailResponse.Code != http.StatusOK {
		t.Fatalf("AI Q7 status = %d, want 200; body = %s", aiDetailResponse.Code, aiDetailResponse.Body.String())
	}
	if aiDetail.Data.ProvenanceMethod != engineering.AIAssistedMethod {
		t.Fatalf("AI Q7 provenance_method = %q, want %q", aiDetail.Data.ProvenanceMethod, engineering.AIAssistedMethod)
	}
}

type unreadableAIWitnessInspector struct {
	application.ProposalReplayInspector
	target engineering.RevisionKey
}

func (i unreadableAIWitnessInspector) InspectAIAssistedCapabilityRevision(
	revision engineering.RevisionEnvelope,
) (engineering.Digest, engineering.Digest, []string, bool, error) {
	if revision.Key == i.target {
		return engineering.Digest{}, engineering.Digest{}, nil, false, errors.New("corrupt AI witness")
	}
	return i.ProposalReplayInspector.InspectAIAssistedCapabilityRevision(revision)
}

func TestCapabilityRevisionReadRejectsUnreadableAIWitness(t *testing.T) {
	deps := newTestDeps()
	handler := transporthttp.NewHandler(deps)
	seedProposalCapability(t, handler, "AI-CORRUPT", "CAP-AI-CORRUPT-REV-1", "The read side must fail closed.")
	_, generated := generateProposal(t, handler, "CAP-AI-CORRUPT")
	accepted := postCanonicalProposalAcceptance(
		t,
		handler,
		"/api/v1/capabilities/CAP-AI-CORRUPT/ai-proposals/accept",
		"CAP-AI-CORRUPT-REV-2",
		generated.Proposal,
		nil,
	)
	if accepted.Code != http.StatusCreated {
		t.Fatalf("accept status = %d, want 201; body = %s", accepted.Code, accepted.Body.String())
	}
	target := engineering.RevisionKey{ArtifactID: "CAP-AI-CORRUPT", RevisionID: "CAP-AI-CORRUPT-REV-2"}
	deps.Inspector = unreadableAIWitnessInspector{
		ProposalReplayInspector: deps.Inspector,
		target:                  target,
	}
	handler = transporthttp.NewHandler(deps)

	for name, path := range map[string]string{
		"Q6": "/api/v1/capabilities/CAP-AI-CORRUPT/revisions",
		"Q7": "/api/v1/capabilities/CAP-AI-CORRUPT/revisions/CAP-AI-CORRUPT-REV-2",
	} {
		t.Run(name, func(t *testing.T) {
			rr := getJSON(t, handler, path, nil)
			if rr.Code != http.StatusInternalServerError || errorCode(t, rr) != "internal_error" {
				t.Fatalf("status/code = %d/%s, want 500/internal_error; body = %s", rr.Code, errorCode(t, rr), rr.Body.String())
			}
			if bytes.Contains(rr.Body.Bytes(), []byte("corrupt")) {
				t.Fatalf("opaque 500 leaked witness detail: %s", rr.Body.String())
			}
		})
	}
}

func TestAIProposalHTTPReturnsOpaqueInternalErrorForCorruptGenerateSource(t *testing.T) {
	deps := newTestDeps()
	handler := transporthttp.NewHandler(deps)
	seedProposalCapability(t, handler, "AI-CORRUPT-SOURCE", "CAP-AI-CORRUPT-SOURCE-REV-1", "A corrupt source must fail the whole generation.")

	recorder := peos.NewRecorder()
	const corruptRevisionID = "CAP-AI-CORRUPT-SOURCE-REV-PARTIAL"
	partial, err := recorder.RecordCapabilityRevision(engineering.CapabilityRevisionInput{
		ArtifactID: "CAP-AI-CORRUPT-SOURCE", RevisionID: corruptRevisionID,
		ContentDigest: engineering.ComputeDigest([]byte("missing structured content")),
		RecordedAt:    time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := deps.UOW.Do(context.Background(), func(repos application.Repositories) error {
		return repos.Revisions.Put(context.Background(), partial)
	}); err != nil {
		t.Fatalf("persist partial proposal source: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/capabilities/CAP-AI-CORRUPT-SOURCE/ai-proposals", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError || errorCode(t, rr) != "internal_error" {
		t.Fatalf("status/code = %d/%s, want 500/internal_error; body = %s", rr.Code, errorCode(t, rr), rr.Body.String())
	}
	for _, forbidden := range []string{"partial", "structured content", corruptRevisionID} {
		if bytes.Contains(rr.Body.Bytes(), []byte(forbidden)) {
			t.Fatalf("opaque generate 500 leaked %q: %s", forbidden, rr.Body.String())
		}
	}
}

func TestAIProposalHTTPReturnsOpaqueInternalErrorForPartialOccupiedAcceptTargetBeforeConflict(t *testing.T) {
	deps := newTestDeps()
	handler := transporthttp.NewHandler(deps)
	seedProposalCapability(t, handler, "AI-CORRUPT-TARGET", "CAP-AI-CORRUPT-TARGET-REV-1", "A partial occupied target must precede conflict classification.")
	_, generated := generateProposal(t, handler, "CAP-AI-CORRUPT-TARGET")
	parsed, err := proposal.ParseProposal(generated.Proposal)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := parsed.Content().Digest()
	if err != nil {
		t.Fatal(err)
	}
	const targetRevisionID = "CAP-AI-CORRUPT-TARGET-REV-2"
	partial, err := peos.NewRecorder().RecordCapabilityRevision(engineering.CapabilityRevisionInput{
		ArtifactID: "CAP-AI-CORRUPT-TARGET", RevisionID: targetRevisionID,
		ContentDigest: digest,
		RecordedAt:    time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := deps.UOW.Do(context.Background(), func(repos application.Repositories) error {
		return repos.Revisions.Put(context.Background(), partial)
	}); err != nil {
		t.Fatalf("persist partial proposal target: %v", err)
	}

	rr := postCanonicalProposalAcceptance(
		t, handler,
		"/api/v1/capabilities/CAP-AI-CORRUPT-TARGET/ai-proposals/accept",
		targetRevisionID,
		generated.Proposal,
		nil,
	)
	if rr.Code != http.StatusInternalServerError || errorCode(t, rr) != "internal_error" {
		t.Fatalf("status/code = %d/%s, want 500/internal_error before immutable conflict; body = %s", rr.Code, errorCode(t, rr), rr.Body.String())
	}
	for _, forbidden := range []string{"partial", "occupied", targetRevisionID, "immutable"} {
		if bytes.Contains(rr.Body.Bytes(), []byte(forbidden)) {
			t.Fatalf("opaque accept 500 leaked %q: %s", forbidden, rr.Body.String())
		}
	}
}
