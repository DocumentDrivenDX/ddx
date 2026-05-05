<bead-review>
  <bead id="ddx-9b36485a" iter=1>
    <title>bead: add real execute-worktree CLI regression for canonical child IDs</title>
    <description>
PROBLEM
The queue contained multiple generated child beads whose IDs started with .execute-bead-wt-..., for example .execute-bead-wt-ddx-8d747049-20260505T023636-7669921a-b6bc9398 and .execute-bead-wt-ddx-c8f79963-20260505T104148-6e54f8ef-8083652e. Those IDs are not valid execute-bead git ref path components and one attempt already failed while preserving refs for such an ID.

ROOT CAUSE
- cli/internal/bead/worktree_prefix_test.go:13-27 and cli/cmd/bead_workspace_test.go:42-84 cover in-process store/command factory paths, but the observed failure came from autonomous decomposition running ddx bead create inside an actual execute-bead linked worktree.
- cli/internal/bead/store.go:1818-1841 derives prefixes from the store working directory and git root, but the real installed-binary/worktree path is not covered by an end-to-end test that creates a linked worktree and runs ddx bead create as a subprocess.

PROPOSED FIX
Add a real integration regression test that builds or invokes the current ddx test binary from inside an execute-bead-shaped linked worktree with DDX_BEAD_DIR=.ddx, creates a child bead, and verifies the child ID and storage location use the origin workspace naming convention. If the test fails, fix the command/store workspace resolution path so child beads are created as ddx-* IDs in the primary workspace store.

NON-SCOPE
- Cleaning historical malformed tracker entries; the queue-quality review already superseded the open queued copies.
- Changing the execute-bead worktree naming convention itself.
    </description>
    <acceptance>
1. TestBeadCreate_ExecuteWorktreeRealBinaryUsesOriginPrefix or equivalent creates a real git repo, commits .ddx/config.yaml and .ddx/beads.jsonl, creates an execute-bead-shaped linked worktree, runs ddx bead create as a subprocess from that worktree with DDX_BEAD_DIR=.ddx, and asserts the new ID matches ^ddx-[0-9a-f]{8}$ or the origin config prefix.
2. The same test asserts the bead row is written to the primary workspace .ddx/beads.jsonl, not the isolated worktree copy.
3. The regression test fails if the ID begins with .execute-bead-wt-.
4. If current code fails the test, fix workspace/store prefix resolution so the test passes for real subprocess execution.
5. cd cli &amp;&amp; go test ./cmd ./internal/bead -run 'TestBeadCreate_ExecuteWorktreeRealBinaryUsesOriginPrefix|TestBeadCreate_RelativeEnvInsideLinkedWorktreeUsesPrimaryNamingAndStore|TestGenID_WorktreePrefix' -count=1 passes.
6. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, area:beads, area:agent, kind:fix, observed-failure, bead-quality, triage:needs-investigation</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260505T184548-9ec8fec6/checks/production-reachability.json</file>
    <file>.ddx/executions/20260505T184548-9ec8fec6/manifest.json</file>
    <file>.ddx/executions/20260505T184548-9ec8fec6/result.json</file>
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

Fizeau owns the agent's transcript/progress/session rendering surface. DDx is
a pass-through/marshalling consumer for Fizeau transcript events: it may
forward Fizeau `ServiceEvent`s and link or copy Fizeau artifacts into the
execution evidence bundle, but DDx does not interpret transcript semantics or
inspect the Fizeau session log body.

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

## Session Log Envelope Boundary

The upstream agent owns the native session-log schema and any harness-specific
details inside it. DDx owns only the envelope around that log:

- request id, layer, parent run id, bead id or produced artifact id
- requested `MinPower` / `MaxPower` and opaque passthrough constraints
- actual model and actual power returned by the agent
- pointer or copied attachment for `ExecuteResponse.SessionLogPath`
- DDx-owned attempt outcome, merge/preserve outcome, gates, and evidence refs

DDx may link or copy the session log attachment for humans, but normal
execution policy must not parse the inner session log to infer routing,
provider health, model fallbacks, or retry destinations. Typed agent response
fields and DDx-owned attempt evidence are the policy inputs.

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
</untrusted-data>
      </content>
    </ref>
  </governing>

  <diff rev="a0498bfa10b24367f510db3839b9e558db3f7d99">
<untrusted-data>
diff --git a/.ddx/executions/20260505T184548-9ec8fec6/checks/production-reachability.json b/.ddx/executions/20260505T184548-9ec8fec6/checks/production-reachability.json
new file mode 100644
index 00000000..89e73251
--- /dev/null
+++ b/.ddx/executions/20260505T184548-9ec8fec6/checks/production-reachability.json
@@ -0,0 +1,4 @@
+{
+  "status": "pass",
+  "message": "no non-test Go files changed"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T184548-9ec8fec6/manifest.json b/.ddx/executions/20260505T184548-9ec8fec6/manifest.json
new file mode 100644
index 00000000..21ff31bc
--- /dev/null
+++ b/.ddx/executions/20260505T184548-9ec8fec6/manifest.json
@@ -0,0 +1,86 @@
+{
+  "attempt_id": "20260505T184548-9ec8fec6",
+  "bead_id": "ddx-9b36485a",
+  "base_rev": "23d4479848abf9ccc521fed73b16726ddc8fc08e",
+  "created_at": "2026-05-05T18:45:51.339963924Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-9b36485a",
+    "title": "bead: add real execute-worktree CLI regression for canonical child IDs",
+    "description": "PROBLEM\nThe queue contained multiple generated child beads whose IDs started with .execute-bead-wt-..., for example .execute-bead-wt-ddx-8d747049-20260505T023636-7669921a-b6bc9398 and .execute-bead-wt-ddx-c8f79963-20260505T104148-6e54f8ef-8083652e. Those IDs are not valid execute-bead git ref path components and one attempt already failed while preserving refs for such an ID.\n\nROOT CAUSE\n- cli/internal/bead/worktree_prefix_test.go:13-27 and cli/cmd/bead_workspace_test.go:42-84 cover in-process store/command factory paths, but the observed failure came from autonomous decomposition running ddx bead create inside an actual execute-bead linked worktree.\n- cli/internal/bead/store.go:1818-1841 derives prefixes from the store working directory and git root, but the real installed-binary/worktree path is not covered by an end-to-end test that creates a linked worktree and runs ddx bead create as a subprocess.\n\nPROPOSED FIX\nAdd a real integration regression test that builds or invokes the current ddx test binary from inside an execute-bead-shaped linked worktree with DDX_BEAD_DIR=.ddx, creates a child bead, and verifies the child ID and storage location use the origin workspace naming convention. If the test fails, fix the command/store workspace resolution path so child beads are created as ddx-* IDs in the primary workspace store.\n\nNON-SCOPE\n- Cleaning historical malformed tracker entries; the queue-quality review already superseded the open queued copies.\n- Changing the execute-bead worktree naming convention itself.",
+    "acceptance": "1. TestBeadCreate_ExecuteWorktreeRealBinaryUsesOriginPrefix or equivalent creates a real git repo, commits .ddx/config.yaml and .ddx/beads.jsonl, creates an execute-bead-shaped linked worktree, runs ddx bead create as a subprocess from that worktree with DDX_BEAD_DIR=.ddx, and asserts the new ID matches ^ddx-[0-9a-f]{8}$ or the origin config prefix.\n2. The same test asserts the bead row is written to the primary workspace .ddx/beads.jsonl, not the isolated worktree copy.\n3. The regression test fails if the ID begins with .execute-bead-wt-.\n4. If current code fails the test, fix workspace/store prefix resolution so the test passes for real subprocess execution.\n5. cd cli \u0026\u0026 go test ./cmd ./internal/bead -run 'TestBeadCreate_ExecuteWorktreeRealBinaryUsesOriginPrefix|TestBeadCreate_RelativeEnvInsideLinkedWorktreeUsesPrimaryNamingAndStore|TestGenID_WorktreePrefix' -count=1 passes.\n6. lefthook run pre-commit passes.",
+    "parent": "ddx-a9d130d0",
+    "labels": [
+      "phase:2",
+      "area:beads",
+      "area:agent",
+      "kind:fix",
+      "observed-failure",
+      "bead-quality",
+      "triage:needs-investigation"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-05T18:45:48Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "3918937",
+      "events": [
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"codex\",\"resolved_model\":\"gpt-5.4-mini\",\"fallback_chain\":[],\"actual_power\":8}",
+          "created_at": "2026-05-05T17:15:45.369950657Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=codex model=gpt-5.4-mini"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260505T171206-e055ef01\",\"harness\":\"codex\",\"model\":\"gpt-5.4-mini\",\"input_tokens\":2778042,\"output_tokens\":12892,\"total_tokens\":2790934,\"cost_usd\":0,\"duration_ms\":213777,\"exit_code\":0}",
+          "created_at": "2026-05-05T17:15:45.602357744Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=2790934 model=gpt-5.4-mini"
+        },
+        {
+          "actor": "erik",
+          "body": "The requested regression test and subprocess helper were added, but the required repository gate cannot be satisfied in this attempt because `lefthook run pre-commit` fails on an unrelated existing cmd test: `TestAgentRunProfileNoViableHarness` in `cli/cmd/agent_run_profile_test.go:206-215` reproduces with `CGO_ENABLED=1 go test ./cmd -run 'TestAgentRunProfileNoViableHarness' -count=1` and fails with \"An error is expected but got nil.\" This is outside the bead's worktree-prefix scope and would need a separate investigation/fix bead before a compliant commit can land.",
+          "created_at": "2026-05-05T17:15:46.321485339Z",
+          "kind": "no_changes_needs_investigation",
+          "source": "ddx agent execute-loop",
+          "summary": "no_changes_needs_investigation"
+        },
+        {
+          "actor": "erik",
+          "body": "no_changes\nrationale: status: needs_investigation\nreason: The requested regression test and subprocess helper were added, but the required repository gate cannot be satisfied in this attempt because `lefthook run pre-commit` fails on an unrelated existing cmd test: `TestAgentRunProfileNoViableHarness` in `cli/cmd/agent_run_profile_test.go:206-215` reproduces with `CGO_ENABLED=1 go test ./cmd -run 'TestAgentRunProfileNoViableHarness' -count=1` and fails with \"An error is expected but got nil.\" This is outside the bead's worktree-prefix scope and would need a separate investigation/fix bead before a compliant commit can land.\nresult_rev=ad1bac78a8db4ca02629588aaefad8ff45cd6151\nbase_rev=ad1bac78a8db4ca02629588aaefad8ff45cd6151\nretry_after=2026-05-05T23:15:46Z",
+          "created_at": "2026-05-05T17:15:46.955743515Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "no_changes"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-05T18:45:48.655740029Z",
+      "execute-loop-last-detail": "no_changes",
+      "execute-loop-last-status": "no_changes",
+      "execute-loop-no-changes-count": 1,
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
+    "dir": ".ddx/executions/20260505T184548-9ec8fec6",
+    "prompt": ".ddx/executions/20260505T184548-9ec8fec6/prompt.md",
+    "manifest": ".ddx/executions/20260505T184548-9ec8fec6/manifest.json",
+    "result": ".ddx/executions/20260505T184548-9ec8fec6/result.json",
+    "checks": ".ddx/executions/20260505T184548-9ec8fec6/checks.json",
+    "usage": ".ddx/executions/20260505T184548-9ec8fec6/usage.json",
+    "worktree": "tmp/ddx-exec-wt/.execute-bead-wt-ddx-9b36485a-20260505T184548-9ec8fec6"
+  },
+  "prompt_sha": "29d894e1f592022ca957bf375a0e0a0746f930f55a4c229c3fd7958f0325741a"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260505T184548-9ec8fec6/result.json b/.ddx/executions/20260505T184548-9ec8fec6/result.json
new file mode 100644
index 00000000..7d1d0b44
--- /dev/null
+++ b/.ddx/executions/20260505T184548-9ec8fec6/result.json
@@ -0,0 +1,25 @@
+{
+  "bead_id": "ddx-9b36485a",
+  "attempt_id": "20260505T184548-9ec8fec6",
+  "base_rev": "23d4479848abf9ccc521fed73b16726ddc8fc08e",
+  "result_rev": "92f841360621f34fad9e58d72634b59fc77a883a",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "codex",
+  "model": "gpt-5.4-mini",
+  "actual_power": 8,
+  "predicted_power": 8,
+  "predicted_cost_source": "subscription",
+  "session_id": "eb-795df6e2",
+  "duration_ms": 243688,
+  "tokens": 3042476,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260505T184548-9ec8fec6",
+  "prompt_file": ".ddx/executions/20260505T184548-9ec8fec6/prompt.md",
+  "manifest_file": ".ddx/executions/20260505T184548-9ec8fec6/manifest.json",
+  "result_file": ".ddx/executions/20260505T184548-9ec8fec6/result.json",
+  "usage_file": ".ddx/executions/20260505T184548-9ec8fec6/usage.json",
+  "started_at": "2026-05-05T18:45:51.340502132Z",
+  "finished_at": "2026-05-05T18:49:55.028960508Z"
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
