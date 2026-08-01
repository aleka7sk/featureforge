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

type repositoryOverrideUOW struct {
	base     application.UnitOfWork
	override func(*application.Repositories)
}

func (u repositoryOverrideUOW) Do(ctx context.Context, fn func(application.Repositories) error) error {
	return u.base.Do(ctx, func(r application.Repositories) error {
		u.override(&r)
		return fn(r)
	})
}

type missingProjectRepository struct {
	application.ProjectRepository
	missing domain.ProjectID
}

func (r missingProjectRepository) Get(ctx context.Context, id domain.ProjectID) (domain.Project, bool, error) {
	if id == r.missing {
		return domain.Project{}, false, nil
	}
	return r.ProjectRepository.Get(ctx, id)
}

type substitutedContentRepository struct {
	application.StructuredContentRepository
	key     engineering.RevisionKey
	content engineering.CapabilitySpecificationContent
}

func (r substitutedContentRepository) Get(ctx context.Context, key engineering.RevisionKey) (engineering.CapabilitySpecificationContent, bool, error) {
	if key == r.key {
		return r.content, true, nil
	}
	return r.StructuredContentRepository.Get(ctx, key)
}

type missingArtifactRepository struct {
	application.ArtifactEnvelopeRepository
	missing engineering.ArtifactKey
}

type shiftedArtifactTimeRepository struct {
	application.ArtifactEnvelopeRepository
	key   engineering.ArtifactKey
	delta time.Duration
}

func (r shiftedArtifactTimeRepository) Get(ctx context.Context, key engineering.ArtifactKey) (engineering.ArtifactEnvelope, bool, error) {
	artifact, found, err := r.ArtifactEnvelopeRepository.Get(ctx, key)
	if err == nil && found && key == r.key {
		artifact.RecordedAt = artifact.RecordedAt.Add(r.delta)
	}
	return artifact, found, err
}

func (r missingArtifactRepository) Get(ctx context.Context, key engineering.ArtifactKey) (engineering.ArtifactEnvelope, bool, error) {
	if key == r.missing {
		return engineering.ArtifactEnvelope{}, false, nil
	}
	return r.ArtifactEnvelopeRepository.Get(ctx, key)
}

func seedCapabilityPairWithoutOwner(t *testing.T, f commandFixture, artifactID, revisionID string, includeArtifact bool) {
	t.Helper()
	ctx := context.Background()
	content := mustContent(t, "Persisted capability")
	digest, err := content.Digest()
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := f.rec.RecordCapabilityArtifact(artifactID, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	revision, err := f.rec.RecordCapabilityRevision(engineering.CapabilityRevisionInput{
		ArtifactID: artifactID, RevisionID: revisionID, ContentDigest: digest, RecordedAt: f.clock.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	order, err := engineering.NewRevisionOrderMetadata(revision.Key, 1, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := f.uow.Do(ctx, func(r application.Repositories) error {
		if includeArtifact {
			if err := r.Artifacts.Put(ctx, artifact); err != nil {
				return err
			}
		}
		if err := r.Revisions.Put(ctx, revision); err != nil {
			return err
		}
		if err := r.StructuredContent.Put(ctx, revision.Key, content); err != nil {
			return err
		}
		return r.RevisionOrder.Put(ctx, order)
	}); err != nil {
		t.Fatalf("seed capability pair: %v", err)
	}
}

func seedForeignRequirementOccupancy(t *testing.T, f commandFixture, artifactID, revisionID string) {
	t.Helper()
	setupProjectAndFeature(t, f)
	establishCapability(t, f)
	acceptCapability(t, f, "ACC-FOREIGN-REQ-SOURCE", "CAP-1", "CAP-1-REV-1")
	cmd := application.EstablishRequirementCommand{
		ArtifactID: artifactID, RevisionID: revisionID,
		Statement: "The system SHALL retain coherent foreign occupancy.", SubjectArtifactID: "CAP-1",
		SourceCapabilityRevisionID: "CAP-1-REV-1", SourceAcceptanceCriterionKey: "AC-1",
		AcceptanceRecordID: memberID("MEM-" + artifactID),
	}
	if _, err := cmd.Execute(context.Background(), f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("seed foreign requirement: %v", err)
	}
}

func TestC2ReplayRequiresItsOwningProject(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	cmd := application.CreateFeatureCommand{
		FeatureCardID: "FC-C2", ProjectID: "PRJ-C2", Title: "C2 owner", Description: "complete act",
	}
	if _, err := (application.CreateProjectCommand{ProjectID: "PRJ-C2", Name: "C2"}).Execute(ctx, f.uow, f.clock); err != nil {
		t.Fatal(err)
	}
	if _, err := cmd.Execute(ctx, f.uow, f.clock); err != nil {
		t.Fatal(err)
	}
	pid, err := domain.NewProjectID("PRJ-C2")
	if err != nil {
		t.Fatal(err)
	}
	uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
		r.Projects = missingProjectRepository{ProjectRepository: r.Projects, missing: pid}
	}}
	release := forbidPersistenceWrites(f)
	defer release()
	if _, err := cmd.Execute(ctx, uow, f.clock); !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("replay with dangling Project err = %v, want ErrStoredStateIntegrity", err)
	}
}

func TestC3ClassifiesPersistedActBeforeRequestedFeatureCard(t *testing.T) {
	ctx := context.Background()

	t.Run("complete act with another requested FeatureCard conflicts", func(t *testing.T) {
		f := newCommandFixture()
		setupProjectAndFeature(t, f)
		establishCapability(t, f)
		release := forbidPersistenceWrites(f)
		defer release()
		_, err := (application.EstablishCapabilitySpecificationCommand{
			FeatureCardID: "FC-MISSING", ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-1",
			Content: mustContent(t, "Homework after a lesson"),
		}).Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		if !errors.Is(err, application.ErrImmutableValueConflict) {
			t.Fatalf("err = %v, want ErrImmutableValueConflict", err)
		}
	})

	t.Run("different founding Revision conflicts before FeatureCard lookup", func(t *testing.T) {
		f := newCommandFixture()
		setupProjectAndFeature(t, f)
		establishCapability(t, f)
		release := forbidPersistenceWrites(f)
		defer release()
		_, err := (application.EstablishCapabilitySpecificationCommand{
			FeatureCardID: "FC-MISSING", ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-2",
			Content: mustContent(t, "Homework after a lesson"),
		}).Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		if !errors.Is(err, application.ErrImmutableValueConflict) {
			t.Fatalf("err = %v, want ErrImmutableValueConflict", err)
		}
	})

	t.Run("existing later capability Revision is a coherent conflict", func(t *testing.T) {
		f := newCommandFixture()
		setupProjectAndFeature(t, f)
		establishCapability(t, f)
		laterContent := mustContent(t, "Later capability")
		if _, err := (application.ReviseCapabilitySpecificationCommand{
			ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-2", Content: laterContent,
		}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
		release := forbidPersistenceWrites(f)
		defer release()
		_, err := (application.EstablishCapabilitySpecificationCommand{
			FeatureCardID: "FC-1", ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-2", Content: laterContent,
		}).Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		if !errors.Is(err, application.ErrImmutableValueConflict) {
			t.Fatalf("err = %v, want ErrImmutableValueConflict", err)
		}
	})

	t.Run("local pair without Artifact is integrity failure", func(t *testing.T) {
		f := newCommandFixture()
		seedCapabilityPairWithoutOwner(t, f, "CAP-DANGLING", "CAP-DANGLING-REV-1", true)
		uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
			r.Artifacts = missingArtifactRepository{
				ArtifactEnvelopeRepository: r.Artifacts,
				missing:                    engineering.ArtifactKey{ArtifactID: "CAP-DANGLING"},
			}
		}}
		release := forbidPersistenceWrites(f)
		defer release()
		_, err := (application.EstablishCapabilitySpecificationCommand{
			FeatureCardID: "FC-MISSING", ArtifactID: "CAP-DANGLING", RevisionID: "CAP-DANGLING-REV-1",
			Content: mustContent(t, "Persisted capability"),
		}).Execute(ctx, uow, f.rec, f.rec, f.clock)
		if !errors.Is(err, application.ErrStoredStateIntegrity) {
			t.Fatalf("err = %v, want ErrStoredStateIntegrity", err)
		}
	})

	t.Run("complete pair without FeatureCard owner is integrity failure", func(t *testing.T) {
		f := newCommandFixture()
		seedCapabilityPairWithoutOwner(t, f, "CAP-OWNERLESS", "CAP-OWNERLESS-REV-1", true)
		release := forbidPersistenceWrites(f)
		defer release()
		_, err := (application.EstablishCapabilitySpecificationCommand{
			FeatureCardID: "FC-MISSING", ArtifactID: "CAP-OWNERLESS", RevisionID: "CAP-OWNERLESS-REV-1",
			Content: mustContent(t, "Persisted capability"),
		}).Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		if !errors.Is(err, application.ErrStoredStateIntegrity) {
			t.Fatalf("err = %v, want ErrStoredStateIntegrity", err)
		}
	})

	t.Run("coherent foreign pair conflicts", func(t *testing.T) {
		f := newCommandFixture()
		seedForeignRequirementOccupancy(t, f, "REQ-C3-FOREIGN", "REQ-C3-FOREIGN-REV-1")
		release := forbidPersistenceWrites(f)
		defer release()
		_, err := (application.EstablishCapabilitySpecificationCommand{
			FeatureCardID: "FC-MISSING", ArtifactID: "REQ-C3-FOREIGN", RevisionID: "REQ-C3-FOREIGN-REV-1",
			Content: mustContent(t, "Capability request"),
		}).Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		if !errors.Is(err, application.ErrImmutableValueConflict) {
			t.Fatalf("err = %v, want ErrImmutableValueConflict", err)
		}
	})

	t.Run("new act still reports a missing FeatureCard", func(t *testing.T) {
		f := newCommandFixture()
		release := forbidPersistenceWrites(f)
		defer release()
		_, err := (application.EstablishCapabilitySpecificationCommand{
			FeatureCardID: "FC-MISSING", ArtifactID: "CAP-NEW", RevisionID: "CAP-NEW-REV-1",
			Content: mustContent(t, "New capability"),
		}).Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		if !errors.Is(err, application.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})
}

func TestC3ValidatesExistingFeatureCardLinkBeforeConflict(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	setupProjectAndFeature(t, f)
	establishCapability(t, f)
	uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
		r.Artifacts = missingArtifactRepository{
			ArtifactEnvelopeRepository: r.Artifacts,
			missing:                    engineering.ArtifactKey{ArtifactID: "CAP-1"},
		}
	}}
	release := forbidPersistenceWrites(f)
	defer release()
	_, err := (application.EstablishCapabilitySpecificationCommand{
		FeatureCardID: "FC-1", ArtifactID: "CAP-NEW", RevisionID: "CAP-NEW-REV-1",
		Content: mustContent(t, "New capability"),
	}).Execute(ctx, uow, f.rec, f.rec, f.clock)
	if !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("err = %v, want corrupt linked capability ErrStoredStateIntegrity", err)
	}
}

func TestManagedHistoryRejectsArtifactFoundingTimeMismatch(t *testing.T) {
	f := newCommandFixture()
	ctx := context.Background()
	setupProjectAndFeature(t, f)
	establishCapability(t, f)
	uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
		r.Artifacts = shiftedArtifactTimeRepository{
			ArtifactEnvelopeRepository: r.Artifacts,
			key:                        engineering.ArtifactKey{ArtifactID: "CAP-1"},
			delta:                      time.Second,
		}
	}}
	release := forbidPersistenceWrites(f)
	defer release()
	cmd := application.EstablishCapabilitySpecificationCommand{
		FeatureCardID: "FC-1", ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-1",
		Content: mustContent(t, "Homework after a lesson"),
	}
	if _, err := cmd.Execute(ctx, uow, f.rec, f.rec, f.clock); !errors.Is(err, application.ErrStoredStateIntegrity) {
		t.Fatalf("err = %v, want founding-time ErrStoredStateIntegrity", err)
	}
}

func TestC4ClassifiesLocalPairBeforeArtifact(t *testing.T) {
	ctx := context.Background()

	t.Run("occupied pair without Artifact is integrity failure", func(t *testing.T) {
		f := newCommandFixture()
		seedCapabilityPairWithoutOwner(t, f, "CAP-C4-DANGLING", "CAP-C4-DANGLING-REV-1", true)
		uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
			r.Artifacts = missingArtifactRepository{
				ArtifactEnvelopeRepository: r.Artifacts,
				missing:                    engineering.ArtifactKey{ArtifactID: "CAP-C4-DANGLING"},
			}
		}}
		release := forbidPersistenceWrites(f)
		defer release()
		_, err := (application.ReviseCapabilitySpecificationCommand{
			ArtifactID: "CAP-C4-DANGLING", RevisionID: "CAP-C4-DANGLING-REV-1",
			Content: mustContent(t, "Persisted capability"),
		}).Execute(ctx, uow, f.rec, f.rec, f.clock)
		if !errors.Is(err, application.ErrStoredStateIntegrity) {
			t.Fatalf("err = %v, want ErrStoredStateIntegrity", err)
		}
	})

	t.Run("absent pair and absent Artifact preserves not found", func(t *testing.T) {
		f := newCommandFixture()
		release := forbidPersistenceWrites(f)
		defer release()
		_, err := (application.ReviseCapabilitySpecificationCommand{
			ArtifactID: "CAP-C4-MISSING", RevisionID: "CAP-C4-MISSING-REV-1",
			Content: mustContent(t, "Missing capability"),
		}).Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		if !errors.Is(err, application.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("coherent foreign pair conflicts", func(t *testing.T) {
		f := newCommandFixture()
		seedForeignRequirementOccupancy(t, f, "REQ-C4-FOREIGN", "REQ-C4-FOREIGN-REV-1")
		release := forbidPersistenceWrites(f)
		defer release()
		_, err := (application.ReviseCapabilitySpecificationCommand{
			ArtifactID: "REQ-C4-FOREIGN", RevisionID: "REQ-C4-FOREIGN-REV-1",
			Content: mustContent(t, "Capability request"),
		}).Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		if !errors.Is(err, application.ErrImmutableValueConflict) {
			t.Fatalf("err = %v, want ErrImmutableValueConflict", err)
		}
	})

	t.Run("coherent foreign Artifact conflicts for an absent pair", func(t *testing.T) {
		f := newCommandFixture()
		seedForeignRequirementOccupancy(t, f, "REQ-C4-ARTIFACT", "REQ-C4-ARTIFACT-REV-1")
		release := forbidPersistenceWrites(f)
		defer release()
		_, err := (application.ReviseCapabilitySpecificationCommand{
			ArtifactID: "REQ-C4-ARTIFACT", RevisionID: "REQ-C4-ARTIFACT-REV-2",
			Content: mustContent(t, "Capability request"),
		}).Execute(ctx, f.uow, f.rec, f.rec, f.clock)
		if !errors.Is(err, application.ErrImmutableValueConflict) {
			t.Fatalf("err = %v, want ErrImmutableValueConflict", err)
		}
	})

	t.Run("capability content contradiction is integrity failure", func(t *testing.T) {
		f := newCommandFixture()
		setupProjectAndFeature(t, f)
		establishCapability(t, f)
		key := mustRevKeyOf(t, "CAP-1", "CAP-1-REV-1")
		uow := repositoryOverrideUOW{base: f.uow, override: func(r *application.Repositories) {
			r.StructuredContent = substitutedContentRepository{
				StructuredContentRepository: r.StructuredContent,
				key:                         key, content: mustContent(t, "Contradictory content"),
			}
		}}
		release := forbidPersistenceWrites(f)
		defer release()
		_, err := (application.ReviseCapabilitySpecificationCommand{
			ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-1", Content: mustContent(t, "Homework after a lesson"),
		}).Execute(ctx, uow, f.rec, f.rec, f.clock)
		if !errors.Is(err, application.ErrStoredStateIntegrity) {
			t.Fatalf("err = %v, want ErrStoredStateIntegrity", err)
		}
	})
}
