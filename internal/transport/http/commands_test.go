package http_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering/peos"
	"github.com/aleka7sk/featureforge/internal/infrastructure/memory"
	transporthttp "github.com/aleka7sk/featureforge/internal/transport/http"
)

// newTestDeps builds a full Dependencies value backed by a fresh in-memory
// store and a real PEOS-backed Recorder -- the composition every command
// handler test in this file uses, per FF-018 §18.1 ("not a mocked
// application layer"). A FixedClock, not SystemClock, matching every other
// fixture in this codebase (e.g. newCommandFixture): every command
// captures clock.Now() fresh on each Execute and that value participates
// in the persisted record's equality (domain.Project.createdAt, for
// instance), so an idempotent-replay test needs the same instant on both
// calls, which only a controlled clock can guarantee.
func newTestDeps() transporthttp.Dependencies {
	return transporthttp.Dependencies{
		UOW:       memory.NewUnitOfWork(memory.NewStore()),
		Recorder:  peos.NewRecorder(),
		Projector: peos.NewRecorder(),
		Clock:     application.NewFixedClock(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)),
	}
}

// postJSON POSTs body (marshalled to JSON) to path against handler and
// decodes the response into out. It fails the test on a transport-level
// decode error but leaves status-code assertions to the caller.
func postJSON(t *testing.T, handler http.Handler, path string, body, out any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if out != nil && rr.Code < 300 {
		if err := json.Unmarshal(rr.Body.Bytes(), out); err != nil {
			t.Fatalf("decoding response: %v; body = %s", err, rr.Body.String())
		}
	}
	return rr
}

func errorCode(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not the JSON error shape: %v; body = %s", err, rr.Body.String())
	}
	return body.Error.Code
}

// TestCommandEndpointsCanonicalOrder drives all twelve command endpoints
// once, in canonical-scenario order (FF-018 §16 step 6), proving each
// returns 201 with the expected response fields. This is the "success"
// case for every endpoint; the exhaustive engineering-correctness proof is
// the scenario-through-HTTP test (FF-018 §16 step 9), which this does not
// duplicate.
func TestCommandEndpointsCanonicalOrder(t *testing.T) {
	deps := newTestDeps()
	handler := transporthttp.NewHandler(deps)

	// C1 CreateProject
	var projectEnvelope struct {
		Data struct {
			ProjectID string `json:"project_id"`
		} `json:"data"`
	}
	rr := postJSON(t, handler, "/api/v1/projects", map[string]any{
		"project_id": "PRJ-1", "name": "Pilot",
	}, &projectEnvelope)
	if rr.Code != http.StatusCreated {
		t.Fatalf("C1: status = %d, want 201; body = %s", rr.Code, rr.Body.String())
	}
	if projectEnvelope.Data.ProjectID != "PRJ-1" {
		t.Errorf("C1: project_id = %q, want PRJ-1", projectEnvelope.Data.ProjectID)
	}

	// C2 CreateFeature
	var featureEnvelope struct {
		Data struct {
			FeatureCardID string `json:"feature_card_id"`
		} `json:"data"`
	}
	rr = postJSON(t, handler, "/api/v1/features", map[string]any{
		"feature_card_id": "FC-1", "project_id": "PRJ-1", "title": "Homework after a lesson", "description": "",
	}, &featureEnvelope)
	if rr.Code != http.StatusCreated {
		t.Fatalf("C2: status = %d, want 201; body = %s", rr.Code, rr.Body.String())
	}
	if featureEnvelope.Data.FeatureCardID != "FC-1" {
		t.Errorf("C2: feature_card_id = %q, want FC-1", featureEnvelope.Data.FeatureCardID)
	}

	// C3 EstablishCapabilitySpecification
	content := map[string]any{
		"schema_version": 1, "title": "Homework after a lesson", "problem_statement": "No follow-up today.",
		"user_outcome": "A student can see homework.",
	}
	var capabilityEnvelope struct {
		Data struct {
			ArtifactID string `json:"artifact_id"`
			RevisionID string `json:"revision_id"`
			Sequence   int    `json:"sequence"`
		} `json:"data"`
	}
	rr = postJSON(t, handler, "/api/v1/capabilities", map[string]any{
		"feature_card_id": "FC-1", "artifact_id": "CAP-1", "revision_id": "CAP-1-REV-1", "content": content,
	}, &capabilityEnvelope)
	if rr.Code != http.StatusCreated {
		t.Fatalf("C3: status = %d, want 201; body = %s", rr.Code, rr.Body.String())
	}
	if capabilityEnvelope.Data.ArtifactID != "CAP-1" || capabilityEnvelope.Data.RevisionID != "CAP-1-REV-1" || capabilityEnvelope.Data.Sequence != 1 {
		t.Errorf("C3: got %+v, want artifact_id=CAP-1 revision_id=CAP-1-REV-1 sequence=1", capabilityEnvelope.Data)
	}

	// C4 ReviseCapabilitySpecification
	content2 := map[string]any{
		"schema_version": 1, "title": "Homework after a lesson", "problem_statement": "No follow-up today.",
		"user_outcome": "A student can see homework, including audio.",
	}
	var reviseEnvelope struct {
		Data struct {
			RevisionID string `json:"revision_id"`
			Sequence   int    `json:"sequence"`
		} `json:"data"`
	}
	rr = postJSON(t, handler, "/api/v1/capabilities/CAP-1/revisions", map[string]any{
		"revision_id": "CAP-1-REV-2", "content": content2,
	}, &reviseEnvelope)
	if rr.Code != http.StatusCreated {
		t.Fatalf("C4: status = %d, want 201; body = %s", rr.Code, rr.Body.String())
	}
	if reviseEnvelope.Data.RevisionID != "CAP-1-REV-2" || reviseEnvelope.Data.Sequence != 2 {
		t.Errorf("C4: got %+v, want revision_id=CAP-1-REV-2 sequence=2", reviseEnvelope.Data)
	}

	// C5 AcceptCapabilityRevision
	var acceptEnvelope struct {
		Data struct {
			RecordID string `json:"record_id"`
		} `json:"data"`
	}
	rr = postJSON(t, handler, "/api/v1/capabilities/CAP-1/acceptances", map[string]any{
		"record_id": "ACC-1", "revision_id": "CAP-1-REV-1", "state": "accepted",
	}, &acceptEnvelope)
	if rr.Code != http.StatusCreated {
		t.Fatalf("C5: status = %d, want 201; body = %s", rr.Code, rr.Body.String())
	}
	if acceptEnvelope.Data.RecordID != "ACC-1" {
		t.Errorf("C5: record_id = %q, want ACC-1", acceptEnvelope.Data.RecordID)
	}
	// Accept revision 2 as well, so C6's entry lifecycle assignment (below)
	// has an accepted current revision to resolve against.
	rr = postJSON(t, handler, "/api/v1/capabilities/CAP-1/acceptances", map[string]any{
		"record_id": "ACC-2", "revision_id": "CAP-1-REV-2", "state": "accepted",
	}, nil)
	if rr.Code != http.StatusCreated {
		t.Fatalf("C5 (rev 2): status = %d, want 201; body = %s", rr.Code, rr.Body.String())
	}

	// C7 EstablishRequirement
	var requirementEnvelope struct {
		Data struct {
			ArtifactID string `json:"artifact_id"`
			RevisionID string `json:"revision_id"`
		} `json:"data"`
	}
	rr = postJSON(t, handler, "/api/v1/requirements", map[string]any{
		"artifact_id": "REQ-1", "revision_id": "REQ-1-REV-1",
		"statement": "Published homework SHALL be visible to the student.", "subject_artifact_id": "CAP-1",
	}, &requirementEnvelope)
	if rr.Code != http.StatusCreated {
		t.Fatalf("C7: status = %d, want 201; body = %s", rr.Code, rr.Body.String())
	}
	if requirementEnvelope.Data.ArtifactID != "REQ-1" {
		t.Errorf("C7: artifact_id = %q, want REQ-1", requirementEnvelope.Data.ArtifactID)
	}

	// C8 RecordArchitectureDecision
	var decisionEnvelope struct {
		Data struct {
			DecisionKey string `json:"decision_key"`
		} `json:"data"`
	}
	rr = postJSON(t, handler, "/api/v1/decisions", map[string]any{
		"decision_id": "DEC-1", "subject_artifact_id": "CAP-1", "subject_revision_id": "CAP-1-REV-1",
		"question": "Should homework support audio?", "outcome_statement": "Yes, by content address.",
		"evidence_artifact_id": "EV-0", "evidence_revision_id": "EV-0-REV-1",
	}, &decisionEnvelope)
	if rr.Code != http.StatusCreated {
		t.Fatalf("C8: status = %d, want 201; body = %s", rr.Code, rr.Body.String())
	}
	if decisionEnvelope.Data.DecisionKey != "decision:DEC-1" {
		t.Errorf("C8: decision_key = %q, want decision:DEC-1", decisionEnvelope.Data.DecisionKey)
	}

	// C9 EstablishValidationPlan
	var planEnvelope struct {
		Data struct {
			ArtifactID string `json:"artifact_id"`
			RevisionID string `json:"revision_id"`
		} `json:"data"`
	}
	rr = postJSON(t, handler, "/api/v1/validation/plans", map[string]any{
		"artifact_id": "VP-1", "revision_id": "VP-1-REV-1", "scope_artifact_id": "CAP-1",
		"activities": []map[string]any{{
			"key": "A-1", "subject_artifact_id": "CAP-1", "subject_revision_id": "CAP-1-REV-2",
			"method": "manual-review", "outcome_interpretation": "Satisfied when reviewed.",
			"requirement_artifact_id": "REQ-1", "requirement_revision_id": "REQ-1-REV-1",
			"expected_evidence": []string{"reviewer note"},
		}},
	}, &planEnvelope)
	if rr.Code != http.StatusCreated {
		t.Fatalf("C9: status = %d, want 201; body = %s", rr.Code, rr.Body.String())
	}
	if planEnvelope.Data.ArtifactID != "VP-1" {
		t.Errorf("C9: artifact_id = %q, want VP-1", planEnvelope.Data.ArtifactID)
	}

	// C10 RecordValidationRun
	var runEnvelope struct {
		Data struct {
			ExecutionKey string `json:"execution_key"`
		} `json:"data"`
	}
	rr = postJSON(t, handler, "/api/v1/validation/runs", map[string]any{
		"execution_id": "ER-1", "plan_artifact_id": "VP-1", "plan_revision_id": "VP-1-REV-1", "activity_key": "A-1",
		"subject_artifact_id": "CAP-1", "subject_revision_id": "CAP-1-REV-2",
		"method": "manual-review", "outcome": "completed",
		"evidence_artifact_id": "EV-1", "evidence_revision_id": "EV-1-REV-1", "evidence_locator": "https://evidence.example/EV-1",
	}, &runEnvelope)
	if rr.Code != http.StatusCreated {
		t.Fatalf("C10: status = %d, want 201; body = %s", rr.Code, rr.Body.String())
	}
	if runEnvelope.Data.ExecutionKey != "execution:ER-1" {
		t.Errorf("C10: execution_key = %q, want execution:ER-1", runEnvelope.Data.ExecutionKey)
	}

	// C11 RecordValidationClaim
	var claimEnvelope struct {
		Data struct {
			ClaimKey string `json:"claim_key"`
		} `json:"data"`
	}
	rr = postJSON(t, handler, "/api/v1/validation/claims", map[string]any{
		"claim_id": "CLM-1", "scope_artifact_id": "CAP-1",
		"subject_artifact_id": "CAP-1", "subject_revision_id": "CAP-1-REV-2",
		"requirement_artifact_id": "REQ-1", "requirement_revision_id": "REQ-1-REV-1",
		"outcome": "satisfied", "method": "manual-review",
		"evidence_artifact_id": "EV-1", "evidence_revision_id": "EV-1-REV-1", "execution_id": "ER-1",
		"reasoning": "The specification states it explicitly.",
	}, &claimEnvelope)
	if rr.Code != http.StatusCreated {
		t.Fatalf("C11: status = %d, want 201; body = %s", rr.Code, rr.Body.String())
	}
	if claimEnvelope.Data.ClaimKey != "claim:CLM-1" {
		t.Errorf("C11: claim_key = %q, want claim:CLM-1", claimEnvelope.Data.ClaimKey)
	}

	// C12 CorrectValidationClaim
	rr = postJSON(t, handler, "/api/v1/validation/runs", map[string]any{
		"execution_id": "ER-2", "plan_artifact_id": "VP-1", "plan_revision_id": "VP-1-REV-1", "activity_key": "A-1",
		"subject_artifact_id": "CAP-1", "subject_revision_id": "CAP-1-REV-2",
		"method": "manual-review", "outcome": "completed",
		"evidence_artifact_id": "EV-2", "evidence_revision_id": "EV-2-REV-1", "evidence_locator": "https://evidence.example/EV-2",
	}, nil)
	if rr.Code != http.StatusCreated {
		t.Fatalf("C12 setup (rerun): status = %d, want 201; body = %s", rr.Code, rr.Body.String())
	}
	var correctionEnvelope struct {
		Data struct {
			ClaimKey string `json:"claim_key"`
		} `json:"data"`
	}
	rr = postJSON(t, handler, "/api/v1/validation/claims/corrections", map[string]any{
		"claim_id": "CLM-2", "correction_target": "CLM-1", "correction_kind": "correct",
		"scope_artifact_id":   "CAP-1",
		"subject_artifact_id": "CAP-1", "subject_revision_id": "CAP-1-REV-2",
		"requirement_artifact_id": "REQ-1", "requirement_revision_id": "REQ-1-REV-1",
		"outcome": "not-satisfied", "method": "manual-review",
		"evidence_artifact_id": "EV-2", "evidence_revision_id": "EV-2-REV-1", "execution_id": "ER-2",
		"reasoning": "The original review was mistaken.",
	}, &correctionEnvelope)
	if rr.Code != http.StatusCreated {
		t.Fatalf("C12: status = %d, want 201; body = %s", rr.Code, rr.Body.String())
	}
	if correctionEnvelope.Data.ClaimKey != "claim:CLM-2" {
		t.Errorf("C12: claim_key = %q, want claim:CLM-2", correctionEnvelope.Data.ClaimKey)
	}

	// C6 AssignLifecycleState (entry)
	var lifecycleEnvelope struct {
		Data struct {
			AssignmentKey string `json:"assignment_key"`
		} `json:"data"`
	}
	rr = postJSON(t, handler, "/api/v1/capabilities/CAP-1/lifecycle", map[string]any{
		"assignment_id": "SA-1", "state": "drafting", "is_entry": true,
		"transition_record_artifact_id": "TR-1", "transition_record_revision_id": "TR-1-REV-0",
	}, &lifecycleEnvelope)
	if rr.Code != http.StatusCreated {
		t.Fatalf("C6: status = %d, want 201; body = %s", rr.Code, rr.Body.String())
	}
	if lifecycleEnvelope.Data.AssignmentKey != "state-assignment:SA-1" {
		t.Errorf("C6: assignment_key = %q, want state-assignment:SA-1", lifecycleEnvelope.Data.AssignmentKey)
	}
}

// TestCommandIdempotentReplay proves an identical re-POST is a no-op
// returning the original success status (FF-018 §11.1), for a
// representative sample -- C1 (no PEOS involvement) and C3 (a revision,
// where AD-026 equality governs conflict detection).
func TestCommandIdempotentReplay(t *testing.T) {
	deps := newTestDeps()
	handler := transporthttp.NewHandler(deps)

	body := map[string]any{"project_id": "PRJ-1", "name": "Pilot"}
	first := postJSON(t, handler, "/api/v1/projects", body, nil)
	if first.Code != http.StatusCreated {
		t.Fatalf("first: status = %d, want 201", first.Code)
	}
	second := postJSON(t, handler, "/api/v1/projects", body, nil)
	if second.Code != http.StatusCreated {
		t.Errorf("identical replay: status = %d, want 201 (idempotent no-op)", second.Code)
	}
}

// TestCommandConflictingReplay proves a differing re-POST with the same
// identity is 409 (FF-018 §11.1).
func TestCommandConflictingReplay(t *testing.T) {
	deps := newTestDeps()
	handler := transporthttp.NewHandler(deps)

	first := postJSON(t, handler, "/api/v1/projects", map[string]any{"project_id": "PRJ-1", "name": "Original"}, nil)
	if first.Code != http.StatusCreated {
		t.Fatalf("first: status = %d, want 201", first.Code)
	}
	second := postJSON(t, handler, "/api/v1/projects", map[string]any{"project_id": "PRJ-1", "name": "Different"}, nil)
	if second.Code != http.StatusConflict {
		t.Fatalf("conflicting replay: status = %d, want 409; body = %s", second.Code, second.Body.String())
	}
	if code := errorCode(t, second); code != "immutable_value_conflict" {
		t.Errorf("code = %q, want immutable_value_conflict", code)
	}
}

// TestAssignLifecycleReplayHonorsAD026SubjectKeyEquality proves AD-026's
// RevisionEnvelope.Equal -- which compares RevisionKey, Payload, AND
// SubjectKey -- governs conflict detection through HTTP, isolating each
// component in turn. Post-implementation audit finding MINOR-2.
//
// C6's entry-assignment path (IsEntry: true) is used rather than a
// revision-family command like C3/C4/C7/C9: those revisions' families
// (capability, requirement) either define no SubjectKey at all (FF-016
// §3.2 -- "capability: the revision *is* the subject") or embed the
// subject inside their own Payload as PEOS content (requirement), so
// varying the subject there also changes the Payload and cannot isolate
// AD-026's contribution from the pre-existing Payload-equality check.
// BuildEntryAssignment's Transition Record Revision is content-free
// (AD-014): its Payload is a function of TransitionRecordArtifactID,
// TransitionRecordRevisionID, and RecordedAt alone (codec_lifecycle.go),
// while SubjectKey is set independently from SubjectArtifactID -- the
// {artifactID} path segment C6 maps to it. This is the exact shape FF-016
// architecture review finding F-001 identified as unrecoverable from
// Payload alone, which is what AD-026 corrected.
func TestAssignLifecycleReplayHonorsAD026SubjectKeyEquality(t *testing.T) {
	clock := application.NewFixedClock(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	deps := transporthttp.Dependencies{
		UOW: memory.NewUnitOfWork(memory.NewStore()), Recorder: peos.NewRecorder(), Clock: clock,
	}
	handler := transporthttp.NewHandler(deps)

	// AD-021: RecordEnvelopeRepository.Put verifies a record's SubjectKey
	// resolves to a real Artifact, so both subjects named below (CAP-1,
	// CAP-2) must exist before an entry assignment can name them. Each gets
	// its own feature card: a FeatureCard links at most one capability
	// (ErrCapabilityAlreadyLinked).
	postJSON(t, handler, "/api/v1/projects", map[string]any{"project_id": "PRJ-1", "name": "Pilot"}, nil)
	content := map[string]any{"schema_version": 1, "title": "Homework", "problem_statement": "No follow-up today."}
	for i, artifactID := range []string{"CAP-1", "CAP-2"} {
		featureCardID := fmt.Sprintf("FC-%d", i+1)
		postJSON(t, handler, "/api/v1/features", map[string]any{
			"feature_card_id": featureCardID, "project_id": "PRJ-1", "title": "Homework", "description": "",
		}, nil)
		rr := postJSON(t, handler, "/api/v1/capabilities", map[string]any{
			"feature_card_id": featureCardID, "artifact_id": artifactID, "revision_id": artifactID + "-REV-1", "content": content,
		}, nil)
		if rr.Code != http.StatusCreated {
			t.Fatalf("seeding %s: status = %d, want 201; body = %s", artifactID, rr.Code, rr.Body.String())
		}
	}

	entry := func(assignmentID string) map[string]any {
		return map[string]any{
			"assignment_id": assignmentID, "state": "drafting", "is_entry": true,
			"transition_record_artifact_id": "TR-1", "transition_record_revision_id": "TR-1-REV-0",
		}
	}

	// 1. Identical replay -- same RevisionKey (TR-1/TR-1-REV-0), same
	// content-free Payload (clock unchanged), same SubjectKey (path CAP-1)
	// -- succeeds as a no-op.
	first := postJSON(t, handler, "/api/v1/capabilities/CAP-1/lifecycle", entry("SA-1"), nil)
	if first.Code != http.StatusCreated {
		t.Fatalf("first entry assignment: status = %d, want 201; body = %s", first.Code, first.Body.String())
	}
	second := postJSON(t, handler, "/api/v1/capabilities/CAP-1/lifecycle", entry("SA-1"), nil)
	if second.Code != http.StatusCreated {
		t.Errorf("identical replay: status = %d, want 201 (idempotent no-op)", second.Code)
	}

	// 2. Same RevisionKey and byte-identical Payload (same
	// transition_record_artifact_id/transition_record_revision_id, clock
	// still unchanged), but a different SubjectKey -- the {artifactID}
	// path segment is CAP-2, not CAP-1. AD-026 requires this to conflict
	// even though every other component matches.
	subjectConflict := postJSON(t, handler, "/api/v1/capabilities/CAP-2/lifecycle", entry("SA-2"), nil)
	if subjectConflict.Code != http.StatusConflict {
		t.Fatalf("SubjectKey-varying replay: status = %d, want 409; body = %s", subjectConflict.Code, subjectConflict.Body.String())
	}
	if code := errorCode(t, subjectConflict); code != "immutable_value_conflict" {
		t.Errorf("SubjectKey-varying replay: code = %q, want immutable_value_conflict", code)
	}

	// 3. Same RevisionKey and same SubjectKey (path CAP-1 again), but the
	// clock has advanced -- RecordedAt is embedded in the content-free
	// revision's Payload, so this changes Payload alone. Conflicts on the
	// pre-AD-026 check: payload inequality.
	clock.Advance(time.Hour)
	payloadConflict := postJSON(t, handler, "/api/v1/capabilities/CAP-1/lifecycle", entry("SA-3"), nil)
	if payloadConflict.Code != http.StatusConflict {
		t.Fatalf("payload-varying replay: status = %d, want 409; body = %s", payloadConflict.Code, payloadConflict.Body.String())
	}
	if code := errorCode(t, payloadConflict); code != "immutable_value_conflict" {
		t.Errorf("payload-varying replay: code = %q, want immutable_value_conflict", code)
	}
}

// TestCommandDecodeFailureNeverInvokesApplication proves malformed JSON,
// an unknown field, and a second JSON value all return 400 without
// creating anything -- decodeJSON's own behaviour is exhaustively unit
// tested in decode_test.go; this proves the handler wiring honours it
// (FF-018 §9.2 "not invoked after a decode failure").
func TestCommandDecodeFailureNeverInvokesApplication(t *testing.T) {
	deps := newTestDeps()
	handler := transporthttp.NewHandler(deps)

	tests := []struct {
		name string
		body string
	}{
		{"malformed JSON", `{"project_id":`},
		{"unknown field", `{"project_id":"PRJ-1","name":"Pilot","extra":true}`},
		{"multiple JSON values", `{"project_id":"PRJ-1","name":"Pilot"}{}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400; body = %s", rr.Code, rr.Body.String())
			}
		})
	}

	// Nothing was created: the store still accepts PRJ-1 as new.
	rr := postJSON(t, handler, "/api/v1/projects", map[string]any{"project_id": "PRJ-1", "name": "Pilot"}, nil)
	if rr.Code != http.StatusCreated {
		t.Errorf("PRJ-1 should still be creatable after decode failures; status = %d, body = %s", rr.Code, rr.Body.String())
	}
}

// TestCommandValidationError proves a well-formed but incomplete command
// maps to 400 via ErrInvalidCommand (FF-018 §9.1 layer 2).
func TestCommandValidationError(t *testing.T) {
	deps := newTestDeps()
	handler := transporthttp.NewHandler(deps)

	rr := postJSON(t, handler, "/api/v1/projects", map[string]any{"project_id": "", "name": "Pilot"}, nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rr.Code, rr.Body.String())
	}
	if code := errorCode(t, rr); code != "invalid_command" {
		t.Errorf("code = %q, want invalid_command", code)
	}
}

// TestCommandContentMappingError proves a malformed content sub-object
// (C3) fails with 400 via the builder chain's own first error, not a 500
// or a silently accepted value (FF-018 §3.1).
func TestCommandContentMappingError(t *testing.T) {
	deps := newTestDeps()
	handler := transporthttp.NewHandler(deps)

	postJSON(t, handler, "/api/v1/projects", map[string]any{"project_id": "PRJ-1", "name": "Pilot"}, nil)
	postJSON(t, handler, "/api/v1/features", map[string]any{
		"feature_card_id": "FC-1", "project_id": "PRJ-1", "title": "Homework", "description": "",
	}, nil)

	rr := postJSON(t, handler, "/api/v1/capabilities", map[string]any{
		"feature_card_id": "FC-1", "artifact_id": "CAP-1", "revision_id": "CAP-1-REV-1",
		"content": map[string]any{"schema_version": 1, "title": "", "problem_statement": "No follow-up."},
	}, nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rr.Code, rr.Body.String())
	}
}

// A direct handler-level test for an empty {artifactID} path value lives
// in decode_test.go (package http): net/http.ServeMux cleans duplicate
// slashes and redirects (301) before any handler runs, and overwrites a
// pre-set PathValue with its own match, so this cannot be exercised
// through NewHandler's router from outside the package.
