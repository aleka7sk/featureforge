package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/aleka7sk/featureforge/internal/application"
)

// envelope is the FF-018 §10.1 response shape: data is always present,
// rationale is omitted when absent -- never rendered as an empty object.
type envelope struct {
	Data      any `json:"data"`
	Rationale any `json:"rationale,omitempty"`
}

// errorResponse is the FF-018 §8.3 error body shape.
type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// writeJSON writes a success envelope. rationale is typed any so a caller
// with none can pass nil, which json:",omitempty" then drops entirely
// (FF-018 §10.1) rather than rendering an empty object.
func writeJSON(w http.ResponseWriter, status int, data, rationale any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{Data: data, Rationale: rationale})
}

// writeError writes the FF-018 §8.3 error body directly, for
// transport-originated failures that never reach the application layer
// (decode errors, unknown routes, unsupported methods -- FF-018 §12.2).
func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{Error: errorBody{Code: code, Message: message}})
}

// errorMapping is one row of the FF-018 §8.1 table: an application
// sentinel, its HTTP status, its stable machine code, and whether the
// error's own message may reach the client.
type errorMapping struct {
	err           error
	status        int
	code          string
	exposeMessage bool
}

// errorMappings is the FF-018 §8.1 table, in the order given there. This
// directly inherits M.4's most expensive lesson -- translation belongs
// where the decision is made, not where the error is raised (M.4 review
// §2.2) -- so every handler funnels its error here through one function
// rather than constructing a status itself.
var errorMappings = []errorMapping{
	{application.ErrInvalidCommand, http.StatusBadRequest, "invalid_command", true},
	{application.ErrNotFound, http.StatusNotFound, "not_found", true},
	{application.ErrImmutableValueConflict, http.StatusConflict, "immutable_value_conflict", true},
	{application.ErrCapabilityAlreadyLinked, http.StatusConflict, "capability_already_linked", true},
	{application.ErrRevisionSequenceConflict, http.StatusConflict, "revision_sequence_conflict", true},
	{application.ErrRevisionSequenceInvalid, http.StatusConflict, "revision_sequence_invalid", true},
	{application.ErrCurrentRevisionAmbiguous, http.StatusConflict, "current_revision_ambiguous", true},
	{application.ErrRevisionOrderMissing, http.StatusConflict, "revision_order_missing", true},
	{application.ErrRevisionReferenceMismatch, http.StatusConflict, "revision_reference_mismatch", true},
	{application.ErrNoAcceptedRevision, http.StatusConflict, "no_accepted_revision", true},
	{application.ErrEngineeringStateIndeterminate, http.StatusConflict, "engineering_state_indeterminate", true},
	{application.ErrAmbiguousLifecycleState, http.StatusConflict, "ambiguous_lifecycle_state", true},
	{application.ErrTimelineSourceInvalid, http.StatusConflict, "timeline_source_invalid", true},
	{application.ErrCorrectionAmbiguous, http.StatusConflict, "correction_ambiguous", true},
	{application.ErrValidationPlanAmbiguous, http.StatusConflict, "validation_plan_ambiguous", true},
	{application.ErrReferencedValueMissing, http.StatusUnprocessableEntity, "referenced_value_missing", true},
	{application.ErrAcceptanceTransitionInvalid, http.StatusUnprocessableEntity, "acceptance_transition_invalid", true},
	{application.ErrCorrectionTargetMissing, http.StatusUnprocessableEntity, "correction_target_missing", true},
	{application.ErrCorrectionFamilyMismatch, http.StatusUnprocessableEntity, "correction_family_mismatch", true},
	{application.ErrCorrectionSelfReference, http.StatusUnprocessableEntity, "correction_self_reference", true},
	{application.ErrCorrectionCycle, http.StatusUnprocessableEntity, "correction_cycle", true},
	{application.ErrUnknownDefinitionVersion, http.StatusUnprocessableEntity, "unknown_definition_version", true},
	{application.ErrNestedTransaction, http.StatusInternalServerError, "internal_error", false},
	{application.ErrTransactionAborted, http.StatusServiceUnavailable, "transaction_aborted", true},
}

const internalErrorMessage = "an unexpected error occurred"

// statusFor maps err to a status, a stable machine code, and whether the
// error's own message may be exposed. It walks errorMappings with
// errors.Is, so a wrapped sentinel (every application error is wrapped
// with fmt.Errorf("%w: ...", ...)) still matches. Anything unmatched is
// 500/internal_error/opaque (FF-018 §8.1, §8.3).
func statusFor(err error) (status int, code string, exposeMessage bool) {
	for _, m := range errorMappings {
		if errors.Is(err, m.err) {
			return m.status, m.code, m.exposeMessage
		}
	}
	return http.StatusInternalServerError, "internal_error", false
}

// writeAppError maps err via statusFor and writes the resulting error
// body, the one path every handler uses for an error returned from the
// application layer (FF-018 §4 step 6). Two conditions also need
// server-side visibility beyond the client-facing body, which is
// deliberately opaque for the 500 case (FF-018 §8.3): an unmapped error is
// logged at error level so the mapping table's opacity does not also
// discard the only record of what happened, and ErrTransactionAborted is
// logged at warn with a Retry-After header, closing the M.4 review's E6
// gap at the only layer that can act on it (FF-018 §14.3).
func writeAppError(w http.ResponseWriter, r *http.Request, deps Dependencies, err error) {
	status, code, exposeMessage := statusFor(err)
	switch status {
	case http.StatusInternalServerError:
		deps.logger().Error("unmapped application error", "method", r.Method, "path", r.URL.Path, "error", err)
	case http.StatusServiceUnavailable:
		deps.logger().Warn("transaction retries exhausted", "method", r.Method, "path", r.URL.Path, "error", err)
		w.Header().Set("Retry-After", "1")
	}
	message := internalErrorMessage
	if exposeMessage {
		message = err.Error()
	}
	writeError(w, status, code, message)
}
