# Cost-Cap Consumer Audit — ddx-b1cf1f6b

Produced by `rg` over `cli/`, `docs/`, `.agents/skills/`, `.claude/skills/`.

---

## 1. "cost cap" (operator-facing message string)

| File | Line | Notes |
|------|------|-------|
| `cli/internal/escalation/infrastructure.go` | 205 | `CostCapTracker.Tripped()` formats the canonical detail string |
| `cli/cmd/agent_cmd.go` | 1692 | `costCapTripped` closure — feeds `BudgetStop`; post-attempt check removed by this bead |
| `cli/internal/server/workers.go` | 844 | `costCapTripped` in server worker — still carries pre+post-attempt check pattern (out-of-scope for this bead) |
| `docs/helix/02-design/adr/ADR-021-operator-prompt-beads-web-write-path.md` | 63 | Design reference only |
| `docs/helix/02-design/plan-2026-04-18-ddx-agent-service-interface.md` | 1030 | Design reference only |
| `.agents/skills/docs/helix/02-design/plan-2026-04-18-ddx-agent-service-interface.md` | 1005 | Skill copy |
| `.claude/skills/docs/helix/02-design/plan-2026-04-18-ddx-agent-service-interface.md` | 1005 | Skill copy |
| `.agents/skills/docs/helix/02-design/solution-designs/SD-014-token-awareness.md` | 13 | Design reference |
| `.claude/skills/docs/helix/02-design/solution-designs/SD-014-token-awareness.md` | 13 | Skill copy |
| `docs/helix/02-design/adr/ADR-024-power-escalation-and-review-routing.md` | 205 | Design reference only |

---

## 2. "max-cost" (flag / config key)

| File | Line | Notes |
|------|------|-------|
| `cli/cmd/agent_cmd.go` | 1520 | Reads `--max-cost` flag |
| `cli/cmd/agent_cmd.go` | 1661–1663 | Creates `CostCapTracker` from the flag value |
| `cli/cmd/work.go` | 83 | Registers `--max-cost` flag (default `DefaultMaxCostUSD`) |
| `cli/internal/agent/execute_bead_loop.go` | 43–45 | `ExecuteBeadLoopRuntime.ReviewCostCap` field comment references the shared budget tracker |
| `cli/internal/server/workers.go` | 797 | TODO comment references `maxCostUSD` spec field (not yet implemented) |
| `.agents/skills/website/content/docs/cli/commands/ddx_agent_execute-loop.md` | 86 | Doc: flag help text |
| `.agents/skills/website/content/docs/cli/commands/ddx_work.md` | 89 | Doc: flag help text |
| `.claude/skills/website/content/docs/cli/commands/ddx_agent_execute-loop.md` | 86 | Skill copy |
| `.claude/skills/website/content/docs/cli/commands/ddx_work.md` | 89 | Skill copy |
| `docs/helix/02-design/plan-2026-04-18-ddx-agent-service-interface.md` | 1029 | Design reference |
| `.agents/skills/docs/helix/02-design/plan-2026-04-18-ddx-agent-service-interface.md` | 1004 | Skill copy |
| `.claude/skills/docs/helix/02-design/plan-2026-04-18-ddx-agent-service-interface.md` | 1004 | Skill copy |
| `docs/helix/01-frame/features/FEAT-010-task-execution.md` | 389 | Feature spec |
| `docs/helix/01-frame/features/FEAT-014-token-awareness.md` | 481 | Feature spec references drain-level budget stop |

---

## 3. "review-cost-deferred" (event kind)

| File | Line | Notes |
|------|------|-------|
| `cli/internal/agent/execute_bead_post_review.go` | 127–128 | Emitter: appended to bead events when reviewer cost trips the cap |
| `cli/internal/agent/execute_bead_loop_stop_test.go` | 203, 208 | Consumer: `TestStopCondition_BudgetAfterReviewCostPreventsClose` (added by this bead) |
| `cli/internal/agent/execute_bead_review_test.go` | 208, 258 | Consumers: `TestRunPostMergeReviewChargesReviewCostAndDefersWhenCapTrips` and `TestRunPostMergeReviewChargesReviewCostOnReviewerError` |

---

## 4. "execution_failed:max-cost"

No hits in `cli/`, `docs/`, `.agents/skills/`, or `.claude/skills/`. The string `execution_failed:max-cost` is not used anywhere — the status field and detail field are separate in `ExecuteBeadReport`. The relevant pattern is `Status: ExecuteBeadStatusExecutionFailed` combined with a detail string containing "cost cap reached".

---

## Key Finding: server/workers.go not yet migrated

`cli/internal/server/workers.go` (lines 847–860) still uses the pre+post-attempt check pattern in `attemptWithCostCap`. This is out of scope for ddx-b1cf1f6b (which targets `cli/cmd/agent_cmd.go`) but should be addressed in a follow-up bead to maintain consistency between the CLI and server worker paths.
