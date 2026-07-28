package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
)

// This file is the entire AD-028 bridge (FF-021 §2). It holds no
// application logic and duplicates no error mapping: it constructs a real
// *http.Request, invokes the existing, unmodified API http.Handler through
// ServeHTTP -- no network socket -- and interprets the real status code and
// response body the API handler wrote. The API handler remains the single
// owner of JSON transport decoding, DTO mapping, application invocation,
// and centralized error mapping; this file recreates none of it. A change
// to the API's conflict semantics, error codes, or response shape is
// observed here automatically, because this is the same handler, not a
// second implementation of it.

// apiResult is the interpreted outcome of one in-process call to the API
// handler: the real HTTP status it wrote, and either the decoded success
// envelope's "data" (plus optional "rationale") payload or the decoded
// error body -- never both.
type apiResult struct {
	Status    int
	OK        bool
	Data      json.RawMessage
	Rationale json.RawMessage
	ErrCode   string
	ErrMsg    string
}

// responseCapture is a minimal http.ResponseWriter that records what the
// API handler writes, without a network socket and without importing
// net/http/httptest into production code.
type responseCapture struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newResponseCapture() *responseCapture {
	return &responseCapture{header: make(http.Header), status: http.StatusOK}
}

func (c *responseCapture) Header() http.Header { return c.header }

func (c *responseCapture) Write(b []byte) (int, error) { return c.body.Write(b) }

func (c *responseCapture) WriteHeader(status int) { c.status = status }

// callAPI invokes api in-process with method and path, encoding body (when
// non-nil) as the request's JSON payload exactly as a real client would,
// and interprets the response using the API's own envelope/error shapes
// (internal/transport/http's writeJSON/writeError: {"data":...} on 2xx,
// {"error":{"code":...,"message":...}} otherwise). ctx propagates
// cancellation from the originating browser request.
func callAPI(ctx context.Context, api http.Handler, method, path string, body any) (apiResult, error) {
	// http.NoBody, not a nil io.Reader: NewRequestWithContext leaves
	// req.Body genuinely nil for a nil reader, unlike a real server
	// request (always non-nil, even when empty). Any handler reading
	// r.Body unconditionally would panic on that difference; NoBody is
	// the standard library's own sentinel for exactly this case.
	reqBody := io.Reader(http.NoBody)
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return apiResult{}, err
		}
		reqBody = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, path, reqBody)
	if err != nil {
		return apiResult{}, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	rec := newResponseCapture()
	api.ServeHTTP(rec, req)

	result := apiResult{Status: rec.status}
	if rec.status >= 200 && rec.status < 300 {
		var envelope struct {
			Data      json.RawMessage `json:"data"`
			Rationale json.RawMessage `json:"rationale"`
		}
		if err := json.Unmarshal(rec.body.Bytes(), &envelope); err != nil {
			return apiResult{}, err
		}
		result.OK = true
		result.Data = envelope.Data
		result.Rationale = envelope.Rationale
		return result, nil
	}

	var errBody struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.body.Bytes(), &errBody); err != nil {
		return apiResult{}, err
	}
	result.ErrCode = errBody.Error.Code
	result.ErrMsg = errBody.Error.Message
	return result, nil
}

// decodeInto unmarshals an apiResult's Data into v, for a handler that
// needs the shape typed rather than raw.
func decodeInto(result apiResult, v any) error {
	if len(result.Data) == 0 {
		return nil
	}
	return json.Unmarshal(result.Data, v)
}

// decodeRationale unmarshals an apiResult's Rationale into v -- Q3, Q4, and
// Q6 carry a "rationale" sibling to "data" the same envelope shape as
// decodeInto's Data (internal/transport/http/errors.go's writeJSON); most
// endpoints omit it, in which case v is left untouched.
func decodeRationale(result apiResult, v any) error {
	if len(result.Rationale) == 0 {
		return nil
	}
	return json.Unmarshal(result.Rationale, v)
}
