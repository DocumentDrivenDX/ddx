---
ddx:
  id: MET-NNN
  depends_on: []
metric:
  schema_version: 1
  unit: <unit>                  # ms | bytes | USD | count | ratio | ...
  direction: lower-is-better    # lower-is-better | higher-is-better
  goal: <number>                # optional aspirational target
  budget: <number>              # optional machine-checkable hard line
  source: exec                  # exec | external
  scope: per-attempt            # per-attempt | per-bead | per-feature | global
---
# Metric: <Short Name>

## What this measures

One paragraph describing the observable, the system boundary, and what a
reader is meant to infer from a single value. Be specific about what is
included and what is excluded.

## Why it matters

Tie the metric back to the governing artifact(s) listed in `ddx.depends_on`.
Explain the decision the metric supports — for example, gating a release,
catching a regression, or guiding a tradeoff.

## How it is observed

Describe the producer of the value:

- **`source: exec`** — name the `ddx exec` definition that emits this
  metric and the field path it populates (e.g. `result.metric.value`).
- **`source: external`** — name the upstream system or report, the
  ingestion pathway, and the cadence.

## Goal vs budget

- **Goal (`<number> <unit>`)** — aspirational target. Documentary only.
- **Budget (`<number> <unit>`)** — hard line. When a gate observes this
  metric via `ddx.execution.metric.metric_id`, the gate's
  `thresholds.ratchet` is authoritative for landing decisions. `ddx doc
  validate` warns when budget and the observing ratchet disagree.

## History and projection

Metric history lives in the generic `exec-runs` collection (see TD-005).
Read it via:

- `ddx metric history MET-NNN` — observed values in order
- `ddx metric show MET-NNN`    — frontmatter, observing gates, partitions
- `ddx metric compare MET-NNN <baseline>` — deterministic delta (refuses
  to mix units)
- `ddx metric trend MET-NNN`   — aggregated series (refuses to mix units)

## Notes

Optional. Capture caveats: known noise sources, sampling assumptions,
unit migrations, or links to follow-on work in the MET v2 backlog.
