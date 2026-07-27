package memory

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/domain"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

func newTestRevisionKey(artifactID, revisionID string) (engineering.RevisionKey, error) {
	return engineering.NewRevisionKey(artifactID, revisionID)
}

func newTestRevisionEnvelope(key engineering.RevisionKey) (engineering.RevisionEnvelope, error) {
	payload := []byte(`{"rev":"` + key.RevisionID + `"}`)
	return engineering.NewRevisionEnvelope(engineering.RevisionEnvelopeInput{
		Key: key, RevisionFamily: engineering.RevisionFamilyCapability,
		ArtifactType: "featureforge:product-capability", IntegrityValue: "sha256:abc",
		Payload: payload, PayloadDigest: engineering.ComputeDigest(payload), RecordedAt: fixedContractTime(),
	})
}

func newTestOrderMetadata(key engineering.RevisionKey, sequence int) (engineering.RevisionOrderMetadata, error) {
	return engineering.NewRevisionOrderMetadata(key, sequence, fixedContractTime())
}

func TestRepositoryContractSuite(t *testing.T) {
	RunRepositoryContractSuite(t, func() application.UnitOfWork {
		return NewUnitOfWork(NewStore())
	})
}

func TestRollbackOnInjectedFailure(t *testing.T) {
	store := NewStore()
	store.SetFailureHook(func(kind string, n int) error {
		if kind == "project" && n == 3 {
			return errors.New("injected failure on the third project write")
		}
		return nil
	})
	uow := NewUnitOfWork(store)

	err := uow.Do(context.Background(), func(r application.Repositories) error {
		for _, id := range []string{"PRJ-1", "PRJ-2", "PRJ-3"} {
			pid, perr := domain.NewProjectID(id)
			if perr != nil {
				return perr
			}
			p, perr := domain.NewProject(pid, "Name", fixedContractTime())
			if perr != nil {
				return perr
			}
			if perr := r.Projects.Put(context.Background(), p); perr != nil {
				return perr
			}
		}
		return nil
	})
	if err == nil {
		t.Fatal("expected the injected failure to propagate")
	}

	err = uow.Do(context.Background(), func(r application.Repositories) error {
		list, err := r.Projects.List(context.Background())
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Errorf("expected no projects to persist after an injected mid-act failure, got %d", len(list))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentPutsAreSafe(t *testing.T) {
	uow := NewUnitOfWork(NewStore())
	var wg sync.WaitGroup
	errs := make([]error, 64)
	for i := range 64 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			pid, err := domain.NewProjectID("PRJ-CONC-" + itoa(i))
			if err != nil {
				errs[i] = err
				return
			}
			p, err := domain.NewProject(pid, "Concurrent", fixedContractTime())
			if err != nil {
				errs[i] = err
				return
			}
			errs[i] = uow.Do(context.Background(), func(r application.Repositories) error {
				return r.Projects.Put(context.Background(), p)
			})
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		list, err := r.Projects.List(context.Background())
		if err != nil {
			return err
		}
		if len(list) != 64 {
			t.Errorf("expected 64 distinct projects, got %d", len(list))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentRevisionSequenceAssignment(t *testing.T) {
	store := NewStore()
	uow := NewUnitOfWork(store)
	if err := uow.Do(context.Background(), func(r application.Repositories) error {
		return r.Artifacts.Put(context.Background(), mustArtifactEnvelope(t, "CAP-CONC"))
	}); err != nil {
		t.Fatal(err)
	}

	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = reviseSequenceOnce(t, uow, "CAP-CONC", "REV-CONC-"+itoa(i))
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}

	err := uow.Do(context.Background(), func(r application.Repositories) error {
		list, err := r.RevisionOrder.ListByArtifact(context.Background(), "CAP-CONC")
		if err != nil {
			return err
		}
		if len(list) != n {
			t.Fatalf("expected %d order entries, got %d", n, len(list))
		}
		seen := map[int]bool{}
		for _, entry := range list {
			if seen[entry.Sequence] {
				t.Errorf("duplicate sequence %d", entry.Sequence)
			}
			seen[entry.Sequence] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// reviseSequenceOnce reads the current max sequence for artifactID and
// writes one new revision at max+1, inside a single transaction --
// mirroring how internal/application's ReviseCapabilitySpecification
// command will compute its sequence in Phase D/E.
func reviseSequenceOnce(t *testing.T, uow application.UnitOfWork, artifactID, revisionID string) error {
	t.Helper()
	return uow.Do(context.Background(), func(r application.Repositories) error {
		existing, err := r.RevisionOrder.ListByArtifact(context.Background(), artifactID)
		if err != nil {
			return err
		}
		next := 1
		for _, e := range existing {
			if e.Sequence >= next {
				next = e.Sequence + 1
			}
		}
		key, err := newTestRevisionKey(artifactID, revisionID)
		if err != nil {
			return err
		}
		env, err := newTestRevisionEnvelope(key)
		if err != nil {
			return err
		}
		if err := r.Revisions.Put(context.Background(), env); err != nil {
			return err
		}
		order, err := newTestOrderMetadata(key, next)
		if err != nil {
			return err
		}
		return r.RevisionOrder.Put(context.Background(), order)
	})
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// TestNoUpdateOrDeleteMethodExists (FF-012 §6): reflects over every
// repository interface's method set and asserts no Update*, Delete*,
// Remove*, or Set* method exists anywhere.
func TestNoUpdateOrDeleteMethodExists(t *testing.T) {
	repoTypes := []any{
		(*application.ProjectRepository)(nil),
		(*application.FeatureCardRepository)(nil),
		(*application.ArtifactEnvelopeRepository)(nil),
		(*application.RevisionEnvelopeRepository)(nil),
		(*application.StructuredContentRepository)(nil),
		(*application.RecordEnvelopeRepository)(nil),
		(*application.RevisionOrderRepository)(nil),
		(*application.RevisionAcceptanceRepository)(nil),
	}
	forbidden := []string{"Update", "Delete", "Remove", "Set"}
	for _, rt := range repoTypes {
		typ := reflect.TypeOf(rt).Elem()
		for i := 0; i < typ.NumMethod(); i++ {
			name := typ.Method(i).Name
			for _, prefix := range forbidden {
				if strings.HasPrefix(name, prefix) {
					t.Errorf("%s.%s looks like a mutation method; repositories are insert-only/append-only", typ, name)
				}
			}
		}
	}
}
