package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A dangling mandatory timeline source is rejected by Q5 as stored-state
// integrity. The reference UI must preserve that failure instead of turning
// the requested identity into a plausible-looking detail page.
func TestTimelineReferencePagePreservesAuthoritativeTimelineIntegrityFailure(t *testing.T) {
	fake := &recordingHandler{respond: func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"code":"internal_error","message":"an unexpected error occurred"}}`))
	}}
	handler := NewHandler(Dependencies{API: fake})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/features/FC-1/timeline/reference?identity=claim%3ACLM-MISSING", nil))

	if fake.gotPath != "/api/v1/features/FC-1/timeline" {
		t.Fatalf("API path = %q, want authoritative Q5", fake.gotPath)
	}
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body = %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "CLM-MISSING") || !strings.Contains(rr.Body.String(), "an unexpected error occurred") {
		t.Fatalf("integrity page leaked or fabricated the requested reference: %s", rr.Body.String())
	}
}
