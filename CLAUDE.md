# FeatureForge Claude Instructions

FeatureForge is a bounded reference consumer of PEOS v1.0.0.

## Source of truth

Priority:

1. docs/spec/
2. accepted architecture decisions
3. tests
4. implementation

PEOS specifications and PEOS Go SDK must not be modified from this repository.

PEOS models engineering state.

Operational FeatureForge entities must remain in the FeatureForge domain and
must not be forced into PEOS values.

The FeatureForge product domain must not import PEOS. Only
internal/engineering/peos may import the PEOS SDK. Application, transport, UI
model, and persistence adapter packages must not. See docs/spec/002-domain-boundaries.md.

## Dependencies

PEOS (github.com/aleka7sk/PEOS v1.0.0) remains the only product-level and
domain-level external dependency: domain, engineering, and application packages
must never import anything but PEOS — confined to internal/engineering/peos —
and the Go standard library.

A milestone may add the minimum infrastructure dependency it explicitly
requires. M.4 adds one maintained PostgreSQL driver, github.com/jackc/pgx/v5,
for internal/infrastructure/postgres. Any such infrastructure dependency must
stay inside its own adapter package and that package's migration or
integration-test support; it must never be imported by domain, engineering, or
application code, and that boundary is architecture-test-enforced.

Do not add an ORM, a query builder, or a general-purpose persistence framework.
Do not add a test framework, an assertion library, a UUID library, or a
general-purpose analysis library. Test-only dependencies are permitted when a
milestone's reproducible integration tests genuinely require them, but must
never appear in a non-test import of a production package. Do not add a replace
directive.

Adding any dependency not already named in an accepted milestone's
docs/spec/007-delivery-roadmap.md entry is an architecture decision: record it
in docs/decisions/README.md before adding it.

Derived state is never stored: no current, ready, satisfied, or status field on a
PEOS value, a FeatureCard, or any record. Current state and the timeline are
computed queries that return a result and a rationale.

Do not make architecture decisions silently. Record material decisions in the
product specifications or in docs/decisions/README.md.

Do not add abstractions before they are required by the approved vertical slice.