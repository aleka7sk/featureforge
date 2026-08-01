# FF-018 — M.5 Phase A HTTP Implementation

Status: Implemented (Phase M.5, Phase A). All eleven §16 steps and all
seven commits of §17 landed; see each step's "As implemented" note and §22.
Date: 2026-07-28
Phase: M.5 — Phase A (HTTP API)
Governs: the complete Phase A HTTP transport, its application-layer additions,
its package and import boundaries, its executable composition, and the ordered
sequence by which they land.

## Numbering note

"FF-017" is already used in committed documentation as the work label for the
AD-026 `SubjectKey`-equality implementation — see
`docs/decisions/README.md` (AD-025's correction pointer) and
`docs/spec/016-revision-subject-discovery.md` §13 step 1. No
`docs/spec/017-*.md` file exists and none will be created. **FF-018 is
therefore the next unambiguous feature-spec identifier**, and is the number
used throughout this document.

## Governance statement

PEOS-000 through PEOS-009 remain authoritative for every PEOS concept.
`docs/spec/` governs FeatureForge. This document adds no PEOS concept, renames
none, and redefines none. PEOS v1.0.0 is used unchanged.

**No HTTP implementation lands in this task.** FF-018 is the complete
implementation packet for M.5 Phase A: it resolves every remaining HTTP-level
planning question and defines an executable implementation sequence, so that a
subsequent implementation agent executes it without making an unresolved
architectural decision.

FF-018 builds on [FF-015](015-http-api-and-ui.md), which remains the accepted
plan and the source of the endpoint surface, the API philosophy, and the
constraint inventory. Where FF-015 left something reserved or imprecise,
FF-018 resolves it and says so explicitly. **FF-018 does not amend FF-015**;
FF-015 is unmodified by this task.

### Evidence base

Every function, type, sentinel, route, and test named below was verified
against the repository at the time of writing, not carried over from planning
prose. Where verification contradicted a planning input, the repository wins
and the discrepancy is recorded (see §11 on the sentinel count, and §10 on
`ParseEvidenceKey`).

**Forward correction (AD-030, FF-022).** Later replay and aggregate-integrity
evidence found three Phase A conclusions incomplete: §3.1's C7/C9 DTOs omit
the acceptance identity their corrected complete acts create; §6.6 mistakes
missing Validation Plan order/acceptance implementation for a governing model
rule; and §11 infers whole-command replay from repository equality. The route
count, paths, success statuses, response bodies, and create-only character of
the commands remain. FF-022 governs the added C7/C9 field, FF-004 plan-current
resolution, and application-level replay recognition. Original implementation
history below remains visible rather than being rewritten retrospectively.
The same correction threads the read inspector through Q3/Q4/Q5 discovery:
subject projection enumerates candidates, while complete Requirement/Plan
history validation authorizes their use and rejects mixed subject/scope state
as `500 internal_error` before plan ambiguity.

---

## 1. Scope

### 1.1 Phase A contains

- The HTTP API only.
- **Twelve command endpoints** — verified against FF-015 §6.1 and against the
  twelve `*Command` structs in `internal/application`
  (`CreateProjectCommand`, `CreateFeatureCommand`,
  `EstablishCapabilitySpecificationCommand`,
  `ReviseCapabilitySpecificationCommand`, `AcceptCapabilityRevisionCommand`,
  `AssignLifecycleStateCommand`, `EstablishRequirementCommand`,
  `RecordArchitectureDecisionCommand`, `EstablishValidationPlanCommand`,
  `RecordValidationRunCommand`, `RecordValidationClaimCommand`,
  `CorrectValidationClaimCommand`).
- **Seven query endpoints** — verified against FF-015 §6.2.
- `cmd/featureforge` composition.
- Memory-backed HTTP scenario coverage.
- PostgreSQL-backed HTTP scenario coverage when `FEATUREFORGE_POSTGRES_TEST_DSN`
  is set.
- The narrow application and engineering additions §9 and §10 specify, which
  exist solely to make the query endpoints answerable without caller-supplied
  identifier lists.

### 1.2 Phase A excludes

UI, templates, browser screens, all of Phase B, authentication, authorization,
pagination, metrics, tracing, distributed-deployment concerns, generic CRUD,
GraphQL, gRPC, WebSocket, SSE, external router frameworks, `Idempotency-Key`
headers, repository-interface redesign, `UnitOfWork` redesign, and any
migration not proven necessary by an implementation requirement. **No migration
is required by Phase A** (§17.4).

FF-015 §19's deliberate non-decisions carry forward unchanged.

---

## 2. Binding resolution of the reserved AD questions

FF-015 §17 records AD-022, AD-023, and AD-024 as proposed and unresolved, and
its closing line states "AD-022, AD-023, and AD-024 remain proposed and
undecided." Verified: `docs/decisions/README.md` contains no AD-022, AD-023, or
AD-024 section. Implementing Phase A against three undecided architecture
questions would violate CLAUDE.md's "do not make architecture decisions
silently."

> **FF-018 resolves the questions reserved as AD-022, AD-023 and AD-024 and
> records their binding decisions in this specification. Standalone
> decision-log pointers may be added later, but are not required for
> implementation because CLAUDE.md permits material decisions to be recorded
> in product specifications.**

No standalone AD-022, AD-023, or AD-024 file exists, and this document does not
claim otherwise. The substantive analysis behind each resolution is FF-015's
own — §5, §4.3, and §10 respectively. FF-018 promotes that analysis to a
binding decision; it does not redesign it.

### 2.1 Reserved AD-022 — HTTP transport contract and public surface

**Context.** FF-010 §3 decomposed the application layer into commands named for
engineering acts, with no `Update*`, no `Delete*`, and no generic
`RecordEngineeringAct`. A resource-CRUD HTTP surface would have to invent
`PUT`/`DELETE` semantics for immutable engineering values and the bounded
stable-establishment operational surface later made explicit by AD-031, and
would flatten twelve distinct engineering acts into four verbs.

**Decision.** The public HTTP surface is **intent-oriented, not resource-CRUD**.
One endpoint per command, one endpoint per query. `GET` and `POST` only; no
`PUT`, `PATCH`, or `DELETE` route may be registered anywhere. Endpoint paths
name the act or the question. The transport is a translator: decode, invoke,
encode, map errors — nothing else. The exact surface is §3's matrix.

**Alternatives considered.** *Resource-CRUD* rejected — it cannot express
immutability honestly and loses the act-level distinctions the domain exists to
preserve. *A single `POST /commands` envelope with a discriminator* rejected —
it moves dispatch into a body field, defeats method/route-level architecture
tests, and makes every endpoint's contract invisible in the route table.
*GraphQL* rejected — a query language over a model whose queries are already
fixed, composed, and rationale-bearing adds a resolver layer with no consumer
asking for it.

**Consequences.** The route table is a readable list of the engineering acts
the system supports. `TestNoUpdateOrDeleteOnEngineeringTables`'s intent gains a
transport-level analogue (§18 criterion 2). Adding an act means adding a
command *and* a route, both visible in review.

**Scope boundary.** Governs the Phase A API surface only. It decides nothing
about Phase B's UI routes, which FF-015 §6.4 sketches and which remain
undecided until Phase B is planned.

### 2.2 Reserved AD-023 — Package ownership and named-holder import permissions

**Context.** `TestNoHTTPDatabaseUIOrAIPackage`
(`internal/architecture/architecture_test.go:399`) forbids importing
`net/http`, `database/sql`, `html/template`, and `text/template` anywhere under
`internal/`, and forbids packages whose path begins with `http`, `ui`, `ai`,
`postgres`, or `sql`. Phase A must import `net/http`. FF-015 §4.3 investigated
this and found three defects, all re-verified here:

1. **Stale milestone wording** — the failure messages still say "which M.3 must
   not use" and "forbidden package present for M.3".
2. **The `postgres`/`sql` prefix check is inert** — it matches the path
   *relative to `internal/`*, so `infrastructure/postgres` never matched. The
   real boundary is enforced by `TestOnlyPostgresInfrastructureImportsDriver`.
3. **`cmd/` is entirely unchecked** — `InternalPackages()`
   (`internal/architecture/imports.go:38`) roots at `internal`, and every
   `walkGoFiles` call joins `ModuleRoot(), "internal"`. Phase A introduces
   `cmd/featureforge`, making this blind spot load-bearing.

**Decision.** **Narrow the prohibition into named-holder permissions; do not
remove it.** The codebase already proves this shape twice —
`TestOnlyIntegrationPackageImportsPEOS` (M.3) and
`TestOnlyPostgresInfrastructureImportsDriver` (M.4). Phase A reuses it rather
than inventing a third form.

| Import | Permitted holder | Forbidden everywhere else |
|---|---|---|
| `net/http` | `internal/transport/http`, `cmd/featureforge` | domain, engineering, application, infrastructure |
| `html/template` | the Phase B UI package only — no holder exists in Phase A | everywhere |
| `text/template` | none | everywhere |
| `database/sql`, `database/sql/driver` | none | everywhere — absolute, per AD-020's native-pgx decision |
| PEOS SDK | `internal/engineering/peos` (AD-005, unchanged) | everywhere, now including `cmd/` |
| `github.com/jackc/pgx` | `internal/infrastructure/postgres` (AD-020, unchanged) | everywhere, now including `cmd/` — **except** `cmd/featureforge` may hold a `*pgxpool.Pool` obtained from `postgres.Connect`; see §17.2 |

Package discovery is extended to `cmd/` (§16) so the PEOS and driver boundaries
cover the executable. The inert prefix check is dropped; the import-based tests
state the real boundary.

**Alternatives considered.** *Delete the test* rejected — it discards a
boundary at the exact moment the boundary begins to matter. *Add `net/http` to
an allow-list inside the existing test* rejected — it keeps one test with four
unrelated intents, so a failure message would not say which boundary broke.
*Leave `cmd/` unchecked* rejected — the executable is precisely where a
shortcut import is most tempting and least visible.

**Consequences.** Four focused tests replace one broad one, each failing with a
message naming its own boundary. `cmd/` gains architecture coverage it has
never had. Each new test must be verified against a deliberate violation before
being trusted (§18 criterion 16) — the discipline the M.4 review confirmed and
FF-017 re-applied.

**Scope boundary.** Governs import permissions and architecture-test shape.
It decides nothing about Phase B's UI package beyond reserving
`html/template` for it, and it does not weaken any existing internal check.

### 2.3 Reserved AD-024 — Executable composition and runtime boundary

**Context.** Phase A introduces the module's first executable. FF-015 §10 also
reserved identity generation at the transport edge to this decision.

**Decision, in two parts.**

*Composition.* `cmd/featureforge` is the sole composition root. It reads
configuration from environment variables, selects one persistence adapter,
applies migrations when the adapter requires them, constructs the application
dependencies, builds the router, and owns the server lifecycle. It contains no
application logic, no transport logic, and no engineering logic — only wiring.
Details in §17.

*Identity generation.* **Client-supplied identity is accepted and is the
default.** The server generates an identity only when the client omits it for a
field the command requires. Client-supplied identity is what makes every write
idempotent for free (§14); a server-generated default would destroy that
property for the caller. No UUID dependency may be added (CLAUDE.md, and
`TestGoModHasOnlyApprovedRequirements` fails the build on any new direct
requirement), so generation uses `crypto/rand` with a project-defined format.

**For Phase A the generator is not built.** Every Phase A endpoint requires the
client to supply the identity, because the only Phase A client is a test
driving the canonical scenario, which knows every identity by construction.
Server generation exists to serve Phase B's UI forms, which have no meaningful
identity to offer. Building it now would be untested infrastructure for an
absent caller. **The decision is recorded here so Phase B implements it without
re-deciding; the code is Phase B's.** A Phase A request omitting a required
identity is `400 ErrInvalidCommand`, which is what the application layer
already returns.

**Alternatives considered.** *Server-generated identity by default* rejected —
it removes free idempotency, which §14 shows is the strongest property the
validated architecture hands the transport. *A UUID library* rejected — a new
direct dependency, build-failing. *Building the generator in Phase A* rejected
as untested infrastructure for a caller that does not exist until Phase B.

**Consequences.** Phase A's command endpoints are pure pass-through on
identity. Phase B adds one generator and one omission branch per affected
endpoint, against a decision already made.

**Scope boundary.** Governs `cmd/featureforge`'s responsibilities and the
identity-generation policy. It decides nothing about deployment, TLS,
supervision, or configuration beyond the minimal set §17.1 lists.

---

## 3. Public HTTP surface

Reproduced from FF-015 §6.1 and §6.2. **Twelve command endpoints, seven query
endpoints, nineteen total.** All paths carry the `/api/v1` prefix (FF-015 §11).
`GET` and `POST` only.

In the matrix below, *App entry point* names the exact exported function or
method verified in `internal/application`. **Every command handler's success
status is `201 Created`** — each command establishes a stable semantic act (an
immutable engineering record or AD-031 operational establishment) — and
**every query handler's is `200 OK`**. Every endpoint may additionally return
`400`, `500`, and `503` per §11; the *Errors* column lists only the
endpoint-specific ones beyond those.

### 3.1 Command endpoints

| # | Operation | Method | Path | Intent | Request source | Request DTO | App entry point | Errors (beyond 400/500/503) |
|---|---|---|---|---|---|---|---|---|
| C1 | `CreateProject` | POST | `/api/v1/projects` | Establish a project | body | `createProjectRequest` | `CreateProjectCommand.Execute(ctx, uow, clock)` | 409 |
| C2 | `CreateFeature` | POST | `/api/v1/features` | Establish a feature card | body | `createFeatureRequest` | `CreateFeatureCommand.Execute(ctx, uow, clock)` | 409, 422 |
| C3 | `EstablishCapabilitySpecification` | POST | `/api/v1/capabilities` | Found a capability + revision 1 | body | `establishCapabilityRequest` | `EstablishCapabilitySpecificationCommand.Execute(ctx, uow, recorder, clock)` | 409, 422 |
| C4 | `ReviseCapabilitySpecification` | POST | `/api/v1/capabilities/{artifactID}/revisions` | Add a revision | path + body | `reviseCapabilityRequest` | `ReviseCapabilitySpecificationCommand.Execute(ctx, uow, recorder, clock)` | 409, 422 |
| C5 | `AcceptCapabilityRevision` | POST | `/api/v1/capabilities/{artifactID}/acceptances` | Append an acceptance entry | path + body | `acceptRevisionRequest` | `AcceptCapabilityRevisionCommand.Execute(ctx, uow, clock)` | 409, 422 |
| C6 | `AssignLifecycleState` | POST | `/api/v1/capabilities/{artifactID}/lifecycle` | Record a lifecycle assignment | path + body | `assignLifecycleRequest` | `AssignLifecycleStateCommand.Execute(ctx, uow, recorder, clock)` | 409, 422 |
| C7 | `EstablishRequirement` | POST | `/api/v1/requirements` | Establish a requirement | body | `establishRequirementRequest` | `EstablishRequirementCommand.Execute(ctx, uow, recorder, clock)` | 409, 422 |
| C8 | `RecordArchitectureDecision` | POST | `/api/v1/decisions` | Record a decision | body | `recordDecisionRequest` | `RecordArchitectureDecisionCommand.Execute(ctx, uow, recorder, clock)` | 409, 422 |
| C9 | `EstablishValidationPlan` | POST | `/api/v1/validation/plans` | Establish a plan | body | `establishPlanRequest` | `EstablishValidationPlanCommand.Execute(ctx, uow, recorder, clock)` | 409, 422 |
| C10 | `RecordValidationRun` | POST | `/api/v1/validation/runs` | Record an execution + evidence | body | `recordRunRequest` | `RecordValidationRunCommand.Execute(ctx, uow, recorder, clock)` | 409, 422 |
| C11 | `RecordValidationClaim` | POST | `/api/v1/validation/claims` | Record a claim | body | `recordClaimRequest` | `RecordValidationClaimCommand.Execute(ctx, uow, recorder, clock)` | 409, 422 |
| C12 | `CorrectValidationClaim` | POST | `/api/v1/validation/claims/corrections` | Record a correcting claim | body | `correctClaimRequest` | `CorrectValidationClaimCommand.Execute(ctx, uow, recorder, clock)` | 409, 422 |

Two shape observations carried from FF-015 §6.1, both load-bearing:
`acceptances` is a collection because AD-015 made acceptance an append-only
journal with no stored field — there is deliberately no `PUT .../acceptance`;
and corrections are a sub-collection of claims because AD-017 models a
correction as a *new* claim referencing an earlier one, not a mutation.

**Path/body precedence.** For C4, C5, C6 the `{artifactID}` path value is
authoritative. If the body also carries an artifact ID field, it must be
absent — `DisallowUnknownFields` (§12) makes a duplicate a `400`, which is the
intended outcome. Request DTOs for these three therefore omit the artifact ID
entirely; the handler injects the path value when building the command.

**Request DTO field mapping.** Each request DTO mirrors its command struct
field-for-field (FF-015 §7), in transport-owned types, with `snake_case` JSON
names. The verified command fields are:

- **C1** `project_id`, `name`
- **C2** `feature_card_id`, `project_id`, `title`, `description`
- **C3** `feature_card_id`, `artifact_id`, `revision_id`, `content`
- **C4** *(path `artifactID`)* `revision_id`, `content`
- **C5** *(path `artifactID`)* `record_id`, `revision_id`, `state`, `reason`, `effective_at`
- **C6** *(path `artifactID`, mapped to `SubjectArtifactID`)* `assignment_id`, `state`, `effective_at`, `transition_record_artifact_id`, `transition_record_revision_id`, `is_entry`, `transition_key`, `from_assignment_id`, `attempted_at`, `completed_at`
- **C7** `artifact_id`, `revision_id`, `statement`, `subject_artifact_id`, `acceptance_record_id`, `source_capability_revision_id`, `source_acceptance_criterion_key`
- **C8** `decision_id`, `subject_artifact_id`, `subject_revision_id`, `question`, `outcome_statement`, `alternatives`, `evidence_artifact_id`, `evidence_revision_id`, `assumptions`, `constraints`, `uncertainties`, `rationale`
- **C9** `artifact_id`, `revision_id`, `scope_artifact_id`, `acceptance_record_id`, `activities` (each: `key`, `subject_artifact_id`, `subject_revision_id`, `method`, `outcome_interpretation`, `requirement_artifact_id`, `requirement_revision_id`, `expected_evidence`)
- **C10** `execution_id`, `plan_artifact_id`, `plan_revision_id`, `activity_key`, `subject_artifact_id`, `subject_revision_id`, `method`, `outcome`, `completed_at`, `evidence_artifact_id`, `evidence_revision_id`, `evidence_locator`
- **C11** `claim_id`, `scope_artifact_id`, `subject_artifact_id`, `subject_revision_id`, `requirement_artifact_id`, `requirement_revision_id`, `outcome`, `method`, `evidence_artifact_id`, `evidence_revision_id`, `execution_id`, `reasoning`, `timestamp`
- **C12** as C11, plus `correction_target`, `correction_kind`

**Forward C8 correction (FF-024 §3.3).** The C8 field inventory remains the
same, but `subject_revision_id` is presence-semantic: omitted or exact empty
means the required `subject_artifact_id` is the Decision's Artifact-level
subject; a present non-empty value names the exact Revision-level subject.
This exposes both Decision subject forms already governed by FF-004 §3.3.

**The `content` field (C3, C4).** `engineering.CapabilitySpecificationContent`
is an opaque value type with unexported fields and a builder API
(`NewCapabilitySpecificationContent`, then `WithUserOutcome`,
`WithFunctionalBehaviours`, `WithConstraints`, `WithAcceptanceCriteria`,
`WithDependencies`, `WithOpenQuestions`). The transport **must not** marshal or
unmarshal it directly. The DTO declares a plain struct —
`schema_version`, `title`, `problem_statement`, `user_outcome`,
`functional_behaviours`, `constraints`, `acceptance_criteria` (each `key`,
`text`), `dependencies`, `open_questions` — and a mapping function calls the
builder chain in order, returning the first builder error as `400`. This is the
one DTO with non-trivial mapping, and it is deliberate: the builders carry
validation the transport must not duplicate (§12).

**Command response body.** Each command returns its own `*Result` struct
(e.g. `EstablishRequirementResult{ArtifactKey, RevisionKey}`). The response DTO
renders the created identities as strings using the verified `String()` forms:
`ArtifactKey.String()` → `artifactID`; `RevisionKey.String()` →
`artifactID + "/" + revisionID`; `RecordKey.String()` → `kind + ":" + id`. No
command response carries `rationale` — nothing about a write is derived.

### 3.2 Query endpoints

| # | Operation | Method | Path | Question answered | App entry point(s) | Rationale in response |
|---|---|---|---|---|---|---|
| Q1 | `ListProjects` | GET | `/api/v1/projects` | Which projects exist? | `Repositories.Projects.List` via a thin application read (§6.5) | no |
| Q2 | `ListFeatures` | GET | `/api/v1/projects/{projectID}/features` | Which feature cards does this project hold? | `Repositories.FeatureCards.ListByProject` via a thin application read (§6.5) | no |
| Q3 | `GetFeature` | GET | `/api/v1/features/{featureCardID}` | Composed overview of this feature | `GetFeatureEngineeringState` + the card itself | yes |
| Q4 | `GetFeatureState` | GET | `/api/v1/features/{featureCardID}/state` | Current engineering state | `GetFeatureEngineeringState` | yes |
| Q5 | `GetFeatureTimeline` | GET | `/api/v1/features/{featureCardID}/timeline` | What happened, in order? | `GetFeatureTimeline` | yes (per event) |
| Q6 | `ListCapabilityRevisions` | GET | `/api/v1/capabilities/{artifactID}/revisions` | Which revisions exist, and which is current? | `ResolveCurrentRevision` + `Repositories.Revisions.ListByArtifact` via §6.5 | yes |
| Q7 | `GetCapabilityRevision` | GET | `/api/v1/capabilities/{artifactID}/revisions/{revisionID}` | This exact revision | `Repositories.Revisions.Get` via §6.5 | no |

**Q3, Q4, Q5 require discovery.** `EngineeringStateInput` and `TimelineInput`
both require caller-supplied identifier lists, and an HTTP client holding only
a `FeatureCardID` has none. §9 specifies exactly how the handler obtains them.
The chain is: `FeatureCardID` → `FeatureCards.Get` →
`FeatureCard.CapabilityArtifactID() (string, bool)` (verified accessor) → the
discovery functions. A card with no linked capability yields a well-formed
response with an empty engineering state, not an error.

**Q7 404 semantics.** `Revisions.Get` returns `(value, found bool, error)` with
no error when absent. The handler maps `found == false` to `404` with code
`not_found`; it does not manufacture `ErrNotFound`.

---

## 4. Handler boundary

**One handler function per endpoint.** Nineteen handlers.

A handler performs exactly six steps, in order:

1. HTTP request decoding (§12).
2. Transport-level syntax validation (§12).
3. DTO → application-input mapping.
4. Invocation of exactly one approved application entry point.
5. Application result → response DTO mapping (§13).
6. Centralized error mapping (§11) on any error from steps 1–4.

**A handler must not:**

- access `application.Repositories` directly;
- call `UnitOfWork.Do` — the atomicity boundary is the command, and a handler
  calling `Do` produces `ErrNestedTransaction`, which §11 classifies `500`
  precisely because it is a transport bug by construction;
- decode or reference any PEOS type;
- inspect domain state to reproduce an application rule;
- implement readiness, ordering, correction-chain, or lifecycle logic;
- reconstruct requirements from claims — AD-025 records why that produces a
  false result, and §6.4 forbids it structurally;
- depend on `internal/infrastructure/memory` or `internal/infrastructure/postgres`;
- expose an internal error message (§11.3).

**Domain and engineering invariants remain owned by their existing layers.**
The transport adds no validity rule. Where the transport appears to need a
check the application lacks, the check belongs in the application — the same
reasoning AD-021 applied to the persistence layer.

**A test-visible consequence.** `TestNoOperationalScenarioEntity`
(`architecture_test.go:305`) forbids any identifier under `internal/`
containing `teacher`, `student`, `lesson`, `homework`, `attachment`, or
`notification`. Transport DTO field names and Go identifiers must respect it;
scenario *content* travels as string values, never as field names.

---

## 5. Package layout

### 5.1 `internal/transport/http`

Flat files grouped by concern, matching `internal/infrastructure/postgres`'s
own convention. No sub-packages.

```
internal/transport/http/
    router.go          route registration, path-value extraction, 404/405
    middleware.go      request logging, panic recovery
    errors.go          the single error-mapping function + JSON error shape
    dto_command.go     the twelve request DTOs + content mapping
    dto_query.go       the seven response DTOs + envelope
    handlers_command.go
    handlers_query.go
    server.go          Handler construction from injected dependencies
    *_test.go
```

| Aspect | Value |
|---|---|
| **Responsibility** | Decode, invoke, encode, map errors. Nothing else. |
| **Permitted imports** | `net/http`, `encoding/json`, `errors`, `fmt`, `io`, `log/slog`, `time`, `strings`, `sort`; `internal/application`, `internal/domain`, `internal/engineering` |
| **Forbidden imports** | `internal/infrastructure/memory`, `internal/infrastructure/postgres`, any PEOS package, `database/sql`, any driver, `html/template`, `text/template` |
| **Public constructor** | `func NewHandler(deps Dependencies) http.Handler` |
| **Injected dependencies** | `Dependencies{ UOW application.UnitOfWork; Recorder application.EngineeringRecorder; Clock application.Clock; Logger *slog.Logger }` |
| **Owns** | JSON encoding/decoding, error mapping, route table |
| **Does not own** | Server lifecycle, adapter selection, migrations |

`NewHandler` returns `http.Handler`, not `*http.Server` — the package builds a
handler; `cmd/featureforge` builds the server. That split is what keeps
`httptest` usage trivial for every handler test this document requires and
keeps lifecycle out of the transport package.

**Why `internal/engineering` is a permitted import.** Query response DTOs read
projected fields from `engineering.RevisionEnvelope` and
`engineering.RecordEnvelope` (for example `Outcome`, `SubjectKey`,
`RecordedAt`) because the application result types embed those envelopes.
This is reading a FeatureForge-owned projection, not a PEOS type — and
`internal/engineering` is itself PEOS-free, enforced by
`TestEngineeringDoesNotImportPEOS`. The transport never touches `Payload`.

### 5.2 `cmd/featureforge`

```
cmd/featureforge/
    main.go        configuration, adapter selection, migration, lifecycle
    main_test.go   only if a composition helper warrants one
```

| Aspect | Value |
|---|---|
| **Responsibility** | Wiring and process lifecycle only |
| **Permitted imports** | `net/http`, `context`, `os`, `os/signal`, `syscall`, `time`, `log/slog`, `errors`; `internal/transport/http`, `internal/application`, `internal/infrastructure/memory`, `internal/infrastructure/postgres`, `internal/engineering/peos` |
| **Forbidden** | Application, transport, domain, or engineering *logic*; a driver import other than through `postgres.Connect` |
| **Owns** | Server lifecycle, adapter selection, migration invocation, configuration |

`cmd/featureforge` is the one place that legitimately imports both adapters and
`internal/engineering/peos` (to construct the `Recorder`) — that is what a
composition root is. It is covered by architecture tests for the first time
(§16).

---

## 6. Application composition for discovery

### 6.1 The placement question, resolved

The M.5 contract investigation's Option G recommends implementing decision,
execution, claim, and evidence discovery "in transport". FF-015 §4.1 freezes
the opposite: *"Repository contracts | Transport never touches them. It calls
commands and queries, which own `Repositories` access"* and *"`UnitOfWork` |
… Transport never calls `Do`."*

**Resolution: the composition belongs to `internal/application`, not to the
HTTP handlers.** This is the only reading consistent with the frozen boundary,
and it is a minimal extension of the pattern `DiscoverRequirementArtifactIDs`
and `DiscoverValidationPlanArtifactIDs` already established in FF-016 — a
read-only query function, not a new repository method, not a new architectural
direction. Handlers call these functions exactly as they call
`GetFeatureEngineeringState`.

The investigation's wording is not overridden on substance: its point was that
*no contract change and no decision record* is needed for these four families,
which remains true. Only the placement is corrected, because "transport" there
would have required the transport to hold `Repositories`.

### 6.2 Required application additions

All read-only. All in `internal/application`. Each integrity-sensitive
signature receives the PEOS-free `EngineeringReplayInspector` authority in
addition to `context.Context` and `Repositories`.

```go
// query_state.go (beside DiscoverRequirementArtifactIDs)
func DiscoverDecisionIDs(ctx context.Context, repos Repositories, inspector EngineeringReplayInspector, capabilityArtifactID string) ([]string, error)

// query_timeline.go (beside DiscoverValidationPlanArtifactIDs)
func DiscoverExecutionAndClaimIDs(ctx context.Context, repos Repositories, inspector EngineeringReplayInspector, capabilityArtifactID, capabilityRevisionID string) (executionIDs, claimIDs []string, err error)

func DiscoverExecutionAndClaimIDsAllRevisions(ctx context.Context, repos Repositories, inspector EngineeringReplayInspector, capabilityArtifactID string) (executionIDs, claimIDs []string, err error)

func DiscoverEvidenceArtifactIDs(ctx context.Context, repos Repositories, inspector EngineeringReplayInspector, executionIDs, claimIDs []string) ([]string, error)

func DiscoverDecisionEvidenceArtifactIDs(ctx context.Context, repos Repositories, inspector EngineeringReplayInspector, decisionIDs []string) ([]string, error)
```

**Forward authoritative-discovery correction (AD-032, FF-023).** The initial
M.5 implementation used family/kind/subject projection queries. Those remain
valid indexed adapter operations but are not completeness witnesses: an
inverse projection mismatch would disappear before the application could
inspect it. `listValidatedRevisions` and `listValidatedRecords` now enumerate
the complete populations through deterministic repository `ListAll`, validate
every payload/digest/projection, and only then allow family, kind or subject
filtering. All discovery functions preserve deduplication and `sort.Strings`
determinism after that validation.

### 6.3 Exact semantics

**Decision discovery.** A Decision's subject is either the capability Artifact
or one exact capability Revision (FF-004 §3.3 and the forward C8 correction in
§3.2). Projection fidelity covers both `ArtifactSubjectKey("CAP-1")` and
`ArtifactRevisionSubjectKey("CAP-1", "CAP-1-REV-1")`. Enumerate and inspect all
Artifact, Revision and Record envelopes first. Build the exact subject set from
the validated capability Artifact plus all validated revisions of that
Artifact, then select validated Decisions whose authoritative subject belongs
to that set. Validate each Decision's complete capability reference before
rendering it; collect `Key.ID`, dedupe, and sort ascending.

Every revision is consulted, not only the current one — a decision recorded
against revision 1 remains part of the feature's history after revision 2
exists, and the timeline must show it.

**Execution and claim discovery.** Executions and claims name a capability
revision as subject (verified for both by
`TestProjectionFidelity_ExecutionRecord` and `TestProjectionFidelity_Claim`).
Enumerate and inspect all Records, then select the two kinds by the exact
validated capability-revision subject. Each result is deduplicated and sorted
ascending independently. Q5 unions this population across every validated
revision of the capability; Q3/Q4 current-state resolution remains scoped to
the current revision.

The caller passes the *current* revision ID, obtained from
`ResolveCurrentRevision`. Where a history-wide view is wanted (the timeline),
the caller iterates revisions as decision discovery does; §6.4's handler
sequence specifies which each endpoint uses. **§24 records that the initial
implementation did not actually do this for Q5, and corrects it** —
`DiscoverExecutionAndClaimIDsAllRevisions` is the history-wide function the
timeline uses; this function remains exactly Q3/Q4's current-revision-scoped
one.

**Evidence discovery.** Decisions, Executions and Claims project exact
`EvidenceKeys` (`engineering.EvidenceKey(artifactID, revisionID)` →
`"evidence:" + artifactID + "/" + revisionID`). Q5 validates each selected
record and every mandatory cross-reference, parses every exact key, resolves
and inspects the cited Evidence Artifact/Revision pair, then deduplicates and
sorts the resulting artifact IDs. The final population is the union of
Decision-basis Evidence and the Evidence cited by history-wide Executions and
Claims. A deliberately unresolved C8 citation returns
`ErrTimelineSourceInvalid` (409); a dangling Execution/Claim citation is stored
corruption (500). Neither case is silently omitted.

**No application input shape is redesigned for transport convenience.**
`EngineeringStateInput` (`CapabilityArtifactID`, `RequirementArtifactIDs`,
`DecisionIDs`) and `TimelineInput` (`Project`, `FeatureCard`,
`CapabilityArtifactID`, `RequirementArtifactIDs`, `DecisionIDs`,
`PlanArtifactID`, `ExecutionIDs`, `EvidenceArtifactIDs`, `ClaimIDs`) keep their
exact verified shapes. Discovery fills them; it does not replace them.

### 6.4 Handler composition sequence for Q3/Q4/Q5

Inside a single `uow.Do` — invoked by the *query composition function*, not by
the handler (§4) — the sequence is:

1. `FeatureCards.Get(featureCardID)` → the card; `404` if absent.
2. `card.CapabilityArtifactID()` → `(artifactID, ok)`. If `!ok`, return an
   empty-but-well-formed state; the feature exists and has no capability yet.
3. `ResolveCurrentRevision(ctx, repos, artifactID)` → current revision.
4. `DiscoverRequirementArtifactIDs(ctx, repos, inspector, artifactID)` — **the only
   permitted source of the requirement population**.
5. `DiscoverValidationPlanArtifactIDs(ctx, repos, inspector, artifactID)` →
   `PlanArtifactID`, resolved by the exactly-one contract in §6.6. **Sort order
   never selects a plan.**
6. `DiscoverDecisionIDs` for all three queries. Q3/Q4 let readiness resolve
   current Claims against the current revision and do not build a separate
   execution/evidence population. Q5 uses history-wide
   `DiscoverExecutionAndClaimIDsAllRevisions`, validates the selected
   Executions/Claims, and unions their exact Evidence with
   `DiscoverDecisionEvidenceArtifactIDs`; every envelope is inspected before
   kind/subject filtering (§24, corrected again by AD-032/FF-023).
7. Build `EngineeringStateInput` / `TimelineInput`; call
   `GetFeatureEngineeringState` / `GetFeatureTimeline`.

Because steps 1–7 all need `Repositories`, this composition is itself an
application-layer function. **Recommended:** one exported function per query
endpoint —
`GetFeatureOverview(ctx, uow, featureCardID) (FeatureOverviewResult, error)`
and equivalents — so the handler makes exactly one call, satisfying §4 step 4
literally. The implementer may name these to match existing convention; the
binding constraint is that the handler holds no `Repositories` and calls no
`Do`.

**Step 4 is load-bearing and must not be replaced.** Deriving requirements from
claims omits FF-011's `REQ-4`, which has no plan activity and no claim, and
readiness would then report a falsely complete result. AD-025 records the full
argument; §18 makes it an acceptance criterion.

### 6.5 Thin reads for Q1, Q2, Q6, Q7

Q1, Q2, Q6, and Q7 need repository reads with no composition. Since the
transport may not hold `Repositories`, each needs an application-layer entry
point. These are trivial wrappers and should be added beside the existing
queries — for example
`ListProjects(ctx, uow) ([]domain.Project, error)`. Their bodies are one
`uow.Do` containing one repository call. They exist for the boundary, not for
logic, and the spec states that plainly so a reviewer does not mistake them for
ceremony.

### 6.6 Validation-plan selection — the exactly-one contract

**AD-030 correction.** The zero/one/many Artifact-discovery contract in this
section remains. Candidate rule 2's “disproven” verdict is superseded: FF-004
always governed order/current state within a Validation Plan Artifact, and C9
now writes A + R + O + immediate accepted M. Exactly-one discovery chooses an
Artifact; `ResolveCurrentRevision` then chooses its current Revision. Neither
step ranks two distinct plan Artifacts.

`TimelineInput.PlanArtifactID` is a single string. Discovery returns a slice.
Something must bridge them, and **sort order must not be that bridge**:
deterministic ordering makes a choice reproducible, it does not make the chosen
plan semantically authoritative. This section replaces the earlier
"first plan in sorted order" rule, which was reproducible but ungrounded.

#### What the repository actually proves

Three candidate rules were tested against the code. **The repository proves
none of them**, and this document does not invent one:

| Candidate rule | Verdict | Evidence |
|---|---|---|
| 1. Exactly one validation plan may exist per capability | **Unproven** | No uniqueness invariant exists anywhere. `EstablishValidationPlanCommand` (`command_validation.go:42`) validates `ArtifactID`, `RevisionID`, `ScopeArtifactID`, and a non-empty activity list, then writes an artifact and a revision. It performs no lookup for an existing plan on the same scope, and no repository enforces one. |
| 2. Several plans may exist, but exactly one may be accepted/current | **Disproven** | Validation plans have **no acceptance journal entry and no revision-order metadata**. The only `RevisionOrder.Put` / `RevisionAcceptance.Append` call sites are `command_capability.go:174, 258, 323` and `command_requirement.go:88, 98` — capabilities and requirements only. `ResolveCurrentRevision` requires an order entry for *every* stored envelope (`query_ordering.go`, step 4, else `ErrRevisionOrderMissing`), so calling it on a plan artifact fails by construction. **"Current plan" is not a defined concept in this model.** |
| 3. Several simultaneously applicable plans are legal | **Unproven** | Structurally possible — nothing forbids two plan artifacts scoping one capability — but nothing blesses it either, and `TimelineInput.PlanArtifactID` being singular means the timeline read model cannot represent it. |

What the repository *does* prove is narrower and sufficient: **the timeline
contract represents at most one plan, zero is legal, and no mechanism exists to
rank two.**

#### The contract

Timeline and state composition require **exactly one applicable validation
plan** whenever a plan identifier is used. `DiscoverValidationPlanArtifactIDs`
keeps returning a deterministically sorted slice; a separate resolution step
consumes it:

| Discovered plans | Behaviour |
|---|---|
| **zero** | `PlanArtifactID` is left empty. **Not an error** — see below. |
| **exactly one** | that plan's artifact ID is used |
| **more than one** | **`ErrValidationPlanAmbiguous`** (§7.3) — the composition fails rather than choosing |

**Zero is deliberately not an error, and this departs from the correction's
suggested shape on verified evidence.** `GetFeatureTimeline` guards its plan
section with `if in.PlanArtifactID != ""` (`query_timeline.go:198`): an empty
plan identifier is an already-supported, already-exercised input meaning "this
feature has no plan events yet." In the canonical FF-011 scenario the plan is
established at step 9, so steps 1–8 describe a capability that legitimately has
none. Returning an error for zero would make the timeline and state endpoints
fail for every young feature and would contradict the application contract as
written. No existing sentinel — including `ErrNoAcceptedRevision`, which has
**zero call sites** and whose name concerns revisions rather than plans — is
adopted for a condition the application already treats as legal.

**Sorting is retained, and is explicitly not precedence.**
`DiscoverValidationPlanArtifactIDs` continues to sort for deterministic
discovery output and reproducible tests. It never resolves semantic ambiguity:
with two plans the sorted slice is stable *and* the composition still fails.

---

## 7. `ParseEvidenceKey`

### 7.1 The confirmed gap

Verified in `internal/engineering/refkeys.go`:

- `EvidenceKey(artifactID, revisionID string) string` exists (line 89),
  producing `"evidence:" + artifactID + "/" + revisionID`.
- **`ParseEvidenceKey` does not exist.** The file's only inverse parser is
  `ParseSubjectKey` (line 56).

Evidence discovery (§6.3) therefore cannot recover an artifact ID without ad
hoc string splitting at the call site — which would put the key's structure in
two places, exactly the duplication `ParseSubjectKey` was introduced to prevent
under AD-021 ("One parser, one definition of what a `SubjectKey` means").

### 7.2 Required addition

**This is an engineering key-utility contract addition, not a repository
change.** No repository interface, adapter, migration, or stored representation
is touched. `internal/engineering` remains PEOS-free.

```go
// ParseEvidenceKey is the inverse of EvidenceKey.
func ParseEvidenceKey(key string) (artifactID, revisionID string, err error)
```

Requirements, mirroring `ParseSubjectKey`'s verified conventions:

- Placed beside `EvidenceKey` in `internal/engineering/refkeys.go`.
- Uses `strings.CutPrefix` on `"evidence:"`, then `strings.Cut` on `"/"`.
- Rejects a missing prefix, a missing separator, an empty artifact ID, and an
  empty revision ID.
- Returns `ErrInvalidEnvelope`-wrapped errors with the same message shape
  `ParseSubjectKey` uses (`"%w: evidence key %q must name evidence:<artifact>/<revision>"`).
- **`EvidenceKey`'s format does not change.**
- **No PEOS decoding.**

Tests, in `internal/engineering`: round-trip
(`ParseEvidenceKey(EvidenceKey(a, r))` returns `a, r`); and malformed inputs —
empty string, missing prefix, wrong prefix, no separator, empty artifact,
empty revision — each asserting `errors.Is(err, ErrInvalidEnvelope)`.

### 7.3 One new application sentinel — `ErrValidationPlanAmbiguous`

§6.6 requires an error for "more than one applicable validation plan." **No
existing sentinel means that**, and the nearest candidate was checked and
rejected on evidence rather than assumed:

- **`ErrCorrectionAmbiguous` does not fit.** Its only production call site is
  `query_correction.go:145` — `"%d competing heads for this subject, scope, and
  criteria"` — and `correction_test.go:253` asserts it for exactly that
  condition. It means *a claim correction chain has no unique head*. A
  capability scoped by two validation plans is not a correction chain and has
  no heads. Reusing it would make one sentinel mean two unrelated things and
  would make the correction tests' guarantee unfalsifiable.
- **`ErrNoAcceptedRevision` does not fit** either — it has zero call sites, and
  it names a revision-acceptance condition that plans do not participate in
  (§6.6, rule 2).

**Required addition:** one narrowly scoped sentinel in
`internal/application/errors.go`, in the existing *Query errors (FF-010 §7,
§8)* block beside `ErrAmbiguousLifecycleState`:

```go
ErrValidationPlanAmbiguous = errors.New("application: validation plan ambiguous")
```

The name follows the established per-concern ambiguity family —
`ErrCurrentRevisionAmbiguous` (revisions), `ErrAmbiguousLifecycleState`
(lifecycle), `ErrCorrectionAmbiguous` (correction chains). This adds the
missing fourth member for validation-plan selection; it does not generalise
any of them.

**Transport classification: `409`**, machine code
`validation_plan_ambiguous`, details exposed. This is consistent with the §8.1
table's own rule — a store state from which no single answer can be derived —
and with all three sibling ambiguity sentinels, which are `409`. It appears as
row 24 in §8.1.

**This is an application-layer contract addition, not a repository change.** No
repository interface, adapter, migration, or stored representation is touched,
and the exhaustiveness test (§8.4) will require it to be mapped, which is how
it stays visible.

---

## 8. Complete error mapping

### 8.1 The sentinel set, verified

**At the time of writing, `internal/application` exposes 23 sentinel errors**,
and FF-017 adds a twenty-fourth (§7.3). The planning input said 21; the source
said 23, and the source governs. FF-015 §8's table covers 19 and is **not
exhaustive despite claiming to be** — four are absent:
`ErrRevisionSequenceInvalid`, `ErrNoAcceptedRevision`, `ErrCorrectionAmbiguous`,
`ErrUnknownDefinitionVersion`. FF-015 is not edited; the corrected table is
here.

**These counts are observations, not architectural constants.** The sentinel
set will grow. The durable contract is the exhaustiveness test (§8.4), which
fails the build when a sentinel exists without a transport mapping — that test,
not any number written in this document, is what keeps the table honest. An
implementer who finds a different count re-derives the table from source and
does not treat the number below as authoritative.

| # | Sentinel | Status | Machine code | Details exposed? |
|---|---|---|---|---|
| 1 | `ErrInvalidCommand` | 400 | `invalid_command` | yes — the message names the invalid field |
| 2 | `ErrNotFound` | 404 | `not_found` | yes — names the absent value |
| 3 | `ErrImmutableValueConflict` | 409 | `immutable_value_conflict` | yes — names the identity |
| 4 | `ErrCapabilityAlreadyLinked` | 409 | `capability_already_linked` | yes |
| 5 | `ErrRevisionSequenceConflict` | 409 | `revision_sequence_conflict` | yes |
| 6 | `ErrRevisionSequenceInvalid` | 409 | `revision_sequence_invalid` | yes |
| 7 | `ErrCurrentRevisionAmbiguous` | 409 | `current_revision_ambiguous` | yes |
| 8 | `ErrRevisionOrderMissing` | 409 | `revision_order_missing` | yes |
| 9 | `ErrRevisionReferenceMismatch` | 409 | `revision_reference_mismatch` | yes |
| 10 | `ErrNoAcceptedRevision` | 409 | `no_accepted_revision` | yes |
| 11 | `ErrEngineeringStateIndeterminate` | 409 | `engineering_state_indeterminate` | yes |
| 12 | `ErrAmbiguousLifecycleState` | 409 | `ambiguous_lifecycle_state` | yes |
| 13 | `ErrTimelineSourceInvalid` | 409 | `timeline_source_invalid` | yes |
| 14 | `ErrCorrectionAmbiguous` | 409 | `correction_ambiguous` | yes |
| 15 | `ErrReferencedValueMissing` | 422 | `referenced_value_missing` | yes |
| 16 | `ErrAcceptanceTransitionInvalid` | 422 | `acceptance_transition_invalid` | yes |
| 17 | `ErrCorrectionTargetMissing` | 422 | `correction_target_missing` | yes |
| 18 | `ErrCorrectionFamilyMismatch` | 422 | `correction_family_mismatch` | yes |
| 19 | `ErrCorrectionSelfReference` | 422 | `correction_self_reference` | yes |
| 20 | `ErrCorrectionCycle` | 422 | `correction_cycle` | yes |
| 21 | `ErrUnknownDefinitionVersion` | 422 | `unknown_definition_version` | yes |
| 22 | `ErrNestedTransaction` | 500 | `internal_error` | **no** — a transport bug by construction; log with the real error, return the opaque body |
| 23 | `ErrTransactionAborted` | 503 | `transaction_aborted` | yes, plus a `Retry-After: 1` header — writes are idempotent (§14), so retry is safe |
| 24 | `ErrValidationPlanAmbiguous` **(new, §7.3)** | 409 | `validation_plan_ambiguous` | yes — names the competing plan artifact IDs |

**Classification reasoning**, following FF-015 §8's own buckets: `400` is a
malformed intent; `404` a named value absent; `409` a conflict with existing
immutable state *or* a store state from which no single answer can be derived;
`422` a well-formed request whose references or transitions are illegal; `500`
an internal defect; `503` retry exhaustion.

The four additions are classified by that same rule.
`ErrRevisionSequenceInvalid` and `ErrNoAcceptedRevision` are ordering-invariant
conditions → `409`, matching their five siblings. `ErrCorrectionAmbiguous` is a
correction-graph state with no unique head, i.e. no derivable answer → `409`,
alongside `ErrCurrentRevisionAmbiguous` rather than alongside the `422`
correction-*legality* errors. `ErrValidationPlanAmbiguous` (row 24) is likewise
a store state from which no single answer can be derived → `409`, matching all
three sibling ambiguity sentinels. `ErrUnknownDefinitionVersion` is a
well-formed request naming a lifecycle definition that does not exist → `422`,
matching
`ErrReferencedValueMissing`.

**Forward correction (AD-032, FF-023).** The historical
`ErrUnknownDefinitionVersion -> 422` row is superseded because no C6 transport
field names a Definition Version. Stored wrong-version or invalid lifecycle
history is opaque `500 internal_error`. New request-side transition failures use
`ErrLifecycleTransitionInvalid -> 422 lifecycle_transition_invalid`; a stale
but otherwise valid predecessor uses
`ErrLifecycleHeadConflict -> 409 lifecycle_head_conflict`. The implemented
error-exhaustiveness test, not this historical row count, remains authoritative.

Transport-originated errors that are not application sentinels: malformed JSON,
unknown field, multiple JSON values, oversized body, and unparseable path
values all map to `400` with code `bad_request` (§12).

### 8.2 The mapping function

One function in `errors.go`, the single translation point:

```go
func statusFor(err error) (status int, code string, exposeMessage bool)
```

It uses `errors.Is` against the table in order, and returns
`(500, "internal_error", false)` for anything unmatched. Handlers never
construct a status themselves. This directly inherits M.4's most expensive
lesson — *translation belongs where the decision is made, not where the error
is raised* — which the M.4 review recorded after eager `mapError` calls in
repositories destroyed retryability information.

### 8.3 Error body and fallback

```json
{ "error": { "code": "immutable_value_conflict", "message": "…" } }
```

For unmatched errors: `500`, code `internal_error`, message a fixed generic
string, and **the original error logged server-side at error level with the
request method and path**. No internal error text reaches the response body.

### 8.4 Exhaustiveness test

A test in the transport package asserts that every exported `Err*` value in
`internal/application` has a mapping. Mechanism, chosen for maintainability:
the transport declares an explicit ordered registry
`var errorMappings = []errorMapping{{application.ErrNotFound, 404, "not_found", true}, …}`;
`statusFor` walks it; and the test parses `internal/application/errors.go` with
`go/ast` (the technique `internal/architecture` already uses), collects every
`Err*` identifier, and asserts each appears in the registry. Adding a sentinel
without classifying it then fails the build — which is the point.

---

## 9. Request decoding and validation

### 9.1 Three layers, none duplicated

| Layer | Owner | Returns |
|---|---|---|
| 1. Syntax | transport | `400` |
| 2. Command/query validation | `internal/application` | `400` via `ErrInvalidCommand` |
| 3. Invariants | domain, engineering, repositories | `422` / `409` |

**Transport owns:** malformed JSON; an empty body where one is required;
unknown JSON fields; more than one JSON value in the body; malformed path or
query syntax; a request body exceeding the size limit; and, where enforced, an
unsupported content type.

**Transport must not duplicate:** artifact or revision invariants; revision
sequencing; acceptance-transition legality; readiness semantics; create-only
conflict logic; correction-graph legality; PEOS validation. Duplicating a check
at the edge creates two definitions of validity that drift — the failure mode
AD-021 closed in the persistence layer.

### 9.2 Decoding requirements

- `json.NewDecoder(r.Body)` with `DisallowUnknownFields()`. A silently ignored
  field is a request the client believes it made and the server did not.
- Exactly one JSON value: after decoding, a second `Decode` must return
  `io.EOF`; anything else is `400`.
- `http.MaxBytesReader` with a limit of **1 MiB**, declared as
  `const maxRequestBody = 1 << 20`. FF-015 specifies no limit, so this is an
  **implementation-level safety limit**, labelled as such: it prevents an
  unbounded read, and the canonical scenario's largest body is orders of
  magnitude smaller. Exceeding it is `400`.
- The body is closed by the server; handlers do not close it, and do not need
  to drain it.
- **The application layer is not invoked after a decode failure.** The handler
  returns immediately.
- Responses are deterministic: `encoding/json` with struct field order, no map
  iteration in any response type.

### 9.3 Content type

Requests with a body must send `Content-Type: application/json` (a bare type or
with a charset parameter). A mismatch is `415`. Responses always set
`Content-Type: application/json; charset=utf-8`. `415` is transport-level and
appears in no application sentinel table.

---

## 10. Response contracts

### 10.1 Envelope

FF-015 §7's envelope, unchanged:

```json
{ "data": { }, "rationale": { } }
```

`rationale` is **omitted when absent, never rendered empty** —
`json:"rationale,omitempty"` with a pointer. A derived answer without rationale
is a bug, and §18 criterion 14's test asserts it.

### 10.2 Conventions

- **Identifiers are strings**, using the verified `String()` forms:
  `RevisionKey` → `"CAP-1/CAP-1-REV-1"`, `RecordKey` → `"claim:CLM-1"`,
  `ArtifactKey` → `"CAP-1"`. Where a client needs the parts, the DTO carries
  both `artifact_id` and `revision_id` as separate fields rather than asking
  the client to split a composite.
- **Timestamps are RFC 3339 UTC** — `time.Time` marshals this way by default;
  the adapters already normalise to UTC.
- **Enums are their existing string values**, verbatim: `ReadinessStatus`
  (`ready`, `not-ready`, `indeterminate`, `incomplete`), `EventKind`
  (`project.created` … `lifecycle.transitioned`), `AcceptanceState`,
  `RevisionFamily`, `RecordKind`. No re-mapping — the wire value is the domain
  value.
- **Empty collections render as `[]`, never `null`.** DTO builders initialise
  every slice with `make([]T, 0, n)`.
- **Absent optional objects are omitted** (`omitempty` + pointer); absent
  optional scalars are omitted rather than sent as `""` or `0`, except where a
  `Found bool` already carries presence explicitly, in which case the boolean
  is sent and the payload omitted.
- **Stable ordering.** Every list is already ordered by the application layer
  (`ListByArtifact` ascending by key; timeline by `(OccurredAt, kindRank,
  SourceIdentity)`; discovery functions sorted). The transport preserves order
  and never re-sorts.
- **No derived field is invented.** `TestNoDerivedStateOnFeatureCard` polices
  the domain; a `FeatureCard` response carries no `status` field.
- **DTOs are not aliases.** No transport type embeds or aliases an
  `application`, `domain`, or `engineering` type. Mapping is explicit, per
  FF-015 §7 and the AD-013 argument it cites.

### 10.3 Field-by-field query mappings

Derived from the verified Go result types.

**Q1 `GET /projects`** — `data.projects[]`: `project_id`, `name`, `created_at`.
No rationale.

**Q2 `GET /projects/{projectID}/features`** — `data.features[]`:
`feature_card_id`, `project_id`, `title`, `description`, `created_at`,
`capability_artifact_id` (omitted when `CapabilityArtifactID()` reports
`false`). No rationale.

**Q4 `GET /features/{featureCardID}/state`** — from
`EngineeringStateResult{CurrentRevision, EffectiveRequirements,
ApplicableDecisions, Readiness, Lifecycle}`:

- `data.current_revision`: `found`; when found — `artifact_id`, `revision_id`,
  `sequence`, `revision_family`, `artifact_type`, `integrity_value`,
  `content_digest` (omitted when zero), `subject_key` (omitted when empty),
  `recorded_at`, `provenance_actor`/`provenance_recorded_at` (each omitted
  unless its `Has*` flag is true).
- `data.effective_requirements[]`: `artifact_id`, `revision_id`, `sequence`.
- `data.applicable_decisions[]`: `decision_id`, `subject_key`, `scope`,
  `occurred_at` (omitted unless `HasOccurredAt`), `outcome`.
- `data.readiness`: `status`; `per_requirement[]` with
  `requirement_artifact_id`, `requirement_revision_id`, `has_claim`,
  `claim_id` (omitted unless `has_claim`), `outcome`, `execution_outcome`,
  `stale`, `stale_sequence` (omitted unless `stale`), `verdict_reason`,
  `rejected[]` (`record_key`, `reason`).
- `data.lifecycle`: `found`; when found — `state_id`, `assignment_id`,
  `subject_key`, `occurred_at`.
- `rationale.current_revision`: from `ResolutionRationale` — `rule`,
  `selected_artifact_id`/`selected_revision_id` (omitted together when no
  revision was selected), `selected_sequence`, `considered[]`
  (`artifact_id`, `revision_id`, `sequence`, `acceptance_state`),
  `rejected[]` (`artifact_id`, `revision_id`, `reason`), `warnings[]`. The
  split `artifact_id`/`revision_id` form is §10.2's own convention (*"the
  DTO carries both `artifact_id` and `revision_id` as separate fields
  rather than asking the client to split a composite"*) applied to this
  rationale, not a single composite `key` field.
- `rationale.lifecycle`: from `LifecycleRationale` — `rule`, `total`,
  `duplicate`.
- **No `rationale.readiness`.** `application.ReadinessResult` carries no
  `Rule`-style rationale field the way `ResolutionRationale` and
  `LifecycleRationale` do — every rationale elsewhere in this codebase is
  application-owned, never transport-synthesized, and inventing one here
  would itself violate §10.2's "no derived field is invented" rule. The
  per-requirement `verdict_reason` values already inside `data.readiness`
  carry the explanation FF-011 requires. (Corrected during implementation,
  §16 step 7's "As implemented" note; this replaces this section's original
  wording, which called for a synthesized precedence-rule string here.)

**Forward Q4 correction (AD-032/AD-033/FF-023).** Each
`effective_requirements[]` element also carries
`source_capability_artifact_id`, `source_capability_revision_id`, and
`source_acceptance_criterion_key` from its validated trace. A found lifecycle
result additionally carries `definition_id`, `definition_version_id`,
`established_by_artifact_id`, and `established_by_revision_id`.
`rationale.lifecycle` now means `rule = unique head of validated lifecycle
predecessor chain` and `total = chain length`; the old `duplicate` tie-break
projection is omitted because a branch or duplicate edge is integrity failure,
not a successful resolution.

**Q3 `GET /features/{featureCardID}`** — `data.feature` (as Q2's element) plus
the whole of Q4's `data` under `data.state`; `rationale` identical to Q4's.

**Q5 `GET /features/{featureCardID}/timeline`** — from
`TimelineResult{Dated, Undated}`. `data.dated[]` and `data.undated[]`, each
event: `event_id`, `feature_card_id`, `kind`, `occurred_at` (omitted in
`undated`), `actor`, `label`, `summary`, `source_identity`, `references[]`,
`corrected` (omitted when empty). `rationale` is per event — each event's own
`Rationale` string is rendered as `rationale` **inside the event object**,
because `TimelineEvent.Rationale` is per-event, not per-response. Top-level
`rationale` is omitted for this endpoint.

**This is a deliberate, endpoint-specific response shape, not an accidental
omission of a top-level `rationale` field.** The application result models
rationale per event — `TimelineEvent.Rationale` is a field on each event, and
`TimelineResult` has no response-level rationale to render. Hoisting one would
mean inventing a rationale the application never computed, which §10.2's
"no derived field is invented" rule forbids. A reviewer finding no top-level
`rationale` on Q5 is looking at the intended contract; §18's rationale-presence
test asserts the per-event form for this endpoint and the envelope form for the
others.

**Q6 `GET /capabilities/{artifactID}/revisions`** — `data.revisions[]` (each as
Q4's `current_revision` payload; `sequence` is populated only for the entry
matching `data.current`'s revision, since `GetCapabilityRevisions` returns no
per-revision order metadata for the rest — every revision's sequence remains
recoverable from `rationale.considered[]`, which lists every considered
revision with its own `sequence`), `data.current` (the resolved current
revision key, omitted when none is accepted). `rationale` from
`ResolutionRationale`, as in Q4. (Corrected during the post-implementation
audit, finding MINOR-6; the original wording overstated per-revision sequence
coverage in `data`.)

**Q7 `GET /capabilities/{artifactID}/revisions/{revisionID}`** — `data`: one
revision object as above. No rationale — a single fetched revision is not
derived. `404` when absent.

---

## 11. Create-only idempotency

**AD-030 correction.** The outcomes in this section remain, but the mechanism
does not. Every command now recognizes a complete persisted semantic act
inside the application UOW before reconstructing time-bearing values. C7
includes `acceptance_record_id` plus its exact source capability
Revision/criterion fields; C9 includes `acceptance_record_id`; C10 includes its
evidence pair. Repository
`Equal` remains a final write guard, not the replay witness. See FF-022 §§4–8.

### 11.1 Classification

**All twelve command endpoints are create-only**, and every one carries a
client-supplied identity. There is no non-create transition among them:
`AcceptCapabilityRevision` appends a journal entry (AD-015);
`CorrectValidationClaim` creates a new claim (AD-017);
`AssignLifecycleState` records a new assignment. Nothing edits.

| Endpoint | Identity source | Identical replay | Conflicting replay |
|---|---|---|---|
| C1 | `project_id` | `201`, no-op | `409 immutable_value_conflict` |
| C2 | `feature_card_id` | `201`, no-op | `409` |
| C3, C4 | `artifact_id` + `revision_id` | `201`, no-op | `409` |
| C5 | `record_id` | `201`, no-op | `409` |
| C6 | `assignment_id` (+ transition-record revision key) | `201`, no-op | `409` |
| C7 | `artifact_id` + `revision_id` | `201`, no-op | `409` |
| C8 | `decision_id` | `201`, no-op | `409` |
| C9 | `artifact_id` + `revision_id` | `201`, no-op | `409` |
| C10 | `execution_id` | `201`, no-op | `409` |
| C11, C12 | `claim_id` | `201`, no-op | `409` |

**Response body on identical replay is identical to the original response.** The
command returns the same `*Result` either way, so the handler needs no
replay-detection branch — which is the property that keeps §4's "no conditional
that decides anything" rule intact.

**No `Idempotency-Key` header.** The contract suite already proves, on both
adapters, that an identical re-`Put` is a no-op (`IdempotentIdenticalPut`) and
a differing one conflicts (`ConflictingPut`). A header would add a second,
weaker mechanism on top of a stronger one that already exists.

### 11.2 AD-026 and revision creation

For C3, C4, C7, C9, and C6's transition-record revision, AD-026's equality rule
now governs conflict detection:

- same `RevisionKey`, same payload, same `SubjectKey` → idempotent;
- same `RevisionKey`, same payload, **different `SubjectKey`** → conflict;
- same `RevisionKey`, different payload → conflict.

**The transport does not recreate these rules.** It calls the command, and the
command's repository `Put` reaches `RevisionEnvelope.Equal`. Every outcome
arrives as `ErrImmutableValueConflict` and is mapped centrally by `statusFor`.
§18 makes preservation of this behaviour through HTTP an acceptance criterion.

### 11.3 Query endpoints

Safe and cacheless. All reads are computed (AD-006); the transport adds no
caching, because a cached derived answer is precisely the staleness AD-006
exists to prevent. No `ETag`, no `Cache-Control` beyond `no-store`.

---

## 12. Routing and `net/http` ownership

### 12.1 The standard library is sufficient — verified

`go.mod` declares `go 1.25.12`. Method-aware `ServeMux` patterns
(`mux.HandleFunc("POST /api/v1/projects", …)`) and `r.PathValue("artifactID")`
have been available since Go 1.22. **No router dependency is needed or
permitted** — `TestGoModHasOnlyApprovedRequirements` fails the build on any new
direct requirement, and FF-015 §18 R5 names a router as the classic first
exception. There is no evidence the standard library is insufficient.

### 12.2 Routing requirements

- **Registration:** one `mux.HandleFunc("<METHOD> <path>", handler)` per
  endpoint, all in `router.go`, in the §3 matrix's order.
- **Path parameters:** `r.PathValue(name)`. An empty value is `400`.
- **Unknown route:** `404` with the standard error body — not `ServeMux`'s
  plain-text default. The router registers a catch-all that emits JSON.
- **Unsupported method on a known path:** Go's `ServeMux` returns `405` with an
  `Allow` header automatically when a pattern exists for the path under other
  methods. The catch-all must not shadow this; a test asserts `405` for
  `DELETE /api/v1/projects`.
- **Trailing slash:** patterns are registered without one; `ServeMux`'s default
  redirect behaviour for `{$}`-free patterns is acceptable and untested beyond
  one assertion that `/api/v1/projects/` does not 500.
- **No `PUT`, `PATCH`, or `DELETE` pattern is ever registered.** §18
  criterion 2's test (`TestNoRouteRegistersUnsupportedMethods`) asserts this.

### 12.3 Narrowed architecture guards

`TestNoHTTPDatabaseUIOrAIPackage` is **decomposed, not deleted**. Replacement
tests, named to match the repository's existing
`TestOnly<Holder>Imports<Thing>` convention:

| New test | Asserts |
|---|---|
| `TestOnlyTransportAndCommandImportNetHTTP` | exactly `internal/transport/http` and `cmd/featureforge` import `net/http` |
| `TestOnlyUIImportsHTMLTemplate` | no package imports `html/template` in Phase A; reserved for the Phase B UI holder |
| `TestDatabaseSQLIsNeverImported` | no package imports `database/sql` or `database/sql/driver`, anywhere, including `cmd/` |
| `TestNoAIPackage` | no package path segment is `ai` |

The stale M.3 wording is corrected in the new messages. The inert
`postgres`/`sql` prefix check is dropped — the import-based tests state the
real boundary, and `TestOnlyPostgresInfrastructureImportsDriver` already
enforces the driver.

**§24 records a defect in this table's coverage, found and fixed in M.5
publication remediation.** `TestOnlyUIImportsHTMLTemplate` above asserts
only the `html/template` half of the Phase A prohibition this document's
§2.2 states (both `html/template` and `text/template` "remain forbidden
everywhere in Phase A"); no test asserted the `text/template` half.
`TestTextTemplateIsNeverImported` restores it, mirroring
`TestDatabaseSQLIsNeverImported`'s absolute, no-holder shape.

**No existing internal check is weakened.** `TestNoForbiddenPackageNames`
(verified list: `workflow`, `engine`, `framework`, `shared`, `common`, `util`,
`pkg`, `integration`, `core`) does not forbid `transport` or `http`, so the new
package names are already permitted.

---

## 13. `cmd/` architecture coverage

`internal/architecture/imports.go` was inspected: `InternalPackages()` (line 38)
walks `filepath.Join(ModuleRoot(), "internal")` only. `cmd/` is invisible to
every architecture test.

**Required minimal helper**, added to `imports.go` beside `InternalPackages()`
and following its conventions exactly (`build.ImportDir`, skip `*build.NoGoError`,
`ModulePath + "/" + relative path`):

```go
// CmdPackages returns every Go package under cmd/, each with its direct,
// non-test imports. It returns an empty slice, not an error, when cmd/ does
// not exist.
func CmdPackages() ([]PackageInfo, error)
```

Absence of `cmd/` must be tolerated — the helper lands in implementation step 1,
before `cmd/featureforge` exists in step 8, so it must return empty rather than
fail in between.

Used to extend coverage so that:

- `cmd/` does not import PEOS — **except** `cmd/featureforge`, which must
  construct the `Recorder` and is therefore an authorized holder alongside
  `internal/engineering/peos`. `TestOnlyIntegrationPackageImportsPEOS` is
  extended to accept exactly these two, and its failure message names both.
- `cmd/` does not import a driver directly — `cmd/featureforge` obtains its
  pool through `postgres.Connect`, so `TestOnlyPostgresInfrastructureImportsDriver`
  gains `cmd/featureforge` as a permitted holder of `*pgxpool.Pool` only if the
  implementer finds a direct import unavoidable. **Preferred:** `postgres`
  exposes what `cmd` needs so no driver import appears in `cmd` at all; the
  test then stays single-holder. The implementer verifies which is true when
  writing step 8 and records the outcome.
- `cmd/` does not bypass the application/transport boundary — no `cmd` package
  imports `internal/infrastructure/*` for anything but adapter construction,
  and none imports `internal/domain` or `internal/engineering` for logic.

---

## 14. Server composition

### 14.1 Configuration

Environment variables only, read with `os.Getenv`, no configuration library:

| Variable | Meaning | Default |
|---|---|---|
| `FEATUREFORGE_ADDR` | listen address | `127.0.0.1:8080` |
| `FEATUREFORGE_ADAPTER` | `memory` or `postgres` | `memory` |
| `FEATUREFORGE_POSTGRES_DSN` | DSN, required when adapter is `postgres` | — |

**`memory` is the default composition**, because it needs no external service
and makes the binary runnable immediately. **PostgreSQL is used in tests when
`FEATUREFORGE_POSTGRES_TEST_DSN` is set**, matching the established gating
convention that keeps `go test ./...` meaningful without Docker.

### 14.2 Responsibilities

1. Read configuration.
2. Select the adapter: `memory.NewUnitOfWork(memory.NewStore())`, or
   `postgres.Connect(ctx, dsn)` → `postgres.Migrate(ctx, pool)` →
   `postgres.NewUnitOfWork(pool)`. **Migration on start applies to the
   PostgreSQL adapter only**; `Migrate` is idempotent and safe to call on every
   start, which is exactly what it was built for.
3. Construct `peos.NewRecorder()` and `application.SystemClock{}`.
4. Build the handler: `transporthttp.NewHandler(deps)`.
5. Build `&http.Server{Addr, Handler, ReadHeaderTimeout: 5s, ReadTimeout: 30s,
   WriteTimeout: 30s, IdleTimeout: 120s}`.
6. Serve; on `SIGINT`/`SIGTERM`, `server.Shutdown(ctx)` with a 10-second
   timeout, then close the pool.
7. Log at startup: adapter selected, migrations applied (PostgreSQL only),
   address bound. Log a fatal error and exit non-zero on failure.

**Not added:** dependency-injection frameworks, configuration libraries,
deployment manifests, daemon supervisors, production observability platforms.

### 14.3 Observability

`log/slog` only, closing the gap the M.4 review recorded as E6:

- one line per request: method, path, status, duration;
- `ErrTransactionAborted` reaching the transport logged at **warn** with the
  endpoint — retry exhaustion becomes visible at the only layer that can act
  on it;
- panic recovery middleware logs and returns `500`. It does **not** interfere
  with `UnitOfWork.Do`'s panic path: `Do` rolls back and re-panics *first*, and
  the middleware catches what escapes, so rollback still happens. The contract
  suite's `RollbackOnPanic` proves the `Do` half; §18 criterion 10's
  `TestRecoverMiddlewarePreservesHTTPContract` proves the transport half.

---

## 15. Security and robustness baseline

**In scope for Phase A:** bounded request body (1 MiB); strict JSON decoding;
deterministic response content type; server read/write/idle timeouts; graceful
shutdown; no internal error leakage; panic recovery; loopback default binding.

**Explicitly deferred:** authentication, authorization, CORS, CSRF, rate
limiting, distributed tracing, production metrics, TLS termination, proxy trust,
multi-tenant concerns.

**The transport asserts no identity and invents none.** FF-007's M.5 exit
criteria require that no authentication feature exists, and FF-001 §3 specifies
one local user. The actor is a fixed configuration value —
`LocalActorRef` in `internal/engineering/peos/vocabulary.go` — already used by
every provenance record. Transport accepts no actor parameter, reads none from
a header, and threads none through. An actor parameter would be the first step
toward the multi-user semantics AD-001 rejected.

Loopback binding is a deployment default, not a security control; this document
claims no security property. None of the deferred items blocks Phase A.

---

## 16. Implementation sequence

Eleven steps. Each names its files, its behaviour, its tests, its dependency,
and its done condition. **No step requires an implementer to choose an
unresolved architectural direction.**

**Step 1 — Narrow the architecture guards; add `CmdPackages()`.**
*Files:* `internal/architecture/imports.go`, `internal/architecture/architecture_test.go`.
*Behaviour:* §12.3's four tests replace `TestNoHTTPDatabaseUIOrAIPackage`;
`CmdPackages()` added; PEOS and driver tests extended to cover `cmd/`.
*Tests:* each new test verified against a deliberate violation — introduce the
forbidden import, observe the failure names the right boundary, revert, observe
green. Record each in the implementation report.
*Depends on:* nothing. *Done when:* the architecture suite passes, `cmd/`
absence is tolerated, and every deliberate violation was demonstrated.

*As implemented (commit `8e43f45`).* `CmdPackages()` added exactly as
specified, tolerating `cmd/`'s pre-Step-8 absence. `TestNoHTTPDatabaseUIOrAIPackage`
replaced by `TestOnlyTransportAndCommandImportNetHTTP`, `TestOnlyUIImportsHTMLTemplate`,
`TestDatabaseSQLIsNeverImported`, `TestNoAIPackage`. `TestNoTimeNowOutsideClock`'s
single-file allowlist was widened to a map including
`internal/transport/http/middleware.go` (needed by Step 4's request-duration
logging), and `TestTransportDoesNotImportPEOS` was added alongside the
existing PEOS/driver guards. Every new test verified against a deliberate
violation (scratch packages and files, including a real `cmd/featureforge`
importing `database/sql`), then reverted.

**Step 2 — `ParseEvidenceKey`.**
*Files:* `internal/engineering/refkeys.go`, `refkeys_test.go` (or the existing
engineering test file).
*Behaviour:* §7.2. *Tests:* round-trip + six malformed cases.
*Depends on:* nothing. *Done when:* `go test ./internal/engineering/...` passes.

*As implemented (commit `267de0e`).* Added mirroring `ParseSubjectKey`
exactly. `TestParseEvidenceKeyRoundTrips` and
`TestParseEvidenceKeyRejectsMalformedInput` (six cases) in
`internal/engineering/refkeys_test.go`.

**Step 3 — Application discovery composition, plan resolution, and the new
sentinel.**
*Files:* `internal/application/errors.go` (add `ErrValidationPlanAmbiguous`,
§7.3), `query_state.go`, `query_timeline.go`, and their tests.
*Behaviour:* §6.2's three discovery functions, §6.5's thin reads, §6.6's
exactly-one plan resolution, and the per-endpoint composition functions §6.4
recommends.
*Tests:* decision, execution, claim, and evidence discovery each proven;
determinism and dedup asserted; **a test asserting the requirement population
comes from `DiscoverRequirementArtifactIDs` and never from claims**; plus the
four plan-selection tests §6.6 requires —
- **zero plans** → `PlanArtifactID` empty, no error, and the resulting timeline
  simply carries no plan events (the canonical scenario before its step 9 is a
  ready-made fixture);
- **exactly one plan** → that artifact ID is used;
- **more than one plan** → `errors.Is(err, ErrValidationPlanAmbiguous)`, and no
  plan is selected;
- **ordering does not resolve ambiguity** → seed two plans whose sorted order is
  known and assert the failure is identical regardless of insertion order, so a
  future reader cannot mistake stable ordering for precedence.

*Depends on:* step 2. *Done when:* `go test ./internal/application/...` passes.

*As implemented (commit `267de0e`).* `ErrValidationPlanAmbiguous` added
beside `ErrAmbiguousLifecycleState`. `DiscoverDecisionIDs`,
`DiscoverExecutionAndClaimIDs`, `DiscoverEvidenceArtifactIDs` added to
`query_state.go`/`query_timeline.go`; `ResolveApplicableValidationPlanID`
added implementing the exactly-one contract; `query_reads.go` added for the
thin reads and Q3/Q4/Q5 composition (`GetFeatureOverview`,
`GetFeatureEngineeringStateForCard`, `GetFeatureTimelineForCard`,
`GetCapabilityRevisions`, `GetCapabilityRevision`, `ListProjects`,
`ListFeaturesByProject`). All four plan-selection cases proven in
`internal/application/discovery_test.go`, including the
insertion-order-independence case.

**Forward correction (AD-032/FF-023).** That paragraph records the initial M.5
commit. The current discovery surface additionally includes
`DiscoverExecutionAndClaimIDsAllRevisions` and
`DiscoverDecisionEvidenceArtifactIDs`; all integrity-sensitive functions take
the inspector and validate global envelope populations before filtering.

**Step 4 — Transport skeleton and DTO contracts.**
*Files:* `internal/transport/http/{router,middleware,server,dto_command,dto_query}.go`.
*Behaviour:* package, `NewHandler`, router with one trivial endpoint (Q1) end
to end, logging and recovery middleware, DTO types.
*Tests:* Q1 handler test; 404/405 behaviour; content type.
*Depends on:* steps 1, 3. *Done when:* Q1 returns a correct envelope through
`httptest`.

*As implemented (commit `27ba08e`).* `Dependencies{UOW, Recorder, Clock,
Logger}` injected, never constructed in the package. Discovered during this
step: a bare `"/"` catch-all pattern pre-empts `ServeMux`'s own 405 logic and
makes 404 and 405 indistinguishable via `mux.Handler` alone (both report an
empty pattern). Resolved with `withJSONNotFoundAndMethodNotAllowed`, which
runs the real mux through a `notFoundInterceptor` and rewrites based on the
status `ServeMux` actually wrote — the only place the two genuinely differ.
Verified with a throwaway diagnostic program, then removed. This is a
transport-internal technique, not a deviation from §3's route table.

**Step 5 — Centralized error mapping.**
*Files:* `internal/transport/http/errors.go` + test.
*Behaviour:* §11's registry, `statusFor`, error body, fallback.
*Tests:* the exhaustiveness test (§8.4); fallback opacity.
*Depends on:* step 4. *Done when:* every sentinel then present in
`internal/application` — including `ErrValidationPlanAmbiguous` from step 3 —
maps, and the exhaustiveness test fails if one is removed from the registry.

*As implemented (commit `27ba08e`).* `errorMappings` (24 rows), `statusFor`,
`writeAppError` (also logs 500s at error level and 503s at warn level with
`Retry-After: 1`). `TestErrorMappingIsExhaustive` parses
`internal/application/errors.go` via `go/ast` and extracts sentinel
name→message pairs directly from the source, rather than a hand-maintained
lookup table that could itself drift. Verified against a deliberate
violation (`ErrScratchUnmapped` added to `errors.go`, confirmed the test
failed, reverted).

**Step 6 — Command handlers.**
*Files:* `handlers_command.go`, `dto_command.go` + tests.
*Behaviour:* C1–C12 in canonical-scenario order (project → feature →
capability → requirement → decision → plan → run → claim → correction →
lifecycle), so the scenario becomes drivable incrementally.
*Tests:* per endpoint — success, decode failure, unknown field, validation
error, conflict replay.
*Depends on:* step 5. *Done when:* every command is reachable and no handler
contains a domain conditional.

*As implemented (commit `ea919cc`).* All twelve handlers follow the same
six-step shape. `TestCommandEndpointsCanonicalOrder` drives all twelve in
order; `TestCommandIdempotentReplay` and `TestCommandConflictingReplay`
prove §14's replay contract, using a `FixedClock` (not `SystemClock`) so two
otherwise-identical requests capture the same `clock.Now()`, matching the
convention every other command fixture in the codebase already uses.
`TestReviseCapabilityRequiresPathValue`'s empty-`{artifactID}` case calls
`handleReviseCapability` directly rather than through the router: `//` in a
URL triggers `ServeMux`'s own redirect before any handler runs, and `ServeMux`
overwrites a pre-set `PathValue` with its own match.

**Step 7 — Query handlers and discovery-backed composition.**
*Files:* `handlers_query.go`, `dto_query.go` + tests.
*Behaviour:* Q1–Q7 per §10.3, using step 3's composition.
*Tests:* per endpoint field mapping; rationale presence; empty-array rendering;
Q7 `404`.
*Depends on:* steps 3, 6. *Done when:* all seven return correct envelopes.

*As implemented (commit `f356824`).* All seven handlers added.
**Correction to §10.3:** `engineeringStateRationaleDTO` carries no
`readiness` entry. `application.ReadinessResult` has no `Rule`-style
rationale field the way `ResolutionRationale` and `LifecycleRationale` do —
every rationale elsewhere in this codebase is application-owned, never
transport-synthesized, and inventing one here would itself violate §10.2's
"no derived field is invented" rule. The per-requirement `verdict_reason`
values already carry the readiness explanation FF-011 requires. This is a
narrow, evidence-grounded correction to this document's own prior wording,
not a contradiction between FF-018 and repository behaviour, so it did not
warrant halting implementation — it is recorded here and in a doc comment on
the type.

**Step 8 — `cmd/featureforge`.**
*Files:* `cmd/featureforge/main.go`.
*Behaviour:* §17. *Tests:* architecture tests from step 1 now cover it; a
smoke test only if a composition helper warrants one.
*Depends on:* step 7. *Done when:* the binary starts on both adapters and
shuts down gracefully.

*As implemented (commit `2f6026f`).* **N1 resolved: no direct `pgx` import.**
`postgres.Connect`'s `*pgxpool.Pool` is carried only through `:=` type
inference into `postgres.Migrate`/`NewUnitOfWork` and a `Close` method
value; `cmd/featureforge/main.go` never names a pgx type.
`TestOnlyPostgresInfrastructureImportsDriver` passes unchanged, confirming
it. No `main_test.go` was warranted — verified instead with a built smoke
binary: the memory adapter starts, serves a real request, and exits 0 on
`SIGINT`; a `postgres` adapter run without `FEATUREFORGE_POSTGRES_DSN` and
an unknown adapter value both fail fast with exit 1 and a clear message.

**Step 9 — Memory scenario-through-HTTP.**
*Files:* `internal/transport/http/scenario_http_test.go`.
*Behaviour:* the canonical FF-011 scenario driven entirely through HTTP
requests against an in-memory-backed handler, asserting the same end state
`assertCanonicalEndState` asserts.
*Depends on:* step 8. *Done when:* it passes. **This is the single
highest-value test in Phase A.**

*As implemented.* `assertCanonicalEndState` itself is unexported test code
in `internal/scenario`, and FF-018 does not authorize modifying that
package, so its checks are reproduced in
`assertCanonicalEndStateThroughHTTP` rather than imported — sourced from the
Q1–Q7 HTTP responses wherever an endpoint exposes the answer, and from a
direct query on the shared `uow` only for the facts no Phase A query
endpoint surfaces (content digests, raw claim/correction fields, lifecycle
transition history). One act — recording the decision's supporting
evidence — has no HTTP command by design (FF-010 §3 fixes the act surface
at ten acts / twelve commands; every other piece of evidence arrives
bundled with an execution via `RecordValidationRunCommand`), so this one act
is performed directly against the shared `uow`/recorder, exactly as
`internal/scenario.Run` itself does internally. `TestCanonicalScenarioThroughHTTP`
passed on its first run.

**Step 10 — PostgreSQL scenario-through-HTTP.**
*Files:* the same test, parameterised by adapter.
*Behaviour:* the identical scenario against PostgreSQL, gated on
`FEATUREFORGE_POSTGRES_TEST_DSN`, skipping cleanly when unset.
*Depends on:* step 9. *Done when:* `make postgres-test` passes with it
included.

*As implemented.* The scenario-driving body was factored into
`runScenarioThroughHTTP(t, ctx, uow, rec, clock) http.Handler`, shared by
`TestCanonicalScenarioThroughHTTP` (memory) and the new
`TestCanonicalScenarioThroughHTTPPostgres` in
`internal/transport/http/scenario_http_postgres_test.go`, which mirrors
`internal/scenario/scenario_postgres_test.go`'s per-test schema isolation
(`newPostgresFixtureHTTP`). `Makefile`'s `postgres-test` target was extended
to include `./internal/transport/http/...`. Run against a real PostgreSQL
container via `make postgres-test`: exit 0, zero failures, both the memory
and PostgreSQL variants passing in the same run.

**Step 11 — Documentation evidence and full verification.**
*Behaviour:* update FF-015 and FF-018 status/evidence per §20; run the full
gate (§18 criterion 19). *Done when:* everything passes and the tree is clean.

*As implemented.* FF-015's status line, §17 table, and closing paragraph
updated to record AD-022/023/024 as accepted and implemented, each pointing
to this document and to its own decision-log entry; FF-015's own status
notes Phase A is implemented and Phase B is not. AD-022, AD-023, and AD-024
were materialized as standalone entries in `docs/decisions/README.md`
(between AD-021 and AD-025), each with Context/Decision/Alternatives/
Consequences and a pointer back to this document's §2 for full text. This
document's own status line and §22 record the full verification gate.
`docs/reports/m5-contract-investigation.md`, `docs/reports/ff016-architecture-review.md`,
and AD-025/AD-026's history were not rewritten. See §22 for the gate
results.

---

## 17. Commit plan

Seven commits. Each must build and keep existing tests green. **Not one commit
per endpoint.** No commits are created by this task.

1. `test(architecture): narrow import guards and cover cmd/` — step 1.
2. `feat(engineering): add ParseEvidenceKey` + `feat(application): discovery composition` — steps 2–3.
3. `feat(transport): HTTP skeleton, DTOs, router, centralized errors` — steps 4–5.
4. `feat(transport): command endpoints` — step 6.
5. `feat(transport): query endpoints` — step 7.
6. `feat(cmd): featureforge server composition` — step 8.
7. `test(transport): canonical scenario through HTTP on both adapters` + documentation evidence — steps 9–11.

---

## 18. Acceptance criteria

Objective and checkable:

1. Exactly twelve command and seven query endpoints exist; no unauthorized
   endpoint is registered.
2. No route registers `PUT`, `PATCH`, or `DELETE`.
3. Every handler calls exactly one application entry point; none holds
   `Repositories`; none calls `UnitOfWork.Do`.
4. No transport type references a PEOS type — test-enforced.
5. `ParseEvidenceKey` is implemented and tested; `EvidenceKey`'s format is
   unchanged.
6. Discovery composes decision, execution, claim, and evidence identifiers
   deterministically, deduplicated and sorted.
7. Requirement and validation-plan identifiers are **not** caller
   prerequisites for Q3/Q4/Q5.
7a. Validation-plan selection follows §6.6 exactly: zero plans leaves
   `PlanArtifactID` empty without error; exactly one is used; more than one
   returns `ErrValidationPlanAmbiguous`. **No code path selects a plan by sort
   order**, and a test proves the multi-plan failure is independent of
   insertion order.
8. Requirements are never reconstructed from claims — asserted, not assumed.
9. Every sentinel present in `internal/application` at implementation time is
   mapped — 23 observed at the time of writing, plus `ErrValidationPlanAmbiguous`
   — and the exhaustiveness test fails when a sentinel is added without a
   mapping. The test, not the count, is the criterion.
10. Unknown errors return `500` with a generic body and no internal text.
11. Create-only replay returns success; conflicting replay returns `409`.
12. AD-026's `SubjectKey` conflict behaviour is preserved through HTTP.
13. `REQ-4` remains visible through the HTTP surface and remains without an
    applicable claim; readiness is not falsely complete.
14. `rationale` is present on every derived answer and omitted, never empty,
    otherwise.
15. The canonical scenario passes through HTTP on memory and on PostgreSQL.
16. The `net/http` guard is narrowed, not removed; each new guard was verified
    against a deliberate violation.
17. `cmd/featureforge` is covered by architecture rules.
18. No UI, template, or Phase B work is present.
19. `gofmt -l .`, `go vet ./...`, `go build ./...`, `go test ./... -count=1`,
    `go test ./... -race -count=1`, and `make postgres-test` all pass.

---

## 19. Open questions and readiness

AD-022, AD-023, and AD-024 are resolved in §2 and are **not open**.

**No blocking questions remain.** Two non-blocking clarifications are recorded
so they are not mistaken for oversights, each with its own resolution already
chosen:

| # | Item | Affected | Blocking? | Resolution |
|---|---|---|---|---|
| N1 | Whether `cmd/featureforge` needs a direct `pgxpool` import, or whether `internal/infrastructure/postgres` can expose enough that it does not | step 8; `TestOnlyPostgresInfrastructureImportsDriver` | no | Prefer no direct import. The implementer determines which is true while writing step 8 and records it; either outcome is architecturally acceptable and §13 states both. |
| N2 | Whether a capability may legitimately have more than one validation plan | Q3/Q4/Q5 `PlanArtifactID` | no | **Undetermined by the repository, and deliberately left so.** §6.6 records that no uniqueness invariant exists, that plans carry no acceptance or order metadata so "current plan" is undefined, and that the timeline contract represents at most one. The composition therefore *refuses* rather than guesses: `ErrValidationPlanAmbiguous` (§7.3). Phase A is unblocked because the condition is unreachable in the canonical scenario and now fails loudly instead of silently. Resolving whether multiple plans are legal — and if so how one is selected — needs a scenario that produces two, and is a decision for whoever encounters it. |

### Verdict

# READY FOR IMPLEMENTATION

1. **Is the complete Phase A public surface determined?** Yes — nineteen
   endpoints, fully specified in §3.
2. **Can it be implemented without changing existing repository interfaces?**
   Yes. No repository interface is touched.
3. **Can it be implemented without changing `UnitOfWork`?** Yes. Untouched.
4. **Are application additions required?** Yes — three discovery functions,
   four thin reads, per-endpoint composition functions (§6), the §6.6
   plan-resolution step, and **one new sentinel,
   `ErrValidationPlanAmbiguous`** (§7.3). All read-only or additive; no
   existing signature changes.
5. **Is `ParseEvidenceKey` required?** Yes — it does not exist and evidence
   discovery cannot be done safely without it (§7).
6. **Are new database migrations required?** No.
7. **Is standard-library `net/http` sufficient?** Yes — Go 1.25.12 provides
   method-aware `ServeMux` patterns and `PathValue` (§15.1).
8. **Is a standalone new architecture-decision file required before
   implementation?** No. §2 records the three reserved decisions in this
   specification, which CLAUDE.md permits. Decision-log pointers may be added
   later.
9. **Are any endpoints blocked?** No.
10. **May implementation begin immediately?** Yes, at step 1.

---

## 20. Documentation impact after implementation

The future implementation task may narrowly update:

- FF-015 status and implementation evidence;
- FF-018 status and implementation evidence;
- a README or run instructions for `cmd/featureforge`, if one is warranted;
- decision-index pointers for the reserved AD resolutions, if the project later
  chooses to materialize them as standalone entries.

**Must not be rewritten:** `docs/reports/m5-contract-investigation.md`,
`docs/reports/ff016-architecture-review.md`, AD-025's history, AD-026's
history, and FF-009. These are historical records; the append-never-rewrite
discipline applies.

## 21. What this document deliberately does not decide

- **Phase B in any respect** — screens, templates, forms, or UI routes. FF-015
  §6.4 sketches them; they are planned when Phase B is planned.
- **The identity generator's format** — AD-024 fixes the policy and the
  constraint (no UUID dependency, `crypto/rand`); the format is Phase B's,
  since Phase B is the first caller.
- **Pagination, caching, and content negotiation** — FF-015 §19's non-decisions
  carry forward unchanged.
- **Whether any query needs materialization** — still open, still requiring
  measured evidence, per AD-006.

---

## 22. Implementation evidence

**Commits.** §17's seven-commit plan landed exactly as specified:

1. `8e43f45` — `test(architecture): narrow import guards and cover cmd/` (step 1)
2. `267de0e` — `feat(engineering,application): add ParseEvidenceKey and discovery composition` (steps 2–3)
3. `27ba08e` — `feat(transport): HTTP skeleton, DTOs, router, centralized errors` (steps 4–5)
4. `ea919cc` — `feat(transport): command endpoints` (step 6)
5. `f356824` — `feat(transport): query endpoints (Q1-Q7)` (step 7)
6. `2f6026f` — `feat(cmd): featureforge server composition` (step 8)
7. `test(transport): canonical scenario through HTTP on both adapters` + documentation evidence (steps 9–11) — this commit

No genuine contradiction between repository behaviour and this document was
found. Every discrepancy implementation surfaced (§16 step 7's
`readiness` rationale correction; three pre-existing bugs in the guard this
document replaced) was narrow, evidence-grounded, and resolved within the
scope FF-018 already authorized, and is recorded at its own step rather than
silently swept aside.

**Acceptance criteria (§18), confirmed:**

1. Nineteen routes registered in `internal/transport/http/router.go`: twelve
   `POST` command routes, seven `GET` query routes. No other route.
2. No `PUT`, `PATCH`, or `DELETE` pattern appears in `router.go`;
   `TestNoRouteRegistersUnsupportedMethods` proves the three methods all
   return `405` against the live handler.
3. Every handler in `handlers_command.go`/`handlers_query.go` calls exactly
   one `application.*Command.Execute` or `application.Get*`/`List*`
   function; none holds `application.Repositories`; none calls
   `UnitOfWork.Do` — the package cannot, since `Dependencies` exposes only
   the `UnitOfWork` interface, never a concrete adapter.
4. `TestTransportDoesNotImportPEOS` and `TestNoPEOSTypeIsCopied`
   (`internal/architecture`) pass; no `dto_*.go` file imports
   `github.com/aleka7sk/PEOS`.
5. `ParseEvidenceKey` implemented and tested
   (`internal/engineering/refkeys.go`, `refkeys_test.go`); `EvidenceKey`'s
   format is untouched — only a parser was added.
6. `DiscoverDecisionIDs`, both Execution/Claim discovery scopes,
   `DiscoverEvidenceArtifactIDs`, and
   `DiscoverDecisionEvidenceArtifactIDs` proven deterministic, deduplicated,
   authoritative-before-filter, and fail-loud on inverse projection or exact
   reference corruption in `internal/application/discovery_test.go`.
7. / 7a. `TestResolveApplicableValidationPlanIDZero/One/Many/OrderingDoesNotResolveAmbiguity`
   cover all four cases; `TestCanonicalScenarioThroughHTTP` reaches Q3/Q4/Q5
   with no plan or requirement identifier supplied by the test beyond a
   `FeatureCardID`/`artifactID` path value.
8. `TestDiscoverRequirementArtifactIDsIsIndependentOfClaims` (carried from
   FF-016/AD-025, re-verified here) plus `TestCanonicalScenarioThroughHTTP`'s
   Q4 assertion of exactly four effective requirements, including `REQ-4`,
   which has no claim.
9. `TestErrorMappingIsExhaustive` parses `internal/application/errors.go` via
   `go/ast` at test time — 24 sentinels observed (23 pre-existing +
   `ErrValidationPlanAmbiguous`) — and was verified to fail when a sentinel
   is added without a mapping (deliberate-violation proof, reverted).
10. `internalErrorMessage` fallback in `errors.go`; `TestUnmappedErrorFallsBackOpaque`
    proves an unmapped error yields `500 internal_error` with the generic
    message and no leaked `err.Error()` text; `TestRecoverMiddlewarePreservesHTTPContract`
    proves a panicking handler yields the same, through `withMiddleware`.
11. `TestCommandIdempotentReplay` (`201`) and `TestCommandConflictingReplay`
    (`409`, code `immutable_value_conflict`).
12. `TestAssignLifecycleReplayHonorsAD026SubjectKeyEquality` isolates all
    three components `RevisionEnvelope.Equal` compares, through C6's entry-
    assignment path (chosen because its Transition Record Revision is
    content-free per AD-014, so `SubjectKey`'s contribution is isolable from
    `Payload`, unlike C3/C4/C7/C9): identical replay is a `201` no-op; a
    differing `SubjectKey` alone (same `RevisionKey`, byte-identical
    `Payload`) is `409`; a differing `Payload` alone (same `RevisionKey`,
    same `SubjectKey`) is `409`. Post-implementation audit finding MINOR-2.
13. `TestCanonicalScenarioThroughHTTP`'s Q4 assertion: `REQ-4` present with
    `has_claim=false`; overall `readiness.status = not-ready`, never `ready`.
14. `TestGetFeatureStateHandler` and `TestCanonicalScenarioThroughHTTP` assert
    `rationale.current_revision.rule` is non-empty on Q4; Q5's per-event
    `rationale` field is asserted present on every dated event, and its
    absence at the envelope's top level is asserted directly.
15. `TestCanonicalScenarioThroughHTTP` (memory) and
    `TestCanonicalScenarioThroughHTTPPostgres` (PostgreSQL, gated on
    `FEATUREFORGE_POSTGRES_TEST_DSN`) both pass; confirmed together via
    `make postgres-test`, exit `0`, zero failures.
16. `TestOnlyTransportAndCommandImportNetHTTP` narrows rather than removes
    the prior blanket guard; every one of the four replacement tests and the
    widened `TestNoTimeNowOutsideClock` allowlist was verified against a
    deliberate violation (introduced, confirmed failing for the right
    reason, reverted) during steps 1 and 4.
17. `allPackagesIncludingCmd`/`CmdPackages()` cover `cmd/featureforge`, and
    (post-implementation audit finding MAJOR-1, closed) `TestOnlyIntegrationPackageImportsPEOS`
    and `TestOnlyPostgresInfrastructureImportsDriver` were extended from
    `InternalPackages()` to `allPackagesIncludingCmd(t)` so the PEOS and
    driver boundaries genuinely inspect `cmd/`, not just the four AD-023
    guards — both still pass with no allow-list entry needed, confirming
    `cmd/featureforge` holds no direct PEOS-SDK or `pgx` import (N1, resolved
    in favour of type inference); each verified against a deliberate
    violation (a blank `github.com/aleka7sk/PEOS` / `github.com/jackc/pgx/v5`
    import added to `cmd/featureforge/main.go`, confirmed failing and naming
    `cmd/featureforge`, reverted).
18. No file under `internal/transport/http` or `cmd/` imports
    `html/template` or `text/template`; `TestOnlyUIImportsHTMLTemplate`
    passes with no holder registered.
19. Full gate, confirmed green after §23's corrections (below):

    ```
    gofmt -l .                        → clean
    go vet ./...                      → clean
    go build ./...                    → clean
    go test ./... -count=1            → ok, all packages
    go test ./... -race -count=1      → ok, all packages
    make postgres-test                → exit 0, zero failures
    ```

## 23. Post-implementation architecture audit

A read-only audit against this document, FF-015, and AD-022/023/024/025/026
was performed after §22's commit landed, treating the repository as an
external reviewer would. It found the transport substantially conformant and
one major, seven minor findings, all closed in a single corrective pass:

| Finding | What it found | Resolution |
|---|---|---|
| MAJOR-1 | `TestOnlyIntegrationPackageImportsPEOS` and `TestOnlyPostgresInfrastructureImportsDriver` iterated `InternalPackages()`, never `allPackagesIncludingCmd(t)` — `cmd/` was invisible to the PEOS and driver boundaries despite §13's and AD-023's claim otherwise | Both switched to `allPackagesIncludingCmd(t)`; both still pass with no allow-list entry needed (§18 criterion 17) |
| MINOR-1 | §16 step 5's fallback-opacity test and §14.3's transport-half panic-recovery test were never written | `TestUnmappedErrorFallsBackOpaque` (`errors_test.go`), `TestRecoverMiddlewarePreservesHTTPContract` (`middleware_test.go`) |
| MINOR-2 | §18 criterion 12 (AD-026 through HTTP) had no test isolating `SubjectKey`; the only conflicting-replay test was C1's `domain.Project` conflict, which never reaches `RevisionEnvelope.Equal` | `TestAssignLifecycleReplayHonorsAD026SubjectKeyEquality` (`commands_test.go`), via C6's content-free entry-assignment path |
| MINOR-3 | Five dangling `§18.1`/`§18.5` cross-references, left over from a `Required tests` section folded into §18 without renumbering | Repointed to §18's actual criteria (2, 10, 14, 16) |
| MINOR-4 | §10.3 still specified a `rationale.readiness` precedence-rule string that §16 step 7's implementation had already correctly omitted | §10.3 amended in place; the field does not exist |
| MINOR-5 | §10.3 specified composite `key` fields (`selected_key`, `considered[].key`, `rejected[].key`); the implementation correctly follows §10.2's own split `artifact_id`/`revision_id` convention instead | §10.3 amended to the split form |
| MINOR-6 | §10.3 overstated Q6's per-revision `sequence` coverage; only the current revision's entry carries it | §10.3 amended to state where the rest is recoverable (`rationale.considered[]`) |
| MINOR-7 | §22 criterion 2's evidence line was internally incoherent (a `grep` over `router.go` cannot match commit messages) | Replaced with a plain, checkable statement |

No architecture decision was reopened. AD-022, AD-024, AD-025, and AD-026 were
found correctly realized as specified; AD-023's decision was found correct,
with only its enforcement extension to `cmd/` incomplete (MAJOR-1) — an
implementation gap against an accepted decision, not grounds to revisit it.

### Pointer: read-surface extension

Subsequent Phase B UI planning found a second, independent gap this document
did not cover: the query surface above exposes identity, projections, and
rationale metadata, but not the engineering content FF-001 §3 requires a
reader to see. That gap and its closure are recorded in
[FF-020](020-read-surface-extension.md) and
[AD-027](../decisions/README.md#ad-027--read-surface-content-is-projected-on-read-never-stored-through-a-sibling-engineeringprojector-port) —
an additive extension to the response DTOs above, not an amendment. The HTTP
surface remains the nineteen operations this document specifies; FF-020 added
no endpoint.

## 24. M.5 publication-remediation correction — Q5 is history-wide, Q3/Q4 are not

A read-only publication-readiness audit of this milestone's local commit
chain, performed before any of it was pushed, found that §6.3 and §6.4 above
say one thing and `discoverEngineeringStateComponents`
(`internal/application/query_reads.go`) did another. Full evidence,
including the executable red/green regression, is in
[m5-publication-remediation.md](../reports/m5-publication-remediation.md);
this section states the corrected contract in place.

**What §6.3 already said.** "Where a history-wide view is wanted (the
timeline), the caller iterates revisions as decision discovery does." That
is correct and remains the rule.

**What the implementation actually did.** `discoverEngineeringStateComponents`
is called by both the current-state queries (Q3 `GetFeatureOverview`, Q4
`GetFeatureEngineeringStateForCard`) and the timeline query (Q5
`GetFeatureTimelineForCard`). It resolved the capability's *current*
revision once and scoped execution, claim, and evidence discovery to that
one revision — correct for Q3/Q4, silently wrong for Q5. An execution, a
piece of evidence, or a claim recorded against an earlier capability
revision disappeared from the timeline the moment a later revision became
current, though nothing corrected or withdrew it. This is exactly the
"visible, not hidden" exit criterion FF-007 states for M.5, and FF-006 §1
defines the timeline as computed over *every* immutable record — not the
current revision's alone.

**The corrected contract.**

| Query | Population |
|---|---|
| Q3 `GetFeatureOverview`, Q4 `GetFeatureEngineeringStateForCard` | Executions, claims: scoped to the **current** capability revision only (unchanged — these are current-*state* queries, and broadening them would let a stale claim read as satisfying the current revision, which FF-010 §7 forbids) |
| Q5 `GetFeatureTimelineForCard` | Decisions, Executions and Claims are discovered only after globally enumerating and inspecting all Revision/Record envelopes. Executions and Claims are then unioned across **every** validated capability revision and deduplicated by their own record ID. Evidence is the deduplicated union of exact citations from the validated Decisions plus that history-wide Execution/Claim population; every cited pair must resolve and validate before any timeline is returned. |

`DiscoverExecutionAndClaimIDsAllRevisions` (`internal/application/query_timeline.go`)
is the new, dedicated, history-wide discovery function Q5 uses; the
existing per-revision `DiscoverExecutionAndClaimIDs` (§6.3) is unchanged and
remains what Q3/Q4's current-state population would use if they ever needed
one — today they do not consume it at all, since readiness resolves claims
independently through `ResolveCurrentClaim`, scoped to the current
revision's subject key. Requirement, decision, and validation-plan discovery
were already history-wide (or plan-cardinality-appropriate) and retain their
result shapes. The initial publication fix added no repository method; the
later AD-032/FF-023 integrity correction adds deterministic `ListAll` to both
Revision and Record repository ports so untrusted projections are never the
first discovery filter. No migration or stored projection is added; the
remaining fix stays in `internal/application`'s discovery/composition layer,
inside the same single `UnitOfWork.Do` each query already used.

**Architecture guards, restated exactly.** §12.3's table above is Phase
A's, before `internal/ui` existed; it is not rewritten. As currently
required, restoring the `text/template` half §12.3 already specified but
the four-way decomposition silently dropped (AD-023, M-2):

- `html/template` — permitted only in `internal/ui`; forbidden everywhere
  else, including `cmd/` (`TestOnlyUIImportsHTMLTemplate`, unchanged).
- `text/template` — forbidden absolutely: no holder, anywhere under
  `internal/` or `cmd/` (`TestTextTemplateIsNeverImported`, restored by this
  remediation, mirroring `TestDatabaseSQLIsNeverImported`'s shape).

**No architecture decision is reopened.** AD-022, AD-024, AD-025, AD-026,
AD-027, and AD-028 are unaffected; AD-023's own decision text already said
`text/template` remains forbidden — only its test coverage had drifted from
it, restored here.
