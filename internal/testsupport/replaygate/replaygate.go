// Package replaygate provides a test-only UnitOfWork decorator that can
// reject every repository mutation while leaving reads untouched. It lets
// transport and adapter conformance tests prove replay made zero write calls,
// not merely that idempotent adapter writes had no net effect.
package replaygate

import (
	"context"
	"errors"
	"sync"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/domain"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

var ErrWriteAttempt = errors.New("replay attempted a repository mutation")

type Gate struct {
	base          application.UnitOfWork
	mu            sync.Mutex
	rejectWrites  bool
	writeAttempts int
}

func New(base application.UnitOfWork) *Gate { return &Gate{base: base} }

func (g *Gate) RejectWrites() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.rejectWrites = true
	g.writeAttempts = 0
}

func (g *Gate) WriteAttempts() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.writeAttempts
}

func (g *Gate) Do(ctx context.Context, fn func(application.Repositories) error) error {
	return g.base.Do(ctx, func(r application.Repositories) error { return fn(g.wrap(r)) })
}

func (g *Gate) beforeWrite() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.writeAttempts++
	if g.rejectWrites {
		return ErrWriteAttempt
	}
	return nil
}

func (g *Gate) wrap(r application.Repositories) application.Repositories {
	return application.Repositories{
		Projects:           projectRepository{ProjectRepository: r.Projects, gate: g},
		FeatureCards:       featureCardRepository{FeatureCardRepository: r.FeatureCards, gate: g},
		Artifacts:          artifactRepository{ArtifactEnvelopeRepository: r.Artifacts, gate: g},
		Revisions:          revisionRepository{RevisionEnvelopeRepository: r.Revisions, gate: g},
		StructuredContent:  structuredContentRepository{StructuredContentRepository: r.StructuredContent, gate: g},
		Records:            recordRepository{RecordEnvelopeRepository: r.Records, gate: g},
		RevisionOrder:      revisionOrderRepository{RevisionOrderRepository: r.RevisionOrder, gate: g},
		RevisionAcceptance: revisionAcceptanceRepository{RevisionAcceptanceRepository: r.RevisionAcceptance, gate: g},
	}
}

type projectRepository struct {
	application.ProjectRepository
	gate *Gate
}

func (r projectRepository) Put(ctx context.Context, value domain.Project) error {
	if err := r.gate.beforeWrite(); err != nil {
		return err
	}
	return r.ProjectRepository.Put(ctx, value)
}

type featureCardRepository struct {
	application.FeatureCardRepository
	gate *Gate
}

func (r featureCardRepository) Put(ctx context.Context, value domain.FeatureCard) error {
	if err := r.gate.beforeWrite(); err != nil {
		return err
	}
	return r.FeatureCardRepository.Put(ctx, value)
}

func (r featureCardRepository) LinkCapability(ctx context.Context, id domain.FeatureCardID, artifactID string) error {
	if err := r.gate.beforeWrite(); err != nil {
		return err
	}
	return r.FeatureCardRepository.LinkCapability(ctx, id, artifactID)
}

type artifactRepository struct {
	application.ArtifactEnvelopeRepository
	gate *Gate
}

func (r artifactRepository) Put(ctx context.Context, value engineering.ArtifactEnvelope) error {
	if err := r.gate.beforeWrite(); err != nil {
		return err
	}
	return r.ArtifactEnvelopeRepository.Put(ctx, value)
}

type revisionRepository struct {
	application.RevisionEnvelopeRepository
	gate *Gate
}

func (r revisionRepository) Put(ctx context.Context, value engineering.RevisionEnvelope) error {
	if err := r.gate.beforeWrite(); err != nil {
		return err
	}
	return r.RevisionEnvelopeRepository.Put(ctx, value)
}

type structuredContentRepository struct {
	application.StructuredContentRepository
	gate *Gate
}

func (r structuredContentRepository) Put(ctx context.Context, key engineering.RevisionKey, value engineering.CapabilitySpecificationContent) error {
	if err := r.gate.beforeWrite(); err != nil {
		return err
	}
	return r.StructuredContentRepository.Put(ctx, key, value)
}

type recordRepository struct {
	application.RecordEnvelopeRepository
	gate *Gate
}

func (r recordRepository) Put(ctx context.Context, value engineering.RecordEnvelope) error {
	if err := r.gate.beforeWrite(); err != nil {
		return err
	}
	return r.RecordEnvelopeRepository.Put(ctx, value)
}

type revisionOrderRepository struct {
	application.RevisionOrderRepository
	gate *Gate
}

func (r revisionOrderRepository) Put(ctx context.Context, value engineering.RevisionOrderMetadata) error {
	if err := r.gate.beforeWrite(); err != nil {
		return err
	}
	return r.RevisionOrderRepository.Put(ctx, value)
}

type revisionAcceptanceRepository struct {
	application.RevisionAcceptanceRepository
	gate *Gate
}

func (r revisionAcceptanceRepository) Append(ctx context.Context, value engineering.RevisionAcceptanceRecord) error {
	if err := r.gate.beforeWrite(); err != nil {
		return err
	}
	return r.RevisionAcceptanceRepository.Append(ctx, value)
}
