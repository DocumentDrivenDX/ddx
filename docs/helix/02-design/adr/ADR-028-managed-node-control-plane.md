---
ddx:
  id: ADR-028
  depends_on:
    - FEAT-002
    - FEAT-006
    - FEAT-008
    - FEAT-020
    - FEAT-026
    - ADR-006
    - ADR-007
    - ADR-021
    - ADR-022
---
# ADR-028: Managed-Node Control Plane

**Status:** Proposed
**Date:** 2026-06-06

## Context

ADR-007 defines read federation: one hub fans out to registered spokes that
remain full `ddx server` instances with their own UI and GraphQL listener. That
solves the "many tabs" problem for nodes that expose an inbound server.

Operators also need a second topology: headless machines that should be fully
controllable from the hub without opening their own ts-net listener. These
machines should start with a hub DNS name, dial out, report their local state,
and receive commands over the established connection.

This ADR names that second topology **managed nodes**. It is not a replacement
for read-federation spokes.

## Decision

DDx supports an outbound **managed-node control plane**:

- The **hub** is the central UI and command surface.
- A **managed node** is a machine running DDx that dials the hub over ts-net
  MagicDNS, registers its node/project identity, pushes state/events, and
  receives commands over the outbound control channel.
- A **spoke** remains the ADR-007 read-federation participant: a full server
  with an inbound GraphQL/UI surface that the hub can query.
- A **worker** remains an execution process (`ddx work`, `ddx try`), not a
  machine. Do not use "worker node" in DDx specs; use "managed node".

Managed nodes do not listen on ts-net by default. They rely on the hub
connection for remote visibility and control. Direct local CLI operation still
works on the managed node when the hub is down.

## Transport And Identity

The managed node connects to the hub using ts-net as a **dialer**, not as an
inbound listener. The hub address is usually a MagicDNS name:

```bash
ddx server --managed-node --hub=ddx-hub
```

The ts-net session authenticates the immediate peer. Every control-plane
message also carries the ADR-006 identity envelope:

- `immediate_actor`: the peer authenticated on the current transport hop
- `origin_actor`: the human or local process that initiated the command
- `forwarding_path`: ordered node IDs that relayed the command
- `node_id` / `project_id`: the local authority target
- `request_id`: idempotency and audit correlation key

V1 authorization is intentionally broad: trusted localhost and trusted ts-net
peers have full remote-control authority. The envelope is still mandatory so
future identity policy can restrict actors without changing command payloads,
audit records, or UI models.

## Hub State Model

A managed node has no inbound GraphQL endpoint for the hub to poll. The hub
therefore materializes a derived read model from pushed registrations,
heartbeats, snapshots, and worker/bead/run/doc events.

The derived model is:

- sufficient for the hub UI to browse projects, beads, workers, runs, logs,
  documents, and governing artifacts
- explicitly marked by freshness metadata (`last_snapshot_at`,
  `last_event_at`, `stale`, `offline`, `dropped_backfill`)
- not authoritative for bead claims, git landings, or durable project state
- rebuildable from a fresh managed-node snapshot after reconnect

The managed node remains authoritative for its local project stores and local
worker processes.

## Command Surface

The hub may send these command classes to a managed node:

| Class | Examples | Authority |
|---|---|---|
| Queue/bead edits | create, update, claim/unclaim, reopen, close when existing local rules allow | managed node bead store |
| Governing documents | read/write project docs and specs | managed node project root |
| Operator prompts | submit, approve, cancel operator-prompt beads | managed node bead store and ADR-021 |
| Workers | start `ddx work`, stop/cancel, view logs | managed node worker supervisor / autonomous workers |
| Configuration | project-scoped runtime options that already have a local write path | managed node config store |

Commands are never broadcast. A command targets one node and, when applicable,
one project. The managed node executes the command locally or rejects it with a
structured reason.

## Local Authority Wins

The hub does not own bead claims, worker leases, or git landings. It sends
requests; the managed node decides using the same local rules it would apply to
a localhost UI request.

Required command semantics:

- **Idempotency:** every mutating command carries `request_id`. Retried
  commands with the same target and request ID return the original result.
- **Reject-on-conflict:** if the target bead/run/project changed, disappeared,
  closed, or is no longer eligible, the node rejects instead of forcing state.
- **No central claim table:** bead-store CAS remains the only claim authority.
- **Auditable rejection:** rejected commands are reported to the hub and
  appended to the local event/audit stream with the identity envelope.
- **Offline behavior:** if the control channel is unavailable, the hub marks
  the node offline and refuses new commands for that node. It does not queue
  unbounded deferred writes.

## Failure Modes

| Failure | Behavior |
|---|---|
| Hub down | Managed node continues local CLI/server work; commands unavailable until reconnect. |
| Managed node down | Hub marks node offline; derived state remains visible with stale/offline badges. |
| Control channel drops mid-command | Command is retried only with the same `request_id`; node idempotency decides outcome. |
| Backfill overflow | Hub marks derived state incomplete and points operators to node-local logs. |
| Conflicting local state | Managed node rejects; hub surfaces the rejection next to the attempted action. |

## Relationship To ADR-007

ADR-007 read-federation spokes and ADR-028 managed nodes may coexist:

- A UI-capable machine may be both a read-federation spoke and a managed node.
- A headless managed node may have no inbound UI/GraphQL listener.
- The hub UI presents both under one node registry but preserves the connection
  mode so operators know whether rows come from pull fan-out or pushed derived
  state.

## Consequences

- The hub becomes a stateful control plane for managed nodes, not just a
  stateless read fan-out UI.
- Full remote control is available through the hub while preserving local
  authority at the managed node.
- Future identity restrictions can be layered on the envelope without changing
  command shapes.
- Tests need a deterministic hub + managed-node fixture, not only
  hub + inbound-spoke federation tests.
