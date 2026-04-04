---
ddx:
  id: TP-014
  depends_on:
    - FEAT-014
    - SD-014
---
# Test Plan: Agent Token Awareness

## Objective

Verify that token/cost capture works for codex and claude, session logs
include the new fields, backward compatibility is preserved, and the usage
command aggregates correctly.

## Test Cases

### TC-001: ExtractUsage parses codex JSON output
**Given** codex `--json` output containing a `turn.completed` JSONL line with
`usage.input_tokens=1000, output_tokens=200`
**When** `ExtractUsage(codexHarness, output)` is called
**Then** returns `UsageData{InputTokens: 1000, OutputTokens: 200}`

### TC-002: ExtractUsage parses claude JSON output
**Given** claude `--output-format=json` output with
`usage.input_tokens=5000, output_tokens=800, total_cost_usd=0.045`
**When** `ExtractUsage(claudeHarness, output)` is called
**Then** returns `UsageData{InputTokens: 5000, OutputTokens: 800, CostUSD: 0.045}`

### TC-003: ExtractUsage falls back on malformed output
**Given** harness output that doesn't contain valid JSON
**When** `ExtractUsage()` is called
**Then** returns zero-value `UsageData` (no panic, no error)

### TC-004: SessionEntry backward compatibility
**Given** a JSONL line with only `"tokens": 1200` (no input/output/cost)
**When** unmarshaled into `SessionEntry`
**Then** `Tokens=1200`, `InputTokens=0`, `OutputTokens=0`, `CostUSD=0`

### TC-005: SessionEntry new fields round-trip
**Given** a `SessionEntry` with `InputTokens=500, OutputTokens=100, CostUSD=0.02`
**When** marshaled to JSON and back
**Then** all fields preserved; `Tokens` = `InputTokens + OutputTokens`

### TC-006: Usage command aggregates by harness
**Given** a sessions.jsonl with 3 codex sessions and 2 claude sessions
**When** `ddx agent usage --format json` is run
**Then** output contains two entries with correct sums per harness

### TC-007: Usage command filters by time
**Given** sessions spanning multiple days
**When** `ddx agent usage --since today --format json` is run
**Then** only today's sessions are included

### TC-008: Usage command filters by harness
**Given** sessions from codex and claude
**When** `ddx agent usage --harness claude --format json` is run
**Then** only claude sessions appear

### TC-009: Cost estimation from pricing table
**Given** a codex session with `input_tokens=10000, output_tokens=500` and
model `o3-mini`
**When** cost is estimated
**Then** cost = `(10000 * 1.10 + 500 * 4.40) / 1_000_000` = `$0.0132`

### TC-010: Codex harness args include --json
**Given** the codex harness from the registry
**Then** `Args` contains `"--json"`

### TC-011: Claude harness args include --output-format json
**Given** the claude harness from the registry
**Then** `Args` contains `"--output-format"` followed by `"json"`

## Implementation

Tests in `cli/internal/agent/agent_test.go` (extraction, schema) and
`cli/cmd/agent_usage_test.go` (command integration).

## Pass Criteria

All 11 test cases pass. `go test ./internal/agent/... ./cmd/...` green.
