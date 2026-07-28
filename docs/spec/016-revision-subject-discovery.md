# FF-016 — Revision Subject Discovery

Status: Proposed (Phase M.5, prerequisite change)
Governs: the `RevisionEnvelope.SubjectKey` projection, the
`RevisionEnvelopeRepository.ListByFamilyAndSubject` operation, their semantics
in both persistence adapters, and the implementation order for landing them.

Decision of record: **AD-025**.

## Governance statement

PEOS-000 through PEOS-009 remain authoritative for every PEOS concept.
`docs/spec/` governs FeatureForge. This document adds no PEOS concept, renames
none, and redefines none. PEOS v1.0.0 is used unchanged.

**Nothing in this document is implemented.** It specifies a change; §13 orders
the work. Until §13 completes, the capability described here does not exist.

## 1. Why this document exists

The M.5 contract investigation
([report](../reports/m5-contract-investigation.md)) established one structural
fact with implementation evidence:

> A record is discoverable by its subject. A revision is not discoverable at
> all unless the caller already knows its artifact identifier.

`RecordEnvelope` projects `SubjectKey`, and
`RecordEnvelopeRepository.ListByKindAndSubject` searches on it. `RevisionEnvelope`
projects no subject, and every revision-listing operation requires a known
artifact ID. Requirements and validation plans are revisions. Therefore no
caller can enumerate the requirements governing a capability without being told
their identifiers in advance.

In M.3 and M.4 the only caller was `internal/scenario`, which created every
record and held its identifiers as fixture constants. M.5 introduces a caller
that holds only a `FeatureCardID`.

**HTTP exposed this; HTTP did not cause it.** The information was never
projected. What changed is that a caller arrived who cannot supply it.

## 2. Scope

**In scope.** One optional projection on `RevisionEnvelope`; one repository
operation; consistent implementation in both adapters; contract-suite coverage
proving parity; the application-query integration that consumes it.

**Out of scope.** Everything else. Specifically not: `UnitOfWork`, transaction
semantics, the `SubjectKey` string representation itself (`refkeys.go` is
unchanged), repository architecture generally, the PEOS integration boundary,
AD-005, AD-006, any other envelope contract, and all general M.5 HTTP and UI
work.

## 3. The `RevisionEnvelope` subject contract

### 3.1 Canonical source

`RevisionEnvelope.SubjectKey` is a **projection**, written once by
`internal/engineering/peos` at record time from the same input the codec
already uses to construct the PEOS value. It is never computed by a reader,
never derived from other projections, and never recomputed after the fact.

It answers exactly one question:

> **Which capability Artifact is this revision about?**

The value is produced by `engineering.ArtifactSubjectKey(artifactID)` — the
existing helper, unchanged — yielding the canonical `artifact:<artifactID>`
form. `ArtifactRevisionSubjectKey` is not used by any revision family, because
no revision codec has revision-level subject data in scope.

### 3.2 Which families have a semantically defined subject

| Family | Subject defined? | Canonical source | PEOS provenance |
|---|---|---|---|
| `RevisionFamilyRequirement` | **yes** | `RequirementInput.SubjectArtifactID` | the requirement's Subject, an `EngineeringSubjectRef` at Artifact identity level (FF-011 §4.2) |
| `RevisionFamilyValidationPlan` | **yes** | `PlanInput.ScopeArtifactID` | the plan's `core.Scope` (FF-011 §4.4) |
| `RevisionFamilyTransitionRecord` | **yes** | `EntryAssignmentInput.SubjectArtifactID` / `TransitionInput.SubjectArtifactID` | the `LifecycleSubjectRef` (FF-011 §4.9) |
| `RevisionFamilyCapability` | **no** | — | the capability revision *is* the subject |
| `RevisionFamilyEvidence` | **no** | — | evidence is cited by records; it is about nothing |

**On the validation plan, stated plainly rather than glossed.** The plan's
source field is a PEOS `Scope`, not a PEOS `Subject`. FF-016 does not assert
that a Scope is a Subject. It asserts that the requirement's subject, the
plan's scope, and the transition record's lifecycle subject each answer §3.1's
single question — *which capability is this revision about* — with
family-specific PEOS provenance. The projection unifies the answer, not the
underlying PEOS construct.

### 3.3 Absence

An empty `SubjectKey` means **the family has no capability it is about**. It is
a positive statement of the model, not a missing value.

It does not mean "unknown", "not yet populated", or "failed to resolve". A
capability revision and an evidence revision are *correct* with an empty
subject; they would be incorrect with a populated one (§3.5).

### 3.4 Why optional rather than universally mandatory

Two of the five families have no subject to record. Making the field mandatory
would force those two to carry either a sentinel value or a self-reference,
both of which state something false: a capability revision is not *about* a
capability, it *is* one.

This mirrors `ContentDigest`, already optional on the same struct — evidence
revisions carry a zero digest because they have no FeatureForge-owned content
to digest. It differs from `RecordEnvelope.SubjectKey`, which is mandatory
because every record kind has a subject.

### 3.5 Supplying a subject for a family that has none is rejected

`NewRevisionEnvelope` returns `ErrInvalidEnvelope` if `SubjectKey` is non-empty
for `RevisionFamilyCapability` or `RevisionFamilyEvidence`.

The rejection is deliberate. A populated subject on those families would be a
codec defect, and a silently accepted one would make
`ListByFamilyAndSubject` return values whose membership nobody intended. The
check sits beside the existing closed-set family switch in the same
constructor.

### 3.6 Malformed subjects are rejected at write time

When `SubjectKey` is non-empty, `NewRevisionEnvelope` requires it to parse via
`engineering.ParseSubjectKey`, returning `ErrInvalidEnvelope` otherwise.

The reason is operational, not stylistic: a malformed projection does not fail
loudly, it makes `ListByFamilyAndSubject` return nothing — a silent empty
population, which is the exact failure mode this whole change exists to
prevent.

**Not checked at write time:** whether the referenced artifact exists. AD-021
decided that question for `RecordEnvelope` and gave it a repository-level check
in both adapters. Extending it to revisions is a separate decision requiring
its own evidence, and this document does not make it. The exclusion is
recorded so a later reader does not read it as an oversight.

### 3.7 The repository treats the value as opaque

After construction-time validation, the persistence layer never parses,
normalises, case-folds, or interprets `SubjectKey`. It is stored and compared
as an exact string. Both adapters treat it as an opaque canonical value.

This is what keeps `refkeys.go` the single definition of the key's shape
(AD-021) and keeps the adapters free of key semantics.

### 3.8 Compatibility with revisions written before the projection exists

A revision persisted before this change has no stored subject. On read it
yields an empty `SubjectKey`, which by §3.3 is indistinguishable from "this
family has no subject".

**Consequence, stated so it is not discovered later:** such a revision is
invisible to `ListByFamilyAndSubject` for every non-empty subject. A store
containing a mixed population — some revisions projected, some not — returns
*incomplete* populations without any error. §12 governs when this can occur;
for the current POC it cannot.

## 4. `ListByFamilyAndSubject`

### 4.1 Signature

Added to `RevisionEnvelopeRepository` in `internal/application/ports.go`:

```go
ListByFamilyAndSubject(ctx context.Context, family engineering.RevisionFamily, subjectKey string) ([]engineering.RevisionEnvelope, error)
```

It mirrors the validated
`RecordEnvelopeRepository.ListByKindAndSubject(ctx, kind, subjectKey)` in
shape, parameter order, and return convention, substituting revision
terminology. **No existing signature changes.**

### 4.2 Semantics

| Aspect | Rule |
|---|---|
| **Family matching** | Exact equality on `engineering.RevisionFamily`. No hierarchy, no wildcard. A family outside the closed set matches nothing and is not an error |
| **Subject equality** | Exact string equality on the canonical value. No parsing, no normalisation, no prefix matching |
| **Ordering** | Ascending by `RevisionKey.String()` — the `<artifactID>/<revisionID>` form — matching `ListByArtifact`. Never map iteration order (FF-009 §5) |
| **No match** | Empty slice, `nil` error. Absence is never `ErrNotFound`; the caller decides whether it matters |
| **Duplicates** | Impossible. `RevisionKey` is unique per revision — the PostgreSQL primary key and the memory map key both enforce it |
| **Empty `subjectKey`** | Returns an empty slice. It does **not** enumerate subject-less revisions. Specified so that "list every capability revision" cannot arise accidentally from a zero-valued argument |
| **Transaction visibility** | Participates in the ambient unit of work like every other operation. Memory sees overlay writes over committed state; PostgreSQL sees its own uncommitted writes. There is no non-transactional path |
| **Null-subject revisions** | Excluded from every non-empty query, by §3.3 and the equality rule above |
| **PEOS payloads** | **Never decoded.** The operation reads only the projected column or field. No adapter imports the PEOS SDK (AD-005) and this operation gives none a reason to |
| **Adapter parity** | Identical observable behaviour in both adapters, proven by the shared contract suite, not asserted |

### 4.3 What it is not

It is not a general relationship model, not a graph query, and not a
requirement index in any broader sense. It answers exactly one question, over
one projected field, with exact equality.

## 5. Relationship to FF-009 — a narrow extension

FF-009 §5's repository table currently enumerates:

| `RevisionEnvelopeRepository` | `Put`, `Get(RevisionKey)`, `ListByArtifact(artifactID)` |

**That single enumeration row is extended** by `ListByFamilyAndSubject`.

Precision matters here. FF-009 nowhere states in prose that requirement
revisions cannot be found from their capability; it *enumerates* an operation
set from which that absence follows. The inference is recorded in Go doc
comments on `EngineeringStateInput` and `TimelineInput` — "FF-009 §5 defines no
requirement-to-capability index" — and those comments become outdated when §13
step 9 lands.

**Superseded:** the implication, arising from that enumeration, that a
requirement or validation-plan revision cannot be discovered from the
capability it concerns without prior knowledge of its artifact identifier.

**Not superseded, and explicitly untouched:**

- FF-009's create-only, idempotency, and conflict semantics;
- its ordering rules and the prohibition on map iteration order;
- its transaction model (§6) and adapter semantics (§7);
- AD-006's decision that current state is computed and never materialised —
  this change materialises no derived answer;
- any notion of a broader derived model, a readiness projection, or arbitrary
  requirement-to-capability indexing. None is created.

FF-009 is **not edited**. This supersession is recorded here, in the document
that introduces the change, following the append-never-rewrite discipline
FF-014 §9 applied to the same situation.

## 6. Relationship to AD-005 and AD-006

**AD-005 is unaffected.** Only `internal/engineering/peos` produces the
projected value, from input it already holds. No other package gains any PEOS
capability, and the read path decodes nothing.

**AD-006 is unaffected.** `SubjectKey` is a projection of a fact already inside
the immutable payload, written once at record time — identical in kind to
`RecordEnvelope.SubjectKey`, which has existed since M.3. No current-state
answer, readiness verdict, or resolved revision is stored. Every derived query
remains computed on read.

## 7. Migration and backfill

### 7.1 Fresh database

The migration adds a nullable column and an index. New writes populate the
projection where §3.2 defines one and leave it NULL otherwise. No backfill is
required or performed.

### 7.2 Existing database with unprojected revision rows

An SQL migration can add the storage and the index. It **cannot** reconstruct
subjects: the information lives inside each revision's PEOS payload, and
reconstructing it requires PEOS decoding.

**SQL must not decode PEOS payloads.** This is not merely a rule; it is already
enforced. `TestNoUpdateOrDeleteOnEngineeringTables`
(`internal/architecture/architecture_test.go`) scans every `.sql` and `.go`
file under the PostgreSQL adapter for the lowercased substring
`update revision_envelopes`. A backfill migration containing
`UPDATE revision_envelopes SET subject_key = …` **fails the build**.
`ALTER TABLE revision_envelopes ADD COLUMN …` is not matched, so the additive
migration is permitted while the backfill is structurally prohibited.

Any future reconstruction must therefore be a **one-off tool inside
`internal/engineering/peos`**, the only package permitted to decode a payload,
using the canonical decoders that package already exposes
(`DecodeRequirementRevision`, `DecodePlanRevision`,
`DecodeTransitionRecordRevision`).

**This document does not design or implement that tool.**

### 7.3 Mixed populations

If a store ever contains both projected and unprojected revisions,
`ListByFamilyAndSubject` returns only the projected ones, silently and without
error (§3.8). Any future backfill work must treat that as its acceptance
criterion.

### 7.4 The empty-database assumption, verified and scoped

For the current POC, backfill is unnecessary, and the assumption is verifiable
rather than asserted:

- No FeatureForge deployment exists; FF-000 states the project is a temporary
  reference consumer, and no phase before M.7 deploys it.
- The only PostgreSQL instance is the test container, whose data directory is
  mounted `tmpfs` in `docker-compose.test.yml` — every `up` starts empty and no
  state survives `down`.
- Every integration test creates a uniquely named schema, migrates it, and
  drops it (`harness_test.go`).

**Scope of the assumption:** it holds for the POC as currently constituted. It
is not a general claim, and it expires the moment any durable FeatureForge
database exists.

## 8. Architecture Freeze exception

This change reopens exactly one thing, on evidence, and the exception is
bounded in §9 of AD-025. The evidence justifies revision subject projection and
discovery, and nothing else.

## 9–12. *(reserved — see AD-025 for context, decision, rejected alternatives, and consequences)*

The decision record carries the full argument. This document carries the
contract and the work.

## 13. Implementation plan

Twelve steps, in order. No general M.5 HTTP or UI work appears here; that
resumes only after step 12.

### Step 1 — Envelope contract and constructor semantics

**Affects.** `internal/engineering/envelope.go`,
`internal/engineering/envelope_test.go`.

**Behaviour.** `SubjectKey string` added to `RevisionEnvelope` and
`RevisionEnvelopeInput`, positioned beside the other projected fields.
`NewRevisionEnvelope` gains two validations: reject non-empty `SubjectKey` for
`RevisionFamilyCapability`/`RevisionFamilyEvidence` (§3.5); require
`ParseSubjectKey` to succeed when non-empty (§3.6). `RevisionEnvelope.Equal`
is unchanged — it compares key and payload only, and a projection is not
identity.

**Tests.** Constructor accepts empty for all five families; accepts a valid
subject for the three that define one; rejects a subject on capability and
evidence; rejects a malformed subject; round-trips the value.

**Done when.** `go test ./internal/engineering/...` passes and no other package
compiles differently.

### Step 2 — Repository interface addition

**Affects.** `internal/application/ports.go`.

**Behaviour.** One method added to `RevisionEnvelopeRepository` (§4.1), with a
doc comment stating ordering, empty-subject behaviour, and that null-subject
revisions are excluded. No existing signature changes.

**Tests.** None directly; the build breaking until both adapters implement it
is the intended signal.

**Done when.** The interface compiles and both adapters fail to compile,
proving no implementation is silently missing.

### Step 3 — Memory adapter

**Affects.** `internal/infrastructure/memory/repositories.go`.

**Behaviour.** Direct scan over committed and overlay revision maps, filtering
on family and exact subject equality, sorted by `Key.String()`. It does **not**
delegate the way `recordRepo.ListByKindAndSubject` delegates to `ListByKind`,
because no `ListByFamily` exists — and none is added speculatively.

**Tests.** Covered by the shared suite in step 6.

**Done when.** `go test ./internal/infrastructure/memory/...` passes.

### Step 4 — PostgreSQL migration

**Affects.** `internal/infrastructure/postgres/migrations/0002_revision_subject_key.sql`.

**Behaviour.**

```sql
ALTER TABLE revision_envelopes ADD COLUMN subject_key text NULL;
CREATE INDEX revision_envelopes_family_subject_idx
    ON revision_envelopes(revision_family, subject_key);
```

Additive only. No `UPDATE`, no backfill (§7.2). The index mirrors
`record_envelopes_kind_subject_idx` and follows the `<table>_<columns>_idx`
convention.

**Tests.** `TestMigrateIsIdempotent` still passes;
`TestMigrateCreatesEveryTable` extended to assert the column exists.

**Done when.** Migration applies to an empty database and re-applies as a
no-op.

### Step 5 — PostgreSQL adapter

**Affects.** `internal/infrastructure/postgres/repositories.go`.

**Behaviour.** `subject_key` added to the write path via the existing
`nullableString` idiom and to `scanRevision` via `stringOrEmpty`. The
thrice-inlined revision SELECT list is extracted into a `revisionSelect`
constant, mirroring the record side's existing `recordSelect`, rather than
adding a fourth copy. `ListByFamilyAndSubject` issues
`… WHERE revision_family = $1 AND subject_key = $2 ORDER BY artifact_id, revision_id`.

**Tests.** Shared suite (step 6) plus step 7.

**Done when.** `make postgres-test` passes.

### Step 6 — Shared contract-suite additions

**Affects.** `internal/infrastructure/contracttest/contract.go`.

**Behaviour.** Three subtests, registered in the existing style and run
unchanged by both adapters:

- `RevisionListByFamilyAndSubject` — writes revisions across two families and
  two subjects; asserts exact membership, ascending order, empty-slice-not-error
  on no match, and that a subject-less revision never appears.
- `RevisionSubjectKeyIsOptional` — a capability revision with no subject
  round-trips and is excluded from subject queries.
- `RevisionRejectsMalformedSubjectKey` — construction fails, and the failure is
  identical on both adapters.

**Tests.** These *are* the tests. Both adapters must pass them without
adapter-specific weakening.

**Done when.** The suite passes on memory, and on PostgreSQL under
`make postgres-test`.

### Step 7 — PostgreSQL-specific tests

**Affects.** `internal/infrastructure/postgres/projection_test.go`.

**Behaviour.** The new column round-trips; SQL `NULL` reads back as empty
string; the projected `subject_key` agrees with what decoding the stored
payload yields — extending the existing typed-column-projection pattern to the
new column.

**Done when.** `make postgres-test` passes.

### Step 8 — Architecture-test updates

**Affects.** `internal/architecture/architecture_test.go` — expected to be
**no change**.

**Behaviour.** Confirm, do not assume: `TestNoDerivedStateOnFeatureCard` is
scoped to `internal/domain/featurecard.go`; `TestNoUpdateOrDeleteMethodExists`
forbids `Update`/`Delete`/`Remove`/`Set` prefixes and `ListByFamilyAndSubject`
begins with `List`; `TestNoPEOSTypeIsCopied` requires a full suspicious field
set that adding one field does not complete; `TestNoShadowStructNames` governs
type names, not fields. Verify `TestNoUpdateOrDeleteOnEngineeringTables` still
passes with the new migration present.

**Done when.** The architecture suite passes unchanged. If any test does need
editing, that is evidence the change is larger than specified and step 8 stops
for review.

### Step 9 — Application query integration

**Affects.** `internal/application/query_state.go`,
`internal/application/query_timeline.go`.

**Behaviour.** This step makes the capability *usable* and is the point at
which the M.5 blocker clears. Requirement and validation-plan populations
become discoverable from the capability artifact ID. The now-outdated doc
comments citing "FF-009 §5 defines no requirement-to-capability index" are
corrected.

The exact shape — whether the existing input structs gain optional discovery or
a new composed query is added — is deliberately left to implementation, with
one binding constraint: **discovery must not silently return an incomplete
population.** Decisions, executions, claims, and evidence remain discoverable
by the existing means the contract investigation identified; no change is made
for them.

**Tests.** Step 10.

**Done when.** A caller holding only a capability artifact ID can obtain the
complete requirement population.

### Step 10 — Canonical FF-011 readiness proof, including uncovered `REQ-4`

**Affects.** `internal/scenario/scenario_test.go` (and its PostgreSQL sibling).

**Behaviour.** The step that proves the change solves the stated problem. After
running the canonical scenario:

1. Discover requirements for `CAP-1` via `ListByFamilyAndSubject`.
2. Assert the population is **exactly** `REQ-1`, `REQ-2`, `REQ-3`, `REQ-4` —
   including `REQ-4`, which has no plan activity and no claim.
3. Feed the *discovered* population to `ResolveReadiness`.
4. Assert `not-ready`, and assert `REQ-4` is reported with no applicable claim.

Step 2 is the assertion that fails under the rejected claim-derived approach.
It must run on **both** adapters.

**Done when.** It passes on memory and PostgreSQL, and deleting `REQ-4` from
the expected set makes it fail.

### Step 11 — Documentation consistency verification

**Affects.** FF-016 (amended to as-built), FF-015 §6.2 and §17, AD-025 if
implementation revealed anything the decision misstated.

**Behaviour.** Verify AD-025, FF-015, FF-016, FF-009, FF-014, and the M.5
contract investigation agree. FF-009 is not edited.

**Done when.** No document contradicts another, and FF-015 no longer describes
the prerequisite as pending.

### Step 12 — Verification and atomic commit boundary

**Behaviour.** `gofmt -l .`, `go vet ./...`, `go build ./...`,
`go test ./... -count=1`, `go test ./... -race -count=1`,
`make postgres-test`, `go mod verify`, `git diff --check`, `git status`.

**One commit.** The change is atomic: the field is inert without the operation,
the operation is unimplementable without the field, and both are meaningless
without adapter parity. Intermediate commits would build while proving nothing,
which is the condition FF-013's commit policy exists to avoid.

**Done when.** Every gate passes and the working tree is clean.

## 14. What this document deliberately does not decide

- **Whether revision subject references should be existence-verified at write
  time.** §3.6 excludes it; AD-021 is the precedent that would govern it.
- **The backfill tool's design.** §7.2 states where it would live and what it
  may use. Nothing more.
- **Any broader relationship or traceability model.** `RelationEnvelope`
  remains deferred (AD-013).
- **How step 9 shapes the application query surface.** Constrained, not
  specified.
