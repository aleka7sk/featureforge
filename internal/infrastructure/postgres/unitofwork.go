package postgres

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// maxAttempts bounds how many times Do re-runs a callback that PostgreSQL
// aborted with a serialization failure. Eight is generous for the contention
// this application generates (a handful of concurrent writers at most) and
// small enough that a genuine livelock surfaces as ErrTransactionAborted
// rather than hanging.
const maxAttempts = 8

// UnitOfWork implements application.UnitOfWork over a PostgreSQL pool.
type UnitOfWork struct {
	pool *pgxpool.Pool

	// holders records which goroutines are currently inside Do, so a nested
	// call is rejected rather than silently opening a second transaction on a
	// second pooled connection. See Do.
	mu      sync.Mutex
	holders map[uint64]struct{}
}

// NewUnitOfWork returns a UnitOfWork backed by pool. Construct exactly one
// per pool: the nested-transaction guard is per-UnitOfWork, so two instances
// sharing a pool would not see each other's in-flight transactions.
func NewUnitOfWork(pool *pgxpool.Pool) *UnitOfWork {
	return &UnitOfWork{pool: pool, holders: map[uint64]struct{}{}}
}

// Do runs fn as a single SERIALIZABLE transaction (FF-009 §6, AD-020).
// Returning nil commits, returning an error rolls back and propagates that
// error unchanged, and a panic rolls back and re-panics.
//
// SERIALIZABLE, with a whole-callback retry, is what makes concurrent
// revision-sequence assignment correct. The application computes a revision's
// next sequence by reading existing order metadata and then writing
// max+1 -- a read-then-write spanning the entire callback, not a single
// statement. A lock taken inside RevisionOrderRepository.Put would be too
// late, because the colliding integer is already fixed by then. PostgreSQL's
// serializable snapshot isolation detects that read-write conflict between
// two concurrent transactions and aborts one, which is retried here and
// recomputes against the now-committed state -- so both writers succeed, with
// sequences n and n+1.
//
// This makes one property load-bearing that was previously incidental: a Do
// callback must be safely re-runnable from scratch. It must not depend on
// anything outside what it writes through Repositories, must not read the
// clock again inside the callback, and must not use randomness. Every command
// in this codebase already captures Clock.Now() once, before calling Do.
func (u *UnitOfWork) Do(ctx context.Context, fn func(application.Repositories) error) error {
	gid := currentGoroutineID()

	// A nested Do would not deadlock -- the pool would simply hand it a
	// second, unrelated connection -- so the ambiguity has to be detected
	// rather than waited on. It cannot be detected through the context
	// either: Do's callback signature is func(Repositories) error, so there
	// is no channel through which to hand a marked context back into fn.
	// That constraint is structural and adapter-independent, and it is why
	// the in-memory adapter identifies the calling goroutine the same way.
	u.mu.Lock()
	if _, nested := u.holders[gid]; nested {
		u.mu.Unlock()
		return application.ErrNestedTransaction
	}
	u.holders[gid] = struct{}{}
	u.mu.Unlock()
	defer func() {
		u.mu.Lock()
		delete(u.holders, gid)
		u.mu.Unlock()
	}()

	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err = u.doOnce(ctx, fn)
		if err == nil || !isRetryable(err) {
			return mapError(err)
		}
		if backoffErr := backoff(ctx, attempt); backoffErr != nil {
			return backoffErr
		}
	}
	return fmt.Errorf("%w: %d serializable attempts exhausted: %v", application.ErrTransactionAborted, maxAttempts, err)
}

// doOnce runs one attempt. It returns the raw error so Do can decide whether
// it is retryable before mapping it.
func (u *UnitOfWork) doOnce(ctx context.Context, fn func(application.Repositories) error) (err error) {
	tx, err := u.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			// A fresh context: ctx may already be cancelled, and the
			// rollback must still be issued.
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()

	if err := fn(reposFor(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	committed = true
	return nil
}

// backoff waits before the next attempt, growing with each one. The delay is
// derived from the attempt number rather than randomness, because a Do
// callback must stay deterministic and because contention here is low enough
// that jitter buys nothing.
func backoff(ctx context.Context, attempt int) error {
	delay := min(time.Duration(attempt*attempt)*time.Millisecond, 100*time.Millisecond)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// currentGoroutineID parses the calling goroutine's numeric ID out of the
// header runtime.Stack writes ("goroutine N [running]:").
//
// This is not a supported Go API, and it is used deliberately: the only
// alternative that identifies the caller is threading a marker through
// context.Context, which UnitOfWork.Do's fixed func(Repositories) error
// callback signature makes impossible without changing an application-layer
// contract this milestone has no mandate to change. The in-memory adapter
// already does exactly this, so reusing it keeps one definition of what a
// nested transaction is, rather than two mechanisms to keep in agreement.
// If a future runtime changes the header format, the shared contract suite's
// NestedTransactionRejected case fails loudly.
func currentGoroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	fields := strings.Fields(strings.TrimPrefix(string(buf[:n]), "goroutine "))
	if len(fields) == 0 {
		return 0
	}
	id, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return 0
	}
	return id
}
