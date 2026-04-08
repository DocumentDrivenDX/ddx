---
ddx:
  id: FEAT-014
  depends_on:
    - helix.prd
    - FEAT-006
---
# Feature: Agent Token Awareness and Usage Tracking

**ID:** FEAT-014
**Status:** In Progress
**Priority:** P1
**Owner:** DDx Team

## Overview

DDx's agent wrapper should understand token consumption across harnesses so
that operators can monitor spend, agents can make intelligent model selection
based on budget, and the system degrades gracefully as limits approach.

## Problem Statement

**Current situation:**
- Token capture and cost tracking are working for codex, claude, and opencode
  (all three harnesses at parity as of v0.5.1).
- `ddx agent usage` provides per-harness aggregation with time windows.
- Claude provides rich per-invocation data (tokens, cost, cache stats, model
  usage with context window) and a historical stats file
  (`~/.claude/stats-cache.json`).
- Codex provides per-invocation token counts but no cost or account limits.
  Account-level rate limits are only accessible via TUI `/status` (PTY
  scraping, as demonstrated by steipete/CodexBar).
- **No self-throttling exists.** Subscription users (Claude Max, Codex Pro)
  have no way to automatically pace agent dispatch to avoid exhausting session
  headroom. Manual budget configuration is impractical for subscription plans
  where limits are opaque and change over time.

**Desired outcome:** Automatic self-throttling for subscription plans that
reads provider signals (stats files, per-invocation usage, rate patterns)
to pace dispatch. For API-key users, pass through provider budget mechanisms
(`--max-budget-usd`). No manual budget configuration required for the common
subscription case.

## Spike Findings (ddx-f5dd5c45, updated ddx-v0.5.1)

### Per-Invocation Data (implemented)

| Harness | Token Data | Cost Data | Status |
|---------|-----------|-----------|--------|
| codex | `turn.completed` JSONL: `input_tokens`, `cached_input_tokens`, `output_tokens` | Not reported | **Working** |
| claude | JSON envelope: `input_tokens`, `output_tokens`, `cache_read/creation_input_tokens` | `total_cost_usd` | **Working** |
| opencode | JSON envelope: `input_tokens`, `output_tokens` (if present) | `total_cost_usd` (if present) | **Working** |
| gemini | Untested; no live auth available | Unknown | Blocked |

### Provider-Side Usage Sources (new research)

| Source | claude | codex | opencode |
|--------|--------|-------|----------|
| **Local stats file** | `~/.claude/stats-cache.json` — daily activity, daily tokens by model, cumulative model usage, session counts, hour-of-day distribution | None known | None known |
| **Account rate limits** | Not exposed programmatically | TUI `/status` only — requires PTY scraping (steipete/CodexBar approach) | Not exposed |
| **Budget passthrough** | `--max-budget-usd` per-session cap | None | None |
| **Per-invocation model metadata** | `modelUsage` block: per-model `contextWindow`, `maxOutputTokens`, `costUSD` | None | None |

### CodexBar Prior Art (steipete/CodexBar)

CodexBar demonstrates three data acquisition strategies relevant to DDx:

1. **PTY scraping** — spawns CLI in a pseudo-terminal, sends `/status`,
   parses ANSI-stripped output for "Credits:", "5h limit", "Weekly limit"
   percentages and reset times. Primary mechanism for codex rate limits.
2. **OAuth credential reuse** — reads Claude CLI OAuth tokens from macOS
   Keychain; calls API directly for usage data.
3. **Local log reading** — reads JSONL session logs from both CLIs for
   cost-over-time calculations.

**Verdict:** For subscription users, the most practical path is reading
provider stats files (claude) and tracking DDx's own session log (all
harnesses). PTY scraping for codex `/status` is feasible but fragile.
API-key users get `--max-budget-usd` passthrough for claude.

## Requirements

### Functional

**Token capture and session logging (complete)**
1. ~~codex harness: `--json` flag, `turn.completed` JSONL parsing~~ ✓
2. ~~claude harness: `--output-format=json`, JSON envelope parsing with
   `total_cost_usd`~~ ✓
3. ~~opencode harness: `run --format json`, JSON envelope parsing~~ ✓
4. ~~`input_tokens`, `output_tokens`, `cost_usd` fields on `SessionEntry`~~ ✓
5. ~~`ddx agent usage` command with `--harness`, `--since`, `--format`~~ ✓
6. gemini harness: investigate `--output-format=json` output when auth is
   available; implement token extraction if format is known

**Provider usage ingestion**
7. **Claude stats reader** — read `~/.claude/stats-cache.json` to obtain
   account-wide daily token consumption by model, session counts, and
   activity patterns. Expose via `ddx agent usage --provider`.
8. **Codex usage probe (stretch)** — optionally spawn codex in a PTY, send
   `/status`, parse rate-limit percentages and reset times. Mark as
   experimental; degrade gracefully if parsing fails.
9. **Provider data cache** — cache provider stats reads with a short TTL
   (e.g., 60s) so multiple rapid `ddx agent run` invocations don't re-read
   or re-probe.

**Self-throttling for subscription plans**
10. **Pre-flight usage check** — before each `ddx agent run`, sum recent
    token consumption from `sessions.jsonl` within a sliding window. If
    consumption exceeds a threshold, warn or block.
11. **Subscription-aware defaults** — detect subscription mode (Claude Max,
    Codex Pro) from provider signals (stats file presence, OAuth auth type)
    and apply appropriate default thresholds without manual config.
12. **Adaptive pacing** — when approaching limits, automatically insert
    delays between dispatches or suggest model downgrade. The algorithm:
    ```
    recent_usage = sum(sessions where timestamp > now - window)
    utilization  = recent_usage / estimated_limit
    if utilization > 0.8: warn
    if utilization > 0.95: block or delay
    ```
13. **Headroom estimation** — for claude, combine per-invocation
    `modelUsage.contextWindow` with `stats-cache.json` daily totals to
    estimate remaining headroom. For codex, rely on session-log-based
    extrapolation.
14. **Override escape hatch** — `ddx agent run --no-throttle` bypasses
    the pre-flight check for cases where the operator knows better.

**Budget passthrough for API-key users**
15. **Claude `--max-budget-usd`** — expose as `ddx agent run --budget <usd>`
    and pass through to claude's `--max-budget-usd` flag.
16. **Config-based budget** — `agent.budget.max_cost_usd_per_day` in
    `.ddx/config.yaml` for API-key users who want hard limits.
17. When budget is exhausted, `ddx agent run` returns a clear error with
    current spend and configured limit.

**Always-on execute-bead runtime metrics**
18. `ddx agent execute-bead` must capture built-in runtime metrics for every
    iteration, independent of any graph-authored execution documents. These are
    DDx runtime facts, not substitutes for project-authored metric docs.
    Minimum fields (where available):
    - `harness`, `model`, `session_id`
    - `elapsed_ms` (wall-clock duration of the agent run)
    - `input_tokens`, `output_tokens`, `total_tokens`
    - `cost_usd` (`-1` when unknown, `0` for local models)
    - `base_rev` (git revision the execution started from)
    - `result_rev` (git revision of the landed or preserved iteration commit)

### Non-Functional

- **Performance:** `ddx agent usage` scans JSONL linearly. For typical logs
  (<10K sessions), this is <100ms. No index needed. Pre-flight throttle check
  must add <50ms to invocation startup.
- **Accuracy:** Token counts from harness JSON output are authoritative.
  Cost estimates use a built-in pricing table that may drift — document this.
  Provider stats files (`stats-cache.json`) reflect the provider's own
  accounting and should be preferred over DDx estimates where available.
- **Backward compatibility:** Old session logs without new fields still load.
  New fields default to zero.
- **Zero-config for subscriptions:** Subscription users (Claude Max, Codex
  Pro) should get useful throttling behavior without any manual budget
  configuration. The system detects subscription mode and applies sensible
  defaults.
- **Graceful degradation:** If provider stats files are missing, unreadable,
  or in an unexpected format, throttling falls back to session-log-only
  analysis. Never fail an invocation due to stats-reading errors.

## CLI Interface

```bash
# Summary for last 30 days (default)
ddx agent usage

# Filter to one harness
ddx agent usage --harness claude

# Specific time window
ddx agent usage --since 7d
ddx agent usage --since 2026-04-01

# Machine-readable
ddx agent usage --format json

# Show provider-level stats (reads ~/.claude/stats-cache.json etc.)
ddx agent usage --provider

# Show budget status (API-key users with configured budget)
ddx agent usage --budget

# Example output (table):
HARNESS   SESSIONS  INPUT TOKENS  OUTPUT TOKENS  EST. COST  AVG DURATION
codex           12       245,000         18,500     $4.87        45.2s
claude           8       180,000         12,000     $3.24        38.1s
opencode         3        45,000          3,200     $0.95        22.4s
TOTAL           23       470,000         33,700     $9.06        38.6s

# Throttle override
ddx agent run --no-throttle --harness codex --prompt task.md

# Per-session budget passthrough (claude only)
ddx agent run --budget 2.00 --harness claude --prompt task.md
```

### Configuration

```yaml
# .ddx/config.yaml — throttling (optional, for API-key users)
agent:
  budget:
    max_cost_usd_per_day: 25.00    # hard limit for API-key users
  throttle:
    enabled: true                   # default: true
    warn_threshold: 0.8             # warn at 80% utilization
    block_threshold: 0.95           # block at 95% utilization
    window: 1h                      # sliding window for rate calculation
```

> **Subscription users** (Claude Max, Codex Pro) do not need to configure
> budgets. DDx detects subscription mode from provider signals and applies
> adaptive throttling automatically.

## User Stories

### US-140: Developer Checks Agent Spend
**As a** developer managing API costs
**I want** to see how many tokens and dollars my agent invocations consumed
**So that** I can track spend and adjust my workflow

**Acceptance Criteria:**
- Given I run `ddx agent usage`, then I see a table with per-harness totals
  for sessions, tokens (input/output), estimated cost, and avg duration
- Given I run `ddx agent usage --since today`, then only today's sessions
  are counted
- Given I run `ddx agent usage --format json`, then output is valid JSON

### US-142: Subscription User Gets Automatic Throttling
**As a** Claude Max / Codex Pro subscriber running multiple agent sessions
**I want** DDx to automatically pace my agent dispatch
**So that** I don't exhaust my session headroom and get locked out

**Acceptance Criteria:**
- Given I am a Claude Max subscriber, when DDx detects
  `~/.claude/stats-cache.json`, then it reads recent daily token consumption
  without requiring any budget configuration
- Given my recent usage exceeds 80% of estimated headroom, when I run
  `ddx agent run`, then I see a warning with current utilization
- Given my recent usage exceeds 95% of estimated headroom, when I run
  `ddx agent run`, then the invocation is blocked with a clear message and
  estimated reset time
- Given I pass `--no-throttle`, then the pre-flight check is bypassed

### US-143: API-Key User Sets Budget Cap
**As an** API-key user paying per token
**I want** to set a daily cost limit and have DDx enforce it
**So that** I don't accidentally overspend

**Acceptance Criteria:**
- Given I configure `agent.budget.max_cost_usd_per_day: 10.00`, when my
  daily spend reaches $10, then `ddx agent run` refuses with current spend
- Given I run `ddx agent run --budget 2.00` with the claude harness, then
  `--max-budget-usd 2.00` is passed through to the claude CLI
- Given I run `ddx agent usage --budget`, then I see remaining budget vs
  configured limit

### US-144: Developer Views Provider-Level Usage
**As a** developer monitoring account-wide consumption
**I want** `ddx agent usage --provider` to show data from provider stats files
**So that** I see the full picture, not just DDx-dispatched sessions

**Acceptance Criteria:**
- Given `~/.claude/stats-cache.json` exists, when I run
  `ddx agent usage --provider`, then I see daily token totals from the
  provider's own accounting
- Given the stats file is missing, then the command shows DDx session data
  only with a note that provider stats are unavailable

### US-141: Token Capture Works for All Active Harnesses
**As** the DDx agent system
**I want** to accurately capture token counts from every agent invocation
**So that** usage aggregation is reliable

**Acceptance Criteria:**
- Given I run `ddx agent run --harness codex`, then `input_tokens` and
  `output_tokens` are captured in the session log (non-zero)
- Given I run `ddx agent run --harness claude`, then `input_tokens`,
  `output_tokens`, and `cost_usd` are captured in the session log
- Given I read an old session log without `input_tokens`, then it loads
  without error (fields default to zero)

### US-145: Execute-bead Runtime Metrics Are Captured Automatically
**As** a developer reviewing bead execution history
**I want** runtime metrics recorded for every execute-bead iteration without manual instrumentation
**So that** iterations are comparable and cost is always visible

**Acceptance Criteria:**
- Given `ddx agent execute-bead` runs with a harness that exposes token and cost data (e.g., claude, codex), when the iteration completes, then the run record contains `harness`, `model`, `session_id`, `elapsed_ms`, `input_tokens`, `output_tokens`, `total_tokens`, and `cost_usd` — with `cost_usd` set to `0` for local models and `-1` when the harness does not report cost.
- Given runtime token and cost data is captured, when the iteration commit summary is written (landed or preserved under a hidden ref), then the summary includes harness, model, total tokens, cost, base revision, and result revision so post-hoc comparison requires only the summary — not opening session attachments.

## Dependencies

- FEAT-006 (Agent Service) — session logging, harness registry
- `.ddx/agent-logs/sessions.jsonl` — existing session log store

## Implementation Strategy

### Phase 1 — Provider stats ingestion (next)
Read `~/.claude/stats-cache.json` as a supplementary usage source. Expose
via `ddx agent usage --provider`. No throttling yet — just visibility.

### Phase 2 — Pre-flight throttle check
Add `CheckBudget()` to `Runner.Run()`. For subscription users, derive
thresholds from provider stats patterns. For API-key users, read configured
budget. Warn at 80%, block at 95%.

### Phase 3 — Codex rate-limit probe (experimental)
Optionally probe codex `/status` via PTY. Cache results. Mark experimental.
Degrade to session-log-only if probe fails.

### Phase 4 — Adaptive pacing
Insert delays between rapid dispatches when approaching limits. Suggest
model downgrade (e.g., opus → sonnet) when headroom is low.

## Out of Scope

- Provider billing API integration (requires separate admin keys beyond
  what CLI tools provide)
- Intercepting harness HTTP traffic for real-time rate-limit headers
- Gemini token capture (blocked on auth investigation)
- Cross-machine usage aggregation (see FEAT-013)
- **Prompt design and model selection strategy** — DDx captures token/cost data; workflow tools and plugins decide which model to use for which task and how to structure prompts
- **Budget policy and spend governance** — DDx provides usage data and throttling primitives; organizational budget policy and approval workflows are out of scope
