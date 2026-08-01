package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering"
	"github.com/aleka7sk/featureforge/internal/engineering/peos"
	"github.com/aleka7sk/featureforge/internal/proposal"
	"github.com/aleka7sk/featureforge/internal/scenario"
)

func currentCapabilityContent(t *testing.T, ctx context.Context, uow application.UnitOfWork, key engineering.RevisionKey) engineering.CapabilitySpecificationContent {
	t.Helper()
	var content engineering.CapabilitySpecificationContent
	if err := uow.Do(ctx, func(repos application.Repositories) error {
		var found bool
		var err error
		content, found, err = repos.StructuredContent.Get(ctx, key)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("content %s is absent", key)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return content
}

func establishMappedRequirement(
	t *testing.T,
	ctx context.Context,
	uow application.UnitOfWork,
	recorder peos.Recorder,
	clock *application.FixedClock,
	artifactID, criterionKey string,
) engineering.RevisionKey {
	t.Helper()
	clock.Advance(time.Hour)
	revisionID := artifactID + "-REV-1"
	acceptanceID := "ACC-" + artifactID
	result, err := (application.EstablishRequirementCommand{
		ArtifactID: artifactID, RevisionID: revisionID,
		Statement:                    artifactID + " SHALL remain independently verifiable.",
		SubjectArtifactID:            scenario.CapabilityArtifactID,
		SourceCapabilityRevisionID:   scenario.CapabilityRevision2,
		SourceAcceptanceCriterionKey: criterionKey,
		AcceptanceRecordID:           &acceptanceID,
	}).Execute(ctx, uow, recorder, recorder, clock)
	if err != nil {
		t.Fatalf("EstablishRequirementCommand(%s): %v", artifactID, err)
	}
	return result.RevisionKey
}

func findUncovered(pack proposal.ContextPack, key string) (proposal.UncoveredCriterion, bool) {
	for _, uncovered := range pack.UncoveredCriteria() {
		if uncovered.CriterionKey() == key {
			return uncovered, true
		}
	}
	return proposal.UncoveredCriterion{}, false
}

func TestAssembleProposalContextClassifiesNoTraceAndOldRevisionTrace(t *testing.T) {
	ctx, uow, recorder, clock := proposalFixture(t)
	previous := currentCapabilityContent(t, ctx, uow, engineering.RevisionKey{
		ArtifactID: scenario.CapabilityArtifactID, RevisionID: scenario.CapabilityRevision2,
	})
	ac5, err := engineering.NewAcceptanceCriterion("AC-5", "A fifth current criterion has no Requirement trace.")
	if err != nil {
		t.Fatal(err)
	}
	content, err := previous.WithAcceptanceCriteria(append(previous.AcceptanceCriteria(), ac5))
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Hour)
	const currentRevision = "CAP-1-REV-3-NO-TRACE"
	if _, err := (application.ReviseCapabilitySpecificationCommand{
		ArtifactID: scenario.CapabilityArtifactID, RevisionID: currentRevision, Content: content,
	}).Execute(ctx, uow, recorder, recorder, clock); err != nil {
		t.Fatalf("ReviseCapabilitySpecificationCommand: %v", err)
	}
	clock.Advance(time.Hour)
	if _, err := (application.AcceptCapabilityRevisionCommand{
		RecordID: "ACC-CAP-3-NO-TRACE", ArtifactID: scenario.CapabilityArtifactID,
		RevisionID: currentRevision, State: engineering.AcceptanceStateAccepted,
	}).Execute(ctx, uow, recorder, clock); err != nil {
		t.Fatalf("AcceptCapabilityRevisionCommand: %v", err)
	}

	pack, err := application.AssembleProposalContext(ctx, uow, recorder, recorder, scenario.CapabilityArtifactID)
	if err != nil {
		t.Fatalf("AssembleProposalContext: %v", err)
	}
	if got := pack.Capability().Revision().RevisionID; got != currentRevision {
		t.Fatalf("current revision = %s, want %s", got, currentRevision)
	}
	for _, key := range []string{"AC-1", "AC-5"} {
		uncovered, found := findUncovered(pack, key)
		if !found || uncovered.Reason() != proposal.UncoveredNoRequirementTrace || len(uncovered.RequirementRevisions()) != 0 {
			t.Errorf("%s uncovered = %#v found=%v, want no_requirement_trace with no current mappings", key, uncovered, found)
		}
	}
	var oldTraceIncluded bool
	for _, requirement := range pack.Requirements() {
		if requirement.Revision().ArtifactID == "REQ-1" {
			oldTraceIncluded = requirement.SourceCapabilityRevision() == (engineering.RevisionKey{
				ArtifactID: scenario.CapabilityArtifactID, RevisionID: scenario.CapabilityRevision2,
			}) && requirement.SourceCriterionKey() == "AC-1"
		}
	}
	if !oldTraceIncluded {
		t.Fatal("REQ-1's exact old-revision trace was not included honestly")
	}
	sources := proposalSourceValues(pack.Sources())
	for _, source := range []string{
		"requirement-trace:REQ-1/REQ-1-REV-1",
		"revision:CAP-1/CAP-1-REV-2",
		"criterion:CAP-1/CAP-1-REV-3-NO-TRACE#AC-5",
	} {
		if !slices.Contains(sources, source) {
			t.Errorf("sources missing %q: %v", source, sources)
		}
	}
}

func TestAssembleProposalContextSeveralMappedRequirementsOneMissingClaim(t *testing.T) {
	ctx, uow, recorder, clock := proposalFixture(t)
	missing := establishMappedRequirement(t, ctx, uow, recorder, clock, "REQ-5", "AC-1")

	pack, err := application.AssembleProposalContext(ctx, uow, recorder, recorder, scenario.CapabilityArtifactID)
	if err != nil {
		t.Fatalf("AssembleProposalContext: %v", err)
	}
	uncovered, found := findUncovered(pack, "AC-1")
	if !found || uncovered.Reason() != proposal.UncoveredMissingCurrentClaim ||
		!slices.Equal(uncovered.RequirementRevisions(), []engineering.RevisionKey{missing}) {
		t.Fatalf("AC-1 uncovered = %#v found=%v, want only missing REQ-5 claim", uncovered, found)
	}
	var satisfiedMapped bool
	for _, claim := range pack.Claims() {
		if claim.RequirementRevision().ArtifactID == "REQ-1" && claim.Outcome() == "satisfied" {
			satisfiedMapped = true
		}
	}
	if !satisfiedMapped {
		t.Fatal("the independently satisfied REQ-1 mapping disappeared")
	}
}

func TestAssembleProposalContextTreatsInconclusiveAsCoveredFinding(t *testing.T) {
	ctx, uow, recorder, clock := proposalFixture(t)
	clock.Advance(time.Hour)
	const claimID = "CLM-3-INCONCLUSIVE"
	if _, err := (application.CorrectValidationClaimCommand{
		ClaimID: claimID, CorrectionTarget: scenario.ClaimForR3, CorrectionKind: "correct",
		ScopeArtifactID:   scenario.CapabilityArtifactID,
		SubjectArtifactID: scenario.CapabilityArtifactID, SubjectRevisionID: scenario.CapabilityRevision2,
		RequirementArtifactID: "REQ-3", RequirementRevisionID: "REQ-3-REV-1",
		Outcome: "inconclusive", Method: "manual-inspection",
		EvidenceArtifactID: "EV-3", EvidenceRevisionID: "EV-3-REV-1", ExecutionID: "ER-3",
		Reasoning: "The available evidence does not decide the criterion.",
	}).Execute(ctx, uow, recorder, recorder, clock); err != nil {
		t.Fatalf("CorrectValidationClaimCommand: %v", err)
	}

	pack, err := application.AssembleProposalContext(ctx, uow, recorder, recorder, scenario.CapabilityArtifactID)
	if err != nil {
		t.Fatalf("AssembleProposalContext: %v", err)
	}
	if _, uncovered := findUncovered(pack, "AC-3"); uncovered {
		t.Fatal("AC-3 is mapped to a current inconclusive Claim and must remain covered")
	}
	for _, finding := range pack.Findings() {
		if finding.CriterionKey() == "AC-3" && finding.ClaimRecord().ID == claimID && finding.Outcome() == proposal.FindingInconclusive {
			return
		}
	}
	t.Fatalf("findings = %#v, want AC-3 inconclusive current Claim %s", pack.Findings(), claimID)
}

func recordDecision(t *testing.T, ctx context.Context, uow application.UnitOfWork, recorder peos.Recorder, clock *application.FixedClock, id, subjectArtifactID, subjectRevisionID string) {
	t.Helper()
	clock.Advance(time.Hour)
	if _, err := (application.RecordArchitectureDecisionCommand{
		DecisionID: id, SubjectArtifactID: subjectArtifactID, SubjectRevisionID: subjectRevisionID,
		Question: "What exact proposal constraint applies?", OutcomeStatement: "Retain the exact governed constraint.",
		EvidenceArtifactID: scenario.DecisionEvidenceID, EvidenceRevisionID: scenario.DecisionEvidenceID + "-REV-1",
		Rationale: "The persisted evidence supports the governed constraint.",
	}).Execute(ctx, uow, recorder, recorder, clock); err != nil {
		t.Fatalf("RecordArchitectureDecisionCommand(%s): %v", id, err)
	}
}

func establishForeignCapability(t *testing.T, ctx context.Context, uow application.UnitOfWork, recorder peos.Recorder, clock *application.FixedClock) engineering.RevisionKey {
	t.Helper()
	clock.Advance(time.Hour)
	if _, err := (application.CreateFeatureCommand{
		FeatureCardID: "FC-FOREIGN", ProjectID: scenario.ProjectID, Title: "Foreign capability",
	}).Execute(ctx, uow, clock); err != nil {
		t.Fatalf("CreateFeatureCommand(foreign): %v", err)
	}
	content := currentCapabilityContent(t, ctx, uow, engineering.RevisionKey{
		ArtifactID: scenario.CapabilityArtifactID, RevisionID: scenario.CapabilityRevision2,
	})
	clock.Advance(time.Hour)
	const artifactID, revisionID = "CAP-FOREIGN", "CAP-FOREIGN-REV-1"
	if _, err := (application.EstablishCapabilitySpecificationCommand{
		FeatureCardID: "FC-FOREIGN", ArtifactID: artifactID, RevisionID: revisionID, Content: content,
	}).Execute(ctx, uow, recorder, recorder, clock); err != nil {
		t.Fatalf("EstablishCapabilitySpecificationCommand(foreign): %v", err)
	}
	clock.Advance(time.Hour)
	if _, err := (application.AcceptCapabilityRevisionCommand{
		RecordID: "ACC-CAP-FOREIGN", ArtifactID: artifactID, RevisionID: revisionID,
		State: engineering.AcceptanceStateAccepted,
	}).Execute(ctx, uow, recorder, clock); err != nil {
		t.Fatalf("AcceptCapabilityRevisionCommand(foreign): %v", err)
	}
	return engineering.RevisionKey{ArtifactID: artifactID, RevisionID: revisionID}
}

func TestAssembleProposalContextIncludesCurrentAndOlderDecisionsAndExcludesValidatedForeign(t *testing.T) {
	ctx, uow, recorder, clock := proposalFixture(t)
	recordDecision(t, ctx, uow, recorder, clock, "DEC-ARTIFACT", scenario.CapabilityArtifactID, "")
	recordDecision(t, ctx, uow, recorder, clock, "DEC-CURRENT", scenario.CapabilityArtifactID, scenario.CapabilityRevision2)
	foreign := establishForeignCapability(t, ctx, uow, recorder, clock)
	recordDecision(t, ctx, uow, recorder, clock, "DEC-FOREIGN", foreign.ArtifactID, foreign.RevisionID)

	pack, err := application.AssembleProposalContext(ctx, uow, recorder, recorder, scenario.CapabilityArtifactID)
	if err != nil {
		t.Fatalf("AssembleProposalContext: %v", err)
	}
	ids := make([]string, 0, len(pack.Decisions()))
	for _, decision := range pack.Decisions() {
		ids = append(ids, decision.DecisionID())
	}
	if !slices.Contains(ids, scenario.DecisionID) || !slices.Contains(ids, "DEC-ARTIFACT") ||
		!slices.Contains(ids, "DEC-CURRENT") || slices.Contains(ids, "DEC-FOREIGN") {
		t.Fatalf("decision ids = %v, want Artifact + older + current and no validated foreign Decision", ids)
	}
	sources := proposalSourceValues(pack.Sources())
	for _, source := range []string{
		"record:decision/DEC-ARTIFACT", "artifact:CAP-1",
		"record:decision/DEC-1", "revision:CAP-1/CAP-1-REV-1",
		"record:decision/DEC-CURRENT", "revision:CAP-1/CAP-1-REV-2",
	} {
		if !slices.Contains(sources, source) {
			t.Errorf("sources missing %q: %v", source, sources)
		}
	}
	for _, source := range []string{"record:decision/DEC-FOREIGN", "revision:CAP-FOREIGN/CAP-FOREIGN-REV-1"} {
		if slices.Contains(sources, source) {
			t.Errorf("sources contain foreign witness %q: %v", source, sources)
		}
	}
}

func TestAssembleProposalContextRejectsCorruptHiddenDecisionProjection(t *testing.T) {
	ctx, uow, recorder, clock := proposalFixture(t)
	foreign := establishForeignCapability(t, ctx, uow, recorder, clock)
	clock.Advance(time.Hour)
	env, err := recorder.RecordDecision(engineering.DecisionInput{
		DecisionID:        "DEC-HIDDEN-CORRUPT",
		SubjectArtifactID: scenario.CapabilityArtifactID, SubjectRevisionID: scenario.CapabilityRevision2,
		Question: "Can a corrupt projection be hidden?", OutcomeStatement: "It must not be hidden.",
		EvidenceArtifactID: scenario.DecisionEvidenceID, EvidenceRevisionID: scenario.DecisionEvidenceID + "-REV-1",
		RecordedAt: clock.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	// The authoritative payload names CAP-1, while the persisted projection
	// looks foreign. Global validation must fail before subject filtering.
	env.SubjectKey = engineering.ArtifactRevisionSubjectKey(foreign.ArtifactID, foreign.RevisionID)
	if err := uow.Do(ctx, func(repos application.Repositories) error { return repos.Records.Put(ctx, env) }); err != nil {
		t.Fatalf("persist corrupt hidden Decision: %v", err)
	}

	if _, err := application.AssembleProposalContext(ctx, uow, recorder, recorder, scenario.CapabilityArtifactID); !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("AssembleProposalContext err = %v, want ErrStoredStateIntegrity", err)
	}
}

func seedDanglingRequirementTrace(t *testing.T, ctx context.Context, uow application.UnitOfWork, recorder peos.Recorder, clock *application.FixedClock) {
	t.Helper()
	clock.Advance(time.Hour)
	const artifactID, revisionID = "REQ-DANGLING-TRACE", "REQ-DANGLING-TRACE-REV-1"
	key := engineering.RevisionKey{ArtifactID: artifactID, RevisionID: revisionID}
	artifact, revision, err := recorder.RecordRequirement(engineering.RequirementInput{
		ArtifactID: artifactID, RevisionID: revisionID,
		Statement:         "A dangling trace SHALL fail the complete proposal assembly.",
		SubjectArtifactID: scenario.CapabilityArtifactID, RecordedAt: clock.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	order, err := engineering.NewRevisionOrderMetadata(key, 1, clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	acceptance, err := engineering.NewRevisionAcceptanceRecord(
		"ACC-REQ-DANGLING-TRACE", key, engineering.AcceptanceStateAccepted,
		clock.Now(), "featureforge:local-user", "",
	)
	if err != nil {
		t.Fatal(err)
	}
	trace, err := engineering.NewRequirementCriterionTrace(
		key,
		engineering.RevisionKey{ArtifactID: scenario.CapabilityArtifactID, RevisionID: scenario.CapabilityRevision2},
		"AC-MISSING", clock.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := uow.Do(ctx, func(repos application.Repositories) error {
		if err := repos.Artifacts.Put(ctx, artifact); err != nil {
			return err
		}
		if err := repos.Revisions.Put(ctx, revision); err != nil {
			return err
		}
		if err := repos.RevisionOrder.Put(ctx, order); err != nil {
			return err
		}
		if err := repos.RequirementTraces.Put(ctx, trace); err != nil {
			return err
		}
		return repos.RevisionAcceptance.Append(ctx, acceptance)
	}); err != nil {
		t.Fatalf("persist dangling Requirement aggregate: %v", err)
	}
}

func seedDanglingClaimSupport(t *testing.T, ctx context.Context, uow application.UnitOfWork, recorder peos.Recorder, clock *application.FixedClock) {
	t.Helper()
	clock.Advance(time.Hour)
	env, err := recorder.RecordClaim(engineering.ClaimInput{
		ClaimID: "CLM-DANGLING-SUPPORT", ScopeArtifactID: scenario.CapabilityArtifactID,
		SubjectArtifactID: scenario.CapabilityArtifactID, SubjectRevisionID: scenario.CapabilityRevision2,
		RequirementArtifactID: "REQ-4", RequirementRevisionID: "REQ-4-REV-1",
		Outcome: "inconclusive", Method: "manual-review",
		EvidenceArtifactID: "EV-DANGLING", EvidenceRevisionID: "EV-DANGLING-REV-1",
		ExecutionID: "ER-DANGLING", Reasoning: "The support records are absent.",
		Timestamp: clock.Now(), RecordedAt: clock.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := uow.Do(ctx, func(repos application.Repositories) error { return repos.Records.Put(ctx, env) }); err != nil {
		t.Fatalf("persist dangling Claim: %v", err)
	}
}

func TestAssembleProposalContextRejectsDanglingTraceOrClaimSupportAsWholePack(t *testing.T) {
	for _, test := range []struct {
		name string
		seed func(*testing.T, context.Context, application.UnitOfWork, peos.Recorder, *application.FixedClock)
	}{
		{name: "Requirement trace", seed: seedDanglingRequirementTrace},
		{name: "Claim support", seed: seedDanglingClaimSupport},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, uow, recorder, clock := proposalFixture(t)
			test.seed(t, ctx, uow, recorder, clock)
			pack, err := application.AssembleProposalContext(ctx, uow, recorder, recorder, scenario.CapabilityArtifactID)
			if !errors.Is(err, application.ErrStoredStateIntegrity) {
				t.Fatalf("AssembleProposalContext err = %v, want ErrStoredStateIntegrity", err)
			}
			if !pack.IsZero() {
				t.Fatalf("failed assembly returned a partial pack: %#v", pack)
			}
		})
	}
}

func assertProposalTargetAbsent(t *testing.T, ctx context.Context, uow application.UnitOfWork, key engineering.RevisionKey) {
	t.Helper()
	if err := uow.Do(ctx, func(repos application.Repositories) error {
		revision, revisionFound, err := repos.Revisions.Get(ctx, key)
		if err != nil {
			return err
		}
		content, contentFound, err := repos.StructuredContent.Get(ctx, key)
		if err != nil {
			return err
		}
		order, orderFound, err := repos.RevisionOrder.Get(ctx, key)
		if err != nil {
			return err
		}
		journal, err := repos.RevisionAcceptance.ListByRevision(ctx, key)
		if err != nil {
			return err
		}
		trace, traceFound, err := repos.RequirementTraces.Get(ctx, key)
		if err != nil {
			return err
		}
		if revisionFound || !revision.Key.IsZero() || contentFound || !content.IsZero() ||
			orderFound || !order.Key.IsZero() || len(journal) != 0 || traceFound || !trace.IsZero() {
			return fmt.Errorf("proposal target %s is not wholly absent", key)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAcceptCapabilityProposalDetectsEachPersistedFreshnessChangeWithZeroTargetWrites(t *testing.T) {
	for index, test := range []struct {
		name   string
		mutate func(*testing.T, context.Context, application.UnitOfWork, peos.Recorder, *application.FixedClock)
	}{
		{
			name: "new Requirement",
			mutate: func(t *testing.T, ctx context.Context, uow application.UnitOfWork, recorder peos.Recorder, clock *application.FixedClock) {
				establishMappedRequirement(t, ctx, uow, recorder, clock, "REQ-FRESHNESS", "AC-1")
			},
		},
		{
			name: "corrected Claim",
			mutate: func(t *testing.T, ctx context.Context, uow application.UnitOfWork, recorder peos.Recorder, clock *application.FixedClock) {
				clock.Advance(time.Hour)
				if _, err := (application.CorrectValidationClaimCommand{
					ClaimID: "CLM-FRESHNESS-CORRECTION", CorrectionTarget: scenario.ClaimForR1, CorrectionKind: "correct",
					ScopeArtifactID:   scenario.CapabilityArtifactID,
					SubjectArtifactID: scenario.CapabilityArtifactID, SubjectRevisionID: scenario.CapabilityRevision2,
					RequirementArtifactID: "REQ-1", RequirementRevisionID: "REQ-1-REV-1",
					Outcome: "not-satisfied", Method: "manual-review",
					EvidenceArtifactID: "EV-1", EvidenceRevisionID: "EV-1-REV-1", ExecutionID: "ER-1",
					Reasoning: "A later review changes the effective validation conclusion.",
				}).Execute(ctx, uow, recorder, recorder, clock); err != nil {
					t.Fatalf("CorrectValidationClaimCommand: %v", err)
				}
			},
		},
		{
			name: "new applicable Decision",
			mutate: func(t *testing.T, ctx context.Context, uow application.UnitOfWork, recorder peos.Recorder, clock *application.FixedClock) {
				recordDecision(t, ctx, uow, recorder, clock, "DEC-FRESHNESS", scenario.CapabilityArtifactID, scenario.CapabilityRevision2)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, uow, recorder, clock := proposalFixture(t)
			generated, err := application.GenerateCapabilityProposal(
				ctx, uow, recorder, recorder, proposal.NewDeterministicGenerator(), scenario.CapabilityArtifactID,
			)
			if err != nil {
				t.Fatalf("GenerateCapabilityProposal: %v", err)
			}
			test.mutate(t, ctx, uow, recorder, clock)
			target := engineering.RevisionKey{
				ArtifactID: scenario.CapabilityArtifactID,
				RevisionID: fmt.Sprintf("CAP-1-STALE-%d", index+1),
			}
			_, err = application.AcceptCapabilityProposal(ctx, uow, recorder, recorder, recorder, clock, application.AcceptCapabilityProposalInput{
				ArtifactID: target.ArtifactID, RevisionID: target.RevisionID, Proposal: generated.Proposal,
			})
			if !errors.Is(err, application.ErrProposalContextStale) {
				t.Fatalf("AcceptCapabilityProposal err = %v, want ErrProposalContextStale", err)
			}
			assertProposalTargetAbsent(t, ctx, uow, target)
		})
	}
}

func TestAcceptCapabilityProposalChangedContentConflictsUnderOccupiedTarget(t *testing.T) {
	ctx, uow, recorder, clock := proposalFixture(t)
	generated, err := application.GenerateCapabilityProposal(
		ctx, uow, recorder, recorder, proposal.NewDeterministicGenerator(), scenario.CapabilityArtifactID,
	)
	if err != nil {
		t.Fatalf("GenerateCapabilityProposal: %v", err)
	}
	target := engineering.RevisionKey{ArtifactID: scenario.CapabilityArtifactID, RevisionID: "CAP-1-PROPOSAL-CONTENT-CONFLICT"}
	created, err := application.AcceptCapabilityProposal(ctx, uow, recorder, recorder, recorder, clock, application.AcceptCapabilityProposalInput{
		ArtifactID: target.ArtifactID, RevisionID: target.RevisionID, Proposal: generated.Proposal,
	})
	if err != nil {
		t.Fatalf("AcceptCapabilityProposal(create): %v", err)
	}
	changedContent, err := generated.Proposal.Content().WithOpenQuestions([]string{"A changed proposed question."})
	if err != nil {
		t.Fatal(err)
	}
	changed, err := proposal.NewProposal(generated.ContextPack, changedContent, generated.Proposal.Rationale(), generated.Proposal.Sources())
	if err != nil {
		t.Fatal(err)
	}
	_, err = application.AcceptCapabilityProposal(ctx, uow, recorder, recorder, recorder, clock, application.AcceptCapabilityProposalInput{
		ArtifactID: target.ArtifactID, RevisionID: target.RevisionID, Proposal: changed,
	})
	if !errors.Is(err, application.ErrImmutableValueConflict) {
		t.Fatalf("changed content replay err = %v, want ErrImmutableValueConflict", err)
	}
	if created.RevisionKey != target {
		t.Fatalf("created key = %s, want %s", created.RevisionKey, target)
	}
	persisted := currentCapabilityContent(t, ctx, uow, target)
	if !persisted.Equal(generated.Proposal.Content()) {
		t.Fatal("conflicting replay changed the occupied target content")
	}
}

func TestAcceptCapabilityProposalRejectsForeignPersistedSourceWitnessBeforeReplay(t *testing.T) {
	ctx, uow, recorder, clock := proposalFixture(t)
	foreign := establishForeignCapability(t, ctx, uow, recorder, clock)
	generated, err := application.GenerateCapabilityProposal(
		ctx, uow, recorder, recorder, proposal.NewDeterministicGenerator(), foreign.ArtifactID,
	)
	if err != nil {
		t.Fatalf("GenerateCapabilityProposal(foreign): %v", err)
	}

	clock.Advance(time.Hour)
	target := engineering.RevisionKey{
		ArtifactID: scenario.CapabilityArtifactID,
		RevisionID: "CAP-1-FOREIGN-SOURCE-WITNESS",
	}
	contentDigest, err := generated.Proposal.Content().Digest()
	if err != nil {
		t.Fatal(err)
	}
	revision, err := recorder.RecordAIAssistedCapabilityRevision(
		engineering.CapabilityRevisionInput{
			ArtifactID: target.ArtifactID, RevisionID: target.RevisionID,
			ContentDigest: contentDigest, RecordedAt: clock.Now(),
		},
		generated.Proposal.ProposalDigest(), generated.Proposal.ContextDigest(),
		proposalSourceValues(generated.Proposal.Sources()),
	)
	if err != nil {
		t.Fatal(err)
	}
	order, err := engineering.NewRevisionOrderMetadata(target, 3, clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := uow.Do(ctx, func(repos application.Repositories) error {
		if err := repos.Revisions.Put(ctx, revision); err != nil {
			return err
		}
		if err := repos.StructuredContent.Put(ctx, target, generated.Proposal.Content()); err != nil {
			return err
		}
		return repos.RevisionOrder.Put(ctx, order)
	}); err != nil {
		t.Fatalf("persist contradictory occupied target: %v", err)
	}

	_, err = application.AcceptCapabilityProposal(ctx, uow, recorder, recorder, recorder, clock, application.AcceptCapabilityProposalInput{
		ArtifactID: target.ArtifactID, RevisionID: target.RevisionID, Proposal: generated.Proposal,
	})
	if !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("foreign persisted witness replay err = %v, want ErrStoredStateIntegrity", err)
	}
}

func persistProposalTargetWitness(
	t *testing.T,
	ctx context.Context,
	uow application.UnitOfWork,
	recorder peos.Recorder,
	clock *application.FixedClock,
	target engineering.RevisionKey,
	reviewed proposal.Proposal,
	sequence int,
) {
	t.Helper()
	clock.Advance(time.Hour)
	digest, err := reviewed.Content().Digest()
	if err != nil {
		t.Fatal(err)
	}
	revision, err := recorder.RecordAIAssistedCapabilityRevision(
		engineering.CapabilityRevisionInput{
			ArtifactID: target.ArtifactID, RevisionID: target.RevisionID,
			ContentDigest: digest, RecordedAt: clock.Now(),
		},
		reviewed.ProposalDigest(), reviewed.ContextDigest(), proposalSourceValues(reviewed.Sources()),
	)
	if err != nil {
		t.Fatal(err)
	}
	order, err := engineering.NewRevisionOrderMetadata(target, sequence, clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := uow.Do(ctx, func(repos application.Repositories) error {
		if err := repos.Revisions.Put(ctx, revision); err != nil {
			return err
		}
		if err := repos.StructuredContent.Put(ctx, target, reviewed.Content()); err != nil {
			return err
		}
		return repos.RevisionOrder.Put(ctx, order)
	}); err != nil {
		t.Fatalf("persist AI-assisted target witness: %v", err)
	}
}

func assertPersistedProposalSourceIntegrity(
	t *testing.T,
	ctx context.Context,
	uow application.UnitOfWork,
	recorder peos.Recorder,
	clock *application.FixedClock,
	target engineering.RevisionKey,
	reviewed proposal.Proposal,
) {
	t.Helper()
	_, err := application.AcceptCapabilityProposal(
		ctx, uow, recorder, recorder, recorder, clock,
		application.AcceptCapabilityProposalInput{
			ArtifactID: target.ArtifactID, RevisionID: target.RevisionID, Proposal: reviewed,
		},
	)
	if !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("persisted source replay err = %v, want ErrStoredStateIntegrity", err)
	}
}

func seedProposalValidationRun(
	t *testing.T,
	ctx context.Context,
	uow application.UnitOfWork,
	recorder peos.Recorder,
	clock *application.FixedClock,
	suffix string,
) (string, string, string) {
	t.Helper()
	executionID := "ER-" + suffix
	evidenceArtifactID := "EV-" + suffix
	evidenceRevisionID := evidenceArtifactID + "-REV-1"
	clock.Advance(time.Hour)
	if _, err := (application.RecordValidationRunCommand{
		ExecutionID:    executionID,
		PlanArtifactID: scenario.PlanArtifactID, PlanRevisionID: scenario.PlanRevisionID, ActivityKey: "A-1",
		SubjectArtifactID: scenario.CapabilityArtifactID, SubjectRevisionID: scenario.CapabilityRevision2,
		Method: "manual-review", Outcome: "completed",
		EvidenceArtifactID: evidenceArtifactID, EvidenceRevisionID: evidenceRevisionID,
		EvidenceLocator: "https://evidence.example/" + evidenceArtifactID,
	}).Execute(ctx, uow, recorder, recorder, clock); err != nil {
		t.Fatalf("RecordValidationRunCommand: %v", err)
	}
	return executionID, evidenceArtifactID, evidenceRevisionID
}

func proposalClaimInput(id, executionID, evidenceArtifactID, evidenceRevisionID string, at time.Time) engineering.ClaimInput {
	return engineering.ClaimInput{
		ClaimID: id, ScopeArtifactID: scenario.CapabilityArtifactID,
		SubjectArtifactID: scenario.CapabilityArtifactID, SubjectRevisionID: scenario.CapabilityRevision2,
		RequirementArtifactID: "REQ-1", RequirementRevisionID: "REQ-1-REV-1",
		Outcome: "satisfied", Method: "manual-review",
		EvidenceArtifactID: evidenceArtifactID, EvidenceRevisionID: evidenceRevisionID,
		ExecutionID: executionID, Reasoning: "Adversarial persisted Claim graph witness.",
		Timestamp: at, RecordedAt: at,
	}
}

func seedForeignRequirementClaim(
	t *testing.T,
	ctx context.Context,
	uow application.UnitOfWork,
	recorder peos.Recorder,
	clock *application.FixedClock,
) string {
	t.Helper()
	foreign := establishForeignCapability(t, ctx, uow, recorder, clock)
	requirementMemberID := "ACC-REQ-FOREIGN-PROPOSAL"
	clock.Advance(time.Hour)
	if _, err := (application.EstablishRequirementCommand{
		ArtifactID: "REQ-FOREIGN-PROPOSAL", RevisionID: "REQ-FOREIGN-PROPOSAL-REV-1",
		Statement:                    "This Requirement belongs to the foreign capability.",
		SubjectArtifactID:            foreign.ArtifactID,
		SourceCapabilityRevisionID:   foreign.RevisionID,
		SourceAcceptanceCriterionKey: "AC-1",
		AcceptanceRecordID:           &requirementMemberID,
	}).Execute(ctx, uow, recorder, recorder, clock); err != nil {
		t.Fatalf("EstablishRequirementCommand(foreign proposal root): %v", err)
	}

	planMemberID := "ACC-VP-FOREIGN-PROPOSAL"
	clock.Advance(time.Hour)
	if _, err := (application.EstablishValidationPlanCommand{
		ArtifactID: "VP-FOREIGN-PROPOSAL", RevisionID: "VP-FOREIGN-PROPOSAL-REV-1",
		ScopeArtifactID: scenario.CapabilityArtifactID, AcceptanceRecordID: &planMemberID,
		Activities: []application.PlanActivityCommandInput{{
			Key:               "A-FOREIGN-PROPOSAL",
			SubjectArtifactID: scenario.CapabilityArtifactID, SubjectRevisionID: scenario.CapabilityRevision2,
			Method: "manual-review", OutcomeInterpretation: "Confirm the deliberately foreign Requirement link.",
			RequirementArtifactID: "REQ-FOREIGN-PROPOSAL", RequirementRevisionID: "REQ-FOREIGN-PROPOSAL-REV-1",
			ExpectedEvidence: []string{"Reviewer note"},
		}},
	}).Execute(ctx, uow, recorder, recorder, clock); err != nil {
		t.Fatalf("EstablishValidationPlanCommand(foreign proposal root): %v", err)
	}

	clock.Advance(time.Hour)
	if _, err := (application.RecordValidationRunCommand{
		ExecutionID:    "ER-FOREIGN-PROPOSAL",
		PlanArtifactID: "VP-FOREIGN-PROPOSAL", PlanRevisionID: "VP-FOREIGN-PROPOSAL-REV-1",
		ActivityKey:       "A-FOREIGN-PROPOSAL",
		SubjectArtifactID: scenario.CapabilityArtifactID, SubjectRevisionID: scenario.CapabilityRevision2,
		Method: "manual-review", Outcome: "completed",
		EvidenceArtifactID: "EV-FOREIGN-PROPOSAL", EvidenceRevisionID: "EV-FOREIGN-PROPOSAL-REV-1",
		EvidenceLocator: "https://evidence.example/foreign-proposal",
	}).Execute(ctx, uow, recorder, recorder, clock); err != nil {
		t.Fatalf("RecordValidationRunCommand(foreign proposal root): %v", err)
	}

	const claimID = "CLM-FOREIGN-PROPOSAL"
	clock.Advance(time.Hour)
	if _, err := (application.RecordValidationClaimCommand{
		ClaimID: claimID, ScopeArtifactID: scenario.CapabilityArtifactID,
		SubjectArtifactID: scenario.CapabilityArtifactID, SubjectRevisionID: scenario.CapabilityRevision2,
		RequirementArtifactID: "REQ-FOREIGN-PROPOSAL", RequirementRevisionID: "REQ-FOREIGN-PROPOSAL-REV-1",
		Outcome: "satisfied", Method: "manual-review",
		EvidenceArtifactID: "EV-FOREIGN-PROPOSAL", EvidenceRevisionID: "EV-FOREIGN-PROPOSAL-REV-1",
		ExecutionID: "ER-FOREIGN-PROPOSAL", Reasoning: "Structurally valid but rooted in another capability.",
	}).Execute(ctx, uow, recorder, recorder, clock); err != nil {
		t.Fatalf("RecordValidationClaimCommand(foreign proposal root): %v", err)
	}
	return claimID
}

func TestAcceptCapabilityProposalRejectsPersistedSourcesOutsideContextPackMembership(t *testing.T) {
	t.Run("Artifact without Artifact-subject Decision", func(t *testing.T) {
		ctx, uow, recorder, clock := proposalFixture(t)
		generated, err := application.GenerateCapabilityProposal(
			ctx, uow, recorder, recorder, proposal.NewDeterministicGenerator(), scenario.CapabilityArtifactID,
		)
		if err != nil {
			t.Fatal(err)
		}
		forged := proposalWithOutsideSource(t, generated.Proposal, "artifact:"+scenario.CapabilityArtifactID)
		target := engineering.RevisionKey{ArtifactID: scenario.CapabilityArtifactID, RevisionID: "CAP-1-SOURCE-ARTIFACT-NO-DECISION"}
		persistProposalTargetWitness(t, ctx, uow, recorder, clock, target, forged, 3)
		assertPersistedProposalSourceIntegrity(t, ctx, uow, recorder, clock, target, forged)
	})

	t.Run("unaccepted Revision without Decision backing", func(t *testing.T) {
		ctx, uow, recorder, clock := proposalFixture(t)
		generated, err := application.GenerateCapabilityProposal(
			ctx, uow, recorder, recorder, proposal.NewDeterministicGenerator(), scenario.CapabilityArtifactID,
		)
		if err != nil {
			t.Fatal(err)
		}
		clock.Advance(time.Hour)
		const draftRevisionID = "CAP-1-REV-UNBACKED-DRAFT"
		if _, err := (application.ReviseCapabilitySpecificationCommand{
			ArtifactID: scenario.CapabilityArtifactID, RevisionID: draftRevisionID,
			Content: generated.Proposal.Content(),
		}).Execute(ctx, uow, recorder, recorder, clock); err != nil {
			t.Fatal(err)
		}
		forged := proposalWithOutsideSource(
			t, generated.Proposal, "revision:"+scenario.CapabilityArtifactID+"/"+draftRevisionID,
		)
		target := engineering.RevisionKey{ArtifactID: scenario.CapabilityArtifactID, RevisionID: "CAP-1-SOURCE-AFTER-UNBACKED-DRAFT"}
		persistProposalTargetWitness(t, ctx, uow, recorder, clock, target, forged, 4)
		assertPersistedProposalSourceIntegrity(t, ctx, uow, recorder, clock, target, forged)
	})

	t.Run("Execution and Evidence without Claim membership", func(t *testing.T) {
		ctx, uow, recorder, clock := proposalFixture(t)
		generated, err := application.GenerateCapabilityProposal(
			ctx, uow, recorder, recorder, proposal.NewDeterministicGenerator(), scenario.CapabilityArtifactID,
		)
		if err != nil {
			t.Fatal(err)
		}
		executionID, evidenceArtifactID, evidenceRevisionID := seedProposalValidationRun(
			t, ctx, uow, recorder, clock, "UNCLAIMED-PROPOSAL-SOURCE",
		)
		forged := proposalWithOutsideSource(t, generated.Proposal, "record:execution/"+executionID)
		forged = proposalWithOutsideSource(t, forged, "revision:"+evidenceArtifactID+"/"+evidenceRevisionID)
		target := engineering.RevisionKey{ArtifactID: scenario.CapabilityArtifactID, RevisionID: "CAP-1-SOURCE-UNCLAIMED-RUN"}
		persistProposalTargetWitness(t, ctx, uow, recorder, clock, target, forged, 3)
		assertPersistedProposalSourceIntegrity(t, ctx, uow, recorder, clock, target, forged)
	})
}

func TestAcceptCapabilityProposalRejectsPersistedSourcesBackedByCyclicClaimGraph(t *testing.T) {
	ctx, uow, recorder, clock := proposalFixture(t)
	generated, err := application.GenerateCapabilityProposal(
		ctx, uow, recorder, recorder, proposal.NewDeterministicGenerator(), scenario.CapabilityArtifactID,
	)
	if err != nil {
		t.Fatal(err)
	}
	executionID, evidenceArtifactID, evidenceRevisionID := seedProposalValidationRun(
		t, ctx, uow, recorder, clock, "CYCLIC-CLAIM-SOURCE",
	)
	firstInput := proposalClaimInput("CLM-CYCLE-A", executionID, evidenceArtifactID, evidenceRevisionID, clock.Now())
	firstInput.HasCorrection, firstInput.CorrectionKind, firstInput.CorrectionTarget = true, "correct", "CLM-CYCLE-B"
	secondInput := proposalClaimInput("CLM-CYCLE-B", executionID, evidenceArtifactID, evidenceRevisionID, clock.Now())
	secondInput.HasCorrection, secondInput.CorrectionKind, secondInput.CorrectionTarget = true, "replace", "CLM-CYCLE-A"
	first, err := recorder.RecordClaim(firstInput)
	if err != nil {
		t.Fatal(err)
	}
	second, err := recorder.RecordClaim(secondInput)
	if err != nil {
		t.Fatal(err)
	}
	if err := uow.Do(ctx, func(repos application.Repositories) error {
		if err := repos.Records.Put(ctx, first); err != nil {
			return err
		}
		return repos.Records.Put(ctx, second)
	}); err != nil {
		t.Fatal(err)
	}

	forged := proposalWithOutsideSource(t, generated.Proposal, "record:claim/"+first.Key.ID)
	forged = proposalWithOutsideSource(t, forged, "record:execution/"+executionID)
	forged = proposalWithOutsideSource(t, forged, "revision:"+evidenceArtifactID+"/"+evidenceRevisionID)
	target := engineering.RevisionKey{ArtifactID: scenario.CapabilityArtifactID, RevisionID: "CAP-1-SOURCE-CYCLIC-CLAIM"}
	persistProposalTargetWitness(t, ctx, uow, recorder, clock, target, forged, 3)
	assertPersistedProposalSourceIntegrity(t, ctx, uow, recorder, clock, target, forged)
}

func TestAcceptCapabilityProposalRejectsClaimSourceWithForeignRequirementRoot(t *testing.T) {
	ctx, uow, recorder, clock := proposalFixture(t)
	generated, err := application.GenerateCapabilityProposal(
		ctx, uow, recorder, recorder, proposal.NewDeterministicGenerator(), scenario.CapabilityArtifactID,
	)
	if err != nil {
		t.Fatal(err)
	}
	claimID := seedForeignRequirementClaim(t, ctx, uow, recorder, clock)
	forged := proposalWithOutsideSource(t, generated.Proposal, "record:claim/"+claimID)
	target := engineering.RevisionKey{ArtifactID: scenario.CapabilityArtifactID, RevisionID: "CAP-1-SOURCE-FOREIGN-REQUIREMENT-CLAIM"}
	persistProposalTargetWitness(t, ctx, uow, recorder, clock, target, forged, 3)
	assertPersistedProposalSourceIntegrity(t, ctx, uow, recorder, clock, target, forged)
}

func TestProposalWorkflowRejectsCorruptEarlierAIAssistedHistory(t *testing.T) {
	ctx, uow, recorder, clock := proposalFixture(t)
	generated, err := application.GenerateCapabilityProposal(
		ctx, uow, recorder, recorder, proposal.NewDeterministicGenerator(), scenario.CapabilityArtifactID,
	)
	if err != nil {
		t.Fatal(err)
	}
	corrupt := proposalWithOutsideSource(t, generated.Proposal, "artifact:ART-DANGLING-HISTORY-SOURCE")
	corruptKey := engineering.RevisionKey{
		ArtifactID: scenario.CapabilityArtifactID, RevisionID: "CAP-1-CORRUPT-AI-HISTORY",
	}
	persistProposalTargetWitness(t, ctx, uow, recorder, clock, corruptKey, corrupt, 3)

	if _, err := application.GenerateCapabilityProposal(
		ctx, uow, recorder, recorder, proposal.NewDeterministicGenerator(), scenario.CapabilityArtifactID,
	); !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("generate beside corrupt earlier AI act err = %v, want ErrStoredStateIntegrity", err)
	}

	newKey := engineering.RevisionKey{
		ArtifactID: scenario.CapabilityArtifactID, RevisionID: "CAP-1-AFTER-CORRUPT-AI-HISTORY",
	}
	_, err = application.AcceptCapabilityProposal(
		ctx, uow, recorder, recorder, recorder, clock,
		application.AcceptCapabilityProposalInput{
			ArtifactID: newKey.ArtifactID, RevisionID: newKey.RevisionID, Proposal: generated.Proposal,
		},
	)
	if !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("accept beside corrupt earlier AI act err = %v, want ErrStoredStateIntegrity", err)
	}
	assertProposalTargetAbsent(t, ctx, uow, newKey)
}

func TestProposalWorkflowNilInspectorFailsClosedWithoutPanic(t *testing.T) {
	ctx, uow, recorder, clock := proposalFixture(t)
	if _, err := application.GenerateCapabilityProposal(
		ctx, uow, recorder, nil, proposal.NewDeterministicGenerator(), scenario.CapabilityArtifactID,
	); !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("GenerateCapabilityProposal(nil inspector) err = %v, want ErrStoredStateIntegrity", err)
	}

	generated, err := application.GenerateCapabilityProposal(
		ctx, uow, recorder, recorder, proposal.NewDeterministicGenerator(), scenario.CapabilityArtifactID,
	)
	if err != nil {
		t.Fatal(err)
	}
	target := engineering.RevisionKey{
		ArtifactID: scenario.CapabilityArtifactID, RevisionID: "CAP-1-NIL-PROPOSAL-INSPECTOR",
	}
	_, err = application.AcceptCapabilityProposal(
		ctx, uow, recorder, recorder, nil, clock,
		application.AcceptCapabilityProposalInput{
			ArtifactID: target.ArtifactID, RevisionID: target.RevisionID, Proposal: generated.Proposal,
		},
	)
	if !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("AcceptCapabilityProposal(nil inspector) err = %v, want ErrStoredStateIntegrity", err)
	}
	assertProposalTargetAbsent(t, ctx, uow, target)
}

func TestAcceptCapabilityProposalCorruptSourcePrecedesRequestConflict(t *testing.T) {
	ctx, uow, recorder, clock := proposalFixture(t)
	generated, err := application.GenerateCapabilityProposal(
		ctx, uow, recorder, recorder, proposal.NewDeterministicGenerator(), scenario.CapabilityArtifactID,
	)
	if err != nil {
		t.Fatal(err)
	}
	corrupt := proposalWithOutsideSource(t, generated.Proposal, "artifact:ART-DANGLING-PRECEDENCE-SOURCE")
	target := engineering.RevisionKey{
		ArtifactID: scenario.CapabilityArtifactID, RevisionID: "CAP-1-CORRUPT-SOURCE-PRECEDENCE",
	}
	persistProposalTargetWitness(t, ctx, uow, recorder, clock, target, corrupt, 3)
	different, err := proposal.NewProposal(
		generated.ContextPack, generated.Proposal.Content(),
		"A different but statically valid reviewed rationale.", generated.Proposal.Sources(),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = application.AcceptCapabilityProposal(
		ctx, uow, recorder, recorder, recorder, clock,
		application.AcceptCapabilityProposalInput{
			ArtifactID: target.ArtifactID, RevisionID: target.RevisionID, Proposal: different,
		},
	)
	if !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("corrupt source crossed with different request err = %v, want integrity before immutable conflict", err)
	}
}

func TestAcceptCapabilityProposalReplaySurvivesLaterValidClaimGraphChanges(t *testing.T) {
	for _, test := range []struct {
		name       string
		revisionID string
		mutate     func(*testing.T, context.Context, application.UnitOfWork, peos.Recorder, *application.FixedClock)
	}{
		{
			name: "supersession", revisionID: "CAP-1-CLAIM-HISTORY-SUPERSESSION",
			mutate: func(t *testing.T, ctx context.Context, uow application.UnitOfWork, recorder peos.Recorder, clock *application.FixedClock) {
				clock.Advance(time.Hour)
				if _, err := (application.CorrectValidationClaimCommand{
					ClaimID: "CLM-REPLAY-SUPERSESSION", CorrectionTarget: scenario.ClaimForR1, CorrectionKind: "correct",
					ScopeArtifactID:   scenario.CapabilityArtifactID,
					SubjectArtifactID: scenario.CapabilityArtifactID, SubjectRevisionID: scenario.CapabilityRevision2,
					RequirementArtifactID: "REQ-1", RequirementRevisionID: "REQ-1-REV-1",
					Outcome: "inconclusive", Method: "manual-review",
					EvidenceArtifactID: "EV-1", EvidenceRevisionID: "EV-1-REV-1", ExecutionID: "ER-1",
					Reasoning: "A later coherent correction supersedes the selected historical Claim.",
				}).Execute(ctx, uow, recorder, recorder, clock); err != nil {
					t.Fatalf("CorrectValidationClaimCommand: %v", err)
				}
			},
		},
		{
			name: "competing coherent heads", revisionID: "CAP-1-CLAIM-HISTORY-COMPETING-HEADS",
			mutate: func(t *testing.T, ctx context.Context, uow application.UnitOfWork, recorder peos.Recorder, clock *application.FixedClock) {
				for _, claimID := range []string{"CLM-REPLAY-HEAD-A", "CLM-REPLAY-HEAD-B"} {
					clock.Advance(time.Hour)
					if _, err := (application.RecordValidationClaimCommand{
						ClaimID: claimID, ScopeArtifactID: scenario.CapabilityArtifactID,
						SubjectArtifactID: scenario.CapabilityArtifactID, SubjectRevisionID: scenario.CapabilityRevision2,
						RequirementArtifactID: "REQ-1", RequirementRevisionID: "REQ-1-REV-1",
						Outcome: "satisfied", Method: "manual-review",
						EvidenceArtifactID: "EV-1", EvidenceRevisionID: "EV-1-REV-1", ExecutionID: "ER-1",
						Reasoning: "A later coherent competing head does not rewrite the committed proposal source.",
					}).Execute(ctx, uow, recorder, recorder, clock); err != nil {
						t.Fatalf("RecordValidationClaimCommand(%s): %v", claimID, err)
					}
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, uow, recorder, clock := proposalFixture(t)
			generated, err := application.GenerateCapabilityProposal(
				ctx, uow, recorder, recorder, proposal.NewDeterministicGenerator(), scenario.CapabilityArtifactID,
			)
			if err != nil {
				t.Fatal(err)
			}
			reviewed := proposalWithSelectedSources(
				t, generated,
				"revision:"+scenario.CapabilityArtifactID+"/"+scenario.CapabilityRevision2,
				"record:claim/CLM-1", "record:execution/ER-1", "revision:EV-1/EV-1-REV-1",
			)
			target := engineering.RevisionKey{
				ArtifactID: scenario.CapabilityArtifactID, RevisionID: test.revisionID,
			}
			input := application.AcceptCapabilityProposalInput{
				ArtifactID: target.ArtifactID, RevisionID: target.RevisionID, Proposal: reviewed,
			}
			created, err := application.AcceptCapabilityProposal(ctx, uow, recorder, recorder, recorder, clock, input)
			if err != nil {
				t.Fatalf("AcceptCapabilityProposal(create): %v", err)
			}
			test.mutate(t, ctx, uow, recorder, clock)
			replayed, err := application.AcceptCapabilityProposal(ctx, uow, recorder, recorder, recorder, clock, input)
			if err != nil {
				t.Fatalf("AcceptCapabilityProposal(replay after %s): %v", test.name, err)
			}
			if replayed != created {
				t.Fatalf("replay = %#v, want %#v", replayed, created)
			}
		})
	}
}

func proposalWithSelectedSources(
	t *testing.T,
	generated application.GenerateCapabilityProposalResult,
	rawSources ...string,
) proposal.Proposal {
	t.Helper()
	sources := make([]proposal.SourceReference, len(rawSources))
	for index, raw := range rawSources {
		parsed, err := proposal.ParseSourceReference(raw)
		if err != nil {
			t.Fatalf("ParseSourceReference(%q): %v", raw, err)
		}
		sources[index] = parsed
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].String() < sources[j].String() })
	reviewed, err := proposal.NewProposal(
		generated.ContextPack,
		generated.Proposal.Content(),
		"The reviewer selected only the exact sources material to this proposal.",
		sources,
	)
	if err != nil {
		t.Fatalf("NewProposal(selected sources): %v", err)
	}
	return reviewed
}

func TestAcceptCapabilityProposalReplaysEveryValidStandaloneSourceSubset(t *testing.T) {
	ctx, uow, recorder, clock := proposalFixture(t)
	currentContent := currentCapabilityContent(t, ctx, uow, engineering.RevisionKey{
		ArtifactID: scenario.CapabilityArtifactID, RevisionID: scenario.CapabilityRevision2,
	})
	const decisionBackedDraft = "CAP-1-DECISION-BACKED-DRAFT"
	clock.Advance(time.Hour)
	if _, err := (application.ReviseCapabilitySpecificationCommand{
		ArtifactID: scenario.CapabilityArtifactID, RevisionID: decisionBackedDraft, Content: currentContent,
	}).Execute(ctx, uow, recorder, recorder, clock); err != nil {
		t.Fatalf("ReviseCapabilitySpecificationCommand(decision-backed draft): %v", err)
	}
	recordDecision(t, ctx, uow, recorder, clock, "DEC-ARTIFACT-SOURCE", scenario.CapabilityArtifactID, "")
	recordDecision(t, ctx, uow, recorder, clock, "DEC-DRAFT-SOURCE", scenario.CapabilityArtifactID, decisionBackedDraft)
	generated, err := application.GenerateCapabilityProposal(
		ctx, uow, recorder, recorder, proposal.NewDeterministicGenerator(), scenario.CapabilityArtifactID,
	)
	if err != nil {
		t.Fatalf("GenerateCapabilityProposal: %v", err)
	}
	current := "revision:" + generated.ContextPack.Capability().Revision().String()
	for index, test := range []struct {
		name    string
		sources []string
	}{
		{name: "Requirement Revision", sources: []string{"revision:REQ-1/REQ-1-REV-1"}},
		{name: "Requirement trace", sources: []string{"requirement-trace:REQ-1/REQ-1-REV-1"}},
		{name: "criterion", sources: []string{"criterion:CAP-1/CAP-1-REV-2#AC-1"}},
		{name: "Claim", sources: []string{"record:claim/CLM-1"}},
		{name: "Execution", sources: []string{"record:execution/ER-1"}},
		{name: "Evidence", sources: []string{"revision:EV-1/EV-1-REV-1"}},
		{name: "Artifact subject without selecting Decision", sources: []string{"artifact:CAP-1"}},
		{name: "Artifact Decision pair", sources: []string{"artifact:CAP-1", "record:decision/DEC-ARTIFACT-SOURCE"}},
		{name: "unaccepted Decision subject without selecting Decision", sources: []string{"revision:CAP-1/" + decisionBackedDraft}},
		{name: "unaccepted Revision Decision pair", sources: []string{"revision:CAP-1/" + decisionBackedDraft, "record:decision/DEC-DRAFT-SOURCE"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			reviewed := proposalWithSelectedSources(t, generated, append([]string{current}, test.sources...)...)
			target := engineering.RevisionKey{
				ArtifactID: scenario.CapabilityArtifactID,
				RevisionID: fmt.Sprintf("CAP-1-SOURCE-SUBSET-%d", index+1),
			}
			input := application.AcceptCapabilityProposalInput{
				ArtifactID: target.ArtifactID, RevisionID: target.RevisionID, Proposal: reviewed,
			}
			created, err := application.AcceptCapabilityProposal(ctx, uow, recorder, recorder, recorder, clock, input)
			if err != nil {
				t.Fatalf("AcceptCapabilityProposal(create): %v", err)
			}
			replayed, err := application.AcceptCapabilityProposal(ctx, uow, recorder, recorder, recorder, clock, input)
			if err != nil {
				t.Fatalf("AcceptCapabilityProposal(replay): %v", err)
			}
			if replayed != created {
				t.Fatalf("replay = %#v, want %#v", replayed, created)
			}
		})
	}
}

func proposalWithOutsideSource(t *testing.T, original proposal.Proposal, outside string) proposal.Proposal {
	t.Helper()
	canonical, err := original.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	type bodyWire struct {
		Content       json.RawMessage `json:"content"`
		Rationale     string          `json:"rationale"`
		Sources       []string        `json:"sources"`
		ContextDigest string          `json:"context_digest"`
	}
	type proposalWire struct {
		Content        json.RawMessage `json:"content"`
		Rationale      string          `json:"rationale"`
		Sources        []string        `json:"sources"`
		ContextDigest  string          `json:"context_digest"`
		ProposalDigest string          `json:"proposal_digest"`
	}
	var wire proposalWire
	if err := json.Unmarshal(canonical, &wire); err != nil {
		t.Fatal(err)
	}
	wire.Sources = append(wire.Sources, outside)
	sort.Strings(wire.Sources)
	body, err := json.Marshal(bodyWire{
		Content: wire.Content, Rationale: wire.Rationale, Sources: wire.Sources, ContextDigest: wire.ContextDigest,
	})
	if err != nil {
		t.Fatal(err)
	}
	wire.ProposalDigest = engineering.ComputeDigest(body).Hex()
	forgedJSON, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	forged, err := proposal.ParseProposal(forgedJSON)
	if err != nil {
		t.Fatalf("ParseProposal(forged canonical proposal): %v", err)
	}
	return forged
}

func TestAcceptCapabilityProposalRejectsCanonicalSourceOutsideFreshPackWithZeroWrites(t *testing.T) {
	ctx, uow, recorder, clock := proposalFixture(t)
	generated, err := application.GenerateCapabilityProposal(
		ctx, uow, recorder, recorder, proposal.NewDeterministicGenerator(), scenario.CapabilityArtifactID,
	)
	if err != nil {
		t.Fatalf("GenerateCapabilityProposal: %v", err)
	}
	forged := proposalWithOutsideSource(t, generated.Proposal, "artifact:CAP-OUTSIDE-FRESH-PACK")
	if err := forged.Validate(); err != nil {
		t.Fatalf("forged proposal is not state-independently canonical: %v", err)
	}
	if err := forged.ValidateAgainst(generated.ContextPack); err == nil {
		t.Fatal("forged proposal unexpectedly validates against the fresh ContextPack")
	}
	target := engineering.RevisionKey{ArtifactID: scenario.CapabilityArtifactID, RevisionID: "CAP-1-OUTSIDE-SOURCE"}
	_, err = application.AcceptCapabilityProposal(ctx, uow, recorder, recorder, recorder, clock, application.AcceptCapabilityProposalInput{
		ArtifactID: target.ArtifactID, RevisionID: target.RevisionID, Proposal: forged,
	})
	if !errors.Is(err, application.ErrInvalidCommand) {
		t.Fatalf("AcceptCapabilityProposal err = %v, want ErrInvalidCommand", err)
	}
	assertProposalTargetAbsent(t, ctx, uow, target)
}

type corruptRequirementProjection struct {
	application.EngineeringProjector
}

func (p corruptRequirementProjection) ProjectRequirementStatement([]byte) (string, error) {
	return " ", nil
}

func TestAcceptCapabilityProposalFailsClosedWhenFreshContextCannotFormValidPack(t *testing.T) {
	ctx, uow, recorder, clock := proposalFixture(t)
	generated, err := application.GenerateCapabilityProposal(
		ctx, uow, recorder, recorder, proposal.NewDeterministicGenerator(), scenario.CapabilityArtifactID,
	)
	if err != nil {
		t.Fatalf("GenerateCapabilityProposal: %v", err)
	}
	target := engineering.RevisionKey{ArtifactID: scenario.CapabilityArtifactID, RevisionID: "CAP-1-CORRUPT-FRESH-PACK"}
	_, err = application.AcceptCapabilityProposal(
		ctx,
		uow,
		recorder,
		corruptRequirementProjection{EngineeringProjector: recorder},
		recorder,
		clock,
		application.AcceptCapabilityProposalInput{
			ArtifactID: target.ArtifactID, RevisionID: target.RevisionID, Proposal: generated.Proposal,
		},
	)
	if !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("AcceptCapabilityProposal err = %v, want ErrStoredStateIntegrity", err)
	}
	assertProposalTargetAbsent(t, ctx, uow, target)
}

var errSimulatedSerializationRetry = errors.New("simulated serialization retry")

type proposalRetryUnitOfWork struct {
	delegate   application.UnitOfWork
	afterFirst func() error
	attempts   int
}

func (u *proposalRetryUnitOfWork) Do(ctx context.Context, fn func(application.Repositories) error) error {
	first := u.delegate.Do(ctx, func(repos application.Repositories) error {
		u.attempts++
		if err := fn(repos); err != nil {
			return err
		}
		return errSimulatedSerializationRetry
	})
	if !errors.Is(first, errSimulatedSerializationRetry) {
		return first
	}
	if u.afterFirst != nil {
		if err := u.afterFirst(); err != nil {
			return err
		}
	}
	return u.delegate.Do(ctx, func(repos application.Repositories) error {
		u.attempts++
		return fn(repos)
	})
}

type proposalTimeObservingRecorder struct {
	peos.Recorder
	recordedAt []time.Time
}

func (r *proposalTimeObservingRecorder) RecordAIAssistedCapabilityRevision(
	in engineering.CapabilityRevisionInput,
	proposalDigest engineering.Digest,
	contextDigest engineering.Digest,
	sources []string,
) (engineering.RevisionEnvelope, error) {
	r.recordedAt = append(r.recordedAt, in.RecordedAt)
	return r.Recorder.RecordAIAssistedCapabilityRevision(in, proposalDigest, contextDigest, sources)
}

func TestAcceptCapabilityProposalSerializationRetryReusesOneCandidateAndCommitsOnce(t *testing.T) {
	ctx, base, recorder, clock := proposalFixture(t)
	generated, err := application.GenerateCapabilityProposal(
		ctx, base, recorder, recorder, proposal.NewDeterministicGenerator(), scenario.CapabilityArtifactID,
	)
	if err != nil {
		t.Fatalf("GenerateCapabilityProposal: %v", err)
	}
	clock.Advance(time.Hour)
	candidate := clock.Now()
	retry := &proposalRetryUnitOfWork{delegate: base}
	observing := &proposalTimeObservingRecorder{Recorder: recorder}
	target := engineering.RevisionKey{ArtifactID: scenario.CapabilityArtifactID, RevisionID: "CAP-1-RETRY-COMMIT"}
	result, err := application.AcceptCapabilityProposal(ctx, retry, observing, recorder, recorder, clock, application.AcceptCapabilityProposalInput{
		ArtifactID: target.ArtifactID, RevisionID: target.RevisionID, Proposal: generated.Proposal,
	})
	if err != nil {
		t.Fatalf("AcceptCapabilityProposal: %v", err)
	}
	if retry.attempts != 2 || len(observing.recordedAt) != 2 ||
		!observing.recordedAt[0].Equal(candidate) || !observing.recordedAt[1].Equal(candidate) {
		t.Fatalf("attempts=%d recordedAt=%v, want two attempts with one candidate %s", retry.attempts, observing.recordedAt, candidate)
	}
	if result.RevisionKey != target || result.Sequence != 3 {
		t.Fatalf("result = %#v, want one committed sequence-3 target", result)
	}
	if err := base.Do(ctx, func(repos application.Repositories) error {
		revisions, err := repos.Revisions.ListByArtifact(ctx, target.ArtifactID)
		if err != nil {
			return err
		}
		count := 0
		for _, revision := range revisions {
			if revision.Key == target {
				count++
				if !revision.RecordedAt.Equal(candidate) {
					return fmt.Errorf("committed candidate time = %s, want %s", revision.RecordedAt, candidate)
				}
			}
		}
		if count != 1 {
			return fmt.Errorf("committed target count = %d, want 1", count)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAcceptCapabilityProposalSerializationRetryCanBecomeStaleWithNoAct(t *testing.T) {
	ctx, base, recorder, clock := proposalFixture(t)
	generated, err := application.GenerateCapabilityProposal(
		ctx, base, recorder, recorder, proposal.NewDeterministicGenerator(), scenario.CapabilityArtifactID,
	)
	if err != nil {
		t.Fatalf("GenerateCapabilityProposal: %v", err)
	}
	clock.Advance(time.Hour)
	candidate := clock.Now()
	retry := &proposalRetryUnitOfWork{delegate: base}
	retry.afterFirst = func() error {
		clock.Advance(time.Hour)
		acceptanceID := "ACC-REQ-RETRY-FRESHNESS"
		_, err := (application.EstablishRequirementCommand{
			ArtifactID: "REQ-RETRY-FRESHNESS", RevisionID: "REQ-RETRY-FRESHNESS-REV-1",
			Statement:                    "A committed concurrent Requirement SHALL make the retried proposal stale.",
			SubjectArtifactID:            scenario.CapabilityArtifactID,
			SourceCapabilityRevisionID:   scenario.CapabilityRevision2,
			SourceAcceptanceCriterionKey: "AC-1",
			AcceptanceRecordID:           &acceptanceID,
		}).Execute(ctx, base, recorder, recorder, clock)
		return err
	}
	observing := &proposalTimeObservingRecorder{Recorder: recorder}
	target := engineering.RevisionKey{ArtifactID: scenario.CapabilityArtifactID, RevisionID: "CAP-1-RETRY-STALE"}
	_, err = application.AcceptCapabilityProposal(ctx, retry, observing, recorder, recorder, clock, application.AcceptCapabilityProposalInput{
		ArtifactID: target.ArtifactID, RevisionID: target.RevisionID, Proposal: generated.Proposal,
	})
	if !errors.Is(err, application.ErrProposalContextStale) {
		t.Fatalf("AcceptCapabilityProposal err = %v, want ErrProposalContextStale", err)
	}
	if retry.attempts != 2 || len(observing.recordedAt) != 1 || !observing.recordedAt[0].Equal(candidate) {
		t.Fatalf("attempts=%d recordedAt=%v, want first rolled-back construction at sole candidate %s", retry.attempts, observing.recordedAt, candidate)
	}
	assertProposalTargetAbsent(t, ctx, base, target)
}
