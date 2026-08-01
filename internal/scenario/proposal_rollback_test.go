package scenario_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering"
	peos "github.com/aleka7sk/featureforge/internal/engineering/peos"
	"github.com/aleka7sk/featureforge/internal/proposal"
	"github.com/aleka7sk/featureforge/internal/scenario"
)

type proposalRollbackFixture func(*testing.T) (application.UnitOfWork, peos.Recorder, *application.FixedClock)

type proposalPutStage string

const (
	proposalRevisionPut proposalPutStage = "revision"
	proposalContentPut  proposalPutStage = "structured-content"
	proposalOrderPut    proposalPutStage = "revision-order"
)

var errInjectedProposalPut = errors.New("injected AI proposal repository Put failure")

// TestAIProposalAcceptRollbackMemory and its PostgreSQL twin deliberately run
// one shared assertion body. The only variable is the UnitOfWork adapter.
func TestAIProposalAcceptRollbackMemory(t *testing.T) {
	assertAIProposalAcceptRollback(t, func(*testing.T) (application.UnitOfWork, peos.Recorder, *application.FixedClock) {
		return newFixture()
	})
}

func TestAIProposalAcceptRollbackPostgres(t *testing.T) {
	assertAIProposalAcceptRollback(t, newPostgresFixture)
}

func assertAIProposalAcceptRollback(t *testing.T, fixture proposalRollbackFixture) {
	t.Helper()
	for _, test := range []struct {
		name             string
		stage            proposalPutStage
		wantRevisionPuts int
		wantContentPuts  int
		wantOrderPuts    int
	}{
		{name: "revision_envelope", stage: proposalRevisionPut, wantRevisionPuts: 1},
		{name: "structured_content", stage: proposalContentPut, wantRevisionPuts: 1, wantContentPuts: 1},
		{name: "revision_order", stage: proposalOrderPut, wantRevisionPuts: 1, wantContentPuts: 1, wantOrderPuts: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			uow, recorder, clock := fixture(t)
			canonical, err := scenario.Run(ctx, uow, recorder, recorder, clock)
			if err != nil {
				t.Fatalf("scenario.Run: %v", err)
			}
			generated, err := application.GenerateCapabilityProposal(
				ctx,
				uow,
				recorder,
				recorder,
				proposal.NewDeterministicGenerator(),
				scenario.CapabilityArtifactID,
			)
			if err != nil {
				t.Fatalf("GenerateCapabilityProposal: %v", err)
			}
			clock.Advance(time.Hour)

			target := engineering.RevisionKey{
				ArtifactID: scenario.CapabilityArtifactID,
				RevisionID: "CAP-1-REV-3",
			}
			before := canonicalPersistenceSnapshot(t, ctx, uow, canonical)
			faults := &proposalPutFaultUOW{
				delegate: uow,
				target:   target,
				stage:    test.stage,
				failure:  fmt.Errorf("%w: %s", errInjectedProposalPut, test.stage),
			}

			_, err = application.AcceptCapabilityProposal(
				ctx,
				faults,
				recorder,
				recorder,
				recorder,
				clock,
				application.AcceptCapabilityProposalInput{
					ArtifactID: target.ArtifactID,
					RevisionID: target.RevisionID,
					Proposal:   generated.Proposal,
				},
			)
			if !errors.Is(err, errInjectedProposalPut) {
				t.Fatalf("AcceptCapabilityProposal error = %v, want injected Put error", err)
			}
			if faults.revisionPuts != test.wantRevisionPuts ||
				faults.contentPuts != test.wantContentPuts ||
				faults.orderPuts != test.wantOrderPuts {
				t.Fatalf(
					"target Put attempts = revision:%d content:%d order:%d, want revision:%d content:%d order:%d",
					faults.revisionPuts,
					faults.contentPuts,
					faults.orderPuts,
					test.wantRevisionPuts,
					test.wantContentPuts,
					test.wantOrderPuts,
				)
			}
			if faults.acceptanceAppends != 0 || faults.tracePuts != 0 {
				t.Fatalf(
					"proposal acceptance attempted unrelated target writes: journal=%d trace=%d",
					faults.acceptanceAppends,
					faults.tracePuts,
				)
			}

			assertProposalTargetAbsent(t, ctx, uow, target)
			after := canonicalPersistenceSnapshot(t, ctx, uow, canonical)
			if !bytes.Equal(after, before) {
				t.Fatal("canonical persisted state changed after failed proposal acceptance")
			}
		})
	}
}

// proposalPutFaultUOW replaces only the three repositories written by a
// successful proposal acceptance. Returning a sentinel from one selected Put
// exercises the real adapter transaction and therefore its rollback path.
type proposalPutFaultUOW struct {
	delegate application.UnitOfWork
	target   engineering.RevisionKey
	stage    proposalPutStage
	failure  error

	revisionPuts      int
	contentPuts       int
	orderPuts         int
	acceptanceAppends int
	tracePuts         int
}

func (u *proposalPutFaultUOW) Do(ctx context.Context, fn func(application.Repositories) error) error {
	return u.delegate.Do(ctx, func(repositories application.Repositories) error {
		repositories.Revisions = proposalFaultRevisionRepository{
			RevisionEnvelopeRepository: repositories.Revisions,
			uow:                        u,
		}
		repositories.StructuredContent = proposalFaultContentRepository{
			StructuredContentRepository: repositories.StructuredContent,
			uow:                         u,
		}
		repositories.RevisionOrder = proposalFaultOrderRepository{
			RevisionOrderRepository: repositories.RevisionOrder,
			uow:                     u,
		}
		repositories.RevisionAcceptance = proposalObservingAcceptanceRepository{
			RevisionAcceptanceRepository: repositories.RevisionAcceptance,
			uow:                          u,
		}
		repositories.RequirementTraces = proposalObservingTraceRepository{
			RequirementCriterionTraceRepository: repositories.RequirementTraces,
			uow:                                 u,
		}
		return fn(repositories)
	})
}

type proposalFaultRevisionRepository struct {
	application.RevisionEnvelopeRepository
	uow *proposalPutFaultUOW
}

func (r proposalFaultRevisionRepository) Put(ctx context.Context, revision engineering.RevisionEnvelope) error {
	if revision.Key == r.uow.target {
		r.uow.revisionPuts++
		if r.uow.stage == proposalRevisionPut {
			return r.uow.failure
		}
	}
	return r.RevisionEnvelopeRepository.Put(ctx, revision)
}

type proposalFaultContentRepository struct {
	application.StructuredContentRepository
	uow *proposalPutFaultUOW
}

func (r proposalFaultContentRepository) Put(ctx context.Context, key engineering.RevisionKey, content engineering.CapabilitySpecificationContent) error {
	if key == r.uow.target {
		r.uow.contentPuts++
		if r.uow.stage == proposalContentPut {
			return r.uow.failure
		}
	}
	return r.StructuredContentRepository.Put(ctx, key, content)
}

type proposalFaultOrderRepository struct {
	application.RevisionOrderRepository
	uow *proposalPutFaultUOW
}

func (r proposalFaultOrderRepository) Put(ctx context.Context, order engineering.RevisionOrderMetadata) error {
	if order.Key == r.uow.target {
		r.uow.orderPuts++
		if r.uow.stage == proposalOrderPut {
			return r.uow.failure
		}
	}
	return r.RevisionOrderRepository.Put(ctx, order)
}

type proposalObservingAcceptanceRepository struct {
	application.RevisionAcceptanceRepository
	uow *proposalPutFaultUOW
}

func (r proposalObservingAcceptanceRepository) Append(ctx context.Context, record engineering.RevisionAcceptanceRecord) error {
	if record.Key == r.uow.target {
		r.uow.acceptanceAppends++
	}
	return r.RevisionAcceptanceRepository.Append(ctx, record)
}

type proposalObservingTraceRepository struct {
	application.RequirementCriterionTraceRepository
	uow *proposalPutFaultUOW
}

func (r proposalObservingTraceRepository) Put(ctx context.Context, trace engineering.RequirementCriterionTrace) error {
	if trace.RequirementRevision == r.uow.target {
		r.uow.tracePuts++
	}
	return r.RequirementCriterionTraceRepository.Put(ctx, trace)
}

func assertProposalTargetAbsent(t *testing.T, ctx context.Context, uow application.UnitOfWork, target engineering.RevisionKey) {
	t.Helper()
	err := uow.Do(ctx, func(repositories application.Repositories) error {
		revision, revisionFound, err := repositories.Revisions.Get(ctx, target)
		if err != nil {
			return fmt.Errorf("get target revision: %w", err)
		}
		if revisionFound || !revision.Key.IsZero() || len(revision.Payload) != 0 {
			return fmt.Errorf("target RevisionEnvelope survived rollback: found=%v value=%#v", revisionFound, revision)
		}

		content, contentFound, err := repositories.StructuredContent.Get(ctx, target)
		if err != nil {
			return fmt.Errorf("get target structured content: %w", err)
		}
		if contentFound || !content.IsZero() {
			return fmt.Errorf("target CapabilitySpecificationContent survived rollback: found=%v value=%#v", contentFound, content)
		}

		order, orderFound, err := repositories.RevisionOrder.Get(ctx, target)
		if err != nil {
			return fmt.Errorf("get target revision order: %w", err)
		}
		if orderFound || !order.Key.IsZero() || order.Sequence != 0 || !order.RecordedAt.IsZero() {
			return fmt.Errorf("target RevisionOrderMetadata survived rollback: found=%v value=%#v", orderFound, order)
		}

		journal, err := repositories.RevisionAcceptance.ListByRevision(ctx, target)
		if err != nil {
			return fmt.Errorf("list target acceptance journal: %w", err)
		}
		if len(journal) != 0 {
			return fmt.Errorf("target acceptance journal survived rollback: %#v", journal)
		}

		trace, traceFound, err := repositories.RequirementTraces.Get(ctx, target)
		if err != nil {
			return fmt.Errorf("get target requirement trace: %w", err)
		}
		if traceFound || !trace.IsZero() {
			return fmt.Errorf("target RequirementCriterionTrace survived rollback: found=%v value=%#v", traceFound, trace)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
