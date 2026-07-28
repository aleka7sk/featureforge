# M.5 Contract Investigation — `EngineeringStateInput` and HTTP compatibility

Status: Final
Date: 2026-07-28
Phase: M.5 (pre-implementation investigation)
Governs: nothing. This is an investigation, not a specification. No contract is
modified by this document and no code was changed.

## Purpose and burden of proof

FF-015 §6.2 identified one architectural uncertainty in the M.5 plan:
`GetFeatureEngineeringState` and `GetFeatureTimeline` require the caller to
supply lists of requirement and decision identifiers, and an HTTP client
arriving with only a `featureCardID` does not have them.

This investigation asks whether the M.4-validated architecture supports HTTP
**unchanged**. The M.4 Architecture Freeze is treated as binding, and its
standard is applied literally:

> Reopening any frozen decision requires evidence of comparable weight to what
> M.4 produced: a working implementation that cannot satisfy the contract, a
> contract test that cannot be made to pass, or a reproducible failure the
> current design cannot express.

**The burden of proof is on changing the architecture, not on preserving it.**
A transport-level solution that is merely uglier than a contract change wins.
Only a transport-level solution that is *impossible or wrong* loses.

**Conclusion in brief.** The question has two halves with different answers.
**Decisions are fully solvable at the transport layer with no contract change
whatsoever.** **Requirements are not**, and the reason is not inconvenience: every
transport-level technique available produces a *wrong answer* rather than an
awkward one. The minimum change is identified in §6, and it extends a pattern
M.4 already validated rather than introducing a new one.

---

## 1. Why the current contract requires caller-supplied identifiers

### 1.1 What the code actually says

`internal/application/query_state.go`:

```go
type EngineeringStateInput struct {
	CapabilityArtifactID   string
	RequirementArtifactIDs []string
	DecisionIDs            []string
}
```

Its doc comment states the reason plainly: *"names the records
GetFeatureEngineeringState draws from, for the same reason TimelineInput does
(FF-009 §5 defines no requirement-to-capability index)."*

`TimelineInput` needs more of the same: `RequirementArtifactIDs`,
`DecisionIDs`, `PlanArtifactID`, `ExecutionIDs`, `EvidenceArtifactIDs`,
`ClaimIDs`.

### 1.2 The structural cause

The cause is precise and is visible in one asymmetry between the two envelope
types (`internal/engineering/envelope.go`):

| Envelope | Projects a subject? |
|---|---|
| `RecordEnvelope` | **Yes** — `SubjectKey string`, plus `Scope`, `CriterionKeys`, `EvidenceKeys`, `ExecutionKeys` |
| `RevisionEnvelope` | **No** — `Key`, `RevisionFamily`, `ArtifactType`, `IntegrityValue`, provenance, digests, payload. No subject field of any kind |

And the repository contract offers exactly these listing capabilities
(`internal/application/ports.go`):

```
ProjectRepository.List()
FeatureCardRepository.ListByProject(projectID)
RevisionEnvelopeRepository.ListByArtifact(artifactID)
RecordEnvelopeRepository.ListByKind(kind)
RecordEnvelopeRepository.ListByKindAndSubject(kind, subjectKey)
RevisionOrderRepository.ListByArtifact(artifactID)
RevisionAcceptanceRepository.ListByRevision(key) / ListByArtifact(artifactID)
```

`ArtifactEnvelopeRepository` has **no** `List`. `RevisionEnvelopeRepository`
can list only *within one already-known artifact*.

The consequence follows mechanically:

> **A record is discoverable by its subject. A revision is not discoverable at
> all unless you already know its artifact ID.**

Requirements, validation plans, and evidence are all `RevisionEnvelope`s.
Decisions, executions, claims, and state assignments are all
`RecordEnvelope`s.

So `EngineeringStateInput` does not ask the caller for identifiers out of
laziness. It asks because, for requirements, **there is no query that could
answer the question**, and the application layer cannot decode a requirement's
PEOS payload to find its subject — `internal/engineering/peos` is the only
package permitted to do that (AD-005).

### 1.3 Why the asymmetry exists

This is not an oversight. `RecordEnvelope`'s projections were introduced
because the correction-chain and readiness algorithms need to *search* by
subject, scope, and criteria (FF-010 §6, §7). `RevisionEnvelope` needed no such
search: every revision query in M.3 and M.4 starts from a known artifact —
`ResolveCurrentRevision(ctx, repos, artifactID)`. The projection set matched
the queries that existed. It still does. What changed is that M.5 introduces a
caller who does not know the artifact ID.

---

## 2. Why this was appropriate for scenario-driven execution

It was appropriate, and it should not be characterised as debt that M.3 and M.4
overlooked.

**The caller genuinely knew.** In M.3 and M.4 the only caller was
`internal/scenario`, which *created* every record moments earlier. `Run` holds
`RequirementArtifactIDs`, `DecisionID`, `PlanArtifactID`, `ExecutionIDs`,
`EvidenceIDs`, and claim IDs as package constants in `fixtures.go`, and returns
them in `scenario.Result` precisely so the assertions can name them. Asking the
scenario driver to supply what it just wrote is not a workaround; it is the
shortest correct path.

**It honoured "no abstraction before it is required."** CLAUDE.md instructs:
*"Do not add abstractions before they are required by the approved vertical
slice."* An index enabling requirement discovery would have been an untested,
unexercised abstraction in both M.3 and M.4 — exactly the kind of speculative
generality the project has repeatedly refused (the `cmd/` directory was dropped
from M.3 on identical reasoning, FF-008).

**It was documented, not hidden.** FF-009 §5 states no such index exists, and
both input structs say so in their doc comments. The M.3 implementation report
listed it as a known limitation. Nothing was concealed.

**It is consistent with AD-006.** Storing a requirement list somewhere would be
derived state, and the project has systematically refused to store derived
state — including a test, `TestNoDerivedStateOnFeatureCard`, that fails the
build if a `FeatureCard` grows a collection field.

The design was correct for its callers. The question now is whether a new
caller invalidates it.

---

## 3. Why an HTTP client struggles

### 3.1 The immediate problem

An HTTP request arrives as `GET /api/v1/features/{featureCardID}/state`. The
server can resolve, from the `FeatureCard`, exactly one engineering identifier:
`CapabilityArtifactID`, via `FeatureCard.CapabilityArtifactID()`. Nothing else.

The client cannot supply requirement IDs, because a browser loading a page for
the first time has never seen them. There is no prior response to have learned
them from — the screen that would list them is the one being rendered.

### 3.2 It is not only an API-shape problem

Two accepted acceptance criteria independently require requirement enumeration,
so this cannot be dismissed as an artefact of one endpoint's signature:

- **FF-001 §3.4 (Requirements screen)** — *"Data: Each requirement's current
  revision and statement; the acceptance criterion it derives from; its current
  claim and outcome."* The screen's entire purpose is to enumerate
  requirements. It cannot ask the reader which ones exist.
- **FF-001 §3.2 (Feature overview)** — requires "effective requirement count"
  and readiness *"with the per-requirement rationale table"*.

FF-007's M.5 exit criteria require that "a reader answers every question in
FF-001 §3's usability acceptance from the UI alone". A requirements screen that
cannot find requirements fails that criterion directly.

### 3.3 The decisive case: `REQ-4`

The canonical scenario contains a requirement that is **deliberately
uncovered** — FF-011 §5 lists `REQ-4` with plan activity "none" and status
"uncovered — no claim". FF-011 §252 explains the intent: it exercises
`incomplete` alongside `not-ready` so the readiness precedence rule "has both
inputs present and is genuinely tested rather than assumed."

This makes `REQ-4` the test case that eliminates most transport-level
workarounds, as §4 shows. Any discovery technique that finds requirements *by
following records that mention them* cannot find `REQ-4`, because nothing
mentions it — no plan activity, no execution, no claim. And `REQ-4`'s absence
of a claim is **precisely the fact readiness exists to report**.

A discovery method that silently omits uncovered requirements does not return a
degraded answer. It returns a **confidently wrong** one: a readiness report
that appears complete while hiding the only requirement nobody validated.

---

## 4. Transport-level options, evaluated

Every realistic option, assessed against architectural impact, implementation
complexity, M.4-freeze compatibility, PEOS-philosophy compatibility, and
whether a new Architecture Decision would be required.

### Option A — Client supplies the identifiers (pure pass-through)

Transport accepts `?requirementId=REQ-1&requirementId=REQ-2&…` and forwards
them.

- **Architectural impact:** none. The contract is used exactly as designed.
- **Complexity:** trivial.
- **M.4 freeze:** fully compatible.
- **PEOS philosophy:** neutral.
- **New AD:** no.
- **Verdict: insufficient.** It relocates the problem to a client that also
  cannot answer it. A first page load has no source for the list. It also fails
  FF-001 §3.4 outright: a requirements screen whose content depends on the
  reader already knowing the requirements is not a requirements screen.

### Option B — Derive requirements from records that cite them

Claims project `CriterionKeys` as `requirement-revision:REQ-n/REQ-n-REV-1`
(`internal/engineering/refkeys.go`). So: `ListByKind(claim)`, filter by
subject, parse the criterion keys, recover requirement IDs. Validation plans
also cite requirements — but a plan is a `RevisionEnvelope` and projects
nothing, so only claims and executions are usable.

- **Architectural impact:** none structurally — it uses existing projections.
- **Complexity:** moderate; requires parsing projected keys, which
  `ParseSubjectKey`'s sibling functions already make principled.
- **M.4 freeze:** compatible.
- **PEOS philosophy:** compatible — reads only FeatureForge projections, never
  PEOS payloads.
- **New AD:** no.
- **Verdict: rejected, and this is the important rejection.** It finds only
  requirements that something *already claims*. `REQ-4` — uncovered by
  construction — is invisible to it. The method's blind spot is exactly the
  population readiness must report, so it converts "incomplete" into "complete"
  silently. This is not a limitation to document; it is a correctness defect.
  A transport layer that produces a different engineering answer than the
  application layer would have produced violates FF-007's "no new business
  logic" at the deepest level: it would be *deciding what the requirement set
  is*.

### Option C — Naming convention (`CAP-1` → `REQ-1…REQ-n`)

- **Architectural impact:** severe. It would make identity *format* load-bearing
  and place identity policy in the transport layer.
- **Complexity:** low to write, unbounded to maintain.
- **M.4 freeze:** incompatible in spirit. AD-003 explicitly forbids assuming
  identifiers are sortable or structured; PEOS-002 forbids assuming Revision
  Identifiers carry ordering semantics. This extends the same forbidden
  assumption to discovery.
- **PEOS philosophy:** directly contrary. PEOS treats identity as opaque.
- **New AD:** would require one, and it would contradict AD-003.
- **Verdict: rejected.** `REQ-1…REQ-4` is a fixture convention in
  `internal/scenario/fixtures.go`, not a contract. Nothing prevents a
  requirement named `AUTH-7`.

### Option D — A transport-side index or registry

Transport maintains its own map of capability → requirements, updated as
commands pass through it.

- **Architectural impact:** severe. It creates a second source of truth for a
  relationship the engineering records already contain, outside the layer that
  owns engineering meaning, with no transaction covering it.
- **Complexity:** high — it must survive restart, stay consistent with writes
  that bypass it, and be rebuildable. It is a materialized projection wearing a
  different hat.
- **M.4 freeze:** compatible in letter (no repository contract changes) and
  incompatible in substance.
- **PEOS philosophy:** contrary. It is derived state stored as authoritative —
  what AD-006 and the PEOS consumer guide both warn against.
- **New AD:** would require one, superseding AD-006 in practice.
- **Verdict: rejected.** The one thing worse than a missing index is an index
  that can disagree with the records.

### Option E — Enumerate and decode payloads in transport

Read every revision, decode its PEOS payload, extract the subject.

- **Architectural impact:** fatal. Decoding requires importing the PEOS SDK
  outside `internal/engineering/peos`.
- **Complexity:** irrelevant.
- **M.4 freeze:** incompatible.
- **PEOS philosophy:** directly violates AD-005, the project's most fundamental
  boundary.
- **New AD:** would require superseding AD-005.
- **Verdict: rejected outright.** Also impossible in practice: no repository
  method enumerates revisions globally, so there is nothing to iterate.

### Option F — Application composition (a new query that discovers)

Add `ListRequirementsForCapability` to `internal/application`, composed from
existing repository methods.

- **Architectural impact:** none *if it could work*.
- **Complexity:** low.
- **M.4 freeze:** compatible — the freeze covers repository contracts and
  transaction semantics, not the set of application queries.
- **PEOS philosophy:** compatible.
- **New AD:** no.
- **Verdict: rejected — it cannot be written.** This option is the most
  attractive on paper and fails on the same wall as everything else: the new
  query would need a repository capability that does not exist. It would have
  to call `Revisions.ListByArtifact(requirementArtifactID)` — which requires the
  answer as input. Composition cannot create information the composed parts do
  not carry.

### Option G — Decisions specifically: enumerate revisions, then query records

For **decisions only**, a genuine transport-level solution exists:

1. `Revisions.ListByArtifact(capabilityArtifactID)` → every capability revision.
2. For each, `Records.ListByKindAndSubject(RecordKindDecision,
   ArtifactRevisionSubjectKey(capabilityID, revisionID))`.
3. Union the results.

This works because a decision is a `RecordEnvelope` and *does* project its
subject — `codec_decision.go:131` sets
`subjectKey = ArtifactRevisionSubjectKey(in.SubjectArtifactID, in.SubjectRevisionID)`,
and FF-011 §4.3 confirms a decision's subject is a capability revision.

- **Architectural impact:** none. Uses two existing methods exactly as
  designed.
- **Complexity:** low — a loop over a small collection.
- **M.4 freeze:** fully compatible.
- **PEOS philosophy:** compatible; reads only projections.
- **New AD:** no.
- **Verdict: accepted.** `DecisionIDs` needs no contract change.

The same technique resolves most of `TimelineInput`:

| Input field | Discoverable at transport? | How |
|---|---|---|
| `DecisionIDs` | **Yes** | Option G |
| `ExecutionIDs` | **Yes** | `ListByKind(execution)`, filter by subject |
| `ClaimIDs` | **Yes** | `ListByKind(claim)`, filter by subject |
| `EvidenceArtifactIDs` | **Yes** | Parse `EvidenceKeys` projected on claims and executions |
| `RequirementArtifactIDs` | **No** | §4 options A–F all fail |
| `PlanArtifactID` | **No** | A `RevisionEnvelope`; nothing projects it |

Five of seven dissolve. Two remain, and both are `RevisionEnvelope`s — the
envelope with no subject projection.

### Option H — Store the list on the `FeatureCard`

- **Verdict: rejected immediately.** `TestNoDerivedStateOnFeatureCard` fails
  the build on any collection field, and AD-001 and AD-006 both forbid it. This
  option is listed only to record that it was considered and is
  test-prohibited.

### Option I — `RelationEnvelope`

Model requirement→capability as a PEOS `relation.Relation` and add the fourth
envelope AD-013 deferred.

- **Architectural impact:** large. A new envelope, a new repository, a new
  codec path.
- **Complexity:** high.
- **M.4 freeze:** would require substantial new contract surface.
- **PEOS philosophy:** compatible in principle — `peos/relation` is a real
  package and `Relation` is JSON-marshalable. But PEOS-002 forbids `Relation`
  having identity, revision history, or lifecycle, so it needs a composite
  `(type, from, to, scope)` key that `RecordEnvelope` cannot express — exactly
  AD-013's stated reason for deferring it.
- **New AD:** yes, superseding part of AD-013.
- **Verdict: rejected as disproportionate.** It would introduce an entire
  construct the canonical scenario never records, to solve a problem a single
  projected string solves. AD-013's deferral reasoning still holds.

---

## 5. Findings

**The uncertainty is smaller than FF-015 estimated, and sharper.**

1. **Decisions require no contract change.** Option G resolves `DecisionIDs`
   using two existing repository methods. FF-015 §6.2 treated requirements and
   decisions as one problem; they are not.
2. **Four of `TimelineInput`'s six list fields are likewise discoverable.**
3. **Requirements and the validation plan cannot be discovered**, and the
   obstacle is one structural fact: `RevisionEnvelope` projects no subject,
   while every listing method for revisions requires a known artifact ID.
4. **Every transport-level technique for requirements is either impossible
   (E, F), forbidden (C, E, H), architecturally worse than the problem (D, I),
   or silently wrong (B).**

Option B deserves emphasis because it is the one that would have been adopted
by default. It looks like a clean projection-based derivation, uses no
forbidden capability, and passes casual inspection — and it systematically
hides uncovered requirements. `REQ-4` exists in the canonical scenario
specifically to make this class of error visible, and it does.

---

## 6. If the contract must change — the minimum change

### 6.1 Why it cannot remain hidden in the transport layer

The M.4 freeze's standard is *"a reproducible failure the current design cannot
express."* That standard is met, and narrowly:

The current design **cannot express the question** "which requirements govern
this capability?" The information exists — it is inside each requirement
revision's PEOS payload — but the only package permitted to read it (AD-005) is
not permitted to be called from a query path that needs it, and no projection
carries it outward. This is not a missing convenience method. It is a fact the
persistence contract does not surface at all.

A transport layer cannot hide the change because a transport layer cannot
create the information. Options A–F establish that every route either needs a
capability the contract lacks, needs a forbidden import, or produces an answer
that differs from the truth.

### 6.2 The smallest change

**Project the subject a revision already has, mirroring what `RecordEnvelope`
already does.**

1. `engineering.RevisionEnvelope` gains one optional field —
   `SubjectKey string` — populated by the codec for revision families that have
   a subject (`requirement` → the capability artifact; `validation-plan` → its
   scope). Empty for families with no subject (`capability`, `evidence`),
   exactly as `ContentDigest` is already optional.
2. `RevisionEnvelopeRepository` gains one method —
   `ListByFamilyAndSubject(ctx, family, subjectKey) ([]RevisionEnvelope, error)`
   — mirroring the existing, validated
   `RecordEnvelopeRepository.ListByKindAndSubject`.

Both adapters implement it: PostgreSQL adds a nullable `subject_key` column and
an index in migration `0002` (a mirror of `record_envelopes_kind_subject_idx`);
memory adds a filtered scan. The shared contract suite gains subtests, so both
adapters are held to identical behaviour — the mechanism M.4 validated.

**Why this is minimal:**

- **Additive only.** No existing signature changes. No existing field changes
  meaning. `EngineeringStateInput` and `TimelineInput` keep their shape; the
  transport simply becomes able to populate them.
- **It introduces no new architectural direction.** It extends to
  `RevisionEnvelope` exactly the projection pattern `RecordEnvelope` has had
  since M.3 and that the correction, readiness, and lifecycle algorithms
  already depend on. M.4's review confirmed the principle it rests on:
  *repository contracts constrain observable behaviour, not storage
  representation* (§9.2).
- **It stores nothing derived.** `SubjectKey` is a projection of a value already
  inside the immutable payload, written once at record time by the codec —
  identical in kind to `RecordEnvelope.SubjectKey`. AD-006 is untouched: no
  current-state answer is materialized.
- **It preserves AD-005.** Only `internal/engineering/peos` extracts the
  subject; everyone else reads the projected string.

**One honest caveat.** Adding the column is straightforward for an empty
database, which is the only situation this POC has. Back-filling
`subject_key` for revisions written before migration `0002` would require
decoding stored payloads, and only `internal/engineering/peos` may do that — so
a back-fill would be a one-off tool in that package, not a SQL migration. For
FeatureForge this is moot; it is recorded because it is the kind of thing that
is not moot later.

### 6.3 Alternative considered and rejected as smaller-but-wrong

A bare `RevisionEnvelopeRepository.ListByFamily(family)` with transport-side
filtering would avoid the projection. It fails: with no `SubjectKey`, the
transport still cannot tell which requirements belong to *this* capability. It
would return every requirement in the store. Smaller in diff, useless in
effect.

### 6.4 Is AD-025 required?

**Yes** — if and only if the change in §6.2 is adopted.

FF-015 §17 provisionally reserved AD-025 for exactly this, conditioned on the
gap proving unresolvable by filtering. This investigation establishes that it
is unresolvable for requirements and the validation plan, and *resolvable* for
decisions, executions, claims, and evidence.

AD-025 should therefore be narrower than FF-015 anticipated. It should record:

- the structural finding (§1.2): records project subjects, revisions do not;
- that decisions and four of `TimelineInput`'s fields need no change (Option G);
- that Option B was rejected for producing wrong answers, with `REQ-4` as the
  worked example — this is the reasoning most likely to be re-litigated later,
  and it should be written down while the evidence is fresh;
- the additive projection plus one repository method as the accepted change;
- that AD-006 and AD-005 are unaffected, with reasons.

It supersedes nothing. FF-009 §5's statement that no requirement-to-capability
index exists becomes outdated on adoption and should be corrected by an errata
note in the M.5 documentation rather than by editing accepted text — the
append-never-rewrite discipline FF-014 §9 used for the same situation.

---

## 7. Recommendation

1. **Adopt Option G immediately and unconditionally.** Decisions, executions,
   claims, and evidence are discoverable today. Implement that in transport
   during Phase A step 5, with no contract change and no decision record.
2. **For requirements and the validation plan, adopt the §6.2 projection**,
   recorded as AD-025 before the code that depends on it, per CLAUDE.md.
3. **Do not adopt Option B**, and record why. It is the trap this investigation
   exists to have found.

The M.4 freeze is satisfied, not bypassed: the burden of proof was applied,
four options were rejected for being merely awkward or forbidden, one was
rejected for being *wrong in a way that would not have shown up in testing*,
and the surviving change is the smallest additive extension of a pattern the
freeze itself validated.

**Nothing in this document has been implemented. No contract has been
modified.**
