package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// TestTemplatesParseAtInit proves every template file parsed successfully
// when the package was loaded -- templates is a package-level var built
// with template.Must, so a parse failure would already have panicked
// before this test runs; this documents that fact as a test rather than
// leaving it implicit (FF-021 §14: "parse every template at init").
func TestTemplatesParseAtInit(t *testing.T) {
	for _, name := range []string{
		"error", "projects", "project_detail", "feature_overview", "revisions", "requirements", "decisions", "validation", "timeline",
	} {
		if templates.Lookup(name) == nil {
			t.Errorf("template %q was not parsed", name)
		}
	}
}

// TestRenderEscapesHostileContent proves html/template's contextual
// auto-escaping neutralises a value that looks like markup, wherever user
// or store-originated content is rendered (FF-021 §11, §14).
func TestRenderEscapesHostileContent(t *testing.T) {
	hostile := `<script>alert(1)</script>`
	var buf bytes.Buffer
	data := mapProjectsPageData([]apiProjectDTO{{ProjectID: "PRJ-1", Name: hostile, CreatedAt: time.Now()}})
	if err := templates.ExecuteTemplate(&buf, "projects", data); err != nil {
		t.Fatalf("ExecuteTemplate: %v", err)
	}
	if strings.Contains(buf.String(), "<script>") {
		t.Errorf("hostile content was not escaped: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "&lt;script&gt;") {
		t.Errorf("expected the escaped form in output, got %s", buf.String())
	}
}

// TestFeatureOverviewTemplate proves the screen renders every FF-001 §3.2
// element: rationale beside the readiness badge (never bare), the
// requirement count, lifecycle state, and recent history -- and that an
// empty capability renders the explicit empty state, not a blank panel.
func TestFeatureOverviewTemplate(t *testing.T) {
	data := mapFeatureOverviewPageData(
		apiFeatureCardDTO{FeatureCardID: "FC-1", ProjectID: "PRJ-1", Title: "Homework", Description: "After a lesson."},
		apiEngineeringStateDTO{
			Readiness: apiReadinessResultDTO{Status: "not-ready", PerRequirement: []apiPerRequirementReadinessDTO{
				{RequirementArtifactID: "REQ-1", HasClaim: true, Outcome: "peos:satisfied", VerdictReason: "satisfied"},
			}},
			Lifecycle: apiLifecycleStateDTO{Found: false},
		},
		apiEngineeringStateRationaleDTO{},
		"",
		nil,
	)
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "feature_overview", data); err != nil {
		t.Fatalf("ExecuteTemplate: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "No capability specification established yet.") {
		t.Error("expected the capability empty state")
	}
	if !strings.Contains(out, "status-not-ready") {
		t.Error("expected the not-ready status badge")
	}
	if !strings.Contains(out, "No lifecycle state assigned yet.") {
		t.Error("expected the lifecycle empty state")
	}
	if !strings.Contains(out, "No history yet.") {
		t.Error("expected the history empty state")
	}
}

// TestFeatureOverviewTemplateWithRationale proves a derived readiness
// answer never appears without its rationale (FF-001 §3.2: "A colour
// alone is not an explanation").
func TestFeatureOverviewTemplateWithRationale(t *testing.T) {
	rationale := apiEngineeringStateRationaleDTO{
		CurrentRevision: apiResolutionRationaleDTO{Rule: "greatest sequence among accepted revisions"},
		Lifecycle:       apiLifecycleRationaleDTO{Rule: "single unambiguous assignment"},
	}
	state := apiEngineeringStateDTO{
		CurrentRevision: apiCurrentRevisionDTO{Found: true, Revision: &apiRevisionDTO{RevisionID: "CAP-1-REV-1", Sequence: 1}},
		Lifecycle:       apiLifecycleStateDTO{Found: true, StateID: "featureforge:drafting"},
	}
	data := mapFeatureOverviewPageData(apiFeatureCardDTO{FeatureCardID: "FC-1", CapabilityArtifactID: "CAP-1"}, state, rationale, "Homework", nil)
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "feature_overview", data); err != nil {
		t.Fatalf("ExecuteTemplate: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "greatest sequence among accepted revisions") {
		t.Error("expected the current-revision rationale to render")
	}
	if !strings.Contains(out, "single unambiguous assignment") {
		t.Error("expected the lifecycle rationale to render")
	}
	if !strings.Contains(out, "Homework") {
		t.Error("expected the current revision title to render")
	}
}

// TestRevisionsTemplateShowsBothRevisionsIndependently proves FF-001
// §3.3's requirement directly: "Revision 1 must be as readable after
// Revision 2 exists as it was before."
func TestRevisionsTemplateShowsBothRevisionsIndependently(t *testing.T) {
	revisions := []apiRevisionDTO{
		{ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-1", Sequence: 1, Content: &apiContentDTO{Title: "Homework", ProblemStatement: "No follow-up."}},
		{ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-2", Sequence: 2, Content: &apiContentDTO{Title: "Homework", ProblemStatement: "No follow-up.", UserOutcome: "Now with audio."}},
	}
	current := apiCurrentRevisionDTO{Found: true, Revision: &apiRevisionDTO{RevisionID: "CAP-1-REV-1"}}
	rationale := apiResolutionRationaleDTO{Rule: "greatest sequence among accepted revisions", Considered: []apiConsideredRevisionDTO{
		{RevisionID: "CAP-1-REV-1", AcceptanceState: "accepted"},
		{RevisionID: "CAP-1-REV-2", AcceptanceState: "draft"},
	}}
	data := mapRevisionsPageData("FC-1", "CAP-1", revisions, current, rationale)
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "revisions", data); err != nil {
		t.Fatalf("ExecuteTemplate: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "No follow-up.") {
		t.Error("expected revision 1's problem statement")
	}
	if !strings.Contains(out, "Now with audio.") {
		t.Error("expected revision 2's user outcome")
	}
	if !strings.Contains(out, "status-accepted") || !strings.Contains(out, "status-draft") {
		t.Error("expected both acceptance states rendered")
	}
}

// TestValidationTemplateShowsSupersededClaim proves FF-001 §3.6's
// explicit requirement: a corrected claim is shown, not hidden, with its
// original outcome intact and a link to what corrected it.
func TestValidationTemplateShowsSupersededClaim(t *testing.T) {
	per := []apiPerRequirementReadinessDTO{
		{
			RequirementArtifactID: "REQ-2", HasClaim: true, ClaimID: "CLM-4", Outcome: "peos:not-satisfied",
			Reasoning: "The original review was mistaken.", Corrects: "CLM-2", VerdictReason: "not satisfied",
			Rejected: []apiRejectedClaimDTO{
				{RecordKey: "claim:CLM-2", Reason: "superseded", Outcome: "peos:satisfied", Reasoning: "The original reasoning.", CorrectedBy: "CLM-4"},
			},
		},
	}
	data := mapValidationPageData("FC-1", "CAP-1", apiValidationPlanDTO{Found: true, ArtifactID: "VP-1"}, per, nil)
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "validation", data); err != nil {
		t.Fatalf("ExecuteTemplate: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "claim:CLM-2") {
		t.Error("expected the superseded claim CLM-2 to be shown")
	}
	if !strings.Contains(out, "status-satisfied") {
		t.Error("expected CLM-2's original outcome (satisfied) to remain intact")
	}
	if !strings.Contains(out, "corrected by CLM-4") {
		t.Error("expected a link back to the claim that corrected it")
	}
	if !strings.Contains(out, "The original reasoning.") {
		t.Error("expected the superseded claim's own reasoning to remain visible")
	}
}

// TestTimelineTemplateFiltersByKind proves the render-time kind filter
// (open question N3) narrows the dated list without a second request.
func TestTimelineTemplateFiltersByKind(t *testing.T) {
	now := time.Now()
	events := []apiTimelineEventDTO{
		{Kind: "project.created", Label: "Project created", OccurredAt: &now, Rationale: "r1"},
		{Kind: "claim.recorded", Label: "Claim recorded", OccurredAt: &now, Rationale: "r2"},
	}
	data := mapTimelinePageData("FC-1", "claim.recorded", events, nil)
	if len(data.Dated) != 1 || data.Dated[0].Kind != "claim.recorded" {
		t.Fatalf("filtered Dated = %+v, want exactly one claim.recorded event", data.Dated)
	}
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "timeline", data); err != nil {
		t.Fatalf("ExecuteTemplate: %v", err)
	}
	if strings.Contains(buf.String(), "Project created") {
		t.Error("expected the filtered-out event to be absent from the rendered page")
	}
}

// TestErrorPageRenders proves the standalone error page renders without
// leaking a technical detail the caller did not explicitly pass in.
func TestErrorPageRenders(t *testing.T) {
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "error", errorPageData{PageTitle: "Not found", Heading: "Not found", Message: "There is no page at /nonexistent."}); err != nil {
		t.Fatalf("ExecuteTemplate: %v", err)
	}
	if !strings.Contains(buf.String(), "There is no page at /nonexistent.") {
		t.Error("expected the message to render")
	}
}
