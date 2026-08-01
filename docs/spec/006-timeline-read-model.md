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
returning a partial list.

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
| `detail` | Kind-specific summary: an outcome, a sequence number, a decision statement, a correction target | Kind-dependent |

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
| An event references a record that cannot be resolved | Error `ErrUnresolvableTimelineReference`, naming both records. The timeline is not rendered with a broken link. |
| A claim's correction target is missing | Same as above — this is the dangling-reference case, and it fails. |
| Two events resolve to the same `event_id` | Error `ErrDuplicateTimelineEvent`. Under §2 this is unreachable; if it occurs, two records share an identity, which is a persistence bug. |
| An execution outcome is `interrupted` or `indeterminate` | Rendered with that outcome verbatim, never normalized toward "completed" or "failed". |

## 7. Worked timeline

The canonical scenario ([FF-005](005-validation-scenario.md)) produces:

| # | Label | Actor | Detail |
|---|---|---|---|
| 1 | Project created | local-user | Belcanto Pilot |
| 2 | Feature card created | local-user | Homework after a lesson |
| 3 | Capability specification created | local-user | `featureforge:product-capability` |
| 4 | Capability revision 1 recorded | local-user | sequence 1 |
| 5 | Capability revision 1 accepted | local-user | draft → accepted |
| 6 | Lifecycle state → drafting | local-user | SA-1, entry TR-1/TR-1-REV-0 |
| 7 | Evidence recorded | local-user | pilot-teacher interview notes |
| 8 | Decision recorded | local-user | audio attachment by URL; 5-second latency |
| 9 | Capability revision 2 recorded | local-user | sequence 2 |
| 10 | Capability revision 2 accepted | local-user | draft → accepted |
| 11 | Requirement R-1 recorded | local-user | exact trace CAP-1-REV-2#AC-1 |
| 12 | Requirement R-2 recorded | local-user | exact trace CAP-1-REV-2#AC-2 |
| 13 | Requirement R-3 recorded | local-user | exact trace CAP-1-REV-2#AC-3 |
| 14 | Requirement R-4 recorded | local-user | exact trace CAP-1-REV-2#AC-4; no claim follows |
| 15 | Lifecycle state → specified | local-user | SA-2, `specify` from SA-1 |
| 16 | Validation plan revision recorded | local-user | activities A-1, A-2, A-3 |
| 17 | Validation activity A-1 executed | local-user | completed |
| 18 | Evidence recorded | local-user | reviewer note V-1 |
| 19 | Lifecycle state → under-validation | local-user | SA-3, `begin-validation` from SA-2 after a plan activity completed |
| 20 | Claim recorded — satisfied | local-user | C-1, criteria R-1 |
| 21 | Validation activity A-2 executed | local-user | completed |
| 22 | Evidence recorded | local-user | reviewer note V-2 |
| 23 | Claim recorded — satisfied | local-user | C-2, criteria R-2 |
| 24 | Validation activity A-3 executed | local-user | completed |
| 25 | Evidence recorded | local-user | inspection note V-3 |
| 26 | Claim recorded — satisfied | local-user | C-3, criteria R-3 |
| 27 | Validation activity A-2 re-executed | local-user | completed |
| 28 | Evidence recorded | local-user | reviewer note V-4 |
| 29 | **Claim recorded — not satisfied — correcting C-2** | local-user | C-4, criteria R-2 |

Row 29 is the row that matters. Row 23 is still there, still says `satisfied`,
and is still readable. That is the whole point of the exercise.

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
- an `indeterminate` execution outcome renders as `indeterminate`;
- deleting every derived model and recomputing produces byte-identical output.
