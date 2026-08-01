# FeatureForge Lessons Learned for Belcanto

Status: Freeze artifact; independent review passed, publication gate status is
recorded in the M.7 audit ledger
Date: 2026-08-01
Audience: the Belcanto team before product architecture begins

---

## 1. Executive conclusion

FeatureForge succeeded as an evidence-producing consumer of PEOS. Its durable
output is the reasoning that separated authority, history, trace, replay,
derived state, persistence, and AI review. Its code is not a Belcanto starter
kit.

Belcanto must not import, copy, extract, vendor, or wrap FeatureForge packages,
and must not create a shared PEOS integration library from this single
consumer. Reuse means re-running the reasoning against Belcanto's domain and
recording Belcanto's own decisions. Redesign means choosing Belcanto's own
entities, authority model, ports, contracts, API, UI, tests, and operations.

## 2. What Belcanto should reuse as rationale

| Lesson to reuse | Why the rationale survived FeatureForge | What reuse means |
|---|---|---|
| Separate operational data from engineering authority | It prevented mutable navigation records from becoming a stale copy of immutable PEOS-backed history | Classify Belcanto concepts by authority and change semantics before selecting storage or endpoints |
| Keep PEOS behind a consumer-owned boundary | Product commands and queries remained PEOS-free while one package owned construction/inspection | Design a Belcanto-specific seam from its use cases; do not expose SDK types across the application boundary |
| Treat payload as authority and projections as discovery aids | It made hidden corruption and projection disagreement explicit instead of silently filtering records out | Declare authority/projection rules per Belcanto record and validate before filtering |
| Append history and derive current state with rationale | Current revision, correction head, lifecycle, and readiness stayed deterministic and auditable | Specify each derived answer, ambiguity case, and explanation before implementation |
| Define semantic replay per engineering act | Retries stayed stable despite server time and historical identity conventions | Identify Belcanto act identity, immutable request semantics, occupancy, corruption precedence, and zero-write branches |
| Persist exact structured trace | UI, timeline, and AI context could name exact sources without parsing prose | Model the exact relations Belcanto must answer, with referential and semantic validation |
| Verify adapters through one behavior suite | A genuinely different PostgreSQL adapter tested the abstraction rather than mirroring the memory store | Build Belcanto's own shared adapter suite from its contracts |
| Make architecture constraints executable | Static tests caught real drift in imports, mutation surfaces, vocabulary, and AI authority | Turn important Belcanto decisions into failing checks and prove the checks with deliberate violations |
| Keep generated proposals non-authoritative | Generation could fail, be discarded, or become stale without silently creating engineering state | Separate context assembly, provider execution, review, and the ordinary authoritative command |
| Audit the consumer journey, not only the end state | M7-01 showed that a perfect final store can still be unreachable through the claimed public surface | Require end-to-end journeys to use every real intent boundary |

The companion [Reusable Patterns](reusable-patterns.md) states these lessons as
context/decision/rationale patterns. Neither document transfers an
implementation.

## 3. What Belcanto must redesign

FeatureForge made bounded choices for one local user and single-digit scenario
data. The following areas require fresh Belcanto decisions rather than extension
of the POC.

| Area | FeatureForge choice | Why Belcanto must redesign it |
|---|---|---|
| Operational entities and mutability | Project and FeatureCard have stable establishment fields plus one monotonic capability link | Belcanto's product entities, edits, audit requirements, and concurrency semantics are not represented by that POC choice |
| Identity and actors | Deterministic fixture IDs, caller-supplied engineering identities, and one fixed `featureforge:local-user` actor | Belcanto needs its own identity authority, actor attribution, authorization relationship, and collision/replay policy |
| Authentication and tenancy | Intentionally absent | These are product architecture concerns with security and data-isolation consequences; they cannot be retrofitted from a one-user fixture |
| API contract | Twelve intent-oriented HTTP commands and a bounded query set | Belcanto must derive commands, queries, errors, versions, pagination, and compatibility from its own user journeys |
| UI | Server-rendered, no JavaScript, in-process handler bridge | This proves comprehensibility and authority routing, not a production interaction architecture |
| Persistence and query scale | Correctness-first repository methods, including global `ListAll` scans | Belcanto volume, isolation, latency, indexing, pagination, and read-model needs require measured design |
| AI provider and review | Stateless deterministic generator with local review/discard/accept | Belcanto must decide provider boundaries, privacy, failure handling, observability, evaluation, prompt/model versioning, and review policy |
| Vocabulary and structured content | `featureforge` namespace and FeatureForge capability/Requirement/plan schemas | These names and shapes encode the POC, not Belcanto's domain language |
| Lifecycle | One fixed persisted definition/version and a linear history | Belcanto must decide whether lifecycle is needed, who governs it, how it evolves, and what concurrency/branching semantics apply |
| Verification | Canonical homework scenario and FeatureForge-specific architecture allowlists | Belcanto needs its own representative journeys, adversarial cases, package topology, and acceptance contract |

## 4. Local compromises that must not become defaults

### 4.1 Unit-of-work reentrancy detection

Both adapters use `runtime.Stack`-based goroutine identification to reject a
nested `UnitOfWork.Do`, because the callback signature cannot return a marked
context to its caller. It produced adapter parity for the POC, but it depends on
runtime formatting rather than an explicit transaction capability. Belcanto
must choose a deliberate reentrancy/context contract; it should not copy this
mechanism.

### 4.2 Fixed serialization retry policy

PostgreSQL uses a fixed `maxAttempts = 8` and a fixed backoff curve. This was
enough to prove whole-callback retry and distinct concurrent revision sequence
assignment. It was not tuned under production load and carries no operational
telemetry. Belcanto must set retry, timeout, cancellation, idempotency, and
observability policy from its own service objectives.

### 4.3 Whole-population discovery

`RevisionEnvelopeRepository.ListAll` and `RecordEnvelopeRepository.ListAll`
support authoritative discovery: validate the full population before applying
projection filters. That favors correctness at POC scale and makes the rule
easy to falsify. It is O(n) and is not evidence that Belcanto should scan all
engineering history per request. Belcanto must preserve the authority rule
while designing indexes, partitions, or validated read models for measured
scale.

### 4.4 In-process, no-JavaScript UI bridge

The UI calls the same HTTP handlers in process and intentionally carries no
application command dependency. This proved that forms could traverse the real
API boundary and that rendering stayed server-owned. It did not exercise
network failure, browser-side state, accessibility beyond the bounded pages,
sessions, distributed tracing, caching, or deployment separation. Treat it as a
test of authority routing, not a UI platform choice.

### 4.5 Deterministic AI generator

The deterministic generator is valuable because it proves that one immutable
context value is sufficient and that generation has no repository or
transaction authority. It says nothing about model quality, provider behavior,
privacy, latency, streaming, quotas, safety review, or reproducibility across
provider versions. Belcanto should retain the narrow seam and redesign
everything provider-facing.

### 4.6 FeatureForge vocabulary, content, and package topology

The `featureforge` vocabulary, structured capability content, Requirement
criterion trace, acceptance-member rules, package names, and architecture
allowlists are all consumer decisions. Copying them would silently turn
FeatureForge assumptions into Belcanto contracts. Belcanto should establish its
own namespace and types, then add its own closed-vocabulary and import guards.

### 4.7 Lifecycle entry and policy workaround

FeatureForge needed a special entry Transition Revision shape, a persisted
fixed policy pair, and consumer validation of every edge and predecessor. That
closed its canonical scenario but exposed PEOS API ergonomics and consumer
responsibilities. Belcanto should begin with its lifecycle questions, not with
FeatureForge's four states or construction sequence.

## 5. PEOS-specific lessons

### 5.1 PEOS is a value model, not an application framework

PEOS correctly leaves transactions, persistence, indexes, referential
integrity, aggregate membership, replay, product queries, and UI rationale to
the consumer. The cost is real and should be planned, but filling those gaps in
a product does not justify changing PEOS or creating a generic workflow engine.

### 5.2 Similar-looking concepts must stay separate

- execution outcome is not Claim outcome;
- lifecycle progress is not release readiness;
- revision acceptance is not lifecycle;
- Origin/Derivation is not the complete product trace model;
- a syntactically valid reference is not a resolved authoritative target;
- a Decision subject may be an Artifact or an Artifact Revision;
- `SubjectKey` is a governed projection, not universally reconstructable
  payload state.

These distinctions prevented duplicated truth. Belcanto should make its own
product terms map explicitly to PEOS constructs before writing commands or
screens.

### 5.3 Constructor success is not consumer invariant success

FeatureForge still had to reject self-correction, correction cycles, invalid
lifecycle edges, stale or corrupt sources, foreign families, contradictory
projections, and incomplete aggregates. Belcanto should inventory these
consumer invariants and their error precedence rather than treating a
successfully constructed PEOS value as a complete product act.

### 5.4 One consumer does not justify a shared integration library

FeatureForge proves that a narrow boundary is possible. It does not prove which
boundary is common. Only after Belcanto independently implements its own
consumer seam could two concrete designs be compared for genuinely identical,
stable abstractions. Even then, duplication may be cheaper and safer than a
cross-product dependency.

## 6. Recommended Belcanto starting discipline

After FeatureForge is formally frozen, Belcanto should start with decisions,
not transferred code:

1. write its product acceptance contract and representative end-to-end
   scenarios;
2. identify operational entities, engineering acts, authority, actors, and
   tenancy boundaries;
3. map each engineering concept to PEOS or to an explicitly consumer-owned
   value;
4. specify immutable history, exact trace, current-state derivation, ambiguity,
   replay, and integrity precedence;
5. define Belcanto-owned ports and run one vertical slice against two adapters
   before treating the abstraction as validated;
6. add API, UI, AI, and architecture verification only from those Belcanto
   decisions.

This is sequencing guidance, not the Belcanto design itself.

## 7. Deliberate stopping point

This handover stops before application design. It does not select Belcanto
screens, routes, DTOs, session model, identity provider, authorization scheme,
login page, deployment topology, or visual system. Those choices require joint
product work after the FeatureForge M.7 freeze evidence is complete.
