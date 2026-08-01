# FF-023 — Domain and lifecycle conformance closure

Status: Implemented (domain and lifecycle conformance closure)
Date: 2026-08-01
Phase: post-M.5 closure before M.6
Governs: AD-031/AD-032/AD-033 implementation, adapter parity, structured
Requirement-criterion traceability, persisted lifecycle policy, linear
lifecycle history, and the corrected canonical scenario.

## 1. Purpose and boundary

FF-022 closed command replay and aggregate integrity. The remaining pre-M.6
audit found three bounded drifts:

1. M.1 described editable operational values while the implemented POC exposes
   stable Project/FeatureCard establishment plus one monotonic capability link.
2. lifecycle Definition/DefinitionVersion were used but not persisted, C6 did
   not validate transition sources, and the canonical scenario skipped the
   configured `specified` state.
3. no persisted value linked a Requirement Revision to the exact
   revision-local capability acceptance criterion the UI and M.6 must report.

AD-031, AD-032 and AD-033 resolve those choices. This packet implements only
their conformance work. It does not add a generic workflow engine, operational
edit surface, authentication, organization/user model, external AI call, or
Belcanto application code.

The PEOS v1.0.0 module remains unchanged and no `replace` directive is added.

## 2. Operational establishment and adapter parity

No public command, DTO, route, UI form, domain field, or migration is added for
Project/FeatureCard mutability.

`ProjectRepository.Put` and `FeatureCardRepository.Put` compare only their base
establishment values. The capability link is a separate monotonic value:

| Operation | Required result |
|---|---|
| base FeatureCard `Put`, then `Get` | exact base value, no link |
| `LinkCapability` absent -> A | success; `Get`/`ListByProject` materialize A |
| `LinkCapability` A -> A | success no-op |
| `LinkCapability` A -> B | `ErrCapabilityAlreadyLinked` |
| identical base `Put` after link | success no-op; link remains A |
| different base `Put` after link | `ErrImmutableValueConflict`; link remains A |
| link in rolled-back UnitOfWork | no link survives |

The shared contract suite runs this matrix unchanged against memory and
PostgreSQL. Comments and architecture guards distinguish stable operational
establishment from immutable engineering rows.

## 3. Requirement criterion trace

`internal/engineering` adds `RequirementCriterionTrace`, keyed by the exact
Requirement `RevisionKey`, with exact source capability `RevisionKey`,
revision-local acceptance-criterion key and recorded-at time. Its constructor
requires non-zero identities/key/time. Equality includes every field.

`RequirementCriterionTraceRepository` supplies create-only `Put` and exact
`Get`; it is added to `Repositories`, the memory transaction state, PostgreSQL
repositories, and the common contract suite. Migration 0003 adds:

```sql
requirement_criterion_traces(
  requirement_artifact_id text not null,
  requirement_revision_id text not null,
  capability_artifact_id text not null,
  capability_revision_id text not null,
  acceptance_criterion_key text not null,
  recorded_at timestamptz not null,
  primary key (requirement_artifact_id, requirement_revision_id),
  foreign key (...) references revisions(...),
  foreign key (...) references revisions(...)
)
```

C7 adds `source_capability_revision_id` and
`source_acceptance_criterion_key`. For a new act it proves that the exact source
is the accepted current capability Revision and that its authoritative
structured content contains the key. It writes the trace atomically with R/O/M.
Replay compares it; any occupied Requirement revision with a missing, dangling,
wrong-family, time-disagreeing, or non-existent-criterion trace is a partial
aggregate and returns opaque 500.

For a genuinely new pair, a missing, foreign, stale or wrong-family source and
an absent criterion key are `422 referenced_value_missing`; unreadable or
projection/digest-contradictory source state is opaque 500. For an occupied
complete pair, exact stored-trace mismatch is 409 after corrupt trace/source
occupancy has received its required 500 precedence.

The canonical scenario creates all four Requirements after Revision 2 is
accepted and maps REQ-1..REQ-4 to AC-1..AC-4 on that exact Revision. No mapping
is inferred from statement text, key coincidence, Origin note, or Artifact
subject.

## 4. Lifecycle persistence model

### 4.1 PEOS-free carriers

`internal/engineering` adds:

```text
LifecycleDefinitionEnvelope
  DefinitionID string
  Payload []byte
  PayloadDigest Digest

LifecycleDefinitionVersionKey
  DefinitionID string
  VersionID string

LifecycleDefinitionVersionEnvelope
  Key LifecycleDefinitionVersionKey
  Payload []byte
  PayloadDigest Digest
  RecordedAt time.Time
```

Constructors reject empty identities, invalid JSON, zero/mismatched digests,
and zero version time. `Equal` means equal identity plus byte-identical payload;
projections never override payload authority. Returned payload slices are
copies.

A PEOS-free `LifecyclePolicy` projection contains only the facts application
policy needs: configured definition/version IDs, initial state IDs, entry
transition ID, and transition ID/source/target sets. It is an inspected read
projection like existing decision and plan projections, not a copied PEOS type.

### 4.2 Ports

`LifecycleDefinitionRepository` supplies:

```text
PutDefinition
GetDefinition
ListDefinitions
PutVersion
GetVersion
ListVersions
```

It is added to `Repositories`. `EngineeringRecorder` constructs the exact fixed
Definition/Version pair. `EngineeringReplayInspector` supplies the configured
key, validates and projects the stored pair, and returns authoritative
assignment/transition details needed by the common history resolver. No port
returns a PEOS type.

### 4.3 Adapters

Memory stores definition and version envelopes in separate transaction-local
maps, deep-copying payloads on ingress, clone, merge, and egress.

Migration `0003_domain_and_lifecycle_conformance.sql` adds these tables together
with §3's trace table:

```sql
lifecycle_definitions(
  definition_id text primary key,
  payload bytea not null,
  payload_digest text not null
)

lifecycle_definition_versions(
  definition_id text not null references lifecycle_definitions(definition_id),
  version_id text not null,
  payload bytea not null,
  payload_digest text not null,
  recorded_at timestamptz not null,
  primary key (definition_id, version_id)
)
```

There is no `UPDATE`, `DELETE`, PEOS-shaped column, JSON reconstruction, or
retroactive grammar check. Repository `Put` uses insert-or-exact-readback and
the existing typed error taxonomy.

## 5. Explicit startup initialization

`EnsureLifecycleConfiguration` is an application service with one UnitOfWork:

1. obtain the fixed canonical pair from `EngineeringRecorder`;
2. list Definitions globally, read both expected identities and list versions
   for `LCD-1`;
3. the repository entirely empty: write Definition then Version;
4. both present: fully inspect and require exact canonical equality;
5. any partial, additional definition or configured version, unreadable payload,
   projection/digest mismatch, or semantic difference: `ErrStoredStateIntegrity`.

The composition root calls this service after migrations and before building
or serving handlers. Canonical-scenario and adapter fixtures call it explicitly.
C6 and read queries require the initialized pair and never create it.

The exact canonical construction inputs (IDs, ordered states and meanings,
ordered graph, scope, subject type, provenance actor, and literal
`2026-01-01T00:00:00Z` configuration time) are those fixed normatively by
AD-032; an implementation may not choose different bytes.

## 6. Common lifecycle history resolution

One application resolver is shared by C6, Q4 and Q5. For a capability Artifact
it:

1. loads and inspects `LCD-1/LCDV-1`;
2. validates the complete capability Artifact history;
3. enumerates all Record envelopes globally, validates every authoritative
   payload/digest/projection, and only then selects State Assignments and
   filters by subject;
4. enumerates all Revision envelopes globally, validates every authoritative
   payload/digest/projection, and only then selects Transition Record Revisions,
   filters by subject, and discovers every establishing Revision and owning
   root;
5. verifies payload/digest/projection agreement, one root, ownership,
   definition/version, subject, predecessor, resulting assignment, target,
   state, and canonical times;
6. validates entry membership and every content-bearing edge against the
   persisted policy;
7. builds the predecessor graph and requires one entry, reachability, no cycle,
   no branch, no disconnected node, and one head.

It does not reconstruct historical product-precondition results. Acceptance
journal rows lack immutable server append time, so later backdated entries make
knowledge-at-attempt unrecoverable. Product preconditions are instead enforced
atomically for every genuinely new C6 act; stored history validation remains
authoritative for policy, payload and graph integrity.

No assignment plus no transition/root occupancy returns a legitimate absent
history. Any partial occupancy returns `ErrStoredStateIntegrity`.

The resolver returns a PEOS-free history with its unique current assignment,
configured identity, establishing revision, root and causal order. It does not
write or repair state.

## 7. C6 new-act rules and precedence

Static identity/presence/timestamp-shape validation remains before the
UnitOfWork. Occupied revision/assignment candidate integrity and exact replay
remain governed by AD-030/FF-022 and happen before treating the request as a new
transition.

For a genuinely new entry:

- history must be absent;
- `State` must be configured initial state `drafting`;
- transition/from fields and attempted/completed times must be absent;
- `EffectiveAt <= RecordedAt`.

For a genuinely new content-bearing transition:

- the decoded predecessor must exist under the same capability/root;
- it must be the resolver's unique head;
- the stored policy must contain the requested transition and permit the exact
  predecessor-state -> requested-state edge;
- `source.EffectiveAt < resulting.EffectiveAt`;
- `source.RecordedAt <= AttemptedAt <= CompletedAt`;
- `CompletedAt <= resulting.EffectiveAt <= RecordedAt`.

After those structural checks, the exact product precondition is evaluated in
the same UnitOfWork. Each supporting value's authoritative time must be no later
than `CompletedAt`. These are entry milestones, not continuously re-evaluated
state invariants:

| Transition | Mandatory fact |
|---|---|
| `specify` | one accepted current capability Revision and at least one effective Requirement coherently traced to that exact Revision |
| `begin-validation` | one current accepted Plan and at least one fully valid Execution of its exact activity against the same current capability Revision |
| `assess` | a non-empty effective Requirement set in which every member has an applicable current Claim with valid supporting Execution/Evidence; negative and inconclusive outcomes are permitted |

Malformed or corrupt supporting state is 500; a coherent missing or too-late
fact is `422 lifecycle_transition_invalid`; multiple applicable Plans retain
the existing 409 ambiguity result. Occupied candidate integrity is classified
first. An exact local replay succeeds only after the persisted configuration
and whole subject history pass the common resolver; a coherent replay of an old
non-head act is 201, while any branch, cycle or wrong version is 500. New-head,
edge and product-precondition checks apply only to a genuinely new act.

The application adds:

```text
ErrLifecycleTransitionInvalid -> 422 lifecycle_transition_invalid
ErrLifecycleHeadConflict      -> 409 lifecycle_head_conflict
```

Wrong version or illegal transition in stored history is
`ErrStoredStateIntegrity -> 500 internal_error`, never the old client-facing
`ErrUnknownDefinitionVersion`. All replay/error branches write nothing.

## 8. Q4, Q5 and transport projection

`ResolveLifecycleState` takes the inspector and uses the common resolver. Its
found result contains:

- state ID;
- assignment ID and subject;
- effective time;
- Definition ID and Definition Version ID;
- establishing Transition Record Artifact/Revision IDs;
- rationale naming the unique-head rule and chain size.

The Q4 HTTP DTO and feature overview render the definition version and
establishing revision rather than only a state label.

Timeline assembly receives the inspector, validates the same lifecycle history
before emitting any lifecycle event, and preserves causal chain order. Each
event references its assignment, establishing revision and definition version;
actor comes from validated provenance. Corrupt lifecycle history fails the
whole Q5 with opaque 500 rather than returning a partial timeline.

Q5 discovers Evidence as the deduplicated union of exact citations from every
validated Decision plus the history-wide Execution and Claim population. Each
selected record is inspected before event emission. A governed C8 citation
whose exact Evidence Artifact and Revision are both absent remains a pending
reference on the readable Decision event and produces no invented Evidence
event. A partially occupied or contradictory C8 pair, and dangling
Execution/Claim evidence, are opaque stored-state integrity (`500`) because the
latter references are mandatory. No corrupt condition may silently omit an
event or return a partial timeline.

## 9. Corrected canonical scenario

Fixtures add:

```text
SpecifiedAssignmentID       = SA-2
SpecifiedTransitionRevision = TR-1-REV-1
ValidationAssignmentID      = SA-3
ValidationTransitionRevision= TR-1-REV-2
```

The ordered chain is:

1. entry `SA-1/drafting`;
2. decision, capability Revision 2 and its acceptance;
3. four traced Requirements mapped to Revision 2's AC-1..AC-4;
4. `specify`, `SA-1 -> SA-2/specified`;
5. accepted Validation Plan;
6. first validation execution/evidence;
7. `begin-validation`, `SA-2 -> SA-3/under-validation`;
8. remaining executions/claims and correction.

The permuted scenario may choose a different first independent activity, but
always records exactly one completed execution before `begin-validation` and
then processes the remaining activities. Resolved engineering state remains
identical.

FF-011's expected inventory and FF-006's worked timeline are updated to the
new legal chain. No `assessed` act is added merely to make the scenario look
complete: lifecycle remains independent of the deliberately `not-ready`
readiness result.

## 10. Required proof

### Codec and repository

- Definition and DefinitionVersion canonical round trip and projection
  fidelity;
- both adapter contract suites: put/get/list, exact no-op, conflict,
  rollback, deterministic list, defensive copies;
- migration idempotence and byte-preserving PostgreSQL reload;
- AD-031 link/base-Put parity matrix;
- RequirementCriterionTrace put/get, exact no-op, conflict, rollback, foreign
  keys and adapter parity.
- `RevisionEnvelopeRepository.ListAll` and
  `RecordEnvelopeRepository.ListAll` expose deterministic complete enumeration
  in both adapters, including same-transaction visibility, rollback and
  defensive-copy guarantees; lifecycle and Q3/Q4/Q5 discovery must validate
  every envelope before any untrusted family/kind/subject projection filters
  candidates. No unused `ListByFamily` port remains.

### Initialization and commands

- bootstrap absent, exact replay, partial occupancy, extra version, corrupt
  payload and atomic rollback;
- entry and every legal graph edge;
- unknown transition, illegal source/target/time;
- each transition product precondition, too-late support, and stability of old
  graph resolution after later acts;
- stale head and competing sibling transitions;
- exact C6 replay with advancing clock and zero writes;
- C7 exact source/current criterion validation, trace replay, missing/corrupt
  trace fail-loud behavior, and zero writes on every error.

### Stored integrity and reads

- wrong definition/version/root/subject/resulting assignment/target;
- branch, cycle, disconnected node, multiple entry, dangling transition or
  assignment owner;
- Q4 fields/rationale and absent state;
- Q4/Q5 opaque 500 on each representative corruption;
- causal timeline event order and references.

### End to end

- corrected scenario in memory and PostgreSQL;
- replay and insertion-order variants;
- HTTP scenario;
- no-JavaScript UI/browser scenario;
- full formatting, vet, build, unit, PostgreSQL and race checks.

## 11. Documentation and completion evidence

Direct contradictions are corrected forward in FF-001/002/003/004/005/006/
007/009/010/011/012/014/015/018/022, the glossary, README and decision log.
Historical reports remain unchanged and are cited as historical evidence.

Implementation was closed with the following evidence:

| Evidence | Verified result |
|---|---|
| Locally audited implementation commit | `b99a1bda558dd94df96b813677619d2f0431681c` |
| Published implementation commit | `7193084ccae7cf8bc2c9c724852675e20792f473` |
| Exact tree shared by both commits | `1774b5929b6a0c6f94312478e90ab2324871f883` |
| Canonical GitHub workflow | [Verify run 30683706791](https://github.com/aleka7sk/featureforge/actions/runs/30683706791): `success`, including formatting, vet, build, PostgreSQL tests and PostgreSQL race tests |
| Independent read-only audit | `READY`; no BLOCKER or MAJOR finding |

The workflow first exposed two adapter/fixture defects rather than hiding
them: the PostgreSQL timeline-history fixture did not initialize the governed
lifecycle pair, and PostgreSQL trace insertion could skip foreign-key
validation on the `ON CONFLICT` path. Both were corrected, covered by the
shared contracts, independently re-audited and then proven by the successful
workflow above.

The completion gate required:

1. all proof above passes locally or in the canonical GitHub workflow;
2. an independent read-only reviewer reports no BLOCKER or MAJOR finding;
3. the exact published commit and tree are verified;
4. this document records those hashes, workflow evidence and review result;
5. status changes to `Implemented`.

All five conditions are met. M.6 may begin from this recorded baseline.
