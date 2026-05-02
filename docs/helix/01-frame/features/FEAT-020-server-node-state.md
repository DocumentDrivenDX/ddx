---
ddx:
  id: FEAT-020
  depends_on:
    - helix.prd
    - FEAT-002
    - SD-019
---
# Feature: Server Node State and Project Registry

**ID:** FEAT-020
**Status:** In Progress
**Priority:** P1
**Owner:** DDx Team

## Overview

The `ddx server` process acquires a stable node identity at startup, persists
its state across restarts, and maintains a registry of projects it is serving.
CLI commands that touch stateful resources (beads, agent sessions, doc graph)
automatically register the calling project with the running server so the
server always has an accurate picture of active work on the machine.

This feature is the foundation for FEAT-021 (multi-node dashboard UI) and the
eventual coordinator model where multiple ddx-server instances on different
machines forward state to a shared upstream.

## Problem Statement

**Current situation:** `ddx server` starts with no identity and no memory.
Each start is stateless. There is no machine-level concept; the server cannot
distinguish one host from another, and it cannot register projects it didn't
start in. CLI commands that interact with beads or agents have no path to
surface their activity to the server UI.

**Pain points:**
- The server UI can only see the project it was started in; projects registered
  by other CLI invocations are invisible
- Restarting the server loses all context about what was running
- There is no stable identifier for a node that a future coordinator can use
  to aggregate state across machines
- The UI has no way to answer "what is this server, and what projects is it
  tracking?"

**Desired outcome:** Every `ddx server` instance has a node name and stable ID
derived from the hostname (or `DDX_NODE_NAME`). When it starts, it loads state
from the previous run. As projects are registered — either by the server itself
at startup, or by CLI commands running in other terminals — the server builds
an accurate project registry. The server writes an address file so CLI commands
can find it without configuration.

## Architecture

### Node Identity

```
node name  →  DDX_NODE_NAME env  >  os.Hostname()  >  "unknown"
node id    →  sha256(name)[:4], prefix "node-", stable across restarts
```

The node name and ID are immutable for the lifetime of the state file. If the
hostname changes (e.g. a container rename), the new name produces a new ID on
the next fresh-state run.

### State Storage

Node state is stored at the **user level**, not inside any project:

```
~/.local/share/ddx/            (XDG_DATA_HOME/ddx if set)
  server-state.json            node identity + project registry
  server.addr                  last-known server URL + PID
```

Rationale: a single `ddx server` can span multiple projects from different
git repositories. State that describes the server instance must not live
inside one project's `.ddx/` directory. The addr file and state file both
belong to the *operator* (the user running the server), not to any project.

`server-state.json` structure:

```json
{
  "schema_version": "1",
  "node": {
    "name": "eitri",
    "id": "node-7029e8d6",
    "started_at": "2026-04-13T19:58:33Z",
    "last_seen": "2026-04-13T20:00:00Z"
  },
  "projects": [
    {
      "id": "proj-96d7ea83",
      "name": "ddx",
      "path": "/Users/erik/Projects/ddx",
      "git_remote": "https://github.com/DocumentDrivenDX/ddx",
      "registered_at": "2026-04-13T19:58:33Z",
      "last_seen": "2026-04-13T20:00:00Z"
    }
  ]
}
```

`server.addr` structure:

```json
{
  "node": "eitri",
  "node_id": "node-7029e8d6",
  "url": "https://0.0.0.0:7743",
  "pid": 4075740,
  "started_at": "2026-04-13T19:58:33Z"
}
```

**Note:** The current implementation stores `state.json` inside the project's
`.ddx/server/` directory. This must be migrated to `~/.local/share/ddx/` as
described above before FEAT-020 is considered complete.

### Project Auto-Registration

Two paths register projects with the server:

**1. Server startup:** registers the project at its own working directory.

**2. CLI commands:** `ddx agent`, `ddx bead`, and `ddx doc` subcommand groups
call `serverreg.TryRegisterAsync(workingDir)` in their `PersistentPreRunE`.
This fires a background goroutine that:
- reads the server URL from `DDX_SERVER_URL` env, then `~/.local/share/ddx/server.addr`, then defaults to `https://localhost:7743`
- POSTs `{"path": "<project path>"}` to `POST /api/projects/register`
- uses a 500ms timeout; any error is silently discarded

The CLI never blocks on or depends on the server being available.

### API Endpoints

```
GET  /api/node                  Node identity (name, id, started_at, last_seen)
GET  /api/projects              All registered projects
POST /api/projects/register     Register a project by path; returns ProjectEntry
POST /graphql                   GraphQL API (SvelteKit frontend queries via GraphQL)
```

These are in addition to the project-scoped routes defined in SD-019
(`/api/projects/:project/...`). The SvelteKit frontend queries project and node
data via GraphQL rather than direct REST calls.

### One Server Per Node

`ddx server` assumes at most one running instance per machine. There is no
intra-node multiplexing. The addr file is a single-entry file; a new server
start overwrites it. If a stale PID is recorded and the process no longer
exists, CLI clients fall back to the default URL. No locking or multi-instance
coordination is required.

### Federation Path (FEAT-026)

The node-aware state file is the forward compatibility contract for multi-node
federation, which is owned end-to-end by FEAT-026 ("Federation /
Hub-Spoke Coordinator") and ADR-007 ("Federation Topology"). FEAT-020
contributes only the compatibility hooks below; no federation behavior is
implemented in FEAT-020 itself.

**Naming convention:** federation uses **federation / hub / spoke**. The terms
`coordinator`, `primary`, and `replica` are reserved or avoided per ADR-007;
the historical `coordinator` naming inside FEAT-010's per-project bead-landing
serializer is unrelated to federation.

#### Federation Role Field

`/api/node` and the corresponding GraphQL `node` query expose
`federation_role`:

```json
{
  "name": "eitri",
  "id": "node-7029e8d6",
  "started_at": "2026-04-13T19:58:33Z",
  "last_seen": "2026-04-13T20:00:00Z",
  "federation_role": "standalone"
}
```

`federation_role` is one of:

| Value | Meaning |
|-------|---------|
| `standalone` | Default. No federation flags set. |
| `hub` | `--hub-mode` set; this node accepts spoke registrations. |
| `spoke` | `--hub=<host>` set; this node registers with that hub. |
| `hub_spoke` | Both flags set. |

#### Federation Flags

The server CLI accepts three federation flags whose semantics are fully
defined in ADR-007 and FEAT-026:

| Flag | Effect |
|------|--------|
| `--hub-mode` | Run as the federation hub; expose `/api/federation/*`. |
| `--hub=<host[:port]>` | Run as a spoke; register and heartbeat with this hub. |
| `--federation-allow-plain-http` | Permit non-ts-net peers; valid only with `--hub-mode` on the hub side, and required on the spoke side to send plain-HTTP. Emits a warning log on each accepted exchange. |

FEAT-020 wires these flags through to server startup configuration and stamps
the resulting `federation_role`. The actual federation endpoints, registry,
fan-out, and UI are owned by FEAT-026.

#### federation-state.json (FEAT-026)

When the node runs as a hub, FEAT-026 persists the federation registry at
`~/.local/share/ddx/federation-state.json` alongside `server-state.json`.
The schema is owned by FEAT-026; FEAT-020's `~/.local/share/ddx/` directory
contract (mode `0700`, files mode `0600`) extends to that file unchanged.

## Requirements

### Functional

1. Server acquires a stable node name and ID at startup from hostname or
   `DDX_NODE_NAME`.
2. Server loads state from the previous run at `~/.local/share/ddx/server-state.json`
   and preserves the registered project list and node ID across restarts.
3. Server registers its own working directory as a project on every startup.
4. Server writes `~/.local/share/ddx/server.addr` on every `ListenAndServeTLS`
   call, overwriting any stale entry from a prior run.
5. `POST /api/projects/register` accepts a project path, upserts it into the
   registry (updating `last_seen` if already present), and persists state.
6. `GET /api/node` returns the node's name, ID, started_at, and last_seen.
7. `GET /api/projects` returns the full project registry as a JSON array.
8. `ddx bead`, `ddx agent`, and `ddx doc` commands call
   `serverreg.TryRegisterAsync` before their subcommands execute.
9. Client registration is fire-and-forget: no CLI command blocks on or fails
   due to server unavailability.

### Non-Functional

- State load must not block server startup for more than 5ms.
- State writes are best-effort; a write failure must never abort a CLI command
  or server request.
- The state file is written with mode `0600`; the addr file with mode `0600`.
- The `~/.local/share/ddx/` directory is created with mode `0700` if absent.

## User Stories

### US-087: Operator Identifies the Running Server Node
**As an** operator managing a machine running ddx-server
**I want** to query the server and learn which node it is
**So that** I can verify I'm looking at the right server instance

**Acceptance Criteria:**
- Given `ddx server` is running, when I call `GET /api/node`, then I receive
  the hostname (or `DDX_NODE_NAME`), a stable ID, and started_at timestamp
- Given I restart the server, then the node ID is the same as before (loaded
  from persisted state)

### US-088: Projects Register Automatically Without Configuration
**As a** developer running ddx commands in a project
**I want** the server to know about my project without manual registration
**So that** I can see it in the UI without configuring anything

**Acceptance Criteria:**
- Given `ddx server` is running and I run `ddx bead list` in a different
  project directory, then that project appears in `GET /api/projects` within
  one second
- Given the server is unavailable, then `ddx bead list` completes normally
  with no error related to registration

### US-089: Server Remembers Projects Across Restarts
**As an** operator restarting the server after a deploy
**I want** the project registry to survive the restart
**So that** I don't need to re-run commands in every project to repopulate it

**Acceptance Criteria:**
- Given projects were registered before a server restart, when the server
  starts again, then `GET /api/projects` returns the previously registered
  projects immediately
- Given the state file is absent or corrupt, then the server starts fresh
  without error

## Implementation Notes

### File Migration

The current implementation writes `state.json` to
`<workingDir>/.ddx/server/state.json`. This must move to
`~/.local/share/ddx/server-state.json`. The migration is:

1. Change `loadServerState` to accept and use the user-level dir
2. Change `New()` to call `serverAddrDir()` for both the state and addr files
3. Remove the now-unused `.ddx/server/state.json` path from all paths

### State File Write Strategy

Writes happen:
- On every project registration (POST /api/projects/register and server startup)
- On server shutdown if signal handling is added (optional)

Writes do not happen on every health check or GET request to avoid churn.

### Security

The addr file records the server URL including scheme. Clients should use
`InsecureSkipVerify: true` when the URL is `https://` and no custom cert is
configured, since the default cert is self-signed (FEAT-020's TLS cert is
auto-generated). This is already the case in `serverreg`.

## Dependencies

- FEAT-002 (server HTTP API)
- SD-019 (multi-project topology — this feature is additive to its registry model)
- FEAT-021 (dashboard UI — consumes /api/node and /api/projects)

## Out of Scope

- Multi-node federation behavior (registry, hub/spoke endpoints, fan-out,
  federated UI). Owned by FEAT-026 and ADR-007. FEAT-020 contributes only the
  `federation_role` field, the federation flags, and the
  `~/.local/share/ddx/` directory contract that `federation-state.json`
  inherits.
- Cross-machine state aggregation
- Authentication for /api/node and /api/projects endpoints
- TTL-based eviction of stale project entries
