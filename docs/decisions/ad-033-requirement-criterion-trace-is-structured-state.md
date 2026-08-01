# AD-033 — Requirement-to-criterion trace is structured product-owned state

Status: Accepted
Date: 2026-08-01
Phase: pre-M.6 traceability closure

## Context

FF-001 requires the AI context pack to include uncovered acceptance criteria,
and the Requirements UI promises to show the acceptance criterion from which
each Requirement derives. Capability acceptance-criterion keys are local to one
exact capability Revision. Persisted Requirement state currently names only the
capability Artifact and its own statement; it does not name a capability
Revision or acceptance-criterion key.

The canonical prose is internally inconsistent: FF-005 originally said AC-4
was not promoted to a Requirement, while FF-011 and the executable scenario
create REQ-4 from AC-4 and call it uncovered because it has no claim. Neither
interpretation can be computed from the current store because no authoritative
AC-to-Requirement edge exists.

PEOS's Requirement `Derivation` relates Requirement Revisions to other
Requirement Revisions, not a product-owned acceptance criterion. `core.Origin`
also documents its note as descriptive free text and says structured references
belong to a relation or specialization. Parsing an Origin note as an
authoritative query key would therefore turn prose into hidden schema.

## Decision

FeatureForge owns one immutable structured trace per Requirement Revision:

```text
RequirementCriterionTrace
  RequirementRevision  RevisionKey
  CapabilityRevision   RevisionKey
  AcceptanceCriterionKey string
  RecordedAt            time.Time
```

The Requirement Revision key is the trace identity. The source capability
Revision is exact, never a mutable "current" alias, and the criterion key is
revision-local. The trace is product-owned engineering metadata like revision
order: it is not a PEOS type, does not claim to be a PEOS Derivation, is
insert-only, and is written atomically as a required member of C7.

C7 gains two caller-controlled semantic inputs:

```text
source_capability_revision_id
source_acceptance_criterion_key
```

`subject_artifact_id` supplies the source Artifact ID. For a new C7 act the
application proves that the exact source is the accepted current capability
Revision, loads its authoritative structured content, and requires the named
criterion to exist exactly once. Missing/stale/wrong-family source is a governed
`422 referenced_value_missing`; a syntactically valid criterion key absent from
that authoritative content has the same outcome. Unreadable, mixed-family, or
digest/projection-contradictory stored source is integrity failure (`500`).

For an occupied complete C7 act, a supplied trace differing from the exact
stored trace is `409 immutable_value_conflict`. Corrupt stored trace or source
occupancy is inspected first and remains opaque 500; it is never hidden behind
the ordinary conflict.

The trace's `RecordedAt` equals the Requirement Revision and order metadata
time. Exact replay compares the caller-controlled source capability revision
and criterion key as immutable request semantics. Stored `Trace.RecordedAt` is
integrity-checked against stored R/O and recovered on replay; it is never
compared with a new clock candidate. A later
Requirement Revision may cite a different exact criterion source; no mapping is
silently inherited from the Artifact or inferred by matching text/key.

## C7 aggregate correction

For each Requirement pair, the completed semantic act is now:

```text
Requirement Artifact/root as applicable
+ Requirement Revision
+ RevisionOrderMetadata
+ one semantic accepted member
+ RequirementCriterionTrace
```

Any occupied Requirement Revision without exactly one coherent trace is a
partial aggregate and returns opaque `500 internal_error`. The current POC has
no deployed durable store, so no trace is inferred or backfilled for old rows.
This deliberately narrows AD-030/FF-022's historical replay compatibility:
identity omission may recover an old acceptance member only when the now-current
complete aggregate, including its trace, can be proven.

The repository provides create-only `Put`, exact `Get`, and deterministic
source lookup only if a later query demonstrates it is needed. M.6 already has
the effective Requirement Revision keys, so it reads each trace by its primary
key; no speculative index is added.

## Uncovered criterion semantics

For the exact current capability Revision, an acceptance criterion is
**uncovered** when either:

1. no effective Requirement Revision carries a valid trace to that exact
   Revision/key; or
2. at least one effective Requirement mapped to it has no applicable current
   Claim.

A mapped Requirement with a `not-satisfied` or `inconclusive` current Claim is
covered but remains an open validation finding and is included separately in
the context pack. This prevents a negative result from being mislabeled as
missing traceability.

The canonical scenario maps REQ-1 through REQ-4 to AC-1 through AC-4 on accepted
capability Revision 2. AC-4 is uncovered because mapped REQ-4 has no current
Claim, resolving FF-005 in favor of the executable four-Requirement scenario.

## Consequences

- Migration 0003 gains one trace table with foreign keys to the Requirement and
  source capability Revision identities; memory gains the equivalent map.
- The shared repository suite proves put/get, exact no-op, conflict, rollback,
  reference preservation and adapter parity.
- C7 command/DTO/forms/scenario fixtures gain the two source fields.
- Requirement history integrity validates every trace and exact source
  criterion, including withdrawn revisions; a malformed or dangling trace is
  never omitted from reads.
- Requirements UI and the M.6 context pack can display the exact capability
  Revision and AC key instead of an inferred label.
- PEOS remains unchanged; no Extension or relation is invented.
