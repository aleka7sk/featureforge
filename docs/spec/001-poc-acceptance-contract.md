# FF-001 — POC Acceptance Contract

Status: Accepted (Phase M.1)
Governs: application use cases, minimal user experience, the AI boundary, the
binding acceptance contract, and the exit criteria that freeze this project.

This document is the contract. FeatureForge is successful only when every
statement in [§6](#6-acceptance-contract) is demonstrated, and it stops the
moment [§7](#7-exit-criteria) is met.

## 1. Application use cases

Use cases are **intent-oriented**, not CRUD-oriented. Each one is a single
engineering act and a single transaction boundary
([FF-003 §7](003-peos-integration.md#7-persistence-contract)). A use case exists
only if it has its own invariants; otherwise it is a parameter of another.

| Use case | Intent | Transaction writes |
|---|---|---|
| `CreateProject` | Establish a naming container | Project |
| `CreateFeatureCard` | Establish the human entry point for a capability | FeatureCard |
| `CreateCapabilitySpecification` | Begin engineering a capability — creates the Artifact **and** its founding revision | Artifact, Revision 1, specification content, sequence 1, FeatureCard link |
| `CreateCapabilityRevision` | Record a new immutable state of the specification | Revision, specification content, sequence *n* |
| `AcceptCapabilityRevision` | Declare which revision text is authoritative | Acceptance journal entry |
| `AddRequirement` | Record an engineering obligation traced to one exact capability criterion | Requirement Artifact when founding, Requirement Revision, revision order, semantic acceptance member, Requirement Criterion Trace |
| `RecordDecision` | Record a decision with its basis | Decision, basis evidence links |
| `CreateValidationPlan` | Declare what will be validated and how | Plan Artifact when founding, Plan Revision with activities, revision order, semantic acceptance member |
| `RecordValidationExecution` | Record that an activity ran, with its evidence and outcome | Execution Record, evidence Artifact + Revision |
| `RecordClaim` | Assert an evaluated outcome | Claim |
| `CorrectClaim` | Assert a new outcome that corrects an earlier claim | Claim with correction reference |
| `AssignLifecycleState` | Record a lifecycle entry or transition | Shared Transition Record Artifact when founding, Transition Record Revision, State Assignment; exact conditional writes are governed by FF-010/FF-022/FF-023 |
| `ResolveFeatureEngineeringState` | Answer "where does this stand" | Nothing — read-only |
| `GetFeatureTimeline` | Answer "how did it get here" | Nothing — read-only |

### Challenged: the candidates that did not survive as independent use cases

| Candidate | Verdict |
|---|---|
| `CreateCapabilitySpecification` separate from creating Revision 1 | **Merged.** PEOS-002 permits an Artifact to exist before its first Revision but says such an Artifact must not be treated as reproducible or validated, and `core.Artifact` retains no creation-time provenance of its own. Creating them together makes the founding Revision's provenance the creation record, and removes an unusable intermediate state. |
| `RecordEvidence` as its own use case | **Merged into `RecordValidationExecution` for validation evidence.** PEOS models produced evidence as `ExecutionRecord.ProducedEvidence()` — one act, one transaction. C8 is the narrow exception: it stores only an exact Decision-basis evidence citation, which may be unresolved under AD-030/FF-022. The canonical Decision forward-cites `EV-1/EV-1-REV-1`; the later A-1 C10 act materialises that same pair, so no standalone Evidence write is needed. |
| `RecordResult` | **Rejected.** There is no Result construct ([FF-003 §3](003-peos-integration.md#3-result-is-not-a-new-construct--resolved)). The execution outcome belongs to `RecordValidationExecution`; the claim outcome belongs to `RecordClaim`. |
| `CorrectClaim` separate from `RecordClaim` | **Kept separate.** It has a distinct invariant `RecordClaim` does not: the correction target must exist, must be a claim, must not create a cycle, and the correction kind must be one of the three PEOS defines. Folding it in as an optional parameter would hide that validation. |
| `UpdateFeatureCard` | **Superseded for this POC by AD-031.** Operational state is distinct from engineering state, but FeatureForge keeps Project and FeatureCard establishment fields stable and implements only the one-time capability link. Belcanto must choose its own edit, audit, concurrency, and replay contract. |
| `AddRequirement` vs. `ReviseRequirement` | **One use case.** Both record a Requirement Revision; the difference is whether the Requirement Artifact already exists. Two use cases would duplicate every invariant. |
| A generic `RecordEngineeringAct` | **Rejected.** That is a workflow engine, which [FF-000](000-product-overview.md) forbids. |

### Invariants every write use case enforces

1. Every mandatory internal reference names a record that exists. C8's
   structurally valid Decision-basis evidence pair is the sole governed
   external/forward-citation exception (AD-030/FF-022); execution, claim,
   correction, trace, lifecycle, and other aggregate references remain
   resolvable at write time.
2. Every PEOS value is constructed through its SDK constructor — no value is
   assembled by unmarshalling hand-written JSON.
3. Provenance carries the configured actor and a recorded timestamp.
4. The whole act commits or none of it does.
5. No derived state is written.

## 2. Data flow constraints

- The application layer never imports the PEOS SDK
  ([FF-002 §4](002-domain-boundaries.md#4-package-layering)).
- A use case returns product-shaped output. A PEOS value never leaves the
  integration layer.
- Derived read use cases return a result **and** its rationale. Direct
  inventory and stored-value reads (Q1 projects, Q2 feature cards, and Q7 one
  exact revision) return the authoritative stored values and do not invent a
  derivation rationale.
  ([FF-004 §3](004-current-state-resolution.md#3-current-state-queries)).

## 3. Minimal user experience

Enough UI to prove a person can understand the state and the history. No visual
system, no authentication, no organization management, no billing, no
notifications, no collaboration. One local user.

### 3.1 Projects

| | |
|---|---|
| Goal | Find or create the project I am working in |
| Data | Project list; feature card count per project |
| Actions | Create project; open project; create feature card |
| Current state | None — this screen has no engineering state |
| History | None |

### 3.2 Feature overview

| | |
|---|---|
| Goal | Understand where this feature stands, in one screen |
| Data | Card title and summary; current revision title and sequence; effective requirement count; release readiness with its rationale; current lifecycle state; last five timeline events |
| Actions | Create the capability specification if absent; navigate to every other screen |
| Current state | Readiness outcome shown **with** the per-requirement rationale table, never as a bare badge |
| History | Recent events, with a link to the full timeline |

If readiness is `not-ready`, this screen says which requirement failed and which
claim says so. A colour alone is not an explanation.

### 3.3 Revisions

| | |
|---|---|
| Goal | Read what was specified, and what changed |
| Data | All revisions with sequence, acceptance state, recorded-at, actor; full specification content per revision; which one is current and why |
| Actions | Create a revision; accept a revision; withdraw a revision |
| Current state | The current revision is marked, with the resolution rationale shown verbatim |
| History | Every revision remains readable, including withdrawn ones. Nothing is hidden. |

Revision 1 must be as readable after Revision 2 exists as it was before. This
screen is where that is demonstrated to a human.

### 3.4 Requirements

| | |
|---|---|
| Goal | See what must be true, and whether it is |
| Data | Each requirement's current revision and statement; the exact source capability revision and revision-local acceptance criterion from its stored trace; its current claim and outcome |
| Actions | Add a requirement; revise a requirement |
| Current state | Per requirement: current revision, current claim, outcome |
| History | Prior requirement revisions readable |

### 3.5 Decisions

| | |
|---|---|
| Goal | Understand why the specification says what it says |
| Data | Question, outcome statement, rationale, subjects, and the full basis — evidence, assumptions, constraints, uncertainties |
| Actions | Record a decision |
| Current state | All decisions naming this capability, in recorded order |
| History | Decisions are immutable; the list is the history |

The basis is displayed, not collapsed. A decision without a visible basis is
indistinguishable from an opinion.

### 3.6 Validation

| | |
|---|---|
| Goal | See what was planned, what ran, what it produced, and what is claimed |
| Data | Plan revision and its activities; execution records with outcomes and evidence; claims with outcomes, criteria, reasoning, and correction links; the readiness rationale table |
| Actions | Create a plan revision; record an execution; record a claim; correct a claim |
| Current state | Per requirement: the current claim and how the correction chain reached it |
| History | Superseded claims shown inline, visibly marked, with their outcome intact and a link to the claim that corrected them |

A corrected claim is **shown**, not hidden. Hiding it would make the UI lie about
what the store contains.

### 3.7 Timeline

| | |
|---|---|
| Goal | Read the engineering history end to end |
| Data | Every event: label, actor, timestamp, references, detail, rationale ([FF-006](006-timeline-read-model.md)) |
| Actions | Filter by kind; open any referenced record |
| Current state | None — the timeline is history, not state |
| History | Complete, including the `undated` group if any event lacks a timestamp |

### Usability acceptance

A reader who has not seen the codebase must be able to answer, from the UI alone:
what this feature is; what the current specification says; what must be true for
it to ship; whether it is ready and why not; what was decided and on what basis;
what was validated, when, by whom, and with what evidence; and what was corrected
and why.

## 4. AI boundary

One optional, bounded capability, specified now and **implemented in M.6, not in
the first vertical slice**:

> Propose a new capability revision from the current revision, the effective
> requirements, and open validation findings.

### Contract

**Input — the context pack.** A derived, read-only value assembled by
FeatureForge:

- the current capability revision's exact reference and its full specification
  content;
- every effective requirement's exact revision reference and statement plus its
  exact stored source capability revision and revision-local acceptance-
  criterion key;
- every current claim with its outcome, criteria, and reasoning — including
  `not-satisfied` and `inconclusive` ones;
- the applicable decisions with their outcome statements;
- the open questions and uncovered acceptance criteria.

Every element carries the **exact revision or record reference it came from**. A
context pack that cannot name its sources is invalid and is not sent.

**Output — the proposal.** Proposed specification content in the same field shape
as a real revision ([FF-003 §5](003-peos-integration.md#5-revision-content-ownership)),
plus a prose rationale, plus the list of source references it used.

### Hard limits

1. **No repository write access.** The AI component receives a value and returns
   a value. It is not given a repository, a transaction, or a use case.
2. **No authoritative PEOS state is ever created automatically.** A proposal is
   not a revision. It is not stored as engineering state.
3. **Explicit human acceptance is required.** A proposal is displayed; the user
   accepts or rejects it. Only acceptance invokes `CreateCapabilityRevision`, and
   that revision's provenance records the human actor and an AI-assisted method
   value, so the assistance is visible in the record rather than hidden.
4. **Provenance is preserved.** The accepted revision's origin note names the
   proposal and the exact source revisions it was derived from.
5. **A rejected proposal leaves no engineering state.** It may be discarded
   entirely.

The demonstration required by [§6](#6-acceptance-contract) is that the context
pack can be generated and that the boundary holds. An actual model call is
optional; a stub proposal generator satisfies the contract, because the contract
being proven is the authority boundary, not the model.

## 5. What "done" means for each concern

| Concern | Owned by | Settled in |
|---|---|---|
| Domain boundary | FeatureForge | [FF-002](002-domain-boundaries.md) |
| Revision ordering semantics | FeatureForge | [FF-004 §2](004-current-state-resolution.md#2-the-revision-ordering-contract) |
| Current-state ownership | FeatureForge | [FF-004 §3](004-current-state-resolution.md#3-current-state-queries) |
| Persistence authority | FeatureForge | [FF-003 §7](003-peos-integration.md#7-persistence-contract) |
| Correction semantics | PEOS constructs, FeatureForge resolution | [FF-004 §3.4](004-current-state-resolution.md#34-latest-non-corrected-claim), [FF-005 §7](005-validation-scenario.md#7-the-correction-flow) |
| Timeline ownership | FeatureForge | [FF-006](006-timeline-read-model.md) |
| AI authority boundary | FeatureForge | §4 above |

None of these is left to an implementation agent.

## 6. Acceptance contract

FeatureForge is successful only when **all** of the following are demonstrated.

### 6.1 Architecture

- [x] The PEOS SDK is unchanged — no file in the PEOS module is modified, and no
      `replace` directive points at a local PEOS checkout.
- [x] The FeatureForge domain does not import PEOS, directly or transitively.
- [x] Product vocabulary lives outside PEOS: every FeatureForge vocabulary value
      is in the `featureforge` namespace, and no value is constructed in the
      `peos` namespace.
- [x] No PEOS type is copied, restated, or shadowed inside FeatureForge.
- [x] Package boundaries are enforced by tests that fail the build, not by
      convention ([FF-002 §4](002-domain-boundaries.md#enforcement)).

### 6.2 History

- [x] Artifact Revision 1 remains fully inspectable after Revision 2 exists, with
      identical content and identical provenance.
- [x] No immutable record is ever updated or deleted — proven by an adapter-level
      test asserting that no update or delete path exists for engineering
      records.
- [x] A correction creates new history: the corrected claim is unchanged, the
      correcting claim references it, and both are readable.
- [x] The timeline explains every engineering act in the scenario.

### 6.3 Persistence

- [x] Every PEOS value the scenario uses persists and reloads.
- [x] JSON round trips preserve values — reloaded values equal the originals, and
      canonical JSON is byte-identical.
- [x] A duplicate identical write is idempotent.
- [x] A conflicting immutable write fails with a distinguishable error.
- [x] Every mandatory internal reference remains resolvable; C8's exact
      Decision-basis evidence citation is the sole governed unresolved-citation
      exception.
- [x] PostgreSQL integration works, passing the same contract test suite as the
      in-memory adapter.

### 6.4 Queries

- [x] The current revision is resolved deterministically.
- [x] Insertion order does not affect resolution — proven by inserting the same
      records in several orders and asserting identical output.
- [x] Ambiguous histories fail explicitly, naming the conflicting records.
- [x] The current claim follows correction chains, including invalidation and
      cycle rejection.
- [x] Every derived query result includes its rationale; direct stored-value
      reads Q1, Q2, and Q7 do not invent one.

### 6.5 Application

- [x] One complete canonical feature lifecycle works through the API and the UI.
- [x] A person who has not seen the code can understand the current state and the
      history from the UI alone ([§3](#3-minimal-user-experience)).
- [x] No operational Belcanto entity is required or present.
- [x] No generic workflow engine is introduced — no state machine over
      user-defined transitions, no rule engine, no expression evaluator.

### 6.6 AI

- [x] A context pack can be generated for the canonical feature.
- [x] Every element of the context pack names its exact source reference.
- [x] An AI proposal cannot become authoritative without explicit human
      acceptance — proven by a test asserting the proposal path has no write
      access.
- [x] Provenance is preserved on an accepted proposal.

### 6.7 Transition to Belcanto

- [x] Reusable patterns are documented, as patterns and rationale, not as a
      library.
- [x] FeatureForge-specific code is not treated as shared infrastructure.
- [x] No shared PEOS integration package is created prematurely. FeatureForge's
      integration layer stays inside FeatureForge. Belcanto will write its own,
      informed by this one; extracting a library from a single consumer would
      generalize from a sample of one.
- [x] Lessons learned identify what Belcanto should reuse and what it should
      redesign.

## 7. Exit criteria

FeatureForge is **frozen** when all of the following hold:

1. One canonical lifecycle passes end to end.
2. PostgreSQL persistence is verified against the same contract suite as the
   in-memory adapter.
3. Current-state resolution is verified, including insertion-order independence
   and explicit ambiguity failure.
4. Correction is verified, with history intact.
5. The timeline is understandable to a reader who has not seen the code.
6. Package-boundary tests pass.
7. The serialization contract passes — round trip, idempotence, conflict.
8. The AI context-pack boundary is demonstrated.
9. An independent consumer audit (M.7) finds no BLOCKER and no MAJOR finding.

On freeze, development stops. FeatureForge does not continue into multi-tenancy,
production authentication, collaboration, notifications, generalized project
management, or commercial deployment.

The next project is **Belcanto Product**.

### Freeze artifacts

Three documents are produced at freeze and are the entire handover:

| Artifact | Content |
|---|---|
| Audit report | The M.7 findings and their disposition |
| Patterns document | What worked, stated as patterns with rationale — not as code to import |
| Lessons learned | What Belcanto should reuse, and what it should redesign and why |
