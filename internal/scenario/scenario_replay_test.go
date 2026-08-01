package scenario_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/domain"
	"github.com/aleka7sk/featureforge/internal/engineering"
	peos "github.com/aleka7sk/featureforge/internal/engineering/peos"
	"github.com/aleka7sk/featureforge/internal/infrastructure/memory"
	"github.com/aleka7sk/featureforge/internal/scenario"
)

var errCanonicalReplayWrite = errors.New("canonical scenario replay attempted a write")

// replayWriteGate is kept around the same adapter UnitOfWork for both runs.
// Once writes are rejected, every repository mutator fails before it can
// reach the adapter. A successful replay therefore proves zero write calls,
// rather than merely proving that idempotent adapter writes had no net effect.
type replayWriteGate struct {
	base          application.UnitOfWork
	rejectWrites  bool
	writeAttempts int
}

func (g *replayWriteGate) Do(ctx context.Context, fn func(application.Repositories) error) error {
	return g.base.Do(ctx, func(r application.Repositories) error {
		return fn(g.wrap(r))
	})
}

func (g *replayWriteGate) rejectAllWrites() {
	g.rejectWrites = true
	g.writeAttempts = 0
}

func (g *replayWriteGate) beforeWrite() error {
	g.writeAttempts++
	if g.rejectWrites {
		return errCanonicalReplayWrite
	}
	return nil
}

func (g *replayWriteGate) wrap(r application.Repositories) application.Repositories {
	return application.Repositories{
		Projects:           gatedProjectRepository{ProjectRepository: r.Projects, gate: g},
		FeatureCards:       gatedFeatureCardRepository{FeatureCardRepository: r.FeatureCards, gate: g},
		Artifacts:          gatedArtifactRepository{ArtifactEnvelopeRepository: r.Artifacts, gate: g},
		Revisions:          gatedRevisionRepository{RevisionEnvelopeRepository: r.Revisions, gate: g},
		StructuredContent:  gatedStructuredContentRepository{StructuredContentRepository: r.StructuredContent, gate: g},
		Records:            gatedRecordRepository{RecordEnvelopeRepository: r.Records, gate: g},
		RevisionOrder:      gatedRevisionOrderRepository{RevisionOrderRepository: r.RevisionOrder, gate: g},
		RevisionAcceptance: gatedRevisionAcceptanceRepository{RevisionAcceptanceRepository: r.RevisionAcceptance, gate: g},
	}
}

type gatedProjectRepository struct {
	application.ProjectRepository
	gate *replayWriteGate
}

func (r gatedProjectRepository) Put(ctx context.Context, project domain.Project) error {
	if err := r.gate.beforeWrite(); err != nil {
		return err
	}
	return r.ProjectRepository.Put(ctx, project)
}

type gatedFeatureCardRepository struct {
	application.FeatureCardRepository
	gate *replayWriteGate
}

func (r gatedFeatureCardRepository) Put(ctx context.Context, card domain.FeatureCard) error {
	if err := r.gate.beforeWrite(); err != nil {
		return err
	}
	return r.FeatureCardRepository.Put(ctx, card)
}

func (r gatedFeatureCardRepository) LinkCapability(ctx context.Context, id domain.FeatureCardID, artifactID string) error {
	if err := r.gate.beforeWrite(); err != nil {
		return err
	}
	return r.FeatureCardRepository.LinkCapability(ctx, id, artifactID)
}

type gatedArtifactRepository struct {
	application.ArtifactEnvelopeRepository
	gate *replayWriteGate
}

func (r gatedArtifactRepository) Put(ctx context.Context, env engineering.ArtifactEnvelope) error {
	if err := r.gate.beforeWrite(); err != nil {
		return err
	}
	return r.ArtifactEnvelopeRepository.Put(ctx, env)
}

type gatedRevisionRepository struct {
	application.RevisionEnvelopeRepository
	gate *replayWriteGate
}

func (r gatedRevisionRepository) Put(ctx context.Context, env engineering.RevisionEnvelope) error {
	if err := r.gate.beforeWrite(); err != nil {
		return err
	}
	return r.RevisionEnvelopeRepository.Put(ctx, env)
}

type gatedStructuredContentRepository struct {
	application.StructuredContentRepository
	gate *replayWriteGate
}

func (r gatedStructuredContentRepository) Put(ctx context.Context, key engineering.RevisionKey, content engineering.CapabilitySpecificationContent) error {
	if err := r.gate.beforeWrite(); err != nil {
		return err
	}
	return r.StructuredContentRepository.Put(ctx, key, content)
}

type gatedRecordRepository struct {
	application.RecordEnvelopeRepository
	gate *replayWriteGate
}

func (r gatedRecordRepository) Put(ctx context.Context, env engineering.RecordEnvelope) error {
	if err := r.gate.beforeWrite(); err != nil {
		return err
	}
	return r.RecordEnvelopeRepository.Put(ctx, env)
}

type gatedRevisionOrderRepository struct {
	application.RevisionOrderRepository
	gate *replayWriteGate
}

func (r gatedRevisionOrderRepository) Put(ctx context.Context, order engineering.RevisionOrderMetadata) error {
	if err := r.gate.beforeWrite(); err != nil {
		return err
	}
	return r.RevisionOrderRepository.Put(ctx, order)
}

type gatedRevisionAcceptanceRepository struct {
	application.RevisionAcceptanceRepository
	gate *replayWriteGate
}

func (r gatedRevisionAcceptanceRepository) Append(ctx context.Context, record engineering.RevisionAcceptanceRecord) error {
	if err := r.gate.beforeWrite(); err != nil {
		return err
	}
	return r.RevisionAcceptanceRepository.Append(ctx, record)
}

type projectSnapshot struct {
	ID        string
	Name      string
	CreatedAt time.Time
}

type featureCardSnapshot struct {
	ID                   string
	ProjectID            string
	Title                string
	Description          string
	CreatedAt            time.Time
	CapabilityArtifactID string
	HasCapability        bool
}

type artifactSnapshot struct {
	Key           string
	Kind          engineering.RecordKind
	ArtifactType  string
	Payload       []byte
	PayloadDigest string
	RecordedAt    time.Time
}

type revisionSnapshot struct {
	Key                  string
	Kind                 engineering.RecordKind
	RevisionFamily       engineering.RevisionFamily
	ArtifactType         string
	IntegrityValue       string
	ProvenanceActor      string
	ProvenanceRecordedAt time.Time
	HasProvenanceActor   bool
	HasProvenanceTime    bool
	ContentDigest        string
	SubjectKey           string
	Payload              []byte
	PayloadDigest        string
	RecordedAt           time.Time
}

type contentSnapshot struct {
	Key       string
	Found     bool
	Canonical []byte
}

type recordSnapshot struct {
	Key                string
	Kind               engineering.RecordKind
	SubjectKey         string
	Scope              string
	OccurredAt         time.Time
	HasOccurredAt      bool
	Outcome            string
	CriterionKeys      []string
	EvidenceKeys       []string
	ExecutionKeys      []string
	CorrectionKind     string
	CorrectionTargetID string
	StateID            string
	Payload            []byte
	PayloadDigest      string
	RecordedAt         time.Time
}

type canonicalStoreSnapshot struct {
	Projects   []projectSnapshot
	Cards      []featureCardSnapshot
	Artifacts  []artifactSnapshot
	Revisions  []revisionSnapshot
	Content    []contentSnapshot
	Records    []recordSnapshot
	Order      []engineering.RevisionOrderMetadata
	Acceptance []engineering.RevisionAcceptanceRecord
}

// canonicalPersistenceSnapshot serializes the complete closed-world state of
// FF-011. Artifact and StructuredContent ports have no global List method, so
// their exhaustive population is obtained from the scenario's fixed Artifact
// identities and each Artifact's exhaustive ListByArtifact revision set.
func canonicalPersistenceSnapshot(t *testing.T, ctx context.Context, uow application.UnitOfWork, result scenario.Result) []byte {
	t.Helper()

	var snapshot canonicalStoreSnapshot
	err := uow.Do(ctx, func(r application.Repositories) error {
		projects, err := r.Projects.List(ctx)
		if err != nil {
			return err
		}
		for _, project := range projects {
			snapshot.Projects = append(snapshot.Projects, projectSnapshot{
				ID: project.ID().String(), Name: project.Name(), CreatedAt: project.CreatedAt(),
			})
			cards, err := r.FeatureCards.ListByProject(ctx, project.ID())
			if err != nil {
				return err
			}
			for _, card := range cards {
				capabilityID, linked := card.CapabilityArtifactID()
				snapshot.Cards = append(snapshot.Cards, featureCardSnapshot{
					ID: card.ID().String(), ProjectID: card.ProjectID().String(), Title: card.Title(),
					Description: card.Description(), CreatedAt: card.CreatedAt(),
					CapabilityArtifactID: capabilityID, HasCapability: linked,
				})
			}
		}

		for _, artifactID := range canonicalArtifactIDs(result) {
			key, err := engineering.NewArtifactKey(artifactID)
			if err != nil {
				return err
			}
			artifact, found, err := r.Artifacts.Get(ctx, key)
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("canonical artifact %s is missing", artifactID)
			}
			snapshot.Artifacts = append(snapshot.Artifacts, artifactSnapshot{
				Key: artifact.Key.String(), Kind: artifact.Kind, ArtifactType: artifact.ArtifactType,
				Payload: artifact.Payload, PayloadDigest: artifact.PayloadDigest.Hex(), RecordedAt: artifact.RecordedAt,
			})

			revisions, err := r.Revisions.ListByArtifact(ctx, artifactID)
			if err != nil {
				return err
			}
			for _, revision := range revisions {
				snapshot.Revisions = append(snapshot.Revisions, revisionSnapshot{
					Key: revision.Key.String(), Kind: revision.Kind, RevisionFamily: revision.RevisionFamily,
					ArtifactType: revision.ArtifactType, IntegrityValue: revision.IntegrityValue,
					ProvenanceActor: revision.ProvenanceActor, ProvenanceRecordedAt: revision.ProvenanceRecordedAt,
					HasProvenanceActor: revision.HasProvenanceActor, HasProvenanceTime: revision.HasProvenanceTime,
					ContentDigest: revision.ContentDigest.Hex(), SubjectKey: revision.SubjectKey,
					Payload: revision.Payload, PayloadDigest: revision.PayloadDigest.Hex(), RecordedAt: revision.RecordedAt,
				})
				content, contentFound, err := r.StructuredContent.Get(ctx, revision.Key)
				if err != nil {
					return err
				}
				contentState := contentSnapshot{Key: revision.Key.String(), Found: contentFound}
				if contentFound {
					contentState.Canonical, err = content.CanonicalJSON()
					if err != nil {
						return err
					}
				}
				snapshot.Content = append(snapshot.Content, contentState)
			}

			orders, err := r.RevisionOrder.ListByArtifact(ctx, artifactID)
			if err != nil {
				return err
			}
			snapshot.Order = append(snapshot.Order, orders...)
			acceptance, err := r.RevisionAcceptance.ListByArtifact(ctx, artifactID)
			if err != nil {
				return err
			}
			snapshot.Acceptance = append(snapshot.Acceptance, acceptance...)
		}

		for _, kind := range []engineering.RecordKind{
			engineering.RecordKindDecision,
			engineering.RecordKindExecution,
			engineering.RecordKindClaim,
			engineering.RecordKindStateAssignment,
		} {
			records, err := r.Records.ListByKind(ctx, kind)
			if err != nil {
				return err
			}
			for _, record := range records {
				snapshot.Records = append(snapshot.Records, recordSnapshot{
					Key: record.Key.String(), Kind: record.Kind, SubjectKey: record.SubjectKey, Scope: record.Scope,
					OccurredAt: record.OccurredAt, HasOccurredAt: record.HasOccurredAt, Outcome: record.Outcome,
					CriterionKeys: record.CriterionKeys, EvidenceKeys: record.EvidenceKeys, ExecutionKeys: record.ExecutionKeys,
					CorrectionKind: record.CorrectionKind, CorrectionTargetID: record.CorrectionTargetID, StateID: record.StateID,
					Payload: record.Payload, PayloadDigest: record.PayloadDigest.Hex(), RecordedAt: record.RecordedAt,
				})
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("capturing canonical persistence snapshot: %v", err)
	}

	// Repository list contracts already sort within a collection. These final
	// sorts make the aggregate snapshot independent of artifact enumeration.
	sort.Slice(snapshot.Cards, func(i, j int) bool { return snapshot.Cards[i].ID < snapshot.Cards[j].ID })
	sort.Slice(snapshot.Revisions, func(i, j int) bool { return snapshot.Revisions[i].Key < snapshot.Revisions[j].Key })
	sort.Slice(snapshot.Content, func(i, j int) bool { return snapshot.Content[i].Key < snapshot.Content[j].Key })
	sort.Slice(snapshot.Records, func(i, j int) bool { return snapshot.Records[i].Key < snapshot.Records[j].Key })
	sort.Slice(snapshot.Order, func(i, j int) bool { return snapshot.Order[i].Key.String() < snapshot.Order[j].Key.String() })
	sort.Slice(snapshot.Acceptance, func(i, j int) bool {
		if snapshot.Acceptance[i].Key != snapshot.Acceptance[j].Key {
			return snapshot.Acceptance[i].Key.String() < snapshot.Acceptance[j].Key.String()
		}
		return snapshot.Acceptance[i].RecordID < snapshot.Acceptance[j].RecordID
	})

	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("encoding canonical persistence snapshot: %v", err)
	}
	return encoded
}

func canonicalArtifactIDs(result scenario.Result) []string {
	ids := []string{
		result.CapabilityArtifactID,
		result.PlanArtifactID,
		scenario.TransitionRecordArtifactID,
	}
	ids = append(ids, result.RequirementArtifactIDs...)
	ids = append(ids, result.EvidenceArtifactIDs...)
	sort.Strings(ids)
	return ids
}

func assertCanonicalScenarioReplay(
	t *testing.T,
	ctx context.Context,
	base application.UnitOfWork,
	recorder peos.Recorder,
	clock *application.FixedClock,
) {
	t.Helper()

	gate := &replayWriteGate{base: base}
	first, err := scenario.Run(ctx, gate, recorder, recorder, clock)
	if err != nil {
		t.Fatalf("first scenario.Run: %v", err)
	}
	assertCanonicalEndState(t, ctx, gate, recorder, first)
	before := canonicalPersistenceSnapshot(t, ctx, gate, first)

	// The same clock instance has already advanced through the first run; move
	// it farther to make any accidental time-bearing reconstruction observable.
	clock.Advance(24 * time.Hour)
	gate.rejectAllWrites()
	second, err := scenario.Run(ctx, gate, recorder, recorder, clock)
	if err != nil {
		t.Fatalf("second scenario.Run: %v", err)
	}
	if gate.writeAttempts != 0 {
		t.Fatalf("second scenario run attempted %d repository writes, want zero", gate.writeAttempts)
	}
	if !reflect.DeepEqual(second, first) {
		t.Fatalf("second result identities differ:\nfirst:  %#v\nsecond: %#v", first, second)
	}
	assertCanonicalEndState(t, ctx, gate, recorder, second)
	after := canonicalPersistenceSnapshot(t, ctx, gate, second)
	if !bytes.Equal(after, before) {
		t.Fatal("complete canonical persisted state changed after replay")
	}
}

func TestCanonicalScenarioReplayMemory(t *testing.T) {
	store := memory.NewStore()
	assertCanonicalScenarioReplay(
		t,
		context.Background(),
		memory.NewUnitOfWork(store),
		peos.NewRecorder(),
		application.NewFixedClock(scenario.FixedStart),
	)
}
