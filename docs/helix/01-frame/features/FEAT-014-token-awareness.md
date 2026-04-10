---
ddx:
  id: FEAT-014
  depends_on:
    - helix.prd
    - FEAT-006
---
# Feature: Agent Usage Awareness and Routing Signals

**ID:** FEAT-014
**Status:** In Progress
**Priority:** P1
**Owner:** DDx Team

## Overview

DDx's agent wrapper should understand provider-native usage and routing signals
across harnesses so that every `ddx agent run` can make an informed decision
about which harness to use based on request fit, availability, cost,
performance, and current quota/headroom when known.

## Problem Statement

**Current situation:**
- DDx can extract per-invocation token and cost data from subprocess output for
  codex, claude, and opencode.
- DDx currently aggregates usage through its own session-log model, but that
  duplicates external provider storage and suppresses native persistence for
  codex and claude by default.
- Codex native session JSONL already carries `token_count` and `rate_limits`
  data when persistence is enabled, which is more useful for routing than DDx's
  current duplicate session ledger.
- Claude exposes historical usage via `~/.claude/stats-cache.json`, but the
  best current-quota source is still unresolved. DDx should exhaust stable
  non-PTY options before accepting PTY automation.
- The embedded `ddx-agent` runtime is gaining its own session logging and OTEL
  telemetry. DDx should consume that telemetry minimally rather than re-owning
  the runtime's logs.
- DDx does not yet have a normalized routing-signal model that combines
  capability, availability, quota/headroom, cost, and DDx-observed performance
  into one preflight harness decision.

**Desired outcome:** Every `ddx agent run` evaluates candidate harnesses using
normalized routing signals:
- request fit (profile/model/effort/permissions)
- provider availability and authentication
- current quota/headroom where known
- cost estimate or cost class
- DDx-observed performance and reliability

External harnesses preserve native persistence. DDx owns only the minimal
activity and performance metadata needed for routing and provenance.

## Spike Findings

### Per-Invocation Data (implemented)

| Harness | Token Data | Cost Data | Status |
|---------|-----------|-----------|--------|
| codex | `turn.completed` JSONL: `input_tokens`, `cached_input_tokens`, `output_tokens` | Not reported | Working |
| claude | JSON envelope: `input_tokens`, `output_tokens`, `cache_read/creation_input_tokens` | `total_cost_usd` | Working |
| opencode | JSON envelope: `input_tokens`, `output_tokens` (if present) | `total_cost_usd` (if present) | Working |
| gemini | Untested; no live auth available | Unknown | Blocked |

### Signal-Source Matrix (re-reviewed)

| Source family | Current quota/headroom | Historical usage | Freshness | Scope | Stable enough for preflight routing? | Notes |
|--------------|------------------------|------------------|-----------|-------|-------------------------------------|-------|
| Codex native session JSONL / local state | Yes, when persistence is enabled and `token_count.rate_limits` is present | Yes, recent usage totals from session JSONL and local state | Near-real-time once Codex writes the session log; stale if the file is missing or unreadable | Machine-local, authenticated Codex session/account state | Yes, preferred live source for Codex routing when readable | Treat missing persistence or unreadable logs as `unknown`; do not rely on inline PTY scraping in the routing path |
| Claude `~/.claude/stats-cache.json` | No stable non-PTY source confirmed yet; surface as `unknown` | Yes, account-wide daily activity, daily tokens by model, cumulative model usage, session counts, and hour-of-day distribution | Cached and delayed; freshness depends on the last stats-cache write | Machine-local cache with account-wide historical usage | Yes for historical usage, no for current quota | Use this source for history and load balancing only; current quota remains `unknown` until a stable live source is confirmed |
| Claude runtime / statusline / SDK / hook artifacts | None confirmed | None confirmed | N/A unless a trustworthy source is discovered | N/A until validated | No, not yet | Investigate these before any PTY fallback; if live probing becomes necessary, feed an async snapshot cache rather than routing-time terminal scraping |
| embedded `ddx-agent` telemetry | Not a provider quota source | Yes, DDx-owned invocation activity, runtime metrics, and session references | Per invocation or per write | Local workspace / install / session | Yes for DDx-observed performance and provenance; no for provider quota routing | Consume references and derived metrics only; do not duplicate provider transcript storage or provider quota state |

Interpretation:

- `unknown` means no trustworthy live source exists, the source is stale, or the data is unreadable.
- Unknown quota/headroom stays viable for routing with reduced confidence; it is not fabricated into `ok` or `blocked`.
- If a current quota source requires active probing, the probe must populate an async snapshot history and never block routing on synchronous PTY scraping.

### Research Priorities

1. **Codex adapter from native logs** — use native session JSONL as the first
   source of truth for current headroom and recent totals.
2. **Claude non-PTY quota research** — investigate statusline, SDK, hook, or
   other local runtime sources before accepting any PTY-based solution.
3. **Minimal DDx-owned routing metrics** — define the smallest DDx state needed
   to compare performance and reliability across harnesses without duplicating
   provider transcripts.
4. **Snapshot history and subscription-cost proxy** — when a current quota
   source requires active probing, DDx should checkpoint quota snapshots over
   time, map them to native usage accumulation, and estimate subscription burn
   pressure well enough to balance providers intelligently.

## Requirements

### Functional

**Per-invocation extraction (complete)**
1. codex harness: `--json` flag, `turn.completed` JSONL parsing
2. claude harness: `--output-format=json`, JSON envelope parsing with
   `total_cost_usd`
3. opencode harness: `run --format json`, JSON envelope parsing
4. `input_tokens`, `output_tokens`, `cost_usd` fields on DDx invocation
   activity rows
5. `ddx agent usage` command with harness/time-window/machine-readable output
6. gemini harness: investigate `--output-format=json` output when auth is
   available; implement token extraction if format is known

**Provider-native signal ingestion**
7. **Native persistence preserved** — DDx must not pass suppressive flags such
   as codex `--ephemeral` or claude `--no-session-persistence` by default for
   external harnesses.
8. **Codex native signal adapter** — read current quota/headroom and recent
   usage from native Codex session JSONL when persistence is enabled.
9. **Claude historical usage adapter** — read `~/.claude/stats-cache.json` to
   obtain account-wide daily token consumption by model, session counts, and
   activity patterns.
10. **Claude current quota spike** — identify a stable non-PTY current-quota
    source if one exists. If no stable source is found, DDx must surface quota
    as `unknown` rather than inventing it. PTY automation is explicitly a
    fallback of last resort, not the MVP path, and must feed an asynchronous
    snapshot/sampling path rather than synchronous routing-time scraping.
11. **Signal freshness/cache policy** — cache provider-native signal reads with
    explicit freshness semantics so `ddx agent run` can distinguish fresh from
    stale state.

**Normalized routing signals**
12. **Routing signal model** — DDx normalizes every candidate's:
    - capability/request fit
    - availability/authentication state
    - quota/headroom state (`ok`, `blocked`, `unknown`)
    - cost estimate or cost class
    - DDx-observed performance metrics
    - freshness / confidence
13. **Minimal DDx-owned metrics** — DDx stores only compact outcome samples,
    async quota snapshots, and derived burn summaries. Outcome samples may
    record recent success/failure, latency, and last observed token/cost
    values when available; DDx does not store provider transcripts or native
    session bodies as routing inputs.
14. **Snapshot history for live quota sources** — when DDx depends on an
    actively probed quota source, it checkpoints time-stamped quota snapshots
    asynchronously and relates them to native usage accumulation to build a
    history of usage plus quota availability.
15. **Subscription-cost proxy** — DDx derives a relative subscription burn
    estimate from quota snapshots and usage deltas well enough to balance
    providers under subscription plans where direct per-token billing is
    unavailable.
16. **Routing integration** — harness selection uses the normalized signal
    model together with requested profile, model, effort, permission mode, and
    explicit harness override semantics.
17. **Operator visibility** — `ddx agent doctor --routing` and `ddx agent
    usage` consume the normalized signal model and report source freshness.

**Budgeting and throttling**
18. **Deferred scope** — budget passthrough, automatic throttling, and pacing
    policy remain follow-on work after the signal-source spikes establish the
    right acquisition model.

**Always-on execute-bead runtime metrics**
19. `ddx agent execute-bead` must capture built-in runtime metrics for every
    iteration, independent of any graph-authored execution documents. These are
    DDx runtime facts, not substitutes for project-authored metric docs.
    Minimum fields (where available):
    - `harness`, `model`, `session_id`
    - `elapsed_ms`
    - `input_tokens`, `output_tokens`, `total_tokens`
    - `cost_usd` (`-1` when unknown, `0` for local models)
    - `base_rev`
    - `result_rev`

### Non-Functional

- **Performance:** provider-native signal reads and DDx metrics lookups should
  add only modest preflight overhead. Dispatch-time routing should remain fast
  enough to avoid noticeable startup delay.
- **Accuracy:** Token counts from harness JSON output are authoritative.
  Provider-native stores reflect the provider's own accounting and should be
  preferred where available.
- **Zero fabrication:** DDx must never fabricate current quota/headroom. When a
  source is unavailable or stale, DDx surfaces `unknown`.
- **Graceful degradation:** If provider-native stores are missing, unreadable,
  or in an unexpected format, DDx falls back to request fit, static cost, and
  DDx-observed performance metrics. Never fail an invocation solely because a
  signal source could not be read.
- **Minimal ownership:** DDx owns only the metadata needed for routing,
  provenance, and operator visibility. It must not become a shadow transcript
  store for external providers.

## CLI Interface

```bash
# Routing state with freshness and quota/headroom where known
ddx agent doctor --routing

# Usage summary derived from provider-native sources + DDx-owned metrics
ddx agent usage

# Filter to one harness
ddx agent usage --harness claude

# Specific time window
ddx agent usage --since 7d
ddx agent usage --since 2026-04-01

# Machine-readable
ddx agent usage --format json
```

## User Stories

### US-140: Developer Checks Agent Usage and Routing Signals
**As a** developer managing agent usage
**I want** to see provider-native usage signals and DDx-observed routing metrics
**So that** I can understand what DDx is routing on

**Acceptance Criteria:**
- Given I run `ddx agent usage`, then I see per-harness usage/cost where
  available, plus DDx-observed runtime metrics
- Given I run `ddx agent usage --since today`, then only today's windows are
  counted
- Given I run `ddx agent usage --format json`, then output is valid JSON

### US-141: DDx Routes Using Current Availability Signals
**As** the DDx agent router
**I want** to combine request fit, availability, quota/headroom, cost, and
performance signals
**So that** I choose the best viable harness for each run

**Acceptance Criteria:**
- Given Codex native session logs are available, when DDx evaluates codex, then
  current quota/headroom comes from those native logs rather than PTY
  automation
- Given Claude has no stable current-quota source yet, when DDx evaluates
  claude, then current quota is surfaced as `unknown` rather than fabricated
- Given quota is explicitly blocked for a candidate, then that candidate is
  rejected
- Given quota is unknown for a candidate, then the candidate remains viable but
  is ranked with reduced confidence rather than hard-rejected

### US-142: Developer Sees Signal Freshness in Doctor Output
**As a** developer debugging harness selection
**I want** `ddx agent doctor --routing` to show where each signal came from and
how fresh it is
**So that** I can trust or question DDx's decision with evidence

**Acceptance Criteria:**
- Given I run `ddx agent doctor --routing`, then each harness reports current
  availability state, quota/headroom state, and last-checked freshness
- Given a signal came from a provider-native source, then doctor output
  identifies that source
- Given a signal source could not be read, then doctor output reports `unknown`
  with an explanatory note instead of omitting the field

### US-145: Execute-bead Runtime Metrics Are Captured Automatically
**As** a developer reviewing bead execution history
**I want** runtime metrics recorded for every execute-bead iteration without
manual instrumentation
**So that** iterations are comparable and cost is always visible

**Acceptance Criteria:**
- Given `ddx agent execute-bead` runs with a harness that exposes token and cost
  data, when the iteration completes, then the run record contains `harness`,
  `model`, `session_id`, `elapsed_ms`, `input_tokens`, `output_tokens`,
  `total_tokens`, and `cost_usd`
- Given runtime token and cost data is captured, when the iteration commit
  summary is written, then the summary includes harness, model, total tokens,
  cost, base revision, and result revision

## Dependencies

- FEAT-006 (Agent Service) — harness registry, invocation activity capture
- provider-native local stores such as `~/.codex/` and `~/.claude/`
- embedded `ddx-agent` runtime telemetry and session references

## Implementation Strategy

### Phase 1 — Signal-source spikes
- Inventory provider-native signal sources and freshness semantics
- Define the minimal DDx-owned routing metrics schema and retention rules in
  SD-015
- Resolve whether Claude has a stable non-PTY current-quota source

### Phase 2 — Preserve native persistence and add adapters
- Remove DDx defaults that suppress codex/claude native persistence
- Implement Codex native signal adapter
- Implement Claude historical usage adapter

### Phase 3 — Normalize routing signals
- Build the shared routing signal model
- Feed doctor/usage/routing from that model
- Treat unknown quota as unknown, not as blocked or OK
- If a live quota source requires active probing, persist async snapshot history
  rather than blocking routing on synchronous PTY gymnastics

### Phase 4 — Follow-on policy work
- Budget passthrough
- Throttling / pacing
- Optional PTY fallback only if non-PTY Claude quota sourcing proves impossible

## Out of Scope

- Provider billing API integration
- Intercepting harness HTTP traffic for real-time rate-limit headers
- Gemini token capture (blocked on auth investigation)
- Cross-machine usage aggregation (see FEAT-013)
- Immediate budget enforcement and adaptive throttling policy
- Prompt design and task strategy beyond harness selection
