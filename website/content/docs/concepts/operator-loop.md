---
title: Operator Loop
weight: 3
---

DDx is built around a continual-improvement loop for agentic software work:

```text
Plan -> Execute -> Measure -> Adapt
```

The loop is close to PDCA, ITIL continual improvement, and lean
build-measure-learn. DDx adapts that pattern to spec-driven work by making
specs the planning surface and execution evidence the measurement surface.

## Plan

Planning turns intent and evidence into a contract that can be executed.

In DDx, planning usually means:

- update governing specs and design artifacts
- define acceptance criteria and expected measurements
- create or refine beads
- wire dependencies, priority, scope, and non-scope

Planning is not separate from decomposition. In DDx, decomposition is how a
plan becomes executable.

## Execute

Execution runs bounded work against the bead queue.

The execution stack is:

- `ddx run` — one agent invocation
- `ddx try <bead>` — one bead attempt in an isolated worktree
- `ddx work` — a queue drain that iterates bead attempts

Each attempt starts from a narrow contract and a fresh context. The result is
captured as commits, run records, review output, preserved worktrees, and bead
events.

## Measure

Measurement checks whether the result satisfied the contract and what the run
taught the project.

Useful signals include:

- acceptance tests and build gates
- review verdicts
- run duration, token use, cost, and model
- failed-attempt evidence
- stale documents and dependency drift
- production or operator feedback

This is where DDx differs from many spec-driven tools. A spec-to-task pipeline
can produce code, but DDx also records the evidence needed to judge the result
and improve the next cycle.

## Adapt

Adaptation changes the system based on measurement.

That can mean:

- close the bead because the evidence passed
- revise the spec because implementation exposed a better requirement
- refine or split beads because the task was too broad
- update templates, skills, checks, or routing policy
- stop because the desired outcome is reached

Adaptation is the loop's control point. A measurement that does not change
specs, tasks, process, or direction is only a report.

## Where HELIX Fits

HELIX is one opinionated implementation of the DDx operator loop. Its
Frame/Design/Test work expands the Plan step. Build/Deploy execute bounded
tasks. Review, Iterate, Align, and Backfill measure and adapt the artifact
stack.

Other workflows can use the same loop with different phases. DDx provides the
substrate: specs, beads, runs, evidence, metrics, and staleness.
