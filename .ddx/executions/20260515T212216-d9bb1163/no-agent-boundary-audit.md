# No-Agent / Routing-Boundary Closure Audit

Run: `20260515T212216-d9bb1163`  
Command audited: `rg -n 'ddx agent|cli/internal/agent|Harness: "agent"|harness=agent' cli docs .agents/skills .claude/skills`

## Result

- `133` hits found.
- `0` live violations found.
- All remaining hits classify as `retained compatibility/durable key`, `historical doc`, `test fixture`, or `literal external-agent reference`.
- Supplemental check: `skills/` adds no extra hits, so the broader epic-closure grep is not blocked by project-local skill content.
- Tracker cleanup performed to keep stale conflicting plans non-dispatchable:
  - `ddx-387a0178` → `cancelled`
  - `ddx-3ee4ddcf` → `cancelled`
  - `ddx-1d867ec1` → `cancelled`
  - `ddx-ddacd4ff` → `cancelled`
  - `ddx-5cb6e6cd` → `cancelled`
- Closure conclusion: after `ddx-bdc0065e` itself closes, `ddx-f088a3fd` is close-ready.

## Classification

### Retained compatibility / durable key

- `cli/tools/lint/routinglint/analyzer.go:115,121,163,263,335`
  Historical `cli/internal/agent` and `Use:"agent"` strings are intentional guardrail literals and diagnostics for preventing regressions.
- `cli/tools/lint/runtimelint/analyzer.go:8,13,20,49,80,90,107,202`
  Historical `cli/internal/agent` scope strings are intentional analyzer-boundary constants/comments for the retired package namespace.
- `cli/internal/agentmetrics/bucket.go:24`
  Comment documents durable status-string synchronization with the historical execution-status source; it is not a live routing or CLI alias surface.

### Historical doc

- `docs/dev/routing-lint.md:37,110`
  Active audit/migration documentation intentionally explains why historical `cli/internal/agent` and `agent` literals remain allowlisted.
- `docs/migrations/routing-config.md:26`
  Migration note explicitly records remaining historical search-hit classes for traceability.
- `docs/plans/plan-2026-05-10-storage-abstractions.md:37`
  Archived planning document references a historical implementation path during sequencing discussion.

### Literal external-agent reference

- `cli/cmd/work_test.go:742`
  Negative assertion for human output that must not print `harness=agent`.
- `cli/internal/agent/agent_runner_service_test.go:146,161,211,251,288`
  Service-runner tests use literal external harness pinning / output strings.
- `cli/internal/agent/execute_bead_intake_test.go:751,767`
  Intake tests exercise literal `Harness: "agent"` and `route: harness=agent` behavior.
- `cli/internal/agent/models_test.go:11`
  Model-label test uses literal external harness identity.
- `cli/internal/agent/prompt_ingress_oversize_test.go:115`
  Config-resolution test uses literal `Harness: "agent"`.
- `cli/internal/agent/session_index_test.go:22,25,97,98,99,155,244,306`
  Session-index tests use literal external harness values in persisted session rows.
- `cli/internal/agent/work_log_renderer_test.go:32`
  Renderer regression test uses literal `route: harness=agent` output.
- `cli/internal/escalation/escalation_test.go:55,103`
  Escalation tests use literal external harness values in attempt records.
- `cli/internal/server/graphql/efficacy_sessions_test.go:47,87`
  GraphQL tests use literal external harness values in session fixtures.
- `cli/internal/server/graphql/providers_unified_test.go:141,145,149,153`
  Provider summary tests use literal external harness values in session fixtures.
- `cli/internal/server/session_cost_summary_test.go:20,48,49`
  Session-cost tests use literal external harness values in session fixtures.

### Test fixture

- `cli/cmd/work_test.go:776,777,787`
  Dirty-path fixture names historical implementation files.
- `cli/internal/agent/candidate_cycle_test.go:603,739,854`
  Review finding fixtures point at test-local `cli/internal/agent/...` locations.
- `cli/internal/agent/execute_bead_checkpoint_test.go:269,383,452`
  Dirty-path checkpoint fixtures name historical implementation paths.
- `cli/internal/agent/execute_bead_intake_decompose_test.go:114,128`
  Decomposition fixtures embed historical `cli/internal/agent/...` source locations.
- `cli/internal/agent/execute_bead_intake_test.go:1058`
  Prompt/body fixture embeds a historical source path.
- `cli/internal/agent/execute_bead_loop_downgrade_regression_test.go:69`
  Regression fixture embeds historical root-cause file paths.
- `cli/internal/agent/execute_bead_loop_stay_alive_test.go:275,360,418,440`
  Dirty-path stay-alive fixtures name historical implementation files.
- `cli/internal/agent/execute_bead_review_classification_test.go:21,25`
  Review-classification fixtures embed `cli/internal/agent/...` evidence paths.
- `cli/internal/agent/execute_bead_review_group_test.go:47`
  Review JSON fixture embeds a historical test path.
- `cli/internal/agent/execute_bead_review_pairing_test.go:102`
  Reviewer-output fixture embeds a historical package path.
- `cli/internal/agent/execute_bead_review_test.go:430`
  Review finding fixture embeds a historical test path.
- `cli/internal/agent/preclaim_intake_rewrite_test.go:25,32,46,68,99,100,120,122,153,157,206`
  Intake-rewrite preservation fixtures intentionally pin historical source paths.
- `cli/internal/agent/recovery_decompose_test.go:30,44,50,130,138,247,257,299,309,353`
  Decomposition fixtures intentionally use historical `cli/internal/agent/...` paths.
- `cli/internal/agent/recovery_integration_test.go:20,38`
  Recovery integration fixtures embed historical source paths.
- `cli/internal/agent/recovery_reframe_test.go:30,47`
  Recovery reframe fixtures embed historical source paths.
- `cli/internal/agent/session_log_format_test.go:202,208`
  Progress-log formatting fixtures mention a historical package path as payload text.
- `cli/internal/agent/testdata/benchmark-feat019.json:24,30,36`
  Benchmark prompt fixtures intentionally reference historical package paths.
- `cli/internal/agent/testdata/benchmark-implementation.json:18,24,30,36`
  Benchmark prompt fixtures intentionally reference historical package paths and harness literals.
- `cli/internal/agent/testdata/omlx-wire/README.md:3,45`
  Fixture README documents testdata living under the historical package tree.
- `cli/internal/agent/testdata/progress_corpus/claude_stream.jsonl:1`
  Captured tool transcript references a historical package path.
- `cli/internal/agent/testdata/progress_corpus/codex_fizeau.jsonl:1`
  Captured tool transcript references a historical package path.
- `cli/internal/agent/testdata/progress_corpus/native_agent.jsonl:1`
  Captured tool transcript references a historical package path.
- `cli/internal/agent/vocab_consistency_test.go:14`
  Vocabulary regression comment references the historical package boundary.
- `cli/internal/bead/accheck/accheck_test.go:291`
  Acceptance-check fixture embeds a historical source path.
- `cli/internal/bead/lifecycle_conformance_test.go:28,29,30,31,32,43,44,45,46,47`
  Lifecycle-conformance fixture explicitly inventories historical cleanup-only files.
- `cli/internal/config/review_max_retries_test.go:16`
  Comment references a historical implementation location for sync context.
- `cli/internal/escalation/escalation_test.go:15`
  Comment references a historical implementation location for sync context.
- `cli/internal/server/frontend/e2e/multi-model-review.spec.ts:77`
  Frontend e2e fixture embeds a historical source path in review text.
- `cli/tools/lint/routinglint/analyzer_test.go:14,61`
  Analyzer tests intentionally pin the historical package path and allowlist reason.
- `cli/tools/lint/routinglint/integration_test.go:16`
  Integration test comment documents the historical package boundary being linted.
- `cli/tools/lint/routinglint/testdata/src/clean/clean.go:21`
  Lint testdata keeps a historical package-path string as an approved fixture.
- `cli/tools/lint/routinglint/testdata/src/cli/internal/agent/legacysurface/legacysurface.go:1,4`
  Lint violation fixture intentionally creates a forbidden historical subpackage path.
- `cli/tools/lint/runtimelint/testdata/src/agent/agent.go:1`
  Analyzer test stub intentionally uses the historical package name.
- `cli/tools/lint/runtimelint/testdata/src/clean/clean.go:1`
  Analyzer test stub intentionally references the historical package path.
- `cli/tools/lint/runtimelint/testdata/src/consumer/consumer.go:1`
  Analyzer test stub intentionally references the historical package path.

## Notes

- `.agents/skills/` and `.claude/skills/` produced no matches under the audited command.
- The raw grep inventory for this pass is stored at `.ddx/executions/20260515T212216-d9bb1163/no-agent-rg-hits.txt`.
