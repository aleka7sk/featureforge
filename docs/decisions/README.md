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

## AD-022 — The Phase A HTTP surface is intent-oriented, not resource-CRUD

Status: Accepted and implemented
Date: 2026-07-28
Phase: M.5 (Phase A)

**Context.** FF-010 §3 decomposed the application layer into commands named
for engineering acts, with no `Update*`, no `Delete*`, and no generic
`RecordEngineeringAct`. A resource-CRUD HTTP surface would have to invent
`PUT`/`DELETE` semantics for immutable engineering values and for the bounded
stable-establishment surface later made explicit by AD-031, and
would flatten twelve distinct engineering acts into four verbs.

**Decision.** The public HTTP surface is intent-oriented: one endpoint per
command, one endpoint per query, `GET` and `POST` only, no `PUT`, `PATCH`, or
`DELETE` route registered anywhere. Full context, the complete route table,
and the acceptance criteria are in
[FF-018 §2.1 and §3](../spec/018-http-phase-a-implementation.md).

**Alternatives.** Resource-CRUD rejected — cannot express immutability
honestly. A single `POST /commands` envelope with a discriminator rejected —
defeats method/route-level architecture tests. GraphQL rejected — no consumer
need for a resolver layer over an already-fixed query set.

**Consequences.** The route table is a readable list of the engineering acts
the system supports; `TestOnlyTransportAndCommandImportNetHTTP` and the
nineteen-endpoint route table hold it in place. Implemented: twelve command
endpoints and seven query endpoints, verified by
`internal/transport/http`'s handler tests and the canonical-scenario-through-
HTTP test (`internal/transport/http/scenario_http_test.go`).

---

## AD-023 — The `net/http`/template import prohibition is narrowed into named-holder permissions

Status: Accepted and implemented
Date: 2026-07-28
Phase: M.5 (Phase A)

**Context.** `TestNoHTTPDatabaseUIOrAIPackage` forbade `net/http`,
`database/sql`, `html/template`, and `text/template` anywhere under
`internal/`, carried stale M.3-era wording, had an inert `postgres`/`sql`
path-prefix check, and left `cmd/` entirely unchecked. Phase A must import
`net/http`. Full investigation, evidence, and the permitted-holder table are
in [FF-018 §2.2](../spec/018-http-phase-a-implementation.md).

**Decision.** Narrow the prohibition into named-holder permissions rather
than removing it — the shape `TestOnlyIntegrationPackageImportsPEOS` (M.3)
and `TestOnlyPostgresInfrastructureImportsDriver` (M.4) already establish.
`net/http` is permitted only in `internal/transport/http` and
`cmd/featureforge`; `html/template`/`text/template` remain forbidden
everywhere in Phase A (reserved for the future Phase B UI package);
`database/sql` remains forbidden absolutely. Package discovery
(`internal/architecture.CmdPackages`) is extended to `cmd/` so the PEOS and
driver boundaries cover the executable too.

**Alternatives.** Deleting the test rejected — discards the boundary exactly
when it starts to matter. Adding `net/http` to an allow-list inside the one
broad test rejected — a failure would not name which boundary broke. Leaving
`cmd/` unchecked rejected — the executable is where a shortcut import is
least visible.

**Consequences.** `TestNoHTTPDatabaseUIOrAIPackage` is replaced by four named
tests (`TestOnlyTransportAndCommandImportNetHTTP`, `TestOnlyUIImportsHTMLTemplate`,
`TestDatabaseSQLIsNeverImported`, `TestNoAIPackage`) in
`internal/architecture/architecture_test.go`, each verified against a
deliberate violation before being trusted. `cmd/featureforge` is covered by
the PEOS and driver import guards for the first time.

**Later bounded extension (recorded here, decided in AD-028).** Phase B
(FF-021) added `internal/ui` as a third permitted `net/http` holder
alongside `internal/transport/http` and `cmd/featureforge` above — a caller
this decision's Phase A holder list did not anticipate, because
`internal/ui` did not exist yet. `TestOnlyTransportAndCommandImportNetHTTP`
enforces exactly these three names, and no others. The extension itself,
and why `internal/ui` needs `net/http` at all, is AD-028's decision, not a
reopening of this one; see AD-028's consequences for the exact statement,
and FF-021 §9 for the test evidence. This decision's own Phase A holder
table above is left as originally written — the record of what Phase A
decided when Phase A decided it — rather than rewritten as though
`internal/ui` had already existed.

**M.5 publication-remediation correction (M-2).** A post-implementation
audit of this milestone's local commit chain found that the four-way
decomposition above silently dropped `text/template`'s prohibition:
`TestNoHTTPDatabaseUIOrAIPackage` forbade it absolutely (see Context, above),
and none of the four replacement tests names it, so no test in the
repository failed when a deliberate `text/template` import was introduced
into a non-UI package during the audit. This decision's own text was never
wrong — it says plainly that "`html/template`/`text/template` remain
forbidden everywhere in Phase A" — but the implementation did not carry
that second half through. `TestTextTemplateIsNeverImported`
(`internal/architecture/architecture_test.go`) restores it as an absolute,
named guard, mirroring `TestDatabaseSQLIsNeverImported`'s shape: no holder,
anywhere, including `cmd/`. Full evidence in
[m5-publication-remediation.md](../reports/m5-publication-remediation.md).

---

## AD-024 — `cmd/featureforge` is the sole composition root; identity generation is deferred to Phase B

Status: Accepted and implemented
Date: 2026-07-28
Phase: M.5 (Phase A)

**Context.** Phase A introduces the module's first executable. FF-015 §10
also reserved identity generation at the transport edge to this decision.
Full context is in
[FF-018 §2.3](../spec/018-http-phase-a-implementation.md).

**Decision.** Two parts. *Composition:* `cmd/featureforge` is the sole
composition root — it reads configuration from environment variables
(`FEATUREFORGE_ADDR`, `FEATUREFORGE_ADAPTER`, `FEATUREFORGE_POSTGRES_DSN`),
selects one persistence adapter, applies migrations when the adapter requires
them, constructs the application dependencies, builds the router, and owns
server lifecycle — no application, transport, or engineering logic of its
own. *Identity generation:* client-supplied identity is accepted and is the
default; server-side generation is Phase B's, deferred rather than built now
for a caller (a UI form) that does not yet exist. No UUID dependency is
added — Phase B will use `crypto/rand` with a project-defined format when it
is needed.

**Alternatives.** Server-generated identity by default rejected — it removes
the free idempotency client-supplied identity gives every write. A UUID
library rejected — a new direct dependency, build-failing per
`TestGoModHasOnlyApprovedRequirements`. Building the generator in Phase A
rejected as untested infrastructure for a caller that does not exist until
Phase B.

**Consequences.** `cmd/featureforge/main.go` is wiring only, covered by the
same architecture rules as every other package. Phase A's command endpoints
are pure pass-through on identity; a request omitting a required identity is
`400 ErrInvalidCommand`. `postgres.Connect`'s `*pgxpool.Pool` is carried only
through type inference, so `cmd/featureforge` needs no direct `pgx` import.

---

## AD-025 — `RevisionEnvelope` gains an optional subject projection, and revisions become discoverable by family and subject

Status: Accepted
Date: 2026-07-28
Phase: M.5 (prerequisite change; implemented)

**Context.** `RecordEnvelope` projects `SubjectKey`, and
`RecordEnvelopeRepository.ListByKindAndSubject` searches on it, so decisions,
executions, claims, and state assignments are discoverable from the value they
are about. `RevisionEnvelope` projects no subject, and every revision-listing
operation — `ListByArtifact` on revisions and on order metadata — requires the
caller to already know the artifact identifier. Requirements and validation
plans are revisions.

The consequence is that no caller can enumerate the requirements governing a
capability unless it was told their identifiers in advance. In M.3 and M.4 the
only caller was `internal/scenario`, which created every record and held its
identifiers as fixture constants, so the gap was invisible and the contract was
correct for its callers. M.5 introduces a caller holding only a
`FeatureCardID`.

**HTTP exposed this; HTTP did not cause it.** The information was never
projected. What changed is the arrival of a caller that cannot supply it. Two
accepted acceptance criteria independently require the enumeration —
FF-001 §3.4's Requirements screen exists to list requirements, and FF-001 §3.2
requires an effective-requirement count and a per-requirement readiness
rationale table.

The full evidence is in
[the M.5 contract investigation](../reports/m5-contract-investigation.md).

**Decision.** Two additive changes, specified in
[FF-016](../spec/016-revision-subject-discovery.md).

1. `RevisionEnvelope` and `RevisionEnvelopeInput` gain
   `SubjectKey string` — a projection written once by
   `internal/engineering/peos` at record time, using the existing
   `engineering.ArtifactSubjectKey` helper, answering one question: *which
   capability Artifact is this revision about?*
2. `RevisionEnvelopeRepository` gains one operation, mirroring the validated
   `ListByKindAndSubject`:

   ```go
   ListByFamilyAndSubject(ctx context.Context, family engineering.RevisionFamily, subjectKey string) ([]engineering.RevisionEnvelope, error)
   ```

Semantics, in full in FF-016 §3–§4 and summarised here: three families define a
subject (`requirement` from its Subject, `validation-plan` from its Scope,
`transition-record` from its lifecycle Subject) and two do not (`capability` —
the revision *is* the subject; `evidence` — it is about nothing). Absence means
the family has no capability it is about, not that a value is missing or
pending. A subject supplied for a family without subject semantics is rejected
at construction, as is a malformed one, because a malformed projection fails
silently rather than loudly. Existence of the referenced artifact is **not**
verified at write time — AD-021 decided that for records, and extending it to
revisions is a separate decision requiring its own evidence. After validation
the persistence layer treats the value as opaque and compares it as an exact
string. Results are ordered ascending by `RevisionKey.String()`; no match is an
empty slice and never `ErrNotFound`; duplicates are impossible; an empty
`subjectKey` argument returns nothing rather than enumerating subject-less
revisions; and the operation never decodes a PEOS payload. Both adapters must
exhibit identical observable behaviour, proven by the shared contract suite
rather than asserted.

**Rejected alternatives.**

*1. Require HTTP callers to supply all revision artifact identifiers.*
Rejected — it relocates the problem to a caller that also cannot answer it. A
browser loading a feature page for the first time has no prior response to have
learned them from, and the screen that would list them is the one being
rendered. It also fails FF-001 §3.4 outright.

*2. Derive requirements from claims or plan activity.* **Rejected as
semantically invalid, not merely incomplete** — this is the most important
rejection in this decision, because it is the alternative that looks correct.
Claims project `CriterionKeys` as `requirement-revision:REQ-n/…`, so
requirements appear derivable from the claims that cite them, using only
existing projections and no forbidden capability.

The counterexample is FF-011's `REQ-4`. It has **no plan activity** and **no
claim** — FF-011 §5 lists it as "uncovered", and FF-011 §252 explains the
intent: it exercises `incomplete` alongside `not-ready` so the readiness
precedence rule "has both inputs present and is genuinely tested rather than
assumed". Nothing cites `REQ-4`, so claim-derived discovery cannot see it.
`REQ-4` would vanish from the discovered population, and its absence of a claim
is precisely the fact readiness exists to report. Readiness would therefore
report a **falsely complete** result: a requirement set that appears fully
accounted for while silently omitting the only requirement nobody validated.

A discovery method whose blind spot is exactly the population the query must
report does not return a degraded answer. It returns a confidently wrong one.
That is a correctness defect, not a limitation to document.

*3. Decode every stored PEOS revision payload during queries.* Rejected — it
requires importing the PEOS SDK outside `internal/engineering/peos`, violating
AD-005, the project's most fundamental boundary. It is also impossible in
practice: no repository operation enumerates revisions globally, so there is
nothing to iterate.

*4. Add an HTTP-only lookup registry.* Rejected — it creates a second source of
truth for a relationship the engineering records already contain, outside the
layer that owns engineering meaning, with no transaction covering it. It is a
materialized projection in a different location, and it can disagree with the
records. Contrary to AD-006 and to the PEOS consumer guide's warning against
recording derived verdicts as authoritative state.

*5. Add a new application query composed only from existing repositories.* This
was the most attractive alternative and **cannot be written**. Such a query
would need to call `Revisions.ListByArtifact(requirementArtifactID)` — which
requires the answer as its input. Composition cannot create information the
composed parts do not carry.

*6. Denormalize broader requirement or readiness state.* Rejected — storing a
requirement list or a readiness verdict is exactly the derived state AD-006
forbids, and `TestNoDerivedStateOnFeatureCard` already fails the build if a
`FeatureCard` grows such a field. A naming-convention scheme (`CAP-1` →
`REQ-1…REQ-n`) was also rejected: it would make identifier *format*
load-bearing, contradicting AD-003 and PEOS-002's prohibition on assuming
identifiers carry structure. `RelationEnvelope` (AD-013) was rejected as
disproportionate — an entire deferred construct to solve what one projected
string solves.

*What needs no change at all.* Decisions, executions, claims, and evidence are
already discoverable using existing operations: a decision's subject is a
capability revision, capability revisions are enumerable via `ListByArtifact`,
and `ListByKindAndSubject` finds records from there; evidence is recoverable
from the `EvidenceKeys` that claims and executions already project. Five of the
seven identifier lists the M.5 queries need dissolve without any contract
change. Only requirements and validation plans required this decision.

**Consequences.** The change is **additive**: one optional field, one new
method, no existing signature altered and no existing field's meaning changed.
Existing consumers remain compatible — `ResolveCurrentRevision`,
`ResolveReadiness`, the timeline, and both adapters' current behaviour are
untouched, and `RevisionEnvelope.Equal` still compares key and payload only, so
a projection cannot affect identity or conflict detection.

> **Corrected by [AD-026](#ad-026--revisionenvelopesubjectkey-participates-in-semantic-equality).**
> The preceding sentence is no longer accurate: `RevisionEnvelope.Equal` also
> compares `SubjectKey` as of AD-026/FF-017. This text is left as written,
> per this log's rule that a decision is never edited to say something
> different — see AD-026 for the corrected statement and its rationale.

**AD-005 is unaffected:** only `internal/engineering/peos` produces the value,
from input it already holds, and no read path decodes anything.
**AD-006 is unaffected:** the projection records a fact already inside the
immutable payload, written once at record time, exactly as
`RecordEnvelope.SubjectKey` has since M.3. No current-state answer, readiness
verdict, or resolved revision is materialized; every derived query remains
computed on read.

The change **extends a projection pattern M.4 validated** rather than
introducing a new direction — the M.4 architecture review §9.2 confirmed that
repository contracts constrain observable behaviour rather than storage
representation, and this is that principle applied to the envelope that lacked
it. **Shared contract coverage is mandatory** for the same reason it was
mandatory in M.4: a contract satisfied by two structurally different adapters
is load-bearing, and one satisfied by a single adapter is merely a description
of it. The subject is **optional rather than universally required** because two
of the five families genuinely have none; forcing a value would make them state
something false.

Costs: one nullable column and one index in PostgreSQL, one filtered scan in
memory, three subtests in the shared suite, and a permanent asymmetry with
`RecordEnvelope.SubjectKey`, which is mandatory — recorded in FF-016 §3.4 so it
reads as deliberate rather than inconsistent.

**Migration and backfill.** For a fresh database the migration adds storage and
an index; new writes populate the projection where it is defined. No backfill
is required for the current POC, and the empty-database assumption is
verifiable rather than assumed: no FeatureForge deployment exists, the test
container's data directory is `tmpfs`, and every integration test creates and
drops its own schema.

For any existing database, SQL can add the column but **cannot reconstruct
subjects** — that requires PEOS decoding, and **SQL must not decode PEOS**.
This is already enforced rather than merely stated:
`TestNoUpdateOrDeleteOnEngineeringTables` fails the build on
`UPDATE revision_envelopes`, so a backfill migration is structurally
prohibited while the additive `ALTER TABLE` is permitted. Any future
reconstruction must be a one-off tool inside `internal/engineering/peos`, using
that package's canonical decoders. This decision does not design it. A mixed
population returns incomplete results without error (FF-016 §3.8, §7.3), which
any such tool must treat as its acceptance criterion.

**Architecture Freeze exception — and its limits.** The M.4 freeze requires,
for reopening any frozen decision, "a working implementation that cannot
satisfy the contract, a contract test that cannot be made to pass, or a
reproducible failure the current design cannot express." The third is met: the
current design **cannot express** the question "which requirements govern this
capability?" The information exists inside each revision's payload, the only
package permitted to read it may not be called from the query path that needs
it, and no projection carries it outward. Every transport-level alternative was
evaluated; one attractive alternative produces a reproducibly false result; the
surviving change is the smallest additive extension of an existing validated
abstraction.

**This exception admits this decision only.** It does not license revisiting
`UnitOfWork`, transaction semantics, the `SubjectKey` string representation
(`refkeys.go` is unchanged), repository architecture generally, the PEOS
integration boundary, AD-005, AD-006, or any other envelope contract. Evidence
about revision subject discovery is evidence about revision subject discovery
and nothing else. A future proposal to reopen an unrelated M.4 contract must
produce its own evidence of comparable weight; it may not cite this decision as
precedent for a lower bar.

**Implementation evidence.** The change landed exactly as specified, in one
atomic commit following the two documentation commits that recorded this
decision. `RevisionEnvelope.SubjectKey` and the matching input field, the
`ListByFamilyAndSubject` method, migration `0002_revision_subject_key.sql`,
both adapters, three shared contract subtests
(`RevisionListByFamilyAndSubject`, `RevisionSubjectKeyIsOptional`,
`RevisionRejectsMalformedSubjectKey`), and a PostgreSQL-specific column
projection test all match FF-016 §13 without deviation. No existing
architecture test required a change; `TestNoUpdateOrDeleteOnEngineeringTables`
was deliberately violated with a temporary `UPDATE revision_envelopes`
statement in the new migration file to confirm it still catches a backfill
attempt in the new file specifically, then reverted. `internal/scenario`
gained a discovery-based proof: `REQ-4` — no plan activity, no claim — is
recovered by `ListByFamilyAndSubject` alone and still drives readiness to
`not-ready`, on both adapters; deleting `REQ-4` from the test's expected set
was confirmed to fail it for the expected reason before being reverted.
`internal/application` gained `DiscoverRequirementArtifactIDs` and
`DiscoverValidationPlanArtifactIDs`; `EngineeringStateInput` and
`TimelineInput` keep their exact shape, as this decision's Consequences
section anticipated — no signature besides the new repository method changed.

See [FF-016](../spec/016-revision-subject-discovery.md) for the binding
contract and the implementation order.

---

## AD-026 — `RevisionEnvelope.SubjectKey` participates in semantic equality

Status: Accepted
Date: 2026-07-28
Phase: M.5 (correction; implemented)

Recorded in [ad-026-subjectkey-equality.md](ad-026-subjectkey-equality.md).

A projection may be excluded from semantic equality only when it is fully
determined by fields that do participate in equality. `SubjectKey` is not, for
the content-free entry-assignment revision, so it joins `RevisionKey` and
`Payload` in `RevisionEnvelope.Equal` and therefore in create-only conflict
detection. Corrects one consequence stated in AD-025; supersedes nothing.

---

## AD-027 — Read-surface content is projected on read, never stored, through a sibling `EngineeringProjector` port

Status: Accepted and implemented
Date: 2026-07-28
Phase: M.5 (read-surface extension, ahead of Phase B UI planning)

**Context.** Phase B UI planning was stopped on a blocking finding: FF-018's
seven query endpoints expose identity, projections, and rationale metadata,
but not the engineering *content* FF-001 §3 requires a reader to see —
capability specification content, requirement statement text, a decision's
full basis, validation-plan activities, and claim reasoning. `internal/application`
had no query reading `StructuredContent` at all (only commands `.Put` it), and
`RecordEnvelope`/`RevisionEnvelope` project identity and outcome fields but
never the free-text content that exists only inside a stored `Payload`. Full
investigation is in
[FF-020](../spec/020-read-surface-extension.md).

**Decision.** Two parts.

*Decode at read time, not write time.* No new stored field, no migration, no
backfill. `Payload` is already authoritative (`RevisionEnvelope`'s own doc:
"Payload is authoritative and every other field is a derived projection");
decoding it is reading the authority, not duplicating it. This keeps adapter
parity structural — both adapters already persist the same `Payload` bytes,
and decoding happens above the adapter boundary — and upholds AD-006: nothing
derived is stored.

*A sibling `EngineeringProjector` port, decoding through the existing PEOS
seam.* `internal/engineering/peos` already carried a complete, round-trip-tested
decoder set (`DecodeDecision`, `DecodeClaim`, `DecodePlanRevision`,
`DecodeRequirementRevision`, …) with no caller outside its own tests. FF-020
adds thin projection functions in that same package — the only permitted PEOS
importer (AD-005) — that decode a payload and return PEOS-free structs defined
in `internal/engineering` (`DecisionDetail`, `PlanActivityDetail`; a
requirement's statement and a claim's reasoning need no dedicated type, each
being a single string). These are exposed through a new
`application.EngineeringProjector` interface — declared in engineering types
only, implemented structurally by the existing `peos.Recorder`, exactly the
shape `EngineeringRecorder` already established for the write side — rather
than added to `EngineeringRecorder` itself, so a query dependency never
implies write authority and vice versa.

**What needed no new mechanism.** A decision's evidence list, a claim's
criterion keys, and a claim's correction target were already projected on
`RecordEnvelope` (`EvidenceKeys`, `CriterionKeys`, `CorrectionTargetID`); FF-020
reads them directly rather than duplicating them into a projection type. The
applicable validation plan's identity was already discovered by every Q3/Q4
caller (`ResolveApplicableValidationPlanID`, §6.6's exactly-one contract) and
silently discarded; FF-020 renders it instead of re-deriving it.

**No new HTTP endpoint.** A companion review challenged the initially-proposed
`GET /features/{id}/validation` endpoint and found no query failed to own its
datum: Q4's own composition already resolved the applicable plan before
discarding it, and Q5's timeline already emitted `plan.revised`,
`execution.recorded`, `evidence.recorded`, and `claim.recorded`/`corrected`
events. A dedicated endpoint would have re-read exactly what Q4 and Q5 already
read, assembled for one screen — a screen-shaped (BFF) endpoint, the same
shape AD-022 already rejected alongside GraphQL. AD-022's nineteen-operation
surface is therefore neither extended nor amended; every FF-020 field is
additive to an existing response.

**Alternatives.** Storing projected content as new envelope fields at write
time, rejected — duplicates the payload's own authority and reopens the
migration/backfill cost FF-016 needed for `SubjectKey`, for no benefit `Payload`
does not already give for free. Adding projection methods to
`EngineeringRecorder` itself, rejected — collapses a query-only dependency into
one that also implies write authority. A new `GET /validation` endpoint,
rejected per the review above.

**Consequences.** No repository method, no migration, no new dependency, no
architecture decision reversed. `internal/transport/http`'s response DTOs gain
fields only — every field added is additive, so FF-018's frozen contract and
its 23+ transport tests keep passing unmodified. One new application sentinel,
`ErrStoredPayloadUnreadable`, closes the one new failure mode a decode
introduces: a payload that will not decode is server-side data corruption, not
client error, and maps to an opaque `500`.

---

## AD-028 — Browser writes go through the existing API handler in-process, never a second network hop

Status: Accepted and implemented
Date: 2026-07-28
Phase: M.5 Phase B (Minimal UI)

**Context.** FF-015 §6.4 originally said "screens submit HTML forms to the
same API endpoints." That sentence is not implementable against the frozen
Phase A contract, verified directly rather than assumed: `decodeJSON`
(`internal/transport/http/decode.go`) returns `415` for any request whose
`Content-Type` is not `application/json`, and a browser's native `<form>`
submission is `application/x-www-form-urlencoded`. Every command's success
response is `201` plus a JSON envelope, which a browser renders as raw text
with no navigation. Changing the API to accept form encoding or to redirect
was rejected outright: FF-018's Phase A contract is frozen, and content
negotiation was already rejected by FF-015 §11 as a needless second shape
for the same operation. FF-015 §19 left this decision open on purpose,
"pending Phase B. If a screen proves unreadable without it, that is
implementation evidence and gets a decision" — this is that evidence, and
this is that decision, made on the schedule FF-015 itself set.

**Decision.** `internal/ui` owns its own `POST` routes, one per command,
accepting `application/x-www-form-urlencoded`. Each route: (1) parses form
*syntax* only — no domain validation; (2) builds the API's exact JSON
request shape; (3) invokes the existing, unmodified API `http.Handler`
**in-process** through `ServeHTTP` against a hand-rolled
`http.ResponseWriter` capture — never a real network socket, never
`net/http/httptest` in production code; (4) interprets the real status code
and response body the API handler wrote; (5) issues a `303 See Other`
(POST-Redirect-GET) on success, or re-renders the originating screen with
submitted values preserved and the API's own client-safe message on a
correctable failure. Reads use the identical in-process mechanism with
`GET`. No JavaScript is written anywhere in the UI.

**Why not the alternatives.**

- *Minimal vanilla JS `fetch` → API over a real network request* — rejected
  on a hard constraint, not a preference: the write path could then only be
  proven by a headless browser, a dependency
  `TestGoModHasOnlyApprovedRequirements` would reject and CLAUDE.md forbids
  adding without a recorded decision. It also contradicts FF-015 §19's own
  stated default (no client-side interactivity assumed) and FF-015 §3's
  exclusion of a JavaScript framework.
- *UI routes calling `internal/application` directly* — rejected: the UI
  would become a second transport, duplicating both DTO mapping and FF-018's
  24-row error-to-status-code table, and would be exactly the "parallel path
  to the application layer" FF-015 §6.4 already rejects.

**Consequence — the strictest import boundary in the codebase.** Because the
UI speaks only HTTP to the engine it drives, `internal/ui` imports nothing
under this module's `internal/` tree besides itself: not
`internal/application`, not `internal/engineering`, not `internal/domain`,
no infrastructure adapter, no PEOS. This is structurally stronger than a
typical test-enforced boundary and is asserted directly by
`TestUIDoesNotImportApplicationOrInfrastructure` and
`TestUIDoesNotImportPEOS`.

**Cost, stated honestly.** One JSON encode/decode per screen that would
otherwise be a direct in-process call, and a small in-process
request/response shim. Both are accepted deliberately: they are what makes
the UI *evidence that the API is usable*, per FF-015 §1's own framing,
rather than a second front door into the engineering model. A change to the
API's conflict semantics, error codes, or response shape is observed by the
UI automatically, with no UI-side code change, because AD-028's bridge calls
the same handler FF-018 built and tested — proven directly by
`TestCallAPIDelegatesErrorOutcome` (`internal/ui/apiclient_test.go`), which
swaps a fake handler's response and shows the UI's outcome follows it with
zero UI-side logic touched.

**Alternatives considered and rejected are listed above; no other design was
seriously entertained** once the frozen-contract and no-headless-browser
constraints were established as facts about the existing system, not
preferences about the new one.

**Consequences.** No repository, `UnitOfWork`, or PEOS access from
`internal/ui`. No new HTTP endpoint on the API surface (nineteen operations,
unchanged). No new dependency. Full implementation evidence, including the
package layout, route table, error-UX mapping, and test coverage, is in
[FF-021](../spec/021-ui-phase-b-implementation.md).

**Bounded extension to AD-023.** This decision's in-process bridge
(`internal/ui/apiclient.go`) constructs a real `*http.Request` and a
hand-rolled `http.ResponseWriter` capture to call the API handler; every
page and form handler in `internal/ui` does the same for its own inbound
request. `internal/ui` is therefore a third permitted `net/http` holder,
alongside `internal/transport/http` and `cmd/featureforge` — an addition to
[AD-023](#ad-023--the-nethttptemplate-import-prohibition-is-narrowed-into-named-holder-permissions)'s
Phase A holder list, not a reopening of it: AD-023 froze what Phase A knew;
this decision is the reason Phase B needed one more holder.
`TestOnlyTransportAndCommandImportNetHTTP` enforces exactly these three
names, and no others (FF-021 §9).

---

## AD-029 — Command identities are required at the client edge

Status: Accepted and implemented
Date: 2026-07-31
Phase: M.5 (publication remediation)

**Context.** [AD-024](#ad-024--cmdfeatureforge-is-the-sole-composition-root-identity-generation-is-deferred-to-phase-b)
deferred identity generation to "Phase B... will use `crypto/rand` with a
project-defined format when it is needed." Phase B ([FF-021](../spec/021-ui-phase-b-implementation.md))
landed and did not need it: every command form resolves an identifier the
UI already knows from a prior query (`capabilityContext`, FF-021 §6) and
asks the person filling the form only for a genuinely new identity. AD-024's
generation contingency was therefore never exercised, and both FF-015 §10
and this log's Open Questions table kept naming "random identity generation
at the transport edge" as owned by M.5 after M.5's own implementation had
already made a different, undocumented choice. A post-implementation
publication-readiness audit of this milestone's local commit chain raised
this as finding M-1's neighbor: if a server ever did generate an identity
whenever a client omitted one, repeating the same request after a lost
response could create a second record, contradicting the idempotency every
other command surface guarantees. Full evidence in
[m5-publication-remediation.md](../reports/m5-publication-remediation.md).

**Decision.** All twelve command surfaces require their existing
authoritative identity field(s), unchanged from FF-018/FF-021. API clients
supply them; browser forms supply genuinely new identities and reuse
context-derived existing identities, exactly as FF-021 §6 already
specifies. The server does not generate an identity when one is omitted:
omission is invalid input and maps to the existing `400 ErrInvalidCommand`
response. Replaying the same identity and identical content remains
idempotent (a no-op, per the contract suite's `IdempotentIdenticalPut`);
replaying the same identity with different content remains `409
ErrImmutableValueConflict` (`ConflictingPut`). No `crypto/rand`,
`math/rand`, UUID package, idempotency-key mechanism, or deterministic
server-side generator is introduced. Any future convenience generation
requires a new accepted decision that explicitly covers lost-response
retries and identity ownership — the exact gap a silently-added generator
would otherwise reopen.

**Alternatives.** Server generates an identity when the client omits one
(AD-024's original contingency) — rejected: a client that sends a command,
loses the response, and retries the same omitted-identity request would
receive a second freshly-generated identity and create a second record,
silently breaking the idempotency guarantee every other command surface
has. A dedicated idempotency-key mechanism alongside client-supplied
identity — rejected: it would duplicate a guarantee client-supplied
identity already provides for free (FF-010 §1), for no caller this
milestone has. Client-side (browser) identity generation — rejected:
AD-028's no-JavaScript constraint leaves no place to generate one, and it
would not change the server's own omission handling regardless of where an
identity came from.

**Consequences.** Supersedes only AD-024's unimplemented "server-generated
when omitted" contingency; AD-024's composition-root provisions and its
"client-supplied identity is accepted and is the default" statement remain
valid and unchanged. The Open Questions entry "Random identity generation at
the transport edge" is resolved below: not used in M.5; client-supplied
identities are required. No code change was required by this decision —
FF-018's and FF-021's identity handling already matched it; this decision
records, rather than changes, the as-built contract, closing the gap where
implementation had silently outpaced its own governing decision.

---

## AD-030 — Command idempotency is recovered from validated persisted acts

Status: Accepted
Date: 2026-08-01
Phase: M.5 (correctness closure before domain analysis)

Recorded in
[ad-030-command-idempotency-and-replay-conformance.md](ad-030-command-idempotency-and-replay-conformance.md).

Exact replay is recognized from the complete, integrity-checked persisted
semantic act before the application reconstructs server time, sequence,
provenance, or transition state. This narrowly supersedes AD-029's claim that
the already-present identity fields and repository re-`Put` behavior made all
twelve commands replay-safe without code changes. AD-029's rejection of
server-generated identity and a separate idempotency-key mechanism remains.

AD-030 also clarifies AD-019's C7 accepted member and makes its new
`acceptance_record_id` caller-owned; supersedes AD-021's statement that no
acceptance lookup by identity is needed; preserves AD-026 repository equality;
and explicitly chooses C9's new complete act as Validation Plan Artifact +
Revision + order metadata + immediate accepted caller-identified member.
One Requirement Artifact now has one lifetime subject and one Validation Plan
Artifact one lifetime scope: new retarget attempts conflict, while mixed stored
history fails integrity. Subject projection only enumerates Q3/Q4/Q5 candidates;
the application inspector validates each complete history before use.
Implementation and completion evidence are governed by
[FF-022](../spec/022-command-replay-and-aggregate-integrity.md).

---

## AD-031 — Operational establishment is stable only within the bounded POC

Status: Accepted
Date: 2026-08-01
Phase: post-M.5 domain closure before M.6

Recorded in
[ad-031-bounded-operational-establishment.md](ad-031-bounded-operational-establishment.md).

Project and FeatureCard remain operational rather than PEOS engineering
records, but FeatureForge deliberately exposes no generic edit command. Their
establishment fields are stable for this POC; the only operational change is
the one-time, monotonic FeatureCard-to-capability link. That link is not part of
base FeatureCard `Put` equality, and both adapters must behave identically
before and after it is established. Belcanto must make its own mutability,
concurrency, audit, and replay decision instead of inheriting this POC
narrowing.

---

## AD-032 — Lifecycle policy is persisted and current state is the unique linear head

Status: Accepted
Date: 2026-08-01
Phase: post-M.5 lifecycle closure before M.6

Recorded in
[ad-032-persisted-lifecycle-policy-and-linear-history.md](ad-032-persisted-lifecycle-policy-and-linear-history.md).

FeatureForge now persists its fixed PEOS Lifecycle Definition and Definition
Version through dedicated PEOS-free carriers initialized before serving. C6,
Q4 and Q5 validate one complete linear assignment/transition history against
that stored policy. Current state is the unique graph head; a new transition
must depart from it and follow the configured source/target edge. Invalid new
transitions are `422`, stale-head concurrency is `409`, and invalid stored
policy or history is opaque `500`. The canonical scenario gains the missing
`specified` state before `under-validation`. Integrity-sensitive discovery
enumerates every Revision and Record envelope, validates it, and only then
filters by family, kind or subject, so a contradictory projection cannot hide a
corrupt lifecycle or Q3/Q4/Q5 member.

---

## AD-033 — Requirement-to-criterion trace is structured product-owned state

Status: Accepted
Date: 2026-08-01
Phase: pre-M.6 traceability closure

Recorded in
[ad-033-requirement-criterion-trace-is-structured-state.md](ad-033-requirement-criterion-trace-is-structured-state.md).

Every Requirement Revision gains an immutable FeatureForge-owned trace to one
exact capability Revision and revision-local acceptance-criterion key. It is a
required C7 aggregate member, not a parsed Origin note or a misused PEOS
Requirement Derivation. This makes Requirements UI traceability and M.6's
uncovered-criterion computation honest. The canonical four Requirements map to
Revision 2's AC-1..AC-4; AC-4 is uncovered because REQ-4 has no current Claim.

---

## AD-034 — AI-assisted proposals are transient reviewed input

Status: Accepted and implemented
Date: 2026-08-01
Phase: M.6 AI context-pack demonstration

Recorded in
[ad-034-ai-assisted-proposals-are-transient-reviewed-input.md](ad-034-ai-assisted-proposals-are-transient-reviewed-input.md).

FeatureForge assembles one exact, source-bearing ContextPack in a read-only
UnitOfWork and invokes a deterministic `internal/proposal` generator only after
that transaction closes. Context and Proposal digests are transient content
addresses, not engineering identities or stored records. Explicit acceptance
revalidates a genuinely new proposal's context in its write UnitOfWork and
creates one ordinary draft capability Revision with local-user actor,
AI-assisted provenance method, source-bearing Origin, caller-owned Revision ID,
and no acceptance append. Exact occupied replay remains `201` with zero writes
before freshness. UI continues through AD-028's in-process HTTP API boundary;
it receives no Generator or application dependency.

Implementation and M.6 exit evidence are governed by
[FF-024](../spec/024-ai-assisted-proposal-workflow.md). Authentication,
provider/model integration, proposal persistence, autonomous writes, and the
actual product application/login surface remain out of scope until after M.7's
independent audit and freeze gate.

M.6 implementation is published at
`395e163180649dcb5735b006ef6e225c80570a69` (tree
`e2960d8504fdaf9ef39ce748bde5dce2174aa89f`). The independent audit found no
BLOCKER or MAJOR. Verification commit
`64cd1a5a7c37cfe632fbc3f2f28a9eb046830ffc` passed the complete PostgreSQL and
race-enabled [GitHub Actions gate](https://github.com/aleka7sk/featureforge/actions/runs/30690421922).

---

## Open questions

None for the implemented M.6 boundary. Every material architecture decision
through M.6 is resolved, and FF-024's implementation and publication evidence
are complete. M.7 owns the independent consumer audit and freeze decision.
Questions deferred to a later phase, with the phase that owns them:

| Question | Owned by |
|---|---|
| Whether any query needs materialization | Still open — only with measured evidence, and none has been gathered |
| Connection-pool sizing, timeouts, and retry tuning under load | M.5, with measurement |
| Whether `RelationEnvelope` is ever required | Deferred until a relation is |
| Random identity generation at the transport edge | Resolved — not used; client-supplied identities are required (AD-029, see below) |
| Whether revision subject references should be existence-verified at write time | Deferred; AD-021 is the precedent that would govern it, and it needs its own evidence (FF-016 §3.6) |
| Whether a subject backfill tool is ever required | Deferred until a durable FeatureForge database exists (FF-016 §7) |
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

Resolved in M.5 planning and implemented ahead of the general M.5 HTTP work:
revision subject projection and discovery (AD-025,
[FF-016](../spec/016-revision-subject-discovery.md)). This clears the
prerequisite the M.5 queries needed for a complete requirement or
validation-plan population; **at the time AD-025 landed**, the HTTP API and
UI implementation §16 of FF-015 orders was separate work and remained open.
It is no longer open: FF-018 (Phase A), FF-020 (read-surface extension),
and FF-021 (Phase B UI) have since landed, and FF-015 records M.5 as
complete. The paragraph above is left as originally written — the record of
what was true when AD-025 landed — rather than rewritten as though Phase A
and Phase B already existed at that point. As of this remediation, the
resulting local commit chain is a **candidate** awaiting the publication
re-audit `m5-publication-remediation.md` records; see that report for
current status.

Resolved and implemented after Phase A, ahead of Phase B UI planning: the
read-surface content gap Phase B UI planning found blocking (AD-027,
[FF-020](../spec/020-read-surface-extension.md)). This clears the prerequisite
FF-001 §3's usability acceptance needs — every screen now has a query
answering it — so Phase B UI planning can resume against a complete read
contract.

Resolved in M.5 publication remediation: whether the transport edge should
generate an identity when a client omits one (AD-024's deferred
contingency). It does not — client-supplied identity remains required, and
omission is `400 ErrInvalidCommand` (AD-029,
[m5-publication-remediation.md](../reports/m5-publication-remediation.md)).
The same report also records the restoration of the `text/template`
architecture guard AD-023 specified but the Phase A/B decomposition dropped
(M-2), and the correction to `GetFeatureTimelineForCard`'s execution/claim/
evidence discovery so prior-revision validation activity remains visible
after a later capability revision becomes current (M-1), per FF-006 §1 and
FF-007's M.5 exit criterion.

Corrected forward in the M.5 pre-domain closure: AD-030 establishes that
client-owned identity is necessary but not sufficient for command replay.
Application-level persisted-act inspection is required before reconstructing
time-bearing values. It also adds caller-owned acceptance identities to C7 and
C9, with a narrow replay-only omission exception for a complete historical C7
act. The historical publication-remediation report remains unchanged; FF-022
owns implementation and evidence for this correction.
