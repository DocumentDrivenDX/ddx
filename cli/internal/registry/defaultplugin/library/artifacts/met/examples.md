# MET-* examples

Two reference shapes plugin authors can copy when authoring metric
artifacts. Both conform to the v1 schema in FEAT-005 and the runtime
contract in TD-005.

## Example 1: `source: exec`, goal + budget

A metric emitted by a `ddx exec` definition. The metric has both an
aspirational goal and a machine-checkable budget. The observing gate's
`thresholds.ratchet` remains authoritative for landing decisions.

```markdown
---
ddx:
  id: MET-001
  depends_on: [FEAT-001, SD-001]
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

Wall-clock duration of `cd cli && go test ./...` on the reference
runner, measured by the `exec-ddx-test-walltime` execution definition.
Excludes build cache warm-up and post-test artifact upload.

## Why it matters

Test wall time is the inner-loop cost of every contributor change. A
budget violation reliably degrades the FEAT-001 contributor experience.

## How it is observed

`ddx exec` definition `exec-ddx-test-walltime@1` runs the suite and
emits `result.metric.value` in milliseconds. History accumulates in the
`exec-runs` collection.

## Goal vs budget

- Goal: 250 ms — aspirational
- Budget: 400 ms — gate `thresholds.ratchet` on the observing
  definition is authoritative; `ddx doc validate` warns if the two
  drift apart.
```

## Example 2: `source: external`, goal only

A metric ingested from an upstream system. No budget is set because the
value is informational rather than gating. `goal` documents the target.

```markdown
---
ddx:
  id: MET-002
  depends_on: [helix.prd]
metric:
  schema_version: 1
  unit: USD
  direction: lower-is-better
  goal: 0.50
  source: external
  scope: per-bead
---
# Metric: cost per closed bead

## What this measures

Average inference + tooling cost attributable to closing one bead,
sourced from the weekly cost report exported into `exec-runs` via the
`ingest-cost-report` external pipeline.

## Why it matters

Tracks the unit economics of agent-driven development against the PRD
sustainability target.

## How it is observed

External ingestion: the weekly cost report writes one row per closed
bead into the `exec-runs` collection with
`labels: ["artifact:MET-002"]` and `result.metric.value` in USD.

## Goal vs budget

- Goal: 0.50 USD — aspirational; trend matters more than any single
  observation. No budget is set; this metric does not gate landing.
```

## What to copy

- The frontmatter shape is the contract. The prose section names are
  recommendations from `template.md`; reorder if a project convention
  prefers something else, but keep the section meanings intact.
- For `source: exec`, you must wire a `ddx exec` definition that
  references the `MET-*` ID (or file a follow-up bead to do so).
- For `source: external`, document the ingestion pathway and cadence
  in the prose so a reader can verify the rows in `exec-runs`.
