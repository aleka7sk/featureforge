# AD-026 — `RevisionEnvelope.SubjectKey` participates in semantic equality

Status: Accepted
Date: 2026-07-28
Phase: M.5 (correction; specified, not yet implemented)

Indexed in [the decision log](README.md). Implemented by **FF-017**, which has
not yet been written; this decision precedes the code that depends on it, per
CLAUDE.md.

## Context

[AD-025](README.md#ad-025) added an optional `SubjectKey` projection to
`RevisionEnvelope` and a `ListByFamilyAndSubject` operation to
`RevisionEnvelopeRepository`, so requirements and validation plans became
discoverable from the capability they concern. That change landed as FF-016 in
commit `aeaa24f`.

The [FF-016 architecture review](../reports/ff016-architecture-review.md)
approved the change but recorded three findings. **F-001** is what forces this
decision; **F-002** and **F-003** are the test gaps that let it reach `main`
unobserved.

### The discovered behaviour, precisely

`RevisionEnvelope.Equal` (`internal/engineering/envelope.go:161–164`) compares
`RevisionKey` and byte-identical `Payload`, and nothing else. Its own comment
states the intent: *"Projections are not compared."* `SubjectKey` is a
projection and is therefore excluded.

Both adapters use `Equal` as the definition of an idempotent create-only `Put`:

- memory — `revisionRepo.Put` (`internal/infrastructure/memory/repositories.go:190`)
  delegates to `putCreateOnly` (`:45–54`), which on an existing key returns
  `nil` **without writing** when `equal(existing, value)` holds, and
  `ErrImmutableValueConflict` otherwise;
- PostgreSQL — `revisionRepo.Put`
  (`internal/infrastructure/postgres/repositories.go:354–381`) issues
  `INSERT … ON CONFLICT (artifact_id, revision_id) DO NOTHING`, and on zero
  rows affected re-reads and returns `nil` if `existing.Equal(env)`, else
  `ErrImmutableValueConflict`.

Consequently, for an existing `RevisionKey`, a `Put` carrying the same payload
but a **different** `SubjectKey` is accepted as an idempotent no-op. The
already-stored `SubjectKey` silently remains authoritative, and the caller
receives `nil`.

**This is observable persisted-state divergence, not adapter inconsistency.**
The two adapters agree exactly — the shared contract mechanism did its job and
prevented divergence between them. What diverges is the value the caller
submitted and the value the store retains, and that divergence is invisible:
no error, no warning, and a subsequent `ListByFamilyAndSubject` answers from a
projection the last writer did not supply.

### Why the exposure is uneven

`SubjectKey` is populated at the PEOS integration boundary. For three of the
four codec paths that project one, the subject is also inside the marshalled
payload, so byte-identical payloads imply identical subjects and no divergent
pair can be constructed through the codec. For `BuildEntryAssignment` it is
not. §"Architectural principle" below explains why that distinction is the
whole decision.

## Architectural principle

The envelope model's founding statement is in the code itself
(`internal/engineering/envelope.go:30–32`):

> It is a carrier, not a domain model: Payload is authoritative and every
> other field is a derived projection.

**This principle is preserved unchanged.** Payload remains authoritative;
`SubjectKey` remains a derived projection, not a source of truth. Nothing in
this decision promotes a projection to authority.

What this decision changes is a second, previously *implicit* rule that
governs when a projection may be omitted from an equality comparison. Stated
explicitly for the first time:

> **A projection may safely be excluded from semantic equality only when it is
> fully determined by fields that do participate in equality.**

This is the invariant that made "compare key and payload only" sound. Where it
holds, comparing the projection adds nothing, because payload equality already
implies projection equality. Where it does not hold, excluding the projection
does not simplify the comparison — it silently narrows it.

### Where the invariant holds

| Codec path | Subject inside the marshalled payload | Determined by compared fields? |
|---|---|---|
| `BuildRequirement` (`codec_requirement.go:63–71`) | `requirement.NewContent(…, []core.EngineeringSubjectRef{subject}, …)` flows into `reqRev`, the marshalled value | **yes** |
| `BuildValidationPlan` (`codec_validation.go:103–116`) | `scope` → `NewScopedPlanApplicability` → `NewPlanContent` → `planRev` | **yes** |
| `BuildTransition` (`codec_lifecycle.go:281–284`) | `lifecycle.NewTransitionRecordContent(subject, …)` → `trRevision` | **yes** |

For these three, the projection is genuinely redundant with respect to the
payload. Two envelopes with byte-identical payloads cannot carry different
subjects, so `Equal`'s current narrowness is invisible and harmless.

### Where the invariant does not hold

`BuildEntryAssignment` (`codec_lifecycle.go:179–195`) builds `entryRev` as a
bare `core.ArtifactRevision` from the transition record's own identifiers,
origin, provenance, and an integrity value of
`TransitionRecordArtifactID + "/" + TransitionRecordRevisionID`. The revision
is **content-free under [AD-014](README.md#ad-014)**. `in.SubjectArtifactID`
never enters it; it reaches only the separate state-assignment
`RecordEnvelope` built alongside.

The projected `SubjectKey` therefore originates in adjacent state-assignment
information rather than in the revision's own payload, and **cannot be
reconstructed from that payload**. The envelope is not self-describing: given
only the stored revision, no reader — and in particular no repository, which
[AD-005](README.md#ad-005) forbids from decoding a payload at all — can
determine whether its projection is the one that was intended.

**AD-014 is not defective.** A lifecycle entry legitimately has no content to
carry; making its revision content-free was correct and remains correct. The
incompatibility is narrower and lies elsewhere: **between a content-free
revision payload and a separately persisted subject projection that equality
ignores.** Either of those two is unobjectionable alone. Together, and with
`Equal` narrowed to the payload, they produce state that can differ without
detection.

## Decision

**`RevisionEnvelope.SubjectKey` participates in `RevisionEnvelope` semantic
equality**, and therefore in create-only repository conflict detection.

For an existing `RevisionKey`:

| Incoming `Put` | Outcome |
|---|---|
| same payload, same `SubjectKey` | **idempotent** — no-op, `nil` |
| same payload, **different** `SubjectKey` | **`ErrImmutableValueConflict`** |
| different payload | `ErrImmutableValueConflict` (unchanged) |

Clarifications that bound the decision:

- `SubjectKey` is **persisted, observable repository state** — a stored column,
  a field returned by `Get`, and the sole predicate of
  `ListByFamilyAndSubject`.
- `SubjectKey` is **not** part of revision identity. Revision identity remains
  `RevisionKey` = (`ArtifactID`, `RevisionID`), which is also the PostgreSQL
  primary key. This decision changes what it means for two envelopes under one
  identity to be *the same value*, never what identifies them.
- **No repository method signature changes.**
- Adapters continue to use `RevisionEnvelope.Equal`. They are not given
  adapter-specific comparison logic.
- **Memory and PostgreSQL inherit the new behaviour identically**, because both
  already route conflict detection through the same `Equal`. This is the
  property that makes the correction one clause rather than two
  implementations, and the shared contract suite proves the parity rather than
  asserting it.
- The repository **still must not decode PEOS payloads** (AD-005). Comparing a
  projected string is not decoding.
- **No referenced-capability existence check is introduced.** FF-016 §3.6
  deliberately excluded that, and [AD-021](README.md#ad-021) is the precedent
  that would govern it; this decision does not reopen it.

## Option A evaluation — `SubjectKey` remains projection-only

**Rejected.**

Option A keeps `Equal` as it is and treats `SubjectKey` as derived state that
equality may ignore.

Assessed against the criteria:

- **Create-only semantics.** Formally preserved in the sense that history is
  never mutated — the first write wins and is never overwritten. But the rule
  "an identical re-`Put` is a no-op and a differing one conflicts" ceases to
  range over the value the repository actually stores. Create-only becomes a
  statement about a subset of the record.
- **Observable persisted state.** Left unprotected. A caller can submit a value
  and receive success while the store retains a different one.
- **Idempotency.** Redefined as idempotency-with-respect-to-payload, which is
  not what any caller of `Put` would assume from the interface.
- **Backward compatibility.** Perfect — nothing changes. This is Option A's
  only genuine advantage.
- **Repository abstraction.** Weakened. The repository silently discards part
  of a submitted value, which is behaviour a contract should never have to
  document as acceptable.
- **Future evolution.** This is where Option A fails decisively. Its
  correctness depends on *every* projection being derivable from the payload.
  That property **cannot be enforced by the repositories**, because AD-005
  prohibits payload decoding at exactly the layer that would have to check it.
  It is therefore an unwritten, unguarded, per-field obligation that must be
  re-audited by hand every time a projection is added — forever, with no test
  that can fail if the audit is skipped.
- **Transition-record implications.** `BuildEntryAssignment` **already violates
  the property.** Option A is not a rule that holds today and might be
  endangered later; it is a rule that is already broken, and whose breach is
  silent.

An architecture whose soundness rests on an unenforceable invariant that is
already violated, with no mechanism able to detect the violation, is not
architecturally coherent. Option A fails on that ground alone.

## Option B evaluation — `SubjectKey` participates in equality

**Selected.**

Assessed against the same criteria:

- **Create-only semantics.** Restored in full. "Identical is a no-op, different
  is a conflict" once again ranges over the whole observable value.
- **Observable persisted state.** Protected. A submitted projection is either
  stored or rejected; it is never accepted and discarded.
- **Idempotency.** Becomes idempotency with respect to the record, which is
  what the interface implies.
- **Compatibility.** No stored representation changes; no data is rewritten.
  The only behavioural change is that a `Put` which previously succeeded while
  discarding a differing subject now returns `ErrImmutableValueConflict`. For
  the three payload-carrying codec paths this case cannot arise, so no existing
  caller is affected. FF-016 §7.4 records that no durable FeatureForge database
  exists, so no stored data can be in the affected state.
- **Implementation complexity.** One comparison clause. Both adapters inherit
  it through the shared `Equal` contract; neither adapter is edited.
- **Migration impact.** None (see §"Migration impact").
- **Future evolution.** The rule is structural rather than conventional. A
  future projection either is determined by compared authoritative state, or it
  participates in equality — and the second branch requires no per-field
  judgement to be safe. The obligation moves from "remember to audit" to
  "the comparison covers it".

Option B is **independent of payload-decodability assumptions**. It costs
nothing where the invariant holds — comparing a field that is already implied
by the payload can never change an outcome — and produces a visible conflict
precisely where projection divergence is possible. It is a no-op strengthening
everywhere except at the one place a signal is wanted.

## Alternatives rejected

**1. Option A — leave `Equal` unchanged (see above).** Rejected because its
correctness depends on an unenforceable, already-violated invariant, and its
failure mode is silent.

**2. Remove the transition-record subject projection.** The narrowest possible
repair: stop projecting a subject for `RevisionFamilyTransitionRecord`, or for
the entry-assignment path specifically, restoring "every projection is derived
from the payload" as a true statement. Nothing currently consumes that
projection — FF-016 §9's application integration queries only
`RevisionFamilyRequirement` and `RevisionFamilyValidationPlan`.

Rejected because it repairs **one current codec path rather than the repository
contract.** The contract would still permit a projection that equality ignores;
the next projection added — and FF-016 §14 already contemplates further ones —
would reintroduce the same hazard with no guard. It treats the instance and
leaves the class. It would also discard a projection that is semantically
correct, purely because equality is too narrow to police it, which inverts the
proper direction of the fix.

**3. Adapter-specific comparison.** Rejected outright, on the reasoning AD-020
already applied to nested-transaction detection: two adapters implementing one
semantic rule two ways means two behaviours to keep in agreement and a shared
contract suite that no longer proves they are equivalent. Routing the change
through `Equal` keeps one definition.

**4. Repository-level verification that the projection matches the payload.**
Rejected — it requires decoding a PEOS payload inside a persistence adapter,
which AD-005 forbids and which no amount of local convenience justifies.

## The seven questions

1. **Is `SubjectKey` part of repository state?** **Yes.** It is a stored
   column, a field returned by `Get`, and the sole predicate of
   `ListByFamilyAndSubject`.
2. **Is `SubjectKey` part of artifact identity?** **No.** Revision identity
   remains `RevisionKey`.
3. **Is `SubjectKey` observable state?** **Yes** — readable through `Get` and
   queryable through `ListByFamilyAndSubject`.
4. **May observable persisted state be ignored by semantic equality?** **Only
   when it is completely determined by fields already participating in
   equality.**
5. **Should projections participate in create-only conflict detection?** **They
   must, when they are not provably determined by compared authoritative
   fields.**
6. **Does `BuildEntryAssignment` reveal a deeper inconsistency?** **Yes** — it
   reveals that the projection-derivability assumption is not universal, which
   is precisely why that assumption could not remain implicit. **It does not
   invalidate the content-free design of AD-014.**
7. **Does the selected rule generalize to future projections?** **Yes.** Future
   projections must either be derivable from compared authoritative state or
   participate in equality.

## Scope boundary

**AD-026 governs `RevisionEnvelope.SubjectKey` and nothing else.**

This decision does not decide, and must not be read as implying, any immediate
change to:

- `ArtifactEnvelope.Equal`;
- `RecordEnvelope.Equal` — including `RecordEnvelope.SubjectKey`, which is
  mandatory and likewise excluded from equality;
- any other existing projection (`ContentDigest`, `Outcome`, `CriterionKeys`,
  `EvidenceKeys`, `ExecutionKeys`, `Scope`, `StateID`, provenance fields);
- other envelope families;
- PEOS, which is used unchanged at v1.0.0;
- the `SubjectKey` string representation — `internal/engineering/refkeys.go` is
  untouched;
- `UnitOfWork`, transaction semantics, or retry behaviour;
- migration strategy generally.

The principle stated in §"Architectural principle" is written to be general
because a rule that applies only to one field is not a principle. **Applying it
to any other envelope or projection requires separate evidence and a separate
decision, or an explicit amendment to this one.** The AD-025 precedent governs:
evidence about one contract is evidence about that contract and nothing else,
and a future proposal may not cite this decision as licence for a lower bar
elsewhere.

Nothing here is known to be wrong with the other envelopes. `RecordEnvelope`'s
subject, in particular, is carried inside every PEOS record payload, so its
exclusion from equality satisfies the invariant. The scope boundary reflects
absence of evidence, not presence of a defect.

## Consequences

**What this makes easy.** Create-only conflict detection once again covers the
whole observable value of a revision envelope, so a caller can rely on `Put`
either storing what it submitted or telling it why not. Divergence between the
submitted and stored projection becomes impossible rather than undetectable.
Future projections inherit the guarantee structurally.

**What it costs.** A behavioural change at the repository boundary: a `Put`
that previously returned `nil` while discarding a differing subject now returns
`ErrImmutableValueConflict`. This is the intended change and the only one.

**What it does not cost.** No schema change, no data rewrite, no signature
change, no adapter-specific work, no new dependency, no relaxation of AD-005.
`RevisionEnvelope` identity is unaffected. The two families the M.5 discovery
path queries are unaffected in practice, because their subjects are encoded in
their payloads and cannot diverge independently.

**One documentation consequence must be handled explicitly.** AD-025's
Consequences section currently states that *"`RevisionEnvelope.Equal` still
compares key and payload only, so a projection cannot affect identity or
conflict detection."* That sentence becomes false when FF-017 lands. Per this
log's rules a decision is never edited to say something different — AD-025 is
amended by naming this decision, not by rewriting the sentence. AD-025 is
**not superseded**: its subject-projection and discovery decisions stand
entirely; only this one consequence is corrected.

## Required implementation follow-up

The smallest FF-017 implementation packet. Described here, deliberately not
designed and not implemented.

**Contracts**

- `RevisionEnvelope.Equal` compares `SubjectKey` in addition to `RevisionKey`
  and `Payload`.
- No signature changes anywhere — not on `Equal`, not on any repository method.

**Shared contract tests** (`internal/infrastructure/contracttest/contract.go`,
executed unchanged by both adapters — these close review finding F-002)

- a subject-bearing idempotent `Put` with an identical `SubjectKey` succeeds as
  a no-op;
- identical key and payload with a different `SubjectKey` returns
  `ErrImmutableValueConflict`, in both directions (stored empty → incoming
  populated, and the reverse);
- transaction visibility for a subject-projected revision is verified —
  overlay-over-committed in memory, own-uncommitted-writes in PostgreSQL;
- both memory and PostgreSQL execute the identical test bodies.

**Codec and projection tests** (closing review finding F-003)

- `BuildEntryAssignment` populates the expected `SubjectKey`;
- `BuildTransition` populates the expected `SubjectKey`;
- transition-record subject discovery returns the revision for the projected
  capability.

**Documentation**

- amend the affected consequence wording in AD-025 by naming AD-026, without
  rewriting the original sentence;
- update FF-016's equality and create-only semantics and its implementation
  evidence;
- mark AD-026 implemented **only after FF-017 lands**, following the AD-025
  precedent of recording implementation evidence at that point;
- **do not modify FF-009**;
- preserve `docs/reports/ff016-architecture-review.md` as a historical record —
  it is not rewritten to reflect the correction it prompted.

**Migration**

- **None.**

## Migration impact

**None.** No stored representation changes: the `subject_key` column added by
`0002_revision_subject_key.sql` keeps its type, nullability, and index. No row
is read, rewritten, or deleted, so nothing approaches the `UPDATE` prohibition
that `TestNoUpdateOrDeleteOnEngineeringTables` enforces. No new migration file
is required.

The change is confined to an in-memory comparison performed before a write is
attempted. FF-016 §7.4 additionally records that no durable FeatureForge
database exists, so no stored data can already be in the divergent state this
decision makes detectable.

## M.5 blocking status

**FF-017 is not a strict prerequisite for beginning M.5 Phase A HTTP
discovery of `Requirement` and `ValidationPlan`**, because their `SubjectKey`
values are encoded in their payloads and cannot diverge independently. The
architecture review reached the same conclusion at F-001, and this decision
does not narrow it.

**FF-017 should be implemented before any caller relies on `TransitionRecord`
subject discovery**, which is the one family where divergence is reachable and
where no test currently covers the projection at all (F-003).

**Completing FF-017 before expanding HTTP remains the recommended
sequencing.** The correction is narrow — one comparison clause plus tests, no
migration, no signature change — and it closes an already-confirmed contract
defect. Sequencing it first means the M.5 transport is built against a
repository contract that is correct rather than one that is correct only for
the two families it happens to use first.

## Decision confidence

**High.** The analysis rests on directly readable facts: the three `Equal`
implementations, the two adapters' `Put` paths, and the four codec call sites
that project a subject. The asymmetry between `BuildEntryAssignment` and the
other three is a property of the code, not an inference about intent, and it is
independently corroborated by the architecture review's F-001.

Confidence in **Option B over Option A** is high, and rests on a single
argument that does not depend on taste: Option A's correctness condition is
unenforceable at the layer that would have to enforce it, and is already
violated. Confidence in **Option B over the rejected "remove the projection"
alternative** is high but is a judgement about scope rather than about
correctness — both repair the present defect, and Option B was chosen because
it establishes a rule that survives the next projection.

The one element carrying **medium** confidence is the practical reachability of
the divergence in a real deployment. It requires either direct repository use
or two entry assignments sharing a transition-record identity and recorded-at
instant while naming different capabilities. This decision does not rest on
that reachability: the contract permits the divergence, and a contract that
permits silent state loss is worth correcting whether or not a current caller
provokes it.
