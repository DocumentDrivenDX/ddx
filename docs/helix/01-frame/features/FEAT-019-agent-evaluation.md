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

FEAT-019 owns the **evaluation UX layer**: comparison views, grading rubric
storage and display, and benchmark result aggregation in the web UI. It is a
child of FEAT-010 (exec substrate) — not a peer.

Workflow shapes (comparison dispatch, replay, benchmark execution) live in the
skills library (`compare-prompts`, `replay-bead`, `benchmark-suite`). FEAT-019
provides the storage, display, and aggregation surfaces that those skills write
into and that users read from.

This is deliberately an **evaluation and reporting layer** built on top of the
FEAT-010 run substrate and FEAT-006 agent service. The primary bead-execution
workflow is `ddx try` (FEAT-010), which runs an agent, captures evidence, and
lands or preserves the iteration. FEAT-019 adds: grading rubric management,
comparison record display, and benchmark result aggregation — it does not define
a competing foundational execution model or dispatch mechanism.

### Ownership split

- **FEAT-006/FEAT-010** own `ddx run`, `ddx try`, and `ddx work`, including
  the canonical bead execution workflow, `ComparisonRecord` persistence, and
  arm execution mechanics used by comparison skills.
- **FEAT-019** (this spec) owns **grading**, **replay**, and
  **benchmark suite execution** — the evaluation and reporting layer built
  on top of execute-bead preserved iterations and comparison dispatch.
- **FEAT-010** owns the exec substrate. FEAT-019 is a child of FEAT-010:
  comparison records, grading results, and benchmark aggregations are stored
  in the FEAT-010 run substrate. Workflow dispatch shapes (comparison,
  replay, benchmark) live in the skills library, not in DDx core.
- **FEAT-004** owns bead semantics including evidence. FEAT-019 adds
  evidence fields (session linkage) but does not modify bead close
  behavior.

### Relationship to the run substrate (FEAT-010)

FEAT-019 is a child of FEAT-010. Comparison, replay, and benchmark records
are stored in the FEAT-010 run substrate. FEAT-019 defines the storage schema
extensions (`type: comparison`, `type: replay`, `type: benchmark`) and owns
the web UI surfaces that display those records, including the tabbed
project-scoped run-detail surface used for drill-down. That shared tab strip
is the same `work` / `try` / `run` structure described in FEAT-010 and exposed
through FEAT-008 and FEAT-021. It does not own worktree management, run
lifecycle, or dispatch mechanics.

When an evaluation record links into a preserved `try` or `run`, the shared
project-scoped run-detail surface from FEAT-008/FEAT-010 is the canonical
drill-down view, including the layer-aware tab structure:

- `work` → `overview`
- `try` → `overview`, `prompt`, `response`, `tools`, `evidence`
- `run` → `overview`, `prompt`, `response`, `session`, `tools`, `evidence`

The canonical access audit semantics stay on that surface. FEAT-019 does not
own a parallel detail page for those links.

### Relationship to manual re-queue (FEAT-010 `runRequeue`)

Re-queue, exposed in the web UI on layer=`try` and layer=`run` Runs rows
(FEAT-008 US-086c), is the **manual operator counterpart** to the
evaluation auto-retry policies skills compose on top of FEAT-019 records.
Both go through the same backend write surface — the FEAT-010
`runRequeue` GraphQL mutation — and both produce the same canonical
`run_requeue` audit event on the originating bead (schema in FEAT-010
§"Re-queue audit event schema"). FEAT-019 does not own dispatch; it consumes the
resulting layer-2 try / layer-1 run records the same way it consumes any
other preserved `ddx try` iteration.

### Relationship to workflow skills and tools

Workflow shapes live in the skills library:

- `compare-prompts` — N-arm dispatch + aggregation
- `replay-bead` — re-run with altered conditions + baseline diff
- `benchmark-suite` — compare across prompt matrix

DDx owns evaluation record storage, grading evidence, and the evaluation UX.
Workflow plugins own invocation policies: when to experiment, which variables to
sweep, quality gates using grading results, power-selection heuristics, and
exploration loops.

### Review Surface and Run Detail Links

Evaluation records (comparison, replay, benchmark) link to preserved `ddx try`
and `ddx run` iterations through the shared project-scoped **run-detail surface**
at `/runs/[runId]`. The review surface does not duplicate the run-detail page;
instead, drill-down links from comparison, replay, and benchmark views navigate
to the canonical run-detail page where users can inspect the full prompt,
response, tool trace, and evidence. The run-detail surface provides:

- **Prompt Tab**: Raw prompt body sent to the agent
- **Response Tab**: Raw response body returned by the agent
- **Tools Tab**: Paginated list of tool calls executed, with inputs and outputs
- **Evidence Tab**: Bundle file listing with inline viewer (64 KB soft limit;
  whitelisted files: `*.txt`, `*.md`, `manifest.json`, `prompt.md`, `result.json`)
- **Session Tab** (run-layer only): Agent session metadata (harness, model, tokens, cost)

Access to the run-detail surface is gated by project-scoped membership visibility.
The `raw_transcript_viewed` audit event is recorded when a user accesses prompt,
response, or stderr fields.

## Problem Statement

**Current situation:**

- `ddx run` can dispatch one agent invocation and capture output/tokens/cost.
- Comparison skills can run multiple arms and check consensus (pass/fail).
- Fizeau gives DDx one execution boundary with route facts, tool-call logging,
  and session evidence.
- Beads define work. Commits capture verified results. But there is no
  structured way to compare harness outputs for the same prompt, replay a
  bead with a different model, or grade the quality of results.

**Pain points:**

- When evaluating whether a local Fizeau route can replace a cloud
  model for a task class, there's no structured way to compare.
- Side-effecting agent runs (file writes, shell commands) can't be safely
  compared without isolation — running two agents in the same worktree
  produces interference.
- No way to answer "is this prompt good enough?" without manual review.
- Closed beads don't record which model/harness produced the result, so
  replaying with a different model requires manual prompt reconstruction.

**Desired outcome:** Comparison, grading, and replay primitives that let
operators answer: "For this prompt, how does harness A compare to harness
B?" and "What if we reran this bead with a different model?" — with
concrete artifacts (diffs, outputs, grades) — automatically and repeatably.

The canonical architecture is
[`SD-023`](../../02-design/solution-designs/SD-023-agent-evaluation.md).

## Requirements

### Functional

> **Note on workflow shapes:** Comparison dispatch, replay, and benchmark
> execution are implemented as skills (`compare-prompts`, `replay-bead`,
> `benchmark-suite`) in the skills library. FEAT-019 owns the storage schema
> for their outputs and the web UI surfaces that display, aggregate, and manage
> those records. Requirements below describe the evaluation UX layer.

**Comparison record storage and display**

1. Comparison records written by the `compare-prompts` skill (or any skill
   that produces a `ComparisonRecord`) are stored in the FEAT-010 run
   substrate with `type: comparison`.
2. The web UI exposes a comparison detail view: per-arm output, effect diff,
   tool-call summary, test results, and grades — all from the stored record.
3. Comparison list view shows recent comparisons with per-arm summary
   (tokens, cost, duration, pass/fail) sortable and filterable by date,
   harness, and bead.
4. `--format json` output for machine-readable consumption by dun checks
   and CI pipelines.

**Side-effect capture storage**
5. Effect diffs (unified diff strings) and tool call log references are
   stored in the comparison record alongside text output.
6. The UI renders diffs with syntax highlighting; tool call logs are
   available as collapsible detail.

**Grading rubric storage and display**
7. Grading rubrics are stored as named artifacts in the DDx artifact store.
   A grading workflow can load a rubric by path; named rubrics
   can be stored and referenced by name.
8. The web UI provides a rubric browser: list, view, and edit stored rubrics.
9. Grading results (score, max_score, pass, rationale per arm) are stored in
   the comparison record and displayed in the comparison detail view.
10. Custom grading rubric content is user/workflow-defined. DDx provides
    the storage, retrieval, and display mechanism.

**Grading invocation and display**
11. A grading workflow sends a comparison record through `ddx run` and records
    the evaluation. DDx owns the resulting evidence schema and display surface;
    Fizeau owns the concrete reviewer route.
12. The workflow can request reviewer strength with `MinPower`; passthrough
    constraints remain raw Fizeau constraints when an operator supplies them.
13. The grading prompt is a standardized template that presents:
    - The original task prompt
    - Each arm's text output
    - Each arm's effect diff
    - (Optional) test results
14. The grader returns a structured evaluation per arm stored in the
    comparison record:
    ```json
    {
      "arm": "agent",
      "score": 7,
      "max_score": 10,
      "pass": true,
      "rationale": "Correct implementation but missed edge case in..."
    }
    ```
15. Grades are appended to the comparison record in the FEAT-010 substrate.
16. Custom grading rubrics can be provided via `--rubric <file>` to
    replace the default template. DDx provides the rubric loading and
    storage mechanism; rubric content is user/workflow-defined (see
    requirements 7–10 above).

**Benchmark result aggregation**
17. Benchmark run records (produced by the `benchmark-suite` skill) are
    stored in the FEAT-010 substrate with `type: benchmark`.
18. The web UI provides a benchmark results view: per-arm aggregate
    statistics (completed, failed, total tokens, cost, avg duration) and
    drill-down to individual comparison records per prompt.
19. `--format json` for machine-readable output (consumed by dun checks,
    workflow evaluation gates, CI pipelines).
19a. Historical benchmark results are retained and queryable; the UI
     supports trend views across benchmark runs of the same suite.

**Bead-session linkage** (foundation for replay)
20. When a bead is closed after an agent run, record the `session_id` (from
    `sessions.jsonl`) and `closing_commit_sha` as bead evidence. The session
    already owns model, harness, tokens, and cost — do not duplicate those
    fields on the bead. `ddx bead show` resolves agent metadata from the
    linked session.
21. If no session link is recorded (manual close, legacy beads), replay
    still works but reports "baseline session unknown" and cannot provide
    a diff comparison.

**Replay result display**
22. Replay execution is handled by the `replay-bead` skill. FEAT-019 owns
    the storage schema for replay records and the web UI display surface.
23. Replay records are stored in the FEAT-010 substrate with `type: replay`,
    linked to the originating bead via `bead_id` and `session_id`.
24. The web UI shows a replay detail view: the new diff, the baseline diff,
    a side-by-side comparison, and any `--post-run` test result.
25. The bead detail view in the web UI includes a "Replays" tab listing all
    replay records for that bead with model/harness/outcome summary.

### Non-Functional

- **Storage:** Comparison records include diffs and outputs which can be
  large. Use the same attachment-backed storage as session logs (FEAT-006).
- **Query performance:** Comparison list and benchmark aggregation views must
  load without full record deserialization — store per-arm summary fields
  at the record envelope level.
- **Format compatibility:** `--format json` output must be stable for CI
  consumers; schema changes must be additive.

## Design Principles

### Evaluation UX, not execution framework

FEAT-019 provides the storage schema and UI surfaces for evaluation results:

- Store and display comparison records (written by skills)
- Store, manage, and display grading rubrics
- Aggregate and display benchmark results
- Invoke the grading harness and record results

FEAT-019 does **not** provide:

- Comparison dispatch (→ `compare-prompts` skill)
- Replay execution (→ `replay-bead` skill)
- Benchmark execution (→ `benchmark-suite` skill)
- Prompt optimization strategies
- Model selection policies
- Grading rubric content (beyond a sensible default)

Skills compose DDx primitives into workflow shapes. FEAT-019 stores and
surfaces what skills produce.

## CLI Interface

> **Workflow dispatch shapes** (`compare`, `replay`, `benchmark`, grading) are
> skills in the skills library. FEAT-019 owns evaluation records and the record
> query/display surface.

```bash
# Grade a comparison using a reviewer route
compare-prompts --grade cmp-abc123 --min-power 10

# Grade with custom rubric
compare-prompts --grade cmp-abc123 --rubric rubrics/code-quality.md

# List recent comparisons through the evaluation UI/API
# Show comparison detail through the shared run-detail surface

# Workflow dispatch shapes are skills — not DDx CLI core:
# compare-prompts skill:  N-arm dispatch + aggregation
# replay-bead skill:      re-run with altered conditions + baseline diff
# benchmark-suite skill:  compare across prompt matrix
```

### Configuration

```yaml
# .ddx/config.yaml
evaluation:
  compare:
    default_grader_min_power: 10 # requested reviewer strength for grading
```

## User Stories

### US-190: Developer Views Comparison Results in Web UI

**As a** developer evaluating whether a local model can handle a task class
**I want** to view comparison records in the web UI after the `compare-prompts` skill runs
**So that** I can see concrete differences in output quality and cost

**Acceptance Criteria:**

- Given a comparison record exists in the FEAT-010 substrate, then the web UI
  comparison list shows per-arm summary (tokens, cost, duration)
- Given I open a comparison detail view, then I see per-arm output, effect diff,
  and grades (if graded)
- Given `--format json`, then machine-readable comparison output is available
  via CLI for CI consumption

### US-191: Developer Grades Agent Outputs

**As a** developer assessing prompt quality
**I want** to have a smart model grade the outputs of a comparison run
**So that** I get a structured quality score without manual review

**Acceptance Criteria:**

- Given a completed comparison, when a grading workflow evaluates `<id>`,
  then a reviewer route evaluates each arm
- Given the grading completes, then each arm has a score, pass/fail, and
  rationale in the comparison record
- Given I specify `--rubric custom.md`, then the custom rubric replaces
  the default grading template

### US-192: CI Pipeline Runs Prompt Regression Test

**As a** CI system ensuring prompt quality
**I want** to compare agent outputs against a recorded baseline
**So that** prompt changes that degrade quality are caught before merge

**Acceptance Criteria:**

- Given a comparison with an `agent` arm and the virtual provider baseline,
  when the DDx Agent output diverges significantly, then the
  comparison record reflects the difference
- Given `--format json` output, then CI can parse the comparison and
  fail the pipeline on grade regression

### US-193: Developer Inspects Effect Diffs in Comparison View

**As a** developer reviewing comparison experiment results
**I want** to see each arm's effect diff in the comparison detail view
**So that** I can understand what each model actually changed in the codebase

**Acceptance Criteria:**

- Given a comparison record with effect diffs, then the web UI comparison
  detail view renders each arm's diff with syntax highlighting
- Given a comparison has tool call logs, then they are accessible as
  collapsible detail in the UI
- Given an arm failed, then its failure reason is shown in the comparison view

### US-194: Developer Views Replay Results on Bead Detail

**As a** developer evaluating local model capability
**I want** to view replay results in the bead detail view after the `replay-bead` skill runs
**So that** I can compare the new model's output against the known-good result

**Acceptance Criteria:**

- Given a replay record exists for a bead, then the bead detail view includes
  a "Replays" tab listing all replays with model/harness/outcome summary
- Given I open a replay detail view, then I see the new diff, the baseline diff,
  and a side-by-side comparison
- Given a replay has a `--post-run` test result, then pass/fail is shown in the
  replay detail view

### US-195: Bead Links to Agent Session on Close

**As a** developer building a replay corpus
**I want** bead close to record the session ID and closing commit
**So that** future replays can reconstruct the exact prompt and baseline

**Acceptance Criteria:**

- Given an agent run closes bead ddx-xxx, then `session_id` and
  `closing_commit_sha` are recorded as bead evidence
- Given `ddx bead show ddx-xxx`, then model/harness/tokens/cost are
  resolved from the linked session (not stored on the bead)
- Given a bead closed without an agent run, then no session link exists
  and `ddx bead show` omits agent metadata

### US-196: Evaluation UI Displays Records from Preserved Try Iterations

**As** a developer evaluating bead execution quality
**I want** FEAT-019 UI to display comparison/grading records sourced from preserved `ddx try` iterations
**So that** the evaluation UX surfaces what skills produce from preserved iterations — not a parallel execution model

**Acceptance Criteria:**

- Given preserved `ddx try` iterations exist for a bead, when skills write
  comparison or benchmark records referencing those iterations, then FEAT-019
  UI displays them without requiring a separate evaluation execution path.
- Given an evaluation record references a preserved try iteration, when
  provenance is inspected in the UI, then it traces back to the originating
  try session ID and hidden ref — no FEAT-019-owned duplicate provenance record exists.
- Given an evaluation record links to a run detail, then opening that link
  uses the shared project-scoped run-detail surface at `/runs/[runId]`, including the layer-
  appropriate tab strip (`work` → `overview`; `try` → `overview`, `prompt`,
  `response`, `tools`, `evidence`; `run` → `overview`, `prompt`, `response`,
  `session`, `tools`, `evidence`), the Evidence tab / download affordances,
  and the same project-membership visibility rules that drive the FEAT-008/
  FEAT-010 audit event.

## Dependencies

- FEAT-010 (Task Execution) — run substrate; comparison/replay/benchmark records
  are stored here; FEAT-019 is a child of FEAT-010
- FEAT-006 (Fizeau Execution Boundary) — execution request envelope, route facts,
  session evidence, and grading workflow dispatch through `ddx run`
- FEAT-008 (Web UI) — UI rendering surfaces for comparison/replay/benchmark views
- FEAT-014 (Token Awareness) — token/cost tracking per arm (displayed in comparison views)
- FEAT-012 (Git Awareness) — git diff rendering in comparison views
- Skills library — `compare-prompts`, `replay-bead`, `benchmark-suite` write the
  records that FEAT-019 displays

## Out of Scope

- **Comparison dispatch** — running N harnesses on the same prompt is the
  `compare-prompts` skill, not DDx core.
- **Replay execution** — running a bead with a different model is the
  `replay-bead` skill, not DDx core.
- **Benchmark execution** — running a prompt matrix is the `benchmark-suite`
  skill, not DDx core.
- **Exploration loops** — "try 10 ideas to improve metric X" is a workflow
  plugin loop that calls skills, not a DDx execution mode.
- **Model selection policies** — workflow tools decide which model for which
  task based on comparison results. DDx provides the data.
- **Prompt optimization** — automatic prompt rewriting is out of scope.
  DDx provides comparison and grading; plugins provide the iteration loop.
- **Structured git trailers** — bead + session log already have the data.
  Don't duplicate into git commit metadata.
- **Autonomy semantics and exploration policy** — DDx does not define what autonomy levels mean or when to run experiments; those are delegated to workflow tools
- **Escalation and supervisory rules** — whether to accept a result, retry, or escalate is workflow tool policy; DDx provides the comparison and grading data
- Container or VM-level sandboxing
- Cross-repo or cross-project comparison

## Test Plan

See `docs/helix/02-design/solution-designs/SD-023-agent-evaluation.md` and
`docs/helix/03-test/test-plans/TP-019-agent-evaluation.md`.
