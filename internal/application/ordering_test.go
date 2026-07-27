package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering"
	"github.com/aleka7sk/featureforge/internal/infrastructure/memory"
)

func fixedTime() time.Time { return time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC) }

func newStoreAndUOW() *memory.UnitOfWork {
	return memory.NewUnitOfWork(memory.NewStore())
}

func mustArtEnv(t *testing.T, artifactID string) engineering.ArtifactEnvelope {
	t.Helper()
	key, err := engineering.NewArtifactKey(artifactID)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"artifact_id":"` + artifactID + `"}`)
	env, err := engineering.NewArtifactEnvelope(key, "featureforge:product-capability", payload, engineering.ComputeDigest(payload), fixedTime())
	if err != nil {
		t.Fatal(err)
	}
	return env
}

func mustRevEnv(t *testing.T, artifactID, revisionID string) engineering.RevisionEnvelope {
	t.Helper()
	key, err := engineering.NewRevisionKey(artifactID, revisionID)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"revision_id":"` + revisionID + `"}`)
	env, err := engineering.NewRevisionEnvelope(engineering.RevisionEnvelopeInput{
		Key: key, RevisionFamily: engineering.RevisionFamilyCapability,
		ArtifactType: "featureforge:product-capability", IntegrityValue: "sha256:abc",
		Payload: payload, PayloadDigest: engineering.ComputeDigest(payload), RecordedAt: fixedTime(),
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
		{"REV-2", 2, engineering.AcceptanceStateDraft},
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
		{"REV-1", 1, engineering.AcceptanceStateDraft},
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
