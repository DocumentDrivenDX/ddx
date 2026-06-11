---
ddx:
  id: FEAT-026
  depends_on:
    - helix.prd
    - FEAT-020
    - FEAT-021
    - FEAT-029
    - ADR-006
    - ADR-007
    - ADR-028
---
# Feature: Federation / Hub-Spoke Coordinator

**ID:** FEAT-026
**Status:** Frame
**Priority:** P2
**Owner:** DDx Team

## Overview

Federation lets multiple `ddx server` nodes present a unified view of beads,
runs, workers, and projects across machines while each node remains fully usable
on its own. One node runs as the **hub**; the others run as **spokes** that
register and heartbeat to the hub. The hub aggregates reads by fanning out to
spokes' existing GraphQL APIs over ts-net. Basic operator writes are
owner-targeted: worker start/stop, bead queue mutations, and spec/document
writes may be forwarded to the node that owns the selected project, but writes
are never broadcast.

This feature covers **read-federation spokes**: nodes with inbound DDx server
surfaces that the hub can query. FEAT-029 / ADR-028 cover **managed nodes**:
machines that do not expose a ts-net listener and instead dial the hub over an
outbound control channel for full remote control.

ADR-007 captures the topology, authority model, ts-net policy, plain-HTTP
opt-out, and write-routing contract. ADR-006 supplies the transport-layer
authentication. FEAT-020 supplies node identity, the project registry, and
the persisted state model that federation-state.json extends. FEAT-021
supplies the `/nodes/:nodeId/...` URL grammar that the hub resolves across
registered spokes.

## Problem Statement

**Current situation:** Every `ddx server` is a single node. The UI sees only
the projects on its own machine; CLI tools cannot ask one server about work
happening on another. Operators with more than one machine — workstation +
server, or several developer boxes on a tailnet — must visit each UI
separately and stitch the picture together by hand.

**Pain points:**
- No single dashboard answers "what is happening across my machines?"
- Combined views (`/nodes/:nodeId/beads`, `/nodes/:nodeId/runs`) are
  pre-wired in FEAT-021 for cross-project but stop at the node boundary.
- Run history, bead queues, and project lists must be reconciled by hand
  when work spans nodes.
- Story 15 (server-side prompt execution) needs a routing contract for
  "which node owns this row" before it can forward writes safely.

**Desired outcome:** An operator can run `ddx server --hub-mode` on one node
and `ddx server --hub=<host>` on the others. The hub UI surfaces a
`/federation` overview, all `/nodes/:nodeId/...` routes resolve across the
federation, combined views accept `?scope=federation` to fan out across spokes,
and the hub can start/stop workers plus mutate beads/specs on the owning node.
Spoke UIs continue to work directly as a fallback.

## Architecture

See **ADR-007** for the topology, authority, ts-net policy, write-routing
contract, naming convention, and version-handshake compatibility matrix.
This spec describes the user-visible surface that ADR-007 makes real.

### Roles

| Role | Flag | Behavior |
|------|------|----------|
| `standalone` | (default) | Single node; no `/api/federation/*`; no register/heartbeat. |
| `hub` | `--hub-mode` | Exposes `/api/federation/*`; persists registry; fans out reads. |
| `spoke` | `--hub=<host>` | Registers and heartbeats to `<host>`; serves federated reads via existing `/graphql`. |
| `hub_spoke` | `--hub-mode --hub=<other>` | Both behaviors on the same node. |

`/api/node` exposes the active role as `node.federation_role`
(see FEAT-020 amendment).

### Federation Registry

Persisted on the hub at `~/.local/share/ddx/federation-state.json`:

```json
{
  "schema_version": "1",
  "hub": {
    "node_id": "node-7029e8d6",
    "node_name": "eitri",
    "started_at": "2026-05-02T19:58:33Z"
  },
  "spokes": [
    {
      "node_id": "node-bf91c204",
      "node_name": "bragi",
      "url": "https://bragi:7743",
      "ddx_version": "0.42.0",
      "schema_version": "1",
      "graphql_schema_version": "2026-04-29",
      "capabilities": ["beads", "runs", "projects", "doc-graph"],
      "registered_at": "2026-05-02T19:58:34Z",
      "last_heartbeat": "2026-05-02T20:00:01Z",
      "compat_status": "live",
      "liveness": "live"
    }
  ]
}
```

`compat_status` is one of `live`, `degraded`, `rejected`. `liveness` is one
of `live`, `stale`, `offline` (see ADR-007).

### Hub HTTP API

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/federation/register` | POST | Idempotent register on stable `node_id`; returns assigned compat_status |
| `/api/federation/heartbeat` | POST | 30s + jitter; updates `last_heartbeat` and re-asserts URL |
| `/api/federation/deregister` | POST | Graceful shutdown; removes spoke from registry |
| `/api/federation/nodes` | GET | List of `{hub, spokes[]}` with liveness + compat_status |

All federation endpoints enforce ts-net or loopback peers unless the hub was
started with `--federation-allow-plain-http` (see ADR-007).

### Federated GraphQL Read Surface

The hub exposes parallel federated fields alongside existing local queries.
Existing queries remain node-local and unchanged; federated fields fan out:

```graphql
type Query {
  federationNodes: [FederationNode!]!
  federatedBeads(scope: FederationScope, projectID: ID): BeadConnection!
  federatedRuns(scope: FederationScope, projectID: ID, layer: RunLayer): RunConnection!
  federatedProjects(scope: FederationScope): [Project!]!
}

enum FederationScope { LOCAL FEDERATION }
```

Every federated row carries the routing-metadata fields described in
ADR-007 (`node_id`, `project_id`, `project_url`, `project_path`,
`write_capability`, `status`).

### Owner-Targeted Write Surface

Federation write routing is a narrow operator-control path, not data
replication. The hub may forward only commands that target one owning node and
one owning project. It must never broadcast a write to multiple spokes.

Forwardable basic-operator commands:

| Command | Owning target | Purpose |
|---|---|---|
| `startWorker` / `workerDispatch(kind: "work")` | project owner | Start one or more autonomous `ddx work` workers on that node. |
| `stopWorker` | worker owner | Request stop for a running worker on the node that owns the worker id. |
| `beadCreate`, `beadUpdate`, `beadApprove`, `beadBlock`, `beadCancel`, `beadReopen` | project owner | Modify the owning project's bead queue. |
| `documentWrite` | project owner | Save project-confined spec/document changes, especially `docs/helix/**`. |

Forwarded writes carry:

- origin identity: the operator identity seen by the hub
- forwarding path: hub node id plus target spoke node id
- request id / idempotency key
- target node id and project id
- target revision or expected version when the mutation needs stale-write
  protection

The owning node is authoritative. It may reject a forwarded write because the
project is missing, the node is offline/stale, the requested write capability is
absent, path confinement fails, the worker cap would be exceeded, the bead state
changed, or the idempotency key maps to a previous result.

### CLI Flags

```
--hub-mode                       Run as federation hub
--hub=<host[:port]>              Run as spoke; register with this hub
--federation-allow-plain-http    Allow non-ts-net peers (hub-mode required to accept;
                                 spokes need it too to send)
```

Precedence (matches ADR-006): flag > env > config (`.ddx.yml` config is a
follow-up).

### UI Routes (delivered by FEAT-021 amendment)

- `/federation` — federation overview (hub + spokes, liveness, compat).
- `/nodes/:nodeId/...` — when the node is a registered spoke, the hub
  resolves and serves the route from the spoke's data.
- Combined views (`/nodes/:nodeId/beads`, `/nodes/:nodeId/runs`) accept
  `?scope=federation` to fan out across all spokes.

## Requirements

### Functional

1. A node started with `--hub-mode` exposes `/api/federation/*` and persists
   the federation registry across restarts.
2. A node started with `--hub=<host>` registers with that hub on startup
   (idempotent on `node_id`), heartbeats every 30s ± 5s jitter, and
   deregisters on graceful shutdown.
3. Re-registration with the same `node_id` updates the recorded URL and
   `last_heartbeat`. A duplicate `node_id` from a mismatched identity is
   rejected with a logged conflict.
4. Hub restart preserves the registry from disk and the next heartbeat cycle
   re-confirms each spoke without operator intervention.
5. Heartbeats overdue by more than 2 minutes mark the spoke `stale`. Fan-out
   failures mark it `offline`. The two states are distinct in the registry
   and in the UI.
6. Version handshake at register time follows the compatibility matrix in
   ADR-007: incompatible spokes are rejected, compatible-but-newer/older
   minors are marked `degraded`.
7. The hub's federated read fan-out is bounded by per-call timeout (default
   5s) and max concurrency (default 8). Partial results are returned with
   per-node error annotations rather than failing the whole query.
8. Default federation transport is ts-net or loopback only; non-ts-net peers
   are accepted only when both ends carry `--federation-allow-plain-http`,
   and the hub emits a warning log on each accepted plain-HTTP exchange.
9. Federated rows expose routing metadata (`node_id`, `project_id`,
   `project_url`, `project_path`, `write_capability`, `status`) so Story 15
   can later forward writes to the owning node.
10. The hub forwards basic operator writes only to the owner node/project
    identified by routing metadata. Broadcast writes are rejected.
11. Forwarded write responses preserve origin/forwarding audit metadata and are
    idempotent on request id.
12. The hub refuses worker, bead, and document writes for offline spokes or
    spokes whose `write_capability` is `read_only`.
13. The hub UI exposes `/federation` and resolves `/nodes/:nodeId/...` for
    registered spokes; spoke UIs remain directly reachable as a fallback.

### Non-Functional

- Federation registration adds < 50ms to spoke startup in the happy path.
- Heartbeat overhead is bounded: one POST per spoke per 30s ± jitter.
- Federated fan-out for 5 spokes returns within 1.5× the slowest spoke (or
  the timeout, whichever is smaller).
- A single offline spoke does not block fan-out for other spokes.
- The federation-state.json file is written with mode `0600`; the
  `~/.local/share/ddx/` directory is `0700` (matches FEAT-020).

## User Stories

### US-095: Operator Promotes One Node to Hub
**As an** operator with several DDx machines on a tailnet
**I want** to designate one of them as the federation hub
**So that** I can see all my machines' work in a single dashboard

**Acceptance Criteria:**
- Given I start `ddx server --hub-mode`, when I call `GET /api/federation/nodes`,
  then I see the hub itself and an empty spoke list
- Given another node has not yet started as a spoke, then it is invisible
  to the hub
- Given I open the hub UI, then `/federation` is reachable and shows the hub
  with no spokes

### US-096: Operator Joins a Spoke to the Federation
**As an** operator running DDx on a second machine
**I want** to point that node at the hub
**So that** the hub can show its beads, runs, and projects

**Acceptance Criteria:**
- Given the hub is running and I start `ddx server --hub=<hub-host>`, then
  the spoke registers within 1s and appears in `/api/federation/nodes`
- Given I restart the spoke, then it re-registers idempotently — the
  registry entry updates `last_heartbeat` and `url` but does not duplicate
- Given the spoke version is incompatible per the ADR-007 matrix, then
  registration fails fast with a clear error and the spoke logs the reason

### US-097: Operator Sees Federated Work in the Hub UI
**As an** operator using the hub UI
**I want** combined views to fan out across spokes
**So that** my dashboard reflects everything happening on my machines

**Acceptance Criteria:**
- Given two spokes are registered, when I open `/nodes/:hubId/beads?scope=federation`,
  then I see beads from the hub and both spokes with node + project badges
- Given one spoke is offline, then its rows are absent and the UI shows an
  `offline` badge for that node rather than failing the page
- Given a spoke is `stale` (no heartbeat in 2m), then its rows still appear
  with a `stale` badge
- Given I click a row originating from a spoke, then I navigate to
  `/nodes/:spokeId/...` resolved through the hub

### US-097b: Operator Controls Spoke Work from the Hub
**As an** operator using the federation hub
**I want** to start and stop workers, mutate bead queues, and save specs on the
owning spoke through the hub UI
**So that** the hub is a single pane of glass without taking authority away
from the node that owns each project

**Acceptance Criteria:**
- Given a spoke project advertises `write_capability=forwardable`, when I start
  a worker for that project from the hub, then the command is forwarded to that
  spoke and exactly one spoke-local worker starts.
- Given I create or edit a bead for a spoke project from the hub, then the bead
  is persisted on the spoke's project and appears in hub federated reads.
- Given I save a `docs/helix/**` spec for a spoke project from the hub, then the
  write is path-confined and persisted on the spoke.
- Given the target spoke is offline or read-only, then the hub refuses the
  command with an operator-visible reason and does not create local phantom
  state.
- Given a forwarded command is retried with the same request id, then the owner
  returns the original result without duplicating workers, bead events, or file
  writes.

**E2E Test:** `federation-owner-writes.spec.ts` — start hub+spoke, forward a
worker start, forward bead create/update, forward spec save, simulate offline
spoke, and verify refusal plus idempotency.

### US-098: Operator Falls Back to a Spoke UI Directly
**As an** operator when the hub is unreachable
**I want** to open the spoke UI directly
**So that** I can keep working without waiting for the hub to recover

**Acceptance Criteria:**
- Given the hub is down, when I open the spoke's local URL, then its UI
  works exactly as in the standalone case
- Given the hub returns later, then the spoke re-registers on the next
  heartbeat without operator intervention

### US-099: Operator Sees Version Skew Surfaced in the UI
**As an** operator running mixed DDx versions
**I want** to know when a spoke is degraded due to schema or version skew
**So that** I can plan upgrades without surprises

**Acceptance Criteria:**
- Given a spoke registered as `degraded`, then `/federation` shows it with
  a yellow badge and a tooltip explaining the skew
- Given any federated row sourced from that spoke, then the row carries the
  same `degraded` badge

### US-100: Operator Confirms Default Transport Is ts-net Only
**As a** security-conscious operator
**I want** federation to refuse non-ts-net peers by default
**So that** a misconfigured network does not leak federation traffic

**Acceptance Criteria:**
- Given the hub started without `--federation-allow-plain-http`, when a
  non-ts-net non-loopback peer attempts to register, then the request is
  refused with a clear error and a log line
- Given the hub started with `--federation-allow-plain-http`, then the same
  request is accepted, the hub logs a warning, and `/federation` shows a
  policy banner

## Out of Scope

- Federation write fan-out / broadcast (writes are read-only-aggregated in v1).
- Managed-node outbound control channels and full remote control (FEAT-029).
- Hub failover / hot standby.
- Auto-discovery of spokes (mDNS, Tailscale tag-based discovery).
- `.ddx.yml` static configuration of `--hub` / `--hub-mode` (follow-up).
- Heartbeat cadence configuration surface (hard-coded 30s ± 5s in v1).
- Soft-rebind UX for duplicate `node_id` conflicts (v1 hard-rejects).
- Mesh / peer-to-peer topology.
- Cross-spoke direct calls (spokes never talk to each other).

## Dependencies

- ADR-006 (ts-net authentication)
- ADR-007 (federation topology — this feature's design contract)
- ADR-028 / FEAT-029 (managed-node outbound control plane and full remote
  control; distinct from this read-federation feature)
- FEAT-020 (server node state and project registry — extended with
  `federation_role` and federation flags)
- FEAT-021 (multi-node dashboard UI — extended with `/federation` route and
  `?scope=federation`)
- FEAT-002 (server HTTP API)
- FEAT-004, FEAT-006, FEAT-010 (data sources fanned out by the hub)

## References

- ADR-007 — federation topology decision and compatibility matrix
- ADR-028 / FEAT-029 — managed-node control plane
- FEAT-020 amendment — `federation-state.json` schema, federation flags,
  `node.federation_role`
- FEAT-021 amendment — `/federation` overview, hub-resolved routes,
  `?scope=federation` on combined views
