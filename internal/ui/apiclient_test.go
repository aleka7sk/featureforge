package ui

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// recordingHandler is a fake API handler that records exactly what it
// received and returns a fixed response -- used only to prove callAPI's
// own request-construction behaviour in isolation from a real API. Every
// other test in this package uses the real internal/transport/http
// handler (server_test.go), per FF-021 §12's "AD-028 delegation tests":
// this one proves the delegation shape itself.
type recordingHandler struct {
	gotMethod      string
	gotPath        string
	gotContentType string
	gotBody        []byte
	respond        func(w http.ResponseWriter)
}

func (h *recordingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.gotMethod = r.Method
	h.gotPath = r.URL.Path
	h.gotContentType = r.Header.Get("Content-Type")
	body, _ := io.ReadAll(r.Body)
	h.gotBody = body
	h.respond(w)
}

// TestCallAPIDelegatesExactRequestShape proves callAPI constructs exactly
// the route, method, and JSON body a real client would send, and that the
// response it returns is controlled entirely by what the handler writes --
// callAPI holds no independent command-execution path (FF-021 §2, §12).
func TestCallAPIDelegatesExactRequestShape(t *testing.T) {
	fake := &recordingHandler{respond: func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"project_id":"PRJ-1"}}`))
	}}

	result, err := callAPI(context.Background(), fake, http.MethodPost, "/api/v1/projects", map[string]any{
		"project_id": "PRJ-1", "name": "Pilot",
	})
	if err != nil {
		t.Fatalf("callAPI: %v", err)
	}

	if fake.gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", fake.gotMethod)
	}
	if fake.gotPath != "/api/v1/projects" {
		t.Errorf("path = %q, want /api/v1/projects", fake.gotPath)
	}
	if fake.gotContentType != "application/json" {
		t.Errorf("Content-Type sent to the API = %q, want application/json", fake.gotContentType)
	}
	var sentBody map[string]any
	if err := json.Unmarshal(fake.gotBody, &sentBody); err != nil {
		t.Fatalf("decoding what was sent to the API: %v; body = %s", err, fake.gotBody)
	}
	if sentBody["project_id"] != "PRJ-1" || sentBody["name"] != "Pilot" {
		t.Errorf("body sent to the API = %+v, want project_id=PRJ-1 name=Pilot", sentBody)
	}

	// The fake handler alone controls the outcome -- proving callAPI has
	// no independent path to a 201/success result.
	if !result.OK || result.Status != http.StatusCreated {
		t.Errorf("result = %+v, want OK with 201 (exactly what the fake handler wrote)", result)
	}
	var data struct {
		ProjectID string `json:"project_id"`
	}
	if err := decodeInto(result, &data); err != nil || data.ProjectID != "PRJ-1" {
		t.Errorf("decoded data = %+v, err = %v, want project_id=PRJ-1", data, err)
	}
}

// TestCallAPIDelegatesErrorOutcome proves a non-2xx response from the
// handler -- not callAPI itself -- controls the error the UI sees,
// including the exact conflict semantics the real API's error mapping
// produces. This is the "a change in API conflict semantics is observed
// by the UI without changing UI command logic" proof FF-021 §2 requires:
// swap what the fake returns, and callAPI's own code is untouched.
func TestCallAPIDelegatesErrorOutcome(t *testing.T) {
	fake := &recordingHandler{respond: func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":{"code":"immutable_value_conflict","message":"differs from the stored value"}}`))
	}}

	result, err := callAPI(context.Background(), fake, http.MethodPost, "/api/v1/projects", map[string]any{"project_id": "PRJ-1"})
	if err != nil {
		t.Fatalf("callAPI: %v", err)
	}
	if result.OK {
		t.Fatal("expected OK = false for a 409 response")
	}
	if result.Status != http.StatusConflict {
		t.Errorf("status = %d, want 409 (exactly what the fake handler wrote)", result.Status)
	}
	if result.ErrCode != "immutable_value_conflict" {
		t.Errorf("ErrCode = %q, want immutable_value_conflict", result.ErrCode)
	}
	if result.ErrMsg != "differs from the stored value" {
		t.Errorf("ErrMsg = %q, want the fake handler's exact message", result.ErrMsg)
	}
}

// TestCallAPIGetSendsNoBody proves a nil body (every GET) sends no
// Content-Type and no request body, matching a real browser-driven read.
func TestCallAPIGetSendsNoBody(t *testing.T) {
	fake := &recordingHandler{respond: func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"projects":[]}}`))
	}}
	if _, err := callAPI(context.Background(), fake, http.MethodGet, "/api/v1/projects", nil); err != nil {
		t.Fatalf("callAPI: %v", err)
	}
	if fake.gotContentType != "" {
		t.Errorf("Content-Type = %q, want empty for a GET with no body", fake.gotContentType)
	}
	if fake.gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", fake.gotMethod)
	}
}
