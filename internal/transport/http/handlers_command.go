package http

import (
	"net/http"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

// Every handler below performs exactly the six steps FF-018 §4 specifies:
// decode, transport-syntax-validate, map to application input, invoke
// exactly one application entry point, map the result, map any error
// centrally. None holds application.Repositories or calls UnitOfWork.Do.
// Every command's success status is 201 Created (FF-018 §3): each command
// creates an immutable record.

// handleCreateProject implements C1.
func handleCreateProject(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createProjectRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		result, err := (application.CreateProjectCommand{
			ProjectID: req.ProjectID, Name: req.Name,
		}).Execute(r.Context(), deps.UOW, deps.Clock)
		if err != nil {
			writeAppError(w, r, deps, err)
			return
		}
		writeJSON(w, http.StatusCreated, createProjectResponse{ProjectID: result.ProjectID.String()}, nil)
	}
}

// handleCreateFeature implements C2.
func handleCreateFeature(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createFeatureRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		result, err := (application.CreateFeatureCommand{
			FeatureCardID: req.FeatureCardID, ProjectID: req.ProjectID,
			Title: req.Title, Description: req.Description,
		}).Execute(r.Context(), deps.UOW, deps.Clock)
		if err != nil {
			writeAppError(w, r, deps, err)
			return
		}
		writeJSON(w, http.StatusCreated, createFeatureResponse{FeatureCardID: result.FeatureCardID.String()}, nil)
	}
}

// handleEstablishCapability implements C3.
func handleEstablishCapability(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req establishCapabilityRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		content, err := mapContentDTO(req.Content)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		result, err := (application.EstablishCapabilitySpecificationCommand{
			FeatureCardID: req.FeatureCardID, ArtifactID: req.ArtifactID, RevisionID: req.RevisionID,
			Content: content,
		}).Execute(r.Context(), deps.UOW, deps.Recorder, deps.Inspector, deps.Clock)
		if err != nil {
			writeAppError(w, r, deps, err)
			return
		}
		writeJSON(w, http.StatusCreated, establishCapabilityResponse{
			ArtifactID: result.ArtifactKey.String(), RevisionID: result.RevisionKey.RevisionID, Sequence: result.Sequence,
		}, nil)
	}
}

// handleReviseCapability implements C4. The {artifactID} path value is
// authoritative (FF-018 §3.1).
func handleReviseCapability(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		artifactID, ok := requirePathValue(w, r, "artifactID")
		if !ok {
			return
		}
		var req reviseCapabilityRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		content, err := mapContentDTO(req.Content)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		result, err := (application.ReviseCapabilitySpecificationCommand{
			ArtifactID: artifactID, RevisionID: req.RevisionID, Content: content,
		}).Execute(r.Context(), deps.UOW, deps.Recorder, deps.Inspector, deps.Clock)
		if err != nil {
			writeAppError(w, r, deps, err)
			return
		}
		writeJSON(w, http.StatusCreated, reviseCapabilityResponse{
			RevisionID: result.RevisionKey.RevisionID, Sequence: result.Sequence,
		}, nil)
	}
}

// handleAcceptRevision implements C5. The {artifactID} path value is
// authoritative (FF-018 §3.1).
func handleAcceptRevision(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		artifactID, ok := requirePathValue(w, r, "artifactID")
		if !ok {
			return
		}
		var req acceptRevisionRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		result, err := (application.AcceptCapabilityRevisionCommand{
			RecordID: req.RecordID, ArtifactID: artifactID, RevisionID: req.RevisionID,
			State: engineering.AcceptanceState(req.State), Reason: req.Reason,
			EffectiveAt: req.EffectiveAt.Value, HasEffectiveAt: req.EffectiveAt.Present,
		}).Execute(r.Context(), deps.UOW, deps.Inspector, deps.Clock)
		if err != nil {
			writeAppError(w, r, deps, err)
			return
		}
		writeJSON(w, http.StatusCreated, acceptRevisionResponse{RecordID: result.RecordID}, nil)
	}
}

// handleAssignLifecycle implements C6. The {artifactID} path value is
// authoritative and maps to SubjectArtifactID (FF-018 §3.1).
func handleAssignLifecycle(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		artifactID, ok := requirePathValue(w, r, "artifactID")
		if !ok {
			return
		}
		var req assignLifecycleRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		result, err := (application.AssignLifecycleStateCommand{
			AssignmentID: req.AssignmentID, SubjectArtifactID: artifactID,
			State: req.State, EffectiveAt: req.EffectiveAt.Value, HasEffectiveAt: req.EffectiveAt.Present,
			TransitionRecordArtifactID: req.TransitionRecordArtifactID, TransitionRecordRevisionID: req.TransitionRecordRevisionID,
			IsEntry: req.IsEntry, TransitionKey: req.TransitionKey, FromAssignmentID: req.FromAssignmentID,
			AttemptedAt: req.AttemptedAt.Value, HasAttemptedAt: req.AttemptedAt.Present,
			CompletedAt: req.CompletedAt.Value, HasCompletedAt: req.CompletedAt.Present,
		}).Execute(r.Context(), deps.UOW, deps.Recorder, deps.Inspector, deps.Clock)
		if err != nil {
			writeAppError(w, r, deps, err)
			return
		}
		writeJSON(w, http.StatusCreated, assignLifecycleResponse{
			TransitionRevisionKey: result.TransitionRevisionKey.String(), AssignmentKey: result.AssignmentKey.String(),
		}, nil)
	}
}

// handleEstablishRequirement implements C7.
func handleEstablishRequirement(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req establishRequirementRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		result, err := (application.EstablishRequirementCommand{
			ArtifactID: req.ArtifactID, RevisionID: req.RevisionID,
			Statement: req.Statement, SubjectArtifactID: req.SubjectArtifactID,
			SourceCapabilityRevisionID:   req.SourceCapabilityRevisionID,
			SourceAcceptanceCriterionKey: req.SourceAcceptanceCriterionKey,
			AcceptanceRecordID:           optionalStringPointer(req.AcceptanceRecordID),
		}).Execute(r.Context(), deps.UOW, deps.Recorder, deps.Inspector, deps.Clock)
		if err != nil {
			writeAppError(w, r, deps, err)
			return
		}
		writeJSON(w, http.StatusCreated, establishRequirementResponse{
			ArtifactID: result.ArtifactKey.String(), RevisionID: result.RevisionKey.RevisionID,
		}, nil)
	}
}

// handleRecordDecision implements C8.
func handleRecordDecision(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req recordDecisionRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		result, err := (application.RecordArchitectureDecisionCommand{
			DecisionID: req.DecisionID, SubjectArtifactID: req.SubjectArtifactID, SubjectRevisionID: req.SubjectRevisionID,
			Question: req.Question, OutcomeStatement: req.OutcomeStatement, Alternatives: req.Alternatives,
			EvidenceArtifactID: req.EvidenceArtifactID, EvidenceRevisionID: req.EvidenceRevisionID,
			Assumptions: req.Assumptions, Constraints: req.Constraints, Uncertainties: req.Uncertainties,
			Rationale: req.Rationale,
		}).Execute(r.Context(), deps.UOW, deps.Recorder, deps.Inspector, deps.Clock)
		if err != nil {
			writeAppError(w, r, deps, err)
			return
		}
		writeJSON(w, http.StatusCreated, recordDecisionResponse{DecisionKey: result.Key.String()}, nil)
	}
}

// handleEstablishPlan implements C9.
func handleEstablishPlan(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req establishPlanRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		result, err := (application.EstablishValidationPlanCommand{
			ArtifactID: req.ArtifactID, RevisionID: req.RevisionID, ScopeArtifactID: req.ScopeArtifactID,
			AcceptanceRecordID: optionalStringPointer(req.AcceptanceRecordID),
			Activities:         mapPlanActivities(req.Activities),
		}).Execute(r.Context(), deps.UOW, deps.Recorder, deps.Inspector, deps.Clock)
		if err != nil {
			writeAppError(w, r, deps, err)
			return
		}
		writeJSON(w, http.StatusCreated, establishPlanResponse{
			ArtifactID: result.ArtifactKey.String(), RevisionID: result.RevisionKey.RevisionID,
		}, nil)
	}
}

// handleRecordRun implements C10.
func handleRecordRun(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req recordRunRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		result, err := (application.RecordValidationRunCommand{
			ExecutionID: req.ExecutionID, PlanArtifactID: req.PlanArtifactID, PlanRevisionID: req.PlanRevisionID,
			ActivityKey: req.ActivityKey, SubjectArtifactID: req.SubjectArtifactID, SubjectRevisionID: req.SubjectRevisionID,
			Method: req.Method, Outcome: req.Outcome,
			CompletedAt: req.CompletedAt.Value, HasCompletedAt: req.CompletedAt.Present,
			EvidenceArtifactID: req.EvidenceArtifactID, EvidenceRevisionID: req.EvidenceRevisionID, EvidenceLocator: req.EvidenceLocator,
		}).Execute(r.Context(), deps.UOW, deps.Recorder, deps.Inspector, deps.Clock)
		if err != nil {
			writeAppError(w, r, deps, err)
			return
		}
		writeJSON(w, http.StatusCreated, recordRunResponse{
			EvidenceArtifactID: result.EvidenceArtifactKey.String(), EvidenceRevisionID: result.EvidenceRevisionKey.RevisionID,
			ExecutionKey: result.ExecutionKey.String(),
		}, nil)
	}
}

// handleRecordClaim implements C11.
func handleRecordClaim(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req recordClaimRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		result, err := (application.RecordValidationClaimCommand{
			ClaimID: req.ClaimID, ScopeArtifactID: req.ScopeArtifactID,
			SubjectArtifactID: req.SubjectArtifactID, SubjectRevisionID: req.SubjectRevisionID,
			RequirementArtifactID: req.RequirementArtifactID, RequirementRevisionID: req.RequirementRevisionID,
			Outcome: req.Outcome, Method: req.Method,
			EvidenceArtifactID: req.EvidenceArtifactID, EvidenceRevisionID: req.EvidenceRevisionID,
			ExecutionID: req.ExecutionID, Reasoning: req.Reasoning,
			Timestamp: req.Timestamp.Value, HasTimestamp: req.Timestamp.Present,
		}).Execute(r.Context(), deps.UOW, deps.Recorder, deps.Inspector, deps.Clock)
		if err != nil {
			writeAppError(w, r, deps, err)
			return
		}
		writeJSON(w, http.StatusCreated, recordClaimResponse{ClaimKey: result.Key.String()}, nil)
	}
}

// handleCorrectClaim implements C12.
func handleCorrectClaim(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req correctClaimRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		result, err := (application.CorrectValidationClaimCommand{
			ClaimID: req.ClaimID, CorrectionTarget: req.CorrectionTarget, CorrectionKind: req.CorrectionKind,
			ScopeArtifactID:   req.ScopeArtifactID,
			SubjectArtifactID: req.SubjectArtifactID, SubjectRevisionID: req.SubjectRevisionID,
			RequirementArtifactID: req.RequirementArtifactID, RequirementRevisionID: req.RequirementRevisionID,
			Outcome: req.Outcome, Method: req.Method,
			EvidenceArtifactID: req.EvidenceArtifactID, EvidenceRevisionID: req.EvidenceRevisionID,
			ExecutionID: req.ExecutionID, Reasoning: req.Reasoning,
			Timestamp: req.Timestamp.Value, HasTimestamp: req.Timestamp.Present,
		}).Execute(r.Context(), deps.UOW, deps.Recorder, deps.Inspector, deps.Clock)
		if err != nil {
			writeAppError(w, r, deps, err)
			return
		}
		writeJSON(w, http.StatusCreated, correctClaimResponse{ClaimKey: result.Key.String()}, nil)
	}
}
