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

The DDx agent service is the unified interface for dispatching work to AI coding agents (codex, claude, gemini, opencode, cursor, etc.). It handles harness discovery, prompt delivery, output capture, routing-signal normalization, minimal DDx invocation activity capture, and multi-agent quorum. Workflow tools and check runners call `ddx agent` instead of implementing their own harness abstraction.

For ordinary users, the primary UX is intent-first rather than harness-first:
DDx should normally route from `--profile`, `--model`, `--effort`, and
permission settings to the best viable harness. Explicit harness selection
remains available as an override for debugging, comparisons, replay, and other
advanced use.

Within the broader DDx execution model (FEAT-010), `ddx agent` is the
dedicated `agent` executor kind. It remains the canonical surface for direct
agent dispatch and the authoritative source of DDx-side routing evidence,
invocation correlation, and runtime metadata for agent-backed execution runs.

## Agent Library Integration Boundary

DDx Agent (formerly forge) is an embeddable Go agent runtime — a tool-calling LLM loop.
DDx embeds the agent library and calls its run function. The boundary:

**Agent library owns** (do not duplicate in DDx):
- Agent loop (prompt → LLM → tool calls → repeat)
- Provider abstraction (OpenAI-compatible, Anthropic, virtual)
- Tool execution (read, write, edit, bash)
- Prompt construction (presets, context file loading, guidelines)
- Session event logging, OTEL/runtime telemetry emission, and replay
- Cost estimation via built-in pricing table

**DDx owns** (do not push into the agent library):
- Harness registry and discovery (agent is one of many harnesses)
- Harness routing policy: intent-first profile/model selection, candidate
  planning, and final harness choice
- Consumption of the shared model-catalog policy for aliases, profiles,
  canonical targets, and deprecations
- Comparison dispatch, grading, replay from bead
- Bead linkage and execution evidence
- Normalized routing-signal views across harnesses
- Minimal DDx-observed performance metrics for routing
- Configuration in `.ddx/config.yaml` (maps to agent runner config)

**Integration rules:**
- DDx calls the agent library run function and maps the result to `agent.Result`
- Configuration (provider, model, base URL, preset, max iterations)
  flows from `.ddx/config.yaml` and env vars into the agent library request
- DDx does not re-implement prompt building — it calls
  `prompt.NewFromPreset()` and passes through
- DDx does not manage agent tools — it constructs the standard tool
  set and passes `WorkDir`
- DDx must not suppress native session persistence for external harnesses by
  default. Native provider logs remain the primary transcript/quota source for
  codex, claude, and other subprocess harnesses.
- For the embedded harness, DDx consumes the runtime's session/telemetry output
  and records only the correlation and metadata needed for DDx workflows.
- Forge's `Result.ToolCalls` are preserved in comparison arms for
  richer evaluation (subprocess harnesses don't have this)

The full routing contract — normalized request model, candidate planning,
ranking rules, rejection criteria, and the DDx-vs-embedded boundary — is
specified in `docs/helix/02-design/solution-designs/SD-015-agent-routing-and-catalog-resolution.md`.

## Problem Statement

**Current situation:** Both HELIX and dun independently implement agent dispatch:
- HELIX has a bash-based harness in `scripts/helix` that manages codex/claude invocation, output capture, token tracking, and cross-model review. It works well but is bash-only.
- Dun has a Go-based harness abstraction (`harnesses.go`, `agent.go`) with quorum/consensus logic but less mature output management.
- Both maintain separate harness registries, caching, and configuration.

**Pain points:**
- Duplicated harness code across two projects
- Inconsistent agent invocation behavior
- No shared routing-signal model across harnesses
- External harness persistence is currently treated as something DDx can
  suppress or replace, which blocks access to native quota/usage signals
- Quorum logic only available in dun, not accessible to HELIX
- New harnesses must be added in multiple places

**Desired outcome:** A single `ddx agent` command that any tool can call to
invoke an agent, with consistent output capture, informed harness selection,
provider-aware availability checks, minimal DDx activity capture, and quorum
support.

## Requirements

### Functional

1. **Harness registry** — built-in support for codex, claude, gemini, opencode, embedded agent/ddx-agent, pi, cursor. Extensible via config. Codex, claude, and opencode are at full subprocess parity. The embedded harness is the in-process agent runtime (see below).
2. **Harness discovery and state** — detect which harnesses are available on the system and model their routing-relevant state: installed, reachable, authenticated, quota/headroom state (`ok`, `blocked`, or `unknown`), policy-restricted, healthy/degraded, and signal freshness. Embedded harnesses are always installed, but may still be unroutable if their provider/backend configuration cannot satisfy the request. DDx may cache these checks with explicit freshness/TTL rules.
3. **Intent-first agent invocation** — `ddx agent run --profile=<cheap|fast|smart>` or `ddx agent run --model <ref-or-exact>` sends a prompt through the DDx routing planner, which selects the best viable harness for the request and captures the output.
3a. **Explicit harness override** — `ddx agent run --harness=<name>` bypasses automatic harness selection and forces one harness. This remains the correct path for debugging, replay, and comparisons.
3b. **Embedded DDx Agent** — `ddx agent run --harness=agent` runs the [DDx Agent](https://github.com/DocumentDrivenDX/agent) loop in-process via the agent library. No subprocess, no binary lookup. The agent library provides a tool-calling LLM loop with read/write/edit/bash tools, supporting any OpenAI-compatible endpoint (LM Studio, Ollama, OpenAI) or Anthropic. Local models run at zero cost. Configuration via the embedded runtime config and its provider/backend settings.
4. **Prompt delivery** — accept prompt from stdin, file, or inline argument. Support prompt envelope format.
5. **Output capture** — capture agent stdout/stderr, parse structured responses, track token usage where available.
6. **Invocation activity capture** — record each DDx agent invocation with routing metadata, elapsed time, harness/model identity, correlation fields, and native session or trace references when available. DDx must not disable or replace native persistence for external harnesses by default.
7. **Signal normalization** — extract and normalize per-invocation usage, provider-native quota/usage signals where available, and DDx-observed routing metrics into a shared routing model.
7a. **Minimal DDx routing metrics** — maintain only the DDx-owned metrics needed for routing decisions (for example recent success/failure, snapshot history, quota availability history, estimated subscription burn, and latency), keyed by the resolved canonical target or concrete model so different models on the same surface stay separate, without duplicating provider transcripts or native session stores.
8. **Quorum dispatch** — `ddx agent run --quorum=majority --harnesses=codex,claude` runs multiple agents and requires consensus.
9. **Quorum strategies** — any (first success), majority, unanimous, numeric threshold.
10. **Automation levels** — manual, plan, auto, yolo — control how much autonomy the agent gets.
11. **Timeout management** — per-invocation timeout with configurable default.
12. **Configuration** — DDx config supports a default profile, optional forced harness, optional forced model ref/pin, timeout, and automation level in `.ddx/config.yaml`.
13. **Capability introspection** — for a selected harness, `ddx agent` can list the routing-relevant capabilities for that harness before invocation: reasoning levels, exact-pin support, effective current profile/model mappings, and deprecation/replacement notes surfaced from the shared catalog where applicable.
13b. **Routing planner contract** — DDx normalizes every request into a routing ask and evaluates one candidate plan per harness. Each candidate records resolved model info, viability, rejection reason if any, and enough metadata to explain why a harness was selected or skipped.
14. **Prompt envelope format** — standard JSON format for structured agent I/O (kind, id, title, prompt, inputs, response_schema, callback).
15. **Response processing** — parse agent response (status, signal, detail, next, issues) and return structured result.

### Non-Functional

- **Performance:** Agent invocation overhead <100ms beyond the agent's own response time.
- **Portability:** Works on macOS, Linux, Windows. Single binary.
- **Reliability:** Graceful handling of agent crashes, timeouts, malformed responses.
- **Observability:** Session logs are human-readable and machine-parseable.
- **Determinism:** Given the same request, catalog snapshot, harness state, and
  config, DDx routing should make the same harness choice and be able to
  explain that choice.

## User Stories

### US-060: Workflow Tool Invokes Agent
**As** a workflow tool
**I want** to call `ddx agent run --profile=cheap --prompt task.md`
**So that** I don't need my own agent dispatch code

**Acceptance Criteria:**
- Given multiple harnesses are installed, when a workflow tool calls
  `ddx agent run --profile=cheap`, then DDx selects the cheapest viable
  healthy harness according to the routing policy, current provider signals,
  and request constraints, and sends the prompt
- Given the invocation completes, then a DDx invocation activity record is
  created with runtime facts and native session or trace references when
  available
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
**I want** to see the supported reasoning levels, exact-pin behavior, and effective model/profile mappings for that harness
**So that** I can choose a valid invocation without trial and error

**Acceptance Criteria:**
- Given I select a harness, when I ask `ddx agent` for capabilities, then I see the available reasoning levels and effective current model/profile mappings for that harness
- Given the harness has no explicit model override, then the capability output still shows the harness default model and any valid reasoning-level options
- Given an invalid or unknown harness selection, then capability introspection returns a clear error instead of an empty or partial list

### US-066: Developer Requests an Exact Model Ref
**As a** developer
**I want** to ask for `qwen3` or another shared model ref without naming a harness
**So that** DDx routes me to the right implementation automatically

**Acceptance Criteria:**
- Given a model ref resolves only on the embedded harness surface, when I run
  `ddx agent run --model qwen3`, then DDx selects the embedded harness
- Given the embedded harness is selected, when the run starts, then DDx does
  not duplicate embedded provider/backend routing logic and instead delegates
  provider/backend choice to the embedded runtime
- Given no harness can satisfy the requested model ref, then DDx returns a
  clear routing error instead of guessing

### US-063: Developer Reviews Agent Invocation Activity
**As a** developer debugging an agent interaction
**I want** to review DDx invocation metadata and any available native session
references for a recent agent invocation
**So that** I can inspect what ran without DDx duplicating provider logs

**Acceptance Criteria:**
- Given agent invocations have occurred, when I run `ddx agent log`, then I
  see recent DDx invocation records with timestamps, harness, model, tokens
  when known, duration, and correlation metadata
- Given I specify a session ID, then I see the recorded DDx metadata plus any
  native session, trace, or transcript references available for that run
- Given the invocation was recorded before native-reference support existed,
  then the entry still loads and shows the available metadata without breaking

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

### Invocation Activity and Native Session Ownership

DDx stores invocation activity locally under `session_log_dir` (default
`.ddx/agent-logs`) as a lightweight metadata collection. For external
harnesses, native provider logs remain authoritative for transcripts, detailed
session history, and quota signals. DDx records only the metadata and
references needed for routing, provenance, and inspection.

Minimum invocation activity fields:

- `id`
- `timestamp`
- `harness`
- `model`
- `tokens` or token subfields when known
- `duration_ms`
- `exit_code`
- `error`
- `correlation`
- `native_session_id` or equivalent native reference when available
- `native_log_ref` or path when available
- `trace_id` / `span_id` when available from embedded runtime telemetry
- optional references to stored prompt, response, and log bodies only when DDx
  is the owner of that data

The `correlation` block is workflow-agnostic and may carry keys such as
`bead_id`, `doc_id`, `workflow`, `request_id`, or `parent_session_id` when
workflow tools provide them.

Storage and retention are policy-driven:

- The authoritative DDx invocation metadata record may live in a dedicated
  bead-schema collection, while optional prompt, response, stdout, stderr, or
  other large bodies live in named attachment files only when DDx owns them.
- By default, external harnesses retain their own full session bodies in their
  native stores; DDx does not suppress or replace those logs.
- Optional redaction rules may mask sensitive substrings before persistence.
- Existing metadata-only JSONL session logs remain readable and must not fail
  session listing or inspection.

Inspection UX:

- `ddx agent log` lists recent invocations using the stored DDx metadata.
- `ddx agent log <session-id>` shows the DDx metadata plus any native session,
  trace, or transcript references for one invocation.
- API and MCP session-detail surfaces mirror the same identity and reference
  model. They may expose full bodies only when DDx or the embedded runtime owns
  that data.

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
6. Capture invocation evidence: runtime metadata, native session identifiers,
   tool-call references where available, and transcript references when DDx or
   the embedded runtime owns them.
7. Resolve applicable execution documents from the document graph inside the
   execution worktree (see FEAT-007). **Execution documents are resolved from
   the base revision before the agent runs.** If the agent modifies execution
   document definitions or ratchet thresholds during its run, the pre-run
   versions govern the current iteration's evaluation.
8. Run all required execution documents plus relevant metric/observation
   executions.
9. Evaluate required execution results and metric ratchets (see TD-005).
   - For `kind: command` executions, success means exit code 0.
   - For `kind: agent` executions, success means exit code 0 (structured result schema validation is optional and governed by the definition).
   - When a `required: true` execution also has a ratchet threshold, landing is blocked if EITHER condition fails (OR semantics) — non-success status OR ratchet regression blocks the merge.
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
hidden ref). The commit message contains a structured JSON trailer block
with the minimum surface needed for post-hoc evaluation without opening
session attachments:

```
ddx-iteration: {
  "bead_id": "ddx-abc12345",
  "session_id": "agent-1744128000000",
  "harness": "agent",
  "model": "qwen3.5-27b",
  "total_tokens": 12500,
  "cost_usd": 0,
  "base_rev": "418a646def01",
  "result_rev": "63f71eeabc12",
  "required_exec_summary": "pass",
  "ratchet_summary": "ok",
  "outcome": "landed"
}
```

Field notes:
- `cost_usd`: `0` for local models; `-1` when the harness does not report cost
- `required_exec_summary`: `"pass"`, `"fail"`, or `"skipped"` (no exec docs found)
- `ratchet_summary`: `"ok"`, `"warn"`, or `"blocked"`
- `outcome`: `"landed"` or `"preserved"`

Full conversation transcripts and detailed session evidence remain in
provider-native stores or embedded-runtime telemetry stores. DDx keeps routing
facts, runtime metadata, and references rather than duplicating those bodies in
git history.

## Comparison Dispatch

DDx's existing quorum mechanism runs the same prompt through multiple
harnesses and checks for consensus. **Comparison dispatch** extends this
for evaluation: run the same prompt through multiple harnesses, capture
all outputs and side effects, and record structured comparison results.

```bash
# Compare agent (local model) against claude on the same task
ddx agent run --compare --harnesses=agent,claude --prompt task.md

# Each arm runs in an isolated worktree to capture side effects
ddx agent run --compare --harnesses=agent,claude --prompt task.md --sandbox
```

### Sandboxed comparison runs

When `--sandbox` is specified (or implied by `--compare`), each harness
arm runs in a temporary git worktree:

1. Create a worktree per harness: `.worktrees/compare-<id>-<harness>/`
2. Run the agent in that worktree (agent: `WorkDir`, subprocess: `WorkDirFlag`)
3. After completion, capture `git diff` as the "effect artifact"
4. Record: prompt, output text, git diff, tool call log (agent), tokens, cost
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
      "harness": "agent",
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
- Routing-signal extraction and activity capture patterns
- Timeout and error handling
- Backward-compatible metadata replay

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
ddx agent run --profile=cheap --prompt task.md      # invoke agent via routing
ddx agent run --model qwen3 --effort high           # exact model ref / effort
ddx agent run --harness=codex --prompt task.md      # explicit harness override
ddx agent run --quorum=majority --harnesses=a,b     # multi-agent
ddx agent run --automation=plan                      # control autonomy
ddx agent execute-bead <bead-id>                    # canonical bead execution workflow
ddx agent execute-bead <bead-id> --from <rev>       # use specific git base
ddx agent execute-bead <bead-id> --no-merge         # preserve iteration without landing
ddx agent list                                       # available harnesses
ddx agent capabilities codex                         # inspect harness capabilities
ddx agent doctor                                     # harness health
ddx agent log                                        # recent DDx invocation activity
ddx agent log <session-id>                           # metadata + native refs for one invocation
```

### Configuration

```yaml
# .ddx/config.yaml
agent:
  profile: cheap                    # default routing intent
  harness: ""                       # optional forced harness override
  model: ""                         # optional model ref or exact pin
  models:                           # per-harness model overrides
    claude: claude-sonnet-4-20250514
  reasoning_levels:                 # per-harness reasoning-level overrides
    codex: [low, medium, high]
  timeout_ms: 300000                # 5 minute default
  automation: auto                  # manual|plan|auto|yolo
  session_log_dir: .ddx/agent-logs  # DDx invocation activity location
```

## Migration Strategy

HELIX and dun have working agent dispatch today. The transition to `ddx agent` must be incremental so nothing breaks during migration.

**Phase 1 — DDx ships basic agent invocation.** `ddx agent run --harness=codex --prompt file.md` works for the single-harness, single-invocation case. Quorum can follow.

**Phase 2 — HELIX and dun add `ddx agent` as an alternative path.** Both tools detect whether `ddx agent` is available. If yes, use it. If no, fall back to their existing harness code. Controlled via env var (`DDX_AGENT=1`) or config.

**Phase 3 — Prove parity.** Run both paths in parallel on real work. Verify output capture, token tracking, native-session correlation, and routing signals match expectations.

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

| Profile | codex flags | claude flags | opencode flags | agent behavior |
|---------|------------|--------------|----------------|----------------|
| safe | (none — default codex behavior) | (none — default claude behavior) | (none — `run` auto-approves) | Tools always execute (embedded) |
| supervised | `--auto-approve-reads` | `--permission-mode default` | (none — no granular control) | Tools always execute (embedded) |
| unrestricted | `--dangerously-bypass-approvals-and-sandbox` | `--permission-mode bypassPermissions --dangerously-skip-permissions` | (none — `run` auto-approves) | Tools always execute (embedded) |

> **Note:** opencode's `run` subcommand auto-approves all tool permissions in
> non-interactive mode. The embedded agent harness runs in-process with direct tool execution — there
> is no permission layer. Both behave as effectively unrestricted.

**Safety invariant:** If `agent.permissions` is not explicitly set in config
AND the `--permissions` flag is not provided, DDx defaults to `safe` and
logs a one-time notice explaining the available modes.

## Provider Usage Data Sources

Each harness exposes different levels of usage data. DDx consumes what is
available and uses it for usage reporting, routing, and future throttling
policy (FEAT-014).

| Source | codex | claude | opencode | agent (embedded) |
|--------|-------|--------|----------|-----------------|
| **Per-invocation tokens** | `turn.completed` JSONL: `input_tokens`, `cached_input_tokens`, `output_tokens` | JSON envelope: `usage.input_tokens`, `output_tokens`, `cache_read_input_tokens`, `cache_creation_input_tokens` | JSON envelope: `usage.input_tokens`, `output_tokens` (if present) | Direct from agent result tokens — `Input`, `Output`, `Total` |
| **Per-invocation cost** | Not reported | `total_cost_usd` in JSON envelope | `total_cost_usd` (if present) | Agent result `CostUSD` — built-in pricing table; $0 for local models, -1 for unknown |
| **Per-invocation model info** | Not reported | `modelUsage` block: per-model token breakdown, `contextWindow`, `maxOutputTokens` | Not reported | Agent result `Model` — provider-reported model name |
| **Historical usage** | Native session JSONL / local state when persistence is enabled | `~/.claude/stats-cache.json`: daily activity, daily tokens by model, cumulative model usage | None known | Embedded runtime telemetry / session events |
| **Current quota/headroom** | Native session JSONL `token_count.rate_limits` when persistence is enabled | No stable non-PTY source confirmed yet; investigate statusline/SDK/hook payloads before PTY fallback | Not exposed | Provider/backend specific telemetry when available |
| **Budget passthrough** | None | `--max-budget-usd` flag (per-session cap) | None | `MaxIterations` in agent request |

### Key implications for self-throttling (see FEAT-014)

- **Claude** has real cost per invocation and a historical stats file. Current
  quota/headroom should use a stable non-PTY source if one exists; PTY-based
  extraction is a fallback of last resort, not the default design.
- **Codex** exposes current rate-limit headroom in its native session JSONL
  when persistence is enabled. DDx should prefer that native source over PTY
  `/status` scraping.
- **opencode** has JSON output support but token/cost reporting in the
  envelope is not yet confirmed in all versions. DDx parses opportunistically.
- **Agent (embedded)** is the richest for DDx because it is embedded: typed
  result struct with exact tokens, cost, model, session ID, and runtime
  telemetry. DDx should consume the correlation and metrics it needs without
  duplicating the runtime's owned logs.

## Out of Scope

- **Autonomy semantics** — DDx does not define what autonomy levels mean behaviorally; that is delegated to workflow tools
- **Workflow routing and orchestration** — DDx does not decide when to invoke execute-bead, what to do with the outcome, or how to sequence workflow phases; that is delegated to workflow tools
- **Escalation and supervisory policy** — follow-on bead creation, stop/continue rules, and conflict escalation are workflow tool concerns
- **Prompt design and engineering strategy** — bead prompt structure, prompt optimization, and rubric content are delegated to plugins; DDx provides the dispatch and grading mechanics
- **Server-side agent dispatch** — `ddx agent run` is CLI-only for security.
  The localhost-only dispatch endpoints in FEAT-002 (items 40-41) delegate to
  the CLI internally and require API key for non-local access.
- Building or hosting AI agents
- Model fine-tuning or prompt optimization
- Agent-to-agent communication protocols
- IDE integration for agent invocation
