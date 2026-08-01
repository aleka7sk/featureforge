package http

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
)

// maxRequestBody bounds decoded request bodies (FF-018 §9.2). FF-015
// specifies no limit; this is an implementation-level safety limit against
// an unbounded read, not a documented API contract.
const maxRequestBody = 1 << 20 // 1 MiB

// decodeJSON decodes exactly one JSON value from r's body into v,
// rejecting an unrecognised Content-Type, unknown fields, and a second
// value in the same body (FF-018 §9.2, §9.3). It reports whether decoding
// succeeded; on failure it has already written the 400/415 response, so
// the caller returns immediately without invoking the application layer
// (FF-018 §9.2: "not invoked after a decode failure").
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	return decodeJSONWithCode(w, r, v, "bad_request")
}

// decodeProposalJSON applies the same strict transport parsing as decodeJSON,
// but uses FF-024's invalid_command machine code for the reviewed-proposal
// command boundary.
func decodeProposalJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	return decodeJSONWithCode(w, r, v, "invalid_command")
}

func decodeJSONWithCode(w http.ResponseWriter, r *http.Request, v any, code string) bool {
	if ct := r.Header.Get("Content-Type"); ct != "" && !isJSONContentType(ct) {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type must be application/json")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, code, "malformed request body: "+err.Error())
		return false
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		writeError(w, http.StatusBadRequest, code, "request body must contain exactly one JSON value")
		return false
	}
	return true
}

// requireEmptyBody enforces FF-024's body-free generate request. Whitespace is
// still a body and is rejected rather than silently ignored.
func requireEmptyBody(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	data, err := io.ReadAll(r.Body)
	if err != nil || len(data) != 0 {
		writeError(w, http.StatusBadRequest, "invalid_command", "generate request body must be empty")
		return false
	}
	return true
}

func isJSONContentType(ct string) bool {
	mediaType, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return false
	}
	return mediaType == "application/json"
}

// requirePathValue reads a ServeMux path parameter, writing 400 and
// reporting failure if it is empty (FF-018 §12.2).
func requirePathValue(w http.ResponseWriter, r *http.Request, name string) (string, bool) {
	v := r.PathValue(name)
	if v == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "missing path parameter: "+name)
		return "", false
	}
	return v, true
}
