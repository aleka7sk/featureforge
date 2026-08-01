# AD-032 — Lifecycle policy is persisted and current state is the unique linear head

Status: Accepted
Date: 2026-08-01
Phase: post-M.5 lifecycle closure before M.6

## Context

FeatureForge's lifecycle writer constructs State Assignments and Transition
Record Revisions against a fixed PEOS `DefinitionVersion` assembled in code,
but does not persist the corresponding PEOS `Definition` or
`DefinitionVersion`. That contradicts FF-001's requirement that every PEOS
value used by the canonical scenario persist and reload, and leaves queries
unable to prove the definition version or transition graph named by stored
assignments.

The existing C6 validates a requested transition's target but not its permitted
source. The canonical scenario consequently records `drafting ->
under-validation` with `begin-validation`, although the configured graph
requires `drafting -> specified -> under-validation`. The current query then
selects the greatest `EffectiveAt` projection without decoding assignments,
validating the configured definition version, or proving one coherent history.

PEOS-003 requires a runtime to verify the actual current State is an allowed
source before completing a Transition and to preserve persistent, inspectable
State History. The correction must keep PEOS types inside the integration
package and must not turn FeatureForge into a configurable workflow engine.

## Decision

### 1. The fixed policy is persisted as its own PEOS value families

FeatureForge persists the one fixed `LCD-1` Definition and `LCDV-1` Definition
Version in two PEOS-free carriers:

```text
LifecycleDefinitionEnvelope
  DefinitionID
  canonical Payload
  PayloadDigest

LifecycleDefinitionVersionEnvelope
  DefinitionID
  VersionID
  canonical Payload
  PayloadDigest
  RecordedAt
```

They are not Artifacts or Records and are not disguised as existing envelope
families. Their authoritative payloads are the PEOS JSON values; projections
are checked against decoded payloads. A dedicated repository stores and reads
them in the same UnitOfWork abstraction as every other family and can enumerate
all Definitions as well as all versions under a Definition.

An explicit startup initialization service runs after adapter migration and
before either HTTP handler is served. Both values absent creates the exact
canonical pair atomically. The exact pair already present is a no-op. Partial,
unreadable, contradictory, any extra Definition, or any extra version under
`LCD-1` fails startup as `ErrStoredStateIntegrity`. Global enumeration makes
that invariant observable. C6 and queries never silently create or repair the
configuration.

The byte-exact canonical construction inputs are normative:

- Definition ID `LCD-1`; Definition Version ID `LCDV-1`;
- scope kind/expression `featureforge:capability|*` and the single subject type
  `featureforge:capability-lifecycle`;
- states, in canonical order, with exact meanings:
  - `featureforge:drafting`: `the capability lifecycle was entered for specification work`;
  - `featureforge:specified`: `entry proved an accepted current capability revision and at least one effective requirement traced to it`;
  - `featureforge:under-validation`: `entry proved a current accepted validation plan and at least one completed execution against the same capability revision`;
  - `featureforge:assessed`: `entry proved an applicable current claim for every effective requirement, regardless of claim outcome`;
- initial state `featureforge:drafting`;
- transitions, in canonical order: `enter` drafting→drafting, `specify`
  drafting→specified, `begin-validation` specified→under-validation, and
  `assess` under-validation→assessed; `enter` is the entry transition;
- Definition Version provenance actor `featureforge:local-user` and literal
  `RecordedAt = 2026-01-01T00:00:00Z`.

The ordered inputs above determine the canonical JSON payloads and digests;
changing any of them requires a separately governed Definition Version rather
than silently changing startup equality.

The application obtains only a PEOS-free policy projection from its inspector:
definition/version identity, initial states, entry transition, and each
transition's permitted sources and targets. The adapter never decodes PEOS and
the application never imports it.

### 2. The one FeatureForge lifecycle remains fixed

The persisted policy is:

| State | Meaning |
|---|---|
| `drafting` | specification work is under way |
| `specified` | an accepted capability revision and at least one effective Requirement traced to that exact revision have been recorded |
| `under-validation` | a current accepted validation plan exists and at least one execution of one of its activities has completed against the same current capability revision |
| `assessed` | every effective Requirement for the current capability revision has an applicable current Claim; negative or inconclusive outcomes remain valid assessments and do not imply release readiness |

The graph is:

```text
entry -> drafting
drafting --specify--> specified
specified --begin-validation--> under-validation
under-validation --assess--> assessed
```

These meanings are entry milestones: they state what the serializable C6
transaction proved before completing the transition into the state. They are
not continuously re-evaluated invariants, so an ordinary later revision,
withdrawal or correction does not rewrite lifecycle history.

The entry Transition remains represented by AD-014's content-free Transition
Record Revision because PEOS v1.0.0 cannot construct transition content from an
unassigned pseudo-state. The persisted Definition Version still names the real
entry transition.

FeatureForge does not add user-defined states, transitions, guard expressions,
or a generic evaluator. The three fixed transitions do have the product-owned,
deterministic precondition checks below; they are ordinary application queries,
not a second guard-expression language.

### 3. One validator owns lifecycle history

C6, Q4 and Q5 use the same full-history resolver. It decodes and validates the
persisted Definition/Version, capability subject, transition root, every
StateAssignment and every establishing Transition Record Revision. It proves
payload/digest/projection agreement, definition version, subject, root,
`FromAssignment`, resulting assignment, target state, ownership and times.

Candidate discovery is authoritative-before-filter: the resolver enumerates
all Record and Revision envelopes through deterministic `ListAll`, validates
each payload/digest/projection, and only then selects State Assignments and
Transition Record Revisions for the requested subject. A family-, kind- or
subject-scoped projection query cannot be the first step because a contradictory
projection would silently hide the very corrupt value the resolver must reject.
The existing indexed projection queries remain storage capabilities; no
speculative `ListByFamily` port is part of this decision.

The resulting graph must have:

- exactly one content-free entry revision and initial assignment;
- one predecessor for every non-entry assignment;
- every node reachable from the entry;
- no cycle, disconnected node, branch, second entry, or dangling owner;
- exactly one head.

The current lifecycle state is that unique head. A greatest-timestamp heuristic
and same-state tie-breaking no longer define current state. Exact command replay
does not create a duplicate node.

Any stored wrong version, illegal edge, branch, cycle, dangling reference,
projection contradiction, partial configuration, or invalid time relation is
stored-state corruption and maps to opaque `500 internal_error`.

The common stored-history resolver validates the persisted policy, payloads and
causal graph. It does not claim to reconstruct the application snapshot that
existed when an old transition was attempted: acceptance journal entries have
an effective time but no immutable server append time, so a later backdated
entry makes knowledge-at-attempt observationally unrecoverable. The runtime
therefore enforces product preconditions atomically for every genuinely new C6
act, as PEOS requires before completion; later acts do not cause an old edge to
be reclassified. Persisting a general guard witness is unnecessary for this
bounded contract and would be a separate decision.

### 4. New C6 acts advance only the current head

After FF-022's occupied-act replay classification, a new entry is permitted
only for a subject with no lifecycle history and must name a configured initial
state. A new non-entry act must name the unique current assignment as its
predecessor and the persisted policy must permit its exact transition,
source, and target.

Outcomes are:

| Condition | Result |
|---|---|
| malformed identity, presence, or timestamp | `400 invalid_command` |
| missing or foreign predecessor/reference | `422 referenced_value_missing` |
| unknown transition or illegal source/target/time for a new act | `422 lifecycle_transition_invalid` |
| valid predecessor is no longer the head | `409 lifecycle_head_conflict` |
| occupied identity has different immutable semantics | `409 immutable_value_conflict` |
| persisted policy or history is invalid | `500 internal_error` |

The serializable UnitOfWork prevents two sibling transitions from both
committing. After retry, a loser observes that its predecessor is stale and
returns `lifecycle_head_conflict`.

After structural policy/head validation, a genuinely new transition must also
pass one fixed precondition inside the same UnitOfWork. Every supporting
record's authoritative engineering/provenance time must be no later than the
request's `CompletedAt`:

- `specify`: the current capability Revision is accepted and at least one
  effective Requirement has a coherent `RequirementCriterionTrace` to
  that exact Revision;
- `begin-validation`: one applicable current accepted Validation Plan
	 exists and at least one fully valid Execution completed no later than
	 `CompletedAt`, naming an activity of that exact Plan Revision and the same
	 current capability Revision;
- `assess`: the effective Requirement set is non-empty and every member
	 has an applicable current Claim, with its supporting Execution and Evidence
	 valid and no later than `CompletedAt`. `not-satisfied` and
  `inconclusive` satisfy assessment completeness but retain their readiness
  consequences.

Malformed or corrupt supporting state is 500. A coherent missing or too-late
fact is `422 lifecycle_transition_invalid`. More than one applicable Plan keeps
the existing `409 validation_plan_ambiguous` outcome. These checks occur after
occupied-act/candidate integrity and exact replay, and after a valid
predecessor has been classified as current or stale.

For a new entry:

```text
EffectiveAt <= RecordedAt
```

For a new content-bearing transition:

```text
source.EffectiveAt < resulting.EffectiveAt
source.RecordedAt <= AttemptedAt <= CompletedAt
CompletedAt <= resulting.EffectiveAt <= RecordedAt
```

Times use the governed canonical UTC precision. The strict effective-time
advance makes the linear causal order observable without permitting a
future-effective head for which FeatureForge has no as-of query.

`ErrUnknownDefinitionVersion` is removed from the client-error path: C6 has no
client-supplied definition version. A wrong stored version is integrity failure.

### 5. Read results expose their authority

Q4 returns the selected state, assignment identity, subject, effective time,
Definition ID, Definition Version ID, and exact establishing Transition Record
Revision. Q5 validates the same complete history before returning any lifecycle
event and includes the assignment, establishing revision, definition-version
reference, actor, state, and effective time. Truly absent history remains a
successful `found=false`; any dangling root/revision/assignment is not absence.

## Canonical scenario correction

The one transition root becomes the legal chain:

```text
TR-1/REV-0 -> SA-1 drafting
TR-1/REV-1 -> SA-2 specified
TR-1/REV-2 -> SA-3 under-validation
```

After requirements, the decision, and accepted capability Revision 2 are
recorded, `specify` establishes `SA-2`. The accepted validation plan is then
recorded. After the first validation execution/evidence act, `begin-validation`
establishes `SA-3`; remaining executions, claims, and correction continue from
there. This closes the prior illegal edge and makes the stored sequence agree
with the stated meaning of `under-validation`.

No historical assignment is rewritten. FeatureForge has no deployed durable
database, so fixtures and ephemeral stores are recreated. If a durable store
with the old illegal history is ever encountered, startup fails; repair would
require a separately governed offline migration or new Definition Version.

## Consequences

- PostgreSQL gains additive lifecycle Definition and Definition Version tables;
  memory gains equivalent transactional collections.
- The shared repository contract covers exact put/get/list, conflict,
  rollback, deterministic order, and byte-preserving payload round trip.
- Lifecycle codec, bootstrap, legal-chain, stale-head, corruption,
  concurrency, Q4/Q5, HTTP/UI, and canonical-scenario tests become required.
- No PEOS module change, generic workflow engine, data backfill, or operational
  application/authentication work is introduced.
