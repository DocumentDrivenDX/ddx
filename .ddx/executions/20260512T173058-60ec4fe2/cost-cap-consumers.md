# Cost Cap Consumer Audit

Generated from:

```bash
rg -n -i "cost cap" cli docs .agents/skills .claude/skills
rg -n -i "max-cost" cli docs .agents/skills .claude/skills
rg -n -i "review-cost-deferred" cli docs .agents/skills .claude/skills
rg -n -i "execution_failed:max-cost" cli docs .agents/skills .claude/skills
```

## `cost cap`

- `docs/helix/02-design/plan-2026-04-18-ddx-agent-service-interface.md:1035` - `- Per-task estimated cost is checked against remaining budget before invocation; tasks that would push over are skipped with explicit "skipped: cost cap" reason.`
- `docs/helix/02-design/plan-2026-04-29-visual-suite.md:297` - `6. **Cost cap $10 sequential** — realistic for 11+ Gemini-3-Pro-Image`
- `docs/helix/01-frame/features/FEAT-014-token-awareness.md:481` - `- Queue-drain budget stop policy and reviewer-cost cap handling, which are`
- `docs/helix/02-design/adr/ADR-024-power-escalation-and-review-routing.md:223` - `Every bead's escalation and auto-recovery attempts are bounded by a configurable per-bead cost cap. The cap applies to the sum of all implementation, review, reframer, and decomposer invocation costs for that bead.`
- `docs/helix/02-design/adr/ADR-024-power-escalation-and-review-routing.md:226` - `- The per-bead cost cap is configured in .ddx/config.yaml under escalation.per_bead_budget_usd. The default is project-specific; missing config means no per-bead cap is enforced beyond the drain-level cap in FEAT-014.`
- `docs/helix/02-design/adr/ADR-024-power-escalation-and-review-routing.md:234` - `- budget:<USD> — override the default per-bead cost cap for this specific bead. Example: budget:5.00 sets a $5.00 per-bead limit. The label value must be a decimal USD amount. Invalid or non-parseable values are ignored; DDx emits a malformed-budget-label warning but does not stop the drain.`
- `docs/helix/02-design/adr/ADR-024-power-escalation-and-review-routing.md:243` - `providers may be excluded from billed-cost caps by explicit cost-class metadata;`
- `docs/helix/02-design/adr/ADR-021-operator-prompt-beads-web-write-path.md:63` - `applicable), cost cap. None of these need to be reinvented for a`
- `docs/helix/02-design/adr/ADR-021-operator-prompt-beads-web-write-path.md:262` - `- All bead invariants apply. Cost caps, evidence ledger, preserve-on-`
- `docs/helix/01-frame/features/FEAT-010-task-execution.md:473` - `These axes do not interact with the drain-level cost cap (FEAT-014), the`
- `docs/helix/01-frame/features/FEAT-010-task-execution.md:1087` - `per-bead cost cap (escalation.per_bead_budget_usd in .ddx/config.yaml).`
- `docs/helix/02-design/solution-designs/SD-014-token-awareness.md:13` - `Fix token/cost capture from codex and claude harnesses by switching to`
- `cli/cmd/execute_loop_shared.go:268` - `Detail: fmt.Sprintf("cost cap reached: $%.2f billed >= $%.2f cap; raise the cap or set 0 to disable. Subscription and local providers do not count.", spent, spec.MaxCostUSD),`
- `cli/internal/server/workers_costcap_test.go:88` - `if !strings.Contains(r2.Detail, "cost cap reached") {`
- `cli/internal/agent/types.go:109` - `// strongest viable route instead of the worker attempt's cost cap.`
- `cli/internal/server/review_session.go:210` - `// Load the manifest to check the cost cap and read the current accumulated`
- `cli/internal/agent/execute_bead_post_review.go:110` - `// AC 1: Pre-dispatch cost cap check — enforce max_billable_usd before each`
- `cli/internal/agent/execute_bead_post_review.go:139` - `review cost cap exhausted: +class`
- `cli/internal/agent/execute_bead_post_review.go:140` - `review cost cap requires operator decision`
- `cli/internal/agent/execute_bead_post_review.go:194` - `_, _ = fmt.Fprintf(in.Log, "review cost cap deferred (%s %s): %s\n", in.Bead.ID, report.ResultRev, detail)`
- `cli/internal/server/workers.go:885` - `Detail: fmt.Sprintf("cost cap reached: $%.2f billed >= $%.2f cap; raise the cap or set 0 to disable. Subscription and local providers do not count.", spent, maxCostUSD),`
- `cli/internal/agent/execute_bead_loop.go:3531` - `// configured cost cap after charging the reviewer cost against the shared`
- `cli/internal/agent/execute_bead_loop_stop_test.go:109` - `Detail: "cost cap reached",`
- `cli/internal/agent/execute_bead_loop_stop_test.go:122` - `assert.Equal(t, "cost cap reached", result.Results[0].Detail)`
- `cli/internal/agent/execute_bead_review_retry_test.go:357` - `// drain-level cost cap is already exhausted before the pre-close review is`
- `cli/internal/agent/execute_bead_review_retry_test.go:366` - `// Build a cost cap tracker already past its limit so Tripped() is true.`
- `cli/internal/agent/execute_bead_review_retry_test.go:397` - `require.False(t, out.Approved, "bead must not be closed when cost cap is exceeded")`
- `cli/internal/agent/execute_bead_review_retry_test.go:398` - `assert.False(t, reviewerCalled, "reviewer must not be dispatched when cost cap is already tripped")`
- `cli/internal/agent/execute_bead_review_retry_test.go:411` - `"cost cap exceeded before dispatch must append a review-error event")`
- `cli/internal/agent/execute_bead_review_retry_test.go:421` - `"cost cap exceeded must not close the bead")`
- `cli/internal/escalation/infrastructure.go:231` - `return fmt.Sprintf("cost cap reached: $%.2f billed >= $%.2f cap; raise the cap or set 0 to disable. Subscription and local providers do not count.", spent, t.MaxUSD), true`

## `max-cost`

- `docs/helix/02-design/plan-2026-04-18-ddx-agent-service-interface.md:1034` - `` `--max-cost-usd <N>` flag (default $0.50) — bench halts the sweep if accumulated cost exceeds the cap. Any harness mid-sweep when the cap is hit completes its current task, then no further candidates run. ``
- `docs/helix/06-iterate/parallel-drain-playbook.md:30` - `ddx work --once --harness claude --max-cost 25`
- `docs/helix/06-iterate/parallel-drain-playbook.md:33` - `ddx work --once --harness codex --max-cost 25`
- `docs/helix/06-iterate/parallel-drain-playbook.md:36` - `ddx work --once --harness openrouter --max-cost 25`
- `docs/helix/06-iterate/parallel-drain-playbook.md:46` - `Each ddx work invocation accepts --max-cost, and that ceiling applies per`
- `cli/cmd/execute_loop_shared.go:58` - `maxCostUSD, _ := cmd.Flags().GetFloat64("max-cost")`
- `cli/cmd/work_test.go:60` - `setFlag("max-cost", "12.5")`
- `cli/cmd/work.go:92` - `cmd.Flags().Float64("max-cost", escalation.DefaultMaxCostUSD, "Stop when accumulated billed cost exceeds USD; 0 = unlimited")`

## `review-cost-deferred`

- `cli/internal/agent/execute_bead_review_test.go:307` - `if ev.Kind == "review-cost-deferred" {`
- `cli/internal/agent/execute_bead_review_test.go:357` - `if ev.Kind == "review-cost-deferred" {`
- `cli/internal/agent/execute_bead_post_review.go:186` - `Kind:      "review-cost-deferred",`
- `cli/internal/agent/execute_bead_post_review.go:187` - `Summary:   "review-cost-deferred",`
- `cli/internal/agent/execute_bead_loop_stop_test.go:288` - `if ev.Kind == "review-cost-deferred" {`
- `cli/internal/agent/execute_bead_loop_stop_test.go:293` - `assert.True(t, foundDeferred, "review-cost-deferred event must be recorded when budget exceeded by reviewer cost")`

## `execution_failed:max-cost`

- No matches under `cli/`, `docs/`, `.agents/skills/`, or `.claude/skills/`.
