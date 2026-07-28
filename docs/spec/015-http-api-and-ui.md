# FF-015 — M.5 HTTP API and Minimal UI: Engineering Plan

Status: Phase A accepted and implemented via
[FF-018](018-http-phase-a-implementation.md); Phase B (minimal UI) accepted and
implemented via [FF-020](020-read-surface-extension.md) (the read-surface
extension Phase B required first) and
[FF-021](021-ui-phase-b-implementation.md). M.5 is complete; M.6 (AI
Context-Pack Demonstration) is next.
Governs: the M.5 transport and interface layer — objectives, scope, HTTP
contracts, and implementation order. Produced before any code is written.

## Governance statement

PEOS-000 through PEOS-009 remain authoritative for every PEOS concept.
`docs/spec/` governs FeatureForge. This document adds no PEOS concept and
changes none.

**This plan assumes the roadmap is unchanged.** FF-007's M.5 entry — "HTTP API
and Minimal UI", one milestone, one set of exit criteria — stands exactly as
accepted. M.5 is sequenced internally as Phase A (HTTP API) then Phase B
(Minimal UI); that is an implementation ordering, not a milestone split. The UI
exists to demonstrate the API is usable by a human, not as an independent
deliverable.

**M.4 is frozen.** Repository contracts, `UnitOfWork`, persistence
abstractions, and transaction semantics are validated infrastructure
(see [the M.4 architecture review](../reports/m4-architecture-review.md) §11).
This plan builds on them and redesigns none of them. Where a transport choice
is made, the justification is stated as a consequence of the validated
architecture rather than as a new architectural direction.

---

## 1. Objectives

M.5 answers the question M.3 and M.4 could not: **can a person drive and
understand the entire engineering lifecycle?**

M.3 proved the lifecycle is expressible. M.4 proved it is
persistence-independent. Both were proven by tests, which is to say by an
author who already knew the answer. M.5 is the first milestone whose subject is
comprehension by someone who does not.

Three objectives, in priority order:

1. **Expose the existing use cases over HTTP without adding business logic.**
   Every endpoint delegates to an existing command or query. If a request
   requires logic that does not already exist in `internal/application`, that
   is a signal the endpoint is wrong, not that the application layer is
   incomplete.
2. **Render every derived answer with its rationale.** FF-001 §3.2 is explicit:
   readiness appears "with the per-requirement rationale table, never as a bare
   badge". The queries already return rationale structures; M.5's job is to not
   discard them.
3. **Keep history visible.** Superseded claims and prior revisions must be
   reachable in the interface, not hidden behind the current answer.

## 2. Scope

**Phase A — HTTP API.**

- One transport package exposing the 12 application commands and the query
  surface over HTTP.
- Transport request/response models containing no PEOS type, test-enforced.
- A single error-mapping point translating application sentinels to status
  codes.
- `cmd/featureforge`: the executable, deferred from M.3 by FF-008 precisely
  until there was a server to run.
- Configuration sufficient to select and connect a persistence adapter.

**Phase B — Minimal UI.**

- The seven screens of FF-001 §3: Projects, Feature overview, Revisions,
  Requirements, Decisions, Validation, Timeline.
- Server-rendered HTML, standard library only.
- Rationale rendered wherever a derived answer is shown.

**Both phases.**

- Architecture tests extended to police the new boundary (§14, §4.3).
- The canonical scenario drivable through the HTTP surface.

## 3. Non-goals

Explicitly out of scope, each with the authority that excludes it:

| Excluded | Authority |
|---|---|
| Authentication, authorization, sessions, accounts | FF-001 §3, FF-007 M.5 exit criteria ("no authentication … exists") |
| Notifications, collaboration, multi-user concurrency semantics | FF-000 "What FeatureForge is not"; FF-001 §3 "One local user" |
| Multi-tenancy | FF-000 non-goals |
| A visual design system, CSS framework, or JavaScript framework | FF-001 §3 "No visual system" |
| Any new business logic in the transport layer | FF-007 M.5 deliverable: "no new business logic" |
| AI context packs | M.6 |
| Materialized read models | AD-006 — still no measured evidence |
| Pool/retry tuning under load | Deferred by FF-014 §10; needs load M.5 does not generate |
| Any second infrastructure dependency | CLAUDE.md dependency rule |

**Deliberately not decided here:** whether the UI is ever exposed beyond
localhost. The POC is a single-user local tool and is frozen at M.7.

## 4. Architectural constraints inherited from M.4

These are not re-litigated. They are inputs.

### 4.1 Frozen and consumed as-is

| Frozen area | How M.5 consumes it |
|---|---|
| Repository contracts | Transport never touches them. It calls commands and queries, which own `Repositories` access |
| `UnitOfWork` | One HTTP request maps to at most one `Do` call, invoked *inside* a command. Transport never calls `Do` |
| Transaction semantics | The atomicity boundary is the command. HTTP introduces no second boundary — no multi-request transactions, no server-side session state |
| Retry semantics | Callbacks must remain re-runnable. Already enforced module-wide by `TestDoCallbacksAreRetrySafe`, which walks all of `internal/` and will cover transport automatically |
| Persistence abstraction | `cmd/featureforge` selects an adapter at startup; nothing above infrastructure knows which |
| Shared contract suite | Untouched. M.5 adds no repository |

### 4.2 Constraints M.5 must actively preserve

- **`internal/engineering/peos` remains the only PEOS importer** (AD-005).
  Transport models are built from `domain`, `engineering`, and `application`
  types, never PEOS ones. FF-007 lists this as a test-enforced deliverable.
- **No `UPDATE`/`DELETE` semantics.** Engineering records are immutable. This
  has a direct HTTP consequence (§6.3).
- **Derived state is never stored** (AD-006). The transport caches nothing.

### 4.3 The `net/http` prohibition — investigation

**Current state.** `TestNoHTTPDatabaseUIOrAIPackage` does two things:

1. Forbids importing `net/http`, `database/sql`, `html/template`,
   `text/template` anywhere under `internal/`.
2. Forbids any package under `internal/` whose relative path begins with
   `http`, `ui`, `ai`, `postgres`, or `sql`.

**Findings.**

*It should be narrowed, not removed.* The test encodes a real and still-valid
intent — transport, rendering, and raw SQL concerns must not leak into the
layers that carry engineering meaning. M.5 invalidates the *scope* of that
prohibition, not its purpose. Removing it would discard a boundary at the exact
moment the boundary starts to matter.

*The codebase already has the correct pattern, proven twice.*
`TestOnlyIntegrationPackageImportsPEOS` (M.3) and
`TestOnlyPostgresInfrastructureImportsDriver` (M.4) both express "exactly one
package may import X". That shape converts a blanket prohibition into a
permission with a named holder, and it has now been validated by two
independent applications. M.5 should reuse it rather than invent a third form.

*Which layers may legitimately import what:*

| Import | Permitted in | Forbidden in |
|---|---|---|
| `net/http` | the transport package; `cmd/featureforge` | domain, engineering, application, infrastructure |
| `html/template` | the UI-rendering package only | everywhere else |
| `text/template` | nowhere | everywhere — no current need; keep prohibited until one exists |
| `database/sql` | nowhere | everywhere — absolute, per AD-020's native-pgx decision |

*Three defects in the current test, surfaced by this investigation:*

- **Stale milestone wording.** It still reports "which M.3 must not use" and
  "forbidden package present for M.3". M.4's plan included rewording it; that
  item was not executed. Recorded here rather than quietly fixed.
- **The `postgres`/`sql` prefix check is inert.** It matches on the path
  *relative to `internal/`*, so `infrastructure/postgres` never matched it.
  The check has never done anything. The real boundary is enforced by
  `TestOnlyPostgresInfrastructureImportsDriver`, which is why nothing was
  noticed.
- **`cmd/` is entirely unchecked.** Every architecture test walks only
  `internal/` (`InternalPackages()` roots at `internal`, and every
  `walkGoFiles` call joins `ModuleRoot(), "internal"`). M.5 introduces
  `cmd/featureforge`, so this blind spot becomes load-bearing: without
  extension, `cmd` could import PEOS or a driver directly and no test would
  object.

**Recommended shape** (to be implemented in Phase A, step 1):

- Split the test by intent: `TestOnlyTransportImportsNetHTTP`,
  `TestOnlyUIImportsHTMLTemplate`, `TestDatabaseSQLIsNeverImported`,
  `TestNoAIPackage`.
- Extend package discovery to include `cmd/`, so the PEOS and driver boundaries
  cover the executable.
- Drop the inert prefix check; the import-based tests state the real boundary.
- Each new test verified against a deliberate violation before being trusted —
  the discipline the M.4 review confirmed as effective (§9.4 of that review).

This is a governance change to an architecture test and must be recorded as a
decision (§17, AD-023).

## 5. Public API philosophy

**The API is intent-oriented, not resource-CRUD.** This is a consequence of an
accepted decision, not a preference.

FF-010 §3 states the application layer has "no command per repository method,
no `Update*`, no `Delete*`, no generic `RecordEngineeringAct`", and names each
command for the engineering act it performs — `EstablishCapabilitySpecification`,
`CorrectValidationClaim`, `AssignLifecycleState`. A CRUD-resource API would
have to invent `PUT /revisions/{id}` semantics for values that are immutable by
construction, and would flatten twelve distinct engineering acts into four
verbs. The mapping would lose exactly the information the domain exists to
preserve.

Four principles follow:

1. **Writes are intents.** Each command is one endpoint. The endpoint name
   states the act.
2. **Reads are questions.** Each query is one endpoint, returning the answer
   *and its rationale*.
3. **The transport is a translator, not a decision-maker.** Its only
   responsibilities are decoding, invoking, and encoding. Any conditional that
   is not input validation or error mapping belongs in `internal/application`.
4. **The API is honest about immutability.** Nothing in the HTTP surface
   suggests a record can be edited.

## 6. Endpoint organization

### 6.1 Command endpoints (Phase A)

Twelve commands, twelve endpoints, all `POST`. Grouped by the entity whose
history they extend:

> **Count note.** FF-010 §3's prose says "Ten commands and two queries" while
> its own table lists twelve command rows, and `internal/application` defines
> twelve `*Command` structs. Twelve is correct; the prose is drifted. Recorded
> here rather than corrected, since FF-010 is accepted and this is a planning
> document — but a reader comparing the two should trust the table and the
> code.

```
POST /api/v1/projects                                    CreateProject
POST /api/v1/features                                    CreateFeature

POST /api/v1/capabilities                                EstablishCapabilitySpecification
POST /api/v1/capabilities/{artifactID}/revisions         ReviseCapabilitySpecification
POST /api/v1/capabilities/{artifactID}/acceptances       AcceptCapabilityRevision
POST /api/v1/capabilities/{artifactID}/lifecycle         AssignLifecycleState

POST /api/v1/requirements                                EstablishRequirement
POST /api/v1/decisions                                   RecordArchitectureDecision

POST /api/v1/validation/plans                            EstablishValidationPlan
POST /api/v1/validation/runs                             RecordValidationRun
POST /api/v1/validation/claims                           RecordValidationClaim
POST /api/v1/validation/claims/corrections               CorrectValidationClaim
```

Two observations on this shape:

- **`acceptances` is a collection, not a state field.** AD-015 made acceptance
  an append-only journal with no stored field. `POST .../acceptances` appends
  an entry; there is deliberately no `PUT .../acceptance`. The URL structure
  carries the domain decision.
- **Corrections are a sub-collection of claims, not a mutation of one.**
  `POST /validation/claims/corrections` creates a *new* claim referencing an
  earlier one. AD-017's model is visible in the route.

### 6.2 Query endpoints (Phase A)

```
GET /api/v1/projects
GET /api/v1/projects/{projectID}/features
GET /api/v1/features/{featureCardID}                     composed overview
GET /api/v1/features/{featureCardID}/state               GetFeatureEngineeringState
GET /api/v1/features/{featureCardID}/timeline            GetFeatureTimeline
GET /api/v1/capabilities/{artifactID}/revisions          revisions + resolution rationale
GET /api/v1/capabilities/{artifactID}/revisions/{revisionID}
```

`GetFeatureEngineeringState` already composes current revision, effective
requirements, applicable decisions, readiness, and lifecycle — each with its own
rationale. The feature-overview screen (FF-001 §3.2) is very nearly a direct
rendering of it, which is evidence the application layer was decomposed along
the right seams.

**One gap, investigated, decided, and now closed.**
`EngineeringStateInput` and `TimelineInput` require the caller to supply
requirement and decision identifier lists. In M.3 and M.4 the caller was the
scenario driver, which knew them by construction; an HTTP client holding only a
`FeatureCardID` does not.

[The M.5 contract investigation](../reports/m5-contract-investigation.md)
resolved this into two halves with different answers:

- **Decisions, executions, claims, and evidence need no contract change.** They
  are `RecordEnvelope`s, which already project `SubjectKey`. A decision's
  subject is a capability revision; capability revisions are enumerable via
  `Revisions.ListByArtifact`; `Records.ListByKindAndSubject` finds the rest.
  Evidence is recoverable from the `EvidenceKeys` claims and executions already
  project. Five of the seven identifier lists dissolve using existing
  operations, in transport, today.
- **Requirements and validation plans cannot be discovered at all.** They are
  `RevisionEnvelope`s, which project no subject, and every revision-listing
  operation requires a known artifact ID. Every transport-level alternative was
  evaluated and rejected — including deriving requirements from claims, which
  is *semantically invalid* rather than merely incomplete: FF-011's `REQ-4` has
  no plan activity and no claim, so it would vanish from the population and
  readiness would report a falsely complete result.

**The resolution is specified by [AD-025](../decisions/README.md#ad-025) and
[FF-016](016-revision-subject-discovery.md). FF-016 has landed.**

The repository capability (`ListByFamilyAndSubject`, and the
`RevisionEnvelope.SubjectKey` projection it searches) now exists in both
adapters, proven by the shared contract suite and the canonical FF-011
scenario, and `internal/application` exposes `DiscoverRequirementArtifactIDs`
and `DiscoverValidationPlanArtifactIDs` for a caller that does not already
know the population. This closes the one prerequisite the M.5 queries needed.
It is **not** the general HTTP API or UI implementation: no route, handler, or
UI screen exists yet. §16 still orders that work, now unblocked at the step
this section names.

### 6.3 Methods

`GET` and `POST` only. **No `PUT`, `PATCH`, or `DELETE` anywhere**, mirroring
`TestNoUpdateOrDeleteOnEngineeringTables` at the transport layer. An
architecture test asserts no route registers those verbs (§14).

### 6.4 UI routes (Phase B)

Seven screens, server-rendered, distinct from the API namespace:

```
GET /                                        Projects            (FF-001 §3.1)
GET /projects/{projectID}
GET /features/{featureCardID}                Feature overview    (§3.2)
GET /features/{featureCardID}/revisions      Revisions           (§3.3)
GET /features/{featureCardID}/requirements   Requirements        (§3.4)
GET /features/{featureCardID}/decisions      Decisions           (§3.5)
GET /features/{featureCardID}/validation     Validation          (§3.6)
GET /features/{featureCardID}/timeline       Timeline            (§3.7)
```

Screens submit HTML forms to the same API endpoints. The UI is a client of the
API, not a parallel path to the application layer — which is what makes it
evidence that the API is usable.

**As implemented (FF-021, [AD-028](../decisions/README.md#ad-028--browser-writes-go-through-the-existing-api-handler-in-process-never-a-second-network-hop)):** "the same
API endpoints" is preserved in substance, not literally — a browser cannot
send the frozen JSON contract directly, so each UI route invokes the
existing, unmodified API `http.Handler` in-process rather than issuing a
second HTTP request. The API remains the single owner of decoding, mapping,
application invocation, and error semantics; the UI adds no parallel path.
This sentence is left as originally written because it correctly states the
intent FF-018 §11 and this section then found unimplementable against the
frozen Phase A contract — the finding that FF-021 §2 records as evidence for
AD-028, not a plan this document silently abandoned.

## 7. Request and response conventions

**Encoding.** JSON request and response bodies; `application/json`. UI routes
return `text/html`.

**Request bodies mirror command structs, field for field, in transport-owned
DTOs.** No transport type embeds or aliases an `application`, `engineering`, or
`domain` type — decoding is explicit. This costs a mapping function per command
and buys the ability to change either side independently, which is the same
argument that justified envelopes in AD-013.

**Response envelope.** Every response carries the answer and, where the answer
is derived, its rationale:

```json
{
  "data":      { },
  "rationale": { }
}
```

`rationale` is **omitted when absent, never rendered empty**. A derived answer
without rationale is a bug, not an empty object — and §14's test asserts it.

**Identifiers are strings, always.** `ProjectID`, `FeatureCardID`, artifact and
revision IDs are already validated identity strings in the domain. The
transport passes them through and lets the domain constructors reject invalid
ones.

**Timestamps are RFC 3339 UTC**, matching what the adapters already normalise
to.

**No field is named for a derived state.** `TestNoDerivedStateOnFeatureCard`
polices this in the domain; transport DTOs must not reintroduce it — a
`FeatureCard` response carries no `status` field.

## 8. Error mapping

**One mapping point.** This directly inherits M.4's most expensive lesson: in
M.4, repositories translated driver errors eagerly, which destroyed the
information `UnitOfWork.Do` needed to decide whether to retry
(M.4 review §2.2). The rule that emerged — *translation belongs where the
decision is made, not where the error is raised* — applies unchanged here.
Handlers return errors; one function maps them to status codes.

| Application sentinel | Status | Reasoning |
|---|---|---|
| `ErrInvalidCommand`, field validation | `400` | Malformed intent |
| `ErrNotFound` | `404` | Named value absent |
| `ErrImmutableValueConflict` | `409` | Same identity, different content — the client is trying to rewrite history |
| `ErrCapabilityAlreadyLinked` | `409` | One-time link already made |
| `ErrReferencedValueMissing` | `422` | Well-formed but references something that does not exist |
| `ErrAcceptanceTransitionInvalid` | `422` | Legal request, illegal transition |
| `ErrCorrectionSelfReference`, `ErrCorrectionCycle`, `ErrCorrectionFamilyMismatch`, `ErrCorrectionTargetMissing` | `422` | Correction graph would be invalid |
| `ErrRevisionSequenceConflict`, `ErrCurrentRevisionAmbiguous`, `ErrRevisionOrderMissing`, `ErrRevisionReferenceMismatch` | `409` | Ordering invariant violated |
| `ErrEngineeringStateIndeterminate`, `ErrAmbiguousLifecycleState`, `ErrTimelineSourceInvalid` | `409` | The store is in a state no answer can be derived from |
| `ErrNestedTransaction` | `500` | A transport bug by construction — a handler called `Do` |
| `ErrTransactionAborted` | `503` + `Retry-After` | Retries exhausted; the request is safe to retry (§10) |
| unmapped | `500` | Never leak an internal message; log it, return an opaque body |

**Error body shape:**

```json
{ "error": { "code": "immutable_value_conflict", "message": "…" } }
```

`code` is a stable machine string derived from the sentinel; `message` is
human-facing. The mapping table is exhaustive over `internal/application`'s
sentinel set, and a test asserts every exported `Err*` in that package appears
in it — so adding a sentinel without deciding its status fails the build.

## 9. Validation strategy

**Three layers, each already established, none duplicated at the transport
edge.**

1. **Decode validation** (transport): is the JSON well-formed, are required
   fields present, are unknown fields rejected? Returns `400`. This is the only
   validation the transport owns.
2. **Command validation** (application): `requireNonEmpty`, identity format,
   value-object construction. Already implemented for all twelve commands.
   Returns `400` via `ErrInvalidCommand`.
3. **Invariant validation** (domain, engineering, repositories): acceptance
   transitions, referential integrity, correction-graph legality. Returns
   `422`/`409`.

**The transport must not re-implement layer 2 or 3.** Duplicating a check at
the edge means two definitions of validity that drift — the exact failure mode
AD-021 was written to close in the persistence layer. Where the transport
appears to need a check the application lacks, the check belongs in the
application.

**Unknown fields are rejected** (`DisallowUnknownFields`). A silently ignored
field is a request the client believes it made and the server did not.

## 10. Idempotency expectations

**Command endpoints are idempotent by construction, with no idempotency-key
mechanism.** This is a gift from the validated architecture, not a design
effort:

- Every write is create-only, keyed by a **client-supplied identity** —
  `ProjectID`, `ArtifactID`, `RevisionID`, `ClaimID`.
- The contract suite already proves, on both adapters, that an identical
  re-`Put` is a no-op (`IdempotentIdenticalPut`) and a differing one conflicts
  (`ConflictingPut`).

Therefore re-`POST`ing an identical command is safe and returns success;
re-`POST`ing the same identity with different content returns `409`. An
`Idempotency-Key` header would add a second, weaker mechanism on top of a
stronger one that already exists.

**Consequence for `ErrTransactionAborted` (`503`).** Because writes are
idempotent, a client may retry safely after retry exhaustion. This is why that
status carries `Retry-After` rather than being an opaque `500`.

**Open decision — identity generation at the transport edge.** The decision log
assigns this to M.5. The recommendation, following from the above: **accept
client-supplied identities; generate one only when the client omits it.**
Client-supplied identity is what makes idempotency free, so the default must
preserve it. Server generation is a convenience for the UI, which has no
meaningful identity to offer. To be recorded as AD-024, including the generator
choice — noting that no UUID dependency may be added (CLAUDE.md), so generation
uses `crypto/rand` and a project-defined format.

**Query endpoints are safe and cacheless.** All reads are computed (AD-006);
the transport adds no caching, because a cached derived answer is precisely the
staleness AD-006 exists to prevent.

## 11. Versioning strategy

**`/api/v1` prefix, no negotiation machinery.**

FeatureForge is a single-consumer, deliberately temporary POC frozen at M.7
(FF-000). There is no external client to break and no deprecation window to
honour. Content negotiation, version headers, or parallel handler trees would
be infrastructure for a problem this project has decided never to have.

The prefix is retained for one reason: it costs nothing now and makes the UI's
routes and the API's routes structurally distinguishable, which the
architecture tests can then assert on.

**Breaking changes within M.5 are permitted** while the API is unstable. The
API is frozen at the M.5 exit criteria, not before.

## 12. Authentication assumptions

**None. Explicitly and by acceptance criterion.**

FF-007's M.5 exit criteria require that "no authentication, notification, or
collaboration feature exists". FF-001 §3 specifies one local user.

The consequence for transport is precise: **the HTTP layer asserts no identity
and invents none.** The actor is already a fixed configuration value —
`featureforge:local-user`, established as `LocalActorRef` in
`internal/engineering/peos/vocabulary.go` — and every provenance record already
uses it. Transport does not accept an actor parameter, does not read one from a
header, and does not thread one through. Introducing an actor parameter would
be the first step toward multi-user semantics that AD-001 rejected.

**Binding.** The server binds to loopback by default. That is a deployment
default, not a security control, and the plan claims no security property.

## 13. Observability expectations

M.4 left retries entirely silent (M.4 review §3.3, E6) and deferred
observability to "the layer where watching begins" — which is this one.

**Minimum, standard library only (`log/slog`):**

- **Request logging:** method, path, status, duration. One line per request.
- **Retry exhaustion surfaced:** `ErrTransactionAborted` reaching the transport
  is logged at warn with the command name. This closes E6's gap at the only
  layer that can act on it.
- **Panic recovery:** a middleware that recovers, logs, and returns `500`. Note
  this does **not** interfere with `UnitOfWork.Do`'s panic path — `Do` rolls
  back and re-panics, and the middleware catches what escapes, so rollback
  still happens first.
- **Startup log:** adapter selected, migrations applied, address bound.

**Explicitly not in scope:** metrics backends, tracing, structured event
pipelines. No dependency may be added for observability, and none is needed for
a single-user local tool.

## 14. Testing strategy

Reusing the strategies M.4 validated, adapted to transport.

**Handler tests (`net/http/httptest`, stdlib).** Each endpoint tested against a
real in-memory-backed application stack — not a mocked application layer.
Mocking the layer under test would prove only that the mock matches the test.

**Error-mapping exhaustiveness.** A test enumerating every exported `Err*` in
`internal/application` and asserting each appears in the status map. Adding a
sentinel without classifying it fails the build.

**Scenario-through-HTTP.** The canonical FF-011 scenario driven end to end
through HTTP requests, asserting the same end state
`assertCanonicalEndState` already asserts for both adapters. This is the direct
analogue of M.4's parity proof: if the same scenario produces the same
engineering answers through a third access path, the transport genuinely adds
no logic. **This is the single highest-value test in M.5.**

**Both adapters.** The HTTP scenario test runs against in-memory always, and
against PostgreSQL when `FEATUREFORGE_POSTGRES_TEST_DSN` is set — matching the
established gating convention so `go test ./...` stays meaningful without
Docker.

**Architecture tests (§4.3, §17).** New boundary tests, each verified against a
deliberate violation before being trusted.

**Transport-model purity.** A test asserting no PEOS type appears in any
transport DTO, reusing the existing `ImportsPEOS` machinery. FF-007 lists this
as a deliverable.

**Rationale-presence.** A test asserting every derived answer in a response
carries rationale — the machine-checkable half of FF-001 §3.2.

**UI tests (Phase B).** Template execution against known data, asserting each
screen renders the fields FF-001 §3 requires. Deliberately not browser
automation: FF-001's usability acceptance is answered by a human reading, and a
test that clicks buttons would not answer it.

## 15. Exit criteria

**FF-007's M.5 exit criteria, unchanged and unsplit:**

- the full lifecycle is drivable through the UI;
- a reader answers every question in FF-001 §3's usability acceptance from the
  UI alone;
- superseded claims and prior revisions are visible, not hidden;
- no authentication, notification, or collaboration feature exists.

**Plus the implementation-verifiable criteria this plan adds:**

- every one of the twelve commands and the query surface is reachable over
  HTTP, and no endpoint contains business logic;
- the canonical scenario passes when driven through HTTP, on both adapters;
- no transport type references a PEOS type, test-enforced;
- no route registers `PUT`, `PATCH`, or `DELETE`;
- every application error sentinel has a classified status;
- the narrowed architecture tests pass, each verified against a deliberate
  violation;
- `go test ./...` remains meaningful without Docker.

The first four are the acceptance bar. The rest are how the work is checked on
the way there.

## 16. Implementation order

Phase A must complete before Phase B begins — the UI is evidence about the API,
so the API must be fixed first. FF-007 anticipates exactly this ordering.

**Phase A — HTTP API**

1. **Narrow the architecture tests** (§4.3). First, so the boundary exists
   before the code that needs it. Includes extending discovery to `cmd/` and
   removing the inert prefix check. Each new test verified against a
   deliberate violation.
2. **Transport skeleton**: package layout, router, middleware (logging,
   recovery), one trivial endpoint end to end to establish the pattern.
3. **Error mapping** plus its exhaustiveness test — before there are handlers
   to be inconsistent with each other.
4. **Command endpoints**, in canonical-scenario order (project → feature →
   capability → requirement → decision → plan → run → claim → correction →
   lifecycle), so the scenario becomes drivable incrementally.
5. **Query endpoints.** Endpoints needing only decisions, executions, claims,
   or evidence can be built using existing operations (§6.2). Endpoints
   needing a complete requirement or validation-plan population, which were
   **blocked on FF-016 §13**, are unblocked: FF-016 has landed, and
   `DiscoverRequirementArtifactIDs` / `DiscoverValidationPlanArtifactIDs` are
   available for a handler to call. No partial or claim-derived substitute may
   be used; AD-025 records why that produces a false result.
6. **`cmd/featureforge`**: configuration, adapter selection, migration on
   start, graceful shutdown.
7. **Scenario-through-HTTP test**, on both adapters. Phase A is complete when
   this passes.

**Phase B — Minimal UI**

8. **Template infrastructure**: layout, shared partials, rationale rendering
   components — rationale is the recurring element, so it is built once.
9. **Screens in dependency order**: Projects → Feature overview → Revisions →
   Requirements → Decisions → Validation → Timeline.
10. **Forms** submitting to the existing API endpoints.
11. **Usability pass** against FF-001 §3's acceptance questions, by reading.

**Closing**

12. Full verification, documentation (FF-015 amended to as-built, M.5
    implementation report), decision log entries, one commit sequence.

## 17. Decisions requiring a record

M.5 cannot be implemented without deciding these. Each must be recorded in
`docs/decisions/README.md` **before** the code that depends on it, per
CLAUDE.md's "do not make architecture decisions silently".

| Proposed | Decision |
|---|---|
| **AD-022** | **Accepted and implemented.** Intent-oriented HTTP API rather than resource-CRUD, following FF-010 §3's non-CRUD command decomposition (§5) |
| **AD-023** | **Accepted and implemented.** Narrowing the blanket `net/http`/template prohibition into named-holder import tests, extending architecture coverage to `cmd/` (§4.3) |
| **AD-024** | **Accepted and implemented.** Identity generation at the transport edge: client-supplied by default, server-generated only when omitted, no UUID dependency (§10) |
| **AD-025** | **Accepted and implemented.** Revision subject discovery: an optional `SubjectKey` projection on `RevisionEnvelope` and `RevisionEnvelopeRepository.ListByFamilyAndSubject` (§6.2) |

**On AD-025.** It was reserved conditionally when this plan was written —
"only if required" — and the condition was tested rather than assumed. The
investigation established that it *is* required, and simultaneously that it is
**narrower** than this plan anticipated: decisions, executions, claims, and
evidence need no change at all, so AD-025 covers only requirements and
validation plans. It is accepted and specified in
[FF-016](016-revision-subject-discovery.md); its implementation has landed,
clearing the prerequisite for §16 step 5. Revision subject discovery itself is
implemented — the general HTTP API and UI work §16 orders is not, and remains
future work.

**On AD-022, AD-023, and AD-024.** All three were resolved, recorded, and
implemented together as the Phase A HTTP transport
([FF-018](018-http-phase-a-implementation.md)), the implementation packet
that turned this plan's remaining open questions into an ordered,
architecture-decision-free implementation sequence. Full context,
alternatives, and consequences for each are in FF-018 §2 and in the decision
log (AD-022, AD-023, AD-024). Phase A itself — the twelve command and seven
query endpoints, `cmd/featureforge`, and the canonical scenario proven
end-to-end through HTTP on both adapters — is implemented; Phase B (the
minimal UI this plan also describes) is not, and remains future work.

## 18. Architectural risks

| # | Risk | Why it is credible | Mitigation |
|---|---|---|---|
| R1 | **Business logic leaks into handlers.** A handler grows a conditional to make a screen convenient | The most common failure mode for a transport layer, and the one FF-007 names explicitly | The scenario-through-HTTP test constrains behaviour to what commands already do; code review against "does this conditional decide anything?" |
| R2 | **The query ID-list gap (§6.2) forces a contract change under schedule pressure.** | It is a real gap with no obvious transport-only answer | Confront it early (step 5, not last); require evidence and AD-025; treat the M.4 freeze as binding |
| R3 | **Rationale is dropped for convenience.** Rendering a readiness badge is easier than a rationale table | FF-001 §3.2 anticipated this exact temptation ("A colour alone is not an explanation") | Machine-checked rationale-presence test; response envelope makes omission structural |
| R4 | **The UI accretes beyond "minimal".** Seven screens become nine; styling becomes a design system | Scope creep in a UI is nearly frictionless | Non-goals (§3) are explicit; screens enumerated from FF-001 §3; stdlib templates only |
| R5 | **A transport dependency creeps in.** A router or JSON library "just for convenience" | The dependency rule has held for four milestones, but a router is the classic first exception | `TestGoModHasOnlyApprovedRequirements` fails the build on any new direct requirement. `net/http`'s `ServeMux` handles the required routing |
| R6 | **Transport models drift into PEOS types.** Reusing an `engineering` envelope directly in a response is tempting | AD-005's boundary has held only because it is test-enforced | Test-enforced transport purity, listed by FF-007 as a deliverable |
| R7 | **Authentication arrives "just a little".** A user header, a name field | Every real system needs it, and this one deliberately does not | Exit criterion forbids it; transport takes no actor parameter (§12) |
| R8 | **Panic middleware masks a rollback failure.** Recovery could swallow a panic before `Do` rolls back | Ordering-dependent and easy to get wrong | `Do` rolls back and re-panics *before* the middleware sees anything; asserted by the contract suite's `RollbackOnPanic`, plus a transport-level test |

## 19. What this plan deliberately does not decide

Stated so that later readers do not mistake silence for oversight:

- **Response pagination.** The canonical scenario produces single-digit
  collections. Adding pagination now would be infrastructure for a load this
  project will never generate. Revisit only with evidence.
- **Concurrent-request behaviour beyond what M.4 validated.** The retry
  mechanism is proven at four concurrent writers; M.5 does not raise that bar
  and does not tune it.
- **Whether the UI needs client-side interactivity.** Assumed not, pending
  Phase B. If a screen proves unreadable without it, that is implementation
  evidence and gets a decision.
- **Deployment.** Out of scope for every phase before M.7.
