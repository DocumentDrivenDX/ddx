---
ddx:
  id: FEAT-029
  depends_on:
    - helix.prd
    - FEAT-002
    - FEAT-006
    - FEAT-008
    - FEAT-020
    - FEAT-026
    - ADR-006
    - ADR-021
    - ADR-022
    - ADR-028
---
# Feature: Managed-Node Remote Control

**ID:** FEAT-029
**Status:** Frame
**Priority:** P1
**Owner:** DDx Team

## Overview

Managed-node remote control lets one hub UI control DDx work on machines that
do not expose their own ts-net listener. A managed node starts with a hub DNS
name, establishes an outbound ts-net connection, reports its projects and
worker state, and accepts bounded commands from the hub.

This is distinct from FEAT-026 read federation. FEAT-026 pulls data from
UI-capable spokes. FEAT-029 pushes state and commands over an outbound control
channel for headless or non-listening nodes.

## Problem Statement

Operators want one browser tab for DDx across a LAN/tailnet:

- browse project queues, workers, logs, runs, docs, and governing artifacts
- edit beads and docs remotely
- submit, approve, and cancel operator prompts remotely
- start and stop workers remotely
- keep local-first operation when the hub is unavailable

Running a full inbound `ddx server` on every machine solves visibility but
forces operators to manage multiple listeners and fallback tabs. Managed nodes
make the hub the control surface while keeping each node authoritative for its
local stores and workers.

## User-Visible Contract

### Starting A Managed Node

```bash
ddx server --managed-node --hub=ddx-hub
```

- `--hub` is a DNS or MagicDNS name for the hub.
- The managed node dials the hub over ts-net.
- The managed node does not expose a ts-net listener unless a separate
  listener flag is configured.
- Localhost access on the managed node remains trusted for local operation.

### Hub UI

The hub UI shows managed nodes alongside local and federated nodes:

- node status: connected, stale, offline
- connection mode: local, read-federation spoke, managed node, or both
- project registry and health
- worker state and logs
- bead queue and run progress
- document/spec browser and editor
- operator prompt submission and approval

Rows sourced from managed-node derived state carry freshness metadata so the
operator can distinguish current data from stale or incomplete data.

## Requirements

1. A managed node connects to the hub over outbound ts-net and registers a
   stable node identity, DDx version, capabilities, and project registry.
2. The hub materializes a derived read model for the managed node from
   snapshots and event/backfill streams.
3. The hub exposes full remote-control actions for trusted peers: bead edits,
   document/spec edits, operator-prompt submit/approve/cancel, worker
   start/stop/cancel, worker logs, and project-scoped config edits that already
   have a local write path.
4. Every mutating command carries the ADR-006 identity envelope, including
   origin actor, immediate peer, forwarding path, node/project target, and
   request ID.
5. Commands are targeted to one managed node and are never broadcast.
6. The managed node executes or rejects each command locally. Local bead-store
   claims, local worker state, and local git landings remain authoritative.
7. Mutating commands are idempotent by request ID. Conflicts are rejected and
   surfaced in the hub UI.
8. If the hub is unavailable, the managed node continues local CLI/server work.
   If the managed node is unavailable, the hub refuses new commands to it and
   shows stale/offline state.
9. The hub never invents authoritative state for incomplete telemetry. Missing
   or stale fields render as unavailable rather than as fake zeroes.
10. The implementation provides a hermetic hub + managed-node test fixture.

## Non-Goals

- Global bead claim authority.
- Shared queue ownership across machines.
- Cross-node cache reuse.
- Broadcast writes.
- Requiring every managed node to expose an inbound UI/GraphQL listener.
- Replacing FEAT-026 read federation for machines that do expose listeners.

## Dependencies

- ADR-028 defines the managed-node topology and authority model.
- ADR-006 defines transport trust and the identity envelope.
- ADR-021 defines operator prompts as the web write path.
- ADR-022 defines autonomous workers and server-derived worker views.
- FEAT-026 remains the read-federation feature for inbound spokes.

## Verification

At minimum, the feature is not ready until tests cover:

- managed-node outbound registration over trusted transport
- identity envelope propagation and audit on remote writes
- hub materialization of snapshots/events/backfill
- worker start/stop/cancel command idempotency
- command rejection on stale/conflicting bead state
- operator-prompt submit/approve/cancel from the hub
- offline/stale node command refusal
- hermetic e2e with no tracked-repo dirtiness
