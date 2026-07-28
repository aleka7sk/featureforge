# M.4 Implementation Report — PostgreSQL Persistence and Persistence-Independence Verification

Status: Final
Date: 2026-07-28
Phase: M.4
Governs: nothing (a report, not a specification). Source of truth remains
`docs/spec/`, then `docs/decisions/`, then tests, then implementation.

## 1. Summary

M.4 adds a real PostgreSQL adapter satisfying the identical repository
contracts the in-memory adapter satisfies, and proves that FeatureForge's
engineering semantics do not depend on which one is installed.

225 tests pass (168 top-level, 57 subtests), including the full canonical
scenario and the complete repository contract suite run twice — once per
adapter — from one shared test body. `gofmt`, `go vet`, `go build`,
`go test`, `go test -race`, and the PostgreSQL integration suite are all
clean.

No domain or application semantics changed to accommodate PostgreSQL. Two
things did change, both because building the adapter exposed real gaps rather
than because PostgreSQL demanded them; both are recorded as decisions and
enforced in **both** adapters.

## 2. What was added

| Package / file | Role |
|---|---|
| `internal/infrastructure/postgres` | The adapter: pool, migration runner, embedded schema, eight repositories, error mapping, `UnitOfWork`. The only package importing `pgx`. |
| `internal/infrastructure/contracttest` | The shared contract suite, relocated from `memory/contract.go` so neither adapter imports the other for test infrastructure. Four new subtests added. |
| `internal/engineering/refkeys.go` | `ParseSubjectKey`, the inverse of the existing subject-key constructors, so both adapters resolve a `SubjectKey` by one shared definition. |
| `internal/scenario/scenario_postgres_test.go` | The canonical scenario against PostgreSQL, reusing `scenario.Run`/`RunPermuted` and the same assertion body unchanged. |
| `docker-compose.test.yml`, `Makefile` | Local and CI PostgreSQL, and one command to run everything. |
| `docs/spec/014-postgresql-persistence.md` | FF-014. |

## 3. Schema

Ten tables. Three decisions shaped it, each documented in FF-014 §3:
payloads are `bytea` rather than `jsonb` (every conflict and digest check
compares exact bytes, and `jsonb` reparses on write); each `Has*` boolean pair
becomes one nullable column so two columns cannot disagree; and `subject_key`
stays a single column rather than being denormalised into foreign-key-able
parts.

No materialized projection and no index beyond what correctness requires. The
decision log's standing "whether any query needs materialization" question
therefore stays open — that *is* the answer AD-006 asks for until measured
evidence exists.

## 4. Migrations

`embed.FS` plus a project-owned `schema_migrations` table, no framework. One
forward migration. Each migration's statements and its version row commit
together, so a partial application can never be recorded as complete.
`Migrate` is idempotent and creates its own tracking table, so the test
harness calls it unconditionally and a developer never creates a table by
hand.

## 5. Transactions and concurrency

Every `Do` callback runs under `SERIALIZABLE`, with the whole callback retried
on `40001`/`40P01`. This is the only mechanism that satisfies the milestone's
concurrency criterion, because the sequence race is an application-layer
read-then-write spanning the entire callback, not a single statement: the
command reads existing order metadata, computes `max+1` in Go, then writes it.
A lock inside `RevisionOrderRepository.Put` would arrive after the colliding
integer was already chosen.

`TestConcurrentRevisionsProduceDistinctSequences` runs four concurrent writers
against one capability and asserts all four succeed with distinct, contiguous
sequences.

### Nested-transaction detection — an explicit trade-off

This is recorded as an architectural decision rather than an implementation
detail, per AD-020 item 5.

**Why a context marker is unavailable.** `UnitOfWork.Do`'s contract is
`Do(ctx, fn func(Repositories) error) error`. The callback receives only
`Repositories`; `Do` has no channel through which to hand a derived, marked
`context.Context` back into the code running inside it. Threading one would
mean changing the signature to `func(context.Context, Repositories) error` — an
application-layer contract change M.4 has no mandate to make, touching every
existing command. The constraint is structural and adapter-independent: it is
the same reason the in-memory adapter never used a context marker, not a
PostgreSQL-specific limitation.

**Alternatives considered and rejected.** *Let it deadlock* — it would not; a
pool hands a nested call a second, unrelated connection, silently splitting one
engineering act across two transactions, which is worse than an error. *Track
depth on the pool rather than the `UnitOfWork`* — identical mechanism, less
obvious scope. *Skip detection in PostgreSQL* — `ErrNestedTransaction` is part
of the shared contract suite both adapters must pass, so it is not optional.
*A different mechanism for PostgreSQL only* — rejected as the worst option:
two adapters detecting one condition two different ways means two behaviours to
keep in agreement, two sets of edge cases, and a contract suite that no longer
proves the adapters are equivalent.

**Why reuse.** Reusing the in-memory adapter's `runtime.Stack`-parsing keeps
one definition of what a nested transaction is.

**Cost accepted.** Parsing the `goroutine N [running]:` header is not a
supported Go API and could break on a future runtime. Accepted because the
alternative is an application-layer contract change; because the same code
already ships in the in-memory adapter, so the risk is reused rather than newly
introduced; and because the failure mode is a loud failure in the shared
suite's `NestedTransactionRejected` case, not silent corruption.

**Documented assumption.** The guard is tracked per `UnitOfWork` value, not per
pool. Every call site constructs exactly one `UnitOfWork` per pool, mirroring
the in-memory adapter's one-per-`Store` pattern, but that is a convention, not
an enforced invariant.

### The new precondition

Retrying a callback makes retry-safety load-bearing: no second clock read, no
randomness, no side effect outside `Repositories`. Every command already
captured `Clock.Now()` once before `Do` — incidentally. It is now enforced by
`TestDoCallbacksAreRetrySafe`, which **found one real violation** in
`CorrectValidationClaimCommand`, fixed in this milestone.

## 6. `revision_acceptance` identity — confirmed, with one correction

The journal is internal, not an externally referenceable entity: the repository
exposes only `Append`, `ListByRevision`, and `ListByArtifact` — never a lookup
by identity — and no foreign key targets it. The surrogate `bigserial` primary
key is therefore correct and was kept.

But the plan's original claim that `record_id` had no uniqueness contract was
true only of the *implementation*. FF-009 §4.3 declares it "Product-owned
identity, unique", and FF-006 §2 derives a timeline event's identity from it
(`query_timeline.go` sets both `EventID` and `SourceIdentity` from
`RecordID`), with FF-006 stating that two records sharing an identity is a
persistence bug. So `record_id` is not *referenceable*, but it is
spec-declared *unique* and externally *visible*. Both adapters now enforce
create-only semantics on it. This required no test fixture changes: the only
duplicate literals in the suite are in two separate tests that build
non-persisted journal slices and never call `Append`.

## 7. Contract-suite reuse

`RunRepositoryContractSuite` moved to `internal/infrastructure/contracttest`
unchanged and now runs 20 subtests — the original 16 plus four for the AD-021
invariants. Both adapters call it with a one-line factory. No PostgreSQL-only
weakening exists anywhere in it.

One non-obvious detail: the factory signature takes no `*testing.T`, because
subtests run in their own goroutines with their own `T` and calling `t.Fatal`
on an outer captured `T` from a subtest goroutine is a Go testing bug. The
PostgreSQL factory therefore panics on unrecoverable setup failure — the
framework attributes a panic to the subtest that caused it — and the
env-var check happens once, in the outer test, with the correct `T`.

## 8. Canonical scenario parity

`scenario.Run` and `scenario.RunPermuted` are unchanged; only the injected
`UnitOfWork` differs. `TestCanonicalScenario`'s ~250-line assertion body was
extracted into `assertCanonicalEndState` and is now run by both the in-memory
and PostgreSQL tests, so parity is one assertion body executed twice rather
than two bodies that could drift. The same was done for the insertion-order
comparison.

Both adapters produce identical results for: current capability revision
(`CAP-1/CAP-1-REV-2`, sequence 2), current claim per requirement (`CLM-1`,
`CLM-4` correcting `CLM-2`, `CLM-3`, and none for the uncovered `REQ-4`),
readiness (`not-ready`), lifecycle state (`featureforge:under-validation`),
and the timeline including `CLM-4`'s link back to `CLM-2`. Insertion-order
independence holds on both.

## 9. Deviations from the approved plan

Three, all discovered by implementation and none silent.

1. **The correction-target write check was dropped.** The plan had
   `RecordEnvelopeRepository.Put` verify `CorrectionTargetID` resolves.
   Implementing it broke four tests that deliberately store dangling,
   self-referential, and cyclic correction graphs to exercise the read-side
   algorithm — which is exactly what AD-017 requires (`ResolveCurrentClaim`
   must stay total over any stored graph, with the actionable write-side check
   living in `CorrectValidationClaimCommand`). A third check at the repository
   layer duplicated the command-layer one and made the read-side guarantee
   unreachable. Recorded in AD-021.
2. **`mapError` moved to a single call site.** The plan had repositories map
   driver errors. That destroys the `*pgconn.PgError` before `Do` can test
   retryability, silently turning every contended write into an error instead
   of a retry — caught by the concurrency test. Repositories now return raw
   driver errors and `Do` maps once, on the way out.
3. **The published port moved from 5433 to 55433.** 5433 was chosen to avoid
   colliding with a developer's PostgreSQL and promptly collided with one on
   this machine. Now defaults to 55433 and is overridable via
   `FEATUREFORGE_POSTGRES_PORT`.

## 10. Tests

| Area | Coverage |
|---|---|
| Repository contracts | 20 subtests, run against both adapters from one body |
| Migrations | Idempotence; every expected table created from an empty database |
| Concurrency | Four concurrent writers producing distinct contiguous sequences; nested `Do` rejected |
| Projection fidelity | Typed columns compared against the decoded payload, and against the adapter's own read-back; payload byte-identity verified against its stored digest |
| Canonical scenario | Full FF-011 end state, on both adapters; insertion-order independence, on both |
| Architecture | 21 tests. Five are new for M.4, and **each was verified against a deliberately introduced violation** and then reverted: application importing the driver (caught by two independent tests), the adapters importing each other, an `UPDATE` against an engineering table, an unapproved direct `go.mod` requirement, and a `Clock.Now()` inside a `Do` callback. |

## 11. Dependencies added

`github.com/jackc/pgx/v5` (direct), plus the transitive set `go mod tidy`
resolved: `pgpassfile`, `pgservicefile`, `puddle/v2`, `golang.org/x/sync`,
`golang.org/x/text`. No ORM, no query builder, no migration library, no test
framework, no assertion library, no testcontainers. No replace directive.
`TestGoModHasOnlyApprovedRequirements` fails the build if any other *direct*
requirement appears.

## 12. PEOS

No PEOS problem was found, and PEOS was not modified. The SDK's symmetric
JSON marshalling made byte-exact payload storage straightforward, and the
adapter never needed to reach into a PEOS value — it stores what the codec
produced and hands it back.

## 13. Known limitations, deferred

- Pool sizing, timeouts, and retry tuning are chosen for one local developer,
  not measured under load.
- No query performance work, and no index beyond correctness.
- One forward-only migration; no rollback, dry-run, or drift detection.
- The two FF-009 drifts (§7's `sync.RWMutex`, §8's three unimplemented
  serialization sentinels) are recorded as errata in FF-014 §9 rather than
  closed — closing either widens M.4 beyond persistence parity for no
  behavioural gain.
- `RelationEnvelope` still deferred (AD-013).

## 14. Readiness for M.5

Every quality gate is green. Domain and application remain free of driver
imports, architecture-test-enforced. Both adapters satisfy one contract suite
and produce one canonical end state. The repository contracts in
`internal/application/ports.go` are unchanged from M.3 apart from two
documented guarantees, so M.5's HTTP surface has a stable substrate to build
on, and `internal/scenario` remains reusable as an end-to-end fixture.
