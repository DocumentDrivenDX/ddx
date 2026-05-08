# Cost-Cap Consumer Audit

bead: ddx-b1cf1f6b  
run-id: 20260508T015941-dfa83c66  
generated: 2026-05-08

Audit of every `rg` hit for the four cost-cap event and detail-string tokens
under `cli/`, `docs/`, `.agents/skills/`, and `.claude/skills/`.

---

## Term: "cost cap"

| File | Line | Content |
|------|------|---------|
| `cli/cmd/agent_cmd.go` | 1692 | `Detail: fmt.Sprintf("cost cap reached: $%.2f billed >= $%.2f cap; ...")` — synthetic `ExecuteBeadStatusExecutionFailed` report returned from `costCapTripped` closure, consumed by `BudgetStop` in the loop |
| `cli/internal/escalation/infrastructure.go` | 205 | `return fmt.Sprintf("cost cap reached: ...")` — `CostCapTracker.Tripped()` detail string; canonical operator-facing message |
| `cli/internal/agent/execute_bead_loop.go` | 2241 | Comment: "configured cost cap after charging the reviewer cost against the shared" |
| `cli/internal/agent/execute_bead_post_review.go` | 142 | `fmt.Fprintf(in.Log, "review cost cap deferred (%s %s): %s\n", ...)` — operator log line inside `chargeReviewCost()` closure |
| `cli/internal/agent/execute_bead_loop_stop_test.go` | 100, 111 | Test fixtures using `"cost cap reached"` as expected detail string |
| `cli/internal/agent/types.go` | 98 | Comment: "strongest viable route instead of the worker attempt's cost cap." |
| `cli/cmd/agent_execute_loop_costcap_test.go` | 64, 100 | `TestExecuteLoopCostCap_ShortCircuitsAfterCap` docstring and assertion on detail |
| `cli/internal/server/workers.go` | 844 | `Detail: fmt.Sprintf("cost cap reached: ...")` — server-side worker `costCapTripped` closure (mirrors CLI) |
| `cli/internal/server/workers_costcap_test.go` | 88 | Server-side test assertion on `"cost cap reached"` detail string |
| `docs/helix/02-design/adr/ADR-024-power-escalation-and-review-routing.md` | 205 | Mentions billed-cost cap exclusion via cost-class metadata |
| `docs/helix/02-design/adr/ADR-021-operator-prompt-beads-web-write-path.md` | 63 | References cost cap in server-side worker context |
| `docs/helix/02-design/plan-2026-04-18-ddx-agent-service-interface.md` | 1030 | Agent-service plan: per-task estimated cost checked against remaining budget |
| `docs/helix/02-design/solution-designs/SD-014-token-awareness.md` | 13 | SD-014 mentions token/cost capture |
| `docs/helix/01-frame/features/FEAT-010-task-execution.md` | 443 | FEAT-010: cost cap non-interaction note |
| `docs/helix/01-frame/features/FEAT-014-token-awareness.md` | 481 | FEAT-014: drain-level budget stop and reviewer-cost cap cross-reference |
| `.agents/skills/docs/helix/02-design/plan-2026-04-18-ddx-agent-service-interface.md` | 1005 | Skill copy of plan doc |
| `.agents/skills/docs/helix/02-design/solution-designs/SD-014-token-awareness.md` | 13 | Skill copy of SD-014 |
| `.claude/skills/docs/helix/02-design/plan-2026-04-18-ddx-agent-service-interface.md` | 1005 | Skill copy of plan doc |
| `.claude/skills/docs/helix/02-design/solution-designs/SD-014-token-awareness.md` | 13 | Skill copy of SD-014 |

---

## Term: "max-cost"

| File | Line | Content |
|------|------|---------|
| `cli/cmd/agent_cmd.go` | 1520 | `maxCostUSD, _ := cmd.Flags().GetFloat64("max-cost")` — reads the `--max-cost` flag value |
| `cli/cmd/agent_cmd.go` | 1661 | Comment: "providers) above --max-cost trips the cap and halts further bead" |
| `cli/cmd/work.go` | 83 | `cmd.Flags().Float64("max-cost", escalation.DefaultMaxCostUSD, ...)` — flag definition on `ddx work` |
| `docs/helix/02-design/plan-2026-04-18-ddx-agent-service-interface.md` | 1029 | `--max-cost-usd <N>` flag description in agent-service plan |
| `.agents/skills/docs/helix/02-design/plan-2026-04-18-ddx-agent-service-interface.md` | 1004 | Skill copy |
| `.agents/skills/website/content/docs/cli/commands/ddx_work.md` | 89 | Generated CLI docs: `--max-cost float` description |
| `.agents/skills/website/content/docs/cli/commands/ddx_agent_execute-loop.md` | 86 | Generated CLI docs: `--max-cost float` description |
| `.claude/skills/website/content/docs/cli/commands/ddx_work.md` | 89 | Skill copy of generated CLI docs |
| `.claude/skills/website/content/docs/cli/commands/ddx_agent_execute-loop.md` | 86 | Skill copy of generated CLI docs |
| `.claude/skills/docs/helix/02-design/plan-2026-04-18-ddx-agent-service-interface.md` | 1004 | Skill copy |

---

## Term: "review-cost-deferred"

| File | Line | Content |
|------|------|---------|
| `cli/internal/agent/execute_bead_post_review.go` | 134–135 | `Kind: "review-cost-deferred"` and `Summary: "review-cost-deferred"` — event emitted inside `chargeReviewCost()` when reviewer cost trips the cap during an APPROVE or error path |
| `cli/internal/agent/execute_bead_loop_stop_test.go` | 203, 208 | `TestStopCondition_BudgetAfterReviewCostPreventsClose` (AC2): asserts `review-cost-deferred` event is recorded and bead stays open |
| `cli/internal/agent/execute_bead_review_test.go` | 208, 258 | `TestRunPostMergeReviewChargesReviewCostAndDefersWhenCapTrips` and `TestRunPostMergeReviewChargesReviewCostOnReviewerError`: assert event is emitted on both APPROVE and error paths |

---

## Term: "execution_failed:max-cost"

No hits in `cli/`, `docs/`, `.agents/skills/`, or `.claude/skills/`.

This token does not appear anywhere in the codebase. The implementation uses
`ExecuteBeadStatusExecutionFailed` as the report status and the "cost cap
reached: …" detail string (from `CostCapTracker.Tripped()`) rather than a
composite `execution_failed:max-cost` status string. The `BudgetStop` callback
and `StopCondition Budget` routing do not use a colon-separated compound value.

---

## Summary: Routing Through StopCondition Budget

The cost-cap stop path after ddx-b1cf1f6b:

1. `--max-cost` flag → `escalation.NewCostCapTracker(maxCostUSD, lookup)` in `agent_cmd.go`
2. Implementer cost accumulated via `costCap.Add(harness, costUSD)` after each attempt
3. Reviewer cost accumulated via `capTracker.Add(slot.Result.ReviewerHarness, slot.Result.CostUSD)` inside `chargeReviewCost()` in `execute_bead_post_review.go`
4. `BudgetStop` callback (`costCapTripped`) set on `ExecuteBeadLoopRuntime`; checked at top of every loop iteration
5. When tripped, `applyStop(work.StopInput{Budget: true})` → `StopConditionBudget` → loop exits cleanly
6. On APPROVE + cap exceeded: `Approved=false`, `review-cost-deferred` event recorded, bead stays open; `BudgetStop` fires on next iteration
