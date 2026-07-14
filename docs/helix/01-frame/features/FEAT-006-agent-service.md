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

DDx requests agent sessions through the upstream Fizeau execution contract,
**CONTRACT-003**, in the `~/Projects/fizeau` repo
(`docs/helix/02-design/contracts/CONTRACT-003-fizeau-service.md`).

That contract is the entire execution boundary. DDx exposes the public
`ddx run` / `ddx try` / `ddx work` layers, forwards raw passthrough constraints
unchanged, and sends requests to Fizeau for execution. Fizeau is the full
harness-of-harnesses: it owns Claude Code, Codex, Gemini, and native invocation;
the session/tool loop; routing and provider fallback; subprocess and process-
tree control; progress/events; native session logs; usage; cancellation; and
harness-specific continuation. DDx never invokes or parses a concrete harness
directly. Upstream
diagnostic/status commands may remain as Fizeau-owned observability, but the
retired workflow namespace is not a public workflow surface and has no legacy
alias.

The authority chain is explicit:

1. DDx claims/selects a bead when applicable, pins the base revision, creates
   the worktree, and constructs a request from the bead, operator constraints,
   and DDx policy facts.
2. Fizeau runs the complete agent session and returns either a typed immediate
   `Execute` error or a public final event/result plus an opaque session-log
   reference.
3. DDx runs repository gates, evaluates result/commit/review evidence, lands or
   preserves the result, and decides bead-attempt success.
4. Only after that Fizeau operation has ended and DDx has classified the attempt
   may DDx decide whether to launch a new bead attempt.

Fizeau session success is an input, not authority to mark a bead successful or
closed.

## DDx-side responsibilities

DDx owns the bead-driven workflow surface. It does NOT own:
- Concrete harness invocation or output parsing (Claude Code, Codex, Gemini,
  or native)
- Session/tool-loop execution, compaction, or tool registration
- Provider/model routing, health, quota handling, or fallback
- Subprocess creation, process-group/process-tree control, or harness cleanup
- Session progress/events, native logs, usage collection, or continuation
- Session cancellation mechanics or session-log shape

Those all live inside Fizeau per CONTRACT-003.

Historical migration notes below preserve older ddx-agent naming and draft
shapes for traceability only. They are not the desired current contract names
or preferred DDx terminology.

DDx owns:

- **Bead-driven request construction.** `ddx try`, `ddx work`, and the server's
  queue-drain worker translate bead state into `ExecuteRequest` values and
  surface DDx-classified attempt results back into the bead tracker. The queue-drain worker
  uses explicit `once`, `drain`, and `watch` modes: direct CLI `ddx work`
  drains and exits by default, while server-managed workers default to
  `watch` with a 30s `idle_interval`. Watch workers stay alive across
  empty polls and surface "no ready work" as a transient `idle` substate.
  Operators choose one-shot CLI work with `--once`.
- **Bead-attempt orchestration.** Worktree creation, base-revision pinning,
  result landing (merge / preserve / no-changes), gate evaluation, evidence
  bundle capture. Fizeau provides the complete agent runtime; DDx provides the
  git-aware orchestration.
- **Structured prompt handoff.** Review, grading, and other prompt-bearing
  invocations use the bounded assembly invariants from FEAT-022 and the
  caller-facing structured evidence envelope described in TD-033.
  DDx requests TD-033's no-tool reviewer constraints; Fizeau is responsible for
  enforcing those constraints in the concrete session.
- **Evidence and session envelope capture.** DDx writes `.ddx/executions/<id>/`
  bundles with prompts, manifests, repository checks, and result artifacts.
  The session-log path in Fizeau's contract-defined terminal result is captured
  into the bundle as an opaque link or copy; DDx does not own or normalize its
  contents.
- **Power policy at the request level.** DDx selects the initial `MinPower`,
  effort, and permissions from bead metadata and explicit user intent. It may
  raise `MinPower` for stronger review intent or for a distinct new bead attempt
  only after capability-sensitive DDx evidence. Route, quota, transport,
  authentication, setup, operator-action, and generic failures never justify a
  power raise. An operator-supplied `MaxPower` is forwarded unchanged as a hard
  cap. DDx never originates or mutates that cap, and leaves the concrete route
  to Fizeau.
- **Agent passthrough constraints.** DDx may accept explicit `--harness`,
  `--provider`, and `--model` values from the operator and pass them unchanged
  to Fizeau. DDx does not validate, rank, fallback, rewrite, or reason about
  these fields; they are opaque constraints for Fizeau to interpret.
- **New-attempt policy.** DDx decides whether to launch another bead attempt
  because DDx owns the evidence
  needed to decide whether an attempt succeeded: commits, merge/preserve
  result, no-changes rationale, post-run gates, review verdicts, cooldowns,
  and prior run metadata. On a distinct new attempt, DDx may raise `MinPower`
  only for a capability-sensitive failure under ADR-024. Fizeau
  owns all retry, fallback, and harness-specific continuation inside a session,
  and maps power bounds to a concrete harness/provider/model.
- **Review capability request facts.** When DDx launches the default adversarial
  pre-close reviewer gate it uses the same `Execute` boundary, requests
  stronger reviewers by raising `MinPower` relative to the implementer's actual
  power, and supplies `role=reviewer` plus DDx correlation metadata (`bead_id`,
  `attempt_id`, `result_rev`, `review_group_id`, reviewer slot, and implementer
  actual power when known). The opaque terminal `SessionLogPath` may be linked
  as evidence but is not a required request field. These facts correlate the review
  and set its abstract minimum capability. `Harness`, `Provider`, `Model`, and
  policy remain unset unless they came from explicit operator passthrough;
  requesting a stronger review changes `MinPower` only. See ADR-024.

DDx owns bead/worker lifecycle progress and durable DDx execution evidence.
Fizeau owns session progress/events, transcript/session rendering,
provider/model discovery and fallback, usage, cancellation, continuation,
process-tree control, and native session presentation. DDx is a
pass-through/marshalling consumer of
opaque Fizeau transcript events: it may forward Fizeau `ServiceEvent`s
unchanged and link or copy Fizeau artifacts into the execution evidence
bundle, but DDx tracks those payloads only as opaque evidence. DDx keeps the
inner Fizeau transcript or session-log content opaque; it does not rehydrate,
parse, render, or semantically interpret it, and those payloads never become
DDx worker state. The session log payload remains opaque to DDx, which only
carries the envelope around it.

## Power Intent

DDx does not choose routes. It sends request-level work facts and a `MinPower`
floor. An operator-supplied `MaxPower` or public Fizeau `Policy` may also pass
through unchanged. Current v0.14.50 has no per-request `Profile` field; legacy
profile settings are migration debt, not an alias DDx may translate into a
route. DDx does not originate or mutate operator constraints.
Fizeau selects the applicable policy and resolves harness, provider, endpoint,
model, health, quota, fallback, and route errors.

Power is an abstract integer scale owned by the Fizeau contract. DDx treats
`MinPower`/`MaxPower` as bounds on that scale, not as model identities. For
example, Fizeau may report:

```text
running with qwen 3.6-27b (power 10)
```

DDx records any requested public `Policy`, requested `MinPower`/operator-owned
`MaxPower`, and the actual model/power returned by Fizeau. Actual model identity
is audit-only. DDx can use only the abstract actual-power evidence on a later
new bead attempt to raise `MinPower` when DDx evidence shows a
capability-sensitive failure; it does not select a profile from a
catalog or choose the next harness, provider, or model. Weak-first routing,
availability handling, provider preference, route degradation, and fallback
are Fizeau policy. Profile names are Fizeau configuration, not DDx constants;
DDx must not hard-code `cheap`/`standard`/`smart` taxonomies or inspect model
metadata to implement routing.

DDx may also pass request facts such as estimated prompt size, whether tools are
required, permissions, timeout values, and effort/reasoning intent. These facts
describe the work; they do not select a model.

## Non-Bead Invocation Intent

Not every layer-1 invocation comes from a bead. Artifact generation,
artifact regeneration, prompt comparison skills, and diagnostics can call
`ddx run` directly. For those calls DDx may choose work intent from the
artifact or operation being performed:

- `MinPower` derived from the artifact's generator, media type, size, risk, or
  explicit operator flags; an explicit operator `MaxPower` is forwarded
  unchanged
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

DDx MUST NEVER originate, rank, infer, loosen, rewrite, or remove these pins,
nor direct Fizeau to a concrete harness/provider/model by any other field.
Review strength and retry escalation change abstract power intent only. The
only concrete route constraints DDx can send are operator-supplied values
forwarded verbatim.

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

If an operator supplies passthrough constraints and DDx starts a distinct new
attempt at higher `MinPower` after capability-sensitive evidence, DDx keeps the
passthrough values unchanged. Fizeau decides
whether the power bounds and passthrough constraints are compatible and reports
the actual model/power or a typed error. DDx records the requested
passthrough values for audit, but it does not use them to select a route.

When hard passthrough pins make the requested power bounds unsatisfiable, DDx
must stop when a current public immediate error (or a compatible future typed
final classification) reports the incompatibility. Generic final error text
remains unclassified. DDx records
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

## Runtime Diagnostics Boundary

DDx has no model/power catalog, provider-health, quota, or route-candidate
dependency or proxy. Its UI may link to a configured Fizeau diagnostics surface
and may render per-run public terminal audit facts. DDx retry policy may use the
prior outcome's abstract actual power to size a distinct new attempt only after
capability-sensitive DDx evidence; it must not query or cache detailed routing
information. Concrete actual route fields appear only on that run's audit
detail and never feed filters, aggregates, a request, or a policy decision.

## Escalation And Review Capability Boundary

ADR-024 is the governing policy for power escalation and review intent. FEAT-006
owns only the request envelope: requested `MinPower`, an unchanged
operator-supplied `MaxPower`, opaque passthrough constraints,
role/correlation metadata, and terminal audit facts returned by Fizeau.
FEAT-010 owns whether a retry or review retry is scheduled. FEAT-014 owns
normalized usage and cost signals.

DDx may raise a later bead attempt's `MinPower` only when DDx-owned outcome
evidence is capability-sensitive. Infrastructure, transport, route, quota,
authentication, setup, operator-action, and generic failures leave power
unchanged. DDx must not derive a policy/profile or route from catalog metadata
or mutate operator-supplied passthrough values.
Review uses the same boundary: DDx requests a stronger abstract `MinPower` and
does not compare implementer and reviewer harness/provider/model identity.
Concrete reviewer selection and any route diversity policy remain inside
Fizeau.

## Session Log Envelope Boundary

Fizeau owns the native session-log schema and any harness-specific
details inside it. DDx owns only the envelope around that log:

- request id, layer, parent run id, bead id or produced artifact id
- requested `MinPower`, unchanged operator `MaxPower`, and opaque passthrough
  constraints
- actual model and actual power returned by Fizeau
- pointer or copied attachment for the terminal result's session-log path
- DDx-owned attempt outcome, merge/preserve outcome, gates, and evidence refs

DDx may link or copy the session log attachment for humans, but normal
execution policy must not render the inner session log, parse transcript
semantics, or infer routing, provider health, model fallbacks, or retry
destinations from it. DDx treats the session log as a Fizeau-owned transcript
artifact and only uses typed agent response fields plus DDx-owned attempt
evidence as policy inputs. DDx copies or links the artifact as evidence; it
does not present the inner session log as a DDx-rendered view.

## Human Stdout Projection Boundary

Fizeau's non-terminal progress, transcript, tool, and routing event payloads
remain opaque to DDx: DDx must not rehydrate or semantically interpret their
inner schemas. DDx decodes the current public `ServiceFinalData` fields: status,
exit code, generic error text as opaque evidence, `FinalText`, duration,
usage/cost, warnings, session-log path, and `RoutingActual`. Current v0.14.50
does not expose general cause, stage, `RetryAfter`, continuation capability, or
process-tree disposition in that final payload. Phase 1 may consume those fields
only after CONTRACT-003 adds them and DDx pins a compatible release. The current
immediate `NoViableProviderForNow` error is separately typed and carries its own
`RetryAfter`; DDx may consume that public type without parsing its message.

DDx may prepend DDx-owned phase context such as `[impl]`, `[review]`, or
`[triage]` while forwarding a Fizeau-provided human-display string or event
unchanged. DDx may also render envelope-level facts after the public final event
arrives (for example, actual model, actual power, and usage). It does
not derive live progress by counting or interpreting tool/session events.
For example, if Fizeau supplies the display text, DDx may forward:

```
[impl] running: codex / claude-sonnet (power 8)
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
| CONTRACT-003 request/terminal consumer boundary, including the rule that Fizeau owns concrete invocation, session/tool loop, routing/fallback, process tree, progress/logs/usage, cancellation, and continuation | Stays in FEAT-006 |
| Opaque passthrough envelope for `--harness`, `--provider`, `--model` and the rule that DDx must not route on those fields | Stays in FEAT-006 |
| Non-bead layer-1 invocation intent: artifact-keyed power bounds, permissions, timeout, effort, and metadata | Stays in FEAT-006 |
| Session-log envelope and pointer/copy capture around the Fizeau-owned inner log | Stays in FEAT-006 |
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
2. **Focused conflict-resolve session** (if `ConflictResolver` is configured on
   the worker). DDx constructs a focused request through the same Fizeau
   boundary; if the repository result is clean, the bead closes as `success`.
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

The work policy must distinguish *worker disruption* from *session completed
without useful repository progress*. A cancelled or failed Fizeau operation
that never gave the implementer a valid opportunity to act is not evidence of
bead no-progress. DDx derives this distinction only from its own context state,
public typed immediate errors, and fields that actually exist on the pinned
Fizeau final result. It never parses a concrete harness error string, generic
final `Error`, transport marker, transcript, or preflight prose.

The loop maps the typed immediate error or final-event outcome into
`ExecuteBeadReport.Disrupted = true` plus a stable DDx `DisruptionReason`:

| Reason | Source |
| --- | --- |
| `context_canceled` | DDx root/attempt context was cancelled and the Fizeau execution stream ended |
| `context_deadline` | DDx attempt deadline elapsed and the Fizeau execution stream ended |
| `route_temporarily_unavailable` | `Execute` returned public `*fizeau.NoViableProviderForNow`; DDx releases the bead claim and honors only its typed queue-level `RetryAfter` |
| `unclassified_fizeau_failure` | Current final status/error indicates failure but exposes no typed cause; record evidence and avoid automatic power/cooldown policy until the contract is upgraded |

When `report.Disrupted` is true:

1. `shouldSuppressNoProgress` returns false unconditionally — the loop
   bypasses `SetExecutionCooldown` so the bead is immediately re-claimable.
2. A `disruption_detected` event is appended to the bead and emitted on the
   loop event sink (`{reason, detail, fizeau_outcome_ref}`) so operators can see
   disruption rates without turning route identity into policy.
3. Genuine no-progress (terminal Fizeau session, BaseRev == ResultRev, not
   Disrupted) still hits the configured `noProgressCooldown` (default 6h).

Until the compatible Phase 1 contract exposes general typed cause/stage, final
failures outside the current public typed errors stay
`unclassified_fizeau_failure`; DDx does not restore fuzzy parsing as a fallback.

## Bead-Attempt Worker Sub-task Discovery Policy

**Design position: option (b) — surface via result.**

Workers executing inside `ddx try` / `ddx work` bead attempts MUST NOT call
`ddx bead create`
in-band during their execution run. Creating beads in-band (option c) allows a
single worker to flood the queue with unreviewed children — in one observed case
(ddx-44236615), one worker spawned 11 P0 children without operator review. This
violates the architectural principle from FEAT-013: *"DDx provides primitives,
not orchestration. Orchestration policy stays in HELIX and other workflow tools."*

Workers MAY surface discovered sub-tasks in a DDx-owned application-result JSON
schema returned as `ServiceFinalData.FinalText`. After the Fizeau operation
ends, DDx validates that declared application result and writes its optional
`discovered_subtasks` array into `result.json`. Each entry is a lightweight
object with at minimum a `title` and optional `description`, `labels`, and
`priority` fields. DDx never derives this field from provider stdout, transcript
events, or the native session log. The `ddx work` drain or supervisor passes the
validated array to the workflow tool (HELIX or operator) for decomposition
decisions. The supervisor decides whether, when, and at what priority to file
new beads — DDx does not do this automatically.

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
orchestrator writes it only after the DDx application-result schema in
`ServiceFinalData.FinalText` validates. The supervisor MUST NOT act on this
field automatically; it is surfaced as an observation for the workflow tool.

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
`issueType: operator-prompt` and drained by the standard work. The
title is the prompt's first line, the body is the full prompt verbatim,
default labels are `kind:operator-prompt,source:web-ui`, and the AC section
is auto-generated from a template. These beads run with the same
operator passthrough constraints and abstract power intent as regular beads but skip the structural AC check
(which assumes pre-authored AC). They cannot mutate other operator-prompt
beads — a hard rule enforced at the bead store, not at planning time.

The trust contract, audit-as-bead model, multi-node delegation policy,
prompt-injection threat model, and allowed-mutation scope are captured in
**ADR-021: Operator-Prompt Beads as the Web Write Path**.

## Worker Contract (per ADR-022)

Workers are **autonomous but server-preferred**. They always read durable
project state and can execute without a server, but continuously discover the
project-scoped `ddx-server`. While connected, every coordination-sensitive
mutation—claim/lease, tracker transition, and landing—goes through the server's
single per-project coordinator. While disconnected, the worker uses the same
local implementations under cross-process locks, appends each mutation to an
ordered durable journal, and continues working. On reconnect it reconciles
that journal idempotently before sending new mutations through the server.

The bead store and git history remain the durable sources of truth. The server
does not replace them with a parallel claim table; it serializes operations
against them. Manual and server-managed workers use the same client and
protocol. A manual worker survives a server outage; a server-managed worker is
owned by the server process tree and exits when the server exits.

ADR-022 rev 6 is the governing decision. Rev 5 implemented continuous
discovery and event backfill but stopped at observability, leaving manual and
server-managed commands with separate process-local land coordinators. Rev 6
extends that state machine to coordination mutations without sacrificing
offline operation.

### Server probe goroutine

In parallel with the autonomous work loop, every worker runs a small
"server probe" goroutine that detects whether a server is reachable via
`~/.local/share/ddx/server.addr` and tracks a Connected ↔ NotConnected
state machine. Probe cadence: immediate first probe on startup, then a
jittered 30s steady-state interval (10s minimum, 5min maximum); five
consecutive failures back the rate off to 5min, resetting to 30s on the
next success. A 410 unknown_worker reply triggers re-registration within
the same cycle.

### Coordination and event mirror

When the worker is Connected it sends coordination mutations to the server and
mirrors lifecycle events best-effort. A coordination transport failure causes
an explicit Connected → NotConnected transition: the worker journals the
operation, completes it locally under the same idempotency key, and continues.
Event-only mirror failures are buffered without blocking work. All endpoints
are project-scoped and backed by `requireTrusted` (per ADR-006):

- `POST /api/workers/register` — emitted on every NotConnected → Connected
  transition (not only at startup). Body is a thin identity envelope
  (`project_root`, `executor_pid`, `executor_host`, `started_at`); concrete
  Fizeau route facts belong to per-run audit evidence, not DDx worker identity.
  The response carries a correlation `worker_id` and the
  coordination protocol version.
- `POST /api/projects/<project>/coordination/mutations` — submit one
  idempotent claim, tracker-transition, or landing mutation to the
  server-side per-project coordinator.
- `POST /api/projects/<project>/coordination/reconcile` — submit the ordered
  offline mutation journal. The server returns an outcome for every
  idempotency key (`applied`, `already_applied`, or `conflict`) and the worker
  retains unresolved conflicts as operator-attention evidence.
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

The server's observational view of workers is **derived** from these reports —
eventually consistent, never authoritative. Coordination outcomes are instead
grounded in the bead store and git history. The server-side workers panel
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
  attempts (mid-attempt poll). On `cancel-requested: true` it cancels the context
  supplied to Fizeau `Execute`; a later contract-defined lifecycle operation
  may replace this only after a compatible release is pinned. Fizeau owns the
  harness-specific safe boundary and process-tree cleanup; after the execution
  stream terminates, DDx
  preserves repository evidence and reports `preserved_for_review` with reason
  `operator_cancel`.
- A worker starting work on a bead that is already cancel-marked
  immediately reports `preserved_for_review` and skips the attempt.
- Idempotency: the worker writes `cancel-honored: true` so a re-applied
  marker does not re-trigger.
- Worst-case detection latency is ~10s plus Fizeau's reported cancellation
  latency. OS signals cancel DDx's root context, which propagates cancellation
  through Fizeau; DDx does not signal a concrete harness process itself.

### Restart and crash behavior

- **Server restart or crash, manual worker.** The worker's next request fails;
  it transitions offline, journals coordination mutations, and continues. On
  the next successful probe it re-registers, reconciles the mutation journal,
  and resumes server coordination and event backfill.
- **Server restart or crash, server-managed worker.** The server-owned worker
  process tree terminates with the server. Durable attempt evidence and claim
  leases drive recovery after the supervisor restarts it.
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
responsibilities" above (worktree creation, base-revision pinning, agent
session request, and evidence capture) execute inside the worker process. The
server, when present, serializes coordination-sensitive mutations against the
same durable bead and git state; it does not call Fizeau or become a parallel
source of truth. See ADR-022 for the full transport, offline journal,
reconciliation, picker, freshness, and lifecycle contracts.

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
- `docs/helix/02-design/adr/ADR-024-power-escalation-and-review-routing.md` — DDx power escalation, review capability, and cost-cap policy boundary
