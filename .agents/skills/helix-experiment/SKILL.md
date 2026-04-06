---
name: helix-experiment
description: Autonomous experiment loop for metric-driven optimization. Use when asked to "run autoresearch", "optimize X in a loop", or "experiment with X".
argument-hint: "[issue-id|goal]"
disable-model-invocation: true
---

# Experiment

Autonomous metric-optimization loop: try ideas, keep what works, discard what
doesn't, repeat until converged or interrupted.

## When to Use

- Performance tuning (test speed, latency, throughput)
- Size optimization (bundle size, binary size, memory usage)
- ML training optimization (loss, accuracy, bits-per-byte)
- Any measurable optimization target on existing, tested code

## Steps

1. **Start or resume** — invoke the experiment action. On first run, it sets up
   the session (branch, benchmark script, session doc, baseline). On subsequent
   runs, it reads existing session state and continues.

2. **Loop** — after each iteration, re-invoke the experiment action. Do not
   stop unless:
   - `EXPERIMENT_STATUS: CONVERGED` — the metric has stabilized
   - `EXPERIMENT_STATUS: NO_IMPROVEMENT` — 5 consecutive non-improvements
   - The user interrupts or redirects

3. **Steer** — incorporate user messages into the next hypothesis. Finish the
   current iteration before changing direction.

4. **Close** — when done, invoke with `--close` to squash-merge the experiment
   branch, update ratchet floors, and close the issue.

## Constraints

- Only modify files declared in scope
- All existing tests must pass after every edit — tests are the specification
- Prefer simpler solutions over marginal complexity gains
- Do not change test expectations

## References

- Action prompt: `workflows/actions/experiment.md`
- Session doc template: `workflows/templates/autoresearch-session.md`
- Worklog template: `workflows/templates/autoresearch-worklog.md`
- Metric definition template: `workflows/templates/metric-definition.yaml`
- Ratchet integration: `workflows/ratchets.md`
- Autoresearch ecosystem: compatible with pi-autoresearch JSONL protocol
