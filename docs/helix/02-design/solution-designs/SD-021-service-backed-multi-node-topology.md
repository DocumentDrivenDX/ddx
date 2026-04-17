---
ddx:
  id: SD-021
  depends_on:
    - FEAT-002
    - FEAT-013
    - SD-019
    - SD-020
---
# Solution Design: Service-Backed Multi-Node Control Plane Topology

## Purpose

Define the framing architecture for DDx's evolution beyond the one-machine
multi-project server to a distributed control plane where a central storage/
control service orchestrates multiple headless `ddx-server` node gateways
across machines. This document describes the boundary between git-authoritative
project state and service-backed storage, node gateway responsibilities,
failure modes, and migration from the current local-first topology.

## Scope

- Central storage/control service architecture
- Headless `ddx-server` node gateway responsibilities and boundaries
- Git-authoritative vs service-backed data separation
- Failure modes, degraded operation, and migration path
- Framing only: no implementation required

Out of scope:

- Actual implementation of the central service or node gateways
- Multi-machine bead store replication (left for future specification)
- Cross-machine worker coordination protocols
- Real-time notification infrastructure

## Architectural Goals

1. **Git remains authoritative** for project state: all durable project data
   (documents, beads, executions) lives in git-tracked files within each
   project repository.
2. **Service is read-optimized**: the central service maintains denormalized
   read models derived from git data, not authoritative storage.
3. **Node gateways remain local-first**: each node runs a headless `ddx-server`
   instance that owns its local project registry, workers, and execution
   artifacts; it synchronizes with the central service for coordination only.
4. **Degrade gracefully**: when the central service is unavailable, each node
   continues to operate as a standalone multi-project server.

## Architecture Overview

```
┌──────────────────────────────────────────────────────────────────────────┐
│                      Central Control Service (Future)                    │
│  ┌─────────────────┐  ┌──────────────────┐  ┌────────────────────────┐ │
│  │   Central UI    │  │  Read-Optimized  │  │  Coordination Services │ │
│  │   (SPA)         │  │  Read Models     │  │  (Future)            │ │
│  └────────┬────────┘  └─────────┬─────────┘  └──────────┬───────────┘ │
│           │                     │                        │              │
│           └─────────────────────┴────────────────────────┘              │
│                                  │                                        │
└──────────────────────────────────┼───────────────────────────────────────┘
                                   │
                   ┌───────────────┼───────────────┐
                   │               │               │
        ┌──────────▼──────┐ ┌─────▼───────┐ ┌─────▼────────┐
        │   Node Gateway 1│ │Node Gateway2│ │Node Gateway N│
        │  (ddx-server)   │ │(ddx-server) │ │(ddx-server)  │
        └────────┬────────┘ └──────┬───────┘ └──────┬───────┘
                   │               │                 │
        ┌──────────▼──────┐ ┌─────▼───────┐ ┌──────▼───────┐
        │  Local Project  │ │Local Proje  │ │Local Project │
        │  Registry       │ │ct Registry  │ │Registry      │
        │  Workers        │ │Workers      │ │Workers       │
        │  Executions     │ │Executions   │ │Executions    │
        │  (git-backed)   │ │(git-backed) │ │(git-backed)  │
        └─────────────────┘ └─────────────┘ └──────────────┘
```

## Data Boundaries

### Git-Authoritative (Single Source of Truth)

Every project's data lives in its own git repository:

- **Documents**: All `.ddx/` artifacts are git-tracked
  - `.ddx/docs.jsonl` (or doc graph index)
  - Frontmatter and content files referenced by docs
- **Beads**: `beads.jsonl` with explicit dependency links
- **Executions**: `.ddx/exec-definitions.jsonl`, `exec-runs.jsonl`
  - Large payloads live in `.ddx/exec-runs.d/<run-id>/` attachment files
- **Workers**: `.ddx/workers/` directory with phase records

No data is authoritative outside of git. The central service never stores
project-level durable state; it only caches or materializes read models.

### Service-Backed (Denormalized Read Models)

The central service maintains derived data for read performance and
cross-project views:

- **Project index**: denormalized project metadata (name, status,
  last_seen) derived from each node's registry entries
- **Global bead index**: read model aggregating beads across projects/nodes,
  filtered by status/labels for search and dashboard views
- **Global agent session index**: cross-project session metadata
- **Node status health checks**: periodic heartbeat results from all nodes

These read models are:
1. Derived from git-backed sources via scheduled sync jobs
2. Recomputable at any time; not preserved across service restarts
3. Never the source of truth for durability or consistency

## Node Gateway Responsibilities

Each `ddx-server` instance (whether running as a local multi-project server
or as a distributed node gateway) owns:

### 1. Project Discovery and Registration

- Maintains a project registry for all projects the node is configured to
  serve (including the current working directory on startup)
- Registers projects with the central service when available
- Supports `DDX_NODE_NAME` environment variable to identify the node in
  multi-node deployments

**Contract**: FEAT-020 defines the project registry schema and
auto-registration behavior. The central service receives these registrations
but does not dictate which projects a node serves.

### 2. Worker Supervision

- Hosts an in-process `WorkerManager` that supervises execute-loop workers
  for each registered project (see SD-019)
- Workers run as goroutines within the `ddx-server` process, one worker per
  project context
- Worker lifecycle (start, logs, stop) is owned by the server; worker records
  persist under `.ddx/workers/` in each project

**Contract**: SD-019 defines worker boundaries and execution isolation per
project. The central service exposes worker state through read-only endpoints
but does not orchestrate workers across nodes.

### 3. Local Execution

- Executes bead workers within isolated project worktrees (see FEAT-006)
- Executes execution definitions and captures results to
  `.ddx/executions/<attempt-id>/` bundles
- Maintains execute-loop coordination (fetch → run → land) per project

**Contract**: Each node operates its own land coordinator per registered
project (see SD-020). The central service may provide visibility but not
control.

### 4. Synchronization with Central Service

Nodes synchronize with the central service through:

- **Registration**: POST `/api/projects/register` registers local project
  roots with the central service (fire-and-forget, as per FEAT-020)
- **Status reporting**: Periodic heartbeats to confirm node availability
- **Read model updates**: Central service polls nodes for their project
  registry and worker status

Nodes do not require the central service to function. When unavailable,
nodes continue as standalone multi-project servers.

## Central Service Components

### 1. Central UI (Single Entry Point)

- Serves the web UI from a single canonical location
- Provides node/project routing: `/nodes/:nodeId/projects/:projectId/...`
- Fetches node and project metadata from the central service
- Enables cross-project bead and session views

**Constraint**: The UI is a single-entry point, not a distributed cluster.
High availability is achieved by running multiple copies of the UI behind
a load balancer, all connecting to the same central service.

### 2. Read-Optimized Data Layer

The central service maintains derived data for performance and cross-project
views:

- **Node registry**: aggregated project registries from all nodes, deduplicated
  by project path and git remote
- **Global bead index**: read model of beads across all projects for search
  and dashboard filtering
- **Global agent session index**: cross-project execution metadata

These indices are:
- Always derived from git-backed sources
- Recomputable on demand
- Never authoritative for durability

### 3. (Future) Coordination Services

Once bead store concurrency is solved, the central service may provide:

- Cross-node bead claim coordination
- Global bead queue ordering
- Cross-project resource scheduling

These are deferred until FEAT-013 beads concurrency is resolved.

## Node Gateway State Boundaries

Each node maintains its own state:

```
~/.local/share/ddx/
  server-state.json      Node identity + project registry (per node)
  server.addr            Last-known server URL (one entry per node)

<project-root>/.ddx/
  beads.jsonl            Project bead store (git-tracked)
  exec-definitions.jsonl Execution definitions
  exec-runs.jsonl        Execution history
  workers/               Worker phase records
  executions/            Execute-bead attempt artifacts
```

**Key invariant**: No project-level state lives outside of git or the
project's `.ddx/` directory. The node-level `server-state.json` tracks only
the server instance identity and its project registry.

## Failure Modes

### 1. Central Service Unavailable

**Degraded operation**: Each node continues operating as a standalone
multi-project server.

- Local project registry remains functional
- Workers continue executing beads within their projects
- Execute-loop coordination (land) continues per project

**Recovery**: When the central service returns:

- Nodes resume registration and heartbeat
- Cross-project views become available again
- Any missed coordination events are reprocessed from git history

### 2. Node Gateway Down

**Impact**: Projects on the down node become unavailable until recovery.

- Central service marks node as `unavailable`
- Workers on that node stop
- Projects may be accessed via other nodes if replicated (future)

**Recovery**: Node restarts and re-registers with the central service.

### 3. Central Service Crash During Write

The central service does not accept writes for project data. It only serves
read models derived from git-backed sources.

- Crash is recoverable by rebuilding read models from git on restart
- No data loss because the source of truth is always git

### 4. Git Remote Unreachable

This affects node gateways that depend on git for their land coordinator.

- Land coordinator fails to push/pull from origin
- Worker preserves execution under `refs/ddx/iterations/<attempt-id>`
- Bead returns to open status for retry

This is handled by SD-020's conflict and divergence paths.

## Migration Path

### Phase 1: Local Multi-Project Server (Current)

- `ddx server` runs as per-user host daemon
- Projects registered via CLI auto-registration (FEAT-020)
- Worker supervision per project (SD-019)

**State**: This is the baseline. All existing deployments operate in this
mode.

### Phase 2: Central Service Exposure (Immediate Next Step)

Expose the central service without requiring it:

- Add central UI at `/nodes/:nodeId/projects/:projectId/...`
- Add `/api/node` and `/api/projects` endpoints for registry aggregation
- Add read models for beads/sessions across nodes

**Constraint**: CLI commands continue to work without central service.
Server startup fails gracefully if registry aggregation is unavailable.

### Phase 3: Multi-Node Deployment (Future)

Operators deploy multiple nodes with the same central service:

- Each node runs `ddx-server` and registers with central service
- Central UI provides cross-project views
- Operators can choose to run nodes on separate machines or containers

**Requirements**: 
- Central service must support high availability (multiple UI instances
  behind load balancer)
- Nodes must support heartbeats for status reporting

### Phase 4: Cross-Node Coordination (Long-Term)

Once bead store concurrency is solved, enable:

- Centralized bead claiming and queue ordering
- Cross-node worker scheduling
- Global execution metrics aggregation

**Dependency**: Bead store concurrency (ddx-2791dc4f) must be resolved
before cross-node coordination can be safe.

## Security and Authentication

### Central Service Access

- localhost-only by default (same as current `ddx server`)
- Optional ts-net (Tailscale) for remote access per ADR-006
- No custom API keys; identity from tailnet membership

### Node Gateway Access

- Each node runs as a per-user host daemon (current model)
- CLI commands auto-register with the running server
- Node-to-node communication happens only through shared git remotes

## Validation Checklist

This framing design should satisfy the following acceptance criteria:

- [ ] **AC1**: Git-authoritative boundary defined
  - All project data lives in git-tracked files within each repository
  - Service-backed storage is read-only derived data
- [ ] **AC2**: Central service topology described
  - Single UI entry point with node/project routing
  - Read-optimized data layer derived from git sources
  - No project-level durable state in central service
- [ ] **AC3**: Node gateway responsibilities defined
  - Project discovery and registration (FEAT-020)
  - Worker supervision (SD-019)
  - Local execution (FEAT-006, SD-020)
  - Synchronization with central service (optional when unavailable)
- [ ] **AC4**: Failure modes and degraded operation defined
  - Central service unavailable → nodes operate standalone
  - Node gateway down → projects on that node unavailable
  - Git remote unreachable → land conflicts handled by SD-020
- [ ] **AC5**: Migration path defined
  - Phase 1: Local multi-project server (current)
  - Phase 2: Central service exposure (next step, no requirement)
  - Phase 3: Multi-node deployment (future)
  - Phase 4: Cross-node coordination (requires bead concurrency)

## Dependencies and Future Work

| Dependency | Status |
|------------|--------|
| FEAT-002 (Server) | Complete; foundation for central service |
| FEAT-013 (Multi-Agent Coordination) | Framing; bead concurrency needed |
| SD-019 (Multi-Project Server) | Complete; defines node gateway model |
| SD-020 (Multi-Machine Coordinator) | Complete; defines git-based coordination |
| ddx-2791dc4f (Bead Store Concurrency) | Deferred; required for cross-node coordination |

## Out of Scope

- Implementation of central service or node gateways
- Multi-machine bead store replication protocol
- Real-time notification infrastructure (WebSocket, SSE)
- Cross-node bead claiming coordination
- Centralized execution queue with global ordering
- Token budget management across nodes

The central service remains a read-only view layer until bead store
concurrency is resolved. All durable state stays git-authoritative in each
project repository.
