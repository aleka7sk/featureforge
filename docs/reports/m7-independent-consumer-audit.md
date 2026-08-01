# M.7 Independent End-to-End PEOS Consumer Audit

Status: Freeze artifact candidate; remediation is published and independent
closure passed, final freeze publication gate pending
Date: 2026-08-01
Phase: M.7
Governs: nothing. This report records an adversarial review and its evidence;
the normative sources remain `docs/spec/`, then `docs/decisions/`, then tests,
then implementation.

---

## 1. Scope, method, and initial result

The audit attempted to falsify every line of
[FF-001 §6](../spec/001-poc-acceptance-contract.md#6-acceptance-contract),
then reviewed the canonical scenario, both persistence adapters, HTTP and UI
consumer journeys, the M.6 proposal boundary, architecture guards, and the
documentation a new consumer would actually read. It treated a passing test as
evidence only when that test exercised the public boundary named by the
contract. A fixture that reached around that boundary was evidence of the
stored result, but not evidence that the real consumer path could produce it.

The first audit result was:

```text
M.7 NOT READY TO FREEZE
0 BLOCKER · 5 MAJOR · 4 MINOR
```

The nine findings are preserved in §3, including their original severity.
Independent re-audit has closed the remediation with no open finding. The
remaining gates are immutable commit/tree identity and green publication CI.

The audit did not evaluate production authentication, tenancy, collaboration,
notifications, billing, generalized project management, or a Belcanto
application design. Those are expressly beyond the FeatureForge POC and begin
only after freeze.

### 1.1 Evidence classes

| Class | Meaning |
|---|---|
| Normative | An accepted FF specification or architecture decision states the obligation. |
| Executable | A named test exercises the obligation and fails when the behavior is absent. |
| Structural | Static import, vocabulary, SQL, interface, or method-set inspection makes a forbidden dependency or mutation observable. |
| Journey | The complete scenario crosses the real application, HTTP, or rendered-UI boundary rather than seeding the desired state behind it. |
| Publication | The exact published commit/tree passes the repository workflow, including PostgreSQL variants. |

No single class substitutes for all the others. In particular, local executable
evidence does not identify the published tree, and a journey does not replace
the static architecture checks.

### 1.2 Independent remediation closure

The first re-audit correctly rejected an earlier candidate with four MAJOR and
two MINOR findings. After those were repaired, a strict second pass found one
remaining executable-evidence gap and two MINOR defects: the canonical
timeline had no positional 28-event oracle, malformed C9 activity syntax was
classified only after an API lookup, and FF-021 contained a damaged sentence.
The final delta re-audit independently verified all three corrections and
returned:

```text
M.7 REMEDIATION RE-AUDIT — READY
0 BLOCKER · 0 MAJOR · 0 MINOR
```

The exact canonical test now compares every position by `EventKind` and
`SourceIdentity`; malformed C9 syntax returns `400` before any API call; and
the normative Timeline-navigation sentence is complete.

A separate independent review of the three freeze artifacts returned
`0 BLOCKER · 0 MAJOR · 0 MINOR · 1 OBSERVATION`. It confirmed the no-library
boundary, reuse/redesign split, PEOS consumer gaps, and deliberate stop before
Belcanto application/authentication design. Its sole terminology observation
was resolved by distinguishing the completed remediation re-audit from the
freeze-artifact review throughout this report.
The reviewer's final editorial delta check returned
`0 BLOCKER · 0 MAJOR · 0 MINOR · 0 OBSERVATION`.

## 2. Baseline and gate ledger

Unknown values are deliberately explicit. They must be replaced with immutable
identities or final outcomes before this report can certify freeze.

| Evidence | Value | Disposition |
|---|---|---|
| Initial M.7 audit baseline commit | `64cd1a5a7c37cfe632fbc3f2f28a9eb046830ffc` | Published verification baseline |
| Initial M.7 audit baseline tree | `57fb48d8094c931510c80609b4919dc4b8b51895` | Immutable audited tree |
| M.6 entrance-gate commit/tree | `395e163180649dcb5735b006ef6e225c80570a69` / `e2960d8504fdaf9ef39ce748bde5dce2174aa89f` | Published implementation |
| M.6 workflow run | [30690421922](https://github.com/aleka7sk/featureforge/actions/runs/30690421922) — `success` | Entrance gate passed |
| M.7 remediation commit/tree | [`40644fc61859f3ea4173d3904680161e01e048ec`](https://github.com/aleka7sk/featureforge/commit/40644fc61859f3ea4173d3904680161e01e048ec) / `bd1468fdaf4d3164df0c96cd582067d96585b2e9` | Published remediation snapshot |
| First M.7 remediation workflow | [30693514500](https://github.com/aleka7sk/featureforge/actions/runs/30693514500) — `failure` | Normal PostgreSQL gate passed; the race package reached its obsolete `20m` timeout while still making progress, with no data race |
| Workflow-only gate fix commit/tree | [`f3f5deda4f4203b404b5bff7d9a3bb9d4e4a4091`](https://github.com/aleka7sk/featureforge/commit/f3f5deda4f4203b404b5bff7d9a3bb9d4e4a4091) / `5daff1d0e0c56188ab1918c94076a6ccb744cfe1` | Finite package/job timeouts only; independent delta re-audit `READY — 0 BLOCKER · 0 MAJOR · 0 MINOR` |
| M.7 remediation publication workflow | [30694521798](https://github.com/aleka7sk/featureforge/actions/runs/30694521798) — `success` | Exact workflow-fix tree passed formatting, vet, build, full PostgreSQL, and full race gates |
| Local normal verification | `gofmt`, `git diff --check`, `go vet ./...`, `go build ./...`, and `go test ./... -count=1` — `PASS` | Final local normal gate passed |
| Local race verification | `go test ./... -race -count=1 -timeout=20m` — `PASS` (UI `589.594s`) | Final local race gate passed on remediation tree |
| Independent remediation re-audit | `M.7 REMEDIATION RE-AUDIT — READY` — `0 BLOCKER · 0 MAJOR · 0 MINOR` | Independent closure passed |
| Freeze-artifact commit/tree | `<PENDING-M7-FREEZE-COMMIT>` / `<PENDING-M7-FREEZE-TREE>` | Pending publication |
| Final GitHub workflow | `<PENDING-M7-FINAL-WORKFLOW-RUN-URL>` — `<PENDING-M7-FINAL-CI-CONCLUSION>` | Required before freeze |
| Published branch/ref | `<PENDING-M7-PUBLISHED-REF>` | Pending publication |

## 3. Findings and remediation disposition

### 3.1 Summary

| Finding | Original severity | Short description | Candidate disposition |
|---|---:|---|---|
| M7-01 | MAJOR | Canonical HTTP/UI journeys manufactured an Evidence record behind the public act surface | Closed by independent re-audit |
| M7-02 | MAJOR | Literal FF-001 screen data was absent or incomplete | Closed by independent re-audit |
| M7-03 | MAJOR | Timeline events and UI did not carry the complete source, actor, reference, ordering, and navigation explanation | Closed by independent re-audit |
| M7-04 | MAJOR | Ambiguity failures did not name the conflicting identities | Closed by independent re-audit |
| M7-05 | MAJOR | Multi-hop correction rationale traversed the correction graph in the wrong direction | Closed by independent re-audit |
| M7-06 | MINOR | Immutability guards omitted lifecycle-policy repositories and tables | Closed by independent re-audit |
| M7-07 | MINOR | Malformed criterion/activity form lines were silently discarded | Closed by independent re-audit |
| M7-08 | MINOR | The closed FeatureForge vocabulary test omitted the AI-assisted provenance method | Closed by independent re-audit |
| M7-09 | MINOR | FF-001 required invented rationale even for direct inventory/stored-value reads | Closed by independent re-audit |

### 3.2 M7-01 — canonical journey bypassed the act surface

**Finding.** The canonical HTTP and UI journeys directly seeded an Evidence
Artifact/Revision solely so that `DEC-1` could cite it. That bypass meant the
journey did not prove that the published intent API and UI could produce the
claimed canonical end state.

**Candidate remediation.** `DEC-1` now forward-cites
`EV-1/EV-1-REV-1`, the one governed unresolved Decision-basis exception. The
later A-1 C10 validation act creates exactly that pair together with `ER-1`.
The canonical set is `EV-1` through `EV-4`; no standalone Evidence writer or
repository seed remains in the canonical application, HTTP, form, or browser
journey. See `internal/scenario/scenario.go`,
`internal/transport/http/scenario_http_test.go`, and
`internal/ui/browser_scenario_test.go`.

**Candidate verification.** `TestCanonicalScenario`,
`TestCanonicalScenarioThroughHTTP`, `TestCanonicalScenarioThroughUIForms`,
`TestCanonicalScenarioThroughUIBrowser`, and their PostgreSQL variants. The
end-state assertion requires four Evidence artifacts and requires `DEC-1` to
name the A-1 Evidence pair.

### 3.3 M7-02 — literal UI acceptance gaps

**Finding.** The screens were navigable but did not expose all data named
literally in FF-001 §3: project feature counts, the exact readiness Claim ID,
prior Requirement revisions, a Decision subject, plan identity, claim criteria,
and an actual correction target link.

**Candidate remediation.** Q4 now returns fully validated
`requirement_history` and the payload-verified Decision `subject_key`. The UI
composes Q1 with Q2 for project counts and renders all named fields, including
fragment links between corrected and correcting claims. Relevant tests are
`TestProjectsPageShowsFeatureCount`,
`TestRequirementsPageShowsEveryPriorRevision`,
`TestGetFeatureEngineeringStateForCard_RequirementHistoryIsAuthoritativeAndOrdered`,
`TestGetFeatureStateHandler_RequirementHistoryAndDecisionSubject`,
`TestDecisionsPageShowsFullBasis`, and
`TestValidationPageShowsSupersededClaim`.

### 3.4 M7-03 — incomplete timeline explanation

**Finding.** Not every event had an actor and its own source reference;
lifecycle events omitted assignment/transition relationships; equal-time and
undated placement were under-explained; and the UI fabricated too little useful
navigation from references.

**Candidate remediation.** Every event now carries a non-empty actor, exact
`SourceIdentity`, and its own source among `References`. Revision, record, and
lifecycle events add their governed subject, criterion, evidence, execution,
correction, policy, predecessor, assignment, and transition references as
applicable. Equal-time rationale names both `kind-rank` and
`source-identity`; undated rationale explains why the event is placed above
dated history. UI routing gives every canonical reference an honest target: a
dedicated Project or Feature page, the exact source event, the validated
embedded lifecycle-policy representation, or (only for the wholly absent C8
Evidence pair) a pending representation backed by the citing Decision. After
C10 materializes that pair, the same URL resolves to the actual Evidence source
event. Unknown identities return `404`; partial, unreadable, or contradictory
occupancy fails with stored-state integrity rather than rendering a broken
link.

**Candidate verification.** `TestEveryTimelineEventExposesActorAndOwnSourceReference`,
`TestEqualTimeOrderingExplainsKindRankAndSourceIdentity`,
`TestUndatedGroupIsSeparate`,
`TestLifecycleTimelineEventNamesAssignmentAndTransitionSources`, and
`TestTimelineTemplateRendersUndatedFirstAndNavigableSourceReferences`.

### 3.5 M7-04 — ambiguity diagnostics hid conflicting identities

**Finding.** Correction and lifecycle ambiguity errors stated that a conflict
existed but did not identify the stored records a consumer must inspect.

**Candidate remediation.** Competing correction heads and correction cycles
name the sorted conflicting Claim IDs; non-cyclic tails are excluded from the
cycle population. Lifecycle duplicate entries, branches, and cycles name
sorted assignment IDs together with exact transition Revision keys. The
ordering makes messages deterministic as well as actionable.

**Candidate verification.** `TestCompetingHeads`, `TestCycle`,
`TestLifecycleStateDuplicateEntryDiagnosticNamesAssignmentsAndTransitions`,
`TestLifecycleStateRejectsBranchedHistory`, and
`TestLifecycleStateCycleDiagnosticNamesAssignmentsAndTransitions`.

### 3.6 M7-05 — correction rationale walked the graph backwards

**Finding.** The selected head could be correct while the prose described the
wrong correcting relationship for a chain longer than one edge.

**Candidate remediation.** Rationale construction starts at the selected head,
walks each head-to-target edge to the original, and then emits the explanation
oldest-to-head. Each edge says whether the newer Claim corrected, replaced, or
invalidated its target. Selection still ignores timestamps and follows
correction structure only.

**Candidate verification.** `TestChainOfTwo`, `TestChainOfThree`,
`TestCorrectionRationaleUsesTheRecordedCorrectionKind`,
`TestSelectionIgnoresTimestamps`, and `TestOriginalRemainsReadable`.

### 3.7 M7-06 — incomplete immutability guards

**Finding.** The adapter-level no-update/no-delete proof covered ordinary
engineering envelopes but omitted `LifecycleDefinitionRepository` and the
PostgreSQL `lifecycle_definitions` and `lifecycle_definition_versions` tables.

**Candidate remediation.** The memory method-set guard includes
`LifecycleDefinitionRepository`; the SQL scan includes both lifecycle tables.

**Candidate verification.** `TestNoUpdateOrDeleteMethodExists` in
`internal/infrastructure/memory/store_test.go` and
`TestNoUpdateOrDeleteOnEngineeringTables` in
`internal/architecture/architecture_test.go`.

### 3.8 M7-07 — malformed form lines were silently lost

**Finding.** A malformed `key: text` criterion or six-field plan activity line
was dropped before the API call. A successful command could therefore persist
different engineering content from the content the person submitted.

**Candidate remediation.** Form parsing returns `400 Bad Request`, invokes no
API call, and performs no write. Domain validation remains command-owned; only
lossless form-syntax parsing is UI-owned.

**Candidate verification.** `TestCapabilityFormRejectsMalformedCriterionInsteadOfDroppingIt`,
`TestPlanFormRejectsMalformedActivityInsteadOfDroppingIt`, and
`TestPlanFormRejectsMalformedActivityBeforeAnyAPICall` in
`internal/ui/form_syntax_test.go`.

### 3.9 M7-08 — closed vocabulary omitted AI assistance

**Finding.** `featureforge:ai-assisted` was a real product vocabulary value but
was missing from the declared/closed vocabulary set, weakening the namespace
proof added for M.6.

**Candidate remediation.** `ProvenanceMethodAIAssisted` is included in both the
declared values and exact closed set.

**Candidate verification.** `TestAllVocabularyValuesUseFeatureForgeNamespace`,
`TestNoDuplicateVocabularyDeclarations`, and `TestVocabularySetIsClosed`.

### 3.10 M7-09 — direct reads were required to invent rationale

**Finding.** FF-001's blanket sentence that every query returns a rationale
conflicted with the established Q1, Q2, and Q7 contracts. Those queries expose
authoritative stored values or inventory; no derivation exists to explain.

**Candidate remediation.** FF-001, FF-004, and FF-015 now require rationale for
derived or interpretive results and explicitly exclude direct Q1, Q2, and Q7
reads from invented rationale. Derived current revision, correction, readiness,
lifecycle, and timeline answers remain explained.

**Candidate verification.** Normative reconciliation in
[FF-001 §2 and §6.4](../spec/001-poc-acceptance-contract.md), plus
`TestReadinessRationaleIsPerRequirement`, `TestChainOfThree`, and the timeline
rationale tests named under M7-03.

## 4. FF-001 §6 evidence matrix

Every row below corresponds to one checkbox in FF-001 §6. "Candidate
evidenced" means the repository contains direct evidence after remediation; it
does **not** mean the final publication gate has passed. The independent
remediation and freeze-artifact reviews have passed separately.

### 4.1 Architecture (§6.1)

| ID | Contract line | Direct evidence | Audit disposition |
|---|---|---|---|
| A1 | PEOS SDK unchanged; no local replacement | `go.mod` pins `github.com/aleka7sk/PEOS v1.0.0` with no `replace`; `TestGoModHasOnlyApprovedRequirements`; final diff must contain no PEOS-module path | Candidate evidenced; final tree check pending |
| A2 | Domain does not import PEOS, directly or transitively | `TestDomainDoesNotImportPEOS`, `TestApplicationDoesNotImportPEOS`, `TestEngineeringDoesNotImportPEOS`, `TestOnlyIntegrationPackageImportsPEOS` | Candidate evidenced; final gate pending |
| A3 | Product vocabulary is outside the `peos` namespace | `TestAllVocabularyValuesUseFeatureForgeNamespace`, `TestVocabularySetIsClosed`, `TestClaimTypeIsNotExtended`; M7-08 closes the AI-assisted omission | Independently closed; final gate pending |
| A4 | No PEOS type copied, restated, or shadowed | `TestNoPEOSTypeIsCopied`, `TestNoShadowStructNames`, and the PEOS-free envelopes/ports under `internal/engineering` and `internal/application` | Candidate evidenced; final gate pending |
| A5 | Package boundaries fail the build | `internal/architecture/architecture_test.go` and `proposal_boundary_test.go`, including import-holder, driver, UI, retry-safety, and proposal-authority guards | Candidate evidenced; deliberate/final guard run pending |

### 4.2 History (§6.2)

| ID | Contract line | Direct evidence | Audit disposition |
|---|---|---|---|
| H1 | Revision 1 remains fully inspectable after Revision 2 | `TestCanonicalScenario` re-reads Revision 1 content and verifies its digest; `TestRevisionsPageShowsBothRevisionsIndependently` and `TestRevisionsTemplateShowsBothRevisionsIndependently` render both | Candidate evidenced; adapter publication gate pending |
| H2 | No immutable engineering record is updated or deleted | Memory `TestNoUpdateOrDeleteMethodExists`; SQL `TestNoUpdateOrDeleteOnEngineeringTables`; shared `IdempotentIdenticalPut`/`ConflictingPut`; M7-06 adds lifecycle policy | Independently closed; final gate pending |
| H3 | Correction adds history and preserves its target | `TestOriginalRemainsReadable`; canonical `CLM-2`/`CLM-4` assertions; `TestValidationPageShowsSupersededClaim` | Candidate evidenced; final gate pending |
| H4 | Timeline explains every canonical engineering act | `TestCanonicalScenario` compares all 28 positions by exact `EventKind + SourceIdentity`; `TestEveryTimelineEventExposesActorAndOwnSourceReference` and the M7-03 tests prove explanation fields | Independently closed |

### 4.3 Persistence (§6.3)

| ID | Contract line | Direct evidence | Audit disposition |
|---|---|---|---|
| P1 | Every PEOS value used by the scenario persists and reloads | Shared `PutThenGet`, codec `TestRoundTrip_*` cases for Artifact, Revisions, Decision, Plan, Execution, Claim, Correction, State Assignment, and Transition; canonical scenarios on both adapters | Candidate evidenced; PostgreSQL workflow pending |
| P2 | JSON round trip preserves equality and canonical bytes | `internal/engineering/peos/codec_test.go`; `TestCanonicalJSONStable`, `TestContentRoundTrip`, and `TestPayloadIsStoredByteIdentical` | Candidate evidenced; final gate pending |
| P3 | Identical duplicate write is idempotent | Shared `IdempotentIdenticalPut`, `RevisionSubjectBearingPutIsIdempotent`, and `TestC1ThroughC12ReplayAfterClockAdvance` | Candidate evidenced; final gate pending |
| P4 | Conflicting immutable write is distinguishable | Shared `ConflictingPut`/`ConflictAbortsAct`; `TestCommandConflictingReplay`; exhaustive HTTP error mapping | Candidate evidenced; final gate pending |
| P5 | Mandatory references resolve, except governed C8 citation | Shared `ReferenceVerification`; command integrity suites; canonical `DEC-1` forward-cites EV-1 and C10 later materializes it; Q5 keeps the wholly absent pair as a pending exact link and rejects partial, unreadable, or contradictory occupancy | Independently closed; final gate pending |
| P6 | PostgreSQL passes the same contract suite as memory | `TestRepositoryContractSuite` and `TestPostgresRepositoryContractSuite` both call `contracttest.RunRepositoryContractSuite`; PostgreSQL canonical, replay, HTTP, UI, proposal, and rollback variants | Candidate evidence exists; final PostgreSQL workflow required |

### 4.4 Queries (§6.4)

| ID | Contract line | Direct evidence | Audit disposition |
|---|---|---|---|
| Q1 | Current revision resolves deterministically | `TestSequenceOneThenTwo`, `TestIgnoresRevisionIDLexicalOrder`, `TestResolutionIsDeterministic` | Candidate evidenced; final gate pending |
| Q2 | Insertion order does not affect resolution | `TestInsertionOrderIndependence`, `TestCanonicalScenarioInsertionOrderIndependence`, and `TestCanonicalScenarioPostgresInsertionOrderIndependence` | Candidate evidenced; final PostgreSQL gate pending |
| Q3 | Ambiguity fails explicitly and names conflicts | `TestCompetingHeads` and `TestCycle` name sorted Claims; lifecycle duplicate-entry/branch/cycle tests name assignments and transition revisions; revision-order ambiguity tests reject duplicates | Independently closed; final gate pending |
| Q4 | Current claim follows correction chains and rejects invalid graphs | `TestChainOfTwo`, `TestChainOfThree`, `TestCorrectionRationaleUsesTheRecordedCorrectionKind`, `TestInvalidatorEvaluatedOnOwnMerits`, `TestCycle`, `TestMissingTarget`, `TestSelfCorrection` | Independently closed; final gate pending |
| Q5 | Every derived answer has rationale; direct reads do not invent one | Current-revision, correction, readiness, lifecycle, and timeline result tests inspect rationale; FF-001 explicitly classifies Q1/Q2/Q7 as direct | Independently closed; final gate pending |

### 4.5 Application (§6.5)

| ID | Contract line | Direct evidence | Audit disposition |
|---|---|---|---|
| APP1 | One complete canonical lifecycle works through API and UI | Application, HTTP, form, and scraped-browser canonical tests; memory and PostgreSQL variants; M7-01 removes the hidden Evidence seed | Candidate evidenced; final workflow pending |
| APP2 | An unfamiliar reader can understand state and history from UI alone | Literal page/render tests for all seven screens, correction links, prior revisions, full Decision basis, plan/claim detail, readiness rationale, and timeline navigation; browser journey | Independently closed; final gate pending |
| APP3 | No operational Belcanto entity is required or present | `TestNoOperationalScenarioEntity`, `TestNoDerivedStateOnFeatureCard`, and the canonical scenario's Project/FeatureCard-only operational roots | Candidate evidenced; final gate pending |
| APP4 | No generic workflow/rule/expression engine exists | `TestNoForbiddenPackageNames`; lifecycle is a fixed persisted policy with consumer validation, not user-defined rules; code/package inspection | Candidate evidenced; final tree inspection pending |

### 4.6 AI (§6.6)

| ID | Contract line | Direct evidence | Audit disposition |
|---|---|---|---|
| AI1 | Context pack can be generated for the canonical feature | `TestAssembleProposalContextUsesExactCurrentEngineeringState`, `TestCanonicalContextRoundTripAndGoldenDigest`, and API/UI proposal journeys | M.6 publication gate passed; final freeze publication gate pending |
| AI2 | Every context element names its exact source | Proposal constructors reject missing/unknown/unpaired sources; persisted-witness tests validate pack membership; `TestDeterministicGeneratorIsByteStableAndNamesGaps` | Candidate evidenced; final gate pending |
| AI3 | Proposal has no authority without explicit human acceptance | `TestProposalPackageIsPure`, `TestProposalCallGuardRejectsIO`, `TestProposalImportAllowlistRejectsAuthorityPackages`; generation purity test; UI generate/discard/accept test | Candidate evidenced; final gate pending |
| AI4 | Accepted proposal preserves provenance | AI witness canonical round-trip tests; `TestAcceptCapabilityProposalCreatesDraftThenReplaysBeforeFreshness`; HTTP revision DTO test; memory/PostgreSQL adapter-parity flows | Candidate evidenced; final gate pending |

### 4.7 Transition to Belcanto (§6.7)

| ID | Contract line | Direct evidence | Audit disposition |
|---|---|---|---|
| B1 | Reusable patterns are rationale, not a library | [Reusable Patterns](reusable-patterns.md) states context, rationale, evidence, limits, and a mandatory no-import/no-copy rule | Freeze-artifact review passed; commit pending |
| B2 | FeatureForge code is not shared infrastructure | Reusable Patterns §1 and [Lessons Learned](lessons-learned.md) classify every recommendation as reasoning to re-evaluate in Belcanto | Freeze-artifact review passed; commit pending |
| B3 | No shared PEOS integration package is created | No such package exists; both handover documents prohibit import, copy, extraction, or a cross-product integration library | Candidate evidenced; final tree check pending |
| B4 | Lessons distinguish reuse from redesign | Lessons Learned §2–§4 separates reusable decision discipline from FeatureForge-local compromises and Belcanto-owned redesign | Freeze-artifact review passed; commit pending |

## 5. Embedded PEOS consumer report

This section is the PEOS-consumer deliverable required by FF-007 M.7. It is
embedded here deliberately; FF-001 permits exactly three freeze artifacts, so
there is no fourth PEOS report.

### 5.1 What was awkward

| Consumer friction | Observed consequence | Appropriate owner |
|---|---|---|
| Entry lifecycle transition construction differs from content-bearing transitions | FeatureForge needed a special entry path and careful validation of the bare establishing Revision | PEOS documentation/API ergonomics backlog; no FeatureForge-side PEOS change |
| `Scope`, ordinary `Subject`, and `LifecycleSubject` are related but distinct shapes | The integration layer needed explicit conversion and could not safely treat them as interchangeable strings | Consumer mapping, aided by clearer PEOS examples |
| Criterion references are broad enough to express several criterion families | FeatureForge had to enforce that satisfaction Claims name the exact Requirement-revision criterion appropriate to this product | Consumer invariant; possibly a PEOS guidance example |
| PEOS supplies values, not persistence, transactions, indexes, or product queries | FeatureForge designed envelopes, repositories, ordering metadata, correction/current-state algorithms, and rationale | Correct separation of ownership, but a material integration cost |

### 5.2 What PEOS deliberately leaves to the consumer

These are not SDK defects discovered by the audit; they are obligations a PEOS
consumer must not assume the constructors discharge.

- A Claim constructor can represent a self-correction reference; FeatureForge
  must reject self-correction and cycles in its command/query rules.
- A transition value can name source and target facts without proving that the
  edge belongs to FeatureForge's configured lifecycle graph; persisted policy
  validation remains consumer-owned.
- There is no native relation from a Requirement Revision to the exact
  capability acceptance criterion that motivated it. FeatureForge therefore
  persists `RequirementCriterionTrace` as product-owned structured state.
- A syntactically valid PEOS reference does not prove the target exists, has the
  expected family, or has an authoritative readable payload. Repository and
  aggregate integrity inspection remain consumer-owned.

### 5.3 Misreads corrected while building FeatureForge

| Misread | Correct interpretation |
|---|---|
| "Result" or "Verdict" should be another entity | Execution outcome and Claim outcome answer different questions; PEOS defines no separate Result/Verdict entity. |
| Lifecycle state means release readiness | Lifecycle records progress through engineering activity; readiness is a derived answer from current requirements and claims. `assessed` can coexist with `not-ready`. |
| Revision acceptance is lifecycle | Acceptance selects authoritative revision text; lifecycle describes the capability's engineering progression. |
| A Claim can stand without Evidence | FeatureForge must construct evidence-backed satisfaction Claims and preserve the exact supporting references. |
| A Decision subject is always a Revision | PEOS permits Artifact and Artifact Revision subjects; consumers must preserve the authoritative subject rather than infer one. |
| `SubjectKey` can always be reconstructed from payload | Some historical/bare shapes do not carry enough payload information; a governed projection may be necessary and must be checked against payload wherever reconstruction is possible. |
| Origin or Derivation is the product trace model | Those values explain provenance/derivation; they do not replace FeatureForge's exact Requirement-to-criterion trace or correction graph. |

### 5.4 Upstream value and the non-library conclusion

Useful PEOS backlog inputs are focused examples and ergonomics: entry lifecycle
construction, the distinctions among subject/scope shapes, criterion-family
guidance, and a prominent statement that references do not establish
referential integrity. FeatureForge does not have evidence for a shared PEOS
integration library. One consumer is a sample of one; its envelopes, ports,
queries, vocabulary, content schemas, and integrity precedence encode product
choices. Belcanto must implement its own boundary after making its own domain
decisions.

## 6. Required closure and publication proof

The remediation candidate is not a freeze until all of the following are
recorded against exact immutable identities:

1. `gofmt -l .` returns no paths and `git diff --check` returns no defects.
2. `go vet ./...` and `go build ./...` pass.
3. `go test ./... -count=1` passes.
4. `go test ./... -race -count=1 -timeout=20m` passes.
5. An independent read-only re-audit finds no open BLOCKER or MAJOR and verifies
   every M7-01…09 disposition against the remediation tree.
6. The exact published freeze commit/tree passes
   `.github/workflows/verify.yml`, including PostgreSQL-backed normal and race
   variants.
7. This ledger is completed without changing the historical first-audit result.

Targeted tests do not replace the full gates, but the re-audit should at least
run or inspect the tests named in §3 and all four canonical boundaries:

```text
internal/scenario
internal/transport/http
internal/ui
internal/architecture
internal/infrastructure/memory
internal/infrastructure/postgres
```

## 7. Current disposition

The remediation candidate addresses all nine original findings and supplies a
direct evidence path for every FF-001 §6 line. Independent remediation and
freeze-artifact review have passed. Publication identities and the final green
workflow remain deliberately unclaimed.

```text
M.7 REMEDIATION RE-AUDIT: READY — 0 BLOCKER · 0 MAJOR · 0 MINOR
FEATUREFORGE FREEZE: <PENDING-PUBLICATION-GATE>
```
