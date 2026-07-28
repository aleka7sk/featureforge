# FeatureForge Decision Log

This log records material architecture decisions. A decision belongs here when
reversing it would change the domain boundary, the persistence authority, the
semantics of "current", the correction model, or what FeatureForge is allowed to
become.

Decisions are second in the source-of-truth priority, after `docs/spec/` and
before tests and implementation. Where a decision and a specification disagree,
the specification governs and the decision must be corrected or withdrawn.

## Rules

- Decisions are not made silently. An implementation phase that finds itself
  choosing between architectures stops and records the decision first.
- A decision is never edited to say something different. It is superseded by a
  new decision that names it. The old text stays. This is the same discipline
  FeatureForge applies to engineering records.
- Every decision states what was rejected and why. A decision with no rejected
  alternative was not a decision.

## Format

```
## AD-nnn — Title

Status: Accepted | Superseded by AD-mmm | Withdrawn
Date: YYYY-MM-DD
Phase: M.n

Context     — what forced a choice
Decision    — what was chosen
Alternatives — what was rejected, and why
Consequences — what this makes easy, and what it costs
```

---

## AD-001 — Operational entities are limited to Project and FeatureCard

Status: Accepted
Date: 2026-07-28
Phase: M.1

**Context.** The candidate operational entity list was Workspace, Project,
FeatureCard, User, Comment, AttachmentMetadata. Every entity kept is a place the
operational Belcanto domain can leak into FeatureForge.

**Decision.** Two operational entities: Project and FeatureCard. Actor identity is
a single fixed configuration value, not a stored entity.

**Alternatives.** Workspace rejected — it exists for tenancy, which is out of
scope for every phase. User rejected as an entity — one local user needs no
record. Comment rejected — collaboration is out of scope, and a remark with
engineering meaning belongs in a Decision rationale or a revision's open
questions, where it is immutable. AttachmentMetadata rejected — the scenario's
audio attachment is *content*, and real cited artifacts are already modelled by
PEOS Evidence with a Representation.

**Consequences.** The scenario is fully expressible. Adding an entity is now a
visible decision rather than a quiet commit. See
[FF-002 §2](../spec/002-domain-boundaries.md#2-operational-entity-list-challenged).

---

## AD-002 — The capability specification is a plain PEOS Artifact with a FeatureForge Artifact Type

Status: Accepted
Date: 2026-07-28
Phase: M.1

**Context.** PEOS defines Artifact Types for Requirement, Validation Plan,
Decision Record, Transition Record, and Quality Profile. None of them is a
product capability specification.

**Decision.** The capability specification is a `core.Artifact` whose Artifact
Type is `featureforge:product-capability`, with ordinary `core.ArtifactRevision`
revisions.

**Alternatives.** Modelling it as a `requirement.Requirement` rejected — that
type requires the PEOS Requirement Artifact Type and would conflate a
specification with a requirement. Asking for a new PEOS Artifact Type rejected —
PEOS is not modified from here, and Artifact Type is an open vocabulary precisely
so products can do this.

**Consequences.** FeatureForge owns the type name, and requirements remain a
distinct PEOS construct that the specification is validated against. See
[FF-003 §2](../spec/003-peos-integration.md#2-concept-by-concept-mapping).

---

## AD-003 — Revision ordering is a product-owned dense integer sequence, linear only

Status: Accepted
Date: 2026-07-28
Phase: M.1

**Context.** PEOS-002 refuses to mandate an ordering mechanism and forbids
assuming Revision Identifiers are sortable. Without an explicit contract, an
implementation would silently order by insertion or timestamp.

**Decision.** Every revision carries a FeatureForge-owned integer sequence,
unique within its Artifact, starting at 1, dense, assigned transactionally. The
current revision is the accepted revision with the greatest sequence. Insertion
order, timestamps, and revision-ID order are never consulted. Duplicate or
ambiguous sequence is an explicit error. Branching is rejected for the POC.

**Alternatives.** Ordering by provenance timestamp rejected — clock skew and
equal timestamps make it non-total, and backdating a record would silently
reorder history. Ordering by revision ID rejected — PEOS-002 explicitly forbids
assuming sortability. Predecessor links rejected — they express the same
information as a sequence but make "greatest" a graph traversal with cycle risk.
Branching rejected — it requires a merge and precedence policy the scenario never
exercises.

**Challenged against PEOS-002.** Sequence numbers are the first mechanism
PEOS-002 lists. Branching is *permitted*, not required, so declaring linear
history narrows what PEOS allows rather than contradicting it. The sequence lives
in FeatureForge storage because an Artifact Revision carries no revision number.
No conflict found.

**Consequences.** Resolution is deterministic and testable by inserting the same
records in several orders. Branching, if ever needed, is an additive extension
that invalidates no stored record. See
[FF-004 §2](../spec/004-current-state-resolution.md#2-the-revision-ordering-contract).

---

## AD-004 — Revision acceptance and PEOS lifecycle state are separate concerns

Status: Accepted
Date: 2026-07-28
Phase: M.1

**Context.** "Which revision is authoritative" and "how far has this capability
progressed" are both state questions, and merging them is tempting.

**Decision.** They stay separate. Revision acceptance (`draft` / `accepted` /
`withdrawn`) is FeatureForge-owned, journalled, and applies per revision. The
PEOS-003 lifecycle applies to the capability Artifact as a whole, with one
Definition Version and four states.

**Alternatives.** Using PEOS-003 for revision acceptance rejected — it would
require a Transition Record Revision and a State Assignment per revision
acceptance, which is heavy for a boolean-shaped question, and PEOS-002 keeps
status off the revision deliberately. Dropping PEOS lifecycle entirely rejected —
the timeline and the current-state queries both require it, and it is one of the
PEOS constructs the POC exists to exercise.

**Consequences.** Two state mechanisms, each answering a different question, each
documented. The cost is that a reader must not confuse them, which is why both
specs state the distinction. See
[FF-004 §2](../spec/004-current-state-resolution.md#acceptance-state).

---

## AD-005 — Only the integration layer imports PEOS; adapters store opaque payloads

Status: Accepted
Date: 2026-07-28
Phase: M.1

**Context.** The domain must not import PEOS. But persisting a PEOS value
requires marshalling it, and marshalling naturally lives in the persistence
adapter — which would make the adapter a second PEOS importer.

**Decision.** `internal/engineering/peos` is the only package importing the PEOS
SDK. It serializes PEOS values to canonical JSON, and repository contracts are
expressed over a FeatureForge-owned envelope of identity, record kind, and opaque
payload. The persistence adapter imports no PEOS package.

**Alternatives.** Letting the adapter import PEOS rejected — it makes "who may
import PEOS" a judgement call rather than a single testable rule, and it couples
the PostgreSQL work to PEOS types. Passing PEOS values through the application
layer to the adapter rejected — it violates the domain rule transitively.

**Consequences.** The architecture test is a one-line assertion. M.4 is about
storage semantics rather than PEOS types. The cost is one serialization hop and
an envelope type. See
[FF-002 §4](../spec/002-domain-boundaries.md#4-package-layering).

---

## AD-006 — Current state is computed, never materialized

Status: Accepted
Date: 2026-07-28
Phase: M.1

**Context.** Current-state answers can be computed on read or maintained as
projections written alongside each act.

**Decision.** All current-state queries are computed. No materialized projection
exists in the POC. Typed columns exist for indexing only and are never
authoritative.

**Alternatives.** Materialized projections rejected for now — a projection that
disagrees with the records is precisely the bug this project exists to catch, and
the scenario has single-digit record counts. A hybrid rejected — it has both
costs and no measured benefit yet.

**Consequences.** Answers cannot go stale. Introducing materialization later
requires measured evidence and a superseding decision. See
[FF-004 §3](../spec/004-current-state-resolution.md#3-current-state-queries).

---

## AD-007 — The timeline is a FeatureForge computed read model with derived event identity

Status: Accepted
Date: 2026-07-28
Phase: M.1

**Context.** A timeline could be an event log written as things happen, or a view
computed from the records.

**Decision.** Computed on read. Event identity is derived as
`source_kind ":" source_identity`. Ordering is
`(occurred_at, kind_rank, source_identity)` — total, so output is deterministic.
Undated events go to a separate flagged group, never silently placed at epoch or
last.

**Alternatives.** A written event log rejected — it is a second source of truth
that can drift from the records, and it invites events with no record behind
them. Ordering by timestamp alone rejected — not total. Ordering by insertion
rejected — it is exactly what [FF-004](../spec/004-current-state-resolution.md)
forbids everywhere else.

**Consequences.** The timeline cannot lie about what is stored, and it can be
deleted and recomputed. See [FF-006](../spec/006-timeline-read-model.md).

---

## AD-008 — Release readiness is a computed query, not a Claim

Status: Accepted
Date: 2026-07-28
Phase: M.1

**Context.** `featureforge:release-readiness` was proposed as a vocabulary value.
It could be a `core.ProductRuleRef` used as a Claim criterion, making readiness an
assertable Claim.

**Decision.** Release readiness is a computed query with a per-requirement
rationale. It is product-only terminology and is not represented as any PEOS
value.

**Alternatives.** Recording readiness as a conformance Claim rejected — readiness
is derived from requirement coverage and current claims, and recording a derived
verdict as authoritative engineering state is what the PEOS consumer guide
explicitly warns against. It would also go stale the moment a claim was
corrected, which the canonical scenario does on purpose.

**Consequences.** Readiness always reflects the current records. If a human ever
needs to *assert* readiness as a governance act, that is a PEOS-004 Decision, not
a recomputed flag. See
[FF-004 §3.6](../spec/004-current-state-resolution.md#36-release-readiness).

---

## AD-009 — `featureforge:requirement-satisfaction` and `featureforge:feature-specification` are rejected

Status: Accepted
Date: 2026-07-28
Phase: M.1

**Context.** Both were proposed for the FeatureForge vocabulary.

**Decision.** Both rejected. `featureforge:requirement-satisfaction` would be a
Claim Type, and `core.ClaimType` is the one PEOS vocabulary family treated as
closed — `core.ClaimTypeSatisfaction` already means exactly this, and declaring a
product value would extend the PEOS ontology. `featureforge:feature-specification`
is a synonym for `featureforge:product-capability`; two names for one Artifact
Type guarantees divergent usage.

**Consequences.** The vocabulary is smaller and has no synonyms. See
[FF-003 §6](../spec/003-peos-integration.md#6-featureforge-vocabulary).

---

## AD-010 — Specification content is FeatureForge-owned, digest-bound to the PEOS revision, and never in an Extension

Status: Accepted
Date: 2026-07-28
Phase: M.1

**Context.** A capability revision needs structured product content that
`core.ArtifactRevision` has no field for.

**Decision.** Content is a FeatureForge-owned insert-only record keyed by the
exact `(ArtifactID, ArtifactRevisionID)` pair. Its canonical JSON is hashed with
SHA-256, and that digest is recorded as the revision's `IntegrityIdentity` using
a content-addressed mechanism over the content scope. The revision also carries
one authoritative Representation built from the same content address with media
type `featureforge:specification-content`. `core.Extension` is not used at all in
the POC.

**Alternatives.** `core.Extension` rejected — an opaque, unvalidated, unqueryable
JSON container, and the project was explicitly told not to hide product content
there. An inline Representation as the primary store rejected — reading a title
would require decoding the whole revision, and no typed projection is possible.

**Consequences.** The link is verifiable, not merely referential: altered content
no longer matches the digest in the immutable revision. The cost is a canonical
serialization rule that M.2 must specify precisely. See
[FF-003 §5](../spec/003-peos-integration.md#5-revision-content-ownership).

---

## AD-011 — "Result" is not a construct; correction is a distinct use case

Status: Accepted
Date: 2026-07-28
Phase: M.1

**Context.** The lifecycle names "produce a result and claim", which reads like
two constructs. PEOS-006 states there is no separate Verdict entity.

**Decision.** No `Result`, `Verdict`, or `Status` type is created. "Result" maps
to `ExecutionRecord.Outcome()` for how the activity concluded and
`Claim.Outcome()` for what was determined. `CorrectClaim` remains a use case
separate from `RecordClaim`.

**Alternatives.** A FeatureForge `Result` type rejected — it would duplicate two
PEOS vocabularies and become a derived field. Folding correction into
`RecordClaim` as an optional parameter rejected — correction has invariants
`RecordClaim` does not (target exists, is a claim, no cycle, valid kind), and
hiding them in an optional field is how they stop being checked.

**Consequences.** Two outcome vocabularies stay distinct, and an interrupted or
indeterminate execution can never be silently read as completed. See
[FF-003 §3](../spec/003-peos-integration.md#3-result-is-not-a-new-construct--resolved).

---

## AD-012 — AI proposals have no write path, and no shared PEOS library is extracted

Status: Accepted
Date: 2026-07-28
Phase: M.1

**Context.** Two ways this project could quietly exceed its scope: an AI
component that writes engineering state, and a "reusable PEOS integration
library" extracted for Belcanto's benefit.

**Decision.** The AI component receives a derived context pack and returns a
proposal. It is never given a repository, a transaction, or a use case. Only
explicit human acceptance creates a revision, and that revision's provenance
records the human actor and an AI-assisted method. Separately, no shared PEOS
integration package is created during FeatureForge; the integration layer stays
inside FeatureForge, and Belcanto writes its own.

**Alternatives.** Letting the AI create draft revisions directly rejected — a
draft revision is still recorded engineering state with provenance, and
`draft` acceptance is not a sandbox. Extracting a shared library rejected —
generalizing from one consumer produces the wrong abstraction, and the handover
is patterns and rationale, not code.

**Consequences.** The AI boundary is testable by asserting the proposal path has
no write access. Belcanto starts from documented lessons rather than inherited
code. See [FF-001 §4](../spec/001-poc-acceptance-contract.md#4-ai-boundary) and
[FF-001 §6.7](../spec/001-poc-acceptance-contract.md#67-transition-to-belcanto).

---

## AD-013 — Three envelope types, not one universal envelope; no RelationEnvelope in M.3

Status: Accepted
Date: 2026-07-28
Phase: M.2

**Context.** AD-005 requires adapters to persist PEOS values without importing
PEOS, which needs a FeatureForge-owned envelope. The shape was open.

**Decision.** Three envelopes — `ArtifactEnvelope`, `RevisionEnvelope`,
`RecordEnvelope` — plus `CapabilitySpecificationContent`, `RevisionOrderMetadata`,
and `RevisionAcceptanceRecord`. No `RelationEnvelope` in M.3.

**Alternatives.** One universal envelope with a closed `RecordKind` rejected —
artifacts, revisions, and records have different identity and lookup semantics,
so one envelope forces the widest key shape on all three and a repository can no
longer state its uniqueness constraint in its own signature. A four-envelope set
including `RelationEnvelope` rejected — M.1 established that `relation.Relation`
is unused in M.3, and relations have no normative PEOS identity, so they need a
composite `(type, from, to, scope)` key that `RecordEnvelope` cannot express.
Adding it now would be a placeholder for a construct the scenario never creates.

**Consequences.** Each repository states its own key and conflict rule, which is
exactly what M.4 needs to write SQL. Adding relations later means adding a fourth
envelope, not reshaping three. See
[FF-009 §2](../spec/009-in-memory-persistence.md#2-one-envelope-or-several).

---

## AD-014 — The lifecycle entry assignment is established by a content-free Transition Record Revision

Status: Accepted
Date: 2026-07-28
Phase: M.2

**Context.** PEOS-003 states a Subject "enters a lifecycle through an entry
Transition **from an unassigned condition**". But `lifecycle.NewTransitionRecordContent`
takes a mandatory `fromAssignment` and, verified against v1.0.0, **rejects a zero
one** with *"source state assignment must not be zero"*. The SDK therefore cannot
express an entry Transition Record: the first assignment needs a source
assignment that by definition does not exist.

**Decision.** The entry State Assignment is established by a plain
`core.ArtifactRevision` of the Transition Record Artifact carrying **no**
`TransitionRecordContent`. Every later transition uses a full
`TransitionRecordRevision` whose `fromAssignment` is the previous assignment.

**Alternatives.** Fabricating a synthetic "unassigned" source assignment rejected
— it would invent a state occupancy that never happened, and PEOS-003 says a
Subject must not be treated as occupying a State merely because it exists.
Dropping lifecycle from M.3 rejected — M.1 requires lifecycle state resolution
and a lifecycle timeline event. Modifying PEOS rejected absolutely.

**Verified support.** `lifecycle.NewStateAssignment` accepts `establishedBy` as a
bare `core.ArtifactRevisionRef` and documents that it performs no lookup. The
PEOS lifecycle example itself relies on this, citing `TR-9001/REV-0` as the
establishing revision of its source assignment without constructing it.

**Consequences.** Conformant — PEOS-002 permits an ordinary Artifact Revision of
a Transition Record Artifact, and PEOS-003's requirement that initial assignment
be "recorded by its Transition Record" is met by a revision of that record. **No
PEOS change is made or required.** The gap is reported to the PEOS backlog as an
M.7 consumer finding. `TestTransitionContentRejectsZeroFromAssignment` pins the
limitation, so a future SDK version that lifts it makes this decision revisitable
rather than invisible. See
[FF-010 §8](../spec/010-application-contracts.md#the-entry-transition-problem-and-its-resolution).

---

## AD-015 — Acceptance is an append-only journal; there is no stored acceptance field

Status: Accepted
Date: 2026-07-28
Phase: M.2

**Context.** AD-004 separated revision acceptance from lifecycle state. M.2 had
to choose the representation, with three candidates on the table.

**Decision.** `RevisionOrderMetadata` carries the sequence **only**. Acceptance is
an append-only `RevisionAcceptanceRecord` journal; a revision's state is the
state of its latest entry ordered by `(EffectiveAt, RecordID)`. A revision with
no entry is `draft` by absence.

**Alternatives.** An `Accepted` boolean on order metadata rejected — order
metadata is insert-only, so a mutable flag on it contradicts that, and a boolean
cannot carry the actor, time, and reason the timeline requires, so a journal
would exist anyway and the flag would duplicate its head. PEOS lifecycle State
Assignment as acceptance rejected on three grounds: *cardinality* — the lifecycle
subject is the capability Artifact, so there is one lifecycle state per
capability while acceptance is per revision; *cost* — every assignment needs an
establishing Transition Record Revision, so accepting a revision would cost three
extra PEOS values; *meaning* — "under-validation" says nothing about which
revision text is authoritative.

**Consequences.** One truth in one place. This confirms AD-004 rather than
revising it, and makes it precise. See
[FF-010 §5](../spec/010-application-contracts.md#5-acceptance-semantics--fully-resolved).

---

## AD-016 — Release readiness has four statuses; precedence is not-ready first

Status: Accepted
Date: 2026-07-28
Phase: M.2
Supersedes: the status list and precedence in FF-004 §3.6

**Context.** M.1 listed five readiness statuses — `ready`, `not-ready`,
`incomplete`, `inconclusive`, `undetermined` — with precedence
`not-ready > inconclusive > incomplete > ready`. The M.2 brief proposed four
statuses with `indeterminate` first.

**Decision.** Four statuses: `ready`, `not-ready`, `indeterminate`, `incomplete`.
Precedence: **`not-ready` > `indeterminate` > `incomplete` > `ready`**.
`inconclusive` and `undetermined` merge into `indeterminate`; "no effective
requirements" becomes `incomplete`; "current revision unresolvable" becomes an
**error**, not a status.

**Alternatives.** `indeterminate` first, as the brief proposed, rejected for two
reasons. *First*, structural failures — digest mismatch, ambiguous revision,
dangling reference, correction cycle — are errors, not statuses, so
`indeterminate` covers only semantic indeterminacy where the computation
succeeded and the answer is genuinely unclear. *Second*, given that narrowing, a
definitive `not-satisfied` is stronger information than an inconclusive: if R2 is
proven unsatisfied while R3 is inconclusive, the capability is not ready, and
reporting `indeterminate` would mask a certain negative behind an uncertain one.
Keeping M.1's five statuses rejected — `undetermined` and `inconclusive` were
never distinguishable in practice.

**Consequences.** A negative signal is never masked by a weaker one. FF-004 §3.6
is corrected to match. See
[FF-010 §7](../spec/010-application-contracts.md#7-release-readiness).

---

## AD-017 — FeatureForge rejects self-correction, because PEOS v1.0.0 does not

Status: Accepted
Date: 2026-07-28
Phase: M.2

**Context.** The correction model assumes a claim cannot correct itself. Verified
against v1.0.0: `Claim.WithCorrection` **accepts** a correction reference whose
target is the claim's own identity, returning no error.

**Decision.** `CorrectValidationClaim` rejects a correction whose target equals
the new claim's identity, with `ErrCorrectionSelfReference`, before any PEOS
construction. The correction-chain query also rejects it on read, so a payload
written by any other means cannot poison resolution.

**Alternatives.** Relying on the SDK rejected — it does not. Detecting it only as
a cycle at read time rejected — the write should fail at the boundary where the
user can still fix it, and a self-loop is a distinguishable mistake from a
multi-node cycle.

**Consequences.** Validation happens in both places, which is deliberate: write-side
for the actionable error, read-side so resolution is total over any stored graph.
See [FF-009 §8](../spec/009-in-memory-persistence.md#correction).

---

## AD-018 — The lifecycle state `validated` is renamed `assessed` and redefined

Status: Accepted
Date: 2026-07-28
Phase: M.2
Supersedes: the lifecycle state set in FF-003 §4

**Context.** M.1 defined four lifecycle states ending in `featureforge:validated`,
meaning "a satisfied claim stands". That is release readiness expressed as a
lifecycle state — the exact duplication that lifecycle and readiness were
separated to avoid.

**Decision.** The state set is `drafting`, `specified`, `under-validation`,
`assessed`. `assessed` means validation has been executed and assessed; it says
nothing about the outcome. **A capability can be `assessed` and still
`not-ready`**, and a test asserts that pair.

**Alternatives.** Keeping `validated` rejected — its definition made lifecycle a
second, staler copy of readiness, so the two would inevitably disagree. Dropping
lifecycle rejected — M.1 requires it. Deriving lifecycle state from claims
rejected — that would make it a derived view stored as authoritative engineering
state, which AD-006 and PEOS-006 both forbid.

**Consequences.** Lifecycle answers "how far has this progressed"; readiness
answers "do the requirements hold". FF-003 §4 is corrected to match. See
[FF-010 §8](../spec/010-application-contracts.md#8-lifecycle-state).

---

## AD-019 — `EstablishRequirement` also writes revision-order metadata and an immediate acceptance entry

Status: Accepted
Date: 2026-07-28
Phase: M.3
Supersedes: nothing; closes a gap between FF-004 §3.2 and FF-010 §3

**Context.** FF-004 §2 states every specification revision has a FeatureForge
sequence and an acceptance state, and §3.2 states requirement revisions
"follow the same ordering contract" as capability revisions —
`ResolveEffectiveRequirements` calls the identical `ResolveCurrentRevision`
used for capabilities. But FF-010 §3's command table lists
`EstablishRequirement`'s engineering act as only "Requirement artifact +
revision", naming neither order metadata nor an acceptance entry. Building
the canonical scenario against the real commands surfaced this: every call to
`ResolveEffectiveRequirements` failed with `ErrRevisionOrderMissing`, because
no command ever wrote a requirement's order metadata or accepted it.

**Decision.** `EstablishRequirementCommand.Execute` now also writes
sequence-`next` `RevisionOrderMetadata` and appends one `accepted`
`RevisionAcceptanceRecord`, in the same transaction, exactly as
`EstablishCapabilitySpecificationCommand` already does for a capability's
founding revision. No new command is introduced, and no repository contract
changes; this closes a gap in one existing command's write set.

**Alternatives.** Adding a separate `AcceptRequirement` command rejected —
FF-010 §3 fixes the count at ten commands, and this scenario never revises or
withdraws a requirement, so a second deliberate step has no observable
behavior to justify it here. Exempting requirement revisions from the
ordering contract in `ResolveCurrentRevision` rejected — it contradicts
FF-004 §3.2's explicit text and would require branching that function's
logic by artifact family, which no other part of the design does. Leaving the
gap and having the scenario driver write order/acceptance directly through
the repositories rejected — that would let one composition site (the
scenario driver) bypass a command's transactional guarantee that every write
a command makes either all commits or all rolls back together.

**Consequences.** A requirement's current revision resolves the same way a
capability's does, with the same rationale shape. If a future phase adds
requirement revision or withdrawal, the acceptance journal is already in
place to carry it. See
[FF-010 §3](../spec/010-application-contracts.md#3-commands) and
[FF-004 §3.2](../spec/004-current-state-resolution.md#32-effective-requirements).

---

## AD-020 — PostgreSQL enters as a driver-only infrastructure dependency, with hand-rolled migrations and SERIALIZABLE-with-retry

Status: Accepted
Date: 2026-07-28
Phase: M.4

**Context.** M.4 adds a second persistence adapter satisfying the identical
FF-009 §5–§7 contracts, which requires FeatureForge's first non-PEOS,
non-stdlib dependency. It also requires a mechanism for concurrent
revision-sequence assignment that PostgreSQL can enforce without the
in-memory adapter's whole-store mutex. Reading the command code settled that
second question: `ReviseCapabilitySpecificationCommand` and
`EstablishRequirementCommand` both compute the next sequence by *reading*
existing order metadata and then *writing* `max+1`, inside one `Do` — a
read-then-write race under real concurrency, not a single-statement one.

**Decision.**

1. `github.com/jackc/pgx/v5`, used natively (`pgxpool.Pool`, `pgx.Tx`), is the
   only new dependency, confined to `internal/infrastructure/postgres` and
   its test support. No ORM, no query builder, no migration framework. No pgx
   type appears in any exported signature.
2. Migrations are hand-rolled: SQL embedded via `embed.FS`, tracked in a
   project-owned `schema_migrations` table. One migration in M.4.
3. Every `Do` callback runs under `SERIALIZABLE`, and `Do` retries the whole
   callback on `40001`/`40P01` up to a bounded attempt count. This is how two
   concurrent revisions of one capability both succeed with sequences n and
   n+1.
4. Envelope and content payloads are stored as `bytea`, not `jsonb`.
5. Nested `Do` calls are detected by parsing the calling goroutine's ID out of
   `runtime.Stack`, tracked on the `UnitOfWork` value — the same technique the
   in-memory adapter already uses.

**Alternatives.** `pgx/v5/stdlib` rejected — this module's own
`TestNoHTTPDatabaseUIOrAIPackage` already forbids `database/sql` module-wide,
and native pgx maps `pgx.Tx` directly onto `Do`. A per-artifact
`SELECT ... FOR UPDATE` counter row, and `pg_advisory_xact_lock` inside
`RevisionOrderRepository.Put`, both rejected — the colliding sequence value is
already computed in application-layer Go by the time `Put` runs, so a lock
there protects nothing; locking earlier, inside the generic
`ListByArtifact` read, would change read semantics for every caller including
pure queries that must not block on unrelated writers. `SERIALIZABLE`-with-retry
operates at the same boundary the in-memory mutex already does — the whole
`Do` call — without touching application-layer code. A migration framework
rejected per the no-persistence-framework constraint and because one file
needs none. `jsonb` rejected — it reparses and reserialises on write while
every idempotency, conflict, and digest check here compares exact bytes, and
no query ever inspects a payload's structure.

*On nested-transaction detection specifically*, four alternatives were
considered and rejected. Letting a nested call deadlock: it would not — a pool
hands it a second connection, silently splitting one act across two
transactions, which is worse than an error. A context marker: `Do`'s
`func(Repositories) error` callback signature gives `Do` no channel to hand a
marked context back into the callback, so this is structurally impossible for
*either* adapter without changing an application-layer contract M.4 has no
mandate to change. Tracking depth on the pool rather than the `UnitOfWork`:
same mechanism, less obvious scope. A different mechanism for PostgreSQL only:
rejected as the worst option — two adapters detecting one condition two ways
means two behaviours to keep in agreement and a contract suite that no longer
proves they are equivalent. Reusing the existing technique keeps one
definition of what a nested transaction is. The cost — parsing an unsupported
runtime format — is accepted because it is reused rather than newly
introduced, and because a runtime change fails loudly in the shared suite.

**Consequences.** `go.mod` gains pgx and its transitive dependencies;
`internal/domain`, `internal/engineering`, and `internal/application` remain
driver-free, architecture-test-enforced, as does the rule that only
`internal/infrastructure/postgres` imports it. Every `Do` callback must now be
safely re-runnable — no second clock read, no randomness — which was already
true of every command but is now load-bearing and enforced by
`TestDoCallbacksAreRetrySafe`. That test found one real violation in
`CorrectValidationClaimCommand` when introduced. See
[FF-014](../spec/014-postgresql-persistence.md).

---

## AD-021 — Repository contracts enforce spec-declared referential and identity invariants, in both adapters

Status: Accepted
Date: 2026-07-28
Phase: M.4

**Context.** Building the PostgreSQL schema surfaced three invariants the
specifications declare that the in-memory adapter never enforced.
`FeatureCardRepository.Put` did not verify its `ProjectID` exists.
`RecordEnvelopeRepository.Put` did not verify its `SubjectKey` resolves to a
real Artifact or Revision — unlike `RevisionEnvelopeRepository.Put`,
`StructuredContentRepository.Put`, and `RevisionOrderRepository.Put`, which do
verify their references. `RevisionAcceptanceRepository.Append` appended
unconditionally, though FF-009 §4.3 declares `RecordID` unique and FF-006 §2
derives a timeline event's identity from it. Under the source-of-truth
priority these are implementation defects, not deliberate narrowings.

**Decision.** Close all three as a contract refinement binding on **both**
adapters, not a PostgreSQL-only strengthening.

`feature_cards.project_id` is a real foreign key in PostgreSQL and an explicit
lookup in memory. `Append` is create-only on `RecordID` in both — identical
re-append is a no-op, a differing one is `ErrImmutableValueConflict` —
enforced by `UNIQUE (record_id)` in PostgreSQL and a scan in memory.

For a record's subject, `SubjectKey` remains the single stored
representation: no denormalised `subject_artifact_id` /
`subject_revision_artifact_id` / `subject_revision_id` columns and no foreign
key. Both adapters instead parse it through a new shared
`engineering.ParseSubjectKey` and look the referenced value up — a map lookup
in memory, a `SELECT` in the same transaction in PostgreSQL. One parser, one
definition of what a `SubjectKey` means.

`revision_acceptance` keeps a surrogate `bigserial` primary key: it remains an
internal journal that nothing looks up by identity and no foreign key targets,
so `record_id`'s uniqueness is an identity invariant, not evidence that the
journal is an externally referenceable entity.

**`CorrectionTargetID` is deliberately excluded** from this refinement.
AD-017 already places that check on the write side in
`CorrectValidationClaimCommand`, which reports the more specific
`ErrCorrectionTargetMissing`, and requires `ResolveCurrentClaim` to remain
total over any stored graph — dangling, self-referential, or cyclic included —
so resolution cannot be poisoned by a payload written by other means. A third
check at the repository layer would duplicate the command-layer one and make
that read-side guarantee unreachable and untestable. Implementation confirmed
this concretely: adding it broke four tests that deliberately store degenerate
correction graphs to exercise the read-side algorithm.

**Alternatives.** Reproducing the in-memory gaps in PostgreSQL rejected — the
two adapters would no longer reject the same inputs, which is precisely the
property this milestone exists to prove. Enforcing only in PostgreSQL via
foreign keys rejected for the same reason. Materialising `SubjectKey` into
foreign-key-able columns rejected — it is already the canonical identifier,
and splitting it would mean maintaining two representations of one
relationship with no query or performance need to justify it; the
repository-logic check costs one extra read inside a transaction that is about
to write anyway, and keeps the storage model close to the domain model.

**Consequences.** Four application query tests, which wrote records naming a
subject that was never established, now seed that subject first — scoped,
expected fallout from strengthening a contract, and arguably more honest
fixtures. `TestAssignLifecycleStateCommand` likewise now establishes the
capability whose lifecycle it assigns. The `RecordID` change needed no fixture
changes. `internal/engineering` gains `ParseSubjectKey`, the inverse of the
existing `ArtifactSubjectKey`/`ArtifactRevisionSubjectKey` constructors, and
the shared contract suite gains four subtests so both adapters are held to all
of it. See [FF-014](../spec/014-postgresql-persistence.md).

---

## Open questions

None. Every material architecture decision for M.1 through M.4 is resolved.
Questions deferred to a later phase, with the phase that owns them:

| Question | Owned by |
|---|---|
| Whether any query needs materialization | Still open — only with measured evidence, and none has been gathered |
| Connection-pool sizing, timeouts, and retry tuning under load | M.5, with measurement |
| Whether `RelationEnvelope` is ever required | Deferred until a relation is |
| Random identity generation at the transport edge | M.5 |
| Which patterns Belcanto reuses or redesigns | M.7 freeze artifacts |

Resolved since M.1: canonical JSON serialization rules
([FF-009 §4.1](../spec/009-in-memory-persistence.md#41-capabilityspecificationcontent))
and repository contracts with the error taxonomy
([FF-009 §5](../spec/009-in-memory-persistence.md#5-repository-contracts),
[§8](../spec/009-in-memory-persistence.md#8-error-model)).

Resolved in M.4: PostgreSQL schema and index design
([FF-014 §3](../spec/014-postgresql-persistence.md), AD-020, AD-021). The
materialization question is deliberately *not* closed — M.4 added no
materialized projection and no index beyond what correctness requires, which
is the answer AD-006 asks for until there is evidence to the contrary.
