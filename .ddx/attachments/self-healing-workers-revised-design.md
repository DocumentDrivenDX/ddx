## Revised Design: DDx Worker Self-Healing

This plan extends the existing worker supervisor rather than replacing it with
a new worker-slot store. The first permanent fix is to make the current
supervisor converge correctly.

## Non-Goals

- Do not introduce a new durable worker-slot schema in this phase.
- Do not let the server autonomously edit implementation code.
- Do not bypass bead lifecycle rules or edit `.ddx/beads.jsonl` directly.
- Do not treat PID-only liveness as sufficient when PGID/session evidence is
  available.

## Current Root Causes

1. Stale terminal suppression

   `cli/internal/server/workers_supervisor.go` suppresses restart when
   `canStartMore` sees any `blockedTerminals` entry. A blocked terminal is
   cleared only when `desired.updated_at > block.TerminalAt`. If the terminal
   timestamp is newer than the desired-state write, restart can remain
   suppressed forever until an operator rewrites desired state.

2. PID-only liveness

   `workers_supervisor.go` uses PID liveness in reconcile even though
   `WorkerRecord` already has PGID. PID reuse or orphaned records can make the
   supervisor trust the wrong process.

3. Readiness schema mismatch

   `cli/internal/agent/preclaim_intake_hook.go:47-49` requires
   `rewrite.acceptance` to decode as a string. Valid readiness output can return
   an array of acceptance lines, matching the already-tolerant
   `suggested_child_beads[].acceptance` decoder.

4. Append JSONL cross-process writes

   Several runtime streams are append-only JSONL and currently rely on
   per-process mutexes rather than cross-process file locks. Managed subprocess
   workers can interleave writes or create recurring merge conflicts for
   `.ddx/metrics/attempts.jsonl`, `.ddx/metrics/locks.jsonl`, execution mirrors,
   and worker ingest streams.

5. Preserved-needs-review eligibility

   Beads with current preserved-needs-review / large-deletion gate evidence can
   remain worker-ready unless lifecycle state or intake policy explicitly parks
   them.

6. EMFILE/resource exhaustion

   Workers can stop on file descriptor exhaustion without a bounded resource
   guard, diagnosis, or recovery path that keeps other projects draining.

## State-Machine Decisions

### Terminal restart suppression

- Keep `blockedTerminals`, but make it scoped and expiring.
- A terminal block suppresses restart only when all are true:
  - it belongs to the same project supervisor;
  - it is newer than `WorkerDesiredState.UpdatedAt`;
  - `now.Sub(block.TerminalAt) <= DefaultTerminalBlockTTL`;
  - its reason is in the active suppression set:
    `operator_attention`, `dirty_root`, `resource_exhausted`.
- `DefaultTerminalBlockTTL` is `10 * time.Minute`, configurable only through the
  supervisor test/options seam in this phase.
- Suppression arithmetic: each active block consumes one desired slot. A project
  with `DesiredCount=3`, one active block, and one live worker may still start
  one more worker; one block must not suppress all desired workers.
- When a block expires and the supervisor restarts a worker, the restart is
  recorded in the same throttle stream used by ordinary restart backoff, with
  reason `expired_terminal_block`. This prevents a TTL-cadence restart loop for
  repeat `operator_attention`, `dirty_root`, or `resource_exhausted` terminals.
- Tests must pin the exact bug: stale terminal newer than desired write no
  longer suppresses restart forever.

### Liveness predicate

- Replace PID-only liveness in supervisor reconcile with `workerRecordLive(rec)`
  that checks:
  - existing in-process `workerHandle` wins while present and not exited;
  - otherwise PID must exist;
  - when `WorkerRecord.PGID != 0`, the live process group for PID must match;
  - when `.ddx/workers/<worker-id>/status.json` exists, `last_activity_at` must
    be within `2 * bead.HeartbeatTTL`;
  - when the worker has a current attempt, `.ddx/run-state/<attempt>.json` (or
    the current run-state convention helper) must either be absent or have
    `expires_at > now`.
- Conflict precedence: a stale sidecar or expired run-state for the same
  worker/attempt makes the record stale even if PID exists. A PGID mismatch
  makes the record stale. Missing sidecar/run-state falls back to PID+PGID only.
- The meta-scan must reuse `workerRecordLive`; it must not introduce a separate
  PID-only cleanup rule.

### Readiness decoding

- Accept both string and `[]string` for `rewrite.acceptance`.
- Normalize arrays to newline-delimited acceptance text.
- Keep malformed object/number values as typed decode errors.
- Preserve the distinction between:
  - readiness dispatch/system errors that may fail open; and
  - decode/schema errors that should produce structured evidence and be
    handled by readiness policy.

### JSONL runtime streams

- Split the fix into two separate concerns.
- Write integrity: add a shared append helper that takes a process-wide mutex
  and an OS advisory lock before appending one full JSONL row.
  - Lock path: `<jsonl path>.lock`.
  - Timeout: default `5s`; on timeout return a typed error and emit local
    diagnostic evidence rather than writing a partial row.
  - Durability: write exactly one newline-terminated row; no partial row on
    error. Existing partial-row recovery is not part of this bead.
- Use it for attempts, locks, routing/ingest/event mirrors, and any
  multi-worker runtime JSONL stream. Initial call sites:
  `cli/internal/lockmetrics/lockmetrics.go`,
  attempt/routing metrics writers, `cli/internal/agent/executions_mirror.go`,
  and `cli/internal/server/worker_ingest.go`.
- Tests must fork or spawn concurrent writers so the proof is cross-process,
  not only goroutine-level.
- Git merge integrity: add `.gitattributes` union merge rules for tracked
  append-only runtime JSONL files under `.ddx/metrics/*.jsonl` and document the
  merge behavior. This is separate from flock; flock prevents same-host row
  interleaving, while union merge prevents routine branch/worktree append
  conflicts.

### Preserved-needs-review gates

- Add a worker-readiness exclusion for beads whose latest durable attempt
  evidence is `preserved-needs-review` with an unresolved large-deletion gate.
- The bead is not worker-ready until either:
  - an operator records explicit acceptance through supported metadata:
    `ddx bead update <id> --set preserved-review-unblocked-at=<RFC3339>
    --set preserved-review-unblocked-attempt=<attempt-id> --notes
    "Unblocked <date>: preserved large-deletion gate reviewed and accepted."`;
    the timestamp must be newer than the latest preserved event and the attempt
    ID must match that event; or
  - the bead is moved to blocked/proposed/cancelled through lifecycle commands.
- The worker must not repeatedly reclaim the same unresolved preserved review.
- Ownership: deterministic eligibility belongs in the bead/ready queue layer.
  The intake prompt may still mention stale-blocker precedence, but it is not
  the source of truth for excluding unresolved preserved review gates.

### Resource exhaustion

- Add startup and periodic resource checks for FD limit, current FD usage,
  worker subprocess count, temp worktree count, and stale execution dir count.
- On EMFILE or FD pressure, the worker emits typed operator attention,
  releases any claim it owns, and the supervisor restarts with backoff only
  after cleanup/recheck.
- Other projects continue reconciling independently.
- Resource thresholds:
  - warn at `fd_used/fd_limit >= 0.80`;
  - operator attention at `>= 0.90` or any EMFILE syscall/provider failure;
  - cleanup actor is the supervisor/housekeeping path, not the failing provider
    process.
- If claim release fails under EMFILE, record best-effort evidence and rely on
  the existing claim heartbeat TTL for safe reclamation after descriptors are
  available again.

### Meta-scan

- Extend `SupervisorRegistry.ReconcileAll` and existing startup housekeeping.
- The scan is deterministic. It may restart missing desired workers, reap stale
  worker records, clear dead-PID run-state, and report stale claims.
- It does not dispatch implementation agents or create code-fix beads in the
  initial phase. Code-defect bead filing remains an explicit operator/tooling
  action with dedupe by stable failure fingerprint.

## Implementation Bead Boundaries

1. Supervisor terminal-block expiry and restart convergence.
   Files: `cli/internal/server/workers_supervisor.go`,
   `cli/internal/server/workers_supervisor_test.go`.

2. Supervisor liveness predicate uses PID+PGID+sidecar/run-state evidence.
   Files: `cli/internal/server/workers_supervisor.go`,
   `cli/internal/server/workers_watchdog_test.go`,
   `cli/internal/workerstatus/*` if needed.

3. Readiness rewrite acceptance string-or-list decoder.
   Files: `cli/internal/agent/preclaim_intake_hook.go`,
   `cli/internal/agent/readiness_classification_test.go`,
   `cli/internal/agent/bead_lifecycle_skill_contract_test.go`.

4. Cross-process locked JSONL append helper.
   Files: `cli/internal/lockmetrics/*`, runtime metric/mirror writers under
   `cli/internal/agent` and `cli/internal/server`.

5. Preserved-needs-review worker eligibility gate.
   Files: `cli/internal/bead/store.go`,
   `cli/internal/agent/execute_bead_loop.go`,
   readiness/queue tests.

6. Resource pressure and EMFILE recovery.
   Files: startup housekeeping/resource preflight under `cli/cmd` and
   `cli/internal/agent`, worker watchdog tests.

7. Doctor/status reporting for self-healing.
   Files: `cli/cmd/doctor*.go`, `cli/cmd/*doctor*_test.go`,
   server status/reporting as needed.

8. Multi-project integration proof.
   Files: `cli/internal/integration/*` or `cli/internal/server/*_test.go`.
   Covers DDx/Cayce/Snorri/Pqueue-shaped fixture with one active project
   failing while others continue reconciling.

Dependency DAG:

- Bead 3 (readiness decoder) is independent and may run first.
- Bead 1 (terminal-block expiry/throttle) must run before Bead 2 and Bead 6.
- Bead 2 (PID+PGID liveness) depends on Bead 1 because both touch supervisor
  reconcile semantics.
- Bead 4a (JSONL locked append helper) and Bead 4b (`.gitattributes` union
  merge for tracked runtime JSONL) are independent of supervisor beads and of
  each other.
- Bead 5 (preserved-needs-review eligibility gate) is independent of supervisor
  restart work but must precede the final integration proof.
- Bead 6 (EMFILE/resource recovery) depends on Bead 1 and Bead 2 so terminal
  suppression and liveness semantics are stable before resource terminals are
  classified.
- Bead 7 (doctor/status reporting) depends on Beads 1, 2, 4a/4b, 5, and 6 so
  it reports final semantics.
- Bead 8 (multi-project integration proof) depends on all implementation beads.

## Review Question

Find remaining BLOCKING issues before bead filing. Focus on whether this revised
plan is now specific enough to split into execution-ready beads without agents
making incompatible state-machine choices.

## Output Contract

### Findings

| Severity | Area | Finding |
|---|---|---|
| BLOCKING | <area> | <specific issue with evidence from the target> |
| WARNING | <area> | <specific issue with evidence from the target> |
| NOTE | <area> | <observation> |

### Verdict: APPROVE | REQUEST_CHANGES | BLOCK

### Summary

2-4 sentences.
