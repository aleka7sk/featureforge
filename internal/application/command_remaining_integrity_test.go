package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/domain"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

type zeroProjectGetRepository struct {
	application.ProjectRepository
	key domain.ProjectID
}

func (r zeroProjectGetRepository) Get(ctx context.Context, key domain.ProjectID) (domain.Project, bool, error) {
	if key == r.key {
		return domain.Project{}, true, nil
	}
	return r.ProjectRepository.Get(ctx, key)
}

type zeroFeatureCardGetRepository struct {
	application.FeatureCardRepository
	key domain.FeatureCardID
}

type corruptRevisionGetRepository struct {
	application.RevisionEnvelopeRepository
	key engineering.RevisionKey
}

func (r corruptRevisionGetRepository) Get(ctx context.Context, key engineering.RevisionKey) (engineering.RevisionEnvelope, bool, error) {
	stored, found, err := r.RevisionEnvelopeRepository.Get(ctx, key)
	if err == nil && found && key == r.key {
		stored.PayloadDigest = engineering.ComputeDigest([]byte("corrupt projected digest"))
	}
	return stored, found, err
}

type corruptRecordGetRepository struct {
	application.RecordEnvelopeRepository
	key engineering.RecordKey
}

func (r corruptRecordGetRepository) Get(ctx context.Context, key engineering.RecordKey) (engineering.RecordEnvelope, bool, error) {
	stored, found, err := r.RecordEnvelopeRepository.Get(ctx, key)
	if err == nil && found && key == r.key {
		stored.PayloadDigest = engineering.ComputeDigest([]byte("corrupt projected record digest"))
	}
	return stored, found, err
}

type zeroProjectListRepository struct{ application.ProjectRepository }

func (r zeroProjectListRepository) List(context.Context) ([]domain.Project, error) {
	return []domain.Project{{}}, nil
}

type zeroFeatureCardListRepository struct {
	application.FeatureCardRepository
}

func (r zeroFeatureCardListRepository) ListByProject(context.Context, domain.ProjectID) ([]domain.FeatureCard, error) {
	return []domain.FeatureCard{{}}, nil
}

func (r zeroFeatureCardGetRepository) Get(ctx context.Context, key domain.FeatureCardID) (domain.FeatureCard, bool, error) {
	if key == r.key {
		return domain.FeatureCard{}, true, nil
	}
	return r.FeatureCardRepository.Get(ctx, key)
}

func TestC1AndC2RejectZeroCreationTimes(t *testing.T) {
	ctx := context.Background()

	t.Run("zero candidate clock blocks only new acts", func(t *testing.T) {
		clock := application.NewFixedClock(time.Time{})
		projectFixture := newCommandFixture()
		release := forbidPersistenceWrites(projectFixture)
		if _, err := (application.CreateProjectCommand{ProjectID: "PRJ-ZERO-CLOCK", Name: "Zero clock"}).Execute(ctx, projectFixture.uow, clock); err == nil {
			t.Fatal("CreateProject accepted a zero clock")
		}
		release()

		featureFixture := newCommandFixture()
		project := application.CreateProjectCommand{ProjectID: "PRJ-ZERO-CLOCK", Name: "Owner"}
		if _, err := project.Execute(ctx, featureFixture.uow, featureFixture.clock); err != nil {
			t.Fatal(err)
		}
		release = forbidPersistenceWrites(featureFixture)
		if _, err := (application.CreateFeatureCommand{
			FeatureCardID: "FC-ZERO-CLOCK", ProjectID: "PRJ-ZERO-CLOCK", Title: "Zero clock",
		}).Execute(ctx, featureFixture.uow, clock); err == nil {
			t.Fatal("CreateFeature accepted a zero clock")
		}
		release()

		acceptFixture := newCommandFixture()
		setupProjectAndFeature(t, acceptFixture)
		if _, err := (application.EstablishCapabilitySpecificationCommand{
			FeatureCardID: "FC-1", ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-1",
			Content: mustContent(t, "Zero acceptance clock"),
		}).Execute(ctx, acceptFixture.uow, acceptFixture.rec, acceptFixture.rec, acceptFixture.clock); err != nil {
			t.Fatal(err)
		}
		release = forbidPersistenceWrites(acceptFixture)
		if _, err := (application.AcceptCapabilityRevisionCommand{
			RecordID: "ACC-ZERO-CLOCK", ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-1",
			State: engineering.AcceptanceStateAccepted,
		}).Execute(ctx, acceptFixture.uow, acceptFixture.rec, clock); err == nil {
			t.Fatal("AcceptCapabilityRevision accepted a zero clock")
		}
		release()
	})

	t.Run("zero candidate clock is ignored on exact replay", func(t *testing.T) {
		clock := application.NewFixedClock(time.Time{})

		projectFixture := newCommandFixture()
		project := application.CreateProjectCommand{ProjectID: "PRJ-ZERO-REPLAY", Name: "Replay"}
		if _, err := project.Execute(ctx, projectFixture.uow, projectFixture.clock); err != nil {
			t.Fatal(err)
		}
		release := forbidPersistenceWrites(projectFixture)
		if _, err := project.Execute(ctx, projectFixture.uow, clock); err != nil {
			t.Fatalf("project replay with zero clock: %v", err)
		}
		release()

		featureFixture := newCommandFixture()
		owner := application.CreateProjectCommand{ProjectID: "PRJ-ZERO-REPLAY", Name: "Replay"}
		feature := application.CreateFeatureCommand{FeatureCardID: "FC-ZERO-REPLAY", ProjectID: owner.ProjectID, Title: "Replay"}
		if _, err := owner.Execute(ctx, featureFixture.uow, featureFixture.clock); err != nil {
			t.Fatal(err)
		}
		if _, err := feature.Execute(ctx, featureFixture.uow, featureFixture.clock); err != nil {
			t.Fatal(err)
		}
		release = forbidPersistenceWrites(featureFixture)
		if _, err := feature.Execute(ctx, featureFixture.uow, clock); err != nil {
			t.Fatalf("FeatureCard replay with zero clock: %v", err)
		}
		release()

		acceptFixture := newCommandFixture()
		setupProjectAndFeature(t, acceptFixture)
		if _, err := (application.EstablishCapabilitySpecificationCommand{
			FeatureCardID: "FC-1", ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-1",
			Content: mustContent(t, "Zero acceptance replay clock"),
		}).Execute(ctx, acceptFixture.uow, acceptFixture.rec, acceptFixture.rec, acceptFixture.clock); err != nil {
			t.Fatal(err)
		}
		accept := application.AcceptCapabilityRevisionCommand{
			RecordID: "ACC-ZERO-REPLAY", ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-1", State: engineering.AcceptanceStateAccepted,
		}
		if _, err := accept.Execute(ctx, acceptFixture.uow, acceptFixture.rec, acceptFixture.clock); err != nil {
			t.Fatal(err)
		}
		release = forbidPersistenceWrites(acceptFixture)
		if _, err := accept.Execute(ctx, acceptFixture.uow, acceptFixture.rec, clock); err != nil {
			t.Fatalf("acceptance replay with zero clock: %v", err)
		}
		release()
	})

	t.Run("stored project", func(t *testing.T) {
		f := newCommandFixture()
		id, _ := domain.NewProjectID("PRJ-ZERO-STORED")
		uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
			r.Projects = zeroProjectGetRepository{ProjectRepository: r.Projects, key: id}
		}}
		assertStoredIntegrityWithoutWrites(t, f, func() (application.CreateProjectResult, error) {
			return (application.CreateProjectCommand{ProjectID: id.String(), Name: "Zero stored"}).Execute(ctx, uow, f.clock)
		})
	})

	t.Run("stored FeatureCard", func(t *testing.T) {
		f := newCommandFixture()
		id, _ := domain.NewFeatureCardID("FC-ZERO-STORED")
		uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
			r.FeatureCards = zeroFeatureCardGetRepository{FeatureCardRepository: r.FeatureCards, key: id}
		}}
		assertStoredIntegrityWithoutWrites(t, f, func() (application.CreateFeatureResult, error) {
			return (application.CreateFeatureCommand{
				FeatureCardID: id.String(), ProjectID: "PRJ-ZERO-STORED", Title: "Zero stored",
			}).Execute(ctx, uow, f.clock)
		})
	})

	t.Run("FeatureCard owning project", func(t *testing.T) {
		f := newCommandFixture()
		cmd := application.CreateProjectCommand{ProjectID: "PRJ-ZERO-OWNER", Name: "Owner"}
		if _, err := cmd.Execute(ctx, f.uow, f.clock); err != nil {
			t.Fatal(err)
		}
		feature := application.CreateFeatureCommand{FeatureCardID: "FC-ZERO-OWNER", ProjectID: cmd.ProjectID, Title: "Owned"}
		if _, err := feature.Execute(ctx, f.uow, f.clock); err != nil {
			t.Fatal(err)
		}
		pid, _ := domain.NewProjectID(cmd.ProjectID)
		uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
			r.Projects = zeroProjectGetRepository{ProjectRepository: r.Projects, key: pid}
		}}
		assertStoredIntegrityWithoutWrites(t, f, func() (application.CreateFeatureResult, error) {
			return feature.Execute(ctx, uow, f.clock)
		})
	})
}

func assertInternalClockFailureWithoutWrites(t *testing.T, f commandFixture, execute func() error) {
	t.Helper()
	release := forbidPersistenceWrites(f)
	defer release()
	err := execute()
	if err == nil || errors.Is(err, application.ErrInvalidCommand) || !strings.Contains(err.Error(), "clock returned a zero time") {
		t.Fatalf("err = %v, want internal zero-clock failure", err)
	}
}

func TestServerOwnedZeroClockIsInternalAcrossNewEngineeringActs(t *testing.T) {
	ctx := context.Background()
	zeroClock := application.NewFixedClock(time.Time{})

	t.Run("C3 capability establishment", func(t *testing.T) {
		f := newCommandFixture()
		setupProjectAndFeature(t, f)
		cmd := application.EstablishCapabilitySpecificationCommand{
			FeatureCardID: "FC-1", ArtifactID: "CAP-ZERO-C3", RevisionID: "CAP-ZERO-C3-REV-1", Content: mustContent(t, "Zero C3 clock"),
		}
		assertInternalClockFailureWithoutWrites(t, f, func() error {
			_, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, zeroClock)
			return err
		})
	})

	t.Run("C4 capability revision", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		cmd := application.ReviseCapabilitySpecificationCommand{
			ArtifactID: "CAP-1", RevisionID: "CAP-ZERO-C4-REV-2", Content: mustContent(t, "Zero C4 clock"),
		}
		assertInternalClockFailureWithoutWrites(t, f, func() error {
			_, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, zeroClock)
			return err
		})
	})

	t.Run("C6 lifecycle entry", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		cmd := application.AssignLifecycleStateCommand{
			AssignmentID: "SA-ZERO-C6", SubjectArtifactID: "CAP-1", State: "drafting", IsEntry: true,
			TransitionRecordArtifactID: "TR-ZERO-C6", TransitionRecordRevisionID: "TR-ZERO-C6-REV-0",
		}
		assertInternalClockFailureWithoutWrites(t, f, func() error {
			_, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, zeroClock)
			return err
		})
	})

	t.Run("C8 architecture decision", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		seedEvidenceRevision(t, f, "EV-ZERO-C8", "EV-ZERO-C8-REV-1")
		cmd := application.RecordArchitectureDecisionCommand{
			DecisionID: "DEC-ZERO-C8", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
			Question: "Does a zero server clock become client input?", OutcomeStatement: "No.",
			EvidenceArtifactID: "EV-ZERO-C8", EvidenceRevisionID: "EV-ZERO-C8-REV-1",
		}
		assertInternalClockFailureWithoutWrites(t, f, func() error {
			_, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, zeroClock)
			return err
		})
	})

	t.Run("C10 validation run", func(t *testing.T) {
		f := newCommandFixture()
		seedValidationFoundationForReplay(t, f)
		cmd := validationRunForReplay("ER-ZERO-C10")
		assertInternalClockFailureWithoutWrites(t, f, func() error {
			_, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, zeroClock)
			return err
		})
	})

	t.Run("C11 validation claim", func(t *testing.T) {
		f := newCommandFixture()
		seedValidationFoundationForReplay(t, f)
		run := validationRunForReplay("ER-ZERO-C11")
		if _, err := run.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		cmd := validationClaimForReplay("CLM-ZERO-C11", run.ExecutionID)
		assertInternalClockFailureWithoutWrites(t, f, func() error {
			_, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, zeroClock)
			return err
		})
	})

	t.Run("C12 validation correction", func(t *testing.T) {
		f := newCommandFixture()
		seedValidationFoundationForReplay(t, f)
		run := validationRunForReplay("ER-ZERO-C12")
		if _, err := run.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		original := validationClaimForReplay("CLM-ZERO-C12-ORIGINAL", run.ExecutionID)
		if _, err := original.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		cmd := application.CorrectValidationClaimCommand{
			ClaimID: "CLM-ZERO-C12", CorrectionTarget: original.ClaimID, CorrectionKind: "correct",
			ScopeArtifactID: "CAP-1", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
			RequirementArtifactID: "REQ-REPLAY", RequirementRevisionID: "REQ-REPLAY-REV-1",
			Outcome: "not-satisfied", Method: "manual-review",
			EvidenceArtifactID: run.EvidenceArtifactID, EvidenceRevisionID: run.EvidenceRevisionID,
			ExecutionID: run.ExecutionID, Reasoning: "Zero server time must remain an internal failure.",
		}
		assertInternalClockFailureWithoutWrites(t, f, func() error {
			_, err := cmd.Execute(ctx, f.uow, f.rec, f.rec, zeroClock)
			return err
		})
	})
}

func TestC3RejectsZeroTimeFeatureCardOwnership(t *testing.T) {
	ctx := context.Background()

	t.Run("new act requested FeatureCard", func(t *testing.T) {
		f := newCommandFixture()
		setupProjectAndFeature(t, f)
		fid, _ := domain.NewFeatureCardID("FC-1")
		uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
			r.FeatureCards = zeroFeatureCardGetRepository{FeatureCardRepository: r.FeatureCards, key: fid}
		}}
		request := application.EstablishCapabilitySpecificationCommand{
			FeatureCardID: "FC-1", ArtifactID: "CAP-ZERO-OWNER-NEW", RevisionID: "CAP-ZERO-OWNER-NEW-REV-1",
			Content: mustContent(t, "Zero-time requested FeatureCard"),
		}
		assertStoredIntegrityWithoutWrites(t, f, func() (application.EstablishCapabilitySpecificationResult, error) {
			return request.Execute(ctx, uow, f.rec, f.rec, f.clock)
		})
	})

	t.Run("new act owning Project", func(t *testing.T) {
		f := newCommandFixture()
		setupProjectAndFeature(t, f)
		pid, _ := domain.NewProjectID("PRJ-1")
		uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
			r.Projects = zeroProjectGetRepository{ProjectRepository: r.Projects, key: pid}
		}}
		request := application.EstablishCapabilitySpecificationCommand{
			FeatureCardID: "FC-1", ArtifactID: "CAP-ZERO-PROJECT-NEW", RevisionID: "CAP-ZERO-PROJECT-NEW-REV-1",
			Content: mustContent(t, "Zero-time owning Project"),
		}
		assertStoredIntegrityWithoutWrites(t, f, func() (application.EstablishCapabilitySpecificationResult, error) {
			return request.Execute(ctx, uow, f.rec, f.rec, f.clock)
		})
	})

	for _, tc := range []struct {
		name     string
		override func(*application.Repositories)
	}{
		{name: "replay Project listing", override: func(r *application.Repositories) {
			r.Projects = zeroProjectListRepository{ProjectRepository: r.Projects}
		}},
		{name: "replay FeatureCard listing", override: func(r *application.Repositories) {
			r.FeatureCards = zeroFeatureCardListRepository{FeatureCardRepository: r.FeatureCards}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCommandFixture()
			setupProjectAndFeature(t, f)
			request := application.EstablishCapabilitySpecificationCommand{
				FeatureCardID: "FC-1", ArtifactID: "CAP-ZERO-OWNER-REPLAY", RevisionID: "CAP-ZERO-OWNER-REPLAY-REV-1",
				Content: mustContent(t, "Zero-time replay owner"),
			}
			if _, err := request.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
				t.Fatal(err)
			}
			uow := repositoryOverrideUOW{base: f.uow, override: tc.override}
			assertStoredIntegrityWithoutWrites(t, f, func() (application.EstablishCapabilitySpecificationResult, error) {
				return request.Execute(ctx, uow, f.rec, f.rec, f.clock)
			})
		})
	}
}

func TestC9AndC10PersistedCorruptionPrecedesMissingDependency(t *testing.T) {
	ctx := context.Background()

	t.Run("C9 corrupt requirement and missing activity subject", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		seedRequirementForC7C9(t, f, "REQ-MIXED-C9", "REQ-MIXED-C9-REV-1", "MEM-REQ-MIXED-C9")
		corruptKey := mustRevKey(t, "REQ-MIXED-C9", "REQ-MIXED-C9-REV-1")
		uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
			r.Revisions = corruptRevisionGetRepository{RevisionEnvelopeRepository: r.Revisions, key: corruptKey}
		}}
		cmd := planForIntegrity("VP-MIXED-C9", "VP-MIXED-C9-REV-1", "ACT-MIXED-C9", "REQ-MIXED-C9", "REQ-MIXED-C9-REV-1", "MEM-VP-MIXED-C9")
		cmd.Activities[0].SubjectArtifactID = "CAP-MISSING-C9"
		cmd.Activities[0].SubjectRevisionID = "CAP-MISSING-C9-REV-1"
		assertStoredIntegrityWithoutWrites(t, f, func() (application.EstablishValidationPlanResult, error) {
			return cmd.Execute(ctx, uow, f.rec, f.rec, f.clock)
		})
	})

	t.Run("C10 corrupt run subject and missing validation plan", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		corruptKey := mustRevKey(t, "CAP-1", "CAP-1-REV-1")
		uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
			r.Revisions = corruptRevisionGetRepository{RevisionEnvelopeRepository: r.Revisions, key: corruptKey}
		}}
		cmd := validationRunForReplay("ER-MIXED-C10")
		cmd.PlanArtifactID = "VP-MISSING-C10"
		cmd.PlanRevisionID = "VP-MISSING-C10-REV-1"
		assertStoredIntegrityWithoutWrites(t, f, func() (application.RecordValidationRunResult, error) {
			return cmd.Execute(ctx, uow, f.rec, f.rec, f.clock)
		})
	})
}

func TestC5RejectsCandidateThatInvalidatesCanonicalJournalOrder(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name        string
		acceptedID  string
		acceptedAt  time.Time
		withdrawnID string
		withdrawnAt time.Time
	}{
		{
			name: "backdated", acceptedID: "ACC-LATE", acceptedAt: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
			withdrawnID: "WD-EARLY", withdrawnAt: time.Date(2026, 8, 1, 11, 0, 0, 0, time.UTC),
		},
		{
			name: "same time but lexically earlier", acceptedID: "ZZ-ACCEPTED", acceptedAt: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
			withdrawnID: "AA-WITHDRAWN", withdrawnAt: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newCommandFixture()
			setupProjectAndFeature(t, f)
			if _, err := (application.EstablishCapabilitySpecificationCommand{
				FeatureCardID: "FC-1", ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-1",
				Content: mustContent(t, "Acceptance ordering"),
			}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
				t.Fatalf("seed capability revision: %v", err)
			}
			accepted := application.AcceptCapabilityRevisionCommand{
				RecordID: tc.acceptedID, ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-1",
				State: engineering.AcceptanceStateAccepted, EffectiveAt: tc.acceptedAt, HasEffectiveAt: true,
			}
			if _, err := accepted.Execute(ctx, f.uow, f.rec, f.clock); err != nil {
				t.Fatalf("seed accepted: %v", err)
			}
			withdrawn := application.AcceptCapabilityRevisionCommand{
				RecordID: tc.withdrawnID, ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-1",
				State: engineering.AcceptanceStateWithdrawn, EffectiveAt: tc.withdrawnAt, HasEffectiveAt: true,
			}
			release := forbidPersistenceWrites(f)
			defer release()
			if _, err := withdrawn.Execute(ctx, f.uow, f.rec, f.clock); !errors.Is(err, application.ErrAcceptanceTransitionInvalid) {
				t.Fatalf("err = %v, want ErrAcceptanceTransitionInvalid", err)
			}
		})
	}
}

func TestC5RejectsBackdatedCandidateAfterTerminalJournalHead(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	setupProjectAndFeature(t, f)
	if _, err := (application.EstablishCapabilitySpecificationCommand{
		FeatureCardID: "FC-1", ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-1",
		Content: mustContent(t, "Terminal acceptance ordering"),
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	terminalTime := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	if _, err := (application.AcceptCapabilityRevisionCommand{
		RecordID: "WD-TERMINAL", ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-1",
		State: engineering.AcceptanceStateWithdrawn, EffectiveAt: terminalTime, HasEffectiveAt: true,
	}).Execute(ctx, f.uow, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	candidate := application.AcceptCapabilityRevisionCommand{
		RecordID: "ACC-BACKDATED", ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-1",
		State: engineering.AcceptanceStateAccepted, EffectiveAt: terminalTime.Add(-time.Hour), HasEffectiveAt: true,
	}
	release := forbidPersistenceWrites(f)
	defer release()
	if _, err := candidate.Execute(ctx, f.uow, f.rec, f.clock); !errors.Is(err, application.ErrAcceptanceTransitionInvalid) {
		t.Fatalf("err = %v, want ErrAcceptanceTransitionInvalid", err)
	}
}

func putDanglingExecutionOwner(t *testing.T, f commandFixture, artifactID string) {
	t.Helper()
	ctx := context.Background()
	execution, err := f.rec.RecordExecution(engineering.ExecutionInput{
		ExecutionID: "ER-OWNER-" + artifactID, PlanArtifactID: "VP-OWNER", PlanRevisionID: "VP-OWNER-REV-1",
		ActivityKey: "A-OWNER", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		Method: "manual-review", Outcome: "completed", CompletedAt: f.clock.Now(),
		EvidenceArtifactID: artifactID, EvidenceRevisionID: artifactID + "-REV-EVIDENCE", RecordedAt: f.clock.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.uow.Do(ctx, func(r application.Repositories) error { return r.Records.Put(ctx, execution) }); err != nil {
		t.Fatalf("seed dangling execution owner: %v", err)
	}
}

func putDanglingStateAssignmentOwner(t *testing.T, f commandFixture, artifactID string) {
	t.Helper()
	ctx := context.Background()
	_, _, assignment, err := f.rec.RecordEntryAssignment(engineering.EntryAssignmentInput{
		AssignmentID: "SA-OWNER-" + artifactID, SubjectArtifactID: "CAP-1", State: "drafting",
		EffectiveAt: f.clock.Now(), TransitionRecordArtifactID: artifactID,
		TransitionRecordRevisionID: artifactID + "-REV-TRANSITION", RecordedAt: f.clock.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.uow.Do(ctx, func(r application.Repositories) error { return r.Records.Put(ctx, assignment) }); err != nil {
		t.Fatalf("seed dangling state-assignment owner: %v", err)
	}
}

func TestManagedAndAbsentRootsRejectForeignReverseOwners(t *testing.T) {
	ctx := context.Background()
	owners := []struct {
		name string
		put  func(*testing.T, commandFixture, string)
	}{
		{name: "execution evidence", put: putDanglingExecutionOwner},
		{name: "state assignment parent", put: putDanglingStateAssignmentOwner},
	}
	for _, owner := range owners {
		t.Run(owner.name+" absent root", func(t *testing.T) {
			f := newCommandFixture()
			seedCapability(t, f)
			owner.put(t, f, "REQ-FOREIGN-ABSENT")
			request := requirementForIntegrity(
				"REQ-FOREIGN-ABSENT", "REQ-FOREIGN-ABSENT-REV-1",
				"The system SHALL reject foreign owners of an absent root.", "MEM-FOREIGN-ABSENT",
			)
			assertStoredIntegrityWithoutWrites(t, f, func() (application.EstablishRequirementResult, error) {
				return request.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
			})
		})

		t.Run(owner.name+" managed root", func(t *testing.T) {
			f := newCommandFixture()
			seedCapability(t, f)
			request := seedRequirementForC7C9(t, f, "REQ-FOREIGN-MANAGED", "REQ-FOREIGN-MANAGED-REV-1", "MEM-FOREIGN-MANAGED")
			owner.put(t, f, request.ArtifactID)
			assertStoredIntegrityWithoutWrites(t, f, func() (application.EstablishRequirementResult, error) {
				return request.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
			})
		})
	}
}

func TestC4MissingArtifactRejectsCorruptRootBeforeNotFound(t *testing.T) {
	ctx := context.Background()
	owners := []struct {
		name string
		put  func(*testing.T, commandFixture, string)
	}{
		{name: "execution evidence", put: putDanglingExecutionOwner},
		{name: "state assignment parent", put: putDanglingStateAssignmentOwner},
	}
	for _, owner := range owners {
		t.Run(owner.name, func(t *testing.T) {
			f := newCommandFixture()
			seedCapability(t, f)
			owner.put(t, f, "CAP-C4-MISSING")
			request := application.ReviseCapabilitySpecificationCommand{
				ArtifactID: "CAP-C4-MISSING", RevisionID: "CAP-C4-MISSING-REV-1",
				Content: mustContent(t, "A corrupt missing root is not an ordinary 404"),
			}
			assertStoredIntegrityWithoutWrites(t, f, func() (application.ReviseCapabilitySpecificationResult, error) {
				return request.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
			})
		})
	}
}

func TestC6ExistingRootSemanticConflictPrecedesMissingRequestedSubject(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedCapability(t, f)
	entry := seedLifecycleEntryForIntegrity(t, f, "TR-C6-PRECEDENCE", "TR-C6-PRECEDENCE-REV-0", "SA-C6-PRECEDENCE-ENTRY", "CAP-1")
	request := application.AssignLifecycleStateCommand{
		AssignmentID: "SA-C6-PRECEDENCE-OTHER", SubjectArtifactID: "CAP-MISSING", State: "drafting", IsEntry: true,
		TransitionRecordArtifactID: entry.TransitionRecordArtifactID,
		TransitionRecordRevisionID: "TR-C6-PRECEDENCE-REV-OTHER",
	}
	assertImmutableConflictWithoutWrites(t, f, func() (application.AssignLifecycleStateResult, error) {
		return request.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
	})
}

func TestC6CorruptDependencyPrecedesAnotherMissingDependency(t *testing.T) {
	ctx := context.Background()

	t.Run("missing subject cannot hide corrupt predecessor", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		entry := seedLifecycleEntryForIntegrity(t, f, "TR-C6-MIXED-PREDECESSOR", "TR-C6-MIXED-PREDECESSOR-REV-0", "SA-C6-MIXED-PREDECESSOR", "CAP-1")
		predecessorKey, err := engineering.NewRecordKey(engineering.RecordKindStateAssignment, entry.AssignmentID)
		if err != nil {
			t.Fatal(err)
		}
		uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
			r.Records = corruptRecordGetRepository{RecordEnvelopeRepository: r.Records, key: predecessorKey}
		}}
		request := application.AssignLifecycleStateCommand{
			AssignmentID: "SA-C6-MIXED-NEW", SubjectArtifactID: "CAP-C6-MISSING", State: "specified",
			TransitionRecordArtifactID: "TR-C6-MIXED-NEW",
			TransitionRecordRevisionID: "TR-C6-MIXED-NEW-REV-1",
			TransitionKey:              "specify", FromAssignmentID: entry.AssignmentID,
		}
		assertStoredIntegrityWithoutWrites(t, f, func() (application.AssignLifecycleStateResult, error) {
			return request.Execute(ctx, uow, f.rec, f.rec, f.clock)
		})
	})

	t.Run("corrupt subject cannot be hidden by missing predecessor", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		artifactKey := engineering.ArtifactKey{ArtifactID: "CAP-1"}
		uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
			r.Artifacts = shiftedArtifactTimeRepository{
				ArtifactEnvelopeRepository: r.Artifacts, key: artifactKey, delta: time.Second,
			}
		}}
		request := application.AssignLifecycleStateCommand{
			AssignmentID: "SA-C6-MIXED-MISSING", SubjectArtifactID: artifactKey.ArtifactID, State: "specified",
			TransitionRecordArtifactID: "TR-C6-MIXED-MISSING",
			TransitionRecordRevisionID: "TR-C6-MIXED-MISSING-REV-1",
			TransitionKey:              "specify", FromAssignmentID: "SA-C6-MISSING-PREDECESSOR",
		}
		assertStoredIntegrityWithoutWrites(t, f, func() (application.AssignLifecycleStateResult, error) {
			return request.Execute(ctx, uow, f.rec, f.rec, f.clock)
		})
	})
}

func seedSecondSemanticRequirement(t *testing.T, f commandFixture) application.EstablishRequirementCommand {
	t.Helper()
	cmd := application.EstablishRequirementCommand{
		ArtifactID: "REQ-SEMANTIC-OTHER", RevisionID: "REQ-SEMANTIC-OTHER-REV-1",
		Statement: "The system SHALL keep another criterion distinct.", SubjectArtifactID: "CAP-1",
		SourceCapabilityRevisionID: "CAP-1-REV-1", SourceAcceptanceCriterionKey: "AC-1",
		AcceptanceRecordID: memberID("MEM-REQ-SEMANTIC-OTHER"),
	}
	if _, err := cmd.Execute(context.Background(), f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("seed second requirement: %v", err)
	}
	return cmd
}

func TestC11ClaimMustMatchExecutedPlanCriterionAndScope(t *testing.T) {
	ctx := context.Background()

	t.Run("new request", func(t *testing.T) {
		f := newCommandFixture()
		seedSemanticValidationFoundation(t, f)
		other := seedSecondSemanticRequirement(t, f)
		run := semanticValidationRun("ER-CLAIM-CRITERION")
		if _, err := run.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		claim := semanticValidationClaim("CLM-WRONG-CRITERION", run.ExecutionID)
		claim.RequirementArtifactID = other.ArtifactID
		claim.RequirementRevisionID = other.RevisionID
		release := forbidPersistenceWrites(f)
		defer release()
		if _, err := claim.Execute(ctx, f.uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrReferencedValueMissing) {
			t.Fatalf("err = %v, want ErrReferencedValueMissing", err)
		}
	})

	t.Run("stored claim", func(t *testing.T) {
		f := newCommandFixture()
		seedSemanticValidationFoundation(t, f)
		other := seedSecondSemanticRequirement(t, f)
		run := semanticValidationRun("ER-STORED-CLAIM-CRITERION")
		if _, err := run.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		claim := semanticValidationClaim("CLM-STORED-WRONG-CRITERION", run.ExecutionID)
		claim.RequirementArtifactID = other.ArtifactID
		claim.RequirementRevisionID = other.RevisionID
		stored, err := f.rec.RecordClaim(engineering.ClaimInput{
			ClaimID: claim.ClaimID, ScopeArtifactID: claim.ScopeArtifactID,
			SubjectArtifactID: claim.SubjectArtifactID, SubjectRevisionID: claim.SubjectRevisionID,
			RequirementArtifactID: claim.RequirementArtifactID, RequirementRevisionID: claim.RequirementRevisionID,
			Outcome: claim.Outcome, Method: claim.Method,
			EvidenceArtifactID: claim.EvidenceArtifactID, EvidenceRevisionID: claim.EvidenceRevisionID,
			ExecutionID: claim.ExecutionID, Reasoning: claim.Reasoning,
			Timestamp: f.clock.Now(), RecordedAt: f.clock.Now(),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := f.uow.Do(ctx, func(r application.Repositories) error { return r.Records.Put(ctx, stored) }); err != nil {
			t.Fatalf("seed stored wrong-criterion claim: %v", err)
		}
		assertStoredIntegrityWithoutWrites(t, f, func() (application.RecordValidationClaimResult, error) {
			return claim.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		})
	})
}

func TestC12ReportsNonClaimCorrectionTargetFamily(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedSemanticValidationFoundation(t, f)
	run := semanticValidationRun("ER-CORRECTION-FAMILY")
	if _, err := run.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	decision := application.RecordArchitectureDecisionCommand{
		DecisionID: "REC-CORRECTION-FAMILY", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		Question: "Can a decision be a correction target?", OutcomeStatement: "No.",
		EvidenceArtifactID: run.EvidenceArtifactID, EvidenceRevisionID: run.EvidenceRevisionID,
	}
	if _, err := decision.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	correction := semanticCorrection("CLM-CORRECTION-FAMILY", decision.DecisionID, run.ExecutionID)
	release := forbidPersistenceWrites(f)
	defer release()
	if _, err := correction.Execute(ctx, f.uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrCorrectionFamilyMismatch) {
		t.Fatalf("err = %v, want ErrCorrectionFamilyMismatch", err)
	}
}

func TestC11AndC12CorruptDependencyPrecedesAnotherMissingDependency(t *testing.T) {
	ctx := context.Background()

	t.Run("C11 missing scope cannot hide corrupt execution", func(t *testing.T) {
		f := newCommandFixture()
		seedValidationFoundationForReplay(t, f)
		run := validationRunForReplay("ER-MIXED-C11")
		if _, err := run.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		executionKey, err := engineering.NewRecordKey(engineering.RecordKindExecution, run.ExecutionID)
		if err != nil {
			t.Fatal(err)
		}
		uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
			r.Records = corruptRecordGetRepository{RecordEnvelopeRepository: r.Records, key: executionKey}
		}}
		claim := validationClaimForReplay("CLM-MIXED-C11", run.ExecutionID)
		claim.ScopeArtifactID = "CAP-MISSING-C11"
		assertStoredIntegrityWithoutWrites(t, f, func() (application.RecordValidationClaimResult, error) {
			return claim.Execute(ctx, uow, f.rec, f.rec, f.clock)
		})
	})

	t.Run("C12 missing target cannot hide corrupt execution", func(t *testing.T) {
		f := newCommandFixture()
		seedValidationFoundationForReplay(t, f)
		run := validationRunForReplay("ER-MIXED-C12")
		if _, err := run.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		executionKey, err := engineering.NewRecordKey(engineering.RecordKindExecution, run.ExecutionID)
		if err != nil {
			t.Fatal(err)
		}
		uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
			r.Records = corruptRecordGetRepository{RecordEnvelopeRepository: r.Records, key: executionKey}
		}}
		correction := semanticCorrection("CLM-MIXED-C12", "CLM-MISSING-C12-TARGET", run.ExecutionID)
		assertStoredIntegrityWithoutWrites(t, f, func() (application.CorrectValidationClaimResult, error) {
			return correction.Execute(ctx, uow, f.rec, f.rec, f.clock)
		})
	})
}

func TestC12CommandIntegrityDoesNotTrustClaimSubjectIndex(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	seedValidationFoundationForReplay(t, f)
	run := validationRunForReplay("ER-C12-SUBJECT-INDEX")
	if _, err := run.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	original := validationClaimForReplay("CLM-C12-SUBJECT-INDEX-ORIGINAL", run.ExecutionID)
	if _, err := original.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	if _, err := (application.ReviseCapabilitySpecificationCommand{
		ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-2", Content: mustContent(t, "A second valid capability revision"),
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	bad, err := f.rec.RecordClaim(engineering.ClaimInput{
		ClaimID: "CLM-C12-SUBJECT-INDEX-CORRUPT", ScopeArtifactID: "CAP-1",
		SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		RequirementArtifactID: "REQ-REPLAY", RequirementRevisionID: "REQ-REPLAY-REV-1",
		Outcome: "not-satisfied", Method: "manual-review",
		EvidenceArtifactID: run.EvidenceArtifactID, EvidenceRevisionID: run.EvidenceRevisionID,
		ExecutionID: run.ExecutionID, Reasoning: "Stored payload remains on the original chain.",
		Timestamp: f.clock.Now(), RecordedAt: f.clock.Now(), HasCorrection: true,
		CorrectionKind: "correct", CorrectionTarget: original.ClaimID,
	})
	if err != nil {
		t.Fatal(err)
	}
	bad.SubjectKey = engineering.ArtifactRevisionSubjectKey("CAP-1", "CAP-1-REV-2")
	if err := f.uow.Do(ctx, func(r application.Repositories) error { return r.Records.Put(ctx, bad) }); err != nil {
		t.Fatalf("seed corrupt shifted-subject correction: %v", err)
	}

	request := semanticCorrection("CLM-C12-SUBJECT-INDEX-NEW", original.ClaimID, run.ExecutionID)
	assertStoredIntegrityWithoutWrites(t, f, func() (application.CorrectValidationClaimResult, error) {
		return request.Execute(ctx, f.uow, f.rec, f.rec, f.clock)
	})
}
