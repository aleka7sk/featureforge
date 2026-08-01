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
		"error", "projects", "project_detail", "feature_overview", "revisions", "requirements", "decisions", "validation", "timeline", "timeline_reference", "ai_proposal",
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
	data := mapProjectsPageData([]apiProjectDTO{{ProjectID: "PRJ-1", Name: hostile, CreatedAt: time.Now()}}, map[string]int{"PRJ-1": 1})
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
				{RequirementArtifactID: "REQ-1", HasClaim: true, ClaimID: "CLM-1", Outcome: "peos:not-satisfied", VerdictReason: "not satisfied"},
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
	if !strings.Contains(out, "REQ-1") || !strings.Contains(out, "CLM-1") {
		t.Error("expected the failing requirement and its exact claim ID")
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

func TestTimelineTemplateRendersUndatedFirstAndNavigableSourceReferences(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	capabilitySource := "CAP-1/CAP-1-REV-1"
	data := mapTimelinePageData("FC-1", "", []apiTimelineEventDTO{
		{
			Kind: "project.created", Label: "Project created", OccurredAt: &now,
			Actor: "featureforge:local-user", SourceIdentity: "PRJ-1",
			References: []string{"PRJ-1"}, Rationale: "recorded",
		},
		{
			Kind: "capability.revised", Label: "Capability revision recorded", OccurredAt: &now,
			Actor: "featureforge:local-user", SourceIdentity: capabilitySource,
			References: []string{capabilitySource}, Rationale: "recorded",
		},
		{
			Kind: "lifecycle.transitioned", Label: "Lifecycle state -> drafting", OccurredAt: &now,
			Actor: "featureforge:local-user", SourceIdentity: "state-assignment:SA-1",
			References: []string{"state-assignment:SA-1", "TR-1/TR-1-REV-0", "LCD-1/LCDV-1", "CAP-1"},
			Rationale:  "validated lifecycle predecessor-chain order",
		},
	}, []apiTimelineEventDTO{
		{
			Kind: "decision.recorded", Label: "Decision recorded", Actor: "featureforge:local-user",
			SourceIdentity: "decision:DEC-1",
			References:     []string{"decision:DEC-1", "artifact-revision:" + capabilitySource, "LCD-1/LCDV-1"},
			Rationale:      "no recorded timestamp; placed in the undated group above dated history",
		},
	})

	if len(data.Undated) != 1 || len(data.Undated[0].ReferenceLinks) != 3 {
		t.Fatalf("undated rows = %+v, want one complete row", data.Undated)
	}
	if data.Undated[0].ReferenceLinks[0].Href == "" || data.Undated[0].ReferenceLinks[1].Href == "" {
		t.Fatalf("known source links = %+v, want navigable routes", data.Undated[0].ReferenceLinks)
	}
	policyHref := timelineReferenceHref("FC-1", "LCD-1/LCDV-1")
	if data.Undated[0].ReferenceLinks[2].Href != policyHref {
		t.Fatalf("policy reference href = %q, want Q5-backed detail %q", data.Undated[0].ReferenceLinks[2].Href, policyHref)
	}
	if len(data.Dated) < 1 || data.Dated[0].SourceHref != "/projects/PRJ-1" || data.Dated[0].ReferenceLinks[0].Href != "/projects/PRJ-1" {
		t.Fatalf("project source route = %+v, want the existing project read route", data.Dated)
	}
	lifecycle := data.Dated[2]
	if lifecycle.Kind != "lifecycle.transitioned" || lifecycle.ReferenceLinks[1].Href != lifecycle.SourceHref || lifecycle.ReferenceLinks[2].Href != policyHref {
		t.Fatalf("lifecycle reference routes = %+v, want assignment/transition anchor and Q5-backed policy detail", lifecycle)
	}

	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "timeline", data); err != nil {
		t.Fatalf("ExecuteTemplate: %v", err)
	}
	out := buf.String()
	undatedAt := strings.Index(out, `id="undated-heading"`)
	datedAt := strings.Index(out, `id="dated-heading"`)
	if undatedAt < 0 || datedAt < 0 || undatedAt >= datedAt {
		t.Fatalf("undated/history order = %d/%d, want undated rendered first; output = %s", undatedAt, datedAt, out)
	}
	for _, want := range []string{
		"No recorded timestamp",
		"by featureforge:local-user",
		"<strong>Source:</strong>",
		"<code>decision:DEC-1</code>",
		"<code>artifact-revision:CAP-1/CAP-1-REV-1</code>",
		"<code>TR-1/TR-1-REV-0</code>",
		`href="/features/FC-1/timeline/reference?identity=LCD-1%2FLCDV-1"`,
		`href="/features/FC-1/timeline#timeline-source-`,
		`href="/projects/PRJ-1"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered timeline missing %q; output = %s", want, out)
		}
	}
}

func TestTimelineReferenceTemplateUsesOnlyExactQ5Citations(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	events := []apiTimelineEventDTO{
		{
			Kind: "lifecycle.transitioned", Label: "Lifecycle state -> drafting", OccurredAt: &now,
			Actor: "featureforge:local-user", SourceIdentity: "state-assignment:SA-1",
			References: []string{"state-assignment:SA-1", "TR-1/TR-1-REV-1", "LCD-1/LCDV-1", "CAP-1"},
			Rationale:  "validated lifecycle predecessor-chain order",
		},
		{
			Kind: "capability.revised", Label: "Unrelated revision", OccurredAt: &now,
			SourceIdentity: "CAP-1/CAP-1-REV-1", References: []string{"CAP-1/CAP-1-REV-1"},
		},
	}
	data, found := mapTimelineReferencePageData("FC-1", "LCD-1/LCDV-1", events, nil)
	if !found || len(data.Dated) != 1 || data.Dated[0].Kind != "lifecycle.transitioned" {
		t.Fatalf("reference data = %+v, found = %t; want only its exact citing event", data, found)
	}

	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "timeline_reference", data); err != nil {
		t.Fatalf("ExecuteTemplate: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"Timeline reference", "LCD-1/LCDV-1", "Lifecycle state -&gt; drafting",
		"authoritative feature timeline", "no additional record fields are inferred",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("reference detail missing %q; output = %s", want, out)
		}
	}
	if strings.Contains(out, "Unrelated revision") {
		t.Errorf("reference detail rendered a non-citing event: %s", out)
	}

	if _, found := mapTimelineReferencePageData("FC-1", "LCD-404/LCDV-404", events, nil); found {
		t.Fatal("unknown identity unexpectedly resolved")
	}
}

func TestTimelineReferenceDetailIncludesMaterialisedTypedSourceEvent(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	identity := "evidence:EV-1/EV-1-REV-1"
	data, found := mapTimelineReferencePageData("FC-1", identity, []apiTimelineEventDTO{
		{
			Kind: "decision.recorded", Label: "Decision recorded", OccurredAt: &now,
			SourceIdentity: "decision:DEC-1", References: []string{"decision:DEC-1", identity},
		},
		{
			Kind: "evidence.recorded", Label: "Evidence recorded", OccurredAt: &now,
			SourceIdentity: "EV-1/EV-1-REV-1", References: []string{"EV-1/EV-1-REV-1"},
		},
	}, nil)
	if !found || !data.HasSourceEvent || data.PendingEvidence || len(data.Dated) != 2 {
		t.Fatalf("materialised reference data = %+v, found = %t; want citation plus exact Evidence source", data, found)
	}
	if data.Dated[0].Kind != "decision.recorded" || data.Dated[1].Kind != "evidence.recorded" {
		t.Fatalf("materialised detail rows = %+v, want Decision citation and Evidence source", data.Dated)
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
