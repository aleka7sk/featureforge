package ui

import "net/http"

// writeAPIErrorPage renders a standalone error page for a GET request that
// the API rejected (a POST's error instead re-renders its own form -- see
// handlers_form.go). result.ErrMsg is already client-safe: the API's own
// writeAppError (internal/transport/http/errors.go) only ever puts a
// sentinel-safe message in the response body -- an unmapped/internal error
// is already the generic "an unexpected error occurred" text by the time it
// reaches here, per FF-018 §8.3's exposeMessage table. This file trusts
// that boundary rather than re-deriving which codes are safe to show,
// which would duplicate FF-018's error-mapping table (FF-021 §2: "must not
// duplicate API error mapping").
// writeInternalErrorPage renders the generic error page for a failure in
// this package's own plumbing (callAPI transport failure, a response body
// that does not decode) -- distinct from writeAPIErrorPage, which renders
// the API's own status and message for a request the API itself rejected.
func writeInternalErrorPage(w http.ResponseWriter) {
	writeErrorPage(w, http.StatusInternalServerError, "Something went wrong", "An unexpected error occurred.")
}

func writeAPIErrorPage(w http.ResponseWriter, result apiResult) {
	heading := "Something went wrong"
	switch result.Status {
	case http.StatusNotFound:
		heading = "Not found"
	case http.StatusBadRequest, http.StatusUnprocessableEntity, http.StatusUnsupportedMediaType:
		heading = "Request could not be completed"
	case http.StatusConflict:
		heading = "This already exists with different content"
	case http.StatusServiceUnavailable:
		heading = "Temporarily unavailable"
	}
	writeErrorPage(w, result.Status, heading, result.ErrMsg)
}
