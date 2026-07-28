# FF-016 Architecture Review

Reviewed commit: `aeaa24f` — *feat(engineering): implement FF-016 revision
subject discovery (AD-025)*
Date: 2026-07-28
Phase: M.5 (prerequisite change)
Scope: review only. No Go, SQL, or other documentation was modified.

## Review objective

Determine whether FF-016 remained entirely inside the narrow Architecture
Freeze exception authorized by [AD-025](../decisions/README.md#ad-025), and
whether general M.5 Phase A HTTP implementation may safely begin.

## Method and evidence classification

The review reads the diff and the current tree. It does not accept the FF-016
implementation report as evidence for any conclusion; where the report and the
code disagree, the code governs. Every conclusion below carries one of four
tags:

| Tag | Meaning |
|---|---|
| **[IMPL]** | Implementation evidence — proven by reading production code |
| **[TEST]** | Test evidence — proven by an executing assertion |
| **[INFER]** | Inferred conclusion — follows from evidence but is not directly asserted anywhere |
| **[OPEN]** | Unresolved uncertainty — not established by this review |

A recurring distinction matters throughout: several behaviours are correct in
the *current* codec paths but are not constrained by the *public repository
contract*. Those are marked as such rather than being reported as safe.

---

## 1. Scope conformity

**Finding: conforms. No scope expansion detected.**

`git show --name-only aeaa24f` lists exactly 19 files. Every one maps to an
authorized bullet: **[IMPL]**

| Authorized change | File(s) |
|---|---|
| `RevisionEnvelope.SubjectKey`, `RevisionEnvelopeInput.SubjectKey` | `internal/engineering/envelope.go` |
| Subject projection at the PEOS boundary | `internal/engineering/peos/codec_{artifact,requirement,validation,lifecycle}.go` |
| `RevisionEnvelopeRepository.ListByFamilyAndSubject` | `internal/application/ports.go` |
| Repository support | `internal/infrastructure/{memory,postgres}/repositories.go` |
| Migration | `internal/infrastructure/postgres/migrations/0002_revision_subject_key.sql` |
| Shared contract additions | `internal/infrastructure/contracttest/contract.go` |
| Application discovery helpers | `internal/application/query_state.go`, `query_timeline.go` |
| Tests | `envelope_test.go`, `postgres_test.go`, `projection_test.go`, `scenario_test.go` |
| Documentation completion | `docs/decisions/README.md`, `docs/spec/015-*.md`, `docs/spec/016-*.md` |

**Not redesigned, verified by absence from the commit:** **[IMPL]**

- `UnitOfWork`, retry semantics, transaction semantics — no `unitofwork*` file
  appears in the commit;
- `SubjectKey` string representation — `internal/engineering/refkeys.go` and
  `keys.go` are absent from the commit; `ParseSubjectKey`,
  `ArtifactSubjectKey`, and `SubjectKind` are consumed, never altered;
- AD-005 — the only package importing PEOS is still
  `internal/engineering/peos`, enforced by `TestOnlyIntegrationPackageImportsPEOS`;
- AD-006 — no derived verdict, readiness value, or resolved revision is
  stored; the new column holds a projection of a fact already inside the
  immutable payload (with one qualification, see §3);
- repository architecture — one method added; no existing signature changed;
- PEOS integration boundaries — no adapter decodes a payload;
- unrelated envelope contracts — `ArtifactEnvelope` and `RecordEnvelope` are
  untouched;
- HTTP / UI — no `internal/http`, `internal/ui`, or `cmd/` package exists;
  `TestNoHTTPDatabaseUIOrAIPackage` still passes.

One change in the commit was not on the authorized list and deserves explicit
classification: the `TestMigrateIsIdempotent` repair
(`internal/infrastructure/postgres/postgres_test.go:26–52`). The prior
assertion was `if applied != 1`, which encoded "exactly one migration file
exists" — not idempotency. It now captures the count before and after and
asserts it is unchanged. **This is a necessary test repair, not scope
expansion:** the old assertion would fail on the arrival of *any* second
migration, independent of FF-016's content. **[IMPL]** Severity: observation.

---

## 2. Revision subject semantics

### 2.1 Per-family contract

Enforced centrally in `NewRevisionEnvelope`
(`internal/engineering/envelope.go:130–138`): **[IMPL]**

```go
if in.SubjectKey != "" {
    switch in.RevisionFamily {
    case RevisionFamilyCapability, RevisionFamilyEvidence:
        return RevisionEnvelope{}, fmt.Errorf("%w: revision family %q has no subject...", ErrInvalidEnvelope, ...)
    }
    if _, _, _, err := ParseSubjectKey(in.SubjectKey); err != nil {
        return RevisionEnvelope{}, fmt.Errorf("%w: revision envelope subject key is malformed: %v", ErrInvalidEnvelope, err)
    }
}
```

| Family | Status | Derived at | From | Malformed | Non-empty on subject-less family |
|---|---|---|---|---|---|
| `Capability` | **forbidden** | — | — | n/a | rejected, `ErrInvalidEnvelope` |
| `Evidence` | **forbidden** | — | — | n/a | rejected, `ErrInvalidEnvelope` |
| `Requirement` | **optional** (populated in practice) | `codec_requirement.go:113` | `RequirementInput.SubjectArtifactID` | rejected | n/a |
| `ValidationPlan` | **optional** (populated in practice) | `codec_validation.go:157` | `PlanInput.ScopeArtifactID` | rejected | n/a |
| `TransitionRecord` | **optional** (populated in practice) | `codec_lifecycle.go:193` (entry), `:371` (transition) | `EntryAssignmentInput` / `TransitionInput.SubjectArtifactID` | rejected | n/a |

"Optional" is the literal contract: the constructor accepts an empty
`SubjectKey` for all five families. No mechanism requires a subject-bearing
family to carry one. **[IMPL]** See §5 F-002.

### 2.2 Legacy revisions

A row written before migration `0002` has `subject_key IS NULL`;
`stringOrEmpty` (`postgres/repositories.go`) maps it to `""`, which the
constructor accepts for every family. Legacy revisions therefore remain valid
and readable, and are excluded from every non-empty subject query. **[IMPL]**
Partially covered by `TestRevisionSubjectKeyColumnProjection`, which exercises
a genuinely-NULL capability row rather than a pre-migration row. **[TEST]**

### 2.3 ValidationPlan — Scope is not Subject

The implementation projects `engineering.ArtifactSubjectKey(in.ScopeArtifactID)`
(`codec_validation.go:157`) — the *same* helper the requirement and lifecycle
paths use, producing the same canonical `artifact:<id>` form. It does not
introduce a scope-specific key kind, and it does not convert a `core.Scope`
into a `core.EngineeringSubjectRef`. **[IMPL]**

FF-016 §3.2 states the distinction correctly and explicitly: *"The plan's
source field is a PEOS `Scope`, not a PEOS `Subject`. FF-016 does not assert
that a Scope is a Subject."* The struct field comment
(`envelope.go:82–89`) and the `revisionEnvelopeInput` comment
(`codec_artifact.go`) both phrase the projection as *"which capability … is
this revision about?"* rather than naming a PEOS construct. **[IMPL]**

**No code or documentation wording was found that could later support a
`Scope == Subject` inference.** The one place the two concepts meet is a
shared *answer*, not a shared type, and every comment on that path says so.

### 2.4 Can callers inject inconsistent projections?

Yes. The constructor validates *form* (parseable, family-appropriate) but never
*agreement with the payload*. Nothing checks that a requirement revision's
projected subject matches the `EngineeringSubjectRef` inside its own payload.
`contracttest` relies on this: `mustRevisionEnvelopeWithSubject`
(`contract.go:857–872`) builds envelopes whose payload is `{"revision_id":…}`
and whose `SubjectKey` is arbitrary. **[IMPL]** This is by design — the
adapters treat the value as opaque (FF-016 §3.7) — but it is the precondition
that makes §3 reachable.

---

## 3. Critical `Equal` review

**This is the primary finding of the review.**

### 3.1 The mechanism

`RevisionEnvelope.Equal` (`internal/engineering/envelope.go:161–164`): **[IMPL]**

```go
// Equal reports whether e and other have equal keys and byte-identical
// payloads. Projections are not compared.
func (e RevisionEnvelope) Equal(other RevisionEnvelope) bool {
	return e.Key == other.Key && samePayload(e.Payload, other.Payload)
}
```

Both adapters build create-only conflict detection directly on it:

- **memory** — `revisionRepo.Put` (`memory/repositories.go:190`) delegates to
  `putCreateOnly` (`:45–54`), which on an existing key returns `nil`
  **without writing** when `equal(existing, value)` is true, and
  `ErrImmutableValueConflict` otherwise. **[IMPL]**
- **PostgreSQL** — `revisionRepo.Put` (`postgres/repositories.go:354–381`)
  issues `INSERT … ON CONFLICT (artifact_id, revision_id) DO NOTHING`; when
  `tag.RowsAffected() == 0` it re-reads and returns `nil` if
  `existing.Equal(env)`, else `ErrImmutableValueConflict`. **[IMPL]**

### 3.2 Case A and Case B

**Case A** — stored `SubjectKey` empty, incoming non-empty, same key and
payload.
**Case B** — stored non-empty, incoming empty, same key and payload.

Both cases resolve identically, and identically in both adapters: **[IMPL]**

| Question | Answer |
|---|---|
| Is the rewrite considered idempotent? | **Yes.** `Equal` ignores `SubjectKey`, so both adapters classify it as an identical re-Put. |
| Does the stored `SubjectKey` silently win? | **Yes.** Memory never writes to the overlay; PostgreSQL's `DO NOTHING` discards the incoming row. The caller receives `nil`. |
| Is it reachable through supported production paths? | **Only through one codec path** — see §3.3. |
| Is it reachable through direct repository use? | **Yes.** `Put` accepts any well-formed envelope; nothing requires it to have come from a codec. |
| Is it reachable through tests? | **Yes.** `mustRevisionEnvelopeWithSubject` already constructs arbitrary key/payload/subject combinations. |
| Does it violate create-only semantics? | **Partially.** History is never mutated, so the immutability rule holds. But "identical re-Put is a no-op, differing Put conflicts" no longer holds over the envelope's full observable state. |
| Is `SubjectKey` purely derived data? | **No** — for one family it is not derivable from the payload (§3.3). |
| Or is it observable persisted state? | **Yes.** It is a stored column, readable through `Get`, and the sole predicate of `ListByFamilyAndSubject`. |

**Adapter parity is preserved.** Memory and PostgreSQL agree exactly, including
on this defect. That is a genuine positive: the shared-contract mechanism did
its job of preventing divergence, even where the shared behaviour is itself
questionable. **[IMPL]**

### 3.3 Reachability depends on whether the payload carries the subject

If the payload contains the subject, then byte-identical payloads imply
identical derived subjects, and the codec cannot produce a divergent pair.
Reading each codec: **[IMPL]**

| Codec path | Subject inside the marshalled payload? | Can a divergent pair arise? |
|---|---|---|
| `BuildRequirement` (`codec_requirement.go:63–71`) | **yes** — `requirement.NewContent(…, []core.EngineeringSubjectRef{subject}, …)` flows into `reqRev`, the marshalled value | no |
| `BuildValidationPlan` (`codec_validation.go:103–116`) | **yes** — `scope` → `NewScopedPlanApplicability` → `NewPlanContent` → `planRev` | no |
| `BuildTransition` (`codec_lifecycle.go:281–284`) | **yes** — `lifecycle.NewTransitionRecordContent(subject, …)` → `trRevision` | no |
| **`BuildEntryAssignment`** (`codec_lifecycle.go:179–195`) | **no** — `entryRev` is a bare `core.ArtifactRevision` built from the *transition record's* IDs, origin, provenance, and an integrity value of `TransitionRecordArtifactID + "/" + TransitionRecordRevisionID`. `in.SubjectArtifactID` never enters it; it reaches only the separate state-assignment `RecordEnvelope`. | **yes** |
| `BuildCapabilityRevision`, `BuildEvidenceArtifactAndRevision` | n/a — no subject projected | n/a |

The entry assignment is content-free by design (AD-014). That design decision,
harmless on its own, is precisely what makes its subject projection
unrecoverable from its payload.

**Concrete divergent pair, through the supported command path:** two
`AssignLifecycleStateCommand{IsEntry: true}` calls sharing
`TransitionRecordArtifactID`, `TransitionRecordRevisionID`, and `RecordedAt`
but naming different `SubjectArtifactID`s produce byte-identical payloads with
different `SubjectKey`s. The second `Put` returns `nil`; the store keeps the
first capability's subject. **[INFER]** — derived from reading the codec; not
executed, because the review may not add a test.

Under a real clock the two calls would differ in `RecordedAt`, which is inside
`provenance` and therefore inside the payload, so the pair would conflict
loudly instead. **[INFER]** The divergence therefore requires a fixed or
coincident clock, or direct repository use. It is a latent contract defect
rather than an active production bug.

### 3.4 Assessment

`SubjectKey` is observable persisted state that does not participate in
conflict detection. For requirement and validation-plan revisions — the only
two families the M.5 discovery path queries — divergence is structurally
impossible, because their payloads carry the subject. The exposure is confined
to entry transition records and to direct repository use.

**Classification: implementation defect.**

Not "no issue": the contract permits a `Put` to succeed while silently
discarding part of the submitted value. Not "documentation clarification"
alone: documenting it would make the behaviour known but would leave
`ListByFamilyAndSubject` answering from a projection the last writer did not
supply. Not "architecture defect": no abstraction boundary is wrong, no layer
is misplaced, and the fix is local to one method plus its tests.

**Smallest correction — recommended, not implemented:**

1. Add the two missing shared contract subtests (§4) that pin the intended
   behaviour, whichever is chosen. This is required regardless of option.
2. Then choose one:
   - **(a) Include `SubjectKey` in `RevisionEnvelope.Equal`.** A differing
     subject then conflicts, and create-only semantics hold over the full
     observable state. Smallest code change (one clause). Requires an AD-025
     amendment, because AD-025's Consequences section currently states
     "`RevisionEnvelope.Equal` still compares key and payload only".
   - **(b) Keep `Equal` as-is and document the exclusion** explicitly in
     FF-016 §3/§4 and AD-025, stating that a re-`Put` with a differing subject
     is a silent no-op and that the first writer's projection is authoritative.

Option (a) is the more robust of the two and is what "observable persisted
state" argues for. **This review does not choose between them** — the choice is
an architecture decision belonging to the maintainer, and either is defensible
provided the tests pin it.

---

## 4. Test coverage

The shared suite registers three new subtests
(`contracttest/contract.go:62–64`), run unchanged by both adapters. **[TEST]**

| Required behaviour | Proven? | Evidence |
|---|---|---|
| Lookup behaviour | **yes** | `testRevisionListByFamilyAndSubject` — exact membership, multiple matches on one subject, a second subject excluded, a second family excluded, subject-less revision never returned |
| Deterministic ordering | **yes, narrowly** | same test asserts ascending `RevisionKey.String()` across two elements |
| Empty / no-match result | **yes** | asserts empty slice with nil error, not `ErrNotFound` |
| Optional `SubjectKey` | **yes** | `testRevisionSubjectKeyIsOptional` — capability revision round-trips with `""` |
| Malformed `SubjectKey` rejection | **yes** | `testRevisionRejectsMalformedSubjectKey` |
| **Idempotent `Put` with a subject** | **no** | — |
| **Conflicting `Put` when `SubjectKey` differs** | **no** | — |
| **Transaction visibility for subject-projected revisions** | **no** | generic rollback/commit subtests use `Projects`, not revisions |

The three gaps were named in the FF-016 implementation directive and were not
delivered. The middle one is the material gap: **a "conflicting Put when
`SubjectKey` differs" test would fail today** (§3), so its absence is not
merely reduced coverage — it is the reason the §3 defect went unobserved.
**[INFER]**

Two further gaps, both implementation-only: **[IMPL]**

- **No test asserts that `BuildEntryAssignment` or `BuildTransition` populate
  `SubjectKey` at all.** Grepping `RevisionFamilyTransitionRecord` across the
  tree returns only two hits, both in `envelope_test.go`, both exercising the
  constructor generically rather than the codec. No test performs
  `ListByFamilyAndSubject(RevisionFamilyTransitionRecord, …)`. The lifecycle
  projection is therefore unverified end to end.
- **No test distinguishes a pre-migration row from a genuinely subject-less
  row.** Both read back as `""`, which is correct by FF-016 §3.3, but the
  "legacy readability" claim rests on `nullableString`/`stringOrEmpty`
  symmetry rather than on an executed legacy-row case.

The PostgreSQL-specific tests are correctly scoped to PostgreSQL-specific
obligations — raw column inspection, NULL round-trip, migration shape — and do
not duplicate the shared suite. `TestRevisionSubjectKeyColumnProjection`
additionally verifies the adapter reproduces the raw column exactly, which the
shared suite cannot see. **[TEST]** No false-confidence duplication was found.

---

## 5. Application integration

**Finding: conforms.**

`discoverArtifactIDsBySubject` (`query_state.go:30–46`): calls
`ListByFamilyAndSubject` with `ArtifactSubjectKey(capabilityArtifactID)`,
deduplicates by `rev.Key.ArtifactID`, and `sort.Strings` the result.
Deterministic; never relies on map iteration order. Returns `[]string` artifact
IDs — the identity form `EngineeringStateInput.RequirementArtifactIDs` and
`TimelineInput.RequirementArtifactIDs` already use, so no new identity form is
introduced. **[IMPL]**

- **Only requirement and validation-plan discovery changed.**
  `DiscoverRequirementArtifactIDs` (`query_state.go:56`) and
  `DiscoverValidationPlanArtifactIDs` (`query_timeline.go:104`) are the only
  two callers; both pass a fixed family constant. **[IMPL]**
- **`EngineeringStateInput` shape unchanged** — still exactly
  `CapabilityArtifactID`, `RequirementArtifactIDs`, `DecisionIDs`
  (`query_state.go:17–21`); only the doc comment changed. **[IMPL]**
- **`TimelineInput` shape unchanged** — all nine fields identical; only the doc
  comment changed. **[IMPL]**
- **No HTTP logic leaked.** `internal/application` imports only `context`,
  `sort`, `fmt`, `slices`, `time`, and the two internal packages;
  `TestNoHTTPDatabaseUIOrAIPackage` passes. **[TEST]**
- **No claim-derived fallback exists.** `GetFeatureEngineeringState` and
  `GetFeatureTimeline` still consume caller-supplied identifier lists and never
  call the discovery helpers themselves, so there is no code path that could
  silently substitute a claim-derived population. Decisions, executions,
  claims, and evidence continue to use `Records.Get` /
  `Records.ListByKindAndSubject` exactly as before. **[IMPL]**

The helpers are additive and opt-in: a caller must invoke them deliberately.
That is a conservative design, and it means the M.5 blocker is cleared without
changing the behaviour of any existing query. **[INFER]**

---

## 6. Canonical REQ-4 proof

The proof lives in `assertCanonicalEndState`
(`internal/scenario/scenario_test.go:306–345`), step 13.

**Executes on both adapters — confirmed.** `assertCanonicalEndState` is the
shared assertion body invoked by `TestCanonicalScenario` (memory,
`scenario_test.go:52`) and `TestCanonicalScenarioPostgres`
(`scenario_postgres_test.go:83`). Both passed under `make postgres-test`.
**[TEST]**

| Claim | Proven? | Evidence |
|---|---|---|
| REQ-4 exists | **yes** | `ResolveEffectiveRequirements(discovered)` succeeds; it errors on any requirement without an accepted revision (`query_readiness.go:42–44`) |
| REQ-4 is discovered by subject lookup | **yes** | `DiscoverRequirementArtifactIDs(ctx, r, "CAP-1")` returns exactly `[REQ-1 REQ-2 REQ-3 REQ-4]`, asserted element-wise |
| REQ-4 has no claim | **yes** | `per.RequirementArtifactID == "REQ-4" && !per.HasClaim` must be observed |
| Discovered population feeds readiness | **yes** | `ResolveReadiness(…, discoveredEffective)` — the *discovered* list, not the fixture list |
| REQ-4 has no plan activity | **no — inferred** | fixture-level fact: `scenario.go:179–183` plans only A-1/A-2/A-3. The test does not assert the plan omits REQ-4 |
| A claim-derived population would omit REQ-4 | **no — inferred** | follows from "REQ-4 has no claim", but no test executes a claim-derived discovery to demonstrate the omission |

**One precision correction to how this result should be described.** The
aggregate verdict asserted is `ReadinessNotReady`, not `incomplete`
(`scenario_test.go:334`). REQ-4 contributes the *incomplete* signal
(`sawIncomplete` in `query_readiness.go:121`), but AD-016 precedence puts
not-ready first because REQ-2 is `peos:not-satisfied`. Any statement that
"readiness remains incomplete" is imprecise; the accurate statement is **"REQ-4
is reported with no applicable claim while the aggregate verdict remains
not-ready."** **[TEST]**

The falsifiability of the proof was demonstrated during implementation by
removing REQ-4 from the expected set and observing
`discovered requirements = [REQ-1 REQ-2 REQ-3 REQ-4], want exactly [REQ-1 REQ-2 REQ-3]`.
That check is not itself part of the suite. **[INFER]**

**Assessment: the proof is real and load-bearing**, not a method-invocation
smoke test. Its two inferred elements are properties of the FF-011 fixture that
other tests already depend on, so the inference is sound — but they are
inferred, not asserted.

---

## 7. Architecture guards

**Finding: all existing guards hold; `architecture_test.go` is unchanged in the
commit.** **[IMPL]**

| Guard | Status |
|---|---|
| `TestNoUpdateOrDeleteOnEngineeringTables` | passes; see below |
| `TestOnlyIntegrationPackageImportsPEOS` | passes — PEOS decoding boundary intact |
| `TestInfrastructureDoesNotImportPEOS` | passes — neither adapter can decode a payload |
| `TestDomainAndApplicationDoNotImportDriver` | passes — dependency direction intact |
| `TestAdaptersDoNotImportEachOther` | passes |
| `TestNoHTTPDatabaseUIOrAIPackage` | passes — no HTTP/UI package exists |
| `TestGoModHasOnlyApprovedRequirements` | passes — no new dependency |

**No `UPDATE` or `DELETE` in the migration.** `0002_revision_subject_key.sql`
contains exactly one `ALTER TABLE … ADD COLUMN` and one `CREATE INDEX`.
**[IMPL]**

**The deliberate violation genuinely proved the guard.** Appending
`UPDATE revision_envelopes SET subject_key = subject_key;` to the new migration
produced:

```
architecture_test.go:161: internal/infrastructure/postgres/migrations/0002_revision_subject_key.sql
  contains "update revision_envelopes"; engineering records are immutable
```

The failure names the *new* file and the intended reason, so the guard's
`filepath.WalkDir` over the migrations directory demonstrably reaches files
added after the guard was written — which is the property that needed proving.
Reverting restored a byte-identical file and a passing test. **[TEST]**

**Invariants currently existing only in prose:**

1. **"Subject-bearing families must project a subject."** FF-016 §3.2 asserts
   it; nothing enforces it. A future codec could omit `SubjectKey` and every
   test would still pass except, indirectly, the canonical scenario — and only
   for the requirement family. Realistically enforceable by a codec-level test
   asserting each `Build*` returns a non-empty `SubjectKey`, which is the same
   gap §4 already records. **No new architecture test is recommended**; a
   normal unit test is the right instrument.
2. **"The projection agrees with the payload."** Deliberately unenforced
   (§2.4), and correctly so — enforcing it would require decoding payloads
   outside `internal/engineering/peos`. Recording it as a known,
   accepted-by-design gap is sufficient.
3. **"`SubjectKey` is excluded from identity."** Stated only in a Go doc
   comment on `Equal`. This is the §3 finding; it belongs in FF-016 and AD-025,
   not in an architecture test.

---

## 8. Documentation consistency

**Finding: consistent, with one wording gap.** **[IMPL]**

| Check | Result |
|---|---|
| FF-016 marked implemented | **yes** — `Status: Implemented (Phase M.5, prerequisite change)`; governance statement rewritten from "Nothing in this document is implemented" to a verified-gates statement; steps 8, 9, 10 carry "As implemented" notes |
| AD-025 marked implemented | **yes** — `Phase: M.5 (prerequisite change; implemented)`, plus an "Implementation evidence" paragraph |
| FF-015 no longer claims pending | **yes** — §6.2 "investigated, decided, and now closed"; §16 step 5 "are unblocked: FF-016 has landed"; §17 "Accepted and implemented" |
| FF-015 still marks HTTP/UI unstarted | **yes** — §6.2 "no route, handler, or UI screen exists yet"; §17 "remains future work" |
| FF-009 untouched | **yes** — absent from `git show --name-only aeaa24f` |
| M.5 investigation remains historical | **yes** — absent from the commit; not rewritten into a retrospective prediction |
| Supersession stays narrow | **yes** — FF-016 §5 supersedes only the discoverability inference and lists what is explicitly *not* superseded |
| Open questions still accurate | **yes** — write-time existence verification and the backfill tool remain listed as deferred |

**The one wording gap:** no document states that `SubjectKey` is excluded from
`RevisionEnvelope.Equal` and therefore from conflict detection. AD-025's
Consequences section says *"`RevisionEnvelope.Equal` still compares key and
payload only, so a projection cannot affect identity or conflict detection"* —
which is factually true and was written as a *reassurance* that the change is
non-invasive. Read after §3, the same sentence is the understated statement of
the defect. It does not overstate any guarantee, but it does not warn either.
Finding F-001's correction (2b) would address this.

No documentation was found that overstates implementation guarantees. In
particular, **no document claims "differing Put remains a conflict when
`SubjectKey` differs"** — verified by grep across `docs/`. Any review premise to
that effect does not correspond to what was written.

---

## 9. Findings

### F-001 — `SubjectKey` is excluded from `Equal`, so a differing subject is a silent no-op

- **Severity:** major
- **Evidence:** `envelope.go:161–164`; `memory/repositories.go:45–54, 190`;
  `postgres/repositories.go:354–381`; divergence reachable via
  `codec_lifecycle.go:179–195` (entry assignment payload omits the subject) and
  via direct repository use.
- **Architectural impact:** `SubjectKey` is observable persisted state that
  does not participate in create-only conflict detection. A `Put` can succeed
  while silently discarding the submitted projection, leaving
  `ListByFamilyAndSubject` answering from a value the last writer did not
  supply. Immutability of history is not violated; the create-only contract is
  weakened over the envelope's full observable state. Both adapters behave
  identically, so this is a contract defect, not a parity defect.
- **Blocks HTTP?** **No.** Requirement and validation-plan payloads both carry
  their subject, so the two families the M.5 discovery path queries cannot
  diverge.
- **Smallest correction:** add the two missing contract subtests (F-002), then
  either include `SubjectKey` in `Equal` (requires an AD-025 amendment) or
  document the exclusion explicitly in FF-016 §3/§4 and AD-025. Required before
  anything relies on transition-record subject discovery.

### F-002 — Shared suite omits idempotent-Put, conflicting-Put, and transaction-visibility coverage

- **Severity:** major
- **Evidence:** `contracttest/contract.go:62–64` registers three subtests; the
  FF-016 implementation directive requested these three additional behaviours.
- **Architectural impact:** the conflicting-Put case is precisely the one that
  would have surfaced F-001 before commit. Its absence is why a contract
  weakening reached `main` with a full green suite. Transaction-visibility
  coverage for the new operation is asserted in FF-016 §4.2 but proven only for
  `Projects`.
- **Blocks HTTP?** **No.**
- **Smallest correction:** three subtests in `contracttest/contract.go`,
  pinning whichever F-001 behaviour is chosen.

### F-003 — Transition-record subject projection is entirely unverified

- **Severity:** minor
- **Evidence:** `codec_lifecycle.go:193, 371` populate `SubjectKey`; grep for
  `RevisionFamilyTransitionRecord` across `internal/` returns only
  `envelope_test.go:73, 88`, both constructor-level. No test issues
  `ListByFamilyAndSubject(RevisionFamilyTransitionRecord, …)`.
- **Architectural impact:** one of the three subject-bearing families rests on
  implementation evidence alone. A regression removing either call site would
  be caught by nothing.
- **Blocks HTTP?** **No** — the M.5 discovery path does not query this family.
- **Smallest correction:** one assertion in an existing lifecycle or scenario
  test that a transition-record revision is discoverable by its capability
  subject.

### F-004 — "Legacy row" compatibility rests on symmetry, not on an executed case

- **Severity:** observation
- **Evidence:** `nullableString("")` → `NULL` (`postgres/repositories.go:47–52`);
  `stringOrEmpty(nil)` → `""`. `TestRevisionSubjectKeyColumnProjection` covers a
  genuinely subject-less capability row, which is the same storage state a
  pre-migration row has.
- **Architectural impact:** none in practice — the two states are
  indistinguishable by construction, which FF-016 §3.8 already documents as an
  accepted consequence. Recorded so the claim is not read as stronger than it
  is.
- **Blocks HTTP?** No.
- **Smallest correction:** none required.

### F-005 — `TestMigrateIsIdempotent` repair

- **Severity:** observation
- **Evidence:** `postgres_test.go:26–52`; the prior `if applied != 1` encoded
  file count rather than idempotency.
- **Architectural impact:** none. Correctly classified as a necessary test
  repair rather than scope expansion; the old assertion would have failed on
  any second migration regardless of FF-016.
- **Blocks HTTP?** No.
- **Smallest correction:** none required.

---

## 10. Architecture confidence

**High confidence — directly proven by code and tests**

- Scope conformity: 19 files, no frozen contract reopened (§1).
- Constructor invariants: malformed rejected, subject-less families reject a
  non-empty subject, empty accepted for all five families (§2.1).
- ValidationPlan models "which capability is this about" without asserting
  `Scope == Subject`, in both code and prose (§2.3).
- Lookup semantics — family equality, subject equality, ordering, empty-result,
  subject-less exclusion — proven identically on both adapters (§4).
- Application integration: input struct shapes unchanged, no claim-derived
  fallback, deterministic ordering and deduplication (§5).
- REQ-4 discovery and its readiness consequence, on both adapters (§6).
- Architecture guards, including demonstrated coverage of the new migration
  file (§7).
- Documentation status flags across AD-025, FF-016, FF-015; FF-009 and the M.5
  investigation untouched (§8).

**Medium confidence — well supported but dependent on convention or partial
coverage**

- Requirement and validation-plan subject projections are correct *and*
  self-consistent with their payloads — proven by reading the codecs and
  exercised by the canonical scenario, but never asserted as a payload-vs-
  projection agreement check.
- Legacy/pre-migration row readability (F-004).
- Ordering determinism is asserted over two elements; PostgreSQL's
  `ORDER BY artifact_id, revision_id` and memory's `sort.Slice` on
  `Key.String()` agree for the tested shapes, and are believed to agree
  generally, but no test exercises a case where the two could differ.

**Low confidence — assumed, untested, or contractually ambiguous**

- Transition-record subject projection: implementation evidence only (F-003).
- Behaviour of `Put` when the submitted `SubjectKey` differs from the stored
  one: no test pins it in either direction, and the current behaviour is
  probably not the intended one (F-001).
- Transaction visibility and rollback semantics *specifically for
  subject-projected revisions*: asserted by FF-016 §4.2, inherited from the
  surrounding transaction machinery, but not exercised.
- **[OPEN]** Whether the `BuildEntryAssignment` divergence is reachable in any
  realistic deployment. This review establishes it is reachable in principle
  and requires a fixed or coincident clock; it does not establish whether any
  intended M.5 caller would produce that condition.

---

## Final verdict

# APPROVED WITH REQUIRED CORRECTIONS

FF-016 was implemented inside the boundary AD-025 authorized. The exception was
not widened: no frozen contract was reopened, no unauthorized abstraction was
introduced, and the change is additive throughout. One latent contract defect
(F-001) and its enabling test gap (F-002) were found; neither blocks the
authorized Phase A work, and both should be closed before the capability is
relied upon beyond requirements and validation plans.

**1. Is FF-016 complete?**
Substantially yes. All twelve steps of FF-016 §13 landed and the specified
contract is in force. Three items from the implementation directive's shared
contract-suite list were not delivered (F-002), so it is not complete in the
strictest reading of its own test plan.

**2. Is AD-025 fully implemented?**
Yes. Both decided changes — the optional projection and
`ListByFamilyAndSubject` — exist in both adapters with the specified semantics.
One sentence in AD-025's Consequences section understates a consequence
(§8) rather than misdescribing the implementation.

**3. Was the Architecture Freeze preserved?**
Yes. UnitOfWork, transaction and retry semantics, the `SubjectKey`
representation, AD-005, AD-006, repository architecture, the PEOS boundary, and
unrelated envelope contracts are all untouched and verified absent from the
commit.

**4. Is `SubjectKey` vs `Equal` safe?**
**Not unconditionally.** It is safe for requirement and validation-plan
revisions, where the payload carries the subject and divergence is structurally
impossible. It is unsafe as a general repository contract: a differing subject
on an existing key+payload is silently accepted and discarded, reachable via
the entry-assignment codec path and via direct repository use. Classified as an
implementation defect (F-001).

**5. May Phase A HTTP implementation begin?**
**Yes** — for the authorized scope. Requirement and validation-plan discovery
is sound, complete, proven on both adapters, and immune to F-001.

**6. Which corrections are required before HTTP?**
**None are strictly required before Phase A begins.** Required before the
capability is relied upon more broadly, and recommended to be done first
because they are small:

- F-002 — add the three missing shared contract subtests. Do this first: it
  pins the behaviour and turns F-001 from an open question into a decided one.
- F-001 — decide between including `SubjectKey` in `Equal` (with an AD-025
  amendment) and documenting the exclusion explicitly; then record the choice.
- F-003 — one assertion covering transition-record subject discovery. Required
  before any caller uses that family.

**7. Does PEOS require any change?**
**No.** PEOS v1.0.0 is used unchanged. No PEOS concept was added, renamed, or
redefined; the projection is a FeatureForge-owned string derived from input the
codec already holds.

---

## Verification results

All commands run against the working tree at commit `aeaa24f`, with no source
file modified.

| Command | Result |
|---|---|
| `git status` | clean before and after the review; only this new report is added |
| `git show aeaa24f` | 19 files, +715 / −58 |
| `gofmt -l .` | no output |
| `go vet ./...` | no output |
| `go build ./...` | success |
| `go test ./... -count=1` | all 8 packages `ok` |
| `go test ./... -race -count=1` | all 8 packages `ok` |
| `make postgres-test` | all PostgreSQL integration tests pass, including the 23-subtest contract suite, `TestRevisionSubjectKeyColumnProjection`, and both canonical scenario tests |

No pre-existing failures were observed.
