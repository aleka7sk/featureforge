package ui

import (
	"context"
	"maps"
	"net/http"
	"net/url"
	"strings"
)

// Every handler below performs exactly AD-028's steps: parse form syntax,
// build the API's exact JSON request shape, invoke the existing API
// handler in-process through callAPI, and interpret the real response --
// 303 on success, the originating screen re-rendered with input preserved
// on a correctable failure (FF-021 §2, §12). None calls
// internal/application, internal/engineering, a repository, or
// UnitOfWork; none re-implements the API's error-code table. Where a
// command needs an identifier the UI already holds (the capability
// artifact ID, the current revision ID), it is resolved here from an
// existing GET query rather than re-typed by the user (FF-021 §5).

// capabilityContext is what several command forms need beyond what their
// own fields carry: the feature's capability artifact ID and its current
// revision ID, both already known to the UI from Q3 -- resolved once here
// so no form asks the user to retype an identifier the UI can already
// retain.
type capabilityContext struct {
	ArtifactID        string
	CurrentRevisionID string
}

func loadCapabilityContext(ctx context.Context, deps Dependencies, featureCardID string) (capabilityContext, *pageProblem) {
	result, err := callAPI(ctx, deps.API, http.MethodGet, "/api/v1/features/"+url.PathEscape(featureCardID), nil)
	if err != nil {
		return capabilityContext{}, &pageProblem{internal: true}
	}
	if !result.OK {
		return capabilityContext{}, &pageProblem{api: result}
	}
	var body struct {
		Feature apiFeatureCardDTO      `json:"feature"`
		State   apiEngineeringStateDTO `json:"state"`
	}
	if err := decodeInto(result, &body); err != nil {
		return capabilityContext{}, &pageProblem{internal: true}
	}
	cc := capabilityContext{ArtifactID: body.Feature.CapabilityArtifactID}
	if body.State.CurrentRevision.Found && body.State.CurrentRevision.Revision != nil {
		cc.CurrentRevisionID = body.State.CurrentRevision.Revision.RevisionID
	}
	return cc, nil
}

// contentFormFields is the field set C3 (establish) and C4 (revise) share,
// both building the same content JSON shape.
type contentFormFields struct {
	title, problemStatement, userOutcome                                               string
	functionalBehaviours, constraints, acceptanceCriteria, dependencies, openQuestions string
}

func readContentFormFields(r *http.Request) contentFormFields {
	return contentFormFields{
		title: r.FormValue("title"), problemStatement: r.FormValue("problem_statement"), userOutcome: r.FormValue("user_outcome"),
		functionalBehaviours: r.FormValue("functional_behaviours"), constraints: r.FormValue("constraints"),
		acceptanceCriteria: r.FormValue("acceptance_criteria"), dependencies: r.FormValue("dependencies"), openQuestions: r.FormValue("open_questions"),
	}
}

// contentJSON builds C3/C4's "content" object. acceptance_criteria uses
// this package's "key: text" per-line convention; a line without a colon
// is dropped -- a syntax decision, not a domain one, since the command
// itself validates every criterion's key and text (FF-021 §2: "parse only
// form syntax").
func (f contentFormFields) contentJSON() map[string]any {
	criteria := make([]map[string]any, 0)
	for _, line := range splitLines(f.acceptanceCriteria) {
		key, text, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		criteria = append(criteria, map[string]any{"key": strings.TrimSpace(key), "text": strings.TrimSpace(text)})
	}
	return map[string]any{
		"schema_version": 1, "title": f.title, "problem_statement": f.problemStatement, "user_outcome": f.userOutcome,
		"functional_behaviours": splitLines(f.functionalBehaviours), "constraints": splitLines(f.constraints),
		"acceptance_criteria": criteria, "dependencies": splitLines(f.dependencies), "open_questions": splitLines(f.openQuestions),
	}
}

func (f contentFormFields) values() map[string]string {
	return map[string]string{
		"title": f.title, "problem_statement": f.problemStatement, "user_outcome": f.userOutcome,
		"functional_behaviours": f.functionalBehaviours, "constraints": f.constraints,
		"acceptance_criteria": f.acceptanceCriteria, "dependencies": f.dependencies, "open_questions": f.openQuestions,
	}
}

func mergeValues(sets ...map[string]string) map[string]string {
	merged := make(map[string]string)
	for _, set := range sets {
		maps.Copy(merged, set)
	}
	return merged
}

// --- C1 CreateProject ---

func handleCreateProject(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !parseForm(w, r) {
			return
		}
		projectID, name := r.FormValue("project_id"), r.FormValue("name")
		result, err := callAPI(r.Context(), deps.API, http.MethodPost, "/api/v1/projects", map[string]any{
			"project_id": projectID, "name": name,
		})
		if err != nil {
			writeInternalErrorPage(w)
			return
		}
		if result.OK {
			redirectAfterCommand(w, r, "/projects/"+url.PathEscape(projectID))
			return
		}
		data, problem := loadProjectsPageData(r.Context(), deps)
		if problem != nil {
			problem.write(w)
			return
		}
		data.FormError = result.ErrMsg
		data.FormValues = formValues(r, "project_id", "name")
		render(w, http.StatusUnprocessableEntity, "projects", data)
	}
}

// --- C2 CreateFeature ---

func handleCreateFeature(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID := r.PathValue("projectID")
		if !parseForm(w, r) {
			return
		}
		featureCardID, title, description := r.FormValue("feature_card_id"), r.FormValue("title"), r.FormValue("description")
		result, err := callAPI(r.Context(), deps.API, http.MethodPost, "/api/v1/features", map[string]any{
			"feature_card_id": featureCardID, "project_id": projectID, "title": title, "description": description,
		})
		if err != nil {
			writeInternalErrorPage(w)
			return
		}
		if result.OK {
			redirectAfterCommand(w, r, "/features/"+url.PathEscape(featureCardID))
			return
		}
		data, found, problem := loadProjectDetailPageData(r.Context(), deps, projectID)
		if problem != nil {
			problem.write(w)
			return
		}
		if !found {
			notFoundPage(w, r)
			return
		}
		data.FormError = result.ErrMsg
		data.FormValues = formValues(r, "feature_card_id", "title", "description")
		render(w, http.StatusUnprocessableEntity, "project_detail", data)
	}
}

// --- C3 EstablishCapabilitySpecification ---

func handleEstablishCapability(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		featureCardID := r.PathValue("featureCardID")
		if !parseForm(w, r) {
			return
		}
		artifactID, revisionID := r.FormValue("artifact_id"), r.FormValue("revision_id")
		content := readContentFormFields(r)
		result, err := callAPI(r.Context(), deps.API, http.MethodPost, "/api/v1/capabilities", map[string]any{
			"feature_card_id": featureCardID, "artifact_id": artifactID, "revision_id": revisionID, "content": content.contentJSON(),
		})
		if err != nil {
			writeInternalErrorPage(w)
			return
		}
		if result.OK {
			redirectAfterCommand(w, r, "/features/"+url.PathEscape(featureCardID))
			return
		}
		data, problem := loadFeatureOverviewPageData(r.Context(), deps, featureCardID)
		if problem != nil {
			problem.write(w)
			return
		}
		data.FormError = result.ErrMsg
		data.FormValues = mergeValues(map[string]string{"artifact_id": artifactID, "revision_id": revisionID}, content.values())
		render(w, http.StatusUnprocessableEntity, "feature_overview", data)
	}
}

// --- C4 ReviseCapabilitySpecification ---

func handleReviseCapability(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		featureCardID := r.PathValue("featureCardID")
		if !parseForm(w, r) {
			return
		}
		revisionID := r.FormValue("revision_id")
		content := readContentFormFields(r)

		cc, problem := loadCapabilityContext(r.Context(), deps, featureCardID)
		if problem != nil {
			problem.write(w)
			return
		}
		result, err := callAPI(r.Context(), deps.API, http.MethodPost, "/api/v1/capabilities/"+url.PathEscape(cc.ArtifactID)+"/revisions", map[string]any{
			"revision_id": revisionID, "content": content.contentJSON(),
		})
		if err != nil {
			writeInternalErrorPage(w)
			return
		}
		if result.OK {
			redirectAfterCommand(w, r, "/features/"+url.PathEscape(featureCardID)+"/revisions")
			return
		}
		data, problem := loadRevisionsPageData(r.Context(), deps, featureCardID)
		if problem != nil {
			problem.write(w)
			return
		}
		data.FormError = result.ErrMsg
		data.FormValues = mergeValues(map[string]string{"revision_id": revisionID}, content.values())
		render(w, http.StatusUnprocessableEntity, "revisions", data)
	}
}

// --- C5 AcceptCapabilityRevision ---

func handleAcceptRevision(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		featureCardID, revisionID := r.PathValue("featureCardID"), r.PathValue("revisionID")
		if !parseForm(w, r) {
			return
		}
		recordID, state, reason := r.FormValue("record_id"), r.FormValue("state"), r.FormValue("reason")

		cc, problem := loadCapabilityContext(r.Context(), deps, featureCardID)
		if problem != nil {
			problem.write(w)
			return
		}
		result, err := callAPI(r.Context(), deps.API, http.MethodPost, "/api/v1/capabilities/"+url.PathEscape(cc.ArtifactID)+"/acceptances", map[string]any{
			"record_id": recordID, "revision_id": revisionID, "state": state, "reason": reason,
		})
		if err != nil {
			writeInternalErrorPage(w)
			return
		}
		if result.OK {
			redirectAfterCommand(w, r, "/features/"+url.PathEscape(featureCardID)+"/revisions")
			return
		}
		data, problem := loadRevisionsPageData(r.Context(), deps, featureCardID)
		if problem != nil {
			problem.write(w)
			return
		}
		data.FormError = result.ErrMsg
		render(w, http.StatusUnprocessableEntity, "revisions", data)
	}
}

// --- C6 AssignLifecycleState ---

func handleAssignLifecycle(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		featureCardID := r.PathValue("featureCardID")
		if !parseForm(w, r) {
			return
		}
		assignmentID, state := r.FormValue("assignment_id"), r.FormValue("state")
		isEntry := r.FormValue("is_entry") == "true"
		transitionRecordArtifactID, transitionRecordRevisionID := r.FormValue("transition_record_artifact_id"), r.FormValue("transition_record_revision_id")
		transitionKey, fromAssignmentID := r.FormValue("transition_key"), r.FormValue("from_assignment_id")

		cc, problem := loadCapabilityContext(r.Context(), deps, featureCardID)
		if problem != nil {
			problem.write(w)
			return
		}
		result, err := callAPI(r.Context(), deps.API, http.MethodPost, "/api/v1/capabilities/"+url.PathEscape(cc.ArtifactID)+"/lifecycle", map[string]any{
			"assignment_id": assignmentID, "state": state, "is_entry": isEntry,
			"transition_record_artifact_id": transitionRecordArtifactID, "transition_record_revision_id": transitionRecordRevisionID,
			"transition_key": transitionKey, "from_assignment_id": fromAssignmentID,
		})
		if err != nil {
			writeInternalErrorPage(w)
			return
		}
		if result.OK {
			redirectAfterCommand(w, r, "/features/"+url.PathEscape(featureCardID))
			return
		}
		data, problem := loadFeatureOverviewPageData(r.Context(), deps, featureCardID)
		if problem != nil {
			problem.write(w)
			return
		}
		data.FormError = result.ErrMsg
		data.FormValues = formValues(r, "assignment_id", "state", "is_entry", "transition_record_artifact_id", "transition_record_revision_id", "transition_key", "from_assignment_id")
		render(w, http.StatusUnprocessableEntity, "feature_overview", data)
	}
}

// --- C7 EstablishRequirement ---

func handleEstablishRequirement(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		featureCardID := r.PathValue("featureCardID")
		if !parseForm(w, r) {
			return
		}
		artifactID, revisionID, statement := r.FormValue("artifact_id"), r.FormValue("revision_id"), r.FormValue("statement")
		acceptanceRecordID := r.FormValue("acceptance_record_id")

		cc, problem := loadCapabilityContext(r.Context(), deps, featureCardID)
		if problem != nil {
			problem.write(w)
			return
		}
		result, err := callAPI(r.Context(), deps.API, http.MethodPost, "/api/v1/requirements", map[string]any{
			"artifact_id": artifactID, "revision_id": revisionID, "statement": statement, "subject_artifact_id": cc.ArtifactID,
			"acceptance_record_id": acceptanceRecordID,
		})
		if err != nil {
			writeInternalErrorPage(w)
			return
		}
		if result.OK {
			redirectAfterCommand(w, r, "/features/"+url.PathEscape(featureCardID)+"/requirements")
			return
		}
		data, problem := loadRequirementsPageData(r.Context(), deps, featureCardID)
		if problem != nil {
			problem.write(w)
			return
		}
		data.FormError = result.ErrMsg
		data.FormValues = formValues(r, "artifact_id", "revision_id", "acceptance_record_id", "statement")
		render(w, http.StatusUnprocessableEntity, "requirements", data)
	}
}

// --- C8 RecordArchitectureDecision ---

func handleRecordDecision(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		featureCardID := r.PathValue("featureCardID")
		if !parseForm(w, r) {
			return
		}
		decisionID, question, outcomeStatement := r.FormValue("decision_id"), r.FormValue("question"), r.FormValue("outcome_statement")
		subjectRevisionID := strings.TrimSpace(r.FormValue("subject_revision_id"))
		alternatives := r.FormValue("alternatives")
		evidenceArtifactID, evidenceRevisionID := r.FormValue("evidence_artifact_id"), r.FormValue("evidence_revision_id")
		assumptions, constraints, uncertainties, rationale := r.FormValue("assumptions"), r.FormValue("constraints"), r.FormValue("uncertainties"), r.FormValue("rationale")

		cc, problem := loadCapabilityContext(r.Context(), deps, featureCardID)
		if problem != nil {
			problem.write(w)
			return
		}
		if subjectRevisionID == "" {
			// Compatibility for forms rendered before the immutable revision
			// witness was added. New forms always carry the exact revision the
			// user was looking at so a later current revision cannot change a
			// replay's command semantics.
			subjectRevisionID = cc.CurrentRevisionID
		}
		result, err := callAPI(r.Context(), deps.API, http.MethodPost, "/api/v1/decisions", map[string]any{
			"decision_id": decisionID, "subject_artifact_id": cc.ArtifactID, "subject_revision_id": subjectRevisionID,
			"question": question, "outcome_statement": outcomeStatement, "alternatives": splitLines(alternatives),
			"evidence_artifact_id": evidenceArtifactID, "evidence_revision_id": evidenceRevisionID,
			"assumptions": splitLines(assumptions), "constraints": splitLines(constraints), "uncertainties": splitLines(uncertainties),
			"rationale": rationale,
		})
		if err != nil {
			writeInternalErrorPage(w)
			return
		}
		if result.OK {
			redirectAfterCommand(w, r, "/features/"+url.PathEscape(featureCardID)+"/decisions")
			return
		}
		data, problem := loadDecisionsPageData(r.Context(), deps, featureCardID)
		if problem != nil {
			problem.write(w)
			return
		}
		data.FormError = result.ErrMsg
		data.FormValues = formValues(r, "decision_id", "subject_revision_id", "question", "outcome_statement", "alternatives",
			"evidence_artifact_id", "evidence_revision_id", "assumptions", "constraints", "uncertainties", "rationale")
		render(w, http.StatusUnprocessableEntity, "decisions", data)
	}
}

// --- C9 EstablishValidationPlan ---

// parsePlanActivities decodes this screen's one-line-per-activity syntax,
// "key|method|outcome interpretation|requirement artifact ID|requirement
// revision ID|expected evidence,comma,separated" -- a static alternative
// to a dynamic add/remove control, which would require JavaScript
// (FF-021 §8). A malformed line (wrong field count) is dropped; the
// command itself validates every field it receives.
func parsePlanActivities(s, subjectArtifactID, subjectRevisionID string) []map[string]any {
	activities := make([]map[string]any, 0)
	for _, line := range splitLines(s) {
		fields := strings.Split(line, "|")
		if len(fields) != 6 {
			continue
		}
		var evidence []string
		for e := range strings.SplitSeq(fields[5], ",") {
			if e = strings.TrimSpace(e); e != "" {
				evidence = append(evidence, e)
			}
		}
		activities = append(activities, map[string]any{
			"key": strings.TrimSpace(fields[0]), "subject_artifact_id": subjectArtifactID, "subject_revision_id": subjectRevisionID,
			"method": strings.TrimSpace(fields[1]), "outcome_interpretation": strings.TrimSpace(fields[2]),
			"requirement_artifact_id": strings.TrimSpace(fields[3]), "requirement_revision_id": strings.TrimSpace(fields[4]),
			"expected_evidence": evidence,
		})
	}
	return activities
}

func handleEstablishPlan(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		featureCardID := r.PathValue("featureCardID")
		if !parseForm(w, r) {
			return
		}
		artifactID, revisionID, activitiesField := r.FormValue("artifact_id"), r.FormValue("revision_id"), r.FormValue("activities")
		acceptanceRecordID := r.FormValue("acceptance_record_id")

		cc, problem := loadCapabilityContext(r.Context(), deps, featureCardID)
		if problem != nil {
			problem.write(w)
			return
		}
		result, err := callAPI(r.Context(), deps.API, http.MethodPost, "/api/v1/validation/plans", map[string]any{
			"artifact_id": artifactID, "revision_id": revisionID, "scope_artifact_id": cc.ArtifactID,
			"acceptance_record_id": acceptanceRecordID,
			"activities":           parsePlanActivities(activitiesField, cc.ArtifactID, cc.CurrentRevisionID),
		})
		if err != nil {
			writeInternalErrorPage(w)
			return
		}
		if result.OK {
			redirectAfterCommand(w, r, "/features/"+url.PathEscape(featureCardID)+"/validation")
			return
		}
		data, problem := loadValidationPageData(r.Context(), deps, featureCardID)
		if problem != nil {
			problem.write(w)
			return
		}
		data.FormError = result.ErrMsg
		data.FormValues = formValues(r, "artifact_id", "revision_id", "acceptance_record_id", "activities")
		data.PlanFormFailed = true
		render(w, http.StatusUnprocessableEntity, "validation", data)
	}
}

// --- C10 RecordValidationRun ---

func handleRecordRun(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		featureCardID := r.PathValue("featureCardID")
		if !parseForm(w, r) {
			return
		}
		executionID, activityKey, method, outcome := r.FormValue("execution_id"), r.FormValue("activity_key"), r.FormValue("method"), r.FormValue("outcome")
		evidenceArtifactID, evidenceRevisionID, evidenceLocator := r.FormValue("evidence_artifact_id"), r.FormValue("evidence_revision_id"), r.FormValue("evidence_locator")

		state, problem := loadEngineeringState(r.Context(), deps, featureCardID)
		if problem != nil {
			problem.write(w)
			return
		}
		cc := capabilityContext{ArtifactID: capabilityIDFromState(state), CurrentRevisionID: ""}
		if state.CurrentRevision.Found && state.CurrentRevision.Revision != nil {
			cc.CurrentRevisionID = state.CurrentRevision.Revision.RevisionID
		}
		result, err := callAPI(r.Context(), deps.API, http.MethodPost, "/api/v1/validation/runs", map[string]any{
			"execution_id": executionID, "plan_artifact_id": state.ValidationPlan.ArtifactID, "plan_revision_id": state.ValidationPlan.RevisionID,
			"activity_key": activityKey, "subject_artifact_id": cc.ArtifactID, "subject_revision_id": cc.CurrentRevisionID,
			"method": method, "outcome": outcome,
			"evidence_artifact_id": evidenceArtifactID, "evidence_revision_id": evidenceRevisionID, "evidence_locator": evidenceLocator,
		})
		if err != nil {
			writeInternalErrorPage(w)
			return
		}
		if result.OK {
			redirectAfterCommand(w, r, "/features/"+url.PathEscape(featureCardID)+"/validation")
			return
		}
		data, problem := loadValidationPageData(r.Context(), deps, featureCardID)
		if problem != nil {
			problem.write(w)
			return
		}
		data.FormError = result.ErrMsg
		data.FormValues = formValues(r, "execution_id", "activity_key", "method", "outcome", "evidence_artifact_id", "evidence_revision_id", "evidence_locator")
		render(w, http.StatusUnprocessableEntity, "validation", data)
	}
}

// --- C11 RecordValidationClaim ---

func handleRecordClaim(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		featureCardID := r.PathValue("featureCardID")
		if !parseForm(w, r) {
			return
		}
		claimID := r.FormValue("claim_id")
		requirementArtifactID, requirementRevisionID := r.FormValue("requirement_artifact_id"), r.FormValue("requirement_revision_id")
		outcome, method := r.FormValue("outcome"), r.FormValue("method")
		evidenceArtifactID, evidenceRevisionID, executionID, reasoning := r.FormValue("evidence_artifact_id"), r.FormValue("evidence_revision_id"), r.FormValue("execution_id"), r.FormValue("reasoning")

		cc, problem := loadCapabilityContext(r.Context(), deps, featureCardID)
		if problem != nil {
			problem.write(w)
			return
		}
		result, err := callAPI(r.Context(), deps.API, http.MethodPost, "/api/v1/validation/claims", map[string]any{
			"claim_id": claimID, "scope_artifact_id": cc.ArtifactID,
			"subject_artifact_id": cc.ArtifactID, "subject_revision_id": cc.CurrentRevisionID,
			"requirement_artifact_id": requirementArtifactID, "requirement_revision_id": requirementRevisionID,
			"outcome": outcome, "method": method,
			"evidence_artifact_id": evidenceArtifactID, "evidence_revision_id": evidenceRevisionID,
			"execution_id": executionID, "reasoning": reasoning,
		})
		if err != nil {
			writeInternalErrorPage(w)
			return
		}
		if result.OK {
			redirectAfterCommand(w, r, "/features/"+url.PathEscape(featureCardID)+"/validation")
			return
		}
		data, problem := loadValidationPageData(r.Context(), deps, featureCardID)
		if problem != nil {
			problem.write(w)
			return
		}
		data.FormError = result.ErrMsg
		data.FormValues = formValues(r, "claim_id", "requirement_artifact_id", "requirement_revision_id", "outcome", "method",
			"evidence_artifact_id", "evidence_revision_id", "execution_id", "reasoning")
		render(w, http.StatusUnprocessableEntity, "validation", data)
	}
}

// --- C12 CorrectValidationClaim ---

func handleCorrectClaim(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		featureCardID := r.PathValue("featureCardID")
		if !parseForm(w, r) {
			return
		}
		claimID, correctionTarget, correctionKind := r.FormValue("claim_id"), r.FormValue("correction_target"), r.FormValue("correction_kind")
		requirementArtifactID, requirementRevisionID := r.FormValue("requirement_artifact_id"), r.FormValue("requirement_revision_id")
		outcome, method := r.FormValue("outcome"), r.FormValue("method")
		evidenceArtifactID, evidenceRevisionID, executionID, reasoning := r.FormValue("evidence_artifact_id"), r.FormValue("evidence_revision_id"), r.FormValue("execution_id"), r.FormValue("reasoning")

		cc, problem := loadCapabilityContext(r.Context(), deps, featureCardID)
		if problem != nil {
			problem.write(w)
			return
		}
		result, err := callAPI(r.Context(), deps.API, http.MethodPost, "/api/v1/validation/claims/corrections", map[string]any{
			"claim_id": claimID, "correction_target": correctionTarget, "correction_kind": correctionKind,
			"scope_artifact_id": cc.ArtifactID, "subject_artifact_id": cc.ArtifactID, "subject_revision_id": cc.CurrentRevisionID,
			"requirement_artifact_id": requirementArtifactID, "requirement_revision_id": requirementRevisionID,
			"outcome": outcome, "method": method,
			"evidence_artifact_id": evidenceArtifactID, "evidence_revision_id": evidenceRevisionID,
			"execution_id": executionID, "reasoning": reasoning,
		})
		if err != nil {
			writeInternalErrorPage(w)
			return
		}
		if result.OK {
			redirectAfterCommand(w, r, "/features/"+url.PathEscape(featureCardID)+"/validation")
			return
		}
		data, problem := loadValidationPageData(r.Context(), deps, featureCardID)
		if problem != nil {
			problem.write(w)
			return
		}
		data.FormError = result.ErrMsg
		// The correction form's own field names on the page are prefixed
		// ("correcting_claim_id", "correction_requirement_artifact_id", ...)
		// to avoid colliding with the claim form's identically-purposed
		// fields on the same page (templates/validation.html); re-key here.
		data.FormValues = map[string]string{
			"correcting_claim_id": claimID, "correction_target": correctionTarget, "correction_kind": correctionKind,
			"correction_requirement_artifact_id": requirementArtifactID, "correction_requirement_revision_id": requirementRevisionID,
			"correction_outcome": outcome, "correction_method": method,
			"correction_evidence_artifact_id": evidenceArtifactID, "correction_evidence_revision_id": evidenceRevisionID,
			"correction_execution_id": executionID, "correction_reasoning": reasoning,
		}
		render(w, http.StatusUnprocessableEntity, "validation", data)
	}
}
