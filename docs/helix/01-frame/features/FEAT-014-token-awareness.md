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
- `ddx agent run` captures token counts from harness output, but the
  extraction is broken for both major harnesses:
  - **claude**: regex looks for "total tokens" text that claude never outputs.
    Zero tokens captured on every run.
  - **codex**: text-mode regex is fragile. Structured JSON output is available
    but not used.
  - **gemini**: no token pattern set at all.
- There is no `ddx agent usage` command to inspect historical consumption.
- No cost tracking despite claude providing `total_cost_usd` in JSON output.
- No way for a planner to select cheaper models when budget is tight.

**Desired outcome:** Accurate per-session token and cost capture for codex and
claude. A `ddx agent usage` command that summarizes consumption by harness,
model, and time window. Foundation for future budget-aware model selection.

## Spike Findings (ddx-f5dd5c45)

| Harness | Token Data Available | Effort |
|---------|---------------------|--------|
| codex | `--json` flag → `turn.completed` event with `usage.input_tokens`, `output_tokens` | Low |
| claude | `--output-format=json` → final JSON with `total_cost_usd`, full `usage` breakdown | Low |
| gemini | `--output-format=json` exists but untested; no live auth available | Medium |
| Provider APIs | OpenAI usage API works with same key; Anthropic needs admin key; Google has no programmatic API | Medium–Hard |

**Verdict:** Feasible with low effort for codex + claude. All aggregation is
local (session JSONL). No external API calls required for v1.

## Requirements

### Functional

**Fix token capture (pre-work)**
1. codex harness: add `--json` flag to exec args; parse `turn.completed`
   JSONL event for `usage.input_tokens` and `usage.output_tokens`
2. claude harness: add `--output-format=json` to args; parse final JSON
   object for `usage.input_tokens`, `usage.output_tokens`, `total_cost_usd`
3. gemini harness: investigate `--output-format=json` output when auth is
   available; implement token extraction if format is known

**Extend session log schema**
4. Add `input_tokens`, `output_tokens` fields to `SessionEntry` (replacing
   ambiguous single `tokens` field; keep `tokens` for backward compat)
5. Add `cost_usd` field to `SessionEntry` — populated when harness provides
   it (claude), estimated from pricing table otherwise
6. Backward compatible: existing session logs with only `tokens` field still
   parse correctly

**`ddx agent usage` command**
7. `ddx agent usage` — summary table: harness, sessions, input tokens,
   output tokens, estimated cost, avg duration
8. `--harness <name>` — filter to one harness
9. `--since <duration|date>` — time window start (e.g., `today`, `7d`,
   `2026-04-01`; default: `30d`)
10. `--format table|json|csv` — output format (default: table)
11. Reads `.ddx/agent-logs/sessions.jsonl` — no external API calls

**Future: budget-aware model selection (v2)**
12. `ddx agent usage --budget` — show remaining budget vs configured limit
13. Config: `agent.budget.monthly_usd` in `.ddx/config.yaml`
14. When budget exceeds threshold, `ddx agent run` warns or auto-downgrades
    model (e.g., o3-mini → gpt-4o-mini)

### Non-Functional

- **Performance:** `ddx agent usage` scans JSONL linearly. For typical logs
  (<10K sessions), this is <100ms. No index needed.
- **Accuracy:** Token counts from harness JSON output are authoritative.
  Cost estimates use a built-in pricing table that may drift — document this.
- **Backward compatibility:** Old session logs without new fields still load.
  New fields default to zero.

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

# Example output (table):
HARNESS  SESSIONS  INPUT TOKENS  OUTPUT TOKENS  EST. COST  AVG DURATION
codex          12       245,000         18,500     $4.87        45.2s
claude          8       180,000         12,000     $3.24        38.1s
TOTAL          20       425,000         30,500     $8.11        42.3s
```

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

## Dependencies

- FEAT-006 (Agent Service) — session logging, harness registry
- `.ddx/agent-logs/sessions.jsonl` — existing session log store

## Out of Scope

- Real-time rate limit monitoring (requires intercepting harness HTTP traffic)
- Provider billing API integration (requires separate admin keys)
- Automatic budget enforcement (v2)
- Gemini token capture (blocked on auth investigation)
