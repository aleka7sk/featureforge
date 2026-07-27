# FF-007 — Delivery Roadmap

Status: Accepted (Phase M.1)
Governs: phase sequence, per-phase objectives, entrance and exit criteria, and
recommended execution mode.

## Phase sequence

```
M.1  Product Definition and Acceptance Contract            ← complete
M.2  Architecture and In-Memory Vertical Slice Specification
M.3  In-Memory Vertical Slice Implementation
M.4  PostgreSQL Persistence and Current-State Queries
M.5  HTTP API and Minimal UI
M.6  AI Context-Pack Demonstration
M.7  Independent End-to-End PEOS Consumer Audit
────────────────────────────────────────────────  FeatureForge frozen
B.0  Belcanto Product Architecture and PEOS Integration Boundary
```

The roadmap is accepted as given. One adjustment was made and is stated
explicitly under M.4.

---

## M.1 — Product Definition and Acceptance Contract

**Objective.** Settle every material architecture decision in writing so no
implementation phase has to choose. Define what FeatureForge is, what it is not,
and when it stops.

**Deliverables.**
- `docs/spec/000` through `docs/spec/007`
- `docs/decisions/README.md` with AD-001 through AD-012 accepted
- `docs/glossary.md`
- `README.md`
- one documentation-only commit

**Entrance criteria.** A repository exists; PEOS v1.0.0 is resolvable.

**Exit criteria.**
- every required document exists and every internal link resolves;
- domain boundary, revision ordering, current-state ownership, persistence
  authority, correction semantics, timeline ownership, and the AI authority
  boundary are all decided;
- no implementation code was created;
- the PEOS module is unmodified.

**Model.** Claude Opus. **Mode.** Product definition and architecture decision.

---

## M.2 — Architecture and In-Memory Vertical Slice Specification

**Objective.** Turn FF-000..FF-007 into an implementation packet precise enough
that M.3 writes code without making architecture decisions.

**Deliverables.**
- package layout under `internal/`, with each package's responsibility and its
  permitted imports;
- the FeatureForge-owned type set: Project, FeatureCard, Specification Content,
  Revision Sequence, acceptance journal, context pack, timeline event;
- repository contract signatures for every family in
  [FF-003 §7](003-peos-integration.md#repository-families), expressed over the
  FeatureForge record envelope so no adapter imports PEOS;
- the integration layer's surface: constructor wrappers, reference conversions,
  the vocabulary constants file;
- use-case signatures, input and output shapes, and error taxonomy;
- the canonical JSON form and digest rule for Specification Content;
- the architecture-test list, with the mechanism each uses;
- the test plan: every "Required tests" list across FF-003..FF-006, gathered and
  assigned to a package.

**Entrance criteria.** M.1 exit criteria met.

**Exit criteria.**
- every use case has a signature and named errors;
- every repository contract is declared by its consumer, not by an adapter;
- no open architecture question remains for M.3;
- still no implementation code.

**Model.** Claude Opus.
**Mode.** Architecture specification and implementation packet preparation.

---

## M.3 — In-Memory Vertical Slice Implementation

**Objective.** Make the canonical scenario pass end to end, in memory, with no
HTTP and no database.

**Deliverables.**
- `go.mod` declaring `github.com/aleka7sk/PEOS v1.0.0` with no `replace`
  directive — this is the first phase that may add it;
- domain, application, integration, and in-memory infrastructure packages;
- the architecture tests from [FF-002 §4](002-domain-boundaries.md#enforcement);
- the full canonical scenario as an executable test, ending `not-ready`;
- persistence contract tests against the in-memory adapter;
- ordering, current-state, correction, and timeline tests.

**Entrance criteria.** M.2 exit criteria met.

**Exit criteria.**
- the canonical scenario passes end to end;
- architecture tests pass and genuinely fail when a boundary is deliberately
  broken (each is verified against an intentional violation);
- insertion-order independence is demonstrated;
- ambiguity cases fail with the specified named errors;
- the correction flow leaves history intact;
- the timeline matches [FF-006 §7](006-timeline-read-model.md#7-worked-timeline)
  exactly;
- the PEOS module is unmodified.

**Model.** Claude Opus. **Mode.** Implementation against a fixed packet.

---

## M.4 — PostgreSQL Persistence and Current-State Queries

**Objective.** Replace the in-memory adapter with PostgreSQL, proving the
persistence contract is real rather than an artefact of in-process storage.

**Deliverables.**
- schema and migrations — the first migrations in the project;
- the PostgreSQL adapter implementing the same contracts;
- the persistence contract suite run against **both** adapters from one shared
  test body;
- transaction-boundary tests: one engineering act commits atomically;
- concurrency tests: two concurrent revision creations produce *n* and *n+1*;
- typed-column projections for indexing, with a test asserting each projection
  agrees with its authoritative payload.

**Adjustment from the given roadmap.** The phase title pairs PostgreSQL with
current-state queries, but the queries are implemented in M.3 against the
in-memory adapter — they must be, since the scenario test depends on them. M.4's
query work is therefore *re-verification against PostgreSQL*, not first
implementation. This is a clarification of sequencing, not a change of scope.

**Entrance criteria.** M.3 exit criteria met.

**Exit criteria.**
- both adapters pass the identical contract suite;
- round-trip, idempotence, and conflict behaviour verified on PostgreSQL;
- no `UPDATE` or `DELETE` statement exists against an engineering-record table;
- every derived model can be dropped and rebuilt with identical output;
- the canonical scenario passes against PostgreSQL.

**Model.** Claude Opus. **Mode.** Implementation and storage semantics
verification.

---

## M.5 — HTTP API and Minimal UI

**Objective.** Prove a person can drive and understand the whole lifecycle.

**Deliverables.**
- HTTP transport over the existing use cases — no new business logic;
- transport models containing no PEOS type, test-enforced;
- the seven screens in
  [FF-001 §3](001-poc-acceptance-contract.md#3-minimal-user-experience);
- rationale rendered in the UI wherever a derived answer is shown.

**Entrance criteria.** M.4 exit criteria met.

**Exit criteria.**
- the full lifecycle is drivable through the UI;
- a reader answers every question in FF-001 §3's usability acceptance from the UI
  alone;
- superseded claims and prior revisions are visible, not hidden;
- no authentication, notification, or collaboration feature exists.

**Model.** Claude Opus, or Claude Sonnet for the UI layer once the API is fixed.
**Mode.** Transport and interface implementation.

---

## M.6 — AI Context-Pack Demonstration

**Objective.** Demonstrate the AI authority boundary from
[FF-001 §4](001-poc-acceptance-contract.md#4-ai-boundary).

**Deliverables.**
- context-pack assembly for the canonical feature, every element carrying its
  exact source reference;
- a proposal type and a proposal generator behind an interface — a stub
  implementation is sufficient;
- an accept/reject UI flow;
- provenance on an accepted proposal naming its sources;
- a test asserting the proposal path has no repository or transaction access.

**Entrance criteria.** M.5 exit criteria met.

**Exit criteria.**
- a context pack is generated and every element names its source;
- a rejected proposal leaves no engineering state;
- an accepted proposal produces an ordinary revision with human provenance and an
  AI-assisted method value;
- the no-write-access test passes.

**Model.** Claude Opus. **Mode.** Bounded capability implementation.

---

## M.7 — Independent End-to-End PEOS Consumer Audit

**Objective.** Have a reviewer with no stake in the implementation try to break
the claims in [FF-001 §6](001-poc-acceptance-contract.md#6-acceptance-contract).

**Deliverables.**
- an audit report with findings classified BLOCKER / MAJOR / MINOR / OBSERVATION;
- for each acceptance-contract line, the evidence that satisfies it or the finding
  that does not;
- a PEOS consumer report: what was awkward, what was missing, what was misread —
  input to PEOS's own backlog, not a change to PEOS from here;
- the patterns and lessons-learned documents from
  [FF-001 §7](001-poc-acceptance-contract.md#freeze-artifacts).

**Entrance criteria.** M.6 exit criteria met.

**Exit criteria.**
- every acceptance-contract line is evidenced;
- no BLOCKER and no MAJOR finding remains open;
- freeze artifacts exist.

**Model.** Claude Opus. **Mode.** Adversarial independent audit — the auditor
reads the specifications and the code and attempts to falsify each claim.

**On freeze:** FeatureForge development stops.

---

## B.0 — Belcanto Product Architecture and PEOS Integration Boundary

**Objective.** Design Belcanto's PEOS integration boundary, informed by
FeatureForge but not inheriting its code.

**Deliverables.**
- Belcanto's domain and PEOS boundary;
- an explicit decision, per FeatureForge pattern, to reuse or redesign;
- Belcanto's own vocabulary namespace.

**Entrance criteria.** FeatureForge frozen; freeze artifacts read.

**Exit criteria.**
- Belcanto's boundary is decided;
- no FeatureForge code is imported as a library;
- the reuse-or-redesign decision is recorded for each pattern.

**Model.** Claude Opus. **Mode.** Architecture specification.

---

## Phase summary

| Phase | Objective | Model | Mode |
|---|---|---|---|
| M.1 | Product definition and acceptance contract | Opus | Product definition and architecture decision |
| M.2 | Architecture and vertical slice specification | Opus | Architecture specification and packet preparation |
| M.3 | In-memory vertical slice | Opus | Implementation against a fixed packet |
| M.4 | PostgreSQL persistence | Opus | Implementation and storage verification |
| M.5 | HTTP API and minimal UI | Opus / Sonnet for UI | Transport and interface implementation |
| M.6 | AI context-pack demonstration | Opus | Bounded capability implementation |
| M.7 | Independent audit | Opus | Adversarial independent audit |
| B.0 | Belcanto boundary | Opus | Architecture specification |

## Constraints that hold across every phase

- The PEOS repository and SDK are never modified.
- The dependency stays exactly `github.com/aleka7sk/PEOS v1.0.0`, with no
  `replace` directive pointing at a local checkout.
- No phase adds an entity, a use case, or an abstraction that the canonical
  scenario does not require.
- Material architecture decisions are recorded in
  [`docs/decisions/`](../decisions/README.md) before they are implemented.
