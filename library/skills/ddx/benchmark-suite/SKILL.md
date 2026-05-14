---
name: benchmark-suite
description: Run a repeatable prompt or task benchmark across a matrix of agent conditions, aggregate comparable outputs, and produce a concise report. Use when evaluating prompt changes, model/power choices, harness passthrough constraints, or regression behavior across a suite. Composes `ddx run` and the compare-prompts workflow; replaces benchmark behavior that should not live in core CLI routing.
---

# Benchmark Suite

A workflow skill for running the same benchmark suite across a matrix of
agent conditions and aggregating the results. This is a skill-level workflow:
DDx owns suite orchestration and evidence capture; Fizeau owns concrete route
selection and provider/model discovery.

Use this when one prompt comparison is too narrow and you need repeated cases:
prompt-regression sets, model/power evaluation, harness passthrough smoke
tests, or cost/latency comparisons across representative tasks.

## Inputs

Define a benchmark suite directory:

```text
benchmarks/<suite-name>/
├── suite.yaml
├── prompts/
│   ├── case-001.md
│   └── case-002.md
└── expected/
    ├── case-001.md
    └── case-002.md
```

`suite.yaml`:

```yaml
name: routing-boundary-smoke
cases:
  - id: case-001
    prompt: prompts/case-001.md
    expected: expected/case-001.md
  - id: case-002
    prompt: prompts/case-002.md
matrix:
  - label: default
    min_power: 10
  - label: top
    top_power: true
  - label: pinned-repro
    min_power: 10
    harness: codex
```

Only vary axes that answer the benchmark question. If the benchmark is about
power, keep prompts and passthrough constraints constant. If it is about a
prompt change, keep power and passthrough constraints constant.

## Workflow

### 1. Confirm the benchmark question

Write one sentence describing what the suite is measuring, for example:

```text
Does the revised review prompt catch routing-boundary regressions with equal or better accuracy at min_power=10?
```

Without a crisp question, the report becomes a pile of outputs rather than a
decision aid.

### 2. Prepare cases with an output contract

Each prompt must ask for comparable output. Prefer short contracts:

```markdown
## Output contract

Verdict: pass | fail

Findings:
- <finding or "none">

Score: <0-5>
```

Expected files are optional, but when present they should describe the
properties that make an answer acceptable. Do not require exact prose matches
unless the task is intentionally deterministic.

### 3. Dispatch the matrix

For each `(case, matrix arm)` pair, run one layer-1 invocation with `ddx run`.
Use power bounds for model quality. Treat `--harness`, `--provider`, and
`--model` as passthrough constraints only, useful for repros or controlled
comparisons.

```bash
OUT=.ddx/benchmarks/$(date +%Y%m%dT%H%M%S)-routing-boundary-smoke
mkdir -p "$OUT"

ddx run --min-power 10 --prompt benchmarks/routing/prompts/case-001.md \
  > "$OUT/case-001.default.md" \
  2> "$OUT/case-001.default.err"
```

Run arms in parallel when the environment can tolerate the load. Capture
stdout and stderr separately for every arm. Never drop failed arms from the
aggregate.

### 4. Grade each result

For every result, record:

- case id
- arm label and full invocation
- exit status
- verdict (`pass`, `fail`, `error`, or `not_graded`)
- score, if the output contract provides one
- latency, token, model, and cost signals when available
- short notes explaining failures or material differences

Use expected files as rubrics, not exact string snapshots, unless the suite is
explicitly for deterministic output.

### 5. Aggregate

Produce a report at:

```text
.ddx/benchmarks/<timestamp>-<suite>/report.md
```

Report structure:

```markdown
# Benchmark: <suite-name>

Question: <one sentence>

## Summary

| Arm | Pass | Fail | Error | Median score | Notes |
|-----|------|------|-------|--------------|-------|

## Case Results

| Case | Arm | Verdict | Score | Model | Duration | Notes |
|------|-----|---------|-------|-------|----------|-------|

## Recommendation

<ship / do not ship / needs follow-up, with reason>
```

Keep the recommendation tied to the benchmark question. If the suite cannot
answer the question, say that and list the missing evidence.

## Relationship To Other Skills

- `compare-prompts` handles one N-arm comparison. `benchmark-suite` repeats
  that shape across a suite of cases and aggregates across the whole matrix.
- `replay-bead` handles one prior bead attempt against a pinned baseline.
  `benchmark-suite` is for planned suites, not one-off replay.
- `adversarial-review` can supply the judging prompt when a benchmark needs an
  independent critic pass.

## Guardrails

- Do not add benchmark axes halfway through a run; start a new run instead.
- Do not compare arms that changed both prompt and power unless the suite is
  explicitly a full-matrix experiment.
- Do not interpret harness/provider/model as DDx routing decisions. They are
  passthrough constraints to Fizeau.
- Do not merge failed or missing arms into averages. Report them separately.
