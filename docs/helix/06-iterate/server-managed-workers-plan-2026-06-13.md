---
ddx:
  id: IP-2026-06-13-server-managed-workers
  type: implementation-plan
  status: reviewed-with-blocker
  depends_on:
    - FEAT-008
    - FEAT-010
    - ADR-022
    - TD-011
    - TP-021
---
# Implementation Plan: Server-Managed Worker Supervision

## Goal

Make DDx server the normal control plane for long-running queue drain workers.
Operators should be able to monitor, start, stop, restart, and reconcile workers
for every registered project without tmux, ad hoc background jobs, or manual
process cleanup.

The finished feature must prove three things:

1. Server-managed workers can drain project queues with the same `ddx work`
   semantics operators rely on today.
2. The server can fully clean up workers and their child agent processes,
   including Claude/Codex subprocesses, on stop, watchdog reap, restart, and
   server shutdown.
3. The behavior is verified by automated tests, including an isolated
   containerized end-to-end test that detects leaked `ddx`, `claude`, `codex`,
   and shell child processes after cleanup.

## Current State

The repo already has a partial server worker implementation:

- `cli/internal/server/workers.go:192-223` defines `WorkerManager`, an
  in-process goroutine manager with watchdog settings.
- `cli/internal/server/workers.go:361-465` starts a server-owned work worker,
  writes `.ddx/workers/<id>/status.json`, captures `worker.log`, and streams
  structured progress.
- `cli/internal/server/workers.go:1214-1308` stops a known in-memory worker,
  releases the bead claim, cancels the context, and tries process-group
  termination when a PID is recorded.
- `cli/internal/server/workers.go:1329-1483` has watchdog reaping for workers
  that exceed runtime and stall budgets.
- `cli/internal/server/graphql/schema.graphql:1931-1947` exposes
  `StartWorkerInput`; `schema.graphql:3271-3285` exposes `workerDispatch`,
  `startWorker`, and `stopWorker`.
- `cli/internal/server/frontend/src/routes/nodes/[nodeId]/projects/[projectId]/workers/+layout.svelte:12-42`
  wires Start/Stop/Add worker mutations.

The gap is that these pieces are not yet a durable supervisor:

- Desired worker count and restart policy are not durable state. A server
  restart loses the intention to keep one worker running for a project.
- Workers started outside the server, such as `tmux` windows or shell
  background jobs, are only externally reported or discovered. The server
  cannot reliably stop, restart, or reap their child process trees.
- Server-owned workers are goroutines; the `PID` field is often zero, so
  `WorkerManager.Stop` cannot kill the agent subprocess tree unless the
  execution path records or owns the spawned child process group.
- There is no single operator command equivalent to "make project X have N
  supervised workers" that goes through the server.
- Existing tests cover many pieces, but not a full isolated stop/restart/no-leak
  scenario across server, worker, and agent child processes.

## Scope

In scope:

- Durable desired-state supervision per registered project.
- Server-side reconcile loop that starts, stops, restarts, and prunes workers
  to match desired state.
- CLI commands that talk to the server for worker lifecycle operations.
- GraphQL/UI support for desired count, restart policy, and explicit restart.
- Process-tree ownership and cleanup for server-managed workers and their
  agent children.
- Containerized end-to-end tests that prove cleanup leaves no managed worker or
  agent child processes behind.
- Migration path for current manual workers: stop tmux/shell workers, then
  re-create equivalent server-managed workers.

Out of scope:

- Making the server authoritative for bead claiming. The bead store remains the
  source of truth for claims and close events.
- Adopting arbitrary already-running external `ddx work` processes into full
  management. The server may report them as external, but cannot promise stop or
  restart unless it started them or they register a managed control channel in a
  later feature.
- Cross-host orchestration. This plan is host-local; federation remains separate.
- Changing Fizeau model routing semantics.

## Design

### Review Status

Claude review was attempted three times on 2026-06-13:

- full read-only plan review with `claude -p --model sonnet --permission-mode plan`
  and Read/Grep/Glob access: no output before the bounded timeout;
- compact no-tool prompt with the plan text on stdin: no output before the
  bounded timeout;
- reduced summary prompt: no output before the bounded timeout.

A trivial `claude -p --model sonnet 'Return exactly: claude-ok'` completed, so
the failure is workload-specific. `--model fable` returned provider
unavailable. This plan therefore has a review-gate blocker: before release, the
implemented feature must include a deterministic plan/work-review path that
cannot silently hang on Claude review. Implementation can proceed under this
plan because the missing review is itself part of the worker-supervision failure
mode being addressed.

### Supervisor Model

Add a durable desired-state file under the project DDx state root:

```text
.ddx/workers/desired.json
```

Proposed schema:

```json
{
  "version": 1,
  "project_root": "/abs/project",
  "desired_count": 1,
  "default_spec": {
    "mode": "watch",
    "idle_interval": "30s",
    "profile": "default",
    "harness": "",
    "provider": "",
    "model": "",
    "label_filter": ""
  },
  "restart": {
    "enabled": true,
    "max_restarts_per_hour": 6,
    "backoff": "30s",
    "backoff_max": "10m"
  },
  "updated_at": "RFC3339"
}
```

The server reconcile loop reads this file for every registered project. On each
tick and on explicit wake:

- If running server-owned workers `< desired_count`, start workers using
  `default_spec`.
- If running server-owned workers `> desired_count`, gracefully stop the newest
  excess workers.
- If a worker exits unexpectedly and restart is enabled, restart after backoff
  unless the project is dirty in a way that would make a new claim unsafe.
- If a stale `.ddx/workers/<id>/status.json` says `running` but the worker is
  not in memory, mark it stale/stopped and release any bead claim.

This preserves autonomous `ddx work` as an escape hatch while making
server-managed workers the routine path.

### Process Ownership

Server-managed workers need a real process tree boundary even though the worker
loop currently runs in-process as a goroutine.

Implementation options:

1. Preferred: server spawns a `ddx work --server-managed <worker-id>` child in a
   new process group, and the child reports progress through the existing event
   protocol. This gives `Stop` and watchdog reap a concrete PGID to terminate.
2. Transitional: keep in-process worker goroutines but require the execution
   runner to register every agent child PID/PGID with the worker handle, then
   terminate those process groups on stop/reap.

The preferred process-supervisor shape is stronger and easier to verify in
Docker. It also matches operator expectations: a managed worker is a process the
server can kill completely.

Required cleanup contract:

- Every server-managed worker runs in its own process group.
- Every harness child launched by that worker is either in that process group or
  registered as a child process group.
- Stop sends SIGTERM to the worker process group, waits
  `WatchdogKillGrace`, then SIGKILLs remaining children.
- Server shutdown performs the same cleanup for all server-owned workers unless
  a worker has explicitly detached, which this plan does not introduce.
- Cleanup records a worker lifecycle event and bead event before killing.
- Cleanup leaves no claim held by the stopped worker.
- Cleanup must be idempotent. Repeating stop/reap/shutdown cleanup for the same
  worker must not kill unrelated processes, re-close a bead, or leave a stale
  `running` status record.
- Cleanup must have a deterministic timeout for every phase: graceful worker
  cancel, SIGTERM grace, SIGKILL wait, status persistence, and claim release.

### Server API And CLI

Add or harden server operations:

- `setWorkerDesiredState(projectId, desiredCount, spec, restartPolicy)`
- `restartWorker(id)`
- `reconcileWorkers(projectId)`
- `stopWorker(id)` remains as explicit stop.
- `workersByProject` includes `managed: true|false`, `desired: true|false`,
  `restartCount`, `lastRestartAt`, `lastExitReason`, and `processGroupId`.
- `reportedWorkersByProject` remains separate from managed workers. Reported
  external workers must never expose Stop or Restart as if the server owns them.

Add CLI commands that use the server when available:

```bash
ddx worker status [--project <path|id>] [--json]
ddx worker set --project <path|id> --count 1 [--harness claude] [--model sonnet]
ddx worker start --project <path|id>
ddx worker stop <worker-id>
ddx worker restart <worker-id>
ddx worker reconcile --project <path|id>
ddx worker cleanup --project <path|id>
```

`ddx work` remains a direct autonomous worker command. For routine operator
mode, docs and UI should point to `ddx worker set --count N` or the Workers page
instead of telling operators to start tmux windows.

### UI

Update the Workers page:

- Show desired count and actual server-managed count.
- Provide stepper controls for desired count.
- Add explicit Restart on each managed running worker.
- Separate "Managed workers" from "Reported external workers".
- Mark external workers as "reported only; stop/restart unavailable".
- Surface stale, restarting, backoff, and cleanup-needed states.

### Testing Strategy

Tests must not require real Claude or Codex credentials. They should use fake
agent binaries named `claude` and `codex` placed at the front of `PATH`. Those
fakes sleep, spawn child processes, trap SIGTERM, and optionally ignore SIGTERM
so the SIGKILL path is exercised.

Fast tests:

- Unit tests for desired-state load/save/validate.
- Unit tests for reconcile decisions.
- Unit tests for restart backoff and max-restarts-per-hour.
- Unit tests for process-tree termination helper on Unix using short-lived
  shell children.
- GraphQL resolver tests for desired state, restart, and external-worker
  labeling.

Integration tests:

- Start a real `ddx server` against an isolated fixture project.
- Use GraphQL or `ddx worker set --count 1` to start a managed worker.
- Verify `.ddx/workers/<id>/status.json`, `worker.log`, and
  `worker-events.jsonl` are written.
- Stop the worker and assert no bead claim remains.
- Restart the worker and assert restart count/lifecycle events are visible.

Containerized cleanup test:

- Build a DDx test image from the current source.
- Create a fixture repo inside the container with a bead whose script harness
  invokes fake `claude` and fake `codex` children.
- Start `ddx server`.
- Set desired worker count to 1.
- Wait until the fake agent child process is running.
- Stop the worker through the server API.
- Assert, from inside the container, that:
  - no `ddx work --server-managed` process remains;
  - no fake `claude` or `codex` process remains;
  - no descendant shell/sleep process remains;
  - the bead is unclaimed or closed according to the injected outcome;
  - worker state is `stopped`, `reaped`, or `restarted` as expected.
- Repeat the scenario for watchdog reap and server shutdown, not only explicit
  stop.
- Repeat stop twice and assert the second call is a no-op with no extra process
  kills and no extra bead state transition.

This can be implemented as either:

```bash
cd cli && go test ./internal/server/... -run TestIntegration_ServerManagedWorker_NoProcessLeaks
```

where the Go test uses Docker, or as:

```bash
scripts/integration/server-managed-workers-docker.sh
```

invoked from Go with `exec.Command` when Docker is available. The test should
skip with a clear message when Docker is unavailable, but CI/release gates
should include a Docker-enabled job.

### Migration Plan For Current Dogfood Workers

1. Inventory current external workers with `ddx work status --all-projects`.
2. For each project currently expected to drain:
   - stop tmux/shell workers gracefully;
   - verify no `ddx work`, `claude`, or `codex` descendants remain for that
     project;
   - set server desired count to 1 with the matching default spec;
   - confirm the server starts a managed worker and records it under the
     project Workers page.
3. Keep monitoring through server APIs only.
4. If a project needs a temporary direct worker because the server is broken,
   label it as external and file a bead before relying on it.

## Work Breakdown

### Phase 1: Desired State And Reconcile Core

Add a `workers/supervisor` layer around `WorkerManager`.

Deliverables:

- Desired-state model and persistence.
- Reconcile loop with start/stop decisions.
- Restart backoff state.
- Unit tests for decisions and persistence.

Exit:

- `cd cli && go test ./internal/server/... -run 'TestWorkerDesired|TestWorkerReconcile|TestWorkerRestart'` passes.

### Phase 2: Managed Process Boundary

Move server-managed drain workers to a process-supervised path, or add a
complete child-process registry if process spawning is deferred.

Deliverables:

- Server-owned worker process groups.
- PID/PGID recorded in `WorkerRecord`.
- Stop/reap kills worker and child process groups.
- Server shutdown cleanup.

Exit:

- A test proves a worker that spawns sleeping fake `claude` and `codex`
  descendants leaves none behind after stop/reap.

### Phase 3: API, CLI, And UI Controls

Expose desired-state supervision through GraphQL, CLI, and Workers UI.

Deliverables:

- GraphQL mutations/fields for desired count and restart.
- `ddx worker` command group.
- UI desired-count control, Restart button, and managed-vs-external grouping.

Exit:

- GraphQL tests pass.
- Frontend worker e2e tests pass for add/remove/restart and external labeling.

### Phase 4: Docker E2E Capability Test

Add containerized verification that starts the full stack and proves cleanup.

Deliverables:

- Dockerfile or test script for isolated DDx worker supervision.
- Fake `claude` and `codex` process fixtures.
- CI-friendly test command.

Exit:

- Docker-enabled test passes locally.
- Test proves no managed worker, fake agent, or child sleep process remains.

### Phase 5: Migration And Dogfood Switch

Switch active project queues from tmux/shell workers to server-managed workers.

Deliverables:

- Stop and clean external workers.
- Start server-managed worker desired count for helix, ddx, tablespec, pqueue,
  and heimq.
- Monitor through server APIs.
- File follow-up beads for any queue that cannot drain under server management.

Exit:

- Each in-flight project has desired count >= 1, actual managed count >= 1, and
  fresh progress events.
- No unmanaged `ddx work`, `claude`, or `codex` process remains from the
  retired tmux workers.

## Validation

Required commands before feature completion:

```bash
cd cli && go test ./internal/server/... ./cmd/... ./internal/agent/... -run 'Worker|worker|ServerManaged|Process|Reconcile|GraphQL'
cd cli && go test ./internal/integration/... ./internal/server/... -run 'ServerManagedWorker|MultiWorker|LockContention'
cd cli/internal/server/frontend && bun test && bun run test:e2e -- workers
scripts/integration/server-managed-workers-docker.sh
lefthook run pre-commit
```

The final migration validation must also run:

```bash
ddx work status --all-projects --json
pgrep -af 'ddx work|claude|codex'
```

and document which remaining processes are expected non-worker interactive
sessions versus managed worker descendants.

## Risks

- **Killing too much.** Process-group cleanup must only target server-owned
  workers, not an operator's interactive Claude/Codex session. Mitigation:
  create a dedicated process group per managed worker and kill only that PGID.
- **Server restart races.** Desired-state reconcile might double-start workers
  after restart. Mitigation: stale status reconciliation and per-project start
  lock.
- **Dirty-root loops.** Automatic restart can repeat a known pre-claim blocker.
  Mitigation: restart policy pauses on operator-attention states until the
  project is clean or the blocker fingerprint changes.
- **Docker availability.** Docker may not be installed on every developer host.
  Mitigation: skip locally with a clear message, but require Docker in release
  CI for this feature.
- **ADR-022 tension.** ADR-022 states workers are autonomous and the server is
  value-added coordination. This plan preserves autonomous `ddx work`, but makes
  server-supervised workers the recommended operator mode. ADR-022 needs an
  amendment or successor ADR during implementation.
- **Reviewer hangs.** Claude review can hang even when trivial prompts work.
  Mitigation: review dispatch must be supervised by the same timeout and process
  cleanup machinery as implementation workers, and stalled review attempts must
  leave durable operator-attention evidence instead of blocking the plan.

## Open Questions

- Should server-managed workers always spawn subprocesses, or can the
  in-process goroutine path remain for tests only?
- Where should desired-state live for multi-project server operation:
  project-local `.ddx/workers/desired.json`, server node state, or both with
  project-local as source of truth?
- Should restart policy default to enabled for all projects, or only after an
  operator sets desired count?
- Should external `ddx work` processes be stoppable through a voluntary control
  channel in a later feature?

## Handoff

This plan should be reviewed by Claude before release. Because Claude review
stalled during planning, implementation should include the review-timeout
supervision gap and then re-run the review gate under the new supervised path.
After the implementation beads are filed:

1. File one epic bead for this plan.
2. File Phase 1-5 child beads with explicit file scopes and test commands.
3. Use `ddx work` or server-managed workers to execute the child beads.
4. After each code change lands, run `make install` so the active `ddx` binary
   matches source.
5. Monitor worker progress from the server surface; avoid tmux/shell workers
   except as a documented emergency fallback.
