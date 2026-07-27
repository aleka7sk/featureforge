# FF-010 — Application Contracts

Status: Accepted (Phase M.2)
Governs: identity and time strategy, commands, queries, and the exact algorithms
for revision ordering, acceptance, correction resolution, release readiness,
lifecycle state, and the timeline.

## Governance statement

PEOS-000 through PEOS-009 remain authoritative for every PEOS concept.
`docs/spec/` governs FeatureForge. This document does not extend the PEOS
ontology. The implementation must use PEOS v1.0.0 unchanged.

## 1. Identity strategy

| Question | Decision |
|---|---|
| Do production constructors accept caller-provided IDs? | **Yes.** Every command carries the identities it will create. |
| Do tests use fixed IDs? | **Yes.** All M.3 fixtures use the deterministic IDs in [FF-011](011-canonical-scenario.md). |
| Is random generation in scope? | **No.** Deferred. No UUID dependency, no `math/rand`, no `crypto/rand` in M.3. |
| Where are PEOS identity values constructed? | **Only** in `internal/engineering/peos`. Every other layer handles product-owned strings and typed keys. |

Rationale: caller-supplied identity is what makes idempotency testable. Retrying
a command with the same identities and the same content must be a no-op, and
that is only expressible if the caller controls the identity. Generation is a
convenience that can be added at the transport edge in M.5 without changing a
single contract.

**Validation.** Identity strings are non-empty after trimming, at most 128 bytes,
and match `[A-Za-z0-9][A-Za-z0-9._:-]*`. `engineering/peos` additionally passes
them to the SDK constructor, which rejects empty values with
`core.ErrEmptyIdentity`; that error is preserved.

**Identity families.** Product-owned: `ProjectID`, `FeatureCardID`,
`AcceptanceRecordID`. Passed through to PEOS: artifact IDs (capability,
requirement, validation plan, evidence, transition record), revision IDs,
`DecisionID`, `ValidationExecutionRecordID`, `ValidationClaimID`,
`StateAssignmentID`.

**Idempotency test.** Every command is executed twice with identical input; the
second execution succeeds and leaves the store byte-identical.

**Conflict test.** Every command is executed twice with the same identities and
differing content; the second fails with `ErrImmutableValueConflict`.

## 2. Time strategy

```
type Clock interface { Now() time.Time }
```

Declared in `internal/application`. One implementation for production
(wall clock), one for tests (`FixedClock`, advancing only when told).

**No test calls `time.Now` directly.** An architecture test asserts that no file
under `internal/` outside the production clock implementation references
`time.Now`.

This is a single interface passed explicitly to command constructors. It is not
a dependency-injection framework, a service locator, or a context value.

| Time | Source |
|---|---|
| Project / FeatureCard creation | `Clock.Now()` |
| Provenance `RecordedAt` | `Clock.Now()` at the recording act |
| Envelope `RecordedAt` | Same instant as the provenance in that act |
| Execution record `CompletedAt` | Command input — it describes when validation actually finished, which is not when it was recorded |
| Claim timestamp | Command input, same reasoning |
| State assignment `EffectiveAt` | Command input, defaulting to `Clock.Now()` |
| Timeline `OccurredAt` | The family's own time (§8) |

All timestamps are normalized to UTC before construction. `core.Timestamp`
provides `Compare`, `Before`, `After`, and `Equal`; comparisons use `Compare` so
that equal instants in different offsets compare equal.

**Correction ordering never uses time.** See §6.

**Equal-timestamp tie-breaking** is by a stable secondary key, specified per
algorithm. No algorithm resolves a tie by "whichever we saw first".

## 3. Commands

Ten commands and two queries. Names follow the intent-oriented form; where they
differ from M.1's use-case list the rename is noted.

| Command | M.1 name | Engineering act |
|---|---|---|
| `CreateProject` | same | Project |
| `CreateFeature` | `CreateFeatureCard` | FeatureCard |
| `EstablishCapabilitySpecification` | `CreateCapabilitySpecification` | Artifact + founding revision + content + order metadata + card link |
| `ReviseCapabilitySpecification` | `CreateCapabilityRevision` | Revision + content + order metadata |
| `AcceptCapabilityRevision` | `AcceptCapabilityRevision` | Acceptance journal entry |
| `EstablishRequirement` | `AddRequirement` | Requirement artifact + revision + order metadata + acceptance entry (AD-019) |
| `RecordArchitectureDecision` | `RecordDecision` | Decision + basis |
| `EstablishValidationPlan` | `CreateValidationPlan` | Plan artifact + plan revision |
| `RecordValidationRun` | `RecordValidationExecution` | Evidence artifact + revision, then execution record |
| `RecordValidationClaim` | `RecordClaim` | Claim |
| `CorrectValidationClaim` | `CorrectClaim` | Claim carrying a correction reference |
| `AssignLifecycleState` | `AssignLifecycleState` | Transition record revision + state assignment |

**Challenged against the M.2 candidate list.** The candidate list omitted
`AcceptCapabilityRevision` and `AssignLifecycleState`. Both are restored:
[FF-004](004-current-state-resolution.md) makes acceptance the input to current-revision
resolution, and M.1 requires lifecycle state resolution and a lifecycle timeline
event. Without them, objectives 15 and 19 of the M.3 objective list are
unreachable.

**Not created:** no command per repository method, no `Update*`, no `Delete*`, no
generic `RecordEngineeringAct`.

### Common command shape

Each command specifies:

- **Type** — a struct of inputs, constructed and validated before execution.
- **Validation** — required fields non-empty; identity format; content
  validation; no repository access.
- **Identities supplied** — by the caller.
- **Identities created** — recorded by this act.
- **Dependencies** — `UnitOfWork`, `Clock`, `EngineeringRecorder`.
- **Repository reads** — what it must check before writing.
- **PEOS construction** — delegated to `EngineeringRecorder`; the command names no
  PEOS type.
- **Transaction** — exactly one `UnitOfWork.Do`.
- **Result** — a typed struct of the keys created.
- **Idempotency** — identical re-execution is a no-op.
- **Errors** — the sentinels it can return.

### 3.1 `EstablishCapabilitySpecification`

The most complex act; the others follow the same pattern.

| | |
|---|---|
| Input | `FeatureCardID`, `CapabilityArtifactID`, `RevisionID`, `CapabilitySpecificationContent` |
| Validation | Content validates per [FF-009 §4.1](009-in-memory-persistence.md#41-capabilityspecificationcontent); identities well-formed |
| Reads | Feature card exists; capability artifact does not already exist with a different type |
| Writes | `ArtifactEnvelope`, `RevisionEnvelope`, structured content, `RevisionOrderMetadata{Sequence: 1}`, feature-card capability link |
| PEOS | `core.NewArtifact`, digest → `core.NewIntegrityIdentity`, `core.NewRepresentationFromContentAddress`, `core.NewArtifactRevision`, `WithRepresentations` |
| Result | `ArtifactKey`, `RevisionKey`, sequence |
| Errors | `ErrNotFound` (card), `ErrImmutableValueConflict`, `ErrInvalidContent`, PEOS sentinels preserved |

Artifact creation and the founding revision are one act, per M.1: PEOS-002
permits an Artifact to precede its first Revision but says such an Artifact must
not be treated as reproducible or validated, and `core.Artifact` retains no
creation provenance of its own.

### 3.2 `ReviseCapabilitySpecification`

Reads the artifact's existing order metadata, computes `max(sequence)+1`, and
writes revision, content, and order metadata in one transaction. Two concurrent
executions must produce *n* and *n+1*, never two of *n* — enforced by the
adapter's write lock plus a uniqueness check on `(ArtifactID, Sequence)`.

### 3.3 `AcceptCapabilityRevision`

Reads the acceptance journal for the revision, validates the transition against
the journal head (§5), and appends one `RevisionAcceptanceRecord`. Writes nothing
else. It records no PEOS value — acceptance is product-owned.

### 3.4 `RecordValidationRun`

Writes the evidence artifact and evidence revision, then the execution record
citing that evidence. One transaction: an execution record whose evidence failed
to write must not exist.

### 3.5 `CorrectValidationClaim`

| | |
|---|---|
| Input | New `ClaimID`, target `ClaimID`, correction kind, and the full claim input |
| Validation | Target ≠ new ID (`ErrCorrectionSelfReference`); kind is one of the three PEOS kinds |
| Reads | Target claim exists and is a claim (`ErrCorrectionTargetMissing`, `ErrCorrectionFamilyMismatch`); the resulting chain has no cycle (`ErrCorrectionCycle`) |
| PEOS | `core.NewRecordCorrectionRef[core.ValidationClaimRef]`, then `Claim.WithCorrection` |
| Errors | The correction family, plus everything `RecordValidationClaim` can return |

Kept separate from `RecordValidationClaim` because it has four invariants that
command does not have. **PEOS v1.0.0 accepts a self-correction** — verified — so
rejecting it is FeatureForge's obligation, not the SDK's.

## 4. Revision ordering algorithm

Resolves the current capability revision. Inputs: the artifact's revision
envelopes, its order metadata, and its acceptance journal.

```
1  envelopes ← RevisionEnvelopeRepository.ListByArtifact(artifactID)
2  order     ← RevisionOrderRepository.ListByArtifact(artifactID)
3  journal   ← RevisionAcceptanceRepository.ListByArtifact(artifactID)

4  for each envelope: require exactly one order entry with the same RevisionKey
       none            → ErrRevisionOrderMissing(key)
       more than one   → ErrRevisionSequenceConflict(key)
5  for each order entry: require its RevisionKey names a stored envelope
       otherwise       → ErrRevisionReferenceMismatch(key)
6  require every Sequence ≥ 1                    else ErrRevisionSequenceInvalid
7  require Sequence values unique within artifact else ErrRevisionSequenceConflict(both keys)
8  acceptance(rev) ← state of the latest journal entry for rev by (EffectiveAt, RecordID);
                     absent → draft
9  accepted ← { rev : acceptance(rev) = accepted }
10 if accepted is empty → return None, rationale "no accepted revision" (NOT an error)
11 top ← max Sequence over accepted
12 candidates ← { rev in accepted : Sequence = top }
13 if |candidates| ≠ 1 → ErrCurrentRevisionAmbiguous(candidates)
14 return the single candidate + ResolutionRationale
```

Step 13 is unreachable given step 7, and is kept as a defensive assertion so a
future branching extension cannot silently return an arbitrary revision.

**Never consulted:** insertion order, `RecordedAt`, revision-ID lexical order.

### `ResolutionRationale`

| Field | Content |
|---|---|
| `Rule` | `"greatest sequence among accepted revisions"` |
| `SelectedKey`, `SelectedSequence` | The winner |
| `Considered` | Every revision key with its sequence and acceptance state |
| `Rejected` | Per rejected revision: key and reason (`not accepted`, `withdrawn`, `lower sequence`) |
| `Warnings` | Non-fatal observations |

The rationale is a value, rendered in the UI in M.5 and asserted in tests now.

## 5. Acceptance semantics — fully resolved

Four concepts, four owners, no overlap:

| Concept | Question | Owner | Cardinality |
|---|---|---|---|
| **Revision sequence** | In what order were revisions recorded? | `RevisionOrderMetadata` | one per revision |
| **Accepted revision** | Which revision text is authoritative now? | acceptance journal | one state per revision |
| **Lifecycle state** | How far has this capability progressed? | PEOS `StateAssignment` | one state per **capability** |
| **Validation readiness** | Do the requirements hold against the current revision? | computed query | one per capability, derived |

### The three options, challenged

| Option | Verdict |
|---|---|
| **A.** `Accepted` boolean on order metadata | **Rejected.** Order metadata is insert-only; a mutable boolean on it contradicts that. And a boolean cannot carry the actor, time, and reason the timeline requires, so a journal would exist anyway — storing both is the same truth twice. |
| **B.** Product-owned append-only acceptance record | **Accepted.** |
| **C.** PEOS lifecycle State Assignment represents acceptance | **Rejected.** Three reasons. *Cardinality:* the lifecycle subject is the capability Artifact, so there is one lifecycle state per capability but acceptance is per revision — representing acceptance this way would need a lifecycle per revision. *Bootstrapping:* every state assignment requires an establishing Transition Record Revision, so accepting a revision would cost three extra PEOS values. *Meaning:* "under-validation" says nothing about which revision text is authoritative. |

Option C would also have made M.1's AD-004 wrong. It is not; it is confirmed and
made precise. Recorded as **AD-015**.

**There is no stored acceptance field.** The state is the journal head. This is
the one place M.3 could have duplicated a truth, and it does not.

## 6. Correction resolution algorithm

Selects the applicable claim for a given subject, scope, and criteria set.

**Time is never used to select.** A correction reference is an explicit,
recorded edge; timestamps are metadata about when someone typed something. Using
`OccurredAt` to pick the "latest" claim would let a backdated record silently
change the current answer, which is the failure mode this project exists to
catch.

The claims form a directed graph: an edge from the correcting claim to its
target. FeatureForge selects the **head** — the claim nothing points at.

```
1  claims ← RecordEnvelopeRepository.ListByKindAndSubject(claim, subjectKey)
2  restrict to claims whose Scope and CriterionKeys equal the requested pair
3  build edges: for each claim c with CorrectionTargetID t
       t not in claims          → ErrCorrectionTargetMissing(c, t)
       t = c.ID                 → ErrCorrectionSelfReference(c)
       t names a non-claim kind → ErrCorrectionFamilyMismatch(c, t)
4  reject cycles by traversal from every node, bounded by |claims|
       cycle found              → ErrCorrectionCycle(nodes)
5  superseded ← { t : some c points at t with kind correct or replace }
   invalidated ← { t : some c points at t with kind invalidate }
6  heads ← claims \ (superseded ∪ invalidated)
7  if |heads| = 0 → return None, rationale naming what invalidated the last claim
                    (a legitimate state, not an error)
8  if |heads| > 1 → ErrCorrectionAmbiguous(heads)
9  return the single head + its chain
```

**Multiple corrections of one target** are not automatically an error at step 3 —
they become two heads, caught at step 8 as `ErrCorrectionAmbiguous`. This is the
correct diagnosis: two people independently corrected the same claim, and a
human must decide.

**Invalidation** removes a claim from the heads without promoting its target. An
invalidating claim is itself evaluated on its own merits, like any other claim.

**Rationale** states the chain in prose — `"C-2 was corrected by C-4; C-4 is not
corrected by anything; C-4 stands"` — plus the full edge list and every rejected
claim with its reason.

## 7. Release readiness

Product-owned, computed, never a PEOS value. Four statuses.

| Status | Condition |
|---|---|
| `ready` | Every effective requirement has an applicable `satisfied` claim whose subject is the **current** capability revision, whose criteria include that requirement's current revision, backed by at least one resolvable evidence reference from an execution whose outcome is `completed` |
| `not-ready` | At least one effective requirement has an applicable claim whose outcome is `not-satisfied` |
| `indeterminate` | At least one applicable claim's outcome is `inconclusive`, or its supporting execution outcome is `interrupted` or `indeterminate` |
| `incomplete` | At least one effective requirement has no applicable claim, or there are no effective requirements at all |

### Precedence — challenged and finalised

**Accepted: `not-ready` > `indeterminate` > `incomplete` > `ready`.**

The M.2 brief proposed `indeterminate` first. That is rejected, for two reasons.

*First*, structural failures are not statuses. A digest mismatch, an ambiguous
revision, a dangling reference, or a correction cycle makes the computation
impossible, and the query **returns an error** (`ErrEngineeringStateIndeterminate`
wrapping the specific cause) rather than a status. So `indeterminate` covers only
*semantic* indeterminacy — an inconclusive claim, or an execution that did not
complete — where the computation succeeded and the answer is genuinely unclear.

*Second*, given that narrowing, a definitive `not-satisfied` is stronger
information than an inconclusive. If R2 is proven unsatisfied while R3 is
inconclusive, the capability is not ready — and saying `indeterminate` would
mask a certain negative behind an uncertain one. A negative signal is never
masked by a weaker one.

This also resolves M.1 [FF-004 §3.6](004-current-state-resolution.md#36-release-readiness),
which listed five statuses including both `inconclusive` and `undetermined`.
Those merge into `indeterminate`; "no effective requirements" becomes
`incomplete`; "current revision unresolvable" becomes an error. Recorded as
**AD-016**.

### Two obligations, discharged

- A claim backed only by an execution whose outcome is `interrupted` or
  `indeterminate` never contributes to `ready`. This discharges PEOS-006's
  requirement that such an outcome is never silently treated as completed.
- A claim whose subject is an **earlier** capability revision does not satisfy
  the current one. It is reported in the rationale as stale, naming the sequence
  it was evaluated against.

### Result

`ReadinessResult{ Status, PerRequirement[], Rationale }`, where each
`PerRequirement` entry carries the requirement key, its current revision key, the
selected claim, its outcome, the supporting execution keys, the evidence keys,
every rejected or superseded claim with a reason, and the verdict reason.

## 8. Lifecycle state

### Vocabulary

Four states, in the `featureforge` namespace:

| State | Meaning |
|---|---|
| `drafting` | The capability exists; specification work is under way |
| `specified` | An accepted revision and at least one requirement exist |
| `under-validation` | A validation plan exists and execution has begun |
| `assessed` | Validation has been executed and assessed |

**`assessed` replaces M.1's `validated`.** M.1 defined `validated` as "a satisfied
claim stands", which is release readiness wearing a lifecycle costume — the exact
duplication this phase was told to prevent. `assessed` means the assessment
happened; it says nothing about the outcome.

**A capability can be `assessed` and still `not-ready`.** That is precisely the
end state of the canonical scenario, and a test asserts the pair. Recorded as
**AD-018**.

### Structure

One `lifecycle.Definition`, one `DefinitionVersion`, fixed as configuration and
recorded once. Subject: the capability **Artifact**. No Guard, Effect, or Trigger
expressions — PEOS defers the expression language and FeatureForge does not
invent one. This is what keeps a workflow engine out of the project.

### The entry-transition problem, and its resolution

PEOS-003 says a Subject "enters a lifecycle through an entry Transition **from an
unassigned condition**". But `lifecycle.NewTransitionRecordContent` requires a
non-zero `fromAssignment`, and **rejects a zero one** — verified against
v1.0.0, which returns *"source state assignment must not be zero"*.

So the SDK cannot express an entry Transition Record: the first assignment needs
a source assignment that by definition does not exist.

`lifecycle.NewStateAssignment`, however, accepts `establishedBy` as a bare
`core.ArtifactRevisionRef` and performs no lookup — it explicitly documents that
it does not verify the reference identifies a real Transition Record Revision.
The PEOS lifecycle example itself relies on this, citing `TR-9001/REV-0` as the
establishing revision of its source assignment without constructing it.

**Accepted resolution.** The entry assignment is established by a plain
`core.ArtifactRevision` of the Transition Record Artifact that carries **no**
`TransitionRecordContent`. Every subsequent transition uses a full
`TransitionRecordRevision` whose `fromAssignment` is the previous assignment.

This is conformant — PEOS-002 permits an ordinary Artifact Revision of a
Transition Record Artifact, and PEOS-003's requirement that initial assignment be
"recorded by its Transition Record" is met by a revision of that record. It is
recorded as **AD-014**, and reported to the PEOS backlog as a consumer finding in
M.7. **No PEOS change is made or required.**

### Resolution algorithm

```
1  assignments ← RecordEnvelopeRepository.ListByKindAndSubject(state-assignment, capabilityArtifactKey)
2  reject any whose definition version ≠ the configured one → ErrUnknownDefinitionVersion
3  if empty → return None with rationale
4  top ← max OccurredAt (EffectiveAt)
5  candidates ← assignments at top
6  if |candidates| > 1 and they name different StateIDs → ErrAmbiguousLifecycleState(candidates)
7  if |candidates| > 1 and they name the same StateID → select lowest RecordID, note the duplicate
8  return the assignment + rationale
```

The tie-break at step 7 applies only where the state is identical, which is a
harmless duplicate. Differing states at the same instant is a real conflict and
fails.

## 9. Timeline read model

Computed on every request, never stored.

### `TimelineEvent`

| Field | Source |
|---|---|
| `EventID` | `kind + ":" + SourceIdentity` |
| `FeatureCardID` | The card the query was scoped to |
| `EventKind` | §9.1 |
| `OccurredAt` | The family's own time; may be absent |
| `Actor` | Projected provenance or execution actor; may be absent |
| `Label` | Human-readable, with record values interpolated |
| `Summary` | Kind-specific detail: outcome, sequence, decision statement |
| `SourceIdentity` | The underlying record's identity string |
| `References` | Typed links to every record the event names |
| `Corrected` | For a correcting claim: the target's identity; empty otherwise |
| `Rationale` | Why the event is placed where it is, and which tie-break applied |

### 9.1 Closed `EventKind` set

`project.created`, `feature.created`, `capability.created`, `capability.revised`,
`capability.accepted`, `capability.withdrawn`, `requirement.revised`,
`decision.recorded`, `plan.revised`, `execution.recorded`, `evidence.recorded`,
`claim.recorded`, `claim.corrected`, `lifecycle.transitioned`.

`claim.corrected` is a display specialization of `claim.recorded`, not a second
event: one claim produces exactly one event.

### 9.2 Source mapping

| Stored family | Event kind |
|---|---|
| `Project` | `project.created` |
| `FeatureCard` | `feature.created` |
| `ArtifactEnvelope`, type `featureforge:product-capability` | `capability.created` |
| `RevisionEnvelope`, family `capability` | `capability.revised` |
| `RevisionAcceptanceRecord`, state `accepted` / `withdrawn` | `capability.accepted` / `capability.withdrawn` |
| `RevisionEnvelope`, family `requirement` | `requirement.revised` |
| `RecordEnvelope`, kind `decision` | `decision.recorded` |
| `RevisionEnvelope`, family `validation-plan` | `plan.revised` |
| `RecordEnvelope`, kind `execution` | `execution.recorded` |
| `RevisionEnvelope`, family `evidence` | `evidence.recorded` |
| `RecordEnvelope`, kind `claim`, no correction | `claim.recorded` |
| `RecordEnvelope`, kind `claim`, with correction | `claim.corrected` |
| `RecordEnvelope`, kind `state-assignment` | `lifecycle.transitioned` |

`ArtifactEnvelope`s for requirement, plan, and evidence artifacts produce **no**
event: their revisions carry the meaning, and an artifact-creation event
alongside its founding revision would double every entry.

### 9.3 Ordering

```
(OccurredAt asc, KindRank asc, SourceIdentity asc)
```

`KindRank` is a fixed table ordering events that share an instant into causal
order: created → revised → accepted → requirement → decision → plan → execution
→ evidence → claim → lifecycle. It never reorders events with distinct
timestamps. `SourceIdentity` is the final tie-break — lexicographic, total, and
stable, so the order always terminates the same way.

### 9.4 Ambiguity

| Condition | Behaviour |
|---|---|
| Equal `OccurredAt` | Ordered by `KindRank` then `SourceIdentity`; both events' rationale names the tie-break |
| Missing timestamp | Placed in a separate `Undated` group returned alongside the dated slice and flagged. Never at epoch, never silently last. |
| Unresolvable reference | `ErrTimelineSourceInvalid`, naming both records. The timeline is not rendered with a broken link. |
| Duplicate `EventID` | `ErrTimelineSourceInvalid`. Unreachable given §9's identity rule; kept as a defensive assertion. |
| `interrupted` / `indeterminate` execution outcome | Rendered verbatim; never normalized |
| Corrected claim | Its own event is unchanged and stays in place; the correcting claim's event carries the link |

**No source value is mutated** while building the timeline. The query takes
copies and returns a fresh slice.

## 10. Queries

| Query | Returns |
|---|---|
| `GetFeatureEngineeringState` | Current revision + rationale; effective requirements; applicable decisions; current claims per requirement; readiness; lifecycle state — each with its own rationale |
| `GetFeatureTimeline` | Ordered dated events, plus the undated group |

Both are read-only, run inside a single `UnitOfWork.Do` for a consistent
snapshot, and write nothing. Both are deterministic: repeated calls on unchanged
data return byte-identical results.
