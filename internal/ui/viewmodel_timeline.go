package ui

// timelinePageData is templates/timeline.html's shape (FF-001 §3.7).
// Kind filtering is render-time over the full result (open question N3):
// Q5 takes no filter parameter, and FF-015 §19 sizes every collection as
// single-digit.
type timelinePageData struct {
	PageTitle     string
	FeatureCardID string
	SelectedKind  string
	Kinds         []string
	Dated         []timelineRow
	Undated       []timelineRow
}

// timelineKinds is every event kind application.EventKind declares
// (internal/application/query_timeline.go), reproduced here as plain
// strings so the filter form has a fixed, complete option list without
// discovering it from a response that might, on a small fixture, be
// missing one.
var timelineKinds = []string{
	"project.created", "feature.created", "capability.created", "capability.revised",
	"capability.accepted", "capability.withdrawn", "requirement.revised", "decision.recorded",
	"plan.revised", "execution.recorded", "evidence.recorded", "claim.recorded",
	"claim.corrected", "lifecycle.transitioned",
}

func mapTimelinePageData(featureCardID, selectedKind string, dated, undated []apiTimelineEventDTO) timelinePageData {
	return timelinePageData{
		PageTitle: "Timeline", FeatureCardID: featureCardID, SelectedKind: selectedKind, Kinds: timelineKinds,
		Dated: filterAndMapTimelineRows(dated, selectedKind), Undated: filterAndMapTimelineRows(undated, selectedKind),
	}
}

func filterAndMapTimelineRows(events []apiTimelineEventDTO, kind string) []timelineRow {
	if kind == "" {
		return mapTimelineRows(events)
	}
	filtered := make([]apiTimelineEventDTO, 0, len(events))
	for _, e := range events {
		if e.Kind == kind {
			filtered = append(filtered, e)
		}
	}
	return mapTimelineRows(filtered)
}
