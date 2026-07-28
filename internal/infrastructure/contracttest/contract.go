// Package contracttest holds the single shared definition of what any
// FeatureForge persistence adapter must do. Both the in-memory and the
// PostgreSQL adapters run this identical suite against their own
// application.UnitOfWork, which is what makes "the domain is independent of
// persistence technology" a tested claim rather than an assertion.
//
// It lives in its own package, rather than in either adapter, so neither
// adapter has to import the other purely to borrow test infrastructure. It is
// non-test code (not _test.go) for the same reason: a _test.go file is not
// importable from another package.
package contracttest

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
// transaction requirement, plus the AD-021 integrity invariants, against
// newUOW's adapter.
//
// newUOW is called fresh for every subtest and deliberately takes no
// *testing.T: each subtest runs in its own goroutine with its own T, and
// calling t.Fatal on an outer, closed-over T from inside a subtest goroutine
// is a Go testing bug. An adapter whose setup can fail should panic instead --
// the testing framework attributes a panic to the subtest that caused it.
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

	// AD-021: invariants the specifications declare and both adapters must
	// now enforce identically.
	t.Run("FeatureCardPutRequiresExistingProject", func(t *testing.T) { testFeatureCardRequiresProject(t, newUOW()) })
	t.Run("RecordEnvelopePutRequiresResolvableSubject", func(t *testing.T) { testRecordRequiresSubject(t, newUOW()) })
	t.Run("RecordEnvelopePutRejectsMalformedSubjectKey", func(t *testing.T) { testRecordRejectsMalformedSubject(t, newUOW()) })
	t.Run("AcceptanceRecordIDIsUnique", func(t *testing.T) { testAcceptanceRecordIDIsUnique(t, newUOW()) })

	// AD-025, FF-016: revision subject discovery.
	t.Run("RevisionListByFamilyAndSubject", func(t *testing.T) { testRevisionListByFamilyAndSubject(t, newUOW()) })
	t.Run("RevisionSubjectKeyIsOptional", func(t *testing.T) { testRevisionSubjectKeyIsOptional(t, newUOW()) })
	t.Run("RevisionRejectsMalformedSubjectKey", func(t *testing.T) { testRevisionRejectsMalformedSubjectKey(t, newUOW()) })

	// AD-026: SubjectKey participates in RevisionEnvelope.Equal, and
	// therefore in create-only conflict detection, identically in both
	// adapters because both dispatch through the same Equal method.
	t.Run("RevisionSubjectBearingPutIsIdempotent", func(t *testing.T) { testRevisionSubjectBearingPutIsIdempotent(t, newUOW()) })
	t.Run("RevisionSubjectKeyDifferenceConflicts", func(t *testing.T) { testRevisionSubjectKeyDifferenceConflicts(t, newUOW()) })
	t.Run("RevisionSubjectVisibilityAcrossTransactionBoundaries", func(t *testing.T) {
		testRevisionSubjectVisibilityAcrossTransactionBoundaries(t, newUOW())
	})
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

// --- AD-021: spec-declared invariants enforced identically by both adapters ---

// testFeatureCardRequiresProject asserts a FeatureCard cannot be written into
// a project that does not exist.
func testFeatureCardRequiresProject(t *testing.T, uow application.UnitOfWork) {
	card, err := domain.NewFeatureCard(
		mustFeatureCardID(t, "FC-ORPHAN"), mustProjectID(t, "PRJ-GHOST"),
		"Orphaned card", "", fixedContractTime(),
	)
	if err != nil {
		t.Fatal(err)
	}
	err = uow.Do(context.Background(), func(r application.Repositories) error {
		return r.FeatureCards.Put(context.Background(), card)
	})
	if !errors.Is(err, application.ErrReferencedValueMissing) {
		t.Errorf("err = %v, want ErrReferencedValueMissing", err)
	}
}

// testRecordRequiresSubject asserts a RecordEnvelope cannot name a subject
// that was never established -- for both subject forms.
func testRecordRequiresSubject(t *testing.T, uow application.UnitOfWork) {
	for _, tc := range []struct {
		name       string
		subjectKey string
	}{
		{"artifact subject", engineering.ArtifactSubjectKey("CAP-GHOST")},
		{"artifact revision subject", engineering.ArtifactRevisionSubjectKey("CAP-GHOST", "REV-1")},
	} {
		env := mustRecordEnvelope(t, engineering.RecordKindClaim, "CLM-"+tc.name, tc.subjectKey)
		err := uow.Do(context.Background(), func(r application.Repositories) error {
			return r.Records.Put(context.Background(), env)
		})
		if !errors.Is(err, application.ErrReferencedValueMissing) {
			t.Errorf("%s: err = %v, want ErrReferencedValueMissing", tc.name, err)
		}
	}
}

// testRecordRejectsMalformedSubject asserts a SubjectKey naming neither
// supported form is rejected rather than silently stored unverifiable.
func testRecordRejectsMalformedSubject(t *testing.T, uow application.UnitOfWork) {
	env := mustRecordEnvelope(t, engineering.RecordKindClaim, "CLM-MALFORMED", "nonsense:CAP-1")
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		return r.Records.Put(context.Background(), env)
	})
	if !errors.Is(err, engineering.ErrInvalidEnvelope) {
		t.Errorf("err = %v, want ErrInvalidEnvelope", err)
	}
}

// testAcceptanceRecordIDIsUnique asserts Append is create-only on RecordID:
// re-appending an identical record is a no-op, and reusing an existing
// RecordID for a different record conflicts. FF-009 4.3 declares RecordID
// unique and FF-006 2 derives a timeline event's identity from it, so a
// duplicate would collide two timeline events onto one event ID.
func testAcceptanceRecordIDIsUnique(t *testing.T, uow application.UnitOfWork) {
	revKey, err := engineering.NewRevisionKey("CAP-UNIQ", "REV-1")
	if err != nil {
		t.Fatal(err)
	}
	err = uow.Do(context.Background(), func(r application.Repositories) error {
		if err := r.Artifacts.Put(context.Background(), mustArtifactEnvelope(t, "CAP-UNIQ")); err != nil {
			return err
		}
		if err := r.Revisions.Put(context.Background(), mustRevisionEnvelope(t, revKey)); err != nil {
			return err
		}
		rec, err := engineering.NewRevisionAcceptanceRecord("ACC-UNIQ", revKey, engineering.AcceptanceStateAccepted, fixedContractTime(), "featureforge:local-user", "")
		if err != nil {
			return err
		}
		return r.RevisionAcceptance.Append(context.Background(), rec)
	})
	if err != nil {
		t.Fatal(err)
	}

	// Identical re-append is idempotent.
	err = uow.Do(context.Background(), func(r application.Repositories) error {
		rec, err := engineering.NewRevisionAcceptanceRecord("ACC-UNIQ", revKey, engineering.AcceptanceStateAccepted, fixedContractTime(), "featureforge:local-user", "")
		if err != nil {
			return err
		}
		return r.RevisionAcceptance.Append(context.Background(), rec)
	})
	if err != nil {
		t.Errorf("identical re-append should be a no-op, got %v", err)
	}

	// Same RecordID, different content, conflicts.
	err = uow.Do(context.Background(), func(r application.Repositories) error {
		rec, err := engineering.NewRevisionAcceptanceRecord("ACC-UNIQ", revKey, engineering.AcceptanceStateWithdrawn, fixedContractTime(), "featureforge:local-user", "")
		if err != nil {
			return err
		}
		return r.RevisionAcceptance.Append(context.Background(), rec)
	})
	if !errors.Is(err, application.ErrImmutableValueConflict) {
		t.Errorf("err = %v, want ErrImmutableValueConflict", err)
	}

	// The journal still holds exactly the one original entry.
	err = uow.Do(context.Background(), func(r application.Repositories) error {
		entries, err := r.RevisionAcceptance.ListByRevision(context.Background(), revKey)
		if err != nil {
			return err
		}
		if len(entries) != 1 {
			t.Errorf("journal has %d entries, want 1", len(entries))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// --- AD-025, FF-016: revision subject discovery ---

// testRevisionListByFamilyAndSubject asserts ListByFamilyAndSubject returns
// exactly the revisions matching both family and subject, in ascending
// RevisionKey.String() order, an empty slice (never an error) on no match,
// and never a subject-less revision, regardless of which subject is queried.
func testRevisionListByFamilyAndSubject(t *testing.T, uow application.UnitOfWork) {
	subjectA := engineering.ArtifactSubjectKey("CAP-SUBJ-A")
	subjectB := engineering.ArtifactSubjectKey("CAP-SUBJ-B")

	writes := []struct {
		artifactID, revisionID string
		family                 engineering.RevisionFamily
		subject                string
	}{
		{"REQ-SUBJ-1", "REV-1", engineering.RevisionFamilyRequirement, subjectA},
		{"REQ-SUBJ-2", "REV-1", engineering.RevisionFamilyRequirement, subjectA},
		{"REQ-SUBJ-3", "REV-1", engineering.RevisionFamilyRequirement, subjectB},
		{"VP-SUBJ-1", "REV-1", engineering.RevisionFamilyValidationPlan, subjectA},
		{"CAP-SUBJ-NOSUBJECT", "REV-1", engineering.RevisionFamilyCapability, ""},
	}
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		for _, w := range writes {
			if err := r.Artifacts.Put(context.Background(), mustArtifactEnvelope(t, w.artifactID)); err != nil {
				return err
			}
			key, err := engineering.NewRevisionKey(w.artifactID, w.revisionID)
			if err != nil {
				return err
			}
			if err := r.Revisions.Put(context.Background(), mustRevisionEnvelopeWithSubject(t, key, w.family, w.subject)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	err = uow.Do(context.Background(), func(r application.Repositories) error {
		got, err := r.Revisions.ListByFamilyAndSubject(context.Background(), engineering.RevisionFamilyRequirement, subjectA)
		if err != nil {
			return err
		}
		wantKeys := []string{"REQ-SUBJ-1/REV-1", "REQ-SUBJ-2/REV-1"}
		if len(got) != len(wantKeys) {
			t.Fatalf("got %d revisions, want %d: %v", len(got), len(wantKeys), got)
		}
		for i, want := range wantKeys {
			if got[i].Key.String() != want {
				t.Errorf("index %d: key = %s, want %s (ascending order)", i, got[i].Key.String(), want)
			}
			if got[i].SubjectKey != subjectA {
				t.Errorf("index %d: subject = %q, want %q", i, got[i].SubjectKey, subjectA)
			}
		}

		// A different family with the same subject is excluded.
		planOnly, err := r.Revisions.ListByFamilyAndSubject(context.Background(), engineering.RevisionFamilyValidationPlan, subjectA)
		if err != nil {
			return err
		}
		if len(planOnly) != 1 || planOnly[0].Key.String() != "VP-SUBJ-1/REV-1" {
			t.Errorf("validation plan query = %v, want exactly VP-SUBJ-1/REV-1", planOnly)
		}

		// No match: an empty slice, not ErrNotFound.
		none, err := r.Revisions.ListByFamilyAndSubject(context.Background(),
			engineering.RevisionFamilyRequirement, engineering.ArtifactSubjectKey("CAP-SUBJ-GHOST"))
		if err != nil {
			return err
		}
		if len(none) != 0 {
			t.Errorf("no-match query returned %d results, want 0", len(none))
		}

		// A subject-less revision never appears under any subject query.
		for _, subject := range []string{subjectA, subjectB} {
			capResults, err := r.Revisions.ListByFamilyAndSubject(context.Background(), engineering.RevisionFamilyCapability, subject)
			if err != nil {
				return err
			}
			if len(capResults) != 0 {
				t.Errorf("capability family query for subject %q returned %d results, want 0 (capability revisions have no subject)", subject, len(capResults))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// testRevisionSubjectKeyIsOptional asserts a capability revision with no
// subject round-trips with an empty SubjectKey and is excluded from every
// subject query (AD-025, FF-016 §3.3, §3.4).
func testRevisionSubjectKeyIsOptional(t *testing.T, uow application.UnitOfWork) {
	key, err := engineering.NewRevisionKey("CAP-NOSUBJ", "REV-1")
	if err != nil {
		t.Fatal(err)
	}
	err = uow.Do(context.Background(), func(r application.Repositories) error {
		if err := r.Artifacts.Put(context.Background(), mustArtifactEnvelope(t, "CAP-NOSUBJ")); err != nil {
			return err
		}
		return r.Revisions.Put(context.Background(), mustRevisionEnvelopeWithSubject(t, key, engineering.RevisionFamilyCapability, ""))
	})
	if err != nil {
		t.Fatal(err)
	}

	err = uow.Do(context.Background(), func(r application.Repositories) error {
		got, found, err := r.Revisions.Get(context.Background(), key)
		if err != nil {
			return err
		}
		if !found {
			t.Fatal("expected the revision to be found")
		}
		if got.SubjectKey != "" {
			t.Errorf("SubjectKey = %q, want empty", got.SubjectKey)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// testRevisionRejectsMalformedSubjectKey asserts NewRevisionEnvelope rejects
// a SubjectKey that does not parse via ParseSubjectKey, identically for
// every adapter -- the rejection happens in the shared constructor, before
// any adapter is reached, so no adapter ever sees a malformed value
// (AD-025, FF-016 §3.6).
func testRevisionRejectsMalformedSubjectKey(t *testing.T, uow application.UnitOfWork) {
	key, err := engineering.NewRevisionKey("REQ-MALFORMED", "REV-1")
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"x":1}`)
	_, err = engineering.NewRevisionEnvelope(engineering.RevisionEnvelopeInput{
		Key: key, RevisionFamily: engineering.RevisionFamilyRequirement,
		ArtifactType: "featureforge:requirement", IntegrityValue: "sha256:abc",
		SubjectKey: "nonsense:CAP-1",
		Payload:    payload, PayloadDigest: engineering.ComputeDigest(payload), RecordedAt: fixedContractTime(),
	})
	if !errors.Is(err, engineering.ErrInvalidEnvelope) {
		t.Errorf("err = %v, want ErrInvalidEnvelope", err)
	}
}

// --- AD-026: SubjectKey participates in RevisionEnvelope.Equal ---

// testRevisionSubjectBearingPutIsIdempotent asserts a subject-bearing
// revision can be re-Put with the identical value, and both the payload and
// the SubjectKey round-trip unchanged.
func testRevisionSubjectBearingPutIsIdempotent(t *testing.T, uow application.UnitOfWork) {
	key, err := engineering.NewRevisionKey("REQ-IDEMP-SUBJ", "REV-1")
	if err != nil {
		t.Fatal(err)
	}
	subject := engineering.ArtifactSubjectKey("CAP-IDEMP-SUBJ")
	env := mustRevisionEnvelopeWithSubject(t, key, engineering.RevisionFamilyRequirement, subject)

	err = uow.Do(context.Background(), func(r application.Repositories) error {
		if err := r.Artifacts.Put(context.Background(), mustArtifactEnvelope(t, "REQ-IDEMP-SUBJ")); err != nil {
			return err
		}
		return r.Revisions.Put(context.Background(), env)
	})
	if err != nil {
		t.Fatal(err)
	}

	for i := range 2 {
		err = uow.Do(context.Background(), func(r application.Repositories) error {
			return r.Revisions.Put(context.Background(), env)
		})
		if err != nil {
			t.Fatalf("put %d: %v", i, err)
		}
	}

	err = uow.Do(context.Background(), func(r application.Repositories) error {
		got, found, err := r.Revisions.Get(context.Background(), key)
		if err != nil {
			return err
		}
		if !found {
			t.Fatal("expected the revision to be found")
		}
		if string(got.Payload) != string(env.Payload) {
			t.Error("payload changed across idempotent re-Puts")
		}
		if got.SubjectKey != subject {
			t.Errorf("SubjectKey = %q, want %q", got.SubjectKey, subject)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// testRevisionSubjectKeyDifferenceConflicts asserts that, for an existing
// RevisionKey with byte-identical Payload, a differing SubjectKey is
// ErrImmutableValueConflict -- in every direction -- and that the failed Put
// leaves the originally stored SubjectKey unchanged rather than merely
// returning an error (AD-026).
func testRevisionSubjectKeyDifferenceConflicts(t *testing.T, uow application.UnitOfWork) {
	subjectA := engineering.ArtifactSubjectKey("CAP-CONFLICT-A")
	subjectB := engineering.ArtifactSubjectKey("CAP-CONFLICT-B")

	cases := []struct {
		name                    string
		artifactID              string
		storedSubject, incoming string
	}{
		{"subject A to subject B", "REQ-CONFLICT-AB", subjectA, subjectB},
		{"empty to subject A", "REQ-CONFLICT-EMPTY-TO-A", "", subjectA},
		{"subject A to empty", "REQ-CONFLICT-A-TO-EMPTY", subjectA, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key, err := engineering.NewRevisionKey(tc.artifactID, "REV-1")
			if err != nil {
				t.Fatal(err)
			}
			stored := mustRevisionEnvelopeWithSubject(t, key, engineering.RevisionFamilyRequirement, tc.storedSubject)
			incoming := mustRevisionEnvelopeWithSubject(t, key, engineering.RevisionFamilyRequirement, tc.incoming)
			if string(stored.Payload) != string(incoming.Payload) {
				t.Fatal("test setup error: stored and incoming payloads must be byte-identical")
			}

			err = uow.Do(context.Background(), func(r application.Repositories) error {
				if err := r.Artifacts.Put(context.Background(), mustArtifactEnvelope(t, tc.artifactID)); err != nil {
					return err
				}
				return r.Revisions.Put(context.Background(), stored)
			})
			if err != nil {
				t.Fatal(err)
			}

			err = uow.Do(context.Background(), func(r application.Repositories) error {
				return r.Revisions.Put(context.Background(), incoming)
			})
			if !errors.Is(err, application.ErrImmutableValueConflict) {
				t.Errorf("err = %v, want ErrImmutableValueConflict", err)
			}

			err = uow.Do(context.Background(), func(r application.Repositories) error {
				got, found, err := r.Revisions.Get(context.Background(), key)
				if err != nil {
					return err
				}
				if !found {
					t.Fatal("expected the original revision to still be present")
				}
				if got.SubjectKey != tc.storedSubject {
					t.Errorf("SubjectKey after a rejected Put = %q, want the original %q unchanged", got.SubjectKey, tc.storedSubject)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// testRevisionSubjectVisibilityAcrossTransactionBoundaries proves FF-016
// §4.2's transaction-visibility claim for a subject-projected revision:
// visible to ListByFamilyAndSubject inside the same Do that wrote it,
// visible from a fresh Do after commit, and absent after a rolled-back Do --
// the same commit/rollback idiom testCommitPersistsAllWrites and
// testRollbackDiscardsAllWrites already use, extended with an in-transaction
// read.
func testRevisionSubjectVisibilityAcrossTransactionBoundaries(t *testing.T, uow application.UnitOfWork) {
	subject := engineering.ArtifactSubjectKey("CAP-VIS-SUBJ")
	key, err := engineering.NewRevisionKey("REQ-VIS-SUBJ", "REV-1")
	if err != nil {
		t.Fatal(err)
	}
	env := mustRevisionEnvelopeWithSubject(t, key, engineering.RevisionFamilyRequirement, subject)

	err = uow.Do(context.Background(), func(r application.Repositories) error {
		if err := r.Artifacts.Put(context.Background(), mustArtifactEnvelope(t, "REQ-VIS-SUBJ")); err != nil {
			return err
		}
		if err := r.Revisions.Put(context.Background(), env); err != nil {
			return err
		}
		inTxn, err := r.Revisions.ListByFamilyAndSubject(context.Background(), engineering.RevisionFamilyRequirement, subject)
		if err != nil {
			return err
		}
		if len(inTxn) != 1 {
			t.Errorf("in-transaction ListByFamilyAndSubject = %d results, want 1 (the write just made in this Do)", len(inTxn))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	err = uow.Do(context.Background(), func(r application.Repositories) error {
		afterCommit, err := r.Revisions.ListByFamilyAndSubject(context.Background(), engineering.RevisionFamilyRequirement, subject)
		if err != nil {
			return err
		}
		if len(afterCommit) != 1 {
			t.Errorf("after commit, ListByFamilyAndSubject = %d results, want 1", len(afterCommit))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	rollbackSubject := engineering.ArtifactSubjectKey("CAP-VIS-ROLLBACK")
	rollbackKey, err := engineering.NewRevisionKey("REQ-VIS-ROLLBACK", "REV-1")
	if err != nil {
		t.Fatal(err)
	}
	rollbackEnv := mustRevisionEnvelopeWithSubject(t, rollbackKey, engineering.RevisionFamilyRequirement, rollbackSubject)
	sentinel := errors.New("deliberate rollback")
	err = uow.Do(context.Background(), func(r application.Repositories) error {
		if err := r.Artifacts.Put(context.Background(), mustArtifactEnvelope(t, "REQ-VIS-ROLLBACK")); err != nil {
			return err
		}
		if err := r.Revisions.Put(context.Background(), rollbackEnv); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want it to match the sentinel unwrapped", err)
	}

	err = uow.Do(context.Background(), func(r application.Repositories) error {
		afterRollback, err := r.Revisions.ListByFamilyAndSubject(context.Background(), engineering.RevisionFamilyRequirement, rollbackSubject)
		if err != nil {
			return err
		}
		if len(afterRollback) != 0 {
			t.Errorf("after rollback, ListByFamilyAndSubject = %d results, want 0", len(afterRollback))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// mustRevisionEnvelopeWithSubject builds a valid RevisionEnvelope for the
// given family and subject, for the AD-025 / FF-016 subtests above that need
// to control both rather than always using RevisionFamilyCapability with no
// subject, as mustRevisionEnvelope does.
func mustRevisionEnvelopeWithSubject(t *testing.T, key engineering.RevisionKey, family engineering.RevisionFamily, subjectKey string) engineering.RevisionEnvelope {
	t.Helper()
	payload := []byte(`{"revision_id":"` + key.RevisionID + `"}`)
	env, err := engineering.NewRevisionEnvelope(engineering.RevisionEnvelopeInput{
		Key: key, RevisionFamily: family,
		ArtifactType: "featureforge:test-artifact", IntegrityValue: "sha256:abc",
		SubjectKey: subjectKey,
		Payload:    payload, PayloadDigest: engineering.ComputeDigest(payload), RecordedAt: fixedContractTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return env
}

func mustFeatureCardID(t *testing.T, s string) domain.FeatureCardID {
	t.Helper()
	id, err := domain.NewFeatureCardID(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustRevisionEnvelope(t *testing.T, key engineering.RevisionKey) engineering.RevisionEnvelope {
	t.Helper()
	payload := []byte(`{"revision_id":"` + key.RevisionID + `"}`)
	env, err := engineering.NewRevisionEnvelope(engineering.RevisionEnvelopeInput{
		Key: key, RevisionFamily: engineering.RevisionFamilyCapability,
		ArtifactType: "featureforge:product-capability", IntegrityValue: "sha256:abc",
		Payload: payload, PayloadDigest: engineering.ComputeDigest(payload), RecordedAt: fixedContractTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return env
}

func mustRecordEnvelope(t *testing.T, kind engineering.RecordKind, id, subjectKey string) engineering.RecordEnvelope {
	t.Helper()
	key, err := engineering.NewRecordKey(kind, id)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"id":"` + id + `"}`)
	env, err := engineering.NewRecordEnvelope(engineering.RecordEnvelopeInput{
		Key: key, SubjectKey: subjectKey,
		OccurredAt: fixedContractTime(), HasOccurredAt: true,
		Payload: payload, PayloadDigest: engineering.ComputeDigest(payload), RecordedAt: fixedContractTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return env
}
