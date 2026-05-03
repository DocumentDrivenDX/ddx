---
ddx:
  id: ADR-022
  depends_on:
    - FEAT-006
    - FEAT-002
    - FEAT-010
    - ADR-006
    - ADR-007
    - ADR-021
---
# ADR-022: Worker Client–Server Architecture

**Status:** Proposed (rev 4 — design pivot 2026-05-03 per user direction)
**Date:** 2026-05-02 (rev 1) / 2026-05-03 (rev 2 + rev 3 + rev 4)
**Authors:** TD bead `ddx-076147ee`

**Rev 4 design pivot (per user direction 2026-05-03):**
"Workers should always be autonomous. If they have a server to talk to,
they send results there. They should be able to do their own work and
store their logs and artifacts locally without regard for the server.
The server is strictly a value-added coordinator."

This shifts the decision from Proposal A (server orchestrates; --local has
in-process API impl) to a Proposal B-shaped design: workers are always
autonomous against the bead store; server is an optional reporting target
that provides centralized observability, cross-worker visibility, and UI
surfaces — but is NOT required for correct operation. `--local` becomes
"force standalone mode; don't try to reach a server" — the same code
path that workers take when no server is reachable.

Rev 4 removes most of rev 3's machinery as no longer needed:
- No worker registration, heartbeat, or claim-lease API (workers claim
  from the bead store atomically as today)
- No long-poll `next-bead` endpoint (workers pick from the bead store)
- No server-side picker (worker-side picker continues, with the
  diagnostic events from commit `80f51574` preserved)
- No server-side runtime registry as authoritative state (server's view
  is derived from worker event reports — eventually-consistent best-effort)
- No claim reconciliation across server restarts (server has no claims
  to reconcile)
- No partition recovery semantics (worker autonomy makes partition
  irrelevant)
- No data migration story (per user: delete history if needed; clean
  slate is fine)

Rev 4 keeps:
- The unified `ExecuteLoopSpec` from `ddx-29058e2a` as a worker-side
  struct (cobra → in-process flow); not a wire format
- The picker priority sort + diagnostic events from commit `80f51574`
- The stay-alive defaults from commit `41cb762e`
- The cross-project boundary fixes from commits `33b97f25` /
  `07ea202d` / `5ee6b02c`
- The thin server-side event ingestion endpoint for best-effort reporting

Earlier revisions (1-3) explored the server-orchestrates direction and
are preserved in git history (commits `16fd637b`, `2c02bafe`, `83bf6ec6`)
for the design rationale.

**Rev 3 amendments (per codex review of rev 2):**
- Added `poll_interval_ms` to registration payload (preserves stay-alive
  fix from commit `41cb762e`)
- Reverted `label_filter` to string (matches current ddx-29058e2a contract;
  array form deferred to separate ADR amendment)
- Defined `model_ref` semantics distinct from `model`
- Removed `from_rev` from worker registration (per-attempt concept, not
  per-worker; moves to next-bead response or bead metadata)
- Removed self-contradicting "structurally impossible" security claim
  from Consequences
- Added Capabilities defaulting rule (omitted/empty → `["bead-attempt"]`
  for backward compat)
- Added Unified timing model section (single state table replacing
  competing claim_lease/heartbeat-timeout/partition-grace expiries)
- Added Server startup recovery section (rehydration from beads +
  legacy `.ddx/workers/`)
- Added Cancel-during-claim race resolution (`first_heartbeat_ms`
  threshold)
- Added Observability contract section (structured server events for
  picker/claim/attempt lifecycle)
- Added Multi-tenant fairness section (server-wide caps for 76+ project
  load)
- Added Data migration section (legacy `.ddx/workers/` rewrite path)
- Replaced "MUST land before C5/C7/C9" with split shippable gates;
  C5/C7 unfreeze at Gate 2, C9 unfreezes at Gate 3
- Added explicit Bead absorption table accounting for already-shipped
  fixes (picker `80f51574`, stay-alive `41cb762e`, GraphQL LAYERs
  `33b97f25`/`07ea202d`/`5ee6b02c`)
- Added API versioning rule (additive within v1, breaking → v2 with
  Accept header)

**Rev 2 amendments:** added Threat model section; added Picker section;
expanded `register` payload; added claim-lease-renewal + terminate
clarification; disambiguated `preserved`; added dropped-attempt commit
preservation; added long-poll back-pressure; added Transport error-shape
parity; added Capabilities dispatch; added ts-net partition recovery;
added Test Transport roadmap step; clarified FEAT-008 reference.
**Layout note:** The bead description suggested `docs/helix/03-decide/`. The
HELIX layout in this repo places ADRs at `docs/helix/02-design/adr/`. This ADR
is filed in the actual location to keep the index discoverable; the next ADR
number after ADR-021 is ADR-022.

## Context

Operators have observed a structural failure mode: when `ddx-server`
restarts, in-flight worker subprocesses lose their connection to the
orchestrator. Today there are two execution paths that diverge in lifecycle,
configuration plumbing, and observability:

- **`--local` path.** `ddx work` (a.k.a. `ddx agent execute-loop`) runs an
  in-process drain loop that talks to the bead store directly
  (`cli/internal/agent/execute_bead_loop.go`).
- **Server-spawned path.** The server forks a worker process, hand-marshals
  an `ExecuteLoopWorkerSpec` over an internal contract, and tracks the PID
  in `.ddx/workers/`. The worker reaches back into the bead store directly,
  not through the server.

Three open beads diagnose the symptom-space:

- `ddx-29058e2a` — five flags silently dropped on the server path because
  the spec is hand-maintained as parallel structs across six layers.
- `ddx-5cb6e6cd` — refactor epic that splits `execute_bead_loop` into
  `run/`, `try/`, `work/` packages, exposing an implicit state machine
  inside a 700-line function.
- `ddx-dc157075` — workers exit prematurely on an empty queue because
  `--poll-interval=0` is the default and the per-Run `attempted` map is
  scoped to one drain pass.

`ddx-4c51d33e` (closed) demonstrated the related security problem: any
worker that touches the bead store directly has to reimplement project
scoping; the GraphQL resolver got that wrong for `DocumentByPath`.

The common cause: **the worker is coupled to local in-process state and
the server has no first-class concept of a worker beyond a PID it forked.**
Restart loses the in-flight context because the context lives nowhere
addressable. Each fix to one of the three beads above fixes a leaf without
addressing the trunk.

This ADR exists because the next round of refactors (the `try/` and `work/`
packages, the `ExecuteLoopSpec` unification, the stay-alive fix) will all
re-cement the current bipartite design unless the architectural shift is
agreed first.

## Decision

**Workers are always autonomous. There is no "--local mode" — the
worker has one mode (autonomous) and detects a server continuously. If
a server is reachable, the worker emits events and results to it
best-effort. If no server is reachable (none running, partition, never
configured), the worker proceeds with its work and tries again later.
The bead store is the source of truth; the server is an optional
observability + coordination target.**

Concretely:

1. A worker process (`ddx work`) reads the bead store directly, picks
   the next eligible bead per the current picker logic (priority asc,
   FIFO within tier), claims it atomically, executes via `try.Attempt`,
   writes evidence locally, and reports the result by writing the bead's
   event log + closing the bead via the store.
2. **All of #1 happens regardless of whether a server is running.**
   This is the existing autonomous behavior; the worker has no
   dependency on a server for correctness.
3. **In parallel**, a small "server probe" goroutine inside the worker
   periodically (every N seconds, default 30) checks
   `.ddx/server.addr` for a reachable server. State transitions:
   - **NotConnected → Connected:** server appeared; worker POSTs
     `register` with a thin identity envelope (project root, harness,
     executor pid/host); on success, worker stores the `worker_id` and
     starts mirroring events to the server best-effort.
   - **Connected → Connected:** server still reachable; continue
     mirroring.
   - **Connected → NotConnected:** server unreachable (crash,
     partition, stopped); worker logs the transition, stops trying to
     mirror events, retains its bead-store-local event log, continues
     working.
   - **NotConnected → NotConnected:** still no server; nothing to do.
4. When in Connected state, the worker mirrors bead events + final
   results to `/api/workers/<id>/event` and the bead-close result to
   the same endpoint best-effort. Failures (server crash mid-POST,
   partition, 5xx) are logged locally and ignored — the bead's local
   event log is authoritative.
5. The server's view (which workers exist, which beads are running,
   recent events) is **derived from worker reports** — eventually
   consistent, NOT authoritative. The server's value is centralized
   observability, cross-worker dashboards, operator UI, and
   server-initiated cancel (which writes a marker to the bead store
   that workers honor on next iteration).
6. Server restart drops the in-memory derived view; on next probe cycle
   the worker re-registers (or notices the server is gone). No
   in-flight work is at risk because workers don't depend on the
   server.

**`--local` flag is removed.** Today's `--local` semantics are the
default behavior; there's nothing to opt into. Operators who today pass
`--local` will see a deprecation warning + the flag becomes a no-op for
one release, then is deleted. The breaking-changes section documents
this.

This is **Proposal B-shaped** (server is passive observer) with one
addition: the server can write back to the bead store to influence
worker decisions (e.g., cancel a bead by setting an extra field; set
priority overrides). Workers see those changes on next bead-store read.

### Why autonomous-default workers

- **Workers don't depend on the server for correctness.** Any server
  restart, partition, crash, or just "no server running" is a no-op for
  the worker's primary job (executing beads).
- **Same code path for every invocation.** `ddx work` and `ddx work
  --local` differ only in whether they bother trying to reach a server.
  No in-process API server, no Transport interface, no test-mode
  fixture — the existing unit-test setup (no server) is the autonomous
  case.
- **Server is opt-in observability.** Operators who want centralized
  dashboards run a server; operators who don't, don't. Either is
  correct.
- **No claim coordination races.** The bead store's atomic claim is the
  only claim primitive. Two workers contending for the same bead use
  the same store-level CAS that exists today; there is no parallel
  server-side claim table to race against.
- **Project boundaries enforced by the existing per-project bead store.**
  Each `.ddx/beads.jsonl` is a project; workers operate against one
  project's store; the cross-project leak class (`ddx-4c51d33e`) is
  prevented at the worker level because workers never read another
  project's store.
- **`ExecuteLoopSpec` from `ddx-29058e2a` still applies** — but as a
  worker-side struct unification (cobra → in-process flow), not a wire
  format. The drop class is closed by the same single-struct
  discipline; it just doesn't need to cross a network boundary.

### Threat model (limits of session-token project binding)

Session tokens prevent **honest worker mixup** (a misconfigured worker for
project A cannot accidentally claim a bead from project B). They are NOT a
defense against a **local malicious process**: `requireTrusted` accepts any
loopback connection without identity proof, so any process on the same host
can register as any project. The session-token model is project-scoping for
legitimate workers, not authentication of worker identity. Future work to
strengthen this (e.g., per-worker keypairs, ts-net WhoIs binding for
loopback registrations) is out of scope for v1 and tracked separately.

### Picker (worker-side, status quo with diagnostic events)

The bead picker stays where it lives today: in the worker, against the
bead store. The fix at commit `80f51574` added `picker.priority_skip`
and `picker.claim_race` diagnostic events; both are preserved. Picker
rules (unchanged from current code):

1. Read eligible beads via `Store.ReadyExecution()` (excludes superseded,
   not-execution-eligible, in retry-cooldown).
2. Sort by priority ascending (P0 first), then by `updated_at` ascending
   (FIFO within tier).
3. If `--label-filter` is set, intersect with bead labels.
4. Skip beads in the per-Run `attempted` map (set after each unsuccessful
   claim attempt; reset on falling through to "no candidate" per the
   stay-alive fix at commit `41cb762e`).
5. Claim atomically via `Store.Claim()`; on CAS loss, emit
   `picker.claim_race` and continue to next eligible.

Two workers contending for the same top bead use the bead store's
existing atomic claim — the same primitive that has worked correctly
for as long as DDx has had a bead store. There is no parallel server-side
claim table to race against.

When the server is reachable, the worker mirrors the
`picker.priority_skip` and `picker.claim_race` events to the server's
ingestion endpoint so operators can see them in the UI. When no server,
the events still land in the bead's local event stream.

### Why not centralized server-side picker (the rev 1-3 design)

- Workers don't need it for correctness — the bead store's atomic claim
  is sufficient. Adding a server-side claim table creates a parallel
  source of truth that has to be reconciled.
- Server-side picker requires workers to depend on the server, which
  contradicts the autonomous-default decision.
- Diagnostic events from commit `80f51574` already make starvation
  observable; centralization didn't add visibility we lacked.

### Why not Proposal A (server orchestrates, in-process API for --local)

(Earlier revisions chose this; rev 4 reverses.)

- The in-process API server for `--local` was complexity in service of a
  uniformity that the user explicitly doesn't need. `--local` working
  *without* a server is the natural and operator-expected behavior.
- "Server is the source of truth for runtime state" requires server
  restart recovery, claim reconciliation, partition handling, and a
  long-poll back-pressure regime — none of which are needed when the
  bead store is the source of truth.
- Workers being long-lived clients of a server requires the server to be
  available for the worker to function, which the user explicitly
  rejected.

## Worker-server interface (best-effort reporting)

The worker-server boundary is small. There is no HTTP API for claim
coordination, no long-poll, no heartbeat lease, no in-process server.
There are exactly two endpoints, both used best-effort by workers and
both backed by `requireTrusted` (loopback or ts-net per ADR-006):

### POST /api/workers/register

Worker-side: called once at startup AFTER a successful `.ddx/server.addr`
discovery. Body is a small identity envelope:

```json
{
  "project_root": "/abs/path",
  "harness": "claude",
  "model": "",
  "executor_pid": 12345,
  "executor_host": "host.local",
  "started_at": "2026-05-03T03:00:00Z"
}
```

Response:

```json
{ "worker_id": "wkr-9f3a..." }
```

The worker uses `worker_id` as a correlation key in subsequent event
reports. If registration fails (no server, network error, 5xx), the
worker proceeds without a `worker_id` and skips event mirroring. No
session token, no heartbeat, no claim lease — none of those concepts
exist in the worker-server protocol because the worker doesn't depend
on the server.

### POST /api/workers/<id>/event

Worker-side: called best-effort whenever the worker would write an event
to a bead's local event log. Body mirrors the bead event:

```json
{
  "bead_id": "ddx-076147ee",
  "attempt_id": "20260503T021424-dac100b6",
  "kind": "attempt.started" | "picker.priority_skip" | "loop.idle" | "result" | "...",
  "body": { "...": "..." }
}
```

Server appends to its derived view (in-memory + optionally backed by
its own `.ddx/server/events.jsonl`). Response: 204. Worker's local
event log is the authoritative copy; server-side is for observability.

If the POST fails for any reason, the worker logs the failure locally
and continues. Lost events are tolerable because the bead's local event
stream has the full record; operators wanting to backfill can replay
from the bead store.

### What the server can do (the value-add)

Once the server has a derived view of who's reporting:

- **Centralized observability**: which workers exist, what they're doing
  right now, recent picker decisions, recent dropped commits — all
  surfaced via existing GraphQL and the workers panel.
- **Cross-worker correlation**: see all workers across all projects in
  one place; useful for multi-project hosts.
- **Operator UI**: queue beads, cancel beads (via writing markers to the
  bead store that workers honor on next iteration), view bead history.
- **Server-initiated cancel**: not a direct API call to the worker;
  instead, server writes `extra.cancel-requested: true` to the bead;
  worker observes on next iteration of its loop and aborts the attempt
  at the next safe point.

### What the server explicitly does NOT do

- It does not assign beads to workers (workers pick from the bead store
  themselves).
- It does not hold authoritative claim state (the bead store does).
- It does not gate worker startup or operation (workers run without it).
- It does not hold session tokens or enforce per-worker auth (a future
  hardening; v1 trusts the requireTrusted boundary).
- It does not need to survive worker death gracefully (workers are
  independent; server's derived view is replayable from the bead
  stores at any time).

### Server crash / restart behavior

Server crashes: workers' next event POST fails; workers log + continue.
Server restarts: workers' next event POST succeeds (or 410 if the
server didn't recover the worker_id, in which case the worker
re-registers and resumes). No in-flight work is at risk.

## Compatibility analysis

### Migrates cleanly (almost everything)

- **Existing autonomous worker behavior.** This is the baseline rev 4
  preserves. Today's `ddx work` and `ddx work --local` already work
  against the bead store; rev 4 is additive (best-effort server
  reporting on top of unchanged behavior).
- **Bead event log shape.** Worker events go to the bead's local event
  log (today's behavior); the server-side ingestion endpoint receives a
  copy with the same shape. No reader changes.
- **Evidence layout.** `.ddx/executions/<run-id>/` unchanged.
- **`try.Attempt` semantics.** Unchanged. The worker calls into `try`
  exactly as today.
- **`requireTrusted` boundary.** Reused for the two new endpoints; no
  new auth plane.
- **Existing per-Run `attempted` map and stay-alive defaults.** Preserved
  from commit `41cb762e`. The autonomous picker is the picker.

### Breaking changes for operators

- **None for the autonomous path.** `ddx work` and `ddx work --local`
  behave identically to today (with the addition of best-effort server
  reporting when a server is reachable).
- **Some legacy server-spawn integration tests will need updating.** The
  current server-spawn path that hand-marshals `ExecuteLoopWorkerSpec`
  over an internal contract is replaced by the server `exec`'ing
  `ddx work` like an operator would. Tests asserting specific spawn
  args will need to assert the new exec form. (No operator-visible
  behavior change here — operators don't call the legacy endpoint
  directly.)
- **Per-user direction**: the previous `ExecuteLoopWorkerSpec` path
  through the server is removed; no compat shim and no data migration
  for legacy `.ddx/workers/` records (delete history if needed).

### Tests that need updating (smaller list than rev 1-3)

- `cli/internal/server/workers_test.go` — exec-spawn tests: assert the
  new exec form (`ddx work` invocation), not the JSON request/response
  envelope.
- Integration tests asserting specific `.ddx/workers/` on-disk format
  may need adjustment if rev 4 changes that layout. Currently rev 4 does
  NOT modify `.ddx/workers/` shape; workers continue to write spec.json
  + status.json there for `ddx agent doctor` consumption.
- New tests for the two endpoints: `TestWorkerRegister_HappyPath`,
  `TestWorkerRegister_ServerDown_WorkerProceeds`,
  `TestWorkerEvent_Mirrored_BestEffort`,
  `TestWorkerEvent_ServerCrash_WorkerLogs_Continues`.

The tests-that-break list is dramatically smaller than rev 1-3 because
the worker state machine, bead store interaction, and evidence layout
are all unchanged. Only the new server-side ingestion endpoint and its
two tests are net-new.

## References

- Bead `ddx-076147ee` — this TD's source.
- Bead `ddx-29058e2a` — `ExecuteLoopSpec` drift, subsumed by registration payload.
- Bead `ddx-5cb6e6cd` — `run`/`try`/`work` refactor epic; ordering depends on this ADR.
- Bead `ddx-dc157075` — stay-alive fix, subsumed by long-poll worker.
- Bead `ddx-4c51d33e` — cross-project leak; project-binding via session token prevents recurrence in worker paths.
- ADR-006 — ts-net authentication (trust model for `requireTrusted`).
- ADR-007 — federation topology (multi-node ownership of project queues).
- ADR-021 — operator-prompt beads (existing trust pattern this ADR mirrors).
- ADR-004 — bead-backed runtime storage (durable substrate beneath the runtime registry).
- FEAT-006 — agent service (this ADR adds a "Worker contract" section there).
- FEAT-002 — server (the API surface this ADR extends).
- FEAT-010 — executions (the per-attempt evidence layout the worker writes).
