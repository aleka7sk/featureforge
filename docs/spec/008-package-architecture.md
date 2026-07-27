# FF-008 — Package Architecture

Status: Accepted (Phase M.2)
Governs: the M.3 Go package layout, each package's responsibility and permitted
imports, and the exported surface an implementation agent may create.

## Governance statement

PEOS-000 through PEOS-009 remain authoritative for every PEOS concept.
`docs/spec/` governs FeatureForge. This document does not extend the PEOS
ontology. The implementation must use PEOS v1.0.0 unchanged, with no `replace`
directive.

## 1. The M.3 objective this layout serves

M.3 implements one complete in-memory engineering lifecycle for the canonical
"Homework after a lesson" capability, demonstrated entirely through automated
tests. No HTTP, no UI, no database, no AI.

## 2. The layout

```
internal/
  domain/                  Project, FeatureCard, product identities        — no PEOS
  engineering/             envelopes, record kinds, content, order metadata — no PEOS
    peos/                  the ONLY package that imports the PEOS SDK
  application/             commands, queries, ports, clock, unit of work    — no PEOS
  infrastructure/
    memory/                in-memory adapter                                — no PEOS
```

Four import rules, all test-enforced:

1. `internal/domain` must not import PEOS.
2. `internal/engineering` must not import PEOS. Its child package may.
3. `internal/infrastructure/...` must not import PEOS.
4. `internal/application` must not import PEOS.

**Only `internal/engineering/peos` imports `github.com/aleka7sk/PEOS/...`.**

Dependency direction:

```
application ──▶ domain
     │           ▲
     │           │
     ├─────▶ engineering ◀────── infrastructure/memory
     │              ▲
     └─────▶ engineering/peos ──▶ PEOS SDK
```

`engineering` never imports `engineering/peos`. The child imports the parent.
That is what makes the envelope vocabulary usable — and testable — without PEOS
present.

## 3. Challenged: every directory

| Directory | Verdict | Reasoning |
|---|---|---|
| `internal/domain` | **Keep** | Holds the two operational entities. It must be provably PEOS-free, which requires its own package. |
| `internal/engineering` | **Keep — this is new in M.2** | M.1's AD-005 requires repositories and adapters to work without importing PEOS. That is only possible if the envelope types live in a package the adapter can import and PEOS cannot reach. Splitting this from `engineering/peos` enforces a real dependency boundary; it is not file categorization. |
| `internal/engineering/peos` | **Keep as ONE package** | See §4. |
| `internal/application` | **Keep** | Owns commands, queries, and the port interfaces it consumes. |
| `internal/infrastructure/memory` | **Keep** | The adapter. `infrastructure/` itself holds no code — only the `memory` package. |
| `cmd/featureforge` | **Drop from M.3** | M.3's objective is satisfied by automated tests. An executable would be an unexercised entry point with no behaviour of its own. Introduce it in M.5, when there is an HTTP server to run. The empty directory is not committed. |

## 4. Should `engineering/peos` be split?

Candidate split: vocabulary, codec, mapping, policy, queries.

**Decision: keep one cohesive `internal/engineering/peos` package in M.3.**

The test for splitting is whether it enforces a dependency boundary that a
single package cannot. Applying it:

| Candidate split | Verdict |
|---|---|
| `vocabulary` | **No.** It would be imported by the codec and by nothing else. Same import set, same test surface — pure file categorization. Vocabulary lives in one file, `vocabulary.go`. |
| `codec` / `mapping` | **No.** These are the same operation seen from two sides: constructing a PEOS value from product input, and projecting it into an envelope. Splitting them forces one to import the other with no boundary gained. |
| `policy` | **No.** There is no PEOS-dependent policy. Ordering, correction, readiness, and timeline policy are all product policy over envelopes — they live in `application` and import no PEOS. |
| `queries` | **No — and this is the load-bearing one.** Queries must not import PEOS at all. See below. |

### Why queries do not live here

The tempting design puts current-state resolution in `engineering/peos`, because
correction-chain resolution appears to need `validation.Claim.Correction()`.

That is avoided by projection. When the codec converts a Claim into a
`RecordEnvelope`, it projects the facts queries need — subject identity, criteria
identities, outcome, correction kind, correction target — into typed envelope
fields ([FF-009 §3](009-in-memory-persistence.md#3-envelope-types)). Every
current-state query then runs over envelopes and product metadata, importing no
PEOS.

This buys three things: queries are unit-testable with hand-built envelopes and
no SDK; the correction, ordering, and readiness algorithms are pure functions
over product types; and the single-PEOS-importer rule survives contact with the
hardest query in the system.

The cost is that the codec must project faithfully. [FF-012](012-test-specification.md)
requires a projection-fidelity test per record family: decode the payload back
into its PEOS type and assert every projected field equals the value read from
the decoded type.

## 5. Package specifications

### 5.1 `internal/domain`

| | |
|---|---|
| Responsibility | The two operational entities and their product identities. Nothing else. |
| Owned types | `ProjectID`, `FeatureCardID`, `Project`, `FeatureCard`, and the domain sentinel errors |
| Permitted imports | stdlib only |
| Forbidden imports | PEOS, `engineering`, `application`, `infrastructure` |
| Exported surface | Constructors returning `(T, error)`; accessor methods; no setters |
| Tests | Construction validation, zero-value rejection, invariants |

`FeatureCard` carries **no** current-revision, readiness, or lifecycle field, and
no collection of requirements, decisions, or claims. Its only engineering link is
an optional capability artifact identity held as a product-owned opaque string.

### 5.2 `internal/engineering`

| | |
|---|---|
| Responsibility | The FeatureForge-owned engineering record vocabulary: envelopes, record kinds, structured content, revision order metadata, acceptance records, and the opaque references that name them |
| Owned types | `ArtifactEnvelope`, `RevisionEnvelope`, `RecordEnvelope`, `RecordKind`, `ArtifactKey`, `RevisionKey`, `RecordKey`, `CapabilitySpecificationContent` and its nested values, `RevisionOrderMetadata`, `RevisionAcceptanceRecord`, `AcceptanceState`, `Digest`, `CanonicalJSON` |
| Permitted imports | stdlib only (`encoding/json`, `crypto/sha256`, `time`) |
| Forbidden imports | **PEOS**, `engineering/peos`, `application`, `infrastructure`, `domain` |
| Exported surface | Envelope constructors with validation; content constructor and canonical-JSON/digest functions; comparison helpers |
| Tests | Canonical JSON determinism, digest stability, envelope validation, equality |

This package must compile and pass its tests with the PEOS module absent from
the build entirely. That is the M.3 proof that AD-005 is real.

### 5.3 `internal/engineering/peos`

| | |
|---|---|
| Responsibility | The single PEOS boundary: construct PEOS values from validated product input, encode them to canonical JSON, decode stored JSON back into exact PEOS types, project identity and query metadata into envelopes, and verify digests |
| Owned types | Vocabulary constants; `Codec`; per-family input structs (`CapabilityRevisionInput`, `RequirementInput`, `DecisionInput`, `PlanInput`, `ExecutionInput`, `ClaimInput`, `LifecycleAssignmentInput`); `Fixture` helpers for tests |
| Permitted imports | stdlib, `github.com/aleka7sk/PEOS/peos/...`, `internal/engineering` |
| Forbidden imports | `application`, `infrastructure`, `domain` |
| Exported surface | §5.3.1 |
| Tests | Vocabulary validation, constructor flows, JSON round trips, projection fidelity, PEOS sentinel-error preservation |

#### 5.3.1 Exported surface shape

Typed operations per family, never a single `Encode(any)` / `Decode(kind, []byte) any`
pair. A universal codec would erase the compile-time family distinction that
makes conflict detection and projection checkable, and would force every caller
to type-assert.

The surface is, conceptually:

- one `Build…` operation per PEOS value family used by the canonical scenario,
  taking a product input struct and returning the envelope(s) that value
  produces;
- one `Decode…` operation per family, returning the exact PEOS type;
- `VerifyDigest(envelope) error`;
- the vocabulary constants.

`Build…` returns envelopes rather than PEOS values, so no PEOS type crosses the
package boundary. `Decode…` returns PEOS types and is used only by this package's
own tests and by projection-fidelity checks — no other package may call it,
because no other package may name its return type. That constraint is structural,
not conventional.

### 5.4 `internal/application`

| | |
|---|---|
| Responsibility | Intent-oriented commands, current-state and timeline queries, the port interfaces it consumes, the clock, and the unit-of-work abstraction |
| Owned types | Command and result types; query types; `Clock`; `UnitOfWork`; every repository interface; the ordering, correction, readiness, lifecycle, and timeline algorithms; application sentinel errors |
| Permitted imports | stdlib, `internal/domain`, `internal/engineering` |
| Forbidden imports | **PEOS**, `internal/engineering/peos` (see below), `internal/infrastructure` |
| Exported surface | Command/query constructors and `Execute`-style methods; port interfaces |
| Tests | Command validation, transaction boundaries, and every query algorithm against hand-built envelopes |

**On `engineering/peos`:** the application layer must invoke PEOS construction,
but must not import the PEOS SDK. It does this through a port interface —
`EngineeringRecorder` — declared in `application` and implemented by
`engineering/peos`. The interface is expressed entirely in `domain` and
`engineering` types, so `application` never names a PEOS type. Wiring happens in
the test's composition root.

This keeps rule 4 exact and testable: `internal/application` has no import of
`engineering/peos` at all, only of the interface it declares itself.

### 5.5 `internal/infrastructure/memory`

| | |
|---|---|
| Responsibility | An in-memory adapter implementing every repository port with the persistence semantics PostgreSQL must later reproduce |
| Owned types | `Store` and the repository implementations; a failure-injection hook for rollback tests |
| Permitted imports | stdlib, `internal/domain`, `internal/engineering`, `internal/application` (for the port interfaces) |
| Forbidden imports | **PEOS**, `internal/engineering/peos` |
| Exported surface | `NewStore(...)` and the repository accessors; internal maps are never exposed |
| Tests | The full persistence contract suite from [FF-009 §7](009-in-memory-persistence.md#7-in-memory-adapter-semantics) |

## 6. Forbidden constructs

No package may be named, or created as, any of: `workflow`, `engine`,
`framework`, `shared`, `common`, `util`, `pkg`, `integration`, `core`.

No package for HTTP, transport, database, migration, frontend, or AI exists in
M.3. Not even empty, and not as a placeholder.

No FeatureForge type restates a PEOS type's field set.

No operational scenario concept — teacher, student, lesson, homework,
attachment, notification — appears as a type, field, or package name.

## 7. Enforcement

[FF-012 §12](012-test-specification.md#12-architecture-tests) specifies the
build-failing architecture tests. They parse the module's package graph and Go
syntax trees using the standard library only — `go/parser` and `go/ast` over the
`internal/` tree, with `go/build` to resolve transitive import sets.

**No third-party analysis dependency is added.** M.3 adds exactly one module
requirement, `github.com/aleka7sk/PEOS v1.0.0`, and nothing else; an architecture
test that needed `golang.org/x/tools` would put a second dependency in `go.mod`
to police the first.

Tests never assert on line numbers or file offsets. Each is verified against a
deliberate violation during M.3, so that a test which cannot fail is caught.
