<bead-review>
  <bead id="ddx-e2bbcbae" iter=1>
    <title>Update docs to make Fizeau transcript ownership explicit</title>
    <description>
Update DDx docs so the boundary is unambiguous: DDx owns worker/bead lifecycle progress; Fizeau owns agent transcript/progress/session rendering. DDx may link or copy Fizeau artifacts and may forward Fizeau ServiceEvents, but must not render inner Fizeau session logs or parse transcript semantics. In-scope files: docs/helix/01-frame/features/FEAT-006-agent-service.md, docs/helix/01-frame/features/FEAT-002-server.md, docs/helix/02-design/plan-2026-04-18-ddx-agent-service-interface.md if still current. Out of scope: code deletion.
    </description>
    <acceptance>
1. FEAT-006 states DDx is a pass-through/marshalling consumer for Fizeau transcript events.\n2. FEAT-002 distinguishes DDx worker lifecycle events from opaque/forwarded Fizeau agent events.\n3. Docs no longer say DDx renders Fizeau inner session logs.\n4. cd cli &amp;&amp; go test ./internal/agent -run TestAgent -count=1 passes or no code tests are needed because only docs changed.
    </acceptance>
    <labels>area:docs, area:agent, kind:cleanup</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260506T102941-82e8f98c/manifest.json</file>
    <file>.ddx/executions/20260506T102941-82e8f98c/result.json</file>
  </changed-files>

  <governing>
    <ref id="FEAT-006" path="docs/helix/01-frame/features/FEAT-006-agent-service.md" title="Feature: DDx Agent Service (consumer of ddx-agent contract)">
      <content>
<untrusted-data>
---
ddx:
  id: FEAT-006
  depends_on:
    - helix.prd
---
# Feature: DDx Agent Service (consumer of ddx-agent contract)

**ID:** FEAT-006
**Status:** In Progress (migrating to ddx-agent CONTRACT-003)
**Priority:** P0
**Owner:** DDx Team

## Overview

DDx invokes LLMs through the `ddx-agent` module, defined by
**CONTRACT-003-ddx-agent-service** in the `~/Projects/agent` repo
(`docs/helix/02-design/contracts/CONTRACT-003-ddx-agent-service.md`).

That contract is the entire execution boundary. DDx exposes the public
`ddx run` / `ddx try` / `ddx work` layers and calls the upstream
`agentlib.DdxAgent` interface for the actual agent invocation. Upstream
diagnostic/status commands may remain mounted under `ddx agent`, but
`ddx agent run`, `ddx agent execute-bead`, and `ddx agent execute-loop` are not
public workflow commands.

## DDx-side responsibilities

DDx owns the bead-driven workflow surface. It does NOT own:
- Harness orchestration (claude, codex, opencode, pi, gemini, native)
- Provider routing
- Model catalog
- Compaction
- Tool registration
- Session-log shape

Those all live inside ddx-agent per CONTRACT-003.

DDx owns:

- **Bead-driven invocation.** `ddx try`, `ddx work`, and the server's
  queue-drain worker translate bead state into `ExecuteRequest` values and
  surface results back into the bead tracker. The queue-drain worker
  defaults to a long-running 30s `--poll-interval` (ddx-dc157075):
  workers stay alive across empty polls and surface "no ready work" as a
  transient `idle` substate. Operators opt out with `--once` (one-shot)
  or `--poll-interval=0` (legacy drain-and-exit).
- **Bead-attempt orchestration.** Worktree creation, base-revision pinning,
  result landing (merge / preserve / no-changes), gate evaluation, evidence
  bundle capture. The agent provides the LLM execution; DDx provides the
  git-aware orchestration.
- **Structured prompt handoff.** Review, grading, and other prompt-bearing
  invocations use the bounded assembly invariants from FEAT-022 and the
  caller-facing structured evidence envelope described in TD-033.
  Review-mode sessions are launched in TD-033's no-tool reviewer mode so the
  agent reasons over the supplied evidence instead of discovering new context
  at runtime.
- **Evidence and session capture.** DDx writes `.ddx/executions/<id>/`
  bundles with prompts, manifests, and result artifacts. The agent's session
  log path (returned in `ExecuteResponse.SessionLogPath`) is captured into
  the bundle.
- **Power policy at the request level.** DDx selects requested agent power
  bounds, effort, and permissions per bead attempt based on bead metadata, user
  input, and prior attempt outcomes. The agent receives `MinPower` and
  optionally `MaxPower` on `ExecuteRequest` and performs the routing.
- **Agent passthrough constraints.** DDx may accept explicit `--harness`,
  `--provider`, and `--model` values from the operator and pass them unchanged
  to the agent. DDx does not validate, rank, fallback, rewrite, or reason about
  these fields; they are opaque constraints for the agent to interpret.
- **Retry policy.** DDx owns the bead retry loop because DDx owns the evidence
  needed to decide whether an attempt succeeded: commits, merge/preserve
  result, no-changes rationale, post-run gates, review verdicts, cooldowns,
  and prior run metadata. On retry, DDx may raise `MinPower`. The agent owns
  how those power bounds map to a concrete model/provider.
- **Review routing request facts.** When DDx launches the default adversarial
  pre-close reviewer gate it uses the same `Execute` boundary, requests
  stronger reviewers by raising `MinPower` relative to the implementer's actual
  power, and supplies `role=reviewer` plus correlation metadata (`bead_id`,
  `attempt_id`, `session_id`, `result_rev`, `review_group_id`, reviewer slot,
  and implementer route facts when known). These are request facts and audit
  metadata, not a DDx-side routing algorithm. See ADR-024.

Fizeau owns the agent's transcript/progress/session rendering surface. DDx is
a pass-through/marshalling consumer for Fizeau transcript events: it may
forward Fizeau `ServiceEvent`s and link or copy Fizeau artifacts into the
execution evidence bundle, but DDx treats those payloads as opaque evidence
only. DDx does not render, rehydrate, or interpret inner Fizeau session logs
or transcript semantics, and it never derives worker state from them. The
session log payload remains opaque to DDx.

## Power Intent

DDx does not route. DDx chooses request-level power bounds and sends them to
`agentlib.DdxAgent.Execute` as `MinPower` and optional `MaxPower`; the agent
resolves harness, provider, endpoint, model, health, quota, fallback, and route
errors.

Power is an abstract integer scale owned by the agent contract. DDx treats
`MinPower`/`MaxPower` as bounds on that scale, not as model identities. For
example, the agent may report:

```text
running with qwen 3.6-27b (power 10)
```

DDx records requested `MinPower`/`MaxPower` and the actual model/power returned
by the agent. DDx can use that evidence on a later retry to raise `MinPower`,
but it still does not choose the next model.

The agent also exposes its available model/power catalog. DDx may read that
catalog to choose a `MinPower` threshold for "top model only" retries, for
example by requesting a lower bound at or above the lowest power among the
current top models. DDx must not use the catalog to pin a concrete
model/provider on the normal execution path.

DDx may also pass request facts such as estimated prompt size, whether tools are
required, permissions, timeout values, and effort/reasoning intent. These facts
describe the work; they do not select a model.

## Non-Bead Invocation Intent

Not every layer-1 invocation comes from a bead. Artifact generation,
artifact regeneration, prompt comparison skills, and diagnostics can call
`ddx run` directly. For those calls DDx may choose work intent from the
artifact or operation being performed:

- `MinPower` / optional `MaxPower` derived from the artifact's generator,
  media type, size, risk, or explicit operator flags
- permissions needed for the operation, such as read-only, workspace-write, or
  network access
- timeout and effort/reasoning intent
- metadata such as `produces_artifact`, artifact id, execution definition id,
  and governing document ids
- review-mode prompt envelopes that are explicitly no-tool and assembled per
  TD-033 / FEAT-022

These are DDx-owned work facts, not routing decisions. DDx must not translate an
artifact key, media type, generator name, or execution definition into a concrete
harness/provider/model. The only concrete route-like fields DDx may send are
operator-supplied passthrough constraints, carried unchanged in the passthrough
envelope described below.

## Agent Passthrough Constraints

`--harness`, `--provider`, and `--model` are permitted on `ddx run`, `ddx try`,
and `ddx work` only as passthrough fields. DDx carries them in one narrow
request envelope and sends them to `agentlib.DdxAgent.Execute` unchanged.

These fields must not leak into DDx routing policy:

- no DDx-side validation against harness/provider/model catalogs
- no preflight `ResolveRoute`
- no provider/model fallback or substitution
- no retry-policy branches that inspect the string values
- no config-driven default harness/provider/model selection
- no worker columns or queue selection logic that treat these as DDx concepts

If an operator supplies passthrough constraints and DDx later retries with a
higher `MinPower`, DDx keeps the passthrough values unchanged. The agent
decides whether the power bounds and passthrough constraints are compatible and
reports the actual model/power or a typed error. DDx records the requested
passthrough values for audit, but it does not use them to select a route.

When hard passthrough pins make the requested power bounds unsatisfiable, DDx
must stop with a typed terminal classification such as
`blocked_by_passthrough_constraint` or `agent_power_unsatisfied`. DDx records
the requested `MinPower`/`MaxPower`, passthrough envelope, and agent-supplied
evidence, then reports operator action required. DDx must not remove pins,
choose alternatives, call `ResolveRoute` to work around the conflict, or loop on
higher `MinPower` values.

The only DDx code allowed to touch raw passthrough fields is the CLI parser, the
single passthrough/intent value, Execute request construction, execution-record
persistence, and human-facing evidence rendering. Retry, queue selection, work
scheduling, catalog threshold helpers, and config defaulting must carry the
opaque value without reading `Harness`, `Provider`, or `Model`.

`ResolveRoute` is not part of normal execution. It is allowed only for
operator-facing status/debug surfaces such as route-status and provider
diagnostics. A `RouteDecision` returned by `ResolveRoute` must never be
re-injected into `Execute`.

`agent.routing.profile_ladders`, `agent.routing.model_overrides`,
`agent.routing.profile_priority`, and the `cheap`/`standard`/`smart` profile
taxonomy are not DDx execution policy. Configs or tests that rely on DDx-managed
ladders, profiles, or model overrides are migration debt.

## Catalog and Status Boundary

Model/power catalog rendering is observability. Catalog status surfaces may show
model names, providers, powers, and route candidates returned by the agent for
debugging. Retry threshold policy may consume only abstract power numbers from
the catalog to compute `MinPower` thresholds. Neither status nor retry code may
return or mutate concrete harness/provider/model pins for execution.

## Escalation And Review Routing Boundary

ADR-024 is the governing policy for power escalation and review routing. FEAT-006
owns only the request envelope: requested `MinPower` / `MaxPower`, opaque
passthrough constraints, role/correlation metadata, and actual route facts
returned by the agent. FEAT-010 owns whether a retry or review retry is
scheduled. FEAT-014 owns normalized usage and cost signals.

DDx may compute the next `MinPower` floor from catalog power numbers, but it
must not translate that floor into a concrete model/provider or mutate
operator-supplied passthrough values. Review pairing uses the same boundary:
DDx can request stronger review and record `review-pairing-degraded` when the
actual reviewer route converges with the implementer, but concrete reviewer
selection remains inside the agent service.

## Session Log Envelope Boundary

The upstream agent owns the native session-log schema and any harness-specific
details inside it. DDx owns only the envelope around that log:

- request id, layer, parent run id, bead id or produced artifact id
- requested `MinPower` / `MaxPower` and opaque passthrough constraints
- actual model and actual power returned by the agent
- pointer or copied attachment for `ExecuteResponse.SessionLogPath`
- DDx-owned attempt outcome, merge/preserve outcome, gates, and evidence refs

DDx may link or copy the session log attachment for humans, but normal
execution policy must not render or parse the inner session log to infer
routing, provider health, model fallbacks, or retry destinations. Typed agent
response fields and DDx-owned attempt evidence are the policy inputs.

## Layer Ownership Migration Table

FEAT-006 is deliberately narrow after the three-layer refactor. It describes
how DDx consumes the upstream agent contract and how layer-1 requests are
formed; it does not own worktree, queue, or CLI namespace design.

| Area | Owner after migration |
| --- | --- |
| CONTRACT-003 `agentlib.DdxAgent` boundary, `ExecuteRequest` / `ExecuteResponse` consumption, `MinPower` / `MaxPower`, actual model/power recording | Stays in FEAT-006 |
| Opaque passthrough envelope for `--harness`, `--provider`, `--model` and the rule that DDx must not route on those fields | Stays in FEAT-006 |
| Non-bead layer-1 invocation intent: artifact-keyed power bounds, permissions, timeout, effort, and metadata | Stays in FEAT-006 |
| Session-log envelope and pointer/copy capture around the agent-owned inner log | Stays in FEAT-006 |
| Worktree creation, base revision pinning, bead prompt resolution, result landing, preserve refs, post-run gates, evidence bundles | Moves to FEAT-010 layer 2 (`ddx try`) |
| Unified `.ddx/runs/<run-id>/` substrate, layer metadata, child/parent run links, migration from `.ddx/exec-runs/` and `.ddx/executions/` | Moves to FEAT-010 |
| Queue iteration, no-progress detection, retry scheduling, cooldowns, stop conditions, loop records | Moves to FEAT-010 layer 3 (`ddx work`) |
| Public top-level command shape: `ddx run`, `ddx try`, `ddx work`, `ddx runs`, `ddx tries`, `ddx work workers` | Moves to FEAT-001 |
| Structural mounting of the upstream agent Cobra root under `ddx agent` and hard-deprecation redirects for legacy DDx workflow commands | Moves to FEAT-001, with the contract boundary referenced here |

### Conflict-recovery structured outcomes (bead ddx-0097af14)

Before parking a bead on `land_conflict` (or on `execution_failed` when the
agent produced a commit before failing), the `ddx work` drain now attempts to reuse
the preserved iteration ref rather than discarding the agent's work:

1. **3-way ort auto-merge** (`git merge --no-ff -s ort -X ours`). If clean,
   the bead closes as `success`.
2. **Focused conflict-resolve agent** (if `ConflictResolver` is configured on
   the worker). If clean, the bead closes as `success`.
3. **Park with structured outcome.** Two new statuses replace the generic
   reopen when both recovery paths fail:

| Status | Event kind | Meaning |
| --- | --- | --- |
| `land_conflict_unresolvable` | `land-conflict-unresolvable` | All auto-recovery failed; retryable after 15-min cooldown. |
| `land_conflict_needs_human` | `land-conflict-needs-human` | Focused-resolve agent returned BLOCKING; human input required. |

Both statuses use `LandConflictCooldown` (15 min) rather than the 24h cap used
for `push_failed`, because land conflicts typically unblock quickly as sibling
beads advance the base branch.

### Disrupted vs no-progress cooldown policy (bead ddx-5b3e57f4)

The execute-loop cooldown branch must distinguish *worker disruption* from
*model gave up*. A failed attempt where the model never had a chance to commit
— context cancelled, executor killed by SIGTERM/SIGKILL, transport-class
provider error, server restart, routing preflight rejection — is NOT evidence
the model could not make progress on the bead. Parking such an attempt under
the 6h `noProgressCooldown` silently freezes important work whenever a worker
is disrupted by an unrelated cause.

The loop classifies the failure via `classifyDisruption(ctx, executorErr)` and
sets `ExecuteBeadReport.Disrupted = true` plus a stable `DisruptionReason`
kind:

| Reason | Trigger |
| --- | --- |
| `context_canceled` | `ctx.Err() == context.Canceled` (SIGINT, parent died, Stop()) |
| `context_deadline` | `ctx.Err() == context.DeadlineExceeded` |
| `transport_error` | Executor error matches a transport-class marker (connection refused/reset, gateway, EOF, TLS handshake, …) |
| `preflight_rejected` | Upstream `RoutePreflight` rejected the (harness, model) pair |

When `report.Disrupted` is true:

1. `shouldSuppressNoProgress` returns false unconditionally — the loop
   bypasses `SetExecutionCooldown` so the bead is immediately re-claimable.
2. A `disruption_detected` event is appended to the bead and emitted on the
   loop event sink (`{reason, detail, harness, model}`) so operators can see
   disruption rates and which class is firing.
3. Genuine no-progress (clean executor return, BaseRev == ResultRev, not
   Disrupted) still hits the configured `noProgressCooldown` (default 6h).

The classifier is intentionally fuzzy on transport markers: a false positive
costs one quick retry; a false negative costs a 6-hour park for what was a
transient outage.

## Bead-Attempt Worker Sub-task Discovery Policy

**Design position: option (b) — surface via result.**

Workers executing inside `ddx try` / `ddx work` bead attempts MUST NOT call
`ddx bead create`
in-band during their execution run. Creating beads in-band (option c) allows a
single worker to flood the queue with unreviewed children — in one observed case
(ddx-44236615), one worker spawned 11 P0 children without operator review. This
violates the architectural principle from FEAT-013: *"DDx provides primitives,
not orchestration. Orchestration policy stays in HELIX and other workflow tools."*

Workers MAY surface discovered sub-tasks as structured data in `result.json`
under a `discovered_subtasks` array. Each entry is a lightweight object with at
minimum a `title` and optional `description`, `labels`, and `priority` fields.
The `ddx work` drain or supervisor reads this array from the result bundle and passes
it to the workflow tool (HELIX or operator) for decomposition decisions. The
supervisor decides whether, when, and at what priority to file new beads — DDx
does not do this automatically.

### Rationale

| Option | Description | Problem |
| --- | --- | --- |
| (a) Append to parent | Worker writes sub-tasks as notes or events on the parent bead | Loses structure; sub-tasks are not queryable or executable without manual decomposition |
| (b) Surface via result (**chosen**) | Agent emits `discovered_subtasks` in result.json; supervisor decides | Gates tracker mutations at an explicit decision point; aligns with "primitives not orchestration" |
| (c) Create in-band (former behavior) | Worker calls `ddx bead create` directly | Unreviewed children flood the queue; P0 tagging without operator review; violates workflow ownership boundary |

Option (b) preserves the operator control point required by workflow tools while
providing structure that automation can act on. It is the only option consistent
with the FEAT-013 boundary.

### Result schema extension

`result.json` gains an optional `discovered_subtasks` array. The bead-attempt
orchestrator writes it from the agent's structured output when present. The
supervisor MUST NOT act on this field automatically; it is surfaced as an
observation for the workflow tool.

```json
{
  "status": "success",
  "detail": "...",
  "discovered_subtasks": [
    {
      "title": "Add read-coverage for X",
      "description": "Optional detail about what is needed",
      "labels": ["area:tests"],
      "priority": "P1"
    }
  ]
}
```

If the agent does not emit `discovered_subtasks`, the field is absent. The
`ddx work` drain treats an absent or empty array identically — no automatic bead
creation occurs in either case.

### Enforcement

The bead-attempt prompt delivered to the agent (via the prompt rationalizer)
MUST include an explicit instruction that workers must not call `ddx bead create`
and must instead emit `discovered_subtasks` in their result if sub-tasks are
identified. This constraint is part of the prompt contract; it is not enforced
by the CLI at runtime.

## Malformed-Bead Remediation Decision (beads ddx-7eab13a6 era)

**Decision: close and refile.**

The layer-1 worktree-path bug fixed in ddx-7eab13a6 caused 22 beads to be
created with IDs of the form `.execute-bead-wt-<parent>-<timestamp>-<random>-<hex>`
(the worktree directory path was mistakenly used as the bead ID). These beads
are invalid: their IDs do not match the `ddx-<8hex>` format expected by all DDx
CLI commands, and the IDs appear in git commit trailers as malformed values.

Repair in-place is not safe. Bead IDs are immutable identifiers embedded in git
commit trailers (`Ddx-Attempt-Id` etc.), inter-bead dependency edges, and the
JSONL append-only store. Renaming an existing bead ID requires rewriting
history, which violates the merge policy recorded in project memory.

The operator (human or supervising workflow tool) should:
1. Inspect each malformed bead's description to determine whether the underlying
   work is still valid.
2. If valid: refile a new bead with the same title and description, using
   `ddx bead create`. The new bead will receive a correct ID.
3. Close the malformed bead with `ddx bead close <id> --reason "malformed-id
   refiled as <new-id>"`.
4. If the work is no longer needed: close the malformed bead with reason
   `malformed-id obsolete`.

No automated bulk remediation is performed. The 22 beads are left open pending
operator triage. A future `ddx bead validate` command (not yet implemented) can
flag structurally invalid bead IDs to aid discovery.

## Asking ddx-agent for changes

When DDx needs new behavior from the agent — a new method, a new field on
`ExecuteRequest`, a new event type, a new policy knob — file a PR against
CONTRACT-003 in the agent repo. Maintainers decide whether the surface grows.

Do not import agent internal packages. They live under `internal/` and the
Go compiler blocks external imports after agent v0.5.0 ships.

### Operator-prompt bead type (Story 15)

Free-form prompts submitted from the web UI are persisted as beads with
`issueType: operator-prompt` and drained by the standard execute-loop. The
title is the prompt's first line, the body is the full prompt verbatim,
default labels are `kind:operator-prompt,source:web-ui`, and the AC section
is auto-generated from a template. These beads run with the same
harness/profile/tier as regular beads but skip the structural AC check
(which assumes pre-authored AC). They cannot mutate other operator-prompt
beads — a hard rule enforced at the bead store, not at planning time.

The trust contract, audit-as-bead model, multi-node delegation policy,
prompt-injection threat model, and allowed-mutation scope are captured in
**ADR-021: Operator-Prompt Beads as the Web Write Path**.

## Worker Contract (per ADR-022)

Workers are **autonomous against the bead store**. They have one mode of
operation: read the bead store, pick the next eligible bead via the
worker-side picker, claim it atomically, execute via `try.Attempt`, write
evidence locally, and close the bead through the store. **All of this
happens regardless of whether `ddx-server` is running.** The bead store is
the source of truth; the server is an optional, value-added coordinator
that provides centralized observability — never a dependency for correct
worker operation.

ADR-022 (rev 5, commit `484b9a08`) is the governing decision. Earlier
revisions (1–3) explored a server-orchestrates direction in which
workers were long-lived API clients of the server; rev 4 reversed that,
and rev 5 finalized the autonomous-default + best-effort-mirror design
described here.

### Server probe goroutine

In parallel with the autonomous work loop, every worker runs a small
"server probe" goroutine that detects whether a server is reachable via
`~/.local/share/ddx/server.addr` and tracks a Connected ↔ NotConnected
state machine. Probe cadence: immediate first probe on startup, then a
jittered 30s steady-state interval (10s minimum, 5min maximum); five
consecutive failures back the rate off to 5min, resetting to 30s on the
next success. A 410 unknown_worker reply triggers re-registration within
the same cycle.

### Best-effort mirror

When the worker is in the Connected state it mirrors bead events and
results to the server's ingestion endpoints best-effort. Mirror failures
(timeout, connection refused, 5xx) are logged locally and the worker
continues; the bead's local event log is the authoritative copy. The
worker-server boundary collapses to a small set of endpoints, all backed
by `requireTrusted` (per ADR-006):

- `POST /api/workers/register` — emitted on every NotConnected → Connected
  transition (not only at startup). Body is a thin identity envelope
  (`project_root`, `harness`, `model`, `executor_pid`, `executor_host`,
  `started_at`); response carries a correlation `worker_id`. There is no
  session token, no claim lease, no heartbeat — none of those concepts
  exist because the worker does not depend on the server.
- `POST /api/workers/<id>/event` — best-effort mirror of any bead event
  the worker would already write to the local event log; uses the
  existing bead event `kind`/`body` shape. 204 on success.
- `POST /api/workers/<id>/backfill` — replays an in-memory ring buffer
  (cap 200 events) of events emitted while the worker was NotConnected;
  emitted on each NotConnected → Connected transition. Overflow drops
  oldest silently and the worker is marked `had_dropped_backfill: true`
  for UI display; bead-local logs remain complete.
- `POST /api/workers/<id>/disconnect` — graceful shutdown signal;
  best-effort, not required for correctness.

### Freshness states

The server's view of workers is **derived** from these reports —
eventually consistent, never authoritative. The server-side workers panel
surfaces three freshness states based on `last_event_at` relative to the
probe interval:

| State | Definition |
| --- | --- |
| `connected` | Event in the last 2× probe interval |
| `stale` | Event between 2× and 10× probe interval |
| `disconnected` | No event in > 10× probe interval (worker presumed dead) |

UI also surfaces `mirror_failures_count` and `had_dropped_backfill` so
operators can spot healthy-but-lossy workers, and labels reported worker
identity as "trusted-peer reported, not authoritative" to reflect the
ADR-006 trust model.

### Cancel via bead extra

Operator-initiated cancel is **not** a direct API call to the worker.
The server's `/api/beads/<id>/cancel` endpoint (or a direct bead-store
write) sets `extra.cancel-requested: true` on the bead. Workers honor
the marker at the next safe point:

- The worker re-reads the bead's `extra` map every 10s during long
  attempts (mid-attempt poll). On `cancel-requested: true` it aborts at
  the next LLM turn / git operation boundary and reports
  `preserved_for_review` with reason `operator_cancel`.
- A worker starting work on a bead that is already cancel-marked
  immediately reports `preserved_for_review` and skips the attempt.
- Idempotency: the worker writes `cancel-honored: true` so a re-applied
  marker does not re-trigger.
- Worst-case latency: ~10s plus the current LLM turn duration; operators
  needing faster cancel use OS signals.

### Restart and crash behavior

- **Server restart or crash.** The worker's next mirror POST fails;
  worker logs the transition and continues working. On the next
  successful probe it re-registers and resumes mirroring; the in-memory
  ring buffer feeds backfill. No in-flight work is at risk because the
  worker never depended on the server.
- **Worker exit.** No reclaim machinery is needed: the bead store's
  atomic claim is the only claim primitive. Other workers contend on
  the same store-level CAS that has worked correctly for as long as DDx
  has had a bead store.

### `--local` deprecation

Today's `--local` semantics — autonomous behavior with no server — are
the always-on default. The flag is preserved as a deprecation no-op for
one alpha release (`v0.6.2-alphaN+1` after the rev 5 work lands) and
then deleted. Operators wanting to suppress the probe simply do not run
a server.

### Compatibility writers

`.ddx/workers/` (spec.json + status.json) is preserved as a fallback
source of truth for `ddx agent doctor` for one alpha release lag, then
deprecated once the doctor migrates to the server's derived view. The
server's append-only event log lives at `.ddx/server/worker-events.jsonl`.

The attempt-orchestration responsibilities listed under "DDx-side
responsibilities" above (worktree creation, base-revision pinning,
result landing, evidence capture) all execute inside the worker process.
The server, when present, observes; it does not run agents itself, hold
authoritative claim state, gate worker startup, or assign beads to
workers. See ADR-022 for the full contract, picker rationale,
freshness/state tables, threat model, compatibility analysis, and
implementation roadmap.

## Migration status

DDx is currently migrating from the legacy DDx-internal routing/harness
implementation to the CONTRACT-003 surface. Migration epic: `ddx-ac5c7fdb`.
Pre-migration, FEAT-006 documented harness orchestration responsibilities
that have since moved upstream.

## Design References

- `docs/helix/02-design/solution-designs/SD-006-agent-session-capture.md` — session log shape and evidence bundles
- `docs/helix/02-design/solution-designs/SD-013-multi-agent-coordination.md` — worktree-aware dispatch, concurrent bead safety
- `docs/helix/02-design/solution-designs/SD-014-token-awareness.md` — provider availability, quota, and signal model
- `docs/helix/02-design/solution-designs/SD-016-process-metrics.md` — bead-level cost and rework derivation
- `docs/helix/02-design/solution-designs/SD-020-multi-machine-coordinator-topology.md` — host+user coordinator scoping
- `docs/helix/02-design/solution-designs/SD-021-service-backed-multi-node-topology.md` — service-backed node topology
- `docs/helix/02-design/technical-designs/TD-006-agent-session-capture.md` — session log and artifact format
- `docs/helix/02-design/technical-designs/TD-010-executions.md` — execution attempt bundles at `.ddx/executions/`
- `docs/helix/02-design/plan-2026-04-18-ddx-agent-service-interface.md` — current thin-consumer migration plan
- `docs/helix/03-test/test-plans/TP-006-agent-session-capture.md` — session capture test coverage
- `docs/helix/03-test/test-plans/TP-014-token-awareness.md` — token-awareness coverage
- `docs/helix/03-test/test-plans/TP-020-agent-routing-and-catalog-resolution.md` — routing and catalog resolution coverage
- `docs/helix/02-design/adr/ADR-021-operator-prompt-beads-web-write-path.md` — operator-prompt beads as the web write path (Story 15)
- `docs/helix/02-design/adr/ADR-022-worker-client-server-architecture.md` — workers as long-lived API clients; server-restart preserves in-flight work
- `docs/helix/02-design/adr/ADR-024-power-escalation-and-review-routing.md` — DDx power escalation, review routing, and cost-cap policy boundary
</untrusted-data>
      </content>
    </ref>
  </governing>

  <diff rev="eeed360de24484cf8445dbc8fc98ac14042bd97b">
<untrusted-data>
diff --git a/.ddx/executions/20260506T102941-82e8f98c/manifest.json b/.ddx/executions/20260506T102941-82e8f98c/manifest.json
new file mode 100644
index 000000000..a60976171
--- /dev/null
+++ b/.ddx/executions/20260506T102941-82e8f98c/manifest.json
@@ -0,0 +1,415 @@
+{
+  "attempt_id": "20260506T102941-82e8f98c",
+  "bead_id": "ddx-e2bbcbae",
+  "base_rev": "488c43080e8906d686af97dedfc850ebc6a6857f",
+  "created_at": "2026-05-06T10:29:44.282564602Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-e2bbcbae",
+    "title": "Update docs to make Fizeau transcript ownership explicit",
+    "description": "Update DDx docs so the boundary is unambiguous: DDx owns worker/bead lifecycle progress; Fizeau owns agent transcript/progress/session rendering. DDx may link or copy Fizeau artifacts and may forward Fizeau ServiceEvents, but must not render inner Fizeau session logs or parse transcript semantics. In-scope files: docs/helix/01-frame/features/FEAT-006-agent-service.md, docs/helix/01-frame/features/FEAT-002-server.md, docs/helix/02-design/plan-2026-04-18-ddx-agent-service-interface.md if still current. Out of scope: code deletion.",
+    "acceptance": "1. FEAT-006 states DDx is a pass-through/marshalling consumer for Fizeau transcript events.\\n2. FEAT-002 distinguishes DDx worker lifecycle events from opaque/forwarded Fizeau agent events.\\n3. Docs no longer say DDx renders Fizeau inner session logs.\\n4. cd cli \u0026\u0026 go test ./internal/agent -run TestAgent -count=1 passes or no code tests are needed because only docs changed.",
+    "parent": "ddx-dda48755",
+    "labels": [
+      "area:docs",
+      "area:agent",
+      "kind:cleanup"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-06T10:29:41Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "601864",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-05T16:10:25.809653005Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260505T160845-2be3894c\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":748901,\"output_tokens\":7443,\"total_tokens\":756344,\"cost_usd\":0,\"duration_ms\":97398,\"exit_code\":0}",
+          "created_at": "2026-05-05T16:10:26.043816179Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=756344 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-05T16:10:35.241860677Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=a7e7f7210330acedb02ffd0649ceaae129de171d\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-05T12:15:40-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=35459\noutput_bytes=0\nelapsed_ms=4175",
+          "created_at": "2026-05-05T16:10:40.645930181Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=a7e7f7210330acedb02ffd0649ceaae129de171d\nbase_rev=206fb5640c5b6cfbe54ed3f863cf12b57d2c89c4",
+          "created_at": "2026-05-05T16:10:40.856092351Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-05T18:01:21.909638862Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260505T180009-0e200319\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":688252,\"output_tokens\":7009,\"total_tokens\":695261,\"cost_usd\":0,\"duration_ms\":70156,\"exit_code\":0}",
+          "created_at": "2026-05-05T18:01:22.130341134Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=695261 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-05T18:01:31.088788771Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=029f4585c98c049ee0dddb0b88baca4071c5bbf3\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-05T14:06:36-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=37928\noutput_bytes=0\nelapsed_ms=4123",
+          "created_at": "2026-05-05T18:01:36.557763212Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=029f4585c98c049ee0dddb0b88baca4071c5bbf3\nbase_rev=e0efaa4f577ff60483f076463a36c0442e2ce621",
+          "created_at": "2026-05-05T18:01:36.789848301Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-05T23:43:52.246843889Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260505T234254-a38b6e20\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":507273,\"output_tokens\":5383,\"total_tokens\":512656,\"cost_usd\":0,\"duration_ms\":55275,\"exit_code\":0}",
+          "created_at": "2026-05-05T23:43:52.469906088Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=512656 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-05T23:43:58.081233427Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=51c528e784baaec6adfa6ac1645d7727dd74236b\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-05T19:49:03-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=40293\noutput_bytes=0\nelapsed_ms=4115",
+          "created_at": "2026-05-05T23:44:03.78068733Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=51c528e784baaec6adfa6ac1645d7727dd74236b\nbase_rev=d4f7b1daa9728dc29f3f2c6935714f25f4f0c6d6",
+          "created_at": "2026-05-05T23:44:04.07523782Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        },
+        {
+          "actor": "erik",
+          "body": "creating isolated worktree: git worktree add: Preparing worktree (detached HEAD 9c9ea2f9)\nerror: unable to create file cli/internal/server/state_runs_detail_test.go: No space left on device\nerror: unable to create file cli/internal/server/state_runs_test.go: No space left on device\nerror: unable to create file cli/internal/server/state_test.go: No space left on device\nerror: unable to create file cli/internal/server/test_isolation_lint_test.go: No space left on device\nerror: unable to create file cli/internal/server/worker_ingest.go: No space left on device\nerror: unable to create file cli/internal/server/worker_ingest_test.go: No space left on device\nerror: unable to create file cli/internal/server/workers.go: No space left on device\nerror: unable to create file cli/internal/server/workers_config_test.go: No space left on device\nerror: unable to create file cli/internal/server/workers_costcap_test.go: No space left on device\nerror: unable to create file cli/internal/server/workers_e2e_test.go: No space left on device\nerror: unable to create file cli/internal/server/workers_escalation_test.go: No space left on device\nerror: unable to create file cli/internal/server/workers_gates_test.go: No space left on device\nerror: unable to create file cli/internal/server/workers_panel_live_transitions_test.go: No space left on device\nerror: unable to create file cli/internal/server/workers_prune_test.go: No space left on device\nerror: unable to create file cli/internal/server/workers_reap_unix.go: No space left on device\nerror: unable to create file cli/internal/server/workers_reap_windows.go: No space left on device\nerror: unable to create file cli/internal/server/workers_resolve_route_test.go: No space left on device\nerror: unable to create file cli/internal/server/workers_stop_propagation_test.go: No space left on device\nerror: unable to create file cli/internal/server/workers_stop_test.go: No space left on device\nerror: unable to create file cli/internal/server/workers_test.go: No space left on device\nerror: unable to create file cli/internal/server/workers_wake_test.go: No space left on device\nerror: unable to create file cli/internal/server/workers_watchdog_test.go: No space left on device\nfatal: cannot create directory at 'cli/internal/serverreg': No space left on device: exit status 128",
+          "created_at": "2026-05-06T00:53:38.632266001Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-06T02:39:39.32264604Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260506T023850-0405f255\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":378529,\"output_tokens\":3960,\"total_tokens\":382489,\"cost_usd\":0,\"duration_ms\":46940,\"exit_code\":0}",
+          "created_at": "2026-05-06T02:39:39.552673196Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=382489 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-06T02:39:45.207004505Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=00d1a7ad832bf2e2f85cd0fe9980ee4938ebbffe\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-05T22:44:51-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=46672\noutput_bytes=0\nelapsed_ms=4121",
+          "created_at": "2026-05-06T02:39:51.308813182Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=00d1a7ad832bf2e2f85cd0fe9980ee4938ebbffe\nbase_rev=730f3efe908a65147977031edf27ad5931697121",
+          "created_at": "2026-05-06T02:39:51.529158641Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-06T05:05:13.587206853Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260506T050355-679800c1\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":755455,\"output_tokens\":7443,\"total_tokens\":762898,\"cost_usd\":0,\"duration_ms\":75515,\"exit_code\":0}",
+          "created_at": "2026-05-06T05:05:13.813272064Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=762898 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-06T05:05:22.187384566Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=702a9da343b436827a88bfabe84124a097c3b4dc\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-06T01:10:27-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=49015\noutput_bytes=0\nelapsed_ms=4083",
+          "created_at": "2026-05-06T05:05:28.005890538Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=702a9da343b436827a88bfabe84124a097c3b4dc\nbase_rev=d4d05e9c6d27b07c1880f0e550681a8cd4161522",
+          "created_at": "2026-05-06T05:05:28.207000387Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-06T06:45:19.117985804Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260506T064358-be40c5a1\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":934048,\"output_tokens\":6233,\"total_tokens\":940281,\"cost_usd\":0,\"duration_ms\":78141,\"exit_code\":0}",
+          "created_at": "2026-05-06T06:45:19.31459039Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=940281 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-06T06:45:25.619933353Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: unparseable\nattempt_count=1\nresult_rev=41741f04c93a6dd0f6de5af1f89acae3ed894e99\n\nreviewer: review-error: unparseable: reviewer output: unparseable JSON verdict: no JSON object found (raw output 59 bytes; see .ddx/executions/20260506T064526-1847fd83)\nharness=claude\nmodel=opus\ninput_bytes=51359\noutput_bytes=59\nelapsed_ms=48793",
+          "created_at": "2026-05-06T06:46:17.603183286Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: unparseable"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=41741f04c93a6dd0f6de5af1f89acae3ed894e99\nbase_rev=946cee569df1274be311e19a388cb3e2fc8269fd",
+          "created_at": "2026-05-06T06:46:17.807285825Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-06T07:54:41.495085832Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260506T075345-9662b3fa\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":457167,\"output_tokens\":5158,\"total_tokens\":462325,\"cost_usd\":0,\"duration_ms\":53438,\"exit_code\":0}",
+          "created_at": "2026-05-06T07:54:41.720704627Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=462325 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-06T07:54:48.60496745Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=44396c8eb23a7435bc342b64a954ced652947b56\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-06T03:59:53-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=53698\noutput_bytes=0\nelapsed_ms=4118",
+          "created_at": "2026-05-06T07:54:54.082301829Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=44396c8eb23a7435bc342b64a954ced652947b56\nbase_rev=9101744e5829dcf08a1e259ee82f988daddd23dd",
+          "created_at": "2026-05-06T07:54:54.271887026Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-06T08:45:07.838152072Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260506T084346-4c8fdf1b\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":514720,\"output_tokens\":8153,\"total_tokens\":522873,\"cost_usd\":0,\"duration_ms\":78638,\"exit_code\":0}",
+          "created_at": "2026-05-06T08:45:08.065072794Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=522873 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-06T08:45:14.495983569Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: unparseable\nattempt_count=1\nresult_rev=5cb464202c7c04df9e3e7a538314fb931f499cc7\n\nreviewer: review-error: unparseable: reviewer output: unparseable JSON verdict: no JSON object found (raw output 59 bytes; see .ddx/executions/20260506T084515-ec2936e2)\nharness=claude\nmodel=opus\ninput_bytes=56047\noutput_bytes=59\nelapsed_ms=48532",
+          "created_at": "2026-05-06T08:46:06.173060988Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: unparseable"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=5cb464202c7c04df9e3e7a538314fb931f499cc7\nbase_rev=414771c8bb1ddb5e08edf94388b1d3bd34fb628a",
+          "created_at": "2026-05-06T08:46:06.388983967Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-06T09:47:01.494734921Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260506T094552-1da8c578\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":628310,\"output_tokens\":6163,\"total_tokens\":634473,\"cost_usd\":0,\"duration_ms\":66091,\"exit_code\":0}",
+          "created_at": "2026-05-06T09:47:01.890501499Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=634473 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"gpt-5.4-mini\",\"resolved_provider\":\"codex\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-06T09:47:09.153278649Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: transport\nattempt_count=1\nresult_rev=db88f469214f78c0eb2ecb83fe2adc59e6bd38ce\n\nreviewer: review-error: transport: agent: execute: no viable provider right now: claude quota-exhausted (retry after 2026-05-06T05:52:14-04:00)\nharness=claude\nmodel=claude-opus-4-6\ninput_bytes=58368\noutput_bytes=0\nelapsed_ms=4119",
+          "created_at": "2026-05-06T09:47:14.86054004Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: transport"
+        },
+        {
+          "actor": "erik",
+          "body": "success\nresult_rev=db88f469214f78c0eb2ecb83fe2adc59e6bd38ce\nbase_rev=e489ced6305ac3a1ecdc0dd6131af4f631726803",
+          "created_at": "2026-05-06T09:47:15.068396797Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "success"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-06T10:29:41.89870181Z",
+      "spec-id": "FEAT-006"
+    }
+  },
+  "governing": [
+    {
+      "id": "FEAT-006",
+      "path": "docs/helix/01-frame/features/FEAT-006-agent-service.md",
+      "title": "Feature: DDx Agent Service (consumer of ddx-agent contract)"
+    }
+  ],
+  "paths": {
+    "dir": ".ddx/executions/20260506T102941-82e8f98c",
+    "prompt": ".ddx/executions/20260506T102941-82e8f98c/prompt.md",
+    "manifest": ".ddx/executions/20260506T102941-82e8f98c/manifest.json",
+    "result": ".ddx/executions/20260506T102941-82e8f98c/result.json",
+    "checks": ".ddx/executions/20260506T102941-82e8f98c/checks.json",
+    "usage": ".ddx/executions/20260506T102941-82e8f98c/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-e2bbcbae-20260506T102941-82e8f98c"
+  },
+  "prompt_sha": "d20532b190572a9bb7f138a408f7a60b10d2a9d9361d61db84f5d5ef926ec09e"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260506T102941-82e8f98c/result.json b/.ddx/executions/20260506T102941-82e8f98c/result.json
new file mode 100644
index 000000000..66701d303
--- /dev/null
+++ b/.ddx/executions/20260506T102941-82e8f98c/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-e2bbcbae",
+  "attempt_id": "20260506T102941-82e8f98c",
+  "base_rev": "488c43080e8906d686af97dedfc850ebc6a6857f",
+  "result_rev": "deb8a95cdfd8d9f7460b9f110352962cdfa06e09",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-4633a44f",
+  "duration_ms": 66665,
+  "tokens": 565901,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260506T102941-82e8f98c",
+  "prompt_file": ".ddx/executions/20260506T102941-82e8f98c/prompt.md",
+  "manifest_file": ".ddx/executions/20260506T102941-82e8f98c/manifest.json",
+  "result_file": ".ddx/executions/20260506T102941-82e8f98c/result.json",
+  "usage_file": ".ddx/executions/20260506T102941-82e8f98c/usage.json",
+  "started_at": "2026-05-06T10:29:44.283072227Z",
+  "finished_at": "2026-05-06T10:30:50.948208122Z"
+}
\ No newline at end of file
</untrusted-data>
  </diff>

  <instructions>
You are reviewing a bead implementation against its acceptance criteria.

For each acceptance-criteria (AC) item, decide whether it is implemented correctly, then assign one overall verdict:

- APPROVE — every AC item is fully and correctly implemented.
- REQUEST_CHANGES — some AC items are partial or have fixable minor issues.
- BLOCK — at least one AC item is not implemented or incorrectly implemented; or the diff is insufficient to evaluate.

## Required output format (schema_version: 1)

Respond with EXACTLY one JSON object as your final response, fenced as a single ```json … ``` code block. Do not include any prose outside the fenced block. The JSON must match this schema:

```json
{
  "schema_version": 1,
  "verdict": "APPROVE",
  "summary": "≤300 char human-readable verdict justification",
  "findings": [
    { "severity": "info", "summary": "what is wrong or notable", "location": "path/to/file.go:42" }
  ]
}
```

Rules:
- "verdict" must be exactly one of "APPROVE", "REQUEST_CHANGES", "BLOCK".
- "severity" must be exactly one of "info", "warn", "block".
- Output the JSON object inside ONE fenced ```json … ``` block. No additional prose, no extra fences, no markdown headings.
- Do not echo this template back. Do not write the words APPROVE, REQUEST_CHANGES, or BLOCK anywhere except as the JSON value of the verdict field.
  </instructions>
</bead-review>
