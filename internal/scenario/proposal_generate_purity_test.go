package scenario_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering/peos"
	"github.com/aleka7sk/featureforge/internal/infrastructure/memory"
	"github.com/aleka7sk/featureforge/internal/proposal"
	"github.com/aleka7sk/featureforge/internal/scenario"
)

var errProposalGeneratorWitness = errors.New("proposal generator witness failure")

type failingProposalGenerator struct {
	called bool
}

func (g *failingProposalGenerator) Generate(proposal.ContextPack) (proposal.Proposal, error) {
	g.called = true
	return proposal.Proposal{}, errProposalGeneratorWitness
}

func TestProposalGenerationSuccessAndGeneratorFailureLeaveWholeStoreByteIdentical(t *testing.T) {
	ctx := context.Background()
	uow := memory.NewUnitOfWork(memory.NewStore())
	recorder := peos.NewRecorder()
	clock := application.NewFixedClock(scenario.FixedStart)
	result, err := scenario.Run(ctx, uow, recorder, recorder, clock)
	if err != nil {
		t.Fatalf("scenario.Run: %v", err)
	}
	before := canonicalPersistenceSnapshot(t, ctx, uow, result)

	first, err := application.GenerateCapabilityProposal(
		ctx, uow, recorder, recorder, proposal.NewDeterministicGenerator(), scenario.CapabilityArtifactID,
	)
	if err != nil {
		t.Fatalf("GenerateCapabilityProposal(first): %v", err)
	}
	second, err := application.GenerateCapabilityProposal(
		ctx, uow, recorder, recorder, proposal.NewDeterministicGenerator(), scenario.CapabilityArtifactID,
	)
	if err != nil {
		t.Fatalf("GenerateCapabilityProposal(second): %v", err)
	}
	firstContext, err := first.ContextPack.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	secondContext, err := second.ContextPack.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	firstProposal, err := first.Proposal.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	secondProposal, err := second.Proposal.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstContext, secondContext) || !bytes.Equal(firstProposal, secondProposal) {
		t.Fatal("two reads of unchanged persisted state did not produce canonical-equal outputs")
	}
	if afterSuccess := canonicalPersistenceSnapshot(t, ctx, uow, result); !bytes.Equal(afterSuccess, before) {
		t.Fatal("successful generation changed the closed-world persisted store")
	}

	failing := &failingProposalGenerator{}
	if _, err := application.GenerateCapabilityProposal(
		ctx, uow, recorder, recorder, failing, scenario.CapabilityArtifactID,
	); !errors.Is(err, errProposalGeneratorWitness) {
		t.Fatalf("GenerateCapabilityProposal(failing) err = %v, want exact generator error", err)
	}
	if !failing.called {
		t.Fatal("failing generator was not invoked")
	}
	if afterFailure := canonicalPersistenceSnapshot(t, ctx, uow, result); !bytes.Equal(afterFailure, before) {
		t.Fatal("generator failure changed the closed-world persisted store")
	}
}
