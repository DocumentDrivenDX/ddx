---
ddx:
  id: AR-2026-04-05-agent-token
  depends_on:
    - FEAT-006
    - FEAT-014
    - SD-006
    - SD-014
    - TD-006
    - TP-006
    - TP-014
---
# Alignment Review: Agent Service and Token Awareness

**Review Date**: 2026-04-05
**Scope**: Agent service (FEAT-006) + Token awareness (FEAT-014)
**Status**: complete
**Review Epic**: `hx-6d163089`
**Tracker Issue**: `hx-264b30ba`

## Prior Review Status

The prior review (AR-2026-04-04-agent-beads.md) verified the agent service post-refactor and found it well-aligned. FEAT-014 (token awareness) was not covered there. This review checks both features against their current specs.

## Agent Service (FEAT-006)

### Acceptance Criteria Status

| Story | Criterion | Status | Evidence |
|-------|-----------|--------|---------|
| US-060 | `ddx agent run` sends prompt, captures response, writes session log | SATISFIED | `cli/internal/agent/runner.go:Run()`, `logSession()` |
| US-061 | Quorum mode: `--quorum=majority --harnesses=a,b` invokes both and computes consensus | SATISFIED | `cli/internal/agent/quorum.go` |
| US-062 | `ddx agent list` shows installed and authenticated harnesses | SATISFIED | `cli/cmd/agent_cmd.go`, `agent/registry.go` |
| US-063 | `ddx agent log` shows recent sessions | SATISFIED | `cli/cmd/log.go` |
| US-064 | `ddx agent capabilities <harness>` reports available reasoning levels and models | SATISFIED | `cli/internal/agent/runner.go:Capabilities()`, `cli/cmd/agent_capabilities_test.go` |

### Component Map

| Component | Status | Notes |
|-----------|--------|-------|
| Executor interface | SATISFIED | `Executor` + `OSExecutor` + `LookPathFunc` — full testability seam |
| Harness registry | SATISFIED | Data-driven; no name-based switches in runner |
| Runner decomposition | SATISFIED | `resolveHarness/resolvePrompt/resolveModel/resolveTimeout/BuildArgs/processResult` |
| Quorum dispatch | SATISFIED | Parallel execution, threshold logic, strategy evaluation |
| Token extraction (structured) | SATISFIED | `ExtractUsage()` handles codex `turn.completed` JSONL event and claude `--output-format json` envelope |
| Token extraction (regex fallback) | DEAD CODE | `TokenPattern` field in codex harness (`registry.go:24`) is superseded by `ExtractUsage()` but never removed |
| Session logging (metadata) | SATISFIED | JSONL append to `.ddx/agent-logs/sessions.jsonl` |
| Session body capture | INCOMPLETE | Prompt and response bodies not yet persisted per TD-006 spec |
| Config loading from `.ddx/config.yaml` | INCOMPLETE | `agent_cmd.go:43` has TODO; default harness, model overrides, timeout from project config are ignored |
| Permission profiles | INCOMPLETE | FEAT-006 defines `safe`/`supervised`/`unrestricted` profiles; implementation currently hardcodes `bypassPermissions` and `--dangerously-skip-permissions` for claude |

### Remaining Gaps

| Gap | Severity | Evidence | Existing Issue |
|-----|----------|----------|----------------|
| Session body capture (prompt/response bodies) not persisted | medium | `cli/internal/agent/runner.go:logSession()` writes metadata only; `docs/helix/02-design/technical-designs/TD-006-agent-session-capture.md` requires body files | tracked in plan doc |
| Config not loaded from `.ddx/config.yaml` | medium | `cli/cmd/agent_cmd.go:43` `// TODO: load agent config` | tracked as F-009 |
| Permission profiles not implemented | medium | `cli/internal/agent/registry.go` — codex uses `--dangerously-bypass-approvals-and-sandbox`, claude uses `bypassPermissions`; no safe/supervised/unrestricted routing | none — new gap |

## Token Awareness (FEAT-014)

### Acceptance Criteria Status

| Story | Criterion | Status | Evidence |
|-------|-----------|--------|---------|
| US-141 | codex: `input_tokens` and `output_tokens` captured (non-zero) | SATISFIED | `ExtractUsage("codex", ...)` parses `turn.completed` event from `--json` output (`runner.go:283`) |
| US-141 | claude: `input_tokens`, `output_tokens`, `cost_usd` captured | SATISFIED | `ExtractUsage("claude", ...)` parses `--output-format json` envelope (`runner.go:306`) |
| US-141 | Old session logs without new fields load without error | SATISFIED | `SessionEntry` uses `omitempty`; zero-value defaults apply |
| US-140 | `ddx agent usage` shows per-harness table with sessions, tokens, cost, avg duration | SATISFIED | `cli/cmd/agent_usage.go:newAgentUsageCommand()` |
| US-140 | `--since today/7d/30d/YYYY-MM-DD` filters by time window | SATISFIED | `parseSince()` in `agent_usage.go` |
| US-140 | `--format json` produces valid JSON | SATISFIED | `renderUsageJSON()` |
| US-140 | `--format csv` produces CSV | SATISFIED | `renderUsageCSV()` |
| US-140 | `--harness <name>` filters to one harness | SATISFIED | `aggregateUsage()` harness filter |

### Schema Alignment

| Field | FEAT-014 Spec | Implementation | Aligned? |
|-------|--------------|----------------|----------|
| `input_tokens` | `SessionEntry.input_tokens` | `cli/internal/agent/types.go:80` | YES |
| `output_tokens` | `SessionEntry.output_tokens` | `cli/internal/agent/types.go:81` | YES |
| `cost_usd` | `SessionEntry.cost_usd` | `cli/internal/agent/types.go:82` | YES |
| `tokens` (legacy) | kept for backward compat | `cli/internal/agent/types.go:79` | YES |
| Cost estimation fallback | pricing table `EstimateCost()` | `cli/internal/agent/pricing.go` | YES |

### Remaining Gaps

| Gap | Severity | Evidence |
|-----|----------|---------|
| Gemini token capture | out of scope | FEAT-014 spec explicitly defers gemini until auth is available |
| Budget-aware model selection (v2) | out of scope | FEAT-014 marks items 12–14 as "Future: v2" |
| Codex `TokenPattern` in registry.go is stale dead code | low | `cli/internal/agent/registry.go:24` — `TokenPattern: "tokens used\n([0-9,]+)"` is never exercised since `ExtractUsage()` handles codex; no bug but adds confusion |

## Execution Issues Generated

| Issue | Type | Goal |
|-------|------|------|
| `hx-perm-profiles` (create below) | task | Implement permission profile routing (safe/supervised/unrestricted) |

## Issue Coverage

| Gap | Covering Issue | Status |
|-----|----------------|--------|
| Session body capture | tracked in plan doc, untracked in beads | OPEN — needs bead |
| Config loading from `.ddx/config.yaml` | F-009 (plan doc) | OPEN |
| Permission profiles | new | to create |
| Dead `TokenPattern` code | low — no issue needed | |

## Open Decisions

None.
