package application

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/aleka7sk/featureforge/internal/domain"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

// EventKind is the closed set of timeline event kinds (FF-010 §9.1).
type EventKind string

const (
	EventProjectCreated        EventKind = "project.created"
	EventFeatureCreated        EventKind = "feature.created"
	EventCapabilityCreated     EventKind = "capability.created"
	EventCapabilityRevised     EventKind = "capability.revised"
	EventCapabilityAccepted    EventKind = "capability.accepted"
	EventCapabilityWithdrawn   EventKind = "capability.withdrawn"
	EventRequirementRevised    EventKind = "requirement.revised"
	EventDecisionRecorded      EventKind = "decision.recorded"
	EventPlanRevised           EventKind = "plan.revised"
	EventExecutionRecorded     EventKind = "execution.recorded"
	EventEvidenceRecorded      EventKind = "evidence.recorded"
	EventClaimRecorded         EventKind = "claim.recorded"
	EventClaimCorrected        EventKind = "claim.corrected"
	EventLifecycleTransitioned EventKind = "lifecycle.transitioned"
)

// kindRank orders events sharing one instant into causal order
// (FF-010 §9.3).
var kindRank = map[EventKind]int{
	EventProjectCreated:        0,
	EventFeatureCreated:        1,
	EventCapabilityCreated:     2,
	EventCapabilityRevised:     3,
	EventCapabilityAccepted:    4,
	EventCapabilityWithdrawn:   4,
	EventRequirementRevised:    5,
	EventDecisionRecorded:      6,
	EventPlanRevised:           7,
	EventExecutionRecorded:     8,
	EventEvidenceRecorded:      9,
	EventClaimRecorded:         10,
	EventClaimCorrected:        10,
	EventLifecycleTransitioned: 11,
}

// TimelineEvent is one entry in a feature's engineering timeline
// (FF-010 §9).
type TimelineEvent struct {
	EventID        string
	FeatureCardID  domain.FeatureCardID
	Kind           EventKind
	OccurredAt     time.Time
	HasOccurredAt  bool
	Actor          string
	Label          string
	Summary        string
	SourceIdentity string
	References     []string
	Corrected      string
	Rationale      string
}

// TimelineResult is a computed timeline: dated events in deterministic
// order, plus events whose source carries no timestamp (FF-010 §9.4).
type TimelineResult struct {
	Dated   []TimelineEvent
	Undated []TimelineEvent
}

// TimelineInput names every record family a timeline draws from, supplied
// by the caller rather than derived here. DecisionIDs, ExecutionIDs,
// EvidenceArtifactIDs, and ClaimIDs were already discoverable by composing
// existing repository listings (m5-contract-investigation.md §4 Option G).
// RequirementArtifactIDs and PlanArtifactID were not: no requirement-to-
// capability index existed in the M.3 repository set (FF-009 §5). AD-025
// and FF-016 close that gap by projecting a subject onto RevisionEnvelope; a
// caller that does not already know the requirement or validation-plan
// population can now obtain it via DiscoverRequirementArtifactIDs and
// DiscoverValidationPlanArtifactIDs before constructing this struct, whose
// shape and caller-supplied contract are otherwise unchanged.
type TimelineInput struct {
	Project                domain.Project
	FeatureCard            domain.FeatureCard
	CapabilityArtifactID   string
	RequirementArtifactIDs []string
	DecisionIDs            []string
	PlanArtifactID         string
	ExecutionIDs           []string
	EvidenceArtifactIDs    []string
	ClaimIDs               []string
}

// DiscoverValidationPlanArtifactIDs finds every validation plan whose
// projected subject -- the plan's PEOS Scope, projected the same way a
// requirement's Subject is (FF-016 §3.2) -- is capabilityArtifactID,
// returning their artifact IDs (AD-025, FF-016 §9). It is the
// TimelineInput.PlanArtifactID counterpart to DiscoverRequirementArtifactIDs.
func DiscoverValidationPlanArtifactIDs(ctx context.Context, repos Repositories, capabilityArtifactID string) ([]string, error) {
	return discoverArtifactIDsBySubject(ctx, repos, engineering.RevisionFamilyValidationPlan, capabilityArtifactID)
}

// ResolveApplicableValidationPlanID applies the exactly-one contract
// (FF-018 §6.6) to a capability's discovered validation-plan population:
// zero is legal and yields an empty PlanArtifactID -- a young feature has
// no plan events yet, which GetFeatureTimeline already treats as a normal
// input; exactly one is used; more than one is ErrValidationPlanAmbiguous,
// because the model defines no acceptance or order metadata for plans and
// therefore no mechanism to rank two. Sort order never selects a plan --
// DiscoverValidationPlanArtifactIDs sorts only for deterministic discovery
// output, and this function fails identically for two plans regardless of
// the order they were discovered or recorded in.
func ResolveApplicableValidationPlanID(ctx context.Context, repos Repositories, capabilityArtifactID string) (string, error) {
	plans, err := DiscoverValidationPlanArtifactIDs(ctx, repos, capabilityArtifactID)
	if err != nil {
		return "", err
	}
	switch len(plans) {
	case 0:
		return "", nil
	case 1:
		return plans[0], nil
	default:
		return "", fmt.Errorf("%w: capability %s has %d applicable validation plans: %v", ErrValidationPlanAmbiguous, capabilityArtifactID, len(plans), plans)
	}
}

// DiscoverExecutionAndClaimIDs finds every execution and claim naming the
// given capability revision as subject, returning their IDs independently
// deduplicated and sorted ascending (FF-018 §6.3). Executions and claims
// both project a capability revision as their subject
// (RecordEnvelope.SubjectKey uses ArtifactRevisionSubjectKey), so this is
// scoped to one revision rather than iterated across every revision the
// way DiscoverDecisionIDs is -- the caller passes the revision it means,
// typically the current one from ResolveCurrentRevision.
func DiscoverExecutionAndClaimIDs(ctx context.Context, repos Repositories, capabilityArtifactID, capabilityRevisionID string) (executionIDs, claimIDs []string, err error) {
	subjectKey := engineering.ArtifactRevisionSubjectKey(capabilityArtifactID, capabilityRevisionID)

	executions, err := repos.Records.ListByKindAndSubject(ctx, engineering.RecordKindExecution, subjectKey)
	if err != nil {
		return nil, nil, err
	}
	executionIDs = make([]string, 0, len(executions))
	for _, e := range executions {
		executionIDs = append(executionIDs, e.Key.ID)
	}
	sort.Strings(executionIDs)

	claims, err := repos.Records.ListByKindAndSubject(ctx, engineering.RecordKindClaim, subjectKey)
	if err != nil {
		return nil, nil, err
	}
	claimIDs = make([]string, 0, len(claims))
	for _, c := range claims {
		claimIDs = append(claimIDs, c.Key.ID)
	}
	sort.Strings(claimIDs)

	return executionIDs, claimIDs, nil
}

// DiscoverExecutionAndClaimIDsAllRevisions finds every execution and claim
// naming ANY revision of capabilityArtifactID as subject -- the
// history-wide counterpart to DiscoverExecutionAndClaimIDs above, which
// scopes to one revision. This is Q5's population, not Q3/Q4's (FF-018
// §24, M.5 publication remediation D1/D2): FF-006 §1 defines the timeline
// as computed over every immutable record, and FF-007's M.5 exit criterion
// requires "superseded claims and prior revisions are visible, not
// hidden" -- an execution, a piece of evidence, or a claim recorded
// against an earlier capability revision must remain on the timeline
// after a later revision becomes current. Revisions are enumerated with
// Revisions.ListByArtifact, exactly as DiscoverDecisionIDs already does;
// each revision's executions and claims are found via the existing
// per-revision DiscoverExecutionAndClaimIDs and unioned by their own
// authoritative record ID, deduplicated and sorted ascending, matching
// discoverArtifactIDsBySubject's determinism guarantee. Used only by
// GetFeatureTimelineForCard -- Q3/Q4's current-state population
// (discoverEngineeringStateComponents) is unaffected and remains scoped to
// the current revision alone: readiness must never let a claim against a
// superseded revision satisfy the current one (FF-010 §7).
func DiscoverExecutionAndClaimIDsAllRevisions(ctx context.Context, repos Repositories, capabilityArtifactID string) (executionIDs, claimIDs []string, err error) {
	revisions, err := repos.Revisions.ListByArtifact(ctx, capabilityArtifactID)
	if err != nil {
		return nil, nil, err
	}
	execSeen, claimSeen := map[string]bool{}, map[string]bool{}
	for _, rev := range revisions {
		revExecIDs, revClaimIDs, err := DiscoverExecutionAndClaimIDs(ctx, repos, capabilityArtifactID, rev.Key.RevisionID)
		if err != nil {
			return nil, nil, err
		}
		for _, id := range revExecIDs {
			if !execSeen[id] {
				execSeen[id] = true
				executionIDs = append(executionIDs, id)
			}
		}
		for _, id := range revClaimIDs {
			if !claimSeen[id] {
				claimSeen[id] = true
				claimIDs = append(claimIDs, id)
			}
		}
	}
	sort.Strings(executionIDs)
	sort.Strings(claimIDs)
	return executionIDs, claimIDs, nil
}

// DiscoverEvidenceArtifactIDs finds every evidence artifact cited by the
// given executions and claims, returning their artifact IDs deduplicated
// and sorted ascending (FF-018 §6.3). Both record kinds project
// EvidenceKeys; ParseEvidenceKey recovers the artifact ID from each. A
// malformed key, or an executionID/claimID that does not resolve to a
// stored record, is an error rather than a silent skip -- both indicate the
// store is in a state no answer can be derived from, matching
// ErrTimelineSourceInvalid's existing use for the same class of condition.
func DiscoverEvidenceArtifactIDs(ctx context.Context, repos Repositories, executionIDs, claimIDs []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(executionIDs)+len(claimIDs))

	collect := func(kind engineering.RecordKind, ids []string) error {
		for _, id := range ids {
			key, err := engineering.NewRecordKey(kind, id)
			if err != nil {
				return err
			}
			rec, found, err := repos.Records.Get(ctx, key)
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("%w: %s does not exist", ErrTimelineSourceInvalid, key)
			}
			for _, evidenceKey := range rec.EvidenceKeys {
				artifactID, _, err := engineering.ParseEvidenceKey(evidenceKey)
				if err != nil {
					return err
				}
				if seen[artifactID] {
					continue
				}
				seen[artifactID] = true
				out = append(out, artifactID)
			}
		}
		return nil
	}

	if err := collect(engineering.RecordKindExecution, executionIDs); err != nil {
		return nil, err
	}
	if err := collect(engineering.RecordKindClaim, claimIDs); err != nil {
		return nil, err
	}

	sort.Strings(out)
	return out, nil
}

// GetFeatureTimeline computes a feature's complete engineering timeline
// (FF-010 §9). It is read-only and deterministic: repeated calls on
// unchanged data return byte-identical results. No source value is mutated.
func GetFeatureTimeline(ctx context.Context, repos Repositories, in TimelineInput) (TimelineResult, error) {
	var events []TimelineEvent

	events = append(events, TimelineEvent{
		EventID: string(EventProjectCreated) + ":" + in.Project.ID().String(),
		Kind:    EventProjectCreated, OccurredAt: in.Project.CreatedAt(), HasOccurredAt: true,
		Label: "Project created", Summary: in.Project.Name(), SourceIdentity: in.Project.ID().String(),
		Rationale: "project creation timestamp",
	})

	events = append(events, TimelineEvent{
		EventID:       string(EventFeatureCreated) + ":" + in.FeatureCard.ID().String(),
		FeatureCardID: in.FeatureCard.ID(), Kind: EventFeatureCreated,
		OccurredAt: in.FeatureCard.CreatedAt(), HasOccurredAt: true,
		Label: "Feature card created", Summary: in.FeatureCard.Title(), SourceIdentity: in.FeatureCard.ID().String(),
		Rationale: "feature card creation timestamp",
	})

	if in.CapabilityArtifactID != "" {
		artEnv, found, err := repos.Artifacts.Get(ctx, engineering.ArtifactKey{ArtifactID: in.CapabilityArtifactID})
		if err != nil {
			return TimelineResult{}, err
		}
		if found {
			events = append(events, timelineFromArtifact(in.FeatureCard.ID(), EventCapabilityCreated, "Capability specification created", artEnv))
		}

		revisions, err := repos.Revisions.ListByArtifact(ctx, in.CapabilityArtifactID)
		if err != nil {
			return TimelineResult{}, err
		}
		for _, rev := range revisions {
			order, foundOrder, err := repos.RevisionOrder.Get(ctx, rev.Key)
			if err != nil {
				return TimelineResult{}, err
			}
			summary := "capability revision"
			if foundOrder {
				summary = fmt.Sprintf("sequence %d", order.Sequence)
			}
			events = append(events, timelineFromRevision(in.FeatureCard.ID(), EventCapabilityRevised, "Capability revision recorded", summary, rev))

			acceptance, err := repos.RevisionAcceptance.ListByRevision(ctx, rev.Key)
			if err != nil {
				return TimelineResult{}, err
			}
			for _, a := range acceptance {
				kind := EventCapabilityAccepted
				label := "Capability revision accepted"
				if a.State == engineering.AcceptanceStateWithdrawn {
					kind = EventCapabilityWithdrawn
					label = "Capability revision withdrawn"
				}
				events = append(events, TimelineEvent{
					EventID: string(kind) + ":" + a.RecordID, FeatureCardID: in.FeatureCard.ID(), Kind: kind,
					OccurredAt: a.EffectiveAt, HasOccurredAt: true, Actor: a.Actor, Label: label,
					Summary: string(a.State), SourceIdentity: a.RecordID, References: []string{rev.Key.String()},
					Rationale: "acceptance journal entry",
				})
			}
		}
	}

	for _, reqID := range in.RequirementArtifactIDs {
		revisions, err := repos.Revisions.ListByArtifact(ctx, reqID)
		if err != nil {
			return TimelineResult{}, err
		}
		for _, rev := range revisions {
			events = append(events, timelineFromRevision(in.FeatureCard.ID(), EventRequirementRevised, "Requirement recorded", reqID, rev))
		}
	}

	for _, decID := range in.DecisionIDs {
		key, err := engineering.NewRecordKey(engineering.RecordKindDecision, decID)
		if err != nil {
			return TimelineResult{}, err
		}
		rec, found, err := repos.Records.Get(ctx, key)
		if err != nil {
			return TimelineResult{}, err
		}
		if found {
			events = append(events, timelineFromRecord(in.FeatureCard.ID(), EventDecisionRecorded, "Decision recorded", "", rec))
		}
	}

	if in.PlanArtifactID != "" {
		revisions, err := repos.Revisions.ListByArtifact(ctx, in.PlanArtifactID)
		if err != nil {
			return TimelineResult{}, err
		}
		for _, rev := range revisions {
			events = append(events, timelineFromRevision(in.FeatureCard.ID(), EventPlanRevised, "Validation plan revision recorded", "", rev))
		}
	}

	for _, execID := range in.ExecutionIDs {
		key, err := engineering.NewRecordKey(engineering.RecordKindExecution, execID)
		if err != nil {
			return TimelineResult{}, err
		}
		rec, found, err := repos.Records.Get(ctx, key)
		if err != nil {
			return TimelineResult{}, err
		}
		if found {
			events = append(events, timelineFromRecord(in.FeatureCard.ID(), EventExecutionRecorded, "Validation activity executed", rec.Outcome, rec))
		}
	}

	for _, evID := range in.EvidenceArtifactIDs {
		revisions, err := repos.Revisions.ListByArtifact(ctx, evID)
		if err != nil {
			return TimelineResult{}, err
		}
		for _, rev := range revisions {
			events = append(events, timelineFromRevision(in.FeatureCard.ID(), EventEvidenceRecorded, "Evidence recorded", "", rev))
		}
	}

	for _, claimID := range in.ClaimIDs {
		key, err := engineering.NewRecordKey(engineering.RecordKindClaim, claimID)
		if err != nil {
			return TimelineResult{}, err
		}
		rec, found, err := repos.Records.Get(ctx, key)
		if err != nil {
			return TimelineResult{}, err
		}
		if !found {
			continue
		}
		kind := EventClaimRecorded
		label := "Claim recorded"
		corrected := ""
		if rec.HasCorrection() {
			kind = EventClaimCorrected
			label = "Claim recorded, correcting an earlier claim"
			corrected = rec.CorrectionTargetID
			if _, found, err := repos.Records.Get(ctx, engineering.RecordKey{Kind: engineering.RecordKindClaim, ID: corrected}); err != nil {
				return TimelineResult{}, err
			} else if !found {
				return TimelineResult{}, fmt.Errorf("%w: claim %s corrects %s, which does not exist", ErrTimelineSourceInvalid, claimID, corrected)
			}
		}
		ev := timelineFromRecord(in.FeatureCard.ID(), kind, label, rec.Outcome, rec)
		ev.Corrected = corrected
		events = append(events, ev)
	}

	if in.CapabilityArtifactID != "" {
		assignments, err := repos.Records.ListByKindAndSubject(ctx, engineering.RecordKindStateAssignment, engineering.ArtifactSubjectKey(in.CapabilityArtifactID))
		if err != nil {
			return TimelineResult{}, err
		}
		for _, a := range assignments {
			events = append(events, timelineFromRecord(in.FeatureCard.ID(), EventLifecycleTransitioned, "Lifecycle state -> "+a.StateID, a.StateID, a))
		}
	}

	return sortTimeline(events), nil
}

func timelineFromArtifact(cardID domain.FeatureCardID, kind EventKind, label string, env engineering.ArtifactEnvelope) TimelineEvent {
	return TimelineEvent{
		EventID: string(kind) + ":" + env.Key.String(), FeatureCardID: cardID, Kind: kind,
		OccurredAt: env.RecordedAt, HasOccurredAt: true, Label: label, Summary: env.ArtifactType,
		SourceIdentity: env.Key.String(), Rationale: "artifact recorded-at time",
	}
}

func timelineFromRevision(cardID domain.FeatureCardID, kind EventKind, label, summary string, env engineering.RevisionEnvelope) TimelineEvent {
	occurredAt := env.RecordedAt
	hasOccurredAt := true
	if env.HasProvenanceTime {
		occurredAt, hasOccurredAt = env.ProvenanceRecordedAt, true
	}
	if summary == "" {
		summary = string(env.RevisionFamily)
	}
	actor := env.ProvenanceActor
	return TimelineEvent{
		EventID: string(kind) + ":" + env.Key.String(), FeatureCardID: cardID, Kind: kind,
		OccurredAt: occurredAt, HasOccurredAt: hasOccurredAt, Actor: actor, Label: label, Summary: summary,
		SourceIdentity: env.Key.String(), Rationale: "revision provenance recorded-at time",
	}
}

// timelineFromRecord builds one timeline event from a record's own
// projected fields. References always names the record's subject, plus --
// for a record that cites evidence or an execution, such as
// execution.recorded and claim.recorded/corrected -- the existing
// EvidenceKeys/ExecutionKeys projections (FF-020 §5, FF-001 §3.6:
// "execution records with outcomes and evidence"), so a reader can follow
// an event to what it produced or relied on without a second query.
func timelineFromRecord(cardID domain.FeatureCardID, kind EventKind, label, summary string, env engineering.RecordEnvelope) TimelineEvent {
	references := []string{env.SubjectKey}
	references = append(references, env.EvidenceKeys...)
	references = append(references, env.ExecutionKeys...)
	return TimelineEvent{
		EventID: string(kind) + ":" + env.Key.String(), FeatureCardID: cardID, Kind: kind,
		OccurredAt: env.OccurredAt, HasOccurredAt: env.HasOccurredAt, Label: label, Summary: summary,
		SourceIdentity: env.Key.String(), References: references, Rationale: "record's own occurred-at time",
	}
}

// sortTimeline partitions events into dated and undated groups and orders
// the dated group by (OccurredAt, KindRank, SourceIdentity) -- a total
// order (FF-010 §9.3, §9.4). No source value passed in is mutated; a fresh
// slice is returned.
func sortTimeline(events []TimelineEvent) TimelineResult {
	var dated, undated []TimelineEvent
	for _, e := range events {
		if e.HasOccurredAt {
			dated = append(dated, e)
		} else {
			undated = append(undated, e)
		}
	}
	sort.SliceStable(dated, func(i, j int) bool {
		a, b := dated[i], dated[j]
		if !a.OccurredAt.Equal(b.OccurredAt) {
			return a.OccurredAt.Before(b.OccurredAt)
		}
		if kindRank[a.Kind] != kindRank[b.Kind] {
			return kindRank[a.Kind] < kindRank[b.Kind]
		}
		return a.SourceIdentity < b.SourceIdentity
	})
	sort.Slice(undated, func(i, j int) bool { return undated[i].SourceIdentity < undated[j].SourceIdentity })
	return TimelineResult{Dated: dated, Undated: undated}
}
