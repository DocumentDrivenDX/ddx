---
ddx:
  id: FEAT-019
  depends_on:
    - helix.prd
    - FEAT-004
    - FEAT-006
    - FEAT-010
    - FEAT-012
    - FEAT-014
---

# Feature: DDx Agent Evaluation UX

**ID:** FEAT-019
**Status:** In Progress
**Priority:** P1
**Owner:** DDx Team

## Overview

FEAT-019 defines DDx's evaluation records and user experience. It lets an
operator compare declared experiment inputs, grade the resulting work, inspect
repository effects, aggregate benchmark runs, and replay preserved work. The
inputs DDx may vary are prompts, rubrics, abstract `MinPower`, and DDx work
inputs such as a bead, base revision, or evidence bundle.

Every agent invocation crosses the Fizeau service boundary. Fizeau is the
agent runtime and independently selects the concrete harness, provider, and
model for each invocation. DDx does not reproduce that routing policy, choose
a concrete route, or turn observed routes into future routing policy.

Evaluation workflows (`compare-prompts`, `replay-bead`, and
`benchmark-suite`) live in the skills library. FEAT-019 owns their persisted
comparison, grade, replay, and benchmark projections and the UI that presents
those records. FEAT-010 remains the execution and run-record substrate.

## Product Boundary

### DDx owns

- The prompt, rubric, abstract `MinPower`, work input, base revision, and
  stable arm identifier declared for an experiment.
- One isolated DDx git worktree per side-effecting comparison arm.
- Repository evidence produced in each worktree: diff, resulting revision,
  and explicitly configured post-run checks.
- Persisted comparison, grade, replay, and benchmark records.
- Grading prompts and grading-result validation against an operator-selected
  rubric.
- Evaluation views over DDx-visible output and repository evidence.
- Abstract escalation: stronger review intent may request higher `MinPower`.
  A failed implementation may do so only in a distinct new DDx attempt after
  capability-sensitive evidence; infrastructure/transport/route/quota/auth/
  setup/operator-action/generic failures keep power unchanged.

### Fizeau owns

- Concrete harness, provider, model, profile, route selection, fallback, and
  runtime policy for every invocation.
- The full agent/tool loop, native events, transcript, session log, and
  process tree.
- Provider availability, credentials, quotas, and route diagnostics.
- Cleanup of the runtime session/process tree when the `Execute` context is
  canceled.

### Hard routing invariant

DDx must leave concrete routing unspecified. If an operator explicitly
supplies a Fizeau harness/provider/model pin, DDx may carry it only as one
comparison-wide passthrough envelope copied identically to every arm. Pins
cannot define or differ between arms and are excluded from arm identity,
fingerprints, grouping, grading, and comparison policy. DDx must not derive,
relax, strengthen, substitute, or synthesize concrete pins.

Fizeau's returned `RoutingActual` is per-run audit evidence only. DDx may show
it while inspecting that exact invocation, but must exclude it from:

- comparison and benchmark grouping;
- grading prompts, scores, and rationales;
- DDx-generated warnings;
- replay inputs and replay policy; and
- route or model efficacy policy.

The identity of an evaluation arm comes from the DDx-declared experiment
input and stable arm ID, not from the concrete route Fizeau happened to select.

## Relationship to Governing Features

- **FEAT-004** owns bead state and evidence attachment semantics.
- **FEAT-006** owns DDx's consumer adapter for the public Fizeau service
  contract.
- **FEAT-010** owns run lifecycle, execution records, worktree coordination,
  and the canonical `work` / `try` / `run` detail surface.
- **FEAT-012** owns git-aware diff and revision evidence.
- **FEAT-014** owns usage and cost projections copied from public Fizeau final
  data.
- **FEAT-019** owns evaluation-specific record types, grading semantics,
  aggregation, and UI projections. It does not create a second executor.

When an evaluation record links to a preserved `try` or `run`, the existing
project-scoped run-detail surface at `/runs/[runId]` remains canonical.
FEAT-019 adds evaluation context and links; it does not duplicate run
lifecycle, transcript, or access-control behavior.

## Current Fizeau Contract

DDx consumes the pinned public service contract:

```go
Execute(
    context.Context,
    fizeau.ServiceExecuteRequest,
) (<-chan fizeau.ServiceEvent, error)
```

Each evaluation arm is one fresh `Execute` operation.

- An immediate returned error ends the operation before an event stream is
  available. DDx records that immediate error on the arm without inventing a
  final event.
- Non-terminal `ServiceEvent` payloads are Fizeau-owned and remain opaque to
  DDx.
- A public final event carries `ServiceFinalData`. DDx may consume its
  `Status`, `ExitCode`, `Error`, `FinalText`, `DurationMS`, `Usage`,
  `Warnings`, `CostUSD`, `SessionLogPath`, and `RoutingActual` fields.
- `FinalText` is the public application output used by comparison and
  grading. Typed usage and cost fields support reporting.
- `SessionLogPath` is retained as an opaque Fizeau-owned reference. DDx does
  not parse the referenced native log or reconstruct tool events from it.
- `RoutingActual` is stored only under the audit restriction above.

The current service contract has no continuation, active-session query, or
cancel-by-session operation. DDx must not design evaluation behavior around
those nonexistent calls. Cancellation uses the context passed to `Execute`.
A replay, retry, or stronger review is a new `Execute` invocation, not a
continuation of a Fizeau session.

## Problem Statement

DDx can preserve agent attempts, but operators lack one coherent way to ask
whether a prompt, rubric, abstract power floor, or work input changed the
quality of a result. Side-effecting arms also need isolation: concurrent
experiments in one worktree interfere and make evidence unreliable.

The evaluation layer must answer questions such as:

- Did prompt B produce a better repository result than prompt A?
- Does a stronger abstract `MinPower` improve review quality for this work?
- Does a revised rubric distinguish correct from incomplete outputs?
- Does the same preserved work input still pass after a prompt change?

Those questions are about DDx-declared inputs and DDx-visible results. They
are not requests for DDx to select, compare, or learn Fizeau's concrete route.

## Requirements

### Comparison execution and isolation

1. A comparison declares two or more stable arms. Each arm identifies the
   DDx-controlled inputs that differ: prompt, rubric, `MinPower`, or work input.
   An explicit operator passthrough envelope is comparison-wide, identical on
   every arm, and not part of arm identity.
2. A side-effecting arm runs in its own DDx-managed git worktree at the same
   recorded base revision as its peers.
3. Each arm invokes Fizeau independently through `Execute(ctx, request)`.
   Unless the comparison has one explicit operator passthrough envelope, DDx
   leaves concrete harness/provider/model constraints unset. When present, the
   exact same envelope is copied to every arm.
4. DDx captures `FinalText` and typed final usage/cost fields from the public
   Fizeau outcome, plus the diff, resulting revision, and post-run checks from
   the arm worktree.
5. Immediate `Execute` errors and public final outcomes are distinct arm
   outcomes. Failure in one arm does not erase completed evidence from other
   arms.
6. The default cleanup path removes comparison worktrees after DDx has
   persisted their evidence. An explicit keep-sandbox option may retain them.

### Comparison records and views

7. Comparison records use the FEAT-010 run substrate with
   `type: comparison` and stable, additive JSON output.
8. Each arm envelope records its declared DDx input, exact operator
   passthrough constraints when present, Fizeau public final envelope or
   immediate error, DDx repository evidence, and grade references.
9. Comparison detail shows declared input differences, `FinalText`, diff,
   result revision, post-run evidence, grade, usage, cost, and the opaque
   session-log reference.
10. A per-invocation detail may show `RoutingActual` in a clearly labeled
    audit-only section. Comparison lists, filters, groupings, rankings, and
    warnings must not use it.
11. DDx must not parse a native Fizeau session log to populate a tools view,
    reconstruct output, infer route success, or repair a missing final
    outcome.

### Rubrics and grading

12. Rubrics are named DDx artifacts with version/provenance metadata and may
    be loaded by path or stable artifact reference.
13. The evaluation UI lists, views, and edits rubrics subject to the existing
    project authorization model.
14. A grading prompt contains the rubric, original task, route-neutral arm labels,
    `FinalText`, DDx repository diffs, and configured post-run evidence. It
    excludes `RoutingActual`, native session-log contents, and inferred
    harness/provider/model quality.
15. A grader invocation uses the same Fizeau `Execute` boundary. DDx may
    request a stronger reviewer by raising abstract `MinPower`; Fizeau chooses
    the concrete reviewer route.
16. DDx must not mutate operator-provided concrete pins or `MaxPower` while
    requesting stronger review.
17. The grader returns a structured result in `ServiceFinalData.FinalText`:

    ```json
    {
      "arms": [
        {
          "arm": "prompt-b",
          "score": 8,
          "max_score": 10,
          "pass": true,
          "rationale": "The repository change satisfies all rubric checks."
        }
      ]
    }
    ```

18. DDx validates the application-level JSON before appending grades. A
    malformed response or failed Fizeau operation leaves prior comparison
    evidence intact.
19. Scores are comparable only when rubric identity, rubric version, and
    `max_score` match.

### Benchmark aggregation

20. Benchmark records use the FEAT-010 run substrate with `type: benchmark`
    and link to their source comparisons.
21. Benchmark grouping uses the declared suite, prompt/rubric/work-input
    identity, stable arm ID, and requested `MinPower`. It never groups by the
    comparison-wide operator passthrough envelope or `RoutingActual`.
22. Benchmark views report completion, grade, duration, tokens, and cost and
    drill down to the source comparison and invocation records.
23. Historical suite results remain queryable for regression trends. Trend
    claims describe declared DDx input changes, not concrete route efficacy.

### Replay

24. `replay-bead` creates a fresh isolated worktree and a fresh Fizeau
    `Execute` invocation. It has no continuation or session-resume mode.
25. Replay reconstruction uses the preserved DDx request/work input, base or
    closing revision, stored `FinalText`, and DDx repository/test evidence.
    The opaque `SessionLogPath` may be retained as provenance but must not be
    parsed to reconstruct the request, tools, or output.
26. A replay may deliberately change prompt, rubric, abstract `MinPower`, or
    work input. An explicit operator pin may be supplied only as an unchanged
    passthrough constraint.
27. Replay policy must not copy, prefer, reject, or escalate from the baseline
    `RoutingActual` value.
28. If the DDx request or repository evidence needed for reconstruction is
    absent, replay reports the missing evidence and does not synthesize it
    from a Fizeau native session log.

### Escalation and failure handling

29. A stronger review may increase abstract `MinPower` by review intent. A
    failed implementation arm may increase it only in a distinct new DDx
    attempt after capability-sensitive evidence. Infrastructure, transport,
    route, quota, authentication, setup, operator-action, and generic failures
    keep power unchanged.
30. Escalation must not name or infer a concrete harness, provider, or model.
31. Each retry or review stores its own request and outcome so operators can
    distinguish DDx's declared escalation from Fizeau's independent routing.
32. `ServiceFinalData.Warnings` may be copied verbatim as Fizeau outcome data,
    but DDx must not generate evaluation warnings from `RoutingActual`.

## Non-Functional Requirements

- **Isolation:** No comparison arm may observe another arm's uncommitted
  filesystem effects.
- **Evidence integrity:** Grades and aggregates link to immutable source
  records and identify the exact rubric and declared arm input used.
- **Storage:** Large DDx-owned prompts, outputs, and diffs use attachment-backed
  storage. A Fizeau native session log remains an external opaque reference;
  DDx does not copy it into a native transcript schema.
- **Query performance:** List and aggregate views read envelope summaries
  without deserializing large attachments.
- **Compatibility:** Machine-readable schemas evolve additively.
- **Security:** Prompt, response, diff, and rubric access follows the existing
  project-scoped authorization and raw-evidence audit rules.
- **Cancellation:** User cancellation cancels the `Execute` context and waits
  for the public operation to end; DDx does not implement cancel-by-session.

## Evaluation Record Shape

A comparison arm stores three separate evidence classes:

| Evidence class | Examples | Permitted use |
|---|---|---|
| Declared DDx input | arm ID, prompt/rubric/work-input refs, base revision, `MinPower` | grouping, replay, grading context, experiment comparison |
| Comparison-wide operator passthrough | exact immutable `MaxPower`/`Harness`/`Provider`/`Model`/`Policy` supplied by the operator | identical forwarding and audit only; excluded from arm identity, fingerprinting, grouping, grading, and comparison policy |
| Public Fizeau outcome | immediate error or `ServiceFinalData`, including `FinalText`, usage, cost, warnings, opaque session-log ref | outcome and resource reporting; `FinalText` may be graded |
| DDx repository evidence | diff, result revision, post-run checks | grading, comparison, replay baseline |

`RoutingActual` is a restricted audit-only member of the public Fizeau outcome.
It is never an arm key or policy input.

## Workflow and UI Surface

Workflow shapes remain skills:

- `compare-prompts` declares arms, creates isolated DDx executions, and writes
  a comparison record.
- `replay-bead` reconstructs a DDx-visible baseline and writes a replay record.
- `benchmark-suite` expands a declared input matrix and writes benchmark
  aggregates.

The web UI provides:

- comparison and benchmark lists grouped by declared DDx experiment identity;
- comparison detail with output, repository evidence, grades, usage, and cost;
- replay detail with baseline/new DDx-visible output and repository evidence;
- rubric browse/edit surfaces; and
- links to the canonical run detail for each invocation.

Concrete route audit facts, when available, appear only inside that single
invocation's audit detail. There is no DDx provider dashboard, route picker,
route ranking, or model recommendation surface.

Example workflow invocations:

```bash
# Compare two declared prompt arms; Fizeau routes each invocation.
compare-prompts --prompt-a prompts/a.md --prompt-b prompts/b.md

# Grade with a stronger abstract reviewer requirement.
compare-prompts --grade cmp-abc123 --min-power 10

# Grade against a named rubric.
compare-prompts --grade cmp-abc123 --rubric rubrics/code-quality.md

# Replay from DDx-preserved request and repository evidence.
replay-bead ddx-abc123 --min-power 8
```

## User Stories

### US-190: Compare Declared Prompt Inputs

**As a** developer refining an agent prompt
**I want** isolated comparison arms for prompt variants
**So that** I can judge their outputs and repository effects without
filesystem interference or route-selection bias

**Acceptance Criteria:**

- Given two prompt arms at one base revision, each runs in a separate DDx
  worktree through an independent Fizeau invocation.
- The comparison view identifies arms by declared prompt input and stable arm
  ID, not `RoutingActual`.
- The detail view shows `FinalText`, diff, post-run evidence, grade, usage, and
  cost for each arm.

### US-191: Grade Work With a Stronger Reviewer

**As a** developer assessing result quality
**I want** rubric-based grading with a stronger abstract reviewer requirement
**So that** I receive a structured judgment without choosing Fizeau's route

**Acceptance Criteria:**

- The grading request contains the rubric and DDx-visible output/repository
  evidence and excludes `RoutingActual` and native session-log contents.
- A stronger review raises only `MinPower`; Fizeau independently chooses the
  harness/provider/model.
- A valid grade is appended with rubric identity, score, pass/fail, and
  rationale; a malformed grade does not corrupt the comparison.

### US-192: Detect Prompt Regressions in CI

**As a** CI system
**I want** stable JSON comparison and benchmark records
**So that** I can detect regressions in declared prompt or rubric variants

**Acceptance Criteria:**

- CI can compare matching rubric versions and declared arm identities across
  benchmark runs.
- Aggregation never groups by `RoutingActual` or emits a concrete route
  recommendation.
- Grade, repository checks, usage, and cost are machine-readable without
  parsing a Fizeau session log.

### US-193: Inspect Repository Effects

**As a** reviewer
**I want** each arm's repository diff and checks beside its public final output
**So that** I can grade what the agent actually changed

**Acceptance Criteria:**

- Diff and post-run evidence come from the isolated DDx worktree.
- A failed arm retains its immediate error or public final outcome and all
  repository evidence available at failure time.
- Native Fizeau tool/session events are not reconstructed into DDx evidence.

### US-194: Replay Preserved Work

**As a** developer investigating a regression
**I want** to replay a preserved DDx work input with deliberate input changes
**So that** I can compare the new DDx-visible result with the baseline

**Acceptance Criteria:**

- Replay reconstructs from the DDx request, revision, `FinalText`, and
  repository/test evidence.
- Replay is a fresh `Execute` invocation and never queries, continues, or
  cancels a prior session by ID.
- Baseline `RoutingActual` neither constrains nor influences the replay.

### US-195: Preserve Evaluation Provenance

**As a** developer maintaining an evaluation corpus
**I want** each record linked to its source work and public Fizeau outcome
**So that** I can reproduce DDx inputs and audit results without owning the
Fizeau transcript

**Acceptance Criteria:**

- The record links the bead/run, declared request, base/closing revision,
  public final envelope or immediate error, and DDx repository evidence.
- `SessionLogPath` is retained only as an opaque Fizeau reference.
- Missing DDx reconstruction evidence is reported explicitly rather than
  recovered by parsing a native session log.

## Dependencies

- FEAT-004 (Beads) — source work and evidence links
- FEAT-006 (Fizeau Execution Boundary) — public execution contract and final
  outcome envelope
- FEAT-008 (Web UI) — evaluation rendering surfaces
- FEAT-010 (Task Execution) — run records, lifecycle, and isolated execution
- FEAT-012 (Git Awareness) — revision and diff evidence
- FEAT-014 (Usage and Cost) — typed usage/cost projections
- Skills library — `compare-prompts`, `replay-bead`, and `benchmark-suite`
- [`SD-023`](../../02-design/solution-designs/SD-023-agent-evaluation.md) —
  canonical solution design
- [`TP-019`](../../03-test/test-plans/TP-019-agent-evaluation.md) —
  verification plan

## Out of Scope

- Concrete harness, provider, model, profile, or route selection
- Provider inventory, health, pricing, route ranking, and recommendation UI
- Learning routing or efficacy policy from `RoutingActual`
- Native Fizeau transcript/session-log parsing, copying, or reconstruction
- Fizeau continuation, active-session query, or cancel-by-session APIs
- A DDx-owned agent/tool loop or provider adapter
- Automatic prompt rewriting or unbounded exploration loops
- Container or VM isolation
- Cross-project evaluation

## Test Plan

See `docs/helix/02-design/solution-designs/SD-023-agent-evaluation.md` and
`docs/helix/03-test/test-plans/TP-019-agent-evaluation.md`.
