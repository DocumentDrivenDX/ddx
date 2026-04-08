---
ddx:
  id: FEAT-006
  depends_on:
    - helix.prd
---
# Feature: DDx Agent Service

**ID:** FEAT-006
**Status:** Complete
**Priority:** P0
**Owner:** DDx Team

## Overview

The DDx agent service is the unified interface for dispatching work to AI coding agents (codex, claude, gemini, opencode, cursor, etc.). It handles harness discovery, prompt delivery, output capture, token tracking, session logging, and multi-agent quorum. Workflow tools and check runners call `ddx agent` instead of implementing their own harness abstraction.

Within the broader DDx execution model (FEAT-010), `ddx agent` is the
dedicated `agent` executor kind. It remains the canonical surface for direct
agent dispatch and the authoritative source of raw prompt/response logs for
agent-backed execution runs.

## Forge Integration Boundary

Forge is an embeddable Go agent runtime — a tool-calling LLM loop.
DDx embeds forge as a library via `forge.Run()`. The boundary:

**Forge owns** (do not duplicate in DDx):
- Agent loop (prompt → LLM → tool calls → repeat)
- Provider abstraction (OpenAI-compatible, Anthropic, virtual)
- Tool execution (read, write, edit, bash)
- Prompt construction (presets, context file loading, guidelines)
- Session event logging (JSONL per session) and replay
- Cost estimation via built-in pricing table

**DDx owns** (do not push into forge):
- Harness registry and discovery (forge is one of many harnesses)
- Model tier resolution (smart/fast per harness)
- Comparison dispatch, grading, replay from bead
- Bead linkage and execution evidence
- Aggregate usage tracking (`ddx agent usage`)
- Configuration in `.ddx/config.yaml` (maps to forge config)

**Integration rules:**
- DDx calls `forge.Run()` and maps `forge.Result` to `agent.Result`
- Configuration (provider, model, base URL, preset, max iterations)
  flows from `.ddx/config.yaml` and env vars into `forge.Request`
- DDx does not re-implement prompt building — it calls
  `prompt.NewFromPreset()` and passes through
- DDx does not manage forge tools — it constructs the standard tool
  set and passes `WorkDir`
- Forge session events log to the DDx session log dir so
  `ddx agent log` has a unified view
- Forge's `Result.ToolCalls` are preserved in comparison arms for
  richer evaluation (subprocess harnesses don't have this)

## Problem Statement

**Current situation:** Both HELIX and dun independently implement agent dispatch:
- HELIX has a bash-based harness in `scripts/helix` that manages codex/claude invocation, output capture, token tracking, and cross-model review. It works well but is bash-only.
- Dun has a Go-based harness abstraction (`harnesses.go`, `agent.go`) with quorum/consensus logic but less mature output management.
- Both maintain separate harness registries, caching, and configuration.

**Pain points:**
- Duplicated harness code across two projects
- Inconsistent agent invocation behavior
- No shared session logging or token tracking
- Quorum logic only available in dun, not accessible to HELIX
- New harnesses must be added in multiple places

**Desired outcome:** A single `ddx agent` command that any tool can call to invoke an agent, with consistent output capture, token tracking, session logging, and quorum support.

## Requirements

### Functional

1. **Harness registry** — built-in support for codex, claude, gemini, opencode, forge, pi, cursor. Extensible via config. Codex, claude, and opencode are at full subprocess parity. Forge is the embedded native agent (see below).
2. **Harness discovery** — detect which harnesses are available on the system (binary exists, authenticated, etc.). Embedded harnesses (forge, virtual) are always available. Cache results.
3. **Agent invocation** — `ddx agent run --harness=<name>` sends a prompt to the specified agent and captures output.
3a. **Embedded forge agent** — `ddx agent run --harness=forge` runs the [forge](https://github.com/DocumentDrivenDX/forge) agent loop in-process via `forge.Run()`. No subprocess, no binary lookup. Forge provides a tool-calling LLM loop with read/write/edit/bash tools, supporting any OpenAI-compatible endpoint (LM Studio, Ollama, OpenAI) or Anthropic. Local models run at zero cost. Configuration via `FORGE_PROVIDER`, `FORGE_BASE_URL`, `FORGE_MODEL` env vars or `.forge/config.yaml`.
4. **Prompt delivery** — accept prompt from stdin, file, or inline argument. Support prompt envelope format.
5. **Output capture** — capture agent stdout/stderr, parse structured responses, track token usage where available.
6. **Session logging** — log each agent invocation (prompt, response, tokens, duration, harness) to a session log directory.
7. **Token tracking** — extract and record token counts from agent responses where the harness supports it.
8. **Quorum dispatch** — `ddx agent run --quorum=majority --harnesses=codex,claude` runs multiple agents and requires consensus.
9. **Quorum strategies** — any (first success), majority, unanimous, numeric threshold.
10. **Automation levels** — manual, plan, auto, yolo — control how much autonomy the agent gets.
11. **Timeout management** — per-invocation timeout with configurable default.
12. **Configuration** — default harness, model overrides, timeout, automation level in `.ddx/config.yaml`.
13. **Capability introspection** — for a selected harness, `ddx agent` can list the reasoning levels and models that are available or configured for that harness before invocation.
14. **Prompt envelope format** — standard JSON format for structured agent I/O (kind, id, title, prompt, inputs, response_schema, callback).
15. **Response processing** — parse agent response (status, signal, detail, next, issues) and return structured result.

### Non-Functional

- **Performance:** Agent invocation overhead <100ms beyond the agent's own response time.
- **Portability:** Works on macOS, Linux, Windows. Single binary.
- **Reliability:** Graceful handling of agent crashes, timeouts, malformed responses.
- **Observability:** Session logs are human-readable and machine-parseable.

## User Stories

### US-060: Workflow Tool Invokes Agent
**As** a workflow tool
**I want** to call `ddx agent run --harness=codex --prompt task.md`
**So that** I don't need my own agent dispatch code

**Acceptance Criteria:**
- Given codex is available, when a workflow tool calls `ddx agent run`, then the prompt is sent and the response is captured
- Given the invocation completes, then a session log entry is created with prompt, response, tokens, and duration
- Given the agent times out, then `ddx agent run` returns a clear timeout error

### US-061: Check Runner Uses Agent for Agent-Type Checks
**As** dun (check runner)
**I want** to call `ddx agent run` for checks that require agent evaluation
**So that** I don't maintain my own harness abstraction

**Acceptance Criteria:**
- Given dun has a prompt envelope, when it calls `ddx agent run --format=envelope`, then the agent receives the prompt and dun gets a structured response
- Given quorum mode, when dun calls `ddx agent run --quorum=majority --harnesses=codex,claude`, then both agents are invoked and consensus is computed

### US-062: Developer Checks Available Agents
**As a** developer setting up a project
**I want** to see which agents are available on my system
**So that** I can configure my workflow tools

**Acceptance Criteria:**
- Given I run `ddx agent list`, then I see which harnesses are installed and authenticated
- Given I run `ddx agent doctor`, then I see detailed status for each harness

### US-064: Developer Inspects Agent Capabilities
**As a** developer selecting an agent for a task
**I want** to see the supported reasoning levels and models for that harness
**So that** I can choose a valid invocation without trial and error

**Acceptance Criteria:**
- Given I select a harness, when I ask `ddx agent` for capabilities, then I see the available reasoning levels and models for that harness
- Given the harness has no explicit model override, then the capability output still shows the harness default model and any valid reasoning-level options
- Given an invalid or unknown harness selection, then capability introspection returns a clear error instead of an empty or partial list

### US-063: Developer Reviews Agent Session Logs
**As a** developer debugging an agent interaction
**I want** to review the session log for a recent agent invocation
**So that** I can see what prompt was sent and what the agent returned

**Acceptance Criteria:**
- Given agent invocations have occurred, when I run `ddx agent log`, then I see recent sessions with timestamps, harness, tokens, duration, and correlation metadata
- Given I specify a session ID, then I see the full prompt, response, stderr, and exit code for that session
- Given the session was recorded before full-body capture existed, then the entry still loads and shows the available metadata without breaking

### US-065: Developer Runs Agent Against a Bead
**As** a developer or workflow tool
**I want** `ddx agent execute-bead` to run an agent on a bead in an isolated, auditable way
**So that** the result is safely landed or preserved without manual git operations

**Acceptance Criteria:**
- Given a valid bead ID, when `ddx agent execute-bead <id>` is invoked, then DDx resolves the bead and governing artifacts and begins the workflow (step 1).
- Given `--from` is omitted, when the base revision is resolved (step 2), then DDx uses `HEAD`.
- Given `--harness`, `--model`, and `--effort` are provided, when the agent runs (step 5), then execute-bead honors them exactly as a normal `ddx agent run` invocation would.
- Given execution completes, when the worktree is cleaned up (step 12), then no temporary worktree created by execute-bead remains in the filesystem.
- Given `--no-merge` is set, when the iteration completes, then DDx creates a committed attempt and preserves it under a hidden ref (step 11) — it is not landed regardless of execution outcomes.
- Given graph-discovered required executions fail (step 9), when the merge decision is made (step 10), then DDx preserves the iteration under a hidden ref and does not fast-forward the target branch.
- Given all required executions pass and ratchets are satisfied (step 9), when the merge decision is made (step 10) and `--no-merge` is not set, then DDx lands the result by fast-forward.
- Given execute-bead completes, when the run record is inspected, then it contains built-in runtime metrics as specified in FEAT-014 US-145, captured automatically for the iteration.

### Session Capture

Agent session evidence is stored locally under `session_log_dir` (default
`.ddx/agent-logs`) as a dedicated bead-backed collection plus attachment
files. Each session record captures the invocation metadata plus references to
the prompt and response bodies needed for inspection.

Minimum session fields:

- `id`
- `timestamp`
- `harness`
- `model`
- `tokens`
- `duration_ms`
- `exit_code`
- `error`
- `correlation`
- references to stored prompt, response, and log bodies

The `correlation` block is workflow-agnostic and may carry keys such as
`bead_id`, `doc_id`, `workflow`, `request_id`, or `parent_session_id` when
workflow tools provide them.

Storage and retention are policy-driven:

- The authoritative session metadata record may live in a dedicated
  bead-schema collection, while prompt, response, stdout, stderr, or other
  large bodies live in named attachment files.
- By default, local session logs retain the full captured bodies for
  inspection.
- Optional redaction rules may mask sensitive substrings before persistence.
- Existing metadata-only JSONL session logs remain readable and must not fail
  session listing or inspection.

Inspection UX:

- `ddx agent log` lists recent sessions using the stored metadata.
- `ddx agent log <session-id>` shows the full stored bodies and correlation
  context for one session.
- API and MCP session-detail surfaces mirror the same session identity and
  attachment-backed detail model.

## Bead Execution Workflow

`ddx agent execute-bead <bead-id> [--from <rev>] [--no-merge]` is the
canonical agent-driven bead execution workflow. It is an agent workflow mode
layered on top of the existing harness/session machinery — not a separate
provenance system.

### Workflow steps

1. Resolve the bead and its governing artifacts from the DDx document graph.
2. Resolve the git base revision: `--from <rev>` if provided, otherwise `HEAD`.
3. If the caller's worktree is dirty, create a checkpoint commit first and use
   that checkpoint as the actual base.
4. Create an isolated execution worktree from the resolved base.
5. Run the agent against the bead using the standard `ddx agent` harness, model,
   and reasoning controls.
6. Capture full session evidence: transcript, tool calls, and runtime metadata.
7. Resolve applicable execution documents from the document graph inside the
   execution worktree (see FEAT-007).
8. Run all required execution documents plus relevant metric/observation
   executions.
9. Evaluate required execution results and metric ratchets (see TD-005).
10. If merge-eligible and `--no-merge` is not set, land by rebase + fast-forward
    semantics.
11. Otherwise, preserve the iteration result under a hidden ref and do not merge
    (see SD-012 for the hidden-ref naming scheme).
12. Always remove the temporary worktree after preserving enough evidence for
    replay and introspection.

### Always-on runtime metrics

`execute-bead` always captures built-in runtime metrics for each iteration,
independent of graph-authored execution documents (see FEAT-014):

- harness, model, session ID
- elapsed duration
- input tokens, output tokens, total tokens
- cost (where available)
- base revision, result revision

### Iteration commit summary

Each execute-bead iteration produces a commit (landed or preserved under a
hidden ref) with a minimum summary surface that enables post-hoc evaluation
without opening session attachments:

- bead ID
- base revision and result revision
- harness and model
- required execution outcome summary (pass / fail / skipped)
- ratchet evaluation summary
- merge vs preserve outcome

Full conversation transcript, tool call detail, and session evidence are stored
in DDx runtime storage (session logs and exec-run attachments), not in git
history.

## Comparison Dispatch

DDx's existing quorum mechanism runs the same prompt through multiple
harnesses and checks for consensus. **Comparison dispatch** extends this
for evaluation: run the same prompt through multiple harnesses, capture
all outputs and side effects, and record structured comparison results.

```bash
# Compare forge (local model) against claude on the same task
ddx agent run --compare --harnesses=forge,claude --prompt task.md

# Each arm runs in an isolated worktree to capture side effects
ddx agent run --compare --harnesses=forge,claude --prompt task.md --sandbox
```

### Sandboxed comparison runs

When `--sandbox` is specified (or implied by `--compare`), each harness
arm runs in a temporary git worktree:

1. Create a worktree per harness: `.worktrees/compare-<id>-<harness>/`
2. Run the agent in that worktree (forge: `WorkDir`, subprocess: `WorkDirFlag`)
3. After completion, capture `git diff` as the "effect artifact"
4. Record: prompt, output text, git diff, tool call log (forge), tokens, cost
5. Clean up worktrees (or preserve with `--keep-sandbox`)

This ensures harness arms don't interfere with each other or with the
user's working tree, and provides a concrete artifact (the diff) for
grading.

### Comparison record schema

Each comparison run produces a `ComparisonRecord` in the session log:

```json
{
  "id": "cmp-<hash>",
  "timestamp": "...",
  "prompt": "...",
  "arms": [
    {
      "harness": "forge",
      "model": "qwen/qwen3-coder-next",
      "output": "...",
      "diff": "...",
      "tool_calls": [...],
      "tokens": { "input": 3465, "output": 120 },
      "cost_usd": 0,
      "duration_ms": 8500,
      "exit_code": 0
    },
    {
      "harness": "claude",
      "model": "claude-sonnet-4-20250514",
      "output": "...",
      "diff": "...",
      "tokens": { "input": 5000, "output": 800 },
      "cost_usd": 0.045,
      "duration_ms": 12000,
      "exit_code": 0
    }
  ],
  "grade": null
}
```

The `grade` field is populated by `ddx agent grade` (see FEAT-019).

## Implementation Notes

### Porting from HELIX

The HELIX bash harness (`scripts/helix`) has proven patterns worth preserving in the Go implementation:
- Output management (capturing stdout/stderr cleanly)
- Token tracking (parsing usage from agent responses)
- Cross-model review (alternating agents for quality)
- Session logging format and directory structure
- Timeout and error handling
- Full-body prompt/response capture with backward-compatible session replay

### Porting from Dun

The dun Go harness has patterns worth preserving:
- Harness registry with preference ordering
- Harness cache (avoid re-probing on every invocation)
- Quorum logic (any/majority/unanimous/numeric strategies)
- Prompt envelope format (structured agent I/O)
- Response schema validation
- Cost-optimized sequential mode vs parallel

### CLI Commands

```bash
ddx agent run --harness=codex --prompt task.md      # invoke agent
ddx agent run --quorum=majority --harnesses=a,b     # multi-agent
ddx agent run --automation=plan                      # control autonomy
ddx agent execute-bead <bead-id>                    # canonical bead execution workflow
ddx agent execute-bead <bead-id> --from <rev>       # use specific git base
ddx agent execute-bead <bead-id> --no-merge         # preserve iteration without landing
ddx agent list                                       # available harnesses
ddx agent capabilities codex                         # inspect harness capabilities
ddx agent doctor                                     # harness health
ddx agent log                                        # recent sessions
ddx agent log <session-id>                           # full session detail
```

### Configuration

```yaml
# .ddx/config.yaml
agent:
  harness: codex                    # default harness
  model: ""                         # model override
  models:                           # per-harness model overrides
    claude: claude-sonnet-4-20250514
  reasoning_levels:                 # per-harness reasoning-level overrides
    codex: [low, medium, high]
  timeout_ms: 300000                # 5 minute default
  automation: auto                  # manual|plan|auto|yolo
  session_log_dir: .ddx/agent-logs  # session log location
```

## Migration Strategy

HELIX and dun have working agent dispatch today. The transition to `ddx agent` must be incremental so nothing breaks during migration.

**Phase 1 — DDx ships basic agent invocation.** `ddx agent run --harness=codex --prompt file.md` works for the single-harness, single-invocation case. Quorum can follow.

**Phase 2 — HELIX and dun add `ddx agent` as an alternative path.** Both tools detect whether `ddx agent` is available. If yes, use it. If no, fall back to their existing harness code. Controlled via env var (`DDX_AGENT=1`) or config.

**Phase 3 — Prove parity.** Run both paths in parallel on real work. Verify output capture, token tracking, and session logging match expectations.

**Phase 4 — Remove old harness code.** Once `ddx agent` is proven, HELIX removes its bash harness functions and dun removes `harnesses.go`, `agent.go`, and `quorum.go`.

This ensures no working functionality is lost at any step.

## Dependencies

- Harness binaries (codex, claude, etc.) installed by user
- DDx CLI infrastructure (config loading, command factory)

## Agent Permission Model

**Problem:** DDx currently hardcodes permissive flags into harness
invocations (`--dangerously-bypass-approvals-and-sandbox` for codex,
`--dangerously-skip-permissions` for claude). This is unsafe for normal
users who may not understand the implications.

**Design:**

DDx defines three permission profiles:

| Profile | Behavior | When to use |
|---------|----------|-------------|
| `safe` (default) | Uses harness's built-in permission model. No bypass flags. Agent asks for approval on destructive actions. | Normal users, first-time setup |
| `supervised` | Auto-approves read operations, prompts for writes and shell commands. Harness-specific flag mapping. | Experienced users with review workflow |
| `unrestricted` | Current behavior — all safety bypassed. Harness runs with full permissions. | Controlled CI environments, experienced operators |

**Configuration:**
```yaml
# .ddx/config.yaml
agent:
  permissions: safe  # safe | supervised | unrestricted
```

**CLI override:** `ddx agent run --permissions unrestricted`

**Harness flag mapping:**

| Profile | codex flags | claude flags | opencode flags | forge behavior |
|---------|------------|--------------|----------------|----------------|
| safe | (none — default codex behavior) | (none — default claude behavior) | (none — `run` auto-approves) | Tools always execute (embedded) |
| supervised | `--auto-approve-reads` | `--permission-mode default` | (none — no granular control) | Tools always execute (embedded) |
| unrestricted | `--dangerously-bypass-approvals-and-sandbox` | `--permission-mode bypassPermissions --dangerously-skip-permissions` | (none — `run` auto-approves) | Tools always execute (embedded) |

> **Note:** opencode's `run` subcommand auto-approves all tool permissions in
> non-interactive mode. Forge runs in-process with direct tool execution — there
> is no permission layer. Both behave as effectively unrestricted.

**Safety invariant:** If `agent.permissions` is not explicitly set in config
AND the `--permissions` flag is not provided, DDx defaults to `safe` and
logs a one-time notice explaining the available modes.

## Provider Usage Data Sources

Each harness exposes different levels of usage data. DDx captures what is
available and uses it for usage tracking (FEAT-014) and self-throttling.

| Source | codex | claude | opencode | forge (embedded) |
|--------|-------|--------|----------|-----------------|
| **Per-invocation tokens** | `turn.completed` JSONL: `input_tokens`, `cached_input_tokens`, `output_tokens` | JSON envelope: `usage.input_tokens`, `output_tokens`, `cache_read_input_tokens`, `cache_creation_input_tokens` | JSON envelope: `usage.input_tokens`, `output_tokens` (if present) | Direct from `forge.Result.Tokens` — `Input`, `Output`, `Total` |
| **Per-invocation cost** | Not reported | `total_cost_usd` in JSON envelope | `total_cost_usd` (if present) | `forge.Result.CostUSD` — built-in pricing table; $0 for local models, -1 for unknown |
| **Per-invocation model info** | Not reported | `modelUsage` block: per-model token breakdown, `contextWindow`, `maxOutputTokens` | Not reported | `forge.Result.Model` — provider-reported model name |
| **Historical stats file** | None known | `~/.claude/stats-cache.json`: daily activity, daily tokens by model, cumulative model usage | None known | `.forge/sessions/*.jsonl` — structured event logs per session |
| **Account limits** | TUI `/status` only (PTY scraping; fragile) | Not exposed programmatically | Not exposed | N/A — local models have no rate limits |
| **Budget passthrough** | None | `--max-budget-usd` flag (per-session cap) | None | `MaxIterations` on forge.Request |

### Key implications for self-throttling (see FEAT-014)

- **Claude** is the richest: real cost per invocation, historical stats file,
  and a built-in budget cap flag. DDx can read `stats-cache.json` for
  account-wide activity and pass `--max-budget-usd` for API-key users.
- **Codex** provides token counts but no cost or account limits. The only
  source for rate-limit headroom is the TUI `/status` command, which requires
  PTY scraping (as demonstrated by steipete/CodexBar). DDx should track its
  own session-log-based usage as the primary signal.
- **opencode** has JSON output support but token/cost reporting in the
  envelope is not yet confirmed in all versions. DDx parses opportunistically.
- **Forge** is the richest for DDx because it's embedded: typed `Result`
  struct with exact tokens, cost, model, session ID, and tool call logs —
  no parsing needed. Local models (via LM Studio/Ollama) report $0 cost.
  Session event logs in `.forge/sessions/` provide full replay capability.

## Out of Scope

- **Autonomy semantics** — DDx does not define what autonomy levels mean behaviorally; that is delegated to workflow plugins
- **Workflow routing and orchestration** — DDx does not decide when to invoke execute-bead, what to do with the outcome, or how to sequence workflow phases; that is delegated to workflow tools
- **Escalation and supervisory policy** — follow-on bead creation, stop/continue rules, and conflict escalation are workflow plugin concerns
- **Prompt design and engineering strategy** — bead prompt structure, prompt optimization, and rubric content are delegated to plugins; DDx provides the dispatch and grading mechanics
- **Server-side agent dispatch** — `ddx agent run` is CLI-only for security.
  The localhost-only dispatch endpoints in FEAT-002 (items 40-41) delegate to
  the CLI internally and require API key for non-local access.
- Building or hosting AI agents
- Model fine-tuning or prompt optimization
- Agent-to-agent communication protocols
- IDE integration for agent invocation
