---
ddx:
  id: FEAT-010
  depends_on:
    - helix.prd
    - FEAT-005
    - FEAT-006
---
# Feature: Task Execution

**ID:** FEAT-010
**Status:** In Progress
**Priority:** P1
**Owner:** DDx Team

## Overview

DDx owns the task execution lifecycle. The lifecycle is made of concrete layers
with stable public names: `run`, `try`, and `work`. Each higher layer composes
the layer beneath it. There are no other run kinds beyond those named layers.

| Layer | CLI | Inputs | Outputs / side effects | Owns |
|---|---|---|---|---|
| 1 | `ddx run` | prompt + execution config | structured output, side effects, run metadata (tokens, model, duration, exit) | invocation atom; consumes upstream Fizeau execution contract |
| 2 | `ddx try <bead>` | bead id, base revision | new worktree state + run evidence; merge or preserve | worktree start/end capture; bead → prompt resolution; side-effect bundling |
| 3 | `ddx work` | bead queue, stop conditions | sequence of `ddx try` records + loop-level record (drained / blocked / deferred) | mechanical queue drain; no-progress detection |

`ddx try` wraps `ddx run`. `ddx work` iterates `ddx try`. One on-disk
substrate; layer metadata distinguishes records.

Implementation code for this boundary should follow the task-execution
vocabulary (`taskexec` for core layer logic, `fizeauadapter` for the service
bridge) rather than the legacy `internal/agent` package name. Those package
names should make the DDx-owned workflow boundary explicit.

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
- No first-class concept of the task execution layers (`run` / `try` /
  `work`), so consumers must intuit which surface to read
- Stop conditions for queue drain are implicit and not observable
- Generated-artifact provenance has no clean hook into the run record
- Open-ended catalog pressure: every new "run flavor" (benchmark, replay,
  comparison) tempts a new on-disk shape or a new top-level verb

**Desired outcome:** A single on-disk run substrate with explicit layer
metadata, three top-level CLI verbs that map 1:1 onto the layers, and a
narrow read-only HTTP/MCP write surface limited to artifact regeneration.

## Task Execution Layers

### Layer 1 — `ddx run` (invocation atom)

A layer-1 run is one AI invocation. Inputs are a prompt, requested
`MinPower` and optional `MaxPower`, optional operator passthrough constraints,
and non-routing execution config; outputs are the structured response (text or
bytes), any side-effects the service performed via tools, and run metadata
(tokens, model, actual power, duration, exit status, session pointer).

Layer 1 calls the upstream Fizeau execution contract directly. DDx does not
reimplement routing or the invocation loop; it wraps one `Execute` call with
provenance capture.

`ddx artifact regenerate <id>` is layer 1 when the generator returns
bytes/text and DDx writes the file; it is layer 2 when the generator
edits the repo directly (bead-less worktree).

### Layer 2 — `ddx try <bead>` (bead attempt)

A layer-2 run is one bead attempt in an isolated worktree. It owns:

- Worktree creation from a base revision and worktree finalization
  (merge or preserve)
- Worktree cleanup after finalization, failed setup, failed publish, and
  ordinary interruption
- Bead → prompt resolution (description, acceptance, governing artifacts)
- Side-effect bundling (commits, evidence, no-changes rationale)
- One or more layer-1 invocations recorded inside the attempt record

A layer-2 record references its child layer-1 records by run id.

Layer 2 owns bead-attempt success classification. DDx determines success from
the artifacts it owns: commit presence, merge/preserve result, no-changes
rationale, post-run checks, review verdicts, and cooldown policy. The agent's
exit status and actual model/power are inputs to that decision, not the whole
decision.

Layer 2 records raw attempt evidence. Layer 3 applies
[`TD-031`](../../02-design/technical-designs/TD-031-bead-state-machine.md) when
that evidence requires durable bead action such as close-as-already-satisfied,
human triage, blocked, retry cooldown, or stale no_changes metadata cleanup.

### Quality hooks

ADR-023 defines two lifecycle quality hooks owned by the layer-2 and layer-3
execution loop. They are hooks inside `ExecuteBeadLoopRuntime`, not new run
layers and not new bead-schema fields.

`BeadReadinessHook` is the product concept: it runs after the worker has
selected a dependency-ready candidate but before DDx claims the bead or
creates the implementation worktree. The canonical decision being made is bead
readiness assessment, the pre-claim check for tractability and actionability;
lint/rubric scoring is the diagnostic pass inside readiness, and post-attempt
triage is a separate after-evidence action.
The implementation may still call the compatibility entrypoint
`PreClaimIntakeHook` and record `MODE: intake` for legacy compatibility, but
those names are aliases only. Within readiness, the nested bead-lifecycle
workflow skill performs lint/rubric scoring to support the decision, and the
score is diagnostic evidence rather than a separate queue action. Post-attempt
triage is separate and runs only after evidence exists. The hook has enough
context to evaluate title, description, acceptance criteria, labels, parent,
deps, bead type, spec-id, prior attempt history, and whether the bead is
atomic enough to execute. The hook invokes the nested bead-lifecycle workflow
skill under the `ddx` skill tree and records readiness evidence in the
layer-3 run record.

`ddx work` wires this hook by default in both CLI and server-managed worker
paths. Decomposition decisions run with a strong `MinPower` floor, defaulting to
the smart/top-power tier floor when no project-specific splitter override is
configured. DDx does not choose the concrete model; it passes the raised power
floor to Fizeau and preserves any operator-supplied harness/provider/model
passthrough constraints. If those constraints cannot satisfy the strong floor,
DDx records `agent_power_unsatisfied` and blocks instead of running weak
decomposition.

The readiness assessment result is one of:

- `actionable_atomic` — claim and execute normally.
- `actionable_but_rewritten` — DDx applied safe, intent-preserving bead updates
  such as formatting the description into the authoring template, adding
  discovered file:line evidence, adding an obvious test command, or wiring
  deterministic labels/parent/deps. The update is recorded through `ddx bead`
  mutation paths before claim.
- `too_large_decomposed` — DDx created child beads, mapped every parent AC to
  child ACs or an explicit `needs_human` / `non_scope` marker, blocked the
  parent, and did not execute the parent.
- `ambiguous_needs_human` — the bead/spec is unclear, contradictory,
  unverifiable, or missing acceptance criteria that DDx cannot safely invent.
  DDx blocks or marks the bead `needs_human` and does not claim it.
- `readiness_error` — readiness infrastructure failed. In migration/WARN mode
  this fails open with evidence; in reliable factory/BLOCK mode it may skip the
  candidate for the current pass but must not park it behind a cooldown unless a
  retryable time-based class is recorded.

Safe rewrites may clarify existing intent; they must not invent product
behavior, choose between conflicting requirements, change scope, or guess a
missing governing artifact. Those cases become `ambiguous_needs_human`.

`PostAttemptTriageHook` runs after the attempt has produced its local evidence:
agent result, commit/no-commit state, no-changes rationale if any, post-run
checks, adversarial review verdict, merge/preserve result, and the readiness
report.
It uses the same bead-lifecycle skill to classify whether the outcome is a
normal attempt result, a quality-policy failure, a missing-evidence failure, or
an infrastructure failure. This is distinct from bead readiness assessment and
from the lint/rubric scoring used inside readiness. The hook must never rewrite
the attempt result or erase artifacts; it only adds triage evidence and feeds
retry/stop classification.

If post-attempt triage finds that an implementation attempt stopped because the
bead was too large or the worker could not legally decompose inside its
worktree/depth context, the layer-3 worker must invoke the same orchestrator
decomposition path used by `BeadReadinessHook`. This is machine-actionable
work: DDx files child beads, records the AC map, and blocks the parent unless
the split is lossy, ambiguous, or at the queue-level decomposition depth cap.
The operator is not required merely because the implementer could not split
from inside the attempted execution.

`ExecuteBeadReport` gains `OutcomeReason` beside the existing `Disrupted`
signal. `Disrupted` remains the mechanical indicator that normal completion was
interrupted. `OutcomeReason` is the stable classification string that explains
why the attempt ended, such as `lint_warn`, `lint_blocked`,
`lint_hook_error_fail_open`, `readiness_ambiguous`, `readiness_decomposed`,
`triage_missing_evidence`, `review_fixable_gap`, `review_spec_gap`,
`review_too_large`, `post_run_check_failed`, or `success`. Layer-3 `ddx work`
records aggregate these reasons when evaluating retry eligibility, no-progress,
blocked, and deferred stop conditions.

The hook evidence lives in the attempt evidence bundle and is surfaced through
the unified run substrate. The originating bead may receive an evidence event
only for explicit operator override (`--force --reason`) or other existing
audit events; lint scores are not copied into `.ddx/beads.jsonl`.

### Layer 3 — `ddx work` (queue drain)

A layer-3 run is one drain of the bead queue. It iterates `ddx try`
across ready beads until a stop condition is met. It owns:

- Queue iteration order
- Pre-claim readiness, safe bead improvement, decomposition, claim acquisition,
  claim release, and shutdown/interruption cleanup for claimed beads, using the
  TD-031 claim-state contract
- Durable bead action after each layer-2 attempt, using TD-031's outcome,
  no_changes, cooldown, and stale-metadata rules
- No-progress / stop-condition evaluation
- A loop-level record that references its child layer-2 records by
  attempt id and reports terminal disposition (drained, blocked,
  deferred, no-progress)

Content-aware supervisory decisions (e.g., "comparison failed → enqueue
reconciliation beads") are not layer-3 concerns. Those are skill or
plugin compositions on top of the task execution layers.

Layer 3 treats raw attempt evidence separately from durable bead action. When a
worker stops, receives SIGTERM/SIGINT, loses its child agent process, or honors
operator cancel before a terminal bead mutation, it follows TD-031: preserve
evidence, record the interruption/disruption, release the claim, and leave the
bead re-claimable unless an explicit retryable cooldown or blocker was recorded.
Shutdown is not itself a reason to park a bead on `execute-loop-retry-after`.

## Agent Power and Retry

DDx owns retry policy between bead attempts. If an attempt produces classified
evidence that more capable reasoning could plausibly change the result, DDx may
retry the bead with a higher `MinPower`.

Fizeau owns routing within the requested power bounds. A retry request says
"use at least this much power" by raising `MinPower`; an explicit `MaxPower`, if
supplied, remains a hard upper bound. Operator-supplied `--harness`,
`--provider`, and `--model` values may be sent to Fizeau as passthrough
constraints, but DDx does not interpret them. Each layer-1 record stores:

- requested `MinPower` and optional `MaxPower`
- requested passthrough constraints, if any
- actual model and actual power reported by Fizeau
- run outcome and DDx attempt outcome

DDx may query Fizeau's available model/power catalog to choose escalation
targets such as "retry using only top-power models." DDx uses the returned power
numbers only to compute `MinPower` thresholds; Fizeau still chooses the
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
- `review_fixable_gap` — adversarial review found implementation or test gaps
  plausibly addressable by another automated cycle.
- `no_changes_after_attempt` — the agent had a valid checkout and task context
  but produced no usable change, subject to the no-progress budget.

Power retry is not eligible for deterministic setup or operator-action
failures:

- dirty worktree, merge/land conflict, missing checkout, invalid bead metadata,
  unresolved dependencies, config parse errors, missing harness binaries,
  authentication failures, command-not-found/toolchain setup failures
- readiness ambiguity, missing acceptance criteria, spec contradictions,
  decomposition overflow, review errors, reviewer context overflow, claim races,
  routing preflight rejection, quota/transport disruption, and any other class
  where the implementer did not receive valid task context and an opportunity to
  act
- `blocked_by_passthrough_constraint` / `agent_power_unsatisfied`
- any status where retry would require DDx to inspect, remove, rewrite, or
  substitute `--harness`, `--provider`, or `--model`

Every retry/stop decision records the classification and evidence used. The
policy may inspect DDx attempt outcomes and the agent's typed status; it must
not branch on concrete provider/model identity.

ADR-024 is the decision record for this policy. FEAT-010 owns when `ddx try` or
`ddx work` schedules a retry, how no-progress and stop conditions are recorded,
how review-error retry scopes are counted, and how budget caps stop a drain.
FEAT-006 owns only the agent request envelope used by those decisions.

### Execution Cycle Traceability

A bead execution is an append-only chain of implementation/review cycles. Layer
3 owns the chain; layer 2 owns the evidence for each cycle. A repair attempt
must create a new cycle record rather than overwrite the rejected one.

Each cycle records:

- `cycle_index`
- `attempt_id`
- `base_rev`
- `result_rev` when a stable candidate exists
- implementer layer-1 run ids, requested power bounds, actual route/power, and
  cost/usage facts
- verification command output and exit status
- `review_group_id`, reviewer layer-1 run ids, reviewer requested/actual
  route facts, aggregate verdict, per-AC evidence summary, and reviewer cost
- prior review linkage (`repair_context_from_review_group`) when this cycle is
  a repair attempt
- final cycle decision: close, retry, decompose, block, manual-required, or
  budget stop

When a review requires another implementation cycle, the next implementation
prompt includes the prior review group's findings as required repair context.
The prior cycle remains immutable so operators can answer what changed, why the
first result was rejected, and why the final result passed.

### Escalation, fallback, retry, and review decision tree

The layer-3 drain evaluates each ready bead through this mechanical sequence:

1. **Eligibility and readiness.** Pick a dependency-ready candidate. Run the
   readiness gate. Safe rewrites happen before claim. Too-large work
   is decomposed before an implementation attempt. Ambiguous or underspecified
   work is blocked with `needs_human`. Readiness infrastructure failure records
   evidence and follows the configured fail-open/factory-mode policy; it never
   creates an unexplained cooldown.
2. **Claim.** Claim only an `actionable_atomic` or safely rewritten bead. Claim
   races skip the bead for the current pass without cooldown.
3. **Primary implementation cycle.** Run one layer-2 implementation attempt.
   A candidate result records `base_rev`, `result_rev`, implementation run ids,
   route/power facts, verification output, and cost. Already-satisfied
   no-changes may close only after the required verification evidence exists;
   otherwise no-changes outcomes follow TD-031.
4. **Adversarial pre-close review.** Review is enabled by default. For every
   close-eligible `result_rev`, layer 2 dispatches two no-tool reviewer runs
   with `role=reviewer`, a stronger `MinPower` floor than the implementer, and
   correlation metadata (`review_group_id`, `result_rev`, reviewer slot, and
   implementer route facts). Close is permitted only when the aggregate review
   is unanimous `APPROVE` with per-AC evidence.
5. **Review classification.** Any evidenced `REQUEST_CHANGES` or `BLOCK`
   prevents close. `review_fixable_gap` schedules a repair cycle on the same
   bead when retry budgets allow, injecting the review findings as required
   repair context and optionally raising `MinPower`. `review_spec_gap`,
   `review_missing_acceptance`, `review_too_large`, and non-mechanical unsafe or
   out-of-scope findings block or decompose instead of asking another
   implementer to guess. Malformed, empty, context-overflow, and transport
   reviewer failures emit `review-error` scoped to `result_rev` and reviewer
   slot; after `review_max_retries` they emit `review-manual-required`, block
   with `needs_human`, and do not close.
6. **Infrastructure fallback.** Transport, quota, rate-limit, command setup,
   context cancellation, routing preflight rejection, and worker disruption are
   not model-capability failures. They emit structured evidence and either stay
   immediately retryable or use a bounded retry-after when the same class is
   time-based. HTTP 429 retry happens inside one attempt with `rate-limit-retry`
   events.
7. **Capability retry.** Failed checks, `review_fixable_gap`,
   capability-insufficient attempts, and eligible no-changes-after-attempt may
   schedule a higher-power retry by raising the next request's `MinPower`.
   Passthrough constraints remain unchanged. If the requested power cannot be
   satisfied under those constraints, DDx records a terminal operator-action
   classification and stops retrying that bead.
8. **Cost stop.** Implementation and review attempts both contribute reported
   billable cost to the drain budget according to FEAT-014 cost-class metadata.
   When the cap trips, the drain records a budget stop and stops claiming new
   work; the cap is not reported as model failure.

## Substrate Unification

### One record shape

All task execution layers persist to a single on-disk shape under
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
4. **`no_progress`** — `N` consecutive valid implementation attempts produced
   no commit and no merged side-effect on the base branch (default `N = 3`;
   configurable). A valid implementation attempt is one where DDx created a
   checkout, supplied task context to the implementer, and the implementer had
   an opportunity to act. Intake blocks, decomposition, claim races, routing
   preflight rejection, quota/transport/auth/tool setup failures, review errors,
   and operator-action classes do not increment this counter.
5. **`signal`** — a SIGINT/SIGTERM was received. On the first signal, DDx
   prints `Cancel received, shutting down gracefully`, cancels the active
   command context, stops claiming new beads, and gives the in-flight
   `ddx try` (if any) one cooperative cleanup path to preserve evidence,
   release its claim, and leave the bead re-claimable. A second signal may
   hard-abort the process.
6. **`resource_exhausted`** — DDx cannot safely create or publish execution
   state because a required execution root is out of bytes, out of inodes, not
   writable, or cannot register/remove git worktrees. Resource exhaustion is
   host infrastructure failure, not bead failure and not model no-progress.
   `ddx work` stops claiming new beads after recording the failed `ddx try`
   setup evidence and a cleanup summary. It must not continue to the next
   ready bead in the same drain.

Stop-condition evaluation normally runs **between** `ddx try` invocations.
`signal` is the exception: the first signal cancels an in-flight attempt
cooperatively rather than waiting for the model to finish. The cancelled
attempt is mechanically disrupted, not a model no-progress failure, and ordinary
interrupt shutdown MUST NOT park the bead behind `execute-loop-retry-after`.

### Execution Resource Cleanup

DDx-created execution resources must have an owner, a liveness signal, a
retention policy, and a cleanup path. Missing ownership, missing liveness, or
unbounded accumulation is a correctness bug in the execution substrate.

Layer 2 owns inline cleanup for one `ddx try` attempt. Before claim or worktree
creation it validates the required execution roots:

- the temporary worktree root, currently `$TMPDIR/ddx-exec-wt` unless
  overridden
- the durable evidence root, currently `.ddx/executions` during migration and
  `.ddx/runs` for new substrate records
- git worktree registration/removal for the project repository

The validation checks writability, free bytes, and free inodes where the
platform exposes inode counts. If validation fails, `ddx try` runs one
immediate DDx-scoped cleanup pass, re-checks, and then either proceeds or
returns `resource_exhausted` without claiming the bead.

After an attempt starts, Layer 2 removes the isolated worktree when the result
has been merged, explicitly preserved, classified as no-changes/no-evidence, or
interrupted through the cooperative shutdown path. Failed setup must remove any
partial unregistered directory it created. A worktree may remain only when DDx
records an explicit preserve decision with evidence pointing at the retained
path or ref.

Layer 3 owns loop cleanup. `ddx work` runs cleanup:

- once at startup, before the first queue claim
- before the next claim after any setup/finalization failure
- periodically while a long-lived poll worker remains active
- during graceful signal shutdown before exit

Long-lived DDx processes also start a background cleanup worker. The background
worker runs occasionally with jitter and a project-level cleanup lock so multiple
workers do not all prune at once. It is conservative and DDx-scoped: it may
remove stale unregistered directories under DDx temp roots, registered DDx
worktrees whose attempt is terminal or whose liveness marker is stale, stale
heartbeat/liveness files for dead PIDs, and partial setup directories that were
never published as complete evidence. It must not remove preserved worktrees,
`refs/ddx/iterations/...`, complete `.ddx/runs/<id>` or
`.ddx/executions/<attempt-id>` evidence, active worktrees with live
PID/session liveness, or non-DDx directories.

Cleanup is observable but not noisy. Routine passes are trace/debug or worker
events. Passes that reclaim significant bytes or inodes emit an operator-visible
summary such as `cleanup: removed 37 stale ddx worktrees, freed 14210 inodes`.
Resource exhaustion after cleanup is a hard visible stop message and a layer-3
`resource_exhausted` disposition.

### Long-running default (`--poll-interval`)

Per ddx-dc157075, the default `--poll-interval` for `ddx work` is **30s**.
With a positive poll interval the
worker stays alive across empty polls (`drained` becomes a transient
"running (idle)" substate, not a terminal exit). The loop exits only on
the conditions enumerated above plus `signal`, fatal config errors, or
the explicit operator opt-outs:

- `--once` — process at most one ready bead, then exit.
- `--poll-interval=0` — legacy "drain-and-exit" semantics: when the
  queue empties, return immediately without polling.

Server-managed workers spawned via `POST /api/workers/execute-loop`
inherit the same 30s default when the request omits `poll_interval`. The
worker record exposes a `substate` field set to `"idle"` while the loop
is sleeping between empty polls; it is cleared as soon as a candidate is
picked. Terminal worker states (`exited`, `failed`, `stopped`, `reaped`)
always clear `substate`.

The structured `loop.end` event carries an `exit_reason` field
(`once_complete`, `explicit_poll_zero`, `sigint`, `sigterm`,
`fatal_config`, `resource_exhausted`; `providers_exhausted` is reserved for the
quota-pause work in ddx-aede917d).

Power escalation is evaluated between attempts as part of retry policy. A
higher-power retry raises `MinPower` and resets neither the evidence history nor
the no-progress counter; it is recorded as the next `ddx try` with its own
requested bounds and actual power metadata.

The evaluation log is persisted on the layer-3 record so a human or
tool can audit which condition fired and on which iteration.

## Quality Hooks

ADR-023 (`../../02-design/adr/ADR-023-bead-lifecycle-quality-policy.md`)
adds two quality hooks to the layer-2/layer-3 lifecycle. The hooks are
implemented in `ExecuteBeadLoopRuntime` at the same boundary that already
owns bead selection, attempt finalization, and retry classification.

`BeadReadinessHook` runs after a bead has been selected and verified as
dependency-eligible, but before DDx claims it or creates the implementation
worktree. It performs the canonical bead readiness assessment, the pre-claim
decision about tractability and actionability; lint/rubric scoring is the
diagnostic pass inside readiness, and post-attempt triage is a separate
after-evidence action. The legacy compatibility name `PreClaimIntakeHook` and
`MODE: intake` may still appear in migration code or notes, but they are not
the product concept.

The hook receives the bead record, current execution policy, hook mode
(`WARN-ONLY` or `BLOCK`), and the layer-3 evidence directory. It invokes the
bead-lifecycle workflow skill from FEAT-011 and writes a bead readiness,
scope, and decomposition report. In WARN-ONLY mode, the report is diagnostic
unless the hook can safely improve or decompose the bead. In BLOCK/factory
mode, a valid low readiness score, unsafe ambiguity, or too-large
classification stops implementation before claim.

`PostAttemptTriageHook` runs after the attempt has produced its owned
evidence: commits or no-changes rationale, command results, review verdicts,
merge/preserve result, and any structural validation result. It invokes the
same bead-lifecycle workflow skill against the attempt bundle and returns an
attempt classification such as prompt-quality issue, task-quality issue,
infrastructure failure, deterministic setup failure, or operator action
required. This classification feeds retry reporting and operator UX; it does
not replace the existing success/no-changes/failed outcome taxonomy.

Both hooks are fail-open for infrastructure failures. If the skill package is
missing, the hook process crashes, evidence cannot be written, or the model
invocation fails, DDx records the hook error and continues with the underlying
attempt flow in WARN-ONLY mode. In BLOCK/factory mode, hook infrastructure
failure may skip the current candidate for the pass with observable evidence,
but it must not park the bead behind an unexplained cooldown. Only a
successfully computed readiness/lint result can block dispatch as an authoring
or scope failure. `--force --reason <text>` records an event with the actor,
reason, hook mode, and overridden criteria before proceeding.

Long-running entrypoints perform a lightweight DDx runtime preflight before
starting work. This is not a full `ddx doctor` run: it avoids network checks,
package-manager checks, route probing, stale-document scans, and other broad
diagnostics. The preflight is limited to conditions that can predictably degrade
the command about to run:

- project root resolution;
- `.ddx/config.yaml` parseability;
- `.ddx/beads.jsonl` parseability for bead-execution commands;
- the root `ddx` project skill under `.agents/skills/ddx/SKILL.md` or
  `.claude/skills/ddx/SKILL.md`;
- the nested `bead-lifecycle` skill when readiness/lint/triage hooks are wired;
- legacy project-local skill symlinks from the pre-FEAT-015 install model.

Passing preflight checks are silent. Failed checks are observable and
actionable. `ddx work`, `ddx try`, and compatibility `ddx agent execute-loop`
fail on missing required project/tracker state, but warn once per process and
continue for optional lifecycle-hook degradation in WARN-ONLY mode. `ddx server`
fails on missing required server project state, but missing lifecycle skills or
legacy skill layout are surfaced as degraded startup diagnostics and health
metadata rather than blocking the HTTP server. When the lifecycle skill is
missing, both startup preflight and hook-time errors must include the project
root, the exact checked paths, and remediation:

```text
Run:
  ddx update --force
  ddx doctor
```

`ExecuteBeadReport` gains an `OutcomeReason` field beside the existing
`Disrupted` flag. `Disrupted` remains the coarse boolean that the attempt did
not complete normally. `OutcomeReason` is the machine-readable reason selected
by lifecycle classification, for example:

- `bead_lint_warn`
- `bead_lint_blocked`
- `readiness_actionable_atomic`
- `readiness_rewritten`
- `readiness_decomposed`
- `readiness_needs_human`
- `forced_with_reason`
- `triage_prompt_quality`
- `triage_task_quality`
- `triage_infrastructure_failure`
- `triage_operator_action`
- `hook_unavailable_fail_open`

Layer-3 `ddx work` records include the hook summaries for each child layer-2
attempt in the stop-condition evaluation log. A blocked lint result counts as a
terminal non-success outcome for that bead under the current policy, so the
existing `blocked`, `deferred`, and `no_progress` loop rules can reason about
it without a fourth run layer or a separate run type.

## No Run-Type Catalog

DDx will not introduce additional run kinds beyond `run`, `try`, and `work`.
Comparison, replay, benchmark, adversarial review, effort estimate,
bead breakdown, and similar workflows are **skill compositions** over
the task execution layers — they emit ordinary layer-1, layer-2, or layer-3
records, optionally tagged with a free-form `skill` label, and do not get
bespoke storage shapes, bespoke verbs, or bespoke spec sections.

This is a load-bearing non-goal: every future "we should add a fourth
layer for X" proposal is rejected by reference to this section. If a
new workflow cannot be expressed as a composition over `run` / `try` /
`work`, the gap is in the task execution contract, not in the catalog.

## Requirements

### Functional

1. **One on-disk substrate** — all run records, regardless of layer,
   persist under `.ddx/runs/<run-id>/` with the common record shape and
   a discriminating `layer` field.
2. **Layer metadata** — every run record carries `layer`,
   `parent_run_id`, and the layer-appropriate extension subobject.
3. **Layer-1 invocation** — `ddx run` produces exactly one layer-1
   record per invocation; consumes upstream Fizeau execution contract
   per CONTRACT-003.
4. **Layer-2 attempt** — `ddx try <bead>` produces exactly one layer-2
   record, references its child layer-1 records, and finalizes the
   worktree as `merge` or `preserve`.
5. **Layer-3 drain** — `ddx work` produces exactly one layer-3 record
   per invocation, references its child layer-2 attempt ids, and reports
   one of the terminal dispositions (`drained`, `blocked`, `deferred`,
   `no_progress`, `signal`, `resource_exhausted`).
6. **Stop conditions** — `ddx work` evaluates the stop conditions
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
14. **Execution cleanup** — `ddx try` and `ddx work` remove stale DDx-owned
    execution worktrees and liveness files through inline, loop, and background
    cleanup paths without deleting preserved attempts or published evidence.
15. **Resource preflight** — `ddx try` validates writable execution roots and
    free bytes/inodes before claim; failed validation may trigger one cleanup
    retry and then returns `resource_exhausted` without claiming.

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
ddx run --prompt <file> [--harness <name>] [--provider <name>] [--model <name>]  # layer 1
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
| (GraphQL) | `Mutation.runRequeue` | Re-queue the originating bead of an existing run record (manual operator re-queue from the Runs UI); requires `idempotencyKey`; emits a canonical `run_requeue` audit event on the bead (schema below) | **write — narrow** |

The HTTP/MCP read surface is total over the unified run substrate.

The write surfaces added by this feature are limited to
`artifactRegenerate` and `runRequeue`. All other run invocation
(layer-1 ad-hoc, layer-2 bead attempts, layer-3 queue drain) remains
CLI-only. Additional write endpoints require a separate feature update;
they are not implicit in this read-coverage expansion.

### Re-queue audit event schema

The `runRequeue` mutation reopens the originating bead and appends a
`run_requeue` event to that bead's audit log. Concurrent or repeat
submissions with the same `idempotencyKey` collapse to a single
re-queue and a single event (`deduplicated=true` is returned to all
subsequent callers); if the cached idempotency record points at a
missing bead, the requeue is replayed against the run's current
originating bead. The event uses the standard `bead.BeadEvent`
envelope and the following field schema:

| Field | Value |
|---|---|
| `kind` | `run_requeue` (`RunRequeueEventKind`) |
| `summary` | `run requeued` |
| `actor` | Resolved operator identity; falls back to `anonymous` when no request identity is available |
| `source` | `graphql:runRequeue` |
| `body` | Single line `key=value` payload: `identity=<identity.kind> actor=<identity.actor> run_id=<runId> idempotency_key=<key> layer_override=<layer-or-empty>` |

The body line is structured for grep/jq parsing: each token is a
`key=value` pair separated by single spaces. `layer_override` is the
empty string when the caller did not pass `RunRequeueInput.layer`.
`identity.kind` is the resolver-selected operator identity class
(`unknown`, `localhost`, or `tsnet`). `actor` is the resolved user or
operator label; when the request carries no identifying header, the
audit fields fall back to `identity=unknown actor=anonymous`.

The GraphQL mutation response may also report `deduplicated=true`, but
that is a write-response flag, not part of the persisted audit event.

Consumers of the audit log (FEAT-008 Runs row expansion, evaluation
skills under FEAT-019) treat `run_requeue` events as the canonical
record that a re-queue happened: there is no separate persisted
`requeue` record beyond (a) the new `open` status on the originating
bead and (b) this event entry.

### Run-detail access audit events

The first time the canonical project-scoped run detail is resolved for a
run record that is visible through project membership, DDx appends a
`run_detail_view` event to the originating bead's audit log. This is the
canonical record that a human inspected the run-detail surface.

The event is read-side only: it is emitted on the canonical detail load,
not on in-page tab switches, evidence downloads, or breadcrumb/back
navigation. If the caller cannot see the project, there is no run detail
and therefore no audit event.

The `body` line is structured for grep/jq parsing and includes the
project-scoped identity of the inspection:

| Field | Value |
|---|---|
| `kind` | `run_detail_view` |
| `summary` | `run detail viewed` |
| `actor` | Viewer identity derived from the authenticated project member |
| `source` | `graphql:run` |
| `body` | Single line: `project_id=<projectId> run_id=<runId> layer=<layer> visibility=project_membership` |

### Layer-to-substrate mapping for the Runs UI

The web Runs view (FEAT-008 §5, FEAT-021) renders three layer chips
backed by the unified substrate plus the legacy detail backings:

| Chip | Substrate row | Detail backing for row expansion |
|---|---|---|
| `work` | `layer: 3` Run record | Layer-3 record's queue inputs / stop-condition log / child layer-2 attempt ids; the shared tab strip exposes `overview` only |
| `try` | `layer: 2` Run record | The `.ddx/executions/<attempt-id>/` bundle attached to the layer-2 record (manifest, prompt, result, checks, verdict); the shared tab strip exposes `overview`, `prompt`, `response`, `tools`, and `evidence` |
| `run` | `layer: 1` Run record | The associated `AgentSession` row (prompt / response / stderr / billing / cached-token detail) joined onto the layer-1 record; the shared tab strip exposes `overview`, `prompt`, `response`, `session`, `tools`, and `evidence` |

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
- Given I press Ctrl-C while `ddx work` or `ddx try` is running, then DDx
  prints `Cancel received, shutting down gracefully` before cleanup begins.
- Given the loop stops, then the layer-3 record's terminal disposition
  is one of `drained`, `blocked`, `deferred`, `no_progress`, `signal`,
  and the evaluation log names which condition fired and on which
  iteration.

### US-093: No-Progress Detection Stops a Spinning Loop

**As a** developer running `ddx work`
**I want** the loop to stop when consecutive attempts make no progress
**So that** I do not burn budget on a pathological queue state

**Acceptance Criteria:**
- Given `N` consecutive valid implementation attempts produce no commit and no
  merged side-effect (default `N = 3`, configurable), then the next
  stop-condition evaluation triggers `no_progress` and the layer-3 record
  terminates with that disposition.
- Given a candidate is skipped by readiness, claim race, routing preflight,
  quota/transport/auth/tool setup failure, review error, or operator-action
  class, then that iteration does not increment the no-progress counter.
- Given retry policy permits escalation before `no_progress` fires, when
  a retry is scheduled, then the next layer-1 run raises `MinPower`
  and records the actual model/power returned by the agent.
- Given the original invocation supplied `--harness`, `--provider`, or
  `--model`, then retries pass the same values through unchanged and DDx does
  not inspect those values when choosing the next requested `MinPower`.
- Given retry escalation would exceed the power available under hard
  passthrough pins, then DDx stops with `blocked_by_passthrough_constraint` or
  `agent_power_unsatisfied`, records the Fizeau-supplied evidence, and does not
  mutate the pins or call `ResolveRoute`.

### US-094: Cross-Layer Run History

**As a** workflow tool
**I want** to query run history across the task execution layers from one place
**So that** I do not have to merge two on-disk shapes by hand

**Acceptance Criteria:**
- Given `run`, `try`, and `work` records exist, when a tool calls
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
`runRequeue` (GraphQL). `runRequeue` does not invoke the execution service —
it reopens the originating bead so the existing `ddx work` loop can claim it
again — but it is still a write because it mutates bead state and appends an
audit event (see "Re-queue audit event schema" above).
No other write surfaces are added; in particular, layer-1, layer-2,
and layer-3 invocation remain CLI-only.

If FEAT-006 implementation reveals additional CONTRACT-003 gaps (e.g.,
upstream Cobra root export details), those amendments are scoped to
FEAT-006 and not to this feature.

## Dependencies

- FEAT-005 (Artifacts) — `produces_artifact` references artifact IDs
- FEAT-006 (Fizeau consumer contract) — provides the layer-1 consumer-side wrapper
  and CONTRACT-003 boundary
- FEAT-007 (Doc Graph) — `generated_by` edges read `produces_artifact`
  from run records
- DDx CLI infrastructure (config loading, command factory)

## Out of Scope

- **Run kinds beyond `run`, `try`, and `work`** — comparison, replay, benchmark,
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
- **Workflow-specific autonomy semantics** — DDx owns the mechanical retry,
  review, cooldown, and budget policy in this feature and ADR-024. Higher-level
  methodology decisions such as "file reconciliation beads after a benchmark"
  remain workflow-tool territory.
- **Hosted run history storage** — records are repo-local and
  file-backed.
- **Separate domain-specific runtime stores** — there is one substrate.
