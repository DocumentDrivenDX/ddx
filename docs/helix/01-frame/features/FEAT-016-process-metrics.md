---
ddx:
  id: FEAT-016
  depends_on:
    - helix.prd
    - FEAT-004
    - FEAT-014
---
# Feature: Process Metrics

**ID:** FEAT-016
**Status:** Not Started
**Priority:** P1
**Owner:** DDx Team

## Overview

DDx tracks process metrics — measures derived from existing stores (beads,
agent sessions) that help teams understand the economics and efficiency of
their workflow. Unlike FEAT-010 operational metrics (which you *run* against
code), process metrics are *computed* from data DDx already collects.

## Problem Statement

**Current situation:** Bead lifecycle data (creation time, close time, reopen
count) and agent session data (token counts, cost, duration) exist in
`.ddx/beads.jsonl` and `.ddx/agent-logs/sessions.jsonl`. But there's no way to
query aggregate process measures like "how much did this bead cost" or "what's
our rework rate this week."

**Pain points:**
- No visibility into the cost of individual beads or features
- No way to detect rework patterns (high reopen rates, many revision cycles)
- Teams can't answer "are we getting faster?" without manual data extraction
- Token/cost data is per-session, not aggregated per-bead or per-feature

**Desired outcome:** A `ddx metrics` surface that computes and reports process
measures from existing DDx data without requiring new data collection.

The canonical solution design is [`SD-016`](../../02-design/solution-designs/SD-016-process-metrics.md).

## Boundary with FEAT-010

| Concern | FEAT-010 (Operational) | FEAT-016 (Process) |
|---------|----------------------|-------------------|
| What it measures | Code behavior (sort speed, API latency) | Workflow behavior (bead cost, rework rate) |
| Data source | Execution runs against definitions | Beads, agent sessions, git history |
| How it runs | `ddx exec run` executes a command | Computed from existing stores |
| Example | "p99 latency is 42ms" | "FEAT-007 cost $12.40 in tokens" |

## Requirements

### Functional

1. **Bead lifecycle metrics** — compute from beads.jsonl:
   - Time from creation to close (cycle time)
   - Reopen count per bead
   - Beads closed per time period
   - Time in each status (open, in_progress, closed)

2. **Cost metrics** — join bead and agent session data:
   - Total tokens per bead (sum sessions correlated by bead_id)
   - Estimated cost per bead (from session cost_usd)
   - Total tokens per feature (aggregate beads by spec-id)
   - Cost per time period

3. **Rework metrics** — derived:
   - Reopen rate (reopened beads / total closed)
   - Revision count (updates after first close)
   - Beads with >N revisions flagged

4. **CLI surface:**
   - `ddx metrics summary` — dashboard of key process measures
   - `ddx metrics cost [--bead <id>] [--feature <spec-id>] [--since <period>]`
   - `ddx metrics cycle-time [--since <period>]`
   - `ddx metrics rework [--since <period>]`
   - All commands support `--json` output

5. **Server endpoints** (read-only, FEAT-002 integration):
   - `GET /api/metrics/summary`
   - `GET /api/metrics/cost?bead=<id>&feature=<spec-id>&since=<period>`

### Non-Functional

- **No new data collection** — all metrics derived from existing stores
- **Performance** — summary computation <1s for repos with 500 beads
- **Incremental** — if a bead has no correlated sessions, cost is "unknown"
  not an error

## User Stories

### US-160: Developer Checks Feature Cost
**As a** developer reviewing a completed feature
**I want** to see how many tokens and dollars it consumed
**So that** I can evaluate the economics of agent-assisted development

**Acceptance Criteria:**
- Given beads with spec-id FEAT-007 and correlated agent sessions, when I
  run `ddx metrics cost --feature FEAT-007`, then I see total tokens, cost,
  and per-bead breakdown

### US-161: Team Lead Reviews Rework Rate
**As a** team lead planning the next sprint
**I want** to see which features had the most rework
**So that** I can identify areas needing better specs or design

**Acceptance Criteria:**
- Given beads that were reopened, when I run `ddx metrics rework --since 7d`,
  then I see reopen rate and the specific beads that were reopened

## Dependencies

- FEAT-004 (beads) — data source
- FEAT-014 (token awareness) — agent session cost data

## Out of Scope

- Operational metrics (code performance, API health) — that's FEAT-010
- Custom metric definitions — plugins can build on this data via hooks
- Historical trend storage — metrics are computed on demand from raw data
