---
ddx:
  id: SD-021
  depends_on:
    - FEAT-002
    - FEAT-013
    - FEAT-026
    - FEAT-029
    - SD-019
    - SD-020
    - ADR-006
    - ADR-022
    - ADR-028
---
# Solution Design: Hub Control Plane and Managed Nodes

## Purpose

Define DDx's multi-node control-plane topology after the managed-node decision
in ADR-028. The hub is the single browser/control surface. Nodes may either be
read-federation spokes with inbound GraphQL/UI surfaces (FEAT-026), managed
nodes with outbound control channels (FEAT-029), or both.

This design replaces the older "central service is read-only" framing. The hub
may issue bounded remote-control commands, but local project stores, local
workers, and local git landings remain authoritative.

## Vocabulary

| Term | Meaning |
|---|---|
| Hub | Central DDx UI/control plane. Operators use this instead of many tabs. |
| Spoke | FEAT-026 read-federation node with inbound GraphQL/UI surface. |
| Managed node | ADR-028 node that dials the hub and receives commands over an outbound ts-net channel. |
| Worker | Execution process (`ddx work` / `ddx try`), never a machine. |

## Topology

```
                 trusted localhost / ts-net operator
                              │
                              ▼
                       ┌────────────┐
                       │    hub     │
                       │ UI + graph │
                       │ commands   │
                       └────┬───┬───┘
                            │   │
             pull /graphql  │   │ outbound control channel
                            │   │ (managed node dials hub)
          ┌─────────────────▼┐ ┌▼──────────────────┐
          │ read-federation  │ │ managed node       │
          │ spoke            │ │ no inbound ts-net  │
          └────────┬─────────┘ └────────┬──────────┘
                   │                    │
          local projects/workers        local projects/workers
          authoritative stores          authoritative stores
```

The hub's UI can show both sources in one node registry. Each row carries a
connection mode so operators know whether the data came from fan-out polling or
from managed-node pushed state.

## Data Boundaries

### Authoritative On The Owning Node

- bead store and bead events
- documents and governing specs
- project config
- worker process lifecycle
- execution evidence and preserved worktrees
- git landing and conflict handling

### Derived On The Hub

- node/project registry view
- worker snapshots and recent events
- read models for beads, runs, documents, and logs
- command results and rejection records
- stale/offline/degraded status

The hub-derived model is sufficient for the UI, but it is not the source of
truth for claims, durable project state, or git history.

## Managed-Node Connection

A managed node starts with a hub name:

```bash
ddx server --managed-node --hub=ddx-hub
```

The node dials the hub over ts-net MagicDNS. It does not expose a ts-net
listener unless separately configured. On connect it registers:

- stable node id and node name
- DDx version, schema version, capabilities
- project registry
- connection mode and freshness metadata

It then pushes snapshots, worker events, run events, log chunks, and backfill.
If backfill drops events, the hub marks the affected state incomplete rather
than rendering fake precision.

## Remote-Control Commands

The hub may send commands defined by FEAT-029:

- bead and queue edits
- document/spec edits
- operator-prompt submit, approve, cancel
- worker start, stop, cancel, and log requests
- project-scoped config writes that already have a local write path

Each command carries the ADR-006 identity envelope and request ID. The managed
node executes the command locally or rejects it. Commands are idempotent by
request ID and are never broadcast.

## Worker Model

ADR-022 remains the worker authority contract:

- workers are autonomous execution processes
- the bead store is the only claim authority
- the hub does not assign beads directly
- remote worker start/stop is a request to the managed node
- progress is reported as derived telemetry

The old in-process `WorkerManager` model is not the control-plane authority for
managed nodes. If a node uses an internal supervisor to start local worker
processes, that supervisor is an implementation detail behind the managed-node
command handler.

## Failure Modes

| Failure | Behavior |
|---|---|
| Hub unavailable | Managed node continues local CLI/server work and reconnects later. |
| Managed node unavailable | Hub marks node offline and refuses new commands to it. |
| Read-federation spoke unavailable | Hub shows FEAT-026 stale/offline state for pull-fan-out views. |
| Command conflict | Managed node rejects and records an auditable result. |
| Git remote conflict | SD-020 landing conflict behavior applies locally. |
| Backfill overflow | Hub marks derived state incomplete and links operators to node-local logs. |

## Migration Path

1. **Local multi-project server:** existing SD-019 topology.
2. **Read federation:** FEAT-026 hub/spoke pull aggregation for nodes that
   expose an inbound DDx server.
3. **Managed nodes:** FEAT-029 outbound channel for nodes controlled from the
   hub without an inbound ts-net listener.
4. **Shared queues/cache reuse:** separate future storage/concurrency design.

## Invariants

1. Trusted localhost and ts-net peers have broad v1 control; identity is still
   recorded in the ADR-006 envelope.
2. A command targets one node/project and is never broadcast.
3. Local node authority wins over hub intent.
4. The hub may cache or materialize read models but must label freshness.
5. Missing telemetry renders unavailable, not zero.
6. Managed-node remote control does not imply distributed bead claims.

## Validation

Coverage belongs in TP-002 and the FEAT-029 implementation plan:

- hub + managed-node fixture with no inbound managed-node listener
- outbound ts-net/dialer identity captured in the envelope
- state snapshot + event/backfill materialization
- command idempotency and conflict rejection
- operator-prompt write-through with origin/forwarding audit
- worker start/stop/cancel as local-node requests
- stale/offline rendering and command refusal
