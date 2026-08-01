# AD-031 — Operational establishment is stable only within the bounded POC

Status: Accepted
Date: 2026-08-01
Phase: post-M.5 domain closure before M.6

## Context

FeatureForge has always separated operational state (`Project` and
`FeatureCard`) from immutable, provenance-bearing engineering state. Early M.1
prose then described the operational values as mutable in place and retained an
`UpdateFeatureCard` candidate. M.2 and the implementation instead exposed only
intent-oriented create commands plus the one-time FeatureCard-to-capability
link, without explicitly superseding the earlier wording.

Adding general edits now would not be a harmless completion of that prose.
AD-030 recognizes C1/C2 replay from the currently stored establishment values;
after a rename those values would no longer prove the original create intent.
The store has no separate create receipt or snapshot. The computed
`project.created` and `feature.created` timeline summaries also use the current
name or title, so an edit would retroactively rewrite the human rendering of a
creation event.

The two persistence adapters additionally disagree about whether the optional
capability link participates in `FeatureCardRepository.Put` equality after the
link has been established. That is an adapter-contract defect, not a reason to
expand the domain surface.

## Decision

`Project` and `FeatureCard` remain operational values. They do not acquire PEOS
identity, provenance, revision, or engineering-record semantics.

Within this bounded FeatureForge POC, their establishment fields are stable:

- Project: `id`, `name`, `created_at`;
- FeatureCard: `id`, `project_id`, `title`, `description`, `created_at`.

FeatureForge implements no generic `UpdateProject` or `UpdateFeatureCard`
command. The M.1 `UpdateFeatureCard` candidate is superseded for this POC and
deferred to Belcanto's own domain design.

The one supported operational change is the monotonic capability link:

```text
absent -> one capability Artifact ID
```

Repeating the same link is a no-op. A different link returns
`ErrCapabilityAlreadyLinked`. The link is stored separately and materialized by
FeatureCard reads. It is not part of the base FeatureCard establishment value or
of `FeatureCardRepository.Put` equality; both adapters must therefore accept an
identical base `Put` before or after linking and return the same materialized
read.

Caller-owned identifiers remain governed by FF-010 and AD-029. Nothing in this
decision restores server-generated Project or FeatureCard identities.

## Consequences

- No new domain field, update command, HTTP route, DTO, UI form, or migration is
  introduced.
- Copy-return domain values and the absence of pointer-mutating setters remain
  useful encapsulation, but are not evidence that operational entities are
  intrinsically immutable.
- The no-`UPDATE`/no-`DELETE` guard applies to engineering tables. Project and
  FeatureCard tables are excluded from that engineering claim; the bounded POC
  surface is enforced separately by the absence of update ports and routes.
- The shared persistence contract must prove link materialization,
  same-link idempotence, different-link conflict, rollback, and base-`Put`
  parity after linking.
- Historical reports remain historical evidence. Directly governing
  specifications receive forward corrections rather than being rewritten as
  though this decision had existed earlier.

## Belcanto boundary

Belcanto should reuse the separation between operational and engineering state,
intent-oriented commands, copy-return values, append-only engineering history,
and consumer-owned ports. It must not inherit FeatureForge's create-only
Project/FeatureCard repositories, blanket route prohibition, or replay witness
for mutable aggregates.

If Belcanto supports operational edits, it must choose its own concurrency,
audit, and lost-response replay contract, such as an operation receipt or a
persisted snapshot of create intent. That decision is intentionally outside
FeatureForge.
