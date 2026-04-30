---
ddx:
  id: TP-014
  depends_on:
    - FEAT-014
    - FEAT-006
    - TD-006
---
# Test Plan: Agent Usage Awareness and Routing Signals

## Objective

Verify that DDx can build trustworthy routing inputs from provider-native and
DDx-owned signals: per-invocation extraction, provider-native quota/usage
adapters, minimal DDx routing metrics, backward compatibility of activity
records, and doctor/usage command behavior.

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

### TC-003: Codex native adapter parses rate-limit state
**Given** a native Codex session JSONL line containing `token_count` with
`rate_limits.primary.used_percent`, `window_minutes`, and `resets_at`
**When** the Codex native signal adapter reads the session
**Then** DDx returns a current quota/headroom signal with freshness and reset
metadata

### TC-004: Claude stats-cache adapter reads historical usage
**Given** `~/.claude/stats-cache.json` with `dailyActivity` and `modelUsage`
**When** the Claude historical usage adapter reads the file
**Then** DDx returns provider-native historical usage totals

### TC-005: Claude current quota is unknown when no stable source exists
**Given** no stable non-PTY current-quota source is available for Claude
**When** DDx probes Claude routing state
**Then** quota/headroom is reported as `unknown` rather than guessed

### TC-006: ExtractUsage falls back on malformed output
**Given** harness output that doesn't contain valid JSON
**When** `ExtractUsage()` is called
**Then** returns zero-value `UsageData` (no panic, no error)

### TC-007: Activity-row backward compatibility
**Given** a JSONL line with only `"tokens": 1200` (no input/output/cost)
**When** unmarshaled into `SessionEntry`
**Then** `Tokens=1200`, `InputTokens=0`, `OutputTokens=0`, `CostUSD=0`

### TC-008: Activity-row native reference fields round-trip
**Given** a `SessionEntry` with token fields plus native session/reference
metadata
**When** marshaled to JSON and back
**Then** all captured fields are preserved

### TC-009: Native persistence defaults are preserved
**Given** the codex and claude harnesses from the registry
**When** their base args are inspected
**Then** codex does not force `--ephemeral` and claude does not force
`--no-session-persistence`

### TC-010: Minimal DDx routing metrics are recorded without transcript duplication
**Given** DDx runs the same harness multiple times
**When** routing metrics are updated
**Then** DDx records recent latency/success data, quota snapshot history, and
estimated subscription burn inputs without storing provider transcripts as a
required input

### TC-011: Routing doctor reports quota tri-state and freshness
**Given** normalized routing signals for codex and claude
**When** `ddx agent doctor --routing` is run
**Then** each harness reports quota/headroom as `ok`, `blocked`, or `unknown`
plus source freshness

### TC-011a: Live-probe quota sources update async snapshots instead of blocking routing
**Given** a harness quota source requires active probing
**When** DDx refreshes quota state
**Then** the live probe updates cached quota snapshots asynchronously and
dispatch continues to consume the freshest available snapshot rather than
waiting on inline terminal automation

### TC-012: Usage command consumes normalized signals
**Given** provider-native adapters and DDx routing metrics both provide data
**When** `ddx agent usage --format json` is run
**Then** the output is derived from those normalized signals rather than only a
DDx-owned duplicate session ledger

### TC-013: Cost estimation still works when provider does not report cost
**Given** a codex run with known model and token counts
**When** cost is estimated
**Then** DDx returns the expected estimated cost

### TC-014: Bead-attempt runtime metrics remain captured automatically
**Given** `ddx try` completes with an agent response exposing token data
**When** the run record is inspected
**Then** built-in runtime metrics are present independent of provider-native
transcript ownership

## Implementation

Tests in `cli/internal/agent/*_test.go` and relevant `cli/cmd/*_test.go`
packages covering extraction, adapters, routing-state reporting, and command
integration.

## Pass Criteria

All 15 test cases pass. `go test ./internal/agent/... ./cmd/...` green.
