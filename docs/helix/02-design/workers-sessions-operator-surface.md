---
ddx:
  id: TD-011
  depends_on:
    - FEAT-006
    - FEAT-008
---
# Workers and Sessions Operator Surface

## Purpose

Workers and Sessions describe different parts of the same execution story.
Workers are long-lived execute-loop processes that drain a project's bead
queue. Sessions are immutable records of individual agent invocations produced
by those workers or by ad-hoc runs.

The web UI should make that distinction visible. Operators use Workers to
start, stop, and inspect queue drains. They use Sessions to review history,
cost, tokens, outcomes, and the detailed prompt/response evidence for one
agent run.

## Controls

The Workers page exposes the lifecycle controls that already exist in the CLI
and server runtime:

- Start worker: calls a GraphQL `startWorker` mutation that dispatches an
  `execute-loop` worker through the same WorkerManager path used by `ddx work`
  / server-managed queue drains.
- Stop: visible only for workers in `running` state. The mutation calls the
  same `WorkerManager.Stop` path reached by `ddx agent workers stop`, so claim
  release, graceful cancellation, process-group termination, and stopped-state
  persistence stay centralized.

The proposed "cancel current bead without killing the loop" control is not
implemented in this pass because there is no separate CLI/runtime primitive for
abandoning only the in-flight bead while preserving the loop process. Adding it
requires an execute-loop semantic change and remains out of scope for this
operator-surface pass.

## Navigation and Audit

Workers and Sessions link to each other:

- Workers page header links to Recent sessions.
- Sessions page header links back to Workers.
- Worker detail lists sessions whose `workerId` matches the worker.
- Expanded session rows link back to the producing worker when `workerId` is
  present.

Every lifecycle action performed through the server records a timestamped
worker lifecycle event with the server's single-operator actor identifier.
Those events are surfaced on the worker detail page beside the live response
and log tail, so an operator can see who started or stopped the process and
when.
