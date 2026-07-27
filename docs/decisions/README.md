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

## Open questions

None. Every material architecture decision for M.1 is resolved. Questions
deferred to a later phase, with the phase that owns them:

| Question | Owned by |
|---|---|
| Exact canonical JSON serialization rules for specification content | M.2 |
| Repository contract signatures and error taxonomy | M.2 |
| PostgreSQL schema and index design | M.4 |
| Whether any query needs materialization | M.4, and only with measured evidence |
| Which patterns Belcanto reuses or redesigns | M.7 freeze artifacts |
