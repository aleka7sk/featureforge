package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/domain"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

type strayOrderMetadataRepository struct {
	application.RevisionOrderRepository
	key        engineering.RevisionKey
	order      engineering.RevisionOrderMetadata
	artifactID string
	rootRows   []engineering.RevisionOrderMetadata
}

func (r strayOrderMetadataRepository) Get(ctx context.Context, key engineering.RevisionKey) (engineering.RevisionOrderMetadata, bool, error) {
	if key == r.key {
		return r.order, true, nil
	}
	return r.RevisionOrderRepository.Get(ctx, key)
}

func (r strayOrderMetadataRepository) ListByArtifact(ctx context.Context, artifactID string) ([]engineering.RevisionOrderMetadata, error) {
	if artifactID == r.artifactID {
		return append([]engineering.RevisionOrderMetadata(nil), r.rootRows...), nil
	}
	return r.RevisionOrderRepository.ListByArtifact(ctx, artifactID)
}

type strayAcceptanceMetadataRepository struct {
	application.RevisionAcceptanceRepository
	key        engineering.RevisionKey
	record     engineering.RevisionAcceptanceRecord
	artifactID string
	rootRows   []engineering.RevisionAcceptanceRecord
}

func (r strayAcceptanceMetadataRepository) ListByRevision(ctx context.Context, key engineering.RevisionKey) ([]engineering.RevisionAcceptanceRecord, error) {
	if key == r.key {
		return []engineering.RevisionAcceptanceRecord{r.record}, nil
	}
	return r.RevisionAcceptanceRepository.ListByRevision(ctx, key)
}

func (r strayAcceptanceMetadataRepository) ListByArtifact(ctx context.Context, artifactID string) ([]engineering.RevisionAcceptanceRecord, error) {
	if artifactID == r.artifactID {
		return append([]engineering.RevisionAcceptanceRecord(nil), r.rootRows...), nil
	}
	return r.RevisionAcceptanceRepository.ListByArtifact(ctx, artifactID)
}

func seedLifecycleEntryForIntegrity(t *testing.T, f commandFixture, transitionArtifactID, transitionRevisionID, assignmentID, subjectArtifactID string) application.AssignLifecycleStateCommand {
	t.Helper()
	cmd := application.AssignLifecycleStateCommand{
		AssignmentID: assignmentID, SubjectArtifactID: subjectArtifactID, State: "drafting", IsEntry: true,
		TransitionRecordArtifactID: transitionArtifactID, TransitionRecordRevisionID: transitionRevisionID,
	}
	if _, err := cmd.Execute(context.Background(), f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("seed lifecycle entry: %v", err)
	}
	return cmd
}

func seedLifecycleTransitionForIntegrity(t *testing.T, f commandFixture, transitionArtifactID, transitionRevisionID, assignmentID, subjectArtifactID, state, transitionKey, predecessorID string) application.AssignLifecycleStateCommand {
	t.Helper()
	// AD-032 requires strictly increasing lifecycle effective times. Integrity
	// fixtures first create a coherent transition, then corrupt one selected
	// member; keep the seed graph valid before applying that corruption.
	f.clock.Advance(time.Minute)
	cmd := application.AssignLifecycleStateCommand{
		AssignmentID: assignmentID, SubjectArtifactID: subjectArtifactID, State: state,
		TransitionRecordArtifactID: transitionArtifactID, TransitionRecordRevisionID: transitionRevisionID,
		TransitionKey: transitionKey, FromAssignmentID: predecessorID,
	}
	if _, err := cmd.Execute(context.Background(), f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("seed lifecycle transition: %v", err)
	}
	return cmd
}

func putTransitionMembers(t *testing.T, f commandFixture, artifact engineering.ArtifactEnvelope, revision engineering.RevisionEnvelope, assignment *engineering.RecordEnvelope) {
	t.Helper()
	ctx := context.Background()
	if err := f.uow.Do(ctx, func(r application.Repositories) error {
		if err := r.Artifacts.Put(ctx, artifact); err != nil {
			return err
		}
		if err := r.Revisions.Put(ctx, revision); err != nil {
			return err
		}
		if assignment != nil {
			return r.Records.Put(ctx, *assignment)
		}
		return nil
	}); err != nil {
		t.Fatalf("put transition members: %v", err)
	}
}

func buildTransitionForIntegrity(t *testing.T, f commandFixture, transitionArtifactID, transitionRevisionID, assignmentID, state, transitionKey, predecessorID string) (engineering.ArtifactEnvelope, engineering.RevisionEnvelope, engineering.RecordEnvelope) {
	t.Helper()
	artifact, revision, assignment, err := f.rec.RecordTransition(engineering.TransitionInput{
		AssignmentID: assignmentID, SubjectArtifactID: "CAP-1", State: state,
		EffectiveAt: f.clock.Now(), TransitionRecordArtifactID: transitionArtifactID,
		TransitionRecordRevisionID: transitionRevisionID, TransitionKey: transitionKey,
		FromAssignmentID: predecessorID, AttemptedAt: f.clock.Now(), CompletedAt: f.clock.Now(),
		RecordedAt: f.clock.Now(),
	})
	if err != nil {
		t.Fatalf("build transition: %v", err)
	}
	return artifact, revision, assignment
}

func assertStoredIntegrityWithoutWrites[T any](t *testing.T, f commandFixture, execute func() (T, error)) {
	t.Helper()
	release := forbidPersistenceWrites(f)
	defer release()
	if _, err := execute(); !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("err = %v, want ErrStoredStateIntegrity", err)
	}
}

func TestC6RejectsTransitionTargetAndAssignmentStateMismatch(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)
	entry := seedLifecycleEntryForIntegrity(t, f, "TR-TARGET-MISMATCH", "TR-TARGET-MISMATCH-REV-0", "SA-TARGET-ENTRY", "CAP-1")
	artifact, revision, _ := buildTransitionForIntegrity(
		t, f, entry.TransitionRecordArtifactID, "TR-TARGET-MISMATCH-REV-1", "SA-TARGET-MISMATCH", "specified", "specify", entry.AssignmentID,
	)
	_, _, wrongStateAssignment := buildTransitionForIntegrity(
		t, f, entry.TransitionRecordArtifactID, "TR-TARGET-MISMATCH-REV-1", "SA-TARGET-MISMATCH", "under-validation", "begin-validation", entry.AssignmentID,
	)
	putTransitionMembers(t, f, artifact, revision, &wrongStateAssignment)
	request := application.AssignLifecycleStateCommand{
		AssignmentID: "SA-TARGET-MISMATCH", SubjectArtifactID: "CAP-1", State: "specified",
		TransitionRecordArtifactID: entry.TransitionRecordArtifactID, TransitionRecordRevisionID: revision.Key.RevisionID,
		TransitionKey: "specify", FromAssignmentID: entry.AssignmentID,
	}
	assertStoredIntegrityWithoutWrites(t, f, func() (application.AssignLifecycleStateResult, error) {
		return request.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
	})
}

func TestC6RejectsEntryRevisionWithNonDraftingCanonicalAssignment(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)
	artifact, revision, _, err := f.rec.RecordEntryAssignment(engineering.EntryAssignmentInput{
		AssignmentID: "SA-ENTRY-STATE", SubjectArtifactID: "CAP-1", State: "drafting", EffectiveAt: f.clock.Now(),
		TransitionRecordArtifactID: "TR-ENTRY-STATE", TransitionRecordRevisionID: "TR-ENTRY-STATE-REV-0", RecordedAt: f.clock.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, wrongStateAssignment, err := f.rec.RecordEntryAssignment(engineering.EntryAssignmentInput{
		AssignmentID: "SA-ENTRY-STATE", SubjectArtifactID: "CAP-1", State: "specified", EffectiveAt: f.clock.Now(),
		TransitionRecordArtifactID: "TR-ENTRY-STATE", TransitionRecordRevisionID: "TR-ENTRY-STATE-REV-0", RecordedAt: f.clock.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	putTransitionMembers(t, f, artifact, revision, &wrongStateAssignment)
	request := application.AssignLifecycleStateCommand{
		AssignmentID: "SA-ENTRY-STATE", SubjectArtifactID: "CAP-1", State: "drafting", IsEntry: true,
		TransitionRecordArtifactID: "TR-ENTRY-STATE", TransitionRecordRevisionID: "TR-ENTRY-STATE-REV-0",
	}
	assertStoredIntegrityWithoutWrites(t, f, func() (application.AssignLifecycleStateResult, error) {
		return request.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
	})
}

func TestC6CorruptSharedSiblingPrecedesDifferentAssignmentConflict(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)
	seedRequirementForReplay(t, f)
	entry := seedLifecycleEntryForIntegrity(t, f, "TR-SIBLING", "TR-SIBLING-REV-0", "SA-SIBLING-ENTRY", "CAP-1")
	primary := seedLifecycleTransitionForIntegrity(t, f, "TR-SIBLING", "TR-SIBLING-REV-1", "SA-SIBLING-PRIMARY", "CAP-1", "specified", "specify", entry.AssignmentID)
	artifact, corruptSibling, _ := buildTransitionForIntegrity(
		t, f, "TR-SIBLING", "TR-SIBLING-REV-2", "SA-SIBLING-UNWRITTEN", "under-validation", "begin-validation", primary.AssignmentID,
	)
	putTransitionMembers(t, f, artifact, corruptSibling, nil)
	request := primary
	request.AssignmentID = "SA-SIBLING-DIFFERENT"
	assertStoredIntegrityWithoutWrites(t, f, func() (application.AssignLifecycleStateResult, error) {
		return request.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
	})
}

func TestC6CorruptCandidateRootPrecedesAbsentRevisionAssignmentConflict(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)
	candidate := seedLifecycleEntryForIntegrity(t, f, "TR-CANDIDATE-CORRUPT", "TR-CANDIDATE-CORRUPT-REV-0", "SA-CANDIDATE-OCCUPIED", "CAP-1")
	artifact, corruptSibling, _ := buildTransitionForIntegrity(
		t, f, candidate.TransitionRecordArtifactID, "TR-CANDIDATE-CORRUPT-REV-1", "SA-CANDIDATE-UNWRITTEN", "specified", "specify", candidate.AssignmentID,
	)
	putTransitionMembers(t, f, artifact, corruptSibling, nil)
	request := application.AssignLifecycleStateCommand{
		AssignmentID: candidate.AssignmentID, SubjectArtifactID: "CAP-1", State: "drafting", IsEntry: true,
		TransitionRecordArtifactID: "TR-REQUEST-ABSENT", TransitionRecordRevisionID: "TR-REQUEST-ABSENT-REV-0",
	}
	assertStoredIntegrityWithoutWrites(t, f, func() (application.AssignLifecycleStateResult, error) {
		return request.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
	})
}

func TestC6AssignmentCandidateIntegrityPrecedesForeignRootConflict(t *testing.T) {
	ctx := context.Background()

	t.Run("dangling candidate returns integrity", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		_, _, dangling, err := f.rec.RecordEntryAssignment(engineering.EntryAssignmentInput{
			AssignmentID: "SA-SECONDARY-DANGLING", SubjectArtifactID: "CAP-1", State: "drafting",
			EffectiveAt: f.clock.Now(), TransitionRecordArtifactID: "TR-DANGLING",
			TransitionRecordRevisionID: "TR-DANGLING-REV-0", RecordedAt: f.clock.Now(),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := f.uow.Do(ctx, func(r application.Repositories) error { return r.Records.Put(ctx, dangling) }); err != nil {
			t.Fatalf("seed dangling assignment: %v", err)
		}
		cmd := application.AssignLifecycleStateCommand{
			AssignmentID: "SA-SECONDARY-DANGLING", SubjectArtifactID: "CAP-1", State: "drafting", IsEntry: true,
			TransitionRecordArtifactID: "CAP-1", TransitionRecordRevisionID: "CAP-1-TR-REQUEST",
		}
		assertStoredIntegrityWithoutWrites(t, f, func() (application.AssignLifecycleStateResult, error) {
			return cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
	})

	t.Run("coherent candidate preserves immutable conflict", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		seedLifecycleEntryForIntegrity(t, f, "TR-SECONDARY-COHERENT", "TR-SECONDARY-COHERENT-REV-0", "SA-SECONDARY-COHERENT", "CAP-1")
		cmd := application.AssignLifecycleStateCommand{
			AssignmentID: "SA-SECONDARY-COHERENT", SubjectArtifactID: "CAP-1", State: "drafting", IsEntry: true,
			TransitionRecordArtifactID: "CAP-1", TransitionRecordRevisionID: "CAP-1-TR-REQUEST",
		}
		release := forbidPersistenceWrites(f)
		defer release()
		if _, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrImmutableValueConflict) {
			t.Fatalf("err = %v, want ErrImmutableValueConflict", err)
		}
	})
}

func TestC6RejectsShiftedTransitionArtifactTimeOnEntryReplay(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)
	cmd := seedLifecycleEntryForIntegrity(t, f, "TR-SHIFTED-TIME", "TR-SHIFTED-TIME-REV-0", "SA-SHIFTED-TIME", "CAP-1")
	uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
		r.Artifacts = shiftedArtifactTimeRepository{
			ArtifactEnvelopeRepository: r.Artifacts,
			key:                        engineering.ArtifactKey{ArtifactID: cmd.TransitionRecordArtifactID},
			delta:                      time.Minute,
		}
	}}
	assertStoredIntegrityWithoutWrites(t, f, func() (application.AssignLifecycleStateResult, error) {
		return cmd.Execute(ctx, uow, f.rec, f.rec, f.clock)
	})
}

func TestC6CleanTransitionRootCannotBeReusedByAnotherSubject(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)
	if _, err := (application.CreateFeatureCommand{
		FeatureCardID: "FC-TR-OTHER", ProjectID: "PRJ-1", Title: "Other lifecycle subject",
	}).Execute(ctx, f.uow, f.clock); err != nil {
		t.Fatal(err)
	}
	if _, err := (application.EstablishCapabilitySpecificationCommand{
		FeatureCardID: "FC-TR-OTHER", ArtifactID: "CAP-TR-OTHER", RevisionID: "CAP-TR-OTHER-REV-1",
		Content: mustContent(t, "Other lifecycle subject"),
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	entry := seedLifecycleEntryForIntegrity(t, f, "TR-SUBJECT-OWNER", "TR-SUBJECT-OWNER-REV-0", "SA-SUBJECT-OWNER", "CAP-1")
	request := application.AssignLifecycleStateCommand{
		AssignmentID: "SA-OTHER-SUBJECT", SubjectArtifactID: "CAP-TR-OTHER", State: "drafting", IsEntry: true,
		TransitionRecordArtifactID: entry.TransitionRecordArtifactID, TransitionRecordRevisionID: "TR-SUBJECT-OWNER-REV-OTHER",
	}
	assertImmutableConflictWithoutWrites(t, f, func() (application.AssignLifecycleStateResult, error) {
		return request.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
	})
}

func TestC7RejectsCanonicalOrphanEvidenceForeignOccupancy(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)
	artifact, revision, err := f.rec.RecordEvidence(engineering.EvidenceInput{
		ArtifactID: "EV-C7-ORPHAN", RevisionID: "EV-C7-ORPHAN-REV-1",
		Locator: "https://evidence.example/orphan", RecordedAt: f.clock.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.uow.Do(ctx, func(r application.Repositories) error {
		if err := r.Artifacts.Put(ctx, artifact); err != nil {
			return err
		}
		return r.Revisions.Put(ctx, revision)
	}); err != nil {
		t.Fatal(err)
	}
	request := application.EstablishRequirementCommand{
		ArtifactID: artifact.Key.ArtifactID, RevisionID: revision.Key.RevisionID,
		Statement: "The system SHALL not accept orphan evidence occupancy.", SubjectArtifactID: "CAP-1",
		SourceCapabilityRevisionID: "CAP-1-REV-1", SourceAcceptanceCriterionKey: "AC-1",
		AcceptanceRecordID: memberID("MEM-C7-ORPHAN"),
	}
	assertStoredIntegrityWithoutWrites(t, f, func() (application.EstablishRequirementResult, error) {
		return request.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
	})
}

func TestC6ExactReplayRejectsUnexpectedRevisionOrderMetadata(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)
	cmd := seedLifecycleEntryForIntegrity(t, f, "TR-EXTRA-ORDER", "TR-EXTRA-ORDER-REV-0", "SA-EXTRA-ORDER", "CAP-1")
	key := mustRevKey(t, cmd.TransitionRecordArtifactID, cmd.TransitionRecordRevisionID)
	order, err := engineering.NewRevisionOrderMetadata(key, 1, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := f.uow.Do(ctx, func(r application.Repositories) error {
		return r.RevisionOrder.Put(ctx, order)
	}); err != nil {
		t.Fatal(err)
	}
	assertStoredIntegrityWithoutWrites(t, f, func() (application.AssignLifecycleStateResult, error) {
		return cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
	})
}

func TestC10ExactReplayRejectsUnexpectedStructuredContent(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedSemanticValidationFoundation(t, f)
	cmd := semanticValidationRun("ER-EXTRA-CONTENT")
	if _, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	key := mustRevKey(t, cmd.EvidenceArtifactID, cmd.EvidenceRevisionID)
	if err := f.uow.Do(ctx, func(r application.Repositories) error {
		return r.StructuredContent.Put(ctx, key, mustContent(t, "Unexpected evidence content"))
	}); err != nil {
		t.Fatal(err)
	}
	assertStoredIntegrityWithoutWrites(t, f, func() (application.RecordValidationRunResult, error) {
		return cmd.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
	})
}

func TestAbsentC7AndC9PairsRejectStrayStructuredContent(t *testing.T) {
	ctx := context.Background()
	for name, execute := range map[string]func(commandFixture, application.UnitOfWork) error{
		"C7": func(f commandFixture, uow application.UnitOfWork) error {
			cmd := application.EstablishRequirementCommand{
				ArtifactID: "REQ-STRAY-CONTENT", RevisionID: "REQ-STRAY-CONTENT-REV-1",
				Statement: "The system SHALL reject stray content.", SubjectArtifactID: "CAP-UNREAD",
				SourceCapabilityRevisionID: "CAP-UNREAD-REV-1", SourceAcceptanceCriterionKey: "AC-1",
				AcceptanceRecordID: memberID("MEM-REQ-STRAY-CONTENT"),
			}
			_, err := cmd.Execute(ctx, uow, f.rec, f.rec, f.clock)
			return err
		},
		"C9": func(f commandFixture, uow application.UnitOfWork) error {
			cmd := staticPlanCommand()
			cmd.ArtifactID = "VP-STRAY-CONTENT"
			cmd.RevisionID = "VP-STRAY-CONTENT-REV-1"
			_, err := cmd.Execute(ctx, uow, f.rec, f.rec, f.clock)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newCommandFixture()
			artifactID, revisionID := "REQ-STRAY-CONTENT", "REQ-STRAY-CONTENT-REV-1"
			if name == "C9" {
				artifactID, revisionID = "VP-STRAY-CONTENT", "VP-STRAY-CONTENT-REV-1"
			}
			key := mustRevKey(t, artifactID, revisionID)
			uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
				r.StructuredContent = substitutedContentRepository{
					StructuredContentRepository: r.StructuredContent,
					key:                         key, content: mustContent(t, "Stray content"),
				}
			}}
			release := forbidPersistenceWrites(f)
			defer release()
			if err := execute(f, uow); !errors.Is(err, application.ErrStoredStateIntegrity) {
				t.Fatalf("err = %v, want ErrStoredStateIntegrity", err)
			}
		})
	}
}

func TestAbsentC6AndC10PairsRejectStrayUnmanagedMetadata(t *testing.T) {
	ctx := context.Background()
	for _, commandName := range []string{"C6", "C10"} {
		for _, metadata := range []string{"order", "acceptance"} {
			t.Run(commandName+" "+metadata, func(t *testing.T) {
				f := newCommandFixture()
				artifactID, revisionID := "TR-STRAY-METADATA", "TR-STRAY-METADATA-REV-0"
				if commandName == "C10" {
					artifactID, revisionID = "EV-STRAY-METADATA", "EV-STRAY-METADATA-REV-1"
				}
				key := mustRevKey(t, artifactID, revisionID)
				uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
					if metadata == "order" {
						order, err := engineering.NewRevisionOrderMetadata(key, 1, f.clock.Now())
						if err != nil {
							t.Fatal(err)
						}
						r.RevisionOrder = strayOrderMetadataRepository{
							RevisionOrderRepository: r.RevisionOrder, key: key, order: order,
						}
						return
					}
					record, err := engineering.NewRevisionAcceptanceRecord(
						"ACC-STRAY-METADATA", key, engineering.AcceptanceStateAccepted, f.clock.Now(), "featureforge:local-user", "stray",
					)
					if err != nil {
						t.Fatal(err)
					}
					r.RevisionAcceptance = strayAcceptanceMetadataRepository{
						RevisionAcceptanceRepository: r.RevisionAcceptance, key: key, record: record,
					}
				}}
				release := forbidPersistenceWrites(f)
				defer release()
				if commandName == "C6" {
					cmd := application.AssignLifecycleStateCommand{
						AssignmentID: "SA-STRAY-METADATA", SubjectArtifactID: "CAP-UNREAD", State: "drafting", IsEntry: true,
						TransitionRecordArtifactID: artifactID, TransitionRecordRevisionID: revisionID,
					}
					if _, err := cmd.Execute(ctx, uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrStoredStateIntegrity) {
						t.Fatalf("err = %v, want ErrStoredStateIntegrity", err)
					}
					return
				}
				cmd := staticRunCommand()
				cmd.EvidenceArtifactID = artifactID
				cmd.EvidenceRevisionID = revisionID
				if _, err := cmd.Execute(ctx, uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrStoredStateIntegrity) {
					t.Fatalf("err = %v, want ErrStoredStateIntegrity", err)
				}
			})
		}
	}
}

func TestC6RejectsSiblingAssignmentUnderMissingTransitionRoot(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)
	_, _, sibling, err := f.rec.RecordEntryAssignment(engineering.EntryAssignmentInput{
		AssignmentID: "SA-HIDDEN-ROOT-OLD", SubjectArtifactID: "CAP-1", State: "drafting",
		EffectiveAt: f.clock.Now(), TransitionRecordArtifactID: "TR-HIDDEN-ROOT",
		TransitionRecordRevisionID: "TR-HIDDEN-ROOT-REV-2", RecordedAt: f.clock.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.uow.Do(ctx, func(r application.Repositories) error {
		return r.Records.Put(ctx, sibling)
	}); err != nil {
		t.Fatalf("seed sibling assignment: %v", err)
	}
	request := application.AssignLifecycleStateCommand{
		AssignmentID: "SA-HIDDEN-ROOT-NEW", SubjectArtifactID: "CAP-1", State: "drafting", IsEntry: true,
		TransitionRecordArtifactID: "TR-HIDDEN-ROOT", TransitionRecordRevisionID: "TR-HIDDEN-ROOT-REV-1",
	}
	assertStoredIntegrityWithoutWrites(t, f, func() (application.AssignLifecycleStateResult, error) {
		return request.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
	})
}

func TestC10RejectsSiblingExecutionUnderMissingEvidenceRoot(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedSemanticValidationFoundation(t, f)
	sibling, err := f.rec.RecordExecution(engineering.ExecutionInput{
		ExecutionID: "ER-HIDDEN-ROOT-OLD", PlanArtifactID: "VP-SEMANTIC", PlanRevisionID: "VP-SEMANTIC-REV-1",
		ActivityKey: "ACT-SEMANTIC", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		Method: "manual-review", Outcome: "completed", CompletedAt: f.clock.Now(),
		EvidenceArtifactID: "EV-HIDDEN-ROOT", EvidenceRevisionID: "EV-HIDDEN-ROOT-REV-2", RecordedAt: f.clock.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.uow.Do(ctx, func(r application.Repositories) error {
		return r.Records.Put(ctx, sibling)
	}); err != nil {
		t.Fatalf("seed sibling execution: %v", err)
	}
	request := semanticValidationRun("ER-HIDDEN-ROOT-NEW")
	request.EvidenceArtifactID = "EV-HIDDEN-ROOT"
	request.EvidenceRevisionID = "EV-HIDDEN-ROOT-REV-1"
	assertStoredIntegrityWithoutWrites(t, f, func() (application.RecordValidationRunResult, error) {
		return request.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
	})
}

func TestC7AndC9RejectSiblingMetadataUnderMissingArtifactRoot(t *testing.T) {
	ctx := context.Background()
	for _, commandName := range []string{"C7", "C9"} {
		for _, metadata := range []string{"order", "acceptance"} {
			t.Run(commandName+" "+metadata, func(t *testing.T) {
				f := newCommandFixture()
				artifactID := "REQ-HIDDEN-METADATA"
				if commandName == "C9" {
					artifactID = "VP-HIDDEN-METADATA"
				}
				siblingKey := mustRevKey(t, artifactID, artifactID+"-REV-2")
				uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
					if metadata == "order" {
						order, err := engineering.NewRevisionOrderMetadata(siblingKey, 2, f.clock.Now())
						if err != nil {
							t.Fatal(err)
						}
						r.RevisionOrder = strayOrderMetadataRepository{
							RevisionOrderRepository: r.RevisionOrder, artifactID: artifactID,
							rootRows: []engineering.RevisionOrderMetadata{order},
						}
						return
					}
					record, err := engineering.NewRevisionAcceptanceRecord(
						"ACC-HIDDEN-METADATA", siblingKey, engineering.AcceptanceStateAccepted,
						f.clock.Now(), "featureforge:local-user", "hidden",
					)
					if err != nil {
						t.Fatal(err)
					}
					r.RevisionAcceptance = strayAcceptanceMetadataRepository{
						RevisionAcceptanceRepository: r.RevisionAcceptance, artifactID: artifactID,
						rootRows: []engineering.RevisionAcceptanceRecord{record},
					}
				}}
				release := forbidPersistenceWrites(f)
				defer release()
				if commandName == "C7" {
					cmd := application.EstablishRequirementCommand{
						ArtifactID: artifactID, RevisionID: artifactID + "-REV-1",
						Statement: "The system SHALL reject hidden root metadata.", SubjectArtifactID: "CAP-UNREAD",
						SourceCapabilityRevisionID: "CAP-UNREAD-REV-1", SourceAcceptanceCriterionKey: "AC-1",
						AcceptanceRecordID: memberID("MEM-REQ-HIDDEN-METADATA"),
					}
					if _, err := cmd.Execute(ctx, uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrStoredStateIntegrity) {
						t.Fatalf("err = %v, want ErrStoredStateIntegrity", err)
					}
					return
				}
				cmd := staticPlanCommand()
				cmd.ArtifactID = artifactID
				cmd.RevisionID = artifactID + "-REV-1"
				cmd.AcceptanceRecordID = memberID("MEM-VP-HIDDEN-METADATA")
				if _, err := cmd.Execute(ctx, uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrStoredStateIntegrity) {
					t.Fatalf("err = %v, want ErrStoredStateIntegrity", err)
				}
			})
		}
	}
}

func TestC3RejectsFeatureCardLinkToMissingCapabilityRoot(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	if _, err := (application.CreateProjectCommand{ProjectID: "PRJ-HIDDEN-LINK", Name: "Hidden link"}).Execute(ctx, f.uow, f.clock); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"FC-HIDDEN-LINK-OLD", "FC-HIDDEN-LINK-NEW"} {
		if _, err := (application.CreateFeatureCommand{
			FeatureCardID: id, ProjectID: "PRJ-HIDDEN-LINK", Title: id,
		}).Execute(ctx, f.uow, f.clock); err != nil {
			t.Fatal(err)
		}
	}
	oldID, err := domain.NewFeatureCardID("FC-HIDDEN-LINK-OLD")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.uow.Do(ctx, func(r application.Repositories) error {
		return r.FeatureCards.LinkCapability(ctx, oldID, "CAP-HIDDEN-LINK")
	}); err != nil {
		t.Fatalf("seed dangling capability link: %v", err)
	}
	request := application.EstablishCapabilitySpecificationCommand{
		FeatureCardID: "FC-HIDDEN-LINK-NEW", ArtifactID: "CAP-HIDDEN-LINK", RevisionID: "CAP-HIDDEN-LINK-REV-1",
		Content: mustContent(t, "Hidden owner must block capability creation"),
	}
	assertStoredIntegrityWithoutWrites(t, f, func() (application.EstablishCapabilitySpecificationResult, error) {
		return request.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
	})
}

func TestC6RejectsStateAssignmentParentUnderForeignArtifact(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)
	_, _, assignment, err := f.rec.RecordEntryAssignment(engineering.EntryAssignmentInput{
		AssignmentID: "SA-FOREIGN-PARENT", SubjectArtifactID: "CAP-1", State: "drafting",
		EffectiveAt: f.clock.Now(), TransitionRecordArtifactID: "CAP-1",
		TransitionRecordRevisionID: "CAP-1-REV-1", RecordedAt: f.clock.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.uow.Do(ctx, func(r application.Repositories) error {
		return r.Records.Put(ctx, assignment)
	}); err != nil {
		t.Fatal(err)
	}
	request := application.AssignLifecycleStateCommand{
		AssignmentID: "SA-FOREIGN-PARENT-NEW", SubjectArtifactID: "CAP-1", State: "drafting", IsEntry: true,
		TransitionRecordArtifactID: "CAP-1", TransitionRecordRevisionID: "CAP-1-REV-NEW",
	}
	assertStoredIntegrityWithoutWrites(t, f, func() (application.AssignLifecycleStateResult, error) {
		return request.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
	})
}

func TestC10RejectsExecutionEvidenceUnderForeignArtifact(t *testing.T) {
	for _, requestedRevisionID := range []string{"CAP-1-REV-1", "CAP-1-REV-NEW"} {
		t.Run(requestedRevisionID, func(t *testing.T) {
			f := newCommandFixture()
			ctx := context.Background()
			seedSemanticValidationFoundation(t, f)
			owner, err := f.rec.RecordExecution(engineering.ExecutionInput{
				ExecutionID: "ER-FOREIGN-EVIDENCE-OLD", PlanArtifactID: "VP-SEMANTIC", PlanRevisionID: "VP-SEMANTIC-REV-1",
				ActivityKey: "ACT-SEMANTIC", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
				Method: "manual-review", Outcome: "completed", CompletedAt: f.clock.Now(),
				EvidenceArtifactID: "CAP-1", EvidenceRevisionID: "CAP-1-REV-1", RecordedAt: f.clock.Now(),
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := f.uow.Do(ctx, func(r application.Repositories) error {
				return r.Records.Put(ctx, owner)
			}); err != nil {
				t.Fatal(err)
			}
			request := semanticValidationRun("ER-FOREIGN-EVIDENCE-NEW")
			request.EvidenceArtifactID = "CAP-1"
			request.EvidenceRevisionID = requestedRevisionID
			assertStoredIntegrityWithoutWrites(t, f, func() (application.RecordValidationRunResult, error) {
				return request.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
			})
		})
	}
}

func TestC10MayCreateEvidencePreviouslyCitedByDecision(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedSemanticValidationFoundation(t, f)
	decision := application.RecordArchitectureDecisionCommand{
		DecisionID: "DEC-CITE-BEFORE-EVIDENCE", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		Question: "May evidence be cited before it is recorded?", OutcomeStatement: "Yes, citations do not occupy evidence.",
		Alternatives: []string{"Require evidence first"}, EvidenceArtifactID: "EV-CITED-LATER",
		EvidenceRevisionID: "EV-CITED-LATER-REV-1", Rationale: "C8 citations are non-owning references.",
	}
	if _, err := decision.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("seed unresolved decision citation: %v", err)
	}
	request := semanticValidationRun("ER-CITED-LATER")
	request.EvidenceArtifactID = decision.EvidenceArtifactID
	request.EvidenceRevisionID = decision.EvidenceRevisionID
	if _, err := request.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("create cited evidence: %v", err)
	}
}

func TestC10ReplayRejectsFeatureCardLinkToEvidenceArtifact(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedSemanticValidationFoundation(t, f)
	request := semanticValidationRun("ER-EVIDENCE-CAPABILITY-LINK")
	if _, err := request.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	if _, err := (application.CreateFeatureCommand{
		FeatureCardID: "FC-EVIDENCE-CAPABILITY-LINK", ProjectID: "PRJ-1", Title: "Invalid evidence link",
	}).Execute(ctx, f.uow, f.clock); err != nil {
		t.Fatal(err)
	}
	cardID, err := domain.NewFeatureCardID("FC-EVIDENCE-CAPABILITY-LINK")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.uow.Do(ctx, func(r application.Repositories) error {
		return r.FeatureCards.LinkCapability(ctx, cardID, request.EvidenceArtifactID)
	}); err != nil {
		t.Fatalf("seed invalid evidence capability link: %v", err)
	}
	assertStoredIntegrityWithoutWrites(t, f, func() (application.RecordValidationRunResult, error) {
		return request.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
	})
}
