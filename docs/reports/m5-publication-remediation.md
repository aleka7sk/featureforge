# M.5 Publication Remediation

Status: Remediation implemented; publication re-audit required
Date opened: 2026-07-31
Date implementation evidence appended: 2026-07-31
Governs: the closure of the publication-blocking findings a read-only audit
raised against the 20-commit M.5 local chain, and the exact tests that prove
each is closed.

**No publication has occurred.** All work recorded in this document is
local. `origin/main` was not fetched, pulled, or pushed to at any point
during this remediation; its `refs/heads/main` remained
`c1b6c952a7becbbc4f5bb79ebc05e979ac38b925` throughout. Whether the
resulting chain is safe to publish is the subject of the independent
re-audit §9 requires, not a conclusion this document draws.

## 1. Baseline this remediation is against

| | |
|---|---|
| Audited local `HEAD` | `87302aa25bb1ba0ff9867d367c6d20f249bcbd76` |
| Remote `refs/heads/main` (unchanged throughout the audit and this remediation) | `c1b6c952a7becbbc4f5bb79ebc05e979ac38b925` |
| Range audited | `c1b6c952a7becbbc4f5bb79ebc05e979ac38b925..87302aa25bb1ba0ff9867d367c6d20f249bcbd76` (20 commits, linear, no merges) |
| Nature of the chain at audit time | Candidate local work, not yet published: 20 ahead / 0 behind `origin/main` |

The audit's full method (topology verification, per-commit isolated
`git archive` builds, deliberate architecture-guard violations, an executable
probe proving population loss) is not reproduced here; only its two
publication-blocking findings and four documentation discrepancies are
carried forward, with the exact remediation each received.

## 2. M-1 — timeline silently lost prior-revision validation activity

**Finding.** `GetFeatureTimelineForCard`
(`internal/application/query_reads.go`) reused
`discoverEngineeringStateComponents`, which scoped execution, claim, and
evidence discovery to the capability's *current* revision only. An
execution, a piece of evidence, or a claim recorded against an earlier
revision disappeared from the timeline the instant a later revision became
current — though nothing corrected or withdrew it.

**Violated contract.** FF-006 §1: the timeline is computed from every
immutable record, not the current revision's alone. FF-007's M.5 exit
criterion: "superseded claims and prior revisions are visible, not hidden."

**Executable evidence at audit time (read-only, never committed).**

```
BEFORE revision 2 — execution=1 evidence=1 claim=1
AFTER  revision 2 — execution=0 evidence=0 claim=0
POPULATION LOSS CONFIRMED: execution.recorded, evidence.recorded,
claim.recorded disappeared from the timeline after CAP-1-REV-2 became
current; the underlying records ER-1/EV-1/CLM-1 are still stored and were
never corrected or withdrawn
```

**Why the canonical scenario never caught it.** FF-011's claims (C-1…C-4)
are all recorded against `CAP-1-REV-2`, which stays current for the rest of
the scenario; no canonical-scenario record is ever recorded against a
revision that later becomes superseded. `DiscoverDecisionIDs` was already
correctly history-wide (a decision *is* recorded against `CAP-1-REV-1` in
the canonical scenario, at step 10, before revision 2 exists) — which is why
this defect was scoped to executions/claims/evidence only, and why it
survived every existing test.

**Disposition.** See §5 (D1, D2) and §6 for the corrected contract and its
tests.

## 3. M-2 — the `text/template` prohibition was silently lost

**Finding.** `origin/main`'s `TestNoHTTPDatabaseUIOrAIPackage` forbade
`net/http`, `database/sql`, `html/template`, and `text/template` anywhere
under `internal/`. AD-023 (in this chain) decomposed it into four named
tests. None of the four asserts anything about `text/template`.

**Violated contract.** AD-023 itself: "`html/template`/`text/template`
remain forbidden everywhere in Phase A." FF-018 §2.2's holder table:
`| text/template | none | everywhere |`.

**Executable evidence at audit time.** A blank `text/template` import
inserted into `internal/transport/http/decode.go`, in an isolated
`git archive` snapshot never committed to the real repository, left
`go test ./internal/architecture/...` **passing**. Three control violations
(PEOS in `cmd/`, `net/http` in `internal/application`, `html/template` in
`internal/transport/http`) in the same run correctly failed, naming the
offending package.

**Disposition.** See §7 for the restored guard and its deliberate-violation
proof.

## 4. Documentation discrepancies m-1…m-4

| # | Finding | Disposition |
|---|---|---|
| m-1 | AD-023 states `net/http` holders as only `internal/transport/http` and `cmd/featureforge`; the implementation correctly added `internal/ui` (AD-028) but neither decision recorded the extension in AD-023's own text | Closed: AD-023 gained a "Later bounded extension" paragraph cross-referencing AD-028; AD-028 gained a "Bounded extension to AD-023" paragraph in its consequences. Neither decision's original text was rewritten. |
| m-2 | AD-024 promises Phase B "will use `crypto/rand`... when it is needed"; Phase B landed and used client-supplied/context-resolved identity instead, with no decision recording the change | Closed: AD-029 (§5, D3). |
| m-3 | The decision log's AD-025 paragraph still said the HTTP API/UI work "remains open" after FF-018/FF-020/FF-021 had all landed | Closed: the paragraph is left as originally written, marked explicitly as true *when AD-025 landed*, with the current status appended after it. |
| m-4 | FF-015 §6.4's original sentence ("screens submit HTML forms to the same API endpoints") is superseded by AD-028 but was not marked as such at the point a reader would encounter it | Closed: §6.4 now carries an explicit "Superseded by AD-028" block with the five-point as-built contract, before the original sentence's surrounding prose. |

## 5. Authorized architecture dispositions (D1–D4)

These were provided as settled decisions for this remediation, not reopened
as alternatives.

**D1 — separate current-state and historical populations.** Q3
(`GetFeatureOverview`) and Q4 (`GetFeatureEngineeringStateForCard`) remain
current-revision queries; historical executions or claims must not
influence current readiness, lifecycle interpretation, or rationale. Q5
(`GetFeatureTimelineForCard`) is history-wide: its population covers every
immutable engineering record belonging to every revision of the capability.
A dedicated application-owned history-wide discovery path serves Q5 only,
composed inside the existing single `UnitOfWork.Do`; transport and UI
remain unaware of repositories, revision iteration, or population
semantics. No new repository method, port, adapter operation, migration, or
stored projection.

**D2 — evidence union.** For Q5: enumerate all capability revisions through
the existing repository contract; discover executions and claims for every
revision's subject; union by authoritative record ID; derive evidence from
the unioned executions and claims; deduplicate evidence by its own
identity; one timeline event per underlying immutable source; deterministic
ordering unchanged; a missing or malformed referenced source remains the
existing hard application error, never a silent omission.

**D3 — AD-029, command identities required at the client edge.** Recorded
in full in `docs/decisions/README.md`. Summary: no server-side identity
generation on omission; omission is `400`; replay of identical
identity+content is idempotent; replay of identical identity with
different content is `409`; no UUID/`crypto/rand`/`math/rand`/idempotency-key
mechanism introduced.

**D4 — named import holders.** `net/http`: `internal/transport/http`,
`internal/ui`, `cmd/featureforge`. `html/template`: `internal/ui` only.
`text/template`: forbidden everywhere under both `internal/` and `cmd/`.

## 6. M-1 remediation — exact tests

| Test | Package | Adapter | Proves |
|---|---|---|---|
| `TestGetFeatureTimelineForCard_PreservesPriorRevisionActivity` | `internal/application` | memory | Red before the fix (confirmed: execution/evidence/claim counts drop to zero after the second revision is accepted), green after |
| `TestGetFeatureEngineeringStateForCard_PriorRevisionClaimStaysStale` | `internal/application` | memory | D1's other half: Q3/Q4 did not broaden — a claim against the superseded revision is reported stale, readiness is not `ready` |
| `TestTimelinePreservesPriorRevisionValidation` | `internal/transport/http` | memory | The same regression proven through the real `GET .../timeline` handler |
| `TestTimelinePreservesPriorRevisionValidationPostgres` | `internal/transport/http` | PostgreSQL (skips cleanly without `FEATUREFORGE_POSTGRES_TEST_DSN`) | The same regression against the PostgreSQL adapter |

Exact commit hash, red/green transcript, and full verification results are
appended to §8 once implementation lands (commit 24).

## 7. M-2 remediation — exact test and deliberate-violation proof

`TestTextTemplateIsNeverImported` (`internal/architecture/architecture_test.go`),
scanning both `internal/` and `cmd/` via the existing `allPackagesIncludingCmd`
helper, rejecting any `text/template` import anywhere, mirroring
`TestDatabaseSQLIsNeverImported`'s absolute, no-holder shape.

Deliberate-violation proof method: one isolated `git archive` snapshot under
a uniquely generated `/tmp` directory, never `git worktree`, never a change
to the real repository; a blank `text/template` import inserted into a
non-UI package inside that snapshot only; the targeted architecture test run
against the snapshot; confirmed failing, naming the inserted import; snapshot
directory removed. Exact evidence appended to §8.

## 8. Implementation evidence

### 8.1 Commits

| Commit | Hash | Subject | Changed paths |
|---|---|---|---|
| 22 | `d41af4da377a810a9108cce4775dfbcbe956bad8` | `fix(application): preserve prior-revision activity in timeline` | `internal/application/query_reads.go`, `internal/application/query_timeline.go`, `internal/application/timeline_history_test.go` (new), `internal/transport/http/timeline_history_test.go` (new) — 4 files, +334/−33 |
| 23 | `3fde4da9c07689bca694527e5bc64ef997b45913` | `test(architecture): restore text/template prohibition` | `internal/architecture/architecture_test.go` — 1 file, +23/−0 |

Both single-parent, both directly on top of commit 21
(`0422203...` `docs(m5): define publication remediation contract`), which
sits directly on the audited baseline `87302aa`. No commit was amended,
squashed, or reordered; no existing commit among the original 20 or commit
21 was touched.

### 8.2 M-1 — red before, green after

**Red**, run against the unfixed production code before any of commit 22's
production changes were made (`internal/application` only, the two new test
functions added first, nothing else changed):

```
=== RUN   TestGetFeatureTimelineForCard_PreservesPriorRevisionActivity
    timeline_history_test.go:71: execution.recorded count after CAP-1-REV-2 became current = 0, want exactly 1 (...)
    timeline_history_test.go:71: evidence.recorded count after CAP-1-REV-2 became current = 0, want exactly 1 (...)
    timeline_history_test.go:71: claim.recorded count after CAP-1-REV-2 became current = 0, want exactly 1 (...)
--- FAIL: TestGetFeatureTimelineForCard_PreservesPriorRevisionActivity (0.00s)
=== RUN   TestGetFeatureEngineeringStateForCard_PriorRevisionClaimStaysStale
--- PASS: TestGetFeatureEngineeringStateForCard_PriorRevisionClaimStaysStale (0.00s)
FAIL
```

The Q3/Q4 non-broadening test passed even before the fix — expected, since
readiness resolution was never touched by the defect (it resolves claims
independently, scoped to the current revision, via `ResolveCurrentClaim`)
and this test proves that guarantee held both before and after.

**Green**, after `DiscoverExecutionAndClaimIDsAllRevisions` was added and
`GetFeatureTimelineForCard` wired to it:

```
=== RUN   TestGetFeatureTimelineForCard_PreservesPriorRevisionActivity
--- PASS: TestGetFeatureTimelineForCard_PreservesPriorRevisionActivity (0.00s)
=== RUN   TestGetFeatureEngineeringStateForCard_PriorRevisionClaimStaysStale
--- PASS: TestGetFeatureEngineeringStateForCard_PriorRevisionClaimStaysStale (0.00s)
PASS
ok  	github.com/aleka7sk/featureforge/internal/application	0.599s
```

### 8.3 Proof: Q3/Q4 stayed current-revision-scoped; Q5 became history-wide

- `TestGetFeatureEngineeringStateForCard_PriorRevisionClaimStaysStale`
  asserts, after `CAP-1-REV-2` is current, that `state.Readiness.Status !=
  ready` and that REQ-1's `PerRequirement` entry is `Stale` — a claim
  against the superseded `CAP-1-REV-1` does not satisfy the new current
  revision. This exercises the same `ResolveReadiness` /
  `ResolveCurrentClaim` path unchanged by commit 22, confirming
  `discoverEngineeringStateComponents` (now stripped of the dead,
  wrongly-scoped fields Q3/Q4 never consumed) still backs Q3/Q4 with
  exactly the current-revision population it always had.
- `TestGetFeatureTimelineForCard_PreservesPriorRevisionActivity` and its
  HTTP-level counterparts assert the opposite for Q5: the same
  prior-revision records remain visible after the same transition.
- `internal/scenario`'s canonical-scenario tests pass unmodified (no file
  in that package changed), confirming the fix does not alter the
  FF-011 end state either adapter already proved.

### 8.4 Proof: prior-revision execution, evidence, and claim each remain exactly once

Every assertion above checks for a count of exactly `1` per kind after the
transition — not "at least one" — ruling out both silent loss and
accidental duplication from the revision-union logic
(`DiscoverExecutionAndClaimIDsAllRevisions` dedupes by record ID across
revisions). Verified on both adapters (§8.6).

### 8.5 M-2 — deliberate-violation evidence

```
=== RUN   TestTextTemplateIsNeverImported
--- PASS: TestTextTemplateIsNeverImported (0.02s)
```
on the real, unviolated repository, both before and after commit 23.

Deliberate-violation snapshot (never committed to this repository): the
working tree containing the new guard was captured with `git stash create`
(a dangling commit object, no working-tree or index change, no entry added
to the stash ref — confirmed via `git stash list` returning empty
afterward), archived with `git archive <that-commit>` into one
`mktemp -d` directory (no `git worktree` used), and a blank `text/template`
import was inserted into `internal/application/errors.go` inside that
snapshot only:

```
=== RUN   TestTextTemplateIsNeverImported
    architecture_test.go:482: github.com/aleka7sk/featureforge/internal/application imports text/template, which no package may import (AD-023)
--- FAIL: TestTextTemplateIsNeverImported (0.02s)
FAIL
```

Three control violations run in the same style of isolated snapshot during
the preceding audit (PEOS import in `cmd/featureforge`, `net/http` in
`internal/application`, `html/template` in `internal/transport/http`) each
failed correctly, confirming the harness itself (not just this one guard)
identifies a genuine violation rather than passing regardless of content.
The temporary directory was removed immediately after the proof; the real
repository's tracked tree was unmodified throughout (`git status
--porcelain` showed only the intended, already-staged `architecture_test.go`
change and the pre-existing untracked `.DS_Store`).

### 8.6 Targeted test results (both adapters)

| Test | Memory | PostgreSQL |
|---|---|---|
| `TestGetFeatureTimelineForCard_PreservesPriorRevisionActivity` | PASS | n/a (`internal/application` is memory-only by this package's established convention) |
| `TestGetFeatureEngineeringStateForCard_PriorRevisionClaimStaysStale` | PASS | n/a |
| `TestTimelinePreservesPriorRevisionValidation` | PASS | — |
| `TestTimelinePreservesPriorRevisionValidationPostgres` | — | PASS (0.17s, against a real, migrated, isolated schema; skips cleanly without `FEATUREFORGE_POSTGRES_TEST_DSN`) |
| `TestTextTemplateIsNeverImported` | PASS | n/a (architecture guard, adapter-independent) |

### 8.7 Full verification gate, on `HEAD` after commit 23

```
gofmt -l .                                        → clean
GOWORK=off GOFLAGS=-mod=readonly go build ./...    → clean
GOWORK=off GOFLAGS=-mod=readonly go vet ./...      → clean
GOWORK=off GOFLAGS=-mod=readonly go test ./... -count=1
    → ok: application, architecture, domain, engineering, engineering/peos,
      infrastructure/memory, infrastructure/postgres, scenario,
      transport/http, ui (cmd/featureforge, infrastructure/contracttest:
      no test files, as established)
GOWORK=off GOFLAGS=-mod=readonly go test ./... -race -count=1
    → ok, all the same packages, zero races
make postgres-test
    → 109 run, 109 PASS, 0 SKIP, 0 FAIL; container started, migrated,
      exercised, and torn down (`docker compose ... down -v`) — genuinely
      executed against PostgreSQL, not skipped
git diff --check c1b6c952a7becbbc4f5bb79ebc05e979ac38b925..HEAD → clean
```

The PostgreSQL run count rose from the audit's original 107 to 109: the two
new HTTP-level timeline regression tests (memory- and Postgres-gated
variants) both executed, with zero skips.

### 8.8 m-1…m-4 disposition

All four closed in commit 21 (`docs(m5): define publication remediation
contract`) — documentation only, no code change:

- **m-1** (AD-023/AD-028 holder cross-reference) — AD-023 gained a "Later
  bounded extension" paragraph; AD-028 gained a "Bounded extension to
  AD-023" paragraph in its own consequences. Neither decision's original
  text was rewritten.
- **m-2** (AD-024 identity-generation promise vs. as-built) — AD-029
  recorded; see §8.9.
- **m-3** (stale "remains open" sentence after AD-025) — left as originally
  written, explicitly marked as true only at the time it was written, with
  current status appended immediately after.
- **m-4** (FF-015 §6.4's superseded sentence) — marked "Superseded by
  AD-028" with the five-point as-built contract, ahead of the original
  sentence's surrounding prose.

### 8.9 AD-029 implementation evidence

No code change was required or made: FF-018's and FF-021's identity
handling already matched AD-029's decision (client-supplied identity
required; omission is `400 ErrInvalidCommand`; no server-side generator).
`internal/application`, `internal/transport/http`, and `internal/ui` were
re-inspected during this remediation (§ Evidence read in FF-021 §6's
`capabilityContext` resolution and `dto_command.go`'s twelve request DTOs)
and confirmed unchanged by, and already conformant with, AD-029. This
decision records the as-built contract; it does not alter it.

## 9. Current status

**Remediation implemented; publication re-audit required.** Commits 22
(M-1), 23 (M-2), and 24 (this evidence appendix) have landed on top of
commit 21. Every acceptance test named in §6 and §7 passes, on both
adapters where applicable. The full verification gate in §8.7 passes on
the resulting `HEAD`. **No publication has occurred** — `origin/main` is
unchanged at `c1b6c952a7becbbc4f5bb79ebc05e979ac38b925` throughout.

This document does not certify the chain ready for a fast-forward push.
That judgment — including whatever this remediation itself may have missed
— belongs to an independent read-only publication audit, not to the
implementer who just finished the work it would be reviewing.
