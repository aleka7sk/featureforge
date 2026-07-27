package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/domain"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

// RunRepositoryContractSuite exercises every FF-009 persistence and
// transaction requirement against newUOW's adapter. It is non-test code
// (not _test.go) specifically so M.4's PostgreSQL adapter can import and
// run this identical suite (FF-013 §1).
func RunRepositoryContractSuite(t *testing.T, newUOW func() application.UnitOfWork) {
	t.Helper()

	t.Run("PutThenGet", func(t *testing.T) { testPutThenGet(t, newUOW()) })
	t.Run("GetMissingReturnsNotFound", func(t *testing.T) { testGetMissingReturnsNotFound(t, newUOW()) })
	t.Run("IdempotentIdenticalPut", func(t *testing.T) { testIdempotentIdenticalPut(t, newUOW()) })
	t.Run("ConflictingPut", func(t *testing.T) { testConflictingPut(t, newUOW()) })
	t.Run("ListIsDeterministic", func(t *testing.T) { testListIsDeterministic(t, newUOW()) })
	t.Run("ReferenceVerification", func(t *testing.T) { testReferenceVerification(t, newUOW()) })
	t.Run("ReturnedSlicesAreCopies", func(t *testing.T) { testReturnedSlicesAreCopies(t, newUOW()) })

	t.Run("CommitPersistsAllWrites", func(t *testing.T) { testCommitPersistsAllWrites(t, newUOW()) })
	t.Run("RollbackDiscardsAllWrites", func(t *testing.T) { testRollbackDiscardsAllWrites(t, newUOW()) })
	t.Run("RollbackOnPanic", func(t *testing.T) { testRollbackOnPanic(t, newUOW()) })
	t.Run("ErrorPropagatesUnwrapped", func(t *testing.T) { testErrorPropagatesUnwrapped(t, newUOW()) })
	t.Run("ConflictAbortsAct", func(t *testing.T) { testConflictAbortsAct(t, newUOW()) })
	t.Run("NestedTransactionRejected", func(t *testing.T) { testNestedTransactionRejected(t, newUOW()) })
	t.Run("AcceptanceJournalHistory", func(t *testing.T) { testAcceptanceJournalHistory(t, newUOW()) })
	t.Run("RevisionOrderHistory", func(t *testing.T) { testRevisionOrderHistory(t, newUOW()) })
	t.Run("SequenceUniquenessEnforced", func(t *testing.T) { testSequenceUniquenessEnforced(t, newUOW()) })
}

func fixedContractTime() time.Time { return time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC) }

func mustProjectID(t *testing.T, s string) domain.ProjectID {
	t.Helper()
	id, err := domain.NewProjectID(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustProject(t *testing.T, id string) domain.Project {
	t.Helper()
	p, err := domain.NewProject(mustProjectID(t, id), "Test Project "+id, fixedContractTime())
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func mustArtifactEnvelope(t *testing.T, artifactID string) engineering.ArtifactEnvelope {
	t.Helper()
	key, err := engineering.NewArtifactKey(artifactID)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"artifact_id":"` + artifactID + `"}`)
	env, err := engineering.NewArtifactEnvelope(key, "featureforge:product-capability", payload, engineering.ComputeDigest(payload), fixedContractTime())
	if err != nil {
		t.Fatal(err)
	}
	return env
}

func testPutThenGet(t *testing.T, uow application.UnitOfWork) {
	p := mustProject(t, "PRJ-1")
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		return r.Projects.Put(context.Background(), p)
	})
	if err != nil {
		t.Fatal(err)
	}
	err = uow.Do(context.Background(), func(r application.Repositories) error {
		got, ok, err := r.Projects.Get(context.Background(), p.ID())
		if err != nil {
			return err
		}
		if !ok {
			t.Error("expected project to be found")
		}
		if got != p {
			t.Errorf("got %v, want %v", got, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func testGetMissingReturnsNotFound(t *testing.T, uow application.UnitOfWork) {
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		got, ok, err := r.Projects.Get(context.Background(), mustProjectID(t, "PRJ-MISSING"))
		if err != nil {
			t.Errorf("expected nil error for a missing key, got %v", err)
		}
		if ok {
			t.Error("expected found = false")
		}
		if got.IsZero() != true {
			t.Error("expected the zero value for a missing key")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func testIdempotentIdenticalPut(t *testing.T, uow application.UnitOfWork) {
	p := mustProject(t, "PRJ-IDEMP")
	for i := range 2 {
		err := uow.Do(context.Background(), func(r application.Repositories) error {
			return r.Projects.Put(context.Background(), p)
		})
		if err != nil {
			t.Fatalf("put %d: %v", i, err)
		}
	}
}

func testConflictingPut(t *testing.T, uow application.UnitOfWork) {
	id := mustProjectID(t, "PRJ-CONFLICT")
	p1, err := domain.NewProject(id, "Original Name", fixedContractTime())
	if err != nil {
		t.Fatal(err)
	}
	p2, err := domain.NewProject(id, "Different Name", fixedContractTime())
	if err != nil {
		t.Fatal(err)
	}
	if err := uow.Do(context.Background(), func(r application.Repositories) error {
		return r.Projects.Put(context.Background(), p1)
	}); err != nil {
		t.Fatal(err)
	}
	err = uow.Do(context.Background(), func(r application.Repositories) error {
		return r.Projects.Put(context.Background(), p2)
	})
	if !errors.Is(err, application.ErrImmutableValueConflict) {
		t.Errorf("err = %v, want ErrImmutableValueConflict", err)
	}
}

func testListIsDeterministic(t *testing.T, uow application.UnitOfWork) {
	ids := []string{"PRJ-Z", "PRJ-A", "PRJ-M"}
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		for _, id := range ids {
			if err := r.Projects.Put(context.Background(), mustProject(t, id)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var first []domain.Project
	for i := range 20 {
		err := uow.Do(context.Background(), func(r application.Repositories) error {
			list, err := r.Projects.List(context.Background())
			if err != nil {
				return err
			}
			if i == 0 {
				first = list
			} else {
				if len(list) != len(first) {
					t.Fatalf("iteration %d: length changed", i)
				}
				for j := range list {
					if list[j] != first[j] {
						t.Fatalf("iteration %d: order changed at index %d", i, j)
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i < len(first); i++ {
		if first[i-1].ID().String() >= first[i].ID().String() {
			t.Errorf("list not sorted ascending by ID: %v", first)
		}
	}
}

func testReferenceVerification(t *testing.T, uow application.UnitOfWork) {
	revKey, err := engineering.NewRevisionKey("CAP-NOARTIFACT", "REV-1")
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"x":1}`)
	env, err := engineering.NewRevisionEnvelope(engineering.RevisionEnvelopeInput{
		Key: revKey, RevisionFamily: engineering.RevisionFamilyCapability,
		ArtifactType: "featureforge:product-capability", IntegrityValue: "sha256:abc",
		Payload: payload, PayloadDigest: engineering.ComputeDigest(payload), RecordedAt: fixedContractTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	err = uow.Do(context.Background(), func(r application.Repositories) error {
		return r.Revisions.Put(context.Background(), env)
	})
	if !errors.Is(err, application.ErrReferencedValueMissing) {
		t.Errorf("err = %v, want ErrReferencedValueMissing", err)
	}
}

func testReturnedSlicesAreCopies(t *testing.T, uow application.UnitOfWork) {
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		return r.Projects.Put(context.Background(), mustProject(t, "PRJ-COPY"))
	})
	if err != nil {
		t.Fatal(err)
	}
	err = uow.Do(context.Background(), func(r application.Repositories) error {
		list, err := r.Projects.List(context.Background())
		if err != nil {
			return err
		}
		if len(list) == 0 {
			t.Fatal("expected at least one project")
		}
		list[0] = domain.Project{}
		list2, err := r.Projects.List(context.Background())
		if err != nil {
			return err
		}
		if list2[0].IsZero() {
			t.Error("mutating a returned slice must not affect stored state")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func testCommitPersistsAllWrites(t *testing.T, uow application.UnitOfWork) {
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		if err := r.Projects.Put(context.Background(), mustProject(t, "PRJ-COMMIT-1")); err != nil {
			return err
		}
		return r.Projects.Put(context.Background(), mustProject(t, "PRJ-COMMIT-2"))
	})
	if err != nil {
		t.Fatal(err)
	}
	err = uow.Do(context.Background(), func(r application.Repositories) error {
		list, err := r.Projects.List(context.Background())
		if err != nil {
			return err
		}
		if len(list) != 2 {
			t.Errorf("expected 2 projects, got %d", len(list))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func testRollbackDiscardsAllWrites(t *testing.T, uow application.UnitOfWork) {
	sentinel := errors.New("deliberate rollback")
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		if err := r.Projects.Put(context.Background(), mustProject(t, "PRJ-ROLLBACK")); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want it to match the sentinel unwrapped", err)
	}
	err = uow.Do(context.Background(), func(r application.Repositories) error {
		_, ok, err := r.Projects.Get(context.Background(), mustProjectID(t, "PRJ-ROLLBACK"))
		if err != nil {
			return err
		}
		if ok {
			t.Error("expected the rolled-back write to be absent")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func testRollbackOnPanic(t *testing.T, uow application.UnitOfWork) {
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected Do to re-panic")
			}
		}()
		_ = uow.Do(context.Background(), func(r application.Repositories) error {
			if err := r.Projects.Put(context.Background(), mustProject(t, "PRJ-PANIC")); err != nil {
				return err
			}
			panic("deliberate panic")
		})
	}()
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		_, ok, err := r.Projects.Get(context.Background(), mustProjectID(t, "PRJ-PANIC"))
		if err != nil {
			return err
		}
		if ok {
			t.Error("expected the panicked write to be absent")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func testErrorPropagatesUnwrapped(t *testing.T, uow application.UnitOfWork) {
	sentinel := errors.New("a specific cause")
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want errors.Is to match the sentinel", err)
	}
}

func testConflictAbortsAct(t *testing.T, uow application.UnitOfWork) {
	id := mustProjectID(t, "PRJ-ACT-CONFLICT")
	p1, _ := domain.NewProject(id, "First", fixedContractTime())
	if err := uow.Do(context.Background(), func(r application.Repositories) error {
		return r.Projects.Put(context.Background(), p1)
	}); err != nil {
		t.Fatal(err)
	}
	p2, _ := domain.NewProject(id, "Second", fixedContractTime())
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		if err := r.Projects.Put(context.Background(), p2); err != nil {
			return err
		}
		if err := r.Projects.Put(context.Background(), mustProject(t, "PRJ-ACT-CONFLICT-SIBLING")); err != nil {
			return err
		}
		return nil
	})
	if !errors.Is(err, application.ErrImmutableValueConflict) {
		t.Fatalf("err = %v, want ErrImmutableValueConflict", err)
	}
	err = uow.Do(context.Background(), func(r application.Repositories) error {
		_, ok, err := r.Projects.Get(context.Background(), mustProjectID(t, "PRJ-ACT-CONFLICT-SIBLING"))
		if err != nil {
			return err
		}
		if ok {
			t.Error("a write from an aborted act must not persist, even if it succeeded before the conflict")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func testNestedTransactionRejected(t *testing.T, uow application.UnitOfWork) {
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		return uow.Do(context.Background(), func(r2 application.Repositories) error {
			return nil
		})
	})
	if !errors.Is(err, application.ErrNestedTransaction) {
		t.Errorf("err = %v, want ErrNestedTransaction", err)
	}
}

func testAcceptanceJournalHistory(t *testing.T, uow application.UnitOfWork) {
	revKey, err := engineering.NewRevisionKey("CAP-ACC", "REV-1")
	if err != nil {
		t.Fatal(err)
	}
	err = uow.Do(context.Background(), func(r application.Repositories) error {
		if err := r.Artifacts.Put(context.Background(), mustArtifactEnvelope(t, "CAP-ACC")); err != nil {
			return err
		}
		payload := []byte(`{"x":1}`)
		env, err := engineering.NewRevisionEnvelope(engineering.RevisionEnvelopeInput{
			Key: revKey, RevisionFamily: engineering.RevisionFamilyCapability,
			ArtifactType: "featureforge:product-capability", IntegrityValue: "sha256:abc",
			Payload: payload, PayloadDigest: engineering.ComputeDigest(payload), RecordedAt: fixedContractTime(),
		})
		if err != nil {
			return err
		}
		if err := r.Revisions.Put(context.Background(), env); err != nil {
			return err
		}
		rec1, err := engineering.NewRevisionAcceptanceRecord("ACC-1", revKey, engineering.AcceptanceStateAccepted, fixedContractTime(), "featureforge:local-user", "")
		if err != nil {
			return err
		}
		return r.RevisionAcceptance.Append(context.Background(), rec1)
	})
	if err != nil {
		t.Fatal(err)
	}
	err = uow.Do(context.Background(), func(r application.Repositories) error {
		entries, err := r.RevisionAcceptance.ListByRevision(context.Background(), revKey)
		if err != nil {
			return err
		}
		if len(entries) != 1 {
			t.Fatalf("expected 1 acceptance entry, got %d", len(entries))
		}
		if entries[0].State != engineering.AcceptanceStateAccepted {
			t.Errorf("state = %v, want accepted", entries[0].State)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func testRevisionOrderHistory(t *testing.T, uow application.UnitOfWork) {
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		if err := r.Artifacts.Put(context.Background(), mustArtifactEnvelope(t, "CAP-ORDER")); err != nil {
			return err
		}
		for i, revID := range []string{"REV-2", "REV-1"} { // insertion order reversed on purpose
			key, err := engineering.NewRevisionKey("CAP-ORDER", revID)
			if err != nil {
				return err
			}
			payload := []byte(`{"rev":"` + revID + `"}`)
			env, err := engineering.NewRevisionEnvelope(engineering.RevisionEnvelopeInput{
				Key: key, RevisionFamily: engineering.RevisionFamilyCapability,
				ArtifactType: "featureforge:product-capability", IntegrityValue: "sha256:abc",
				Payload: payload, PayloadDigest: engineering.ComputeDigest(payload), RecordedAt: fixedContractTime(),
			})
			if err != nil {
				return err
			}
			if err := r.Revisions.Put(context.Background(), env); err != nil {
				return err
			}
			seq := 1
			if revID == "REV-2" {
				seq = 2
			}
			order, err := engineering.NewRevisionOrderMetadata(key, seq, fixedContractTime())
			if err != nil {
				return err
			}
			if err := r.RevisionOrder.Put(context.Background(), order); err != nil {
				return err
			}
			_ = i
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = uow.Do(context.Background(), func(r application.Repositories) error {
		list, err := r.RevisionOrder.ListByArtifact(context.Background(), "CAP-ORDER")
		if err != nil {
			return err
		}
		if len(list) != 2 || list[0].Sequence != 1 || list[1].Sequence != 2 {
			t.Errorf("ListByArtifact must return entries sorted by sequence ascending, got %v", list)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func testSequenceUniquenessEnforced(t *testing.T, uow application.UnitOfWork) {
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		if err := r.Artifacts.Put(context.Background(), mustArtifactEnvelope(t, "CAP-DUPSEQ")); err != nil {
			return err
		}
		for _, revID := range []string{"REV-A", "REV-B"} {
			key, err := engineering.NewRevisionKey("CAP-DUPSEQ", revID)
			if err != nil {
				return err
			}
			payload := []byte(`{"rev":"` + revID + `"}`)
			env, err := engineering.NewRevisionEnvelope(engineering.RevisionEnvelopeInput{
				Key: key, RevisionFamily: engineering.RevisionFamilyCapability,
				ArtifactType: "featureforge:product-capability", IntegrityValue: "sha256:abc",
				Payload: payload, PayloadDigest: engineering.ComputeDigest(payload), RecordedAt: fixedContractTime(),
			})
			if err != nil {
				return err
			}
			if err := r.Revisions.Put(context.Background(), env); err != nil {
				return err
			}
		}
		keyA, _ := engineering.NewRevisionKey("CAP-DUPSEQ", "REV-A")
		keyB, _ := engineering.NewRevisionKey("CAP-DUPSEQ", "REV-B")
		orderA, err := engineering.NewRevisionOrderMetadata(keyA, 1, fixedContractTime())
		if err != nil {
			return err
		}
		if err := r.RevisionOrder.Put(context.Background(), orderA); err != nil {
			return err
		}
		orderB, err := engineering.NewRevisionOrderMetadata(keyB, 1, fixedContractTime())
		if err != nil {
			return err
		}
		return r.RevisionOrder.Put(context.Background(), orderB)
	})
	if !errors.Is(err, application.ErrRevisionSequenceConflict) {
		t.Errorf("err = %v, want ErrRevisionSequenceConflict", err)
	}
}
