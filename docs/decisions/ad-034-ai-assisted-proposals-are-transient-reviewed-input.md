# AD-034 — AI-assisted proposals are transient reviewed input

Status: Accepted
Date: 2026-08-01
Phase: M.6 AI context-pack demonstration

## Context

FF-001 and AD-012 require FeatureForge to demonstrate an AI boundary without
giving an AI component authority over engineering state. The pre-M.6 closure
now makes the required context computable: current capability content is
authoritative, effective Requirements carry AD-033's exact
Requirement-to-criterion trace, current Claims are resolved through correction
history, and applicable Decisions can be decoded with exact identities.

The remaining architectural choice is where a proposal lives and how explicit
review becomes an ordinary capability Revision without introducing a second
engineering store, a hidden write path, or an unverifiable snapshot. A proposal
cannot be treated as a draft Revision: draft is still persisted PEOS state. It
also cannot be accepted merely because a client returns plausible content;
FeatureForge must prove that the exact context reviewed by the user is still
current at the write boundary.

The POC has no authenticated identity model. It retains FeatureForge's fixed
local human actor and uses PEOS Provenance's method field to distinguish the
AI-assisted review path. The explicit accept act remains the human-review
boundary.

## Decision

### 1. The proposal core is a pure, inward dependency

`internal/proposal` may import only the Go standard library and
`internal/engineering`. It owns immutable `ContextPack`, `Proposal`, canonical
encoding/digest helpers, and this interface:

```text
Generator.Generate(ContextPack) -> Proposal
```

The generator receives exactly one value. It receives no `context.Context`,
repository, UnitOfWork, clock, application service, transport client, PEOS
value, provider SDK, environment accessor, or callback. The M.6 implementation
is deterministic and local: equal canonical context bytes produce equal
canonical proposal bytes and digests.

Application code assembles a complete ContextPack in one read-only UnitOfWork,
closes that UnitOfWork, and only then invokes the generator. The package cannot
read or write engineering state even accidentally.

### 2. Context is exact, complete, and fail-loud

One ContextPack is rooted at the accepted current capability Revision and
contains:

- that exact Revision key and its complete decoded
  `CapabilitySpecificationContent`;
- every effective Requirement Revision, statement, and AD-033 trace naming its
  exact source capability Revision and revision-local criterion key;
- every applicable current Claim, including exact Record identity, criteria,
  outcome and reasoning;
- every FF-004-applicable Decision whose exact authoritative subject is the
  capability Artifact or any of its Revisions, with its identity and outcome
  statement;
- every open question from the exact current capability content;
- every uncovered current acceptance criterion; and
- validation findings for mapped Requirements whose current Claim is
  `not-satisfied` or `inconclusive`.

Every element names its exact persisted source. The application uses the
existing authoritative-before-filter inspectors and current-state resolvers.
Unreadable, dangling, contradictory, mixed-family or ambiguous state fails the
whole assembly; the pack never silently omits it.

Each applicable Decision contributes two source witnesses: its exact Decision
record reference and its exact authoritative subject reference. For a Decision
about an older capability Revision, that older Revision reference remains in
the pack and later in the accepted Origin; the current Revision reference is
not substituted for it.

For a criterion on the exact current capability Revision, **uncovered** means:

1. no effective Requirement trace names that exact Revision/key; or
2. at least one mapped effective Requirement has no applicable current Claim.

A `not-satisfied` or `inconclusive` Claim is coverage, not absence. It is
included as a finding so the proposal cannot mistake negative evidence for
missing traceability.

### 3. Context and proposal identities are content addresses, not records

The ContextPack has deterministic canonical JSON and
`ContextDigest = SHA-256(context bytes)`. A Proposal contains:

- proposed `CapabilitySpecificationContent` in the ordinary revision shape;
- a non-empty rationale;
- a canonical, duplicate-free list of exact source references drawn from the
  ContextPack;
- the ContextDigest; and
- `ProposalDigest = SHA-256(canonical proposal body)`.

The proposal digest binds the proposed content, rationale, ordered sources and
ContextDigest. Neither digest is a caller-generated engineering identity. No
proposal, context pack, digest, review decision or rejection is written to a
repository, database table, session, cache, file, queue, PEOS value or
background job. They may exist only in process memory, an HTTP response and the
review form that returns the canonical proposal.

### 4. Acceptance creates one ordinary draft capability Revision

Acceptance takes the existing capability Artifact identity from the route and
requires two caller-controlled values:

```text
revision_id
canonical proposal
```

For a genuinely new target Revision, the application opens one UnitOfWork,
reassembles the complete current ContextPack, recomputes its digest, and
requires exact equality with the proposal's ContextDigest before writing. A
different digest is `409 proposal_context_stale` with zero writes. This check
and the write share the same serializable transaction, so no source act can
change between validation and commit.

The successful write is exactly the ordinary capability-revision act:

```text
Capability Revision
+ CapabilitySpecificationContent
+ RevisionOrderMetadata(max + 1)
```

It appends no acceptance record. The new Revision is therefore draft by
absence and does not become the accepted current Revision until the existing C5
flow explicitly accepts it.

The Revision uses:

```text
Provenance.Actor  = "featureforge:local-user"
Provenance.Method = "featureforge:ai-assisted"
```

and a deterministic known-Origin note containing the ProposalDigest,
ContextDigest and canonical exact source references. The actor is the existing
fixed POC human actor; the method is the observable marker that the actor chose
the explicit AI-assisted acceptance path. This exactly implements AD-012's
provenance requirement. No proposal-specific envelope, relation, record family
or persistence schema is introduced.

### 5. Replay follows the persisted act before freshness

The caller-supplied `revision_id` is the primary identity. FF-022 precedence is
preserved:

| Target occupancy and request | Outcome | Writes |
|---|---|---:|
| absent target; fresh canonical proposal | create ordinary draft Revision, `201` | R + content + order |
| absent target; stale ContextDigest | `409 proposal_context_stale` | 0 |
| complete AI-assisted target; same canonical proposal and identity | replay original `201` result | 0 |
| complete target; changed proposal or non-AI target semantics | `409 immutable_value_conflict` | 0 |
| partial, unreadable, dangling or contradictory target | opaque `500 internal_error` | 0 |

An exact occupied replay is recovered before context freshness. Later changes
to Requirements, Claims or Decisions cannot make an already committed act stop
being idempotent. Freshness applies only when the requested target Revision is
genuinely absent.

### 6. Generate and accept are the only server actions

M.6 adds one HTTP generate endpoint and one HTTP accept endpoint. Generate
returns a transient ContextPack and Proposal; accept round-trips the canonical
Proposal and caller-provided Revision ID. Reject is only a UI discard action:
leaving the review page drops the values and invokes no command or endpoint.

Only the API HTTP application's dependency set receives the Generator. The UI
retains AD-028's in-process HTTP/API boundary: it invokes those endpoints
through its existing API client and imports no proposal, application,
engineering or infrastructure package. It never constructs or receives a
Generator directly.

There is no authentication, authorization, AI provider, model call, network
client, prompt persistence, proposal persistence, async processing, streaming,
tool use, or provider configuration in M.6.

## Alternatives

**Persist proposals as draft revisions.** Rejected. Draft is an authoritative
acceptance state by absence, not a private AI scratch space, and would violate
AD-012's no-write boundary.

**Persist proposals in a separate table.** Rejected. It adds a second product
workflow and retention policy when a content-addressed round trip is sufficient
for the POC.

**Let the generator receive repositories or an application callback.**
Rejected. An interface name does not establish an authority boundary if its
implementation can still access mutable state.

**Accept by ProposalDigest alone.** Rejected. A digest is not recoverable
content; the server must receive and validate the complete canonical proposal
it is about to record.

**Always rerun freshness checks on exact replay.** Rejected. It would make a
successfully committed command cease to replay after unrelated later context
changes and would contradict FF-022's occupied-act precedence.

**Put `featureforge:ai-assisted` in the actor field.** Rejected. It would erase
the fixed human actor and misuse identity to encode a process. PEOS Provenance
already has a method field for exactly this distinction.

**Keep the local actor but omit the method.** Rejected. An Origin note alone is
not the governed provenance marker promised by FF-001 and AD-012.

## Consequences

- AI quality is explicitly not being evaluated. The deterministic local
  generator demonstrates context completeness and authority separation.
- Proposal acceptance is stale-safe without persisting a context snapshot:
  exact canonical bytes and their digest are recomputed in the write
  transaction.
- Rejection is proven by whole-store equality because there is no reject write
  path.
- Accepted output participates in all existing capability history, replay,
  ordering, inspection and adapter contracts.
- The inspector permits `featureforge:ai-assisted` only as the Provenance
  method of a capability Revision carrying the exact AI-assisted Origin form;
  every existing time-only local-user provenance rule remains unchanged.
- The architecture test can prove the import boundary mechanically, while
  spies prove the generator is invoked only after the UnitOfWork callback has
  returned.
- AD-028 remains intact: UI boundary tests prove proposal generation and
  acceptance are delegated to the API handler, not reimplemented or directly
  injected into `internal/ui`.
- Authentication, provider integration, proposal retention, editing,
  collaboration and the application login surface remain outside M.6.
