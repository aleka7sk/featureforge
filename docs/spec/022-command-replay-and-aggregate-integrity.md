# FF-022 — Command replay and aggregate integrity

Status: Implemented (M.5 command replay and aggregate-integrity correction)
Date: 2026-08-01
Phase: M.5 correctness closure before domain analysis
Governs: command-level idempotency for C1–C12, persisted-act integrity
inspection, C7/C9 acceptance-member identity, Validation Plan ordering and
current resolution, and the evidence required before domain analysis begins

## Governance statement

PEOS-000 through PEOS-009 remain authoritative for PEOS concepts. This packet
does not add, rename, or reinterpret a PEOS value. FeatureForge specifications
remain authoritative for FeatureForge product behavior.

[AD-030](../decisions/ad-030-command-idempotency-and-replay-conformance.md)
is the accepted architecture decision implemented by this packet. AD-019,
AD-021, AD-026, and AD-029 retain the portions AD-030 explicitly preserves;
the reconciliation matrix in §3 names every corrected statement.

This document was accepted as an implementation contract before work began.
§15 now records the committed, independently verified completion evidence that
advanced its status to `Implemented`.

## 1. Baseline and evidence that forced the correction

Planning baseline:

```text
repository: aleka7sk/featureforge
branch: main
HEAD: b9e11e51c15b822d9094d370f9aae4da1a94e0c2
```

The baseline was clean when the investigation began. Two historical commits
are part of the governing evidence:

- `64514ff7c...` introduced C7 as Artifact + Revision only;
- `6a3a00cc578f870749125ee38e1b7fe77c16bdcc` added AD-019,
  Requirement order metadata, and immediate acceptance, using the derived
  acceptance ID later rejected by AD-030.

The correction is required because:

1. FF-010 requires exact command replay to be a no-op.
2. Current command tests predominantly use a non-advancing clock.
3. The full canonical scenario is not executed twice against the same store.
4. Repository `Put` equality cannot recognize a replay before a command has
   regenerated time, order, transition state, or a hidden member identity.
5. FF-004 applies revision order/current-state rules to Validation Plan
   revisions, while C9 and the later read surface omit them.

The implementation must treat these as correctness defects. It must not amend
the product contract to match the baseline behavior.

## 2. Scope and exclusions

### In scope

- C1–C12 replay recognition inside the application transaction;
- full integrity inspection of every member of the named persisted act;
- caller-owned `acceptance_record_id` for new C7 and C9 acts;
- C7 replay-only identity omission compatibility;
- one global acceptance lookup by `RecordID` in both adapters;
- C9 order metadata, immediate acceptance, and FF-004 current-plan resolution;
- transport DTO, UI form, application, projector, adapter, and test changes
  required by those behaviors;
- the PostgreSQL `RevisionOrder.ListByArtifact` ordering defect found while
  proving adapter parity.

### Out of scope

- changes under `internal/domain`;
- PEOS changes or a new PEOS concept;
- database migrations or backfill;
- new routes, a second API representation, or a changed success status;
- operation-origin, membership, representation-version, replay-fingerprint,
  or idempotency-key persistence;
- authentication, authorization, distributed coordination, deployment tuning,
  or M.6;
- choosing among multiple applicable Validation Plan Artifacts;
- the later domain-analysis work this packet gates.

If implementation appears to require an excluded change, work stops and the
new need is governed separately; it is not smuggled into FF-022.

## 3. Normative reconciliation matrix

| Existing source | Preserved | Corrected by AD-030 / FF-022 |
|---|---|---|
| FF-004 | sequence, acceptance states, current = greatest accepted sequence, ambiguity fails | Validation Plan revisions are no longer exempted by later implementation prose |
| FF-009 | payload authority, create-only repositories, draft by absence, one UOW | acceptance repository adds global `GetByRecordID`; command replay is stronger than repository re-`Put` |
| FF-010 | client-owned identities, twelve command intents, atomic acts | exact replay is recognized from stored semantic acts before reconstruction; C7/C9 act tables gain member identity |
| FF-014 / AD-021 | acceptance `RecordID` is globally unique, append-only, schema/index remain | “never lookup by identity” is superseded by one read-only candidate-integrity lookup |
| FF-015 | HTTP status vocabulary and centralized mapping | idempotency is not a gift from repository `Put`; integrity failures map to 500 |
| FF-018 §3.1 | twelve POST routes and existing result bodies | C7/C9 DTOs gain `acceptance_record_id`; C10's evidence pair is part of act identity |
| FF-018 §6.6 | zero/one/many plan-Artifact discovery and fail-loud multiplicity | the claim that plans have no order/current resolution is superseded |
| FF-018 §11 | all commands are create-only | handler-free replay and repository equality alone are insufficient; application replay branches are required |
| FF-020 §6 | plan activities are decoded through `EngineeringProjector` | the selected plan Artifact is resolved through FF-004 rather than choosing its only/last revision |
| FF-021 §6–7 | server-side forms, no JavaScript, context-derived existing IDs | C7/C9 forms collect the genuinely new acceptance identity |
| AD-019 | C7 immediate accepted postcondition | the accepted record is the semantic member and its new identity is caller-owned |
| AD-026 | `RevisionEnvelope.Equal`, including `SubjectKey`, remains repository equality | command integrity decodes before semantic comparison; adapters still do not decode PEOS |
| AD-029 | no server-generated identity and no idempotency-key mechanism | not all identities were already present; C7 has one narrow replay-only omission exception |

Historical reports remain historical evidence. In particular,
`docs/reports/m5-publication-remediation.md` is not rewritten; its D3 and §8.9
claims are corrected forward by AD-030 and this document.

## 4. Terms and universal algorithm

### 4.1 Terms

**Primary identity** is the caller-controlled key or key tuple that names one
command act.

**Local occupancy** is every persisted member whose presence under that
primary identity means an act has begun. A shared owning Artifact is not local
occupancy for every revision beneath it.

**Complete act** is the exact atomic postcondition in §6, after payload,
projection, digest, key, family, type, subject, order, journal, and reference
checks have all passed.

**Request semantics** are immutable caller intent after command-boundary
normalization. They exclude server-generated representation details listed in
§5.

### 4.2 Algorithm inside one UnitOfWork

Every C1–C12 execution follows this order:

1. Validate request fields that need no persisted state.
2. Capture at most one UTC, microsecond-precision candidate time before
   `UnitOfWork.Do`; reuse it if the transaction callback is retried.
3. Enter one `UnitOfWork` and inspect primary occupancy.
4. If any local member exists, prove the complete postcondition before
   comparing request semantics.
5. Return an integrity error for partial, unreadable, dangling, or
   contradictory state.
6. For a complete act, compare request semantics and every caller-owned
   identity. Equal means replay; different means immutable conflict.
7. Only for an absent act, validate dependencies, inspect secondary identity
   occupancy, consume the captured candidate time, allocate order where
   applicable, construct values, and write the complete act atomically.

No replay path consumes the candidate time, allocates a sequence, validates a
historical transition as a new transition, constructs a representation from
new server-owned defaults, or calls a write method. A recorder may reconstruct
an expected value from validated stored times as a final semantic comparison.
The pre-transaction clock read is deliberately not part of request semantics
and cannot change replay classification.

### 4.3 Outcome precedence

| Condition | HTTP | Machine/application classification | Writes |
|---|---:|---|---:|
| invalid non-state-dependent request | 400 | `invalid_command` | 0 |
| occupied primary act is partial, unreadable, dangling, or contradictory | 500 | `internal_error` / stored-integrity sentinel | 0 |
| complete act, equal request semantics and identities | 201 | original result/body | 0 |
| complete act differs, or primary identity has coherent foreign occupancy | 409 | `immutable_value_conflict` | 0 |
| absent act, ordinary required reference missing or transition illegal | 422 | existing governed machine code | 0 |
| absent act, valid request and free identities | 201 | created result/body | atomic act only |

For a syntactically valid secondary candidate identity, dangling, unreadable,
partial, or contradictory candidate occupancy returns 500 before an otherwise
ordinary 409. A coherent occupant owned by another complete act returns 409.

## 5. Time, order, provenance, and equality

The following values are verified for integrity but excluded from replay
request equality:

- Project/FeatureCard creation time;
- PEOS envelope and record `RecordedAt`;
- server provenance;
- revision sequence and order-record timestamp;
- C7/C9 fixed acceptance actor, reason, and effective time.

Defaultable caller time fields are presence-aware:

| Commands | Fields |
|---|---|
| C5 | `effective_at` |
| C6 | `effective_at`, `attempted_at`, `completed_at` |
| C10 | `completed_at` |
| C11, C12 | `timestamp` |

If supplied, a normalized value participates in request equality. If omitted,
the stored default is recovered on replay and the captured candidate time is
irrelevant.

All caller-supplied and server-owned times are normalized to UTC and truncated
to microsecond precision before construction or comparison, matching the
PostgreSQL representation. The one candidate time is captured before the
`UnitOfWork.Do` callback and reused if PostgreSQL retries the callback; only a
new act persists it.

Application semantic equality is evaluated only after decoding authoritative
payloads. At minimum the decoded value must agree with stored payload bytes,
canonical digest, embedded integrity, envelope key, `ArtifactType`,
`RevisionFamily`, `SubjectKey`, and all references applicable to its family.
Repository `Equal` remains a final create-only safety check and is not reused
as the application equality witness.

## 6. C1–C12 act matrix

| # | Primary/caller identities | Required complete act | Request-semantic witness beyond identities | Replay-specific requirement |
|---|---|---|---|---|
| C1 | `project_id` | Project | name | ignore stored creation time |
| C2 | `feature_card_id` | FeatureCard and valid owning Project | project, title, description | inspect card before rebuilding with new time |
| C3 | capability A/R pair | capability A, R, structured content, O(sequence 1), card link | FeatureCard and canonical capability content | verify all five members; no reconstructed timestamp |
| C4 | capability A/R pair | valid owning A, R, content, original O | canonical capability content | recover stored sequence; never recompute `max+1` on replay |
| C5 | `record_id` | valid Revision and exactly the named journal record | pair, target state, reason, presence-aware effective time | lookup record before transition validation |
| C6 | `assignment_id` and transition A/R pair | shared transition A, R, assignment record, predecessor when non-entry | all transition/assignment inputs and presence-aware times | classify every split-occupancy combination |
| C7 | Requirement A/R pair and new `acceptance_record_id` | valid shared A, R, O and semantic M | canonical Requirement statement, subject and fixed vocabulary | §7 |
| C8 | `decision_id` | Decision record and complete capability subject; structurally valid, optionally unresolved Evidence citation | every decision input including ordered collections | ignore server `RecordedAt`; do not dereference the evidence citation |
| C9 | Validation Plan A/R pair and `acceptance_record_id` | valid shared A, R, O and semantic M | canonical scope and ordered plan activities | §8 |
| C10 | `execution_id` and Evidence A/R pair | Evidence A, R and Execution record | plan/activity/subject/method/outcome/locator and presence-aware completion | classify evidence/execution split occupancy |
| C11 | `claim_id` | Claim and valid references | all claim inputs and presence-aware timestamp | C11/C12 share Claim identity namespace |
| C12 | `claim_id` | correcting Claim, existing target, acyclic valid chain | all claim inputs, correction kind and target | same ID containing a non-correction C11 is 409 |

Collection equality uses the command's governed canonical order. It must not
be replaced by map iteration or unexplained digest comparison.

The shared root in C7 and C9 has one lifetime applicability: all Requirement
revisions under one Artifact share one canonical subject, and all Validation
Plan revisions under one Artifact share one canonical scope. A new later pair
that attempts to retarget its root is an immutable conflict. Any stored mixed
history, including a withdrawn disagreeing revision, is contradictory state.

C8's evidence pair is a citation, not a member created by the Decision act.
The stored Decision must decode and project exactly one canonical Evidence key,
but that key may be unresolved. A readable standalone Evidence A/R pair cited
by a complete Decision is coherent foreign occupancy for a later C10 using the
same pair and returns 409. A standalone pair with no complete Execution owner
and no complete Decision citation, or a corrupt cited pair, returns 500.

## 7. C7 closure contract

For `P = (artifact_id, revision_id)`:

```text
OCC(P) = {Revision R, RevisionOrderMetadata O, acceptance journal J}
```

The Requirement Artifact `A` is a shared owning root. `P` is absent exactly
when R and O are absent and J is empty, regardless of whether A exists.

An occupied P is complete only when:

- A is a readable Requirement Artifact and its payload/projections agree;
- R is a readable Requirement Revision belonging to A and P;
- R's payload, family, type, subject, digest, integrity, and projections agree;
- O exists, names P, and is coherent with A's dense unique sequence history;
- every J record is readable, names P, and the total journal order describes
  permitted acceptance transitions;
- exactly one J record has stored state `accepted`;
- every required cross-reference resolves.
- every Requirement revision under A has the same canonical subject.

The unique accepted record is semantic member `M`. A later valid withdrawn
record is history and does not replace M.

### 7.1 Presence and grammar

- New P: `acceptance_record_id` is required, caller-owned, and must satisfy
  FF-010 grammar.
- Complete P: omission is allowed only for replay recovery.
- Exact supplied stored M ID: may replay even if that opaque historical value
  fails the later grammar.
- Non-matching malformed supplied ID: 400.
- No historical ID is regenerated from a prefix, concatenation, hash, or other
  formula.

### 7.2 C7 outcome matrix

| Primary state/request | Outcome |
|---|---|
| absent P, omitted member ID | 400, zero writes |
| partial or contradictory P | 500, zero writes |
| complete P, omitted ID, equal semantics | 201 original body, zero writes |
| complete P, omitted ID, different semantics | 409, zero writes |
| complete P, exact M ID, equal semantics | 201 original body, zero writes |
| complete P, exact M ID, different semantics | 409, zero writes |
| complete P, different valid ID naming corrupt occupancy | 500, zero writes |
| complete P, different valid free/coherently-owned ID | 409, zero writes |
| absent P, valid proposed ID naming corrupt occupancy | 500, zero writes |
| absent P, valid proposed ID owned by another complete act | 409, zero writes |
| absent P, valid free ID and valid dependencies | create A when absent or later R/O/M under valid A; 201 |
| absent P under valid A, requested subject differs from A history | 409, zero writes; use another Artifact identity |
| occupied A has mixed-subject history | 500, zero writes |
| zero or multiple accepted records under occupied P | 500, zero writes |

The successful response remains exactly the stored `artifact_id` and
`revision_id` representation already governed by FF-018.

## 8. C9 and current Validation Plan

C9 adopts the explicit AD-030 postcondition:

```text
Validation Plan Artifact A
+ Revision R
+ RevisionOrderMetadata O
+ immediate accepted semantic member M
```

`acceptance_record_id` is mandatory for every request, including replay. C9
has no omitted-ID historical compatibility path because the baseline created
no complete C9 act containing O and M. The exact same ID and request semantics
replay; a different ID or semantics conflicts under §4.3. C9 uses the same
member uniqueness, journal integrity, candidate-occupancy, shared-root, and
later-revision rules as C7.

Every revision under one Validation Plan Artifact preserves one canonical
scope. A later revision that requests another scope is 409; stored mixed-scope
history is 500.

The fixed new member is:

```text
State       = accepted
Actor       = "featureforge:local-user"
Reason      = "validation plan established"
EffectiveAt = the act's one captured candidate time
```

A baseline-style A+R without O is an occupied partial act and returns 500. It
is never classified as a coherent foreign occupant and never repaired.

Plan selection now has two separate steps:

1. `DiscoverValidationPlanArtifactIDs` uses the subject projection only to
   enumerate candidates, then the application inspector validates every
   candidate Artifact's complete history and stable scope. Zero is allowed,
   one is selected, more than one valid Artifact returns
   `ErrValidationPlanAmbiguous`; corruption returns 500 before ambiguity.
2. For the selected Artifact, `ResolveCurrentRevision` selects the greatest
   accepted sequence and returns its existing resolution rationale. No accepted
   revision is a well-formed “no current plan” result; corrupt order or journal
   state is an error.

The plan activities projected into Q3/Q4 come from that resolved current
Revision, never from key order, list order, or an assumption that an Artifact
has only one Revision. The application result and transport DTO carry the
resolution rationale using the existing rationale shape. No new endpoint is
introduced.

## 9. Required application and projector work

The application gains a reusable persisted-act inspection facility with
family-specific validation. It may share mechanics, but it must not collapse
the twelve acts into a generic CRUD command or expose PEOS types outside the
engineering integration seam.

Required capabilities:

- inspect each primary and secondary identity inside the command UOW;
- decode every stored PEOS family used by command equality;
- recompute canonical content/digests and validate all envelope projections;
- validate order density and uniqueness where FF-004 applies;
- validate acceptance journal structure and semantic member uniqueness;
- validate a lifetime-stable Requirement subject and Validation Plan scope on
  both command and Q3/Q4/Q5 read paths;
- distinguish absent, complete, coherent foreign, and corrupt occupancy;
- recover the exact original command result from a complete act.

`EngineeringProjector` remains the PEOS-free decode seam. It may gain the
narrow family projections needed for integrity and semantic equality.
`internal/application` and persistence adapters must not import the PEOS SDK.

A dedicated stored-integrity application sentinel must remain distinguishable
with `errors.Is` and map to `500 internal_error`. Existing invalid-command,
not-found/reference, invalid-transition, serialization, ambiguity, and
immutable-conflict sentinels retain their mappings.

## 10. Repository and adapter work

`RevisionAcceptanceRepository` gains:

```go
GetByRecordID(
    ctx context.Context,
    recordID string,
) (engineering.RevisionAcceptanceRecord, bool, error)
```

Contract:

- absent → zero value, `false`, `nil`;
- one readable record → that record, `true`, `nil`;
- unreadable or contradictory persistence → error, never “absent”.

Both adapters already have internal identity-search mechanics and PostgreSQL
already has a unique indexed `record_id`; expose the behavior through the
shared contract suite. `Append`, list methods, and journal order remain
unchanged.

PostgreSQL `RevisionOrder.ListByArtifact` must return order records by governed
`sequence`, matching the in-memory adapter and FF-004. Ordering by
`revision_id` is a baseline defect. Fix the query and add a parity contract
test. No migration is required.

The current memory whole-UOW mutex and PostgreSQL `SERIALIZABLE` transaction
with whole-callback retry remain sufficient. No lock table, advisory lock,
compare-and-swap field, or isolation change is authorized.

## 11. Transport and UI work

The twelve routes and nineteen-operation surface remain unchanged.

- C7 request DTO gains presence-aware `acceptance_record_id`.
- C9 request DTO gains required `acceptance_record_id`.
- HTTP exact replay returns the same 201 response body as first creation.
- Corrupt stored acts map to 500 `internal_error`; no internal detail leaks.
- Immutable differences map to 409 `immutable_value_conflict`.
- Ordinary new-act missing references and illegal transitions retain their 422
  machine codes.

The no-JavaScript UI adds one plain text field to the C7 and C9 forms and
preserves it on correctable failures. The person supplies these genuinely new
identities; existing artifact/revision context continues to be resolved
server-side. UI forwarding remains through the existing in-process API
handler established by AD-028.

## 12. Implementation order

1. Land AD-030 and this accepted packet.
2. Add acceptance identity lookup to the port, both adapters, and the shared
   contract suite; correct PostgreSQL order sorting.
3. Add persisted-act inspection and family projection/integrity support.
4. Convert C1–C6 replay paths, with advancing-clock tests.
5. Convert C7 and implement its full closure matrix.
6. Convert C8–C12; implement C9's new A/R/O/M postcondition and C10 split-act
   inspection.
7. Add C7/C9 DTO and UI fields; update HTTP replay/error tests.
8. Route selected Validation Plan Artifacts through FF-004 current revision
   resolution and project activities from the winner.
9. Run the complete evidence gate in §14 and record §15.
10. Obtain an independent read-only closure audit before changing this
    document's status.

Steps may be split into reviewable commits, but no intermediate commit may
claim FF-022 is implemented.

## 13. Required focused cases

In addition to the universal C1–C12 matrix, tests must cover:

- C4 replay returns the original sequence after another clock value is visible;
- C5 same-record replay is recognized before transition validation, while a
  genuinely new invalid transition remains 422;
- every C6 split occupancy across transition A/R and assignment;
- every C10 split occupancy across evidence A/R and execution;
- C11/C12 reuse of one Claim ID across correction/non-correction semantics;
- C7 absent/present shared A, absent/partial/complete P, zero/one/multiple
  accepted members, accepted then withdrawn, omitted/exact/different member
  ID, exact opaque legacy ID, malformed nonmatching ID, foreign/corrupt
  candidate, and both concatenation-collision pairs;
- C9 A/R/O/M atomicity, ID grammar and presence, baseline A+R partial state,
  later revision under one A, current selection, withdrawn/no-current state,
  and multiple plan Artifacts remaining fail-loud;
- C7 later-subject and C9 later-scope retarget attempts returning 409, stored
  mixed histories returning 500, and Q3/Q4/Q5 refusing those histories before
  current selection or plan ambiguity;
- candidate corruption taking 500 precedence over an ordinary semantic 409.

Every negative and replay test asserts zero writes either by comparing complete
store snapshots or by an instrumented UnitOfWork gate that rejects and counts
every repository mutation. Selected row counts alone are insufficient.

## 14. Acceptance gate before domain analysis

### Application and adapter gate

- Table-driven C1–C12: first execute, advance clock, exact execute again;
  result is equal, store is byte-identical, and replay writes zero values.
- For each command, change one caller-semantic field under the same identity;
  receive immutable conflict and zero writes.
- For each composite act, exercise partial and contradictory occupancy;
  receive stored-integrity error and zero writes.
- Shared acceptance lookup contract passes against memory and PostgreSQL.
- Revision-order sequence ordering passes identically on both adapters.
- Canonical scenario runs twice against the same store for each adapter and
  produces one byte-identical final state.

### HTTP and UI gate

- All twelve POST endpoints are replayed after advancing the clock on memory
  and PostgreSQL; each returns 201 and the original body.
- Changed semantics produce 409; corrupt state produces 500; each branch
  proves zero writes.
- C7/C9 JSON presence and grammar matrices pass.
- C7/C9 UI forms submit and preserve `acceptance_record_id`.
- The canonical browser flow remains complete with C9 immediate acceptance.

### Repository-wide gate

```text
gofmt/check formatting
go vet
go build
all unit and contract tests
race tests
PostgreSQL integration suite
architecture guards
clean working tree
```

The exact project commands used and their outputs belong in §15. A skipped
PostgreSQL gate is not completion evidence.

## 15. Implementation evidence

Status at packet acceptance: **not yet implemented**. Completion evidence,
recorded 2026-08-01:

| Evidence | Committed result |
|---|---|
| implementation commit | Published branch commit [`cf5f96526ffadda47f1bd5e67be3d810dce73be3`](https://github.com/aleka7sk/featureforge/commit/cf5f96526ffadda47f1bd5e67be3d810dce73be3), "Close command replay and aggregate integrity". Its tree is `11f1256e78b43f8ad3b31479765885e3b226e690`; the independently audited local commit `05bfd9e95cfc21341c3e4c6ffe9f3bd794b8818b` has that exact tree. |
| C1–C12 advancing-clock replay | `TestC1ThroughC12ReplayAfterClockAdvance` passes and rejects every replay write. `assertCanonicalScenarioHTTPReplay` drives all twelve POSTs twice after clock advance on memory and PostgreSQL. |
| changed-semantics / corrupt-state zero-write proof | `TestCommandConflictingReplay`, `TestCorruptPersistedActMapsTo500AndAttemptsZeroWrites`, the semantic-conflict suite, and the C6/C7/C9/C10/C11/C12 integrity regressions pass behind complete snapshots or the rejecting/counting `replaygate` UnitOfWork. |
| C7 closure matrix | `TestC7CallerMemberIdentityAndReplayOnlyOmission`, `TestC7LaterRevisionUsesSharedArtifactAndReplaysWithoutWrites`, `TestC7CallerOwnedMembersDoNotRepeatTheRemovedConcatenationCollision`, `TestC7WithdrawnHistoryReplaysUsingOriginalSemanticMember`, `TestC7RejectsZeroAndMultipleAcceptedMembersAsStoredIntegrity`, `TestC7LegacyOpaqueMemberAndMalformedNonmatchingIdentity`, and candidate-corruption precedence pass. |
| C9 A/R/O/M and current-plan resolution | `TestC9CallerMemberIdentityAndRequiredReplayPresence`, `TestC9LaterRevisionBecomesCurrentAndReplaysWithoutWrites`, `TestC9WithdrawnHistoryHasNoCurrentRevisionButStillReplaysItsAct`, `TestC9RejectsPartialZeroAndMultipleMemberState`, the stable-scope tests, and the applicable-plan resolver tests pass. |
| memory/PostgreSQL contract parity | `TestRepositoryContractSuite` and `TestPostgresRepositoryContractSuite` pass, including acceptance lookup by `RecordID`, revision-order history, immutable conflict, and idempotent put. PostgreSQL corrupt-materialization tests also pass without state change. |
| canonical scenario double-run | `TestCanonicalScenarioReplayMemory` and `TestCanonicalScenarioReplayPostgres` run the complete command stream twice, return identical results, attempt zero second-run writes, and compare the complete persisted snapshot byte-for-byte. The HTTP PostgreSQL replay and browser PostgreSQL scenario pass as well. |
| full verification | GitHub Actions [`Verify` run `30678968317`](https://github.com/aleka7sk/featureforge/actions/runs/30678968317) on exact commit `cf5f965` completed `success`: formatting, `go vet ./...`, `go build ./...`, `go test ./... -count=1`, and `go test ./... -race -count=1`. Both test steps ran with a healthy `postgres:16-alpine` service and `FEATUREFORGE_POSTGRES_TEST_DSN`; the PostgreSQL suite was not skipped. |
| independent closure audit | An independent read-only reviewer audited local `05bfd9e`, verified remote `cf5f965` has the same tree, checked the published branch and successful exact-hash workflow, and returned `READY` with no production or governance blocker. |

No predicted test, local uncommitted observation, or repository-level
`IdempotentIdenticalPut` result may be recorded as command-replay evidence.

## 16. Completion boundary

FF-022 is complete only when:

1. every §12 implementation step has landed;
2. every §13 focused case and §14 gate passes;
3. §15 names committed evidence rather than plans;
4. an independent audit finds no unresolved command-idempotency or aggregate-
   integrity contradiction;
5. the branch is clean and published.

Only then may the status become:

```text
Status: Implemented (M.5 command replay and aggregate-integrity correction)
```

All five conditions are satisfied by the evidence in §15. The pre-domain block
is closed. Nothing in this packet itself authorizes a change to
`internal/domain`, PEOS, or database migrations; later work still requires its
own governing scope.
