---
ddx:
  id: PLAN-2026-06-08-SERVER-OPERATOR-WORKBENCH
  depends_on:
    - FEAT-002
    - FEAT-008
    - FEAT-021
    - FEAT-026
    - ADR-007
    - ADR-022
    - TP-002
---
# Server Operator Workbench Plan

## Goal

Make `ddx server` the basic operator surface for DDx work:

- see all workers across all local projects, and across all nodes when the
  current node is a federation hub
- see what every worker is doing now: project, bead, attempt, phase, route,
  elapsed time, and freshness
- start and stop more autonomous `ddx work` workers from the web UI
- mutate bead queues for any registered project
- edit project specs/documents, especially `docs/helix/**`

This plan intentionally excludes Axon backend rollout and 2k-scale artifact
performance. The basic contract is JSONL-backed projects, existing server
worker launch, existing GraphQL/web stack, and owner-targeted federation writes.

## Design Anchors

- FEAT-021 US-100 through US-104 define the operator stories.
- FEAT-026 US-097b defines owner-targeted federation writes.
- ADR-022 remains load-bearing: workers are autonomous and the bead store is the
  claim authority. The server launches, observes, and requests stop; it does not
  become the correctness authority.
- ADR-007 remains load-bearing: hub/spoke federation never broadcasts writes.
  Every write targets exactly one owner node and one owner project.
- TP-002 TC-016 through TC-019 define the E2E contract for basic functionality.

## E2E Test Plan

1. `workers-single-pane.spec.ts`
   - Local fixture: two registered projects with reported and live workers.
   - Assert `/nodes/:nodeId/workers` merges rows with project badges, current
     bead, phase, route/model, elapsed time, and freshness.
   - Assert row detail shows prompt, output, recent events, attempt id, current
     bead link, and stop action.
   - Federation fixture: hub plus spoke. Assert `scope=federation` merges hub
     and spoke workers with node badges.

2. `workers-dispatch.spec.ts`
   - Start two workers for one project through `startWorker`.
   - Assert both rows appear and the queue/worker summary updates.
   - Assert `workers.max_count` refusal is visible.
   - Stop one worker and assert terminal/stopping state plus audit event.

3. `federation-worker-control.spec.ts`
   - Start hub and spoke.
   - From the hub, start a worker for a spoke project.
   - Assert exactly one worker starts on the spoke and appears in hub
     federation scope.
   - Stop the worker through the hub.
   - Simulate offline spoke and assert command refusal with no phantom row.

4. `beads-queue-mutations.spec.ts`
   - Local multi-project fixture: create/edit/lifecycle-mutate a bead in
     project A and assert project B isolation.
   - Federation fixture: create/edit a bead in a spoke project from the hub and
     assert the spoke owns the mutation and the hub read model updates.

5. `spec-editing.spec.ts`
   - Edit and save a `docs/helix/**` document in one project.
   - Assert path confinement rejects absolute and traversal writes.
   - Assert stale-write protection refuses overwrites.
   - Repeat save through hub-to-spoke forwarding.

## Implementation Plan

### Wave 1: Local Worker Workbench

Build the local node-wide and project-scoped worker views around existing
`reportedWorkers`, `workersByProject`, `queueAndWorkersSummary`, `startWorker`,
`stopWorker`, and `workerProgress` surfaces. The UI should prefer server-derived
reported worker state and fall back to polling where WebSocket/subscription
streams are unavailable.

### Wave 2: Local Worker Dispatch Hardening

Make `startWorker` accept count/mode/filter inputs safely, enforce
`workers.max_count`, return one result per started worker, and emit audit events.
Keep each worker autonomous. Launch failures must surface as typed UI errors,
not silent rows.

### Wave 3: Owner-Targeted Federation Writes

Add forwarding for `startWorker`, `stopWorker`, bead mutations, and
`documentWrite`. Each forwarded command carries origin identity, forwarding path,
request id, target node/project, and expected version where needed. The owner
executes locally or rejects. The hub never creates local fallback state for a
spoke write.

### Wave 4: Queue and Spec Mutation UX

Complete the bead editor/lifecycle actions and spec editor flows for local and
federated projects. Use path confinement and stale-write protection for document
writes. Show mutation errors inline and refresh the affected lists/details after
success.

### Wave 5: Reliability Gate

Wire the new E2E specs into the functional gate. Keep scale/perf and Axon
coverage separate so basic operator functionality can go green independently.

## Non-Scope

- Axon backend implementation or Axon subscriptions.
- 2k artifact fixture performance and FEAT-008 budget locking.
- Managed-node outbound command channel from FEAT-029/ADR-028.
- Distributed consensus, replication, or hub failover.
- Changing the bead store claim authority from project-local stores to the
  server.

## Bead Breakdown

The implementation beads for this plan should carry:

- `spec-id=PLAN-2026-06-08-SERVER-OPERATOR-WORKBENCH`
- labels including `plan:server-operator-workbench`, `phase:iterate`, and the
  relevant `area:*`
- acceptance with named Go `Test*` symbols, named Playwright specs, `cd cli &&
  go test ...`, the relevant `bunx playwright test ...` command, and `lefthook
  run pre-commit`
