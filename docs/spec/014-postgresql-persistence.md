# FF-014 — PostgreSQL Persistence

Status: Accepted (Phase M.4)
Governs: the PostgreSQL schema, its migration mechanism, the transaction and
concurrency model, and the error mapping every FeatureForge persistence
adapter must reproduce.

## Governance statement

PEOS-000 through PEOS-009 remain authoritative for every PEOS concept.
`docs/spec/` governs FeatureForge. This document adds no PEOS concept and
changes none. PEOS v1.0.0 is used unchanged, and no PEOS problem was found
during M.4.

## 1. Why PostgreSQL is not the centre of this milestone

M.3 proved the canonical scenario runs end to end. It did not prove the
engineering answers were independent of *where* the records were kept —
with a single in-process store, a query could have accidentally depended on
Go map iteration order, on insertion order, or on the fact that every write
was serialised by one mutex.

M.4 settles that by adding a second, genuinely different adapter and
requiring both to satisfy one shared contract suite and produce one identical
canonical-scenario end state. PostgreSQL is the instrument, not the subject.
Nothing in `internal/domain`, `internal/engineering`, or
`internal/application` changed to accommodate it, and an architecture test
fails the build if any of them ever imports the driver.

The claim this milestone makes is narrow and testable: **the meaning of
current-revision resolution, correction chains, readiness, lifecycle state,
and the timeline is a property of the algorithms in `internal/application`,
not of the store beneath them.**

## 2. Package structure

| Package | Role |
|---|---|
| `internal/infrastructure/postgres` | The adapter. The only package permitted to import `github.com/jackc/pgx/v5`. |
| `internal/infrastructure/contracttest` | The shared contract suite, moved here from the in-memory adapter so neither adapter imports the other to borrow test infrastructure. |
| `internal/infrastructure/memory` | Unchanged in role; now runs the relocated suite. |

`postgres` exposes `Connect`, `Migrate`, `NewUnitOfWork`, and nothing else.
No pgx type appears in any exported signature, so a future adapter can
replace it without touching a caller.

## 3. Schema

One migration, `migrations/0001_initial_schema.sql`. Ten tables: `projects`,
`feature_cards`, `feature_card_capability_links`, `artifact_envelopes`,
`revision_envelopes`, `structured_content`, `record_envelopes`,
`revision_order`, `revision_acceptance`, plus the runner's own
`schema_migrations`.

Three decisions shape it.

**Payloads are `bytea`, not `jsonb`.** Every idempotency, conflict, and
digest check in this codebase compares canonical JSON as exact bytes
(`samePayload`, `Digest.Equal`). PostgreSQL's `jsonb` reparses and
reserialises on write with no byte-identity guarantee, so a payload written
and read back could fail its own digest check. `jsonb`'s one real advantage
— querying inside the document — is unused here, because every field a query
needs is already projected into its own column. `TestPayloadIsStoredByteIdentical`
pins this.

**A `Has*` pair becomes one nullable column.** `RevisionEnvelope` carries
`ProvenanceActor`/`HasProvenanceActor`; the table carries a nullable
`provenance_actor` where SQL `NULL` *is* the absence. Two columns that can
disagree is a bug waiting to happen.

**`SubjectKey` stays one column.** It is already the canonical identifier for
a record's subject. Splitting it into foreign-key-able parts
(`subject_artifact_id`, `subject_revision_artifact_id`, …) purely so
PostgreSQL could enforce it would mean maintaining two representations of one
relationship, with no query or performance need to justify the duplication.
Existence is verified instead by `recordRepo.Put`, which parses the key
through the shared `engineering.ParseSubjectKey` and `SELECT`s the referenced
row inside the same transaction as the insert — structurally the same check
the in-memory adapter performs. See AD-021.

`criterion_keys`, `evidence_keys`, and `execution_keys` are plain `text[]`
with no GIN index: every reader lists by kind, or by kind and subject, and
then filters in Go. No SQL predicate ever searches inside these arrays, so an
index would optimise a query that does not exist.

### `revision_acceptance` identity

The primary key is a surrogate `bigserial`, because the acceptance journal is
internal: the repository exposes only `Append`, `ListByRevision`, and
`ListByArtifact` — never a lookup by identity — and no foreign key anywhere
targets it.

`record_id` is nonetheless `UNIQUE`. [FF-009 §4.3](009-in-memory-persistence.md)
declares it a product-owned unique identity, and
[FF-006 §2](006-timeline-read-model.md) derives a timeline event's identity
from it, so two entries sharing a `record_id` would collide two timeline
events onto one event ID — which FF-006 itself calls a persistence bug.
`Append` is therefore create-only on `RecordID` in *both* adapters.

## 4. Migrations

No migration framework. SQL files are embedded with `embed.FS` and tracked in
a project-owned `schema_migrations` table that the runner creates itself, so
`Migrate` is safe to call on an empty database, on every process start, and
at the top of every integration test. Each migration's statements and its
version row are written in one transaction, so a partially applied migration
can never be recorded as complete.

`schema_migrations` is deliberately absent from the migration file: it is
runner infrastructure, not schema content.

## 5. Transactions and concurrency

Every `Do` callback runs inside a `SERIALIZABLE` transaction. On SQLSTATE
`40001` or `40P01`, `UnitOfWork.Do` re-runs **the whole callback**, up to
eight attempts, before surfacing `ErrTransactionAborted`.

This is not a default choice; it is the only mechanism that makes concurrent
revision-sequence assignment correct. `ReviseCapabilitySpecificationCommand`
reads existing order metadata, computes `max+1` in Go, and then writes it —
a read-then-write spanning the entire callback. A lock taken inside
`RevisionOrderRepository.Put` would be too late, because the colliding
integer is already fixed by the time `Put` runs. Serialisable snapshot
isolation detects the read-write conflict, aborts one transaction, and the
retry recomputes against the now-committed state, so **both** writers succeed
with sequences n and n+1. `TestConcurrentRevisionsProduceDistinctSequences`
asserts exactly that.

### The precondition this creates

A callback that may be re-run must be safely re-runnable: no side effect
outside what it writes through `Repositories`, no second read of the clock,
no randomness. Every command already captured `Clock.Now()` once before
calling `Do` — but that was incidental, and is now load-bearing.
`TestDoCallbacksAreRetrySafe` enforces it, and found one real violation in
`CorrectValidationClaimCommand` when it was introduced.

### Nested transactions

A nested `Do` cannot be allowed to proceed: a pool would simply hand it a
second, unrelated connection, silently splitting one engineering act across
two transactions. It also cannot be detected through `context.Context` —
`Do`'s callback signature is `func(Repositories) error`, so `Do` has no
channel through which to hand a marked context back into the callback. That
constraint is structural and adapter-independent; it is the same reason the
in-memory adapter never used a context marker either.

Both adapters therefore identify the calling goroutine by parsing the header
`runtime.Stack` writes. This is deliberately an unsupported Go API, accepted
because the alternative is changing an application-layer contract this
milestone has no mandate to change, because the same technique already ships
in the in-memory adapter (so the risk is reused, not newly introduced), and
because a runtime change would fail loudly in the shared contract suite's
`NestedTransactionRejected` case rather than corrupt data. See AD-020.

**Documented assumption:** the guard is tracked per `UnitOfWork` value, not
per pool. Every call site constructs exactly one `UnitOfWork` per pool,
mirroring the in-memory adapter's one-per-`Store` pattern. Two `UnitOfWork`
values sharing one pool would not see each other's in-flight transactions.

## 6. Error mapping

`mapError` is called in exactly one place — `UnitOfWork.Do`, on the way out.
Repository methods deliberately return raw driver errors. Mapping inside a
repository would replace the `*pgconn.PgError` with a wrapped sentinel, and
`Do` could then no longer tell a retryable serialization failure from a
permanent one — silently turning every contended write into an error instead
of a retry. That was a real bug during implementation, caught by the
concurrency test.

| Condition | Detection | Maps to |
|---|---|---|
| Foreign-key violation | SQLSTATE `23503` | `ErrReferencedValueMissing` |
| Unresolvable `subject_key` | explicit `SELECT` in `recordRepo.Put` | `ErrReferencedValueMissing`, raised directly in Go |
| Same key, differing payload | `ON CONFLICT DO NOTHING`, then compare | `nil` if identical, else `ErrImmutableValueConflict` |
| Duplicate `(artifact_id, sequence)` | `23505` on `revision_order_artifact_sequence_key` | `ErrRevisionSequenceConflict` |
| Any other unique violation | `23505` | `ErrImmutableValueConflict` |
| Serialization failure or deadlock, retries exhausted | `40001` / `40P01` | `ErrTransactionAborted` |
| Not a `*pgconn.PgError` | `errors.As` fails | returned unchanged, so `errors.Is` still matches the original cause |

Create-only writes use `ON CONFLICT DO NOTHING` plus an explicit comparison
rather than catching `23505`, because idempotent re-writes are routine here —
a retried callback re-issues every insert it already made — and should not
travel the error path.

## 7. Immutability

No `UPDATE` or `DELETE` statement exists against any engineering table.
`TestNoUpdateOrDeleteOnEngineeringTables` scans the adapter's Go and SQL
sources and fails the build if one appears. Correcting a record means writing
a new record that references it, exactly as in memory.

## 8. Running the tests

```
make postgres-test     # starts PostgreSQL, migrates, runs every DB-backed test, tears down
make verify            # the above plus fmt, vet, build, test, and -race
```

`make postgres-test` is the only command needed; migrations are applied by the
test harness itself, so no schema or table is ever created by hand. Each test
gets its own uniquely named schema, migrated fresh and dropped afterwards, so
no test depends on `TRUNCATE` ordering across the foreign-key graph.

PostgreSQL-backed tests are gated on `FEATUREFORGE_POSTGRES_TEST_DSN` rather
than a build tag: an unset variable makes them skip, so plain
`go test ./...` stays meaningful without Docker, whereas a build tag would
silently not compile them at all. The container publishes port 55433 by
default — clear of the 5432–5434 range local instances tend to occupy — and
`FEATUREFORGE_POSTGRES_PORT` overrides it.

## 9. Corrections to FF-009

FF-009 is accepted and is not rewritten. Two drifts between its prose and the
M.3 implementation were found while building this adapter, and are recorded
here as errata rather than by editing already-accepted text — the same
append-never-rewrite discipline the decision log uses.

| FF-009 says | The code does | Resolution |
|---|---|---|
| §7: the in-memory store guards itself with `sync.RWMutex` | It uses a plain `sync.Mutex` | Errata only. Every `Do` holds the lock for its full duration, so a read/write distinction would buy nothing; the prose is simply more specific than the design requires. |
| §8: lists `ErrStoredPayloadInvalid`, `ErrPayloadDigestMismatch`, `ErrContentIntegrityMismatch` | None of the three exists; neither adapter re-verifies a digest on read | Errata only. Payload validation happens in the envelope constructors at write time, and `bytea` guarantees byte-exact storage, so there is no adapter-introduced corruption path for a read-time check to catch. Introducing the sentinels now would add unreachable code. |

Both are deliberate non-changes: closing either would widen M.4 beyond
persistence parity for no behavioural gain.

## 10. Deferred to M.5 and beyond

- Connection-pool sizing, timeouts, and retry tuning under real load. The
  current values are chosen for a single local developer, not measured.
- Query performance. No index exists beyond what correctness requires, and
  the open question "whether any query needs materialisation" stays open
  until there is measured evidence (AD-006).
- A production migration workflow — rollback, dry-run, drift detection. M.4
  ships one forward-only migration for an empty database.
- `RelationEnvelope`, still deferred until a relation is actually recorded
  (AD-013).
