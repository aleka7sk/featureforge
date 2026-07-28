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

Phase **M.4 — PostgreSQL Persistence**: complete. The canonical scenario
(FF-011) runs end to end against a real PEOS v1.0.0 SDK on **both** an
in-memory store and a real PostgreSQL database, producing identical
engineering answers, with one shared repository contract suite run against
both adapters. See the
[M.4 implementation report](docs/reports/m4-implementation-report.md), the
[M.3 report](docs/reports/m3-implementation-report.md), and the
[delivery roadmap](docs/spec/007-delivery-roadmap.md).

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
| [Decision log](docs/decisions/README.md) | What was decided, and why? |
| [Glossary](docs/glossary.md) | What does this word mean here? |

## Governing rules

- FeatureForge specifications govern FeatureForge.
- PEOS-000 through PEOS-009 govern PEOS concepts.
- FeatureForge documents do not extend the PEOS ontology.
- The PEOS repository and the PEOS Go SDK are never modified from here.
- The FeatureForge product domain never imports PEOS. All PEOS use is confined
  to one integration package; see [FF-002](docs/spec/002-domain-boundaries.md).
