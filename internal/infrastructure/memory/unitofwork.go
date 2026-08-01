package memory

import (
	"context"
	"maps"

	"github.com/aleka7sk/featureforge/internal/application"
)

// UnitOfWork implements application.UnitOfWork over a Store.
type UnitOfWork struct {
	store *Store
}

// NewUnitOfWork returns a UnitOfWork backed by store.
func NewUnitOfWork(store *Store) *UnitOfWork {
	return &UnitOfWork{store: store}
}

// Do runs fn as a single transaction (FF-009 §6). The store's lock is held
// for the full duration, which trivially guarantees isolation: no reader
// ever observes another transaction's uncommitted writes, and concurrent Do
// calls serialize on commit by serializing entirely. Writes made during fn
// land in a per-transaction overlay; a nil return merges the overlay into
// committed state, a non-nil return propagates unwrapped, and a panic
// discards the overlay and re-propagates. A same-goroutine reentrant call
// (Do inside Do) is detected before locking and returns
// application.ErrNestedTransaction; a different goroutine's concurrent call
// simply blocks until this one completes, exactly as intended.
func (u *UnitOfWork) Do(ctx context.Context, fn func(application.Repositories) error) (err error) {
	gid := currentGoroutineID()

	u.store.holderMu.Lock()
	if u.store.holderGoroutine == gid {
		u.store.holderMu.Unlock()
		return application.ErrNestedTransaction
	}
	u.store.holderMu.Unlock()

	u.store.mu.Lock()
	u.store.holderMu.Lock()
	u.store.holderGoroutine = gid
	u.store.holderMu.Unlock()
	defer func() {
		u.store.holderMu.Lock()
		u.store.holderGoroutine = 0
		u.store.holderMu.Unlock()
		u.store.mu.Unlock()
	}()

	txn := &transaction{store: u.store, overlay: newState()}
	repos := reposFor(ctx, txn)

	committed := false
	defer func() {
		if committed {
			txn.merge()
		}
		// If !committed (error return or panic in flight), the overlay is
		// simply discarded here: nothing was ever written to u.store.committed.
	}()

	err = fn(repos)
	if err != nil {
		return err
	}
	committed = true
	return nil
}

// transaction is the per-Do overlay: uncommitted writes accumulate here and
// are merged into the store's committed state only on successful return.
type transaction struct {
	store   *Store
	overlay *state
}

// merge copies every overlay entry into the store's committed state. Called
// only while the store's lock is held (Do never releases it before this
// runs), so the copy is atomic from every other transaction's perspective.
func (t *transaction) merge() {
	c := t.store.committed
	maps.Copy(c.projects, t.overlay.projects)
	maps.Copy(c.featureCards, t.overlay.featureCards)
	maps.Copy(c.capabilityLinks, t.overlay.capabilityLinks)
	maps.Copy(c.artifacts, t.overlay.artifacts)
	maps.Copy(c.revisions, t.overlay.revisions)
	maps.Copy(c.content, t.overlay.content)
	maps.Copy(c.records, t.overlay.records)
	maps.Copy(c.order, t.overlay.order)
	maps.Copy(c.traces, t.overlay.traces)
	for key, definition := range t.overlay.lifecycleDefinitions {
		c.lifecycleDefinitions[key] = cloneLifecycleDefinition(definition)
	}
	for key, version := range t.overlay.lifecycleVersions {
		c.lifecycleVersions[key] = cloneLifecycleVersion(version)
	}
	for k, entries := range t.overlay.acceptance {
		c.acceptance[k] = append(c.acceptance[k], entries...)
	}
}
