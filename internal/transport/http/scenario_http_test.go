package http_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering"
	peos "github.com/aleka7sk/featureforge/internal/engineering/peos"
	"github.com/aleka7sk/featureforge/internal/infrastructure/memory"
	"github.com/aleka7sk/featureforge/internal/scenario"
	transporthttp "github.com/aleka7sk/featureforge/internal/transport/http"
)

// mustPost is postJSON with the 201 check inlined, for the many scenario
// acts below that don't need their response decoded.
func mustPost(t *testing.T, handler http.Handler, path string, body map[string]any) {
	t.Helper()
	rr := postJSON(t, handler, path, body, nil)
	if rr.Code != http.StatusCreated {
		t.Fatalf("POST %s: status = %d, want 201; body = %s", path, rr.Code, rr.Body.String())
	}
}

// queryRepo runs one read-only query directly against uow, mirroring
// internal/scenario/scenario_test.go's doQuery helper (unexported there,
// so re-implemented here rather than imported).
func queryRepo[T any](t *testing.T, uow application.UnitOfWork, fn func(application.Repositories) (T, error)) T {
	t.Helper()
	var result T
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		var err error
		result, err = fn(r)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func capabilityRevision1ContentJSON() map[string]any {
	return map[string]any{
		"schema_version":    1,
		"title":             "Homework after a lesson",
		"problem_statement": "After a lesson ends, a teacher has no way to give the student follow-up work, so assignments are passed verbally and lost.",
		"user_outcome":      "A student can see the homework their teacher set after a lesson.",
		"functional_behaviours": []string{
			"A teacher can attach homework to a completed lesson.",
			"A teacher can publish homework.",
			"Published homework becomes visible to that lesson's student.",
		},
		"constraints": []string{"Homework is visible only to the student of that lesson."},
		"acceptance_criteria": []map[string]any{
			{"key": "AC-1", "text": "Published homework is visible to the intended student."},
			{"key": "AC-2", "text": "Homework is not visible to any unrelated user."},
		},
		"dependencies":   []string{"Lesson completion state must be available."},
		"open_questions": []string{"Should homework support an audio attachment?", "What is the acceptable publication latency?"},
	}
}

func capabilityRevision2ContentJSON() map[string]any {
	return map[string]any{
		"schema_version":    1,
		"title":             "Homework after a lesson",
		"problem_statement": "After a lesson ends, a teacher has no way to give the student follow-up work, so assignments are passed verbally and lost.",
		"user_outcome":      "A student can see the homework their teacher set after a lesson, including any audio attachment.",
		"functional_behaviours": []string{
			"A teacher can attach homework to a completed lesson.",
			"A teacher can publish homework.",
			"Published homework becomes visible to that lesson's student.",
			"A teacher may attach one optional audio file to homework.",
			"A published audio attachment is retrievable by the student.",
		},
		"constraints": []string{
			"Homework is visible only to the student of that lesson.",
			"Publication completes within 5 seconds of the teacher's action.",
		},
		"acceptance_criteria": []map[string]any{
			{"key": "AC-1", "text": "Published homework is visible to the intended student."},
			{"key": "AC-2", "text": "Homework is not visible to any unrelated user."},
			{"key": "AC-3", "text": "An optional audio attachment has a resolvable representation for the student."},
			{"key": "AC-4", "text": "Publication is observable to the student within 5 seconds."},
		},
		"dependencies": []string{"Lesson completion state must be available."},
	}
}

// recordEvidenceDirectly records the decision's supporting evidence the
// same way internal/scenario.Run does: directly through the recorder and
// repositories, against the very uow/rec the HTTP handler in this test is
// built on. There is no HTTP command for this act. FF-010 §3 fixes the
// engineering act surface at ten acts (twelve commands); a standalone
// "record evidence" endpoint was never part of it, exactly because every
// other piece of evidence in the scenario arrives bundled with an
// execution via RecordValidationRunCommand. This is the one act genuinely
// outside the HTTP surface, not a shortcut around it.
func recordEvidenceDirectly(t *testing.T, ctx context.Context, uow application.UnitOfWork, rec peos.Recorder, now time.Time, evidenceID, locator string) {
	t.Helper()
	err := uow.Do(ctx, func(r application.Repositories) error {
		artEnv, revEnv, err := rec.RecordEvidence(engineering.EvidenceInput{
			ArtifactID: evidenceID, RevisionID: evidenceID + "-REV-1", Locator: locator, RecordedAt: now,
		})
		if err != nil {
			return err
		}
		if err := r.Artifacts.Put(ctx, artEnv); err != nil {
			return err
		}
		return r.Revisions.Put(ctx, revEnv)
	})
	if err != nil {
		t.Fatalf("recording decision evidence directly: %v", err)
	}
}

// runActivityViaHTTP POSTs a validation run and its claim, mirroring
// internal/scenario.Run's runActivity.
func runActivityViaHTTP(t *testing.T, handler http.Handler, tick func(), activityKey, requirementID, method, executionID, evidenceID, claimID, reasoning string) {
	t.Helper()
	mustPost(t, handler, "/api/v1/validation/runs", map[string]any{
		"execution_id": executionID, "plan_artifact_id": scenario.PlanArtifactID, "plan_revision_id": scenario.PlanRevisionID,
		"activity_key": activityKey, "subject_artifact_id": scenario.CapabilityArtifactID, "subject_revision_id": scenario.CapabilityRevision2,
		"method": method, "outcome": "completed",
		"evidence_artifact_id": evidenceID, "evidence_revision_id": evidenceID + "-REV-1", "evidence_locator": "https://evidence.example/" + evidenceID,
	})
	tick()
	mustPost(t, handler, "/api/v1/validation/claims", map[string]any{
		"claim_id": claimID, "scope_artifact_id": scenario.CapabilityArtifactID,
		"subject_artifact_id": scenario.CapabilityArtifactID, "subject_revision_id": scenario.CapabilityRevision2,
		"requirement_artifact_id": requirementID, "requirement_revision_id": requirementID + "-REV-1",
		"outcome": "satisfied", "method": method,
		"evidence_artifact_id": evidenceID, "evidence_revision_id": evidenceID + "-REV-1", "execution_id": executionID,
		"reasoning": reasoning,
	})
	tick()
}

// runScenarioThroughHTTP drives the FF-011 canonical "Homework after a
// lesson" scenario (internal/scenario) entirely through HTTP requests
// against a handler built on uow/rec/clock -- the same identities, order,
// and act-by-act timing internal/scenario.Run uses against the application
// layer directly. It is parameterised by adapter (FF-018 §16 step 10):
// TestCanonicalScenarioThroughHTTP calls it against memory,
// TestCanonicalScenarioThroughHTTPPostgres against PostgreSQL, and both
// then run the identical assertCanonicalEndStateThroughHTTP. This is
// FF-018 §16 step 9, "the single highest-value test in Phase A": proof
// that the transport preserves every engineering answer, not just that
// each endpoint decodes and responds.
//
// assertCanonicalEndState itself is unexported test code in another
// package and FF-018 does not authorize modifying internal/scenario, so
// its checks are reproduced in assertCanonicalEndStateThroughHTTP rather
// than imported -- through the HTTP query endpoints wherever Q1-Q7 expose
// the answer, and through a direct repository query on the shared uow
// only for the handful of facts (raw claim/correction fields, content
// digests, lifecycle transition history) no Phase A query endpoint
// surfaces.
func runScenarioThroughHTTP(t *testing.T, ctx context.Context, uow application.UnitOfWork, rec peos.Recorder, clock *application.FixedClock) http.Handler {
	t.Helper()
	deps := transporthttp.Dependencies{UOW: uow, Recorder: rec, Clock: clock}
	handler := transporthttp.NewHandler(deps)
	tick := func() { clock.Advance(time.Hour) }

	// 1. Project.
	mustPost(t, handler, "/api/v1/projects", map[string]any{
		"project_id": scenario.ProjectID, "name": "Belcanto Pilot",
	})
	tick()

	// 2. Feature card.
	mustPost(t, handler, "/api/v1/features", map[string]any{
		"feature_card_id": scenario.FeatureCardID, "project_id": scenario.ProjectID,
		"title": "Homework after a lesson", "description": "",
	})
	tick()

	// 3. Capability specification + Revision 1.
	mustPost(t, handler, "/api/v1/capabilities", map[string]any{
		"feature_card_id": scenario.FeatureCardID, "artifact_id": scenario.CapabilityArtifactID,
		"revision_id": scenario.CapabilityRevision1, "content": capabilityRevision1ContentJSON(),
	})
	tick()

	// 4. Accept Revision 1.
	mustPost(t, handler, "/api/v1/capabilities/"+scenario.CapabilityArtifactID+"/acceptances", map[string]any{
		"record_id": "ACC-1", "revision_id": scenario.CapabilityRevision1, "state": "accepted",
	})
	tick()

	// 5. Lifecycle entry: drafting.
	mustPost(t, handler, "/api/v1/capabilities/"+scenario.CapabilityArtifactID+"/lifecycle", map[string]any{
		"assignment_id": scenario.EntryAssignmentID, "state": "drafting", "is_entry": true,
		"transition_record_artifact_id": scenario.TransitionRecordArtifactID,
		"transition_record_revision_id": scenario.EntryTransitionRevisionID,
	})
	tick()

	// 6. Requirements REQ-1..REQ-4 (FF-011 §5). REQ-4 deliberately gets no
	// validation activity and no claim.
	requirementStatements := map[string]string{
		"REQ-1": "Published homework SHALL be visible to the student of the lesson it belongs to.",
		"REQ-2": "Published homework SHALL NOT be visible to any user who is not the student of that lesson.",
		"REQ-3": "Where homework has an audio attachment, that attachment SHALL have a representation the student can resolve.",
		"REQ-4": "Published homework SHALL become observable to the student within 5 seconds of publication.",
	}
	for _, artifactID := range scenario.RequirementArtifactIDs {
		mustPost(t, handler, "/api/v1/requirements", map[string]any{
			"artifact_id": artifactID, "revision_id": artifactID + "-REV-1",
			"statement": requirementStatements[artifactID], "subject_artifact_id": scenario.CapabilityArtifactID,
		})
		tick()
	}

	// 7. Decision evidence -- no HTTP command exists for this act; see
	// recordEvidenceDirectly's doc comment.
	recordEvidenceDirectly(t, ctx, uow, rec, clock.Now(), scenario.DecisionEvidenceID, "https://evidence.example/"+scenario.DecisionEvidenceID)
	tick()

	// 8. The decision, resolving Revision 1's open questions.
	mustPost(t, handler, "/api/v1/decisions", map[string]any{
		"decision_id": scenario.DecisionID, "subject_artifact_id": scenario.CapabilityArtifactID,
		"subject_revision_id": scenario.CapabilityRevision1,
		"question":            "Should homework support an optional audio attachment, and what publication latency is acceptable?",
		"outcome_statement":   "Homework supports at most one optional audio attachment, stored outside the capability record and retained as a content-addressed representation reference; publication must be observable to the student within 5 seconds.",
		"alternatives": []string{
			"Store audio inline in the capability record.",
			"Store audio externally and retain a content-addressed representation reference.",
			"Defer audio entirely.",
		},
		"evidence_artifact_id": scenario.DecisionEvidenceID, "evidence_revision_id": scenario.DecisionEvidenceID + "-REV-1",
		"assumptions":   []string{"Audio files are hosted by an existing media service."},
		"constraints":   []string{"No binary storage in the first release."},
		"uncertainties": []string{"Interview sample was 4 teachers."},
		"rationale":     "Referencing by content address avoids introducing binary storage into the first release; 5 seconds is the longest delay the pilot teachers described as acceptable.",
	})
	tick()

	// 9. Capability Revision 2, then accept it.
	mustPost(t, handler, "/api/v1/capabilities/"+scenario.CapabilityArtifactID+"/revisions", map[string]any{
		"revision_id": scenario.CapabilityRevision2, "content": capabilityRevision2ContentJSON(),
	})
	tick()
	mustPost(t, handler, "/api/v1/capabilities/"+scenario.CapabilityArtifactID+"/acceptances", map[string]any{
		"record_id": "ACC-2", "revision_id": scenario.CapabilityRevision2, "state": "accepted",
	})
	tick()

	// 10. Validation plan: A-1 (REQ-1), A-2 (REQ-2), A-3 (REQ-3); REQ-4 has none.
	mustPost(t, handler, "/api/v1/validation/plans", map[string]any{
		"artifact_id": scenario.PlanArtifactID, "revision_id": scenario.PlanRevisionID,
		"scope_artifact_id": scenario.CapabilityArtifactID,
		"activities": []map[string]any{
			{
				"key": "A-1", "subject_artifact_id": scenario.CapabilityArtifactID, "subject_revision_id": scenario.CapabilityRevision2,
				"method": "manual-review", "outcome_interpretation": "Satisfied when the reviewer confirms student visibility is specified and traceable.",
				"requirement_artifact_id": "REQ-1", "requirement_revision_id": "REQ-1-REV-1",
				"expected_evidence": []string{"Reviewer note confirming student visibility is specified"},
			},
			{
				"key": "A-2", "subject_artifact_id": scenario.CapabilityArtifactID, "subject_revision_id": scenario.CapabilityRevision2,
				"method": "manual-review", "outcome_interpretation": "Satisfied when the reviewer confirms non-student access is excluded.",
				"requirement_artifact_id": "REQ-2", "requirement_revision_id": "REQ-2-REV-1",
				"expected_evidence": []string{"Reviewer note confirming non-student access is excluded"},
			},
			{
				"key": "A-3", "subject_artifact_id": scenario.CapabilityArtifactID, "subject_revision_id": scenario.CapabilityRevision2,
				"method": "manual-inspection", "outcome_interpretation": "Satisfied when the inspector confirms the attachment representation is specified as resolvable.",
				"requirement_artifact_id": "REQ-3", "requirement_revision_id": "REQ-3-REV-1",
				"expected_evidence": []string{"Inspection note confirming the attachment representation is resolvable"},
			},
		},
	})
	tick()

	// 11. Lifecycle: begin validation.
	mustPost(t, handler, "/api/v1/capabilities/"+scenario.CapabilityArtifactID+"/lifecycle", map[string]any{
		"assignment_id": scenario.FirstAssignmentID, "state": "under-validation",
		"transition_record_artifact_id": scenario.TransitionRecordArtifactID,
		"transition_record_revision_id": scenario.FirstTransitionRevisionID,
		"transition_key":                "begin-validation", "from_assignment_id": scenario.EntryAssignmentID,
	})
	tick()

	// 12. Execute activities A-1, A-2, A-3, each producing evidence and a
	// satisfied claim.
	runActivityViaHTTP(t, handler, tick, "A-1", "REQ-1", "manual-review", "ER-1", "EV-1", scenario.ClaimForR1, "The specification states student visibility explicitly.")
	runActivityViaHTTP(t, handler, tick, "A-2", "REQ-2", "manual-review", "ER-2", "EV-2", scenario.ClaimIncorrect, "The specification states who may view homework.")
	runActivityViaHTTP(t, handler, tick, "A-3", "REQ-3", "manual-inspection", "ER-3", "EV-3", scenario.ClaimForR3, "The specification names a resolvable representation for the attachment.")

	// 13. Re-run A-2 and correct CLM-2.
	mustPost(t, handler, "/api/v1/validation/runs", map[string]any{
		"execution_id": "ER-4", "plan_artifact_id": scenario.PlanArtifactID, "plan_revision_id": scenario.PlanRevisionID,
		"activity_key": "A-2", "subject_artifact_id": scenario.CapabilityArtifactID, "subject_revision_id": scenario.CapabilityRevision2,
		"method": "manual-review", "outcome": "completed",
		"evidence_artifact_id": "EV-4", "evidence_revision_id": "EV-4-REV-1", "evidence_locator": "https://evidence.example/EV-4",
	})
	tick()
	mustPost(t, handler, "/api/v1/validation/claims/corrections", map[string]any{
		"claim_id": scenario.ClaimCorrecting, "correction_target": scenario.ClaimIncorrect, "correction_kind": "correct",
		"scope_artifact_id":   scenario.CapabilityArtifactID,
		"subject_artifact_id": scenario.CapabilityArtifactID, "subject_revision_id": scenario.CapabilityRevision2,
		"requirement_artifact_id": "REQ-2", "requirement_revision_id": "REQ-2-REV-1",
		"outcome": "not-satisfied", "method": "manual-review",
		"evidence_artifact_id": "EV-4", "evidence_revision_id": "EV-4-REV-1", "execution_id": "ER-4",
		"reasoning": "Revision 2 specifies who may view homework but does not state that other users are excluded. The original review treated the positive statement as implying the exclusion. It does not.",
	})
	tick()

	return handler
}

// TestCanonicalScenarioThroughHTTP runs runScenarioThroughHTTP against a
// memory-backed handler (FF-018 §16 step 9).
func TestCanonicalScenarioThroughHTTP(t *testing.T) {
	ctx := context.Background()
	uow := memory.NewUnitOfWork(memory.NewStore())
	rec := peos.NewRecorder()
	clock := application.NewFixedClock(scenario.FixedStart)
	handler := runScenarioThroughHTTP(t, ctx, uow, rec, clock)
	assertCanonicalEndStateThroughHTTP(t, ctx, handler, uow, rec)
}

// assertCanonicalEndStateThroughHTTP is the HTTP-sourced counterpart of
// internal/scenario/scenario_test.go's assertCanonicalEndState: the same
// FF-011 §9 facts, asserted through Q1-Q7 wherever they are exposed there,
// and through a direct query on the shared uow only where no Phase A query
// endpoint carries the answer.
func assertCanonicalEndStateThroughHTTP(t *testing.T, ctx context.Context, handler http.Handler, uow application.UnitOfWork, rec peos.Recorder) {
	t.Helper()

	// 1. Project and feature card exist (Q1, Q2).
	var projects struct {
		Data struct {
			Projects []struct {
				ProjectID string `json:"project_id"`
			} `json:"projects"`
		} `json:"data"`
	}
	if rr := getJSON(t, handler, "/api/v1/projects", &projects); rr.Code != http.StatusOK {
		t.Fatalf("Q1: status = %d, want 200", rr.Code)
	}
	foundProject := false
	for _, p := range projects.Data.Projects {
		if p.ProjectID == scenario.ProjectID {
			foundProject = true
		}
	}
	if !foundProject {
		t.Errorf("Q1: project %s not found in %+v", scenario.ProjectID, projects.Data.Projects)
	}

	var features struct {
		Data struct {
			Features []struct {
				FeatureCardID        string `json:"feature_card_id"`
				CapabilityArtifactID string `json:"capability_artifact_id"`
			} `json:"features"`
		} `json:"data"`
	}
	if rr := getJSON(t, handler, "/api/v1/projects/"+scenario.ProjectID+"/features", &features); rr.Code != http.StatusOK {
		t.Fatalf("Q2: status = %d, want 200", rr.Code)
	}
	if len(features.Data.Features) != 1 || features.Data.Features[0].FeatureCardID != scenario.FeatureCardID {
		t.Fatalf("Q2: features = %+v, want exactly [%s]", features.Data.Features, scenario.FeatureCardID)
	}
	if features.Data.Features[0].CapabilityArtifactID != scenario.CapabilityArtifactID {
		t.Errorf("Q2: capability_artifact_id = %q, want %s", features.Data.Features[0].CapabilityArtifactID, scenario.CapabilityArtifactID)
	}

	// 2, 5, 6, 9, 10, 11. Engineering state (Q4): current revision,
	// effective requirements, applicable decisions, readiness (including
	// current-claim resolution per requirement), and lifecycle.
	var state struct {
		Data struct {
			CurrentRevision struct {
				Found    bool `json:"found"`
				Revision struct {
					ArtifactID string `json:"artifact_id"`
					RevisionID string `json:"revision_id"`
					Sequence   int    `json:"sequence"`
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
					ClaimID               string `json:"claim_id"`
					Outcome               string `json:"outcome"`
				} `json:"per_requirement"`
			} `json:"readiness"`
			Lifecycle struct {
				Found   bool   `json:"found"`
				StateID string `json:"state_id"`
			} `json:"lifecycle"`
		} `json:"data"`
	}
	if rr := getJSON(t, handler, "/api/v1/features/"+scenario.FeatureCardID+"/state", &state); rr.Code != http.StatusOK {
		t.Fatalf("Q4: status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}

	if !state.Data.CurrentRevision.Found || state.Data.CurrentRevision.Revision.RevisionID != scenario.CapabilityRevision2 {
		t.Errorf("Q4: current_revision = %+v, want found with %s", state.Data.CurrentRevision, scenario.CapabilityRevision2)
	}
	if state.Data.CurrentRevision.Revision.Sequence != 2 {
		t.Errorf("Q4: current_revision.sequence = %d, want 2", state.Data.CurrentRevision.Revision.Sequence)
	}

	if len(state.Data.EffectiveRequirements) != len(scenario.RequirementArtifactIDs) {
		t.Fatalf("Q4: effective_requirements = %v, want %d entries (proves AD-025 discovery ran through HTTP, including REQ-4)",
			state.Data.EffectiveRequirements, len(scenario.RequirementArtifactIDs))
	}

	sawDecision := false
	for _, d := range state.Data.ApplicableDecisions {
		if d.DecisionID == scenario.DecisionID {
			sawDecision = true
		}
	}
	if !sawDecision {
		t.Errorf("Q4: decision %s not found in applicable_decisions %+v", scenario.DecisionID, state.Data.ApplicableDecisions)
	}

	if state.Data.Readiness.Status != string(application.ReadinessNotReady) {
		t.Errorf("Q4: readiness.status = %q, want %q (REQ-2 not-satisfied, REQ-4 uncovered)", state.Data.Readiness.Status, application.ReadinessNotReady)
	}
	wantClaims := map[string]struct {
		hasClaim bool
		claimID  string
		outcome  string
	}{
		"REQ-1": {true, scenario.ClaimForR1, "peos:satisfied"},
		"REQ-2": {true, scenario.ClaimCorrecting, "peos:not-satisfied"}, // current-claim resolution selects the correction head
		"REQ-3": {true, scenario.ClaimForR3, "peos:satisfied"},
		"REQ-4": {false, "", ""}, // never validated
	}
	seen := map[string]bool{}
	for _, per := range state.Data.Readiness.PerRequirement {
		want, ok := wantClaims[per.RequirementArtifactID]
		if !ok {
			t.Errorf("Q4: unexpected requirement %s in per_requirement", per.RequirementArtifactID)
			continue
		}
		seen[per.RequirementArtifactID] = true
		if per.HasClaim != want.hasClaim {
			t.Errorf("Q4: %s has_claim = %v, want %v", per.RequirementArtifactID, per.HasClaim, want.hasClaim)
			continue
		}
		if want.hasClaim && (per.ClaimID != want.claimID || per.Outcome != want.outcome) {
			t.Errorf("Q4: %s claim = (%s, %s), want (%s, %s)", per.RequirementArtifactID, per.ClaimID, per.Outcome, want.claimID, want.outcome)
		}
	}
	for req := range wantClaims {
		if !seen[req] {
			t.Errorf("Q4: %s missing from per_requirement entirely", req)
		}
	}

	if !state.Data.Lifecycle.Found || state.Data.Lifecycle.StateID != "featureforge:under-validation" {
		t.Errorf("Q4: lifecycle = %+v, want found with featureforge:under-validation (REQ-4 unvalidated keeps assessment incomplete)", state.Data.Lifecycle)
	}

	// 12. Timeline links CLM-4's correction back to CLM-2 (Q5).
	var timeline struct {
		Data struct {
			Dated []struct {
				Kind      string `json:"kind"`
				Corrected string `json:"corrected"`
			} `json:"dated"`
		} `json:"data"`
	}
	if rr := getJSON(t, handler, "/api/v1/features/"+scenario.FeatureCardID+"/timeline", &timeline); rr.Code != http.StatusOK {
		t.Fatalf("Q5: status = %d, want 200", rr.Code)
	}
	sawCorrection := false
	for _, ev := range timeline.Data.Dated {
		if ev.Kind == "claim.corrected" && ev.Corrected == scenario.ClaimIncorrect {
			sawCorrection = true
		}
	}
	if !sawCorrection {
		t.Error("Q5: timeline does not link CLM-4's correction back to CLM-2")
	}

	// Q6, Q7: both capability revisions are individually readable and
	// carry a non-empty content digest.
	var revisions struct {
		Data struct {
			Revisions []struct {
				RevisionID string `json:"revision_id"`
			} `json:"revisions"`
			Current struct {
				Found bool `json:"found"`
			} `json:"current"`
		} `json:"data"`
	}
	if rr := getJSON(t, handler, "/api/v1/capabilities/"+scenario.CapabilityArtifactID+"/revisions", &revisions); rr.Code != http.StatusOK {
		t.Fatalf("Q6: status = %d, want 200", rr.Code)
	}
	if len(revisions.Data.Revisions) != 2 || !revisions.Data.Current.Found {
		t.Errorf("Q6: revisions = %+v, current.found = %v; want 2 revisions and a found current", revisions.Data.Revisions, revisions.Data.Current.Found)
	}
	for _, revID := range []string{scenario.CapabilityRevision1, scenario.CapabilityRevision2} {
		var revision struct {
			Data struct {
				ArtifactID    string `json:"artifact_id"`
				RevisionID    string `json:"revision_id"`
				ContentDigest string `json:"content_digest"`
			} `json:"data"`
		}
		if rr := getJSON(t, handler, "/api/v1/capabilities/"+scenario.CapabilityArtifactID+"/revisions/"+revID, &revision); rr.Code != http.StatusOK {
			t.Fatalf("Q7 %s: status = %d, want 200", revID, rr.Code)
		}
		if revision.Data.RevisionID != revID || revision.Data.ContentDigest == "" {
			t.Errorf("Q7 %s: got %+v, want matching revision_id and a non-empty content_digest", revID, revision.Data)
		}
	}

	// The checks below have no Phase A query endpoint and go straight at
	// the shared uow, exactly as assertCanonicalEndState does.

	// 3, 4. Content is readable for both revisions and its digest matches
	// the revision's integrity value.
	for _, revID := range []string{scenario.CapabilityRevision1, scenario.CapabilityRevision2} {
		revKey := mustRevisionKeyHTTP(t, scenario.CapabilityArtifactID, revID)
		env := queryRepo(t, uow, func(r application.Repositories) (engineering.RevisionEnvelope, error) {
			e, found, err := r.Revisions.Get(ctx, revKey)
			if err != nil {
				return engineering.RevisionEnvelope{}, err
			}
			if !found {
				t.Fatalf("revision %s not found", revID)
			}
			return e, nil
		})
		content := queryRepo(t, uow, func(r application.Repositories) (engineering.CapabilitySpecificationContent, error) {
			c, found, err := r.StructuredContent.Get(ctx, revKey)
			if err != nil {
				return engineering.CapabilitySpecificationContent{}, err
			}
			if !found {
				t.Fatalf("content for %s not found", revID)
			}
			return c, nil
		})
		if err := rec.VerifyContentDigest(env, content); err != nil {
			t.Errorf("VerifyContentDigest(%s): %v", revID, err)
		}
	}

	// 7. Plan, executions, and evidence exist.
	planRevisions := queryRepo(t, uow, func(r application.Repositories) (int, error) {
		revs, err := r.Revisions.ListByArtifact(ctx, scenario.PlanArtifactID)
		return len(revs), err
	})
	if planRevisions != 1 {
		t.Errorf("plan revisions = %d, want 1", planRevisions)
	}
	for _, execID := range []string{"ER-1", "ER-2", "ER-3", "ER-4"} {
		found := queryRepo(t, uow, func(r application.Repositories) (bool, error) {
			key, err := engineering.NewRecordKey(engineering.RecordKindExecution, execID)
			if err != nil {
				return false, err
			}
			_, found, err := r.Records.Get(ctx, key)
			return found, err
		})
		if !found {
			t.Errorf("execution %s not found", execID)
		}
	}
	for _, evID := range []string{scenario.DecisionEvidenceID, "EV-1", "EV-2", "EV-3", "EV-4"} {
		found := queryRepo(t, uow, func(r application.Repositories) (bool, error) {
			revs, err := r.Revisions.ListByArtifact(ctx, evID)
			return len(revs) == 1, err
		})
		if !found {
			t.Errorf("evidence %s not found", evID)
		}
	}

	// 8. CLM-2 is unmutated; CLM-4 carries the correction reference.
	clm2 := queryRepo(t, uow, func(r application.Repositories) (engineering.RecordEnvelope, error) {
		key, err := engineering.NewRecordKey(engineering.RecordKindClaim, scenario.ClaimIncorrect)
		if err != nil {
			return engineering.RecordEnvelope{}, err
		}
		env, found, err := r.Records.Get(ctx, key)
		if err != nil {
			return engineering.RecordEnvelope{}, err
		}
		if !found {
			t.Fatal("CLM-2 not found")
		}
		return env, nil
	})
	if clm2.Outcome != "peos:satisfied" {
		t.Errorf("CLM-2 outcome = %q, want peos:satisfied (must remain unchanged)", clm2.Outcome)
	}
	clm4 := queryRepo(t, uow, func(r application.Repositories) (engineering.RecordEnvelope, error) {
		key, err := engineering.NewRecordKey(engineering.RecordKindClaim, scenario.ClaimCorrecting)
		if err != nil {
			return engineering.RecordEnvelope{}, err
		}
		env, found, err := r.Records.Get(ctx, key)
		if err != nil {
			return engineering.RecordEnvelope{}, err
		}
		if !found {
			t.Fatal("CLM-4 not found")
		}
		return env, nil
	})
	if !clm4.HasCorrection() || clm4.CorrectionTargetID != scenario.ClaimIncorrect || clm4.CorrectionKind != engineering.CorrectionKindCorrect {
		t.Errorf("CLM-4 correction = (has=%v target=%q kind=%q), want (true, %q, %q)",
			clm4.HasCorrection(), clm4.CorrectionTargetID, clm4.CorrectionKind, scenario.ClaimIncorrect, engineering.CorrectionKindCorrect)
	}

	// 14. Transition-record subject discovery (AD-026): both lifecycle
	// acts recorded through HTTP -- the entry assignment and the first
	// transition -- are revisions of RevisionFamilyTransitionRecord
	// projecting CAP-1 as their subject.
	transitionRecords := queryRepo(t, uow, func(r application.Repositories) ([]engineering.RevisionEnvelope, error) {
		return r.Revisions.ListByFamilyAndSubject(ctx, engineering.RevisionFamilyTransitionRecord, engineering.ArtifactSubjectKey(scenario.CapabilityArtifactID))
	})
	wantTransitionKeys := []string{
		scenario.TransitionRecordArtifactID + "/" + scenario.EntryTransitionRevisionID,
		scenario.TransitionRecordArtifactID + "/" + scenario.FirstTransitionRevisionID,
	}
	if len(transitionRecords) != len(wantTransitionKeys) {
		t.Fatalf("transition-record revisions = %v, want exactly %v", transitionRecords, wantTransitionKeys)
	}
	for i, want := range wantTransitionKeys {
		if transitionRecords[i].Key.String() != want {
			t.Errorf("transition record index %d: key = %s, want %s", i, transitionRecords[i].Key.String(), want)
		}
	}
}

func mustRevisionKeyHTTP(t *testing.T, artifactID, revisionID string) engineering.RevisionKey {
	t.Helper()
	key, err := engineering.NewRevisionKey(artifactID, revisionID)
	if err != nil {
		t.Fatal(err)
	}
	return key
}
