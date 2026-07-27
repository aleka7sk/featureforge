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

Do not make architecture decisions silently. Record material decisions in the
product specifications or in docs/decisions/README.md.

Do not add abstractions before they are required by the approved vertical slice.