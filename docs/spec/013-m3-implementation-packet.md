# FF-013 — M.3 Implementation Packet

Status: Accepted (Phase M.2)
Governs: the exact M.3 file tree, the ordered implementation checklist, and the
execution rules and commit policy for the implementation phase.

## Governance statement

PEOS-000 through PEOS-009 remain authoritative for every PEOS concept.
`docs/spec/` governs FeatureForge. This document does not extend the PEOS
ontology. The implementation must use PEOS v1.0.0 unchanged.

**This packet leaves no material architecture decision to the implementation
agent.** Every choice — package layout, envelope shape, ordering algorithm,
correction algorithm, readiness precedence, lifecycle bootstrap, error taxonomy,
transaction semantics — is settled in FF-008 through FF-012. If M.3 encounters a
decision that is not covered here, it stops and records it before proceeding.

## 1. File tree

```
go.mod                                        + PEOS v1.0.0 requirement
go.sum                                        created by the go tool

internal/
  domain/
    ids.go                 ProjectID, FeatureCardID, validation
    project.go             Project
    featurecard.go         FeatureCard
    errors.go              domain sentinels
    domain_test.go
    architecture_test.go   FF-012 §1 reflection tests

  engineering/
    keys.go                ArtifactKey, RevisionKey, RecordKey, RecordKind, RevisionFamily
    envelope.go            ArtifactEnvelope, RevisionEnvelope, RecordEnvelope
    content.go             CapabilitySpecificationContent + nested values
    canonical.go           canonical JSON encoder, Digest
    order.go               RevisionOrderMetadata
    acceptance.go          RevisionAcceptanceRecord, AcceptanceState
    errors.go              engineering sentinels
    content_test.go        FF-012 §3
    canonical_test.go      FF-012 §3
    envelope_test.go       FF-012 §5

    peos/
      vocabulary.go        every featureforge value; PEOS constant re-exports
      inputs.go            per-family input structs
      recorder.go          EngineeringRecorder implementation
      codec_artifact.go    artifact + revision build/decode/project
      codec_requirement.go
      codec_decision.go
      codec_validation.go  plan, execution record, claim
      codec_lifecycle.go   definition, transition record, state assignment
      digest.go            VerifyDigest
      errors.go            wrapping helpers preserving PEOS sentinels
      vocabulary_test.go   FF-012 §2
      codec_test.go        FF-012 §4.1, §4.3, §4.4
      projection_test.go   FF-012 §4.2

  application/
    ports.go               all repository interfaces, Repositories, UnitOfWork
    recorder.go            EngineeringRecorder interface (declared here)
    clock.go               Clock, FixedClock
    errors.go              application sentinels
    commands.go            the ten command types + results
    command_capability.go  establish / revise / accept
    command_requirement.go
    command_decision.go
    command_validation.go  plan / run / claim / correct
    command_lifecycle.go
    query_ordering.go      current-revision algorithm + rationale
    query_correction.go    correction-chain algorithm + rationale
    query_readiness.go     readiness + precedence + rationale
    query_lifecycle.go     current lifecycle state + rationale
    query_timeline.go      timeline event model + ordering
    query_state.go         GetFeatureEngineeringState composition
    ordering_test.go       FF-012 §8
    correction_test.go     FF-012 §9
    readiness_test.go      FF-012 §10
    lifecycle_test.go      FF-012 §10
    timeline_test.go       FF-012 §11
    command_test.go        validation + idempotency + conflict

  infrastructure/
    memory/
      store.go             Store, overlay, locking, failure injection
      repositories.go      the eight repository implementations
      unitofwork.go        Do, rollback, nesting rejection
      store_test.go        FF-012 §6, §7 via the shared contract suite
      contract.go          shared contract suite, exported for M.4 reuse

  scenario/
    fixtures.go            FF-011 §2 identities, content, fixed clock
    scenario.go            the eleven acts as a reusable driver
    scenario_test.go       FF-012 §13 end-to-end + insertion-order variant

  architecture/
    imports.go             import-graph helpers over go/build, go/parser
    architecture_test.go   FF-012 §12
```

### Tree decisions

| Choice | Reasoning |
|---|---|
| No `cmd/` | M.3's objective is met by tests. An executable would have no behaviour of its own. It arrives in M.5 with the HTTP server. |
| `internal/scenario` is its own package | The scenario driver is used by the end-to-end test **and** will be reused by M.4 against PostgreSQL. Burying it in a `_test.go` file would make it unreusable. |
| `internal/architecture` is its own package | The import-graph helpers are ordinary code that the architecture tests consume. Keeping them out of the packages under inspection prevents a test importing what it is asserting about. |
| `contract.go` is non-test code | M.4 must run the identical suite against PostgreSQL. A `_test.go` file cannot be imported by another package's tests. |
| No placeholder packages | Nothing for HTTP, PostgreSQL, UI, or AI — not even empty directories. |

## 2. Ordered implementation checklist

Follow in order. Each step is independently compilable and testable; do not begin
a step until the previous step's tests pass.

**Phase A — foundations, no PEOS**

1. `go.mod`: add `github.com/aleka7sk/PEOS v1.0.0`. No `replace`. Run
   `go mod download` and `go mod verify`.
2. `internal/domain` — ids, entities, errors. Tests: FF-012 §1.
3. `internal/engineering` — keys, canonical JSON, digest, content. Tests:
   FF-012 §3. **Verify this package's tests pass with no PEOS import present.**
4. `internal/engineering` — envelopes, order metadata, acceptance records.
   Tests: FF-012 §5.

**Phase B — the PEOS boundary**

5. `internal/engineering/peos/vocabulary.go` — all values, constructed at init.
   Tests: FF-012 §2.
6. Codecs, one family per step, each with its round-trip, projection-fidelity,
   and constructor-flow tests before moving on: artifact/revision → requirement
   → decision → validation → lifecycle. Tests: FF-012 §4.
7. `recorder.go` — implement the `EngineeringRecorder` port.

**Phase C — ports and persistence**

8. `internal/application/ports.go`, `clock.go`, `errors.go` — interfaces only.
9. `internal/infrastructure/memory` — store, overlay, repositories, unit of work.
   Tests: FF-012 §6 and §7 via the shared contract suite.

**Phase D — algorithms**

10. `query_ordering.go` + acceptance resolution. Tests: FF-012 §8.
11. `query_correction.go`. Tests: FF-012 §9.
12. `query_readiness.go`. Tests: FF-012 §10.
13. `query_lifecycle.go`. Tests: FF-012 §10.
14. `query_timeline.go`. Tests: FF-012 §11.
15. `query_state.go` — composition.

**Phase E — commands**

16. The ten commands, in FF-010 §3 order. Tests: validation, idempotency,
    conflict, one-transaction-per-act.

**Phase F — proof**

17. `internal/architecture` + FF-012 §12. **Verify each test against a
    deliberate violation**, then revert the violation.
18. `internal/scenario` — fixtures, driver, end-to-end test, insertion-order
    variant. Tests: FF-012 §13.
19. Full run: `go build ./...`, `go vet ./...`, `go test ./...`,
    `go test -race ./...`, `go mod verify`.

## 3. M.3 execution rules

### At the start

1. Add `github.com/aleka7sk/PEOS v1.0.0` to `go.mod`.
2. **Do not add a `replace` directive.**
3. Verify the resolved version is exactly `v1.0.0` (`go list -m github.com/aleka7sk/PEOS`).
4. Verify checksums (`go mod verify` reports all modules verified).
5. **Inspect the real public API before implementing each family.** FF-011 §1
   records ten verified facts; treat them as authoritative but confirm signatures
   at the call site rather than recalling them.
6. Do not change M.1 or M.2 architecture silently. A required deviation is
   recorded in `docs/decisions/README.md` before the code is written.

### Throughout

- The only dependency added is PEOS. Not a UUID library, not a test framework,
  not an assertion library, not `golang.org/x/tools`.
- No `.go` file is created outside `internal/`.
- No PEOS repository file is modified — the module cache is read-only.
- Every error is a named sentinel; no generic internal error.
- No `time.Now` outside the production clock.
- Table-driven tests where the shape repeats.

### Definition of done

`go build ./...`, `go vet ./...`, `go test ./...`, and `go test -race ./...` all
pass; every FF-012 test exists and passes; each architecture test has been shown
to fail against a deliberate violation; `go.mod` declares exactly one requirement
with no `replace`; `go mod verify` passes.

### Recommended model and mode

**Claude Sonnet**, in **direct implementation from the approved specification**
mode. The architecture work is complete; M.3 is transcription plus disciplined
testing. Escalate to Opus only if a step reveals a genuine architecture gap, and
in that case record the decision first.

## 4. M.3 commit policy

**Recommendation: small, reviewable commits — one per checklist phase, six in
total.**

| Commit | Contents |
|---|---|
| 1 | `feat(domain): add product entities and engineering record types` — steps 1–4 |
| 2 | `feat(peos): add the PEOS integration boundary` — steps 5–7 |
| 3 | `feat(persistence): add ports and the in-memory adapter` — steps 8–9 |
| 4 | `feat(query): add current-state and timeline resolution` — steps 10–15 |
| 5 | `feat(application): add engineering commands` — step 16 |
| 6 | `test(scenario): add architecture tests and the canonical end-to-end scenario` — steps 17–19 |

One atomic commit was considered and rejected: the M.3 diff is several thousand
lines across five layers, and a single commit would make bisecting a boundary
regression impossible and review impractical. Six commits each leave the tree
green, which keeps `git bisect` useful.

Each commit must build and pass its own tests. A commit that leaves `go test ./...`
red is not acceptable, even mid-phase.

**Allowed files in M.3:** `go.mod`, `go.sum`, and files under `internal/`.
Documentation changes belong in their own commit and only when a decision is
recorded.

**Forbidden in M.3:** `.idea/`, any `cmd/` package, any HTTP/SQL/UI/AI package,
any second dependency, any `replace` directive, any modification to
`docs/spec/000`–`013` other than a recorded decision.

## 5. What M.3 does not do

| Deferred to | Item |
|---|---|
| M.4 | PostgreSQL schema, migrations, the adapter, and re-running the shared contract suite against it |
| M.5 | HTTP transport, the seven screens, `cmd/featureforge` |
| M.6 | AI context pack and the proposal boundary |
| M.7 | Independent audit and the freeze artifacts |
| Later, if ever | `RelationEnvelope`, branching revision histories, materialized projections, random identity generation, `decision.Record` |

Each deferral is a recorded decision, not an oversight. Implementing any of them
in M.3 is scope expansion and must be rejected.
