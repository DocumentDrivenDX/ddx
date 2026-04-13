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
- DDx orchestration config in `.ddx/config.yaml`: harness selection, timeout,
  session log dir, permissions, and DDx routing controls (not provider config)

**Integration rules:**
- DDx calls the agent library run function and maps the result to `agent.Result`
- Native `.agent/config.yaml` is the authoritative source for embedded-harness
  provider config (provider type, base URL, API key, model, preset, max
  iterations). See "Embedded Harness Configuration Ownership" below.
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

## Embedded Harness Configuration Ownership

The embedded `agent` harness resolves its provider configuration through a
layered precedence chain. The design goal is that `.agent/config.yaml` (the
native ddx-agent runtime config) is the single authoritative source for
provider-level settings. DDx orchestration config (`.ddx/config.yaml`) owns
only cross-harness routing controls and must not duplicate provider-level fields
from the native runtime config.

### Authoritative Config Source

**`.agent/config.yaml`** (native ddx-agent runtime config) is the authoritative
source for embedded-harness provider configuration:

- Provider type (`openai-compat`, `anthropic`)
- Base URL / endpoint
- API key
- Default model
- Prompt preset
- Max iterations
- Named provider definitions with OpenRouter and multi-endpoint support

This file is loaded from `.agent/config.yaml` in the working directory or
`~/.config/agent/config.yaml` as a global fallback.

### Precedence Chain

Config resolution for one embedded-agent invocation applies layers in this
order (highest priority first):

1. **CLI flags** (`--model`, `--harness`, `--effort`, etc.) — always win
2. **Native `.agent/config.yaml`** — authoritative provider config when present
3. **`.ddx/config.yaml` `agent_runner` section** — deprecated fallback mirror;
   used only when no native config file exists. See "Migration" below.
4. **Built-in defaults** — `openai-compat` provider, `http://localhost:1234/v1`
   base URL, `agent` preset, 100 max iterations

### Environment Variables

The `AGENT_*` env vars (`AGENT_MODEL`, `AGENT_PROVIDER`, `AGENT_BASE_URL`,
`AGENT_API_KEY`, `AGENT_PRESET`) currently apply only in the `.ddx/config.yaml`
fallback path. They are **not** forwarded into the native config path. This is a
known gap to be closed as part of the `agent_runner` mirror removal work.

Native agent config supports `$ENV_VAR` expansion inside the YAML file itself;
use that pattern when targeting the native config path.

### Special Cases

- **OpenRouter models** (vendor/model format, e.g. `qwen/qwen3.6`): when the
  requested model contains a `/`, DDx prefers the `openrouter` provider from
  native config if present, falling back to default provider otherwise.
- **Named legacy presets** (e.g. `qwen-local` from `.ddx/config.yaml`
  `agent_runner.models`): when the model name matches a named preset in
  `.ddx/config.yaml` and a native config is also present, the legacy preset
  takes precedence to preserve backward compatibility. This behavior is
  deprecated and will be removed with the `agent_runner` mirror.

### DDx Orchestration Config (`.ddx/config.yaml`)

The `agent` section of `.ddx/config.yaml` owns DDx-level routing controls only:

```yaml
agent:
  harness: claude          # default harness for ddx agent run
  model: gpt-4o            # per-harness model override (routing hint)
  timeout_ms: 300000       # invocation timeout
  session_log_dir: ...     # where DDx writes invocation metadata
  permissions: safe        # automation level for subprocess harnesses
  routing:                 # cross-harness routing policy
    profile_priority: [cheap]
    default_harness: agent
    model_overrides:
      cheap: qwen/qwen3.6
```

The `agent_runner` sub-section is a **deprecated lossy mirror** of native
`.agent/config.yaml` fields. New deployments should use `.agent/config.yaml`
instead. The `agent_runner` section will be removed once all known usages have
migrated.

### Migration

To migrate from `.ddx/config.yaml` `agent_runner` to native `.agent/config.yaml`:

1. Create `.agent/config.yaml` with `providers`, `default`, `preset`, and
   `max_iterations` fields matching your current `agent_runner` values.
2. For OpenRouter, add an `openrouter` provider entry with your API key.
3. Remove the `agent_runner` block from `.ddx/config.yaml`.

The implementation work to remove the `agent_runner` mirror from DDx config
schema, apply `AGENT_*` env vars in the native path, and clean up the fallback
logic is tracked separately (see bead `ddx-remove-agent-runner-mirror`).

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
- Given a valid bead ID, when `ddx agent execute-bead <id>` is invoked, then DDx resolves the bead and governing artifacts and begins the workflow.
- Given `--from` is omitted, when the base revision is resolved, then DDx uses `HEAD`.
- Given `--harness`, `--model`, and `--effort` are provided, when the agent runs, then execute-bead honors them exactly as a normal `ddx agent run` invocation would.
- Given graph-discovered required executions fail, when the merge decision is made, then DDx preserves the iteration under a hidden ref and does not fast-forward the target branch.
- Given all required executions pass and ratchets are satisfied, when the merge decision is made and `--no-merge` is not set, then DDx lands the result by fast-forward.
- Given `--no-merge` is set, when the iteration completes, then DDx creates a committed attempt and preserves it under a hidden ref. It is not landed regardless of execution outcomes.
- Given execution completes, when the worktree is cleaned up, then no temporary worktree created by execute-bead remains in the filesystem.
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

The single-project supervision contract for this workflow is documented in
`docs/helix/02-design/contracts/API-001-execute-bead-supervisor-contract.md`.
It keeps readiness validation, queue scanning, and worker lifecycle scoped to
one project context at a time.

This workflow is the canonical **single-ticket** execution primitive. It is
not the final contract for epic execution. Epics use a separate epic-scoped
worker mode because their branch/worktree lifecycle and merge policy are
different.

### Workflow steps

1. Resolve the git base revision: `--from <rev>` if provided, otherwise `HEAD`.
2. If tracked root state differs from `HEAD`, create a checkpoint commit first
   and use that checkpoint as the actual base. `execute-bead` requires a
   reproducible git base revision, not a pristine root checkout. Ignored or
   disposable runtime scratch must not block launch or contaminate the
   checkpoint snapshot.
3. Resolve the bead and its governing artifacts from the DDx document graph.
   Use the shared DDx execution validator against the resolved base revision
   and governing execution-contract snapshot to confirm the bead is
   structurally execution-ready before launch; HELIX-specific policy does not
   participate in readiness validation.
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
10. If the agent produced tracked worktree edits without creating commits,
    synthesize a DDx-owned result commit before merge/preserve evaluation so
    the work is not discarded merely because the harness left file edits
    uncommitted.
11. If merge-eligible and `--no-merge` is not set, land by rebase + fast-forward
    semantics, then reset the worker worktree to the updated branch tip.
12. Otherwise, preserve the iteration result under a hidden ref and do not merge
    (see SD-012 for the hidden-ref naming scheme).
13. Always remove the temporary worktree after preserving enough evidence for
    replay and introspection.

### Prompt Rationalizer Contract

Before launching the agent, `execute-bead` compiles a **rationalizer prompt**
from machine-readable inputs and writes it to `prompt.md` in the execution
bundle. This compilation is DDx-owned; it does not mutate the bead.

**Inputs (all from tracked sources):**

- bead fields: `id`, `title`, `description`, `acceptance`, `parent`, `labels`,
  `spec-id`, and any structured metadata
- resolved governing references from the document graph: artifact ID, path, and
  title for each governing document
- base git revision
- execution bundle path

**Prompt structure:**

The rationalizer emits a structured prompt wrapped in an `<execute-bead>`
root element with machine-significant XML-tagged sections so the prompt
can be parsed, validated, diffed, and replayed deterministically. The
required top-level sections are:

```
<execute-bead>
  <bead id="...">                          — identity fields (id as attribute; title, description, acceptance, labels, metadata as children)
    <title>...</title>                     — bead title verbatim
    <description>...</description>         — bead description verbatim
    <acceptance>...</acceptance>           — bead acceptance criteria verbatim
    <labels>...</labels>                   — comma-separated label list (empty element when none)
    <metadata parent="..." spec-id="..." base-rev="..." bundle="..."/>
                                           — parent, spec-id, base git revision, and execution bundle path
  </bead>
  <governing>                              — resolved governing artifact list
    <ref id="..." path="...">title</ref>   — one element per governing document
  </governing>
  <instructions>...</instructions>         — DDx-injected per-bead task instructions and execution conventions
</execute-bead>
```

The rationalizer must not invent unavailable fields; it emits empty
self-closing elements (e.g. `<description/>`) for missing content rather
than emitting placeholder text. When no governing references resolve,
the `<governing>` section contains a single `<note>` element describing
the discovery fallback instead of being omitted, so downstream tooling
can distinguish "no refs" from "section not emitted".

The prompt is the agent's primary input for the attempt. The rationalizer
contract is independent of the harness: every harness receives the same
compiled `prompt.md` regardless of whether it is a subprocess or embedded
runner.

**Prompt override:** If `--prompt-file` is provided, the rationalizer is
bypassed and the given file is used as-is. The bundle still records the
source as `prompt_source: override` in `manifest.json`.

### Execute-Bead Evidence Bundle

`execute-bead` produces a tracked attempt bundle under:

```text
.ddx/executions/<attempt-id>/
```

This bundle is distinct from the generic `exec-runs` substrate described in
FEAT-010.

- `exec-runs` / `exec-runs.d` remain the generic runtime store for reusable
  execution definitions and execution runs.
- `.ddx/executions/<attempt-id>/` is the tracked execute-bead attempt bundle
  used for replay, commit provenance, and autoresearch.

**Tracked artifact set (committed with the attempt):**

| File | When written | Purpose |
|------|-------------|---------|
| `prompt.md` | before agent runs | compiled rationalizer prompt |
| `manifest.json` | before agent runs | attempt identity, bead snapshot, governing refs, paths, prompt source |
| `result.json` | after agent finishes | runtime metrics, outcome, gate results, native session references |
| `checks.json` | when gates ran | per-gate results with stdout/stderr |
| `usage.json` | when harness reports usage | token counts, cost, model identity |

`result.json` and `checks.json` are the canonical machine-readable sources for
commit-message provenance. Commit-message metadata must be projected from these
files, never from ad hoc runtime state.

**Embedded-harness runtime state path:**

When execute-bead invokes the embedded `agent` harness, any session logs,
telemetry, or other runtime state the harness would ordinarily write to its
working directory is redirected into `embedded/` inside the execution bundle:

```text
.ddx/executions/<attempt-id>/embedded/
```

DDx owns the artifact location for managed runs: the embedded harness must
not accumulate root-level runtime state (e.g. `./.agent-session.json`,
loose `*.jsonl` session files) inside the execution worktree, because that
would contaminate the tracked worktree snapshot and the commit provenance
pipeline. The redirection is implemented by passing a per-run
`SessionLogDir` override from execute-bead to the embedded runner.

This is distinct from provider-native external stores (e.g. codex local
state, `~/.claude/projects/`): those remain outside DDx ownership. The
`embedded/` path applies only to the embedded DDx-agent harness where DDx
owns the runtime loop.

**Ignored runtime scratch (not committed, not tracked):**

The following paths are disposable and must not contaminate the checkpoint
snapshot or the tracked bundle:

- `.ddx/exec-runs.d/` — generic runtime execution history (append-only store)
- `.ddx/agent-logs/` — DDx invocation activity metadata
- `.ddx/.execute-bead-wt-*/` — ephemeral isolated execution worktrees
- provider-native session stores (e.g., `~/.claude/projects/`, codex local state)

These paths must be listed in the repository's `.gitignore` (or the DDx
default gitignore template) so they never accidentally enter the tracked snapshot.

**Default commit policy:**

- landed attempts: the `.ddx/executions/<attempt-id>/` bundle is committed with
  the landed work or in an immediately adjacent execution-evidence commit on the
  same branch
- preserved attempts: the same bundle exists on the preserved ref
- provider-native transcripts remain external by default; DDx records references
  (`native_session_id`, `native_log_ref`) rather than duplicating bodies

This split is intentional: the generic execution store remains append-only
runtime history, while execute-bead bundles are tracked git artifacts tied to a
specific implementation attempt.

### Output And Commit Contract

`execute-bead` evaluates the resulting tracked worktree state, not only
agent-authored commits.

- DDx owns landing and preservation.
- Agent-created commits are optional, not required.
- A run that leaves coherent tracked edits in the managed worktree still counts
  as produced work.
- A run is only `no_changes` when the managed worktree ends clean and there are
  no new tracked changes to preserve or land.
- If DDx needs a commit object for preserve/merge mechanics and the agent left
  only tracked edits, DDx synthesizes the result commit itself.

### Epic Execution Workflow

Epics are worked differently from single tickets.

- The ordinary `ddx agent execute-loop` worker prioritizes ready non-epic
  beads ahead of epics and does not launch open epics by default.
- An epic is consumed by an **epic-scoped worker** that owns one long-lived
  worktree and one branch for that epic.
- The epic branch is named after the epic, using the DDx-managed branch naming
  convention for epics such as `ddx/epics/<epic-id>`.
- The worker selects ready child beads within that epic and executes them
  sequentially in the same epic worktree.
- Each child bead lands as its own commit on the epic branch and may be
  closed individually when its acceptance and required gates pass in the epic
  branch context.
- Multiple epics may be worked in parallel, but each epic gets its own
  worktree/branch and worker.
- When the epic itself is ready to land, DDx runs the epic's merge gates on
  the merge candidate and integrates the epic branch to the target branch with
  a regular merge commit so the child commit history remains intact.

#### Epic worker contract

The epic worker obeys a fixed contract so its behavior is predictable from
outside the loop:

1. **Branch naming.** The epic branch is `ddx/epics/<epic-id>` and is the
   durable integration surface for the epic. The worker creates the branch
   from the resolved base revision of the target branch on first launch and
   reuses the existing branch on subsequent launches of the same epic.
2. **Persistent worktree.** One managed worktree is attached to the epic
   branch for the full lifetime of the epic worker. Child-bead executions do
   not create or destroy per-child worktrees. The worktree is removed only
   when the epic is merged, abandoned, or reset.
3. **Sequential child execution.** Children are executed one at a time in
   topological (dependency-respecting) queue order inside the epic worktree.
   The worker does not start the next child until the previous child has
   either been recorded as a child commit on the epic branch or been recorded
   as a failed attempt.
4. **Child commits.** Each child bead produces one ordinary commit on the
   epic branch. Child commits are never fast-forwarded directly to the target
   branch. A failed child does not block subsequent children unless the
   worker was launched with stop-on-failure semantics.
5. **Child close semantics.** A child bead may be closed as soon as its own
   acceptance and required gates pass inside the epic branch context. Closing
   a child mid-epic does not land that child's commit on the target branch;
   the child's work remains on the epic branch until the epic itself merges.
   A closed child whose commit still lives only on the epic branch is
   considered "closed on epic", not "landed on target".
6. **Epic merge gates.** Before the final merge, the worker evaluates the
   epic's aggregate merge gates on the merge candidate (the epic branch tip
   rebased onto the latest target tip). Merge gates must confirm that:
   all required children are closed, no in-flight child executions remain,
   no unresolved in-flight dependencies exist outside the epic, and any
   declared epic-level required executions pass on the merge candidate.
7. **Final merge.** When the gates pass, the epic branch is integrated into
   the target branch with a regular merge commit (`git merge --no-ff`),
   preserving the full child-commit history. Fast-forward landing is
   explicitly forbidden for epic branches so the epic remains visible as a
   single merge in target history.
8. **Cleanup.** After a successful merge (or an explicit abandon/reset), the
   epic worktree is removed and the epic branch may be deleted or retained
   per project policy. A failed merge gate preserves the epic branch and
   worktree for operator inspection rather than discarding work.

This means DDx owns two execution modes:

- single-ticket mode: isolated temporary worktree, linear rebase + fast-forward
- epic mode: persistent epic worktree, sequential child execution, regular
  merge commit for the epic branch

The epic worker reuses the same execution evidence, gate evaluation, and
result-recording model where possible, but it does not collapse the epic into a
series of independent fast-forward landings on the target branch.

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
  "attempt_id": "20260413T140544-6b4034a1",
  "session_id": "agent-1744128000000",
  "harness": "agent",
  "model": "qwen3.5-27b",
  "total_tokens": 12500,
  "cost_usd": 0,
  "base_rev": "418a646def01",
  "result_rev": "63f71eeabc12",
  "required_exec_summary": "pass",
  "ratchet_summary": "ok",
  "outcome": "landed",
  "execution_bundle": ".ddx/executions/20260413T140544-6b4034a1"
}
```

Field notes:
- `attempt_id`: matches the execution bundle directory name for cross-referencing
- `cost_usd`: `0` for local models; `-1` when the harness does not report cost
- `required_exec_summary`: `"pass"`, `"fail"`, or `"skipped"` (no exec docs found)
- `ratchet_summary`: `"ok"`, `"warn"`, or `"blocked"`
- `outcome`: `"landed"` or `"preserved"`
- `execution_bundle`: relative path to the tracked evidence bundle for this attempt

**Provenance rule:** All values in the `ddx-iteration` trailer must be
projected from the tracked artifact files in `.ddx/executions/<attempt-id>/`
(primarily `result.json`), never from ad hoc in-memory runtime state. The
purpose is reproducibility: anyone can re-derive the commit trailer from the
tracked bundle without re-running the agent.

#### Canonical Git trailers

In addition to the structured `ddx-iteration` JSON block, every execute-bead
iteration commit (landed or preserved) carries a fixed set of canonical Git
trailer lines. These trailers are the minimum provenance surface that any
tool — including `git log`, `git interpret-trailers`, and other specs that
reference execute-bead commit provenance — can rely on without parsing the
JSON block:

```
Ddx-Attempt-Id: 20260413T140544-6b4034a1
Ddx-Worker-Id: <worker-id>
Ddx-Harness: agent
Ddx-Model: qwen3.5-27b
Ddx-Result-Status: success
```

Trailer semantics:

- `Ddx-Attempt-Id` — matches the `.ddx/executions/<attempt-id>/` bundle
  directory name and the JSON block's `attempt_id` field.
- `Ddx-Worker-Id` — stable identifier for the worker instance that ran the
  attempt. For a local run this is the DDx session or host-scoped worker ID;
  for a server-hosted run this is the server's worker identity. When no
  distinct worker identity exists, DDx uses the session ID from
  `result.json`.
- `Ddx-Harness` — harness name as recorded in `result.json` (e.g. `agent`,
  `claude`, `codex`).
- `Ddx-Model` — resolved model identity as recorded in `result.json`.
- `Ddx-Result-Status` — one of the documented execute-bead result statuses
  (`success`, `execution_failed`, `post_run_check_failed`, `land_conflict`,
  `structural_validation_failed`, `no_changes`, `already_satisfied`).

Like the JSON block, all trailer values must be projected from the tracked
artifact files in `.ddx/executions/<attempt-id>/` — never from ad hoc
in-memory runtime state. The trailers and the JSON block are two views of
the same underlying result and must agree by construction. Other specs
(API-001, FEAT-012, FEAT-014) that reference commit provenance refer to
this trailer set as the canonical contract.

Full conversation transcripts and detailed session evidence remain in
provider-native stores or embedded-runtime telemetry stores. DDx keeps routing
facts, runtime metadata, and references rather than duplicating those bodies in
git history. The tracked exception is the execute-bead attempt bundle under
`.ddx/executions/<attempt-id>/`, which stores the normalized prompt, manifest,
result, checks, and provenance pointers for one implementation attempt.

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
