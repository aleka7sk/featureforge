package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestDecodeJSON proves the three decode-layer rules FF-018 §9.2/§9.3
// require: unknown fields rejected, exactly one JSON value required, and
// the Content-Type checked -- all before any application call, which is
// why this is tested directly against decodeJSON rather than through a
// full handler.
func TestDecodeJSON(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}

	tests := []struct {
		name        string
		body        string
		contentType string
		wantOK      bool
		wantStatus  int
	}{
		{"valid", `{"name":"x"}`, "application/json", true, 0},
		{"valid with charset", `{"name":"x"}`, "application/json; charset=utf-8", true, 0},
		{"no content type", `{"name":"x"}`, "", true, 0},
		{"unknown field", `{"name":"x","extra":1}`, "application/json", false, http.StatusBadRequest},
		{"multiple JSON values", `{"name":"x"}{"name":"y"}`, "application/json", false, http.StatusBadRequest},
		{"malformed JSON", `{"name":`, "application/json", false, http.StatusBadRequest},
		{"wrong content type", `{"name":"x"}`, "text/plain", false, http.StatusUnsupportedMediaType},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(tc.body))
			if tc.contentType != "" {
				r.Header.Set("Content-Type", tc.contentType)
			}
			w := httptest.NewRecorder()
			var v payload
			ok := decodeJSON(w, r, &v)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v; body = %s", ok, tc.wantOK, w.Body.String())
			}
			if !ok && w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tc.wantStatus)
			}
		})
	}
}
