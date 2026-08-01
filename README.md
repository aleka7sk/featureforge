# FeatureForge

FeatureForge is a bounded reference consumer of
[PEOS](https://github.com/aleka7sk/PEOS) v1.0.0.

It exists to prove, end to end, that PEOS v1.0.0 can be used by a real product
before Belcanto Product begins. It is intentionally temporary: it is frozen as
soon as the acceptance contract is met, and it is not a product with a future.

FeatureForge tracks the **engineering state of one product capability** — its
specification, revisions, requirements, decisions, validation, claims,
corrections, and history. It does not implement the capability itself.

## Status

Phase **M.5 — HTTP API and Minimal UI** and its command-replay correctness
closure are implemented. [AD-030](docs/decisions/ad-030-command-idempotency-and-replay-conformance.md)
records the command-replay and aggregate-integrity correction;
[FF-022](docs/spec/022-command-replay-and-aggregate-integrity.md) records its
advancing-clock, zero-write, memory/PostgreSQL and independent-audit evidence.
The final pre-M.6 domain/lifecycle conformance scope is accepted in
[AD-031](docs/decisions/ad-031-bounded-operational-establishment.md),
[AD-032](docs/decisions/ad-032-persisted-lifecycle-policy-and-linear-history.md),
[AD-033](docs/decisions/ad-033-requirement-criterion-trace-is-structured-state.md),
and [FF-023](docs/spec/023-domain-and-lifecycle-conformance.md). Its
implementation is in progress; M.6 does not begin until FF-023 records its
completion evidence.

The M.4 canonical scenario continues to run end to end against a real PEOS
v1.0.0 SDK on both an in-memory store and PostgreSQL. Historical milestone
reports remain available under `docs/reports/` and are not rewritten by the
forward correction.

## Running the tests

```sh
make test            # everything that needs no database (PostgreSQL tests skip)
make postgres-test   # starts PostgreSQL, migrates, runs the DB-backed tests, tears down
make verify          # fmt, vet, build, test, -race, and postgres-test
```

`make postgres-test` needs Docker. Migrations are applied by the test harness
itself, so no schema or table is ever created by hand.

## Documentation

Start with the [product overview](docs/spec/000-product-overview.md).

| Document | Answers |
|---|---|
| [FF-000 Product Overview](docs/spec/000-product-overview.md) | What is FeatureForge, and what is it not? |
| [FF-001 POC Acceptance Contract](docs/spec/001-poc-acceptance-contract.md) | When is this project done? |
| [FF-002 Domain Boundaries](docs/spec/002-domain-boundaries.md) | What does FeatureForge own, and what may import what? |
| [FF-003 PEOS Integration](docs/spec/003-peos-integration.md) | How does FeatureForge use PEOS, and what does it store? |
| [FF-004 Current-State Resolution](docs/spec/004-current-state-resolution.md) | How is "current" resolved, deterministically? |
| [FF-005 Validation Scenario](docs/spec/005-validation-scenario.md) | What gets validated, and how is it corrected? |
| [FF-006 Timeline Read Model](docs/spec/006-timeline-read-model.md) | How is history presented to a human? |
| [FF-007 Delivery Roadmap](docs/spec/007-delivery-roadmap.md) | In what order is this built? |
| [FF-008 Package Architecture](docs/spec/008-package-architecture.md) | How is the implementation packaged, and what may import what? |
| [FF-009 In-Memory Persistence](docs/spec/009-in-memory-persistence.md) | How is engineering state persisted in memory? |
| [FF-010 Application Contracts](docs/spec/010-application-contracts.md) | What are the exact application commands, queries, and algorithms? |
| [FF-011 Canonical Scenario](docs/spec/011-canonical-scenario.md) | What are the exact fixture identities and PEOS value inventory? |
| [FF-012 Test Specification](docs/spec/012-test-specification.md) | What must the test suite prove? |
| [FF-013 M.3 Implementation Packet](docs/spec/013-m3-implementation-packet.md) | What must M.3 build, and in what commit order? |
| [FF-014 PostgreSQL Persistence](docs/spec/014-postgresql-persistence.md) | How is engineering state persisted in PostgreSQL, and why is that an adapter? |
| [FF-015 HTTP API and UI](docs/spec/015-http-api-and-ui.md) | What public transport and UI constraints govern M.5? |
| [FF-016 Revision Subject Discovery](docs/spec/016-revision-subject-discovery.md) | How are subject-bearing revisions discovered without decoding in adapters? |
| [FF-018 Phase A HTTP Implementation](docs/spec/018-http-phase-a-implementation.md) | How were the twelve commands and seven queries exposed over HTTP? |
| [FF-020 Read-Surface Extension](docs/spec/020-read-surface-extension.md) | How is authoritative engineering content projected for readers? |
| [FF-021 Phase B Minimal UI](docs/spec/021-ui-phase-b-implementation.md) | How does the no-JavaScript UI reuse the API handler? |
| [FF-022 Command Replay and Aggregate Integrity](docs/spec/022-command-replay-and-aggregate-integrity.md) | How will all C1–C12 retries and C7/C9 aggregate integrity be proven before domain analysis? |
| [FF-023 Domain and Lifecycle Conformance](docs/spec/023-domain-and-lifecycle-conformance.md) | How are bounded operational establishment, persisted lifecycle policy, and legal linear history closed before M.6? |
| [Decision log](docs/decisions/README.md) | What was decided, and why? |
| [Glossary](docs/glossary.md) | What does this word mean here? |

## Governing rules

- FeatureForge specifications govern FeatureForge.
- PEOS-000 through PEOS-009 govern PEOS concepts.
- FeatureForge documents do not extend the PEOS ontology.
- The PEOS repository and the PEOS Go SDK are never modified from here.
- The FeatureForge product domain never imports PEOS. All PEOS use is confined
  to one integration package; see [FF-002](docs/spec/002-domain-boundaries.md).
