---
ddx:
  id: FEAT-002
  depends_on:
    - helix.prd
    - FEAT-004
    - FEAT-010
    - FEAT-007
    - FEAT-012
    - FEAT-020
---
# Feature: DDx Server

**ID:** FEAT-002
**Status:** Complete
**Priority:** P0
**Owner:** DDx Team

## Overview

`ddx-server` is a lightweight Go web server that exposes DDx platform services
over HTTP and MCP endpoints. It serves documents, beads, execution definitions
and run history, the document dependency graph, DDx agent invocation activity
plus embedded-agent telemetry references, and (via FEAT-008) an embedded web
UI — all from a single binary.

`ddx-server` runs as a per-user host daemon: one server process per machine,
scoped to one operating-system user. It holds its identity and project
registry in user-level state — not inside any project — and supervises
execute-loop workers for every project it has registered. The state file lives
at `~/.local/share/ddx/server-state.json` (`$XDG_DATA_HOME/ddx/server-state.json`
when set), and the last-known server URL is written to
`~/.local/share/ddx/server.addr` (see FEAT-020). The server binds multiple
project roots concurrently; each request is resolved against one explicit
project context before adapters run.

## Architecture

```
ddx-server binary
├── /projects  → Project registry / project picker
├── /            → Web UI (embedded SPA, FEAT-008)
├── /api/        → HTTP REST API (JSON)
└── /mcp/        → MCP tool endpoints (Streamable HTTP transport)
```

All three surfaces share the same underlying services. The web UI calls the HTTP API. MCP tools call the same service layer.

### Project Registry and Routing

`ddx-server` resolves every request against an explicit project context
before dispatching to feature-specific adapters.

**Registry contract**

- the server holds one project registry per host+user, persisted at
  `~/.local/share/ddx/server-state.json` as specified in FEAT-020
- projects enter the registry by auto-registration from CLI commands
  (`ddx bead`, `ddx agent`, `ddx doc`) running in any project directory on
  the machine, and by the server registering its own startup working directory
- optional seed entries may be provided via `server.projects` in config so a
  fresh install can boot with a known project list before any CLI commands run
- each entry carries a stable `id`, an absolute `root`, an optional display
  `name`, a `git_remote`, and `registered_at`/`last_seen` timestamps
- if the state file is absent, the server synthesizes a singleton project from
  the current working directory so today's `ddx server` invocation still works

**Canonical project-scoped surfaces**

- `GET /api/projects` - list configured project roots and health
- `GET /api/projects/:project` - show one project context
- `ddx_list_projects` - enumerate projects over MCP
- `ddx_show_project` - show one project context over MCP

**Selection rules**

1. explicit project in the request path
2. explicit project supplied by the caller
3. configured default project
4. implicit singleton project when only one project exists

Legacy unscoped `/api/...` and `/mcp/...` forms remain only as compatibility aliases when the server can resolve exactly one project context. They are not the canonical multi-project contract.

### Worker Boundaries

`ddx-server` hosts an in-process `WorkerManager` that supervises
execute-loop workers as goroutines. Each execute-loop worker runs a
`ddx agent execute-loop` against exactly one registered project context; it
never crosses project boundaries. Worker lifecycle (start, live logs, stop,
record on disk) is owned by the server, so the host+user daemon is the single
point of coordination for all long-running agent activity on the machine. The
supervisor exposes worker state through the same project-scoped API surface
used for beads and executions, and worker records persist under the project's
own `.ddx/workers/` directory so preservation and cleanup stay scoped per
project.

### Replay-Backed Execution Evidence

Execute-bead attempts store their normalized prompt, manifest, transcript, and
runtime metrics under `.ddx/executions/<attempt-id>/` inside the project that
owns the bead (see FEAT-006). These bundles are the replay-backed source of
truth that server endpoints and dashboards read to reconstruct attempt history;
the server does not own a separate transcript store.

## Requirements

### Functional

Unless otherwise noted, the canonical resource routes below are project-scoped
at `/api/projects/:project/...`, and project-aware MCP tools resolve against an
explicit or selected project context. Legacy unscoped `/api/...` and `/mcp/...`
forms remain only as singleton compatibility aliases when the server can
resolve exactly one project context.

**Document Library**
1. `GET /api/projects/:project/documents` — list documents by category
2. `GET /api/projects/:project/documents/:path` — read document content
3. `GET /api/projects/:project/search?q=<query>` — full-text search across document contents
4. `GET /api/projects/:project/personas/:role` — resolve persona for role from project config
5. MCP tools: `ddx_list_documents`, `ddx_read_document`, `ddx_search`, `ddx_resolve_persona` (project selector required unless singleton compatibility mode applies)

**Bead Tracker (FEAT-004)**
6. `GET /api/projects/:project/beads` — list beads with optional status/label filters
7. `GET /api/projects/:project/beads/:id` — show one bead with all fields
8. `GET /api/projects/:project/beads/ready` — list ready beads (no unclosed deps)
9. `GET /api/projects/:project/beads/blocked` — list blocked beads
10. `GET /api/projects/:project/beads/status` — summary counts
11. `GET /api/projects/:project/beads/dep/tree/:id` — dependency tree for a bead
12. MCP tools: `ddx_list_beads`, `ddx_show_bead`, `ddx_bead_ready`, `ddx_bead_status` (project selector required unless singleton compatibility mode applies)

**Document Graph (FEAT-007)**
13. `GET /api/projects/:project/docs/graph` — full dependency graph as JSON
14. `GET /api/projects/:project/docs/stale` — list stale documents
15. `GET /api/projects/:project/docs/:id` — document metadata and staleness status
16. `GET /api/projects/:project/docs/:id/deps` — upstream dependencies
17. `GET /api/projects/:project/docs/:id/dependents` — downstream dependents
18. MCP tools: `ddx_doc_graph`, `ddx_doc_stale`, `ddx_doc_show`, `ddx_doc_deps` (project selector required unless singleton compatibility mode applies)

**Agent Activity (FEAT-006)**
19. `GET /api/projects/:project/agent/sessions` — list recent DDx agent invocations
20. `GET /api/projects/:project/agent/sessions/:id` — invocation detail, including native
    session/trace references and any DDx-owned transcript data
21. MCP tool: `ddx_agent_sessions` (project selector required unless singleton compatibility mode applies)

**Worker Progress (FEAT-006 embedded-agent progress contract)**

Worker state is read from the WorkerManager's in-memory registry plus the
per-project `.ddx/workers/` directory. The WorkerManager is the single
authoritative source for live phase state; the on-disk records are the
authoritative source for historical phase summaries.

22. `GET /api/projects/:project/workers` — list active and recently completed
    workers for a project; each entry includes worker identity, current status
    (`idle` | `running` | `stopped` | `error`), and, when a run is in flight, the
    current attempt identity and phase.
23. `GET /api/projects/:project/workers/:id` — worker detail including:
    - worker identity (`id`, `project_id`, `started_at`)
    - current status and current attempt fields (same as list entry)
    - `recent_phases`: ordered list of the last N phase-transition events for
      the current or most recent attempt, so the UI can show a phase timeline
      without a live stream connection
    - `last_attempt`: summary of the most recently completed attempt with
      outcome, duration, and token totals
24. `GET /api/projects/:project/workers/:id/progress` — Server-Sent Events
    stream of live progress events for the running worker. Each event is a
    JSON-encoded progress event as defined in FEAT-006 "Embedded-Agent Progress
    Events". The stream delivers phase-transition and heartbeat events in
    real time. When no run is active the stream stays open and emits a
    keepalive comment line (`": keepalive"`) at the configured heartbeat interval
    so clients can distinguish a quiet-but-alive worker from a dropped connection.
    When the attempt reaches a terminal phase (`done`, `preserved`, or `failed`),
    the server emits the final event and closes the stream.
    - Polling alternative: clients that cannot use SSE may poll
      `GET /api/projects/:project/workers/:id` at their own interval and use the
      `recent_phases` field to reconstruct phase history.
25. MCP tool: `ddx_worker_list`, `ddx_worker_show` — project-scoped worker
    state reads (project selector required unless singleton compatibility mode applies).
    MCP tools do not expose the SSE stream; polling via `ddx_worker_show` is the
    MCP-compatible alternative.

Worker state object structure (both list and detail responses embed the same
shape for the in-flight attempt, enabling a single read model across CLI and UI):

```json
{
  "id":         "worker-abc123",
  "project_id": "proj-xyz",
  "status":     "running",
  "current_attempt": {
    "attempt_id": "20260413T140544-6b4034a1",
    "bead_id":    "ddx-abc12345",
    "bead_title": "Add structured progress output",
    "harness":    "agent",
    "model":      "qwen3.5-27b",
    "profile":    "cheap",
    "phase":      "running",
    "phase_seq":  3,
    "started_at": "2026-04-14T05:09:51Z",
    "elapsed_ms": 45000,
    "tokens":     { "input": 8500, "output": 350, "total": 8850 },
    "cost_usd":   0
  },
  "recent_phases": [
    { "phase": "queueing",  "ts": "2026-04-14T05:09:00Z", "phase_seq": 1 },
    { "phase": "launching", "ts": "2026-04-14T05:09:10Z", "phase_seq": 2 },
    { "phase": "running",   "ts": "2026-04-14T05:09:51Z", "phase_seq": 3 }
  ],
  "last_attempt": null
}
```

`recent_phases` retains only phase-transition events (not heartbeats) and is
capped at the last 20 entries. This is the shared read model for both the CLI
`ddx agent log --worker` command and the UI status dashboard worker cards.

**Provider Availability and Utilization (FEAT-014)**

The provider dashboard endpoints expose the same normalized routing signal model that `ddx agent run` uses for harness selection. They are read-only; all signal data is derived from provider-native sources and DDx-observed metrics, never fabricated. Unknown values are surfaced as `unknown`, not omitted.

26. `GET /api/providers` — list all configured harnesses with current routing availability, auth/health state, quota/headroom, and signal freshness; not scoped to a project (provider config is host+user global, shared across projects). Response is an array of provider summary objects.
27. `GET /api/providers/:harness` — detail for one harness: full routing signal snapshot, per-model quota/headroom when available, historical usage summary (last 7d / 30d), recent latency/success rates, burn estimate, and freshness timestamps with source attribution.

Provider summary object (list response):

```json
{
  "harness":        "claude",
  "display_name":   "Claude (Anthropic)",
  "status":         "available",
  "auth_state":     "authenticated",
  "quota_headroom": "unknown",
  "signal_sources": ["stats-cache"],
  "freshness_ts":   "2026-04-14T05:00:00Z",
  "last_checked_ts":"2026-04-14T05:00:00Z",
  "recent_success_rate": 0.97,
  "recent_latency_p50_ms": 4200,
  "cost_class":     "subscription"
}
```

Provider detail object (`/api/providers/:harness`):

```json
{
  "harness":        "claude",
  "display_name":   "Claude (Anthropic)",
  "status":         "available",
  "auth_state":     "authenticated",
  "models": [
    {
      "model":          "claude-sonnet-4-6",
      "quota_headroom": "unknown",
      "source":         "none",
      "source_note":    "no stable non-PTY quota source confirmed"
    }
  ],
  "historical_usage": {
    "window_7d": {
      "input_tokens":  420000,
      "output_tokens": 38000,
      "total_tokens":  458000,
      "cost_usd":      0,
      "cost_note":     "subscription plan; per-token cost not billed"
    },
    "window_30d": {
      "input_tokens":  1800000,
      "output_tokens": 160000,
      "total_tokens":  1960000,
      "cost_usd":      0
    }
  },
  "burn_estimate": {
    "daily_token_rate":  65400,
    "subscription_burn": "moderate",
    "source":            "stats-cache+ddx-metrics",
    "confidence":        "low",
    "freshness_ts":      "2026-04-14T05:00:00Z"
  },
  "routing_signals": {
    "availability":   "available",
    "request_fit":    "capable",
    "cost_estimate":  "unknown",
    "performance": {
      "p50_latency_ms":  4200,
      "p95_latency_ms":  9800,
      "success_rate":    0.97,
      "sample_count":    34,
      "window":          "7d"
    }
  },
  "signal_sources": ["stats-cache", "ddx-metrics"],
  "freshness_ts":   "2026-04-14T05:00:00Z"
}
```

Field semantics:
- `status`: `available` | `unavailable` | `unknown` — routing-level reachability
- `auth_state`: `authenticated` | `unauthenticated` | `unknown`
- `quota_headroom`: `ok` | `blocked` | `unknown` — never fabricated; `unknown` means no trustworthy live source
- `signal_sources`: which sources contributed to this snapshot (`stats-cache`, `native-session-jsonl`, `ddx-metrics`, `none`)
- `freshness_ts`: when the oldest contributing signal was last observed
- `burn_estimate.confidence`: `high` (live source, recent) | `medium` (cached, recent) | `low` (cached, stale or inferred)
- `cost_usd`: `-1` when unknown; `0` for local models or subscription plans where per-token billing is unavailable

MCP tools: `ddx_provider_list`, `ddx_provider_show` — host+user global; not project-scoped (provider config is per host+user, not per project).

**Executions (FEAT-010)**
22. `GET /api/projects/:project/exec/definitions` — list execution definitions with optional artifact filter
23. `GET /api/projects/:project/exec/definitions/:id` — show one execution definition
24. `GET /api/projects/:project/exec/runs` — list execution runs with optional artifact/definition/status filters
25. `GET /api/projects/:project/exec/runs/:id` — show one execution run with structured result metadata
26. `GET /api/projects/:project/exec/runs/:id/log` — show raw captured logs for one execution run
27. MCP tools: `ddx_exec_definitions`, `ddx_exec_show`, `ddx_exec_history` (project selector required unless singleton compatibility mode applies)

**Configuration**
28. Library path, optional `server.projects` seed registry, port, optional
    ts-net hostname via CLI flags or config file (see ADR-006, SD-019, and
    FEAT-020). Runtime state (`server-state.json`, `server.addr`) lives at the
    host+user level in `~/.local/share/ddx/`, not inside any project.
29. Default: localhost only, no auth required
30. One server per machine: the addr file is single-entry and a new
    `ddx server` overwrites any prior entry (see FEAT-020)

### Non-Functional

- **Performance:** Document reads <200ms, search <500ms, graph build <500ms for 100+ documents
- **Stateless:** Reads from filesystem on each request. No database.
- **Single binary:** Embeds web UI (FEAT-008) via `embed.FS`
- **Security:** Localhost-only by default. Optional ts-net (Tailscale) listener for non-local access (ADR-006).

**Bead Mutations (FEAT-008 UI interaction)**
33. `POST /api/projects/:project/beads` — create a bead
34. `PUT /api/projects/:project/beads/:id` — update bead fields (status, labels, description, etc.)
35. `POST /api/projects/:project/beads/:id/claim` — claim a bead for the current session
36. `POST /api/projects/:project/beads/:id/unclaim` — release a claim
37. `POST /api/projects/:project/beads/:id/reopen` — re-open a closed bead with a reason
38. `POST /api/projects/:project/beads/:id/deps` — add/remove dependencies
39. MCP tools: `ddx_bead_create`, `ddx_bead_update`, `ddx_bead_claim` (project selector required unless singleton compatibility mode applies)

**Execution Dispatch (UI-initiated, localhost-only)**
40. `POST /api/projects/:project/exec/run/:id` — dispatch an execution run (delegates to
    `ddx exec run` internally). Localhost-only or via ts-net (ADR-006) for
    non-local access.
41. `POST /api/projects/:project/agent/run` — dispatch an agent invocation with harness,
    model, effort, and prompt. Localhost-only or via ts-net (ADR-006) for non-local access.
42. MCP tools: `ddx_exec_dispatch`, `ddx_agent_dispatch` (project selector required unless singleton compatibility mode applies; localhost-only)

## Technology

| Component | Choice | Reference |
|-----------|--------|-----------|
| HTTP routing | Chi or net/http | ADR-001 |
| MCP transport | mcp-go (Streamable HTTP) | |
| Embedded web UI | Go embed.FS | ADR-002 |
| Frontend | Vite + React + Tailwind | ADR-002, FEAT-008 |

## Dependencies

- FEAT-004 (Beads) — bead endpoints read from bead store
- FEAT-010 (Executions) — execution endpoints read definitions and immutable run history
- FEAT-007 (Doc Graph) — graph/stale endpoints use doc graph engine
- FEAT-006 (Agent Service) — agent activity endpoints read DDx invocation
  metadata and embedded telemetry references; execute-bead attempt artifacts
  live in each project's `.ddx/executions/<attempt-id>/` bundle
- FEAT-008 (Web UI) — embedded SPA served at `/`; provider dashboard view consumes `/api/providers`
- FEAT-014 (Agent Usage Awareness and Routing Signals) — provider availability
  and utilization endpoints expose the same routing signal model governed by
  FEAT-014; field semantics, unknown-state rules, and freshness conventions
  are owned by FEAT-014
- FEAT-020 (Server Node State) — host+user state file, addr file, and
  project auto-registration
- FEAT-021 (Dashboard UI) — per-project beads/sessions/graph surfaced under
  `/nodes/:nodeId/projects/:projectId/...`
- mcp-go SDK for MCP transport

**Document Write + Commit (FEAT-012)**
43. `PUT /api/docs/:id` — write document content and auto-commit
44. MCP tool: `ddx_doc_write` — write document by artifact ID, commit by
    default
45. `GET /api/docs/:id/history` — document commit history
46. `GET /api/docs/:id/diff` — document content diff between refs
47. MCP tools: `ddx_doc_history`, `ddx_doc_diff`, `ddx_doc_changed`

Write endpoints commit by default (configurable via `git.auto_commit` in
`.ddx/config.yaml`). Commit messages follow the structured format defined
in FEAT-012.

## Service Manager Integration

`ddx-server` runs as a user-level background service on each supported
platform. Service manager integration is always **user-level** for this
phase; machine-level (root/LaunchDaemon) installs are explicitly out of
scope. The contract below covers Linux (systemd) and macOS (launchd) and
is the template for future platforms.

Shared contract across platforms:

- **Working directory:** the project root supplied at install time, or the
  user's home directory if no project root is configured
- **State location:** unchanged from FEAT-020 — `~/.local/share/ddx/`, or
  `$XDG_DATA_HOME/ddx` when set. State and address files never live inside
  the service-manager unit directory
- **Environment overrides:** `DDX_NODE_NAME`, `DDX_DATA_HOME` (when used),
  and any TLS certificate path overrides are passed through to the server
  process by the service manager
- **Restart on crash:** the service manager must restart the server on
  unclean exit with a minimum back-off to prevent tight crash loops
- **Start on login/boot:** the service must start automatically when the
  user's session starts
- **Lifecycle parity:** install enables and starts the service; uninstall
  disables, stops, and removes the unit; status reports the running state
  and recent exit reason

### Linux (systemd user unit)

- **Unit path:** `~/.config/systemd/user/ddx-server.service`
- **Working directory:** the project root passed via `--workdir`, defaulting
  to the current directory at install time
- **Logs:** `<workdir>/.ddx/logs/ddx-server.log` via `StandardOutput=append:`
  and `StandardError=append:` (both streams share one file). `journalctl
  --user -u ddx-server -f` remains available for live tailing
- **Environment file:** `<workdir>/.ddx/server.env`, written with mode
  `0600` at install time. Loaded through systemd `EnvironmentFile=`
- **Restart policy:** `Restart=on-failure`, `RestartSec=5`
- **Lifecycle commands:**
  - install/enable: `systemctl --user daemon-reload && systemctl --user enable --now ddx-server.service`
  - disable/remove: `systemctl --user disable --now ddx-server.service`
  - status: `systemctl --user status ddx-server.service`
  - restart: `systemctl --user restart ddx-server.service`
- **Install target:** `WantedBy=default.target` so the service starts with
  the user session

### macOS (launchd user agent)

- **Plist path:** `~/Library/LaunchAgents/com.documentdriven.ddx-server.plist`
  (user LaunchAgent; never a machine-level `/Library/LaunchDaemons` entry
  in this phase)
- **Label:** `com.documentdriven.ddx-server`
- **Working directory:** `WorkingDirectory` set to the project root passed
  at install time, or the user's home directory if none is configured
- **Logs:** `~/Library/Logs/ddx-server/stdout.log` for `StandardOutPath`
  and `~/Library/Logs/ddx-server/stderr.log` for `StandardErrorPath`. The
  installer must create `~/Library/Logs/ddx-server/` with mode `0700` if
  absent
- **Environment overrides:** `EnvironmentVariables` carries `DDX_NODE_NAME`,
  `DDX_DATA_HOME` (when set), and any TLS certificate path overrides. API
  keys from `.ddx/server.env` are read by the server at startup; they are
  not duplicated into the plist
- **Run policy:**
  - `RunAtLoad = true` — start when the user logs in
  - `KeepAlive = true` — restart when the process exits for any reason
  - `ThrottleInterval = 10` — minimum 10 seconds between restarts to
    prevent tight crash loops
- **Lifecycle commands:**
  - install/enable: `launchctl load -w ~/Library/LaunchAgents/com.documentdriven.ddx-server.plist`
  - disable/remove: `launchctl unload ~/Library/LaunchAgents/com.documentdriven.ddx-server.plist`
    followed by deletion of the plist file
  - restart: `launchctl kickstart -k gui/$(id -u)/com.documentdriven.ddx-server`
  - status: `launchctl print gui/$(id -u)/com.documentdriven.ddx-server`
- **Install target:** the user's GUI domain (`gui/<uid>`), so the service
  follows the login session rather than a system boot

Machine-level installs (LaunchDaemon under `/Library/LaunchDaemons`, or
system-level systemd units) are out of scope for this phase and will be
specified separately if and when multi-user server hosting is required.

## Out of Scope

- Agent/execution invocation from non-localhost without ts-net (security
  boundary — dispatch endpoints are localhost-only or ts-net only, per ADR-006)
- User authentication beyond ts-net identity (no custom auth middleware)
- Multi-library aggregation
- Hosting as a cloud service
- Branch management or merge conflict resolution via API
- Machine-level service installs (LaunchDaemon, system systemd units)
