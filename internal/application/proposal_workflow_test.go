package application_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering"
	"github.com/aleka7sk/featureforge/internal/engineering/peos"
	"github.com/aleka7sk/featureforge/internal/infrastructure/memory"
	"github.com/aleka7sk/featureforge/internal/proposal"
	"github.com/aleka7sk/featureforge/internal/scenario"
)

func proposalFixture(t *testing.T) (context.Context, *memory.UnitOfWork, peos.Recorder, *application.FixedClock) {
	t.Helper()
	ctx := context.Background()
	uow := memory.NewUnitOfWork(memory.NewStore())
	recorder := peos.NewRecorder()
	clock := application.NewFixedClock(scenario.FixedStart)
	if _, err := scenario.Run(ctx, uow, recorder, recorder, clock); err != nil {
		t.Fatalf("scenario.Run: %v", err)
	}
	return ctx, uow, recorder, clock
}

func TestAssembleProposalContextUsesExactCurrentEngineeringState(t *testing.T) {
	ctx, uow, recorder, _ := proposalFixture(t)

	pack, err := application.AssembleProposalContext(ctx, uow, recorder, recorder, scenario.CapabilityArtifactID)
	if err != nil {
		t.Fatalf("AssembleProposalContext: %v", err)
	}
	if err := pack.Validate(); err != nil {
		t.Fatalf("ContextPack.Validate: %v", err)
	}
	capability := pack.Capability()
	if capability.Revision() != (engineering.RevisionKey{ArtifactID: scenario.CapabilityArtifactID, RevisionID: scenario.CapabilityRevision2}) || capability.Sequence() != 2 {
		t.Fatalf("capability root = %s sequence %d, want CAP-1/CAP-1-REV-2 sequence 2", capability.Revision(), capability.Sequence())
	}
	if got := len(pack.Requirements()); got != 4 {
		t.Fatalf("requirements = %d, want 4", got)
	}
	claims := pack.Claims()
	if got := len(claims); got != 3 {
		t.Fatalf("current claims = %d, want 3", got)
	}
	for _, claim := range claims {
		if claim.RequirementRevision().ArtifactID == "REQ-2" && (claim.Record().ID != scenario.ClaimCorrecting || claim.Outcome() != "not-satisfied") {
			t.Fatalf("REQ-2 claim = %s %q, want corrected CLM-4 not-satisfied", claim.Record(), claim.Outcome())
		}
	}
	decisions := pack.Decisions()
	if len(decisions) != 1 || decisions[0].DecisionID() != scenario.DecisionID ||
		decisions[0].Subject().Kind() != engineering.SubjectKindArtifactRevision ||
		decisions[0].Subject().RevisionID() != scenario.CapabilityRevision1 {
		t.Fatalf("decisions = %#v, want DEC-1 against the exact older CAP-1-REV-1", decisions)
	}
	uncovered := pack.UncoveredCriteria()
	if len(uncovered) != 1 || uncovered[0].CriterionKey() != "AC-4" ||
		uncovered[0].Reason() != proposal.UncoveredMissingCurrentClaim ||
		!slices.Equal(uncovered[0].RequirementRevisions(), []engineering.RevisionKey{{ArtifactID: "REQ-4", RevisionID: "REQ-4-REV-1"}}) {
		t.Fatalf("uncovered = %#v, want AC-4 missing REQ-4 current claim", uncovered)
	}
	findings := pack.Findings()
	if len(findings) != 1 || findings[0].CriterionKey() != "AC-2" || findings[0].ClaimRecord().ID != scenario.ClaimCorrecting || findings[0].Outcome() != proposal.FindingNotSatisfied {
		t.Fatalf("findings = %#v, want corrected AC-2 not-satisfied finding", findings)
	}
	sources := proposalSourceValues(pack.Sources())
	for _, required := range []string{
		"revision:CAP-1/CAP-1-REV-1",
		"revision:CAP-1/CAP-1-REV-2",
		"record:decision/DEC-1",
		"revision:REQ-4/REQ-4-REV-1",
		"requirement-trace:REQ-4/REQ-4-REV-1",
		"criterion:CAP-1/CAP-1-REV-2#AC-4",
	} {
		if !slices.Contains(sources, required) {
			t.Errorf("sources missing %q: %v", required, sources)
		}
	}
}

func TestAssembleProposalContextPreservesOptionalEmptyClaimReasoning(t *testing.T) {
	ctx, uow, recorder, clock := proposalFixture(t)
	clock.Advance(time.Hour)
	const correctedID = "CLM-EMPTY-REASON"
	if _, err := (application.CorrectValidationClaimCommand{
		ClaimID: correctedID, CorrectionTarget: scenario.ClaimForR1, CorrectionKind: "correct",
		ScopeArtifactID:   scenario.CapabilityArtifactID,
		SubjectArtifactID: scenario.CapabilityArtifactID, SubjectRevisionID: scenario.CapabilityRevision2,
		RequirementArtifactID: "REQ-1", RequirementRevisionID: "REQ-1-REV-1",
		Outcome: "satisfied", Method: "manual-review",
		EvidenceArtifactID: "EV-1", EvidenceRevisionID: "EV-1-REV-1", ExecutionID: "ER-1",
	}).Execute(ctx, uow, recorder, recorder, clock); err != nil {
		t.Fatalf("CorrectValidationClaimCommand: %v", err)
	}
	pack, err := application.AssembleProposalContext(ctx, uow, recorder, recorder, scenario.CapabilityArtifactID)
	if err != nil {
		t.Fatalf("AssembleProposalContext: %v", err)
	}
	for _, claim := range pack.Claims() {
		if claim.RequirementRevision().ArtifactID != "REQ-1" {
			continue
		}
		if claim.Record().ID != correctedID || claim.Reasoning() != "" {
			t.Fatalf("REQ-1 claim = %s reasoning %q, want current %s with exact empty reasoning", claim.Record(), claim.Reasoning(), correctedID)
		}
		return
	}
	t.Fatal("REQ-1 current claim missing from ContextPack")
}

type orderCheckingUOW struct {
	delegate application.UnitOfWork
	active   bool
	calls    int
}

func (u *orderCheckingUOW) Do(ctx context.Context, fn func(application.Repositories) error) error {
	u.calls++
	u.active = true
	defer func() { u.active = false }()
	return u.delegate.Do(ctx, fn)
}

type orderCheckingGenerator struct {
	uow    *orderCheckingUOW
	called bool
}

func (g *orderCheckingGenerator) Generate(pack proposal.ContextPack) (proposal.Proposal, error) {
	if g.uow.active {
		return proposal.Proposal{}, fmt.Errorf("generator called inside UnitOfWork")
	}
	g.called = true
	return proposal.NewDeterministicGenerator().Generate(pack)
}

func TestGenerateCapabilityProposalClosesReadUnitOfWorkBeforeGenerator(t *testing.T) {
	ctx, delegate, recorder, _ := proposalFixture(t)
	uow := &orderCheckingUOW{delegate: delegate}
	generator := &orderCheckingGenerator{uow: uow}

	result, err := application.GenerateCapabilityProposal(ctx, uow, recorder, recorder, generator, scenario.CapabilityArtifactID)
	if err != nil {
		t.Fatalf("GenerateCapabilityProposal: %v", err)
	}
	if uow.calls != 1 || !generator.called {
		t.Fatalf("Do calls = %d, generator called = %v; want one closed read and one call", uow.calls, generator.called)
	}
	if err := result.Proposal.ValidateAgainst(result.ContextPack); err != nil {
		t.Fatalf("generated Proposal.ValidateAgainst: %v", err)
	}
}

func TestGenerateCapabilityProposalReturnsNotFoundForAbsentCapability(t *testing.T) {
	ctx, uow, recorder, _ := proposalFixture(t)

	_, err := application.GenerateCapabilityProposal(
		ctx,
		uow,
		recorder,
		recorder,
		proposal.NewDeterministicGenerator(),
		"CAP-ABSENT",
	)
	if !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("GenerateCapabilityProposal(absent) err = %v, want ErrNotFound", err)
	}
}

func TestAcceptCapabilityProposalCreatesDraftThenReplaysBeforeFreshness(t *testing.T) {
	ctx, uow, recorder, clock := proposalFixture(t)
	generated, err := application.GenerateCapabilityProposal(ctx, uow, recorder, recorder, proposal.NewDeterministicGenerator(), scenario.CapabilityArtifactID)
	if err != nil {
		t.Fatalf("GenerateCapabilityProposal: %v", err)
	}
	clock.Advance(time.Hour)
	input := application.AcceptCapabilityProposalInput{
		ArtifactID: scenario.CapabilityArtifactID,
		RevisionID: "CAP-1-REV-3",
		Proposal:   generated.Proposal,
	}
	created, err := application.AcceptCapabilityProposal(ctx, uow, recorder, recorder, recorder, clock, input)
	if err != nil {
		t.Fatalf("AcceptCapabilityProposal(create): %v", err)
	}
	if created.Sequence != 3 || created.RevisionKey.RevisionID != input.RevisionID {
		t.Fatalf("create result = %#v, want sequence-3 CAP-1-REV-3", created)
	}

	err = uow.Do(ctx, func(repos application.Repositories) error {
		revision, found, err := repos.Revisions.Get(ctx, created.RevisionKey)
		if err != nil || !found {
			return fmt.Errorf("created revision lookup: found=%v err=%v", found, err)
		}
		content, found, err := repos.StructuredContent.Get(ctx, created.RevisionKey)
		if err != nil || !found || !content.Equal(generated.Proposal.Content()) {
			return fmt.Errorf("created content lookup: found=%v err=%v", found, err)
		}
		order, found, err := repos.RevisionOrder.Get(ctx, created.RevisionKey)
		if err != nil || !found || order.Sequence != 3 {
			return fmt.Errorf("created order lookup: found=%v sequence=%d err=%v", found, order.Sequence, err)
		}
		journal, err := repos.RevisionAcceptance.ListByRevision(ctx, created.RevisionKey)
		if err != nil || len(journal) != 0 {
			return fmt.Errorf("created proposal revision is not draft by absence: %v, %v", journal, err)
		}
		trace, found, err := repos.RequirementTraces.Get(ctx, created.RevisionKey)
		if err != nil || found || !trace.IsZero() {
			return fmt.Errorf("created capability revision has a Requirement trace: found=%v trace=%#v err=%v", found, trace, err)
		}
		proposalDigest, contextDigest, sources, assisted, err := recorder.InspectAIAssistedCapabilityRevision(revision)
		if err != nil || !assisted || !proposalDigest.Equal(generated.Proposal.ProposalDigest()) ||
			!contextDigest.Equal(generated.Proposal.ContextDigest()) || !slices.Equal(sources, proposalSourceValues(generated.Proposal.Sources())) {
			return fmt.Errorf("persisted AI witness mismatch: assisted=%v err=%v", assisted, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	clock.Advance(time.Hour)
	if _, err := (application.AcceptCapabilityRevisionCommand{
		RecordID: "ACC-PROPOSAL-3", ArtifactID: scenario.CapabilityArtifactID,
		RevisionID: input.RevisionID, State: engineering.AcceptanceStateAccepted,
	}).Execute(ctx, uow, recorder, clock); err != nil {
		t.Fatalf("accept proposed draft through C5: %v", err)
	}

	replayed, err := application.AcceptCapabilityProposal(ctx, uow, recorder, recorder, recorder, clock, input)
	if err != nil {
		t.Fatalf("AcceptCapabilityProposal(replay after freshness changed): %v", err)
	}
	if replayed != created {
		t.Fatalf("replay result = %#v, want original %#v", replayed, created)
	}

	changed, err := proposal.NewProposal(generated.ContextPack, generated.Proposal.Content(), "changed reviewed rationale", generated.Proposal.Sources())
	if err != nil {
		t.Fatalf("NewProposal(changed): %v", err)
	}
	input.Proposal = changed
	if _, err := application.AcceptCapabilityProposal(ctx, uow, recorder, recorder, recorder, clock, input); !errors.Is(err, application.ErrImmutableValueConflict) {
		t.Fatalf("changed occupied proposal err = %v, want ErrImmutableValueConflict", err)
	}

	input.RevisionID = "CAP-1-REV-4"
	input.Proposal = generated.Proposal
	if _, err := application.AcceptCapabilityProposal(ctx, uow, recorder, recorder, recorder, clock, input); !errors.Is(err, application.ErrProposalContextStale) {
		t.Fatalf("absent target with old context err = %v, want ErrProposalContextStale", err)
	}
	err = uow.Do(ctx, func(repos application.Repositories) error {
		_, found, err := repos.Revisions.Get(ctx, engineering.RevisionKey{ArtifactID: input.ArtifactID, RevisionID: input.RevisionID})
		if err != nil {
			return err
		}
		if found {
			return fmt.Errorf("stale proposal wrote its target revision")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

type overridingUOW struct {
	delegate application.UnitOfWork
	mutate   func(*application.Repositories)
}

func (u overridingUOW) Do(ctx context.Context, fn func(application.Repositories) error) error {
	return u.delegate.Do(ctx, func(repos application.Repositories) error {
		u.mutate(&repos)
		return fn(repos)
	})
}

type strayTraceRepository struct {
	application.RequirementCriterionTraceRepository
	target engineering.RevisionKey
	trace  engineering.RequirementCriterionTrace
}

func (r strayTraceRepository) Get(ctx context.Context, key engineering.RevisionKey) (engineering.RequirementCriterionTrace, bool, error) {
	if key == r.target {
		return r.trace, true, nil
	}
	return r.RequirementCriterionTraceRepository.Get(ctx, key)
}

func TestAcceptCapabilityProposalTreatsStrayTargetTraceAsIntegrity(t *testing.T) {
	ctx, uow, recorder, clock := proposalFixture(t)
	generated, err := application.GenerateCapabilityProposal(ctx, uow, recorder, recorder, proposal.NewDeterministicGenerator(), scenario.CapabilityArtifactID)
	if err != nil {
		t.Fatalf("GenerateCapabilityProposal: %v", err)
	}
	target := engineering.RevisionKey{ArtifactID: scenario.CapabilityArtifactID, RevisionID: "CAP-1-REV-STRAY"}
	trace, err := engineering.NewRequirementCriterionTrace(
		target,
		engineering.RevisionKey{ArtifactID: scenario.CapabilityArtifactID, RevisionID: scenario.CapabilityRevision2},
		"AC-1",
		clock.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	override := overridingUOW{delegate: uow, mutate: func(repos *application.Repositories) {
		repos.RequirementTraces = strayTraceRepository{
			RequirementCriterionTraceRepository: repos.RequirementTraces,
			target:                              target,
			trace:                               trace,
		}
	}}
	_, err = application.AcceptCapabilityProposal(ctx, override, recorder, recorder, recorder, clock, application.AcceptCapabilityProposalInput{
		ArtifactID: target.ArtifactID, RevisionID: target.RevisionID, Proposal: generated.Proposal,
	})
	if !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("stray target trace err = %v, want ErrStoredStateIntegrity", err)
	}
}

func TestAcceptCapabilityProposalRejectsCompleteNonProposalOccupants(t *testing.T) {
	ctx, uow, recorder, clock := proposalFixture(t)
	generated, err := application.GenerateCapabilityProposal(ctx, uow, recorder, recorder, proposal.NewDeterministicGenerator(), scenario.CapabilityArtifactID)
	if err != nil {
		t.Fatalf("GenerateCapabilityProposal: %v", err)
	}
	for _, target := range []engineering.RevisionKey{
		{ArtifactID: scenario.CapabilityArtifactID, RevisionID: scenario.CapabilityRevision2},
		{ArtifactID: "REQ-1", RevisionID: "REQ-1-REV-1"},
	} {
		_, err := application.AcceptCapabilityProposal(ctx, uow, recorder, recorder, recorder, clock, application.AcceptCapabilityProposalInput{
			ArtifactID: target.ArtifactID, RevisionID: target.RevisionID, Proposal: generated.Proposal,
		})
		if !errors.Is(err, application.ErrImmutableValueConflict) {
			t.Errorf("occupied target %s err = %v, want ErrImmutableValueConflict", target, err)
		}
	}
}

func proposalSourceValues(sources []proposal.SourceReference) []string {
	result := make([]string, len(sources))
	for index, source := range sources {
		result[index] = source.String()
	}
	return result
}
