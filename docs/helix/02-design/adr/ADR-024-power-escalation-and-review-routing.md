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

## Context

DDx now has three overlapping concerns in the bead execution path:

- retrying a bead attempt when stronger reasoning could plausibly help
- running a post-merge reviewer that is independent from the implementer
- enforcing cost visibility and budget stops without taking ownership of
  provider/model routing

The upstream agent service owns harness/provider/model routing. DDx owns bead
state, worktree isolation, evidence, gates, review verdicts, cooldowns, and
the queue-drain loop. The escalation policy therefore has to be expressed in
terms of DDx-owned evidence and abstract power bounds, not concrete model names.

Earlier planning used "tier" and profile language. That vocabulary is now
migration debt when it implies DDx choosing a provider, model, or fallback chain.
The stable contract is `MinPower` / `MaxPower` plus opaque passthrough
constraints.

## Decision

DDx power escalation is a DDx retry-policy decision expressed only by changing
the next request's `MinPower`. DDx does not choose, substitute, downgrade, or
fallback concrete harnesses, providers, or models.

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
from the agent's catalog power numbers. It may skip catalog power floors that
have no viable auto-routable model, but it still sends only a new `MinPower`
bound to the agent. The agent chooses the concrete route and reports actual
model, provider, and power.

### Passthrough Stickiness

Operator-supplied passthrough constraints are sticky across every retry.

If the operator supplied `--harness`, `--provider`, or `--model`, DDx forwards
the same values unchanged. If those constraints make a later power request
unsatisfiable, DDx records a typed terminal classification such as
`blocked_by_passthrough_constraint` or `agent_power_unsatisfied` and reports
operator action required. DDx must not remove pins, widen pins, substitute a
fallback route, call `ResolveRoute` to work around the conflict, or loop over
concrete route names.

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

### Reviewer Routing

Post-merge review is a separate review-mode invocation over assembled evidence.
The reviewer must run in TD-033 no-tool reviewer mode.

DDx requests a reviewer that is stronger than the implementer by using the
implementer's actual power as evidence and setting reviewer `MinPower` to a
higher floor. DDx also supplies structured correlation facts:

- `role=reviewer`
- `bead_id`
- `attempt_id`
- `session_id`
- `result_rev`
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

Reviewer output must satisfy the structured verdict contract. `APPROVE` closes
the bead. `REQUEST_CHANGES` and `BLOCK` reopen the bead with review evidence and
feed the review-triage ladder. Malformed output, empty provider output,
context-overflow, and transport failure are `review-error` classes, scoped to
the reviewed `result_rev`.

Review errors retry the review path up to `review_max_retries` for the same
`result_rev`. On exhaustion, DDx emits `review-manual-required` and parks the
bead for operator review without closing it. A new implementation result starts
a new review-error retry scope.

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
- FEAT-010 owns retry scheduling, stop-condition evaluation, no-progress
  handling, review retry scopes, and budget stops.
- FEAT-014 owns normalized usage, cost, cost-class, and freshness semantics.
- FEAT-022 and TD-033 own review evidence assembly and no-tool reviewer mode.
- DDx routing lint should treat concrete route mutation in `run`, `try`, or
  `work` policy as a regression.

## Non-Goals

- Specifying Fizeau's internal routing, fallback, provider-health, or model
  scoring algorithm.
- Guaranteeing a different reviewer provider. DDx can request stronger review
  and record degradation, not force a route.
- Retrying by concrete model name or profile mutation.
- Adding a fourth run layer or a review-specific execution substrate.

## References

- `docs/helix/01-frame/features/FEAT-006-agent-service.md`
- `docs/helix/01-frame/features/FEAT-010-executions.md`
- `docs/helix/01-frame/features/FEAT-014-token-awareness.md`
- `docs/helix/01-frame/features/FEAT-022-prompt-evidence-assembly.md`
- `docs/helix/02-design/technical-designs/TD-033-multi-turn-structured-evidence-assembly.md`
