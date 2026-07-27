# FF-004 — Current-State Resolution

Status: Accepted (Phase M.1)
Governs: the revision ordering contract, every current-state query, and the
determinism and ambiguity rules that apply to all of them.

## 1. Why this document exists

PEOS deliberately refuses to define "current". PEOS-002 §Revision Ordering says
revisions *may* have an explicit ordering, lists five permitted mechanisms, and
forbids assuming that Revision Identifiers are sortable. The PEOS consumer guide
states that current-revision selection, current lifecycle state, and "the most
recent non-invalidated Claim" are all consumer responsibilities.

So FeatureForge must define them. If it does not, an implementation agent will
invent one silently — probably `ORDER BY created_at` — and the project will have
failed its own purpose.

## 2. The revision ordering contract

### The policy

Applies to capability specification revisions. Requirement revisions and
validation plan revisions follow the same policy, since they are also Artifact
Revisions of FeatureForge-managed Artifacts.

1. Every specification revision has an **integer sequence**, owned by
   FeatureForge, stored in the FeatureForge Revision Sequence record — never
   inside the PEOS revision.
2. The sequence is **unique within one Artifact**. Across Artifacts, sequences
   are independent.
3. Sequences **start at 1** and are dense: the next sequence for an Artifact is
   `max(existing) + 1`.
4. Sequence assignment is **transactional**, in the same transaction that writes
   the PEOS revision and the specification content. Two concurrent attempts to
   create revision *n* cannot both succeed.
5. Every revision has an **acceptance state**: `draft`, `accepted`, or
   `withdrawn`.
6. **The current revision is the accepted revision with the greatest sequence.**
7. **Insertion order is irrelevant.** Storage order, creation timestamps, and
   revision identifier strings play no part in resolution.
8. **Duplicate or ambiguous sequence is an error**, raised explicitly. It is
   never repaired by picking one.
9. **Resolution returns both the revision and a rationale** explaining how it was
   chosen and what was rejected.

### Challenged against PEOS-002

| PEOS-002 statement | Consequence for this policy |
|---|---|
| "Revisions of an Artifact MAY have an explicit ordering." | Permissive. FeatureForge choosing to have one is conformant. |
| Ordering may be expressed through "sequence numbers; timestamps; predecessor relationships; version identifiers; another deterministic mechanism." | Sequence numbers are the **first listed** mechanism. The policy uses exactly this. |
| "An implementation MUST NOT assume that Revision Identifiers are inherently sortable unless their governing contract explicitly guarantees ordering semantics." | The policy never sorts Revision IDs. Rule 7 makes this explicit. **Conformant.** |
| "The Artifact Model does not require every Artifact to have a single linear revision history. Branching Revision histories are permitted." | *Permitted*, not required. FeatureForge's contract is the governing Product contract for its own Artifacts, and it declares linear history. **Conformant, and it is a narrowing, not a contradiction.** |
| An Artifact Revision carries no revision number and no status field. | The sequence and the acceptance state therefore live in FeatureForge storage, never written back into the PEOS value. Rule 1. **Conformant.** |

No conflict with PEOS-002 was found. The policy is approved. Recorded as
**AD-003**.

### Sequence ownership

FeatureForge owns the sequence, exclusively. It is:

- assigned by the application layer inside the engineering-act transaction;
- stored in the FeatureForge Revision Sequence record, keyed by exact
  `(ArtifactID, ArtifactRevisionID)`;
- never present in any PEOS payload;
- never inferred from anything.

### Acceptance state

Acceptance is FeatureForge's answer to "which revision text is authoritative
right now". It is deliberately **not** the PEOS lifecycle state, which answers
"how far has this capability progressed" ([FF-003 §4](003-peos-integration.md#4-lifecycle-in-scope-and-what-it-costs)).
Conflating them would either force a full lifecycle per revision or hide a
lifecycle inside an ad-hoc boolean.

| State | Meaning |
|---|---|
| `draft` | Recorded, inspectable, not authoritative |
| `accepted` | Authoritative if it has the greatest sequence among accepted revisions |
| `withdrawn` | Recorded, inspectable, permanently not authoritative |

Acceptance is the single permitted product-owned mutable transition in the
system. It is constrained:

- permitted transitions are `draft → accepted`, `draft → withdrawn`, and
  `accepted → withdrawn`;
- `withdrawn` is terminal, and `accepted → draft` is rejected;
- every transition is journalled as an append-only FeatureForge record carrying
  actor, timestamp, and reason, and that journal feeds the timeline.

The PEOS revision itself is never touched by any of this.

### Ambiguity behaviour

| Condition | Behaviour |
|---|---|
| Two revisions of one Artifact share a sequence | Error `ErrAmbiguousRevisionSequence`, naming both revision IDs. Never resolved by tie-break. |
| Sequence gap (1, 2, 4) | Error at write time — rule 3 makes gaps unreachable. If found on read, error `ErrRevisionSequenceGap`. |
| No accepted revision exists | Not an error. The query returns "no current revision" with a rationale, and the UI shows the drafts. |
| All revisions withdrawn | Same as above. |
| Accepted revision missing its specification content | Error `ErrMissingSpecificationContent`. A revision without content is a broken write, not a valid state. |
| Content digest disagrees with the revision's recorded integrity value | Error `ErrContentIntegrityMismatch`. Never silently served. |

Every error names the exact revisions involved. No current-state query ever
guesses.

### Branch behaviour

Branching is **rejected** for the POC. There is one linear sequence per Artifact,
and a second revision claiming an existing sequence is a conflict.

This is a narrowing of what PEOS-002 permits, taken because branching would
require a merge and precedence policy that the canonical scenario never
exercises, and inventing one now would be an abstraction ahead of need.

### Future extension path

If branching is ever required, the extension is additive and does not invalidate
stored data: add a product-owned branch label, make the sequence unique within
`(Artifact, branch)`, and extend resolution to take a branch as a parameter with
`main` as the default. Existing records are all on the default branch. Nothing
recorded under the current contract has to be rewritten.

### Required tests

- revisions inserted in the order 3, 1, 2 resolve identically to 1, 2, 3;
- resolution ignores creation timestamps entirely — a later-created revision with
  a lower sequence never wins;
- resolution ignores revision-ID lexical order — a test uses IDs whose lexical
  order is the reverse of their sequence order;
- an accepted revision 2 followed by a draft revision 3 resolves to 2;
- withdrawing the accepted revision 2 resolves to accepted revision 1;
- duplicate sequence produces `ErrAmbiguousRevisionSequence`;
- no accepted revision produces a non-error "none" result with rationale;
- concurrent creation of two revisions yields sequences *n* and *n+1*, never two
  of *n*;
- resolution output is byte-identical across repeated calls on unchanged data.

## 3. Current-state queries

All queries are **computed**, not materialized. Rationale: the canonical scenario
has single-digit record counts, computed queries cannot go stale, and a
materialized projection that disagrees with the records is the classic bug this
project exists to avoid. Materialization is introduced only if M.4 produces
measured evidence that a query is too slow, and that would be a recorded
decision. Recorded as **AD-006**.

Every query obeys three universal rules:

- **Deterministic output.** Same stored records, same output, byte for byte.
  Every ordering is total: where a natural key could tie, a stable secondary key
  breaks it.
- **Rationale is mandatory.** Every result carries a machine-readable rationale:
  the rule applied, the records considered, and the records rejected with a
  reason. This is displayed in the UI, not just logged.
- **Ambiguity fails explicitly.** No query ever picks arbitrarily among
  candidates it cannot rank.

**Derived state is never written back into a PEOS value.** Not as a field, not
as an Extension payload, not as a new revision.

### 3.1 Current capability revision

| | |
|---|---|
| Source | FeatureForge Revision Sequence records; `core.ArtifactRevision` values; Specification Content |
| Policy | The accepted revision with the greatest sequence (§2) |
| Ambiguity | Duplicate sequence → error; no accepted revision → "none" with rationale |
| Output | Revision reference, sequence, acceptance state, specification content, provenance |
| Rationale | "Selected sequence 2 of 3 recorded revisions: sequence 3 is `draft`, sequence 1 is superseded by a higher accepted sequence." |

### 3.2 Effective requirements

| | |
|---|---|
| Source | `requirement.Requirement` and `requirement.Revision` values linked to the capability |
| Policy | For each Requirement Artifact, its current revision by the same ordering contract; a Requirement whose current revision is withdrawn is excluded |
| Ambiguity | Any requirement failing resolution fails the whole query, naming it — a partial requirement set would silently understate what must be satisfied |
| Output | Ordered list of requirement revision references and statements, ordered by requirement artifact ID |
| Rationale | Per requirement: which revision was chosen and why; plus the list excluded as withdrawn |

### 3.3 Applicable decision

| | |
|---|---|
| Source | `decision.Decision` values whose subjects include the capability Artifact or one of its revisions |
| Policy | All decisions naming the capability as a subject, ordered by provenance `RecordedAt`, then by Decision ID |
| Ambiguity | A decision with no provenance timestamp is not silently ordered last — it is returned in a separate "unordered" group and flagged |
| Output | Ordered list: decision ID, question, outcome statement, rationale, basis evidence references |
| Rationale | Which subject reference matched, and how ordering was applied |

The POC does not compute "the one applicable decision". Decision supersession is
a PEOS-004 concept the scenario does not exercise, and inventing a precedence
rule would be an abstraction ahead of need.

### 3.4 Latest non-corrected claim

This is the query the correction flow exists to prove.

| | |
|---|---|
| Source | `validation.Claim` values whose subject is the capability or one of its revisions |
| Policy | Follow correction chains (below) |
| Ambiguity | Two uncorrected claims for the same subject, scope, and criteria set → error `ErrAmbiguousCurrentClaim`, naming both |
| Output | The claim, its outcome, its evidence, its execution records, and the full chain that led to it |
| Rationale | The chain, stated as "claim C-1 was corrected by C-2; C-2 is not corrected by anything; C-2 stands" |

**Chain rules:**

1. Build the set of claims for the subject and scope.
2. A claim is **superseded** if a later claim carries a
   `core.RecordCorrectionRef` naming it, with kind `correct` or `replace`.
3. A claim is **invalidated** if a later claim names it with kind `invalidate`.
   An invalidated claim is not current, and the invalidating claim is evaluated
   on its own merits like any other.
4. The current claim is the unique claim that is neither superseded nor
   invalidated.
5. If none remains, the result is "no current claim", with the rationale naming
   what invalidated the last one. This is a legitimate state, not an error.
6. If a correction reference names a claim that does not exist, that is an error,
   not a skipped link.
7. Correction chains are followed to termination; a cycle is an error
   (`ErrCorrectionCycle`), and detection is bounded by the number of claims.

An outcome is never inferred. If the current claim's outcome is `inconclusive`,
the answer is `inconclusive` — not "not yet satisfied".

### 3.5 Current lifecycle state

| | |
|---|---|
| Source | `lifecycle.StateAssignment` values whose subject is the capability Artifact |
| Policy | The assignment with the greatest `EffectiveAt`; ties broken by State Assignment ID, which is total |
| Ambiguity | Two assignments with equal `EffectiveAt` and different states → error `ErrAmbiguousLifecycleState`, naming both. The ID tie-break applies only where the state is identical, which is a harmless duplicate. |
| Output | State ID, effective-at, definition version reference, establishing transition record revision |
| Rationale | "3 assignments; selected the one effective 2026-03-04, established by transition record revision TRR-2." |

Assignments from a different Definition Version than the configured one are an
error, not silently accepted — the POC has exactly one Definition Version.

### 3.6 Release readiness

A **computed** result, never a stored Claim ([FF-003 §6](003-peos-integration.md#6-featureforge-vocabulary)).

Inputs: the current capability revision (3.1), the effective requirements (3.2),
and for each requirement the current claim citing it as criteria (3.4).

Outcome, from a closed set:

| Outcome | Condition |
|---|---|
| `ready` | Every effective requirement has a current claim whose outcome is `satisfied`, whose subject is the current capability revision, and whose criteria include that requirement's current revision |
| `not-ready` | At least one effective requirement has a current claim whose outcome is `not-satisfied` |
| `indeterminate` | At least one current claim's outcome is `inconclusive`, or its supporting execution outcome is `interrupted` or `indeterminate` |
| `incomplete` | At least one effective requirement has no current claim, or there are no effective requirements at all |

Precedence when several apply: `not-ready` > `indeterminate` > `incomplete` >
`ready`. A negative signal is never masked by a weaker one.

> **Corrected in M.2 by [AD-016](../decisions/README.md#ad-016--release-readiness-has-four-statuses-precedence-is-not-ready-first).**
> This document originally listed five statuses, including both `inconclusive`
> and `undetermined`, which were never distinguishable in practice. They merge
> into `indeterminate`. "No effective requirements" becomes `incomplete`, and
> "the current revision cannot be resolved" becomes an **error**
> (`ErrEngineeringStateIndeterminate`), not a status — a structural failure means
> the computation is impossible, not that its answer is uncertain.

Two obligations are discharged here explicitly:

- A claim supported only by an execution record whose outcome is `interrupted` or
  `indeterminate` does **not** contribute to `ready`. It is reported as
  `indeterminate`, with the execution record named. This discharges PEOS-006's
  requirement that an indeterminate or interrupted outcome never be silently
  treated as completed.
- A claim whose subject is an *earlier* capability revision does **not** satisfy
  the *current* one. It is listed in the rationale as "stale, evaluated against
  sequence 1". This is what makes the two-revision scenario meaningful.

The rationale is a per-requirement table — requirement, current revision, current
claim, outcome, supporting execution records, and the reason for the verdict.
That table is the Validation screen
([FF-001 §3.6](001-poc-acceptance-contract.md#36-validation)).

### 3.7 Complete timeline

Defined in [FF-006](006-timeline-read-model.md).

## 4. Query summary

| Query | Source | Materialized? | Fails explicitly on |
|---|---|---|---|
| Current capability revision | Sequence + revisions | No | Duplicate sequence, missing content, digest mismatch |
| Effective requirements | Requirement revisions | No | Any unresolvable requirement |
| Applicable decision | Decisions | No | — (unordered decisions are flagged, not fatal) |
| Latest non-corrected claim | Claims + corrections | No | Two uncorrected claims, dangling correction, cycle |
| Current lifecycle state | State assignments | No | Equal-timestamp conflict, unknown definition version |
| Release readiness | Composition of the above | No | Inherits every failure above |
| Timeline | All records | No | Unresolvable reference |
