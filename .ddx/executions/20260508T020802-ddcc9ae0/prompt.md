<bead-review>
  <bead id="ddx-b1cf1f6b" iter=1>
    <title>work: route cost-cap stops through StopCondition and audit consumers</title>
    <description>
PROBLEM
C9 requires budget/cost-cap stops to use the same durable stop policy as other drain exits. Today implementer cost and reviewer cost are accumulated in related but separate places, and the cost-cap stop is represented as a synthetic execution_failed report returned from the executor closure instead of a Budget StopCondition.

ROOT CAUSE WITH FILE:LINE
- cli/cmd/agent_cmd.go:1659-1694 builds costCap, accumulateBilledCost, and costCapTripped as local closures in runAgentExecuteLoopImpl.
- cli/cmd/agent_cmd.go:1789-1813 checks the cap inside the ExecuteBeadWorker executor closure and returns a fabricated ExecuteBeadReport when the cap is tripped.
- cli/internal/agent/execute_bead_post_review.go:116-139 charges reviewer slot cost to ReviewCostCap and emits review-cost-deferred, but that path does not feed a first-class Budget stop decision before close eligibility.
- cli/internal/agent/execute_bead_post_review.go:214-227 charges review cost immediately before the APPROVE close path, leaving no shared work-layer decision point between cost accumulation and CloseWithEvidence.

PROPOSED FIX
- Route max-cost decisions through the StopCondition Budget path introduced by ddx-89ab3fda.
- Keep local and subscription provider exclusion behavior from cli/cmd/agent_cmd.go:1663-1681.
- Charge implementer cost and reviewer group cost to the same budget accumulator before evaluating close eligibility.
- If reviewer cost trips the cap before close, record the existing review-cost-deferred evidence and leave the bead open instead of closing through APPROVE.
- Preserve the existing operator-facing max-cost message text unless tests intentionally update it.
- Commit a downstream-consumer audit at .ddx/executions/&lt;run-id&gt;/cost-cap-consumers.md listing cost-cap/maximum-cost event and detail-string consumers found with rg.

NON-SCOPE
- Do not alter no-progress accounting.
- Do not implement review repair cycles or MinPower repair escalation.
- Do not change Fizeau pricing/model catalog behavior.

DEPS
Depends on ddx-89ab3fda so Budget is a typed StopCondition before cost-cap behavior is rewired.
    </description>
    <acceptance>
1. Budget stops produced by --max-cost flow through the StopCondition Budget helper rather than being represented only as a synthetic executor failure report.
2. Reviewer group cost is charged to the same budget accumulator before close eligibility; TestStopCondition_BudgetAfterReviewCostPreventsClose covers an APPROVE review whose reviewer cost trips the cap and proves CloseWithEvidence is not called.
3. TestStopCondition_BudgetAfterImplementerCostStopsBeforeNextClaim covers implementer cost tripping the cap and proves the next bead is not claimed.
4. Existing TestExecuteLoopCostCap_ShortCircuitsAfterCap and TestExecuteLoopCostCap_LocalProvidersDoNotCount still pass.
5. The audit report .ddx/executions/&lt;run-id&gt;/cost-cap-consumers.md is committed and lists every rg hit for "cost cap", "max-cost", "review-cost-deferred", and "execution_failed:max-cost" under cli/, docs/, .agents/skills/, and .claude/skills/.
6. cd cli &amp;&amp; go test ./cmd/... ./internal/agent/... -run "TestStopCondition_Budget|TestExecuteLoopCostCap|TestRunPostMergeReviewChargesReviewCost" -count=1 passes.
7. cd cli &amp;&amp; go test ./cmd/... ./internal/agent/... passes.
8. lefthook run pre-commit passes.
    </acceptance>
    <labels>phase:2, story:10, area:agent, area:cost, area:work, kind:refactor, reliability, adr:024, spec:FEAT-010, split-from:ddx-9228a484</labels>
  </bead>

  <changed-files>
    <file>.ddx/executions/20260508T015941-dfa83c66/cost-cap-consumers.md</file>
    <file>.ddx/executions/20260508T015941-dfa83c66/manifest.json</file>
    <file>.ddx/executions/20260508T015941-dfa83c66/result.json</file>
  </changed-files>

  <governing>
    <ref id="FEAT-010" path="docs/helix/01-frame/features/FEAT-010-task-execution.md" title="Feature: Task Execution">
      <content>
<untrusted-data>
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

## Agent Interaction Modes

DDx defines four named agent interaction modes (`DDX_MODE`) to separate
broad interactive queue stewardship from worker execution. The mode governs
what an agent opened in a DDx project is permitted to do.

| DDX_MODE | Description | Allowed actions | Prohibited actions |
|---|---|---|---|
| `queue_steward` | Default interactive mode. Surveys, triages, and advises on the queue without claiming or executing beads. | Read tracker and docs; report status; advise on bead quality; suggest readiness fixes; run `ddx bead list/ready/status/show`. | Claiming beads; creating isolated worktrees; starting `ddx work` or `ddx try` without an explicit user directive. |
| `bead_execution` | Worker mode. Executes one or more beads from the queue in isolated worktrees. | Full FEAT-010 layer-2 / layer-3 lifecycle: worktree creation, bead attempt, merge or preserve, evidence bundle. | Scope creep outside the named bead; switching to direct implementation without a bead. |
| `direct_user_implementation` | Human-directed implementation. The user has explicitly asked the agent to edit code or docs, bypassing the bead queue. | Edit code and docs as instructed; commit to the current branch. | Starting autonomous queue drain; claiming beads autonomously. |
| `review` | Read-only review. Grades an existing implementation against AC or specs. | Read code, docs, and evidence; return a structured verdict with per-AC findings. | Writing commits; claiming beads; modifying the tracker. |

### Precedence

`bead_execution` supersedes `queue_steward` when DDx invokes a worker
explicitly (via `ddx work`, `ddx try <id>`, or `ddx agent execute-bead`).
`direct_user_implementation` supersedes `queue_steward` when the user
explicitly asks for code or doc edits unrelated to the queue. `review`
supersedes `queue_steward` when the user explicitly requests a graded review
pass.

Tracker instructions, merge-policy, and safety rules in AGENTS.md are
load-bearing in every mode and are never overridden by mode selection.

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
- `actionable_but_rewritten` — DDx applied a validated replacement rewrite or
  metadata update that makes the bead more execution-ready before claim. Safe
  rewrites may replace noisy, stale, duplicated, or brittle description text
  with a fit-for-purpose bead body instead of appending more prompt burden. They may
  also format the description into the authoring template, add discovered
  file:line evidence, add an obvious test command, or wire deterministic
  labels/parent/deps. The mutation is recorded through `ddx bead` paths before
  claim, with before/after hashes and enough evidence to audit the replacement.
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

Readiness must reject the rewrite and park the bead as `ambiguous_needs_human`
when preservation cannot be proven from durable anchors. It must not invent
product behavior, choose between conflicting requirements, change scope, delete
unresolved constraints, or guess a missing governing artifact.

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
- `parked` — repair budget exhausted without approval; bead moved to `needs_human`.
- `conflicted` — approved candidate failed to land due to merge conflict; bead
  returned to `open` for re-attempt with a fresh `base_rev`.
- `budget-stopped` — drain-level cost or no-progress budget tripped; worktree
  preserved with evidence.

**Candidate-cycle sequence** (in the still-live attempt worktree, before land):

1. Create isolated worktree from `base_rev`.
2. Run implementation pass; collect `base_rev`, `result_rev`, run ids,
   verification output, and cost facts.
3. Pin candidate ref; emit `candidate-pinned`.
4. Run candidate checks (post-run tests, lint, gates). Failure emits
   `candidate-checks-failed`; plausibly capability-sensitive failures are
   eligible for a repair cycle when `repair_max_cycles` allows.
5. Dispatch read-only tool reviewers in the still-live worktree, **before land**.
   Record `review_group_id`, reviewer run ids, per-AC verdict, and cost.
6. On unanimous `APPROVE`: land, emit `final-result-landed`, close bead, clean up.
7. On `REQUEST_CHANGES` / `BLOCK` classified `review_fixable_gap` within
   `repair_max_cycles`: emit `repair-cycle-started`, run append-only repair in
   same worktree, return to step 3.
8. On `repair_max_cycles` exhausted: emit `repair-cycle-exhausted`, preserve
   worktree, park bead in `needs_human`.
9. On approved candidate that fails to land (merge conflict): emit
   `approved-land-conflict`, release claim, return bead to `open`.

**Budget axes** (each is distinct from cost, rate-limit/provider, and
no-progress budgets):

- **`repair_max_cycles`** — maximum repair cycles per attempt. Exhaustion fires
  `repair-cycle-exhausted` and parks the bead in `needs_human`; it does not
  consume the no-progress counter.
- **`review_max_retries_per_candidate`** — maximum reviewer retry attempts for a
  single candidate ref when reviewer invocations fail. Exhaustion fires
  `review-manual-required`. A new candidate ref (next repair cycle or fresh
  `ddx try`) resets this counter independently; it is strictly per-candidate.

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
   before an implementation attempt. Ambiguous or underspecified work is blocked
   with `needs_human`. Readiness infrastructure failure records evidence and
   follows the configured fail-open/factory-mode policy; it never creates an
   unexplained cooldown.
2. **Claim.** Claim only an `actionable_atomic` or safely rewritten bead. Claim
   races skip the bead for the current pass without cooldown.
3. **Primary implementation cycle.** Run one layer-2 implementation attempt.
   A candidate result records `base_rev`, `result_rev`, implementation run ids,
   route/power facts, verification output, and cost. Already-satisfied
   no-changes may close only after the required verification evidence exists;
   otherwise no-changes outcomes follow TD-031.
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
   retry path and does not park the bead in the `needs_human` lane.
   `review_spec_gap`, `review_missing_acceptance`, `review_too_large`, and
   non-mechanical unsafe or out-of-scope findings park the bead in the
   `needs_human` operator-attention lane, clear the active claim, and do not ask
   another implementer to guess. Malformed, empty, context-overflow, and
   transport reviewer failures emit `review-error` scoped to `result_rev` and
   reviewer slot; after `review_max_retries_per_candidate` they emit
   `review-manual-required`, clear the active claim, park the bead in the
   `needs_human` lane, and do not close. The `review_fixable_gap` path stays on
   the automatic retry track instead of converting into operator review.
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

`ddx work` and server-managed workers use the same cleanup manager before the
first claim and before later claims whenever any checked temp or evidence root
falls below the soft cleanup trigger of **512 MiB free bytes** or **8192 free
inodes**. If cleanup does not restore all roots above the hard stop floor of
**64 MiB free bytes** and **1024 free inodes**, the loop stops visibly with
`resource_exhausted` and claims no more beads.

After an attempt starts, Layer 2 removes the isolated worktree when the result
has been merged, explicitly preserved, classified as no-changes/no-evidence, or
interrupted through the cooperative shutdown path. Failed setup must remove any
partial unregistered directory it created. A worktree may remain only when DDx
records an explicit preserve decision with evidence pointing at the retained
path or ref.

DDx-owned cleanup scope includes execution worktrees, DDx-created test and e2e
scratch roots, generated test binaries, and run-state or liveness files. The
cleanup manager may delete only DDx-owned paths. Recognized DDx-owned scratch
prefixes are: `ddx-exec-wt`, `ddx-claim-heartbeats`, `ddx-metric-keepalive`,
`ddx-test-`, and `ddx-e2e-`. Any temp directory that contains a `cleanup.json`
metadata file with matching project ownership is also DDx-owned regardless of
its name.

Deletion is permitted only when all of the following hold:

- **Metadata-backed paths**: the `cleanup.json` liveness record is expired or
  the owning attempt has reached a terminal state.
- **Metadata-less recognized-prefix paths**: the directory's mtime is at least
  **6 hours** old and no live PID or active session is attached.

The manager must preserve published evidence, registered active worktrees, and
anything outside DDx-owned roots.

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

Cleanup reporting includes scratch roots removed, bytes and inodes reclaimed,
preserved paths, and blocked warnings so operators can see why cleanup stopped
short.

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
    execution worktrees, DDx-created test/e2e scratch roots, generated test
    binaries, and liveness files through inline, loop, and background cleanup
    paths without deleting preserved attempts or published evidence.
15. **Resource preflight** — `ddx try` validates writable execution roots and
    free bytes/inodes before claim; failed validation may trigger one cleanup
    retry, and `ddx work` / server-managed workers run cleanup before claim
    whenever temp free space drops below a soft high-water threshold. If the
    temp roots remain below the hard floor after cleanup, the loop returns
    `resource_exhausted` without claiming.

Expected implementation tests include `TestExecutionCleanup_RemovesStaleDDXScratchDirs`,
`TestWorkResourcePreflight_RunsCleanupBelowSoftFloor`, and
`TestWorkResourcePreflight_StopsBelowHardFloorAfterCleanup`.

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
</untrusted-data>
      </content>
    </ref>
  </governing>

  <diff rev="3c6a49add544e4bbca14b15db2812483ba154a0b">
<untrusted-data>
diff --git a/.ddx/executions/20260508T015941-dfa83c66/cost-cap-consumers.md b/.ddx/executions/20260508T015941-dfa83c66/cost-cap-consumers.md
deleted file mode 100644
index 674fd7026..000000000
--- a/.ddx/executions/20260508T015941-dfa83c66/cost-cap-consumers.md
+++ /dev/null
@@ -1,86 +0,0 @@
-# Cost-Cap Consumer Audit
-
-bead: ddx-b1cf1f6b  
-run-id: 20260508T015941-dfa83c66  
-generated: 2026-05-08
-
-Audit of every `rg` hit for the four cost-cap event and detail-string tokens
-under `cli/`, `docs/`, `.agents/skills/`, and `.claude/skills/`.
-
----
-
-## Term: "cost cap"
-
-| File | Line | Content |
-|------|------|---------|
-| `cli/cmd/agent_cmd.go` | 1692 | `Detail: fmt.Sprintf("cost cap reached: $%.2f billed >= $%.2f cap; ...")` — synthetic `ExecuteBeadStatusExecutionFailed` report returned from `costCapTripped` closure, consumed by `BudgetStop` in the loop |
-| `cli/internal/escalation/infrastructure.go` | 205 | `return fmt.Sprintf("cost cap reached: ...")` — `CostCapTracker.Tripped()` detail string; canonical operator-facing message |
-| `cli/internal/agent/execute_bead_loop.go` | 2241 | Comment: "configured cost cap after charging the reviewer cost against the shared" |
-| `cli/internal/agent/execute_bead_post_review.go` | 142 | `fmt.Fprintf(in.Log, "review cost cap deferred (%s %s): %s\n", ...)` — operator log line inside `chargeReviewCost()` closure |
-| `cli/internal/agent/execute_bead_loop_stop_test.go` | 100, 111 | Test fixtures using `"cost cap reached"` as expected detail string |
-| `cli/internal/agent/types.go` | 98 | Comment: "strongest viable route instead of the worker attempt's cost cap." |
-| `cli/cmd/agent_execute_loop_costcap_test.go` | 64, 100 | `TestExecuteLoopCostCap_ShortCircuitsAfterCap` docstring and assertion on detail |
-| `cli/internal/server/workers.go` | 844 | `Detail: fmt.Sprintf("cost cap reached: ...")` — server-side worker `costCapTripped` closure (mirrors CLI) |
-| `cli/internal/server/workers_costcap_test.go` | 88 | Server-side test assertion on `"cost cap reached"` detail string |
-| `docs/helix/02-design/adr/ADR-024-power-escalation-and-review-routing.md` | 205 | Mentions billed-cost cap exclusion via cost-class metadata |
-| `docs/helix/02-design/adr/ADR-021-operator-prompt-beads-web-write-path.md` | 63 | References cost cap in server-side worker context |
-| `docs/helix/02-design/plan-2026-04-18-ddx-agent-service-interface.md` | 1030 | Agent-service plan: per-task estimated cost checked against remaining budget |
-| `docs/helix/02-design/solution-designs/SD-014-token-awareness.md` | 13 | SD-014 mentions token/cost capture |
-| `docs/helix/01-frame/features/FEAT-010-task-execution.md` | 443 | FEAT-010: cost cap non-interaction note |
-| `docs/helix/01-frame/features/FEAT-014-token-awareness.md` | 481 | FEAT-014: drain-level budget stop and reviewer-cost cap cross-reference |
-| `.agents/skills/docs/helix/02-design/plan-2026-04-18-ddx-agent-service-interface.md` | 1005 | Skill copy of plan doc |
-| `.agents/skills/docs/helix/02-design/solution-designs/SD-014-token-awareness.md` | 13 | Skill copy of SD-014 |
-| `.claude/skills/docs/helix/02-design/plan-2026-04-18-ddx-agent-service-interface.md` | 1005 | Skill copy of plan doc |
-| `.claude/skills/docs/helix/02-design/solution-designs/SD-014-token-awareness.md` | 13 | Skill copy of SD-014 |
-
----
-
-## Term: "max-cost"
-
-| File | Line | Content |
-|------|------|---------|
-| `cli/cmd/agent_cmd.go` | 1520 | `maxCostUSD, _ := cmd.Flags().GetFloat64("max-cost")` — reads the `--max-cost` flag value |
-| `cli/cmd/agent_cmd.go` | 1661 | Comment: "providers) above --max-cost trips the cap and halts further bead" |
-| `cli/cmd/work.go` | 83 | `cmd.Flags().Float64("max-cost", escalation.DefaultMaxCostUSD, ...)` — flag definition on `ddx work` |
-| `docs/helix/02-design/plan-2026-04-18-ddx-agent-service-interface.md` | 1029 | `--max-cost-usd <N>` flag description in agent-service plan |
-| `.agents/skills/docs/helix/02-design/plan-2026-04-18-ddx-agent-service-interface.md` | 1004 | Skill copy |
-| `.agents/skills/website/content/docs/cli/commands/ddx_work.md` | 89 | Generated CLI docs: `--max-cost float` description |
-| `.agents/skills/website/content/docs/cli/commands/ddx_agent_execute-loop.md` | 86 | Generated CLI docs: `--max-cost float` description |
-| `.claude/skills/website/content/docs/cli/commands/ddx_work.md` | 89 | Skill copy of generated CLI docs |
-| `.claude/skills/website/content/docs/cli/commands/ddx_agent_execute-loop.md` | 86 | Skill copy of generated CLI docs |
-| `.claude/skills/docs/helix/02-design/plan-2026-04-18-ddx-agent-service-interface.md` | 1004 | Skill copy |
-
----
-
-## Term: "review-cost-deferred"
-
-| File | Line | Content |
-|------|------|---------|
-| `cli/internal/agent/execute_bead_post_review.go` | 134–135 | `Kind: "review-cost-deferred"` and `Summary: "review-cost-deferred"` — event emitted inside `chargeReviewCost()` when reviewer cost trips the cap during an APPROVE or error path |
-| `cli/internal/agent/execute_bead_loop_stop_test.go` | 203, 208 | `TestStopCondition_BudgetAfterReviewCostPreventsClose` (AC2): asserts `review-cost-deferred` event is recorded and bead stays open |
-| `cli/internal/agent/execute_bead_review_test.go` | 208, 258 | `TestRunPostMergeReviewChargesReviewCostAndDefersWhenCapTrips` and `TestRunPostMergeReviewChargesReviewCostOnReviewerError`: assert event is emitted on both APPROVE and error paths |
-
----
-
-## Term: "execution_failed:max-cost"
-
-No hits in `cli/`, `docs/`, `.agents/skills/`, or `.claude/skills/`.
-
-This token does not appear anywhere in the codebase. The implementation uses
-`ExecuteBeadStatusExecutionFailed` as the report status and the "cost cap
-reached: …" detail string (from `CostCapTracker.Tripped()`) rather than a
-composite `execution_failed:max-cost` status string. The `BudgetStop` callback
-and `StopCondition Budget` routing do not use a colon-separated compound value.
-
----
-
-## Summary: Routing Through StopCondition Budget
-
-The cost-cap stop path after ddx-b1cf1f6b:
-
-1. `--max-cost` flag → `escalation.NewCostCapTracker(maxCostUSD, lookup)` in `agent_cmd.go`
-2. Implementer cost accumulated via `costCap.Add(harness, costUSD)` after each attempt
-3. Reviewer cost accumulated via `capTracker.Add(slot.Result.ReviewerHarness, slot.Result.CostUSD)` inside `chargeReviewCost()` in `execute_bead_post_review.go`
-4. `BudgetStop` callback (`costCapTripped`) set on `ExecuteBeadLoopRuntime`; checked at top of every loop iteration
-5. When tripped, `applyStop(work.StopInput{Budget: true})` → `StopConditionBudget` → loop exits cleanly
-6. On APPROVE + cap exceeded: `Approved=false`, `review-cost-deferred` event recorded, bead stays open; `BudgetStop` fires on next iteration
diff --git a/.ddx/executions/20260508T015941-dfa83c66/manifest.json b/.ddx/executions/20260508T015941-dfa83c66/manifest.json
new file mode 100644
index 000000000..c8991ab02
--- /dev/null
+++ b/.ddx/executions/20260508T015941-dfa83c66/manifest.json
@@ -0,0 +1,234 @@
+{
+  "attempt_id": "20260508T015941-dfa83c66",
+  "bead_id": "ddx-b1cf1f6b",
+  "base_rev": "a95284ee3ce0882c853dda1ac715596b2838dc0d",
+  "created_at": "2026-05-08T01:59:45.483138443Z",
+  "requested": {
+    "prompt": "synthesized"
+  },
+  "bead": {
+    "id": "ddx-b1cf1f6b",
+    "title": "work: route cost-cap stops through StopCondition and audit consumers",
+    "description": "PROBLEM\nC9 requires budget/cost-cap stops to use the same durable stop policy as other drain exits. Today implementer cost and reviewer cost are accumulated in related but separate places, and the cost-cap stop is represented as a synthetic execution_failed report returned from the executor closure instead of a Budget StopCondition.\n\nROOT CAUSE WITH FILE:LINE\n- cli/cmd/agent_cmd.go:1659-1694 builds costCap, accumulateBilledCost, and costCapTripped as local closures in runAgentExecuteLoopImpl.\n- cli/cmd/agent_cmd.go:1789-1813 checks the cap inside the ExecuteBeadWorker executor closure and returns a fabricated ExecuteBeadReport when the cap is tripped.\n- cli/internal/agent/execute_bead_post_review.go:116-139 charges reviewer slot cost to ReviewCostCap and emits review-cost-deferred, but that path does not feed a first-class Budget stop decision before close eligibility.\n- cli/internal/agent/execute_bead_post_review.go:214-227 charges review cost immediately before the APPROVE close path, leaving no shared work-layer decision point between cost accumulation and CloseWithEvidence.\n\nPROPOSED FIX\n- Route max-cost decisions through the StopCondition Budget path introduced by ddx-89ab3fda.\n- Keep local and subscription provider exclusion behavior from cli/cmd/agent_cmd.go:1663-1681.\n- Charge implementer cost and reviewer group cost to the same budget accumulator before evaluating close eligibility.\n- If reviewer cost trips the cap before close, record the existing review-cost-deferred evidence and leave the bead open instead of closing through APPROVE.\n- Preserve the existing operator-facing max-cost message text unless tests intentionally update it.\n- Commit a downstream-consumer audit at .ddx/executions/\u003crun-id\u003e/cost-cap-consumers.md listing cost-cap/maximum-cost event and detail-string consumers found with rg.\n\nNON-SCOPE\n- Do not alter no-progress accounting.\n- Do not implement review repair cycles or MinPower repair escalation.\n- Do not change Fizeau pricing/model catalog behavior.\n\nDEPS\nDepends on ddx-89ab3fda so Budget is a typed StopCondition before cost-cap behavior is rewired.",
+    "acceptance": "1. Budget stops produced by --max-cost flow through the StopCondition Budget helper rather than being represented only as a synthetic executor failure report.\n2. Reviewer group cost is charged to the same budget accumulator before close eligibility; TestStopCondition_BudgetAfterReviewCostPreventsClose covers an APPROVE review whose reviewer cost trips the cap and proves CloseWithEvidence is not called.\n3. TestStopCondition_BudgetAfterImplementerCostStopsBeforeNextClaim covers implementer cost tripping the cap and proves the next bead is not claimed.\n4. Existing TestExecuteLoopCostCap_ShortCircuitsAfterCap and TestExecuteLoopCostCap_LocalProvidersDoNotCount still pass.\n5. The audit report .ddx/executions/\u003crun-id\u003e/cost-cap-consumers.md is committed and lists every rg hit for \"cost cap\", \"max-cost\", \"review-cost-deferred\", and \"execution_failed:max-cost\" under cli/, docs/, .agents/skills/, and .claude/skills/.\n6. cd cli \u0026\u0026 go test ./cmd/... ./internal/agent/... -run \"TestStopCondition_Budget|TestExecuteLoopCostCap|TestRunPostMergeReviewChargesReviewCost\" -count=1 passes.\n7. cd cli \u0026\u0026 go test ./cmd/... ./internal/agent/... passes.\n8. lefthook run pre-commit passes.",
+    "parent": "ddx-5cb6e6cd",
+    "labels": [
+      "phase:2",
+      "story:10",
+      "area:agent",
+      "area:cost",
+      "area:work",
+      "kind:refactor",
+      "reliability",
+      "adr:024",
+      "spec:FEAT-010",
+      "split-from:ddx-9228a484"
+    ],
+    "metadata": {
+      "claimed-at": "2026-05-08T01:58:20Z",
+      "claimed-machine": "eitri",
+      "claimed-pid": "1033746",
+      "events": [
+        {
+          "actor": "erik",
+          "body": "{\"rationale\":\"All 8 rubric criteria pass. (1) Title is imperative and names subsystem + change. (2) Description contains PROBLEM, ROOT CAUSE with specific file:line references (agent_cmd.go:1659-1694, :1789-1813, execute_bead_post_review.go:116-139, :214-227), PROPOSED FIX, and NON-SCOPE. (3) AC is numbered and verifiable; items 2, 3, 4 name specific Test* functions; item 6 includes a -run filter; items 7 and 8 are the required `cd cli \\u0026\\u0026 go test` and `lefthook run pre-commit` gate lines. (4) Labels include phase:2, area:agent/cost/work, kind:refactor, adr:024, spec:FEAT-010. (5) Parent ddx-5cb6e6cd is explicit. (6) Dep ddx-89ab3fda is explicit with a rationale sentence in DEPS section. (7) No /tmp paths; evidence file targets .ddx/executions/\\u003crun-id\\u003e/ per convention. (8) Cold-read self-test passes: file paths, line numbers, and test names give a sub-agent a clear starting point with no operator hand-curation required.\",\"score\":8,\"suggested_fixes\":[],\"waivers_applied\":[]}",
+          "created_at": "2026-05-07T17:57:18.282834117Z",
+          "kind": "bead-quality.lint",
+          "source": "ddx agent execute-loop",
+          "summary": "score=8"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"actual_harness\":\"claude\",\"actual_model\":\"sonnet\",\"actual_power\":0,\"actual_provider\":\"\",\"attempt_id\":\"20260507T175720-d57edc41\",\"bead_id\":\"ddx-b1cf1f6b\",\"rejected_route_pins\":null,\"requested_tier\":\"standard\",\"routing_intent_degraded\":false,\"routing_intent_note\":\"\",\"routing_intent_source\":\"heuristic\",\"smart_justification\":\"\"}",
+          "created_at": "2026-05-07T18:16:42.108707774Z",
+          "kind": "execution-routing-intent",
+          "source": "ddx agent execute-loop",
+          "summary": "source=heuristic tier=standard model=sonnet harness=claude"
+        },
+        {
+          "actor": "erik",
+          "body": "{\"base_rev\":\"a7aee500e8996c00cc48b4fe961033fb84e30d98\",\"classification\":\"transport\",\"detail\":\"git rev-parse HEAD: chdir /var/tmp/ddx-exec-wt/.execute-bead-wt-ddx-b1cf1f6b-20260507T175720-d57edc41: no such file or directory: failed to read worktree HEAD: git rev-parse HEAD: chdir /var/tmp/ddx-exec-wt/.execute-bead-wt-ddx-b1cf1f6b-20260507T175720-d57edc41: no such file or directory\",\"rationale\":\"The worktree directory /var/tmp/ddx-exec-wt/.execute-bead-wt-ddx-b1cf1f6b-20260507T175720-d57edc41 did not exist when the post-execution HEAD read was attempted. result_rev equals base_rev, confirming no commits landed. The $4.10 spend and 19-minute duration suggest the agent ran but the worktree was evicted (tmpfs purge, OS reboot, or OOM sweep) before the merge phase could read HEAD. This is an infrastructure/transport failure unrelated to bead correctness or test failures.\",\"recommended_action\":\"retry\",\"result_rev\":\"a7aee500e8996c00cc48b4fe961033fb84e30d98\",\"session_id\":\"eb-0cfd622c\",\"status\":\"execution_failed\",\"suggested_amendments\":\"none\",\"suggested_followup_beads\":[]}",
+          "created_at": "2026-05-07T18:16:53.930547882Z",
+          "kind": "bead-quality.triage",
+          "source": "ddx agent execute-loop",
+          "summary": "transport: retry"
+        },
+        {
+          "actor": "erik",
+          "body": "git rev-parse HEAD: chdir /var/tmp/ddx-exec-wt/.execute-bead-wt-ddx-b1cf1f6b-20260507T175720-d57edc41: no such file or directory: failed to read worktree HEAD: git rev-parse HEAD: chdir /var/tmp/ddx-exec-wt/.execute-bead-wt-ddx-b1cf1f6b-20260507T175720-d57edc41: no such file or directory\nresult_rev=a7aee500e8996c00cc48b4fe961033fb84e30d98\nbase_rev=a7aee500e8996c00cc48b4fe961033fb84e30d98\noutcome_reason=transport",
+          "created_at": "2026-05-07T18:16:53.99974789Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "erik",
+          "body": "{\"rationale\":\"All 8 rubric criteria pass. (1) Title is imperative and names subsystems (agent/cost/work) plus the change. (2) Description has PROBLEM, ROOT CAUSE with four specific file:line citations, PROPOSED FIX with actionable steps, and a clear NON-SCOPE block. (3) AC is numbered 1-8; AC2 and AC3 name new TestStopCondition_* functions, AC4 names two existing TestExecuteLoopCostCap_* tests, AC6 supplies a precise -run filter, AC7 is the full suite gate, AC8 is the lefthook gate — both required terminal lines are present. (4) Labels carry phase:2, area:*, kind:refactor, and cross-refs adr:024, spec:FEAT-010, split-from:ddx-9228a484. (5) Parent ddx-5cb6e6cd and dep ddx-89ab3fda are explicit, with a prose rationale for the dep. (6) No /tmp or out-of-repo paths; evidence target is .ddx/executions/\\u003crun-id\\u003e/cost-cap-consumers.md. (7) Root cause section has four file:line anchors covering both cmd and internal/agent. (8) AC2 and AC3 wire new assertions directly to named Test* functions, providing a verifiable gate for the new behavior.\",\"score\":8,\"suggested_fixes\":[],\"waivers_applied\":[]}",
+          "created_at": "2026-05-07T18:22:42.803699177Z",
+          "kind": "bead-quality.lint",
+          "source": "ddx agent execute-loop",
+          "summary": "score=8"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"resolved_model\":\"sonnet\",\"fallback_chain\":[]}",
+          "created_at": "2026-05-07T18:30:46.453760017Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude model=sonnet"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260507T182244-fff11bcb\",\"harness\":\"claude\",\"model\":\"sonnet\",\"input_tokens\":59,\"output_tokens\":9273,\"total_tokens\":9332,\"cost_usd\":0.7974016500000001,\"duration_ms\":479318,\"exit_code\":0}",
+          "created_at": "2026-05-07T18:30:46.51689923Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=9332 cost_usd=0.7974 model=sonnet"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"actual_harness\":\"claude\",\"actual_model\":\"sonnet\",\"actual_power\":0,\"actual_provider\":\"\",\"attempt_id\":\"20260507T182244-fff11bcb\",\"bead_id\":\"ddx-b1cf1f6b\",\"rejected_route_pins\":null,\"requested_tier\":\"standard\",\"routing_intent_degraded\":false,\"routing_intent_note\":\"\",\"routing_intent_source\":\"heuristic\",\"smart_justification\":\"\"}",
+          "created_at": "2026-05-07T18:30:53.793505034Z",
+          "kind": "execution-routing-intent",
+          "source": "ddx agent execute-loop",
+          "summary": "source=heuristic tier=standard model=sonnet harness=claude"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"escalation_count\":0,\"fallback_chain\":[],\"final_tier\":\"\",\"requested_profile\":\"\",\"requested_tier\":\"\",\"resolved_model\":\"sonnet\",\"resolved_provider\":\"claude\",\"resolved_tier\":\"\"}",
+          "created_at": "2026-05-07T18:30:54.092229649Z",
+          "kind": "routing",
+          "source": "ddx agent execute-loop",
+          "summary": "provider=claude model=sonnet"
+        },
+        {
+          "actor": "erik",
+          "body": "failure_class=review-error: unparseable\nattempt_count=1\nresult_rev=24ba16c3880841825a2a3222a8e9aea0b6aff464\n\nreviewer output: unparseable JSON verdict: approve verdict without per-AC evidence\nharness=claude\nmodel=opus\ninput_bytes=76092\noutput_bytes=1484\nelapsed_ms=111607",
+          "created_at": "2026-05-07T18:34:30.836895193Z",
+          "kind": "review-error",
+          "source": "ddx agent execute-loop",
+          "summary": "review-error: unparseable"
+        },
+        {
+          "actor": "erik",
+          "body": "pre-close review: review-error: unparseable\nCost-cap stops now route through StopCondition Budget: post-executor synthetic-failure check removed (agent_cmd.go:1798-), BudgetStop callback wired (agent_cmd.go:1821) hits work.ClassifyStop{Budget:true} (execute_bead_loop.go:574). chargeReviewCost returns bool; APPROVE path skips CloseWithEvidence and sets Approved=false when cap tripped. New TestStopCondition_Budget* tests added; updated review test asserts Approved=false. cost-cap-consumers.md committed at .ddx/executions/20260507T182244-fff11bcb/.\nresult_rev=24ba16c3880841825a2a3222a8e9aea0b6aff464\nbase_rev=b04f5f727ec1df2c80e2cb86614347155c77a0f5",
+          "created_at": "2026-05-07T18:34:30.905942452Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "review_malfunction"
+        },
+        {
+          "actor": "erik",
+          "body": "{\"rationale\":\"All 8 rubric criteria satisfied. Title is imperative and names the subsystem. Description has PROBLEM, ROOT CAUSE with four specific file:line anchors, PROPOSED FIX, NON-SCOPE, and DEPS sections. AC is numbered; items 2, 3, and 4 name concrete Test* functions; items 6-7 carry the go-test gate and item 8 carries the lefthook gate. Labels include phase, story, area (×3), kind, reliability, adr, and spec cross-refs. Parent (ddx-5cb6e6cd) and dep (ddx-89ab3fda) are explicit. No /tmp references. Execution-evidence path follows the \\u003crun-id\\u003e convention. Bead is self-contained and actionable cold.\",\"score\":8,\"suggested_fixes\":[],\"waivers_applied\":[]}",
+          "created_at": "2026-05-07T19:03:12.220677262Z",
+          "kind": "bead-quality.lint",
+          "source": "ddx agent execute-loop",
+          "summary": "score=8"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"actual_harness\":\"\",\"actual_model\":\"\",\"actual_power\":0,\"actual_provider\":\"\",\"attempt_id\":\"\",\"bead_id\":\"ddx-b1cf1f6b\",\"rejected_route_pins\":null,\"requested_tier\":\"standard\",\"routing_intent_degraded\":true,\"routing_intent_note\":\"actual route facts unavailable\",\"routing_intent_source\":\"heuristic\",\"smart_justification\":\"\"}",
+          "created_at": "2026-05-07T19:03:16.811852902Z",
+          "kind": "execution-routing-intent",
+          "source": "ddx agent execute-loop",
+          "summary": "source=heuristic tier=standard note=actual route facts unavailable"
+        },
+        {
+          "actor": "erik",
+          "body": "{\"base_rev\":\"\",\"classification\":\"tests_red\",\"detail\":\"pre-execute-bead checkpoint: synthesize commit: ╭──────────────────────────────────────╮\\n│ 🥊 lefthook v2.1.6  hook: pre-commit │\\n╰──────────────────────────────────────╯\\n│  Skipping hook sync: core.hooksPath is set locally to '/Users/erik/Projects/ddx/.git/hooks'            \\n│                                                                                                        \\n│  hint: Unset it:                                                                                       \\n│  hint:   git config --unset-all --local core.hooksPath                                                 \\n│  hint:                                                                                                 \\n│  hint: Run 'lefthook install --reset-hooks-path' to automatically unset it.                            \\n│  hint:                                                                                                 \\n│  hint: Run 'lefthook install --force' to install hooks anyway in '/Users/erik/Projects/ddx/.git/hooks'.\\n│  test-engineer-persona-drift (skip) no matching staged files\\n│  debug-python (skip) no files for inspection\\n│  design-md-lint (skip) no matching staged files\\n┃  large-files ❯ \\n\\n\\n┃  ddx-validate ❯ \\n\\n\\n┃  skill-schema ❯ \\n\\n\\n┃  sync-embedded-skills ❯ \\n\\n\\n┃  conflicts ❯ \\n\\n\\n┃  secrets ❯ \\n\\n\\n┃  go-test ❯ \\n\\ngo: -race requires cgo; enable cgo by setting CGO_ENABLED=1\\n\\nexit status 2┃  go-fmt ❯ \\n\\n\\n┃  go-lint ❯ \\n\\n0 issues.\\n\\n┃  evidence-lint ❯ \\n\\n\\n┃  runtime-lint ❯ \\n\\n\\n┃  go-build ❯ \\n\\ngithub.com/DocumentDrivenDX/ddx/tools/lint/evidencelint/cmd/evidencelint\\ngithub.com/DocumentDrivenDX/ddx/tools/lint/routinglint/cmd/routinglint\\ngithub.com/DocumentDrivenDX/ddx/tools/lint/runtimelint/cmd/runtimelint\\ngithub.com/DocumentDrivenDX/ddx\\ngithub.com/DocumentDrivenDX/ddx/tools/gendoc\\n\\n                                      \\n  ────────────────────────────────────\\nsummary: (done in 3.01 seconds)       \\n✔️ large-files (0.01 seconds)\\n✔️ ddx-validate (0.01 seconds)\\n✔️ skill-schema (0.02 seconds)\\n✔️ conflicts (0.02 seconds)\\n✔️ secrets (0.03 seconds)\\n✔️ sync-embedded-skills (0.02 seconds)\\n✔️ go-fmt (0.09 seconds)\\n✔️ go-lint (0.83 seconds)\\n✔️ evidence-lint (0.90 seconds)\\n✔️ runtime-lint (0.92 seconds)\\n✔️ go-build (3.01 seconds)\\n🥊 go-test (0.08 seconds): exit status 1\",\"rationale\":\"The pre-commit go-test hook failed with 'go: -race requires cgo; enable cgo by setting CGO_ENABLED=1' (exit status 2). This is an environment configuration issue in the worktree — CGO_ENABLED=0 or a missing cgo toolchain prevented the race-detector test run. The implementation work itself appears complete (review-malfunction event shows all AC addressed, cost-cap-consumers.md committed, Budget StopCondition wired). The failure is not in test logic but in the test runner environment.\",\"recommended_action\":\"retry\",\"result_rev\":\"\",\"session_id\":\"\",\"status\":\"execution_failed\",\"suggested_amendments\":\"Set CGO_ENABLED=1 in the execute-bead environment or configure the lefthook go-test hook to use -count=1 without -race when cgo is unavailable. Alternatively, amend the bead AC to specify 'cd cli \\u0026\\u0026 CGO_ENABLED=1 go test ...' explicitly so the agent sets the variable before running the pre-commit gate.\",\"suggested_followup_beads\":[]}",
+          "created_at": "2026-05-07T19:03:28.813789389Z",
+          "kind": "bead-quality.triage",
+          "source": "ddx agent execute-loop",
+          "summary": "tests_red: retry"
+        },
+        {
+          "actor": "erik",
+          "body": "pre-execute-bead checkpoint: synthesize commit: ╭──────────────────────────────────────╮\n│ 🥊 lefthook v2.1.6  hook: pre-commit │\n╰──────────────────────────────────────╯\n│  Skipping hook sync: core.hooksPath is set locally to '/Users/erik/Projects/ddx/.git/hooks'            \n│                                                                                                        \n│  hint: Unset it:                                                                                       \n│  hint:   git config --unset-all --local core.hooksPath                                                 \n│  hint:                                                                                                 \n│  hint: Run 'lefthook install --reset-hooks-path' to automatically unset it.                            \n│  hint:                                                                                                 \n│  hint: Run 'lefthook install --force' to install hooks anyway in '/Users/erik/Projects/ddx/.git/hooks'.\n│  test-engineer-persona-drift (skip) no matching staged files\n│  debug-python (skip) no files for inspection\n│  design-md-lint (skip) no matching staged files\n┃  large-files ❯ \n\n\n┃  ddx-validate ❯ \n\n\n┃  skill-schema ❯ \n\n\n┃  sync-embedded-skills ❯ \n\n\n┃  conflicts ❯ \n\n\n┃  secrets ❯ \n\n\n┃  go-test ❯ \n\ngo: -race requires cgo; enable cgo by setting CGO_ENABLED=1\n\nexit status 2┃  go-fmt ❯ \n\n\n┃  go-lint ❯ \n\n0 issues.\n\n┃  evidence-lint ❯ \n\n\n┃  runtime-lint ❯ \n\n\n┃  go-build ❯ \n\ngithub.com/DocumentDrivenDX/ddx/tools/lint/evidencelint/cmd/evidencelint\ngithub.com/DocumentDrivenDX/ddx/tools/lint/routinglint/cmd/routinglint\ngithub.com/DocumentDrivenDX/ddx/tools/lint/runtimelint/cmd/runtimelint\ngithub.com/DocumentDrivenDX/ddx\ngithub.com/DocumentDrivenDX/ddx/tools/gendoc\n\n                                      \n  ────────────────────────────────────\nsummary: (done in 3.01 seconds)       \n✔️ large-files (0.01 seconds)\n✔️ ddx-validate (0.01 seconds)\n✔️ skill-schema (0.02 seconds)\n✔️ conflicts (0.02 seconds)\n✔️ secrets (0.03 seconds)\n✔️ sync-embedded-skills (0.02 seconds)\n✔️ go-fmt (0.09 seconds)\n✔️ go-lint (0.83 seconds)\n✔️ evidence-lint (0.90 seconds)\n✔️ runtime-lint (0.92 seconds)\n✔️ go-build (3.01 seconds)\n🥊 go-test (0.08 seconds): exit status 1\noutcome_reason=tests_red",
+          "created_at": "2026-05-07T19:03:28.882792083Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "execution_failed"
+        },
+        {
+          "actor": "erik",
+          "body": "{\"rationale\":\"All 8 criteria pass. (1) Title is imperative, names subsystem (agent/cost-cap) and the change (route through StopCondition + audit). (2) Description has all four required sections: PROBLEM, ROOT CAUSE WITH FILE:LINE (four specific line ranges across two files), PROPOSED FIX, NON-SCOPE, plus a DEPS rationale inline. (3) AC is numbered 1–8, every item is independently verifiable, and ACs 2–4/6 name specific Test* functions and a go test -run filter. (4) Final two AC lines are the canonical `cd cli \\u0026\\u0026 go test ./cmd/... ./internal/agent/...` gate followed by `lefthook run pre-commit passes`. (5) Labels include phase:2, three area: tags, kind:refactor, adr:024, spec:FEAT-010 cross-refs, and a reliability tag. (6) Parent (ddx-5cb6e6cd) and dep (ddx-89ab3fda) are explicit and the dep is explained in the description. (7) Evidence artifact uses `\\u003crun-id\\u003e` template path under .ddx/executions/, no /tmp or chat references anywhere. (8) Cold self-test passes: a competent agent can identify exact files and lines to edit, know which tests to write, know where to commit the audit report, and run the full gate without operator clarification.\",\"score\":8,\"suggested_fixes\":[],\"waivers_applied\":[]}",
+          "created_at": "2026-05-07T22:45:19.286861459Z",
+          "kind": "bead-quality.lint",
+          "source": "ddx agent execute-loop",
+          "summary": "score=8"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"resolved_provider\":\"claude\",\"resolved_model\":\"sonnet\",\"fallback_chain\":[]}",
+          "created_at": "2026-05-07T22:53:54.537905146Z",
+          "kind": "routing",
+          "source": "ddx agent execute-bead",
+          "summary": "provider=claude model=sonnet"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"attempt_id\":\"20260507T224521-041c0949\",\"harness\":\"claude\",\"model\":\"sonnet\",\"input_tokens\":29,\"output_tokens\":16391,\"total_tokens\":16420,\"cost_usd\":1.2055456,\"duration_ms\":510044,\"exit_code\":0}",
+          "created_at": "2026-05-07T22:53:54.60233608Z",
+          "kind": "cost",
+          "source": "ddx agent execute-bead",
+          "summary": "tokens=16420 cost_usd=1.2055 model=sonnet"
+        },
+        {
+          "actor": "erik",
+          "body": "{\"auto_merge_error\":\"advancing refs/heads/main to 38dff984a5a3eb28ddc94f7d71b4f49789dcee53: git update-ref refs/heads/main: fatal: update_ref failed for ref 'refs/heads/main': cannot lock ref 'refs/heads/main': is at 1dfaeba65fa44d369c2c8254f0e01a837d273336 but expected 38dff984a5a3eb28ddc94f7d71b4f49789dcee53: exit status 128\",\"base_rev\":\"3ddfcc22cc7145d1f49a0b0471edd7eeddc984af\",\"preserve_ref\":\"refs/ddx/iterations/ddx-b1cf1f6b/20260507T224521-041c0949-3ddfcc22cc71\",\"result_rev\":\"b5f8c83a8888bbd95a30bce660185e302311cbf2\",\"session_id\":\"eb-582f22ce\"}",
+          "created_at": "2026-05-07T22:53:59.373544553Z",
+          "kind": "land-conflict-unresolvable",
+          "source": "ddx agent try",
+          "summary": "preserved iteration could not be auto-recovered; parked for operator"
+        },
+        {
+          "actor": "ddx",
+          "body": "{\"actual_harness\":\"claude\",\"actual_model\":\"sonnet\",\"actual_power\":0,\"actual_provider\":\"\",\"attempt_id\":\"20260507T224521-041c0949\",\"bead_id\":\"ddx-b1cf1f6b\",\"rejected_route_pins\":null,\"requested_tier\":\"standard\",\"routing_intent_degraded\":false,\"routing_intent_note\":\"\",\"routing_intent_source\":\"heuristic\",\"smart_justification\":\"\"}",
+          "created_at": "2026-05-07T22:53:59.557617447Z",
+          "kind": "execution-routing-intent",
+          "source": "ddx agent execute-loop",
+          "summary": "source=heuristic tier=standard model=sonnet harness=claude"
+        },
+        {
+          "actor": "erik",
+          "body": "land_conflict_unresolvable: preserve_ref=refs/ddx/iterations/ddx-b1cf1f6b/20260507T224521-041c0949-3ddfcc22cc71\npreserve_ref=refs/ddx/iterations/ddx-b1cf1f6b/20260507T224521-041c0949-3ddfcc22cc71\nresult_rev=b5f8c83a8888bbd95a30bce660185e302311cbf2\nbase_rev=3ddfcc22cc7145d1f49a0b0471edd7eeddc984af\nretry_after=2026-05-07T23:08:59Z\noutcome_reason=unknown",
+          "created_at": "2026-05-07T22:53:59.619368259Z",
+          "kind": "execute-bead",
+          "source": "ddx agent execute-loop",
+          "summary": "land_conflict_unresolvable"
+        },
+        {
+          "actor": "erik",
+          "body": "{\"rationale\":\"All 8 rubric criteria met. (1) Title is imperative and names subsystem + change clearly. (2) Root cause cites four specific file:line ranges across two files. (3) AC is numbered 1–8 and names four Test* functions (two new, two regression). (4) AC 2 and 3 are wired-in assertions specifying the exact observable invariant (CloseWithEvidence not called; next bead not claimed). (5) AC 6 gives a targeted -run filter, AC 7 gives broad go test, AC 8 is lefthook gate — correct final-two-line pattern. (6) Labels include phase:2, area:agent/cost/work, kind:refactor, adr:024, spec:FEAT-010. (7) Parent ddx-5cb6e6cd and dep ddx-89ab3fda are explicit. (8) Description is self-contained: no /tmp paths, no chat references, dep rationale inlined in DEPS section.\",\"score\":8,\"suggested_fixes\":[],\"waivers_applied\":[]}",
+          "created_at": "2026-05-08T01:59:41.477146557Z",
+          "kind": "bead-quality.lint",
+          "source": "ddx agent execute-loop",
+          "summary": "score=8"
+        }
+      ],
+      "execute-loop-heartbeat-at": "2026-05-08T01:58:20.052458052Z",
+      "execute-loop-last-detail": "land_conflict_unresolvable: preserve_ref=refs/ddx/iterations/ddx-b1cf1f6b/20260507T224521-041c0949-3ddfcc22cc71",
+      "execute-loop-last-status": "land_conflict_unresolvable",
+      "execute-loop-retry-after": "2026-05-07T23:08:59Z",
+      "spec-id": "FEAT-010",
+      "split-from": "ddx-9228a484"
+    }
+  },
+  "governing": [
+    {
+      "id": "FEAT-010",
+      "path": "docs/helix/01-frame/features/FEAT-010-task-execution.md",
+      "title": "Feature: Task Execution"
+    }
+  ],
+  "paths": {
+    "dir": ".ddx/executions/20260508T015941-dfa83c66",
+    "prompt": ".ddx/executions/20260508T015941-dfa83c66/prompt.md",
+    "manifest": ".ddx/executions/20260508T015941-dfa83c66/manifest.json",
+    "result": ".ddx/executions/20260508T015941-dfa83c66/result.json",
+    "checks": ".ddx/executions/20260508T015941-dfa83c66/checks.json",
+    "usage": ".ddx/executions/20260508T015941-dfa83c66/usage.json",
+    "worktree": "var/tmp/ddx-exec-wt/.execute-bead-wt-ddx-b1cf1f6b-20260508T015941-dfa83c66"
+  },
+  "prompt_sha": "f7272ebb5d7bddcb7376f7a76b97aec8fa5e87287753949bda2bccfebc2e8296"
+}
\ No newline at end of file
diff --git a/.ddx/executions/20260508T015941-dfa83c66/result.json b/.ddx/executions/20260508T015941-dfa83c66/result.json
new file mode 100644
index 000000000..bf01c32fe
--- /dev/null
+++ b/.ddx/executions/20260508T015941-dfa83c66/result.json
@@ -0,0 +1,23 @@
+{
+  "bead_id": "ddx-b1cf1f6b",
+  "attempt_id": "20260508T015941-dfa83c66",
+  "base_rev": "a95284ee3ce0882c853dda1ac715596b2838dc0d",
+  "result_rev": "b95ae19c17944e119164216a205eea22451a0c18",
+  "outcome": "task_succeeded",
+  "status": "success",
+  "detail": "success",
+  "harness": "claude",
+  "model": "sonnet",
+  "session_id": "eb-6baab5ad",
+  "duration_ms": 483532,
+  "tokens": 8627,
+  "cost_usd": 0.7525864999999998,
+  "exit_code": 0,
+  "execution_dir": ".ddx/executions/20260508T015941-dfa83c66",
+  "prompt_file": ".ddx/executions/20260508T015941-dfa83c66/prompt.md",
+  "manifest_file": ".ddx/executions/20260508T015941-dfa83c66/manifest.json",
+  "result_file": ".ddx/executions/20260508T015941-dfa83c66/result.json",
+  "usage_file": ".ddx/executions/20260508T015941-dfa83c66/usage.json",
+  "started_at": "2026-05-08T01:59:45.483847234Z",
+  "finished_at": "2026-05-08T02:07:49.016449155Z"
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
