# FF-012 — Test Specification

Status: Accepted (Phase M.2)
Governs: every test M.3 must deliver, what each proves, and the end-to-end
scenario test that demonstrates all 25 M.3 objectives.

## Governance statement

PEOS-000 through PEOS-009 remain authoritative for every PEOS concept.
`docs/spec/` governs FeatureForge. This document does not extend the PEOS
ontology. The implementation must use PEOS v1.0.0 unchanged.

## 0. Conventions

Every test states setup, action, expected result, expected error where
applicable, and the invariant it proves. Tests are table-driven where the shape
repeats. No test calls `time.Now`; all use `FixedClock`. No test uses
`math/rand`. Error assertions use `errors.Is` against the named sentinel, never
string matching.

**A test that cannot fail is a defect.** Every architecture test in §12 is
verified during M.3 against a deliberate violation, and that verification is
recorded in the M.3 report.

## 1. Domain tests — `internal/domain`

| Test | Setup → Action | Expected | Proves |
|---|---|---|---|
| `TestNewProject` | Valid ID and name | Success; accessors return inputs | Construction |
| `TestNewProjectRejectsEmptyID` | Empty ID | `ErrProjectIDRequired` | Zero-value rejection |
| `TestNewProjectRejectsEmptyName` | Empty/whitespace name | `ErrProjectNameRequired` | Trimming applied before validation |
| `TestNewFeatureCard` | Valid inputs | Success | Construction |
| `TestNewFeatureCardRejectsMissingProject` | Empty project ID | `ErrFeatureProjectRequired` | A card cannot be orphaned |
| `TestNewFeatureCardRejectsBlankTitle` | `"   "` | `ErrInvalidFeatureTitle` | Whitespace is not a title |
| `TestFeatureCardHasNoDerivedState` | Reflect over the struct | No field named/typed for current revision, readiness, lifecycle state, or any collection of requirements, decisions, or claims | AD-001 and the FF-002 prohibition |
| `TestDomainHasNoSetters` | Reflect over exported methods | No `Set*`; no pointer-receiver mutator | Operational entities are created once |

## 2. Vocabulary tests — `internal/engineering/peos`

| Test | Expected | Proves |
|---|---|---|
| `TestAllVocabularyValuesUseFeatureForgeNamespace` | Every FeatureForge-declared value has namespace `featureforge` | Namespace ownership |
| `TestVocabularyLocalNamesNonEmpty` | Every value's local name is non-empty and matches `[a-z0-9-]+` | Well-formedness |
| `TestNoDuplicateVocabularyDeclarations` | The declared set has no repeated `namespace:value` | No synonyms |
| `TestVocabularySetIsClosed` | The declared set equals the FF-011 §3 table exactly | Scope containment |
| `TestNoPEOSNamespaceValueIsConstructed` | Parse `internal/` for `core.NewVocabularyValue` calls; no literal first argument is `"peos"` | FeatureForge never mints a PEOS value |
| `TestPEOSConstantsUsedForPEOSVocabulary` | `requirement.SubjectCombinationIndependent`, `core.ClaimTypeSatisfaction`, and the other SDK constants in FF-011 §3 are referenced rather than reconstructed | The verified fact 8 trap is closed |
| `TestClaimTypeIsNotExtended` | No FeatureForge value is passed to `core.NewClaimType` | `core.ClaimType` stays closed |

## 3. Content and digest tests — `internal/engineering`

| Test | Setup → Action | Expected | Proves |
|---|---|---|---|
| `TestContentValidation` | Table of invalid contents | Each returns its named sentinel | Required fields, trimming, key uniqueness, size bounds, schema version |
| `TestCanonicalJSONFieldOrder` | Fully populated content → canonical JSON | Byte-exact expected string, fields in declared order | Determinism |
| `TestCanonicalJSONStable` | Marshal the same content 1 000 times | All outputs byte-identical | No map iteration leaks into output |
| `TestCanonicalJSONEmptyListsAreArrays` | Content with empty optional lists | `[]`, never `null`, never omitted | Round-trip stability |
| `TestCanonicalJSONDoesNotEscapeHTML` | Content containing `<`, `>`, `&` | Characters preserved verbatim | `SetEscapeHTML(false)` applied |
| `TestDigestStability` | Same content, repeated | Identical hex digest | Digest is a function of value |
| `TestDigestSensitivity` | Two contents differing only in list **order** | Different digests | Order is content |
| `TestDigestSensitiveToSchemaVersion` | Same fields, version 1 vs 2 | Different digests | Version participates |
| `TestContentEqualityIsCanonicalBytes` | Two independently built equal contents | Equal | Equality has one definition |
| `TestContentRoundTrip` | Content → canonical JSON → parse → canonical JSON | Byte-identical | No information lost |

## 4. Codec tests — `internal/engineering/peos`

### 4.1 Round trips

For each of the eleven families — `core.Artifact`, `core.ArtifactRevision`,
`requirement.Requirement`, `requirement.Revision`, `decision.Decision` (with
Basis), `validation.Plan`, `validation.PlanRevision`,
`validation.ExecutionRecord`, `validation.Claim`, `validation.Claim` with a
correction reference, and `lifecycle.StateAssignment`:

| Test | Expected | Proves |
|---|---|---|
| `TestRoundTrip_<Family>` | Encode → decode → re-encode is byte-identical | Serialization is lossless |
| `TestDecodeRejectsInvalidJSON` | `ErrStoredPayloadInvalid` | Corrupt payloads fail loudly |
| `TestDecodePreservesReceiverOnFailure` | Target value unchanged after a failed decode | Matches the SDK's documented contract |
| `TestDecodeToleratesUnknownFields` | An added unknown field decodes successfully | The SDK's forward-compatibility default is not defeated |
| `TestDecodeRejectsExplicitNullForOptional` | Explicit `null` for an optional field errors | Absent ≠ null, per the SDK |
| `TestDecodeRejectsCorruptDiscriminator` | A union field with an unknown kind errors with the PEOS sentinel preserved | Discriminator corruption is detected |

The eleventh case — a claim **with** a correction reference — is separate because
the correction is an optional field whose survival across a round trip is the
whole basis of the correction algorithm.

### 4.2 Projection fidelity

`TestProjectionFidelity_<Family>` — build the value, project it into its
envelope, decode the payload back into the PEOS type, and assert every projected
field equals the value read from the decoded type.

This is the test that makes [FF-008 §4](008-package-architecture.md#why-queries-do-not-live-here)
safe: queries trust projections, so projections must be provably faithful.
Fields covered: subject key, scope, occurred-at, outcome, criterion keys,
evidence keys, execution keys, correction kind and target, state ID, artifact
type, integrity value, provenance actor and recorded-at.

### 4.3 Error preservation

| Test | Expected | Proves |
|---|---|---|
| `TestPEOSSentinelsRemainMatchable` | A rejected satisfaction claim matches **both** `validation.ErrInvalidSatisfactionClaim` and the FeatureForge wrapper | `%w` wrapping preserves `errors.Is` |
| `TestPEOSNestedSentinelsPreserved` | Both the general sentinel and the specific cause match | The SDK's nested-sentinel behaviour survives |
| `TestNoGenericInternalError` | Parse `internal/`; no error is replaced by an unwrapped generic value | Error model rule 3 |

### 4.4 Constructor-flow tests

One per family, asserting the FF-011 §4 constructor order succeeds and that
omitting each mandatory argument fails with the SDK's sentinel. Specifically
including:

- `TestSatisfactionClaimRequiresRequirementCriterion` — a product-rule-only
  criterion is rejected (verified fact 3);
- `TestClaimRequiresEvidence` — nil evidence is rejected (verified fact 4);
- `TestPlannedActivityRequiresOutcomeInterpretation` — empty is rejected;
- `TestEntryStateAssignmentAcceptsBareEstablishedBy` — the AD-014 workaround
  works (verified fact 7);
- `TestTransitionContentRejectsZeroFromAssignment` — documents the SDK
  limitation that forces AD-014 (verified fact 6).

The last one asserts a *limitation*, deliberately. If a future PEOS version
lifts it, this test fails and tells us AD-014 can be revisited.

## 5. Envelope tests — `internal/engineering`

| Test | Expected | Proves |
|---|---|---|
| `TestEnvelopeValidation` | Empty key, empty payload, non-JSON payload, digest mismatch, unknown kind each rejected | Construction validation |
| `TestEnvelopeEqualityIsKeyPlusPayload` | Equal keys and bytes are equal; differing projections on identical payloads are still equal | Equality has one definition |
| `TestEnvelopeDigestMatchesPayload` | `PayloadDigest` = SHA-256 of `Payload` | Digest integrity |
| `TestRecordKeyDistinguishesKinds` | Same ID string under two kinds does not collide | Composite key correctness |
| `TestEngineeringPackageCompilesWithoutPEOS` | The package's own test binary links with no PEOS package in its import graph | AD-005 is structurally real |

## 6. Repository tests — `internal/infrastructure/memory`

Written once as a **shared contract suite** parameterized by a repository
factory, so M.4 runs the identical body against PostgreSQL.

| Test | Expected | Proves |
|---|---|---|
| `TestPutThenGet` | Value returned, found true | Basic persistence |
| `TestGetMissingReturnsNotFound` | Zero value, found false, nil error | Absence is not an error |
| `TestIdempotentIdenticalPut` | Second identical put succeeds; store unchanged | Retry safety |
| `TestConflictingPut` | `ErrImmutableValueConflict`, message names the key | Immutability enforced |
| `TestNoUpdateOrDeleteMethodExists` | Reflect over each repository interface; no `Update*`, `Delete*`, `Remove*`, `Set*` | Insert-only is structural |
| `TestListIsDeterministic` | Seed keys in an order differing from sorted order; list 100 times | Identical output every time |
| `TestListOrderIsDocumentedOrder` | Compare against the FF-009 §5 ordering | Ordering is specified, not incidental |
| `TestReferenceVerification` | Put a revision whose artifact is absent | `ErrReferencedValueMissing` naming both keys |
| `TestConcurrentPutsAreSafe` | 64 goroutines putting distinct keys | No race under `-race`; all present |
| `TestReturnedSlicesAreCopies` | Mutate a returned slice, re-read | Stored state unchanged |
| `TestNoExposedMaps` | Reflect over the store's exported surface | No map-typed export |

## 7. Transaction tests

| Test | Expected | Proves |
|---|---|---|
| `TestCommitPersistsAllWrites` | All writes visible after `Do` returns nil | Commit |
| `TestRollbackDiscardsAllWrites` | Callback returns an error; no write visible | Atomicity |
| `TestRollbackOnInjectedFailure` | Third write fails via the injection hook | Partial acts leave no trace |
| `TestRollbackOnPanic` | Callback panics | Store unchanged; panic re-raised |
| `TestErrorPropagatesUnwrapped` | Callback returns a wrapped sentinel | `errors.Is` still matches after `Do` |
| `TestConflictAbortsAct` | A conflicting put inside a multi-write act | Nothing from that act persists |
| `TestNestedTransactionRejected` | `Do` inside `Do` | `ErrNestedTransaction` |
| `TestRepositoriesUnreachableOutsideDo` | Compile-time: repositories are only reachable via the callback parameter | No non-transactional path |
| `TestEachEngineeringActIsOneTransaction` | Instrument `Do`; run each of the eleven acts | Exactly one `Do` per act |

## 8. Ordering tests — `internal/application`

Run against hand-built envelopes; no PEOS involved.

| Test | Setup | Expected | Proves |
|---|---|---|---|
| `TestSequenceOneThenTwo` | Revisions 1, 2 both accepted | Current = 2 | Baseline |
| `TestInsertionOrderIndependence` | Persist 2 then 1; also 3, 1, 2 | Identical result in all orders | The core M.1 claim |
| `TestIgnoresRecordedAt` | Lower sequence with a **later** `RecordedAt` | Current = higher sequence | Timestamps never order revisions |
| `TestIgnoresRevisionIDLexicalOrder` | IDs whose lexical order reverses sequence order | Current = greatest sequence | PEOS-002's sortability prohibition honoured |
| `TestDraftHigherSequenceRejected` | 1 accepted, 2 draft | Current = 1; rationale names 2 as not accepted | Acceptance gates currency |
| `TestWithdrawnAcceptedFallsBack` | 1 accepted, 2 accepted then withdrawn | Current = 1 | Withdrawal is honoured |
| `TestNoAcceptedRevision` | All draft | `None` + rationale, **not** an error | A legitimate state |
| `TestDuplicateSequence` | Two revisions at sequence 2 | `ErrRevisionSequenceConflict` naming both | Ambiguity fails loudly |
| `TestMissingOrderMetadata` | Revision with no order entry | `ErrRevisionOrderMissing` | Broken writes detected |
| `TestOrphanOrderMetadata` | Order entry with no revision | `ErrRevisionReferenceMismatch` | Both directions checked |
| `TestNonPositiveSequence` | Sequence 0 or negative | `ErrRevisionSequenceInvalid` | Bounds |
| `TestConcurrentRevisionCreation` | Two concurrent `ReviseCapabilitySpecification` | Sequences *n* and *n+1*, never two of *n* | Transactional assignment |
| `TestResolutionIsDeterministic` | Repeat 100 times | Byte-identical result and rationale | Determinism |
| `TestRationaleNamesRejected` | Mixed states | Every rejected revision appears with a reason | Rationale completeness |

### Acceptance tests

| Test | Expected |
|---|---|
| `TestAcceptanceDefaultsToDraft` | A revision with no journal entry is `draft` |
| `TestAcceptanceHeadWins` | Latest entry by `(EffectiveAt, RecordID)` determines state |
| `TestAcceptanceTieBreakByRecordID` | Equal `EffectiveAt` resolves by record ID, deterministically |
| `TestAcceptedToDraftRejected` | `ErrAcceptanceTransitionInvalid` |
| `TestWithdrawnIsTerminal` | Any transition out of `withdrawn` rejected |
| `TestNoStoredAcceptanceField` | Reflect over `RevisionOrderMetadata`: no acceptance field | AD-015 — one truth, one place |

## 9. Correction tests — `internal/application`

| Test | Setup | Expected | Proves |
|---|---|---|---|
| `TestSingleClaimIsHead` | One claim | It is current | Baseline |
| `TestChainOfTwo` | `CLM-2` ← `CLM-4` | Current = `CLM-4`; chain reported | The canonical case |
| `TestChainOfThree` | A ← B ← C | Current = C | Traversal to termination |
| `TestOriginalRemainsReadable` | After correction | `CLM-2` fetches unchanged and still reads `satisfied` | History intact |
| `TestSelectionIgnoresTimestamps` | Correcting claim backdated **before** its target | Current is still the head | Time never selects |
| `TestInvalidateLeavesNoCurrent` | Sole claim invalidated | `None` + rationale naming the invalidator, not an error | Invalidation semantics |
| `TestInvalidatorEvaluatedOnOwnMerits` | Invalidating claim also carries an outcome | It is a head candidate itself | Correct treatment |
| `TestMissingTarget` | Correction naming an absent claim | `ErrCorrectionTargetMissing` | Dangling refs fail |
| `TestSelfCorrection` | Claim correcting itself | `ErrCorrectionSelfReference` | **The SDK permits this; FeatureForge must not** |
| `TestFamilyMismatch` | Correction target names an execution record ID | `ErrCorrectionFamilyMismatch` | Family checked |
| `TestCycle` | A ← B ← A | `ErrCorrectionCycle` naming the nodes; terminates | Bounded traversal |
| `TestCompetingHeads` | Two claims correcting the same target | `ErrCorrectionAmbiguous` naming both | Human decision required |
| `TestScopeAndCriteriaPartitioning` | Claims for different requirements | Each resolves independently | Partitioning correct |
| `TestCorrectionRationale` | Any chain | Prose chain, edge list, rejected claims with reasons | Rationale completeness |

## 10. Readiness and lifecycle tests

### Readiness

| Test | Setup | Expected |
|---|---|---|
| `TestReadyWhenAllSatisfied` | All requirements satisfied, completed executions, resolvable evidence | `ready` |
| `TestNotReadyOnNegativeClaim` | One `not-satisfied` | `not-ready` |
| `TestIncompleteOnMissingClaim` | One requirement with no claim | `incomplete` |
| `TestIndeterminateOnInconclusiveClaim` | One `inconclusive` | `indeterminate` |
| `TestIndeterminateOnInterruptedExecution` | Satisfied claim backed only by an `interrupted` execution | `indeterminate`, execution named | Discharges the PEOS-006 obligation |
| `TestIndeterminateOnIndeterminateExecution` | Same with `indeterminate` | `indeterminate` |
| `TestPrecedenceNotReadyBeatsIndeterminate` | One `not-satisfied` **and** one `inconclusive` | `not-ready` | The finalised precedence |
| `TestPrecedenceIndeterminateBeatsIncomplete` | One `inconclusive` and one uncovered | `indeterminate` |
| `TestPrecedenceIncompleteBeatsReady` | Three satisfied, one uncovered | `incomplete` |
| `TestStaleClaimDoesNotSatisfy` | Claim whose subject is revision 1 while current is revision 2 | Not counted; reported stale with the sequence | The two-revision scenario has meaning |
| `TestNoRequirementsIsIncomplete` | No requirements | `incomplete` |
| `TestStructuralFailureIsError` | Ambiguous revision / dangling reference | `ErrEngineeringStateIndeterminate` wrapping the cause — **an error, not a status** |
| `TestReadinessRationaleIsPerRequirement` | Any state | Every requirement appears with selected claim, rejected claims, evidence, and reason |
| `TestReadinessIsDeterministic` | Repeat 100 times | Byte-identical |

### Lifecycle

| Test | Expected |
|---|---|
| `TestNoAssignmentsReturnsNone` | `None` + rationale |
| `TestLatestEffectiveAtWins` | Greatest `EffectiveAt` selected |
| `TestEqualTimestampSameStateTieBreaks` | Lowest record ID; duplicate noted |
| `TestEqualTimestampDifferentStatesFails` | `ErrAmbiguousLifecycleState` naming both |
| `TestUnknownDefinitionVersionRejected` | `ErrUnknownDefinitionVersion` |
| `TestEntryAssignmentResolves` | `SA-1` established by the content-free entry revision resolves normally | AD-014 works end to end |
| `TestAssessedAndNotReadyCoexist` | Lifecycle `assessed`, readiness `not-ready` | **AD-018 — lifecycle does not duplicate readiness** |
| `TestLifecycleStateNotDerivedFromClaims` | Change claims only | Lifecycle state unchanged | Independence |

## 11. Timeline tests

| Test | Expected | Proves |
|---|---|---|
| `TestCanonicalTimeline` | Exactly the expected events, in order | The scenario's history is complete |
| `TestInsertionOrderIndependence` | Persist records in reverse; identical timeline | Ordering is computed |
| `TestEventIDIsDerived` | Event ID = `kind:sourceIdentity` for every event | No stored identity |
| `TestEqualTimestampsOrderByKindRankThenID` | Two events at one instant | Deterministic, and both rationales name the tie-break |
| `TestUndatedGroupIsSeparate` | An event whose source has no timestamp | Appears in `Undated`, flagged; never position 1 of the dated slice |
| `TestCorrectedClaimRendersLink` | `CLM-4` | Carries `Corrected = CLM-2`; `CLM-2`'s own event unchanged and still present |
| `TestArtifactEnvelopesProduceNoDuplicateEvents` | Requirement/plan/evidence artifacts | No artifact-creation event beside their founding revisions |
| `TestDanglingReferenceFails` | Correction target absent | `ErrTimelineSourceInvalid` naming both |
| `TestInterruptedOutcomeRenderedVerbatim` | An `interrupted` execution | Rendered `interrupted`, never normalized |
| `TestTimelineDoesNotMutateSources` | Snapshot before and after | Byte-identical stored state |
| `TestTimelineIsDeterministic` | Repeat 100 times | Byte-identical |
| `TestTimelineNotStored` | Reflect over the store | No timeline collection exists |

## 12. Architecture tests

Standard library only — `go/parser`, `go/ast`, `go/build`. No line-number or
offset assertions. Each is verified against a deliberate violation in M.3.

| Test | Asserts |
|---|---|
| `TestDomainDoesNotImportPEOS` | `internal/domain`'s transitive import set contains no `github.com/aleka7sk/PEOS/...` |
| `TestEngineeringDoesNotImportPEOS` | Same for `internal/engineering` (excluding its `peos` child) |
| `TestApplicationDoesNotImportPEOS` | Same for `internal/application`, transitively |
| `TestInfrastructureDoesNotImportPEOS` | Same for `internal/infrastructure/...`, transitively |
| `TestOnlyIntegrationPackageImportsPEOS` | Exactly one package in the module directly imports the PEOS SDK, and it is `internal/engineering/peos` |
| `TestApplicationDoesNotImportIntegration` | `internal/application` does not import `internal/engineering/peos`; it consumes the `EngineeringRecorder` port it declares |
| `TestEngineeringDoesNotImportItsChild` | `internal/engineering` does not import `internal/engineering/peos` |
| `TestNoPEOSTypeIsCopied` | No FeatureForge struct reproduces the field set of a listed PEOS type |
| `TestNoShadowStructNames` | No FeatureForge type is named `Artifact`, `ArtifactRevision`, `Claim`, `Requirement`, `Decision`, `ExecutionRecord`, `StateAssignment`, `Provenance`, `Representation` |
| `TestNoForbiddenPackageNames` | No package named `workflow`, `engine`, `framework`, `shared`, `common`, `util`, `pkg`, `integration`, `core` |
| `TestNoOperationalScenarioEntity` | No type, field, or package named for teacher, student, lesson, homework, attachment, or notification |
| `TestNoDerivedStateOnFeatureCard` | `FeatureCard` has no current/readiness/lifecycle field |
| `TestOperationalTypesDoNotEmbedEnvelopes` | Neither `Project` nor `FeatureCard` embeds or fields any envelope type |
| `TestNoHTTPDatabaseUIOrAIPackage` | No package or import for `net/http`, a SQL driver, a template engine, or an AI client |
| `TestNoTimeNowOutsideClock` | No `time.Now` reference outside the production clock implementation |
| `TestGoModHasOnlyPEOSRequirement` | `go.mod` declares exactly one non-stdlib requirement, `github.com/aleka7sk/PEOS v1.0.0`, and no `replace` directive |
| `TestNoPEOSFileModified` | The PEOS module in the build list resolves to v1.0.0 and `go mod verify` passes |

## 13. End-to-end scenario test

One test, `TestCanonicalScenario`, executing [FF-011](011-canonical-scenario.md)
in order against the in-memory adapter with a fixed clock.

It must demonstrate all 25 M.3 objectives:

| # | Objective | Assertion |
|---|---|---|
| 1 | Project creation | `PRJ-1` retrievable |
| 2 | FeatureCard creation | `FC-1` retrievable, linked to `PRJ-1` |
| 3 | Capability artifact | `CAP-1` stored, type `featureforge:product-capability` |
| 4 | Capability Revision 1 | `CAP-1-REV-1` stored, sequence 1 |
| 5 | Structured content linked to the exact revision | Content at `RevisionKey(CAP-1, CAP-1-REV-1)`; digest matches the revision's integrity value |
| 6 | Requirement + revision | `REQ-1`…`REQ-4` with their revisions |
| 7 | Decision + basis | `DEC-1` with evidence, assumption, constraint, uncertainty |
| 8 | Capability Revision 2 | `CAP-1-REV-2`, sequence 2, accepted |
| 9 | Plan + plan revision | `VP-1-REV-1` with activities `A-1`…`A-3` |
| 10 | Execution record | `ER-1`…`ER-4`, outcome `completed` |
| 11 | Evidence | `EV-1`…`EV-4` revisions in the evidence role |
| 12 | Validation result | Execution outcomes and claim outcomes are distinct fields with distinct vocabularies |
| 13 | Incorrect claim | `CLM-2` `satisfied` |
| 14 | Corrected claim | `CLM-4` `not-satisfied` correcting `CLM-2`; `CLM-2` unchanged |
| 15 | Current revision resolution | `CAP-1-REV-2` with rationale |
| 16 | Effective requirements | All four resolved |
| 17 | Current non-corrected claim | `CLM-1`, `CLM-4`, `CLM-3`, and none for `REQ-4` |
| 18 | Release readiness | `not-ready`, with per-requirement rationale naming `REQ-2` and `REQ-4` |
| 19 | Lifecycle state | `featureforge:under-validation` |
| 20 | Complete timeline | Full event list in the expected order, `CLM-4` linked to `CLM-2` |
| 21 | JSON round trips | Every stored payload re-encodes byte-identically |
| 22 | Idempotent writes | Replaying the whole scenario changes nothing |
| 23 | Immutable identity conflicts | Replaying with altered content fails with `ErrImmutableValueConflict` |
| 24 | Ambiguity errors | A duplicate sequence and a competing correction head each fail with their sentinel |
| 25 | Architecture boundaries | §12 passes |

### Structure

1. **Arrange** — fixed clock at `2026-03-01T00:00:00Z`, empty store, wired
   composition root.
2. **Act** — the eleven engineering acts in order, each asserting its result keys.
3. **Assert state** — the FF-011 §9 end-state table, field by field.
4. **Assert history** — `CAP-1-REV-1` and `CLM-2` byte-identical to their
   recorded form; no update or delete occurred.
5. **Assert determinism** — every query run twice, byte-identical.
6. **Assert replay** — the whole scenario replayed is a no-op (objective 22);
   replayed with altered content, it conflicts (objective 23).

### Insertion-order variant

`TestCanonicalScenarioInsertionOrderIndependence` — replay the scenario writing
records in a permuted order wherever the engineering acts permit, and assert that
current revision, current claims, readiness, lifecycle state, and the timeline are
byte-identical to the canonical run.

This is the single strongest test in M.3, because it is the one that fails if any
algorithm has quietly started depending on storage order.
