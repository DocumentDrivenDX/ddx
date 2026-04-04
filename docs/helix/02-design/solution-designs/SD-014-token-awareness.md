---
ddx:
  id: SD-014
  depends_on:
    - FEAT-014
    - FEAT-006
    - ADR-001
---
# Solution Design: Agent Token Awareness

## Overview

Fix token/cost capture from codex and claude harnesses by switching to
structured JSON output, extend the session log schema, and add a `ddx agent
usage` aggregation command.

## Changes by Component

### 1. Harness Registry (`internal/agent/registry.go`)

**codex:** Add `"--json"` to `Args` slice (after `exec --ephemeral`). The
codex `exec --json` mode emits JSONL to stdout. Each line is a JSON object
with a `type` field. The `turn.completed` event contains
`usage.input_tokens`, `usage.cached_input_tokens`, `usage.output_tokens`.

**claude:** Add `"--output-format", "json"` to `Args` slice. In JSON mode,
claude emits a single JSON object to stdout with:
- `usage.input_tokens`, `usage.output_tokens`
- `usage.cache_creation_input_tokens`, `usage.cache_read_input_tokens`
- `total_cost_usd`

Both changes only affect exec/print modes already in use. No behavioral change
to the agent invocation itself.

### 2. Token Extraction (`internal/agent/runner.go`)

Replace regex-based `ExtractTokens()` with a structured parser:

```go
type UsageData struct {
    InputTokens  int
    OutputTokens int
    CostUSD      float64
}

func ExtractUsage(harness Harness, output string) UsageData
```

For codex: scan JSONL lines for `"type":"turn.completed"`, unmarshal usage.
For claude: parse the entire output as JSON, extract `usage` and
`total_cost_usd`.
Fallback: if structured parsing fails, try the legacy regex (backward compat
for old harness versions).

### 3. Session Log Schema (`internal/agent/types.go`)

```go
type SessionEntry struct {
    // existing fields unchanged...
    Tokens      int     `json:"tokens"`                  // kept for backward compat
    InputTokens  int    `json:"input_tokens,omitempty"`  // new
    OutputTokens int    `json:"output_tokens,omitempty"` // new
    CostUSD      float64 `json:"cost_usd,omitempty"`     // new
}
```

`Tokens` continues to be `input + output` for backward compat. New fields
are `omitempty` so old logs without them parse cleanly.

### 4. Usage Command (`cmd/agent_cmd.go`)

New subcommand `ddx agent usage` that:
1. Reads `.ddx/agent-logs/sessions.jsonl` line by line
2. Filters by `--since` and `--harness`
3. Aggregates: sum input/output tokens, sum cost, count sessions, avg duration
4. Renders as table (default), JSON, or CSV

Time parsing for `--since`: accept ISO dates, relative durations (`7d`, `30d`,
`today`), or git-style refs. Keep it simple — `time.Parse` + a few shortcuts.

### 5. Pricing Table (`internal/agent/pricing.go`)

Built-in map of model → per-token pricing for cost estimation when the harness
doesn't provide cost directly:

```go
var pricing = map[string]ModelPricing{
    "o3-mini":                  {InputPer1M: 1.10, OutputPer1M: 4.40},
    "gpt-4o":                   {InputPer1M: 2.50, OutputPer1M: 10.00},
    "gpt-5.4":                  {InputPer1M: 2.00, OutputPer1M: 8.00},
    "claude-sonnet-4-20250514": {InputPer1M: 3.00, OutputPer1M: 15.00},
    "claude-opus-4-20250514":   {InputPer1M: 15.00, OutputPer1M: 75.00},
}
```

If the harness provides `cost_usd` (claude does), use that. Otherwise
estimate from the pricing table. If the model isn't in the table, show "N/A".

## Test Strategy

See TP-014 for detailed test cases. Key verification:

1. Unit tests for `ExtractUsage()` with fixture JSON from each harness
2. Unit test for session log backward compatibility (old format → new struct)
3. Unit test for usage aggregation (filter, sum, render)
4. Integration: `ddx agent usage` against a fixture sessions.jsonl
