package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering"
	peos "github.com/aleka7sk/featureforge/internal/engineering/peos"
	"github.com/aleka7sk/featureforge/internal/infrastructure/postgres"
)

// TestConcurrentRevisionsProduceDistinctSequences is the milestone's
// concurrency exit criterion: two revisions of one capability, created
// concurrently through the real application command, must BOTH succeed and
// receive sequences n and n+1 -- not one succeed and one fail.
//
// This is what SERIALIZABLE-plus-retry buys. The command reads existing order
// metadata, computes max+1 in Go, and writes it, all inside one Do. Under
// concurrency PostgreSQL detects the read-write conflict and aborts one
// transaction; UnitOfWork.Do retries the whole callback, which recomputes the
// sequence against the now-committed state.
func TestConcurrentRevisionsProduceDistinctSequences(t *testing.T) {
	dsn := requireDSN(t)
	pool := newIsolatedPool(t, dsn)
	uow := postgres.NewUnitOfWork(pool)
	recorder := peos.NewRecorder()
	clock := application.NewFixedClock(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	ctx := context.Background()

	seedCapability(t, uow, recorder, recorder, clock)

	const writers = 4
	var wg sync.WaitGroup
	errs := make([]error, writers)
	sequences := make([]int, writers)
	for i := range writers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cmd := application.ReviseCapabilitySpecificationCommand{
				ArtifactID: "CAP-1",
				RevisionID: "CAP-1-REV-CONC-" + string(rune('A'+i)),
				Content:    mustContent(t, "Concurrent revision "+string(rune('A'+i))),
			}
			result, err := cmd.Execute(ctx, uow, recorder, recorder, clock)
			errs[i] = err
			sequences[i] = result.Sequence
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("writer %d failed; every concurrent writer must succeed: %v", i, err)
		}
	}
	if t.Failed() {
		return
	}

	// Sequences must be distinct and contiguous from 2 (the founding revision
	// holds 1), which is what "n and n+1" means for more than two writers.
	seen := map[int]bool{}
	for i, seq := range sequences {
		if seen[seq] {
			t.Errorf("writer %d reused sequence %d", i, seq)
		}
		seen[seq] = true
	}
	for want := 2; want <= writers+1; want++ {
		if !seen[want] {
			t.Errorf("sequence %d was never assigned; got %v", want, sequences)
		}
	}
}

// TestNestedDoIsRejected asserts a Do inside a Do is refused rather than
// silently opening a second transaction on a second pooled connection.
func TestNestedDoIsRejected(t *testing.T) {
	dsn := requireDSN(t)
	uow := postgres.NewUnitOfWork(newIsolatedPool(t, dsn))

	err := uow.Do(context.Background(), func(_ application.Repositories) error {
		return uow.Do(context.Background(), func(_ application.Repositories) error {
			t.Error("the nested callback must never run")
			return nil
		})
	})
	if !errors.Is(err, application.ErrNestedTransaction) {
		t.Errorf("err = %v, want ErrNestedTransaction", err)
	}
}

// seedCapability establishes PRJ-1, FC-1, and CAP-1 with its founding
// revision at sequence 1.
func seedCapability(t *testing.T, uow application.UnitOfWork, recorder application.EngineeringRecorder, inspector application.EngineeringReplayInspector, clock application.Clock) {
	t.Helper()
	ctx := context.Background()
	if _, err := (application.CreateProjectCommand{ProjectID: "PRJ-1", Name: "Pilot"}).Execute(ctx, uow, clock); err != nil {
		t.Fatal(err)
	}
	if _, err := (application.CreateFeatureCommand{FeatureCardID: "FC-1", ProjectID: "PRJ-1", Title: "Homework"}).Execute(ctx, uow, clock); err != nil {
		t.Fatal(err)
	}
	cmd := application.EstablishCapabilitySpecificationCommand{
		FeatureCardID: "FC-1", ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-1",
		Content: mustContent(t, "Homework after a lesson"),
	}
	if _, err := cmd.Execute(ctx, uow, recorder, inspector, clock); err != nil {
		t.Fatal(err)
	}
}

func mustContent(t *testing.T, title string) engineering.CapabilitySpecificationContent {
	t.Helper()
	c, err := engineering.NewCapabilitySpecificationContent(1, title, "problem statement")
	if err != nil {
		t.Fatal(err)
	}
	return c
}
