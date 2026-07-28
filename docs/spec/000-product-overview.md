# FF-000 — FeatureForge Product Overview

Status: Accepted (Phase M.1)
Governs: FeatureForge product purpose, scope, non-goals, and canonical scenario.

## Governance statement

FeatureForge specifications (FF-000 through FF-007) govern FeatureForge.

PEOS-000 through PEOS-009 govern PEOS concepts. Those specifications ship inside
the `github.com/aleka7sk/PEOS` module under `spec/`. Where a FeatureForge
document and a PEOS specification appear to disagree about a PEOS concept, the
PEOS specification governs, and the FeatureForge document is wrong and must be
corrected.

FeatureForge documents do not extend the PEOS ontology. FeatureForge never
defines a new PEOS construct, never renames one, and never redefines the meaning
of one. Where FeatureForge needs a term PEOS does not define, that term is a
FeatureForge product term and lives in the `featureforge` vocabulary namespace,
described in [FF-003](003-peos-integration.md).

The PEOS repository and the PEOS Go SDK are read-only from FeatureForge. No
phase of this project modifies them.

## What FeatureForge is

FeatureForge is a bounded product-engineering tracker. It exists for exactly one
reason: to prove that PEOS v1.0.0 is usable end to end by a real consumer
product before Belcanto Product begins.

It lets one product developer take a single product feature through this
lifecycle:

1. create a feature card;
2. create an engineering specification for it;
3. create immutable specification revisions;
4. add engineering requirements;
5. record a design or architecture decision with its basis;
6. create a validation plan;
7. record validation execution and the evidence it produced;
8. produce a result and a claim;
9. correct an erroneous validation record without rewriting history;
10. resolve and display the current engineering state;
11. display a complete audit timeline.

FeatureForge is deliberately temporary. It is a reference consumer, not a
product with a future. It is frozen the moment the criteria in
[FF-001](001-poc-acceptance-contract.md) are met, and the next project is
Belcanto Product.

## What FeatureForge is not

FeatureForge will not become, and must not grow toward:

- Jira or any issue tracker;
- a project-management platform;
- a workflow engine;
- a generic ALM system;
- a SaaS product;
- a CRM;
- a replacement for Git;
- a replacement for PEOS;
- a production system serving multiple organizations.

Concretely, the following are out of scope for every phase: multi-tenancy,
production authentication, collaboration, notifications, generalized project
management, and commercial deployment. A single local user is sufficient
throughout.

## The distinction that makes this project work

FeatureForge tracks **engineering state about a capability**. It does not
implement the capability.

The canonical feature is *"Homework after a lesson"*: a teacher publishes
homework after a lesson, and the student must be able to see it, including an
optional audio attachment.

FeatureForge models the engineering history of that capability — the
specification, its revisions, the requirements, the decision, the validation and
its claim. FeatureForge does **not** implement teachers, students, lessons,
homework, attachments, or notifications. Those are *scenario content*: they
appear as text inside specification revisions and requirement statements, and
nowhere else.

If a `Teacher` type, a `Homework` table, or a `PublishHomework` operation ever
appears in FeatureForge, the boundary has been violated. See
[FF-002](002-domain-boundaries.md) for the enforced rule.

## The canonical scenario

One scenario is defined, and the whole project is judged against it. Its full
narrative is in [FF-005](005-validation-scenario.md); the shape is:

| # | Engineering act | Represented as |
|---|---|---|
| 1 | Project created | FeatureForge entity |
| 2 | Feature card created | FeatureForge entity |
| 3 | Capability specification created | PEOS Artifact |
| 4 | Capability Revision 1 recorded | PEOS Artifact Revision + FeatureForge content |
| 5 | Requirements added | PEOS Requirement + Requirement Revision |
| 6 | Decision recorded with basis | PEOS Decision + Decision Basis |
| 7 | Capability Revision 2 recorded | PEOS Artifact Revision + FeatureForge content |
| 8 | Validation plan created | PEOS Validation Plan + Plan Revision |
| 9 | Validation executed | PEOS Validation Execution Record |
| 10 | Evidence recorded | PEOS Artifact Revision in the Evidence role |
| 11 | Result and claim recorded | Execution Outcome + PEOS Validation Claim |
| 12 | Erroneous claim corrected | New PEOS Validation Claim carrying a correction reference |
| 13 | Lifecycle transition recorded | PEOS Transition Record + State Assignment |
| 14 | Current state resolved | FeatureForge computed query |
| 15 | Timeline displayed | FeatureForge computed read model |

Revision 1 remains fully inspectable after Revision 2 exists. The erroneous
claim remains fully inspectable after its correction exists. Nothing in the
history is ever updated or deleted.

## Reading order

| Document | Answers |
|---|---|
| FF-000 (this) | What is FeatureForge, and what is it not? |
| [FF-001](001-poc-acceptance-contract.md) | When is this project done, and how do we know? |
| [FF-002](002-domain-boundaries.md) | What does FeatureForge own, and what may import what? |
| [FF-003](003-peos-integration.md) | How does FeatureForge use PEOS, and what does it store? |
| [FF-004](004-current-state-resolution.md) | How is "current" resolved, deterministically? |
| [FF-005](005-validation-scenario.md) | What exactly gets validated, and how is it corrected? |
| [FF-006](006-timeline-read-model.md) | How is history presented to a human? |
| [FF-007](007-delivery-roadmap.md) | In what order is this built? |
| [FF-008](008-package-architecture.md) | How is the implementation packaged, and what may import what? |
| [FF-009](009-in-memory-persistence.md) | How is engineering state persisted in memory? |
| [FF-010](010-application-contracts.md) | What are the exact application commands, queries, and algorithms? |
| [FF-011](011-canonical-scenario.md) | What are the exact fixture identities and PEOS value inventory? |
| [FF-012](012-test-specification.md) | What must the test suite prove? |
| [FF-013](013-m3-implementation-packet.md) | What must M.3 build, and in what commit order? |
| [FF-014](014-postgresql-persistence.md) | How is engineering state persisted in PostgreSQL, and why is that an adapter? |
| [Decision log](../decisions/README.md) | What was decided, and why? |
| [Glossary](../glossary.md) | What does this word mean here? |

## Source-of-truth priority

1. `docs/spec/` (this set)
2. accepted architecture decisions in [`docs/decisions/`](../decisions/README.md)
3. tests
4. implementation

Where an implementation disagrees with a specification, the implementation is
wrong. Where a specification is found to be wrong, it is corrected in a
documentation change before the implementation is changed.
