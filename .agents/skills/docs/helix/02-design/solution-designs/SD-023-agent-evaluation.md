---
ddx:
  id: SD-023
  depends_on:
    - FEAT-019
    - FEAT-006
    - FEAT-004
    - FEAT-012
    - FEAT-014
    - SD-006
    - TD-006
    - TD-010
---
# Solution Design: Agent Evaluation and Prompt Comparison

## Purpose

FEAT-019 adds an evaluation layer above DDx's existing agent execution
surface. The layer answers three operator questions:

- What changed when the same prompt ran through multiple harness/model arms?
- Which arm produced the highest-quality result under an explicit rubric?
- What would another harness or model have done for a preserved bead attempt?

This design defines the architecture for comparison isolation, grading,
benchmark aggregation, and replay. It does not replace `ddx agent
execute-bead`; it evaluates preserved executions and comparison runs produced
by existing agent primitives.

## Scope

- Isolated comparison arms for side-effecting agent runs
- `ComparisonRecord` and `ComparisonGrade` semantics
- Grading rubric loading, prompt construction, parsing, and score attachment
- Replay source-of-truth across execution bundles, session logs, and bead
  metadata
- Relationship between comparison, benchmark, quorum, grade, and replay
  commands

Out of scope:

- Model-selection policy and escalation rules
- Prompt optimization loops
- Container or VM isolation
- Cross-project evaluation
- Replacing provider-native transcript stores

## Existing Command Surface

The implementation already contains several FEAT-019-adjacent primitives:

- `ddx agent run --compare`: immediate comparison dispatch. It runs N arms
  against the same prompt and emits a `ComparisonRecord`.
- `ddx agent benchmark`: suite runner. It expands suite arms/prompts into
  repeated comparison dispatches and aggregates statistics.
- `ddx agent run --quorum`: consensus gate. It reuses multi-harness dispatch
  ideas but returns pass/fail consensus, not graded comparison evidence.
- `ddx agent replay`: bead replay entry point. This design tightens its
  evidence lookup and diff-comparison contract.
- `ddx agent grade`: target CLI for rubric grades on a persisted comparison.
  The grading library path already defines the record and parser shape.
- `ddx agent compare`: target inspection surface for persisted comparison
  records. Until persistence lands, `run --compare` is the executable
  comparison surface.

The design keeps these surfaces separate because they answer different
questions. `quorum` answers "did enough agents agree or pass?" Comparison and
benchmark answer "what did each arm do and cost?" Grading answers "which result
best satisfies this rubric?" Replay answers "what would this other harness have
done from the same evidence-backed starting point?"

## Architecture Overview

```text
prompt source
    |
    v
comparison dispatcher
    |
    +-- arm A sandbox worktree
    |       --> harness/model --> output + tool log + diff + post-run
    |
    +-- arm B sandbox worktree
    |       --> harness/model --> output + tool log + diff + post-run
    |
    v
ComparisonRecord
    |
    +-- grade pipeline --> ComparisonGrade[]
    |
    +-- benchmark aggregation --> per-arm suite statistics
    |
    +-- compare inspection --> list/show/json
```

Replay starts from a bead or execution attempt instead of an arbitrary prompt:

```text
bead id
    |
    v
execution bundle + session log lookup
    |
    v
reconstruct prompt + base revision + original result diff
    |
    v
single-arm sandbox run
    |
    v
ReplayComparison
```

The comparison dispatcher is the shared execution core. Benchmark and replay
compose it; quorum remains a sibling that shares harness dispatch and
parallelism but intentionally does not create comparison grades.

## Comparison Arm Isolation

### Isolation Boundary

Each side-effecting arm runs in its own git worktree rooted at the same base
revision. The worktree is the only filesystem boundary FEAT-019 requires.

Default comparison behavior:

1. Resolve the base repository root from the requested `WorkDir`.
2. Generate a comparison ID such as `cmp-<hex>`.
3. For each arm, create a detached worktree under
   `.worktrees/<comparison-id>-<arm-label>/` from the same base revision.
4. Dispatch the harness with its working directory set to that worktree.
5. Capture stdout/stderr-equivalent output from the harness result.
6. Capture side effects with `git diff HEAD` plus untracked files.
7. Run the optional post-run command in the same worktree and capture pass/fail.
8. Remove all comparison worktrees unless `--keep-sandbox` was requested.

The base revision must be recorded on the comparison record when persistence
lands. For `run --compare`, the base is the current `HEAD` at dispatch time.
For replay, the base is derived from the original attempt evidence.

### Arm Identity

An arm is identified by:

- `harness`
- resolved `model`
- optional tier/profile or explicit model pin
- stable display label
- sandbox path while retained

Labels must be stable within a comparison. When the same harness appears
multiple times with different models, callers should supply labels like
`agent-fast` and `agent-smart`; otherwise report rendering cannot disambiguate
grades and benchmark summaries.

### Parallelism

Comparison arms run concurrently by default because each arm has an independent
worktree. Sequential mode is an execution-policy flag, not a different record
shape. Both modes produce the same `ComparisonRecord`; only timestamps and
durations differ.

### Side-Effect Capture

Every arm records two categories of evidence:

- Common evidence for all harnesses: output, exit code, duration, token/cost
  fields when available, post-run result, and git diff.
- DDx Agent evidence: typed tool-call log entries for reads, writes, edits,
  and bash calls. Subprocess harnesses do not expose this detail, so their
  `tool_calls` field stays absent rather than an empty audit trail.

The effect diff is the canonical side-effect summary for cross-harness
comparison because every harness can produce it. Tool-call logs are richer
diagnostic evidence, not the comparison key.

### Failure Semantics

Comparison dispatch is best-effort across arms:

- Worktree creation failure marks that arm failed and does not block other
  arms whose worktrees were created.
- Harness failure records `exit_code` and `error` on the arm.
- Post-run failure records `post_run_ok=false` and captured output; it does not
  erase the harness result.
- Cleanup failures are warnings in the inspection output and should include
  retained sandbox paths for manual removal.

The comparison command exits non-zero only when dispatch itself cannot produce
a record, such as invalid input, no arms, unreadable prompt, or repository root
resolution failure.

## Data Model

### ComparisonRecord

`ComparisonRecord` is the durable unit for comparison, benchmark, and grading.

Required fields:

- `id`
- `timestamp`
- `base_rev`
- `prompt`
- `prompt_source`
- `arms[]`

Optional fields:

- `suite` and `prompt_id` when produced by benchmark
- `correlation` for workflow identifiers such as `bead_id`, `attempt_id`, or
  `spec_id`
- `source_execution` when comparison consumes preserved execute-bead attempts
- `grades[]`
- `storage_refs` for large prompt, output, diff, and tool-log attachments

### ComparisonArm

Required fields:

- `label`
- `harness`
- `exit_code`
- `duration_ms`

Optional fields:

- `model`
- `tier` or `profile`
- `output`
- `diff`
- `tool_calls`
- `post_run_out`
- `post_run_ok`
- `tokens`
- `input_tokens`
- `output_tokens`
- `cost_usd`
- `error`
- `sandbox_path` when `--keep-sandbox` is used

### ComparisonGrade

Required fields:

- `arm`
- `score`
- `max_score`
- `pass`
- `rationale`

Optional fields:

- `grader_harness`
- `grader_model`
- `rubric_id`
- `rubric_version`
- `graded_at`
- `raw_response_ref`

Scores are rubric-local. DDx may summarize averages within a suite only when
all grades use the same rubric identity and `max_score`.

## Persistence

Comparison records use the same attachment-backed pattern as agent sessions:
small metadata is stored in a JSONL row, while large prompts, outputs, diffs,
tool logs, and raw grader responses are stored as referenced attachments.

The persisted collection is logically `agent-comparisons`. The physical store
may remain file-backed under `.ddx/` until a bead-backed collection is
available. Readers must treat the logical schema as authoritative and avoid
depending on the current file path.

Persistence is required for:

- `ddx agent compare --list`
- `ddx agent compare --show <id>`
- `ddx agent grade --comparison <id>`
- benchmark output history
- CI gates that consume prior comparison IDs

`ddx agent run --compare --json` may still emit an ephemeral record without
storing it when explicitly requested by tests or scripts, but the default
operator workflow should persist records so grading and inspection can follow.

## Grading Pipeline

### Rubric Resolution

Grading is deterministic at the DDx orchestration layer even when the grader
model is probabilistic. DDx resolves exactly one rubric before invoking the
grader:

1. If `--rubric <file>` is provided, load that file as the complete rubric.
2. Else if the comparison record has a suite rubric reference, load it.
3. Else use the built-in default rubric for correctness, completeness, and
   implementation quality.

DDx owns rubric loading and provenance. Workflow tools own rubric content.

### Prompt Construction

The grading prompt includes:

- Rubric text and required JSON schema
- Original task prompt
- Per-arm label, harness, model, status, tokens, cost, and duration
- Per-arm output
- Per-arm diff
- Post-run result and output when present
- Tool-call summary for DDx Agent arms when present

Large fields should be attached or truncated by explicit policy. Truncation
must be called out in the prompt so the grader knows evidence is incomplete.

### Structured Response

The grader must return JSON in this shape:

```json
{
  "arms": [
    {
      "arm": "agent-smart",
      "score": 8,
      "max_score": 10,
      "pass": true,
      "rationale": "Correct implementation with minor test coverage gaps."
    }
  ]
}
```

The parser may tolerate non-JSON preamble by extracting the first object, but
the saved `raw_response_ref` must preserve the original response for audit.

### Attachment and Mutation

Grading appends to the comparison record; it does not rewrite arm evidence.
If a record already has grades for the same `rubric_id` and grader, DDx keeps
the prior grades and appends a new grading event with a later `graded_at`
timestamp. Consumers pick the latest grade by default and can request history.

Malformed grader output fails the grade command without corrupting the
comparison record. Harness failure records a grading error event and leaves
existing grades untouched.

### Pass/Fail

The default pass threshold is `score >= 7` when `max_score` is `10`. Custom
rubrics may specify another pass rule, but the grader must still return the
boolean `pass` for each arm. DDx does not reinterpret score thresholds after
the fact; it reports the grader's structured decision.

## Benchmark Architecture

`ddx agent benchmark` is a batch wrapper around comparison dispatch.

Suite input:

- `name`
- `version`
- `arms[]`
- `prompts[]`
- optional `sandbox`
- optional `post_run`
- optional `timeout`
- optional rubric reference

For each prompt, the benchmark runner builds `CompareOptions` from suite arms
and calls the comparison dispatcher. The benchmark result stores every
comparison record plus an aggregate summary:

- completed arm count
- failed arm count
- total tokens
- total cost
- average duration
- average score when a single rubric was applied

Benchmark does not own a separate execution engine. Any fix to comparison
isolation, diff capture, model resolution, or post-run behavior must flow
through benchmark automatically.

## Replay Source of Truth

Replay has stricter provenance requirements than an ad hoc comparison because
it claims to recreate the inputs to a prior bead attempt.

### Source Precedence

Replay reconstructs evidence in this order:

1. `.ddx/executions/<attempt-id>/` execution bundle, when the bead or session
   links to an attempt ID.
2. Agent session log entry linked by bead `session_id`.
3. Provider-native session reference linked from the DDx session entry.
4. Bead title, description, and acceptance criteria as degraded fallback.

The execution bundle is the best source for DDx execute-bead attempts because
it is the preserved workflow artifact: manifest, prompt files, result files,
base revision, result revision, hidden iteration ref, and session-log path
travel together. The session log is the best source for harness metadata:
harness, model, token counts, cost, native session ID, and transcript pointers.

Neither source supersedes the other. Replay joins them:

- Use the execution bundle for base/result refs and immutable prompt artifact
  when present.
- Use the session log for harness/model/cost metadata and prompt transcript
  when the bundle does not include a prompt body.
- Use bead prose only when both execution and session evidence are missing.

### Prompt Reconstruction

For preserved execute-bead iterations, replay uses the exact prompt artifact
stored in the execution bundle. This avoids rebuilding a prompt from today's
bead state after labels, dependencies, instructions, or acceptance criteria
may have changed.

For non-execute-bead agent runs, replay uses the prompt stored or referenced by
the linked session log.

Fallback to bead prose must print "baseline session unknown" and mark the
result `degraded_prompt=true` in JSON output.

### Base Revision

Default replay answers: "what would this harness have done then?"

Base selection:

1. Prefer execution bundle `base_rev`.
2. Else use the parent of bead `closing_commit_sha`.
3. Else use current `HEAD` and mark the baseline as degraded.

`--at-head` explicitly switches the question to: "what would this harness do
today?" In that mode replay uses current `HEAD` while still reporting the
original source evidence for context.

Tracker-only close commits must not masquerade as implementation baselines. If
the only closing commit is a metadata-only tracker update, replay should omit
the original diff comparison and report that the governed implementation
baseline is unknown.

### Original Diff

Replay compares the new sandbox diff with the original result diff:

1. Prefer execution bundle result diff when present.
2. Else compute `git diff <base_rev> <result_rev>` when both refs are known.
3. Else compute `git diff <closing_commit_sha>^ <closing_commit_sha>` when the
   close commit is the implementation commit.
4. Else report no original diff.

The replay result is a comparison-style record with one new arm plus original
baseline metadata. It can be graded using the same grade pipeline by treating
the original result as a synthetic arm labeled `baseline`.

## Relationship to Execute-Bead Iterations

`ddx agent execute-bead` remains the canonical bead execution workflow. FEAT-019
evaluation consumes preserved attempts; it does not create a separate
evaluation-specific execution path.

When an operator wants to try multiple approaches for one bead, the workflow
tool should:

1. Run `ddx agent execute-bead <id> --no-merge` multiple times from the same
   base revision.
2. Preserve each attempt under its execution bundle and hidden iteration ref.
3. Build a comparison record whose arms point at those preserved attempts.
4. Grade the resulting comparison.
5. Let workflow policy decide whether to land, retry, or escalate.

The comparison record stores references to source attempts. It does not copy or
redefine execute-bead evidence. Provenance inspection must trace back to the
originating session ID, attempt ID, base revision, result revision, and hidden
iteration ref.

## CLI Contracts

### Compare

```bash
ddx agent run --compare --harnesses agent,claude --prompt task.md
ddx agent run --compare \
  --arm agent:gpt-5.4:agent-smart \
  --arm claude:opus:claude-smart \
  --prompt task.md
ddx agent compare --list
ddx agent compare --show cmp-abc123 --format json
```

`run --compare` creates records. `compare --list` and `compare --show` inspect
persisted records. A future `compare --from-executions` may assemble records
from preserved execute-bead attempts without rerunning agents.

### Grade

```bash
ddx agent grade --comparison cmp-abc123
ddx agent grade --comparison cmp-abc123 --grader claude --rubric rubrics/code.md
```

The grade command mutates only the comparison record's grading events.

### Benchmark

```bash
ddx agent benchmark --suite benchmarks/implementation.json --output results.json
```

Benchmark output includes full comparison records, not only summary rows.

### Quorum

```bash
ddx agent run --quorum majority --harnesses codex,claude --prompt review.md
```

Quorum does not persist comparison records by default because it is a gate, not
an evaluation corpus. If a workflow needs graded quorum evidence, it should run
comparison plus grade and then apply its own policy.

### Replay

```bash
ddx agent replay ddx-52d42ccb --model gpt-5.4 --harness agent
ddx agent replay ddx-52d42ccb --model gpt-5.4 --harness agent --at-head
```

Replay emits JSON with source evidence flags so CI and reviewers can
distinguish exact replay from degraded fallback.

## Error Handling

- Prompt file missing: command fails before creating arms.
- No harnesses or arms: command fails before creating a comparison ID.
- Worktree creation fails for one arm: arm records failure; other arms
  continue.
- Harness unavailable: arm records failure with service error.
- Post-run command fails: arm records `post_run_ok=false`; command still
  returns record.
- Grader output is malformed: grade command fails; record evidence is
  unchanged.
- Comparison ID missing: grade/show command fails with not found.
- Replay evidence missing: replay uses the next fallback and marks
  degradation.
- Base revision missing: replay uses `HEAD` and marks degradation unless
  `--at-head` was explicit.

## Security and Privacy

Comparison and replay records may contain proprietary code, prompts, diffs,
tool outputs, and model responses. They are repository-local by default and
must follow the same redaction and retention controls as agent sessions.

Rubric files are local input. DDx does not fetch rubrics from the network.

Worktree sandboxes prevent cross-arm filesystem interference, not malicious
code execution. Stronger isolation belongs to DDx Agent tool permissions or
external sandboxing.

## Observability

Every persisted comparison, grade, benchmark, and replay should include:

- start and end timestamps
- harness and model per arm
- base revision
- prompt source
- session IDs produced by each arm
- token and cost fields when available
- post-run command and status
- evidence degradation flags

Process metrics can aggregate benchmark and comparison costs after the records
are persisted, but FEAT-019 does not define a new metrics engine.

## Validation

The concrete validation matrix is TP-019. The design requires coverage for:

- worktree creation and cleanup
- arm isolation
- diff and untracked-file capture
- DDx Agent tool-call capture
- post-run pass/fail recording
- comparison record schema
- grade prompt construction and JSON parsing
- custom rubric loading
- malformed grader response handling
- benchmark suite expansion and summary aggregation
- replay prompt/source precedence
- replay base revision selection
- degraded replay fallback

## Implementation Order

1. Stabilize `ComparisonRecord` persistence and `ddx agent compare --list/show`.
2. Ensure `run --compare` records `base_rev`, prompt source, and arm labels.
3. Complete grading CLI and append-only grade events.
4. Update benchmark to persist or emit full comparison records with suite
   provenance.
5. Tighten replay evidence lookup to join execution bundles and session logs.
6. Add replay baseline diff comparison and JSON degradation flags.
7. Add CI-oriented fixtures using virtual providers and temp git repositories.

## Risks

- Diffs and outputs become too large for JSONL rows: store large bodies as
  attachments and keep only refs in metadata.
- Grader scores are mistaken for objective truth: persist rubric identity and
  rationale; workflows decide policy.
- Replay reconstructs today's prompt instead of original prompt: prefer
  execution bundle prompt artifact and session prompt over bead prose.
- Benchmark hides per-prompt failures in aggregate stats: store full
  comparison records and summarize from them.
- Quorum and grading semantics blur: keep quorum as a pass/fail dispatch gate;
  use comparison plus grade for quality scoring.
