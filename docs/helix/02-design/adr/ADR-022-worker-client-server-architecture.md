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

**Status:** Proposed (rev 2 — amended 2026-05-03 per fresh-eyes review)
**Date:** 2026-05-02 (rev 1) / 2026-05-03 (rev 2)
**Authors:** TD bead `ddx-076147ee`

**Rev 2 amendments:** added Threat model section (limits of session-token
project binding); added Picker section (priority sort + capability/label
filter + atomic claim); expanded `register` payload to full ExecuteLoopSpec
(18 fields); added claim-lease-renewal semantics + `terminate` safety
clarification to `heartbeat`; disambiguated `preserved` outcome into
`preserved_for_review` (worker) vs `preserved_orphaned` (server); added
dropped-attempt commit preservation (`refs/dropped/<bead-id>/<attempt-id>`);
added long-poll back-pressure limits (8 per project, 1s min between polls);
added Transport error-shape parity rule (HTTP and in-process return same
envelope); added Capabilities dispatch rules; added ts-net partition
recovery section with `partition_grace_seconds` / `partition_abort_seconds`;
added Test Transport bead to roadmap; clarified FEAT-008 reference does
NOT block UI bead.
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

**Workers are long-lived API clients of the server. The CLI `--local` mode
is a degenerate case of the same client, served by an in-process server
implementation.**

Concretely:

1. A worker process (whether started by `ddx work`, `ddx agent
   execute-loop`, or spawned by the server) registers with a server
   endpoint (loopback or in-process) and receives a `worker_id` and
   `session_token`.
2. The worker drives its own state machine (`idle → claiming → executing →
   reviewing → idle`) and pulls bead assignments from the server via a
   long-poll endpoint.
3. The worker reports progress and final results to the server via API.
4. The server holds the authoritative view of which workers exist, what
   they're doing, and which beads are claimed.
5. If the server restarts, workers detect heartbeat failure, finish the
   current bead in their isolated worktree, and reconnect on server return.
6. If a worker dies, the server's heartbeat-timeout reclaims the bead for
   another worker.

This is **Proposal A** from the bead description, with the **`--local`
collapse** detail of Proposal C: there is one code path, parameterised by
which API implementation the worker is wired to (HTTP for cross-process,
in-process function calls for `--local`).

### Why Proposal A (server orchestrates, workers are long-lived clients)

- **Restart-survival is the requirement, not a nice-to-have.** Operators
  reported the current model loses in-flight work on server restart.
  Proposal A is the only one of the three that makes the worker survive
  restart by design — Proposal B sidesteps the question by removing the
  server, and Proposal C makes restart survival contingent on a fallback
  mode.
- **Single source of truth for queue state.** Today the bead store is the
  source of truth, and both the server and workers query it. With
  Proposal A the server owns the *runtime* projection of queue state
  (claims, in-flight attempts, heartbeats); the bead store remains the
  durable substrate. This eliminates the class of bugs where two readers
  of the store reach inconsistent conclusions.
- **One spec, one boundary.** `ExecuteLoopSpec` becomes the registration
  payload. `ddx-29058e2a` is solved by construction: there is exactly one
  struct that crosses one boundary (register), instead of six layers of
  parallel structs.
- **Aligns with the `run/try/work/` split.** `ddx-5cb6e6cd`'s `work/`
  package becomes the worker client; `try/` is invoked by the worker on
  each claim; `run/` is the inner agent invocation. The state machine
  becomes explicit, server-visible, and testable.
- **Project boundaries are enforced once.** The session token is bound to
  a project at registration; every subsequent API call carries it. The
  cross-project leak class (`ddx-4c51d33e`) cannot recur for any
  worker-driven mutation because the token is the project.

### Threat model (limits of session-token project binding)

Session tokens prevent **honest worker mixup** (a misconfigured worker for
project A cannot accidentally claim a bead from project B). They are NOT a
defense against a **local malicious process**: `requireTrusted` accepts any
loopback connection without identity proof, so any process on the same host
can register as any project. The session-token model is project-scoping for
legitimate workers, not authentication of worker identity. Future work to
strengthen this (e.g., per-worker keypairs, ts-net WhoIs binding for
loopback registrations) is out of scope for v1 and tracked separately.

### Picker (server-side bead → worker assignment)

When multiple workers long-poll `next-bead` concurrently, the server picks
which bead each worker receives using these rules, in order:

1. **Filter by worker capabilities.** The bead's required `kind:`-label or
   inferred capability (e.g., `review` for review beads) must intersect the
   worker's `capabilities` set from registration. Beads with no capability
   constraint are eligible for any worker.
2. **Filter by worker `label_filter`.** The bead's labels must intersect
   the worker's `label_filter` set; an empty filter accepts all beads.
3. **Filter by execution eligibility** (existing `ReadyExecution`
   semantics): not in retry-cooldown, not superseded, not flagged
   `execution-eligible: false`, all deps closed.
4. **Sort eligible beads** by priority ascending (P0 first), then by
   `updated_at` ascending (oldest first within priority — FIFO at each
   tier).
5. **Assign one bead per polling worker** in arrival order on the
   long-poll. Two workers polling for the same top bead: first arrival
   gets the bead and a claim record; the second worker's poll either
   returns the next eligible bead or waits.
6. **Claim is atomic** at the moment of assignment. The picker holds a
   server-side mutex around the eligibility scan + claim creation so two
   concurrent picks cannot collide.

This makes priority sort a **server-enforced property**, not a worker
heuristic. The current per-Run `attempted` map disappears entirely (no
longer needed because the server prevents re-handing-out a bead to the
same worker that just rejected/failed it).

The diagnostic events from the stay-alive fix at commit `41cb762e`
(`picker.priority_skip`, `picker.claim_race`, `loop.idle`/`loop.active`
substate) migrate to the server's picker as: skip events when the picker
passes over a higher-priority bead because no worker matches its
constraints; claim_race events on the server-side mutex contention; and
idle/active substate from the worker's heartbeat `state` field.

### Why not Proposal B (server-as-passive-observer)

- Loses centralised cancel, dynamic priority, and label-filter overrides —
  all of which are on the FEAT-002 / FEAT-006 roadmap.
- Two execution paths persist forever (autonomous local vs.
  server-observed), which is exactly the divergence this TD is trying to
  retire.
- Best-effort event emission is a worse audit trail than what beads
  already give us via `ADR-004`/`ADR-021` (audit-as-bead).

### Why not Proposal C (hybrid with autonomous fallback)

- Hybrid keeps two execution paths and adds a third (the fallback
  transition itself), each with its own race conditions.
- The "fall back to autonomous on heartbeat timeout" branch is the most
  dangerous failure surface: a worker that decides on its own to keep
  running can race a returning server that has already reclaimed the bead.
- The legitimate part of Proposal C — that `--local` should not require a
  network server — is preserved by serving the API in-process for
  `--local`.

## Worker–server API contract

All endpoints require `requireTrusted` (loopback or ts-net `WhoIs`
identity, per ADR-006). The session token is bound to one project; the
server rejects any call whose `worker_id` was issued for a different
project.

### POST /api/workers/register

Request — the full `ExecuteLoopSpec` plus runtime worker identity. **All
fields currently scattered across `cli/cmd/agent_cmd.go`,
`cli/internal/server/server.go:handleStartExecuteLoopWorker`, and
`cli/internal/server/workers.go:ExecuteLoopWorkerSpec` collapse here:**

```json
{
  "spec_version": 1,
  "project_root": "/abs/path/to/project",

  "harness": "claude",
  "model": "",
  "model_ref": "",
  "profile": "",
  "provider": "",
  "effort": "",

  "label_filter": ["phase:2", "kind:fix"],
  "capabilities": ["bead-attempt", "review"],

  "from_rev": "",
  "once": false,
  "no_review": false,
  "review_harness": "",
  "review_model": "",

  "max_cost_usd": 0.0,
  "request_timeout_ms": 0,
  "min_power": 0,
  "max_power": 0,
  "opaque_passthrough": false,

  "executor_pid": 12345,
  "executor_host": "host.local"
}
```

Response (201):

```json
{
  "worker_id": "wkr-9f3a...",
  "session_token": "tkn-...",
  "heartbeat_interval_ms": 5000,
  "claim_lease_ms": 60000,
  "long_poll_seconds_default": 30,
  "long_poll_seconds_max": 60,
  "server_build_sha": "abc1234"
}
```

Field semantics:

- `spec_version`: integer; server returns 400 if unsupported. v1 freezes
  the schema above. New fields go in v2 with explicit migration.
- `harness`/`model`/`provider`/`profile`/`effort`/`model_ref`: routing
  intent passed to fizeau on each `try.Attempt`. Empty fields trigger
  fizeau's profile/auto-route (per `ddx-1e516bc9`).
- `from_rev`: optional base revision pin for attempts; empty = use
  bead's `metadata.base-rev`.
- `opaque_passthrough`: when true, `harness`/`model`/`provider` are
  passed to fizeau as opaque hints with no DDx-side validation
  (preserves `ddx work --harness=<name>` semantics).
- `max_cost_usd` / `request_timeout_ms` / `min_power` / `max_power`:
  per-attempt budget caps; zero = use server defaults.
- `capabilities`: declares what the worker is willing to do; the picker
  filters eligible beads against this set.
- `label_filter`: bead-side filter, intersected with bead labels by the
  picker.
- `executor_pid` / `executor_host`: for the server's runtime registry;
  used by `ddx agent doctor` to surface worker provenance.

Drop is impossible by construction: every field that today travels through
the `agent_cmd.go → server.go → workers.go → runWorker` chain is one
struct field here. Adding a new flag = (a) one struct field + (b) one
cobra binding. The drop class (`ddx-29058e2a`) is closed.

### POST /api/workers/<id>/heartbeat

Request:

```json
{
  "state": "executing",
  "current_bead_id": "ddx-076147ee",
  "current_attempt_id": "20260503T021424-dac100b6",
  "queue_depth_seen": 3
}
```

Response (200):

```json
{
  "server_command": "continue",
  "session_token_renewed": null,
  "claim_lease_extended_until": "2026-05-03T02:15:24Z"
}
```

`server_command ∈ {continue, pause, drain, terminate}`:

- `continue`: keep going.
- `pause`: finish the current bead and stop claiming new work; keep
  heartbeating so the server sees you're alive.
- `drain`: finish the current bead, then exit cleanly. Worker decrements
  to `state: draining` and reports `disconnect` when idle.
- `terminate`: stop the current attempt at the next safe point (between
  LLM turns, NOT mid-tool-call), preserve the worktree (so a human can
  inspect), then exit. The worker is responsible for waiting for any
  in-flight git or subprocess to complete before exiting; the server does
  NOT SIGKILL — operators wanting hard kill should use OS signals
  directly.

**Claim lease renewal.** Heartbeats while `state: executing` or
`state: claiming` implicitly extend the claim lease by `claim_lease_ms`
(default 60s) measured from the heartbeat receipt time. The server
returns the new expiry in `claim_lease_extended_until`. A worker that
fails to heartbeat for `3× heartbeat_interval_ms` loses the claim
(server reclaims the bead) and the next heartbeat receives `410
unknown_worker`, triggering re-registration.

Heartbeat is the mutual liveness signal AND the claim-renewal signal.
Workers must heartbeat at least every `heartbeat_interval_ms` regardless
of state.

### GET /api/workers/<id>/next-bead

Long-poll. Server holds for up to `wait_for_seconds` (default 30) before
returning either an assignment or a bare `wait` envelope. The worker MAY
reissue immediately on bare-wait response.

Response (200):

```json
{
  "bead": { "...": "..." },
  "attempt_id": "20260503T021424-dac100b6",
  "base_rev": "528bb6ee...",
  "claim_lease_ms": 60000
}
```

Or:

```json
{ "wait_for_seconds": 30 }
```

Claim is created server-side at the moment of return; the worker has
`claim_lease_ms` to send the first heartbeat with `state: claiming` or
`executing` before the claim expires.

### POST /api/workers/<id>/event

Request:

```json
{
  "kind": "attempt.started",
  "bead_id": "ddx-076147ee",
  "attempt_id": "20260503T021424-dac100b6",
  "body": { "...": "..." }
}
```

Response: 204.

`kind` mirrors today's bead event log. The server appends to the bead's
event stream; existing readers (CLI, MCP, web UI) see worker events
without code changes.

### POST /api/workers/<id>/result

Request:

```json
{
  "bead_id": "ddx-076147ee",
  "attempt_id": "20260503T021424-dac100b6",
  "status": "merged",
  "evidence_dir": ".ddx/executions/20260503T021424-dac100b6",
  "commit_sha": "deadbeef...",
  "no_changes_rationale": null,
  "preserve_ref": null
}
```

`status` enum (named distinctly to avoid the previous "preserved" overload):

- `merged`: attempt produced a commit; merged into the integration branch.
- `preserved_for_review`: worker explicitly requested human review (merge
  conflict it can't resolve, ambiguous outcome, escalation request).
  Worktree retained at `preserve_ref`.
- `no_changes`: model returned cleanly with no commit; rationale at
  `no_changes_rationale`.
- `failed_rejected`: attempt produced a commit but the gate (review,
  reachability check, etc.) rejected it. Worktree retained at
  `preserve_ref` for inspection.

**`preserved_orphaned`** is set by the SERVER (not the worker) when the
worker dies mid-attempt and the heartbeat-timeout fires. Reported via the
worker death recovery path, not via this endpoint. UI consumers must
distinguish:

- `preserved_for_review` → human action requested
- `preserved_orphaned` → infrastructure failure; another worker may
  retry

Response: 204.

### POST /api/workers/<id>/disconnect

Graceful shutdown. Releases any unclaimed lease, marks the worker as
gone in the runtime registry, but does not clean up worktrees. Response:
204.

### Error envelope

All endpoints share the existing server JSON error envelope (matching the
`serverPromptCapBytes` cap-error shape from ADR-021's prompt-cap rule).

### Long-poll back-pressure

`next-bead` is a held connection. With multiple workers per project,
connection pools fill quickly. Limits:

- Per-project: maximum `8` concurrent `next-bead` long-polls. The 9th
  worker's poll returns `503 too_many_pollers` with `Retry-After: <ms>`.
- Per-worker: minimum `1s` between successive `next-bead` requests after
  a bare-wait response (worker SHOULD honor; server enforces with
  `429 too_many_polls`).
- Long-poll wait is capped at `long_poll_seconds_max` from registration
  (default 60); workers may request shorter via `?wait=N`.

These defaults are operator-tunable in `.ddx/config.yaml` under
`server.workers.long_poll`.

### Concurrent transport error semantics (HTTP and in-process parity)

The HTTP transport returns standard 4xx/5xx with the JSON error envelope;
the in-process `--local` transport must surface the SAME envelope, not
raw Go errors, to preserve behavior parity. The `Transport` interface is
shaped around `(*Response, error)` where `Response` carries the envelope
on non-2xx outcomes; `error` is reserved for transport-layer failures
(connection refused, ts-net partition). This guarantees that a unit test
written against the in-process transport observes the same error shape as
production.

### Capabilities dispatch

`capabilities` declares what the worker is willing to do. Today's recognised
capabilities:

- `bead-attempt`: the worker can perform a `try.Attempt` for an
  implementation bead.
- `review`: the worker can perform post-attempt review (cf. the
  reviewer harness/model registration).

A bead's required capability is inferred from its `kind:`-label
(`kind:review` → requires `review`; everything else → requires
`bead-attempt`). Workers without the required capability are skipped by
the picker for that bead. Workers MAY register multiple capabilities;
workers MUST register at least one.

Future capabilities (e.g., `compute:gpu`, `network:internet`) extend this
without ADR change as long as the picker rule (intersection) is preserved.

## Sequence diagrams

### Registration

```mermaid
sequenceDiagram
  participant W as Worker (ddx work)
  participant S as Server (or in-proc API)
  W->>S: POST /api/workers/register {spec}
  S-->>W: 201 {worker_id, session_token, hb_interval}
  W->>W: Spawn heartbeat goroutine
  W->>S: GET /api/workers/<id>/next-bead (long-poll)
```

### Bead claim and execute

```mermaid
sequenceDiagram
  participant W as Worker
  participant S as Server
  participant T as try.Attempt
  W->>S: GET /next-bead (long-poll)
  S->>S: Pick eligible bead, create claim
  S-->>W: 200 {bead, attempt_id, base_rev}
  W->>T: try.Attempt(bead, base_rev)
  T-->>W: events stream
  loop per event
    W->>S: POST /event {kind, body}
    S-->>W: 204
  end
  T-->>W: Outcome{status, commit, evidence}
  W->>S: POST /result {status, ...}
  S-->>W: 204
  W->>S: GET /next-bead (next iteration)
```

### Server restart recovery

```mermaid
sequenceDiagram
  participant W as Worker
  participant S as Server
  W->>S: POST /heartbeat
  Note over S: server crashes
  W--xS: 503 / connection refused
  W->>W: Mark "disconnected", continue current attempt
  Note over S: server returns
  W->>S: POST /heartbeat
  S-->>W: 410 unknown_worker
  W->>S: POST /register {same spec, prior_worker_id?}
  S-->>W: 201 {new worker_id, session_token}
  W->>W: Finish in-flight attempt
  W->>S: POST /result {status: merged} (using new id)
  S->>S: Reconcile result with reclaimed claim if any
```

The reconciliation rule: if the server already reissued the bead to
another worker after heartbeat-timeout, the late `result` from the
original worker is recorded as a dropped attempt with
`reason: post_restart_late_result` and the new worker's outcome wins.
The bead store's existing claim semantics (one in-flight attempt per
bead) are preserved.

**Dropped-attempt commit preservation.** If the original worker's late
`result.commit_sha` is non-empty, the server MUST preserve that commit
under a forensic ref `refs/dropped/<bead-id>/<attempt-id>` before
recording the dropped event. The branch is local-only (not pushed) and
operators can inspect via `git log refs/dropped/<bead-id>/<attempt-id>`
or `ddx exec show --dropped <attempt-id>`. Retention: 30 days; pruned by
`ddx server gc` (a future bead). This prevents silent loss of the
original worker's work product when restart races a late commit.

### Worker death recovery

```mermaid
sequenceDiagram
  participant W as Worker
  participant S as Server
  W->>S: POST /heartbeat (state: executing)
  Note over W: worker process dies
  S->>S: heartbeat_timeout (3× hb_interval) elapses
  S->>S: Reclaim bead, mark attempt as preserved_orphaned
  S->>S: New eligible workers see bead in next-bead poll
```

The `preserved_orphaned` outcome is distinct from the worker-reported
`preserved_for_review` (see `/result` endpoint). Orphaned attempts may
be retried automatically by the next eligible worker; review-preserved
attempts wait for human action.

### ts-net partition recovery (remote workers)

ADR-022 v1 explicitly limits remote workers to ts-net-bound nodes per
ADR-006. If a remote worker partitions from the server:

- Worker continues executing the current attempt; cannot heartbeat or
  reach `/event`/`/result`.
- After `partition_grace_seconds` (default `300s`), the worker enters
  `state: partitioned` and stops trying API calls; logs locally.
- After `partition_abort_seconds` (default `1800s` = 30 min), the
  worker aborts the current attempt at the next safe point, preserves
  the worktree under a local marker, and exits.
- The server reclaims the bead at `heartbeat_timeout` and reassigns;
  the abandoned worktree on the partitioned host requires manual
  cleanup (`ddx server reconcile-orphans` — future bead).

Without these limits, a partitioned worker could hold a worktree
indefinitely. Both timeouts are operator-tunable per server.

### Graceful shutdown

```mermaid
sequenceDiagram
  participant Op as Operator
  participant W as Worker
  participant S as Server
  Op->>W: SIGINT
  W->>S: POST /heartbeat {state: idle}
  S-->>W: {server_command: drain}
  W->>S: POST /disconnect
  W->>W: exit 0
```

If the worker is mid-attempt when SIGINT arrives, it sets
`state: draining`, finishes the attempt (so worktree state lands or is
preserved coherently), reports `result`, then `disconnect`s.

## --local mode collapses into the same path

`ddx work --local` (and `ddx try`, which is a one-shot drain) construct an
**in-process server** that implements the same handler surface as the HTTP
server, backed by direct calls into the bead store and the runtime claim
table. The worker uses the same client; the `Transport` interface
(HTTP-or-direct) is the only difference.

```text
cli/internal/agent/work/
  client.go        // worker-side: register, heartbeat, claim, event, result
  transport.go     // interface: HTTP impl OR in-process impl
  local_api.go     // in-process implementation (used by --local)
  worker.go        // the state machine using client+try
```

This means:

- **One state machine.** The `ddx-5cb6e6cd` refactor's `work/` package
  collapses to the client + state machine; the in-process API is just a
  different `Transport`.
- **No "two paths" tests.** Today's `--local` vs server-spawned tests
  collapse to one path with two transport implementations. Existing
  integration tests can be reused with the in-process transport.
- **Server-spawned workers are not special.** The server starts a worker
  by exec'ing `ddx work` with the right env (server URL + bootstrap
  token) — exactly what an operator running `ddx work` against a remote
  ts-net node does. The "spawn-and-track-PID" hand-roll in
  `cli/internal/server/workers.go` retires.

## Compatibility analysis

### Migrates cleanly

- **Bead event consumers.** Events posted via `/api/workers/<id>/event`
  use the same `kind`/`body` shape as today's bead events; the server
  appends them to the same store. CLI, MCP, and web-UI readers do not
  change.
- **Evidence layout.** `.ddx/executions/<run-id>/` continues to be the
  per-attempt evidence directory; the worker writes there exactly as
  `try.Attempt` does today.
- **Land-coordinator and post-merge review.** The worker calls into the
  same `try` package; merge / preserve / no-changes semantics are
  unchanged.
- **`requireTrusted` boundary.** All worker endpoints reuse ADR-006's
  loopback-or-ts-net rule; no new auth plane is introduced.
- **`.ddx/workers/` runtime files.** Replaced by the server's runtime
  registry; the on-disk file format may be retired or kept as a debug
  view of what the server reports for `ddx agent doctor`.

### Breaking changes for operators

- **`--local` semantics shift slightly.** Today `--local` does not start a
  server; tomorrow it starts an in-process API. There is no observable
  difference for the operator (no port is opened), but a test that
  asserted "no listener exists in `--local` mode" needs updating to
  assert "no TCP listener," not "no API."
- **Server-spawned worker restart behaviour.** Today a server-spawned
  worker dies if the server dies. Tomorrow it survives, finishes the
  bead, and reconnects. Any test that relied on "kill server → workers
  exit" becomes wrong. Documented as a deliberate behaviour change; the
  CHANGELOG must call it out.
- **`ExecuteLoopWorkerSpec` consolidation.** The hand-marshalled
  request/response struct between `cli/cmd/agent_cmd.go` and
  `cli/internal/server/server.go` is replaced by the `register` payload.
  Any external script that POSTed to the legacy `/api/agent/workers/...`
  endpoints needs migration.
- **Graceful drain command.** `ddx agent stop-loop` and the server's
  `stopWorker` mutation become wrappers over `POST /heartbeat`'s
  `server_command: drain` reply. Operators using these commands see the
  same outcome; the underlying mechanism changes.

### Tests that break (and the migration)

- `cli/internal/server/workers_test.go` — exec-spawn tests need to
  exercise the new register/poll flow. Most assertions about exec args
  become assertions about the registration payload.
- `cli/cmd/agent_execute_loop_test.go` — `--local` tests run against the
  in-process transport; assertions about "no server" may need rewording.
- `cli/internal/agent/execute_bead_loop_test.go` — the loop is replaced
  by the worker state machine; tests migrate to the `work/` package.
- Integration tests under `cli/internal/server/integration/` that asserted
  specific `.ddx/workers/` filesystem state need rewriting to query the
  server's runtime registry.

A pre-merge checklist (recorded against the implementation epic) tracks
each test file's migration status.

## Sequencing relative to in-flight work

This ADR sits **above** these beads:

- `ddx-29058e2a` (ExecuteLoopSpec unification) — **subsumed.** The unified
  spec is the registration payload; this ADR satisfies the goal and the
  bead either closes as duplicate or becomes "implement registration
  payload per ADR-022."
- `ddx-5cb6e6cd` (refactor epic) — **prerequisite stays valid, ordering
  shifts.** The `run/`/`try/`/`work/` split is independently good. After
  this ADR lands, the `work/` package's contents are the worker state
  machine plus the API client, not the current loop body. C5/C7/C9 of
  the refactor are re-scoped before they execute so they don't refactor
  against the old model.
- `ddx-dc157075` (stay-alive fix) — **subsumed.** A long-lived API client
  stays alive by definition; the fix degenerates to "the worker
  long-polls `next-bead`, so empty queue is not a termination signal."
  The default-poll-interval flip is no longer needed. The bead either
  closes as duplicate or shrinks to "ensure long-poll defaults are
  correct."
- `ddx-4c51d33e` (cross-project leak) — **closed; ADR-022 prevents
  recurrence.** Project scoping moves from "every reader checks" to
  "session token binds project, server enforces."

Implementation MUST land **before** any execution-path refactor children
of `ddx-5cb6e6cd` that touch the loop body (C5, C7, C9 in that epic),
otherwise those children refactor against the old model and need
re-doing.

Implementation MAY land in parallel with bug fixes that don't touch the
execution path (e.g. graphql layer-2/layer-3 follow-ups, persona
lifecycle, library registry).

## Implementation roadmap

Ordered list of follow-up beads to file (titles only — actual filing
happens after this ADR is approved). Each bead carries
`depends_on: ADR-022` and the parent that owns its area.

1. **TD: define worker–server API contract in `cli/internal/server/api/worker.go`** — schemas (per the registration payload's full ExecuteLoopSpec), error envelope, OpenAPI annotations; no behaviour change.
1a. **TD: define `Transport` interface and test-mode implementation** — interface contract (HTTP/in-process/test); deterministic-timing test transport with controllable clock for migration of existing tests under `Tests that break`.
2. **server: implement runtime worker registry (in-memory; durability deferred)** — the source of truth for "which workers exist, what they're doing, who claims what."
3. **server: implement `POST /api/workers/register`** — requireTrusted, project-binding, session token issuance, conflict on duplicate worker_id.
4. **server: implement `POST /api/workers/<id>/heartbeat`** — including `server_command` plumbing for pause/drain/terminate.
5. **server: implement `GET /api/workers/<id>/next-bead`** — long-poll, claim creation, lease expiry.
6. **server: implement `POST /api/workers/<id>/event` and `POST /api/workers/<id>/result`** — append to bead event log; reconcile claim on result.
7. **server: implement `POST /api/workers/<id>/disconnect`** — graceful release; runtime registry GC.
8. **agent/work: extract worker state machine + client** — Transport interface, HTTP transport implementation.
9. **agent/work: implement in-process Transport for `--local`** — same handlers, no listener.
10. **migrate `ddx work` and `ddx agent execute-loop` to use work.Worker** — retire `execute_bead_loop.go` body; delete the per-Run `attempted` map (subsumes `ddx-dc157075`).
11. **migrate server-spawned worker path to register-then-poll** — replace inline-execution with `exec ddx work --server-url ... --bootstrap-token ...`; retire `ExecuteLoopWorkerSpec` and the `.ddx/workers/` filesystem registry (subsumes `ddx-29058e2a`).
12. **server: claim-reclaim on heartbeat timeout** — heartbeat_timeout = 3× hb_interval; preserve attempt as a recorded dropped-event.
13. **server: restart-recovery handshake** — `410 unknown_worker` semantics; client re-registers; reconcile late results.
14. **CLI: `ddx agent doctor` reads from server runtime registry** — `.ddx/workers/` directory becomes legacy; doctor falls back to it only if the server is absent.
15. **operator-facing UI: workers panel reads from runtime registry** — file as a child of FEAT-008 if it exists at implementation time; otherwise this bead OWNS surfacing the workers panel and the FEAT reference here is provisional. Implementation MUST NOT block on FEAT-008 being created.
16. **CHANGELOG + operator migration note** — call out the restart-survival behaviour change explicitly.

Each bead should carry a structural AC referencing the relevant section
of this ADR. The roadmap is intentionally finer-grained than the C5/C7/C9
slicing in `ddx-5cb6e6cd` because each bead is independently shippable
and reviewable.

## Consequences

- **One write/execute path.** Server-spawned and `--local` collapse to one
  state machine plus two transports. Future changes (new heartbeat
  fields, new `server_command` values, new event kinds) land once.
- **Server restart preserves in-flight work.** The headline operator
  requirement.
- **Server is now a stateful runtime authority.** Today the bead store is
  the only stateful thing; tomorrow the server's runtime registry holds
  ephemeral claim/heartbeat state. Loss of the server still means loss of
  the registry; recovery is automatic on reconnect, but a server crash +
  network partition + worker death is still a recoverable-but-noisy
  scenario. ADR-007 federation does not change this — each node owns its
  registry.
- **Project boundaries enforced by token binding.** Cross-project leaks
  via worker paths become structurally impossible; the recurrence of
  `ddx-4c51d33e`-class bugs in this code path is prevented.
- **`ExecuteLoopSpec` consolidation is a side effect.** The bead-flag
  drift class (`ddx-29058e2a`) is closed by construction.
- **Long-poll requires back-pressure care.** A misconfigured worker that
  registers with no `label_filter` and a tight poll loop on bare-wait
  responses can busy-loop the server. The `wait_for_seconds` default
  (30s) and a server-side rate limit on `next-bead` per worker_id are
  required (tracked in roadmap step 5).
- **No remote workers in v1.** The trust model assumes loopback or ts-net
  identity per ADR-006. Workers running on a remote machine without a
  ts-net binding are out of scope and explicitly rejected at registration.

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
