# M.5 Publication Remediation

Status: Implementation pending
Date opened: 2026-07-31
Governs: the closure of the publication-blocking findings a read-only audit
raised against the 20-commit M.5 local chain, and the exact tests that prove
each is closed.

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

_(Appended by commit 24, once commits 22 and 23 land and full verification
passes. Until this section is populated, treat this report as recording
intent and acceptance tests only — not completion.)_

## 9. Current status

**Implementation pending.** Commits 22 (M-1), 23 (M-2), and 24 (this
report's evidence appendix) have not yet landed as of this document's
creation in commit 21. **Publication of the local chain to `origin/main`
remains prohibited** until: commits 22–24 land; every acceptance test in
§6 and §7 passes; the full verification gate (`gofmt`, `go build`,
`go vet`, `go test ./...`, `go test ./... -race`, `make postgres-test`)
passes on the resulting `HEAD`; and an independent read-only publication
audit — not this document, not the implementer — confirms the chain is
ready for a fast-forward push.
