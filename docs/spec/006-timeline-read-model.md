# FF-006 — Timeline Read Model

Status: Accepted (Phase M.1)
Governs: the engineering timeline — its ownership, event model, ordering, and
ambiguity behaviour.

## 1. Ownership

The timeline is a **FeatureForge product read model**. It is not a PEOS type, it
is not stored as engineering state, and it is never written back into a PEOS
value.

It is **computed** from immutable records plus the FeatureForge-owned
engineering metadata enumerated in
[FF-002 §3](002-domain-boundaries.md#3-product-owned-records-that-are-not-operational-entities),
on every request. It can be deleted and recomputed with identical output. Recorded
as **AD-007**.

Its purpose is a person: someone who has never seen this feature should be able
to read the timeline top to bottom and understand what was decided, what was
validated, what went wrong, and what was done about it.

## 2. Event identity

A timeline event's identity is derived, not stored:

```
event_id = <source_kind> ":" <source_identity>
```

`source_identity` is the identity of the underlying record — an Artifact Revision
reference, a Decision ID, an Execution Record ID, a Claim ID, a State Assignment
ID, or a FeatureForge record key.

This makes event identity **stable** (the same included source record always
produces the same event ID), **derivable** (no event table, nothing to keep in
sync), and **unique** (each included source record produces exactly one event).

The timeline does not project every persisted value as a separate event. In
particular, the semantic acceptance members created atomically by C7 and C9 are
validated as part of the Requirement or Plan aggregate but are not emitted as
additional acceptance events: the same engineering act is represented by its
`requirement.revised` or `plan.revised` event. Capability acceptance is a
separate C5 act and therefore remains a distinct journal-derived event.

An event is never invented for something with no record behind it. If it is not
recorded, it is not on the timeline.

## 3. Event kinds

| Kind | Source | Display label |
|---|---|---|
| `project.created` | FeatureForge Project | Project created |
| `feature.created` | FeatureForge FeatureCard | Feature card created |
| `capability.created` | `core.Artifact` (`featureforge:product-capability`) | Capability specification created |
| `capability.revised` | `core.ArtifactRevision` + sequence | Capability revision *n* recorded |
| `capability.accepted` | Acceptance journal entry | Capability revision *n* accepted |
| `capability.withdrawn` | Acceptance journal entry | Capability revision *n* withdrawn |
| `requirement.revised` | `requirement.Revision` | Requirement *R-x* recorded / revised |
| `decision.recorded` | `decision.Decision` | Decision recorded |
| `plan.revised` | `validation.PlanRevision` | Validation plan revision recorded |
| `execution.recorded` | `validation.ExecutionRecord` | Validation activity *A-x* executed — *outcome* |
| `evidence.recorded` | Evidence `core.ArtifactRevision` | Evidence recorded |
| `claim.recorded` | `validation.Claim` | Claim recorded — *outcome* |
| `claim.corrected` | `validation.Claim` carrying a correction reference | Claim recorded — *outcome* — correcting *C-x* |
| `lifecycle.transitioned` | validated `lifecycle.StateAssignment` + its transition record revision + persisted Definition Version | Lifecycle state → *state* |

`claim.corrected` is a display specialization of `claim.recorded`, not a second
event: one claim, one event. The correcting claim's event carries the link; the
corrected claim's event is unchanged and stays exactly where it was.

**Forward correction (AD-032/FF-023).** Before emitting any lifecycle event,
Q5 validates the complete persisted policy and whole predecessor graph. Each
event references its exact establishing Transition Record Revision and
`LCD-1/LCDV-1`; a partial, branched, cyclic, wrong-version or unreadable history
fails the entire timeline with opaque stored-state integrity rather than
returning a partial list. Its detail includes the decoded Definition/Version,
entry transition, initial states and complete transition-edge summary, so the
persisted configuration reference has an authoritative read representation
without inventing a separate user act or timeline event for startup
configuration.

**C8 forward-citation correction (AD-030/FF-022).** A structurally valid
Decision may be recorded while its exact basis Evidence Artifact/Revision pair
is wholly absent. Q5 still emits the inspected Decision and its exact
`evidence:<artifact>/<revision>` reference, but emits no Evidence event. The UI
opens that identity as a pending reference backed by the Decision event. Once
C10 atomically materialises the exact pair, recomputation links the same
identity to the Evidence source event. One-sided occupancy, unreadable content,
or a contradictory family is corruption and still fails the complete timeline.

## 4. Event fields

| Field | Source | Absent when |
|---|---|---|
| `event_id` | Derived (§2) | Never |
| `kind` | §3 | Never |
| `label` | §3, with the record's own values interpolated | Never |
| `occurred_at` | `Provenance.RecordedAt`, `ExecutionRecord.CompletedAt`, `StateAssignment.EffectiveAt`, or the FeatureForge record's timestamp | Provenance timestamp absent — see §6 |
| `actor` | `Provenance.Actor`, or `ExecutionRecord.Actor` | Provenance actor absent — see §6 |
| `references` | Typed links to the records this event names | Never — always at least the source record |
| `rationale` | Why this event is placed where it is, and how ties were broken | Never |
| `detail` | Kind-specific summary: a Decision outcome; plan activity keys, methods, interpretations and expected evidence; an Execution activity key and outcome; a sequence number; a correction target; or the validated lifecycle policy summary | Kind-dependent |

`rationale` on the timeline is not decoration. When two events share a timestamp,
the rationale states which tie-break applied. When an event has no timestamp, the
rationale states that and where it was placed.

## 5. Ordering

The ordering key is total, so the timeline is deterministic:

```
(occurred_at ASC, kind_rank ASC, source_identity ASC)
```

**`kind_rank`** is a fixed table that orders events which genuinely happened
within the same recorded instant, in causal order: creation before revision,
revision before acceptance, plan before execution, execution before evidence,
evidence before claim, claim before lifecycle transition. It never reorders
events with distinct timestamps.

**`source_identity`** is the final tie-break. It is lexicographic over the record
identity string, which is total and stable — so it always terminates, and it
always terminates the same way.

Timestamps are compared as instants, normalized to UTC. Two timestamps expressing
the same instant in different offsets are equal, and the tie-break applies.

## 6. Ambiguity behaviour

The timeline never silently invents a position.

| Condition | Behaviour |
|---|---|
| Two events share `occurred_at` | Ordered by `kind_rank`, then `source_identity`. Both events' rationale states the tie-break applied. |
| An event's source has no timestamp | It is **not** placed at epoch and **not** placed last silently. It goes into a separate `undated` group rendered above the dated timeline, each entry flagged "no recorded timestamp". |
| A C8 Decision cites an Evidence pair whose Artifact and Revision are both absent | Emit the Decision with an honest pending exact-reference target and no Evidence event. This is the sole unresolved-reference exception. |
| Any other reference cannot be resolved, or C8 Evidence occupancy is partial/contradictory | Error `ErrUnresolvableTimelineReference` / stored-state integrity as governed by the source family. The timeline is not rendered partially. |
| A claim's correction target is missing | Same as above — this is the dangling-reference case, and it fails. |
| Two events resolve to the same `event_id` | Error `ErrDuplicateTimelineEvent`. Under §2 this is unreachable; if it occurs, two records share an identity, which is a persistence bug. |
| An execution outcome is `interrupted` or `indeterminate` | Rendered with that outcome verbatim, never normalized toward "completed" or "failed". |

## 7. Worked timeline

The canonical public scenario ([FF-011](011-canonical-scenario.md)) produces
exactly these 28 dated events:

| # | Label | Actor | Detail |
|---|---|---|---|
| 1 | Project created | `featureforge:local-user` | `PRJ-1` — Belcanto Pilot |
| 2 | Feature card created | `featureforge:local-user` | `FC-1` — Homework after a lesson |
| 3 | Capability specification created | `featureforge:local-user` | `CAP-1`, `featureforge:product-capability` |
| 4 | Capability revision recorded | `featureforge:local-user` | `CAP-1/CAP-1-REV-1`, sequence 1 |
| 5 | Capability revision accepted | `featureforge:local-user` | `ACC-1`, `CAP-1/CAP-1-REV-1`, `accepted` |
| 6 | Lifecycle state → `featureforge:drafting` | `featureforge:local-user` | `SA-1`, entry `TR-1/TR-1-REV-0` |
| 7 | Decision recorded | `featureforge:local-user` | `DEC-1`; exact forward citation `evidence:EV-1/EV-1-REV-1`, materialised at row 17 |
| 8 | Capability revision recorded | `featureforge:local-user` | `CAP-1/CAP-1-REV-2`, sequence 2 |
| 9 | Capability revision accepted | `featureforge:local-user` | `ACC-2`, `CAP-1/CAP-1-REV-2`, `accepted` |
| 10 | Requirement recorded | `featureforge:local-user` | `REQ-1/REQ-1-REV-1`; exact trace `CAP-1/CAP-1-REV-2#AC-1` |
| 11 | Requirement recorded | `featureforge:local-user` | `REQ-2/REQ-2-REV-1`; exact trace `CAP-1/CAP-1-REV-2#AC-2` |
| 12 | Requirement recorded | `featureforge:local-user` | `REQ-3/REQ-3-REV-1`; exact trace `CAP-1/CAP-1-REV-2#AC-3` |
| 13 | Requirement recorded | `featureforge:local-user` | `REQ-4/REQ-4-REV-1`; exact trace `CAP-1/CAP-1-REV-2#AC-4`; no claim follows |
| 14 | Lifecycle state → `featureforge:specified` | `featureforge:local-user` | `SA-2`, `TR-1/TR-1-REV-1` (`specify` from `SA-1`) |
| 15 | Validation plan revision recorded | `featureforge:local-user` | `VP-1/VP-1-REV-1`; activities `A-1`, `A-2`, `A-3` |
| 16 | Validation activity executed | `featureforge:local-user` | `ER-1`, `A-1`, `completed` |
| 17 | Evidence recorded | `featureforge:local-user` | `EV-1/EV-1-REV-1`, materialised by the same C10 act as `ER-1`; resolves `DEC-1`'s citation |
| 18 | Lifecycle state → `featureforge:under-validation` | `featureforge:local-user` | `SA-3`, `TR-1/TR-1-REV-2` (`begin-validation` from `SA-2`) |
| 19 | Claim recorded | `featureforge:local-user` | `CLM-1`, `peos:satisfied`, criterion `requirement-revision:REQ-1/REQ-1-REV-1` |
| 20 | Validation activity executed | `featureforge:local-user` | `ER-2`, `A-2`, `completed` |
| 21 | Evidence recorded | `featureforge:local-user` | `EV-2/EV-2-REV-1` |
| 22 | Claim recorded | `featureforge:local-user` | `CLM-2`, `peos:satisfied`, criterion `requirement-revision:REQ-2/REQ-2-REV-1` |
| 23 | Validation activity executed | `featureforge:local-user` | `ER-3`, `A-3`, `completed` |
| 24 | Evidence recorded | `featureforge:local-user` | `EV-3/EV-3-REV-1` |
| 25 | Claim recorded | `featureforge:local-user` | `CLM-3`, `peos:satisfied`, criterion `requirement-revision:REQ-3/REQ-3-REV-1` |
| 26 | Validation activity executed | `featureforge:local-user` | `ER-4`, re-run of `A-2`, `completed` |
| 27 | Evidence recorded | `featureforge:local-user` | `EV-4/EV-4-REV-1` |
| 28 | **Claim recorded, correcting an earlier claim** | `featureforge:local-user` | `CLM-4`, `peos:not-satisfied`, criterion `requirement-revision:REQ-2/REQ-2-REV-1`, corrects `CLM-2` |

Row 28 is the row that matters. Row 22 is still present, still says
`peos:satisfied`, and is still readable. That is the whole point of the
exercise.

## 8. Scope of the timeline

| Question | Answer |
|---|---|
| Does it show operational scenario activity? | It shows stable Project and FeatureCard establishment as human context. This POC has no operational edit activity. |
| Does it show mutable field edits on a FeatureCard? | No. AD-031 exposes no rename/edit command in this POC. A future operational edit in another product would need its own audit and replay policy and would not automatically become a PEOS engineering event. |
| Does it show acceptance transitions? | It shows capability C5 acceptance/withdrawal transitions because those are separate acts that change the authoritative capability revision. The atomically created C7/C9 semantic acceptance members are validated with their aggregates but are not projected as duplicate timeline events. |
| Does it merge events across feature cards? | No. The timeline is scoped to one FeatureCard's capability. |
| Is it paginated? | Not in the POC. The scenario produces under 30 events. |
| Can it be filtered by kind? | Yes, client-side, in M.5. Filtering never changes ordering. |

## 9. Required tests

- the scenario produces exactly the events in §7, in that order;
- inserting the records in reverse order produces the identical timeline;
- two events with equal timestamps order by `kind_rank`, then `source_identity`,
  identically across repeated runs;
- an event whose source has no timestamp appears in the `undated` group and is
  flagged, never at position 1 of the dated list;
- a claim carrying a correction reference renders the link, and the corrected
  claim's own event is unchanged;
- a dangling correction reference produces `ErrUnresolvableTimelineReference`;
- a wholly absent C8 Evidence pair leaves the Decision and its pending link
  readable; materialising that pair resolves the same link, while one-sided
  occupancy fails closed;
- every materialised canonical reference opens a dedicated, source-event, or
  validated embedded detail representation; criterion identities remain
  sub-record context and are never mislabelled as standalone records;
- an `indeterminate` execution outcome renders as `indeterminate`;
- deleting every derived model and recomputing produces byte-identical output.
