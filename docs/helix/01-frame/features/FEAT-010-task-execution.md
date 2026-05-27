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
bridge). Treat `internal/agent` as legacy naming only; new DDx-owned execution
work should make the workflow boundary explicit.

`ddx artifact regenerate <id>` is sugar over layer 1 (or layer 2 when the
generator edits the repo) with `produces_artifact: <id>` metadata. It is
not a fourth layer — it is a labeled invocation at an existing layer.

## Agent Interaction Modes

DDx defines four named agent interaction modes (`DDX_MODE`) to separate
broad interactive queue stewardship from worker execution. The mode governs
what an agent opened in a DDx project is permitted to do.

| DDX_MODE | Description | Allowed actions | Prohibited actions |
|---|---|---|---|
| `queue_steward` | Default interactive mode. Surveys, triages, and advises on the queue without claiming or executing beads. | Read tracker and docs; report status; advise on bead quality; suggest readiness fixes; run `ddx bead list/ready/status/show`. | Claiming beads; creating isolated workspaces; starting `ddx work` or `ddx try` without an explicit user directive. |
| `bead_execution` | Worker mode. Executes one or more beads from the queue in isolated workspaces. | Full FEAT-010 layer-2 / layer-3 lifecycle: workspace creation, bead attempt, merge or preserve, evidence bundle. | Scope creep outside the named bead; switching to direct implementation without a bead. |
| `direct_user_implementation` | Human-directed implementation. The user has explicitly asked the agent to edit code or docs, bypassing the bead queue. | Edit code and docs as instructed; commit to the current branch. | Starting autonomous queue drain; claiming beads autonomously. |
| `review` | Read-only review. Grades an existing implementation against AC or specs. | Read code, docs, and evidence; return a structured verdict with per-AC findings. | Writing commits; claiming beads; modifying the tracker. |

### Precedence

`bead_execution` supersedes `queue_steward` when DDx invokes a worker
explicitly (via `ddx work`, `ddx try <id>`, or an internal execute-bead
worker invocation).
`direct_user_implementation` supersedes `queue_steward` when the user
explicitly asks for code or doc edits unrelated to the queue. `review`
supersedes `queue_steward` when the user explicitly requests a graded review
pass.

Tracker instructions, merge-policy, and safety rules in AGENTS.md are
load-bearing in every mode and are never overridden by mode selection.

### Lifecycle Migration Startup Gate

`ddx work`, `ddx try`, server-managed workers, GraphQL worker starts, REST/MCP
worker starts, and queue status/readiness commands must run the TD-027 §18 lifecycle
startup preflight before claiming beads or deriving worker eligibility. When the
active queue still contains unmigrated legacy lifecycle labels or
pseudo-statuses, these surfaces fail closed with a configuration/startup error.
They do not call Fizeau, do not classify the failure as an agent attempt, and do
not add cooldown or retry metadata.

Allowed bypass commands are limited to help/version output, `ddx doctor` or
other read-only diagnostics that do not mutate lifecycle, and the lifecycle
migration command:

```bash
ddx bead migrate --lifecycle --dry-run
ddx bead migrate --lifecycle --apply
```

The error must print counts of legacy labels and pseudo-statuses, a small sample
of affected bead IDs, and the exact migration command. The migration is one-way:
old `needs_human` and `triage:needs-investigation` lanes are not runtime
compatibility modes. If the migration is wrong, the recovery path is git
rollback of the tracker commit.

### Worker-capacity suggestion rules

An agent in `queue_steward` mode may suggest running `ddx work` or point
the operator to worker-capacity settings when:

- The queue contains execution-ready beads that have not been attempted.
- The queue is blocked and operator action is required.
- The operator asks "why isn't anything running?" or a similar capacity question.

The agent must not autonomously start `ddx work` from `queue_steward` mode.
Suggesting the command or explaining the conditions is the correct action.

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

A layer-2 run is one bead attempt in an isolated workspace. It owns:

- Workspace creation from a base revision and workspace finalization
  (merge or preserve)
- Workspace cleanup after finalization, failed setup, failed publish, and
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

Layer 2 records raw attempt evidence. Layer 3 applies the outcome → state
mapping in [`TD-031 §2`](../../02-design/technical-designs/TD-031-bead-state-machine.md)
when that evidence requires durable bead action such as close-as-already-satisfied,
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
paths. Decomposition decisions request Fizeau's `smart` model-ref and run with a
strong `MinPower` floor, defaulting to the smart/top-power class floor when no
project-specific splitter override is configured. DDx does not choose the
concrete model; it passes the raised power floor to Fizeau and preserves any
operator-supplied harness/provider/model passthrough constraints. If those
constraints cannot satisfy the strong floor, DDx records readiness as
unavailable (`readiness_error` / `intake_error`) instead of treating the bead as
ambiguous or in need of human review; the worker then continues through the
normal implementation route with the same operator passthrough constraints.

The readiness assessment result is one of:

- `actionable_atomic` — claim and execute normally.
- `actionable_but_rewritten` — DDx applied a validated replacement rewrite or
  metadata update that makes the bead more execution-ready before claim. Safe
  rewrites may replace noisy, stale, duplicated, or brittle description text
  with a fit-for-purpose bead body instead of appending more prompt burden. They may
  also format the description into the authoring template, add discovered
  file:line evidence, add an obvious test command, or wire deterministic
  labels/parent/deps. The mutation is recorded through `ddx bead` paths before
  claim, with before/after hashes and enough evidence to audit the replacement.
- `too_large_decomposed` — DDx created child, sibling, or replacement beads,
  mapped every parent AC to generated ACs or an explicit operator-required /
  `non_scope` marker, left the oversized bead `status=open` with dependency,
  execution-eligibility, or supersession metadata that lets the queue advance,
  and did not execute the oversized bead directly.
- `ambiguous_requires_operator` — the bead/spec is unclear, contradictory,
  unverifiable, or missing acceptance criteria that DDx cannot safely invent.
  DDx moves the bead to `status=proposed` and does not claim it.
- `readiness_error` / `intake_error` — readiness infrastructure failed. In
  migration/WARN mode this fails open with evidence; in reliable factory/BLOCK
  mode it may skip the candidate for the current pass but must not park it
  behind a cooldown unless a retryable time-based class is recorded.

Safe rewrites are validated against durable context, not against a vague
"same spirit" comparison to the prior bead text. The prior bead can itself be
the problem: it may contain stale line numbers, duplicated history, chat-shaped
noise, or contradictory wording. A safe replacement must preserve explicit
commitments that remain valid, especially acceptance criteria, non-scope, named
files/tests, dependencies, and governing artifact references; it must either
carry forward durable root-cause evidence or replace it with current file:line
evidence from the repository or governing artifact. The original text is kept in
readiness evidence, not copied into the replacement body. Rewrites must optimize
prompt fitness rather than raw length: expanding a one-line underspecified bead
is safe when the added durable context is required for execution, and compressing
a noisy bead is safe when explicit commitments remain preserved.

Readiness must reject the rewrite when preservation cannot be proven from
durable anchors. Rejected rewrite does not by itself authorize operator
attention: DDx next tries safe decomposition or replacement work when that can
preserve explicit scope, and moves the bead to `status=proposed` only when the
remaining ambiguity, missing governing artifact, or scope conflict requires
operator judgment. It must not invent product behavior, choose between
conflicting requirements, change scope, delete unresolved constraints, or guess
a missing governing artifact.

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
work: DDx files child, sibling, or replacement beads, records the AC map, and
leaves the oversized bead open with dependency, execution-eligibility, or
supersession metadata that lets the queue advance. Only lossy or ambiguous
splits require operator attention.
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
  TD-027 §12 claim-state contract
- Durable bead action after each layer-2 attempt, using TD-031 §2 outcome,
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
operator cancel before a terminal bead mutation, it follows TD-027 §12.2: preserve
evidence, record the interruption/disruption, release the claim, and leave the
bead re-claimable unless an explicit retryable cooldown or blocker was recorded.
Shutdown is not itself a reason to park a bead on `work-retry-after`.

#### Idle-Path Diagnosis and Auto-Remediation

When the picker returns no executable bead, Layer 3 does not exit. It diagnoses
each non-ready bead in the breakdown (`Epics`, `EpicClosureCandidates`,
`DepWaiting`, `OperatorAttention`) with a per-bead reason code, fires the
matching auto-remediation in the same loop iteration, then re-scans. The drain
exits only when every blocker class has been resolved or has been routed to an
operator surface with an exhausted-auto-remediation marker.

**Reason taxonomy** (closed enum; new codes require this spec to be updated):

- `superseded_pending_close` — bead has `superseded-by:<Y>`, Y is closed.
  Drives a one-hop cascade close inside `Close(Y)`.
- `closure_candidate_misclassified` — epic with `openChildCount == 0 &&
  totalChildCount > 0` mistakenly in the epic-container bucket. Drives a
  classifier-correction reroute into the existing closure-evaluation path.
- `dead_intermediate_all_children_closed` — `execution-eligible == false`,
  all children closed. Drives the same closure path.
- `dead_intermediate_open_children_pending` — `execution-eligible == false`
  with open children that have their own auto-remediation. Skip; recursion
  via the children resolves the parent.
- `epic_of_epic` — only open children are themselves epics. Skip; the inner
  epic's auto-remediation resolves the chain.
- `dep_blocked_by:<id>(status=<s>)` — bead has open deps. No direct action;
  the cascade of supersession/closure remediations is expected to unblock
  most. Remaining real blocks surface in `ddx work focus` Section A with the
  blocker ID and status cited.
- `genuinely_needs_decomposition` — `IssueType == epic`,
  `totalChildCount == 0`, not closure-pending, no `manual-hold` /
  `no-auto-decompose` / `container` operator override, valid decomposition
  source (PROBLEM/PROPOSED FIX/AC sections per the bead-authoring template,
  plus a `spec:*` or `area:*` label). Drives one dispatch of the pre-claim
  decomposer (`runPreClaimDecomposer`) per loop iteration.
- `parent_child_state_conflict` — parent/child state mismatch the classifier
  does not expect. Surface for operator.
- `claimed_in_progress` — bead currently claimed by an active worker. No
  action; honored by claim semantics.
- `provider_route_unavailable` — last dispatch failed because the selected
  provider route is unavailable. In broad `ddx work` drains this remains
  retryable: record the failed route, keep the bead open, and let the next
  attempt choose a different live route. Targeted runs still surface the
  provider hint.
- `gated_by_budget_or_cooldown` — auto-remediation matched but
  `--max-recovery-cost` exhausted, cooldown active, or `--no-*` override set.
  Surface with the gate name.
- `malformed_parent_or_dep_ref` — parent/dep ID does not resolve. Surface
  for operator (tracker integrity issue).
- `dependency_cycle` — cycle detected in dep DAG. Surface for operator.
- `closed_or_missing_parent` — parent resolves to closed or non-existent
  bead. Surface for operator.
- `stale_graph_index` — child/dep index disagrees with raw JSONL scan.
  Trigger index rebuild; re-diagnose next loop.
- `auto_remediation_exhausted` — attempts cap hit or cooldown active for the
  same reason. Surface for operator.
- `no_diagnosis` — none of the above matched. Log + surface as bug.

**Safety contract (applies to every auto-remediation):**

- Per-bead attempt cap of 3 per reason code, tracked via event count.
  Beyond the cap the diagnosis becomes `auto_remediation_exhausted` and the
  bead surfaces in `ddx work focus` Section A.
- Cost budget: `--max-recovery-cost` (default $2). Auto-decompose counts
  against it; cheap pure-Go remediations (RC-1 reroute, RC-2 cascade, RC-3
  cleanup) do not.
- Cooldown: failed auto-decompose calls `SetExecutionCooldown` for 15m to
  prevent the same epic from being repicked next scan.
- Cycle guard: per-loop-iteration visited set for any recursive operation
  (RC-2 cascade, RC-3 walk-up, RC-4 epic-of-epic recursion).
- Operator overrides: per-handler kill switches `--no-auto-supersede-close`,
  `--no-auto-epic-decompose`, `--no-auto-closure-reclassify`; per-project
  defaults under config key `work.autoRemediations`.
- Audit: every auto-action emits an event with the reason code, the bead
  ID, actor `auto-remediator`, and (for cascade ops) the trigger ID.
- Re-scan policy: if any auto-remediation succeeded in the pass, the loop
  re-runs `nextCandidate()` without sleeping. Otherwise it falls through to
  the existing `sleepOrWake` idle path.
- Throughput: at most one auto-decompose dispatch per loop iteration to bound
  cost; cheap remediations batch in a single pass.
- Read-only paths (`ddx work plan --explain`, `ddx work focus`) never mutate;
  only the live `ddx work` loop fires auto-actions.

**Operator surface (`ddx work focus`)** is two sections:

- **Section A — Operator-required:** beads with diagnoses that have no
  automated remediation or have exhausted attempts.
- **Section B — Auto-remediating next loop:** one-line count of beads with an
  auto-action queued, with `--verbose` opening the per-bead detail. Silent
  hiding of auto-remediable beads is forbidden — operators must always be able
  to see what the loop is about to do.

The epic-specific behavior under this contract is documented in FEAT-004
"Queue Semantics For Epics" and inherits all safety guards above.

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
required; it does not remove pins, widen pins, call any route-selection helper,
or retry in a loop.

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

### Candidate-Cycle Pipeline

A **candidate** is the stable commit produced by one implementation pass and
pinned as the subject of the review gate before land. A candidate exists when
the implementation produces at least one commit and post-run checks have been
attempted.

A **candidate ref** is a project-root git ref
(`refs/ddx/iterations/<attempt-id>/<cycle-index>`) that pins the candidate
commit so reviewers can inspect a stable revision even after worktree cleanup.

A **cycle** is one implementation pass plus its associated review gate: one
implementation pass that produces a candidate, the read-only tool reviewer gate
run against that candidate in the still-live worktree, and the resulting
disposition (land, repair, or stop).

A **repair cycle** is a cycle triggered when the prior cycle's review verdict is
`review_fixable_gap`. The repair implementation runs in the same still-live
attempt worktree and is **append-only** — no `git reset`, `git commit --amend`,
`git squash`, or `git rebase` against the prior candidate is permitted. The
repair prompt includes the prior review group's findings as required context.

A **read-only tool reviewer** is a reviewer agent that runs in the still-live
attempt worktree with access to read-only filesystem tools (file reads,
searches) but no write tools. The bounded evidence bundle assembled per FEAT-022
is the canonical review input; same-worktree read-only tool access is
supplemental and does not override the bounded bundle. The reviewer cannot
create commits or modify worktree state.

**Terminal dispositions** for a candidate-cycle attempt:
- `landed` — candidate was approved and merged to the base branch.
- `preserved` — candidate is approved but land was deferred; worktree retained.
- `parked` — automatic repair, review retry, reframe, and decomposition paths
  exhausted without an executable next step; bead moved to `status=proposed`.
- `conflicted` — approved candidate failed to land due to merge conflict; bead
  returned to `open` for re-attempt with a fresh `base_rev`.
- `budget-stopped` — drain-level cost or no-progress budget tripped; workspace
  preserved with evidence.

**Candidate-cycle sequence** (in the still-live attempt workspace, before land):

1. Create isolated workspace from `base_rev`.
2. Run implementation pass; collect `base_rev`, `result_rev`, run ids,
   verification output, and cost facts.
3. Pin candidate ref; emit `candidate-pinned`.
4. Run candidate checks (post-run tests, lint, gates). Failure emits
   `candidate-checks-failed`; plausibly capability-sensitive failures are
   eligible for a repair cycle when `repair_max_cycles` allows.
5. Dispatch read-only tool reviewers in the still-live workspace, **before land**.
   Record `review_group_id`, reviewer run ids, per-AC verdict, and cost.
6. On unanimous `APPROVE`: land, emit `final-result-landed`, close bead, clean up.
7. On `REQUEST_CHANGES` / `BLOCK` classified `review_fixable_gap` within
   `repair_max_cycles`: emit `repair-cycle-started`, run append-only repair in
   same workspace, return to step 3.
8. On `repair_max_cycles` exhausted: emit `repair-cycle-exhausted`, preserve
   workspace, and return the bead to the TD-031 auto-recovery path. Move to
   `status=proposed` only after automatic reframe/decompose/replacement cannot
   produce executable follow-up work or the finding requires operator judgment.
9. On approved candidate that fails to land (merge conflict): emit
   `approved-land-conflict`, release claim, return bead to `open`.

**Budget axes** (each is distinct from cost, rate-limit/provider, and
no-progress budgets):

- **`repair_max_cycles`** — maximum repair cycles per attempt. Exhaustion fires
  `repair-cycle-exhausted` and enters the TD-031 auto-recovery path; it moves
  the bead to `status=proposed` only after automatic recovery fails or requires
  operator judgment. It does not consume the no-progress counter.
- **`review_max_retries_per_candidate`** — maximum reviewer retry attempts for a
  single candidate ref when reviewer invocations fail. Exhaustion returns to
  automatic recovery unless the error class proves operator action is required;
  only operator-required exhaustion fires `review-manual-required`. A new
  candidate ref (next repair cycle or fresh `ddx try`) resets this counter
  independently; it is strictly per-candidate.

These axes do not interact with the drain-level cost cap (FEAT-014), the
rate-limit retry budget internal to a single attempt, or the no-progress counter
tracking consecutive attempts with no commit.

**Transitional single-slot allowance.** Until the two-slot quorum reviewer bead
lands, a single reviewer slot satisfies the review gate. The removal condition
is the landing of the bead that implements two-slot quorum aggregation.

### Escalation, fallback, retry, and review decision tree

The layer-3 drain evaluates each ready bead through this mechanical sequence:

1. **Eligibility and readiness.** Pick a dependency-ready candidate. Run the
   readiness gate. Validated replacement rewrites happen before claim and should
   leave a clearer implementation prompt, expanding underspecified beads or
   compressing noisy/stale beads as the durable context requires; original text
   is preserved in readiness evidence for audit. Too-large work is decomposed
   before an implementation attempt, using sibling or replacement decomposition
   when child depth is exhausted. Ambiguous or underspecified work moves to
   `status=proposed` only when the gate reaches a hard operator-required
   condition after safe rewrite/decomposition options are exhausted; otherwise
   WARN-ONLY findings proceed on the open forward-progress lane. Readiness
   infrastructure failure records evidence and follows the
   configured fail-open/factory-mode policy; it never creates an unexplained
   cooldown.
2. **Claim.** Claim only an `actionable_atomic` or safely rewritten bead. Claim
   races skip the bead for the current pass without cooldown.
3. **Primary implementation cycle.** Run one layer-2 implementation attempt.
   A candidate result records `base_rev`, `result_rev`, implementation run ids,
   route/power facts, verification output, and cost. Already-satisfied
   no-changes may close only after the required verification evidence exists;
   otherwise no-changes outcomes follow TD-031 §6.1 NoChangesContract.
4. **Adversarial pre-close review.** Review is enabled by default. For every
   close-eligible `result_rev`, layer 2 pins a candidate ref and dispatches two
   read-only tool reviewer runs **in the still-live attempt worktree, before
   land**, each carrying `role=reviewer`, a stronger `MinPower` floor than the
   implementer, and correlation metadata (`review_group_id`, `result_rev`,
   reviewer slot, and implementer route facts). The bounded evidence bundle is
   the canonical review input; same-worktree read-only tool access is
   supplemental. Close is permitted only when the aggregate review is unanimous
   `APPROVE` with per-AC evidence.
5. **Review classification.** Any evidenced `REQUEST_CHANGES` or `BLOCK`
   prevents close. `review_fixable_gap` schedules a repair cycle on the same
   bead when `repair_max_cycles` allows, injecting the review findings as
   required repair context and optionally raising `MinPower`; it preserves the
   retry path and does not move the bead to `status=proposed`.
   `review_spec_gap`, `review_missing_acceptance`, `review_too_large`, and
   non-mechanical unsafe or out-of-scope findings enter the TD-031
   decomposition-first path: safe reframe, child decomposition, and
   sibling/replacement decomposition are attempted when they can preserve
   explicit scope. DDx moves the bead to `status=proposed` only when those paths
   would be lossy or require operator judgment, and it does not ask another
   implementer to guess. Malformed, empty, context-overflow, and transport
   reviewer failures emit `review-error` scoped to `result_rev` and reviewer
   slot; after `review_max_retries_per_candidate` they return to automatic
   recovery unless the error class proves operator action is required. Only
   operator-required review errors emit `review-manual-required`, clear the
   active claim, move the bead to `status=proposed`, and prevent close. The
   `review_fixable_gap` path stays on the automatic retry track instead of
   converting into operator review.
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
  upstream session id, structured response pointer, normalized
  `ToolCallEntry` stream captured at drain time
- `layer2` — bead id, base revision, workspace path, finalization mode
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

The lifecycle-state migration is separate from this run-substrate migration.
TD-027 §18 owns that one-way queue migration contract. Unlike legacy run-record
readers, normal task execution does not read through old lifecycle labels after
the startup gate is enabled.

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
interrupt shutdown MUST NOT park the bead behind `work-retry-after`.

### Execution Resource Cleanup

DDx-created execution resources must have an owner, a liveness signal, a
retention policy, and a cleanup path. Missing ownership, missing liveness, or
unbounded accumulation is a correctness bug in the execution substrate.

Layer 2 owns inline cleanup for one `ddx try` attempt. Before claim or
workspace creation it validates the required execution roots:

- the temporary execution root, resolved in this order: `DDX_EXEC_WT_DIR`,
  `executions.temp_worktree_root`, then the per-user cache root
  (`$XDG_CACHE_HOME/ddx/exec-wt` on Linux, or the platform cache equivalent)
- the durable evidence root, currently `.ddx/executions` during migration and
  `.ddx/runs` for new substrate records
- workspace backend setup/teardown for the project repository

The validation checks writability, free bytes, and free inodes where the
platform exposes inode counts. If validation fails, `ddx try` runs one
immediate DDx-scoped cleanup pass, re-checks, and then either proceeds or
returns `resource_exhausted` without claiming the bead.

The attempt backend is selected by `--attempt-backend` or
`executions.attempt_backend`. `worktree` is the default linked git worktree
backend. `local-clone` creates a full local clone under the same execution root
and imports result commits back into the project repo before land. `docker-clone`
uses that clone as the Docker working tree; it requires
`executions.docker.image` and can set Docker memory, CPU, pids, tmpfs, and
network limits under `executions.docker`. `in-tree` (opt-in) runs the attempt
directly in the project checkout; it requires clean working tree before claim,
enforces exclusive single-worker locking, and is designed for data-heavy
validation tasks that need access to project-local state (untracked datasets,
running services, checked-out fixtures). On failure with dirty tree, the bead is
unclaimed and a rescue message is left for manual recovery.

`ddx work` and server-managed workers use the same cleanup manager before the
first claim and before later claims whenever any checked temp or evidence root
falls below the soft cleanup trigger of **512 MiB free bytes** or **8192 free
inodes**. If cleanup does not restore all roots above the hard stop floor of
**64 MiB free bytes** and **1024 free inodes**, the loop stops visibly with
`resource_exhausted` and claims no more beads.

After an attempt starts, Layer 2 removes the isolated workspace when the result
has been merged, explicitly preserved, classified as no-changes/no-evidence, or
interrupted through the cooperative shutdown path. Failed setup must remove any
partial unregistered directory it created. A workspace may remain only when DDx
records an explicit preserve decision with evidence pointing at the retained
path or ref.

DDx-owned cleanup scope includes execution workspaces, helper scratch roots
created beside the configured execution root, legacy `$TMPDIR/ddx-exec-wt`
resources, DDx-created test and e2e scratch roots, generated test binaries, and
run-state or liveness files. The cleanup manager may delete only DDx-owned
paths. Recognized DDx-owned scratch prefixes are: `ddx-test-`, `ddx-e2e-`,
`ddx-test-bin-`, `ddx-test-binary-`, `ddx-lifecycle-`,
`ddx-agent-support-keepalive`, `ddx-config-anchor-`, `ddx-exec-keepalive`,
`ddx-metric-keepalive`, `ddx-metaprompt-keepalive`,
`ddx-persona-keepalive`, `ddx-vale-`, `ddx-gate-wt-`,
`ddx-land-finalize-`, `ddx-land-wt-`, `ddx-push-recover-`, and
`ddx-conflict-recover-`. Any temp directory that contains a `cleanup.json`
metadata file with matching project ownership is also DDx-owned regardless of
its name.

Deletion is permitted only when all of the following hold:

- **Metadata-backed paths**: the `cleanup.json` liveness record is expired or
  the owning attempt has reached a terminal state.
- **Metadata-less recognized-prefix paths**: the directory's mtime is at least
  **6 hours** old and no live PID or active session is attached.

The manager must preserve published evidence, active workspaces, and anything
outside DDx-owned roots.

Layer 3 owns loop cleanup. `ddx work` runs cleanup:

- once at startup, before the first queue claim
- before the next claim after any setup/finalization failure
- periodically while a long-lived poll worker remains active
- during graceful signal shutdown before exit

Long-lived DDx processes also start a background cleanup worker. The background
worker runs occasionally with jitter and a project-level cleanup lock so multiple
workers do not all prune at once. It is conservative and DDx-scoped: it may
remove stale unregistered directories under DDx temp roots, registered DDx
worktrees or metadata-backed workspaces whose attempt is terminal or whose
liveness marker is stale, stale
heartbeat/liveness files for dead PIDs, and partial setup directories that were
never published as complete evidence. It must not remove preserved workspaces,
`refs/ddx/iterations/...`, complete `.ddx/runs/<id>` or
`.ddx/executions/<attempt-id>` evidence, active workspaces with live
PID/session liveness, or non-DDx directories.

Cleanup is observable but not noisy. Routine passes are trace/debug or worker
events. Passes that reclaim significant bytes or inodes emit an operator-visible
summary such as `cleanup: removed 37 stale ddx worktrees, freed 14210 inodes`.
Resource exhaustion after cleanup is a hard visible stop message and a layer-3
`resource_exhausted` disposition.

Cleanup reporting includes scratch roots removed, bytes and inodes reclaimed,
preserved paths, and blocked warnings so operators can see why cleanup stopped
short.

### `ddx work` Run Modes

`ddx work` supports three named run modes that govern when the worker exits.
These are the public operator semantics; there are no other mode names.

| Mode | CLI flag | Semantics |
|---|---|---|
| `once` | `--once` | Process at most one execution-ready bead, then exit immediately. |
| `drain` | (default, no flag) | Process all currently execution-ready beads until a terminal stop condition fires, then exit. The worker does not sleep or wait for new work to arrive. |
| `watch` | `--watch` | Process ready work, then remain idle and rescan for newly arriving execution-ready work at each idle interval, until `signal`, fatal config error, budget/resource stop, or operator stop fires. |

**Idle interval** is the sleep duration between empty-queue scans while in `watch`
mode. The public flag is `--idle-interval <duration>` with a default of **30s**
in watch mode. There is no idle interval concept in `once` or `drain` modes.

**`--poll-interval` is removed from the public CLI and generated docs.** There is
no legacy alias. Operators previously relying on `--poll-interval 30s` to
stay alive should use `--watch`. Operators previously relying on
`--poll-interval=0` to drain-and-exit should use bare `ddx work` (drain mode,
the new default).

#### Direct CLI defaults

Bare `ddx work` with no mode flag uses **drain** mode: the worker processes all
currently execution-ready beads and exits when a stop condition fires. Operators
who want long-running watch behavior must pass `--watch`.

#### Server-managed worker defaults

Server-managed workers spawned via `POST /api/workers/work` default to
**watch** mode with a **30s idle interval** when the request omits `mode` and
`idle_interval`. The request vocabulary uses `mode` + `idle_interval` fields:

```json
{
  "mode": "once" | "drain" | "watch",
  "idle_interval": "<duration-string>"
}
```

`poll_interval` is not accepted as a server/API input. Workers previously
launched with `poll_interval` must migrate to `mode` + `idle_interval`.

The worker record exposes a `substate` field set to `"idle"` while the loop is
sleeping between empty scans in watch mode; it is cleared as soon as a candidate
is picked. Terminal worker states (`exited`, `failed`, `stopped`, `reaped`)
always clear `substate`.

The structured `loop.end` event carries an `exit_reason` field (`once_complete`,
`drain_complete`, `sigint`, `sigterm`, `fatal_config`, `resource_exhausted`;
`providers_exhausted` is reserved for the quota-pause work in ddx-aede917d).

Power escalation is evaluated between attempts as part of retry policy. A
higher-power retry raises `MinPower` and resets neither the evidence history nor
the no-progress counter; it is recorded as the next `ddx try` with its own
requested bounds and actual power metadata.

The evaluation log is persisted on the layer-3 record so a human or tool can
audit which condition fired and on which iteration.

### `ddx work` Stdout Contract

This section defines the human-facing stdout produced by `ddx work`. Every item
here is a stable operator-visible surface; the structured event log (JSONL) is a
separate machine-readable channel.

#### Terminal summary

When `ddx work` exits for any reason, it prints a terminal summary block. The
summary states:

- **Attempts:** total bead attempts made in this drain.
- **Closed:** count of beads closed during this drain.
- **Changed / already-satisfied:** breakdown of closed beads into those that
  produced a commit (`changed`) and those closed as already-satisfied with no
  commit (`already-satisfied`).
- **Failures:** count of attempts that ended in a non-success terminal outcome.
- **Stop reason:** the named stop condition that fired (`drained`, `blocked`,
  `deferred`, `no_progress`, `signal`, or `resource_exhausted`).
- **Remaining queue state:** a summary of open beads broken into buckets (see
  below).

Example terminal summary:

```
ddx work — drain complete
  attempts:          7
  closed:            5  (changed: 4, already-satisfied: 1)
  failures:          2
  stop reason:       drained

remaining queue:
  execution-ready:   0
  waiting-on-deps:   3
  proposed:          1
  cooldown/deferred: 1
  not-eligible:      0
  superseded:        0
  epic/closure:      0
```

#### Remaining queue-state buckets

The remaining queue-state summary distinguishes the following buckets where DDx
can compute them:

| Bucket | Definition |
|---|---|
| `execution-ready` | Beads whose dependencies are satisfied and that have no outstanding block |
| `waiting-on-deps` | `status=open` beads with unsatisfied dependencies; derived waiting, not `status=blocked` |
| `blocked` | Beads with `status=blocked` because an external recheckable blocker is recorded |
| `proposed` | Beads awaiting operator decision before autonomous execution |
| `cooldown/deferred` | Beads on retry-after cooldown or explicitly deferred |
| `not-eligible` | Beads in a non-runnable state (closed, cancelled, superseded-pending-removal) |
| `superseded` | Beads explicitly superseded by later work |
| `epic/closure-candidate` | Parent/epic beads that may be closable when all children complete |

#### Operator-review dependency pressure

When open beads are transitively waiting behind `status=proposed` operator
review blockers, the terminal summary and each watch-mode idle poll include a
dependency-pressure section. The wording is:

```
30 beads waiting behind 3 proposed blockers:
  ddx-a1b2c3d4 "resolve auth ambiguity" (12 downstream)
  ddx-e5f6a7b8 "clarify rate-limit spec" (11 downstream)
  ddx-c9d0e1f2 "investigate flaky test" (7 downstream)
```

**Total waiting count** is the number of unique open downstream beads that are
transitively waiting behind one or more proposed operator-review blockers — the
deduplicated union across all such blockers. **Per-blocker
downstream count** is the count of unique open beads that are transitively
waiting through that specific blocker. Per-blocker counts may sum higher than
the total when a bead is waiting behind multiple operator-review blockers.

#### Watch mode idle stdout

While in watch mode with an empty queue, every idle poll prints a compact
queue-state line:

```
idle: no execution-ready beads; sleeping 30s [execution-ready: 0, waiting-on-deps: 3, proposed: 1]
```

The idle line always includes: the literal `idle:` label, a reason phrase, the
sleep duration, and a compact inline queue-state summary with at minimum the
`execution-ready`, `waiting-on-deps`, and `proposed` counts.

The full blocker list (blocker IDs, titles, and per-blocker downstream counts)
may be suppressed on repeated idle polls when the queue state is unchanged from
the prior poll. The counts and sleep duration remain visible on every poll.

#### Next-ready-bead transition stdout

When watch mode wakes up from an idle sleep and claims new work, stdout separates
the new bead from the prior idle block with a transition line printed before the
bead execution block starts:

```
taking next ready bead from queue: ddx-a1b2c3d4 — "implement login rate limiting"
```

The format is `taking next ready bead from queue: <id> — <title>`.

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

### Forward-Progress Readiness Policy

**Open is the execution lane.** `status=open` is the default runnable state for
beads that are not yet closed, superseded, or awaiting external blockers.
Readiness findings do not move beads from `open` to `proposed` unless a hard
operator-required condition exists after safe rewrite and decomposition options
are exhausted.

**Proposed is the operator-decision escape.** `status=proposed` is an explicit
operator-review gate. An operator moves a bead from `proposed` to `open` when
they accept it as actionable despite readiness warnings or ambiguity.

**Operator-promoted beads are durable.** When an operator moves a bead from
`status=proposed` back to `status=open` (operator promotion), readiness must not
move that same bead back to `proposed` for the same rule or finding unless:
- A prompt-relevant field (description, acceptance, labels, parent, deps) has
  changed materially, **or**
- The operator explicitly requests re-triage via `--force`

This non-regression clause prevents readiness from recreating a proposed→open→proposed
loop after an operator has already made an explicit acceptance decision.

Readiness assessment reports must be diagnostic and distinct from hard parking
decisions. In WARN-ONLY mode (the default per ADR-023), readiness warnings
proceed by default, and only a hard operator-required condition may park the
bead in `status=proposed`. BLOCK mode (opt-in) may stop dispatch on a valid
low readiness score, but it still honors the operator-promotion non-regression
clause. The staged rollout and factory mode details are in ADR-023 "Staged
Rollout And Factory Mode" and "Operator-Promotion Non-Regression Clause".

The hook receives the bead record, current execution policy, hook mode
(`WARN-ONLY` or `BLOCK`), and the layer-3 evidence directory. It invokes the
bead-lifecycle workflow skill from FEAT-011 and writes a bead readiness,
scope, and decomposition report. In WARN-ONLY mode, the report is diagnostic
and the open forward-progress lane continues unless the hook can safely
improve or decompose the bead or reaches a hard operator-required condition.
In BLOCK/factory mode, a valid low readiness score, unsafe ambiguity, or
too-large classification stops implementation before claim.

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
- TD-027 §18 lifecycle migration status for queue-derived and worker-start
  commands;
- the root `ddx` project skill under `.agents/skills/ddx/SKILL.md` or
  `.claude/skills/ddx/SKILL.md`;
- the nested `bead-lifecycle` skill when readiness/lint/triage hooks are wired;
- legacy project-local skill symlinks from the pre-FEAT-015 install model.

Passing preflight checks are silent. Failed checks are observable and
actionable. `ddx work` and `ddx try` fail on missing required project/tracker
state. Unmigrated lifecycle queues fail closed before worker startup with the
TD-027 §18 migration counts, affected bead sample, `ddx bead migrate --lifecycle --dry-run`,
`ddx bead migrate --lifecycle --apply`, and git rollback guidance. Optional
lifecycle-hook degradation in WARN-ONLY mode still warns once per process and
continues after the lifecycle migration gate is clean. `ddx server`
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
- `readiness_proposed`
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
    execution worktrees, DDx-created test/e2e scratch roots, generated test
    binaries, and liveness files through inline, loop, and background cleanup
    paths without deleting preserved attempts or published evidence.
15. **Resource preflight** — `ddx try` validates writable execution roots and
    free bytes/inodes before claim; failed validation may trigger one cleanup
    retry, and `ddx work` / server-managed workers run cleanup before claim
    whenever temp free space drops below a soft high-water threshold. If the
    temp roots remain below the hard floor after cleanup, the loop returns
    `resource_exhausted` without claiming.
16. **Network-free drain boundary** — `Land()` performs only the
    worktree-merge into local main, under a brief local lock. Origin-sync
    (`git fetch` + merging `origin/main` into local main) and origin-push
    (`git push`) are separate operations, operator-driven or
    background-deferred, and are never invoked from `Land()` or from the
    drain loop. See FEAT-023 for the designated origin-sync command.
17. **Automatic recovery from persistent failure** — when a bead's within-cycle
    escalation ladder is exhausted on `N` consecutive drain cycles (default
    `N = 2`, configurable in `.ddx/config.yaml` as
    `escalation.auto_recovery_threshold`), `ddx work` attempts
    (a) reframe via a strong-power reframer agent, then (b) decomposition via
    a strong-power decomposer agent, before parking the bead at
    `status=proposed`. Each recovery step follows ADR-024 P4 and P3 (strong
    `MinPower` floor, passthrough constraints forwarded unchanged). Outcomes
    and status transitions are defined in TD-031 §2 (`reframe_applied`,
    `decompose_applied`, `auto_recovery_failed`) and the sequence in
    SD-025 Layer 3.5.

Expected implementation tests include `TestExecutionCleanup_RemovesStaleDDXScratchDirs`,
`TestWorkResourcePreflight_RunsCleanupBelowSoftFloor`, and
`TestWorkResourcePreflight_StopsBelowHardFloorAfterCleanup`.

### Non-Functional

- **Per-bead cost budget:** every bead's escalation attempts (implementation,
  review, reframer, and decomposer invocations) are bounded by a configurable
  per-bead cost cap (`escalation.per_bead_budget_usd` in `.ddx/config.yaml`).
  When the cap is exceeded, DDx records `per_bead_budget_exhausted` (TD-031 §2),
  releases the claim without cooldown, and leaves the bead `status=open` and
  re-claimable. Budget exhaustion is a recheckable signal, not a terminal state;
  an operator may raise the cap or the bead may be retried in a later drain.
  See ADR-024 Per-Bead Budget.
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
- **Network isolation:** single-developer/single-node drain does not block
  on remote I/O; origin-sync and origin-push are decoupled and may run in
  parallel or in the background without holding the drain or land lock.

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

### Run resolver contract

The canonical deep loader for a run detail surface is `run(id:)`. It returns
the run-layer fields used by FEAT-008:

- `prompt`
- `response`
- `stderr`
- `bundleFiles[]`

The tool-call resolver is the `toolCalls(first, after)` stream exposed via
`runToolCalls(id:, first:, after:)`. It returns the normalized tool-call
stream as a paginated `RunToolCallConnection`, where each node projects the
canonical drain-time `ToolCallEntry` shape:

- `tool`
- `input`
- `output`
- `duration_ms`
- `error`

`ToolCallEntry` is the normalized agent-side record of one tool execution. DDx
persists the stream at drain time so the run substrate can page the stored
sequence without rehydrating raw agent logs.

The `bundleFile(path)` lookup is exposed via `runBundleFile(id:, path:)`. It
is confined to the run's bundle root, rejects path traversal, absolute paths,
and symlink escapes, and inlines content only for the whitelist of small text
files documented by FEAT-008.

### Layer-to-substrate mapping for the Runs UI

The web Runs view (FEAT-008 §5, FEAT-021) renders three layer chips
backed by the unified substrate plus the legacy detail backings. The
shared run-detail tab strip is the canonical drill-down surface used by
FEAT-008, FEAT-019, and FEAT-021:

| Chip | Substrate row | Detail backing for row expansion |
|---|---|---|
| `work` | `layer: 3` Run record | Layer-3 record's queue inputs / stop-condition log / child layer-2 attempt ids; the shared tab strip exposes `overview` only |
| `try` | `layer: 2` Run record | The `.ddx/executions/<attempt-id>/` bundle attached to the layer-2 record (manifest, prompt, result, checks, verdict); the shared tab strip exposes `overview`, `prompt`, `response`, `tools`, and `evidence` |
| `run` | `layer: 1` Run record | The associated `AgentSession` row (prompt / response / stderr / billing / cached-token detail) joined onto the layer-1 record; the shared tab strip exposes `overview`, `prompt`, `response`, `session`, `tools`, and `evidence` |

`AgentSession` rows that have no parent layer-2 attempt (raw `ddx
agent log` invocations) surface as synthesized `layer=run` Runs rows
keyed by session id, so no agent-session row is dropped during the
Sessions/Executions tab retirement.

The shared tab strip is:

- `work` → `overview`
- `try` → `overview`, `prompt`, `response`, `tools`, `evidence`
- `run` → `overview`, `prompt`, `response`, `session`, `tools`, `evidence`

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
  mutate the pins or call any route-selection helper.

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
