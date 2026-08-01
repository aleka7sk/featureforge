package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering"
	peos "github.com/aleka7sk/featureforge/internal/engineering/peos"
	"github.com/aleka7sk/featureforge/internal/infrastructure/memory"
)

func fixedTime() time.Time { return time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC) }

// lenientEnvelopeInspector lets legacy algorithm-focused fixtures keep their
// intentionally minimal non-PEOS envelopes while delegating every richer
// lifecycle/configuration inspection to the real recorder. Integrity-focused
// suites use peos.Recorder directly.
type lenientEnvelopeInspector struct {
	peos.Recorder
}

func newLenientEnvelopeInspector() lenientEnvelopeInspector {
	return lenientEnvelopeInspector{Recorder: peos.NewRecorder()}
}

func (lenientEnvelopeInspector) ValidateArtifact(engineering.ArtifactEnvelope) error { return nil }
func (lenientEnvelopeInspector) ValidateRevision(engineering.RevisionEnvelope) error { return nil }
func (lenientEnvelopeInspector) ValidateRecord(engineering.RecordEnvelope) error     { return nil }

func newStoreAndUOW() *memory.UnitOfWork {
	return memory.NewUnitOfWork(memory.NewStore())
}

// newStoreWithRecordSubject returns a fresh store in which the capability
// artifact and revisions that this package's record-level tests name as their
// subject already exist.
//
// RecordEnvelopeRepository.Put verifies that a record's SubjectKey resolves
// to a stored artifact or revision (AD-021), so a test that writes a claim,
// execution, or state assignment must establish its subject first -- exactly
// as every application command does. Seeding is idempotent, so a test may
// also call a fixture that re-seeds the same values.
func newStoreWithRecordSubject(t *testing.T) *memory.UnitOfWork {
	t.Helper()
	uow := newStoreAndUOW()
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		if err := r.Artifacts.Put(context.Background(), mustArtEnv(t, "CAP-1")); err != nil {
			return err
		}
		if err := r.Revisions.Put(context.Background(), mustRevEnv(t, "CAP-1", "CAP-1-REV-1")); err != nil {
			return err
		}
		return r.Revisions.Put(context.Background(), mustRevEnv(t, "CAP-1", "CAP-1-REV-2"))
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder := peos.NewRecorder()
	if err := application.EnsureLifecycleConfiguration(context.Background(), uow, recorder, recorder); err != nil {
		t.Fatal(err)
	}
	return uow
}

func mustArtEnv(t *testing.T, artifactID string) engineering.ArtifactEnvelope {
	t.Helper()
	env, err := peos.NewRecorder().RecordCapabilityArtifact(artifactID, fixedTime())
	if err != nil {
		t.Fatal(err)
	}
	return env
}

func mustRevEnv(t *testing.T, artifactID, revisionID string) engineering.RevisionEnvelope {
	t.Helper()
	env, err := peos.NewRecorder().RecordCapabilityRevision(engineering.CapabilityRevisionInput{
		ArtifactID: artifactID, RevisionID: revisionID,
		ContentDigest: engineering.ComputeDigest([]byte("test capability content")),
		RecordedAt:    fixedTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return env
}

func mustOrder(t *testing.T, artifactID, revisionID string, sequence int, recordedAt time.Time) engineering.RevisionOrderMetadata {
	t.Helper()
	key, err := engineering.NewRevisionKey(artifactID, revisionID)
	if err != nil {
		t.Fatal(err)
	}
	order, err := engineering.NewRevisionOrderMetadata(key, sequence, recordedAt)
	if err != nil {
		t.Fatal(err)
	}
	return order
}

func mustAcceptance(t *testing.T, id, artifactID, revisionID string, state engineering.AcceptanceState, effectiveAt time.Time) engineering.RevisionAcceptanceRecord {
	t.Helper()
	key, err := engineering.NewRevisionKey(artifactID, revisionID)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := engineering.NewRevisionAcceptanceRecord(id, key, state, effectiveAt, "featureforge:local-user", "")
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

// seedRevisions writes an artifact plus a set of (revisionID, sequence,
// acceptanceState) triples, each accepted at fixedTime(), into a fresh
// store, and returns the UnitOfWork for querying.
type revisionSpec struct {
	revisionID string
	sequence   int
	state      engineering.AcceptanceState
}

type resolutionRevisionRepo struct {
	application.RevisionEnvelopeRepository
	envelopes []engineering.RevisionEnvelope
}

func (r resolutionRevisionRepo) ListByArtifact(context.Context, string) ([]engineering.RevisionEnvelope, error) {
	return append([]engineering.RevisionEnvelope(nil), r.envelopes...), nil
}

type resolutionOrderRepo struct {
	application.RevisionOrderRepository
	order []engineering.RevisionOrderMetadata
}

func (r resolutionOrderRepo) ListByArtifact(context.Context, string) ([]engineering.RevisionOrderMetadata, error) {
	return append([]engineering.RevisionOrderMetadata(nil), r.order...), nil
}

type resolutionAcceptanceRepo struct {
	application.RevisionAcceptanceRepository
	journal  []engineering.RevisionAcceptanceRecord
	identity map[string]engineering.RevisionAcceptanceRecord
}

func (r resolutionAcceptanceRepo) ListByArtifact(context.Context, string) ([]engineering.RevisionAcceptanceRecord, error) {
	return append([]engineering.RevisionAcceptanceRecord(nil), r.journal...), nil
}

func (r resolutionAcceptanceRepo) GetByRecordID(_ context.Context, recordID string) (engineering.RevisionAcceptanceRecord, bool, error) {
	if r.identity != nil {
		entry, found := r.identity[recordID]
		return entry, found, nil
	}
	var found engineering.RevisionAcceptanceRecord
	count := 0
	for _, entry := range r.journal {
		if entry.RecordID == recordID {
			found = entry
			count++
		}
	}
	if count > 1 {
		return engineering.RevisionAcceptanceRecord{}, false, application.ErrStoredStateIntegrity
	}
	return found, count == 1, nil
}

func resolutionRepos(
	envelopes []engineering.RevisionEnvelope,
	order []engineering.RevisionOrderMetadata,
	acceptance resolutionAcceptanceRepo,
) application.Repositories {
	return application.Repositories{
		Revisions:          resolutionRevisionRepo{envelopes: envelopes},
		RevisionOrder:      resolutionOrderRepo{order: order},
		RevisionAcceptance: acceptance,
	}
}

func seedRevisions(t *testing.T, artifactID string, specs []revisionSpec) application.UnitOfWork {
	t.Helper()
	uow := newStoreAndUOW()
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		if err := r.Artifacts.Put(context.Background(), mustArtEnv(t, artifactID)); err != nil {
			return err
		}
		for i, s := range specs {
			if err := r.Revisions.Put(context.Background(), mustRevEnv(t, artifactID, s.revisionID)); err != nil {
				return err
			}
			order := mustOrder(t, artifactID, s.revisionID, s.sequence, fixedTime())
			if err := r.RevisionOrder.Put(context.Background(), order); err != nil {
				return err
			}
			if s.state != "" {
				rec := mustAcceptance(t, "ACC-"+s.revisionID, artifactID, s.revisionID, s.state, fixedTime().Add(time.Duration(i)*time.Hour))
				if err := r.RevisionAcceptance.Append(context.Background(), rec); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return uow
}

func resolveCurrent(t *testing.T, uow application.UnitOfWork, artifactID string) application.CurrentRevisionResult {
	t.Helper()
	var result application.CurrentRevisionResult
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		var err error
		result, err = application.ResolveCurrentRevision(context.Background(), r, artifactID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestSequenceOneThenTwo(t *testing.T) {
	uow := seedRevisions(t, "CAP-1", []revisionSpec{
		{"REV-1", 1, engineering.AcceptanceStateAccepted},
		{"REV-2", 2, engineering.AcceptanceStateAccepted},
	})
	result := resolveCurrent(t, uow, "CAP-1")
	if !result.Found || result.Sequence != 2 {
		t.Errorf("result = %+v, want sequence 2", result)
	}
}

func TestInsertionOrderIndependence(t *testing.T) {
	orders := [][]revisionSpec{
		{{"REV-1", 1, engineering.AcceptanceStateAccepted}, {"REV-2", 2, engineering.AcceptanceStateAccepted}, {"REV-3", 3, engineering.AcceptanceStateAccepted}},
		{{"REV-3", 3, engineering.AcceptanceStateAccepted}, {"REV-1", 1, engineering.AcceptanceStateAccepted}, {"REV-2", 2, engineering.AcceptanceStateAccepted}},
		{{"REV-2", 2, engineering.AcceptanceStateAccepted}, {"REV-3", 3, engineering.AcceptanceStateAccepted}, {"REV-1", 1, engineering.AcceptanceStateAccepted}},
	}
	var results []application.CurrentRevisionResult
	for _, specs := range orders {
		uow := seedRevisions(t, "CAP-1", specs)
		results = append(results, resolveCurrent(t, uow, "CAP-1"))
	}
	for i := 1; i < len(results); i++ {
		if results[i].Sequence != results[0].Sequence || results[i].Revision.Key != results[0].Revision.Key {
			t.Errorf("order %d diverged: %+v vs %+v", i, results[i], results[0])
		}
	}
	if results[0].Sequence != 3 {
		t.Errorf("sequence = %d, want 3", results[0].Sequence)
	}
}

func TestIgnoresRevisionIDLexicalOrder(t *testing.T) {
	// "AAA-lowest-lexically" gets the highest sequence; resolution must
	// still pick it by sequence, not be swayed by lexical order.
	uow := seedRevisions(t, "CAP-1", []revisionSpec{
		{"ZZZ-first", 1, engineering.AcceptanceStateAccepted},
		{"AAA-lowest-lexically", 2, engineering.AcceptanceStateAccepted},
	})
	result := resolveCurrent(t, uow, "CAP-1")
	if result.Revision.Key.RevisionID != "AAA-lowest-lexically" {
		t.Errorf("selected %s, want AAA-lowest-lexically (highest sequence, despite losing lexically)", result.Revision.Key.RevisionID)
	}
}

func TestDraftHigherSequenceRejected(t *testing.T) {
	uow := seedRevisions(t, "CAP-1", []revisionSpec{
		{"REV-1", 1, engineering.AcceptanceStateAccepted},
		{"REV-2", 2, ""},
	})
	result := resolveCurrent(t, uow, "CAP-1")
	if !result.Found || result.Sequence != 1 {
		t.Errorf("result = %+v, want sequence 1 (REV-2 is draft)", result)
	}
	foundRejected := false
	for _, rej := range result.Rationale.Rejected {
		if rej.Key.RevisionID == "REV-2" && rej.Reason == "not accepted" {
			foundRejected = true
		}
	}
	if !foundRejected {
		t.Error("expected REV-2 to be named as rejected with reason 'not accepted'")
	}
}

func TestWithdrawnAcceptedFallsBack(t *testing.T) {
	uow := newStoreAndUOW()
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		if err := r.Artifacts.Put(context.Background(), mustArtEnv(t, "CAP-1")); err != nil {
			return err
		}
		for _, revID := range []string{"REV-1", "REV-2"} {
			if err := r.Revisions.Put(context.Background(), mustRevEnv(t, "CAP-1", revID)); err != nil {
				return err
			}
		}
		if err := r.RevisionOrder.Put(context.Background(), mustOrder(t, "CAP-1", "REV-1", 1, fixedTime())); err != nil {
			return err
		}
		if err := r.RevisionOrder.Put(context.Background(), mustOrder(t, "CAP-1", "REV-2", 2, fixedTime())); err != nil {
			return err
		}
		if err := r.RevisionAcceptance.Append(context.Background(), mustAcceptance(t, "A1", "CAP-1", "REV-1", engineering.AcceptanceStateAccepted, fixedTime())); err != nil {
			return err
		}
		if err := r.RevisionAcceptance.Append(context.Background(), mustAcceptance(t, "A2", "CAP-1", "REV-2", engineering.AcceptanceStateAccepted, fixedTime().Add(time.Hour))); err != nil {
			return err
		}
		return r.RevisionAcceptance.Append(context.Background(), mustAcceptance(t, "A3", "CAP-1", "REV-2", engineering.AcceptanceStateWithdrawn, fixedTime().Add(2*time.Hour)))
	})
	if err != nil {
		t.Fatal(err)
	}
	result := resolveCurrent(t, uow, "CAP-1")
	if !result.Found || result.Sequence != 1 {
		t.Errorf("result = %+v, want sequence 1 (REV-2 withdrawn)", result)
	}
}

func TestNoAcceptedRevision(t *testing.T) {
	uow := seedRevisions(t, "CAP-1", []revisionSpec{
		{"REV-1", 1, ""},
	})
	result := resolveCurrent(t, uow, "CAP-1")
	if result.Found {
		t.Error("expected Found = false")
	}
	if len(result.Rationale.Warnings) == 0 {
		t.Error("expected a warning explaining no accepted revision")
	}
}

func TestDuplicateSequence(t *testing.T) {
	uow := newStoreAndUOW()
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		if err := r.Artifacts.Put(context.Background(), mustArtEnv(t, "CAP-1")); err != nil {
			return err
		}
		if err := r.Revisions.Put(context.Background(), mustRevEnv(t, "CAP-1", "REV-1")); err != nil {
			return err
		}
		return r.RevisionOrder.Put(context.Background(), mustOrder(t, "CAP-1", "REV-1", 1, fixedTime()))
	})
	if err != nil {
		t.Fatal(err)
	}
	// Force a second revision at the same sequence by writing it directly
	// to a second artifact key sharing... instead, simulate the duplicate
	// by resolving with an order list containing two entries at sequence 1
	// via two revisions (the adapter's own write-time check would reject
	// this; we test the read-time defensive check directly against the
	// same store used for TestSequenceUniquenessEnforced in the memory
	// package's contract suite -- here we confirm ResolveCurrentRevision
	// itself would reject it, using a second artifact to bypass the write
	// guard is not meaningful, so this test targets ErrRevisionSequenceConflict
	// via the adapter's own enforcement instead.
	err = uow.Do(context.Background(), func(r application.Repositories) error {
		if err := r.Revisions.Put(context.Background(), mustRevEnv(t, "CAP-1", "REV-2")); err != nil {
			return err
		}
		return r.RevisionOrder.Put(context.Background(), mustOrder(t, "CAP-1", "REV-2", 1, fixedTime()))
	})
	if !errors.Is(err, application.ErrRevisionSequenceConflict) {
		t.Errorf("err = %v, want ErrRevisionSequenceConflict", err)
	}
}

func TestMissingOrderMetadata(t *testing.T) {
	uow := newStoreAndUOW()
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		if err := r.Artifacts.Put(context.Background(), mustArtEnv(t, "CAP-1")); err != nil {
			return err
		}
		return r.Revisions.Put(context.Background(), mustRevEnv(t, "CAP-1", "REV-1"))
	})
	if err != nil {
		t.Fatal(err)
	}
	err = uow.Do(context.Background(), func(r application.Repositories) error {
		_, err := application.ResolveCurrentRevision(context.Background(), r, "CAP-1")
		return err
	})
	if !errors.Is(err, application.ErrRevisionOrderMissing) {
		t.Errorf("err = %v, want ErrRevisionOrderMissing", err)
	}
}

func TestRevisionSequenceMustBeDense(t *testing.T) {
	uow := seedRevisions(t, "CAP-1", []revisionSpec{
		{"REV-1", 1, engineering.AcceptanceStateAccepted},
		{"REV-2", 3, engineering.AcceptanceStateAccepted},
	})
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		_, err := application.ResolveCurrentRevision(context.Background(), r, "CAP-1")
		return err
	})
	if !errors.Is(err, application.ErrRevisionSequenceInvalid) {
		t.Fatalf("err = %v, want ErrRevisionSequenceInvalid", err)
	}
}

func TestResolveRejectsStructurallyInvalidAcceptanceRecord(t *testing.T) {
	revision := mustRevEnv(t, "CAP-1", "REV-1")
	order := mustOrder(t, "CAP-1", "REV-1", 1, fixedTime())
	invalid := mustAcceptance(t, "A1", "CAP-1", "REV-1", engineering.AcceptanceStateAccepted, fixedTime())
	invalid.Actor = ""

	repos := resolutionRepos(
		[]engineering.RevisionEnvelope{revision},
		[]engineering.RevisionOrderMetadata{order},
		resolutionAcceptanceRepo{journal: []engineering.RevisionAcceptanceRecord{invalid}},
	)
	_, err := application.ResolveCurrentRevision(context.Background(), repos, "CAP-1")
	if !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("err = %v, want ErrStoredStateIntegrity", err)
	}
}

func TestResolveRejectsDanglingAcceptanceRecord(t *testing.T) {
	revision := mustRevEnv(t, "CAP-1", "REV-1")
	order := mustOrder(t, "CAP-1", "REV-1", 1, fixedTime())
	dangling := mustAcceptance(t, "A2", "CAP-1", "REV-MISSING", engineering.AcceptanceStateAccepted, fixedTime())

	repos := resolutionRepos(
		[]engineering.RevisionEnvelope{revision},
		[]engineering.RevisionOrderMetadata{order},
		resolutionAcceptanceRepo{journal: []engineering.RevisionAcceptanceRecord{dangling}},
	)
	_, err := application.ResolveCurrentRevision(context.Background(), repos, "CAP-1")
	if !errors.Is(err, application.ErrRevisionReferenceMismatch) {
		t.Fatalf("err = %v, want ErrRevisionReferenceMismatch", err)
	}
}

func TestResolveRejectsDuplicateAcceptanceRecordID(t *testing.T) {
	revisions := []engineering.RevisionEnvelope{
		mustRevEnv(t, "CAP-1", "REV-1"),
		mustRevEnv(t, "CAP-1", "REV-2"),
	}
	order := []engineering.RevisionOrderMetadata{
		mustOrder(t, "CAP-1", "REV-1", 1, fixedTime()),
		mustOrder(t, "CAP-1", "REV-2", 2, fixedTime()),
	}
	journal := []engineering.RevisionAcceptanceRecord{
		mustAcceptance(t, "A-DUP", "CAP-1", "REV-1", engineering.AcceptanceStateAccepted, fixedTime()),
		mustAcceptance(t, "A-DUP", "CAP-1", "REV-2", engineering.AcceptanceStateAccepted, fixedTime()),
	}

	repos := resolutionRepos(revisions, order, resolutionAcceptanceRepo{journal: journal})
	_, err := application.ResolveCurrentRevision(context.Background(), repos, "CAP-1")
	if !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("err = %v, want ErrStoredStateIntegrity", err)
	}
}

func TestResolveRejectsInvalidAcceptanceTransitionHistory(t *testing.T) {
	revision := mustRevEnv(t, "CAP-1", "REV-1")
	order := mustOrder(t, "CAP-1", "REV-1", 1, fixedTime())
	journal := []engineering.RevisionAcceptanceRecord{
		mustAcceptance(t, "A1", "CAP-1", "REV-1", engineering.AcceptanceStateAccepted, fixedTime()),
		mustAcceptance(t, "A2", "CAP-1", "REV-1", engineering.AcceptanceStateAccepted, fixedTime().Add(time.Hour)),
	}

	repos := resolutionRepos(
		[]engineering.RevisionEnvelope{revision},
		[]engineering.RevisionOrderMetadata{order},
		resolutionAcceptanceRepo{journal: journal},
	)
	_, err := application.ResolveCurrentRevision(context.Background(), repos, "CAP-1")
	if !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("err = %v, want ErrStoredStateIntegrity", err)
	}
}

func TestResolveRejectsZeroAcceptanceEffectiveTimeAsStoredIntegrity(t *testing.T) {
	revision := mustRevEnv(t, "CAP-1", "REV-1")
	order := mustOrder(t, "CAP-1", "REV-1", 1, fixedTime())
	entry := mustAcceptance(t, "A-ZERO-TIME", "CAP-1", "REV-1", engineering.AcceptanceStateAccepted, fixedTime())
	entry.EffectiveAt = time.Time{}

	repos := resolutionRepos(
		[]engineering.RevisionEnvelope{revision},
		[]engineering.RevisionOrderMetadata{order},
		resolutionAcceptanceRepo{journal: []engineering.RevisionAcceptanceRecord{entry}},
	)
	_, err := application.ResolveCurrentRevision(context.Background(), repos, "CAP-1")
	if !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("err = %v, want ErrStoredStateIntegrity", err)
	}
}

func TestResolveRejectsAcceptanceIdentityIndexMismatch(t *testing.T) {
	revision := mustRevEnv(t, "CAP-1", "REV-1")
	order := mustOrder(t, "CAP-1", "REV-1", 1, fixedTime())
	listed := mustAcceptance(t, "A1", "CAP-1", "REV-1", engineering.AcceptanceStateAccepted, fixedTime())
	indexed := listed
	indexed.Reason = "different stored record"

	repos := resolutionRepos(
		[]engineering.RevisionEnvelope{revision},
		[]engineering.RevisionOrderMetadata{order},
		resolutionAcceptanceRepo{
			journal:  []engineering.RevisionAcceptanceRecord{listed},
			identity: map[string]engineering.RevisionAcceptanceRecord{"A1": indexed},
		},
	)
	_, err := application.ResolveCurrentRevision(context.Background(), repos, "CAP-1")
	if !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("err = %v, want ErrStoredStateIntegrity", err)
	}
}

func TestNonPositiveSequence(t *testing.T) {
	key, err := engineering.NewRevisionKey("CAP-1", "REV-1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = engineering.NewRevisionOrderMetadata(key, 0, fixedTime())
	if err == nil {
		t.Error("expected NewRevisionOrderMetadata to reject sequence 0")
	}
}

func TestResolutionIsDeterministic(t *testing.T) {
	uow := seedRevisions(t, "CAP-1", []revisionSpec{
		{"REV-1", 1, engineering.AcceptanceStateAccepted},
		{"REV-2", 2, engineering.AcceptanceStateAccepted},
	})
	first := resolveCurrent(t, uow, "CAP-1")
	for i := range 20 {
		got := resolveCurrent(t, uow, "CAP-1")
		if got.Sequence != first.Sequence || got.Revision.Key != first.Revision.Key {
			t.Fatalf("iteration %d: result diverged", i)
		}
	}
}

func TestAcceptanceDefaultsToDraft(t *testing.T) {
	uow := seedRevisions(t, "CAP-1", []revisionSpec{{"REV-1", 1, ""}})
	result := resolveCurrent(t, uow, "CAP-1")
	if result.Found {
		t.Error("a revision with no acceptance entry must default to draft, not be found as current")
	}
}

func TestAcceptedToDraftRejected(t *testing.T) {
	key, _ := engineering.NewRevisionKey("CAP-1", "REV-1")
	journal := []engineering.RevisionAcceptanceRecord{
		mustAcceptance(t, "A1", "CAP-1", "REV-1", engineering.AcceptanceStateAccepted, fixedTime()),
	}
	err := application.ValidateAcceptanceTransition(journal, key, engineering.AcceptanceStateDraft)
	if !errors.Is(err, application.ErrAcceptanceTransitionInvalid) {
		t.Errorf("err = %v, want ErrAcceptanceTransitionInvalid", err)
	}
}

func TestWithdrawnIsTerminal(t *testing.T) {
	key, _ := engineering.NewRevisionKey("CAP-1", "REV-1")
	journal := []engineering.RevisionAcceptanceRecord{
		mustAcceptance(t, "A1", "CAP-1", "REV-1", engineering.AcceptanceStateWithdrawn, fixedTime()),
	}
	for _, target := range []engineering.AcceptanceState{engineering.AcceptanceStateDraft, engineering.AcceptanceStateAccepted} {
		err := application.ValidateAcceptanceTransition(journal, key, target)
		if !errors.Is(err, application.ErrAcceptanceTransitionInvalid) {
			t.Errorf("target %s: err = %v, want ErrAcceptanceTransitionInvalid", target, err)
		}
	}
}
