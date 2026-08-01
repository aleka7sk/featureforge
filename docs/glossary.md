# FeatureForge Glossary

Every term used across FF-000..FF-007. Each entry states who owns the term:

- **PEOS** — defined by PEOS-000..PEOS-009. FeatureForge uses it as PEOS defines
  it and never redefines it. The PEOS specification governs.
- **FeatureForge** — a product term FeatureForge owns.
- **Scenario content** — text inside a specification or requirement. Never a
  type, a table, or an operation.

---

## PEOS terms

**Artifact** *(PEOS)* — a stable, persistent logical engineering identity,
independent of any revision, representation, or storage location. It has no
"current revision" field.

**Artifact Revision** *(PEOS)* — one identifiable, fixed, immutable recorded state
of an Artifact. Carries no revision number, no predecessor link, and no status.

**Artifact Revision Reference** *(PEOS)* — an exact reference to one revision:
the owning Artifact plus the exact revision.

**Artifact Type** *(PEOS)* — an open namespaced vocabulary value classifying an
Artifact. FeatureForge declares its own values in the `featureforge` namespace.

**Artifact Role** *(PEOS)* — a namespaced vocabulary value describing the role a
revision plays; `peos:evidence` is the one FeatureForge uses.

**Claim** — see *Validation Claim*.

**Claim Outcome** *(PEOS)* — what was determined about an evaluated subject:
`satisfied`, `not-satisfied`, or `inconclusive`. Distinct from *Execution
Outcome*; the two vocabularies never share values.

**Claim Type** *(PEOS)* — the Claim specialization. The one PEOS vocabulary family
treated as **closed**. FeatureForge uses `peos:satisfaction` and declares no value
of its own.

**Correction Kind** *(PEOS)* — `correct`, `replace`, or `invalidate`. Carried by a
correction reference.

**Correction Reference** *(PEOS)* — a typed reference carried by a *new* record
naming the earlier record it corrects, replaces, or invalidates. The earlier
record is never edited or deleted.

**Criterion Reference** *(PEOS)* — the closed union naming what a Claim is
*evaluated against*, as distinct from what it is *about*. FeatureForge cites
requirement revisions through it.

**Decision** *(PEOS)* — a recorded engineering decision with its own identity,
subjects, question, outcome, rationale, and provenance.

**Decision Basis** *(PEOS)* — what a Decision rests on: evidence, assumptions,
constraints, and uncertainties.

**Engineering Subject Reference** *(PEOS)* — the closed union naming what
something is *about*.

**Evidence** *(PEOS)* — an Artifact Revision serving the evidence role, always
cited at the exact revision level.

**Execution Outcome** *(PEOS)* — how one attempted execution concluded:
`completed`, `failed`, `interrupted`, or `indeterminate`. An indeterminate or
interrupted outcome must never be silently treated as completed.

**Extension** *(PEOS)* — a namespaced container for product-specific data.
**FeatureForge does not use it** ([AD-010](decisions/README.md#ad-010--specification-content-is-featureforge-owned-digest-bound-to-the-peos-revision-and-never-in-an-extension)).

**Integrity Identity** *(PEOS)* — the mandatory integrity value on every Artifact
Revision. FeatureForge records the specification content's SHA-256 digest here.

**Lifecycle Definition / Definition Version** *(PEOS)* — the declaration of states
and transitions, and one versioned form of it. FeatureForge has exactly one of
each.

**Local Key** *(PEOS)* — the identity of a value owned by one Artifact Revision,
such as a planned validation activity. Stable only within its owning revision;
it is not a global identity.

**Origin** *(PEOS)* — the engineering basis explaining why a revision exists.
Where origin is unknown, unavailable, disputed, or reconstructed, that must be
explicit.

**Planned Validation Activity** *(PEOS)* — a revision-owned value inside a
Validation Plan Revision, naming a subject, a method, criteria, and expected
evidence, identified by a plan-local key.

**Provenance** *(PEOS)* — who or what produced a record and when. Mandatory on
every Artifact Revision.

**Representation** *(PEOS)* — a physical or logical encoding of a revision's
content, with a media type and a classification. Carries no identity of its own.

**Requirement / Requirement Revision** *(PEOS)* — a PEOS Artifact and its
immutable revisions, stating an engineering obligation.

**Scope** *(PEOS)* — a representation-independent scope value. PEOS defines no
query language over it; its owning construct interprets it.

**State Assignment** *(PEOS)* — a record that a lifecycle subject was assigned a
state at a time, established by a Transition Record Revision. Never revised.

**Transition Record** *(PEOS)* — the Artifact recording a lifecycle transition;
its revisions carry the transition content.

**Validation Claim** *(PEOS)* — an immutable record asserting an outcome about a
subject, evaluated against criteria, supported by execution records and evidence.
Records an assertion; authorizes nothing.

**Validation Execution Record** *(PEOS)* — an immutable record that a validation
activity was executed, with its actor, method, outcome, and produced evidence.
Never revised.

**Validation Method** *(PEOS)* — an open namespaced vocabulary value naming how
validation was performed.

**Validation Plan / Plan Revision** *(PEOS)* — a PEOS Artifact declaring what will
be validated and how, and its immutable revisions.

**Verdict** *(PEOS — explicitly does not exist)* — PEOS-006 states there is no
separate Verdict entity. FeatureForge creates no such type.

**Vocabulary Value** *(PEOS)* — an open, namespaced value written
`namespace:value`, split on the first colon.

---

## FeatureForge terms

**Acceptance Journal** — the append-only record of revision acceptance
transitions, carrying actor, timestamp, and reason. Feeds the timeline.

**Acceptance State** — a managed capability, Requirement, or Validation Plan
revision's product-owned state: `draft`, `accepted`, or `withdrawn`. C7/C9
establish their semantic member as immediately `accepted`. Not a PEOS lifecycle state
([AD-004](decisions/README.md#ad-004--revision-acceptance-and-peos-lifecycle-state-are-separate-concerns)).

**Capability Specification** — the PEOS Artifact of type
`featureforge:product-capability` holding a feature's engineering specification.

**Context Pack** — the derived, read-only input given to the AI component. Every
element names its exact source reference.

**Current Capability Revision** — the accepted revision with the greatest
sequence. A computed answer, never a stored field.

**Effective Requirements** — the current revision of every requirement linked to
a capability, excluding withdrawn ones. Computed.

**Acceptance Journal** — see above. Its head is the *only* place a revision's
acceptance state exists; there is no stored acceptance field
([AD-015](decisions/README.md#ad-015--acceptance-is-an-append-only-journal-there-is-no-stored-acceptance-field)).

**Canonical JSON** — FeatureForge's deterministic serialization of structured
content: declared field order, no insignificant whitespace, HTML escaping
disabled, empty lists emitted as `[]`. The input to every digest, and the basis
of both content equality and payload conflict detection.

**Codec** — the operations in `internal/engineering/peos` that build PEOS values
from validated product input, encode them to canonical JSON, decode stored JSON
back into exact PEOS types, and project query metadata into envelopes.

**Digest** — lowercase-hex SHA-256 of a canonical JSON value. Bound into a PEOS
revision's `IntegrityIdentity` as `sha256:<hex>`, which is what makes the link
between a PEOS revision and its FeatureForge content verifiable rather than
merely referential.

**Engineering Record Envelope** — a FeatureForge-owned persistence and
projection carrier holding a key, a kind, an authoritative canonical-JSON
payload, its digest, and typed projections. Three variants: `ArtifactEnvelope`,
`RevisionEnvelope`, `RecordEnvelope`. It exists so no persistence adapter imports
PEOS ([AD-005](decisions/README.md#ad-005--only-the-integration-layer-imports-peos-adapters-store-opaque-payloads),
[AD-013](decisions/README.md#ad-013--three-envelope-types-not-one-universal-envelope-no-relationenvelope-in-m3)).

**Engineering State** — the immutable, provenance-bearing record of what was
specified, required, decided, validated, and claimed. Modelled with PEOS values
plus governed FeatureForge metadata such as content, order, acceptance journal,
and Requirement Criterion Trace; insert-only or, for the journal, append-only.

**EngineeringRecorder** — the port declared in `internal/application` and
implemented by `internal/engineering/peos`, expressed entirely in `domain` and
`engineering` types. It is how the application layer causes PEOS values to be
constructed without naming a PEOS type.

**Entry Assignment** — the first lifecycle State Assignment for a subject,
established by a Transition Record Revision that carries no transition content,
because PEOS v1.0.0 cannot express an entry Transition from an unassigned
condition ([AD-014](decisions/README.md#ad-014--the-lifecycle-entry-assignment-is-established-by-a-content-free-transition-record-revision)).

**FeatureCard** — the operational entry point for one capability. Stable title
and description in this POC; one optional, monotonic link to a capability
Artifact; **no derived state**.

**FeatureForge Namespace** — `featureforge`. The only namespace FeatureForge
constructs vocabulary values in.

**Operational State** — FeatureForge's own product bookkeeping, distinct from
PEOS engineering state. The category permits product-owned mutation but does
not promise it: AD-031 keeps establishment fields stable in this POC and allows
only the one-time capability link. It has no engineering meaning and is never
represented as a PEOS value.

**Project** — an operational naming container for feature cards. No engineering
semantics.

**Rationale** — the mandatory explanation accompanying every derived answer: the
rule applied, the records considered, and the records rejected with reasons.
Displayed in the UI, not merely logged.

**Release Readiness** — a computed outcome over requirement coverage and current
claims: `ready`, `not-ready`, `indeterminate`, or `incomplete`.
Product-only terminology; never a PEOS value
([AD-008](decisions/README.md#ad-008--release-readiness-is-a-computed-query-not-a-claim)).

**Revision Sequence** — the product-owned dense integer ordering of an Artifact's
revisions, unique within the Artifact, starting at 1, assigned transactionally.
Carried by `RevisionOrderMetadata`, which carries ordering **only**.

**Correction Head** — the claim that no other claim corrects, replaces, or
invalidates. Selecting it is how the applicable claim is found; timestamps are
never used to select
([FF-010 §6](spec/010-application-contracts.md#6-correction-resolution-algorithm)).

**Projection** — a typed field copied out of an envelope's authoritative payload
so queries can run without decoding it, and therefore without importing PEOS. A
projection is never authoritative; disagreement with its payload is a codec bug,
caught by a projection-fidelity test.

**Specification Content** — the FeatureForge-owned structured content of one
capability revision: title, problem statement, user outcome, functional
behaviour, constraints, acceptance criteria, dependencies, open questions.
Insert-only, keyed by exact revision reference, digest-bound to the PEOS revision.

**Timeline** — the computed product read model presenting the engineering history
in a total, deterministic order. Not a PEOS type.

**Undated Group** — timeline events whose source record carries no timestamp,
rendered separately and flagged rather than silently ordered.

---

## Scenario content

These appear only as text inside specification revisions and requirement
statements. None is a FeatureForge type, table, entity, or operation, and an
architecture test asserts as much.

**Teacher**, **Student**, **Lesson**, **Homework**, **Audio Attachment**,
**Publication**, **Notification** *(scenario content)*.

---

## Terms deliberately not defined

| Term | Why |
|---|---|
| Workspace, Comment, Attachment Metadata | Rejected operational entities ([AD-001](decisions/README.md#ad-001--operational-entities-are-limited-to-project-and-featurecard)) |
| Result, Verdict, Status | No such construct; PEOS-006 forbids a Verdict entity ([AD-011](decisions/README.md#ad-011--result-is-not-a-construct-correction-is-a-distinct-use-case)) |
| Workflow, State Machine, Rule Engine, Guard, Effect, Trigger | FeatureForge introduces no workflow engine and no expression language |
| Tenant, Organization, Role, Permission | Multi-tenancy and authentication are out of scope for every phase |
| Branch, Merge | Branching revision histories are rejected for the POC ([AD-003](decisions/README.md#ad-003--revision-ordering-is-a-product-owned-dense-integer-sequence-linear-only)) |
