package application

import (
	"context"

	"github.com/aleka7sk/featureforge/internal/domain"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

// Every repository below is family-specific (FF-009 §5): no generic
// Repository[T], no bare map or interface{} in any signature, and no PEOS
// type. No repository exposes an update or delete method -- immutable
// records and journal entries are insert-only/append-only, structurally.

// ProjectRepository persists Projects. Put is create-only.
type ProjectRepository interface {
	Put(ctx context.Context, p domain.Project) error
	Get(ctx context.Context, id domain.ProjectID) (domain.Project, bool, error)
	List(ctx context.Context) ([]domain.Project, error)
}

// FeatureCardRepository persists FeatureCards. Put is create-only, and
// returns ErrReferencedValueMissing if the card's ProjectID does not name a
// stored Project (AD-021).
//
// LinkCapability is a narrowly-scoped addition beyond FF-009 §5's table,
// documented in the M.3 implementation report: it is the one-time monotonic
// completion of a card's capability link, required because ArtifactEnvelope
// carries no FeatureCardID field and Put's create-only semantics cannot
// re-establish an existing card with different content. It is idempotent if
// the same artifact is already linked, and returns ErrImmutableValueConflict
// if a different one is.
type FeatureCardRepository interface {
	Put(ctx context.Context, c domain.FeatureCard) error
	Get(ctx context.Context, id domain.FeatureCardID) (domain.FeatureCard, bool, error)
	ListByProject(ctx context.Context, projectID domain.ProjectID) ([]domain.FeatureCard, error)
	LinkCapability(ctx context.Context, id domain.FeatureCardID, artifactID string) error
}

// ArtifactEnvelopeRepository persists ArtifactEnvelopes.
type ArtifactEnvelopeRepository interface {
	Put(ctx context.Context, env engineering.ArtifactEnvelope) error
	Get(ctx context.Context, key engineering.ArtifactKey) (engineering.ArtifactEnvelope, bool, error)
}

// RevisionEnvelopeRepository persists RevisionEnvelopes. ListByArtifact
// returns entries ordered by key ascending (FF-009 §5).
//
// ListByFamilyAndSubject returns every revision of family whose SubjectKey
// equals subjectKey exactly, ordered by RevisionKey.String() ascending, like
// ListByArtifact (AD-025, FF-016 §4). Revisions with no subject (§3.3) never
// match a non-empty subjectKey, and a non-matching or empty result is an
// empty slice with a nil error, never ErrNotFound. It never decodes a PEOS
// payload; it reads only the projected SubjectKey.
type RevisionEnvelopeRepository interface {
	Put(ctx context.Context, env engineering.RevisionEnvelope) error
	Get(ctx context.Context, key engineering.RevisionKey) (engineering.RevisionEnvelope, bool, error)
	ListByArtifact(ctx context.Context, artifactID string) ([]engineering.RevisionEnvelope, error)
	ListByFamilyAndSubject(ctx context.Context, family engineering.RevisionFamily, subjectKey string) ([]engineering.RevisionEnvelope, error)
}

// StructuredContentRepository persists CapabilitySpecificationContent,
// keyed by the exact RevisionKey it belongs to.
type StructuredContentRepository interface {
	Put(ctx context.Context, key engineering.RevisionKey, content engineering.CapabilitySpecificationContent) error
	Get(ctx context.Context, key engineering.RevisionKey) (engineering.CapabilitySpecificationContent, bool, error)
}

// RecordEnvelopeRepository persists RecordEnvelopes: executions, claims,
// decisions, and state assignments.
//
// Put returns ErrReferencedValueMissing if SubjectKey does not resolve --
// through engineering.ParseSubjectKey -- to a stored ArtifactEnvelope or
// RevisionEnvelope, or if CorrectionTargetID is set and names no stored claim
// (AD-021). That target check establishes only that the claim exists; whether
// the resulting correction graph is acyclic remains a read-time concern of
// ResolveCurrentClaim (FF-010 §6).
type RecordEnvelopeRepository interface {
	Put(ctx context.Context, env engineering.RecordEnvelope) error
	Get(ctx context.Context, key engineering.RecordKey) (engineering.RecordEnvelope, bool, error)
	ListByKind(ctx context.Context, kind engineering.RecordKind) ([]engineering.RecordEnvelope, error)
	ListByKindAndSubject(ctx context.Context, kind engineering.RecordKind, subjectKey string) ([]engineering.RecordEnvelope, error)
}

// RevisionOrderRepository persists RevisionOrderMetadata: sequence only,
// insert-only.
type RevisionOrderRepository interface {
	Put(ctx context.Context, order engineering.RevisionOrderMetadata) error
	Get(ctx context.Context, key engineering.RevisionKey) (engineering.RevisionOrderMetadata, bool, error)
	ListByArtifact(ctx context.Context, artifactID string) ([]engineering.RevisionOrderMetadata, error)
}

// RevisionAcceptanceRepository persists the append-only acceptance journal.
//
// Append is create-only on RecordID: re-appending an identical record is a
// no-op, and a differing record reusing an existing RecordID is
// ErrImmutableValueConflict. FF-009 §4.3 declares RecordID unique and
// FF-006 §2 derives a timeline event's identity from it, so a duplicate would
// collide two timeline events onto one event ID (AD-021).
type RevisionAcceptanceRepository interface {
	Append(ctx context.Context, record engineering.RevisionAcceptanceRecord) error
	ListByRevision(ctx context.Context, key engineering.RevisionKey) ([]engineering.RevisionAcceptanceRecord, error)
	ListByArtifact(ctx context.Context, artifactID string) ([]engineering.RevisionAcceptanceRecord, error)
}

// Repositories bundles every repository, handed to the UnitOfWork callback.
// Repositories are reachable only inside Do (FF-009 §6) -- no field here is
// otherwise obtainable.
type Repositories struct {
	Projects           ProjectRepository
	FeatureCards       FeatureCardRepository
	Artifacts          ArtifactEnvelopeRepository
	Revisions          RevisionEnvelopeRepository
	StructuredContent  StructuredContentRepository
	Records            RecordEnvelopeRepository
	RevisionOrder      RevisionOrderRepository
	RevisionAcceptance RevisionAcceptanceRepository
}

// UnitOfWork runs one engineering act as a single transaction. Returning
// nil from fn commits; returning an error rolls back and propagates that
// error unchanged (errors.Is still matches the original cause); a panic
// rolls back and re-panics. Calling Do from inside fn returns
// ErrNestedTransaction (FF-009 §6).
type UnitOfWork interface {
	Do(ctx context.Context, fn func(Repositories) error) error
}
