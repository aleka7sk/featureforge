# FF-024 — AI-assisted proposal workflow

Status: Accepted (implementation pending)
Date: 2026-08-01
Phase: M.6 AI context-pack demonstration
Governs: AD-034 implementation, exact context-pack assembly, transient proposal
identity, deterministic generation, reviewed acceptance, HTTP/UI flow, replay,
and the M.6 exit gate

## 1. Purpose and boundary

This packet implements the bounded AI demonstration selected by FF-001 §4,
AD-012 and AD-034. It proves three things:

1. FeatureForge can assemble one exact, source-bearing view of the current
   engineering state;
2. a proposal generator can operate without any read or write authority; and
3. only explicit acceptance can convert reviewed proposal content into an
   ordinary capability Revision.

This packet does not add authentication, authorization, users, organizations,
an AI/LLM provider, a network call, a prompt store, a proposal repository, a
session, a cache, a background job, a generic agent runtime, streaming, tools,
vector search, retrieval infrastructure, or a new PEOS concept. It does not
change acceptance/current-revision semantics: an accepted proposal produces a
draft Revision and C5 remains the only way to accept it.

The PEOS v1.0.0 module remains unchanged and no `replace` directive is added.

## 2. Package and authority architecture

### 2.1 Pure proposal package

Create `internal/proposal`. Its production imports are limited to:

```text
Go standard library
github.com/aleka7sk/featureforge/internal/engineering
```

It must not import:

```text
internal/application
internal/domain
internal/infrastructure
internal/transport
internal/ui
internal/engineering/peos
github.com/aleka7sk/peos-go
any provider or network package
```

It owns only values and pure functions:

```text
ContextPack
Proposal
SourceReference
canonical context/proposal encoding and digest validation
Generator
DeterministicGenerator
```

The generator interface is intentionally narrower than a conventional service
interface:

```go
type Generator interface {
    Generate(ContextPack) (Proposal, error)
}
```

There is no `context.Context` parameter and no optional dependency. The only
information available to a generator is the immutable ContextPack value.

### 2.2 Application ownership

`internal/application` owns:

- `AssembleProposalContext`, which reads in exactly one UnitOfWork;
- `GenerateCapabilityProposal`, which calls the assembler, waits for the
  UnitOfWork callback to return, and then calls `Generator.Generate`;
- `AcceptCapabilityProposal`, which validates/replays or records the proposed
  Revision in exactly one UnitOfWork; and
- application sentinels for invalid/stale proposal input.

Only the API HTTP application's dependencies receive the Generator explicitly
from `cmd/featureforge`. The HTTP handler passes it to the application generate
operation. `internal/ui` does not receive or construct a Generator and does not
import `internal/proposal`, `internal/application`, `internal/engineering`,
`internal/domain` or `internal/infrastructure`. It retains AD-028's existing
in-process HTTP client/handler boundary for both reads and writes.

### 2.3 Call-order invariant

Generate has this observable order:

```text
validate route/input
-> UnitOfWork.Do(read and assemble ContextPack)
-> UnitOfWork callback returns
-> transaction closes
-> Generator.Generate(pack)
-> application result
-> API HTTP response
```

The generator is never invoked from inside `UnitOfWork.Do`. A spy UnitOfWork
and spy Generator must prove this order. Generator failure occurs after a
read-only transaction and therefore leaves the store unchanged.

The UI can observe the result only by sending an in-process HTTP request to the
API handler and decoding its HTTP response; it is not another caller of the
application operation.

Accept does not call the generator. It validates the canonical proposal the
review page returns.

## 3. Exact ContextPack contract

### 3.1 Root

The input to context assembly is one capability Artifact ID. Inside one
UnitOfWork, the application:

1. validates the capability Artifact, its unique FeatureCard ownership and its
   complete managed Revision/order/acceptance history;
2. resolves the accepted current Revision through FF-004;
3. decodes and recomputes the exact current
   `CapabilitySpecificationContent` and digest;
4. resolves the remaining sections below against that exact current Revision;
5. constructs canonical ordering and source references; and
6. computes the ContextDigest before leaving the callback.

No accepted current Revision is `ErrNoAcceptedRevision`. Foreign capability
occupancy is immutable conflict. Any unreadable payload, contradictory
projection, dangling reference, duplicate authoritative identity, mixed family
or invalid managed history is stored-state integrity. No partial ContextPack is
returned.

### 3.2 Wire/value shape

The PEOS-free proposal value is equivalent to:

```text
ContextPack
  Capability
    Revision                 exact RevisionKey
    Sequence                 positive managed sequence
    Content                  exact CapabilitySpecificationContent
    ContentDigest            recomputed engineering Digest
  Requirements[]
    Revision                 exact Requirement RevisionKey
    Sequence
    Statement
    SourceCapabilityRevision exact AD-033 RevisionKey
    SourceCriterionKey       exact revision-local key
  Claims[]
    Record                    exact validation Claim RecordKey
    RequirementRevision      exact Requirement RevisionKey
    CapabilityRevision       exact current capability RevisionKey
    ScopeArtifactID
    CriterionKeys[]
    Outcome
    Reasoning
    ExecutionReferences[]
    EvidenceReferences[]
  Decisions[]
    DecisionID
    Subject                  exact authoritative Artifact or Revision subject
    OutcomeStatement
  OpenQuestions[]
    CapabilityRevision       exact current capability RevisionKey
    Ordinal                  one-based content-list position
    Text
  UncoveredCriteria[]
    CapabilityRevision       exact current capability RevisionKey
    CriterionKey
    CriterionText
    Reason                   no_requirement_trace | missing_current_claim
    RequirementRevisions[]   exact affected Requirement RevisionKeys
  Findings[]
    CapabilityRevision       exact current capability RevisionKey
    CriterionKey
    RequirementRevision      exact Requirement RevisionKey
    ClaimRecord              exact current Claim RecordKey
    Outcome                  not-satisfied | inconclusive
    Reasoning
  Sources[]                  canonical exact SourceReference values
  ContextDigest              digest of every field above except itself
```

All slices are non-nil. Values returned from constructors and accessors are
defensive copies. The zero ContextPack is invalid.

### 3.3 Exact source references

`SourceReference` is a canonical string with one of these grammars:

```text
artifact:<artifact_id>
revision:<artifact_id>/<revision_id>
requirement-trace:<requirement_artifact_id>/<requirement_revision_id>
criterion:<capability_artifact_id>/<capability_revision_id>#<criterion_key>
record:<record_kind>/<record_id>
```

The existing identity and criterion grammars exclude `/` and `#`, so these
forms are unambiguous. The ContextPack source set is the deduplicated union of:

- its current capability Revision;
- every effective Requirement Revision, its trace, and the trace's exact source
  capability Revision/criterion;
- every exact current capability criterion;
- every included current Claim;
- every included Claim's exact Execution/Evidence references; and
- for every applicable Decision, both `record:decision/<decision_id>` and the
  Decision's exact authoritative subject reference: `artifact:<artifact_id>`
  or `revision:<artifact_id>/<revision_id>`.

The Decision subject pair is mandatory even if the subject Revision is older
than the current Revision. The already-present current Revision source never
stands in for that exact historical subject.

References are sorted bytewise by their complete canonical string. A dangling
reference is not included as text; it fails authoritative assembly according
to the existing integrity rule for that family.

### 3.4 Effective Requirements

Requirement discovery uses FF-016/FF-023 authoritative-before-filter
enumeration. It validates every discovered managed Requirement history before
selecting the histories whose authoritative subject is the capability
Artifact. It then resolves one accepted current Revision per Requirement and
loads the exact AD-033 trace by that Revision key.

Every effective Requirement is included, even when its trace names an older
capability Revision. The stored trace is reported honestly; only a trace to the
exact current Revision/key counts toward current criterion coverage.

Rows are sorted by `(Requirement ArtifactID, Requirement RevisionID)`. A
missing, corrupt, dangling, wrong-family or non-existent-criterion trace fails
the whole pack as stored-state integrity.

### 3.5 Current Claims

For every effective Requirement, derive its Requirement criterion key and run
the FF-010 correction resolver for:

```text
subject = exact current capability Revision
scope   = featureforge:capability|<capability artifact id>
criteria= [exact Requirement criterion key]
```

When a unique current Claim exists, decode its criteria, outcome and reasoning
and include it. `not-satisfied` and `inconclusive` Claims are included. A
legitimate no-head/no-claim result adds no Claim row and participates in
uncovered computation. A correction cycle, dangling target, family mismatch or
multiple current heads fails through the existing governed error.

Rows are sorted by `(Requirement ArtifactID, Requirement RevisionID,
Claim RecordID)`; resolution still uses graph semantics, never this sort or a
timestamp.

### 3.6 Applicable Decisions

Reuse FF-004 §3.3/FF-018's existing applicability rule: a Decision is
applicable when its authoritative decoded subject names the capability Artifact
or any validated Revision in that Artifact's history. A Decision against an
earlier Revision remains relevant after a later Revision becomes current; this
is how canonical DEC-1, which explains the move from Revision 1 to Revision 2,
remains in the pack. Discovery enumerates Decision records globally, validates
authoritative payload/digest/projection agreement, then filters; a projected
subject is never trusted first. The pack includes its Decision ID, exact
authoritative Artifact-or-Revision subject and decoded outcome statement. Rows
follow FF-004's governed order by provenance `RecordedAt`, then Decision ID.
Its source contribution is always the exact Decision record plus that exact
subject, including an older Revision when that is what the Decision names.

Decision evidence is not reinterpreted as proposal input. A coherent unresolved
C8 evidence citation remains governed C8 state and does not erase the Decision;
an unreadable or projection-contradictory Decision is stored-state integrity.

### 3.7 Open questions

Open questions are copied in their exact order from the current capability
content. Each row names the exact current Revision and its one-based ordinal.
They are not inferred as resolved by matching Decision prose.

## 4. Coverage and finding algorithms

Iterate acceptance criteria in their current content order. For each exact
`(current Revision, criterion key)`:

```text
mapped = effective Requirements whose AD-033 trace equals that exact pair
missing = mapped Requirements for which no applicable current Claim was found
```

Then classify:

| Stored semantic state | Uncovered? | Finding? |
|---|---:|---:|
| `mapped` is empty | yes, `no_requirement_trace` | no |
| `mapped` non-empty and `missing` non-empty | yes, `missing_current_claim` | only for other mapped negative/inconclusive Claims, if any |
| every mapped Requirement has `satisfied` current Claim | no | no |
| every mapped Requirement has a Claim and at least one is `not-satisfied` | no | yes for each negative Claim |
| every mapped Requirement has a Claim and at least one is `inconclusive` | no | yes for each inconclusive Claim |

If a criterion has several mapped Requirements, all must have an applicable
current Claim for the criterion to be covered. A negative or inconclusive
Claim is still a Claim and therefore supplies coverage. Findings are sorted by
criterion order and then Requirement Revision key. Uncovered rows follow
criterion order; their affected Requirements are sorted by Revision key.

The canonical scenario must produce:

- AC-1 through AC-3 covered according to their mapped current Claims;
- AC-2 as a `not-satisfied` finding;
- AC-4 uncovered with `missing_current_claim`; and
- no false `no_requirement_trace` classification for AC-4, because AD-033
  persistently maps REQ-4 to it.

## 5. Canonical bytes and content addresses

### 5.1 Context encoding

Context canonical JSON follows the same rules as capability content:

- one private explicit wire struct with declaration order as field order;
- no maps and no `omitempty`;
- validated strings and identities;
- UTC microsecond time normalization if a future governed field adds a time
  (M.6's selected pack contains no time-dependent value);
- empty slices encoded as `[]`, never `null`;
- deterministic row ordering from §§3–4;
- HTML escaping disabled; and
- no insignificant whitespace or trailing newline.

`ContextDigest` is `engineering.ComputeDigest(canonical context bytes without
the digest field)`. It is rendered in the same lower-case SHA-256 form as every
existing engineering digest.

### 5.2 Proposal value

The Proposal shape is:

```text
Proposal
  Content          CapabilitySpecificationContent
  Rationale        non-empty validated string
  Sources[]        non-empty canonical subset of ContextPack.Sources
  ContextDigest    engineering Digest
  ProposalDigest   digest of the canonical body above, excluding itself
```

The source list is duplicate-free and bytewise sorted. It must include the
current capability Revision reference. Every source must be a member of the
fresh ContextPack source set; a proposal cannot assert an external or inferred
source. If it cites an applicable Decision record, it must also cite that
Decision's paired exact authoritative subject reference.

Canonical proposal JSON embeds capability content in its ordinary canonical
field shape, not as base64 and not as an arbitrary JSON blob. Parsing performs
full content validation, rebuilds canonical bytes, recomputes ProposalDigest,
and rejects an empty rationale, invalid digest, duplicate/unsorted source,
missing current source, or unknown source.

Neither digest is an engineering identity and neither reserves an identity
namespace.

## 6. Deterministic generator

The M.6 `DeterministicGenerator` is deliberately a local demonstration, not a
mock external model. For a valid ContextPack it:

1. starts from the exact current capability content;
2. produces deterministic proposed content without clock, randomness,
   environment, global mutable state or I/O;
3. emits a non-empty rationale that names uncovered criterion keys and
   negative/inconclusive findings in canonical pack order;
4. cites the canonical ContextPack source set it used; and
5. returns a fully validated Proposal bound to the pack's ContextDigest.

The exact same ContextPack must produce byte-identical Proposal canonical JSON
and ProposalDigest across repeated calls and process runs. Generator quality is
not an M.6 acceptance criterion. A generator may conservatively leave the
current specification content unchanged; the demonstration is the authority
boundary, context integrity and reviewed write, not autonomous authorship.

An invalid pack returns an error and no proposal. The generator cannot repair,
drop or refetch an invalid source.

## 7. AcceptCapabilityProposal application contract

### 7.1 Input and static validation

```text
AcceptCapabilityProposal
  ArtifactID     route identity of an existing capability
  RevisionID     caller-controlled identity for the new Revision
  Proposal       complete canonical Proposal value
```

Static validation applies FF-010 identity grammar to ArtifactID and RevisionID,
validates proposal content/rationale/source/digests, and captures exactly one
normalized clock value before `UnitOfWork.Do`. No state-dependent source
membership or freshness conclusion is made outside the transaction.

### 7.2 Occupied target precedence

Inside one UnitOfWork, inspect occupancy for
`P = (ArtifactID, RevisionID)` before reading mutable proposal dependencies.

| Occupancy | Required behavior |
|---|---|
| partial, unreadable, dangling or contradictory | `ErrStoredStateIntegrity`; zero writes |
| coherent foreign-family pair | `ErrImmutableValueConflict`; zero writes |
| complete ordinary non-AI capability Revision | `ErrImmutableValueConflict`; zero writes |
| complete AI-assisted Revision with same canonical proposal witness | replay original result; zero writes |
| complete AI-assisted Revision with different content, ContextDigest, ProposalDigest, sources or origin witness | `ErrImmutableValueConflict`; zero writes |
| absent | continue to freshness and new-act checks |

The complete AI-assisted act is a valid capability Artifact plus the exact
Revision, structured content and one order row, with:

- payload/content/digest/projection/integrity agreement;
- one valid sequence in the artifact's complete managed history;
- `Provenance.Actor == featureforge:local-user`;
- `Provenance.Method == featureforge:ai-assisted`;
- the deterministic known-Origin note described in §7.4; and
- no requirement that the Revision be accepted.

An exact occupied replay returns `201` before freshness. Subsequent Requirement,
Claim or Decision changes do not retroactively invalidate the committed act.

### 7.3 Freshness and new-act validation

Only for an absent target Revision, reassemble the complete ContextPack inside
this same UnitOfWork using §§3–4. Recompute its canonical digest.

Apply these checks in order:

1. proposal ContextDigest differs from fresh ContextDigest ->
   `ErrProposalContextStale`;
2. proposal source is not in the fresh source set, the current Revision source
   is absent, or source ordering/uniqueness is invalid -> `ErrInvalidCommand`;
3. capability owner/history/current Revision is otherwise invalid -> its
   existing governed error;
4. all equal -> allocate `max(sequence)+1`, construct and write the act.

The read, digest comparison, sequence allocation and three writes occur in the
same serializable UnitOfWork. A transaction retry reuses the one candidate
clock instant and reruns the entire fresh read. If the context changes before a
retry, the retry returns stale and commits nothing.

### 7.4 Persisted representation

The new Revision contains the Proposal's exact capability content. Its PEOS
provenance and origin are:

```text
Actor      = featureforge:local-user
Method     = featureforge:ai-assisted
RecordedAt = the one normalized command candidate time
OriginKind = known
OriginNote = featureforge:ai-assisted;proposal=<proposal digest>;
             context=<context digest>;sources=[<canonical refs>]
```

The actual note is one line with no spaces or line breaks, sources in canonical
order, and no locale-dependent formatting. IDs cannot contain its delimiters.
The stored note is the immutable replay witness for proposal/context/source
identity after the transient HTTP value disappears.

The PEOS integration adds the governed `featureforge:ai-assisted` vocabulary
value and a dedicated capability-revision construction path. It does not expose
a generic caller-selected Actor or Method. Revision inspection admits exactly
two provenance forms for capability Revisions:

1. the existing `featureforge:local-user` actor, recorded-at time, no method,
   and existing ordinary Origin rules; or
2. the same local-user actor and recorded-at time plus method
   `featureforge:ai-assisted` and the exact AI-assisted Origin form above.

The second form is invalid for every other Revision/Record family. A local-user
capability Revision with an AI Origin but no method, an AI method with a manual
Origin, any different method, or any source/external-source/extension metadata
is stored-state integrity. No new envelope projection is necessary because the
authoritative payload is decoded before replay classification.

The atomic writes are exactly:

```text
RevisionEnvelope
CapabilitySpecificationContent
RevisionOrderMetadata
```

There is no acceptance append, proposal row, context row or review row. The
result is the existing capability-revision result shape:

```json
{
  "artifact_id": "CAP-1",
  "revision_id": "CAP-1-REV-3",
  "sequence": 3
}
```

## 8. HTTP and no-JavaScript UI

### 8.1 API routes

Add exactly two `/api/v1` routes:

| Intent | Method | Route | Success |
|---|---|---|---:|
| Generate transient proposal | POST | `/api/v1/capabilities/{artifactID}/ai-proposals` | `200 OK` |
| Accept reviewed proposal | POST | `/api/v1/capabilities/{artifactID}/ai-proposals/accept` | `201 Created` |

Generate has no request body. Its success envelope contains the complete
ContextPack and Proposal DTOs, including both digests and all exact sources.
It sends `Cache-Control: no-store`. Repeated generation over equal state
returns byte-equivalent `data` values; ordinary JSON response newline is not
part of either canonical digest.

Accept body is:

```json
{
  "revision_id": "CAP-1-REV-3",
  "proposal": {
    "content": {},
    "rationale": "...",
    "sources": [],
    "context_digest": "...",
    "proposal_digest": "..."
  }
}
```

`content` uses the existing capability-content DTO field shape. Unknown fields,
multiple JSON values, invalid content, invalid digest, malformed sources and
missing RevisionID are `400 invalid_command`. The response uses the existing
success/error envelope and central error mapper.

There is no API reject route. Unsupported `DELETE`, `PUT`, `PATCH` or a guessed
`POST .../reject` remains the existing method/not-found response and cannot
write.

### 8.2 UI routes and review flow

Add:

```text
POST /features/{featureCardID}/ai-proposals
POST /features/{featureCardID}/ai-proposals/accept
```

The first route resolves the card context through the existing UI API client,
sends `POST /api/v1/capabilities/{artifactID}/ai-proposals` to the configured
API handler in-process, and renders its decoded review response containing:

- current Revision identity and complete current/proposed content comparison;
- effective Requirements with exact AD-033 sources;
- current Claims, negative/inconclusive findings;
- applicable Decision outcomes;
- open questions and uncovered criteria;
- rationale, ContextDigest, ProposalDigest and exact sources;
- a required Revision ID field and Accept submit; and
- a Discard link back to the feature/revisions page.

The accept form round-trips the complete canonical Proposal through hidden form
fields (or one canonical JSON field) and posts it to the second UI route. That
route translates the form to the API accept JSON and invokes
`POST /api/v1/capabilities/{artifactID}/ai-proposals/accept` through the same
in-process API handler. The UI never calls an application command or Generator.
The API server never trusts a hidden digest without reparsing and recomputing
the full proposal. Success redirects to the revisions page using
`303 See Other`.

Discard is navigation only. It has no handler, command or transport call. The
proposal disappears when the review response is abandoned. Both proposal UI
responses use `Cache-Control: no-store`. No JavaScript is required.

## 9. Error and zero-write matrix

| Condition | Application result | HTTP/code | Writes |
|---|---|---|---:|
| malformed artifact/revision identity | `ErrInvalidCommand` | `400 invalid_command` | 0 |
| malformed/non-canonical proposal, content, digest or sources | `ErrInvalidCommand` | `400 invalid_command` | 0 |
| requested capability absent, or UI FeatureCard absent | `ErrNotFound` | `404 not_found` | 0 |
| existing capability lacks its required FeatureCard owner/link | `ErrStoredStateIntegrity` | `500 internal_error` opaque | 0 |
| no accepted current capability Revision | `ErrNoAcceptedRevision` | `409 no_accepted_revision` | 0 |
| current Revision or Claim ambiguity | existing sentinel | existing `409` code | 0 |
| fresh ContextDigest differs for an absent target | `ErrProposalContextStale` | `409 proposal_context_stale` | 0 |
| occupied complete target differs | `ErrImmutableValueConflict` | `409 immutable_value_conflict` | 0 |
| coherent foreign-family target occupancy | `ErrImmutableValueConflict` | `409 immutable_value_conflict` | 0 |
| corrupt/partial target or required source state | `ErrStoredStateIntegrity` / unreadable sentinel | `500 internal_error` opaque | 0 |
| generator returns an unexpected error | propagated opaque error | `500 internal_error` opaque | 0 |
| transaction retries exhausted | `ErrTransactionAborted` | `503 transaction_aborted` | 0 |
| valid generate | ContextPack + Proposal | `200` | 0 |
| valid new accept | ordinary Revision result | `201` | exactly R + content + order |
| exact occupied accept replay | original ordinary Revision result | `201` | 0 |

`ErrProposalContextStale` is a new application sentinel and one explicit
central transport mapping. It is not wrapped as immutable value conflict: the
target identity is free, but the reviewed source snapshot is no longer current.
All 500 responses remain opaque.

## 10. Identity, freshness and replay matrix

| Target Revision | Proposal/context | Expected result | Clock/order behavior |
|---|---|---|---|
| absent | exact fresh proposal, caller RevisionID valid | create `201` | consume captured time; allocate next sequence |
| absent | same content but different fresh ContextDigest | `409 proposal_context_stale` | no allocation/write |
| absent | digest self-mismatch | `400 invalid_command` | no UOW write |
| absent | proposal source outside fresh pack | `400 invalid_command` | no allocation/write |
| absent | current pack corrupt | opaque `500` | no allocation/write |
| complete AI target | same RevisionID and canonical proposal | replay `201` | do not consume new time or allocate order |
| complete AI target | same RevisionID, changed proposed content | `409 immutable_value_conflict` | no write |
| complete AI target | same content, changed rationale/sources/digest | `409 immutable_value_conflict` | no write |
| complete AI target | exact proposal but context later changed | replay `201` | occupied act wins before freshness |
| complete manual target | any proposal | `409 immutable_value_conflict` | no write |
| coherent foreign-family target | any proposal | `409 immutable_value_conflict` | no write |
| partial/corrupt target | any proposal | opaque `500` | no write |
| absent after serialization retry | context changed during retry | `409 proposal_context_stale` | retry commits nothing |
| two generate calls, unchanged state | equal ContextDigest/ProposalDigest | two `200` transient values | no clock and no write |
| discard generated proposal | none | UI navigation only | whole store byte-identical |

The ProposalDigest is the transient proposal identity; the RevisionID is the
only new persisted identity. ContextDigest is a freshness witness. None of
them is an idempotency key, acceptance identity or server-generated Revision
identity.

## 11. Required tests

### 11.1 Proposal package

| Test | Proof |
|---|---|
| canonical context round trip | all fields/source refs survive; empty slices are `[]` |
| context digest golden | exact canonical bytes have one stable SHA-256 digest |
| ordering permutation | equivalent unsorted input normalizes to equal bytes/digest |
| invalid/duplicate source | constructor rejects it |
| proposal digest golden | content+rationale+sources+context bind one stable digest |
| proposal mutation table | changing any bound field changes digest |
| defensive copies | caller mutation cannot change pack/proposal |
| deterministic generator | repeated equal pack yields byte-identical proposal |
| no clock/random/I/O | architecture/static test rejects forbidden imports/calls |

### 11.2 Context assembly

| Test | Proof |
|---|---|
| canonical ContextPack | exact current content, four Requirements/traces, Claims, Decision, questions and sources |
| uncovered AC-4 | reason is `missing_current_claim`, affected revision is REQ-4 |
| negative AC-2 | covered, with `not-satisfied` finding |
| no trace | criterion is `no_requirement_trace` |
| trace to old Revision | included honestly but does not cover current criterion |
| several mapped Requirements | one missing Claim makes criterion uncovered |
| inconclusive Claim | covered and surfaced as finding |
| correction | only unique current Claim is included; superseded one remains outside pack |
| applicable Decisions | capability-Artifact, current-Revision and older-Revision subjects are included; each contributes Decision-record + exact-subject sources; another capability is excluded only after validation |
| corrupt hidden projection | global authoritative enumeration fails 500 rather than omission |
| dangling/corrupt trace or Claim support | whole assembly fails; no partial pack |
| one UnitOfWork | every source read belongs to one callback |

### 11.3 Authority boundary

| Test | Proof |
|---|---|
| import allowlist | `internal/proposal` imports only stdlib + engineering |
| no persistence symbols | proposal package contains no repository/UOW/SQL/HTTP/provider dependency |
| closed-before-generate spy | generator observes UnitOfWork callback already returned |
| API-only injection | only API HTTP dependencies receive Generator; UI dependencies do not |
| AD-028 UI imports | `internal/ui` still imports nothing under `internal/` except itself |
| generator error | whole store before/after is byte-identical |
| generate twice | whole store byte-identical and canonical outputs equal |
| discard | no reject route/command and whole store byte-identical |

### 11.4 Acceptance command

Run the complete §10 matrix with an advancing clock. In addition prove:

- the new Revision content equals proposal content exactly;
- sequence is prior maximum plus one;
- provenance actor is exactly `featureforge:local-user` and method is exactly
  `featureforge:ai-assisted`;
- Origin note decodes to exact proposal/context digests and source references;
- a Decision against an older Revision contributes both
  `record:decision/<id>` and that older exact Revision source to ContextPack,
  Proposal and persisted Origin;
- no acceptance record is appended and current accepted Revision is unchanged;
- exact replay makes zero repository writes and returns the original sequence;
- changed proposal under the same target Revision is immutable conflict;
- stale context, including a new Requirement, corrected Claim or applicable
  Decision, returns `proposal_context_stale` with zero writes;
- malformed/corrupt occupied target returns opaque 500 before conflict;
- a serializable retry uses one recorded-at candidate and either commits one
  act or returns stale with no act; and
- rollback after any of the three writes leaves none of them visible.

Run unchanged against memory and PostgreSQL, including transaction rollback,
freshness conflict, persisted actor/method/origin round trip, and replay parity.

### 11.5 HTTP and browser flow

Prove:

- exact route/method/status/body shapes from §8;
- unknown fields, malformed JSON and forged digests are 400;
- stale accept is `409 proposal_context_stale`;
- corrupt source/target state is opaque 500;
- generate and review responses carry `Cache-Control: no-store`;
- generated context exposes exact Requirement trace and Claim/Decision sources;
- UI generate/accept handlers delegate exact method/path/body to the in-process
  API handler and have no Generator/application fallback;
- no unsupported reject/API mutation route exists;
- no-JavaScript browser flow generates, displays, discards with no state, then
  generates again and accepts with caller RevisionID;
- accepted draft appears in revision history with the local-user actor and
  AI-assisted method and is not current until separately accepted through C5;
- existing canonical HTTP/UI scenario remains green.

### 11.6 Repository-wide verification

Required completion commands include formatting, full unit/integration suites,
race tests, vet, build, migration replay from empty PostgreSQL and architecture
guards. No new migration is expected; a migration diff is a review blocker
unless separately governed.

## 12. M.6 exit

M.6 is complete only when all of the following are true:

1. one canonical ContextPack is generated and every element names its exact
   persisted source;
2. AD-033 uncovered semantics are proven, including AC-4 missing a current
   Claim and AC-2 remaining covered but negative;
3. deterministic generation occurs only after the read UnitOfWork closes;
4. architecture tests prove `internal/proposal` has no repository,
   transaction, PEOS, transport, provider or network authority, and prove
   `internal/ui` retains AD-028 with no proposal/application/infrastructure
   import or Generator dependency;
5. generate and discard leave memory and PostgreSQL stores unchanged;
6. stale acceptance is `409 proposal_context_stale` with zero writes;
7. accepted output is one ordinary draft capability Revision with caller-owned
   RevisionID, next order, exact proposal content, local-user actor,
   AI-assisted method, source-bearing Origin and no acceptance append;
8. exact replay is `201` with zero writes under an advancing clock;
9. the API and no-JavaScript review/discard/accept flow pass end to end;
10. memory/PostgreSQL parity, rollback, full repository, race, vet and build
    checks pass;
11. an independent read-only audit reports no BLOCKER or MAJOR finding; and
12. the published commit/tree and workflow evidence are recorded before status
    changes from `Accepted (implementation pending)` to `Implemented`.

Only then may M.7 begin. M.7 is the independent end-to-end PEOS consumer audit
and freeze gate. Authentication, provider-backed generation and the actual
application/login experience begin only after that gate and are not implied by
M.6 completion.
