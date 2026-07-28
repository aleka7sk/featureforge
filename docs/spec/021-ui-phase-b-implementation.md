# FF-021 — M.5 Phase B Minimal UI Implementation

Status: Implemented
Date: 2026-07-28
Phase: M.5 Phase B (final piece of M.5; M.6 — AI Context-Pack Demonstration —
is next)
Governs: `internal/ui`, the composition change in `cmd/featureforge`, and
[AD-028](../decisions/README.md#ad-028--browser-writes-go-through-the-existing-api-handler-in-process-never-a-second-network-hop) — the browser write path this phase
required.

## Governance statement

PEOS-000 through PEOS-009 remain authoritative for every PEOS concept. This
document adds no PEOS concept, redefines none, and adds no dependency.
`internal/ui` does not import PEOS (`TestUIDoesNotImportPEOS`).

## 1. What this document is

The implementation record for the planning packet accepted ahead of coding
(see git history for the plan; its content is not reproduced here — this
document records what was actually built and where it deviated). Phase A
(FF-018) and the read-surface extension (FF-020) are prerequisites and are
unamended by this work; every field Phase B renders was already on the wire.

## 2. Scope as built

Seven FF-001 §3 screens, all nineteen existing API operations reachable, all
twelve commands reachable, no new API endpoint, no new architecture decision
beyond AD-028 (reserved by FF-015 §19 for exactly this phase).

## 3. AD-028 — the write path

Recorded in full in
[docs/decisions/README.md](../decisions/README.md#ad-028--browser-writes-go-through-the-existing-api-handler-in-process-never-a-second-network-hop). Summary: a
browser POST is form-encoded and cannot carry the frozen JSON contract or
read a 201+JSON response as anything but text, so `internal/ui` owns POST
routes that translate form fields into the API's exact JSON shape and
invoke the existing, unmodified API `http.Handler` **in-process** —
`ServeHTTP` against a hand-rolled `http.ResponseWriter` capture, never a
network socket — then interpret the real response: 303 redirect on success,
the originating screen re-rendered with input preserved on a correctable
failure. The API remains the sole owner of decoding, mapping, application
invocation, and error-code semantics.

## 4. Package boundary, as verified

`internal/ui` imports nothing under this module's `internal/` tree except
itself — not `internal/application`, not `internal/engineering`, not
`internal/domain`, not an infrastructure adapter. Only `net/http`,
`html/template`, `encoding/json`, `embed`, `net/url`, `context`, `strings`,
`maps`, `regexp` (test-only), `log/slog`, `bytes`, `io`, `time`. Four
architecture guards enforce this (§9); each was proven against a deliberate,
reverted violation before being trusted.

## 5. Screens and routes, as built

| Screen | Route(s) | FF-001 | Queries | Command forms on this screen |
|---|---|---|---|---|
| Projects | `GET /`, `GET /projects/{projectID}` | §3.1 | Q1 (filtered client-side for the detail view), Q2 | C1 (on `/`), C2 (on the detail view) |
| Feature overview | `GET /features/{featureCardID}` | §3.2 | Q3, Q7 (current revision title), Q5 (last 5) | C3 (only while no capability exists), C6 |
| Revisions | `GET /features/{featureCardID}/revisions` | §3.3 | Q3, Q6 | C4, C5 (one form per revision) |
| Requirements | `GET /features/{featureCardID}/requirements` | §3.4 | Q4 | C7 |
| Decisions | `GET /features/{featureCardID}/decisions` | §3.5 | Q4 | C8 |
| Validation | `GET /features/{featureCardID}/validation` | §3.6 | Q4, Q5 (filtered to `execution.recorded`) | C9 (only while no plan exists), C10, C11, C12 |
| Timeline | `GET /features/{featureCardID}/timeline` | §3.7 | Q5 | — (kind filter is a `GET` query parameter, render-time) |

Command POST routes:

```
POST /projects                                              C1
POST /projects/{projectID}/features                         C2
POST /features/{featureCardID}/capability                   C3
POST /features/{featureCardID}/lifecycle                     C6
POST /features/{featureCardID}/revisions                     C4
POST /features/{featureCardID}/revisions/{revisionID}/acceptance  C5
POST /features/{featureCardID}/requirements                  C7
POST /features/{featureCardID}/decisions                     C8
POST /features/{featureCardID}/validation-plan                C9
POST /features/{featureCardID}/validation-runs                C10
POST /features/{featureCardID}/claims                         C11
POST /features/{featureCardID}/claims/corrections             C12
```

A projectID absent from Q1's list, or a featureCardID the API itself 404s
on, renders the UI's own not-found page rather than a blank error. `GET
/{$}` (not a bare `/`) avoids the ServeMux-shadowing bug FF-018 §16 step 4
already found and solved.

**One deviation from FF-001 §3's literal action list, stated explicitly, as
FF-021's plan required:** C6 (`AssignLifecycleState`) appears on no FF-001
§3 action list, but is placed on the Feature overview screen beside the
lifecycle state that screen already displays — required by FF-007's M.5
exit criterion, "the full lifecycle is drivable through the UI."

## 6. Identifiers resolved by the UI, never re-typed

Every command form's `subject_artifact_id` / `subject_revision_id` /
`scope_artifact_id` is resolved server-side from Q3 (the feature's
`capability_artifact_id` and the current revision Q4 already names) —
`capabilityContext` in `internal/ui/handlers_form.go` — rather than asked of
the person filling the form. Only genuinely new identities (a new
requirement's artifact ID, a new claim's ID) are typed.

## 7. List-valued and multi-record form fields

No JavaScript means no dynamic add/remove control. Two conventions, applied
uniformly:

- **List fields** (functional behaviours, constraints, alternatives,
  assumptions, uncertainties, dependencies, open questions, expected
  evidence): a `<textarea>`, one item per line (`splitLines`,
  `internal/ui/formutil.go`).
- **Acceptance criteria** (needs a key and text per item): one line per
  criterion, `"key: text"` (`contentFormFields.contentJSON`,
  `internal/ui/handlers_form.go`).
- **Validation-plan activities** (needs six fields per activity, and C9 is
  the only chance to define them — there is no "revise plan" command): one
  line per activity, pipe-delimited —
  `key|method|outcome interpretation|requirement artifact ID|requirement
  revision ID|expected evidence,comma,separated`
  (`parsePlanActivities`).

A malformed line is dropped, not rejected — this is syntax parsing, not
domain validation; the command itself validates every field it receives, and
a dropped line simply omits that entry from what gets submitted.

## 8. Error UX, as built

Every command form handler follows one shape: on success, 303 to the screen
that shows the result; on any other outcome, the originating screen is
reloaded through the same `load*PageData` function the `GET` handler uses,
with `FormError` set to the API's own (already client-safe) message and
`FormValues` set to exactly what was submitted. This is why the "load
page data" functions in `internal/ui/handlers_page.go` are factored out of
their `GET` handlers rather than inlined — a form failure reuses the same
Q1–Q7 composition a fresh page load would use, so the rest of the screen
(existing projects, existing revisions, existing claims) stays visible
around the error, per the plan's "current read state stays visible on every
correctable failure."

## 9. Architecture guards

| Guard | What it proves |
|---|---|
| `TestOnlyUIImportsHTMLTemplate` | `html/template` is imported by `internal/ui` and nowhere else |
| `TestOnlyTransportAndCommandImportNetHTTP` | `net/http` is imported only by `internal/transport/http`, `internal/ui`, `cmd/featureforge` |
| `TestUIDoesNotImportPEOS` | `internal/ui` has no transitive PEOS import |
| `TestUIDoesNotImportApplicationOrInfrastructure` | `internal/ui` imports nothing under `internal/` besides itself |
| `TestNoTimeNowOutsideClock` (extended) | `internal/ui/middleware.go` is the one narrow, named exception (request-duration logging), matching the same pattern already established for `internal/transport/http/middleware.go` |

Every guard above was verified against a deliberate, then-reverted violation
before being trusted (`c863340`); none was weakened to make Phase B's code
pass.

## 10. Testing evidence

| File | What it proves |
|---|---|
| `apiclient_test.go` | AD-028 delegation: `callAPI` sends exactly the route/method/JSON shape a real client would, and the fake handler alone controls the outcome — including a swapped conflict response, proving a change in API semantics reaches the UI with no UI-side code change |
| `render_test.go` | Every template parses at init; hostile content is escaped; each screen's non-empty and empty rendering paths, including the superseded-claim and both-revisions-independently properties, against hand-built view-model fixtures |
| `server_test.go` | The whole pipeline against a real, empty in-memory API — empty state, static asset, unknown-route 404 — and against a real API with one project created through it |
| `pages_test.go` | All seven `GET` screens against a real API seeded with two capability revisions, a decision with a full basis, an executed validation run, and a superseded/correcting claim pair — including the project-detail 404 case |
| `forms_test.go` | All twelve commands driven through the UI's form-encoded `POST` routes with hardcoded, known action paths; a correctable-failure input-preservation proof; a proof that the API's own field-order-independent validation message is what the UI surfaces |
| `browser_scenario_test.go` / `browser_scenario_postgres_test.go` | The same twelve-command lifecycle, this time with every form action and every inter-screen link **scraped from the actually-rendered HTML**, not hardcoded — the highest-value test in this phase, run on memory and (via `FEATUREFORGE_POSTGRES_TEST_DSN`) PostgreSQL from one shared assertion body |

29 tests in `internal/ui` (28 always-run plus the PostgreSQL variant, which
skips cleanly without a DSN).

## 11. Composition root

`cmd/featureforge` builds `transporthttp.NewHandler(deps)` once, passes it
into `ui.NewHandler(ui.Dependencies{API: apiHandler, Logger: logger})`, and
mounts both on one root `http.ServeMux`: `/api/v1/` unmodified, `/`
everything else. `internal/transport/http`'s own tests, its own
`NewHandler`, and its own routing are untouched — the API remains
independently servable with no UI mounted. No UI logic exists in `main.go`;
adapter selection, migration, timeouts, and graceful shutdown are unchanged
from FF-018.

## 12. A gap the browser scenario found, and its fix

Building the link-scraped scenario (§10) surfaced a real navigability
defect the hardcoded-route tests could not see: only the Feature overview
screen carried the cross-links between Revisions / Requirements / Decisions
/ Validation / Timeline. Reaching Decisions from Requirements required
detouring back through the overview first. Fixed by extracting a shared
`feature-nav` template partial and including it on every feature-scoped
screen (`bae26ba`), rather than duplicating the link markup five times.

## 13. Documentation updates

- [AD-028](../decisions/README.md#ad-028--browser-writes-go-through-the-existing-api-handler-in-process-never-a-second-network-hop) recorded in full.
- [FF-015](015-http-api-and-ui.md)'s status line updated to record Phase B
  as implemented via FF-020 and this document; its §6.4 gains a short,
  clearly-marked "as implemented" note next to the sentence AD-028
  supersedes in substance, rather than rewriting that sentence — the
  original text is the evidence AD-028's context section cites.
- `Makefile`'s `postgres-test` target gains `./internal/ui/...`.

## 14. Verification

```
gofmt -l .                    clean
go vet ./...                  clean
go build ./...                clean
go test ./... -count=1        all packages pass
go test ./... -race -count=1  all packages pass
make postgres-test            all packages pass, including both
                               internal/ui PostgreSQL-backed tests
```

Checked directly: exactly nineteen `/api/v1/*` operations remain (unchanged
count and shapes); every FF-018/FF-020 test still passes unmodified; all
seven screens are reachable and none shadows an `/api/v1/*` route; `go.mod`
gained no dependency (`TestGoModHasOnlyApprovedRequirements` unchanged);
`grep` across every template for `<script` or an inline `on*=` handler
returns nothing; no architecture decision was reopened.

## 15. Deviations from the plan, stated explicitly

- The Projects screen (`GET /`) does not show a per-project feature-card
  count. The accepted plan's screen table listed "Q2×N" for that reason,
  but the screen's own template (built in the templates/view-models step)
  never carried a count field, and no acceptance criterion or test
  requires one — FF-001 §3.1's essential data (project identity, the
  ability to open a project) is satisfied without it, and adding N extra
  `GET` calls to the Projects list for a cosmetic count was not judged
  worth the added round-trips. Not a contradiction requiring a stop: a
  normal implementation-detail simplification within the accepted screen
  set.
- `apiclient.go`'s envelope decoding initially dropped the `"rationale"`
  sibling of `"data"` (`internal/transport/http/errors.go`'s `writeJSON`
  envelope), which three screens' current-revision and lifecycle rationale
  need. Fixed in the same commit that wired the read screens (`670bdfa`)
  before it could reach a later phase.
- §12's `feature-nav` extraction, not itemized in the original template
  step, added once the browser-scenario test exposed the gap it fixes.

No other deviation occurred. Every other planned property — no JavaScript,
no new dependency, no new endpoint, the twelve-command coverage, the
seven-screen set, the accessibility baseline, the CSRF reasoning left
undisturbed, the six-commit-minimum separation — was built and verified as
specified.

## 16. Final verdict

# M.5 COMPLETE
