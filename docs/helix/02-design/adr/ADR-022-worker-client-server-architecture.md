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

**Status:** Proposed (rev 5 — fold codex rev 4 review feedback)
**Date:** 2026-05-02 (rev 1) / 2026-05-03 (rev 2-5)
**Authors:** TD bead `ddx-076147ee`

**Rev 5 amendments (per codex review of rev 4):**
- Resolved "No worker registration" banner contradiction: it's "no
  authoritative registration/heartbeat/lease protocol" — there IS a
  thin identity envelope POST for correlation
- Resolved `--local` contradiction: deprecated no-op for one alpha
  release (`v0.6.2-alphaN+1` after rev 5 lands), then deleted; NO
  "force standalone" behavior
- Resolved register cadence contradiction: continuous probe (no "once
  at startup"), with explicit timing/jitter/backoff
- Renamed event log file consistently: `.ddx/server/worker-events.jsonl`
  (single name; corrected from `.jsonl` vs `events.jsonl` slip)
- Added Probe + freshness state table (replaces deleted rev 3 timing
  model with the smaller surface rev 4 actually needs)
- Added Disconnected-work backfill section (worker replays event log
  to server on Connected transition)
- Added Cancel SLA section (worker checks `cancel-requested` every N
  seconds during long attempts; default 10s mid-attempt poll)
- Resolved `.ddx/workers/` contradiction: KEPT as compatibility writer
  for `ddx agent doctor` until Gate D removes (one alpha release lag)
- Resolved server address path: standardized as `~/.local/share/ddx/server.addr`
  (XDG-compliant, matches current code) NOT `~/.local/share/ddx/server.addr` (XDG-compliant, matches current code)
- Removed C7 from "NOT FROZEN" list — C7 deletes the per-Run
  `attempted` map that rev 4 preserves; C7 is coordinated, not
  unfrozen
- Expanded roadmap from 5 to 9 beads: added UI/GraphQL consumption,
  `ddx agent doctor` update, FEAT-006 amendment, server-spawn path
  migration, backfill mechanism
- Added derived-view contract: GraphQL schema fields, freshness
  indicators, stale-state markers, duplicate-worker display
- Added auth-context UI labeling: "trusted-peer reported, not
  authoritative" per codex's threat-model operationalization request

**Rev 4 design pivot (per user direction 2026-05-03):**

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
- No authoritative worker registration, heartbeat, or claim-lease API
  (there IS a thin identity-envelope POST for correlation; workers
  claim from the bead store atomically as today)
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
   `~/.local/share/ddx/server.addr` (XDG-compliant, matches current code) for a reachable server. State transitions:
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
default behavior; there's nothing to opt into. Operators who pass
`--local` see a deprecation warning + the flag is a no-op (no
"force standalone" behavior — the worker probes for a server normally;
operators wanting to suppress the probe can stop the server). After one
alpha release with the deprecation warning, the flag is deleted. The
breaking-changes section documents this.

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

Worker-side: called by the server-probe goroutine on every
NotConnected → Connected transition (NOT just at startup). On
re-connect after server crash, the worker re-registers and gets a new
`worker_id`. Discovery uses
`~/.local/share/ddx/server.addr` (XDG-compliant, matches current code)
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

### Probe + freshness state model

Smaller than rev 3's claim/heartbeat model but still needs a single
table. The server-probe goroutine:

| Event | Threshold | Action |
|---|---|---|
| Worker process start | immediate (0s) | Probe `~/.local/share/ddx/server.addr`; on success, register |
| Probe interval (steady state) | 30s default; 10s min; 5min max; jittered ±20% | Re-check reachability; emit register if NotConnected→Connected; nothing if Connected→Connected |
| Connected POST fails (timeout, conn refused, 5xx) | immediate | Worker enters NotConnected; logs locally; continues working |
| 5 consecutive probe failures | ~2.5 min default | Worker reduces probe rate to 5min (backoff); resets to 30s on next success |
| Server replies 410 unknown_worker | immediate | Worker re-registers within same probe cycle |
| Worker process exit (graceful) | immediate | Best-effort POST disconnect; not required for correctness |

**Freshness indicators** (server-side, surfaced in the workers panel UI):

- `worker_state` ∈ {connected, stale, disconnected}: connected = event
  in last 2× probe interval; stale = 2-10× probe interval; disconnected
  = > 10× probe interval (worker presumed dead)
- `last_event_at`: timestamp of most recent event from this worker
- `mirror_failures_count`: total POST failures since worker_id was issued
  (operator can see if a worker is healthy but lossy)

### Disconnected-work backfill

A worker that started while no server was running may have claimed and
finished beads before reaching Connected. To prevent the operator's UI
from missing those events:

- The worker maintains an in-memory ring buffer of the last N=200
  events it emitted while NotConnected (kind, body, bead_id,
  attempt_id, timestamp).
- On NotConnected → Connected transition, the worker POSTs a
  `/api/workers/<id>/backfill` request with the buffered events. Server
  acknowledges with 204; worker clears the buffer.
- If the buffer overflows (worker generated >200 events while
  disconnected — possible during a long outage or many short attempts),
  oldest events are dropped silently. The bead's local event log
  remains complete; only the server's derived view is incomplete. UI
  marks the worker as `had_dropped_backfill: true` so operators know to
  consult bead-local logs.
- Backfill is best-effort: a 5xx during backfill leaves the buffer
  intact; worker tries again on next NotConnected → Connected.

### Cancel SLA

Operator-initiated cancel writes `extra.cancel-requested: true` to the
bead via the server's `/api/beads/<id>/cancel` endpoint (or directly
via the bead store). The worker honors at the next safe point:

- **Mid-attempt poll** (default every 10s during long attempts): the
  worker re-reads the bead's `extra` map. If `cancel-requested: true`
  appears, the worker aborts the next LLM turn / next git operation
  boundary and reports `preserved_for_review` with reason `operator_cancel`.
- **Worst-case latency**: 10s (mid-attempt poll interval) plus the
  current LLM turn duration (typically 5-30s). Operators expecting
  faster cancel use OS signals.
- **Idempotency**: a bead with `cancel-requested: true` already worked
  is silently consumed (worker writes `cancel-honored: true` next to
  it); a worker starting work on a bead with `cancel-requested: true`
  immediately reports `preserved_for_review` and skips the attempt.

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

## Sequencing relative to in-flight work

Rev 4 dramatically simplifies sequencing. The architecture is additive:
the worker keeps working as it does today; server-side reporting is new
machinery that sits alongside. There is no "freeze C5/C7/C9 until X"
rule because nothing about the worker state machine changes.

- **`ddx-29058e2a` (ExecuteLoopSpec unification)** — independent. Rev 4
  does NOT make ExecuteLoopSpec a wire format. The bead remains valuable
  as a worker-side struct unification (cobra → in-process flow) and
  should land independently.
- **`ddx-5cb6e6cd` C5 (no_changes adjudication)** — NOT FROZEN. Rev 5
  doesn't reshape no_changes generation; mirroring stays an observer.
- **`ddx-5cb6e6cd` C7 (uniform Guard contract; delete attempted/hookFailed maps)** — **COORDINATE.** Direct conflict: rev 5's worker keeps the per-Run `attempted` map (preserved from commit `41cb762e`); C7 wants to delete it. Resolution: C7 lands BEFORE rev 5 implementation work, OR rev 5 implementation work coordinates with C7's replacement (Guard-derived exclusion). Until coordinated, C7 stays parked.
- **`ddx-5cb6e6cd` C9 (StopCondition + cost-cap)** — NOT FROZEN. The autonomous worker keeps the stay-alive defaults; C9's StopCondition becomes an internal worker construct, server sees terminal state via the result event.
- **`ddx-dc157075` (stay-alive at commit `41cb762e`)** — already shipped
  and preserved. Worker's autonomous behavior plus 30s poll-interval
  default carry forward unchanged.
- **`ddx-4c51d33e` (cross-project leak at commit `33b97f25`)** — already
  shipped. Rev 4 prevents recurrence at the worker level (one worker
  per project bead store; no cross-store reads).
- **`ddx-9d55601f` (picker priority bug at commit `80f51574`)** — picker
  diagnostic events stay; rev 4 explicitly mirrors them to the server
  endpoint when the worker is Connected.
- **`ddx-9e4c238d` (auto-routing rejection bug, just filed)** —
  independent. Fixing DDx's pre-flight to pass through to fizeau is
  orthogonal to the autonomous-default architecture. Should land in
  parallel.
- **`ddx-1d867ec1` (rename `execute_bead_loop.go`)** — coordinate. The
  rename should align with whatever package layout the worker uses
  post-rev-4. Probably stays as `cli/internal/agent/execute_bead_loop.go`
  for now since rev 4 doesn't reshape the worker package; revisit when
  C13 lands.
- **`ddx-076147ee` (this TD)** — closes when the rev 4 ADR is approved
  and the implementation roadmap below is filed as beads.
- **`ddx-50da9674` (clean fixture repo)** — supports rev 4's acceptance
  test; independent.

## Implementation roadmap

Rev 5 expands from rev 4's 5 beads to 9, after codex flagged missing
UI/doctor/FEAT-006/server-spawn-migration/backfill scope.

1. **server: ingestion endpoints** — implement `POST /api/workers/register` + `POST /api/workers/<id>/event` + `POST /api/workers/<id>/backfill`; in-memory derived view backed by append-only `.ddx/server/worker-events.jsonl`; freshness state machine (connected/stale/disconnected); requireTrusted boundary. ~300 LOC.

2. **worker: server-probe goroutine + best-effort event mirror** — periodic reachability check on `~/.local/share/ddx/server.addr` (XDG-compliant, matches current code) with jittered 30s interval, immediate first probe, exponential backoff on consecutive failures; transitions emit register/disconnect; in-memory ring buffer for backfill (200-event cap); event POSTs are best-effort. ~250 LOC.

3. **server: operator-cancel** — `/api/beads/<id>/cancel` writes `extra.cancel-requested: true`; worker mid-attempt poll (10s default) checks bead `extra` and aborts at next safe point with `preserved_for_review` reason `operator_cancel`; idempotency via `cancel-honored: true`. ~150 LOC.

4. **CLI: deprecate `--local`** — flag becomes no-op with deprecation warning; update CLAUDE.md, AGENTS.md, getting-started, and the cobra help text. Existing tests asserting `--local`-specific behavior get rewritten or deleted. (Codex notes the sweep is larger than 50 LOC: `cli/cmd/work_test.go`, `agent_execute_loop_test.go`, `zero_config_work_test.go`, `skills/ddx/reference/work.md`, demos.) ~150 LOC + doc edits.

5. **server: derived-view GraphQL + UI workers panel** — schema fields: `workers { id, project, harness, state, last_event_at, mirror_failures_count, had_dropped_backfill, current_bead, current_attempt }`; UI surfaces freshness indicators + duplicate-worker display + the "trusted-peer reported, not authoritative" labeling. Existing workers panel migrated. ~400 LOC frontend + ~100 LOC backend.

6. **`ddx agent doctor` migration** — read worker state from server's runtime registry when available; fall back to `.ddx/workers/` on-disk files when no server. The on-disk format stays as the fallback-source-of-truth for one alpha release lag, then deprecated. ~150 LOC.

7. **server-spawn path migration** — `cli/internal/server/workers.go` and `handleStartExecuteLoopWorker` switch from hand-marshalled spec to `exec ddx work` with env-vars. The legacy spec serialization deletes when this lands. ~200 LOC + test updates.

8. **FEAT-006 amendment** — replace the rev 3 worker-contract section with rev 5's autonomous-default + best-effort-mirror description. Drop session-token/heartbeat/`next-bead`/result/disconnect contract; add register/event/backfill/cancel + freshness state. ~200 lines docs.

9. **acceptance + soak** — uses `ddx-50da9674` clean fixture repo. Multi-worker drain WITH server; restart server mid-flight; verify worker continues + reconnects + backfills. Then drain with NO server; verify identical behavior. Operator-cancel test (write marker, observe `preserved_for_review` with `operator_cancel` reason within SLA). Soak: ≥1 week of normal operator use. Final bead.

Plus one COORDINATION bead, NOT new work but a parking-lot for the
C7 conflict:

10. **coordinate: C7 (Guard contract, delete attempted map) with rev 5 worker** — either C7 lands first and rev 5 implementation uses Guard-derived exclusion, OR C7 stays parked until rev 5's worker code defines its replacement for the per-Run attempted map. Decision documented; no LOC.

Total estimated effort: 2-4 weeks (vs 1-2 weeks for rev 4's understated 5-bead roadmap, vs 8-10 weeks for rev 3's design).

## Consequences

- **Workers don't depend on the server.** The headline operator
  requirement (server restart preserves in-flight work) is satisfied
  by construction: workers don't notice server restarts as anything
  more than transient mirror failures.
- **No `--local` flag.** Removed from the CLI surface. Behavior of
  today's `--local` becomes the always-on default. Backward compat
  preserved for one release via a no-op flag with deprecation warning.
- **Server is opt-in observability.** Operators who don't run a server
  see no behavior change from today. Operators who do see
  centralized event aggregation, cross-worker visibility, and an
  operator UI. Both are correct configurations.
- **`ExecuteLoopSpec` unification still happens.** Per `ddx-29058e2a` —
  the spec is a worker-side struct, not a wire format. The drop class
  is closed by single-struct discipline; it just doesn't need to cross
  a network boundary.
- **No claim coordination races.** Bead store's atomic claim is the
  only claim primitive. Two workers contending use the same store
  CAS that has worked correctly for as long as DDx has had a bead
  store.
- **No "kill server → workers exit" tests left valid.** Any test that
  relied on this behavior gets updated to assert "kill server →
  workers continue working autonomously."
- **Server restart loses derived view briefly.** The server's view of
  who's working what comes back within one probe cycle (default 30s)
  as workers re-register. No in-flight work is at risk.
- **Operator UX for cancel changes slightly.** Today operators kill
  worker subprocesses or wait. After: operators send cancel via the
  server (or write the marker to the bead store directly); worker
  honors at next safe point. Documented in CHANGELOG.
- **Evidence layout unchanged.** `.ddx/executions/<run-id>/` still
  receives all per-attempt artifacts; worker writes there exactly as
  today.

## References

- Bead `ddx-076147ee` — this TD's source.
- Bead `ddx-29058e2a` — `ExecuteLoopSpec` drift; rev 5 keeps this as a worker-side struct unification (NOT a wire format).
- Bead `ddx-5cb6e6cd` — `run`/`try`/`work` refactor epic; rev 5 sequences C5/C9 in parallel, C7 coordinated.
- Bead `ddx-dc157075` — stay-alive fix at commit `41cb762e`; rev 5 keeps the autonomous-loop semantics.
- Bead `ddx-4c51d33e` — cross-project leak (LAYER 1 at commit `33b97f25`); rev 5 prevents recurrence at the worker level (one worker per project bead store).
- Bead `ddx-9d55601f` — picker priority bug at commit `80f51574`; diagnostic events mirror to server when Connected.
- Bead `ddx-9e4c238d` — auto-routing rejection bug; orthogonal to rev 5.
- Bead `ddx-50da9674` — clean fixture repo; rev 5's acceptance soak depends on it.
- ADR-006 — ts-net authentication (trust model for `requireTrusted`).
- ADR-007 — federation topology (multi-node ownership of project queues).
- ADR-021 — operator-prompt beads (existing trust pattern this ADR mirrors).
- ADR-004 — bead-backed runtime storage (durable substrate beneath the runtime registry).
- FEAT-006 — agent service (this ADR adds a "Worker contract" section there).
- FEAT-002 — server (the API surface this ADR extends).
- FEAT-010 — executions (the per-attempt evidence layout the worker writes).
