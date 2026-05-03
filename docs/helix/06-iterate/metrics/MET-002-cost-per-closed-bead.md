---
ddx:
  id: MET-002
  depends_on:
    - helix.prd
    - FEAT-016
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
sourced from a weekly cost report exported into the `exec-runs`
collection. The boundary is "all spend tied to attempts that produced
the closing commit"; it excludes spend on attempts against beads that
remain open at report time, and it excludes fixed-cost subscription
overhead that cannot be attributed per-bead.

## Why it matters

Tracks the unit economics of agent-driven development against the PRD
sustainability target. FEAT-016 process metrics aggregate bead
lifecycle data; this metric layers cost attribution on top so a reader
can see whether closing more beads is getting cheaper or more
expensive per unit.

## How it is observed

`source: external`. A weekly ingestion job (`ingest-cost-report`)
reads the upstream billing export, joins it against the bead store
(see FEAT-016), and writes one row per closed bead into the
`exec-runs` collection with:

- `labels: ["artifact:MET-002"]`
- `result.metric.value` in USD
- `result.metric.unit: "USD"`
- `partition` keyed by closing commit + bead ID

Cadence is weekly; backfills are written with the original close-time
timestamp, not ingestion time, so trend queries remain stable.

Inspect history with:

- `ddx metric history MET-002`
- `ddx metric show MET-002`
- `ddx metric trend MET-002`

## Goal vs budget

- **Goal: 0.50 USD** — aspirational target. Trend matters more than
  any single observation; a single noisy week should not drive a
  process change.
- **Budget:** intentionally omitted. This metric is informational
  rather than gating; it does not block landing and no observing gate
  is wired to its ID.

## Notes

This is a canonical reference for `source: external`. The defining
trait is that DDx itself never produces the value — an upstream
system does, and an ingestion pathway carries it into `exec-runs`.
Plugin authors introducing human-supplied or third-party-supplied
metrics may copy this artifact's shape verbatim and substitute the
unit, direction, ingestion pathway, and cadence.
