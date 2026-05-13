---
ddx:
  id: FEAT-006
  depends_on:
    - helix.prd
---
# Feature: DDx Consumer of the Fizeau Execution Contract

**ID:** FEAT-006
**Status:** In Progress (migrating to CONTRACT-003 Fizeau execution contract)
**Priority:** P0
**Owner:** DDx Team

## Overview

DDx invokes LLMs through the upstream Fizeau execution contract,
**CONTRACT-003**, in the `~/Projects/agent` repo
(`docs/helix/02-design/contracts/CONTRACT-003-fizeau-service.md`).

That contract is the entire execution boundary. DDx exposes the public
`ddx run` / `ddx try` / `ddx work` layers, forwards raw passthrough constraints
unchanged, and sends requests to Fizeau for the actual invocation. Upstream
diagnostic/status commands may remain as Fizeau-owned observability, but the
retired workflow namespace is not a public workflow surface and has no legacy
alias.

## DDx-side responsibilities

DDx owns the bead-driven workflow surface. It does NOT own:
- Harness orchestration (claude, codex, opencode, pi, gemini, native)
- Provider routing
- Model catalog
- Compaction
- Tool registration
- Session-log shape

Those all live inside Fizeau per CONTRACT-003.

Historical migration notes below preserve older ddx-agent naming and draft
shapes for traceability only. They are not the desired current contract names
or preferred DDx terminology.

DDx owns:

- **Bead-driven invocation.** `ddx try`, `ddx work`, and the server's
  queue-drain worker translate bead state into `ExecuteRequest` values and
  surface results back into the bead tracker. The queue-drain worker
  uses explicit `once`, `drain`, and `watch` modes: direct CLI `ddx work`
  drains and exits by default, while server-managed workers default to
  `watch` with a 30s `idle_interval`. Watch workers stay alive across
  empty polls and surface "no ready work" as a transient `idle` substate.
  Operators choose one-shot CLI work with `--once`.
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
- **Power policy at the request level.** DDx selects requested power bounds,
  effort, and permissions per bead attempt based on bead metadata, user input,
  and prior attempt outcomes. DDx sends `MinPower` and optionally `MaxPower`
  to Fizeau and leaves the concrete route to Fizeau.
- **Agent passthrough constraints.** DDx may accept explicit `--harness`,
  `--provider`, and `--model` values from the operator and pass them unchanged
  to Fizeau. DDx does not validate, rank, fallback, rewrite, or reason about
  these fields; they are opaque constraints for Fizeau to interpret.
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

DDx owns bead/worker lifecycle progress and execution evidence capture.
Fizeau owns transcript/progress/session rendering, provider/model discovery,
and session presentation. DDx is a pass-through/marshalling consumer of
opaque Fizeau transcript events: it may forward Fizeau `ServiceEvent`s
unchanged and link or copy Fizeau artifacts into the execution evidence
bundle, but DDx tracks those payloads only as opaque evidence. DDx keeps the
inner Fizeau transcript or session-log content opaque; it does not rehydrate,
parse, render, or semantically interpret it, and those payloads never become
DDx worker state. The session log payload remains opaque to DDx, which only
carries the envelope around it.

## Power Intent

DDx does not choose concrete routes. DDx chooses request-level intent and sends
it to Fizeau as a profile/policy name when one can be selected from Fizeau
metadata, plus optional `MinPower` / `MaxPower` bounds. Fizeau resolves harness,
provider, endpoint, model, health, quota, fallback, and route errors.

Power is an abstract integer scale owned by the Fizeau contract. DDx treats
`MinPower`/`MaxPower` as bounds on that scale, not as model identities. For
example, Fizeau may report:

```text
running with qwen 3.6-27b (power 10)
```

DDx records requested profile/policy, requested `MinPower`/`MaxPower`, and the
actual model/power returned by Fizeau. DDx can use that evidence on a later
retry to request a stronger profile or raise `MinPower`, but it still does not
choose the next model.

Fizeau exposes available profiles/policies and model metadata. DDx may read
that metadata to select a request profile or, if profiles are unavailable, a
`MinPower` threshold. Profile names are Fizeau configuration, not DDx constants:
names such as cheap/standard/smart are shortcuts when a Fizeau installation
chooses to expose them, not canonical strings DDx should hard-code.

Default selection is weak-first and progress-biased:

- choose the cheapest and fastest available profile that DDx reasonably expects
  can complete the task;
- reserve the strongest profiles for breakdown, debugging, high-risk review,
  architecture-sensitive work, or retries with concrete lower-tier failure
  evidence;
- if no profile satisfies the ideal power band but a free/available provider can
  attempt the work, try it and record the route as degraded rather than blocking
  the bead before execution;
- fail with an execution/provider-availability error only when no provider is
  available to attempt the work.

DDx must not use profile or model metadata to pin a concrete model/provider on
the normal execution path.

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
request envelope and sends them to Fizeau unchanged. Fizeau interprets those
raw strings.

For example:

```bash
ddx run --min-power 10 --model qwen36 --prompt task.md
```

DDx forwards `qwen36` verbatim. Fizeau owns any alias or constraint matching,
provider fallback, or typed error returned for that raw string.
DDx passes the raw string unchanged.

These fields must not leak into DDx routing policy:

- no DDx-side validation against harness/provider/model catalogs
- no preflight route-selection helper
- no provider/model fallback or substitution
- no retry-policy branches that inspect the string values
- no config-driven default harness/provider/model selection
- no worker columns or queue selection logic that treat these as DDx concepts

If an operator supplies passthrough constraints and DDx later retries with a
higher `MinPower`, DDx keeps the passthrough values unchanged. Fizeau decides
whether the power bounds and passthrough constraints are compatible and reports
the actual model/power or a typed error. DDx records the requested
passthrough values for audit, but it does not use them to select a route.

When hard passthrough pins make the requested power bounds unsatisfiable, DDx
must stop with a typed terminal classification such as
`blocked_by_passthrough_constraint` or `agent_power_unsatisfied`. DDx records
the requested `MinPower`/`MaxPower`, passthrough envelope, and Fizeau-supplied
evidence, then reports operator action required. DDx must not remove pins,
choose alternatives, call any route-selection helper to work around the
conflict, or loop on higher `MinPower` values.

The only DDx code allowed to touch raw passthrough fields is the CLI parser, the
single passthrough/intent value, Execute request construction, execution-record
persistence, and human-facing evidence rendering. Retry, queue selection, work
scheduling, catalog threshold helpers, and config defaulting must carry the
opaque value without reading `Harness`, `Provider`, or `Model`.

Route-resolution diagnostics are not part of normal execution. Fizeau may expose
operator-facing status/debug surfaces such as route-status and provider
diagnostics, but DDx must never feed a diagnostic route decision back into
`Execute`.

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
returned by Fizeau. FEAT-010 owns whether a retry or review retry is
scheduled. FEAT-014 owns normalized usage and cost signals.

DDx may compute the next profile intent or `MinPower` floor from profile/model
metadata, but it must not translate that intent into a concrete model/provider
or mutate operator-supplied passthrough values. Review pairing uses the same
boundary: DDx can request stronger review and record `review-pairing-degraded`
when the actual reviewer route converges with the implementer, but concrete
reviewer selection remains inside Fizeau.

## Session Log Envelope Boundary

The upstream agent owns the native session-log schema and any harness-specific
details inside it. DDx owns only the envelope around that log:

- request id, layer, parent run id, bead id or produced artifact id
- requested `MinPower` / `MaxPower` and opaque passthrough constraints
- actual model and actual power returned by the agent
- pointer or copied attachment for `ExecuteResponse.SessionLogPath`
- DDx-owned attempt outcome, merge/preserve outcome, gates, and evidence refs

DDx may link or copy the session log attachment for humans, but normal
execution policy must not render the inner session log, parse transcript
semantics, or infer routing, provider health, model fallbacks, or retry
destinations from it. DDx treats the session log as a Fizeau-owned transcript
artifact and only uses typed agent response fields plus DDx-owned attempt
evidence as policy inputs. DDx copies or links the artifact as evidence; it
does not present the inner session log as a DDx-rendered view.

## Human Stdout Projection Boundary

Fizeau **typed service events** remain **opaque** to DDx — DDx must not parse,
rehydrate, or semantically interpret the inner event schema or payload. This
opacity applies to `ServiceEvent` fields, structured transcript events, session
events, tool-call details, model-routing metadata, and any event payload that
Fizeau owns.

DDx **may** project summary information derived from envelope-level Fizeau
response fields (such as actual model, actual power, route label, and tool call
index from `ExecuteResponse`) into **phase-labeled human stdout** without
mutating the event schema. For example, DDx may emit a one-line progress note
such as:

```
[impl] running: codex / claude-sonnet (power 8) — tool calls: 14
```

DDx must not emit Fizeau inner event details (provider-internal routing
decisions, model fallback chains, raw transcript tokens, or session-log content)
as human stdout. Phase labels (`[impl]`, `[review]`, `[triage]`) are
DDx-owned display context; they are not Fizeau event fields and must not be
confused with changes to the Fizeau event schema.

## Layer Ownership Migration Table

FEAT-006 is deliberately narrow after the three-layer refactor. It describes
how DDx consumes the upstream agent contract and how layer-1 requests are
formed; it does not own worktree, queue, or CLI namespace design.

| Area | Owner after migration |
| --- | --- |
| CONTRACT-003 Fizeau execution boundary, `ExecuteRequest` / `ExecuteResponse` consumption, `MinPower` / `MaxPower`, actual model/power recording | Stays in FEAT-006 |
| Opaque passthrough envelope for `--harness`, `--provider`, `--model` and the rule that DDx must not route on those fields | Stays in FEAT-006 |
| Non-bead layer-1 invocation intent: artifact-keyed power bounds, permissions, timeout, effort, and metadata | Stays in FEAT-006 |
| Session-log envelope and pointer/copy capture around the agent-owned inner log | Stays in FEAT-006 |
| Worktree creation, base revision pinning, bead prompt resolution, result landing, preserve refs, post-run gates, evidence bundles | Moves to FEAT-010 layer 2 (`ddx try`) |
| Unified `.ddx/runs/<run-id>/` substrate, layer metadata, child/parent run links, migration from `.ddx/exec-runs/` and `.ddx/executions/` | Moves to FEAT-010 |
| Queue iteration, no-progress detection, retry scheduling, cooldowns, stop conditions, loop records | Moves to FEAT-010 layer 3 (`ddx work`) |
| Public top-level command shape: `ddx run`, `ddx try`, `ddx work`, `ddx runs`, `ddx tries`, `ddx work workers` | Moves to FEAT-001 |
| Structural mounting of the upstream agent Cobra root under the retired DDx workflow namespace and hard-deprecation redirects for legacy workflow commands | Moves to FEAT-001, with the contract boundary referenced here |

### Conflict-recovery structured outcomes (bead ddx-0097af14)

Before parking a bead on `land_conflict` (or on `execution_failed` when the
agent produced a commit before failing), the `ddx work` drain now attempts to reuse
the preserved iteration ref rather than discarding the agent's work:

1. **3-way ort auto-merge** (`git merge --no-ff -s ort -X ours`). If clean,
   the bead closes as `success`.
2. **Focused conflict-resolve agent** (if `ConflictResolver` is configured on
   the worker). If clean, the bead closes as `success`.
3. **Park with structured outcome.** Two structured outcomes replace the generic
   reopen when both recovery paths fail:

| Outcome | Event kind | Meaning |
| --- | --- | --- |
| `land_conflict_unresolvable` | `land-conflict-unresolvable` | All auto-recovery failed; retryable after 15-min cooldown. |
| `land_conflict_operator_required` | `land-conflict-operator-required` | Focused-resolve agent returned BLOCKING; move bead to `status=proposed`. |

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

## Asking Fizeau for changes

When DDx needs new behavior from the service — a new method, a new field on
`ExecuteRequest`, a new event type, a new policy knob — file a PR against
CONTRACT-003 in the Fizeau repo. Maintainers decide whether the surface grows.

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
source of truth for Fizeau observability for one alpha release lag, then
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
- Historical thin-consumer migration plan — superseded by the current Fizeau consumer spec
- `docs/helix/03-test/test-plans/TP-006-agent-session-capture.md` — session capture test coverage
- `docs/helix/03-test/test-plans/TP-014-token-awareness.md` — token-awareness coverage
- `docs/helix/03-test/test-plans/TP-020-fizeau-boundary-and-pass-through.md` — DDx boundary coverage for raw passthrough and no-local-routing behavior
- `docs/helix/02-design/adr/ADR-021-operator-prompt-beads-web-write-path.md` — operator-prompt beads as the web write path (Story 15)
- `docs/helix/02-design/adr/ADR-022-worker-client-server-architecture.md` — workers as long-lived API clients; server-restart preserves in-flight work
- `docs/helix/02-design/adr/ADR-024-power-escalation-and-review-routing.md` — DDx power escalation, review routing, and cost-cap policy boundary
