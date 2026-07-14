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
    - ADR-028
---
# ADR-022: Worker Client–Server Architecture

**Status:** Accepted (rev 6 — server-preferred coordination with offline reconciliation)
**Date:** 2026-05-02 (rev 1) / 2026-05-03 (rev 2-5) / 2026-07-11 (rev 6)
**Authors:** TD bead `ddx-076147ee`

**Rev 6 amendments (incident-driven operator clarification):**
- Preserves autonomous workers and continuous server discovery, but extends
  the Connected state from observability-only reporting to coordination of
  claim/lease, tracker-transition, and landing mutations.
- Keeps the bead store and git history authoritative. The server is the
  preferred serializer for those durable stores, not a parallel source of
  truth and not a server-side picker.
- Adds a durable ordered offline mutation journal. Manual workers continue
  locally when disconnected and reconcile idempotently before resuming online
  mutations.
- Makes manual and server-managed workers use one coordination client and
  state machine. Their only difference is process ownership: a server-managed
  worker terminates with the server; a manual worker survives the outage.
- Replaces process-local “single writer” claims with one server coordinator
  while connected and a cross-process project lock while offline.

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
  for `legacy agent doctor` until Gate D removes (one alpha release lag)
- Resolved server address path: standardized as `~/.local/share/ddx/server.addr`
  (XDG-compliant, matches current code) NOT `~/.local/share/ddx/server.addr` (XDG-compliant, matches current code)
- Removed C7 from "NOT FROZEN" list — C7 deletes the per-Run
  `attempted` map that rev 4 preserves; C7 is coordinated, not
  unfrozen
- Expanded roadmap from 5 to 9 beads: added UI/GraphQL consumption,
  `legacy agent doctor` update, FEAT-006 amendment, server-spawn path
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

- **`--local` path.** `ddx work` runs an
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

**Workers are always autonomous and server-preferred. There is no separate
manual/server execution mode. Every worker detects a project-scoped server
continuously. When connected, it sends coordination-sensitive mutations to
the server's per-project coordinator. When disconnected, it operates against
the same durable bead store and git repository under cross-process locks,
journals mutations, and periodically retries. Reconnection reconciles the
journal idempotently before new online writes.**

Concretely:

1. A worker process (`ddx work`) reads the bead store directly and picks the
   next eligible bead per the current worker-side picker. Picking remains
   local; mutation coordination is transport-aware.
2. **Connected:** claim/lease, tracker transitions, and landing go through the
   project-scoped server. The server serializes them against the bead store and
   git repository with durable idempotency keys. Agent execution, worktree
   changes, and evidence capture remain inside the worker.
3. **NotConnected:** the worker performs the same mutations locally under a
   cross-process project coordination lock and appends their request,
   idempotency key, and observed outcome to an ordered durable journal. Lack of
   a server never prevents ordinary work.
4. **In parallel**, a small "server probe" goroutine inside the worker
   periodically (every N seconds, default 30) checks
   `~/.local/share/ddx/server.addr` (XDG-compliant, matches current code) for a reachable server. State transitions:
   - **NotConnected → Connected:** server appeared; worker POSTs
     `register` with a thin identity envelope (project root, harness,
     executor pid/host); on success, worker stores the `worker_id` and
     reconciles any offline mutation journal, then starts sending coordination
     mutations and mirroring events to the server.
   - **Connected → Connected:** server still reachable; continue
     mirroring.
   - **Connected → NotConnected:** server unreachable (crash,
     partition, stopped); worker logs the transition, retains local evidence,
     switches coordination mutations to the journaled offline path, and
     continues working.
   - **NotConnected → NotConnected:** still no server; nothing to do.
5. When in Connected state, the worker mirrors bead events + final
   results to `/api/workers/<id>/event` and the bead-close result to
   the same endpoint best-effort. Failures (server crash mid-POST,
   partition, 5xx) are logged locally and ignored — the bead's local
   event log is authoritative.
6. The server's observational view (which workers exist, which beads are running,
   recent events) is **derived from worker reports** — eventually
   consistent, NOT authoritative. The server's value is centralized
   observability, cross-worker dashboards, operator UI, and
   server-initiated cancel (which writes a marker to the bead store
   that workers honor on next iteration).
7. A manual worker survives server restart, falls back offline, and reconciles
   after re-registration. A server-managed worker is part of the server-owned
   process tree and terminates with the server; desired-state supervision
   restarts it from durable attempt/claim state.

**`--local` flag is removed.** Today's `--local` semantics are the
default behavior; there's nothing to opt into. Operators who pass
`--local` see a deprecation warning + the flag is a no-op (no
"force standalone" behavior — the worker probes for a server normally;
operators wanting to suppress the probe can stop the server). After one
alpha release with the deprecation warning, the flag is deleted. The
breaking-changes section documents this.

This keeps Proposal B's autonomous worker and durable local truth, but the
server is an active serializer while reachable rather than a passive observer.
It can also write to the bead store to influence worker decisions (for example,
cancel or queue-rank changes), which workers see on the next read.

### Why autonomous-default workers

- **Workers don't depend on the server for correctness.** Any server
  restart, partition, crash, or just "no server running" is a no-op for
  the worker's primary job (executing beads).
- **Same code path for every invocation.** `ddx try`, `ddx work`, and
  server-managed workers use one coordination client. Tests exercise both its
  server transport and offline implementation against the same behavioral
  contract.
- **Server availability is optional, coordination is not duplicated.** The
  server provides host-level serialization while connected; offline work uses
  the same operations and idempotency keys under a cross-process lock.
- **No parallel claim truth.** The bead store's atomic claim remains the claim
  primitive. The server invokes it on behalf of connected workers; offline
  workers invoke it locally and later reconcile the recorded outcome.
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
2. Sort by priority ascending (P0 first), then by explicit `queue-rank`
   ascending inside that priority bucket, then by `created_at` ascending
   and `id` ascending for deterministic FIFO-style tie-breaking.
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

### Why not centralized server-side picker

- Workers don't need it for correctness — the bead store's atomic claim
  is sufficient. Adding a server-side claim table creates a parallel
  source of truth that has to be reconciled.
- Centralizing mutation serialization does not require centralizing selection.
  Keeping the picker in the worker preserves offline operation and avoids a
  long-poll scheduling dependency.
- Diagnostic events from commit `80f51574` already make starvation
  observable; centralization didn't add visibility we lacked.

### Why not server-required orchestration

(Earlier revisions chose this; rev 4 reverses.)

- The server is not the source of truth and is not required to execute an
  attempt. Requiring it would turn a control-plane outage into a work outage.
- The shared client therefore has a real offline implementation, not an
  in-process fake server. The local implementation is contract-tested against
  the HTTP implementation and records enough journal state to reconcile.

## Worker-server interface (coordination + best-effort reporting)

The worker-server boundary separates coordination requests from event
reporting. Coordination requests are project-scoped, idempotent, and return a
durable outcome. Event reports remain best-effort. There is no long-poll picker
and no parallel server claim database. Endpoints are backed by `requireTrusted`
(loopback or ts-net per ADR-006).

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
{ "worker_id": "wkr-9f3a...", "coordination_protocol": 1 }
```

The worker uses `worker_id` as a correlation key in subsequent event and
coordination requests. If registration fails, it enters NotConnected and uses
the journaled offline coordinator. A successful registration must be followed
by offline-journal reconciliation before the worker sends new online
coordination mutations.

### POST /api/projects/<project>/coordination/mutations

Submits one coordination mutation with `worker_id`, `operation`,
`idempotency_key`, project-relative payload, and the worker's last observed
bead/git version. V1 operations are `claim`, `renew_claim`, `release_claim`,
`tracker_transition`, and `land`. The server invokes the same bead-store and
landing implementations used offline, serialized by its per-project
coordinator. The response is `applied`, `already_applied`, or `conflict`, plus
the resulting durable version or revision.

### POST /api/projects/<project>/coordination/reconcile

Submits the worker's ordered offline journal after reconnect. The server checks
each idempotency key and durable bead/git state in order. Already-observed
operations are acknowledged without replay; safe missing operations are
applied; incompatible state is returned as a structured conflict and retained
as operator-attention evidence. Reconciliation is bounded and resumable: the
response includes the highest contiguous journal sequence acknowledged.

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
- **Host-local serialization**: coordinate claim/lease, tracker transitions,
  and landing across both manual and server-managed workers without creating a
  parallel source of truth.
- **Operator UI**: queue beads, cancel beads (via writing markers to the
  bead store that workers honor on next iteration), view bead history.
- **Server-initiated cancel**: not a direct API call to the worker;
  instead, server writes `extra.cancel-requested: true` to the bead;
  worker observes on next iteration of its loop and aborts the attempt
  at the next safe point.

### Managed-node control mode

ADR-028 adds a managed-node topology without changing the worker authority
model in this ADR. In managed-node mode, the machine running DDx dials the hub
over ts-net and receives remote-control commands over the outbound channel. The
execution process is still a **worker**; the machine is a **managed node**.

Remote hub commands are requests to the managed node, not central claim
authority:

- **Start worker:** managed node starts the same autonomous `ddx work` loop it
  would start from localhost, scoped to the selected project.
- **Stop/cancel worker:** managed node writes the same local cancellation or
  interruption marker a localhost server request would write; the worker honors
  it at the next safe point.
- **Queue edits and operator prompts:** managed node applies ADR-021 and local
  bead-store rules. The hub does not mutate the bead store directly.
- **Progress/logs:** managed node reports worker events, logs, and backfill to
  the hub's derived view.

Every command carries the ADR-006 identity envelope and a request ID. Replays
with the same request ID are idempotent. If local state has changed, the managed
node rejects the command with a structured reason and the hub surfaces that
rejection to the operator.

### What the server explicitly does NOT do

- It does not assign beads to workers (workers pick from the bead store
  themselves).
- It does not hold authoritative claim state (the bead store does).
- It does not gate worker startup or operation (workers run without it).
- It does not hold session tokens or enforce per-worker auth (a future
  hardening; v1 trusts the requireTrusted boundary).
- It does not own manual-worker lifetime. It does own managed-worker process
  trees, which terminate on server shutdown.
- In managed-node mode, the hub does not become an authoritative worker
  scheduler. It sends commands to the managed node; local authority still wins.

### Server crash / restart behavior

On a server crash, manual workers' next request fails and they switch to the
journaled offline coordinator. Server-managed workers terminate with the
server. After restart, a surviving manual worker re-registers, reconciles the
offline journal, then resumes online coordination. Desired-state supervision
restarts managed workers from durable claim and attempt evidence.

### Probe + freshness state model

Smaller than rev 3's claim/heartbeat model but still needs a single
table. The server-probe goroutine:

| Event | Threshold | Action |
|---|---|---|
| Worker process start | immediate (0s) | Probe `~/.local/share/ddx/server.addr`; on success, register |
| Probe interval (steady state) | 30s default; 10s min; 5min max; jittered ±20% | Re-check reachability; emit register if NotConnected→Connected; nothing if Connected→Connected |
| Connected coordination POST fails or has unknown outcome | immediate | Worker enters NotConnected; journals the idempotent mutation; resolves it locally under the project lock |
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

### Offline coordination journal

Event backfill is not sufficient for coordination. Before applying any offline
coordination mutation, the worker appends a durable journal entry containing a
monotonic sequence, operation, idempotency key, request payload hash, and
precondition. After applying it locally, the worker appends the observed
outcome and resulting bead/git version. The journal is project-scoped and
protected by the same cross-process lock as the mutation. It is compacted only
after the server acknowledges a contiguous sequence. A conflict never causes
automatic replay or rollback; it produces durable operator-attention evidence.

### Cancel SLA

Operator-initiated cancel writes `extra.cancel-requested: true` to the
bead via the server's `/api/beads/<id>/cancel` endpoint (or directly
via the bead store). The worker honors it through the boundary it actually
owns:

- **Mid-attempt poll** (default every 10s during a live operation): the worker
  re-reads the bead's `extra` map. If `cancel-requested: true` appears while
  Fizeau `Execute` is in flight, the worker cancels the context it supplied to
  that call and waits for the public stream to end. DDx has no LLM-turn
  boundary, cancel-by-session method, or authority to signal Fizeau's child
  process tree. Before or after `Execute`, it stops at the next DDx-owned git or
  state-mutation boundary and reports `preserved_for_review` with reason
  `operator_cancel`.
- **Latency semantics**: DDx promises only the polling bound before it requests
  context cancellation. End-to-end termination latency belongs to the pinned
  Fizeau contract; DDx does not estimate it from an underlying agent turn.
- **Idempotency**: a bead with `cancel-requested: true` already worked
  is silently consumed (worker writes `cancel-honored: true` next to
  it); a worker starting work on a bead with `cancel-requested: true`
  immediately reports `preserved_for_review` and skips the attempt.

### Worker shutdown and claim cleanup

Claim cleanup follows TD-027 §12 (storage contract) and TD-031 §6 (operational triage policy) and stays worker-side because the bead store is the
only claim authority. A graceful worker stop releases any claim whose attempt has
not already reached a terminal bead mutation. SIGTERM/SIGINT, child-agent
termination, and operator cancel are recorded as mechanical interruption or
disruption evidence; they do not by themselves justify a retry cooldown. If the
worker dies before cleanup runs, stale-claim recovery is the bounded repair path:
the bead remains `in_progress` only until the configured stale threshold, then
the store releases the claim and appends recovery evidence. The server may show
the worker as stale or disconnected, but it does not own or reconcile bead
claims.

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
  + status.json there for worker diagnostics consumption.
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

## Rev 6 implementation roadmap

1. **Shared coordination client and offline journal** — extend continuous
   discovery with a transport-neutral coordination interface, durable ordered
   journal, cross-process offline lock, and contract tests shared by the local
   and HTTP implementations.
2. **Server coordination endpoints** — expose project-scoped idempotent
   mutation and reconcile endpoints backed by the existing bead store and
   per-project land coordinator.
3. **Uniform worker wiring** — route `ddx try`, manual `ddx work`, and
   `ddx work --server-managed` through the shared client. Remove unconditional
   process-local land coordinators from command construction.
4. **Real integration proof** — run real server, manual worker, and managed
   worker processes against one real git/bead fixture; exercise online
   contention, server loss, offline progress, restart, reconciliation, and
   managed-process death without mocks.

Rev 5's event ingestion, probe, freshness, cancel, UI, and diagnostics work
remains valid and becomes the observability half of the shared client.

## Consequences

- **Manual workers don't depend on the server.** They survive a server outage,
  switch to durable offline coordination, and reconcile after restart.
- **Managed workers share semantics, not lifetime.** They use the same
  coordination client but terminate with the server-owned process tree.
- **No `--local` flag.** Removed from the CLI surface. Behavior of
  today's `--local` becomes the always-on default. Backward compat
  preserved for one release via a no-op flag with deprecation warning.
- **Server is preferred coordination plus observability.** Operators who do
  not run a server retain correct offline behavior. Operators who do get one
  host-local serialization point and centralized visibility.
- **`ExecuteLoopSpec` unification still happens.** Per `ddx-29058e2a` —
  the spec is a worker-side struct, not a wire format. The drop class
  is closed by single-struct discipline; it just doesn't need to cross
  a network boundary.
- **No parallel claim authority.** The bead-store CAS remains the claim
  primitive whether invoked by the server or the offline client.
- **Server death has two explicit outcomes.** Manual workers continue offline;
  server-managed workers exit and are restarted by desired-state supervision.
- **Server restart loses derived view briefly.** Manual workers restore it by
  registration, event backfill, and coordination reconciliation. Managed
  workers restore it when the supervisor relaunches them.
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
