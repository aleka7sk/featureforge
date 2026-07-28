# M.4 Architecture Review

Status: Final
Date: 2026-07-28
Phase: M.4 (retrospective)
Governs: nothing. This is a review, not a specification. Source of truth
remains `docs/spec/`, then `docs/decisions/`, then tests, then implementation.

---

## Executive summary

**The question M.4 asked.** M.3 proved the canonical scenario runs end to end
against a single in-memory store. That proved the scenario worked; it did not
prove the *architecture* worked. With one process-local store, every query
could silently have depended on Go map iteration order, on insertion order, or
on the fact that a single mutex serialised every write. M.4 existed to settle
one question: **are FeatureForge's engineering answers a property of the
algorithms, or of the store beneath them?**

**Was the architecture validated?** Yes. A second adapter — PostgreSQL,
sharing zero code with the first below the repository interfaces — satisfies
the identical 20-subtest contract suite and produces a byte-for-byte identical
canonical end state, including insertion-order independence. The abstraction
held under a genuinely different implementation.

**Was any architectural redesign required?** **No.** The complete set of
production changes M.4 made to the layers above infrastructure was:
`internal/domain` — *zero changes*; `internal/application/ports.go` —
*comment-only*, no signature altered; `command_validation.go` — six lines
fixing a latent bug that PostgreSQL exposed but did not cause;
`engineering/refkeys.go` — forty additive lines. **No interface signature was
changed to accommodate PostgreSQL.**

Three implementation-level corrections were made — a correction-target check
withdrawn because it conflicted with AD-017, error mapping relocated to a
single point, and a test port moved. All three are control-flow corrections
inside the adapter or its tests. None is an architectural change.

**Is FeatureForge ready to continue?** **READY.** All five FF-007 M.4 exit
criteria are met. Four issues are recorded as named, non-blocking follow-ups;
none obstructs an HTTP layer.

**Overall conclusion.** M.4 did not merely fail to break the architecture — it
actively validated it, and in three places the architecture's own verification
machinery found defects that review had not. The most valuable single outcome
was not the PostgreSQL adapter; it was the discovery that **contracts
constrain data shape but say nothing about control flow**, and that the gaps
this leaves are real, are findable by test, and were found.

---

## 1. Architecture validation

Each decision is assessed as *assumption before implementation → what
implementation proved → verdict → confidence*.

### 1.1 Repository contracts as three envelope types (AD-013)

**Assumption.** Three envelope types — artifact, revision, record — with
family-specific repositories, would map onto a relational schema without
reshaping.

**What implementation proved.** Each envelope became one table with its own
natural key. `RecordEnvelope`'s projected fields became indexed columns
directly. No envelope needed splitting, merging, or a discriminator column.
AD-013's rejected alternative — one universal envelope — would have forced the
widest key shape on all three tables and made per-table uniqueness
inexpressible.

**Verdict.** Valid, and specifically vindicated: the reason AD-013 gave for
rejecting a universal envelope ("a repository can no longer state its
uniqueness constraint in its own signature") is exactly what made the SQL
schema straightforward.

**Confidence: High.** *Validated by: contract tests (both adapters);
implementation observation.*

### 1.2 The `UnitOfWork` abstraction

**Assumption.** `Do(ctx, func(Repositories) error) error` is sufficient to
express an atomic engineering act on any store.

**What implementation proved.** It is sufficient — and its shape turned out to
be load-bearing in a way not anticipated. The signature gives `Do` no channel
to hand a marked `context.Context` back into the callback, which forced both
adapters to detect nested transactions the same way (E5, §2.3). The
abstraction also proved sufficient to express `SERIALIZABLE` retry without any
caller change, because the retry boundary and the atomicity boundary are the
same boundary.

**Verdict.** Valid. One constraint is now understood to be structural rather
than incidental.

**Confidence: High.** *Validated by: contract tests; concurrency tests
(`TestNestedDoIsRejected`); implementation observation E5.*

### 1.3 Domain and application independence of persistence (AD-005)

**Assumption.** Domain and application would need no change when persistence
was replaced.

**What implementation proved.** Measured against the commit: `internal/domain`
changed by zero lines. `internal/application/ports.go` changed by
comment-only additions. The only application logic change was six lines fixing
a bug PostgreSQL *exposed* rather than caused. No interface signature changed.

**Verdict.** Valid, with the strongest evidence available short of a third
adapter.

**Confidence: High.** *Validated by: `git show --stat` on the M.4 commit;
architecture verification tests (`TestDomainAndApplicationDoNotImportDriver`,
`TestOnlyPostgresInfrastructureImportsDriver`); deliberate violation tests.*

### 1.4 A shared contract test suite

**Assumption.** One suite, run against every adapter, is the right way to
define "conforming".

**What implementation proved.** It is, and it caught real divergence during
development rather than after. The suite required relocation to its own
package (`internal/infrastructure/contracttest`) so neither adapter imports
the other for test infrastructure — a mechanical consequence of there being
two adapters, invisible when there was one.

One structural detail proved essential: the factory signature takes no
`*testing.T`, because subtests run in their own goroutines with their own `T`,
and calling `t.Fatal` on an outer captured `T` from a subtest goroutine is a
Go testing bug. The PostgreSQL factory therefore panics on unrecoverable
setup failure.

**Verdict.** Valid. This is the single most load-bearing test asset in the
project.

**Confidence: High.** *Validated by: 20 subtests × 2 adapters, passing.*

### 1.5 In-memory adapter parity

**Assumption.** The in-memory adapter is a faithful reference, not a toy.

**What implementation proved.** Mostly true, with two measured asymmetries.
PostgreSQL exposed three invariants the specifications declared but the
in-memory adapter never enforced (AD-021). And two behavioural differences
remain by design: the nested-transaction guard shape (E5) and the failure
injection mechanism (E9), which is memory-only.

**Verdict.** Valid *after* AD-021 closed the enforcement gaps. Before M.4 the
in-memory adapter was weaker than its own specification.

**Confidence: Medium-High.** High for everything the shared suite covers;
Medium for what it does not — PostgreSQL never exercises mid-act *injected*
failure rollback (E9), though it does exercise `RollbackDiscardsAllWrites` and
`RollbackOnPanic`.

*Validated by: contract tests; implementation observation E9.*

### 1.6 `SubjectKey` as a single opaque string

**Assumption.** A record's subject can be a projected string rather than a
structured reference.

**What implementation proved.** The relational instinct is to denormalise
`SubjectKey` into foreign-key-able columns so the database can enforce it.
That was considered and rejected: `SubjectKey` is already the canonical
identifier, and splitting it would mean maintaining two representations of one
relationship. Instead both adapters parse it through one shared
`engineering.ParseSubjectKey` and verify existence in application code — a map
lookup in memory, a `SELECT` in the same transaction in PostgreSQL.

The result is that the two adapters' subject-verification code is
*structurally identical*, differing only in the lookup primitive.

**Verdict.** Valid. The storage model stayed close to the domain model at the
cost of one extra read inside a transaction that was about to write anyway.

**Confidence: High.** *Validated by: contract tests
(`RecordEnvelopePutRequiresResolvableSubject`,
`RecordEnvelopePutRejectsMalformedSubjectKey`); scenario execution on both
adapters.*

### 1.7 Migration strategy

**Assumption.** Embedded SQL plus a project-owned version table is sufficient;
no migration framework needed.

**What implementation proved.** Sufficient, and one real bug surfaced
immediately: the runner bootstrapped `schema_migrations` *and* the migration
file created it, so the first run against a real database failed with
`42P07`. The fix clarified ownership — the version table is runner
infrastructure, not schema content.

**Verdict.** Valid for one forward migration on an empty database. Explicitly
not validated for rollback, drift detection, or migrating a populated
database, none of which M.4 attempted.

**Confidence: Medium.** High for what was tested; the strategy is simply
untested beyond a single forward migration.

*Validated by: `TestMigrateIsIdempotent`, `TestMigrateCreatesEveryTable`;
implementation observation E11.*

### 1.8 `SERIALIZABLE` plus whole-callback retry

**Assumption (formed during M.4 planning, from reading the command code).**
The revision-sequence race is an application-layer read-then-write spanning
the whole callback, so only a whole-callback mechanism can be correct.

**What implementation proved.** Correct, and the reasoning was load-bearing
rather than academic: the first implementation retried nothing, because
repositories mapped driver errors eagerly and destroyed the retryability
signal (E2). The concurrency test failed, which is how the design was found to
be right and the implementation wrong.

**Verdict.** Valid at the concurrency level tested.

**Confidence: Medium.** Proven correct with four concurrent writers on one
artifact. Never tested under sustained load, and retries are silent (E6), so
there is no evidence about behaviour under heavier contention.

*Validated by: concurrency tests
(`TestConcurrentRevisionsProduceDistinctSequences`); implementation
observation E2.*

### 1.9 Embedded schema

**Assumption.** `embed.FS` keeps schema and code versioned together.

**What implementation proved.** Uneventful — the schema compiles into the
binary, the test harness applies it with no external step, and no developer
ever creates a table by hand. Nothing was learned that would change it.

**Verdict.** Valid.

**Confidence: High**, though this is the least interesting decision in the
list — it was never under stress. *Validated by: `TestMigrateCreatesEveryTable`;
`make postgres-test` from zero containers.*

### 1.10 Architecture verification tests

**Assumption.** Import-boundary rules encoded as build-failing tests are worth
their maintenance cost.

**What implementation proved.** Decisively worth it. Five new tests were added
and **each was verified against a deliberately introduced violation** before
being trusted. One of them — `TestDoCallbacksAreRetrySafe` — found a genuine
latent bug in existing M.3 code (E1) on the day it was written.

A test that has never failed has not been verified. Five out of five did.

**Verdict.** Valid, and this is the mechanism that produced the milestone's
highest-value finding.

**Confidence: High.** *Validated by: deliberate violation tests (E12); the E1
bug it found.*

### 1.11 Computed, never materialised (AD-006)

**Assumption.** Current state should be computed on read, never stored.

**What implementation proved.** No materialised projection was needed, and no
index beyond what correctness requires was added. The relational adapter — the
context in which the temptation to denormalise is strongest — produced no
evidence that materialisation was necessary.

**Verdict.** Valid. Notably, the decision log's open question "whether any
query needs materialization" remains open, and *that is the correct outcome*:
AD-006 asks for measured evidence, and none was produced because none was
needed.

**Confidence: High** for the scenario's data volume; **no evidence** at larger
volumes, which is stated rather than assumed. *Validated by: scenario
execution on both adapters; implementation observation — no materialisation
was written.*

### 1.12 FF-007 M.4 exit criteria

| Criterion | Status | Validated by |
|---|---|---|
| Both adapters pass the identical contract suite | Met | 20 subtests × 2 adapters |
| Round-trip, idempotence, conflict behaviour verified on PostgreSQL | Met | Contract tests; `TestPayloadIsStoredByteIdentical` |
| No `UPDATE` or `DELETE` against an engineering-record table | Met | `TestNoUpdateOrDeleteOnEngineeringTables`, verified against an injected violation |
| Every derived model can be dropped and rebuilt with identical output | Met **vacuously** | There is no stored derived model to drop (AD-006) |
| Canonical scenario passes against PostgreSQL | Met | `TestCanonicalScenarioPostgres` + insertion-order variant |

The fourth deserves precision. It is satisfied because nothing derived is
stored at all — which is the correct outcome under AD-006, but it is *not the
same as having been tested*. If materialisation ever arrives, this criterion
becomes a real obligation rather than a tautology.

**Correction to an earlier draft finding.** A verification pass initially
reported that the concurrency and UPDATE/DELETE criteria were satisfied only
by memory-only tests. That is false: `postgres/unitofwork_test.go:26` and
`architecture_test.go:134`. Both are covered against PostgreSQL. The claim is
recorded here because a review that propagates an unverified coverage claim is
worse than no review.

---

## 2. Unexpected discoveries

### 2.1 `Clock.Now()` inside a retry callback (E1)

`CorrectValidationClaimCommand` read the clock *inside* its `Do` callback. Under
`SERIALIZABLE` retry, two attempts would stamp two different times on one
engineering act, and whichever attempt committed would carry a timestamp that
never corresponded to when the act was attempted.

**Why it happened.** Every other command captured the clock before `Do` — by
habit, not by rule. Nothing enforced it because before M.4 nothing re-ran a
callback.

**Why planning missed it.** Planning reasoned about *what* commands write.
This is a property of *when* they read.

**Should architecture change?** No — but the invariant is now explicit and
enforced by `TestDoCallbacksAreRetrySafe`. It was promoted from incidental
habit to load-bearing rule.

### 2.2 `mapError` placement (E2)

Repositories initially translated driver errors to application sentinels
directly. This replaced the `*pgconn.PgError` with a wrapped sentinel, so `Do`
could no longer distinguish a retryable serialization failure from a permanent
error — silently turning every contended write into a failure.

**Why it happened.** Mapping at the point of raising is the obvious placement,
and it is wrong here.

**Why planning missed it.** The plan specified the error-mapping *table*
correctly and said nothing about where the mapping executes. The table was
right; the placement was not derivable from it.

**Should architecture change?** The rule is now recorded in FF-014 §6:
translation belongs where the *decision* is made, not where the error is
raised. No structural change.

### 2.3 Correction-graph validation conflicts with AD-017 (E3)

The plan called for `RecordEnvelopeRepository.Put` to verify
`CorrectionTargetID` resolves. Implementing it broke exactly four tests
(`TestMissingTarget`, `TestSelfCorrection`, `TestCycle`,
`TestDanglingReferenceFails`) which deliberately store degenerate correction
graphs to exercise the read-side algorithm.

**Why it happened.** AD-017 deliberately places correction validation in *two*
places: write-side in `CorrectValidationClaimCommand` for the actionable
error, and read-side in `ResolveCurrentClaim` so resolution stays total over
any stored graph. A third check at the repository layer duplicated the first
and made the second unreachable.

**Why planning missed it.** The plan treated referential integrity as
uniformly desirable. AD-017 had already decided that for *this* reference it
is not — the read side must tolerate what the write side rejects.

**Should architecture change?** No. Implementation *confirmed* AD-017. This is
the clearest case in the milestone of tests defending an accepted decision
against a well-intentioned change.

### 2.4 Nested-guard shape diverges by adapter (E5)

The in-memory adapter tracks a single `uint64`; PostgreSQL tracks a
`map[uint64]struct{}`.

**Why it happened.** Memory holds one mutex for the whole callback, so at most
one goroutine can ever be inside `Do`. PostgreSQL takes no global lock — each
`Do` gets its own pooled connection — so N goroutines are genuinely in flight
and the guard must be a set.

**Why planning missed it.** The plan specified the *mechanism* (goroutine ID)
and assumed the data structure would follow. The concurrency model dictated
the structure.

**Should architecture change?** No. This is correct divergence: the two
adapters have different concurrency models, and identical behaviour is
required, not identical code.

### 2.5 `revision_acceptance` identity

FF-009 §4.3 declares `RecordID` "Product-owned identity, unique", and FF-006 §2
derives a timeline event's identity from it — so two entries sharing one would
collide two timeline events onto one event ID. The in-memory adapter never
enforced this; `Append` appended unconditionally.

**Why planning missed it.** The M.4 plan initially recorded that `Append` had
"no uniqueness contract", which was true of the *implementation* and false of
the *specification*.

**Should architecture change?** The contract was strengthened in both adapters
(AD-021). The table keeps a surrogate `bigserial` key — the journal remains
internal, with no lookup by identity and no foreign key targeting it — while
`record_id` gains a `UNIQUE` constraint. Identity uniqueness and external
referenceability are different properties.

### 2.6 Environment assumptions (E11)

Port 5433 was chosen *specifically* to avoid colliding with a developer's local
PostgreSQL, and collided with one on the first run. `schema_migrations` was
created twice. Both were fixed in minutes and neither is architectural, but
they are recorded because both were assumptions that survived planning
unexamined and died on first contact with a real environment.

### 2.7 The common root cause

E1, E2, and E3 share one shape: **planning reasoned about the contracts;
implementation reasoned about the control flow between them.** Each was
invisible in a design document and unmissable once code executed. This is the
milestone's central lesson (§8).

---

## 3. Architectural debt

Real items only. Where no debt exists in a category, it is stated.

### 3.1 Temporary debt

**Test-harness connection churn (E7).** The contract factory captures the
outer `t`, so `t.Cleanup` registers on the parent test rather than the
subtest. One suite run therefore creates 20 schemas and leaves 20
schema-scoped pools open simultaneously until the whole suite finishes. It
works today against a local container; against a small shared server it could
exhaust `max_connections`. Test-only. Fix is mechanical.

**Environment assumptions (E11-class).** The default port is now high (55433)
and overridable, but the class of assumption — "this port is free" — is
inherently fragile and will recur in CI.

### 3.2 Permanent trade-offs

**`runtime.Stack` goroutine-ID parsing.** Knowingly accepted in AD-020. Not a
supported Go API. Accepted because the alternative is changing an
application-layer callback signature, because the technique is *reused* from
the in-memory adapter rather than newly introduced, and because a runtime
change fails loudly in the shared suite's `NestedTransactionRejected` case
rather than corrupting data. This will not be paid off; it is a standing cost.

**Guard-shape divergence (E5).** Two implementations of one behaviour. Correct,
but it means the nested-transaction guarantee is verified twice rather than
once.

### 3.3 Deliberate simplifications

**Silent retries (E6).** No log, metric, or counter on retry. Only exhaustion
surfaces. Deliberate — there is no observability substrate yet — but it means
retry frequency is currently unobservable in principle, not merely
unmeasured.

**O(n) acceptance scan (E8).** The in-memory adapter scans the whole journal
per `Append` to enforce `RecordID` uniqueness, where PostgreSQL uses an
indexed constraint. Irrelevant at scenario scale; a real difference in
asymptotic behaviour between adapters that are otherwise behaviourally
identical.

**Failure-injection asymmetry (E9).** `FailureHook` is memory-only. PostgreSQL
never exercises mid-act *injected* failure rollback. It does exercise
`RollbackDiscardsAllWrites` and `RollbackOnPanic` through the shared suite, so
rollback itself is covered — but arbitrary mid-act failure is not.

**No index beyond correctness.** Deliberate under AD-006. Recorded so that a
future performance investigation does not mistake absence for oversight.

### 3.4 Future scalability concerns

**Pool and retry tuning.** Both are set for one local developer. `maxAttempts
= 8` and the backoff curve are unmeasured choices.

**AD-010 verification gap (E4, detailed in §5 and the Appendix).** The
strongest integrity guarantee the project claims is verifiable but never
verified in a production path.

**Attribution.** E4 is a **pre-existing M.3 gap that M.4 surfaced**, not debt
M.4 introduced. The distinction matters: M.4 did not weaken the guarantee, it
revealed that the guarantee was never enforced.

---

## 4. Contract review

The contracts stabilised or introduced in M.4:

| Contract | Status after M.4 |
|---|---|
| Eight repository interfaces (`ports.go`) | Unchanged in signature; two doc-level guarantees added |
| `UnitOfWork.Do` | Unchanged; retry semantics now defined behind it |
| Transaction semantics | Commit-on-nil, rollback-on-error, rollback-and-repanic-on-panic, `ErrNestedTransaction` on reentry — verified identically on both adapters |
| Retry guarantees | New: callbacks may be re-run; must be re-runnable |
| Identity guarantees | New (AD-021): acceptance `RecordID` unique |
| Referential guarantees | New (AD-021): feature card → project; record → subject |

**Should any public contract change before M.5?** No.

**Why freezing is now appropriate.** A contract satisfied by two
implementations that share zero code below the interface — one built on Go
maps with a global mutex, one on SQL with foreign keys, arrays, surrogate
keys, and snapshot isolation — is *empirically* load-bearing rather than
merely asserted. Before M.4 the interfaces described one implementation.
After M.4 they describe a genuine abstraction, because a second implementation
was fitted to them without a single signature changing.

**Two known gaps, neither requiring a contract change.** E9 means one
behaviour (mid-act injected failure) is verified on one adapter only. E4 means
one documented guarantee is not enforced in any production path. Both are
recorded as follow-ups; neither is a defect in the contract's *shape*.

*Validated by: contract tests; scenario execution; `git show --stat`
confirming no signature change.*

---

## 5. PEOS impact

**No PEOS change is required, and none is recommended.**

M.4 was the milestone most likely to expose a modelling limitation, because
mapping an ontology onto a relational schema is where impedance mismatches
normally surface. It did not.

- PEOS values needed no restructuring to persist. Every value serialises to
  canonical JSON and was stored byte-exact as `bytea`; symmetric
  `MarshalJSON`/`UnmarshalJSON` across the SDK made round-tripping
  uneventful.
- The adapter never needed to reach inside a PEOS value. Every field a query
  needs was already projected at the `internal/engineering/peos` boundary
  during M.3, so the persistence layer treats PEOS payloads as opaque — which
  is exactly what AD-005 intended.
- AD-014's entry-transition workaround was not re-encountered; it is confined
  to M.3's construction path and did not complicate persistence.
- No PEOS constructor, sentinel, or vocabulary value was found to be missing,
  ambiguous, or wrongly typed.

The one PEOS-related decision M.4 revisited — AD-017's two-place correction
validation — was *confirmed* by implementation, not challenged (§2.3).

No stylistic change is recommended, per the conservatism this review requires.
PEOS v1.0.0 is used unchanged and remains fit for purpose.

---

## 6. Readiness assessment

**Verdict: READY.**

| Dimension | Assessment |
|---|---|
| Architecture stability | No redesign required; no signature changed |
| Public API stability | Contracts recommended for freeze (§4) |
| Infrastructure maturity | Adequate for development; not production-tuned |
| Test strategy | Shared suite proven across two adapters; deliberate-violation discipline proven effective |
| Scenario coverage | Full canonical scenario on both adapters, including insertion-order independence |
| Contract coverage | 20 subtests × 2 adapters; one asymmetry (E9) |
| Failure modes | Rollback, panic, nested transaction, conflict, and serialization failure all covered; injected mid-act failure covered on one adapter only |

**Why not READY WITH CONDITIONS.** A condition should name something that
must be done *before* the next phase can safely proceed. Applying that test
honestly, none of the four open items qualifies:

- **E4** (digest verification) is a pre-existing gap that an HTTP layer neither
  worsens nor depends on.
- **E6** (silent retries) becomes actionable when there is an observability
  substrate — which M.5 supplies, not consumes.
- **E7** (pool churn) is test-only.
- **E9** (failure-injection asymmetry) narrows coverage but leaves rollback
  itself verified on both adapters.

Recording these as follow-ups is accurate. Elevating them to conditions would
be theatre.

---

## 7. Architecture confidence

Ratings derive from implementation evidence, not intuition. An even
distribution of "High" would indicate flattery rather than assessment.

| Area | Confidence | Justification | Evidence |
|---|---|---|---|
| Repository contracts | **High** | Two implementations sharing no code satisfy one suite | 20 subtests × 2 adapters |
| `UnitOfWork` | **High** | Sufficient for atomicity, nesting, and retry with no caller change | Contract tests; `TestNestedDoIsRejected` |
| Persistence independence | **High** | Domain unchanged; ports comment-only; no signature altered | `git show --stat`; architecture tests; deliberate violations |
| Migration strategy | **Medium** | Correct for one forward migration on an empty database; rollback and populated-database migration untested | `TestMigrateIsIdempotent`, `TestMigrateCreatesEveryTable` |
| Transaction model | **High** | Commit, rollback, panic, and nesting verified identically on both adapters | Contract tests (both adapters) |
| Retry strategy | **Medium** | Correct at four-writer contention; never load-tested; retries unobservable | `TestConcurrentRevisionsProduceDistinctSequences`; E6 |
| Performance characteristics | **Low** | Nothing measured. Two asymptotic differences known (E7, E8), neither quantified | No benchmark exists |
| Operational readiness | **Low** | No observability, no pool tuning, no failure-mode instrumentation | E6; absence of any logging in the adapter |

The two **Low** ratings are not defects — M.4 did not set out to produce a
production-tuned deployment, and FF-014 §10 defers both explicitly. They are
rated Low because *no evidence exists*, and this review does not convert
absence of evidence into confidence.

---

## 8. Lessons learned

Implementation-derived. Each traces to a specific episode.

1. **Contracts constrain data shape; they say nothing about control flow.**
   The repository interfaces were correct and complete, and three distinct
   defects (E1, E2, E3) lived entirely in *when* and *where* code ran. Design
   review cannot find these; execution can.

2. **An invariant enforced in one layer can make another layer's guarantee
   unreachable.** Adding a correction-target check to the repository would
   have made `ResolveCurrentClaim`'s totality guarantee untestable and
   therefore unmaintainable (E3). Defence in depth is not free when the layers
   have deliberately different tolerances.

3. **Error translation belongs where the decision is made, not where the error
   is raised.** Mapping in repositories destroyed the information `Do` needed
   to decide whether to retry (E2). The obvious placement was the wrong one.

4. **A second implementation is the only real test of an abstraction.** Every
   claim about persistence independence was speculative until a structurally
   different adapter was fitted to the same interfaces without changing them.

5. **A test that has never failed has not been verified.** Five of five new
   architecture tests were confirmed against deliberately injected violations
   before being trusted, and one of them found a real bug the same day (E12,
   E1).

6. **Specification and implementation drift silently in the direction of
   under-enforcement.** Three invariants the specs declared were never enforced
   (AD-021), and none had ever caused a visible failure. A second
   implementation is what surfaced them.

---

## 9. Architectural principles confirmed

These are no longer hypotheses.

### 9.1 Public contracts are validated only by multiple independent implementations

**Evidence.** 20 contract subtests pass against an in-memory adapter built on
Go maps and a PostgreSQL adapter built on SQL — sharing no code below
`application.Repositories`.

**Why implementation confirmed it.** Before M.4, "the domain is
persistence-independent" was an assertion supported by an import-graph test.
The import test proves the domain *cannot see* persistence; it cannot prove
the interface is an abstraction rather than a description of one
implementation. Only a second implementation distinguishes those.

**Status: Confirmed.**

### 9.2 Repository contracts constrain observable behaviour, not storage representation

**Evidence.** The same contract is satisfied by Go maps with a global mutex
and by a schema using foreign keys, `text[]` columns, a surrogate `bigserial`
key, nullable-as-absence columns, and snapshot isolation. `AcceptanceRecordIDIsUnique`
passes on both — enforced by a full scan in one and an index in the other.

**Why implementation confirmed it.** Every storage representation differs; every
observable behaviour matches. That is precisely what a behavioural contract
should permit.

**Status: Confirmed.**

### 9.3 Control-flow invariants rank equally with data invariants

**Evidence.** E1 (clock read inside a retried callback) and E2 (error mapping
placement). Neither is expressible as a constraint on data.

**Why implementation confirmed it.** Both were defects in otherwise correct
code that satisfied every data-level contract. `TestDoCallbacksAreRetrySafe`
demonstrates such invariants are mechanically enforceable — it is an AST scan,
not a convention.

**Status: Confirmed.**

### 9.4 Architecture verification through deliberate violations is effective protection

**Evidence.** Five of five new architecture tests were verified by introducing
a real violation, observing the specific failure, and reverting (E12).
`TestDoCallbacksAreRetrySafe` found a genuine pre-existing bug on introduction.

**Why implementation confirmed it.** An unverified assertion is
indistinguishable from a passing test that checks nothing. Deliberate
violation converts a test from an assumption into a measurement — and in one
case the measurement was immediately non-trivial.

**Status: Confirmed.**

### 9.5 Domain and application remain persistence-agnostic under infrastructure replacement

**Evidence.** `internal/domain`: zero changes. `ports.go`: comment-only.
`command_validation.go`: six lines, a bug fix rather than an accommodation.
`refkeys.go`: additive. No interface signature changed. `peos.Recorder` is
`struct{}` with value receivers — stateless, hence safe under callback replay
without modification.

**Why implementation confirmed it.** Replacing the entire persistence
substrate is the strongest available test of this property short of a third
adapter, and it required no upward change.

**Status: Confirmed.**

---

## 10. Open architectural questions

Conscious deferrals, not defects.

| Question | Why deferred | Why safe to defer | Expected phase |
|---|---|---|---|
| `RelationEnvelope` | No relation is recorded by the canonical scenario (AD-013) | Adding it later means a fourth envelope, not reshaping the existing three — M.4 confirmed the three map cleanly | Whenever a relation is genuinely recorded |
| Materialised projections | AD-006 requires measured evidence; none was produced | Nothing is materialised, so nothing can be stale. The relational adapter — where the temptation is strongest — produced no need | Only with measured evidence |
| Projection rebuilding | Vacuous while nothing is materialised | The FF-007 criterion is currently satisfied trivially; it becomes a real obligation only if materialisation arrives | Follows materialisation |
| Retry tuning | `maxAttempts = 8` and the backoff curve are unmeasured | Correct at tested contention; no production load exists to tune against | M.5+, with load |
| Pool tuning | No `MaxConns` override; defaults used | Adequate for one developer; E7 is the known amplifier | M.5+, with load |
| Observability | No substrate exists yet (E6) | Retries being silent matters only when someone is watching; M.5 introduces the layer where watching begins | M.5 |
| Query optimisation | No index beyond correctness | Deliberate under AD-006; scenario volumes are single-digit | Only with measured evidence |

---

## 11. Architecture freeze

The following areas are considered **frozen**: validated by implementation
evidence, and to be built upon rather than redesigned.

| Frozen area | Why implementation validated it | Why future phases build on it |
|---|---|---|
| **Repository contracts** | Two implementations sharing no code satisfy one 20-subtest suite | The interfaces are a proven abstraction, not a description of one store. A third adapter should fit them unchanged |
| **`UnitOfWork` abstraction** | Expressed atomicity, nesting rejection, and whole-callback retry with no caller change | Its shape is load-bearing: the callback signature is what forces both adapters to detect nesting identically |
| **`SubjectKey` representation** | Verified identically by a map lookup and by a `SELECT`, through one shared parser | Denormalising it was considered and rejected on evidence; reopening requires evidence that a query needs it |
| **Persistence abstraction** | Domain unchanged, ports comment-only, no signature altered | The boundary is architecture-test-enforced in both directions |
| **Migration strategy** | Idempotent, creates a complete schema from empty, no framework | Sufficient for forward migration; extend it rather than replace it |
| **Transaction semantics** | Commit/rollback/panic/nesting verified identically on both adapters | Callers depend on these guarantees today and they are adapter-independent |
| **Retry semantics** | Both concurrent writers succeed with distinct sequences; retry-safety is now test-enforced | The precondition (callbacks must be re-runnable) is enforced, not merely documented |
| **Architecture verification strategy** | Five of five new tests verified against real violations; one found a real bug | The discipline paid for itself within the milestone that introduced it |
| **Shared contract suite** | The single asset that makes "conforming adapter" a testable claim | Any future adapter is defined as one that passes it |

### On redesign

M.4 changed the epistemic status of these decisions. Before it, they were
well-argued designs supported by one implementation. After it, they are
supported by two implementations with fundamentally different concurrency
models, storage representations, and failure modes, verified by a shared suite
and by tests confirmed against deliberate violations.

**Architectural redesign is therefore no longer the default response to a
problem.** When a future phase encounters friction, the first obligation is to
solve it *within* the validated architecture — a new query, a new adapter
behind the existing contracts, an additional test, a documented deviation.
Redesign is the last resort, not the first instinct, because the cost of
reopening a validated decision is not the redesign itself but the loss of the
evidence that made it trustworthy.

Reopening any frozen decision requires evidence of comparable weight to what
M.4 produced: a working implementation that cannot satisfy the contract, a
contract test that cannot be made to pass, or a reproducible failure the
current design cannot express. An argument from taste, convention, or
anticipated future need does not meet that bar.

> Future FeatureForge phases are expected to build on these architectural
> decisions rather than revisit them. Any modification requires implementation
> evidence comparable to that produced during M.4.

---

## Appendix — Decision review (recommendations only)

Recommendations. Nothing here is applied; no Architecture Decision is
modified by this document.

### AD-010 — recommend documenting a verified discrepancy

AD-010's Consequences state:

> The link is verifiable, not merely referential: altered content no longer
> matches the digest in the immutable revision.

**The wording is supported, and this must be said plainly.** The digest is
genuinely bound into the immutable revision: `contentAddressedIntegrity`
produces a `core.IntegrityIdentity` with
`IntegrityMechanismContentAddressedReference` over
`IntegrityProtectedScopeContent`, alongside an authoritative content-addressed
`Representation` (`codec_artifact.go:136-172`). That payload is stored
byte-exact as `bytea` and is itself digest-protected. `VerifyContentDigest`
exists, works, and is exposed all the way up to the `EngineeringRecorder` port
(`application/recorder.go:25`).

**The discrepancy is between *verifiable* and *verified*.** No production code
path performs the comparison. `VerifyContentDigest`'s only call sites in the
entire module are two lines in `scenario_test.go` (:112, :138).
`StructuredContentRepository.Get` returns content untested against its
revision's digest in **both** adapters. Secondarily, the `content_digest`
column is a nullable, unvalidated *projection* — which does not destroy the
authoritative binding, since that lives inside the payload, but does mean the
projection can be absent without error (`envelope.go:81,97,133`;
`validatePayload` never inspects it).

So AD-010's guarantee is real and available, and is exercised exactly once, by
a test. Whether that is sufficient is a judgement this review does not make.

**Recommendation:** record the observation against AD-010. **Do not resolve it
here, do not modify AD-010, and do not adopt an implementation** — deciding
where verification belongs (repository read, query boundary, or an explicit
integrity-check operation) is itself an architecture decision and is out of
scope for a review.

### AD-017 — recommend no change; implementation confirmed it

The correction-graph episode (§2.3) actively validated AD-017's two-place
design. Worth recording as confirmation, since a future reader encountering
the "missing" repository-level check should find the reason it is absent.

### AD-006 — recommend no change; confirmed

Nothing needed materialising, including in the relational adapter. The open
question stays open with "none required so far" as its evidence-backed status.

### AD-013 — recommend no change; confirmed

The three-envelope split survived a relational mapping with no reshaping
(§1.1).

### AD-020, AD-021 — recommend no change

Both were written after the evidence they cite, during M.4 itself. They are
current by construction.

### Single-error-mapping-point rule — recommend no new AD

The rule discovered in E2 is already recorded in FF-014 §6 with its reasoning.
Promoting it to an AD would duplicate a specification statement without adding
governance value.
