package ui_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/aleka7sk/featureforge/internal/ui"
)

func TestCapabilityFormRejectsMalformedCriterionInsteadOfDroppingIt(t *testing.T) {
	handler := newTestHandler()
	mustPostForm(t, handler, "/projects", url.Values{"project_id": {"PRJ-SYNTAX"}, "name": {"Pilot"}}, "/projects/PRJ-SYNTAX")
	mustPostForm(t, handler, "/projects/PRJ-SYNTAX/features", url.Values{
		"feature_card_id": {"FC-SYNTAX"}, "title": {"Homework"},
	}, "/features/FC-SYNTAX")

	rr := postForm(t, handler, "/features/FC-SYNTAX/capability", url.Values{
		"artifact_id": {"CAP-SYNTAX"}, "revision_id": {"CAP-SYNTAX-REV-1"},
		"title": {"Homework"}, "acceptance_criteria": {"AC-1 missing delimiter"},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "key: text") {
		t.Errorf("expected criterion syntax guidance, got %s", rr.Body.String())
	}

	_, body := getPage(t, handler, "/features/FC-SYNTAX")
	if strings.Contains(body, "CAP-SYNTAX-REV-1") {
		t.Errorf("malformed criterion submission wrote a capability revision: %s", body)
	}
}

func TestPlanFormRejectsMalformedActivityInsteadOfDroppingIt(t *testing.T) {
	handler := newTestHandler()
	mustPostForm(t, handler, "/projects", url.Values{"project_id": {"PRJ-PLAN-SYNTAX"}, "name": {"Pilot"}}, "/projects/PRJ-PLAN-SYNTAX")
	mustPostForm(t, handler, "/projects/PRJ-PLAN-SYNTAX/features", url.Values{
		"feature_card_id": {"FC-PLAN-SYNTAX"}, "title": {"Homework"},
	}, "/features/FC-PLAN-SYNTAX")
	mustPostForm(t, handler, "/features/FC-PLAN-SYNTAX/capability", url.Values{
		"artifact_id": {"CAP-PLAN-SYNTAX"}, "revision_id": {"CAP-PLAN-SYNTAX-REV-1"},
		"title": {"Homework"}, "problem_statement": {"No follow-up."},
		"acceptance_criteria": {"AC-1: Homework is visible."},
	}, "/features/FC-PLAN-SYNTAX")

	rr := postForm(t, handler, "/features/FC-PLAN-SYNTAX/validation-plan", url.Values{
		"artifact_id": {"VP-SYNTAX"}, "revision_id": {"VP-SYNTAX-REV-1"},
		"acceptance_record_id": {"ACC-VP-SYNTAX"},
		"activities":           {"A-1|manual-review|missing remaining fields"},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "six pipe-separated fields") {
		t.Errorf("expected activity syntax guidance, got %s", rr.Body.String())
	}
}

func TestPlanFormRejectsMalformedActivityBeforeAnyAPICall(t *testing.T) {
	apiCalls := 0
	api := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		apiCalls++
		http.Error(w, "the API must not be called for malformed form syntax", http.StatusInternalServerError)
	})
	handler := ui.NewHandler(ui.Dependencies{API: api})

	rr := postForm(t, handler, "/features/FC-PLAN-SYNTAX/validation-plan", url.Values{
		"artifact_id": {"VP-SYNTAX"}, "revision_id": {"VP-SYNTAX-REV-1"},
		"acceptance_record_id": {"ACC-VP-SYNTAX"},
		"activities":           {"A-1|manual-review|missing remaining fields"},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rr.Code, rr.Body.String())
	}
	if apiCalls != 0 {
		t.Fatalf("API calls = %d, want zero before malformed-form 400", apiCalls)
	}
}
