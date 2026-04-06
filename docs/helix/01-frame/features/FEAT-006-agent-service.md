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

The DDx agent service is the unified interface for dispatching work to AI coding agents (codex, claude, gemini, opencode, cursor, etc.). It handles harness discovery, prompt delivery, output capture, token tracking, session logging, and multi-agent quorum. Workflow tools (HELIX) and check runners (dun) call `ddx agent` instead of implementing their own harness abstraction.

Within the broader DDx execution model (FEAT-010), `ddx agent` is the
dedicated `agent` executor kind. It remains the canonical surface for direct
agent dispatch and the authoritative source of raw prompt/response logs for
agent-backed execution runs.

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

1. **Harness registry** — built-in support for codex, claude, gemini, opencode, pi, cursor. Extensible via config.
2. **Harness discovery** — detect which harnesses are available on the system (binary exists, authenticated, etc.). Cache results.
3. **Agent invocation** — `ddx agent run --harness=<name>` sends a prompt to the specified agent and captures output.
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
**As** HELIX (or another workflow tool)
**I want** to call `ddx agent run --harness=codex --prompt task.md`
**So that** I don't need my own agent dispatch code

**Acceptance Criteria:**
- Given codex is available, when HELIX calls `ddx agent run`, then the prompt is sent and the response is captured
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

| Profile | codex flags | claude flags |
|---------|------------|--------------|
| safe | (none — default codex behavior) | (none — default claude behavior) |
| supervised | `--auto-approve-reads` | `--permission-mode default` |
| unrestricted | `--dangerously-bypass-approvals-and-sandbox` | `--permission-mode bypassPermissions --dangerously-skip-permissions` |

**Safety invariant:** If `agent.permissions` is not explicitly set in config
AND the `--permissions` flag is not provided, DDx defaults to `safe` and
logs a one-time notice explaining the available modes.

## Out of Scope

- **Server-side agent dispatch** — `ddx agent run` is CLI-only for security.
  The localhost-only dispatch endpoints in FEAT-002 (items 40-41) delegate to
  the CLI internally and require API key for non-local access.
- Building or hosting AI agents
- Model fine-tuning or prompt optimization
- Agent-to-agent communication protocols
- IDE integration for agent invocation
