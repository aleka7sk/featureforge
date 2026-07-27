# M.3 Implementation Report — In-Memory Vertical Slice

Status: Final
Date: 2026-07-28
Phase: M.3
Governs: nothing (a report, not a specification). Source of truth remains
`docs/spec/`, then `docs/decisions/`, then tests, then implementation.

## 1. Summary

M.3 implements the complete in-memory vertical slice specified by FF-008
through FF-013: domain, engineering models, PEOS integration, in-memory
persistence, application commands and queries, architecture tests, and the
canonical end-to-end scenario. The scenario runs against the real PEOS
v1.0.0 SDK — no PEOS type is mocked or stubbed anywhere in the test suite.

195 tests pass across seven packages. `go build ./...`, `go vet ./...`,
`go test ./... -count=1`, and `go test ./... -race -count=1` are all clean.

## 2. Packages implemented

| Package | Contents |
|---|---|
| `internal/domain` | `Project`, `FeatureCard`, identity value types. No PEOS import, no derived engineering state, no setters. |
| `internal/engineering` | PEOS-independent envelopes (`ArtifactEnvelope`, `RevisionEnvelope`, `RecordEnvelope`), `CapabilitySpecificationContent`, canonical JSON + digest, revision order metadata, the acceptance journal type, and `refkeys.go` — the plain-string key-projection functions both the write side (codec) and read side (application queries) share. |
| `internal/engineering/peos` | The single PEOS SDK import boundary (AD-005): vocabulary, per-family codecs, the `Recorder`. |
| `internal/infrastructure/memory` | `Store`, `UnitOfWork` (goroutine-reentrancy-safe, copy-on-write overlay), eight repositories, a reusable repository contract test suite. |
| `internal/application` | Ports, ten commands, five current-state queries, error taxonomy, `Clock`. |
| `internal/architecture` | Import-graph tooling (`go/build`, `go/ast`, no third-party dependency) and 16 architecture tests. |
| `internal/scenario` | The canonical scenario driver (`Run`, `RunPermuted`) and its fixtures, reusable by M.4 (FF-013 §1). |

## 3. Completed canonical lifecycle

`internal/scenario.Run` executes all FF-011 engineering acts through the
real application commands, against a real `memory.UnitOfWork` and a real
`peos.Recorder`, on a clock that advances one hour per act (FF-011 §2):

Project → FeatureCard → capability Revision 1 → accept Revision 1 →
lifecycle entry (`SA-1`, drafting) → REQ-1..REQ-4 → decision evidence
(`EV-0`) → decision (`DEC-1`) → capability Revision 2 → accept Revision 2 →
validation plan (`VP-1`, activities `A-1`/`A-2`/`A-3`) → lifecycle transition
(`SA-2`, under-validation) → executions + claims for `A-1`/`A-2`/`A-3`
(`CLM-1`, `CLM-2` wrongly satisfied, `CLM-3`) → re-execution of `A-2`
(`ER-4`/`EV-4`) → correcting claim `CLM-4` (corrects `CLM-2`,
not-satisfied).

`TestCanonicalScenario` asserts the full FF-011 §9 expected end state.
`TestCanonicalScenarioInsertionOrderIndependence` runs the same scenario
twice on independent stores with the requirement-creation order and the
per-activity validation order permuted, and asserts the resolved current
revision, current claim per requirement, lifecycle state, and readiness
status are identical regardless — proving FF-004 §2 rule 7 ("insertion order
is irrelevant") and the FF-010 §6 correction-chain algorithm for the full
scenario, not only unit-level orderings.

## 4. PEOS values used

Every PEOS value in FF-011 §4's inventory is constructed and exercised:
`core.Artifact`/`ArtifactRevision`, `requirement.Requirement`/`Revision`,
`decision.Decision`/`Basis`, `validation.Plan`/`PlanRevision`/
`ExecutionRecord`/`Claim` (including `WithCorrection`),
`lifecycle.Definition`/`DefinitionVersion`/`StateAssignment`/
`TransitionRecordRevision`. No PEOS type is copied into a FeatureForge
struct (`TestNoPEOSTypeIsCopied`, `TestNoShadowStructNames`); every value
crosses `internal/engineering/peos`'s boundary through an envelope.

## 5. Consumer-side invariants added

Beyond what PEOS itself enforces:

- Revision ordering is a dense per-artifact sequence, resolved without
  reference to insertion order, timestamps, or revision-ID text (FF-004 §2,
  `ResolveCurrentRevision`).
- Correction resolution treats claims as a directed graph and selects the
  unique head; self-correction is rejected before any PEOS construction
  (AD-017), and a cycle is rejected on read regardless of how it was
  written.
- Release readiness has four statuses with precedence
  `not-ready > indeterminate > incomplete > ready` (AD-016), and a claim
  evaluated against a superseded capability revision is reported `stale`
  rather than silently read as absent.
- Lifecycle state and release readiness are resolved independently and can
  disagree (`assessed` + `not-ready` is representable, AD-018) — the
  canonical scenario itself ends `under-validation` + `not-ready`, not
  `assessed`, because `REQ-4` was never validated.

## 6. Tests

195 tests across seven packages, including:

- Domain: no derived state, no setters (reflection-based).
- Engineering: canonical JSON field order, digest sensitivity, envelope
  validation, PEOS-independent compilation.
- PEOS integration: round-trip byte-identity for all eleven constructor
  flows, error-preservation through the wrapping boundary, vocabulary
  closure, constructor-flow pins for the two verified SDK limitations
  (§7).
- In-memory persistence: the shared repository contract suite (conflict,
  idempotency, rollback, nested-transaction rejection, sequence
  uniqueness), 64-goroutine concurrent-write and 8-goroutine concurrent-
  sequence races (`-race` clean).
- Application: every command and query, including the full validation
  chain (plan → run → claim → correct) against the real recorder.
- Architecture: 16 tests, 6 of them verified against real deliberate
  violations introduced via `git` and reverted (domain importing PEOS,
  `time.Now()` outside `clock.go`, a forbidden package name, a shadow
  struct name, an operational-scenario-entity name, a `go.mod` replace
  directive) — each one caught by the intended test before being reverted.
- Scenario: the full canonical run plus its insertion-order-independence
  variant.

## 7. Deviations from the M.2 specification packet

Two bounded corrections, both recorded as decisions or documented inline;
neither changes PEOS, the package architecture, or any repository contract.

1. **AD-019** — `EstablishRequirementCommand` also writes revision-order
   metadata and an immediate `accepted` acceptance-journal entry. FF-010 §3
   listed `EstablishRequirement`'s engineering act as only "Requirement
   artifact + revision", but FF-004 §3.2 requires requirement revisions to
   resolve through the same ordering-and-acceptance contract capability
   revisions use — building the scenario against the real commands
   surfaced the gap directly (`ErrRevisionOrderMissing` on every
   `ResolveEffectiveRequirements` call). See the decision log for the
   rejected alternatives.
2. **Decision evidence recorded without a paired execution.** `EV-0` (the
   decision's basis evidence) is recorded via `recorder.RecordEvidence`
   inside a direct `internal/scenario`-composed transaction, the same way
   `RecordValidationRunCommand` records evidence internally — but without
   an execution record, since a decision's basis evidence has no validation
   activity to pair with. This adds no new command (FF-010 §3's ten stand
   unchanged); the scenario driver composes the same `UnitOfWork` +
   `EngineeringRecorder` primitives a command would, for one act the
   accepted command list does not name. Not recorded as a decision-log
   entry: it changes no domain boundary, persistence authority, "current"
   semantics, or correction model.

## 8. Known limitations

- No PostgreSQL adapter, no HTTP surface, no UI, no AI integration — all
  explicitly out of scope for M.3, deferred per the roadmap.
- `RevisionOrder`/`RevisionAcceptance` conflict detection is in-process
  lock-based, matching FF-009's in-memory contract; M.4 must restate these
  as database constraints.
- The Lifecycle Definition Version's `begin-validation` transition is
  declared with source state `specified`, but the canonical scenario moves
  directly from `drafting` to `under-validation` using that transition key
  (FF-011 §4.9, §8) — PEOS v1.0.0 does not cross-validate a
  `TransitionRecordContent`'s named transition against the
  `DefinitionVersion`'s declared source/target states at construction
  time (verified: `BuildTransition` never fetches or checks the
  definition), so this is accepted, not rejected, at write time. This is
  the accepted M.2 design (FF-011 §4.9's own worked example), not an M.3
  finding, but is called out here because it means transition-legality
  checking is a gap in what PEOS v1.0.0 validates, not something
  FeatureForge currently re-enforces on top of it.

## 9. Readiness for M.4

Every quality gate is green. `go.mod` requires exactly
`github.com/aleka7sk/PEOS v1.0.0`, direct, no replace directive.
`internal/engineering/peos` is the only importer. The repository contracts
in `internal/application/ports.go` are the exact surface M.4 must implement
against PostgreSQL; `internal/infrastructure/memory/contract.go`'s shared
suite is written to be reusable, unchanged, against any conforming
implementation. `internal/scenario` is production code specifically so M.4
can run the identical canonical scenario against the new adapter without
rewriting it.
