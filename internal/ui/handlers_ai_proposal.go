package ui

import (
	"encoding/json"
	"net/http"
	"net/url"
)

// handleGenerateAIProposal retains AD-028's boundary: it resolves the
// FeatureCard's API-owned capability identity, delegates generation to the
// API, and renders only the returned transient values. It has no generator
// dependency and no write path of its own.
func handleGenerateAIProposal(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !parseForm(w, r) {
			return
		}
		featureCardID := r.PathValue("featureCardID")
		capability, problem := loadCapabilityContext(r.Context(), deps, featureCardID)
		if problem != nil {
			problem.write(w)
			return
		}
		result, err := callAPI(
			r.Context(), deps.API, http.MethodPost,
			"/api/v1/capabilities/"+url.PathEscape(capability.ArtifactID)+"/ai-proposals",
			nil,
		)
		if err != nil {
			writeInternalErrorPage(w)
			return
		}
		if !result.OK {
			writeAPIErrorPage(w, result)
			return
		}
		var generated apiGenerateCapabilityProposalDTO
		if err := decodeInto(result, &generated); err != nil {
			writeInternalErrorPage(w)
			return
		}
		var contextPack apiProposalContextPackDTO
		if err := json.Unmarshal(generated.ContextPack, &contextPack); err != nil {
			writeInternalErrorPage(w)
			return
		}
		var proposal apiCapabilityProposalDTO
		if err := json.Unmarshal(generated.Proposal, &proposal); err != nil {
			writeInternalErrorPage(w)
			return
		}
		data := mapAIProposalPageData(featureCardID, capability.ArtifactID, contextPack, proposal, string(generated.Proposal))
		render(w, http.StatusOK, "ai_proposal", data)
	}
}

// handleAcceptAIProposal returns exactly the hidden canonical Proposal plus
// the caller-entered Revision ID to the API. On success it follows the
// existing POST/Redirect/GET convention back to revision history.
func handleAcceptAIProposal(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !parseForm(w, r) {
			return
		}
		featureCardID := r.PathValue("featureCardID")
		capability, problem := loadCapabilityContext(r.Context(), deps, featureCardID)
		if problem != nil {
			problem.write(w)
			return
		}
		canonicalProposal := []byte(r.FormValue("proposal"))
		proposalJSON := json.RawMessage(canonicalProposal)
		if !json.Valid(proposalJSON) {
			// Keep validation authority in the API: wrap malformed form text as
			// a valid JSON string so callAPI can delegate it and the API's
			// Proposal parser returns its governed invalid_command response.
			encoded, err := json.Marshal(string(canonicalProposal))
			if err != nil {
				writeInternalErrorPage(w)
				return
			}
			proposalJSON = encoded
		}
		result, err := callAPI(
			r.Context(), deps.API, http.MethodPost,
			"/api/v1/capabilities/"+url.PathEscape(capability.ArtifactID)+"/ai-proposals/accept",
			apiAcceptCapabilityProposalDTO{RevisionID: r.FormValue("revision_id"), Proposal: proposalJSON},
		)
		if err != nil {
			writeInternalErrorPage(w)
			return
		}
		if !result.OK {
			writeAPIErrorPage(w, result)
			return
		}
		redirectAfterCommand(w, r, "/features/"+url.PathEscape(featureCardID)+"/revisions")
	}
}
