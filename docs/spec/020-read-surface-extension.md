# FF-020 — Read-surface extension

Status: Implemented (Phase M.5, ahead of Phase B UI planning)
Date: 2026-07-28
Phase: M.5 — read-surface extension (FF-018 Phase A remains unamended)
Governs: the addition of engineering *content* — as opposed to identity,
projections, and rationale metadata — to the Phase A query surface, and the
`internal/engineering/peos` → `internal/application` seam that decodes it.

## Numbering note

`017` names the AD-026 `SubjectKey`-equality work as a human label; no
`docs/spec/017-*.md` file exists. `018` is FF-018 (Phase A HTTP
implementation). `019` names the FF-018 post-implementation audit closure
(commit `f128952`, recorded as FF-018 §23); no `docs/spec/019-*.md` file
exists either, and none should be created — a reader searching "FF-019"
would otherwise find two different things. `020` is the next unambiguous
identifier.

## Governance statement

PEOS-000 through PEOS-009 remain authoritative for every PEOS concept.
`docs/spec/` governs FeatureForge. This document adds no PEOS concept,
renames none, redefines none, and adds no repository method, no migration,
and no dependency. PEOS v1.0.0 is used unchanged.

## 1. Why this document exists

Phase B (Minimal UI) planning was stopped on a blocking finding, confirmed by
direct repository inspection rather than assumed: FF-018's seven query
endpoints expose *identity, projections, and rationale metadata*, but not the
engineering *content* [FF-001 §3](001-poc-acceptance-contract.md#3-minimal-user-experience)
requires a reader to see. FF-007's M.5 exit criterion — "a reader answers
every question in FF-001 §3's usability acceptance from the UI alone" — was
therefore unreachable on the nineteen operations FF-018 delivered, regardless
of how well a UI was built on top of them. This document is the minimum
read-surface extension that closes that gap; it contains no UI work. Phase B
UI planning resumes against the contract this document completes.

## 2. Exactly which FF-001 §3 questions could not be answered

Derived by reading each screen's **Data** row against the actual DTOs in
`internal/transport/http/dto_query.go` at FF-018's HEAD (`f128952`):

| FF-001 §3 | Required datum | Was available? |
|---|---|---|
| §3.1 Projects, §3.7 Timeline | — | Yes — fully satisfiable, including Timeline's per-event rationale |
| §3.2 Feature overview | everything except current-revision title | Yes, mostly |
| §3.3 Revisions | full specification content per revision | No — `StructuredContent` was written by commands, never read by any query |
| §3.4 Requirements | requirement statement text | No — payload-only |
| §3.5 Decisions | question, outcome statement, rationale, alternatives, full basis ("displayed, not collapsed") | No — payload-only |
| §3.6 Validation | plan revision activities; claim reasoning; correction links | No — payload-only, and superseded claims carried no outcome/reasoning/correction attribution |

Two decisive confirmations grounded this scope, not inference:

- No application query ever read `StructuredContentRepository.Get`, though
  the method already existed. Capability specification content was written,
  digest-verified, and unreadable through any query.
- `RecordEnvelope` projected `Outcome`, `CriterionKeys`, `EvidenceKeys`,
  `ExecutionKeys`, `CorrectionKind`, `CorrectionTargetID`, `StateID` — never a
  decision's `Question`/`OutcomeStatement`/`Alternatives`/`Assumptions`/
  `Constraints`/`Uncertainties`, nor a claim's `Reasoning`. Those exist only
  inside `Payload`.

## 3. Two classes of missing content

**Class A — FeatureForge-owned, already stored, never read.** Capability
specification content. `StructuredContentRepository.Get(ctx, RevisionKey)`
already existed; no PEOS involvement, no new repository method. Only an
application query and a transport DTO were missing.

**Class B — PEOS-owned, inside `Payload`.** Requirement statements, decision
detail, plan activities, claim reasoning. `internal/engineering/peos` already
carried a complete, round-trip-tested decoder set (`DecodeRequirementRevision`,
`DecodeDecision`, `DecodePlanRevision`, `DecodeClaim`, …), with no caller
outside its own tests. This was not new PEOS integration; it was projecting
what was already decodable, through the seam that already existed.

## 4. Decode at read time, not write time (AD-027)

Decision, alternatives, and full rationale recorded in
[AD-027](../decisions/README.md#ad-027--read-surface-content-is-projected-on-read-never-stored-through-a-sibling-engineeringprojector-port).
Summary: project on read by decoding the stored payload; add no projected
field at write time, no migration, no backfill. Adapter parity is structural
— both adapters persist the same `Payload` bytes, and decoding happens above
the adapter boundary.

## 5. Where decoding happens, and how it reaches the caller

**Decoding stays in `internal/engineering/peos`** (AD-005,
`TestOnlyIntegrationPackageImportsPEOS`, extended by FF-019 to cover `cmd/`
too). New file `internal/engineering/peos/projections.go` adds four
functions — `ProjectRequirementStatement`, `ProjectDecisionDetail`,
`ProjectPlanActivities`, `ProjectClaimReasoning` — each decoding via the
existing `Decode*` functions and returning PEOS-free types.

**New PEOS-free projection types**, `internal/engineering/projections.go`:

```go
type DecisionDetail struct {
    Question, OutcomeStatement, Rationale string
    Alternatives, Assumptions, Constraints, Uncertainties []string
}

type PlanActivityDetail struct {
    Key, Method, OutcomeInterpretation string
    ExpectedEvidence []string
}
```

A requirement's statement and a claim's reasoning need no dedicated type —
each is a single string. Names avoid `TestNoShadowStructNames`'s forbidden
set (`Decision`, `Claim`, `Requirement`, …) and `TestNoPEOSTypeIsCopied`'s
suspicious field-set check; neither carries a mirror of the PEOS object, only
what a screen renders.

**Exposed through a new sibling port**, `application.EngineeringProjector`
(`internal/application/projector.go`) — declared in engineering types only,
implemented structurally by `peos.Recorder` via four thin delegating methods
(`internal/engineering/peos/recorder.go`), the exact pattern
`VerifyContentDigest` already established. Kept separate from
`EngineeringRecorder` so a query dependency never implies write authority.

**What needed no new mechanism.** A decision's evidence list
(`RecordEnvelope.EvidenceKeys`), a claim's criterion keys
(`RecordEnvelope.CriterionKeys`), and a claim's correction target
(`RecordEnvelope.CorrectionTargetID`/`CorrectionKind`) were already projected;
the transport DTO layer reads them directly. `EngineeringStateInput` already
discovered `PlanArtifactID` (`ResolveApplicableValidationPlanID`, §6.6's
exactly-one contract) and discarded it before this document; it is rendered
instead of re-derived.

## 6. Application layer — as implemented

- `EngineeringStateInput` gains `PlanArtifactID string`.
- `EffectiveRequirement` gains `Statement string`; `ApplicableDecision` gains
  `Detail engineering.DecisionDetail`; `EngineeringStateResult` gains
  `ValidationPlan ValidationPlanResult` (`Found bool; ArtifactID, RevisionID
  string; Activities []engineering.PlanActivityDetail`).
- `GetFeatureEngineeringState` takes a new `projector EngineeringProjector`
  parameter: decodes each effective requirement's statement, each applicable
  decision's detail, and — when `PlanArtifactID != ""` — the plan's one
  revision's activities (a plan artifact carries exactly one revision; no
  command revises a validation plan). A `decorateReadinessReasoning` pass
  fills in the current claim's and every rejected claim's reasoning after
  `ResolveReadiness` returns, since `ResolveReadiness` itself has no
  projector and many existing tests call it directly.
- `RejectedClaim` (`query_correction.go`) gains `Outcome`, `CorrectedBy`,
  `Reasoning`. `ResolveCurrentClaim`'s existing correction-edge computation
  already had everything needed for `Outcome`/`CorrectedBy`; no decode
  required there.
- `GetFeatureOverview`/`GetFeatureEngineeringStateForCard` take the new
  projector parameter and thread it through `resolveFeatureCardAndState`,
  which also stops discarding `components.planArtifactID`.
- `RevisionWithContent{Revision, Content, HasContent}` replaces the bare
  `engineering.RevisionEnvelope` `GetCapabilityRevisions`/`GetCapabilityRevision`
  returned; both now also fetch `StructuredContent.Get` inside the same
  `uow.Do`.
- One new sentinel, `ErrStoredPayloadUnreadable`
  (`internal/application/errors.go`), wrapping any projector decode failure.

Every change above is additive to an existing signature or type except the
two `GetFeatureOverview`/`GetFeatureEngineeringStateForCard` parameter
additions and the `GetCapabilityRevisions`/`GetCapabilityRevision` return-type
change — both have exactly one caller each in `internal/transport/http`,
updated in the same change.

## 7. Transport layer — as implemented

Every change is additive to FF-018's frozen contract; no existing field
changes name, type, or meaning.

| FF-001 need | Where |
|---|---|
| Revision specification content | `revisionDTO.content` (new `*contentDTO`, reusing C3/C4's request shape via new `mapContentDTOFromContent`), rendered only by the new `mapRevisionWithContentDTO` — Q6 and Q7 only. Q4's `current_revision` (via `mapRevisionDTO`) deliberately carries no content: `CurrentRevisionResult` has no content lookup, and adding one would have widened a much more broadly-used type for a field only Q6/Q7 need. |
| Requirement statement | `effectiveRequirementDTO.statement` |
| Decision detail | `applicableDecisionDTO` gains `question`, `outcome_statement`, `rationale`, `alternatives[]`, `basis{evidence[], assumptions[], constraints[], uncertainties[]}` — `basis.evidence` reads the existing `EvidenceKeys` projection, not a decode |
| Applicable validation plan + activities | `engineeringStateDTO.validation_plan` (new `validationPlanDTO`; its activity type is named `planActivityDetailDTO`, distinct from `dto_command.go`'s request-side `planActivityDTO`) |
| Current claim detail | `readiness.per_requirement[]` gains `reasoning`, `criterion_keys[]` (from `Claim.CriterionKeys`), `corrects` (from `Claim.CorrectionTargetID` when `Claim.HasCorrection()`) |
| Superseded claims "shown, not hidden" (FF-001 §3.6) | `rejected[]` gains `outcome`, `reasoning`, `corrected_by` |
| Execution/claim references | `timelineFromRecord` (`query_timeline.go`) appends the record's own `EvidenceKeys`/`ExecutionKeys` onto `References`, alongside the existing `SubjectKey` entry — an existing `[]string` field, no new field |

`ErrStoredPayloadUnreadable` is mapped to `500`/`internal_error`/opaque in
`errorMappings`, forcing `TestErrorMappingIsExhaustive` to stay honest.

**The HTTP surface remains exactly nineteen operations.** An earlier draft of
this document proposed a twentieth, `GET /features/{id}/validation`. A
dedicated endpoint-minimization review (recorded in AD-027) found no query
failed to own its datum — Q4 already discovered and discarded the applicable
plan; Q5 already emitted every validation act — and that a dedicated endpoint
would have been a screen-shaped (BFF) grouping, the same shape AD-022 already
rejected alongside GraphQL. §3.6's Validation screen is a UI composition of
Q4 + Q5, which is already the norm: §3.2's own "last five timeline events"
makes the Feature overview screen a composition of Q3 + Q5.

`Dependencies` gains `Projector application.EngineeringProjector`, wired in
`cmd/featureforge/main.go` and every test fixture from the same
`peos.NewRecorder()` value already used for `Recorder` — one value
structurally satisfies both ports.

## 8. What remains hidden

Not a privacy matter — a single-user local POC (FF-001 §3, AD-001). Raw
`Payload` bytes/`PayloadDigest`, integrity-mechanism internals beyond the
exposed value, PEOS vocabulary namespaces beyond what was already on the wire
(`peos:satisfied` etc. remain unmapped, per AD-022's "the wire value is the
domain value"), `LocalActorRef` construction detail, and lifecycle
definition-version internals. Nothing FF-001 §3 asks a reader to see is
hidden or collapsed.

## 9. Errors, versioning, forward compatibility

`ErrStoredPayloadUnreadable` (§6) is the one new failure mode: a stored
payload that will not decode is server-side data corruption, not a client
mistake. `TestDecodeToleratesUnknownFields` and
`TestDecodeRejectsCorruptDiscriminator` (pre-existing, `internal/engineering/peos`)
already proved the underlying `Decode*` functions tolerate forward-compatible
additions and reject genuine corruption; the new `Project*` functions inherit
both properties unchanged. A missing `StructuredContent` row for an existing
revision is not an error: it renders an explicit empty state.

## 10. Architecture guards

No guard weakened; no new guard category needed. Verified, all green,
unmodified:

- `TestOnlyIntegrationPackageImportsPEOS` — still exactly one direct PEOS
  importer.
- `TestEngineeringDoesNotImportPEOS` / `TestApplicationDoesNotImportPEOS` /
  `TestTransportDoesNotImportPEOS` — the new types stay PEOS-free at every
  layer.
- `TestNoShadowStructNames`, `TestNoPEOSTypeIsCopied` — passed on first
  write of the new types; their naming/field-set constraints were treated as
  design input (§5), not discovered as a late failure.
- `TestErrorMappingIsExhaustive` — passed once `ErrStoredPayloadUnreadable`
  was added to `errorMappings`.
- `TestGoModHasOnlyApprovedRequirements` — unchanged; no new dependency.

## 11. Tests — as implemented

- **peos projection tests** (`internal/engineering/peos/projections_test.go`):
  round-trip fidelity for all four projections against `buildFixtures`'
  known input values, plus `TestProjectionsRejectCorruptPayload` proving
  every projection surfaces a decode error on a corrupt payload rather than
  a zero-value success.
- **application tests** (`internal/application/read_surface_test.go`):
  `GetFeatureEngineeringStateForCard` renders every FF-020 field correctly
  end to end; a card with no linked capability yields the well-formed empty
  read surface (`ValidationPlan.Found` false, no error); `GetCapabilityRevision`
  proves both a present and an absent `HasContent`; a payload corrupted after
  a real, valid write (the only way to construct the case, since no command
  can produce a genuinely undecodable payload) surfaces
  `errors.Is(err, ErrStoredPayloadUnreadable)`.
- **transport tests** (`internal/transport/http/queries_test.go`):
  `TestGetFeatureStateHandler_ReadSurfaceContent` proves every new Q4 field
  through a real handler; `TestListCapabilityRevisionsHandler`/
  `TestGetCapabilityRevisionHandler` extended for `content`;
  `TestGetFeatureTimelineHandler` extended for reference enrichment.
- **canonical scenario** (`internal/transport/http/scenario_http_test.go`,
  the shared `assertCanonicalEndStateThroughHTTP` body): extended to assert
  every effective requirement's statement is non-empty, DEC-1's question/
  outcome_statement/alternatives, `validation_plan.found` with all three
  activities, REQ-2's current claim (CLM-4) carries its reasoning and
  `corrects` CLM-2, and CLM-2 appears in `rejected[]` with its original
  outcome and reasoning intact and `corrected_by` naming CLM-4 — run through
  HTTP, on both adapters (`TestCanonicalScenarioThroughHTTP`,
  `TestCanonicalScenarioThroughHTTPPostgres`, one shared assertion body, as
  FF-018 §16 step 10 established).
- **Contract suite**: unaffected. `StructuredContent.Get` already existed;
  no repository method changed.

## 12. Verification

```
gofmt -l .                        → clean
go vet ./...                      → clean
go build ./...                    → clean
go test ./... -count=1            → ok, all packages
go test ./... -race -count=1      → ok, all packages
make postgres-test                → exit 0, zero failures, both adapters
```

## 13. What this document deliberately does not decide

- **Phase B in any respect** — screens, templates, forms, UI routes, or
  rendering technology. FF-015 §6.4 sketches them; Phase B UI planning
  resumes against this document's completed contract.
- **Pagination, caching, content negotiation** — FF-015 §19's non-decisions
  carry forward unchanged; FF-020's collections remain single-digit.
- **Whether execution records need their own dedicated response shape** —
  the timeline's existing per-event rendering (§7) covers FF-001 §3.6's
  "execution records with outcomes and evidence"; a dedicated execution-detail
  projection was not required and was not added.
- **Materialization of any FF-020 field** — AD-006 still requires measured
  evidence before anything computed becomes stored.

## 14. Open questions

None remaining. The one recorded during planning — whether decision/claim/plan
projections belong on `EngineeringRecorder` or a sibling port — is resolved by
§5/AD-027 in favour of the sibling port.

## Verdict

**Implemented.** All thirteen sections above reflect the landed change, not a
plan for one. Phase B UI planning may resume against this document's
completed read contract.
