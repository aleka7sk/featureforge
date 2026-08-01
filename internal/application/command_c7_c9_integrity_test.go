package application_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

type acceptanceJournalViewRepository struct {
	application.RevisionAcceptanceRepository
	key       engineering.RevisionKey
	transform func([]engineering.RevisionAcceptanceRecord) []engineering.RevisionAcceptanceRecord
}

func (r acceptanceJournalViewRepository) GetByRecordID(ctx context.Context, recordID string) (engineering.RevisionAcceptanceRecord, bool, error) {
	original, err := r.RevisionAcceptanceRepository.ListByRevision(ctx, r.key)
	if err != nil {
		return engineering.RevisionAcceptanceRecord{}, false, err
	}
	transformed := r.transform(append([]engineering.RevisionAcceptanceRecord(nil), original...))
	for _, record := range transformed {
		if record.RecordID == recordID {
			return record, true, nil
		}
	}
	for _, record := range original {
		if record.RecordID == recordID {
			return engineering.RevisionAcceptanceRecord{}, false, nil
		}
	}
	return r.RevisionAcceptanceRepository.GetByRecordID(ctx, recordID)
}

func (r acceptanceJournalViewRepository) ListByRevision(ctx context.Context, key engineering.RevisionKey) ([]engineering.RevisionAcceptanceRecord, error) {
	stored, err := r.RevisionAcceptanceRepository.ListByRevision(ctx, key)
	if err != nil || key != r.key {
		return stored, err
	}
	return r.transform(append([]engineering.RevisionAcceptanceRecord(nil), stored...)), nil
}

func (r acceptanceJournalViewRepository) ListByArtifact(ctx context.Context, artifactID string) ([]engineering.RevisionAcceptanceRecord, error) {
	stored, err := r.RevisionAcceptanceRepository.ListByArtifact(ctx, artifactID)
	if err != nil || artifactID != r.key.ArtifactID {
		return stored, err
	}
	return r.transform(append([]engineering.RevisionAcceptanceRecord(nil), stored...)), nil
}

type missingRevisionOrderRepository struct {
	application.RevisionOrderRepository
	missing engineering.RevisionKey
}

func (r missingRevisionOrderRepository) Get(ctx context.Context, key engineering.RevisionKey) (engineering.RevisionOrderMetadata, bool, error) {
	if key == r.missing {
		return engineering.RevisionOrderMetadata{}, false, nil
	}
	return r.RevisionOrderRepository.Get(ctx, key)
}

func requirementForIntegrity(artifactID, revisionID, statement, member string) application.EstablishRequirementCommand {
	return application.EstablishRequirementCommand{
		ArtifactID: artifactID, RevisionID: revisionID, Statement: statement,
		SubjectArtifactID: "CAP-1", SourceCapabilityRevisionID: "CAP-1-REV-1",
		SourceAcceptanceCriterionKey: "AC-1", AcceptanceRecordID: memberID(member),
	}
}

func seedRequirementForC7C9(t *testing.T, f commandFixture, artifactID, revisionID, member string) application.EstablishRequirementCommand {
	t.Helper()
	cmd := requirementForIntegrity(artifactID, revisionID, "The system SHALL support C7 and C9 integrity tests.", member)
	if _, err := cmd.Execute(context.Background(), f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("seed requirement: %v", err)
	}
	return cmd
}

func planForIntegrity(artifactID, revisionID, activityKey, requirementArtifactID, requirementRevisionID, member string) application.EstablishValidationPlanCommand {
	return application.EstablishValidationPlanCommand{
		ArtifactID: artifactID, RevisionID: revisionID, ScopeArtifactID: "CAP-1",
		AcceptanceRecordID: memberID(member),
		Activities: []application.PlanActivityCommandInput{{
			Key: activityKey, SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
			Method: "manual-review", OutcomeInterpretation: "The governed act is complete.",
			RequirementArtifactID: requirementArtifactID, RequirementRevisionID: requirementRevisionID,
			ExpectedEvidence: []string{"integrity report"},
		}},
	}
}

func acceptanceJournal(t *testing.T, f commandFixture, key engineering.RevisionKey) []engineering.RevisionAcceptanceRecord {
	t.Helper()
	ctx := context.Background()
	var journal []engineering.RevisionAcceptanceRecord
	if err := f.uow.Do(ctx, func(r application.Repositories) error {
		var err error
		journal, err = r.RevisionAcceptance.ListByRevision(ctx, key)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return journal
}

func currentRevision(t *testing.T, f commandFixture, artifactID string) application.CurrentRevisionResult {
	t.Helper()
	ctx := context.Background()
	var current application.CurrentRevisionResult
	if err := f.uow.Do(ctx, func(r application.Repositories) error {
		var err error
		current, err = application.ResolveCurrentRevision(ctx, r, artifactID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return current
}

func withAcceptanceJournalView(f commandFixture, key engineering.RevisionKey, transform func([]engineering.RevisionAcceptanceRecord) []engineering.RevisionAcceptanceRecord) application.UnitOfWork {
	return repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
		r.RevisionAcceptance = acceptanceJournalViewRepository{
			RevisionAcceptanceRepository: r.RevisionAcceptance,
			key:                          key, transform: transform,
		}
	}}
}

func forbidSharedArtifactRewrite(f commandFixture) func() {
	f.store.SetFailureHook(func(kind string, _ int) error {
		if kind == "artifact" {
			return errors.New("test: later revision attempted to rewrite its shared Artifact")
		}
		return nil
	})
	return func() { f.store.SetFailureHook(nil) }
}

func seedSecondCapabilityForManagedSubject(t *testing.T, f commandFixture) {
	t.Helper()
	ctx := context.Background()
	if _, err := (application.CreateFeatureCommand{
		FeatureCardID: "FC-2", ProjectID: "PRJ-1", Title: "Second managed subject",
	}).Execute(ctx, f.uow, f.clock); err != nil {
		t.Fatal(err)
	}
	if _, err := (application.EstablishCapabilitySpecificationCommand{
		FeatureCardID: "FC-2", ArtifactID: "CAP-2", RevisionID: "CAP-2-REV-1",
		Content: mustContent(t, "Second managed subject"),
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	if _, err := (application.AcceptCapabilityRevisionCommand{
		RecordID: "ACC-CAP-2", ArtifactID: "CAP-2", RevisionID: "CAP-2-REV-1", State: engineering.AcceptanceStateAccepted,
	}).Execute(ctx, f.uow, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
}

func zeroAccepted(records []engineering.RevisionAcceptanceRecord) []engineering.RevisionAcceptanceRecord {
	for i := range records {
		if records[i].State == engineering.AcceptanceStateAccepted {
			records[i].State = engineering.AcceptanceStateWithdrawn
		}
	}
	return records
}

func multipleAccepted(records []engineering.RevisionAcceptanceRecord) []engineering.RevisionAcceptanceRecord {
	if len(records) == 0 {
		return records
	}
	extra := records[0]
	extra.RecordID += "-DUPLICATE"
	extra.State = engineering.AcceptanceStateAccepted
	extra.EffectiveAt = extra.EffectiveAt.Add(time.Minute)
	return append(records, extra)
}

func TestC7LaterRevisionUsesSharedArtifactAndReplaysWithoutWrites(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)
	first := requirementForIntegrity("REQ-C7-SHARED", "REQ-C7-SHARED-REV-1", "The system SHALL support revision one.", "MEM-C7-SHARED-1")
	if _, err := first.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("first requirement revision: %v", err)
	}
	f.clock.Advance(time.Hour)
	second := requirementForIntegrity("REQ-C7-SHARED", "REQ-C7-SHARED-REV-2", "The system SHALL support revision two.", "MEM-C7-SHARED-2")
	releaseArtifactGuard := forbidSharedArtifactRewrite(f)
	want, err := second.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
	releaseArtifactGuard()
	if err != nil {
		t.Fatalf("later requirement revision: %v", err)
	}
	current := currentRevision(t, f, "REQ-C7-SHARED")
	if !current.Found || current.Sequence != 2 || current.Revision.Key.RevisionID != second.RevisionID {
		t.Fatalf("current requirement = %+v, want revision 2 at sequence 2", current)
	}
	f.clock.Advance(time.Hour)
	release := forbidPersistenceWrites(f)
	defer release()
	got, err := second.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
	if err != nil {
		t.Fatalf("exact later-revision replay: %v", err)
	}
	if got != want {
		t.Fatalf("replay result = %+v, want %+v", got, want)
	}
}

func TestC7CallerOwnedMembersDoNotRepeatTheRemovedConcatenationCollision(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)

	// Both pairs collapse to ACC-X-Y-Z under the removed
	// "ACC-"+artifact_id+"-"+revision_id convention.
	left := requirementForIntegrity("X-Y", "Z", "The system SHALL keep the left pair distinct.", "MEM-C7-COLLISION-LEFT")
	right := requirementForIntegrity("X", "Y-Z", "The system SHALL keep the right pair distinct.", "MEM-C7-COLLISION-RIGHT")
	if _, err := left.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("left collision pair: %v", err)
	}
	if _, err := right.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("right collision pair: %v", err)
	}

	leftJournal := acceptanceJournal(t, f, mustRevKey(t, left.ArtifactID, left.RevisionID))
	rightJournal := acceptanceJournal(t, f, mustRevKey(t, right.ArtifactID, right.RevisionID))
	if len(leftJournal) != 1 || leftJournal[0].RecordID != *left.AcceptanceRecordID {
		t.Fatalf("left member = %+v, want caller identity %q", leftJournal, *left.AcceptanceRecordID)
	}
	if len(rightJournal) != 1 || rightJournal[0].RecordID != *right.AcceptanceRecordID {
		t.Fatalf("right member = %+v, want caller identity %q", rightJournal, *right.AcceptanceRecordID)
	}
	if err := f.uow.Do(ctx, func(r application.Repositories) error {
		if _, found, err := r.RevisionAcceptance.GetByRecordID(ctx, "ACC-X-Y-Z"); err != nil {
			return err
		} else if found {
			t.Fatal("removed concatenation identity ACC-X-Y-Z was persisted")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	release := forbidPersistenceWrites(f)
	defer release()
	if _, err := left.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("left exact replay: %v", err)
	}
	if _, err := right.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("right exact replay: %v", err)
	}
}

func TestC7AndC9KeepOneStableSubjectAcrossArtifactHistory(t *testing.T) {
	ctx := context.Background()

	t.Run("C7 later requirement revision cannot retarget its artifact", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		seedSecondCapabilityForManagedSubject(t, f)
		first := requirementForIntegrity("REQ-STABLE-SUBJECT", "REQ-STABLE-SUBJECT-REV-1", "The system SHALL remain scoped to CAP-1.", "MEM-REQ-STABLE-1")
		if _, err := first.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		later := requirementForIntegrity("REQ-STABLE-SUBJECT", "REQ-STABLE-SUBJECT-REV-2", "The system SHALL not move to CAP-2.", "MEM-REQ-STABLE-2")
		later.SubjectArtifactID = "CAP-2"
		later.SourceCapabilityRevisionID = "CAP-2-REV-1"
		assertImmutableConflictWithoutWrites(t, f, func() (application.EstablishRequirementResult, error) {
			return later.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
	})

	t.Run("C9 later validation plan revision cannot retarget its scope", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		seedSecondCapabilityForManagedSubject(t, f)
		seedRequirementForC7C9(t, f, "REQ-STABLE-PLAN-1", "REQ-STABLE-PLAN-1-REV-1", "MEM-REQ-STABLE-PLAN-1")
		requirement2 := requirementForIntegrity("REQ-STABLE-PLAN-2", "REQ-STABLE-PLAN-2-REV-1", "The system SHALL validate CAP-2.", "MEM-REQ-STABLE-PLAN-2")
		requirement2.SubjectArtifactID = "CAP-2"
		requirement2.SourceCapabilityRevisionID = "CAP-2-REV-1"
		if _, err := requirement2.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		first := planForIntegrity("VP-STABLE-SCOPE", "VP-STABLE-SCOPE-REV-1", "ACT-STABLE-1", "REQ-STABLE-PLAN-1", "REQ-STABLE-PLAN-1-REV-1", "MEM-VP-STABLE-1")
		if _, err := first.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		later := planForIntegrity("VP-STABLE-SCOPE", "VP-STABLE-SCOPE-REV-2", "ACT-STABLE-2", requirement2.ArtifactID, requirement2.RevisionID, "MEM-VP-STABLE-2")
		later.ScopeArtifactID = "CAP-2"
		later.Activities[0].SubjectArtifactID = "CAP-2"
		later.Activities[0].SubjectRevisionID = "CAP-2-REV-1"
		assertImmutableConflictWithoutWrites(t, f, func() (application.EstablishValidationPlanResult, error) {
			return later.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
	})
}

func TestSubjectDiscoveryRejectsMixedRequirementAndPlanHistories(t *testing.T) {
	ctx := context.Background()

	t.Run("requirement", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		seedSecondCapabilityForManagedSubject(t, f)
		first := seedRequirementForC7C9(t, f, "REQ-MIXED-SUBJECT", "REQ-MIXED-SUBJECT-REV-1", "MEM-REQ-MIXED-SUBJECT-1")
		f.clock.Advance(time.Hour)
		_, revision, err := f.rec.RecordRequirement(engineering.RequirementInput{
			ArtifactID: first.ArtifactID, RevisionID: "REQ-MIXED-SUBJECT-REV-2",
			Statement: "The system SHALL expose mixed subject corruption.", SubjectArtifactID: "CAP-2", RecordedAt: f.clock.Now(),
		})
		if err != nil {
			t.Fatal(err)
		}
		order, err := engineering.NewRevisionOrderMetadata(revision.Key, 2, f.clock.Now())
		if err != nil {
			t.Fatal(err)
		}
		member, err := engineering.NewRevisionAcceptanceRecord("MEM-REQ-MIXED-SUBJECT-2", revision.Key, engineering.AcceptanceStateAccepted, f.clock.Now(), "featureforge:local-user", "requirement established")
		if err != nil {
			t.Fatal(err)
		}
		if err := f.uow.Do(ctx, func(r application.Repositories) error {
			if err := r.Revisions.Put(ctx, revision); err != nil {
				return err
			}
			if err := r.RevisionOrder.Put(ctx, order); err != nil {
				return err
			}
			return r.RevisionAcceptance.Append(ctx, member)
		}); err != nil {
			t.Fatal(err)
		}
		f.clock.Advance(time.Minute)
		withdrawn, err := engineering.NewRevisionAcceptanceRecord(
			"WD-REQ-MIXED-SUBJECT-2", revision.Key, engineering.AcceptanceStateWithdrawn,
			f.clock.Now(), "featureforge:local-user", "mixed historical revision withdrawn",
		)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.uow.Do(ctx, func(r application.Repositories) error {
			return r.RevisionAcceptance.Append(ctx, withdrawn)
		}); err != nil {
			t.Fatal(err)
		}
		assertStoredIntegrityWithoutWrites(t, f, func() (application.EstablishRequirementResult, error) {
			return first.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
		err = f.uow.Do(ctx, func(r application.Repositories) error {
			_, err := application.DiscoverRequirementArtifactIDs(ctx, r, f.rec, "CAP-1")
			return err
		})
		if !errors.Is(err, application.ErrStoredStateIntegrity) {
			t.Fatalf("discovery err = %v, want ErrStoredStateIntegrity", err)
		}
		for name, query := range map[string]func() error{
			"Q3": func() error {
				_, err := application.GetFeatureOverview(ctx, f.uow, f.rec, f.rec, mustFeatureCardID(t, "FC-1"))
				return err
			},
			"Q4": func() error {
				_, err := application.GetFeatureEngineeringStateForCard(ctx, f.uow, f.rec, f.rec, mustFeatureCardID(t, "FC-1"))
				return err
			},
			"Q5": func() error {
				_, err := application.GetFeatureTimelineForCard(ctx, f.uow, f.rec, mustFeatureCardID(t, "FC-1"))
				return err
			},
		} {
			if err := query(); !errors.Is(err, application.ErrStoredStateIntegrity) {
				t.Errorf("%s err = %v, want ErrStoredStateIntegrity", name, err)
			}
		}
	})

	t.Run("validation plan", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		seedSecondCapabilityForManagedSubject(t, f)
		seedRequirementForC7C9(t, f, "REQ-MIXED-PLAN-1", "REQ-MIXED-PLAN-1-REV-1", "MEM-REQ-MIXED-PLAN-1")
		requirement2 := requirementForIntegrity("REQ-MIXED-PLAN-2", "REQ-MIXED-PLAN-2-REV-1", "The system SHALL validate CAP-2 in the corrupt witness.", "MEM-REQ-MIXED-PLAN-2")
		requirement2.SubjectArtifactID = "CAP-2"
		requirement2.SourceCapabilityRevisionID = "CAP-2-REV-1"
		if _, err := requirement2.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		first := planForIntegrity("VP-MIXED-SCOPE", "VP-MIXED-SCOPE-REV-1", "ACT-MIXED-SCOPE-1", "REQ-MIXED-PLAN-1", "REQ-MIXED-PLAN-1-REV-1", "MEM-VP-MIXED-SCOPE-1")
		if _, err := first.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		f.clock.Advance(time.Hour)
		_, revision, err := f.rec.RecordValidationPlan(engineering.PlanInput{
			ArtifactID: first.ArtifactID, RevisionID: "VP-MIXED-SCOPE-REV-2", ScopeArtifactID: "CAP-2",
			Activities: []engineering.PlanActivityInput{{
				Key: "ACT-MIXED-SCOPE-2", SubjectArtifactID: "CAP-2", SubjectRevisionID: "CAP-2-REV-1",
				Method: "manual-review", OutcomeInterpretation: "The stored revision is individually valid.",
				RequirementArtifactID: requirement2.ArtifactID, RequirementRevisionID: requirement2.RevisionID,
				ExpectedEvidence: []string{"mixed scope witness"},
			}}, RecordedAt: f.clock.Now(),
		})
		if err != nil {
			t.Fatal(err)
		}
		order, err := engineering.NewRevisionOrderMetadata(revision.Key, 2, f.clock.Now())
		if err != nil {
			t.Fatal(err)
		}
		member, err := engineering.NewRevisionAcceptanceRecord("MEM-VP-MIXED-SCOPE-2", revision.Key, engineering.AcceptanceStateAccepted, f.clock.Now(), "featureforge:local-user", "validation plan established")
		if err != nil {
			t.Fatal(err)
		}
		if err := f.uow.Do(ctx, func(r application.Repositories) error {
			if err := r.Revisions.Put(ctx, revision); err != nil {
				return err
			}
			if err := r.RevisionOrder.Put(ctx, order); err != nil {
				return err
			}
			return r.RevisionAcceptance.Append(ctx, member)
		}); err != nil {
			t.Fatal(err)
		}
		assertStoredIntegrityWithoutWrites(t, f, func() (application.EstablishValidationPlanResult, error) {
			return first.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
		coherent := planForIntegrity("AAA-VP-MIXED-SCOPE-COMPANION", "AAA-VP-MIXED-SCOPE-COMPANION-REV-1", "ACT-MIXED-SCOPE-COMPANION", "REQ-MIXED-PLAN-1", "REQ-MIXED-PLAN-1-REV-1", "MEM-VP-MIXED-SCOPE-COMPANION")
		if _, err := coherent.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatalf("seed coherent companion plan: %v", err)
		}
		err = f.uow.Do(ctx, func(r application.Repositories) error {
			_, err := application.ResolveApplicableValidationPlanID(ctx, r, f.rec, "CAP-1")
			return err
		})
		if !errors.Is(err, application.ErrStoredStateIntegrity) {
			t.Fatalf("plan resolution err = %v, want ErrStoredStateIntegrity before ambiguity", err)
		}
		for name, query := range map[string]func() error{
			"Q3": func() error {
				_, err := application.GetFeatureOverview(ctx, f.uow, f.rec, f.rec, mustFeatureCardID(t, "FC-1"))
				return err
			},
			"Q4": func() error {
				_, err := application.GetFeatureEngineeringStateForCard(ctx, f.uow, f.rec, f.rec, mustFeatureCardID(t, "FC-1"))
				return err
			},
			"Q5": func() error {
				_, err := application.GetFeatureTimelineForCard(ctx, f.uow, f.rec, mustFeatureCardID(t, "FC-1"))
				return err
			},
		} {
			if err := query(); !errors.Is(err, application.ErrStoredStateIntegrity) {
				t.Errorf("%s err = %v, want ErrStoredStateIntegrity", name, err)
			}
		}
	})
}

func TestC7WithdrawnHistoryReplaysUsingOriginalSemanticMember(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)
	cmd := seedRequirementForC7C9(t, f, "REQ-C7-WITHDRAWN", "REQ-C7-WITHDRAWN-REV-1", "MEM-C7-ORIGINAL")
	f.clock.Advance(time.Hour)
	withdraw := application.AcceptCapabilityRevisionCommand{
		RecordID: "MEM-C7-WITHDRAWAL", ArtifactID: cmd.ArtifactID, RevisionID: cmd.RevisionID,
		State: engineering.AcceptanceStateWithdrawn, Reason: "superseded",
	}
	if _, err := withdraw.Execute(ctx, f.uow, f.rec, f.clock); err != nil {
		t.Fatalf("append withdrawal: %v", err)
	}
	key := mustRevKey(t, cmd.ArtifactID, cmd.RevisionID)
	before := acceptanceJournal(t, f, key)
	if len(before) != 2 || before[0].RecordID != "MEM-C7-ORIGINAL" || before[0].State != engineering.AcceptanceStateAccepted || before[1].State != engineering.AcceptanceStateWithdrawn {
		t.Fatalf("journal = %+v, want original accepted member followed by withdrawal", before)
	}

	f.clock.Advance(time.Hour)
	release := forbidPersistenceWrites(f)
	defer release()
	if _, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("replay with original semantic member: %v", err)
	}
	omitted := cmd
	omitted.AcceptanceRecordID = nil
	if _, err := omitted.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("omitted-ID replay with withdrawn history: %v", err)
	}
	withdrawalAsMember := cmd
	withdrawalAsMember.AcceptanceRecordID = memberID("MEM-C7-WITHDRAWAL")
	if _, err := withdrawalAsMember.Execute(ctx, f.uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrImmutableValueConflict) {
		t.Fatalf("withdrawal supplied as member err = %v, want ErrImmutableValueConflict", err)
	}
	after := acceptanceJournal(t, f, key)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("journal changed on replay/error: before=%+v after=%+v", before, after)
	}
}

func TestC7RejectsZeroAndMultipleAcceptedMembersAsStoredIntegrity(t *testing.T) {
	for name, transform := range map[string]func([]engineering.RevisionAcceptanceRecord) []engineering.RevisionAcceptanceRecord{
		"zero accepted":     zeroAccepted,
		"multiple accepted": multipleAccepted,
	} {
		t.Run(name, func(t *testing.T) {
			f := newCommandFixture()
			seedCapability(t, f)
			cmd := seedRequirementForC7C9(t, f, "REQ-C7-CORRUPT", "REQ-C7-CORRUPT-REV-1", "MEM-C7-CORRUPT")
			key := mustRevKey(t, cmd.ArtifactID, cmd.RevisionID)
			uow := withAcceptanceJournalView(f, key, transform)
			replay := cmd
			replay.AcceptanceRecordID = nil
			release := forbidPersistenceWrites(f)
			defer release()
			if _, err := replay.Execute(context.Background(), uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrStoredStateIntegrity) {
				t.Fatalf("err = %v, want ErrStoredStateIntegrity", err)
			}
		})
	}
}

func TestC7LegacyOpaqueMemberAndMalformedNonmatchingIdentity(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)
	cmd := seedRequirementForC7C9(t, f, "REQ-C7-LEGACY", "REQ-C7-LEGACY-REV-1", "MEM-C7-NORMAL")
	key := mustRevKey(t, cmd.ArtifactID, cmd.RevisionID)
	const opaqueLegacyID = "legacy member id outside new grammar"
	uow := withAcceptanceJournalView(f, key, func(records []engineering.RevisionAcceptanceRecord) []engineering.RevisionAcceptanceRecord {
		for i := range records {
			if records[i].State == engineering.AcceptanceStateAccepted {
				records[i].RecordID = opaqueLegacyID
			}
		}
		return records
	})

	release := forbidPersistenceWrites(f)
	defer release()
	exactOpaque := cmd
	exactOpaque.AcceptanceRecordID = memberID(opaqueLegacyID)
	if _, err := exactOpaque.Execute(ctx, uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("exact opaque legacy member replay: %v", err)
	}
	malformedDifferent := cmd
	malformedDifferent.AcceptanceRecordID = memberID("another malformed identity")
	if _, err := malformedDifferent.Execute(ctx, uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrInvalidCommand) {
		t.Fatalf("malformed nonmatching member err = %v, want ErrInvalidCommand", err)
	}
}

func TestC7CorruptDifferentMemberCandidatePrecedesPrimaryConflict(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)
	primary := seedRequirementForC7C9(t, f, "REQ-C7-PRIMARY", "REQ-C7-PRIMARY-REV-1", "MEM-C7-PRIMARY")
	candidate := seedRequirementForC7C9(t, f, "REQ-C7-CANDIDATE", "REQ-C7-CANDIDATE-REV-1", "MEM-C7-CANDIDATE")
	uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
		r.Artifacts = missingArtifactRepository{
			ArtifactEnvelopeRepository: r.Artifacts,
			missing:                    engineering.ArtifactKey{ArtifactID: candidate.ArtifactID},
		}
	}}
	conflicting := primary
	conflicting.Statement = "The system SHALL conflict with its stored semantics."
	conflicting.AcceptanceRecordID = candidate.AcceptanceRecordID
	release := forbidPersistenceWrites(f)
	defer release()
	if _, err := conflicting.Execute(ctx, uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("err = %v, want corrupt candidate ErrStoredStateIntegrity before primary conflict", err)
	}
}

func TestC9LaterRevisionBecomesCurrentAndReplaysWithoutWrites(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)
	seedRequirementForC7C9(t, f, "REQ-C9-SHARED", "REQ-C9-SHARED-REV-1", "MEM-REQ-C9-SHARED")
	first := planForIntegrity("VP-C9-SHARED", "VP-C9-SHARED-REV-1", "ACT-C9-1", "REQ-C9-SHARED", "REQ-C9-SHARED-REV-1", "MEM-VP-C9-1")
	if _, err := first.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("first plan revision: %v", err)
	}
	f.clock.Advance(time.Hour)
	second := planForIntegrity("VP-C9-SHARED", "VP-C9-SHARED-REV-2", "ACT-C9-2", "REQ-C9-SHARED", "REQ-C9-SHARED-REV-1", "MEM-VP-C9-2")
	releaseArtifactGuard := forbidSharedArtifactRewrite(f)
	want, err := second.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
	releaseArtifactGuard()
	if err != nil {
		t.Fatalf("later plan revision: %v", err)
	}
	current := currentRevision(t, f, "VP-C9-SHARED")
	if !current.Found || current.Sequence != 2 || current.Revision.Key.RevisionID != second.RevisionID {
		t.Fatalf("current validation plan = %+v, want revision 2 at sequence 2", current)
	}
	f.clock.Advance(time.Hour)
	release := forbidPersistenceWrites(f)
	defer release()
	got, err := second.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
	if err != nil {
		t.Fatalf("exact later-plan replay: %v", err)
	}
	if got != want {
		t.Fatalf("replay result = %+v, want %+v", got, want)
	}
}

func TestC9WithdrawnHistoryHasNoCurrentRevisionButStillReplaysItsAct(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)
	seedRequirementForC7C9(t, f, "REQ-C9-WITHDRAWN", "REQ-C9-WITHDRAWN-REV-1", "MEM-REQ-C9-WITHDRAWN")
	cmd := planForIntegrity("VP-C9-WITHDRAWN", "VP-C9-WITHDRAWN-REV-1", "ACT-C9-WITHDRAWN", "REQ-C9-WITHDRAWN", "REQ-C9-WITHDRAWN-REV-1", "MEM-VP-C9-WITHDRAWN")
	want, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
	if err != nil {
		t.Fatalf("seed validation plan: %v", err)
	}
	f.clock.Advance(time.Hour)
	withdraw := application.AcceptCapabilityRevisionCommand{
		RecordID: "WD-VP-C9-WITHDRAWN", ArtifactID: cmd.ArtifactID, RevisionID: cmd.RevisionID,
		State: engineering.AcceptanceStateWithdrawn, Reason: "plan superseded",
	}
	if _, err := withdraw.Execute(ctx, f.uow, f.rec, f.clock); err != nil {
		t.Fatalf("withdraw validation plan revision: %v", err)
	}

	current := currentRevision(t, f, cmd.ArtifactID)
	if current.Found {
		t.Fatalf("current validation plan = %+v, want no current revision after withdrawal", current)
	}
	if len(current.Rationale.Warnings) != 1 || current.Rationale.Warnings[0] != "no accepted revision" {
		t.Fatalf("warnings = %v, want no accepted revision", current.Rationale.Warnings)
	}
	if len(current.Rationale.Rejected) != 1 || current.Rationale.Rejected[0].Key.RevisionID != cmd.RevisionID || current.Rationale.Rejected[0].Reason != "withdrawn" {
		t.Fatalf("rejected rationale = %+v, want withdrawn %s", current.Rationale.Rejected, cmd.RevisionID)
	}

	f.clock.Advance(time.Hour)
	release := forbidPersistenceWrites(f)
	defer release()
	got, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
	if err != nil {
		t.Fatalf("exact C9 replay after withdrawal: %v", err)
	}
	if got != want {
		t.Fatalf("replay result = %+v, want %+v", got, want)
	}
}

func TestC9RejectsPartialZeroAndMultipleMemberState(t *testing.T) {
	for name, override := range map[string]func(commandFixture, engineering.RevisionKey) application.UnitOfWork{
		"missing order": func(f commandFixture, key engineering.RevisionKey) application.UnitOfWork {
			return repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
				r.RevisionOrder = missingRevisionOrderRepository{RevisionOrderRepository: r.RevisionOrder, missing: key}
			}}
		},
		"zero accepted": func(f commandFixture, key engineering.RevisionKey) application.UnitOfWork {
			return withAcceptanceJournalView(f, key, zeroAccepted)
		},
		"multiple accepted": func(f commandFixture, key engineering.RevisionKey) application.UnitOfWork {
			return withAcceptanceJournalView(f, key, multipleAccepted)
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newCommandFixture()
			seedCapability(t, f)
			seedRequirementForC7C9(t, f, "REQ-C9-CORRUPT", "REQ-C9-CORRUPT-REV-1", "MEM-REQ-C9-CORRUPT")
			cmd := planForIntegrity("VP-C9-CORRUPT", "VP-C9-CORRUPT-REV-1", "ACT-C9-CORRUPT", "REQ-C9-CORRUPT", "REQ-C9-CORRUPT-REV-1", "MEM-VP-C9-CORRUPT")
			if _, err := cmd.Execute(context.Background(), f.uow, f.rec, f.rec, f.clock); err != nil {
				t.Fatalf("seed plan: %v", err)
			}
			key := mustRevKey(t, cmd.ArtifactID, cmd.RevisionID)
			uow := override(f, key)
			release := forbidPersistenceWrites(f)
			defer release()
			if _, err := cmd.Execute(context.Background(), uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrStoredStateIntegrity) {
				t.Fatalf("err = %v, want ErrStoredStateIntegrity", err)
			}
		})
	}
}

func TestC9CorruptDifferentMemberCandidatePrecedesPrimaryConflict(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)
	seedRequirementForC7C9(t, f, "REQ-C9-CANDIDATE", "REQ-C9-CANDIDATE-REV-1", "MEM-REQ-C9-CANDIDATE")
	primary := planForIntegrity("VP-C9-PRIMARY", "VP-C9-PRIMARY-REV-1", "ACT-C9-PRIMARY", "REQ-C9-CANDIDATE", "REQ-C9-CANDIDATE-REV-1", "MEM-VP-C9-PRIMARY")
	if _, err := primary.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("seed primary plan: %v", err)
	}
	candidate := planForIntegrity("VP-C9-CANDIDATE", "VP-C9-CANDIDATE-REV-1", "ACT-C9-CANDIDATE", "REQ-C9-CANDIDATE", "REQ-C9-CANDIDATE-REV-1", "MEM-VP-C9-CANDIDATE")
	if _, err := candidate.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("seed candidate plan: %v", err)
	}
	uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
		r.Artifacts = missingArtifactRepository{
			ArtifactEnvelopeRepository: r.Artifacts,
			missing:                    engineering.ArtifactKey{ArtifactID: candidate.ArtifactID},
		}
	}}
	conflicting := primary
	conflicting.Activities = append([]application.PlanActivityCommandInput(nil), primary.Activities...)
	conflicting.Activities[0].OutcomeInterpretation = "Conflicting stored semantics."
	conflicting.AcceptanceRecordID = candidate.AcceptanceRecordID
	release := forbidPersistenceWrites(f)
	defer release()
	if _, err := conflicting.Execute(ctx, uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("err = %v, want corrupt candidate ErrStoredStateIntegrity before primary conflict", err)
	}
}

func TestC7AndC9ClassifyExistingArtifactBeforeUsingCandidateTime(t *testing.T) {
	ctx := context.Background()
	zeroClock := application.NewFixedClock(time.Time{})

	t.Run("coherent foreign artifact is immutable conflict", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		release := forbidPersistenceWrites(f)
		defer release()

		requirement := requirementForIntegrity(
			"CAP-1", "CAP-1-REQ-REV", "The system SHALL classify a foreign owner before reading the clock.", "MEM-C7-FOREIGN-TIME",
		)
		if _, err := requirement.Execute(ctx, f.uow, f.rec, f.rec, zeroClock); !errors.Is(err, application.ErrImmutableValueConflict) {
			t.Fatalf("C7 err = %v, want ErrImmutableValueConflict", err)
		}

		plan := planForIntegrity("CAP-1", "CAP-1-VP-REV", "ACT-C9-FOREIGN-TIME", "REQ-NOT-READ", "REQ-NOT-READ-REV", "MEM-C9-FOREIGN-TIME")
		if _, err := plan.Execute(ctx, f.uow, f.rec, f.rec, zeroClock); !errors.Is(err, application.ErrImmutableValueConflict) {
			t.Fatalf("C9 err = %v, want ErrImmutableValueConflict", err)
		}
	})

	t.Run("corrupt same-family artifact is stored-state integrity", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			seed func(commandFixture) error
			run  func(commandFixture) error
		}{
			{
				name: "requirement",
				seed: func(f commandFixture) error {
					artifact, _, err := f.rec.RecordRequirement(engineering.RequirementInput{
						ArtifactID: "REQ-PARTIAL-TIME", RevisionID: "REQ-PARTIAL-TIME-SEED",
						Statement: "The system SHALL expose partial C7 occupancy.", SubjectArtifactID: "CAP-1", RecordedAt: f.clock.Now(),
					})
					if err != nil {
						return err
					}
					return f.uow.Do(ctx, func(r application.Repositories) error { return r.Artifacts.Put(ctx, artifact) })
				},
				run: func(f commandFixture) error {
					cmd := requirementForIntegrity("REQ-PARTIAL-TIME", "REQ-PARTIAL-TIME-NEW", "The system SHALL not mask corruption with clock validation.", "MEM-C7-PARTIAL-TIME")
					_, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, zeroClock)
					return err
				},
			},
			{
				name: "validation plan",
				seed: func(f commandFixture) error {
					activities := []engineering.PlanActivityInput{{
						Key: "ACT-PARTIAL-TIME", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
						Method: "manual-review", OutcomeInterpretation: "The partial plan is inspected.",
						RequirementArtifactID: "REQ-NOT-READ", RequirementRevisionID: "REQ-NOT-READ-REV",
						ExpectedEvidence: []string{"integrity report"},
					}}
					artifact, _, err := f.rec.RecordValidationPlan(engineering.PlanInput{
						ArtifactID: "VP-PARTIAL-TIME", RevisionID: "VP-PARTIAL-TIME-SEED", ScopeArtifactID: "CAP-1",
						Activities: activities, RecordedAt: f.clock.Now(),
					})
					if err != nil {
						return err
					}
					return f.uow.Do(ctx, func(r application.Repositories) error { return r.Artifacts.Put(ctx, artifact) })
				},
				run: func(f commandFixture) error {
					cmd := planForIntegrity("VP-PARTIAL-TIME", "VP-PARTIAL-TIME-NEW", "ACT-C9-PARTIAL-TIME", "REQ-NOT-READ", "REQ-NOT-READ-REV", "MEM-C9-PARTIAL-TIME")
					_, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, zeroClock)
					return err
				},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				f := newCommandFixture()
				seedCapability(t, f)
				if err := tc.seed(f); err != nil {
					t.Fatalf("seed partial artifact: %v", err)
				}
				release := forbidPersistenceWrites(f)
				defer release()
				if err := tc.run(f); !errors.Is(err, application.ErrStoredStateIntegrity) {
					t.Fatalf("err = %v, want ErrStoredStateIntegrity", err)
				}
			})
		}
	})

	t.Run("same-family later act requires a non-zero candidate time", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		seedRequirementForC7C9(t, f, "REQ-LATER-ZERO-TIME", "REQ-LATER-ZERO-TIME-REV-1", "MEM-REQ-LATER-ZERO-TIME-1")
		requirement := requirementForIntegrity(
			"REQ-LATER-ZERO-TIME", "REQ-LATER-ZERO-TIME-REV-2", "The system SHALL reject a zero candidate time.", "MEM-REQ-LATER-ZERO-TIME-2",
		)
		release := forbidPersistenceWrites(f)
		if _, err := requirement.Execute(ctx, f.uow, f.rec, f.rec, zeroClock); err == nil || errors.Is(err, application.ErrInvalidCommand) || errors.Is(err, application.ErrImmutableValueConflict) {
			t.Fatalf("C7 err = %v, want internal clock failure", err)
		}
		release()

		seedRequirementForC7C9(t, f, "REQ-PLAN-LATER-ZERO", "REQ-PLAN-LATER-ZERO-REV-1", "MEM-REQ-PLAN-LATER-ZERO")
		firstPlan := planForIntegrity("VP-LATER-ZERO-TIME", "VP-LATER-ZERO-TIME-REV-1", "ACT-LATER-ZERO-1", "REQ-PLAN-LATER-ZERO", "REQ-PLAN-LATER-ZERO-REV-1", "MEM-VP-LATER-ZERO-1")
		if _, err := firstPlan.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatalf("seed plan: %v", err)
		}
		laterPlan := planForIntegrity("VP-LATER-ZERO-TIME", "VP-LATER-ZERO-TIME-REV-2", "ACT-LATER-ZERO-2", "REQ-PLAN-LATER-ZERO", "REQ-PLAN-LATER-ZERO-REV-1", "MEM-VP-LATER-ZERO-2")
		release = forbidPersistenceWrites(f)
		defer release()
		if _, err := laterPlan.Execute(ctx, f.uow, f.rec, f.rec, zeroClock); err == nil || errors.Is(err, application.ErrInvalidCommand) || errors.Is(err, application.ErrImmutableValueConflict) {
			t.Fatalf("C9 err = %v, want internal clock failure", err)
		}
	})
}
