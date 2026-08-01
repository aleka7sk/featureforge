# FF-002 — Domain Boundaries

Status: Accepted (Phase M.1)
Governs: FeatureForge-owned operational entities, package layering, and the
enforced separation between the product domain and PEOS.

## 1. The two kinds of state

FeatureForge holds exactly two kinds of state, and confusing them is the primary
architectural risk of this project.

**Operational state** is FeatureForge's own bookkeeping: which projects and
feature cards exist and which capability a card names. It is product-owned,
has no engineering meaning, and is never represented as a PEOS value.
"Operational" describes ownership and semantics; it does not by itself promise
a generic in-place edit surface. Under AD-031 the Project and FeatureCard
establishment fields are stable for this bounded POC. The only supported change
is the one-time `FeatureCard -> capability Artifact` link.

**Engineering state** is the immutable, provenance-bearing record of what was
specified, required, decided, validated, and claimed. It is modelled with PEOS
values plus the explicitly governed product-owned engineering metadata in §3.
Those values are immutable/insert-only, except that the acceptance journal is
append-only; none is edited in place.

A FeatureCard is operational. The engineering specification associated with that
FeatureCard is engineering state, represented through PEOS. The FeatureCard
holds a reference to it; it does not contain it.

## 2. Operational entity list, challenged

The candidate list was Workspace, Project, FeatureCard, User, Comment,
AttachmentMetadata. Each was assessed against a single test: *does the canonical
scenario in [FF-005](005-validation-scenario.md) fail without it?*

| Candidate | Verdict | Reasoning |
|---|---|---|
| **Project** | **Keep** | The scenario needs a container so that "list feature cards" is a bounded query and the Projects screen has content. Cheap, and it is the only grouping concept. |
| **FeatureCard** | **Keep** | The scenario's entry point. It is the operational handle a human uses; it owns the link to the capability Artifact. |
| Workspace | **Drop** | A Workspace exists to separate tenants or teams. Multi-tenancy is explicitly out of scope for every phase ([FF-000](000-product-overview.md)). A second grouping level above Project buys nothing and invites tenancy modelling. |
| User | **Drop as an entity** | A single local user is sufficient. Actor identity is still needed — every PEOS Provenance carries one — but it is a fixed configured identity, not a stored, managed record. See §3. |
| Comment | **Drop** | Collaboration is explicitly out of scope. A comment carries no engineering meaning: it is neither a Requirement, a Decision rationale, nor Evidence. If a remark matters to engineering, it belongs in a Decision's rationale or a Revision's open questions, where it is immutable and provenance-bearing. |
| AttachmentMetadata | **Drop** | This is the boundary's sharpest trap. The scenario's "optional audio attachment" is *scenario content* — a sentence inside a specification revision and a requirement statement. It is not a file FeatureForge stores. Where a real artifact must be cited (a validation report), PEOS already models it: an Artifact Revision in the Evidence role whose Representation is an external reference with a media type. Adding an attachment table would model the operational Belcanto domain inside FeatureForge. |

Accepted operational entities: **Project** and **FeatureCard**. Nothing else.

This list is deliberately closed for the POC. Adding an entity to it is a
material architecture decision and requires a recorded decision in
[`docs/decisions/`](../decisions/README.md).

### Project

| Property | Value |
|---|---|
| Identity | Product-owned, opaque, caller-supplied (FF-010, AD-029) |
| Supported change | None after establishment in this POC |
| References PEOS | No |
| May be a PEOS Artifact | Never |

A Project is a naming container. It has no engineering semantics, participates
in no claim, and appears in the timeline only as the context in which a
FeatureCard was created.

### FeatureCard

| Property | Value |
|---|---|
| Identity | Product-owned, opaque, caller-supplied (FF-010, AD-029) |
| Supported change | One-time absent-to-present capability link; title, description, and Project remain stable |
| References PEOS | Yes — one optional capability Artifact reference, stored as opaque identity strings, never as a PEOS type |
| May be a PEOS Artifact | Never |

A FeatureCard is the human entry point. It carries at most one link to a
capability specification. That link is stored as plain identity strings in the
domain layer and is converted to a `core.ArtifactRef` only inside the
integration layer (§4).

The stable establishment policy is a FeatureForge POC narrowing, not a claim
that operational entities are intrinsically immutable. See AD-031.

The FeatureCard stores **no** derived engineering state: no current revision, no
readiness flag, no claim outcome, no lifecycle state. Every one of those is a
computed query described in [FF-004](004-current-state-resolution.md). Caching
any of them on the card would make a derived view authoritative, which
[FF-003](003-peos-integration.md) forbids.

### Actor identity

Every PEOS Provenance, Execution Record, and State Assignment requires an actor.
FeatureForge supplies one fixed configured identity —
namespace `featureforge`, identifier `local-user` — for the entire POC. It is
configuration, not a stored entity, and there is no user table, no registration,
and no authentication.

## 3. Product-owned records that are not operational entities

Five further things are FeatureForge-owned but are neither operational entities
nor PEOS values. They are stated here so no phase mistakes them for either.

| Record | What it is | Mutability |
|---|---|---|
| **Specification Content** | The structured product content of one capability revision — title, problem, outcome, behaviour, constraints, acceptance criteria, dependencies, open questions. Keyed by an exact Artifact Revision reference. Defined in [FF-003 §5](003-peos-integration.md#5-revision-content-ownership). | Insert-only, never edited |
| **Revision Sequence** | The product-owned integer ordering of managed capability, Requirement, and Validation Plan revisions. Defined in [FF-004 §2](004-current-state-resolution.md#2-the-revision-ordering-contract). | Insert-only |
| **Acceptance Journal** | The append-only record of revision acceptance transitions, carrying actor, time, and reason. A revision's acceptance state is its journal head — there is no stored acceptance field ([AD-015](../decisions/README.md#ad-015--acceptance-is-an-append-only-journal-there-is-no-stored-acceptance-field)). | Append-only |
| **Requirement Criterion Trace** | Product-owned structured metadata binding one Requirement revision to one exact capability revision and acceptance-criterion key. It is established atomically with C7 and is not a PEOS relationship or an encoded note. Defined by [AD-033](../decisions/ad-033-requirement-criterion-trace-is-structured-state.md). | Insert-only, exact-revision metadata |
| **Timeline / current-state read models** | Computed views over PEOS values and the stored engineering values above. Defined in [FF-004](004-current-state-resolution.md) and [FF-006](006-timeline-read-model.md). | Derived, rebuildable, never authoritative |

## 4. Package layering

```
transport (HTTP, UI models)        ← M.5
        │
        ▼
    application            (use cases; the only layer that orchestrates)
        │            ╲
        ▼             ╲
     domain            ▼
  (Project,      engineering/peos   (the ONLY package that imports the PEOS SDK)
   FeatureCard)         │
                        ▼
                    PEOS SDK
```

Rules, in order of importance:

1. **`internal/domain` must not import PEOS.** Not `peos/core`, not any PEOS
   package, not transitively.
2. **`internal/engineering/peos` is the only package permitted to import the
   PEOS SDK.** It owns every conversion between product identity strings and
   PEOS reference values, and it owns the FeatureForge vocabulary constants.
3. **`internal/application` must not import the PEOS SDK directly.** It depends
   on `internal/domain` and on `internal/engineering/peos`, and it expresses use
   cases in terms of the integration layer's own types.
4. **Transport and UI models must not import PEOS.** A PEOS value never appears
   in an HTTP request body type, an HTTP response body type, or a UI view model.
   The integration layer produces plain product-shaped output for those layers.
5. **`internal/engineering/peos` must not import `internal/application` or any
   transport package.** Dependencies point inward and downward only.
6. **Infrastructure implements repository contracts owned by the integration or
   application layer.** The contract is declared by the consumer, not by the
   adapter. `internal/infrastructure` may import PEOS only where it must
   serialize PEOS values — see the qualification below.

### The one qualification on rule 2

Persisting a PEOS value requires marshalling it, and marshalling happens in the
persistence adapter. Two options were weighed: let the adapter import PEOS, or
have the integration layer hand the adapter pre-serialized bytes.

**Accepted: the integration layer serializes; the adapter stores bytes.**
Repository contracts are expressed in terms of a FeatureForge-owned envelope —
an identity, a record kind, and an opaque canonical JSON payload — so the
persistence adapter needs no PEOS import at all. This keeps the PEOS import set
to exactly one package, makes the architecture test trivially checkable, and
means the PostgreSQL work in M.4 is about storage semantics rather than about
PEOS types. Recorded as **AD-005** in the [decision log](../decisions/README.md).

### Enforcement

These are not conventions. M.3 delivers architecture tests that fail the build
when they are broken:

| Test | Asserts |
|---|---|
| `TestDomainDoesNotImportPEOS` | The transitive import set of `internal/domain` contains no `github.com/aleka7sk/PEOS/...` package. |
| `TestOnlyIntegrationImportsPEOS` | Exactly one package — `internal/engineering/peos` — directly imports the PEOS SDK. |
| `TestApplicationDoesNotImportPEOS` | `internal/application` has no direct PEOS import. |
| `TestTransportDoesNotImportPEOS` | No transport or UI-model package imports PEOS, directly or transitively. |
| `TestNoPEOSTypeIsCopied` | No FeatureForge type declaration reproduces the field set of a PEOS type. Checked by name-and-shape assertion over a fixed list of PEOS types, not by heuristic. |
| `TestNoOperationalScenarioEntity` | No FeatureForge type or table is named for a scenario concept (teacher, student, lesson, homework, attachment, notification). |
| `TestPEOSNamespaceNotWritten` | No FeatureForge code constructs a PEOS vocabulary value in the `peos` namespace. Only PEOS-provided constants are used. |

The import tests read the module's own package graph; they do not shell out and
do not depend on network access.

## 5. What must never happen

- A PEOS type appearing as a field on `Project` or `FeatureCard`.
- A PEOS type appearing in an HTTP or UI model.
- A FeatureForge type that restates a PEOS type's fields ("shadow struct").
- A derived verdict — current, satisfied, ready, conformant — stored on a PEOS
  value or on a FeatureCard.
- A vocabulary value constructed in the `peos` namespace by FeatureForge.
- An operational scenario concept modelled as a FeatureForge entity or a PEOS
  Artifact.
- A shared, extracted "PEOS integration library" created during this project for
  Belcanto's benefit. See
  [FF-001 §6.7](001-poc-acceptance-contract.md#67-transition-to-belcanto).
