# FF-003 — PEOS Integration

Status: Accepted (Phase M.1)
Governs: which FeatureForge concepts are represented in PEOS, the FeatureForge
vocabulary, revision content ownership, and the persistence contract.

Prerequisite reading: [FF-002](002-domain-boundaries.md) for the layering rules
this document operates inside.

## 1. What PEOS gives us, and what it deliberately does not

PEOS is a value model. It has no database, no query language, and no notion of
"current". Everything in this section follows from that.

PEOS owns: identity, provenance, origin, integrity, references, representations,
and the immutability of every recorded engineering value.

FeatureForge owns: persistence, ordering, current-state resolution, the timeline,
structured specification content, and every operational concern.

PEOS-002 states that an Artifact Revision carries no revision number, no
predecessor link, and no status field, and that revision ordering is
Product-defined. PEOS-006 states that there is no separate Verdict entity.
Both are load-bearing for [FF-004](004-current-state-resolution.md).

## 2. Concept-by-concept mapping

Every concept in scope was assessed for whether it is represented in PEOS, and
if so, as what.

| Concept | Representation | Notes |
|---|---|---|
| Capability specification | `core.Artifact`, Artifact Type `featureforge:product-capability` | A plain PEOS Artifact with a FeatureForge-owned type. It is **not** a `requirement.Requirement`; that type demands the PEOS Requirement Artifact Type. |
| Capability revision | `core.ArtifactRevision` + FeatureForge Specification Content | §5 |
| Requirement | `requirement.Requirement` + `requirement.Revision` | PEOS-owned Artifact Type. |
| Requirement criterion trace | `engineering.RequirementCriterionTrace` | FeatureForge-owned insert-only metadata binding a Requirement revision to an exact capability revision and criterion key. It is deliberately separate from the PEOS Requirement payload and is neither a `relation.Relation` nor a PEOS `Extension`. |
| Decision | `decision.Decision` | Carries its own `core.DecisionID`, subjects, question, outcome, and provenance. |
| Decision basis | `decision.Basis` on the Decision | Cites evidence as `core.EvidenceArtifactRevisionRef`; may carry assumptions, constraints, uncertainties. |
| Validation plan | `validation.Plan` + `validation.PlanRevision` + `PlanContent` + `PlannedActivity` | Activities are Revision-owned values with plan-local keys. |
| Execution record | `validation.ExecutionRecord` | Immutable, own identity, never revisioned. |
| Evidence | `core.ArtifactRevision` carrying the `core.ArtifactRoleEvidence` role, cited as `core.EvidenceArtifactRevisionRef` | Evidence is always cited at the exact Revision level. |
| Result | **Not a distinct construct** | See §3. |
| Claim | `validation.Claim`, Claim Type `core.ClaimTypeSatisfaction` | Subject is the capability revision; criteria are the requirement revisions. |
| Lifecycle assignment | `lifecycle.Definition`, `DefinitionVersion`, `TransitionRecord`, `TransitionRecordRevision`, `StateAssignment` | §4 |
| Correction reference | `core.RecordCorrectionRef[core.ValidationClaimRef]` via `Claim.WithCorrection` | The new claim points backward; the old one is untouched. |
| Provenance | `core.Provenance` on every revision and record | Actor is the fixed local identity; `RecordedAt` is always set. |
| Project | **FeatureForge only** | No PEOS representation. |
| FeatureCard | **FeatureForge only** | No PEOS representation. |
| Timeline | **FeatureForge only** | A computed read model. [FF-006](006-timeline-read-model.md). |
| Release readiness | **FeatureForge only** | A computed query. [FF-004](004-current-state-resolution.md). |

### Deliberately not used in the POC

| PEOS construct | Why not |
|---|---|
| `decision.Record` | A Decision Record is an Artifact wrapping a Decision reference, useful when a decision must be superseded or related as an Artifact. The scenario has one decision and never supersedes it. Adding it now would be an abstraction ahead of need. |
| `relation.Relation` | The scenario's relationships are all expressed by the constructs that already carry them (a Claim cites its criteria; a Decision names its subjects). No free-standing relation is required, and PEOS assigns graph traversal to a future Traceability Model. |
| `quality`, `runtime`, `template` packages | Nothing in the canonical scenario is a quality profile, a runtime contract, or a template. |
| `requirement` relationship wrappers (Derivation, Refinement, Decomposition, Dependency, Conflict, Supersession) | The scenario has independent requirements. |

Each of these becomes justified only if the scenario changes, and that would be
a recorded decision.

## 3. "Result" is not a new construct — resolved

The lifecycle in [FF-000](000-product-overview.md) lists "produce a result and
claim". This is the single most likely place to accidentally invent a type PEOS
already forbids. PEOS-006 is explicit that there is no separate Verdict entity.

"Result" therefore maps onto two existing, distinct things:

| Question | PEOS value | Vocabulary |
|---|---|---|
| Did the activity run, and how did it conclude? | `ExecutionRecord.Outcome()` — a `core.ExecutionOutcome` | completed / failed / interrupted / indeterminate |
| What was determined about the subject? | `Claim.Outcome()` — a `core.ClaimOutcome` | satisfied / not-satisfied / inconclusive |

These vocabularies never share values. A completed execution may carry a
not-satisfied claim; that is normal, not an error. An indeterminate or
interrupted execution outcome must never be silently treated as completed —
PEOS-006 places that obligation on the consumer, and
[FF-004](004-current-state-resolution.md) discharges it explicitly.

No FeatureForge type named `Result`, `Verdict`, or `Status` is created.

## 4. Lifecycle: in scope, and what it costs

Lifecycle is in scope because [FF-006](006-timeline-read-model.md) must show a
lifecycle transition and [FF-004](004-current-state-resolution.md) must resolve a
current lifecycle state. It is the most expensive PEOS construct in the POC,
because a State Assignment requires a Transition Record Revision to establish it.

Scope is held down as follows:

- **One** Lifecycle Definition, with **one** Definition Version, fixed by
  FeatureForge and persisted through dedicated opaque-payload repositories by
  AD-032's explicit startup initialization. C6 and queries read and validate
  that stored policy; they never silently synthesize it.
- Four states: `featureforge:drafting`, `featureforge:specified`,
  `featureforge:under-validation`, `featureforge:assessed`.

  > **Corrected in M.2 by [AD-018](../decisions/README.md#ad-018--the-lifecycle-state-validated-is-renamed-assessed-and-redefined).**
  > This document originally named the last two states `validating` and
  > `validated`, defining `validated` as "a satisfied claim stands". That made
  > lifecycle a second, staler copy of release readiness — the duplication the
  > two concepts were separated to avoid. `assessed` means validation was
  > executed and assessed, and says nothing about the outcome: a capability can
  > be `assessed` and still `not-ready`.
- The Lifecycle Subject is the capability **Artifact** (identity level), not a
  revision — the capability progresses, not any single revision of it.
- The scenario exercises the transitions it needs and no more.
- No Guard, Effect, or Trigger expressions. PEOS defers the expression language,
  and FeatureForge does not invent one. This is what keeps a workflow engine out
  of the project.

Lifecycle state and revision acceptance are **different questions** and must not
be merged: lifecycle answers "how far has this capability progressed", revision
acceptance answers "which revision text is authoritative right now". Recorded as
**AD-004**.

## 5. Revision content ownership

### The problem

A capability revision needs structured product content: title, problem
statement, user outcome, functional behaviour, constraints, acceptance criteria,
dependencies, open questions. PEOS's `core.ArtifactRevision` has no field for
any of it, and rightly so — that content is entirely product-specific.

Three options were weighed.

| Option | Verdict |
|---|---|
| Put the content in `core.Extension` | **Rejected.** Extension is an opaque namespaced JSON container with no schema, no validation, and no queryability. Burying the specification body there makes it unreadable to the persistence layer, untestable as a schema, and exactly the "uncontrolled Extension JSON blob" this project was told to avoid. |
| Serialize the content into an inline `Representation` and treat that as the store | **Rejected as the primary store.** It would make the authoritative content a byte blob nested inside the revision's JSON, so every read of a title requires decoding the whole revision, and no typed projection is possible. |
| Store the content as a FeatureForge-owned record, linked by exact Artifact Revision reference | **Accepted.** |

### The accepted model

**PEOS owns identity, origin, provenance, integrity, and representations.
FeatureForge owns the structured specification content. They are bound by an
exact `core.ArtifactRevisionRef` and by a digest.**

For each capability revision, FeatureForge records:

1. A `core.ArtifactRevision` with the capability's Artifact ID, a fresh Revision
   ID, an `Origin`, a `Provenance` (actor + recorded-at), and a mandatory
   `IntegrityIdentity`.
2. A FeatureForge **Specification Content** record keyed by the exact
   `(ArtifactID, ArtifactRevisionID)` pair.

The binding is made verifiable rather than merely referential:

- The Specification Content is serialized to a canonical JSON form —
  field order fixed, no insignificant whitespace, UTF-8, deterministic — and
  hashed with SHA-256.
- That digest is what the revision's `IntegrityIdentity` records, using
  mechanism `core.IntegrityMechanismContentAddressedReference` and protected
  scope `core.IntegrityProtectedScopeContent`.
- The revision additionally carries one authoritative `Representation`
  constructed from the same content address, with media type
  `featureforge:specification-content` and role
  `core.RepresentationRoleAuthoritative`.

The consequence is the property the POC needs: if the stored specification
content is ever altered, its recomputed digest no longer matches the digest
recorded in the immutable PEOS revision, and the mismatch is detectable. The
link is not a bare foreign key.

`core.Extension` is **not used at all** in the POC — not for specification
content, not for anything else. Introducing an Extension payload is a recorded
decision, and it must come with a declared namespace and a validated schema.

### Specification Content fields

All fields are product-owned. `title` and `problem_statement` are required;
the rest may be empty.

| Field | Shape |
|---|---|
| `title` | text, required |
| `problem_statement` | text, required |
| `user_outcome` | text |
| `functional_behaviour` | ordered list of text |
| `constraints` | ordered list of text |
| `acceptance_criteria` | ordered list of `{key, text}`, keys unique within the revision |
| `dependencies` | ordered list of text |
| `open_questions` | ordered list of text |

Acceptance criteria carry a stable key so a Requirement and a Planned Activity
can cite one precisely. The key is local to the revision; it carries no global
identity, exactly as PEOS treats plan-local activity keys.

Ordered lists are ordered: their order is content, it participates in the
digest, and it is preserved on read.

## 6. FeatureForge vocabulary

FeatureForge owns the `featureforge` namespace and nothing else. PEOS owns
`peos`. FeatureForge never constructs a value in the `peos` namespace; it uses
the PEOS-provided constants, and an architecture test enforces this.

PEOS vocabulary values are namespaced as `namespace:value`, split on the first
colon.

### The five proposed values, assessed

| Proposed | Verdict | Assignment |
|---|---|---|
| `featureforge:product-capability` | **Accept** | **Artifact Type.** The capability specification Artifact. |
| `featureforge:feature-specification` | **Reject** | A synonym for the value above. Two names for one Artifact Type guarantees that half the code uses each. Collapsed into `featureforge:product-capability`. |
| `featureforge:requirement-satisfaction` | **Reject** | This is a Claim Type, and `core.ClaimType` is the one vocabulary family PEOS treats as **closed**. `core.ClaimTypeSatisfaction` already means exactly this. Declaring a product value here would be a direct ontology violation. |
| `featureforge:release-readiness` | **Accept, reclassified** | **Product-only terminology**, naming a computed query — *not* a PEOS value. See the note below. |
| `featureforge:validation-report` | **Accept** | **Representation media type**, used on the Representation of an evidence Artifact Revision. |

**On release readiness.** The tempting move is to make it a `core.ProductRuleRef`
used as a Claim criterion, so readiness becomes an assertable Claim. That was
rejected for the POC: readiness in FeatureForge is *derived* from requirement
coverage and current claims, and recording a derived verdict as an authoritative
Claim is precisely what [§7](#7-persistence-contract) forbids. Release readiness
is a computed query with a rationale, defined in
[FF-004](004-current-state-resolution.md). Recorded as **AD-008**.

### Complete `featureforge` vocabulary

| Value | Kind | Used as |
|---|---|---|
| `featureforge:product-capability` | Artifact Type | The capability specification Artifact |
| `featureforge:validation-evidence` | Artifact Type | An Artifact whose revisions serve the Evidence role |
| `featureforge:specification-content` | Media type | Representation of a capability revision |
| `featureforge:validation-report` | Media type | Representation of an evidence revision |
| `featureforge:manual-review` | Validation Method | A human review activity |
| `featureforge:manual-inspection` | Validation Method | A human inspection activity |
| `featureforge:capability` | Scope kind | Scope of a Plan, a Claim, and a Lifecycle Definition Version |
| `featureforge:capability-lifecycle` | Lifecycle subject type | Declared on the Definition Version |
| `featureforge:drafting` | Lifecycle State | Initial recorded entry milestone |
| `featureforge:specified` | Lifecycle State | Entry milestone: the current capability revision is accepted and at least one effective Requirement is traced to one of its exact criteria |
| `featureforge:under-validation` | Lifecycle State | Entry milestone: one current accepted Plan exists and at least one of its exact activities completed against that same current capability revision |
| `featureforge:assessed` | Lifecycle State | Entry milestone: every effective Requirement has an applicable current Claim backed by valid execution/evidence; outcome remains independent of the lifecycle state |
| `featureforge:local-user` | Actor identifier | The single configured actor, in namespace `featureforge` |

Every value above is declared in one place — a single constants file in
`internal/engineering/peos` — and constructed once at package initialization.
Values are not built ad hoc at call sites.

### Namespace validation

Two rules, both test-enforced in M.3:

1. Every vocabulary value FeatureForge constructs has namespace `featureforge`,
   except values obtained from PEOS-exported constants.
2. The declared vocabulary set is closed. A value not on the table above is not
   constructed anywhere in the codebase.

## 7. Persistence contract

PostgreSQL is the final target and is deferred to M.4. M.3 uses an in-memory
adapter that obeys the identical contract, so that semantics are validated
before storage engineering begins. This document defines behaviour; it does not
design tables.

### Principles

1. **Immutable PEOS values are insert-only.** No `UPDATE` and no `DELETE` ever
   touches a recorded engineering value. This holds for the in-memory adapter
   too.
2. **Identical repeated writes are idempotent.** Writing the same identity with a
   byte-identical canonical payload succeeds and changes nothing. This makes
   retries safe.
3. **Same identity, different payload, is a conflict.** It fails loudly with a
   distinguishable error. It is never a silent overwrite and never a silent
   no-op.
4. **The PEOS JSON payload is authoritative.** Whatever a PEOS value marshals to
   is the record. On read, the value is reconstructed by unmarshalling that
   payload, never by reassembling it from columns.
5. **Typed columns are projections.** Any column holding an artifact ID, a
   timestamp, an outcome, or a sequence exists for indexing and query. A
   projection disagreeing with the payload is a bug in the writer, and the
   payload wins.
6. **Product revision content is stored separately** from the PEOS payload, per
   §5, and is itself insert-only.
7. **Derived read models are rebuildable.** Any projection or cache can be
   dropped and recomputed from the immutable records. Nothing is only in a
   derived model.
8. **Mandatory internal references must be resolvable.** They are checked at
   write time by the application layer — PEOS constructors validate structure,
   not existence. AD-030/FF-022 define one narrow exception: C8 may persist a
   structurally valid exact Decision-basis evidence citation whose Evidence
   pair is not yet locally present. Execution, claim, correction, trace,
   lifecycle, and every other aggregate reference remain resolvable.
9. **A transaction boundary is one engineering act.** Recording a capability
   revision writes the PEOS revision, the specification content, and the
   sequence assignment in one atomic transaction, or writes none of them.

### Repository families

Conceptual operations only; signatures are M.2's job.

| Family | Operations |
|---|---|
| Projects | create, get, list |
| Feature cards | create, get, list by project, one-time capability link (AD-031) |
| Artifacts | put artifact, get artifact |
| Artifact revisions | put revision, get revision by exact reference, list revisions of artifact |
| Specification content | put content for exact revision reference, get by exact revision reference |
| Revision sequence | assign next sequence transactionally, get sequence, set acceptance, list ordered |
| Requirements | put requirement, put requirement revision, get, list by capability |
| Requirement criterion traces | put, get by exact Requirement revision reference |
| Decisions | put decision, get, list by subject |
| Validation plans | put plan, put plan revision, get, list |
| Execution records | put, get, list by plan revision, list by activity key |
| Claims | put, get, list by subject, list by criteria |
| Evidence | put evidence artifact and revision, get, list by citing record |
| Lifecycle | put definition, put definition version, put transition record revision, put state assignment, list assignments for subject |

### Required persistence tests

Applied identically to the in-memory adapter (M.3) and PostgreSQL (M.4):

- every PEOS value used by the scenario persists and reloads;
- reloaded values are equal to the originals, and their canonical JSON round
  trips byte-for-byte;
- a duplicate identical write is idempotent;
- a conflicting write of the same identity fails with a conflict error;
- an unresolvable reference is rejected at write time;
- a partially failing engineering act writes nothing;
- dropping and rebuilding every derived model reproduces identical query output.
