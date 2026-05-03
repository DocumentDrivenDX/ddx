---
ddx:
  id: MET-001
  depends_on:
    - FEAT-010
    - SD-005
metric:
  schema_version: 1
  unit: ms
  direction: lower-is-better
  goal: 250
  budget: 400
  source: exec
  scope: per-attempt
---
# Metric: ddx test wall time

## What this measures

Wall-clock duration of `cd cli && go test ./...` on the reference runner,
measured by the `exec-ddx-test-walltime` execution definition. Excludes
build cache warm-up and post-test artifact upload. The value is the
wall-time delta between the runner reporting "test phase started" and
"test phase ended"; it does not include the Go build step that precedes
the test phase.

## Why it matters

Test wall time is the inner-loop cost of every contributor change. It
governs how often developers run the full suite locally and how long CI
holds up a merge. A budget violation reliably degrades the FEAT-010
execution feedback loop and the contributor experience SD-005 is
designed to support.

## How it is observed

`source: exec`. The `exec-ddx-test-walltime@1` `ddx exec` definition
runs the suite and emits `result.metric.value` in milliseconds. Each
attempt produces one row in the generic `exec-runs` collection (see
SD-005), tagged with `labels: ["artifact:MET-001"]` so `ddx metric`
queries can isolate it.

Inspect history with:

- `ddx metric history MET-001` — observed values in order
- `ddx metric show MET-001` — frontmatter, observing gates, partitions
- `ddx metric compare MET-001 <baseline>` — deterministic delta
- `ddx metric trend MET-001` — aggregated series

## Goal vs budget

- **Goal: 250 ms** — aspirational target for a healthy inner loop.
  Documentary only; missing goal does not gate landing.
- **Budget: 400 ms** — hard line. When a gate observes this metric via
  `ddx.execution.metric.metric_id`, the gate's `thresholds.ratchet` is
  authoritative for landing decisions. `ddx doc validate` warns when
  this `budget` and the observing ratchet drift apart.

## Notes

This is a canonical reference for `source: exec`. Plugin authors
introducing their own exec-emitted metrics may copy this artifact's
shape verbatim and substitute the unit, direction, observable, and
producer wiring.
