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
	"slices"
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
	t.Run("RevisionListAll", func(t *testing.T) { testRevisionListAll(t, newUOW()) })
	t.Run("RecordListAll", func(t *testing.T) { testRecordListAll(t, newUOW()) })
	t.Run("ReferenceVerification", func(t *testing.T) { testReferenceVerification(t, newUOW()) })
	t.Run("ReturnedSlicesAreCopies", func(t *testing.T) { testReturnedSlicesAreCopies(t, newUOW()) })

	t.Run("CommitPersistsAllWrites", func(t *testing.T) { testCommitPersistsAllWrites(t, newUOW()) })
	t.Run("RollbackDiscardsAllWrites", func(t *testing.T) { testRollbackDiscardsAllWrites(t, newUOW()) })
	t.Run("RollbackOnPanic", func(t *testing.T) { testRollbackOnPanic(t, newUOW()) })
	t.Run("ErrorPropagatesUnwrapped", func(t *testing.T) { testErrorPropagatesUnwrapped(t, newUOW()) })
	t.Run("ConflictAbortsAct", func(t *testing.T) { testConflictAbortsAct(t, newUOW()) })
	t.Run("NestedTransactionRejected", func(t *testing.T) { testNestedTransactionRejected(t, newUOW()) })
	t.Run("AcceptanceJournalHistory", func(t *testing.T) { testAcceptanceJournalHistory(t, newUOW()) })
	t.Run("AcceptanceLookupByRecordID", func(t *testing.T) { testAcceptanceLookupByRecordID(t, newUOW()) })
	t.Run("RevisionOrderHistory", func(t *testing.T) { testRevisionOrderHistory(t, newUOW()) })
	t.Run("SequenceUniquenessEnforced", func(t *testing.T) { testSequenceUniquenessEnforced(t, newUOW()) })
	t.Run("RequirementCriterionTrace", func(t *testing.T) { testRequirementCriterionTrace(t, newUOW()) })
	t.Run("RequirementCriterionTraceRollback", func(t *testing.T) { testRequirementCriterionTraceRollback(t, newUOW()) })
	t.Run("LifecycleDefinitionsPutGetList", func(t *testing.T) { testLifecycleDefinitionsPutGetList(t, newUOW()) })
	t.Run("LifecycleDefinitionVersionsPutGetList", func(t *testing.T) { testLifecycleDefinitionVersionsPutGetList(t, newUOW()) })
	t.Run("LifecycleConfigurationCreateOnly", func(t *testing.T) { testLifecycleConfigurationCreateOnly(t, newUOW()) })
	t.Run("LifecycleConfigurationReferenceAndRollback", func(t *testing.T) { testLifecycleConfigurationReferenceAndRollback(t, newUOW()) })

	// AD-021: invariants the specifications declare and both adapters must
	// now enforce identically.
	t.Run("FeatureCardPutRequiresExistingProject", func(t *testing.T) { testFeatureCardRequiresProject(t, newUOW()) })
	t.Run("CapabilityLinkRequiresExistingFeatureCard", func(t *testing.T) { testCapabilityLinkRequiresFeatureCard(t, newUOW()) })

	// AD-031: the link is a separate monotonic value, materialized by reads
	// but excluded from base FeatureCard Put equality.
	t.Run("CapabilityLinkIsMaterialized", func(t *testing.T) { testCapabilityLinkIsMaterialized(t, newUOW()) })
	t.Run("CapabilityLinkSameValueIsIdempotent", func(t *testing.T) { testCapabilityLinkSameValueIsIdempotent(t, newUOW()) })
	t.Run("CapabilityLinkDifferentValueConflicts", func(t *testing.T) { testCapabilityLinkDifferentValueConflicts(t, newUOW()) })
	t.Run("CapabilityLinkRollsBack", func(t *testing.T) { testCapabilityLinkRollsBack(t, newUOW()) })
	t.Run("FeatureCardBasePutIgnoresMaterializedLink", func(t *testing.T) { testFeatureCardBasePutIgnoresMaterializedLink(t, newUOW()) })
	t.Run("FeatureCardPutCannotEstablishLink", func(t *testing.T) { testFeatureCardPutCannotEstablishLink(t, newUOW()) })

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

func listAllRevisionFixture(t *testing.T) []engineering.RevisionEnvelope {
	t.Helper()
	capabilitySubject := engineering.ArtifactSubjectKey("A-CAP-LIST-ALL")
	specs := []struct {
		artifactID string
		revisionID string
		family     engineering.RevisionFamily
		subject    string
	}{
		{"A-CAP-LIST-ALL", "REV-A", engineering.RevisionFamilyCapability, ""},
		{"B-TRANSITION-LIST-ALL", "REV-B", engineering.RevisionFamilyTransitionRecord, capabilitySubject},
		{"M-PLAN-LIST-ALL", "REV-M", engineering.RevisionFamilyValidationPlan, capabilitySubject},
		{"Y-EVIDENCE-LIST-ALL", "REV-Y", engineering.RevisionFamilyEvidence, ""},
		{"Z-REQUIREMENT-LIST-ALL", "REV-Z", engineering.RevisionFamilyRequirement, capabilitySubject},
	}
	want := make([]engineering.RevisionEnvelope, len(specs))
	for i, spec := range specs {
		key, err := engineering.NewRevisionKey(spec.artifactID, spec.revisionID)
		if err != nil {
			t.Fatal(err)
		}
		want[i] = mustRevisionEnvelopeWithSubject(t, key, spec.family, spec.subject)
	}
	return want
}

func putListAllRevisions(t *testing.T, r application.Repositories, revisions []engineering.RevisionEnvelope) error {
	t.Helper()
	ctx := context.Background()
	// Deliberately insert in neither key nor family order.
	for _, index := range []int{4, 1, 3, 0, 2} {
		revision := revisions[index]
		if err := r.Artifacts.Put(ctx, mustArtifactEnvelope(t, revision.Key.ArtifactID)); err != nil {
			return err
		}
		if err := r.Revisions.Put(ctx, revision); err != nil {
			return err
		}
	}
	return nil
}

func assertRevisionListAll(t *testing.T, got, want []engineering.RevisionEnvelope) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("ListAll returned %d revisions, want %d: %v", len(got), len(want), revisionKeys(got))
	}
	seenFamilies := make(map[engineering.RevisionFamily]bool, len(got))
	for i := range want {
		seenFamilies[got[i].RevisionFamily] = true
		if got[i].Key != want[i].Key || got[i].RevisionFamily != want[i].RevisionFamily ||
			got[i].SubjectKey != want[i].SubjectKey || string(got[i].Payload) != string(want[i].Payload) {
			t.Fatalf("ListAll[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
	for _, family := range []engineering.RevisionFamily{
		engineering.RevisionFamilyCapability,
		engineering.RevisionFamilyRequirement,
		engineering.RevisionFamilyValidationPlan,
		engineering.RevisionFamilyTransitionRecord,
		engineering.RevisionFamilyEvidence,
	} {
		if !seenFamilies[family] {
			t.Errorf("ListAll projection-filtered revision family %q", family)
		}
	}
}

func revisionKeys(revisions []engineering.RevisionEnvelope) []engineering.RevisionKey {
	keys := make([]engineering.RevisionKey, len(revisions))
	for i, revision := range revisions {
		keys[i] = revision.Key
	}
	return keys
}

func testRevisionListAll(t *testing.T, uow application.UnitOfWork) {
	ctx := context.Background()
	want := listAllRevisionFixture(t)
	if err := uow.Do(ctx, func(r application.Repositories) error {
		// Keep the oracle's nested slices independent from the values handed to
		// the adapter, so a shallow storage alias cannot mask a shallow read.
		if err := putListAllRevisions(t, r, listAllRevisionFixture(t)); err != nil {
			return err
		}
		got, err := r.Revisions.ListAll(ctx)
		if err != nil {
			return err
		}
		assertRevisionListAll(t, got, want)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := uow.Do(ctx, func(r application.Repositories) error {
		for i := range 10 {
			got, err := r.Revisions.ListAll(ctx)
			if err != nil {
				return err
			}
			assertRevisionListAll(t, got, want)
			if i == 0 {
				// Both the returned slice and every nested payload are caller-owned.
				got[0].Payload[0] = '!'
				got[1] = engineering.RevisionEnvelope{}
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	rollbackKey, _ := engineering.NewRevisionKey("C-ROLLBACK-LIST-ALL", "REV-C")
	rollbackRevision := mustRevisionEnvelopeWithSubject(t, rollbackKey, engineering.RevisionFamilyRequirement,
		engineering.ArtifactSubjectKey("A-CAP-LIST-ALL"))
	sentinel := errors.New("rollback revision ListAll member")
	err := uow.Do(ctx, func(r application.Repositories) error {
		if err := r.Artifacts.Put(ctx, mustArtifactEnvelope(t, rollbackKey.ArtifactID)); err != nil {
			return err
		}
		if err := r.Revisions.Put(ctx, rollbackRevision); err != nil {
			return err
		}
		got, err := r.Revisions.ListAll(ctx)
		if err != nil {
			return err
		}
		if len(got) != len(want)+1 || !containsRevisionKey(got, rollbackKey) {
			t.Errorf("same-act rollback candidate is not visible: %v", revisionKeys(got))
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("rollback err = %v, want sentinel", err)
	}
	if err := uow.Do(ctx, func(r application.Repositories) error {
		got, err := r.Revisions.ListAll(ctx)
		if err != nil {
			return err
		}
		assertRevisionListAll(t, got, want)
		if containsRevisionKey(got, rollbackKey) {
			t.Errorf("rolled-back revision survived: %v", revisionKeys(got))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func containsRevisionKey(revisions []engineering.RevisionEnvelope, key engineering.RevisionKey) bool {
	for _, revision := range revisions {
		if revision.Key == key {
			return true
		}
	}
	return false
}

func listAllRecordFixture(t *testing.T) []engineering.RecordEnvelope {
	t.Helper()
	subject := engineering.ArtifactSubjectKey("CAP-RECORD-LIST-ALL")
	specs := []struct {
		kind engineering.RecordKind
		id   string
	}{
		{engineering.RecordKindClaim, "Z-CLAIM-LIST-ALL"},
		{engineering.RecordKindDecision, "A-DECISION-LIST-ALL"},
		{engineering.RecordKindExecution, "M-EXECUTION-LIST-ALL"},
		{engineering.RecordKindStateAssignment, "B-STATE-LIST-ALL"},
	}
	want := make([]engineering.RecordEnvelope, len(specs))
	for i, spec := range specs {
		key, err := engineering.NewRecordKey(spec.kind, spec.id)
		if err != nil {
			t.Fatal(err)
		}
		payload := []byte(`{"id":"` + spec.id + `"}`)
		want[i], err = engineering.NewRecordEnvelope(engineering.RecordEnvelopeInput{
			Key: key, SubjectKey: subject, Scope: "featureforge:list-all", Outcome: "peos:test",
			OccurredAt: fixedContractTime(), HasOccurredAt: true,
			CriterionKeys: []string{"criterion:" + spec.id}, EvidenceKeys: []string{"evidence:" + spec.id},
			ExecutionKeys: []string{"execution:" + spec.id}, StateID: "featureforge:test-state",
			Payload: payload, PayloadDigest: engineering.ComputeDigest(payload), RecordedAt: fixedContractTime(),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return want
}

func assertRecordListAll(t *testing.T, got, want []engineering.RecordEnvelope) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("ListAll returned %d records, want %d: %v", len(got), len(want), recordKeys(got))
	}
	seenKinds := make(map[engineering.RecordKind]bool, len(got))
	for i := range want {
		seenKinds[got[i].Key.Kind] = true
		if got[i].Key != want[i].Key || got[i].Kind != want[i].Kind || got[i].SubjectKey != want[i].SubjectKey ||
			got[i].Scope != want[i].Scope || got[i].Outcome != want[i].Outcome || got[i].StateID != want[i].StateID ||
			string(got[i].Payload) != string(want[i].Payload) ||
			!slices.Equal(got[i].CriterionKeys, want[i].CriterionKeys) ||
			!slices.Equal(got[i].EvidenceKeys, want[i].EvidenceKeys) ||
			!slices.Equal(got[i].ExecutionKeys, want[i].ExecutionKeys) {
			t.Fatalf("ListAll[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
	for _, kind := range []engineering.RecordKind{
		engineering.RecordKindDecision,
		engineering.RecordKindExecution,
		engineering.RecordKindClaim,
		engineering.RecordKindStateAssignment,
	} {
		if !seenKinds[kind] {
			t.Errorf("ListAll projection-filtered record kind %q", kind)
		}
	}
}

func recordKeys(records []engineering.RecordEnvelope) []engineering.RecordKey {
	keys := make([]engineering.RecordKey, len(records))
	for i, record := range records {
		keys[i] = record.Key
	}
	return keys
}

func testRecordListAll(t *testing.T, uow application.UnitOfWork) {
	ctx := context.Background()
	want := listAllRecordFixture(t)
	if err := uow.Do(ctx, func(r application.Repositories) error {
		if err := r.Artifacts.Put(ctx, mustArtifactEnvelope(t, "CAP-RECORD-LIST-ALL")); err != nil {
			return err
		}
		candidates := listAllRecordFixture(t)
		for _, index := range []int{3, 0, 2, 1} {
			if err := r.Records.Put(ctx, candidates[index]); err != nil {
				return err
			}
		}
		got, err := r.Records.ListAll(ctx)
		if err != nil {
			return err
		}
		assertRecordListAll(t, got, want)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := uow.Do(ctx, func(r application.Repositories) error {
		for i := range 10 {
			got, err := r.Records.ListAll(ctx)
			if err != nil {
				return err
			}
			assertRecordListAll(t, got, want)
			if i == 0 {
				got[0].Payload[0] = '!'
				got[0].CriterionKeys[0] = "mutated"
				got[0].EvidenceKeys[0] = "mutated"
				got[0].ExecutionKeys[0] = "mutated"
				got[1] = engineering.RecordEnvelope{}
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	rollback := mustRecordEnvelope(t, engineering.RecordKindDecision, "ROLLBACK-LIST-ALL",
		engineering.ArtifactSubjectKey("CAP-RECORD-LIST-ALL"))
	sentinel := errors.New("rollback record ListAll member")
	err := uow.Do(ctx, func(r application.Repositories) error {
		if err := r.Records.Put(ctx, rollback); err != nil {
			return err
		}
		got, err := r.Records.ListAll(ctx)
		if err != nil {
			return err
		}
		if len(got) != len(want)+1 || !containsRecordKey(got, rollback.Key) {
			t.Errorf("same-act rollback candidate is not visible: %v", recordKeys(got))
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("rollback err = %v, want sentinel", err)
	}
	if err := uow.Do(ctx, func(r application.Repositories) error {
		got, err := r.Records.ListAll(ctx)
		if err != nil {
			return err
		}
		assertRecordListAll(t, got, want)
		if containsRecordKey(got, rollback.Key) {
			t.Errorf("rolled-back record survived: %v", recordKeys(got))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func containsRecordKey(records []engineering.RecordEnvelope, key engineering.RecordKey) bool {
	for _, record := range records {
		if record.Key == key {
			return true
		}
	}
	return false
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

func testAcceptanceLookupByRecordID(t *testing.T, uow application.UnitOfWork) {
	revKey, err := engineering.NewRevisionKey("CAP-ACC-LOOKUP", "REV-2")
	if err != nil {
		t.Fatal(err)
	}
	want, err := engineering.NewRevisionAcceptanceRecord(
		"ACC-LOOKUP", revKey, engineering.AcceptanceStateAccepted,
		fixedContractTime(), "featureforge:local-user", "lookup contract",
	)
	if err != nil {
		t.Fatal(err)
	}

	// The lookup must observe a journal append made earlier in the same act.
	err = uow.Do(context.Background(), func(r application.Repositories) error {
		if err := r.Artifacts.Put(context.Background(), mustArtifactEnvelope(t, "CAP-ACC-LOOKUP")); err != nil {
			return err
		}
		if err := r.Revisions.Put(context.Background(), mustRevisionEnvelope(t, revKey)); err != nil {
			return err
		}
		if err := r.RevisionAcceptance.Append(context.Background(), want); err != nil {
			return err
		}
		got, found, err := r.RevisionAcceptance.GetByRecordID(context.Background(), want.RecordID)
		if err != nil {
			return err
		}
		if !found {
			t.Error("GetByRecordID did not observe an append in the current act")
			return nil
		}
		if !sameAcceptanceRecord(got, want) {
			t.Errorf("GetByRecordID = %+v, want %+v", got, want)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// A committed record remains globally addressable without its RevisionKey,
	// while absence follows the standard (zero, false, nil) repository shape.
	err = uow.Do(context.Background(), func(r application.Repositories) error {
		got, found, err := r.RevisionAcceptance.GetByRecordID(context.Background(), want.RecordID)
		if err != nil {
			return err
		}
		if !found {
			t.Error("GetByRecordID did not find the committed record")
		} else if !sameAcceptanceRecord(got, want) {
			t.Errorf("GetByRecordID = %+v, want %+v", got, want)
		}

		missing, found, err := r.RevisionAcceptance.GetByRecordID(context.Background(), "ACC-MISSING")
		if err != nil {
			return err
		}
		if found {
			t.Errorf("GetByRecordID found missing record: %+v", missing)
		}
		if missing != (engineering.RevisionAcceptanceRecord{}) {
			t.Errorf("GetByRecordID missing value = %+v, want zero value", missing)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func sameAcceptanceRecord(a, b engineering.RevisionAcceptanceRecord) bool {
	return a.RecordID == b.RecordID && a.Key == b.Key && a.State == b.State &&
		a.EffectiveAt.Equal(b.EffectiveAt) && a.Actor == b.Actor && a.Reason == b.Reason
}

func testRevisionOrderHistory(t *testing.T, uow application.UnitOfWork) {
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		if err := r.Artifacts.Put(context.Background(), mustArtifactEnvelope(t, "CAP-ORDER")); err != nil {
			return err
		}
		entries := []struct {
			revisionID string
			sequence   int
		}{
			{revisionID: "REV-A-SECOND", sequence: 2},
			{revisionID: "REV-Z-FIRST", sequence: 1},
		}
		for _, entry := range entries {
			revID := entry.revisionID
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
			order, err := engineering.NewRevisionOrderMetadata(key, entry.sequence, fixedContractTime())
			if err != nil {
				return err
			}
			if err := r.RevisionOrder.Put(context.Background(), order); err != nil {
				return err
			}
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
		if len(list) != 2 ||
			list[0].Sequence != 1 || list[0].Key.RevisionID != "REV-Z-FIRST" ||
			list[1].Sequence != 2 || list[1].Key.RevisionID != "REV-A-SECOND" {
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

func testRequirementCriterionTrace(t *testing.T, uow application.UnitOfWork) {
	ctx := context.Background()
	requirementKey, err := engineering.NewRevisionKey("REQ-TRACE", "REQ-TRACE-REV-1")
	if err != nil {
		t.Fatal(err)
	}
	capabilityKey, err := engineering.NewRevisionKey("CAP-TRACE", "CAP-TRACE-REV-1")
	if err != nil {
		t.Fatal(err)
	}
	want, err := engineering.NewRequirementCriterionTrace(requirementKey, capabilityKey, "AC-1", fixedContractTime())
	if err != nil {
		t.Fatal(err)
	}

	err = uow.Do(ctx, func(r application.Repositories) error {
		for _, key := range []engineering.RevisionKey{requirementKey, capabilityKey} {
			if err := r.Artifacts.Put(ctx, mustArtifactEnvelope(t, key.ArtifactID)); err != nil {
				return err
			}
			if err := r.Revisions.Put(ctx, mustRevisionEnvelope(t, key)); err != nil {
				return err
			}
		}
		if err := r.RequirementTraces.Put(ctx, want); err != nil {
			return err
		}
		got, found, err := r.RequirementTraces.Get(ctx, requirementKey)
		if err != nil {
			return err
		}
		if !found || !got.Equal(want) {
			t.Errorf("same-transaction trace = (%+v, %v), want %+v", got, found, want)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Exact re-Put is a no-op across transaction boundaries.
	if err := uow.Do(ctx, func(r application.Repositories) error {
		return r.RequirementTraces.Put(ctx, want)
	}); err != nil {
		t.Fatalf("identical trace re-Put: %v", err)
	}

	err = uow.Do(ctx, func(r application.Repositories) error {
		got, found, err := r.RequirementTraces.Get(ctx, requirementKey)
		if err != nil {
			return err
		}
		if !found || !got.Equal(want) {
			t.Errorf("committed trace = (%+v, %v), want %+v", got, found, want)
		}
		missingKey, _ := engineering.NewRevisionKey("REQ-TRACE", "REQ-MISSING")
		missing, found, err := r.RequirementTraces.Get(ctx, missingKey)
		if err != nil {
			return err
		}
		if found || !missing.IsZero() {
			t.Errorf("missing trace = (%+v, %v), want zero/false", missing, found)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	different, err := engineering.NewRequirementCriterionTrace(requirementKey, capabilityKey, "AC-2", fixedContractTime())
	if err != nil {
		t.Fatal(err)
	}
	err = uow.Do(ctx, func(r application.Repositories) error {
		return r.RequirementTraces.Put(ctx, different)
	})
	if !errors.Is(err, application.ErrImmutableValueConflict) {
		t.Errorf("conflicting trace err = %v, want ErrImmutableValueConflict", err)
	}

	missingRequirement, _ := engineering.NewRevisionKey("REQ-GHOST", "REQ-GHOST-REV-1")
	invalid, _ := engineering.NewRequirementCriterionTrace(missingRequirement, capabilityKey, "AC-1", fixedContractTime())
	err = uow.Do(ctx, func(r application.Repositories) error {
		return r.RequirementTraces.Put(ctx, invalid)
	})
	if !errors.Is(err, application.ErrReferencedValueMissing) {
		t.Errorf("missing requirement reference err = %v, want ErrReferencedValueMissing", err)
	}

	missingCapability, _ := engineering.NewRevisionKey("CAP-GHOST", "CAP-GHOST-REV-1")
	invalid, _ = engineering.NewRequirementCriterionTrace(requirementKey, missingCapability, "AC-1", fixedContractTime())
	err = uow.Do(ctx, func(r application.Repositories) error {
		return r.RequirementTraces.Put(ctx, invalid)
	})
	if !errors.Is(err, application.ErrReferencedValueMissing) {
		t.Errorf("missing capability reference err = %v, want ErrReferencedValueMissing", err)
	}
}

func testRequirementCriterionTraceRollback(t *testing.T, uow application.UnitOfWork) {
	ctx := context.Background()
	requirementKey, _ := engineering.NewRevisionKey("REQ-TRACE-ROLLBACK", "REQ-TRACE-ROLLBACK-REV-1")
	capabilityKey, _ := engineering.NewRevisionKey("CAP-TRACE-ROLLBACK", "CAP-TRACE-ROLLBACK-REV-1")
	trace, _ := engineering.NewRequirementCriterionTrace(requirementKey, capabilityKey, "AC-1", fixedContractTime())
	sentinel := errors.New("rollback trace")
	err := uow.Do(ctx, func(r application.Repositories) error {
		for _, key := range []engineering.RevisionKey{requirementKey, capabilityKey} {
			if err := r.Artifacts.Put(ctx, mustArtifactEnvelope(t, key.ArtifactID)); err != nil {
				return err
			}
			if err := r.Revisions.Put(ctx, mustRevisionEnvelope(t, key)); err != nil {
				return err
			}
		}
		if err := r.RequirementTraces.Put(ctx, trace); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("rollback err = %v, want sentinel", err)
	}
	err = uow.Do(ctx, func(r application.Repositories) error {
		got, found, err := r.RequirementTraces.Get(ctx, requirementKey)
		if err != nil {
			return err
		}
		if found || !got.IsZero() {
			t.Errorf("rolled-back trace survived: (%+v, %v)", got, found)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// --- AD-032: persisted lifecycle configuration parity across adapters ---

func mustLifecycleDefinitionEnvelope(t *testing.T, definitionID, marker string) engineering.LifecycleDefinitionEnvelope {
	t.Helper()
	payload := []byte(`{"definition_id":"` + definitionID + `","marker":"` + marker + `"}`)
	env, err := engineering.NewLifecycleDefinitionEnvelope(definitionID, payload, engineering.ComputeDigest(payload))
	if err != nil {
		t.Fatal(err)
	}
	return env
}

func mustLifecycleVersionEnvelope(t *testing.T, definitionID, versionID, marker string, recordedAt time.Time) engineering.LifecycleDefinitionVersionEnvelope {
	t.Helper()
	key, err := engineering.NewLifecycleDefinitionVersionKey(definitionID, versionID)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"definition_id":"` + definitionID + `","version_id":"` + versionID + `","marker":"` + marker + `"}`)
	env, err := engineering.NewLifecycleDefinitionVersionEnvelope(key, payload, engineering.ComputeDigest(payload), recordedAt)
	if err != nil {
		t.Fatal(err)
	}
	return env
}

func sameLifecycleDefinition(a, b engineering.LifecycleDefinitionEnvelope) bool {
	return a.DefinitionID == b.DefinitionID && string(a.Payload) == string(b.Payload) && a.PayloadDigest.Equal(b.PayloadDigest)
}

func sameLifecycleVersion(a, b engineering.LifecycleDefinitionVersionEnvelope) bool {
	return a.Key == b.Key && string(a.Payload) == string(b.Payload) &&
		a.PayloadDigest.Equal(b.PayloadDigest) && a.RecordedAt.Equal(b.RecordedAt)
}

func testLifecycleDefinitionsPutGetList(t *testing.T, uow application.UnitOfWork) {
	ctx := context.Background()
	ids := []string{"LCD-Z", "LCD-A", "LCD-M"}
	expected := make(map[string]engineering.LifecycleDefinitionEnvelope, len(ids))
	for _, id := range ids {
		expected[id] = mustLifecycleDefinitionEnvelope(t, id, "canonical")
	}

	// Writes and reads are visible inside one act. Mutating the caller-owned
	// payload after Put must not mutate the transaction's stored value.
	if err := uow.Do(ctx, func(r application.Repositories) error {
		for _, id := range ids {
			candidate := mustLifecycleDefinitionEnvelope(t, id, "canonical")
			if err := r.LifecycleDefinitions.PutDefinition(ctx, candidate); err != nil {
				return err
			}
			candidate.Payload[0] = '!'
		}
		got, found, err := r.LifecycleDefinitions.GetDefinition(ctx, "LCD-A")
		if err != nil {
			return err
		}
		if !found || !sameLifecycleDefinition(got, expected["LCD-A"]) {
			t.Errorf("same-act GetDefinition = (%+v, %v), want %+v", got, found, expected["LCD-A"])
		}
		listed, err := r.LifecycleDefinitions.ListDefinitions(ctx)
		if err != nil {
			return err
		}
		if len(listed) != 3 || listed[0].DefinitionID != "LCD-A" || listed[1].DefinitionID != "LCD-M" || listed[2].DefinitionID != "LCD-Z" {
			t.Errorf("ListDefinitions order = %v, want LCD-A, LCD-M, LCD-Z", lifecycleDefinitionIDs(listed))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Get and List both return defensive payload copies, and committed list
	// order remains deterministic across repeated calls.
	if err := uow.Do(ctx, func(r application.Repositories) error {
		got, found, err := r.LifecycleDefinitions.GetDefinition(ctx, "LCD-M")
		if err != nil {
			return err
		}
		if !found {
			t.Fatal("committed lifecycle definition not found")
		}
		got.Payload[0] = '!'
		again, found, err := r.LifecycleDefinitions.GetDefinition(ctx, "LCD-M")
		if err != nil {
			return err
		}
		if !found || !sameLifecycleDefinition(again, expected["LCD-M"]) {
			t.Errorf("GetDefinition leaked mutable payload: (%+v, %v)", again, found)
		}

		for i := range 10 {
			listed, err := r.LifecycleDefinitions.ListDefinitions(ctx)
			if err != nil {
				return err
			}
			if len(listed) != 3 || listed[0].DefinitionID != "LCD-A" || listed[1].DefinitionID != "LCD-M" || listed[2].DefinitionID != "LCD-Z" {
				t.Fatalf("ListDefinitions iteration %d order = %v", i, lifecycleDefinitionIDs(listed))
			}
			listed[0].Payload[0] = '!'
			listed[1] = engineering.LifecycleDefinitionEnvelope{}
		}
		listed, err := r.LifecycleDefinitions.ListDefinitions(ctx)
		if err != nil {
			return err
		}
		if !sameLifecycleDefinition(listed[0], expected["LCD-A"]) {
			t.Errorf("ListDefinitions leaked mutable payload: %+v", listed[0])
		}

		missing, found, err := r.LifecycleDefinitions.GetDefinition(ctx, "LCD-MISSING")
		if err != nil {
			return err
		}
		if found || missing.DefinitionID != "" || len(missing.Payload) != 0 || !missing.PayloadDigest.IsZero() {
			t.Errorf("missing definition = (%+v, %v), want zero/false", missing, found)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func lifecycleDefinitionIDs(definitions []engineering.LifecycleDefinitionEnvelope) []string {
	ids := make([]string, len(definitions))
	for i, definition := range definitions {
		ids[i] = definition.DefinitionID
	}
	return ids
}

func testLifecycleDefinitionVersionsPutGetList(t *testing.T, uow application.UnitOfWork) {
	ctx := context.Background()
	definition := mustLifecycleDefinitionEnvelope(t, "LCD-VERSIONS", "parent")
	otherDefinition := mustLifecycleDefinitionEnvelope(t, "LCD-OTHER", "parent")
	versionIDs := []string{"LCDV-Z", "LCDV-A", "LCDV-M"}
	expected := make(map[string]engineering.LifecycleDefinitionVersionEnvelope, len(versionIDs))
	for i, id := range versionIDs {
		expected[id] = mustLifecycleVersionEnvelope(t, definition.DefinitionID, id, "canonical", fixedContractTime().Add(time.Duration(i)*time.Minute))
	}

	if err := uow.Do(ctx, func(r application.Repositories) error {
		if err := r.LifecycleDefinitions.PutDefinition(ctx, definition); err != nil {
			return err
		}
		if err := r.LifecycleDefinitions.PutDefinition(ctx, otherDefinition); err != nil {
			return err
		}
		for _, id := range versionIDs {
			candidate := mustLifecycleVersionEnvelope(t, definition.DefinitionID, id, "canonical", expected[id].RecordedAt)
			if err := r.LifecycleDefinitions.PutVersion(ctx, candidate); err != nil {
				return err
			}
			candidate.Payload[0] = '!'
		}
		other := mustLifecycleVersionEnvelope(t, otherDefinition.DefinitionID, "LCDV-OTHER", "other", fixedContractTime())
		if err := r.LifecycleDefinitions.PutVersion(ctx, other); err != nil {
			return err
		}

		got, found, err := r.LifecycleDefinitions.GetVersion(ctx, expected["LCDV-A"].Key)
		if err != nil {
			return err
		}
		if !found || !sameLifecycleVersion(got, expected["LCDV-A"]) {
			t.Errorf("same-act GetVersion = (%+v, %v), want %+v", got, found, expected["LCDV-A"])
		}
		listed, err := r.LifecycleDefinitions.ListVersions(ctx, definition.DefinitionID)
		if err != nil {
			return err
		}
		if len(listed) != 3 || listed[0].Key.VersionID != "LCDV-A" || listed[1].Key.VersionID != "LCDV-M" || listed[2].Key.VersionID != "LCDV-Z" {
			t.Errorf("ListVersions order/filter = %v, want LCDV-A, LCDV-M, LCDV-Z", lifecycleVersionIDs(listed))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := uow.Do(ctx, func(r application.Repositories) error {
		got, found, err := r.LifecycleDefinitions.GetVersion(ctx, expected["LCDV-M"].Key)
		if err != nil {
			return err
		}
		if !found {
			t.Fatal("committed lifecycle definition version not found")
		}
		got.Payload[0] = '!'
		again, found, err := r.LifecycleDefinitions.GetVersion(ctx, expected["LCDV-M"].Key)
		if err != nil {
			return err
		}
		if !found || !sameLifecycleVersion(again, expected["LCDV-M"]) {
			t.Errorf("GetVersion leaked mutable payload: (%+v, %v)", again, found)
		}

		for i := range 10 {
			listed, err := r.LifecycleDefinitions.ListVersions(ctx, definition.DefinitionID)
			if err != nil {
				return err
			}
			if len(listed) != 3 || listed[0].Key.VersionID != "LCDV-A" || listed[1].Key.VersionID != "LCDV-M" || listed[2].Key.VersionID != "LCDV-Z" {
				t.Fatalf("ListVersions iteration %d order = %v", i, lifecycleVersionIDs(listed))
			}
			listed[0].Payload[0] = '!'
			listed[1] = engineering.LifecycleDefinitionVersionEnvelope{}
		}
		listed, err := r.LifecycleDefinitions.ListVersions(ctx, definition.DefinitionID)
		if err != nil {
			return err
		}
		if !sameLifecycleVersion(listed[0], expected["LCDV-A"]) {
			t.Errorf("ListVersions leaked mutable payload: %+v", listed[0])
		}

		missingKey, _ := engineering.NewLifecycleDefinitionVersionKey(definition.DefinitionID, "LCDV-MISSING")
		missing, found, err := r.LifecycleDefinitions.GetVersion(ctx, missingKey)
		if err != nil {
			return err
		}
		if found || !missing.Key.IsZero() || len(missing.Payload) != 0 || !missing.PayloadDigest.IsZero() || !missing.RecordedAt.IsZero() {
			t.Errorf("missing version = (%+v, %v), want zero/false", missing, found)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func lifecycleVersionIDs(versions []engineering.LifecycleDefinitionVersionEnvelope) []string {
	ids := make([]string, len(versions))
	for i, version := range versions {
		ids[i] = version.Key.VersionID
	}
	return ids
}

func testLifecycleConfigurationCreateOnly(t *testing.T, uow application.UnitOfWork) {
	ctx := context.Background()
	definition := mustLifecycleDefinitionEnvelope(t, "LCD-CREATE-ONLY", "canonical")
	version := mustLifecycleVersionEnvelope(t, definition.DefinitionID, "LCDV-CREATE-ONLY", "canonical", fixedContractTime())
	if err := uow.Do(ctx, func(r application.Repositories) error {
		if err := r.LifecycleDefinitions.PutDefinition(ctx, definition); err != nil {
			return err
		}
		if err := r.LifecycleDefinitions.PutDefinition(ctx, mustLifecycleDefinitionEnvelope(t, definition.DefinitionID, "canonical")); err != nil {
			return err
		}
		if err := r.LifecycleDefinitions.PutVersion(ctx, version); err != nil {
			return err
		}
		return r.LifecycleDefinitions.PutVersion(ctx, mustLifecycleVersionEnvelope(t, definition.DefinitionID, version.Key.VersionID, "canonical", fixedContractTime()))
	}); err != nil {
		t.Fatalf("same-act identical Put: %v", err)
	}
	if err := uow.Do(ctx, func(r application.Repositories) error {
		if err := r.LifecycleDefinitions.PutDefinition(ctx, mustLifecycleDefinitionEnvelope(t, definition.DefinitionID, "canonical")); err != nil {
			return err
		}
		return r.LifecycleDefinitions.PutVersion(ctx, mustLifecycleVersionEnvelope(t, definition.DefinitionID, version.Key.VersionID, "canonical", fixedContractTime()))
	}); err != nil {
		t.Fatalf("cross-act identical Put: %v", err)
	}

	err := uow.Do(ctx, func(r application.Repositories) error {
		return r.LifecycleDefinitions.PutDefinition(ctx, mustLifecycleDefinitionEnvelope(t, definition.DefinitionID, "different"))
	})
	if !errors.Is(err, application.ErrImmutableValueConflict) {
		t.Errorf("definition conflict err = %v, want ErrImmutableValueConflict", err)
	}
	err = uow.Do(ctx, func(r application.Repositories) error {
		return r.LifecycleDefinitions.PutVersion(ctx, mustLifecycleVersionEnvelope(t, definition.DefinitionID, version.Key.VersionID, "different", fixedContractTime()))
	})
	if !errors.Is(err, application.ErrImmutableValueConflict) {
		t.Errorf("version payload conflict err = %v, want ErrImmutableValueConflict", err)
	}
	err = uow.Do(ctx, func(r application.Repositories) error {
		return r.LifecycleDefinitions.PutVersion(ctx, mustLifecycleVersionEnvelope(t, definition.DefinitionID, version.Key.VersionID, "canonical", fixedContractTime().Add(time.Second)))
	})
	if !errors.Is(err, application.ErrImmutableValueConflict) {
		t.Errorf("version recorded-at conflict err = %v, want ErrImmutableValueConflict", err)
	}
}

func testLifecycleConfigurationReferenceAndRollback(t *testing.T, uow application.UnitOfWork) {
	ctx := context.Background()
	missingParentVersion := mustLifecycleVersionEnvelope(t, "LCD-GHOST", "LCDV-GHOST", "orphan", fixedContractTime())
	err := uow.Do(ctx, func(r application.Repositories) error {
		return r.LifecycleDefinitions.PutVersion(ctx, missingParentVersion)
	})
	if !errors.Is(err, application.ErrReferencedValueMissing) {
		t.Errorf("missing definition err = %v, want ErrReferencedValueMissing", err)
	}

	definition := mustLifecycleDefinitionEnvelope(t, "LCD-ROLLBACK", "rollback")
	version := mustLifecycleVersionEnvelope(t, definition.DefinitionID, "LCDV-ROLLBACK", "rollback", fixedContractTime())
	sentinel := errors.New("rollback lifecycle configuration")
	err = uow.Do(ctx, func(r application.Repositories) error {
		if err := r.LifecycleDefinitions.PutDefinition(ctx, definition); err != nil {
			return err
		}
		if err := r.LifecycleDefinitions.PutVersion(ctx, version); err != nil {
			return err
		}
		gotDefinition, found, err := r.LifecycleDefinitions.GetDefinition(ctx, definition.DefinitionID)
		if err != nil {
			return err
		}
		if !found || !sameLifecycleDefinition(gotDefinition, definition) {
			t.Errorf("definition not visible before rollback: (%+v, %v)", gotDefinition, found)
		}
		gotVersion, found, err := r.LifecycleDefinitions.GetVersion(ctx, version.Key)
		if err != nil {
			return err
		}
		if !found || !sameLifecycleVersion(gotVersion, version) {
			t.Errorf("version not visible before rollback: (%+v, %v)", gotVersion, found)
		}
		definitions, err := r.LifecycleDefinitions.ListDefinitions(ctx)
		if err != nil {
			return err
		}
		versions, err := r.LifecycleDefinitions.ListVersions(ctx, definition.DefinitionID)
		if err != nil {
			return err
		}
		if len(definitions) != 1 || len(versions) != 1 {
			t.Errorf("same-act lists = (%d definitions, %d versions), want (1, 1)", len(definitions), len(versions))
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("rollback err = %v, want sentinel", err)
	}
	if err := uow.Do(ctx, func(r application.Repositories) error {
		gotDefinition, definitionFound, err := r.LifecycleDefinitions.GetDefinition(ctx, definition.DefinitionID)
		if err != nil {
			return err
		}
		gotVersion, versionFound, err := r.LifecycleDefinitions.GetVersion(ctx, version.Key)
		if err != nil {
			return err
		}
		definitions, err := r.LifecycleDefinitions.ListDefinitions(ctx)
		if err != nil {
			return err
		}
		versions, err := r.LifecycleDefinitions.ListVersions(ctx, definition.DefinitionID)
		if err != nil {
			return err
		}
		if definitionFound || gotDefinition.DefinitionID != "" || versionFound || !gotVersion.Key.IsZero() || len(definitions) != 0 || len(versions) != 0 {
			t.Errorf("rolled-back lifecycle configuration survived: definition=(%+v,%v), version=(%+v,%v), lists=(%d,%d)", gotDefinition, definitionFound, gotVersion, versionFound, len(definitions), len(versions))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
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

func testCapabilityLinkRequiresFeatureCard(t *testing.T, uow application.UnitOfWork) {
	ctx := context.Background()
	cardID := mustFeatureCardID(t, "FC-LINK-GHOST")
	err := uow.Do(ctx, func(r application.Repositories) error {
		return r.FeatureCards.LinkCapability(ctx, cardID, "CAP-GHOST")
	})
	if !errors.Is(err, application.ErrReferencedValueMissing) {
		t.Fatalf("missing-card link err = %v, want ErrReferencedValueMissing", err)
	}

	project := mustProject(t, "PRJ-LINK-GHOST")
	card, err := domain.NewFeatureCard(cardID, project.ID(), "Created after failed link", "", fixedContractTime())
	if err != nil {
		t.Fatal(err)
	}
	err = uow.Do(ctx, func(r application.Repositories) error {
		if err := r.Projects.Put(ctx, project); err != nil {
			return err
		}
		if err := r.FeatureCards.Put(ctx, card); err != nil {
			return err
		}
		stored, found, err := r.FeatureCards.Get(ctx, cardID)
		if err != nil {
			return err
		}
		if !found {
			t.Fatal("FeatureCard not found after creation")
		}
		if linked, found := stored.CapabilityArtifactID(); found || linked != "" {
			t.Fatalf("failed missing-card link leaked into later FeatureCard: (%q, %v)", linked, found)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func storeFeatureCard(t *testing.T, uow application.UnitOfWork, suffix string) (domain.Project, domain.FeatureCard) {
	t.Helper()
	project := mustProject(t, "PRJ-LINK-"+suffix)
	card, err := domain.NewFeatureCard(
		mustFeatureCardID(t, "FC-LINK-"+suffix), project.ID(),
		"Capability link "+suffix, "AD-031 repository contract", fixedContractTime(),
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := uow.Do(ctx, func(r application.Repositories) error {
		if err := r.Projects.Put(ctx, project); err != nil {
			return err
		}
		return r.FeatureCards.Put(ctx, card)
	}); err != nil {
		t.Fatal(err)
	}
	return project, card
}

func assertCapabilityLink(t *testing.T, card domain.FeatureCard, want string, wantLinked bool) {
	t.Helper()
	got, linked := card.CapabilityArtifactID()
	if linked != wantLinked || got != want {
		t.Fatalf("CapabilityArtifactID() = (%q, %v), want (%q, %v)", got, linked, want, wantLinked)
	}
}

func testCapabilityLinkIsMaterialized(t *testing.T, uow application.UnitOfWork) {
	project, card := storeFeatureCard(t, uow, "MATERIALIZED")
	ctx := context.Background()
	if err := uow.Do(ctx, func(r application.Repositories) error {
		return r.FeatureCards.LinkCapability(ctx, card.ID(), "CAP-MATERIALIZED")
	}); err != nil {
		t.Fatal(err)
	}

	if err := uow.Do(ctx, func(r application.Repositories) error {
		stored, found, err := r.FeatureCards.Get(ctx, card.ID())
		if err != nil {
			return err
		}
		if !found {
			t.Fatal("linked FeatureCard not found")
		}
		assertCapabilityLink(t, stored, "CAP-MATERIALIZED", true)

		cards, err := r.FeatureCards.ListByProject(ctx, project.ID())
		if err != nil {
			return err
		}
		if len(cards) != 1 || cards[0].ID() != card.ID() {
			t.Fatalf("ListByProject() = %+v, want the linked FeatureCard", cards)
		}
		assertCapabilityLink(t, cards[0], "CAP-MATERIALIZED", true)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func testCapabilityLinkSameValueIsIdempotent(t *testing.T, uow application.UnitOfWork) {
	_, card := storeFeatureCard(t, uow, "IDEMPOTENT")
	ctx := context.Background()
	for i := range 3 {
		if err := uow.Do(ctx, func(r application.Repositories) error {
			return r.FeatureCards.LinkCapability(ctx, card.ID(), "CAP-IDEMPOTENT")
		}); err != nil {
			t.Fatalf("LinkCapability replay %d: %v", i, err)
		}
	}
	if err := uow.Do(ctx, func(r application.Repositories) error {
		stored, found, err := r.FeatureCards.Get(ctx, card.ID())
		if err != nil {
			return err
		}
		if !found {
			t.Fatal("linked FeatureCard not found")
		}
		assertCapabilityLink(t, stored, "CAP-IDEMPOTENT", true)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func testCapabilityLinkDifferentValueConflicts(t *testing.T, uow application.UnitOfWork) {
	_, card := storeFeatureCard(t, uow, "CONFLICT")
	ctx := context.Background()
	if err := uow.Do(ctx, func(r application.Repositories) error {
		return r.FeatureCards.LinkCapability(ctx, card.ID(), "CAP-ORIGINAL")
	}); err != nil {
		t.Fatal(err)
	}
	err := uow.Do(ctx, func(r application.Repositories) error {
		return r.FeatureCards.LinkCapability(ctx, card.ID(), "CAP-DIFFERENT")
	})
	if !errors.Is(err, application.ErrCapabilityAlreadyLinked) {
		t.Fatalf("different link err = %v, want ErrCapabilityAlreadyLinked", err)
	}
	if err := uow.Do(ctx, func(r application.Repositories) error {
		stored, found, err := r.FeatureCards.Get(ctx, card.ID())
		if err != nil {
			return err
		}
		if !found {
			t.Fatal("linked FeatureCard not found")
		}
		assertCapabilityLink(t, stored, "CAP-ORIGINAL", true)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func testCapabilityLinkRollsBack(t *testing.T, uow application.UnitOfWork) {
	_, card := storeFeatureCard(t, uow, "ROLLBACK")
	ctx := context.Background()
	sentinel := errors.New("deliberate capability-link rollback")
	err := uow.Do(ctx, func(r application.Repositories) error {
		if err := r.FeatureCards.LinkCapability(ctx, card.ID(), "CAP-ROLLBACK"); err != nil {
			return err
		}
		stored, found, err := r.FeatureCards.Get(ctx, card.ID())
		if err != nil {
			return err
		}
		if !found {
			t.Fatal("FeatureCard not found inside transaction")
		}
		assertCapabilityLink(t, stored, "CAP-ROLLBACK", true)
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("rollback err = %v, want sentinel", err)
	}
	if err := uow.Do(ctx, func(r application.Repositories) error {
		stored, found, err := r.FeatureCards.Get(ctx, card.ID())
		if err != nil {
			return err
		}
		if !found {
			t.Fatal("FeatureCard not found after rollback")
		}
		assertCapabilityLink(t, stored, "", false)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func testFeatureCardBasePutIgnoresMaterializedLink(t *testing.T, uow application.UnitOfWork) {
	_, base := storeFeatureCard(t, uow, "BASE-PUT")
	ctx := context.Background()
	if err := uow.Do(ctx, func(r application.Repositories) error {
		return r.FeatureCards.LinkCapability(ctx, base.ID(), "CAP-BASE-PUT")
	}); err != nil {
		t.Fatal(err)
	}

	materialized, err := base.WithCapabilityArtifactID("CAP-BASE-PUT")
	if err != nil {
		t.Fatal(err)
	}
	differentProjection, err := base.WithCapabilityArtifactID("CAP-IGNORED")
	if err != nil {
		t.Fatal(err)
	}
	if err := uow.Do(ctx, func(r application.Repositories) error {
		for _, candidate := range []domain.FeatureCard{base, materialized, differentProjection} {
			if err := r.FeatureCards.Put(ctx, candidate); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("identical base Put after linking: %v", err)
	}

	if err := uow.Do(ctx, func(r application.Repositories) error {
		stored, found, err := r.FeatureCards.Get(ctx, base.ID())
		if err != nil {
			return err
		}
		if !found {
			t.Fatal("FeatureCard not found after base Put")
		}
		assertCapabilityLink(t, stored, "CAP-BASE-PUT", true)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	conflict, err := domain.NewFeatureCard(base.ID(), base.ProjectID(), "Changed establishment", base.Description(), base.CreatedAt())
	if err != nil {
		t.Fatal(err)
	}
	err = uow.Do(ctx, func(r application.Repositories) error {
		return r.FeatureCards.Put(ctx, conflict)
	})
	if !errors.Is(err, application.ErrImmutableValueConflict) {
		t.Fatalf("changed base Put err = %v, want ErrImmutableValueConflict", err)
	}
}

func testFeatureCardPutCannotEstablishLink(t *testing.T, uow application.UnitOfWork) {
	ctx := context.Background()
	project := mustProject(t, "PRJ-LINK-PUT-ONLY")
	base, err := domain.NewFeatureCard(
		mustFeatureCardID(t, "FC-LINK-PUT-ONLY"), project.ID(),
		"Put cannot establish link", "AD-031 repository contract", fixedContractTime(),
	)
	if err != nil {
		t.Fatal(err)
	}
	linkedProjection, err := base.WithCapabilityArtifactID("CAP-MUST-BE-IGNORED")
	if err != nil {
		t.Fatal(err)
	}
	if err := uow.Do(ctx, func(r application.Repositories) error {
		if err := r.Projects.Put(ctx, project); err != nil {
			return err
		}
		return r.FeatureCards.Put(ctx, linkedProjection)
	}); err != nil {
		t.Fatal(err)
	}
	if err := uow.Do(ctx, func(r application.Repositories) error {
		stored, found, err := r.FeatureCards.Get(ctx, base.ID())
		if err != nil {
			return err
		}
		if !found {
			t.Fatal("FeatureCard not found after Put")
		}
		assertCapabilityLink(t, stored, "", false)
		return nil
	}); err != nil {
		t.Fatal(err)
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
