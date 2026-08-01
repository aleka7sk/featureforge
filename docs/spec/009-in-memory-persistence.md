# FF-009 — Engineering Records, Repositories, and In-Memory Persistence

Status: Accepted (Phase M.2)
Governs: the FeatureForge persistence envelope, structured revision content,
repository contracts, the transaction model, in-memory adapter semantics, and
the complete error taxonomy.

## Governance statement

PEOS-000 through PEOS-009 remain authoritative for every PEOS concept.
`docs/spec/` governs FeatureForge. This document does not extend the PEOS
ontology. The implementation must use PEOS v1.0.0 unchanged.

## 1. Why an envelope exists

M.1's AD-005 requires that no persistence adapter import PEOS. The envelope is
how: `internal/engineering/peos` serializes a PEOS value to canonical JSON and
projects the facts queries need into typed fields; everything downstream stores
and queries the envelope.

The envelope is a **persistence and projection carrier**, not a domain model. It
adds no engineering meaning. The payload is authoritative; every projected field
is a copy of something already inside it.

## 2. One envelope or several?

Three shapes were weighed.

| Option | Verdict |
|---|---|
| **A.** One universal envelope with a closed `RecordKind` | **Rejected.** Artifacts, revisions, and immutable records have genuinely different identity and lookup semantics. One envelope forces the widest key shape on all three, so "which fields are populated" becomes a runtime convention instead of a compile-time fact, and a repository can no longer state its own uniqueness constraint in its signature. |
| **B.** Separate envelopes for Artifact, Revision, Relation, and ImmutableRecord | **Rejected as specified** — only because of `Relation`. M.1 established that `relation.Relation` is not used in M.3; adding a `RelationEnvelope` now would be a placeholder for a construct the scenario never creates. |
| **C.** A small combination: three envelopes plus two product records | **Accepted.** |

**Accepted set:** `ArtifactEnvelope`, `RevisionEnvelope`, `RecordEnvelope`,
plus `CapabilitySpecificationContent`, `RevisionOrderMetadata`, and
`RevisionAcceptanceRecord`.

That is the original M.3 set. AD-032/FF-023 add the dedicated
`LifecycleDefinitionEnvelope` and `LifecycleDefinitionVersionEnvelope`
configuration carriers, and AD-033 adds the product-owned
`RequirementCriterionTrace`. The trace is exact-revision metadata, not a
fourth PEOS envelope family.

`RelationEnvelope` is **not** created in M.3. When a relation is first required,
it is added as a fourth envelope with its own composite uniqueness rule —
relations have no normative PEOS identity, so they are keyed by
`(type, from, to, scope)` and cannot share `RecordEnvelope`'s single-identity
key. Recorded as **AD-013**.

## 3. Envelope types

All three share four fields — `Kind`, `Payload`, `PayloadDigest`, `RecordedAt` —
and differ in their key and their projections.

### 3.1 `ArtifactEnvelope`

| Field | Meaning |
|---|---|
| `Key` | `ArtifactKey{ArtifactID string}` |
| `Kind` | `RecordKindArtifact` |
| `ArtifactType` | Projected vocabulary string, e.g. `featureforge:product-capability` |
| `Payload` | Canonical JSON of `core.Artifact` |
| `PayloadDigest` | SHA-256 of `Payload` |
| `RecordedAt` | Application clock time of the recording act |

Lookup: by `ArtifactID` only. Uniqueness: `ArtifactID`.

### 3.2 `RevisionEnvelope`

| Field | Meaning |
|---|---|
| `Key` | `RevisionKey{ArtifactID, RevisionID string}` |
| `Kind` | `RecordKindRevision` |
| `RevisionFamily` | Which specialization the payload is: `capability`, `requirement`, `validation-plan`, `transition-record`, `evidence` |
| `ArtifactType` | Projected vocabulary string |
| `IntegrityValue` | Projected `core.IntegrityIdentity` value, e.g. `sha256:…` |
| `ProvenanceActor` | Projected actor, `namespace:identifier` |
| `ProvenanceRecordedAt` | Projected provenance timestamp, optional |
| `ContentDigest` | For a capability revision, the digest of its structured content; empty otherwise |
| `Payload` | Canonical JSON of the family's revision type |
| `PayloadDigest`, `RecordedAt` | As above |

Lookup: by exact `(ArtifactID, RevisionID)`, and list-by-`ArtifactID`.
Uniqueness: the exact pair.

`RevisionFamily` is required because a requirement revision, a plan revision, and
a bare capability revision are all revisions but decode into different Go types.
Without it, decoding would have to guess.

### 3.3 `RecordEnvelope`

Covers immutable records with family-specific identity: execution records,
claims, decisions, and state assignments.

| Field | Meaning |
|---|---|
| `Key` | `RecordKey{Kind RecordKind, ID string}` |
| `Kind` | `RecordKindDecision`, `RecordKindExecution`, `RecordKindClaim`, or `RecordKindStateAssignment` |
| `SubjectKey` | Projected subject identity — for the scenario always a revision or artifact key, rendered canonically |
| `Scope` | Projected scope, `kind\|expression` |
| `OccurredAt` | The family's own time: claim timestamp, execution `CompletedAt`, state assignment `EffectiveAt`, decision provenance `RecordedAt` |
| `Outcome` | Projected outcome vocabulary: claim outcome, execution outcome, or empty |
| `CriterionKeys` | Ordered projected criteria identities — claims and executions |
| `EvidenceKeys` | Ordered projected evidence revision keys |
| `ExecutionKeys` | For claims: the execution records cited |
| `CorrectionKind` | `correct` / `replace` / `invalidate`, or empty |
| `CorrectionTargetID` | The corrected record's ID, or empty |
| `StateID` | For state assignments |
| `Payload`, `PayloadDigest`, `RecordedAt` | As above |

Lookup: by `(Kind, ID)`; list by `Kind`; list by `(Kind, SubjectKey)`.
Uniqueness: `(Kind, ID)`.

`CorrectionKind` and `CorrectionTargetID` are the projections that let the
correction-chain algorithm run in `application` with no PEOS import
([FF-008 §4](008-package-architecture.md#why-queries-do-not-live-here)).

### 3.4 Envelope rules

**Identity.** The key is the identity. Two envelopes with equal keys are the same
record.

**Payload authority.** `Payload` is the record. Projections are convenience. A
projection disagreeing with its payload is a writer bug; the payload wins, and
[FF-012](012-test-specification.md) requires a fidelity test per family.

**Equality.** Envelopes are equal when their keys are equal and their payloads
are byte-identical. Projections are not compared — they are derived, so a
projection difference on identical payloads is a codec bug, surfaced by the
fidelity test rather than by equality.

**Idempotency.** Writing an envelope whose key exists and whose payload is
byte-identical succeeds and changes nothing.

**Conflict detection.** Writing an envelope whose key exists with a different
payload fails with `ErrImmutableValueConflict`. Byte comparison of canonical JSON
is the test — not a structural diff, because canonical JSON makes byte equality
exactly equivalent to value equality.

**Validation on construction.** A non-empty key; a non-empty payload that parses
as JSON; a digest matching the payload; a known `Kind`. An envelope that fails
these is never stored.

**JSON receiver behaviour.** Envelopes are FeatureForge types with ordinary
struct JSON. They are not PEOS values and impose no PEOS wire concerns. On
decode failure the receiver is left untouched, matching the SDK's own contract so
that adapter behaviour is uniform.

## 4. Structured revision content

### 4.1 `CapabilitySpecificationContent`

Fields, in canonical order:

| Field | Type | Required |
|---|---|---|
| `schema_version` | integer | yes, must be `1` in M.3 |
| `title` | string | yes, non-empty after trimming |
| `problem_statement` | string | yes, non-empty after trimming |
| `user_outcome` | string | no |
| `functional_behaviours` | ordered list of string | no |
| `constraints` | ordered list of string | no |
| `acceptance_criteria` | ordered list of `{key, text}` | no; keys unique and non-empty |
| `dependencies` | ordered list of string | no |
| `open_questions` | ordered list of string | no |

**Validation.** Zero-value rejected. Required strings non-empty after trimming;
the trimmed value is stored. No element of any list may be empty after trimming.
Acceptance-criterion keys are unique within the content and match `[A-Za-z0-9-]+`.
`schema_version` other than `1` is rejected with `ErrUnsupportedSchemaVersion`.

**Maximum constraints.** Strings are bounded to 4 000 bytes and lists to 64
elements — not for storage reasons, but so a malformed input fails at the
boundary instead of producing an unbounded digest input. Exceeding either is
`ErrContentTooLarge`.

**Canonical JSON.** The digest is only meaningful if serialization is
deterministic. The rules are:

1. Field order is the declared order above, not alphabetical and not Go struct
   order by accident — the marshaller emits fields explicitly in this sequence.
2. No insignificant whitespace: no spaces after `:` or `,`, no newlines.
3. UTF-8, with Go's default HTML escaping **disabled** (`json.Encoder.SetEscapeHTML(false)`),
   so `<`, `>`, and `&` are not rewritten.
4. Empty optional lists are emitted as `[]`, never omitted and never `null`.
   Empty optional strings are emitted as `""`. Omission is not used, so a
   round trip cannot change the byte form.
5. Integers are emitted without exponent form.
6. Nested objects follow the same rules recursively.

**Equality.** Two contents are equal when their canonical JSON is byte-identical.
No field-by-field comparison is written.

**Digest.** `sha256(canonicalJSON)`, lowercase hex. The `Digest` type stores the
hex string; the PEOS `IntegrityIdentity` value is `"sha256:" + hex`. Verified
against the SDK: `core.NewIntegrityIdentity(core.IntegrityMechanismContentAddressedReference,
"sha256:"+hex, core.IntegrityProtectedScopeContent)` is accepted.

**Schema-version handling.** `schema_version` participates in the digest. A
future version 2 changes the digest of otherwise-identical content, which is
correct: it is a different canonical form. M.3 accepts only version 1 and
rejects others rather than attempting migration.

**Storage linkage.** Content is stored separately from the revision envelope and
keyed by the same `RevisionKey`. The revision envelope carries `ContentDigest`;
the codec's `VerifyDigest` recomputes the content digest and compares it with the
`IntegrityValue` recorded in the immutable PEOS payload. Mismatch is
`ErrPayloadDigestMismatch`.

Outside `engineering/peos`, the link is named by `RevisionKey` — a FeatureForge
opaque key — never by `core.ArtifactRevisionRef`.

### 4.2 `RevisionOrderMetadata`

| Field | Meaning |
|---|---|
| `Key` | `RevisionKey` |
| `Sequence` | Positive integer, unique within the artifact |
| `RecordedAt` | Clock time of assignment |

**Insert-only.** No update path exists.

**Predecessor is not retained in M.3.** A dense sequence with linear history
carries the same information: the predecessor of sequence *n* is sequence *n−1*.
Storing both would be two sources of one truth. The limitation this accepts is
explicit: **branching cannot be represented**, and adding it later requires a
branch label plus either a predecessor field or a per-branch sequence
([FF-004 §2](004-current-state-resolution.md#future-extension-path)).

**Acceptance is not stored here.** See §4.3.

### 4.3 `RevisionAcceptanceRecord`

| Field | Meaning |
|---|---|
| `RecordID` | Product-owned identity, unique |
| `Key` | `RevisionKey` |
| `State` | `draft`, `accepted`, or `withdrawn` |
| `EffectiveAt` | Clock time |
| `Actor` | `namespace:identifier` |
| `Reason` | Free text, may be empty |

**Append-only journal.** A revision's current acceptance state is the state of
its latest acceptance record, ordered by `(EffectiveAt, RecordID)` — a total
order. There is **no stored acceptance field anywhere**; storing one would
duplicate the journal head, which §13 of the M.2 brief forbids and which would
be the classic derived-field bug this project exists to catch.

A revision with no acceptance record is `draft` by absence. Recording the first
`accepted` record is what makes it authoritative.

Permitted transitions — `draft→accepted`, `draft→withdrawn`,
`accepted→withdrawn` — are validated by the command against the journal head.
`accepted→draft` is rejected with `ErrAcceptanceTransitionInvalid`, and
`withdrawn` is terminal.

## 5. Repository contracts

Interfaces are declared in `internal/application` and implemented by
`internal/infrastructure/memory`. They are **family-specific**, not one generic
`Repository[T]`: a generic repository cannot state per-family uniqueness,
ordering, or conflict rules in its signature, which is exactly the information
M.4 needs when it writes SQL.

No method returns `map`, `any`, or `interface{}`. No method exposes a PEOS type.

| Repository | Operations |
|---|---|
| `ProjectRepository` | `Put` (create-only), `Get`, `List` |
| `FeatureCardRepository` | `Put` the stable base establishment value, `Get`, `ListByProject`, `LinkCapability` once |
| `ArtifactEnvelopeRepository` | `Put`, `Get(ArtifactKey)` |
| `RevisionEnvelopeRepository` | `Put`, `Get(RevisionKey)`, `ListAll`, `ListByArtifact(artifactID)`, `ListByFamilyAndSubject(family, subjectKey)` |
| `StructuredContentRepository` | `Put`, `Get(RevisionKey)` |
| `RecordEnvelopeRepository` | `Put`, `Get(RecordKey)`, `ListAll`, `ListByKind(kind)`, `ListByKindAndSubject(kind, subjectKey)` |
| `RevisionOrderRepository` | `Put`, `Get(RevisionKey)`, `ListByArtifact(artifactID)` |
| `RevisionAcceptanceRepository` | `Append`, `GetByRecordID(recordID)`, `ListByRevision(RevisionKey)`, `ListByArtifact(artifactID)` |
| `RequirementCriterionTraceRepository` | `Put`, `Get(RevisionKey)` |
| `LifecycleDefinitionRepository` | `PutDefinition`, `GetDefinition`, `ListDefinitions`, `PutVersion`, `GetVersion(DefinitionVersionKey)`, `ListVersions(definitionID)` |
| `UnitOfWork` | `Do(ctx, func(Repositories) error) error` |

**Forward correction (AD-030, FF-022).** `GetByRecordID` is the one narrow
addition to the accepted M.3 table. It exists so an application command can
classify a caller-named acceptance identity before deciding replay, conflict,
or stored-integrity failure. It is read-only; the journal remains append-only.
Absence returns `(zero, false, nil)`, one readable record returns
`(record, true, nil)`, and unreadable or contradictory persistence returns an
error rather than masquerading as absence.

**Forward integrity-discovery correction (AD-032, FF-023).** `ListAll` on
Revision and Record envelopes is the later, concrete addition required by
integrity-sensitive Q3/Q4/Q5 and lifecycle discovery. Both operations return
the complete envelope population in typed-key ascending order inside the
ambient UnitOfWork. The application must inspect every authoritative payload,
digest and projection before filtering by `RevisionFamily`, `Kind` or
`SubjectKey`. `ListByFamilyAndSubject` and `ListByKindAndSubject` remain valid
projection queries, but they are not completeness witnesses: using either as
the first integrity filter could silently omit an envelope whose stored
projection contradicts its payload. No speculative `ListByFamily` port is
introduced.

### Per-operation semantics

| Aspect | Rule |
|---|---|
| Intent | `Put` records an immutable value. `Append` adds a journal entry. Neither ever overwrites. |
| Input | Typed envelope or record value; keys are typed, never bare strings |
| Output | The value and a found flag, or a typed error — never a nil-value sentinel |
| Not found | `Get` returns `(zero, false, nil)`. Absence is not an error; the caller decides whether it is. Callers that require presence wrap it in `ErrNotFound` with the key named. |
| Idempotency | `Put` with an identical payload for an existing key is a success no-op |
| Conflict | `Put` with a differing payload for an existing key returns `ErrImmutableValueConflict` |
| Ordering | Every `List` returns a deterministic order, specified per repository: envelopes by key ascending; order metadata by sequence ascending; acceptance records by `(EffectiveAt, RecordID)` ascending. **Never map iteration order.** |
| Transaction | Every operation participates in the ambient unit of work; there is no non-transactional path |

Repository idempotency in this table remains a storage guarantee. It is not by
itself proof that a whole application command is replay-safe: AD-030 and
FF-022 require the application to recover and compare the complete persisted
semantic act before reconstructing server-owned time, order, or provenance.

`ProjectRepository.Put` and `FeatureCardRepository.Put` establish the stable
base value selected by AD-031: a second `Put` for an existing ID with different
establishment content is `ErrImmutableValueConflict`. The separately stored
capability link is excluded from FeatureCard `Put` equality; after linking, an
identical base `Put` remains an idempotent no-op in both adapters.
`LinkCapability` performs the sole supported operational transition: absent to
one Artifact ID, same-link no-op, different-link
`ErrCapabilityAlreadyLinked`. No general update or delete operation is defined
for this bounded POC.

## 6. Transaction model

```
UnitOfWork.Do(ctx, func(r Repositories) error { ... })
```

| Aspect | Rule |
|---|---|
| Interface | `Repositories` is a struct of the ten repository interfaces, handed to the callback. Repositories are reachable **only** inside `Do`. |
| Callback semantics | Returning `nil` commits. Returning an error rolls back and propagates that error unchanged, so `errors.Is` still matches the original cause. A panic rolls back and re-panics. |
| Rollback | No write performed inside the callback is visible after a rollback — not to a later transaction, and not to a concurrent reader. |
| Conflict propagation | `ErrImmutableValueConflict` from any `Put` propagates out and aborts the act. It is never swallowed or downgraded to a no-op. |
| Nested transactions | **Forbidden.** Calling `Do` inside `Do` returns `ErrNestedTransaction`. One engineering act is one transaction; nesting would make the atomicity boundary ambiguous. |
| Scope | Single-process, single-store. No distributed transaction, no two-phase commit, no saga. |

Each of the eleven engineering acts in [FF-010 §3](010-application-contracts.md#3-commands)
is exactly one `Do` call, and each commits fully or leaves no trace.

## 7. In-memory adapter semantics

The adapter is a **semantic test double**, not a shortcut. Everything below is
required behaviour that the PostgreSQL adapter must reproduce in M.4.

| Requirement | Implementation rule |
|---|---|
| Thread safety | A single `sync.RWMutex` guards the store. Concurrent `Do` calls serialize on commit. Tests exercise concurrent revision creation. |
| Transactional isolation | `Do` operates on a **copy-on-write overlay**: reads fall through to the committed state, writes land in the overlay, and commit merges the overlay under the write lock. Rollback discards the overlay. |
| Insert-only | The store exposes no update or delete path for engineering records. Overwriting is structurally impossible, not merely unused. |
| Idempotent identical writes | Compared by canonical payload bytes |
| Conflict on differing payload | `ErrImmutableValueConflict`, naming the key |
| Composite uniqueness | Revision keys are composite `(ArtifactID, RevisionID)`; record keys are `(Kind, ID)`. Distinct kinds may share an ID string without colliding. |
| Deterministic listing | Every `List` sorts explicitly by its documented key before returning. A test seeds keys whose insertion order differs from sorted order and asserts stable output across repeated calls. |
| Explicit not-found | `(zero, false, nil)`; never a zero value that reads as valid |
| Reference verification | Where the repository owns the constraint, `Put` verifies referenced keys exist and returns `ErrReferencedValueMissing`. Applies to: a revision's artifact; content's revision; order metadata's revision; an acceptance record's revision; a Requirement Criterion Trace's Requirement and source capability revisions; a Lifecycle Definition Version's owning Definition; a record envelope's subject; and a claim's cited execution records and correction target. |
| Failure injection | `Store` accepts an optional hook that fails the *n*th write of a given kind, so rollback is tested without contriving a real conflict |
| No exposed maps | Internal collections are unexported. Every accessor returns copies; a caller mutating a returned slice cannot affect stored state. |

Conceptual collections: projects, feature cards, artifact envelopes, revision
envelopes, structured content, record envelopes, revision order metadata,
acceptance journal, requirement criterion traces, lifecycle definitions, and
lifecycle definition versions. Eleven maps, each keyed by its typed key.

## 8. Error model

All errors are package-level sentinels wrapped with `fmt.Errorf("%w", …)` plus
context, so `errors.Is` matches the sentinel and the message names the key
involved.

### Domain — `internal/domain`

`ErrProjectIDRequired`, `ErrProjectNameRequired`, `ErrFeatureIDRequired`,
`ErrInvalidFeatureTitle`, `ErrFeatureProjectRequired`

### Engineering — `internal/engineering`

`ErrInvalidEnvelope`, `ErrUnsupportedPayloadKind`, `ErrUnsupportedSchemaVersion`,
`ErrContentTooLarge`, `ErrInvalidContent`, `ErrDuplicateAcceptanceCriterionKey`

### Persistence — `internal/application` (ports) and the adapter

`ErrNotFound`, `ErrImmutableValueConflict`, `ErrReferencedValueMissing`,
`ErrTransactionAborted`, `ErrNestedTransaction`

`ErrRelationConflict` is **not** defined in M.3: no relation is stored, and a
sentinel with no producer is a claim the code does not honour. It is introduced
with `RelationEnvelope`.

### Ordering

`ErrRevisionOrderMissing`, `ErrRevisionSequenceConflict`,
`ErrRevisionSequenceInvalid`, `ErrNoAcceptedRevision`,
`ErrCurrentRevisionAmbiguous`, `ErrRevisionReferenceMismatch`,
`ErrAcceptanceTransitionInvalid`

### Correction

`ErrCorrectionTargetMissing`, `ErrCorrectionFamilyMismatch`,
`ErrCorrectionSelfReference`, `ErrCorrectionCycle`, `ErrCorrectionAmbiguous`

`ErrCorrectionSelfReference` is required because **PEOS v1.0.0 does not reject a
self-correction** — verified against the SDK, which accepts a correction
reference whose target is the claim's own identity. Rejecting it is
FeatureForge's obligation. Recorded as **AD-017**.

### Query

`ErrEngineeringStateIndeterminate`, `ErrTimelineSourceInvalid`,
`ErrValidationPlanAmbiguous`

### Lifecycle and stored integrity (AD-032/FF-023)

`ErrLifecycleTransitionInvalid`, `ErrLifecycleHeadConflict`,
`ErrStoredStateIntegrity`

The former `ErrAmbiguousLifecycleState` and `ErrUnknownDefinitionVersion`
sentinels are removed. Branching/equal-time ambiguity and a wrong stored
Definition Version are persisted integrity failures, not client choices.

### Serialization

`ErrStoredPayloadInvalid`, `ErrPayloadDigestMismatch`, `ErrContentIntegrityMismatch`

### Wrapping rules

1. A FeatureForge sentinel is always the outermost matchable error for a
   FeatureForge-owned failure.
2. **PEOS sentinels remain visible.** When a PEOS constructor rejects input,
   `engineering/peos` wraps the SDK error with `%w` so that both
   `errors.Is(err, validation.ErrInvalidSatisfactionClaim)` and any FeatureForge
   sentinel it adds continue to match. The SDK's nested-sentinel behaviour —
   general sentinel and specific cause both matching — is preserved and tested.
3. No generic "internal error" replaces a specific cause anywhere in domain,
   engineering, or application code.
4. Every error names the identity involved. `ErrImmutableValueConflict` without
   the key is not acceptable.
