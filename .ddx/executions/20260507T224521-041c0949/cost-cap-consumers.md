# Cost-Cap Consumer Audit

Run: `20260507T224521-041c0949`  Bead: `ddx-b1cf1f6b`

Search terms: `cost cap`, `max-cost`, `review-cost-deferred`, `execution_failed:max-cost`
Search roots: `cli/`, `docs/`, `.agents/skills/`, `.claude/skills/`

---

## "cost cap"

```
cli/cmd/agent_cmd.go:1692                           Detail: fmt.Sprintf("cost cap reached: ...")
cli/cmd/agent_execute_loop_costcap_test.go:64       // TestExecuteLoopCostCap_ShortCircuitsAfterCap asserts the cost cap halts
cli/cmd/agent_execute_loop_costcap_test.go:100      if !strings.Contains(r2.Detail, "cost cap reached") {
cli/internal/agent/execute_bead_loop.go:2233        // configured cost cap after charging the reviewer cost against the shared
cli/internal/agent/execute_bead_loop_stop_test.go:100  Detail: "cost cap reached",
cli/internal/agent/execute_bead_loop_stop_test.go:111  assert.Equal(t, "cost cap reached", result.Results[0].Detail)
cli/internal/agent/execute_bead_post_review.go:135  fmt.Fprintf(in.Log, "review cost cap deferred (%s %s): %s\n", ...)
cli/internal/agent/types.go:98                     // strongest viable route instead of the worker attempt's cost cap.
cli/internal/escalation/infrastructure.go:205      return fmt.Sprintf("cost cap reached: $%.2f billed >= $%.2f cap; ..."), true
cli/internal/server/workers.go:844                 Detail: fmt.Sprintf("cost cap reached: $%.2f billed >= $%.2f cap; ...")
cli/internal/server/workers_costcap_test.go:88     if !strings.Contains(r2.Detail, "cost cap reached") {
docs/helix/01-frame/features/FEAT-010-task-execution.md:426   drain-level cost cap (FEAT-014)
docs/helix/01-frame/features/FEAT-014-token-awareness.md:481  Queue-drain budget stop policy and reviewer-cost cap handling
docs/helix/02-design/adr/ADR-021-operator-prompt-beads-web-write-path.md:63   applicable), cost cap.
docs/helix/02-design/adr/ADR-024-power-escalation-and-review-routing.md:205  providers may be excluded from billed-cost caps
docs/helix/02-design/plan-2026-04-18-ddx-agent-service-interface.md:1030  "skipped: cost cap" reason.
.agents/skills/docs/helix/02-design/plan-2026-04-18-ddx-agent-service-interface.md:1005  "skipped: cost cap" reason.
.agents/skills/docs/helix/02-design/solution-designs/SD-014-token-awareness.md:13   Fix token/cost capture
.claude/skills/docs/helix/02-design/plan-2026-04-18-ddx-agent-service-interface.md:1005  "skipped: cost cap" reason.
.claude/skills/docs/helix/02-design/solution-designs/SD-014-token-awareness.md:13   Fix token/cost capture
```

## "max-cost"

```
cli/cmd/agent_cmd.go:1520           maxCostUSD, _ := cmd.Flags().GetFloat64("max-cost")
cli/cmd/agent_cmd.go:1661           // providers) above --max-cost trips the cap
cli/cmd/work.go:83                  cmd.Flags().Float64("max-cost", escalation.DefaultMaxCostUSD, ...)
docs/helix/02-design/plan-2026-04-18-ddx-agent-service-interface.md:1029  --max-cost-usd <N> flag
.agents/skills/docs/helix/02-design/plan-2026-04-18-ddx-agent-service-interface.md:1004  --max-cost-usd <N> flag
.agents/skills/website/content/docs/cli/commands/ddx_agent_execute-loop.md:86  --max-cost float
.agents/skills/website/content/docs/cli/commands/ddx_work.md:89            --max-cost float
.claude/skills/docs/helix/02-design/plan-2026-04-18-ddx-agent-service-interface.md:1004  --max-cost-usd <N> flag
.claude/skills/website/content/docs/cli/commands/ddx_agent_execute-loop.md:86  --max-cost float
.claude/skills/website/content/docs/cli/commands/ddx_work.md:89            --max-cost float
```

## "review-cost-deferred"

```
cli/internal/agent/execute_bead_loop_stop_test.go:203   if ev.Kind == "review-cost-deferred" {
cli/internal/agent/execute_bead_loop_stop_test.go:208   assert.True(t, foundDeferred, ...)
cli/internal/agent/execute_bead_post_review.go:127      Kind:      "review-cost-deferred",
cli/internal/agent/execute_bead_post_review.go:128      Summary:   "review-cost-deferred",
cli/internal/agent/execute_bead_review_test.go:208      if ev.Kind == "review-cost-deferred" {
cli/internal/agent/execute_bead_review_test.go:258      if ev.Kind == "review-cost-deferred" {
```

## "execution_failed:max-cost"

```
(no hits)
```

The string `execution_failed:max-cost` does not appear in the codebase. Cost-cap stops formerly used only a synthetic `ExecuteBeadStatusExecutionFailed` report. After ddx-89ab3fda the loop termination is routed through `work.ClassifyStop(StopInput{Budget: true})` producing `StopConditionBudget` / `exit_reason="budget"`. The report is still appended for result telemetry, but no consumer encodes the literal `execution_failed:max-cost` string.

---

## Key production consumers

| File | Role |
|------|------|
| `cli/cmd/agent_cmd.go:1685` | `costCapTripped` closure — BudgetStop callback for CLI drain |
| `cli/internal/escalation/infrastructure.go:205` | `CostCapTracker.Tripped()` — canonical detail string generator |
| `cli/internal/agent/execute_bead_loop.go:582` | `BudgetStop` check — routes through `work.ClassifyStop{Budget:true}` |
| `cli/internal/agent/execute_bead_post_review.go:116` | `chargeReviewCost()` — accumulates reviewer slot cost, emits review-cost-deferred |
| `cli/internal/server/workers.go:844` | Server-side BudgetStop callback (parallel to CLI) |
