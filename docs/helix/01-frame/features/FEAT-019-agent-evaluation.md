---
ddx:
  id: FEAT-019
  depends_on:
    - helix.prd
    - FEAT-006
    - FEAT-014
---
# Feature: Agent Evaluation and Prompt Comparison

**ID:** FEAT-019
**Status:** In Progress
**Priority:** P1
**Owner:** DDx Team

## Overview

DDx can dispatch the same prompt to multiple agent harnesses and capture
structured results. This feature adds the evaluation surface: sandboxed
comparison runs that capture side effects, automated grading of outputs,
and comparison records that make prompt quality measurable.

This is deliberately a **platform primitive**, not a prompt engineering
framework. DDx provides: "run N harnesses on the same prompt in isolation,
capture everything, grade the results." Workflow tools (HELIX) and quality
runners (dun) compose these primitives into methodology-specific evaluation
policies.

### Relationship to FEAT-010 (Executions)

FEAT-010 exec and FEAT-019 comparison are **peer concepts**, not
parent-child. Exec runs one operation and captures one result. Comparison
runs N operations in parallel and captures comparative results. They share
infrastructure (bead linkage, provenance, structured results) but have
fundamentally different shapes. Do not attempt to model comparison as an
exec projection.

### Relationship to HELIX

DDx owns the comparison, grading, and replay primitives. HELIX owns the
policies that use them: when to experiment, which variables to sweep,
quality gates using experiment results, automatic model selection, and
exploration loops. "Try 10 ideas to improve metric X" is a HELIX workflow
loop that calls DDx comparison primitives — it is not a DDx execution mode.

## Problem Statement

**Current situation:**
- `ddx agent run` can dispatch to any harness and capture output/tokens/cost.
- Quorum mode runs multiple harnesses and checks consensus (pass/fail).
- Forge gives DDx full control of the agent loop with tool call logging.
- Beads define work. Commits capture verified results. But there is no
  structured way to compare harness outputs for the same prompt, replay a
  bead with a different model, or grade the quality of results.

**Pain points:**
- When evaluating whether a local model (via forge) can replace a cloud
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

## Requirements

### Functional

**Sandboxed comparison dispatch**
1. `ddx agent run --compare --harnesses=forge,claude --prompt task.md` runs
   each harness arm in an isolated environment and records a comparison.
2. Each arm runs in a temporary git worktree created from the current HEAD.
   Existing `resolveWorktree` infrastructure is reused.
3. After each arm completes, capture `git diff HEAD` in the worktree as the
   "effect diff" — the concrete artifact of what the agent changed.
4. For forge arms, also capture the full `ToolCallLog` (every read, write,
   edit, bash call with inputs and outputs).
5. For subprocess arms (codex, claude, opencode), capture stdout/stderr and
   the effect diff. Tool call detail is not available.
6. Temporary worktrees are cleaned up after the comparison unless
   `--keep-sandbox` is specified.
7. Arms run in parallel by default (reuses quorum parallelism). Sequential
   mode available via `--sequential` for resource-constrained environments.

**Side-effect capture**
8. The effect diff is captured as a unified diff string and stored in the
   comparison record alongside the text output.
9. For forge, the tool call log provides a complete audit trail: which files
   were read, what edits were made, what commands were run, what output they
   produced. This is richer than the diff alone.
10. Optionally run a test suite in each worktree after the agent completes
    (`--post-run "make test"`) and capture pass/fail as a quality signal.

**Grading primitive**
11. `ddx agent grade --comparison <id>` sends a comparison record to a
    grading harness and records the evaluation.
12. `ddx agent grade --comparison <id> --grader claude` specifies which
    harness performs the grading (default: highest-preference available).
13. The grading prompt is a standardized template that presents:
    - The original task prompt
    - Each arm's text output
    - Each arm's effect diff
    - (Optional) test results
14. The grader returns a structured evaluation per arm:
    ```json
    {
      "arm": "forge",
      "score": 7,
      "max_score": 10,
      "pass": true,
      "rationale": "Correct implementation but missed edge case in..."
    }
    ```
15. Grades are appended to the comparison record in the session log.
16. Custom grading rubrics can be provided via `--rubric <file>` to
    replace the default template. DDx provides the rubric loading
    mechanism; the rubric content is user/workflow-defined.

**Comparison reporting**
17. `ddx agent compare --list` shows recent comparison runs with summary.
18. `ddx agent compare --show <id>` displays the full comparison record
    including per-arm outputs, diffs, and grades.
19. `--format json` for machine-readable output (consumed by dun checks,
    HELIX evaluation gates, CI pipelines).

**Bead-close metadata** (foundation for replay)
20. `ddx bead close` accepts `--model`, `--harness`, `--tokens`, `--cost`
    flags and records them as structured evidence on the bead.
21. `ddx bead show` displays the agent metadata when present.
22. This metadata is the foundation for replay — without it, we can't
    reconstruct what model/harness produced a bead's result.

**Replay from bead**
23. `ddx agent replay <bead-id> --model <model> --harness <harness>` reads
    the bead's title, description, and acceptance criteria to reconstruct a
    prompt, dispatches to the specified model/harness in a sandbox worktree,
    runs `--post-run` evaluation, and reports the result.
24. Replay compares the new result against the original commit's diff when
    available (the bead's closing commit is discoverable via `git log`).
25. Replay is the key primitive for answering "what if we reran this bead
    with qwen3.5-27b instead of claude-opus?"

### Non-Functional

- **Isolation:** Worktree sandboxes must prevent cross-arm interference.
  Each arm sees a clean copy of HEAD.
- **Performance:** Parallel arm execution. Worktree creation is fast
  (git worktree add is <1s for typical repos).
- **Storage:** Comparison records include diffs and outputs which can be
  large. Use the same attachment-backed storage as session logs (FEAT-006).
- **Determinism:** For forge with the virtual provider, comparison runs
  are fully deterministic — enables CI-based prompt regression testing.

## Design Principles

### Platform primitive, not framework

DDx provides the mechanics:
- Run N harnesses on the same prompt in sandboxes
- Capture outputs and side effects
- Send results to a grading harness
- Record everything

DDx does **not** provide:
- Prompt optimization strategies
- Benchmark suites or leaderboards
- Model selection policies
- Grading rubric content (beyond a sensible default)

Workflow tools compose these primitives. HELIX might define: "Before
promoting a bead, run the implementation prompt through forge+claude,
require grade ≥8/10 on both arms." Dun might define a check:
"regression-test this prompt against the recorded baseline."

### Sandboxing strategy

Git worktrees are the natural sandbox for code-generating agents:
- Fast to create (copy-on-write on modern filesystems)
- Full git state (HEAD, index, config, hooks)
- Clean isolation (separate working directory)
- Cheap to clean up (`git worktree remove`)
- Diff capture is trivial (`git diff HEAD`)

For forge specifically, the tool sandbox is already built into the
`WorkDir` parameter — setting it to the worktree path is sufficient.
For subprocess harnesses, `WorkDirFlag` (codex: `-C`, opencode: `--dir`)
achieves the same.

No container or VM sandboxing is needed for comparison runs because:
- The threat model is "prevent cross-arm interference," not "prevent
  malicious code execution"
- Worktrees provide filesystem isolation
- Network isolation is out of scope (agents need API access)

If stronger isolation is needed (e.g., untrusted model output running
shell commands), that's a future concern addressed by forge's tool
permission layer or external sandbox tooling.

## CLI Interface

```bash
# Compare two harnesses on the same prompt
ddx agent run --compare --harnesses=forge,claude --prompt task.md

# Compare with per-arm model selection
ddx agent run --compare \
  --arm forge:qwen3.5-27b:forge-fast \
  --arm claude:claude-opus-4-6:claude-smart \
  --prompt task.md --sandbox

# Compare with post-run test
ddx agent run --compare --harnesses=forge,claude --prompt task.md \
  --post-run "cd cli && make test"

# Grade a comparison using claude as grader
ddx agent grade --comparison cmp-abc123 --grader claude

# Grade with custom rubric
ddx agent grade --comparison cmp-abc123 --rubric rubrics/code-quality.md

# List recent comparisons
ddx agent compare --list

# Show comparison detail
ddx agent compare --show cmp-abc123 --format json

# Replay a closed bead with a different model
ddx agent replay ddx-52d42ccb --model qwen3.5-27b --harness forge

# Replay with post-run verification
ddx agent replay ddx-52d42ccb --model qwen3.5-27b --harness forge \
  --post-run "cd cli && make test"

# Run a benchmark suite
ddx agent benchmark --suite benchmarks/implementation.json --output results.json
```

### Configuration

```yaml
# .ddx/config.yaml
agent:
  compare:
    default_grader: claude         # harness to use for grading
    keep_sandbox: false            # preserve worktrees after comparison
    parallel: true                 # run arms in parallel
    post_run: ""                   # command to run after each arm
```

## User Stories

### US-190: Developer Compares Local vs Cloud Model
**As a** developer evaluating whether a local model can handle a task class
**I want** to run the same prompt through forge (local) and claude (cloud)
**So that** I can see concrete differences in output quality and cost

**Acceptance Criteria:**
- Given I run `ddx agent run --compare --harnesses=forge,claude --prompt task.md`,
  then both harnesses receive the same prompt text
- Given both arms complete, then I see a comparison summary with per-arm
  output, tokens, cost, and duration
- Given I add `--keep-sandbox`, then I can inspect the worktree contents
  after the run

### US-191: Developer Grades Agent Outputs
**As a** developer assessing prompt quality
**I want** to have a smart model grade the outputs of a comparison run
**So that** I get a structured quality score without manual review

**Acceptance Criteria:**
- Given a completed comparison, when I run `ddx agent grade --comparison <id>`,
  then a grading harness evaluates each arm
- Given the grading completes, then each arm has a score, pass/fail, and
  rationale in the comparison record
- Given I specify `--rubric custom.md`, then the custom rubric replaces
  the default grading template

### US-192: CI Pipeline Runs Prompt Regression Test
**As a** CI system ensuring prompt quality
**I want** to compare agent outputs against a recorded baseline
**So that** prompt changes that degrade quality are caught before merge

**Acceptance Criteria:**
- Given a comparison with `--harnesses=forge` and the virtual provider
  baseline, when the forge output diverges significantly, then the
  comparison record reflects the difference
- Given `--format json` output, then CI can parse the comparison and
  fail the pipeline on grade regression

### US-193: Developer Captures Side Effects Safely
**As a** developer running comparison experiments
**I want** each agent arm to run in an isolated worktree
**So that** file changes from one arm don't contaminate the other or my
working tree

**Acceptance Criteria:**
- Given I run a comparison with two harnesses that both edit files, then
  each arm's changes are captured as separate diffs
- Given the comparison completes, then my original working tree is
  unchanged
- Given an arm fails mid-run, then its worktree is still cleaned up
  (unless `--keep-sandbox`)

### US-194: Developer Replays a Bead With a Different Model
**As a** developer evaluating local model capability
**I want** to rerun a previously-closed bead with a different model
**So that** I can compare the new model's output against the known-good result

**Acceptance Criteria:**
- Given a closed bead with agent metadata, when I run
  `ddx agent replay <bead-id> --model qwen3.5-27b --harness forge`,
  then the bead's prompt is reconstructed and dispatched to the model
- Given the replay completes in a sandbox, then I see the diff, build/test
  results, tokens, cost, and duration
- Given the original commit is discoverable, then the replay output shows
  a comparison of the new diff against the original

### US-195: Bead Captures Agent Provenance on Close
**As a** developer building a replay corpus
**I want** `ddx bead close` to record which model and harness produced the result
**So that** future replays know what the baseline was

**Acceptance Criteria:**
- Given I close a bead with `--model opus-4-6 --harness claude --tokens 1500 --cost 0.28`,
  then `ddx bead show` displays these as structured evidence
- Given a bead was closed without agent metadata, then replay still works
  but reports "baseline model unknown"

## Dependencies

- FEAT-006 (Agent Service) — harness registry, quorum parallelism, worktree
  support, session logging, comparison dispatch
- FEAT-014 (Token Awareness) — token/cost tracking per arm
- FEAT-012 (Git Awareness) — git worktree operations
- Forge library — embedded agent with tool call logging

## Out of Scope

- **Exec projection** — comparison is a peer to FEAT-010 exec, not a
  child. Do not model comparisons as exec runs.
- **Exploration loops** — "try 10 ideas to improve metric X" is a HELIX
  workflow loop that calls DDx comparison primitives, not a DDx execution
  mode.
- **Model selection policies** — HELIX decides which model for which task
  based on comparison results. DDx provides the data.
- **Prompt optimization** — automatic prompt rewriting is out of scope.
  DDx provides comparison and grading; HELIX provides the iteration loop.
- **Structured git trailers** — bead + session log already have the data.
  Don't duplicate into git commit metadata.
- Container or VM-level sandboxing
- Cross-repo or cross-project comparison
