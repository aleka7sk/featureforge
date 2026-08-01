# Reusable Patterns from FeatureForge

Status: Freeze artifact; independent review passed, publication gate status is
recorded in the M.7 audit ledger
Date: 2026-08-01
Audience: Belcanto architecture work after FeatureForge freeze

---

## 1. Mandatory handover rule: reuse reasoning, never implementation

This document transfers patterns and rationale. It does not designate any
FeatureForge package as shared infrastructure.

Belcanto **must not import, copy, extract, vendor, or wrap FeatureForge code**.
It must not create a shared PEOS integration library by moving FeatureForge's
`internal/engineering/peos`, envelopes, repositories, commands, inspectors,
projectors, vocabulary, or content types into a common module. It must not
couple to FeatureForge through a service API either.

Each pattern below is a question and a burden of proof for Belcanto. Belcanto
may reach a different answer when its domain, authority, tenancy, or scale is
different. Reimplementation is intentional: a second independent consumer is
the evidence needed before any genuinely common abstraction can be discussed.

## 2. Separate operational establishment from engineering authority

**Context.** Product-facing Project and FeatureCard records answer navigation
and naming questions. PEOS-backed Artifacts, Revisions, Records, and lifecycle
policy answer engineering-authority questions.

**Pattern.** Give operational and engineering values different types,
repositories, mutation rules, and lifecycle semantics. Link them explicitly;
do not embed a mutable product record inside an immutable engineering envelope
or materialize derived engineering state on the operational root.

**Why it worked.** FeatureForge could keep Project/FeatureCard establishment
small while proving immutable engineering history and replacing the persistence
adapter without changing the domain. `TestNoDerivedStateOnFeatureCard`,
`TestOperationalTypesDoNotEmbedEnvelopes`, and
`TestNoOperationalScenarioEntity` make the separation executable.

**Belcanto use.** Re-evaluate which Belcanto entities are operational and which
records establish engineering authority. Do not inherit FeatureForge's
establishment-only mutability choice; that was a POC constraint.

## 3. Use one PEOS integration seam with consumer-owned ports

**Context.** Application intent should not depend on SDK object graphs, while
the adapter that constructs and decodes PEOS values needs the SDK.

**Pattern.** Isolate PEOS imports in one consumer-owned integration package.
Expose product-shaped inputs, outputs, projectors, and inspectors across the
application boundary. Keep persistence ports consumer-owned and PEOS-free.

**Why it worked.** `internal/application`, `internal/domain`,
`internal/engineering`, infrastructure, HTTP, and UI do not import PEOS. Only
`internal/engineering/peos` constructs and inspects PEOS values. Architecture
tests fail if that import boundary drifts.

**Belcanto use.** Create Belcanto's seam from Belcanto use cases and vocabulary.
Do not copy FeatureForge's interfaces: their methods encode FeatureForge's
aggregate and replay decisions.

## 4. Treat decoded payload as authority before using projections

**Context.** Indexed projections make discovery practical, but a projection can
be stale, forged, incomplete, or insufficient to identify the authoritative
semantic value.

**Pattern.** Use projections to discover candidates, then decode the complete
population required by the rule, validate canonical payload/digest/integrity,
and prove payload–projection agreement before filtering or returning a result.

**Why it worked.** Hidden foreign-family and inverse projection contradictions
become stored-state integrity failures instead of disappearing from a query.
The M.7 Requirement-history and Decision-subject corrections follow the same
rule: no screen gets authority from `ContentDigest + SubjectKey + Key` alone.

**Belcanto use.** Define which fields are authoritative and which are indexes
for each Belcanto record. If Belcanto later materializes a read model, specify
how it is rebuilt and checked; FeatureForge's no-materialization choice is not
a universal prescription.

## 5. Store immutable history; derive current state with rationale

**Context.** "Current" revision, Claim, lifecycle state, and readiness are
answers over history, not fields to update in place.

**Pattern.** Append immutable facts. Resolve current state with explicit,
deterministic algorithms that reject ambiguity and return the evidence/rationale
that selected the answer. Direct inventory reads should not invent rationale
when no derivation occurred.

**Why it worked.** Revision ordering is independent of insertion and lexical ID
order; correction follows graph structure; lifecycle follows a validated linear
predecessor chain; readiness exposes a per-Requirement explanation. Prior
Revisions and corrected Claims stay readable.

**Belcanto use.** Preserve this decision discipline even if Belcanto chooses
different aggregates or current-state algorithms. Make every ambiguity policy
explicit and name conflicting identities in errors.

## 6. Recognize semantic replay before reconstructing a time-bearing act

**Context.** An exact retry may arrive after the clock advances. Reconstructing
the candidate first can manufacture different server-owned timestamps and turn
a replay into a false immutable conflict.

**Pattern.** Define the immutable semantic act and its identity separately from
incidental reconstruction values. Inspect the complete stored aggregate first;
recognize exact replay from validated stored semantics; write nothing on replay
or conflict. Give corrupt named occupancy precedence over ordinary conflict.

**Why it worked.** C1–C12 replay became stable after clock changes, and C7 could
grandfather opaque historical member identities without regenerating the old,
non-injective concatenation formula. M.6 proposal acceptance uses the same
discipline plus a persisted source witness and freshness check.

**Belcanto use.** Define replay per Belcanto act. Do not import FeatureForge's
identity fields, machine-code precedence, or aggregate occupancy sets.

## 7. Persist exact trace and return structured source references

**Context.** Prose, naming conventions, and a generic Origin note cannot prove
which exact immutable source justified a downstream engineering record.

**Pattern.** Persist a structured trace at the act that creates the dependent
record. Validate both ends and the relationship atomically. Carry exact source
references through read models, timeline events, context packs, and UI.

**Why it worked.** Every Requirement Revision identifies the exact capability
Revision and revision-local acceptance criterion it implements. AI context
members identify their source Artifact Revision or Record. Timeline events name
their own source plus governed references instead of relying on prose parsing.

**Belcanto use.** Inventory Belcanto trace questions first, then model the
smallest exact relations they require. Do not assume FeatureForge's
`RequirementCriterionTrace` is the right relation or key shape.

## 8. Specify adapter behavior once and run it unchanged everywhere

**Context.** An interface signature cannot express idempotence, transaction
visibility, rollback, conflict taxonomy, deterministic list order, or reference
semantics.

**Pattern.** Put observable adapter behavior in one shared consumer-side
contract suite. Run the same suite against every adapter. Keep each engineering
act inside one `UnitOfWork` callback and make retry safety a tested control-flow
invariant.

**Why it worked.** Memory and PostgreSQL implementations with unrelated storage
and concurrency mechanics produce the same answers. PostgreSQL exposed bugs
that the in-memory implementation alone could not reveal, while the shared
suite prevented adapter-specific interpretations.

**Belcanto use.** Write a Belcanto contract suite from Belcanto repository
semantics. Reuse the method, not FeatureForge's suite or `UnitOfWork` signature.

## 9. Give each intent one authoritative command boundary

**Context.** A canonical test can accidentally produce an impossible product
state by writing a repository directly, even when every end-state assertion
passes.

**Pattern.** Define one public application command per engineering intent. A
canonical application/API/UI journey must use that boundary for every write.
Test-only direct storage is limited to corruption, ordering, or adapter fixtures
whose purpose explicitly requires it.

**Why it worked.** M7-01 found that a direct Evidence seed overstated the
consumer journey. Replacing it with a governed forward citation later
materialized by C10 made the scenario prove the real act surface.

**Belcanto use.** Build Belcanto journeys only after its own intent inventory is
settled. Do not inherit FeatureForge's C1–C12 command list.

## 10. Keep AI output transient, source-bound, and human-reviewed

**Context.** A generated proposal is useful input but has neither engineering
authority nor permission to write.

**Pattern.** Assemble a canonical, source-complete context pack inside a
read-only transaction; close it before generation; give the generator exactly
one immutable value; bind the returned proposal to its context; require an
explicit ordinary human command to create authoritative state. Rejecting or
merely generating a proposal leaves engineering storage byte-identical.

**Why it worked.** The deterministic M.6 generator demonstrates the authority
boundary without hiding model calls, repositories, clocks, or transactions in
the generator. Architecture guards reject I/O and authority-package imports;
memory/PostgreSQL tests prove rollback and freshness behavior.

**Belcanto use.** Preserve the authority and trace principles. Redesign the
provider, review policy, identity, observability, privacy, and UX for Belcanto;
FeatureForge's deterministic stub is only a proof instrument.

## 11. Make architecture and vocabulary executable

**Context.** Prose boundaries decay as packages, dependencies, tables, and
vocabulary grow.

**Pattern.** Encode closed import holders, forbidden dependencies, immutable
method/table surfaces, retry-safety constraints, and exact consumer vocabulary
sets in tests that fail the build. Verify valuable guards with deliberate
violations during review.

**Why it worked.** Architecture tests caught lost `text/template` coverage,
kept PEOS imports in one package, and made M7-06/M7-08 concrete omissions that
could be repaired. The consumer-owned vocabulary test prevents accidental PEOS
namespace extension and unreviewed FeatureForge terms.

**Belcanto use.** Derive a new guard and vocabulary inventory from Belcanto's
decisions. Copying FeatureForge's allowlists would encode the wrong package
topology and could create a false sense of safety.

## 12. Handover boundary

These patterns are the complete transferable output. They do not authorize a
FeatureForge dependency or a common integration module, and they do not decide
Belcanto's product model, authentication, tenancy, API, UI, or deployment.
Those decisions begin in Belcanto after the FeatureForge freeze gate is proven.
