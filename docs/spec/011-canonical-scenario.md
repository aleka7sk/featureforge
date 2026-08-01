# FF-011 — Canonical Scenario and PEOS Value Inventory

Status: Accepted (Phase M.2)
Governs: the exact fixture identities, PEOS value inventory, constructor flows,
requirements, decision, and validation chain that M.3 must implement.

## Governance statement

PEOS-000 through PEOS-009 remain authoritative for every PEOS concept.
`docs/spec/` governs FeatureForge. This document does not extend the PEOS
ontology. The implementation must use PEOS v1.0.0 unchanged.

**Every constructor flow in this document was executed against PEOS v1.0.0
during M.2 and compiles.** Nothing here is inferred from documentation. Where the
SDK's behaviour differed from expectation, the finding is stated inline.

## 1. Verified SDK facts

These were established empirically in M.2 and are load-bearing for M.3.

| # | Fact | Consequence |
|---|---|---|
| 1 | `core.CriterionRefFromRequirementRevision` is accepted as a Satisfaction Claim criterion | M.1's revision-level citation policy holds |
| 2 | `core.CriterionRefFromRequirement` (identity level) is **also** accepted | FeatureForge must *enforce* revision-level as policy; the SDK permits both |
| 3 | A Satisfaction Claim with only a non-Requirement criterion is rejected with `validation.ErrInvalidSatisfactionClaim` | Criteria selection is validated by the SDK |
| 4 | `validation.NewClaim` **requires at least one evidence reference** | A claim can never be recorded without evidence. R4 therefore has no claim at all, rather than an evidence-free one |
| 5 | `Claim.WithCorrection` **accepts a self-correction** | FeatureForge must reject it — `ErrCorrectionSelfReference` |
| 6 | `lifecycle.NewTransitionRecordContent` rejects a zero `fromAssignment` | Entry transitions need the AD-014 workaround ([FF-010 §8](010-application-contracts.md#the-entry-transition-problem-and-its-resolution)) |
| 7 | `lifecycle.NewStateAssignment` accepts `establishedBy` as a bare reference with no lookup | The workaround is available |
| 8 | `requirement.SubjectCombinationIndependent` is exported | FeatureForge must use it, not hand-build `peos:independent` |
| 9 | `WithCriteria`, `WithProducedEvidence`, `WithExecutionRecords` take **slices**, not variadics | Call sites must pass slices |
| 10 | All seven families round-trip JSON byte-identically | The serialization contract is achievable |

## 2. Fixture identities

Deterministic and fixed. No generation.

| Entity | Identity |
|---|---|
| Project | `PRJ-1` |
| Feature card | `FC-1` |
| Capability artifact | `CAP-1` |
| Capability revision 1 | `CAP-1-REV-1` |
| Capability revision 2 | `CAP-1-REV-2` |
| Requirement artifacts | `REQ-1` … `REQ-4` |
| Requirement revisions | `REQ-n-REV-1` |
| Decision | `DEC-1` |
| Validation plan artifact | `VP-1` |
| Plan revision | `VP-1-REV-1` |
| Evidence artifacts | `EV-0` (interview notes) … `EV-4` |
| Evidence revisions | `EV-n-REV-1` |
| Execution records | `ER-1`, `ER-2`, `ER-3`, `ER-4` |
| Claims | `CLM-1`, `CLM-2`, `CLM-3`, `CLM-4` |
| Transition record artifact | `TR-1` |
| Transition record revisions | `TR-1-REV-0` (entry), `TR-1-REV-1` (specify), `TR-1-REV-2` (begin validation) |
| State assignments | `SA-1` (drafting), `SA-2` (specified), `SA-3` (under-validation) |
| Lifecycle definition / version | `LCD-1` / `LCDV-1` |
| Acceptance records | `ACC-1`, `ACC-2`, `ACC-REQ-1` … `ACC-REQ-4`, `ACC-VP-1` |

**Claim naming.** M.1's narrative used `C-2`/`C-4`; the fixtures use `CLM-n` to
avoid collision with acceptance-criterion keys `AC-n`. `CLM-2` is the incorrect
claim and `CLM-4` corrects it, preserving M.1's mapping.

Fixed timestamps run from `2026-03-01T00:00:00Z`, advanced one hour per
engineering act by the test clock.

## 3. Vocabulary

Declared once in `internal/engineering/peos/vocabulary.go`, constructed at
package initialization, and never built ad hoc at a call site.

| Value | Classification | Used as |
|---|---|---|
| `featureforge:product-capability` | **ArtifactType** | Capability artifact |
| `featureforge:validation-evidence` | **ArtifactType** | Evidence artifacts |
| `featureforge:specification-content` | **Representation media type** | Capability revision representation |
| `featureforge:validation-report` | **Representation media type** | Evidence revision representation |
| `featureforge:sha256` | **VocabularyValue** | Content-address algorithm |
| `featureforge:manual-review` | **VocabularyValue** → `core.ValidationMethod` | Review activities |
| `featureforge:manual-inspection` | **VocabularyValue** → `core.ValidationMethod` | Inspection activity |
| `featureforge:capability` | **VocabularyValue** → `core.Scope` kind | Plan, claim, decision, lifecycle scope |
| `featureforge:capability-lifecycle` | **VocabularyValue** | Lifecycle subject type |
| `featureforge:drafting` | **VocabularyValue** → `lifecycle.StateID` | Lifecycle state |
| `featureforge:specified` | **VocabularyValue** → `lifecycle.StateID` | Lifecycle state |
| `featureforge:under-validation` | **VocabularyValue** → `lifecycle.StateID` | Lifecycle state |
| `featureforge:assessed` | **VocabularyValue** → `lifecycle.StateID` | Lifecycle state |
| `featureforge:local-user` | **Actor / Authority identifier** | `core.NewActorRef("featureforge", "local-user")` |
| `featureforge:release-readiness` | **Product-only query terminology** | Not a PEOS value at all |

### Confirmations

- `core.ClaimType` remains **closed**. FeatureForge declares no claim type.
- `core.ClaimTypeSatisfaction` is used for every claim in the scenario.
- `featureforge:requirement-satisfaction` is **not created** (AD-009).
- `featureforge:feature-specification` is **not created** (AD-009).
- `featureforge:release-readiness` names a computed query and never becomes a
  `ProductRuleRef`, a criterion, or a claim (AD-008).
- **No FeatureForge constant is added to PEOS.** Every PEOS-namespace value used
  comes from an SDK-exported constant: `core.OriginKindKnown`,
  `core.IntegrityMechanismContentAddressedReference`,
  `core.IntegrityProtectedScopeContent`, `core.RepresentationRoleAuthoritative`,
  `core.ClaimTypeSatisfaction`, `core.ClaimOutcome*`, `core.ExecutionOutcome*`,
  `core.CorrectionKind*`, `requirement.ArtifactTypeRequirement`,
  `requirement.SubjectCombinationIndependent`,
  `validation.ArtifactTypeValidationPlan`, `lifecycle.ArtifactTypeTransitionRecord`,
  `lifecycle.TransitionOutcomeSucceeded`, `decision.CommitmentEffectEstablishes`.

## 4. PEOS value inventory

Common to every revision and record: `origin = core.NewOrigin(core.OriginKindKnown, "")`;
`provenance = core.NewProvenance().WithActor(actor).WithRecordedAt(ts)` where
`actor = core.NewActorRef("featureforge", "local-user")`;
`scope = core.NewScope(featureforgeCapability, "CAP-1")`.

### 4.1 Capability artifact and revisions

| | |
|---|---|
| Type | `core.Artifact`, then `core.ArtifactRevision` |
| Identity | `CAP-1`; revisions `CAP-1-REV-1`, `CAP-1-REV-2` |
| Constructor order | `core.NewArtifactID` → `core.NewArtifactType(vv)` → `core.NewArtifact(id, type)`; then digest → `core.NewIntegrityIdentity(core.IntegrityMechanismContentAddressedReference, "sha256:"+hex, core.IntegrityProtectedScopeContent)` → `core.NewRepresentationFromContentAddress(sha256Vocab, hex, specContentMediaType, core.RepresentationRoleAuthoritative)` → `core.NewArtifactRevision(artifactID, revisionID, origin, provenance, integrity)` → `.WithRepresentations(rep)` |
| Optional modifiers | `WithRepresentations` (used); `WithExtension` (**never used**) |
| Sentinels | `core.ErrEmptyIdentity`, `core.ErrInvalidArtifact`, `core.ErrInvalidArtifactRevision`, `core.ErrInvalidIntegrityIdentity`, `core.ErrInvalidRepresentation` |
| Envelope | `ArtifactEnvelope`; `RevisionEnvelope` family `capability` |
| Repository | `ArtifactEnvelopeRepository`; `RevisionEnvelopeRepository` + `StructuredContentRepository` |

`WithRepresentations` is variadic; the others noted in §1 fact 9 are not.

### 4.2 Requirement and revision

| | |
|---|---|
| Type | `requirement.Requirement`, `requirement.Revision` |
| Constructor order | `core.NewArtifact(id, requirement.ArtifactTypeRequirement)` → `requirement.New(artifact)` → `requirement.NewStatement(text)` → subject via `core.NewArtifactRef(CAP-1)` + `core.EngineeringSubjectRefFromArtifact` → `requirement.NewContent(statements, subjects, requirement.SubjectCombinationIndependent, requirement.NewUnrestrictedApplicability())` → `core.NewArtifactRevision(...)` → `requirement.NewRevision(req, coreRev, content)` |
| Subject | The capability **Artifact** (identity level): the obligation is on the capability, not on one revision of its text |
| Optional modifiers | `WithRationale` (used); `WithOrigins`, `WithAuthorities`, `WithClassifications` (unused) |
| Sentinels | `requirement.ErrInvalidRequirement`, `requirement.ErrInvalidContent`, `requirement.ErrArtifactTypeMismatch` |
| Envelope | `RevisionEnvelope` family `requirement` |

The integrity identity for a requirement revision is the digest of its own
canonical statement content, computed by the same rule as capability content.

### 4.3 Decision and basis

| | |
|---|---|
| Type | `decision.Decision` with `decision.Basis` |
| Constructor order | `decision.NewOutcome(statement, decision.CommitmentEffectEstablishes)` → `core.NewAuthorityRef("featureforge","local-user")` → `decision.NewAuthority([]core.AuthorityRef{a}, []core.AuthorityRef{a})` → `decision.New(id, subjects, question, outcome, applicability, authority)` → `.WithBasis(basis)` → `.WithProvenance(prov)` → `.WithRationale(text)` |
| Basis | `decision.NewBasisFrom(evidence, assumptions, constraints, uncertainties)`, evidence = `core.NewEvidenceArtifactRevisionRef("EV-0","EV-0-REV-1")` |
| Subjects | Capability **Revision 1** — the decision resolves that revision's open questions |
| Sentinels | `decision.ErrInvalidDecision`, `decision.ErrInvalidOutcome`, `decision.ErrInvalidAuthority`, `decision.ErrInvalidBasis` |
| Envelope | `RecordEnvelope` kind `decision` |

`decision.Record` (the Decision Record **Artifact**) is **not** used — M.1
deferred it, and the scenario never supersedes or relates the decision as an
artifact.

### 4.4 Validation plan

| | |
|---|---|
| Type | `validation.Plan`, `validation.PlanRevision`, `PlanContent`, `PlannedActivity` |
| Constructor order | `core.NewArtifact(VP-1, validation.ArtifactTypeValidationPlan)` → `validation.NewPlan(artifact)` → per activity: `core.NewLocalKey("A-n")` → `validation.NewPlannedActivity(key, subject, method, outcomeInterpretation)` → `.WithCriteria([]core.CriterionRef{...})` → `.WithExpectedEvidence([]string{...})`; then `validation.NewScopedPlanApplicability(scope)` → `validation.NewPlanContent(scope, applicability, provenance, activities)` → `core.NewArtifactRevision(...)` → `validation.NewPlanRevision(plan, coreRev, content)` |
| Activity subject | Capability **Revision 2** (`core.EngineeringSubjectRefFromArtifactRevision`) |
| Activity criteria | `core.CriterionRefFromRequirementRevision(core.NewRequirementArtifactRevisionRef(REQ-n, REQ-n-REV-1))` |
| `outcomeInterpretation` | Mandatory, non-empty — the SDK rejects empty |
| Sentinels | `validation.ErrInvalidValidationPlan`, `ErrInvalidPlannedActivity`, `ErrInvalidPlanApplicability`, `ErrDuplicatePlanLocalKey`, `ErrUnknownPlanLocalKey` |
| Envelope | `RevisionEnvelope` family `validation-plan` |

### 4.5 Execution record

| | |
|---|---|
| Type | `validation.ExecutionRecord` |
| Constructor order | `planRev.Ref()` → `validation.NewPlannedActivityReference(planRevRef, key)` → `validation.NewExecutionRecord(id, activityRef, subject, method, outcome, completedAt, actor, provenance)` → `.WithProducedEvidence([]core.EvidenceArtifactRevisionRef{...})` → `.WithCriteria(...)` |
| Outcome | `core.ExecutionOutcomeCompleted` for all four runs |
| Sentinels | `validation.ErrInvalidExecutionRecord` |
| Envelope | `RecordEnvelope` kind `execution` |

### 4.6 Evidence

Evidence is **not a distinct PEOS construct**. It is a `core.Artifact` of type
`featureforge:validation-evidence` with an ordinary `core.ArtifactRevision`,
cited as `core.EvidenceArtifactRevisionRef`.

The evidence artifact carries `core.ArtifactRoleEvidence` via
`Artifact.WithRoles(core.ArtifactRoleEvidence)`. Its revision carries one
`Representation` built with `core.NewRepresentationFromExternalReference(locator,
featureforgeValidationReport, core.RepresentationRoleAuthoritative)` — evidence is
an external document, cited by locator, not stored by FeatureForge.

Envelope: `RevisionEnvelope` family `evidence`.

### 4.7 Result

**There is no `Result` type, and none is created.** Result semantics live in two
places, exactly as M.1 established:

| Question | Where it lives |
|---|---|
| Did the activity run, and how did it conclude? | `ExecutionRecord.Outcome()` — `core.ExecutionOutcome` |
| What was determined about the subject? | `Claim.Outcome()` — `core.ClaimOutcome` |

These are projected into `RecordEnvelope.Outcome` for their respective kinds and
never merged into one field with one meaning.

### 4.8 Claims

| | |
|---|---|
| Type | `validation.Claim` |
| Constructor order | `validation.NewClaim(id, core.ClaimTypeSatisfaction, subject, scope, outcome, method, criteria, evidence, timestamp, provenance)` → `.WithExecutionRecords([]core.ValidationExecutionRecordRef{...})` → `.WithReasoning(text)` → for `CLM-4` only, `.WithCorrection(ref)` |
| Subject | Capability **Revision 2** |
| Criteria | Exactly one `CriterionRefFromRequirementRevision` per claim |
| Evidence | At least one — mandatory (fact 4) |
| Correction | `core.NewRecordCorrectionRef(core.CorrectionKindCorrect, core.NewValidationClaimRef(CLM-2))` |
| Sentinels | `validation.ErrInvalidValidationClaim`, `ErrInvalidSatisfactionClaim` |
| Envelope | `RecordEnvelope` kind `claim`, with `CorrectionKind` / `CorrectionTargetID` projected |

### 4.9 Lifecycle

| | |
|---|---|
| Types | `lifecycle.Definition`, `DefinitionVersion`, `State`, `TransitionDefinition`, `TransitionRecord`, `TransitionRecordRevision`, `TransitionRecordContent`, `StateAssignment` |
| Definition | `lifecycle.NewDefinition(LCD-1)` |
| Version | `lifecycle.NewDefinitionVersion(LCDV-1, defRef, scope, subjectTypes, states, initialStates, transitions, entryTransition, provenance)` |
| States | `lifecycle.NewState(stateID, meaning)` for the four states in [FF-010 §8](010-application-contracts.md#vocabulary) |
| Transitions | `specify` (drafting→specified), `begin-validation` (specified→under-validation), `assess` (under-validation→assessed), plus entry transition `enter` (→drafting) |
| Subject | `core.NewLifecycleSubjectRefFromArtifact(core.NewArtifactRef(CAP-1))` |
| `SA-1` (entry) | `lifecycle.NewStateAssignment(SA-1, subject, dvRef, drafting, effectiveAt, provenance, ref(TR-1, TR-1-REV-0))` where `TR-1-REV-0` is a plain `core.ArtifactRevision` with **no** transition content — see AD-014 |
| `SA-2` | `specify`: full `TransitionRecordRevision` from `SA-1/drafting`, resulting in `SA-2/specified` |
| `SA-3` | `begin-validation`: full `TransitionRecordRevision` from `SA-2/specified`, resulting in `SA-3/under-validation` after the first execution has been recorded |
| Sentinels | `lifecycle.ErrInvalidStateAssignment`, `ErrInvalidTransitionRecordRevision`, `ErrInvalidDefinitionVersion` |
| Envelope | `RecordEnvelope` kind `state-assignment`; transition record revisions are `RevisionEnvelope` family `transition-record` |

## 5. Canonical requirements

Four requirements. Each subject is the capability Artifact `CAP-1`; each scope is
`featureforge:capability|CAP-1`; each has one revision `REQ-n-REV-1`.

| ID | Statement | Exact source trace | Claim criterion | Plan activity | Final status |
|---|---|---|---|---|---|
| `REQ-1` | Published homework SHALL be visible to the student of the lesson it belongs to. | `CAP-1/CAP-1-REV-2#AC-1` | `REQ-1/REQ-1-REV-1` | `A-1` | **satisfied** (`CLM-1`) |
| `REQ-2` | Published homework SHALL NOT be visible to any user who is not the student of that lesson. | `CAP-1/CAP-1-REV-2#AC-2` | `REQ-2/REQ-2-REV-1` | `A-2` | **not-satisfied** (`CLM-4`, correcting `CLM-2`) |
| `REQ-3` | Where homework has an audio attachment, that attachment SHALL have a representation the student can resolve. | `CAP-1/CAP-1-REV-2#AC-3` | `REQ-3/REQ-3-REV-1` | `A-3` | **satisfied** (`CLM-3`) |
| `REQ-4` | Published homework SHALL become observable to the student within 5 seconds of publication. | `CAP-1/CAP-1-REV-2#AC-4` | `REQ-4/REQ-4-REV-1` | **none** | **uncovered** — no claim |

**Requirement text lives in `requirement.Statement`**, a genuine PEOS-005 field.
No PEOS field is invented, and requirement text is not duplicated into
FeatureForge structured content. Only *capability specification* content is
FeatureForge-owned; requirement content is PEOS-owned.

### Why `REQ-4` is uncovered

M.1 left AC-4 without a requirement. AD-033 confirms M.2's executable choice to
promote it to a requirement and leaves it **without a validation activity and
without a claim**. Its exact Revision 2/AC-4 source is persisted. This is
strictly
better: it exercises `incomplete` (a requirement with no claim) alongside
`not-ready` (a requirement with a negative claim), so the readiness precedence
rule in [FF-010 §7](010-application-contracts.md#precedence--challenged-and-finalised)
has both inputs present and is genuinely tested rather than assumed.

Note that fact 4 makes the alternative impossible anyway: a claim cannot be
recorded without evidence, so "a requirement with an evidence-free claim" is not
a representable state.

## 6. Canonical decision

| Field | Value |
|---|---|
| Identity | `DEC-1` |
| Subjects | Capability Revision 1 (`CAP-1/CAP-1-REV-1`) |
| Question | Should homework support an optional audio attachment, and what publication latency is acceptable? |
| Alternatives considered | Store audio inline in the capability record; store audio externally and retain a content-addressed representation reference; defer audio entirely |
| Selected outcome | Homework supports at most one optional audio attachment, stored outside the capability record and retained as a content-addressed representation reference; publication must be observable to the student within 5 seconds. |
| Commitment effect | `decision.CommitmentEffectEstablishes` |
| Basis — evidence | `EV-0/EV-0-REV-1` — pilot-teacher interview notes |
| Basis — assumption | Audio files are hosted by an existing media service |
| Basis — constraint | No binary storage in the first release |
| Basis — uncertainty | Interview sample was 4 teachers |
| Authority | `featureforge:local-user`, as both requirement and basis |
| Actor | `featureforge:local-user` |
| Revisions concerned | Resolves Revision 1's open questions; is the recorded reason Revision 2 exists |
| Envelope | `RecordEnvelope` kind `decision` |
| Timeline label | *Decision recorded — audio stored externally by content address; 5-second latency* |

Alternatives are recorded with `decision.NewAlternative` and attached via
`WithAlternatives`, so the rejected options remain inspectable.

## 7. Validation chain

**Question:** Does Capability Revision 2 satisfy Requirements REQ-1 … REQ-4?

### Plan

`VP-1` / `VP-1-REV-1`, scope `featureforge:capability|CAP-1`, applicability
scoped to the same. Three activities — REQ-4 has none:

| Key | Method | Subject | Criteria | Expected evidence |
|---|---|---|---|---|
| `A-1` | `featureforge:manual-review` | `CAP-1/CAP-1-REV-2` | `REQ-1/REQ-1-REV-1` | Reviewer note confirming student visibility is specified |
| `A-2` | `featureforge:manual-review` | `CAP-1/CAP-1-REV-2` | `REQ-2/REQ-2-REV-1` | Reviewer note confirming non-student access is excluded |
| `A-3` | `featureforge:manual-inspection` | `CAP-1/CAP-1-REV-2` | `REQ-3/REQ-3-REV-1` | Inspection note confirming the attachment representation is resolvable |

### Executions, evidence, claims

| Execution | Activity | Outcome | Evidence | Claim | Claim outcome |
|---|---|---|---|---|---|
| `ER-1` | `A-1` | `completed` | `EV-1/EV-1-REV-1` | `CLM-1` | `satisfied` |
| `ER-2` | `A-2` | `completed` | `EV-2/EV-2-REV-1` | `CLM-2` | `satisfied` ← **incorrect** |
| `ER-3` | `A-3` | `completed` | `EV-3/EV-3-REV-1` | `CLM-3` | `satisfied` |
| `ER-4` | `A-2` (re-run) | `completed` | `EV-4/EV-4-REV-1` | `CLM-4` | `not-satisfied`, corrects `CLM-2` |

### The correction

`CLM-2` asserts REQ-2 is satisfied. A second reader observes that Revision 2
specifies who *may* view homework but never states that other users are
*excluded* — the exclusion is implied, not specified.

`CLM-4` is recorded with reasoning to that effect and
`core.NewRecordCorrectionRef(core.CorrectionKindCorrect, ref(CLM-2))`.

`correct` rather than `replace` or `invalidate`: the earlier claim was answering
the right question and got it wrong.

**Nothing is mutated.** `CLM-2`, `ER-2`, and `EV-2` are byte-identical before and
after, and `CLM-2` still reads `satisfied` when fetched directly.

### Evidence resolution

An evidence reference is resolved by looking up `RevisionEnvelope` at the
`RevisionKey` derived from the `EvidenceArtifactRevisionRef`'s artifact and
revision IDs, and asserting `RevisionFamily = evidence`. A reference that does
not resolve in C10 or a Claim is `ErrReferencedValueMissing` at write time.
AD-030/FF-022 make C8 the sole exception: a Decision may persist its exact
structurally valid evidence citation before that Evidence pair is locally
present. A read or timeline that requires the link to resolve fails loudly
until it exists (`ErrTimelineSourceInvalid` on the timeline). Evidence is never
resolved by scanning.

## 8. Lifecycle progression

| Assignment | State | Established by |
|---|---|---|
| `SA-1` | `featureforge:drafting` | `TR-1/TR-1-REV-0` — entry revision, no transition content (AD-014) |
| `SA-2` | `featureforge:specified` | `TR-1/TR-1-REV-1` — `specify`, `fromAssignment = SA-1`, after requirements/decision/accepted Revision 2 |
| `SA-3` | `featureforge:under-validation` | `TR-1/TR-1-REV-2` — `begin-validation`, `fromAssignment = SA-2`, after the first execution |

The scenario ends in `under-validation`, not `assessed`: REQ-4 was never
validated, so the assessment is not complete. This is a deliberate demonstration
that lifecycle state and readiness are independent — and the M.3 test suite also
asserts the `assessed` + `not-ready` pair is representable, which is the
duplication check AD-018 exists to enforce.

## 9. Expected end state

| Query | Expected result |
|---|---|
| Current capability revision | `CAP-1/CAP-1-REV-2`, sequence 2, accepted |
| Effective requirements | `REQ-1`, `REQ-2`, `REQ-3`, `REQ-4` |
| Applicable decisions | `DEC-1` with full basis |
| Current claim — REQ-1 | `CLM-1` `satisfied` |
| Current claim — REQ-2 | `CLM-4` `not-satisfied`, chain `CLM-2 → CLM-4` |
| Current claim — REQ-3 | `CLM-3` `satisfied` |
| Current claim — REQ-4 | none |
| **Release readiness** | **`not-ready`** — REQ-2 not satisfied; REQ-4 also reported uncovered |
| Lifecycle state | `featureforge:under-validation`, unique head `SA-3`, `LCD-1/LCDV-1`, established by `TR-1/TR-1-REV-2` |
| Timeline | All acts, ordered, `CLM-4` linked to `CLM-2` |
| History integrity | Revision 1 and `CLM-2` fully inspectable; nothing updated or deleted |

The scenario ends `not-ready` by design. A scenario ending green would prove only
that the happy path serializes; this one proves a wrong record can be corrected
without history being rewritten, and that the correction changes the derived
answer.
