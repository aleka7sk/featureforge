# AD-030 — Command idempotency is recovered from validated persisted acts

Status: Accepted
Date: 2026-08-01
Phase: M.5 correctness closure before domain analysis
Supersedes: the replay mechanism asserted by AD-029 and FF-018 §11; the
Validation Plan narrowing asserted by FF-018 §6.6 and FF-020 §6

**Forward amendment (AD-033, FF-023).** A complete C7 Requirement revision act
also includes one immutable `RequirementCriterionTrace` to an exact capability
Revision and acceptance-criterion key. Missing or contradictory trace occupancy
is partial state (`500`); the two source fields participate in replay semantics.
No historical trace is inferred from subject, statement text, or Origin note.

## Context

[FF-010 §1](../spec/010-application-contracts.md#1-identity-strategy)
requires an exact command retry to be a no-op and an immutable mismatch to be
a conflict. The implementation evidence previously cited for that guarantee
proved only that an identical value can be put into a repository twice. It did
not prove that a command can be reconstructed twice across a real lost-response
retry.

That distinction is material. Every command records at least one value that
contains server-owned time, generated order, or provenance. A later execution
can therefore construct different bytes from the same request. Three commands
also fail before repository equality can help:

- C4 computes `max(sequence)+1` again, so a replay proposes a new sequence;
- C5 validates the journal transition before its idempotent append, so a replay
  of an accepted entry observes `accepted -> accepted` and is rejected;
- C7 computes order again and historically derived an acceptance identity with
  `"ACC-" + artifact_id + "-" + revision_id`.

The C7 formula was neither reserved by a governing contract nor injective. For
example, `(artifact_id = "X-Y", revision_id = "Z")` and
`(artifact_id = "X", revision_id = "Y-Z")` both produce `ACC-X-Y-Z`.
The repository stores no command-origin marker from which the historical
writer can be recovered.

A second inconsistency affects C9. [FF-004 §2](../spec/004-current-state-resolution.md#2-the-revision-ordering-contract)
applies the same ordering and current-state policy to Validation Plan
revisions, but C9 currently records only an Artifact and Revision. FF-018 §6.6
and FF-020 §6 later treated the missing order and acceptance support as a model
property. That was an implementation drift, not a valid narrowing of FF-004.

The correction must preserve four boundaries:

1. PEOS payloads remain authoritative engineering values.
2. Repository equality remains the create-only adapter contract established by
   AD-026; adapters do not decode PEOS.
3. The HTTP surface remains twelve commands and seven queries.
4. No durable FeatureForge database exists, and the current schema already
   stores every value this decision needs.

## Decision

### 1. A command retry is recognized before reconstruction

Each command defines one persisted **semantic act**: the complete set of
immutable values whose atomic presence is the command's successful
postcondition. An Artifact shared by several revisions may be a prerequisite
or owning root without being a local member of every revision act.

After static request validation, the application captures at most one
normalized candidate time before `UnitOfWork.Do`, so a PostgreSQL callback
retry cannot observe another instant. Inside the `UnitOfWork`, it first
inspects the identities named by the request and classifies the stored act.
It does this before consuming that candidate time, allocating a new sequence,
validating a transition as though it were new, or constructing a new
time-bearing envelope. Replay ignores the candidate and performs no write.

The classification is:

- **absent** — no local member of the act exists;
- **complete** — every required member exists, decodes, agrees with its
  projections and references, and satisfies the act's structural invariants;
- **coherent foreign occupancy** — the identity is occupied by a complete
  value of a different immutable act or family;
- **partial or contradictory** — some required member exists but the complete
  postcondition cannot be proven, a payload is unreadable, a projection
  contradicts its payload, a required reference is dangling, or a journal is
  structurally invalid.

Repository presence alone is not proof of completeness. Before comparing
request semantics, the application decodes every authoritative payload the act
depends on and verifies its key, type, family, subject, canonical digest,
embedded integrity, projections, and required cross-object references.

### 2. One outcome precedence governs C1–C12

The following precedence is binding:

1. A statically invalid request returns `400 invalid_command`, with zero
   writes.
2. Partial, unreadable, dangling, or contradictory persisted occupancy returns
   `500 internal_error`, with zero writes.
3. A complete expected act whose stored request semantics equal the request is
   an exact replay: return `201 Created` and the original response body, with
   zero writes.
4. A complete expected act whose request semantics differ, or a coherent
   foreign occupant, returns `409 immutable_value_conflict`, with zero writes.
5. An absent act with all required, valid, free identities may be created
   atomically. Ordinary missing command references and illegal transitions
   retain their governed `422` mappings.

When a request names a different secondary or member identity, corrupt
occupancy of that named candidate is an integrity failure and therefore takes
precedence over the ordinary immutable conflict.

All error and replay branches commit zero writes.

### 3. Request semantics exclude generated representation details

Request-semantic equality compares immutable caller intent, not a newly
constructed representation. The following server-owned values are excluded
from replay equality but remain subject to integrity validation:

- creation and envelope `RecordedAt` values;
- provenance recorded by the server;
- generated revision sequence and order-record time;
- fixed acceptance actor, reason, and effective time created by C7 and C9.

For a command field that the caller may either supply or omit and allow the
clock to default, presence is semantic:

- a supplied value must equal the stored normalized value;
- an omitted value recovers the stored default; the captured candidate clock
  value is irrelevant on replay.

This applies to the defaultable times in C5, C6, C10, C11, and C12. A changed
explicit caller value remains an immutable conflict.

The application may compare recomputed canonical bytes or a canonical digest
only after decoding the payload and proving that stored payload, digest,
integrity, key, type, family, subject, and projections agree. A projection
tuple by itself is not an authoritative equality witness.

### 4. Command-act identities and required members

| Command | Caller-controlled act identity | Complete persisted act |
|---|---|---|
| C1 `CreateProject` | `project_id` | Project |
| C2 `CreateFeature` | `feature_card_id` | FeatureCard with its owning Project reference valid |
| C3 `EstablishCapabilitySpecification` | `artifact_id`, `revision_id` | capability Artifact, founding Revision, structured content, sequence-1 order metadata, FeatureCard link |
| C4 `ReviseCapabilitySpecification` | `artifact_id`, `revision_id` | Revision, structured content, its original order metadata under a valid capability Artifact |
| C5 `AcceptCapabilityRevision` | `record_id` | one valid acceptance journal record under the named Revision |
| C6 `AssignLifecycleState` | `assignment_id`, transition Artifact/revision pair | shared Transition Artifact, transition Revision, StateAssignment and required predecessor references |
| C7 `EstablishRequirement` | Requirement pair plus `acceptance_record_id` and exact source revision/criterion for a new act | shared Requirement Artifact, Revision, order metadata, semantic acceptance member and RequirementCriterionTrace |
| C8 `RecordArchitectureDecision` | `decision_id` | Decision record and a complete capability subject; its evidence citation must be structurally valid but may be unresolved |
| C9 `EstablishValidationPlan` | Validation Plan pair plus `acceptance_record_id` | shared Validation Plan Artifact, Revision, order metadata and semantic acceptance member |
| C10 `RecordValidationRun` | `execution_id`, evidence Artifact/revision pair | Evidence Artifact, Evidence Revision and Execution record |
| C11 `RecordValidationClaim` | `claim_id` | Claim record and all required references |
| C12 `CorrectValidationClaim` | `claim_id` in the same Claim namespace as C11 | correcting Claim, valid target and valid correction chain |

This table corrects FF-018 §11.1 where C10 was described as identified by only
`execution_id` and C7/C9 omitted the identity of the acceptance record their
completed acts create.

C8 records an engineering citation, not an Evidence act. It validates the
Decision payload/projections and the complete capability subject. Its
`evidence_artifact_id` and `evidence_revision_id` must form the canonical
Evidence key, but C8 neither creates nor requires that pair to exist. If such a
readable Evidence A/R pair later occupies the same identity without a C10
Execution owner, a complete Decision citation makes it coherent foreign
occupancy for C10 (`409`); an uncited orphan or corrupt pair is integrity
failure (`500`).

**FF-024 subject-form clarification.** “Complete capability subject” includes
both FF-004-governed forms: the capability Artifact and one exact capability
Revision. C8 always requires `subject_artifact_id`; omitted or exact empty
`subject_revision_id` selects the Artifact form, while a present value selects
the exact Revision form and must pass the identity grammar. Replay compares the
same selected subject form byte-for-byte through the rebuilt PEOS value and
its authoritative `SubjectKey` projection.

### 5. C7 uses semantic membership, not historical command origin

For a proposed Requirement pair `P = (artifact_id, revision_id)`:

```text
A = ArtifactEnvelope(artifact_id)
R = RevisionEnvelope(P)
O = RevisionOrderMetadata(P)
J = all RevisionAcceptanceRecord where Key == P
T = RequirementCriterionTrace(P)
OCC(P) = {R, O, J, T}
```

`A` is a shared owning root and prerequisite. Its existence alone does not
occupy `P`. `P` is absent exactly when `R`, `O`, and `T` are absent and `J` is
empty.
A valid Requirement Artifact may therefore own several complete revision acts.
Every revision under that shared Requirement Artifact has one immutable
canonical subject for the Artifact's entire history. A genuinely new pair
that attempts to retarget the Artifact to another capability is `409
immutable_value_conflict`; a stored history whose revisions disagree on
subject is contradictory and returns `500 internal_error`, including when the
disagreeing revision is withdrawn.

An occupied `P` is complete only after the full Artifact, Revision, order,
journal, projection, payload, digest, reference, and sequence-history
inspection succeeds. Then:

```text
M = the only record in J whose stored State is accepted
```

is the authoritative C7 member. Exactly one accepted record produces exactly
one `M`; zero or more than one is `500 internal_error`. A later valid
`withdrawn` record remains journal history and does not replace `M`. Actor,
reason, effective time, and ordering are integrity data, not membership
selectors. Which historical command inserted `M` is deliberately irrelevant.

For every genuinely new C7 act, `acceptance_record_id` is mandatory,
caller-supplied, presence-aware, and validated with FF-010's identity grammar.
It becomes `M.RecordID`; the server never derives or generates it. The fixed
new-act values remain:

```text
Actor       = "featureforge:local-user"
Reason      = "requirement established"
EffectiveAt = the one candidate time captured for the act
```

Historical stored member IDs are opaque. For a complete existing aggregate,
an omitted ID may recover `M` only as a replay; it can never create an act. An
exact supplied historical `M.RecordID` may replay even when that opaque value
does not satisfy the later grammar. A different malformed supplied ID is
invalid input. No implementation may reconstruct a historical ID with the
removed concatenation formula.

AD-019 remains valid: C7 still writes immediate acceptance atomically. This
decision clarifies the member and transfers new-member identity ownership to
the caller.

### 6. C9 now has an explicit accepted founding act

FF-004 already requires order metadata for every Validation Plan revision and
defines absence of an acceptance entry as `draft`. It did **not** previously
require C9 to accept a plan immediately. This decision makes that additional
product choice explicitly:

```text
C9 = Validation Plan A + R + O + immediate accepted M
```

For a genuinely new C9 act, caller-supplied `acceptance_record_id` is mandatory
and becomes `M.RecordID`. The server-owned member values are:

```text
Actor       = "featureforge:local-user"
Reason      = "validation plan established"
EffectiveAt = the one candidate time captured for the act
```

C9 uses the same semantic-member and integrity rules as C7. A Validation Plan
Artifact is a shared owning root; a new absent pair under a valid root may form
a later plan revision only when it preserves the Artifact's immutable scope.
A retarget request is `409`; a stored mixed-scope history is `500`, including
withdrawn history. A current plan revision is resolved with FF-004's
sequence-and-acceptance algorithm. Selection among several plan Artifacts for
one capability remains a different question: the existing exactly-zero-or-one
discovery contract and `ErrValidationPlanAmbiguous` remain unchanged.

Subject-based Requirement and Validation Plan discovery is an index lookup,
not an authority. Before a discovered Artifact participates in Q3, Q4, or Q5,
the application inspects its complete history, including payload/projection
agreement, stable subject/scope, order, journal, member, and references.
Corruption therefore precedes ordinary multiple-plan ambiguity.

The choice of immediate acceptance preserves the canonical flow, which has no
separate plan-acceptance step, while making the plan displayed and referenced
by that flow authoritative under the same current-state policy as capability
and Requirement revisions.

A stored C9-shaped `A + R` without `O` is partial and returns
`500 internal_error`; it is not made coherent by the fact that an older writer
returned success. No migration or silent repair is authorized.

### 7. Repository and representation boundaries remain intact

AD-026 remains the repository equality rule. `RevisionEnvelope.Equal` still
compares its governed repository representation, including `SubjectKey`.
AD-030 adds an earlier application-level replay and integrity decision; it does
not weaken or replace repository conflict detection for new writes.

Acceptance candidate integrity requires a narrow lookup by global
`RecordID`. AD-021's create-only uniqueness remains valid, while its statement
that nothing looks up acceptance by identity is superseded. The existing
PostgreSQL unique constraint and index already support the lookup; no schema
change is required.

No operation-origin field, membership flag, representation version, replay
fingerprint, idempotency-key table, or migration is introduced. The meaning of
the stored PEOS values and every `internal/domain` type remains unchanged.

## Alternatives

**Rely on repository `Put`/`Append` alone.** Rejected. It sees a reconstructed
representation only after time, sequence, and transition decisions have
already diverged from the first execution.

**Freeze or reuse the clock for retries.** Rejected. A process cannot know the
time used by a lost prior response, and C4/C5 still fail for non-time reasons.

**Add an `Idempotency-Key`.** Rejected. The immutable command identities already
name the act; a second identity system adds storage and expiry policy without
solving aggregate integrity.

**Persist command origin or a membership flag.** Rejected as unnecessary. A
fully validated stored act supplies the required semantic evidence. Historical
writer provenance is intentionally not part of the contract.

**Keep C7's deterministic acceptance formula.** Rejected. It is ungoverned,
non-injective, and violates caller ownership for a newly created identity.

**Leave C9 as an unordered draft.** Rejected. It contradicts FF-004's order
contract and leaves the canonical plan outside governed current-state
resolution.

**Add only order metadata and leave every new C9 revision draft for a later C5
act.** Rejected for this POC. It would add a new deliberate step to the
canonical flow solely to make the plan usable, while Requirement establishment
already provides the justified immediate-acceptance precedent in AD-019.

**Backfill or tolerate partial historical aggregates.** Rejected. No durable
database has been declared, and silently repairing an unreadable or incomplete
act would guess at authoritative engineering state.

## Consequences

- Exact C1–C12 replay is an application guarantee proven with an advancing
  clock, not inferred from repository tests.
- Replay and every failure path preserve byte-identical state and commit zero
  writes.
- C7 and C9 gain one presence-aware transport and UI field,
  `acceptance_record_id`, for genuinely new acts.
- C7 alone retains a narrow omitted-ID compatibility path for recovery of a
  complete stored act.
- Validation Plan revisions rejoin FF-004 ordering and current-state
  resolution; plan-Artifact multiplicity remains fail-loud.
- A Requirement Artifact has one lifetime subject and a Validation Plan
  Artifact one lifetime scope; retargeting requires a new Artifact identity.
- Q3/Q4/Q5 validate each discovered Requirement/Plan Artifact before using
  projected discovery membership or selecting a current revision.
- The acceptance repository gains a read capability by `RecordID`, but its
  append-only contract and both adapters' persistence representation remain.
- No Go domain type, PEOS value, route, status success code, database table, or
  migration is authorized by this decision.
- [FF-022](../spec/022-command-replay-and-aggregate-integrity.md) is the binding
  implementation and evidence packet. Domain analysis may begin only after
  FF-022 is implemented, its gates pass on both adapters, and its independent
  closure audit is complete.
