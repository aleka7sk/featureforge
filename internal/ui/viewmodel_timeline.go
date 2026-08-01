package ui

import (
	"encoding/hex"
	"net/url"
	"strings"
)

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

// timelineReferencePageData is the read-only detail surface behind a
// Timeline reference. It deliberately contains only the exact identity and
// the Q5 events that cite it: the UI neither reaches into a repository nor
// invents record fields that the authoritative query did not return.
type timelineReferencePageData struct {
	PageTitle       string
	FeatureCardID   string
	Identity        string
	HasSourceEvent  bool
	PendingEvidence bool
	Dated           []timelineRow
	Undated         []timelineRow
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
	allRows := append(mapTimelineRows(dated), mapTimelineRows(undated)...)
	sourceRoutes := timelineSourceRoutes(featureCardID, allRows)
	mapLinks := func(rows []timelineRow) []timelineRow {
		for i := range rows {
			rows[i].Anchor = timelineAnchor(rows[i].Kind, rows[i].SourceIdentity)
			rows[i].SourceHref = timelineSourceHref(featureCardID, rows[i])
			rows[i].ReferenceLinks = make([]timelineReference, 0, len(rows[i].References))
			for _, reference := range rows[i].References {
				href := sourceRoutes[timelineReferencedSourceIdentity(reference)]
				if reference == rows[i].SourceIdentity {
					// The row itself disambiguates an own-source reference even
					// if another source family reused the same raw identity.
					href = rows[i].SourceHref
				}
				if href == "" && reference != "" {
					// Not every governed reference family has a dedicated read
					// query or page (lifecycle policy/version is the notable
					// example). Its exact Q5 citation is still an authoritative,
					// navigable read surface rather than a fabricated record page.
					href = timelineReferenceHref(featureCardID, reference)
				}
				rows[i].ReferenceLinks = append(rows[i].ReferenceLinks, timelineReference{
					Identity: reference,
					Href:     href,
				})
			}
		}
		return rows
	}

	return timelinePageData{
		PageTitle: "Timeline", FeatureCardID: featureCardID, SelectedKind: selectedKind, Kinds: timelineKinds,
		Dated: mapLinks(filterAndMapTimelineRows(dated, selectedKind)), Undated: mapLinks(filterAndMapTimelineRows(undated, selectedKind)),
	}
}

// mapTimelineReferencePageData resolves an exact identity only when Q5
// returned it as a source or reference for this Feature. A guessed, stale, or
// dangling identity therefore cannot acquire a plausible-looking detail page.
func mapTimelineReferencePageData(featureCardID, identity string, dated, undated []apiTimelineEventDTO) (timelineReferencePageData, bool) {
	timeline := mapTimelinePageData(featureCardID, "", dated, undated)
	targetSource := timelineReferencedSourceIdentity(identity)
	hasSourceEvent := false
	for _, row := range append(append([]timelineRow(nil), timeline.Dated...), timeline.Undated...) {
		if row.SourceIdentity == targetSource || timelineHasEmbeddedSource(row, identity) {
			hasSourceEvent = true
			break
		}
	}
	filter := func(rows []timelineRow) []timelineRow {
		matched := make([]timelineRow, 0)
		for _, row := range rows {
			found := row.SourceIdentity == identity || row.SourceIdentity == targetSource || timelineHasEmbeddedSource(row, identity)
			for _, reference := range row.References {
				if reference == identity {
					found = true
					break
				}
			}
			if found {
				matched = append(matched, row)
			}
		}
		return matched
	}

	data := timelineReferencePageData{
		PageTitle: "Timeline reference", FeatureCardID: featureCardID, Identity: identity,
		HasSourceEvent:  hasSourceEvent,
		PendingEvidence: strings.HasPrefix(identity, "evidence:") && !hasSourceEvent,
		Dated:           filter(timeline.Dated), Undated: filter(timeline.Undated),
	}
	return data, identity != "" && (len(data.Dated) > 0 || len(data.Undated) > 0)
}

// A lifecycle Definition Version is persisted configuration rather than a
// separate engineering act, so FF-006 correctly gives it no invented event.
// ResolveLifecycleHistory has nevertheless decoded and validated it before
// producing each lifecycle event; that event's policy summary is therefore
// the authoritative embedded source representation for its exact third ref.
func timelineHasEmbeddedSource(row timelineRow, identity string) bool {
	return row.Kind == "lifecycle.transitioned" && len(row.References) >= 3 && row.References[2] == identity
}

// timelineSourceRoutes indexes the exact read surfaces already owned by the
// UI. Product roots have dedicated pages. Every other source can be read as
// its exact computed event on the unfiltered Timeline page. A duplicate raw
// identity across source kinds is deliberately left without a route because
// a bare reference cannot select one of those events honestly.
func timelineSourceRoutes(featureCardID string, rows []timelineRow) map[string]string {
	routes := make(map[string]string, len(rows))
	ambiguous := make(map[string]bool)
	add := func(identity, href string) {
		if existing, exists := routes[identity]; exists {
			if existing != href {
				ambiguous[identity] = true
			}
			return
		}
		routes[identity] = href
	}
	for _, row := range rows {
		href := timelineSourceHref(featureCardID, row)
		add(row.SourceIdentity, href)
		if row.Kind == "lifecycle.transitioned" && len(row.References) >= 2 {
			// One lifecycle event is the validated composite of its source
			// assignment and exact establishing Transition Revision. The
			// event anchor is therefore the honest read route for either
			// identity even though only the assignment derives EventID.
			add(row.References[1], href)
		}
	}
	for identity := range ambiguous {
		delete(routes, identity)
	}
	return routes
}

func timelineSourceHref(featureCardID string, row timelineRow) string {
	switch row.Kind {
	case "project.created":
		return "/projects/" + url.PathEscape(row.SourceIdentity)
	case "feature.created":
		return "/features/" + url.PathEscape(row.SourceIdentity)
	default:
		return "/features/" + url.PathEscape(featureCardID) + "/timeline#" + timelineAnchor(row.Kind, row.SourceIdentity)
	}
}

func timelineReferenceHref(featureCardID, identity string) string {
	query := url.Values{"identity": []string{identity}}
	return "/features/" + url.PathEscape(featureCardID) + "/timeline/reference?" + query.Encode()
}

func timelineAnchor(kind, sourceIdentity string) string {
	return "timeline-source-" + hex.EncodeToString([]byte(kind+"\x00"+sourceIdentity))
}

// Timeline references use governed typed prefixes while TimelineEvent's
// SourceIdentity is the underlying envelope/record identity. Normalize only
// the closed reference forms whose target event is unambiguous; all other
// forms retain their exact text and use the generic Q5 reference detail.
func timelineReferencedSourceIdentity(reference string) string {
	for _, prefix := range []string{"artifact-revision:", "requirement-revision:", "evidence:", "artifact:"} {
		if identity, found := strings.CutPrefix(reference, prefix); found {
			return identity
		}
	}
	return reference
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
