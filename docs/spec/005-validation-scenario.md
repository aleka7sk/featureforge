# FF-005 — Canonical Validation Scenario

Status: Accepted (Phase M.1)
Governs: the one scenario the entire project is judged against, including the
validation chain and the correction flow.

## 1. Scope discipline

This is a **modelled engineering validation**. FeatureForge records that a
validation was planned, executed, and claimed. It does not run the homework
application, because there is no homework application. The "execution" is a human
review, its outcome is entered by that human, and its evidence is a document
cited by reference.

That is not a shortcut around the hard part. The hard part — immutable records,
provenance, criteria that cite exact requirement revisions, corrections that
create history — is fully exercised. What is deliberately absent is the
operational Belcanto domain, which [FF-002](002-domain-boundaries.md) keeps out.

## 2. The canonical question

> **Does the current capability revision satisfy the approved requirements for
> homework publication and student access?**

## 3. Scenario content

The following is *content* — text stored inside specification revisions and
requirement statements. None of it becomes a FeatureForge type, table, or
operation.

**Capability:** Homework after a lesson.
A teacher publishes homework after a lesson. The student must be able to see the
homework, including an optional audio attachment.

### Capability Revision 1 (sequence 1)

| Field | Content |
|---|---|
| Title | Homework after a lesson |
| Problem statement | After a lesson ends, a teacher has no way to give the student follow-up work, so assignments are passed verbally and lost. |
| User outcome | A student can see the homework their teacher set after a lesson. |
| Functional behaviour | A teacher can attach homework to a completed lesson; a teacher can publish it; a published homework becomes visible to that lesson's student. |
| Constraints | Homework is visible only to the student of that lesson. |
| Acceptance criteria | `AC-1` Published homework is visible to the intended student. `AC-2` Homework is not visible to any unrelated user. |
| Dependencies | Lesson completion state must be available. |
| Open questions | Should homework support an audio attachment? What is the acceptable publication latency? |

Revision 1 is **accepted** when recorded. It is the current revision until
Revision 2 is accepted, and it remains fully inspectable forever afterward.

### Capability Revision 2 (sequence 2)

Revision 2 exists because the decision in §5 resolves Revision 1's open
questions. It is a new immutable revision; Revision 1 is not edited.

Changes from Revision 1:

| Field | Change |
|---|---|
| Functional behaviour | Adds: a teacher may attach one optional audio file to homework; a published audio attachment is retrievable by the student. |
| Constraints | Adds: publication completes within 5 seconds of the teacher's action. |
| Acceptance criteria | Adds `AC-3` An optional audio attachment has a resolvable representation for the student. Adds `AC-4` Publication is observable to the student within 5 seconds. |
| Open questions | Both original questions are removed; they are now answered by the decision. |

Both revisions carry the same Artifact ID and different Revision IDs. Both are
readable side by side in the Revisions screen.

## 4. Requirements

Three Requirement Artifacts, each with one revision. Each cites the acceptance
criterion it derives from, in its own statement text.

| Requirement | Statement | From |
|---|---|---|
| **R-1** | Published homework SHALL be visible to the student of the lesson it belongs to. | AC-1 |
| **R-2** | Published homework SHALL NOT be visible to any user who is not the student of that lesson. | AC-2 |
| **R-3** | Where homework has an audio attachment, that attachment SHALL have a representation the student can resolve. | AC-3 |

Each Requirement's subject is the capability Artifact, converted through
`core.EngineeringSubjectRefFromArtifact`. The requirement package never sees a
FeatureForge type, and the validation package never sees a requirement type —
the crossing happens in FeatureForge's integration layer, which is the one place
permitted to import both.

**AC-4** (the 5-second latency criterion) is deliberately **not** promoted to a
requirement in this scenario. It is the criterion that stays uncovered, so that
release readiness has something real to report as `incomplete`, and so that the
"every approved requirement is covered by evidence" check has a genuine negative
case to distinguish from a positive one.

## 5. The decision

| Field | Content |
|---|---|
| Question | Should homework support an optional audio attachment, and what publication latency is acceptable? |
| Outcome statement | Homework supports at most one optional audio attachment, referenced by URL rather than stored inline, and publication must be observable to the student within 5 seconds. |
| Rationale | Referencing by URL avoids introducing binary storage into the first release; 5 seconds is the longest delay the pilot teachers described as acceptable. |
| Subjects | The capability Artifact, and Capability Revision 1 |
| Basis — evidence | An evidence Artifact Revision holding the pilot-teacher interview notes |
| Basis — assumption | Audio files are hosted by an existing media service |
| Basis — constraint | No binary storage in the first release |
| Basis — uncertainty | Interview sample was 4 teachers |

The decision is the recorded reason Revision 2 exists. Its basis is what makes
that reason auditable, which is the point of recording it at all.

## 6. The validation chain

```
Requirement (R-1, R-2, R-3)
     │  cited as criteria
     ▼
Validation Plan Revision  ── planned activities A-1, A-2, A-3
     │  executed
     ▼
Execution Record (per activity)  ── outcome: completed / failed / …
     │  produced
     ▼
Evidence (Artifact Revision in the Evidence role)
     │
     ▼
Result  ── execution outcome  +  claim outcome     (two things, not one)
     │
     ▼
Validation Claim (satisfaction)  ── subject: Capability Revision 2
                                    criteria: R-1 / R-2 / R-3 revisions
```

### The plan

One Validation Plan Artifact with one Plan Revision. Its scope is the capability;
its applicability names Capability Revision 2. Three planned activities, each
with a plan-local key:

| Key | Method | Subject | Criteria | Expected evidence |
|---|---|---|---|---|
| `A-1` | `featureforge:manual-review` | Capability Revision 2 | R-1's current revision | Reviewer note recording that student visibility is specified and traceable |
| `A-2` | `featureforge:manual-review` | Capability Revision 2 | R-2's current revision | Reviewer note recording that non-student access is excluded |
| `A-3` | `featureforge:manual-inspection` | Capability Revision 2 | R-3's current revision | Inspection note recording that the attachment representation is specified as resolvable |

Criteria are cited at the **exact requirement revision** level, not at the
requirement identity level. This is what makes a claim's meaning stable when a
requirement is later revised: the claim says what it was evaluated against, and
that never drifts.

### Execution and evidence

Three Execution Records, one per activity, each naming its planned activity by
`(Plan Revision reference, plan-local key)`. Each carries actor, completion time,
method, criteria, outcome, and the evidence it produced.

Evidence is a `featureforge:validation-evidence` Artifact with one revision per
document, in the Evidence role, whose Representation is an external reference
with media type `featureforge:validation-report`. Evidence is always cited at the
exact Revision level.

### Claims

Three satisfaction claims, one per requirement. Each has:

- subject: Capability Revision 2;
- criteria: that requirement's current revision;
- scope: the capability;
- outcome: `satisfied` or `not-satisfied`;
- the execution records that support it, and the evidence they produced;
- reasoning, in prose;
- provenance.

## 7. The correction flow

This is the flow the acceptance contract cares most about, because it is where a
system that quietly mutates history gets caught.

### What goes wrong

Activity `A-2` checks that homework is **not** visible to unrelated users. The
reviewer misreads Revision 2's constraint, concludes the exclusion is specified,
and records:

> **Claim C-2** — subject: Capability Revision 2; criteria: R-2 revision 1;
> outcome: **satisfied**; supported by execution record E-2; evidence: reviewer
> note V-2.

Later, a second reader notices that Revision 2 specifies who *can* see homework
but never states that anyone else *cannot* — the exclusion is implied, not
specified. C-2 is wrong.

### What must not happen

C-2 is not edited. Its outcome is not flipped. It is not deleted. E-2 is not
touched. V-2 is not touched. Nothing that was recorded changes.

### What happens instead

A second review is executed, producing a new execution record E-4 and new
evidence V-4. Then a **new claim** is recorded:

> **Claim C-4** — subject: Capability Revision 2; criteria: R-2 revision 1;
> outcome: **not-satisfied**; supported by E-4; evidence V-4; reasoning: "Revision
> 2 specifies who may view homework but does not state that other users are
> excluded. The original review treated the positive statement as implying the
> exclusion. It does not."
> **Correction reference:** kind `correct`, target **C-2**.

After this:

| | |
|---|---|
| C-2 | Still stored, still readable, still `satisfied`, now superseded |
| C-4 | Current for R-2, outcome `not-satisfied` |
| E-2, V-2 | Untouched and still readable |
| Release readiness | `not-ready`, because R-2's current claim is `not-satisfied` |
| Timeline | Shows C-2, then E-4, then V-4, then C-4 with an explicit "corrects C-2" link |

The correction is a new fact about the past, not a change to it. Choosing kind
`correct` rather than `replace` or `invalidate` is itself meaningful: the earlier
claim was answering the right question and got it wrong.

### Required tests

- after correction, C-2 is still retrievable and still reads `satisfied`;
- E-2 and V-2 are byte-identical before and after;
- the current-claim query for R-2 returns C-4 and reports the chain;
- release readiness flips from `ready` to `not-ready` with R-2 named;
- a correction naming a non-existent claim is rejected;
- a correction cycle is rejected;
- a second uncorrected claim for the same subject, scope, and criteria produces
  `ErrAmbiguousCurrentClaim`.

## 8. Concrete validation checks, and where each lands

The checks the scenario was asked to model, and their honest disposition:

| Check | Where it lands |
|---|---|
| Published homework is visible to the intended student | R-1 → A-1 → E-1 → V-1 → C-1 (`satisfied`) |
| Student access does not expose homework to unrelated users | R-2 → A-2 → E-2 → V-2 → C-2 (`satisfied`, **wrong**) → corrected by C-4 (`not-satisfied`) |
| Optional audio attachment has a resolvable representation | R-3 → A-3 → E-3 → V-3 → C-3 (`satisfied`) |
| Publication result available within the time constraint | AC-4 only — deliberately no requirement, no plan activity, no claim. Surfaces as an uncovered acceptance criterion. |
| Every approved requirement is covered by evidence | Not a claim. This is the release-readiness query ([FF-004 §3.6](004-current-state-resolution.md#36-release-readiness)), computed over the three requirements. |

The last row is the one most likely to be got wrong by an implementation:
"every requirement is covered" is a **derived view**, and recording it as a Claim
would make a computed verdict authoritative. It stays a query.

## 9. End state

After the full scenario:

| Question | Answer |
|---|---|
| Current capability revision | Revision 2 (sequence 2, accepted) |
| Effective requirements | R-1, R-2, R-3 |
| Applicable decision | The audio-attachment and latency decision, with basis |
| Current claims | C-1 `satisfied`, C-4 `not-satisfied` (correcting C-2), C-3 `satisfied` |
| Release readiness | `not-ready` — R-2 is not satisfied |
| Lifecycle state | `featureforge:under-validation` (renamed by [AD-018](../decisions/README.md#ad-018--the-lifecycle-state-validated-is-renamed-assessed-and-redefined)) |
| Timeline | Every act above, in order, with actors, timestamps, and the correction link |
| History integrity | Revision 1 and claim C-2 both fully inspectable; nothing updated, nothing deleted |

That the scenario ends **not ready** is intentional. A scenario that ends green
proves only that the happy path serializes. This one proves that a wrong record
can be corrected without history being rewritten, and that the correction
propagates into the derived answer.
