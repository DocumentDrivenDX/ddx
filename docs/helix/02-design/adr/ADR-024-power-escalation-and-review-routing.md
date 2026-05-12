---
ddx:
  id: ADR-024
  depends_on:
    - FEAT-006
    - FEAT-010
    - FEAT-014
    - FEAT-022
---
# ADR-024: Power Escalation and Review Routing Policy

**Status:** Accepted
**Date:** 2026-05-06
**Authors:** bead `ddx-5f1eac4f`

## Principles

The escalation and auto-recovery design is anchored by five explicit principles:

- **P1 — Cheapest first:** DDx always dispatches the lowest-power model that can plausibly handle the work. Power escalation is a recovery step, not a default operating mode.
- **P2 — Escalate on failure:** When a capability-sensitive failure class is detected, DDx raises `MinPower` for the next attempt. It does not alter harness, provider, or model values; Fizeau resolves the concrete route.
- **P3 — Reviewer always stronger:** The adversarial pre-close reviewer's `MinPower` floor is always set higher than the implementer's actual reported power. A reviewer may not run at a weaker power than the implementer it is reviewing.
- **P4 — Reframe then decompose on persistent failure:** When a bead's escalation ladder is exhausted on consecutive drain cycles, DDx first attempts a strong-tier reframer pass. If reframing fails or produces no change, DDx attempts a strong-tier decomposer pass. Only after both fail does DDx park the bead at `status=proposed`.
- **P5 — No human in the loop for routing or budget:** DDx stops at `status=proposed` or at a configured budget cap; it never silently retries indefinitely or makes content-aware routing decisions. Operator action is a terminal state reached only after automated escalation and auto-recovery have been exhausted.

## Context

DDx now has three overlapping concerns in the bead execution path:

- retrying a bead attempt when stronger reasoning could plausibly help
- running an adversarial pre-close reviewer gate that is independent from the
  implementer
- enforcing cost visibility and budget stops without taking ownership of
  provider/model routing

Fizeau owns harness/provider/model routing. DDx owns bead
state, worktree isolation, evidence, gates, review verdicts, cooldowns, and
the queue-drain loop. The escalation policy therefore has to be expressed in
terms of DDx-owned evidence and abstract power bounds, not concrete model names.

Earlier planning used "tier" and profile language. That vocabulary is now
migration debt when it implies DDx choosing a provider, model, or fallback chain.
The stable contract is `MinPower` / `MaxPower` plus opaque passthrough
constraints.

## Decision

DDx power escalation is a DDx retry-policy decision expressed only by changing
the next request's `MinPower` and carrying the same request facts forward.
DDx only changes `MinPower` and carries request facts forward; Fizeau chooses
the concrete route.

### Primary Attempt Escalation

For a primary bead attempt, DDx may schedule a higher-power retry only when all
of the following are true:

1. a valid attempt started and produced DDx-owned evidence;
2. the outcome is classified as plausibly capability-sensitive; and
3. no stop condition, hard passthrough constraint, budget cap, cooldown, or
   operator-action class has already taken ownership of the result.

Eligible capability-sensitive classes include failed post-run checks,
implementation-quality failures, review blocks that identify fixable reasoning
or design gaps, and no-changes-after-attempt when the model had valid context.

Ineligible classes include routing/passthrough conflicts, missing tools,
authentication or quota failures, invalid bead metadata, dirty worktree, hard
merge/land conflicts that need human judgment, command-not-found setup failures,
and any failure where retry would require DDx to inspect or mutate
`--harness`, `--provider`, or `--model`.

When a higher-power retry is allowed, DDx computes the next `MinPower` floor
from Fizeau's catalog power numbers. It may skip catalog power floors that have
no viable auto-routable model, but it still sends only a new `MinPower` bound
to Fizeau. Fizeau chooses the concrete route and reports actual model,
provider, and power.

### Passthrough Stickiness

Operator-supplied passthrough constraints are sticky across every retry.

If the operator supplied `--harness`, `--provider`, or `--model`, DDx forwards
the same values unchanged. If those constraints make a later power request
unsatisfiable, DDx records a typed terminal classification such as
`blocked_by_passthrough_constraint` or `agent_power_unsatisfied` and reports
operator action required. DDx must not remove pins, widen pins, substitute a
fallback route, call any route-selection helper to work around the conflict, or
loop over concrete route names.

### Infrastructure And Rate-Limit Fallback

Infrastructure failures are not capability failures. Provider 5xx responses,
network unreachability, command-not-found, authentication failures, quota
exhaustion, and analogous transport/setup failures do not consume escalation
budget. DDx records the failure and either leaves the bead immediately
reclaimable or applies a retry-after cooldown only when time passing could
plausibly fix the same attempt class.

HTTP 429 / rate-limit handling is internal to one attempt. The claim stays held,
DDx honors a parseable `Retry-After` when present, otherwise uses bounded
exponential backoff, and emits `rate-limit-retry` evidence. When the retry
budget is exhausted, the attempt falls back to the ordinary `execution_failed`
mapping.

### Default Adversarial Pre-Close Review Gate

Every close-eligible implementation result passes through an adversarial
pre-close review gate before DDx mutates the bead to `closed`. Review is enabled
by default. Disabling it is an explicit operator override and must record an
auditable reason.

The gate runs after the implementation attempt has produced stable evidence
(`base_rev`, `result_rev`, diff, verification command output, bead metadata, and
governing artifacts), and before the durable close mutation. The candidate
commit may already exist in git so the reviewer can inspect a stable
`result_rev`; the bead is not closed until the review gate approves that result.

The default gate dispatches two independent reviewer invocations. Each reviewer
runs as a **read-only tool reviewer** in the still-live attempt worktree, before
land. The bounded evidence bundle (candidate diff, acceptance criteria, governing
artifacts) is the canonical review input; same-worktree read-only tool access
(file reads, searches) is supplemental and does not override the bounded bundle.
The reviewer cannot write files or create commits. Each reviewer must return a
structured verdict with per-acceptance-criterion evidence.

**Transitional single-slot allowance.** Until the two-slot quorum reviewer
implementation lands, a single reviewer slot satisfies the gate. The removal
condition is the landing of the bead that implements two-slot quorum aggregation.

DDx requests a reviewer that is stronger than the implementer by using the
implementer's actual power as evidence and setting reviewer `MinPower` to a
higher floor. DDx also supplies structured correlation facts:

- `role=reviewer`
- `bead_id`
- `attempt_id`
- `session_id`
- `result_rev`
- `review_group_id`
- `reviewer_index`
- implementer harness/provider/model/power when known

These fields are Day-1 observability. DDx records them so operators and metrics
can see whether review pairing degraded. Future agent-side routing
intelligence may use `Role` and correlation internally, but DDx does not
specify that algorithm and must not depend on it.

Different-provider review is best effort. If the agent returns a reviewer on
the same provider as the implementer, DDx emits a
`review-pairing-degraded` event. That event affects triage bias and operator
visibility; it is not by itself a review failure.

### Review Outcomes And Retry

Reviewer output must satisfy the structured verdict contract. A valid approval
requires per-AC evidence; an `APPROVE` without evidence is malformed, not a
pass. The aggregate gate is conservative:

- unanimous reviewer `APPROVE` with per-AC evidence permits close;
- any evidenced `REQUEST_CHANGES` or `BLOCK` prevents close;
- reviewer disagreement is preserved in the review group evidence, and the
  non-approve verdict wins when it cites evidence;
- malformed, empty, context-overflow, and transport failures are
  `review-error` classes scoped to the reviewed `result_rev` and reviewer.

Non-approve reviewer findings are classified before the next action:

- `review_fixable_gap` — implementation or test work is missing, incomplete, or
  erroneous. DDx schedules a new implementation cycle on the same bead when
  retry budgets allow. The next implementation prompt includes the review
  findings as required repair context. DDx may raise `MinPower` only when the
  finding is plausibly capability-sensitive.
- `review_spec_gap` / `review_missing_acceptance` — the bead or governing spec
  is ambiguous, unverifiable, contradictory, or missing acceptance criteria. DDx
  moves the bead to `status=proposed`, clears the active claim, and does not ask
  another implementer to guess. The operator resolves that state by retry,
  split, obsolete, or defer actions recorded in the bead history.
- `review_too_large` — the result or bead is too broad for bounded review. DDx
  runs the intake/decomposition path, decomposes the parent when lossless child
  work is possible, or moves it to `status=proposed` when operator input is
  required. It does not re-run the same monolithic implementation attempt.
- `review_unsafe_or_out_of_scope` — the implementation changed forbidden scope,
  removed checks, weakened behavior, or otherwise needs explicit repair. If the
  repair is mechanical it follows `review_fixable_gap`; otherwise it moves to
  `status=proposed`.

Review errors retry the review path up to `review_max_retries_per_candidate` for
the same candidate ref (`result_rev`) and reviewer slot. On exhaustion, DDx
emits `review-manual-required`, clears the active claim, moves the bead to
`status=proposed`, and leaves it for operator review without closing it. This
terminal resolution is the same contract used by `review_spec_gap` and
`review_missing_acceptance`: the bead carries the human-review context and waits
for an operator decision rather than another automatic implementation attempt. A
new candidate ref (from a repair cycle or a fresh `ddx try`) resets this counter
independently; it is strictly per-candidate. A new implementation result starts a
new review-error retry scope, while
`review_fixable_gap` continues through the normal repair retry path counted
against `repair_max_cycles` instead of entering the operator lane.

Each implementation/review cycle is append-only. Repair commits in the worktree
are also append-only — no `git reset`, `git commit --amend`, `git squash`, or
`git rebase` against the prior candidate is permitted in a repair cycle. A
repair attempt creates a new cycle record linked to the prior review group
(`repair_context_from_review_group`) and records its own `base_rev`,
`result_rev`, implementer run ids, verification output, reviewer run ids,
aggregate verdict, retry/decomposition/block decision, and cost summary. No
cycle overwrites prior evidence.

### Escalation Sequencing

DDx applies escalation in a strict priority order within and across drain cycles:

1. **Within-cycle (ladder climb):** For a single bead execution, DDx escalates `MinPower` across successive retries until the configured power ceiling is reached or a stop condition fires. Each retry is a new layer-1 invocation in the same layer-2 attempt record.
2. **Cross-cycle (reframe):** When the escalation ladder is exhausted on `consecutive_ladder_exhaustions >= 2` drain cycles, DDx dispatches a strong-tier reframer agent (per P3, `MinPower` set to the strong-tier floor). The reframer rewrites the bead description and/or acceptance criteria in-place; the bead re-enters the execution-ready queue with `status=open` and a reset ladder.
3. **Cross-cycle (decompose):** If the reframe attempt fails or produces no change, DDx dispatches a strong-tier decomposer agent to split the bead into 2–5 executable child beads. If child depth is exhausted, the decomposer switches to sibling or replacement bead specs under the nearest safe parent/root. The oversized bead is left `status=open` with dependency, `execution-eligible=false`, or supersession metadata that lets the queue advance through the generated executable work.
4. **Final escape (`status=proposed`):** If child decomposition and sibling/replacement decomposition both fail, or the decomposition would be lossy and require operator judgment, DDx parks the bead at `status=proposed` with `auto-recovery-failed` evidence and clears the active claim.

No step may be skipped or reordered. DDx must not move a bead directly to `status=proposed` from within-cycle escalation while cross-cycle options remain available and the per-bead budget has not been exhausted.

### Per-Bead Budget

Every bead's escalation and auto-recovery attempts are bounded by a configurable per-bead cost cap. The cap applies to the sum of all implementation, review, reframer, and decomposer invocation costs for that bead.

- When cumulative per-bead cost exceeds the configured limit, DDx records outcome `per_bead_budget_exhausted` (TD-031 §2), appends the total cost to the event body, releases the claim without cooldown, and leaves the bead `status=open` and re-claimable. The budget exhaustion is a recheckable signal, not a terminal state: an operator may raise the cap or the bead may be retried in a later drain when conditions change.
- The per-bead cost cap is configured in `.ddx/config.yaml` under `escalation.per_bead_budget_usd`. The default is project-specific; missing config means no per-bead cap is enforced beyond the drain-level cap in FEAT-014.
- Per-bead budget exhaustion MUST NOT set `execute-loop-retry-after`; the cause is not time-resolvable without explicit operator action or config change.

### Operator Escape Hatches

Two bead labels modify the default escalation and auto-recovery behavior:

- **`recovery:manual`** — skip all automatic cross-cycle recovery steps (reframe, decompose). When this label is present, exhausting the within-cycle ladder moves the bead directly to `status=proposed` without attempting reframe or decompose. This is the operator's signal that they want to review the bead before any structural changes are made.
- **`budget:<USD>`** — override the default per-bead cost cap for this specific bead. Example: `budget:5.00` sets a $5.00 per-bead limit. The label value must be a decimal USD amount. Invalid or non-parseable values are ignored; DDx emits a `malformed-budget-label` warning but does not stop the drain.

These labels are read at claim time, before each retry decision, and before each auto-recovery dispatch. Operators may add or remove them between drain cycles.

### Cost Accounting

Primary attempts and review attempts both report usage and cost into the same
run evidence stream. FEAT-014 defines the normalized cost signal fields.
FEAT-010 owns the queue-drain budget stop. Subscription-bundled and local
providers may be excluded from billed-cost caps by explicit cost-class metadata;
unknown cost class counts by default.

Reviewer cost is not special. When a review invocation reports billable cost,
that cost contributes to the same drain-level cap as implementation attempts.
If the cap is reached, DDx stops claiming more work and records an observable
budget stop instead of disguising the stop as model failure.

## Consequences

- FEAT-006 owns request construction: `MinPower` / `MaxPower`, passthrough
  envelope, role/correlation metadata, and actual power recording.
- FEAT-010 owns intake, retry scheduling, stop-condition evaluation,
  no-progress handling, adversarial review aggregation, review retry scopes, and
  budget stops.
- FEAT-014 owns normalized usage, cost, cost-class, and freshness semantics.
- FEAT-022 and TD-033 own review evidence assembly and read-only tool reviewer mode.
- DDx routing lint should treat concrete route mutation in `run`, `try`, or
  `work` policy as a regression.

## Non-Goals

- Specifying Fizeau's internal routing, fallback, provider-health, or model
  scoring algorithm.
- Guaranteeing a different reviewer provider or model. DDx can request stronger
  review, dispatch independent reviewer slots, and record degradation, not force
  a route.
- Retrying by concrete model name or profile mutation.
- Adding a fourth run layer or a review-specific execution substrate.

## References

- `docs/helix/01-frame/features/FEAT-006-agent-service.md`
- `docs/helix/01-frame/features/FEAT-010-task-execution.md`
- `docs/helix/01-frame/features/FEAT-014-token-awareness.md`
- `docs/helix/01-frame/features/FEAT-022-prompt-evidence-assembly.md`
- `docs/migrations/routing-config.md`
- `docs/helix/02-design/technical-designs/TD-033-multi-turn-structured-evidence-assembly.md`
