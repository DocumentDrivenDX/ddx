# Bead Authoring Template

> A bead's description + AC must be sufficient context for a competent
> sub-agent to execute it without hand-curation.
> — Reliability principle **P7** (`docs/helix/06-iterate/reliability-principles.md`)

This document is the canonical template + rubric for creating beads in
this repository. It exists because the 2026-05-03 bead-quality audit
(`.ddx/executions/20260503T155638-bead57f0cb9e/bead-quality-audit-2026-05-03.md`)
found that only **1 of 20** representative open beads scored 8/8 on
the sufficient-sub-agent-prompt rubric. The most common failure modes
were: no specific test names in AC (80 % miss), no `go test` +
lefthook gate in AC (80 % miss), and no root cause with file:line in
description (65 % miss).

---

## 1. The 8-criterion rubric

Every bead must satisfy:

| # | Criterion | Verifier |
|---|-----------|----------|
| (a) | **Title**: one-line scope clarity, imperative, names subsystem + change | "Pagination off-by-one in bead list endpoint" not "Pagination bug" |
| (b) | **Description**: problem + ROOT CAUSE WITH FILE:LINE + proposed fix + non-scope | Root cause cites `cli/path/file.go:42-50` |
| (c) | **AC**: numbered, verifiable, includes specific test names | At least one AC names `TestFoo` or a `go test -run` filter |
| (d) | **AC** includes "wired-in" assertion when introducing new code paths | New function → AC asserts a caller invokes it (production or test) |
| (e) | **AC** includes `go test` command + lefthook gate | Both `cd cli && go test ./...` and `lefthook run pre-commit` named |
| (f) | **Labels**: phase + area + kind + cross-refs | `phase:N`, `area:*`, `kind:*`, plus adr/spec/prevention if applicable |
| (g) | **Parent + Deps**: explicit | Parent set; deps either listed or "no deps" stated |
| (h) | Description reads as **sufficient sub-agent prompt** | A competent agent given only the bead body can pick a file and run tests without asking |

Score 8/8 = execution-ready. Less = retrofit before dispatch.

---

## 2. Template fields

```text
TITLE
  <area>: <imperative one-line scope>
  e.g. "agent: unify ExecuteLoopSpec across cobra/HTTP/server layers"

TYPE
  task | bug | epic | chore

LABELS
  phase:<N>, area:<subsystem>, kind:<category>
  + cross-refs: prevention, adr:<id>, spec:<id>, story:<n>, refactor

PARENT
  <ddx-id> of governing epic / story / spec
  (omit only if this is a root)

DEPS
  - <ddx-id-1>: <one-line why this dep matters>
  - <ddx-id-2>: <one-line why this dep matters>
  (or explicit "No deps.")

SPEC-ID (when applicable)
  --set spec-id=FEAT-NNN  | SD-NNN | TD-NNN | ADR-NNN

DESCRIPTION
  PROBLEM
    <one-paragraph statement of what is broken or missing>

  ROOT CAUSE
    <function/file:line where the defect lives>
    <what the code does today vs what it should do>
    For new features: ROOT CAUSE -> "no existing implementation"
    is OK; replace with CURRENT STATE describing the gap.

  PROPOSED FIX
    <bulleted concrete changes; cite file:line for each>
    - cli/internal/foo/bar.go:120-145  rename Baz -> Quux
    - cli/internal/foo/bar_test.go      add TestQuux_RoundTrip

  NON-SCOPE
    - <thing readers might assume is included but isn't>
    - <thing covered by sister bead ddx-NNNN>

  INTERSECTIONS (optional)
    - <ddx-id>: <how this bead relates to that one>

ACCEPTANCE
  1. <verifiable assertion 1 — names a Test, file path, command, or
     observable artifact>
  2. <verifiable assertion 2>
  3. ...
  N-1. cd cli && go test ./<paths>/... green.
  N. lefthook run pre-commit passes.
```

---

## 3. Examples

### 3.1 Example of a strong bead (audit score 8/8)

`ddx-1e516bc9` — *"fizeau Execute: explicit Harness + empty Model + non-empty Profile/MinPower must route within harness's eligible models"*

Why it scored 8/8:

- **Title (a)** names subsystem (`fizeau Execute`), the precondition (`explicit Harness + empty Model`), and the expected behavior (`route within harness's eligible models`).
- **Description (b)** opens with **OBSERVED** (one paragraph, file:line) → **SCOPE — UPSTREAM (fizeau) ONLY** → **INVESTIGATION FIRST** (with explicit close-as-no-op outcome) → **UPSTREAM CHANGE** (file:line into `service_execute.go:193-224`, `models.yaml:31-60`, `registry.go`) → explicit **NOT IN SCOPE** list naming five excluded paths.
- **AC (c)** names two concrete tests:
  `TestExecute_ExplicitHarnessEmptyModelWithProfile_RoutesWithinHarness` and
  `TestExecute_ExplicitHarnessEmptyModelNoProfile_FailsClearly`.
- **Wired-in (d)**: AC #6 asserts the resolved model surfaces in
  `.ddx/workers/<id>/spec.json`, `.ddx/executions/<run-id>/result.json`, and the
  workers UI — three distinct call-graph endpoints.
- **Test command + lefthook (e)**: AC #9 — `cd cli && go test ./internal/agent/... ./cmd/... green; lefthook pre-commit passes.`
- **Labels (f)**: `phase:2, area:agent, kind:fix, upstream-fizeau, observed-failure`.
- **Parent + Deps (g)**: parent `ddx-e34994e2`; dep `ddx-cfedee8e` with explanation that the ladder must supply MinPower for fizeau to act on.
- **Sufficient (h)**: a sub-agent given only the bead body knows which file in fizeau to read, what the gap looks like, what the negative-test outcome should be, and how to verify the end-to-end fix without asking.

### 3.2 Example of a weak bead (audit score 1/8) — annotated

`ddx-0793ac75` — *"federation: federated GraphQL resolvers + routing-metadata fields"*

Verbatim description:
```
GraphQL resolvers that aggregate across spokes (federatedWorkers,
federatedRuns, federatedDocuments, etc.). Add routing-metadata fields
exposing which spoke owns each result. Write-routing contract: hub
forwards mutations to owning node, never broadcasts (Story 15
dependency).
```

Verbatim AC:
```
1. federated* resolvers in schema.
2. Routing-metadata visible per result.
3. Mutations route to owner; tested.
4. Tests cover happy + degraded paths.
```

Failures, criterion by criterion:

- **(b) FAIL** — no problem statement (what's broken today?), no root cause, no file:line, no non-scope. The reader cannot tell where the schema lives, where resolvers go, or what "routing-metadata" should look like as a Go type.
- **(c) FAIL** — "tested" and "Tests cover happy + degraded paths" name no `Test*` function. Sub-agent has no way to know what to write or where.
- **(d) PARTIAL** — "tested" implies the resolvers are wired, but no production-call-graph assertion (e.g., "schema.graphql Mutation block exposes field X").
- **(e) FAIL** — no `go test` command, no lefthook gate.
- **(h) FAIL** — a sub-agent reading this cold cannot answer: which schema file? which resolver package? what fields on routing-metadata? what's the wire format? Hand-curation by the operator is the only path forward.

**Retrofit shape** (illustrative — not an actual edit):
```
PROBLEM
  Story 14 needs aggregate-across-spokes resolvers. Today the GraphQL
  schema (cli/internal/server/graphql/schema.graphql) exposes only
  per-node Worker/Run/Document queries. Federation requires
  federated* counterparts that fan out to spokes and stitch results.

ROOT CAUSE
  No federated resolver layer exists. cli/internal/server/graphql_resolver.go
  contains only single-node resolvers. Spoke fan-out is in
  cli/internal/server/federation_spoke.go but nothing in the GraphQL
  layer calls it.

PROPOSED FIX
  - schema.graphql: add Federated{Worker,Run,Document} types with
    `ownerSpoke: String!` field.
  - graphql_resolver.go: add federatedWorkers/federatedRuns/
    federatedDocuments resolvers calling federation_spoke.FanOut.
  - graphql_mutation.go: write-routing helper routes mutations to
    owning node via federation_spoke.RouteToOwner.

NON-SCOPE
  - New non-federated resolvers (out: covered by Story 13).
  - Spoke discovery (out: covered by ddx-fe63969f).

ACCEPTANCE
  1. schema.graphql exposes federatedWorkers/federatedRuns/
     federatedDocuments queries with ownerSpoke metadata.
  2. TestFederatedWorkers_FanOutAggregates verifies fan-out + stitch.
  3. TestFederatedMutation_RoutesToOwner verifies write-routing.
  4. TestFederatedWorkers_DegradedSpoke returns partial results +
     warning.
  5. cd cli && go test ./internal/server/... green.
  6. lefthook run pre-commit passes.
```

---

## 4. Authoring checklist (paste into your editor before running `ddx bead create`)

```text
[ ] (a) Title: imperative, one line, names subsystem + change
[ ] (b) Description has all four sub-elements:
        [ ] PROBLEM (one paragraph)
        [ ] ROOT CAUSE with file:line citation (or CURRENT STATE for new features)
        [ ] PROPOSED FIX (bulleted, file:line per item)
        [ ] NON-SCOPE
[ ] (c) Each AC line is verifiable; at least one names Test* or `go test -run`
[ ] (d) New code path → AC asserts a production or test caller invokes it
[ ] (e) AC includes:
        [ ] cd cli && go test ./<paths>/... green
        [ ] lefthook run pre-commit passes
[ ] (f) Labels: phase:<N>, area:<subsystem>, kind:<category>, +cross-refs
[ ] (g) Parent set; deps listed or "no deps" stated
[ ] (h) Self-test: re-read the bead from cold. Could a competent
        sub-agent execute it without asking a question? If no, retrofit.
[ ] No /tmp/ paths cited as load-bearing context. Inline the relevant
    excerpt instead — /tmp does not survive between machines.
```

---

## 5. Anti-patterns surfaced by the audit

- **`/tmp/` references as load-bearing context.** 5 of 20 audited beads cite a `/tmp/...` plan file. These are functionally broken links to a sub-agent on a different machine. **Inline the excerpt.**
- **"See parent" without inline summary.** Refactor children of `ddx-5cb6e6cd` (C4/C7/C11/C13) systematically defer detail to the parent epic. Each child fails the stand-alone test. **Inline the parent's relevant 5-10 lines.**
- **Vague test phrases.** "Tests cover X." "All execute-bead tests still pass." Both omit the actual `Test*` name or `-run` filter the executor needs.
- **Story-doc copy-paste.** One-paragraph beads (`ddx-0793ac75`, `ddx-0e5c3005`, `ddx-1d52a30c`, `ddx-0ca4ffda`) are story excerpts that were never expanded for execution. Story documents are *upstream* of beads, not the bead itself.
- **Implicit lefthook.** Lefthook runs in pre-commit hooks anyway, but if AC doesn't name it, executors omit it from their own verification before committing.

---

## 6. When to skip a criterion

- **Epic beads** are aggregates by design. Criterion (c) (specific test names) and (d) (wired-in) commonly pass via children. The epic should still satisfy (a), (b), (e) — pointing at the children's collective `go test` set — and (f), (g), (h).
- **Doc-only beads** legitimately omit (c) (no tests) and (d) (no code path). They must still satisfy (e)'s lefthook clause and (h).
- **Pure deletion / rename beads** legitimately omit (d). They must still cite the deletion-target's file:line in (b) and assert byte-identical behavior preservation in (c).

---

## 7. References

- `docs/helix/06-iterate/reliability-principles.md` (planned via `ddx-06b77652`) — P7 source.
- `.ddx/executions/20260503T155638-bead57f0cb9e/bead-quality-audit-2026-05-03.md` — the audit that produced this template.
- `.agents/skills/ddx/reference/beads.md` and `.claude/skills/ddx/reference/beads.md` — operational guidance on creating beads via `ddx bead create`.
