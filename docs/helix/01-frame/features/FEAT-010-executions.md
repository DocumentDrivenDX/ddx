---
ddx:
  id: FEAT-010
  depends_on:
    - helix.prd
    - FEAT-005
    - FEAT-006
---
# Feature: Three-Layer Run Architecture

**ID:** FEAT-010
**Status:** In Progress
**Priority:** P1
**Owner:** DDx Team

## Overview

DDx owns three explicit, layered run primitives. Each higher layer composes
the layer beneath it. There are no other run kinds beyond these three.

| Layer | CLI | Inputs | Outputs / side effects | Owns |
|---|---|---|---|---|
| 1 | `ddx run` | prompt + agent config | structured output, side effects, run metadata (tokens, model, duration, exit) | invocation atom; consumes upstream `ddx-agent` per CONTRACT-003 |
| 2 | `ddx try <bead>` | bead id, base revision | new worktree state + agent-run evidence; merge or preserve | worktree start/end capture; bead → prompt resolution; side-effect bundling |
| 3 | `ddx work` | bead queue, stop conditions | sequence of `ddx try` records + loop-level record (drained / blocked / deferred) | mechanical queue drain; no-progress detection |

`ddx try` wraps `ddx run`. `ddx work` iterates `ddx try`. One on-disk
substrate; layer metadata distinguishes records.

`ddx artifact regenerate <id>` is sugar over layer 1 (or layer 2 when the
generator edits the repo) with `produces_artifact: <id>` metadata. It is
not a fourth layer — it is a labeled invocation at an existing layer.

## Problem Statement

**Current situation:** DDx has accreted two independent run-storage shapes
— `.ddx/exec-runs/` for generic execution evidence and
`.ddx/executions/<attempt-id>/` for tracked bead-attempt bundles. Loop behavior,
worktree lifecycle, and bead resolution are spread across legacy command
surfaces without a clean layering story. There is no single place to introspect "what runs
happened, at what layer, with what evidence."

**Pain points:**
- Two on-disk record shapes for fundamentally similar artifacts (a single
  agent invocation, with logs, structured result, and provenance)
- No first-class concept of the three composing layers (atom / attempt /
  drain), so consumers must intuit which surface to read
- Stop conditions for queue drain are implicit and not observable
- Generated-artifact provenance has no clean hook into the run record
- Open-ended catalog pressure: every new "run flavor" (benchmark, replay,
  comparison) tempts a new on-disk shape or a new top-level verb

**Desired outcome:** A single on-disk run substrate with explicit layer
metadata, three top-level CLI verbs that map 1:1 onto the layers, and a
narrow read-only HTTP/MCP write surface limited to artifact regeneration.

## Three-Layer Architecture

### Layer 1 — `ddx run` (invocation atom)

A layer-1 run is one agent invocation. Inputs are a prompt, requested
`MinPower` and optional `MaxPower`, optional agent passthrough constraints, and
non-routing execution config;
outputs are the structured response (text or bytes), any side-effects the agent
performed via tools, and run metadata (tokens, model, actual power, duration,
exit status, session pointer).

Layer 1 calls the upstream `ddx-agent` service contract directly. DDx does not
reimplement agent routing or the invocation loop; it wraps one `Execute` call
with provenance capture.

`ddx artifact regenerate <id>` is layer 1 when the generator returns
bytes/text and DDx writes the file; it is layer 2 when the generator
edits the repo directly (bead-less worktree).

### Layer 2 — `ddx try <bead>` (bead attempt)

A layer-2 run is one bead attempt in an isolated worktree. It owns:

- Worktree creation from a base revision and worktree finalization
  (merge or preserve)
- Bead → prompt resolution (description, acceptance, governing artifacts)
- Side-effect bundling (commits, evidence, no-changes rationale)
- One or more layer-1 invocations recorded inside the attempt record

A layer-2 record references its child layer-1 records by run id.

Layer 2 owns bead-attempt success classification. DDx determines success from
the artifacts it owns: commit presence, merge/preserve result, no-changes
rationale, post-run checks, review verdicts, and cooldown policy. The agent's
exit status and actual model/power are inputs to that decision, not the whole
decision.

### Layer 3 — `ddx work` (queue drain)

A layer-3 run is one drain of the bead queue. It iterates `ddx try`
across ready beads until a stop condition is met. It owns:

- Queue iteration order
- No-progress / stop-condition evaluation
- A loop-level record that references its child layer-2 records by
  attempt id and reports terminal disposition (drained, blocked,
  deferred, no-progress)

Content-aware supervisory decisions (e.g., "comparison failed → enqueue
reconciliation beads") are not layer-3 concerns. Those are skill or
plugin compositions on top of the three layers.

## Agent Power and Retry

DDx owns retry policy between bead attempts. If an attempt produces classified
evidence that more capable reasoning could plausibly change the result, DDx may
retry the bead with a higher `MinPower`.

The agent owns routing within the requested power bounds. A retry request says
"use at least this much power" by raising `MinPower`; an explicit `MaxPower`, if
supplied, remains a hard upper bound. Operator-supplied `--harness`,
`--provider`, and `--model` values may be sent to the agent as passthrough
constraints, but DDx does not interpret them. Each layer-1 record stores:

- requested `MinPower` and optional `MaxPower`
- requested passthrough constraints, if any
- actual model and actual power reported by the agent
- run outcome and DDx attempt outcome

DDx may query the agent's available model/power catalog to choose escalation
targets such as "retry using only top-power models." DDx uses the returned power
numbers only to compute `MinPower` thresholds; the agent still chooses the
concrete model.

Passthrough constraints stay sticky across retries. DDx may increase `MinPower`
between attempts, but it must not alter or drop operator-supplied
harness/provider/model passthrough values to make escalation work. If the
combination is invalid or constraining, the agent owns that typed error or
actual route. DDx stops with `blocked_by_passthrough_constraint` or
`agent_power_unsatisfied`, records the evidence, and reports operator action
required; it does not remove pins, widen pins, call `ResolveRoute`, or retry in a
loop.

### Retry eligibility

Power retry is eligible only when DDx-owned evidence shows that a stronger model
could plausibly help after a valid attempt started. Eligible classes include:

- `capability_insufficient` — the agent attempted the bead, but reasoning
  quality or implementation quality was insufficient.
- `post_run_check_failed` — tests or gates failed after a valid checkout and
  attempted change.
- `review_blocked_capability` — review blocked the result for issues plausibly
  addressable by stronger reasoning.
- `no_changes_after_attempt` — the agent had a valid checkout and task context
  but produced no usable change, subject to the no-progress budget.

Power retry is not eligible for deterministic setup or operator-action
failures:

- dirty worktree, merge/land conflict, missing checkout, invalid bead metadata,
  unresolved dependencies, config parse errors, missing harness binaries,
  authentication failures, command-not-found/toolchain setup failures
- `blocked_by_passthrough_constraint` / `agent_power_unsatisfied`
- any status where retry would require DDx to inspect, remove, rewrite, or
  substitute `--harness`, `--provider`, or `--model`

Every retry/stop decision records the classification and evidence used. The
policy may inspect DDx attempt outcomes and the agent's typed status; it must
not branch on concrete provider/model identity.

## Substrate Unification

### One record shape

All three layers persist to a single on-disk shape under
`.ddx/runs/<run-id>/`. The record has the following common fields and a
discriminating `layer` field; layer-specific extensions live in named
subobjects rather than separate file trees.

Common fields:
- `run_id` — stable, sortable identifier
- `layer` — `1` | `2` | `3`
- `parent_run_id` — pointer to the enclosing layer's run, if any
- `started_at` / `finished_at`
- `terminal_status` — `success` | `failure` | `timeout` | `error` |
  `no_progress` (layer 3 only) | `preserved` (layer 2 only)
- `actor`, `host`, `git_revision`, `ddx_version`
- `produces_artifact` — optional artifact id when this run regenerates
  an artifact
- `attachments[]` — references to large bodies (logs, structured payloads,
  bundles) stored alongside the record rather than inline

Layer extensions:
- `layer1` — prompt reference, agent config, model id, token usage,
  upstream session id, structured response pointer
- `layer2` — bead id, base revision, worktree path, finalization mode
  (`merge` | `preserve`), child layer-1 run ids, evidence-bundle pointer
- `layer3` — queue snapshot pointer, stop-condition evaluation log,
  child layer-2 attempt ids, drain disposition

Records are append-only. A new run never rewrites a previous one.

### On-disk migration

The two pre-existing trees collapse into the unified substrate:

- `.ddx/exec-runs/<run-id>/` → `.ddx/runs/<run-id>/` with `layer: 1`
- `.ddx/executions/<attempt-id>/` → `.ddx/runs/<run-id>/` with `layer: 2`
  and the existing prompt / manifest / result / checks / provenance
  surfaced as layer-2 evidence-bundle attachments

Layer-3 records are new (no pre-existing tree).

Migration is one-way and read-compatible:
- Writers always target `.ddx/runs/`
- Readers tolerate legacy paths during a documented migration window
  and surface them through the unified read API as if they were layer-1
  or layer-2 records, then a one-shot rewrite normalizes them on disk
- After the migration window closes, legacy paths are removed; readers
  no longer fall back

The migration window, the rewrite tool, and the cutoff signal are
specified in the implementation epic, not here.

## `ddx work` Stop Conditions

`ddx work` drains the queue until **any** of the following stop
conditions is met. The terminal disposition of the layer-3 record names
which condition fired.

1. **`drained`** — queue is empty (no ready beads remain).
2. **`blocked`** — every remaining ready bead has produced a terminal
   non-success outcome on its most recent attempt and re-attempt is not
   permitted under the current policy.
3. **`deferred`** — a configured wall-clock or attempt-count budget is
   exhausted. The remaining queue is intact and a subsequent `ddx work`
   resumes from where this one stopped.
4. **`no_progress`** — `N` consecutive `ddx try` attempts produced no
   commit and no merged side-effect on the base branch (default
   `N = 3`; configurable). This is the mechanical "we are spinning"
   detector, not a content-aware judgment.
5. **`signal`** — a SIGINT/SIGTERM was received between attempts. The
   in-flight `ddx try` (if any) finalizes per its own merge/preserve
   policy before the loop record closes.

Stop-condition evaluation runs **between** `ddx try` invocations; an
in-flight attempt is never aborted to satisfy a stop condition (`signal`
excepted, and only after the in-flight attempt finalizes).

### Long-running default (`--poll-interval`)

Per ddx-dc157075, the default `--poll-interval` for both `ddx work` and
`ddx agent execute-loop` is **30s**. With a positive poll interval the
worker stays alive across empty polls (`drained` becomes a transient
"running (idle)" substate, not a terminal exit). The loop exits only on
the conditions enumerated above plus `signal`, fatal config errors, or
the explicit operator opt-outs:

- `--once` — process at most one ready bead, then exit.
- `--poll-interval=0` — legacy "drain-and-exit" semantics: when the
  queue empties, return immediately without polling.

Server-managed workers spawned via `POST /api/agent/workers/execute-loop`
inherit the same 30s default when the request omits `poll_interval`. The
worker record exposes a `substate` field set to `"idle"` while the loop
is sleeping between empty polls; it is cleared as soon as a candidate is
picked. Terminal worker states (`exited`, `failed`, `stopped`, `reaped`)
always clear `substate`.

The structured `loop.end` event carries an `exit_reason` field
(`once_complete`, `explicit_poll_zero`, `sigint`, `sigterm`,
`fatal_config`; `providers_exhausted` is reserved for the quota-pause
work in ddx-aede917d).

Power escalation is evaluated between attempts as part of retry policy. A
higher-power retry raises `MinPower` and resets neither the evidence history nor
the no-progress counter; it is recorded as the next `ddx try` with its own
requested bounds and actual power metadata.

The evaluation log is persisted on the layer-3 record so a human or
tool can audit which condition fired and on which iteration.

## No Run-Type Catalog

DDx will not introduce additional run kinds beyond the three layers.
Comparison, replay, benchmark, adversarial review, effort estimate,
bead breakdown, and similar workflows are **skill compositions** over
the three layers — they emit ordinary layer-1, layer-2, or layer-3
records, optionally tagged with a free-form `skill` label, and do not
get bespoke storage shapes, bespoke verbs, or bespoke spec sections.

This is a load-bearing non-goal: every future "we should add a fourth
layer for X" proposal is rejected by reference to this section. If a
new workflow cannot be expressed as a composition over `run` / `try` /
`work`, the gap is in the three layers' contract, not in the catalog.

## Requirements

### Functional

1. **One on-disk substrate** — all run records, regardless of layer,
   persist under `.ddx/runs/<run-id>/` with the common record shape and
   a discriminating `layer` field.
2. **Layer metadata** — every run record carries `layer`,
   `parent_run_id`, and the layer-appropriate extension subobject.
3. **Layer-1 invocation** — `ddx run` produces exactly one layer-1
   record per invocation; consumes upstream `ddx-agent` per
   CONTRACT-003.
4. **Layer-2 attempt** — `ddx try <bead>` produces exactly one layer-2
   record, references its child layer-1 records, and finalizes the
   worktree as `merge` or `preserve`.
5. **Layer-3 drain** — `ddx work` produces exactly one layer-3 record
   per invocation, references its child layer-2 attempt ids, and reports
   one of the five terminal dispositions (`drained`, `blocked`,
   `deferred`, `no_progress`, `signal`).
6. **Stop conditions** — `ddx work` evaluates the five stop conditions
   between attempts and persists the evaluation log on the layer-3
   record.
7. **No-progress detection** — the `no_progress` condition is mechanical
   (no commit, no merged side-effect) and configurable via
   `.ddx/config.yaml` (default `N = 3`).
8. **Append-only history** — a new run never rewrites a previous record
   at any layer.
9. **Cross-layer query** — `ddx runs` lists records across all three
   layers with layer filtering; `ddx tries` and `ddx work workers`
   are layer-specific projections over the same substrate.
10. **Run detail inspection** — for any run id, callers can retrieve the
    common fields, the layer extension, attachments, and child run ids.
11. **Generated-artifact provenance** — runs that regenerate an artifact
    set `produces_artifact: <id>` on the run record; FEAT-007 reads this
    to populate `generated_by` graph edges.
12. **Migration from legacy paths** — readers tolerate
    `.ddx/exec-runs/` and `.ddx/executions/<attempt-id>/` during a
    documented migration window and surface them as layer-1 / layer-2
    records; writers always target `.ddx/runs/`.
13. **Configuration** — substrate root, no-progress threshold, deferred
    budget, and retention settings are configurable in
    `.ddx/config.yaml`.

### Non-Functional

- **Determinism:** the persisted record reflects the exact prompt,
  response, logs, and structured result of one invocation.
- **Durability:** record writes are atomic or serialized so concurrent
  writers cannot leave partial records.
- **Observability:** stored records are human-readable enough for
  debugging and machine-parseable enough for automation.
- **Portability:** records remain repo-local and file-backed; no hosted
  service or database is required.
- **Low overhead:** layer-1 record bookkeeping adds minimal overhead
  beyond the underlying agent invocation.
- **Payload resilience:** large prompt, response, log, and bundle
  bodies are stored as attachments and never require rewriting a shared
  history file to persist one run safely.

## CLI Commands

The three top-level verbs map 1:1 onto the layers:

```bash
ddx run --prompt <file> [--agent <name>]                # layer 1
ddx try <bead-id> [--from <rev>] [--no-merge]           # layer 2
ddx work [--workers N] [--budget <duration>]            # layer 3
```

Cross-layer evidence introspection lives under `ddx runs`:

```bash
ddx runs list [--layer 1|2|3] [--artifact ID] [--bead ID]
ddx runs show <run-id>                                  # full record + layer extension
ddx runs log <run-id>                                   # raw logs / attachments
ddx runs result <run-id> [--json]                       # structured result
```

Layer-specific projections:

```bash
ddx tries list [--bead ID]                              # layer-2 only
ddx tries show <attempt-id>
ddx work workers list                                   # layer-3 worker mgmt
```

`ddx artifact regenerate <id>` is sugar over `ddx run` (or `ddx try`
when the generator edits the repo) with `produces_artifact: <id>`
recorded on the run.

## Server Endpoints (FEAT-002 integration)

| MCP Tool | HTTP Endpoint | Description | Mode |
|----------|--------------|-------------|------|
| `ddx_runs_list` | `GET /api/runs` | List run records across layers, filterable by `layer`, `artifact`, `bead` | read |
| `ddx_runs_show` | `GET /api/runs/:id` | Show one run record with layer extension and attachment refs | read |
| `ddx_runs_log` | `GET /api/runs/:id/log` | Raw logs / attachment bodies | read |
| `ddx_runs_result` | `GET /api/runs/:id/result` | Structured result payload | read |
| `ddx_artifact_regenerate` | `POST /api/artifacts/:id/regenerate` | Trigger regeneration of one artifact; returns the new run id | **write — narrow** |
| (GraphQL) | `Mutation.runRequeue` | Re-queue the originating bead of an existing run record (manual operator re-queue from the Runs UI); requires `idempotencyKey`; emits a `run_requeue` audit event on the bead | **write — narrow** |

The HTTP/MCP read surface is total over the unified run substrate.

The write surfaces added by this feature are limited to
`artifactRegenerate` and `runRequeue`. All other run invocation
(layer-1 ad-hoc, layer-2 bead attempts, layer-3 queue drain) remains
CLI-only. Additional write endpoints require a separate feature update;
they are not implicit in this read-coverage expansion.

### Re-queue audit events

The `runRequeue` mutation reopens the originating bead and appends a
`run_requeue` event to that bead's audit log. Concurrent or repeat
submissions with the same `idempotencyKey` collapse to a single
re-queue and a single event (`deduplicated=true` is returned to all
subsequent callers); if the cached idempotency record points at a
missing bead, the requeue is replayed against the run's current
originating bead. The event uses the standard `bead.BeadEvent` envelope
with the following fields:

| Field | Value |
|---|---|
| `kind` | `run_requeue` (constant `RunRequeueEventKind`) |
| `summary` | `run requeued` |
| `actor` | Operator identity (`unknown`/`anonymous` when no identity is resolvable from the inbound HTTP request) |
| `source` | `graphql:runRequeue` |
| `body` | Single line: `identity=<kind> actor=<actor> run_id=<runId> idempotency_key=<key> layer_override=<layer-or-empty>` |

The `body` line is structured for grep/jq parsing: each token is a
`key=value` pair separated by single spaces. `layer_override` is the
empty string when the caller did not pass `RunRequeueInput.layer`.
Operator identity is captured via the same audit pathway used by
operator-prompt beads (ADR-021); when the request carries no
identifying header, the audit fields fall back to
`identity=unknown actor=anonymous`.

Consumers of the audit log (FEAT-008 Runs row expansion, evaluation
skills under FEAT-019) treat `run_requeue` events as the canonical
record that a re-queue happened: there is no separate persisted
`requeue` record beyond (a) the new `open` status on the originating
bead and (b) this event entry.

### Layer-to-substrate mapping for the Runs UI

The web Runs view (FEAT-008 §5, FEAT-021) renders three layer chips
backed by the unified substrate plus the legacy detail backings:

| Chip | Substrate row | Detail backing for row expansion |
|---|---|---|
| `work` | `layer: 3` Run record | Layer-3 record's queue inputs / stop-condition log / child layer-2 attempt ids |
| `try` | `layer: 2` Run record | The `.ddx/executions/<attempt-id>/` bundle attached to the layer-2 record (manifest, prompt, result, checks, verdict) |
| `run` | `layer: 1` Run record | The associated `AgentSession` row (prompt / response / stderr / billing / cached-token detail) joined onto the layer-1 record |

`AgentSession` rows that have no parent layer-2 attempt (raw `ddx
agent log` invocations) surface as synthesized `layer=run` Runs rows
keyed by session id, so no agent-session row is dropped during the
Sessions/Executions tab retirement.

## User Stories

### US-090: Developer Inspects a Layer-1 Run

**As a** developer running an ad-hoc agent invocation
**I want** to inspect the resulting run record
**So that** I can audit prompt, response, tokens, and side-effects

**Acceptance Criteria:**
- Given I invoke `ddx run`, then DDx writes one layer-1 record under
  `.ddx/runs/<run-id>/` with `layer: 1` and the layer-1 extension
  populated.
- Given the run completes, when I run `ddx runs show <run-id>`, then I
  see common fields, the layer-1 extension, requested `MinPower`/`MaxPower`,
  actual model/power, requested passthrough constraints if supplied, and attachment
  references.

### US-091: Developer Inspects a Layer-2 Bead Attempt

**As a** developer reviewing a bead attempt
**I want** to see the attempt's worktree finalization and the child
layer-1 invocations
**So that** I can audit what the agent did inside the attempt

**Acceptance Criteria:**
- Given I invoke `ddx try <bead-id>`, then DDx writes one layer-2
  record with `layer: 2` referencing one or more layer-1 run ids.
- Given the attempt finalizes, when I run `ddx runs show <run-id>`,
  then the record reports `merge` or `preserve` and surfaces the
  evidence bundle.

### US-092: Developer Drains the Queue and Audits the Drain

**As a** developer running `ddx work`
**I want** the loop to stop on a clearly named condition
**So that** I can tell whether the queue was drained, blocked, deferred,
spinning, or interrupted

**Acceptance Criteria:**
- Given I invoke `ddx work`, then DDx writes exactly one layer-3 record
  per invocation referencing each child layer-2 attempt id.
- Given the loop stops, then the layer-3 record's terminal disposition
  is one of `drained`, `blocked`, `deferred`, `no_progress`, `signal`,
  and the evaluation log names which condition fired and on which
  iteration.

### US-093: No-Progress Detection Stops a Spinning Loop

**As a** developer running `ddx work`
**I want** the loop to stop when consecutive attempts make no progress
**So that** I do not burn budget on a pathological queue state

**Acceptance Criteria:**
- Given `N` consecutive `ddx try` attempts produce no commit and no
  merged side-effect (default `N = 3`, configurable), then the next
  stop-condition evaluation triggers `no_progress` and the layer-3
  record terminates with that disposition.
- Given retry policy permits escalation before `no_progress` fires, when
  a retry is scheduled, then the next layer-1 run raises `MinPower`
  and records the actual model/power returned by the agent.
- Given the original invocation supplied `--harness`, `--provider`, or
  `--model`, then retries pass the same values through unchanged and DDx does
  not inspect those values when choosing the next requested `MinPower`.
- Given retry escalation would exceed the power available under hard
  passthrough pins, then DDx stops with `blocked_by_passthrough_constraint` or
  `agent_power_unsatisfied`, records the agent-supplied evidence, and does not
  mutate the pins or call `ResolveRoute`.

### US-094: Cross-Layer Run History

**As a** workflow tool
**I want** to query run history across all three layers from one place
**So that** I do not have to merge two on-disk shapes by hand

**Acceptance Criteria:**
- Given runs exist at all three layers, when a tool calls
  `GET /api/runs?layer=...`, then it receives records of the requested
  layer with consistent common fields.
- Given a layer-3 record exists, then its child layer-2 attempt ids and
  their child layer-1 run ids are reachable by following
  `parent_run_id` pointers.

### US-095: Artifact Regeneration Records Provenance

**As a** consumer of the document graph
**I want** generated artifacts to point back to the run that produced
them
**So that** staleness can be evaluated against generator inputs

**Acceptance Criteria:**
- Given I regenerate an artifact via `ddx artifact regenerate <id>`,
  then the resulting run record has `produces_artifact: <id>`.
- Given a generated artifact has been produced by a run, then
  FEAT-007's `generated_by` edge resolves to that run id.

### US-096: Operator Migrates From Legacy Run Trees

**As a** repo operator adopting the unified substrate
**I want** legacy `.ddx/exec-runs/` and `.ddx/executions/<attempt-id>/`
records to remain inspectable during the migration window
**So that** adopting `.ddx/runs/` does not strand prior evidence

**Acceptance Criteria:**
- Given a repository contains legacy run data, when DDx reads run
  history, then legacy paths surface through the unified read API as
  layer-1 / layer-2 records during the documented migration window.
- Given the migration window closes, then writers target only
  `.ddx/runs/` and a one-shot rewrite has normalized prior records into
  the unified substrate.

## CONTRACT-003 Amendment

This feature requires two narrow read-only-amendment-adjacent changes
to CONTRACT-003: write surfaces for `artifactRegenerate` (HTTP/MCP) and
`runRequeue` (GraphQL). `runRequeue` does not invoke an agent — it
reopens the originating bead so the existing `ddx agent execute-loop`
can claim it again — but it is still a write because it mutates bead
state and appends an audit event (see "Re-queue audit events" above).
No other write surfaces are added; in particular, layer-1, layer-2,
and layer-3 invocation remain CLI-only.

If FEAT-006 implementation reveals additional CONTRACT-003 gaps (e.g.,
upstream Cobra root export details), those amendments are scoped to
FEAT-006 and not to this feature.

## Dependencies

- FEAT-005 (Artifacts) — `produces_artifact` references artifact IDs
- FEAT-006 (Agent Service) — provides the layer-1 consumer-side wrapper
  and CONTRACT-003 boundary
- FEAT-007 (Doc Graph) — `generated_by` edges read `produces_artifact`
  from run records
- DDx CLI infrastructure (config loading, command factory)

## Out of Scope

- **Run kinds beyond the three layers** — comparison, replay, benchmark,
  adversarial review, effort estimate, and similar workflows are skill
  compositions, not new layers or new on-disk shapes.
- **Content-aware supervisory loop decisions** — `ddx work` performs
  mechanical queue drain only. Decisions like "comparison failed →
  enqueue reconciliation beads" are skill / plugin / workflow-tool
  territory.
- **Server-side layer-1/2/3 invocation** — only `artifactRegenerate`
  has a write endpoint in this feature.
- **Workflow-specific action semantics** — phase routing, methodology
  scoring, tool-specific issue closing — delegated to workflow tools.
- **Autonomy semantics and escalation policy** — DDx provides the
  substrate; workflow tools decide policy.
- **Hosted run history storage** — records are repo-local and
  file-backed.
- **Separate domain-specific runtime stores** — there is one substrate.
