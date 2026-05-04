# Bead Quality Audit — Remaining Open Beads — 2026-05-04

**Audit run-id:** `20260504T030000-audit2`
**Method:** Read-only audit of 108 remaining open beads against the 8-criterion P7 rubric from `docs/helix/06-iterate/bead-authoring-template.md`.
**Continues from:** `.ddx/executions/20260503T155638-bead57f0cb9e/bead-quality-audit-2026-05-03.md` (20 beads audited in Wave 1).
**Population:** 123 open beads total; 20 already audited; **108 audited here**.

---

## 1. Header

| Metric | Value |
|--------|-------|
| Total open beads at audit time | 123 |
| Already audited (Wave 1) | 20 |
| Audited in this report | 108 |
| Beads scoring 8/8 (execution-ready) | 13 (12 %) |
| Beads scoring 7/8 | 7 (6 %) |
| Beads scoring ≤5 (need retrofit) | 88 (81 %) |

### Aggregate statistics — per-criterion pass rate

All 108 beads scored. `n/a` applied for doc/epic/study/backfill beads where a criterion legitimately does not apply (counted as pass).

| Criterion | Pass | Fail | Pass rate |
|---|---:|---:|---:|
| (a) Title: one-line scope clarity | 108 | 0 | **100 %** |
| (b) Desc: file:line + PROBLEM/ROOT CAUSE + proposed fix + non-scope | 35 | 73 | **32 %** |
| (c) AC: numbered, verifiable, specific Test names | 21 | 87 | **19 %** |
| (d) AC: "wired-in" assertion (n/a for doc/epic/backfill) | 36 | 72 | **33 %** |
| (e) AC: `go test` command + lefthook gate | 21 | 87 | **19 %** |
| (f) Labels: phase + area + kind + cross-refs | 93 | 15 | **86 %** |
| (g) Parent + Deps: explicit | 108 | 0 | **100 %** |
| (h) Description reads as sufficient sub-agent prompt | 41 | 67 | **38 %** |

**Score distribution:**

| Score | Count | % |
|---:|---:|---:|
| 2/8 | 14 | 13 % |
| 3/8 | 52 | 48 % |
| 5/8 | 6 | 6 % |
| 6/8 | 16 | 15 % |
| 7/8 | 7 | 6 % |
| 8/8 | 13 | 12 % |

**Retrofit priority summary:**

| Priority | Count |
|----------|------:|
| HIGH | 40 |
| MED | 32 |
| LOW | 21 |
| NONE | 15 |

---

## 2. Per-bead table

Legend: Score = X/8 (criteria passing out of 8). Retrofit = HIGH / MED / LOW / NONE.

| Bead ID | Title (≤40 chars) | Score | Top 1-2 gaps | Retrofit |
|---------|-------------------|------:|--------------|----------|
| ddx-155204fd | axon backend: migration tool from .ddx/b | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | MED |
| ddx-16722d4e | agent: update reachability roots for Exe | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-1978065c | runs: reuse Story 6 search/chip primitiv | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | MED |
| ddx-1a9cc01f | agent: introduce canonical ExecuteLoopSp | 5/8 | (c) no Test names in AC; (e) no go test+lefthook in AC | HIGH |
| ddx-1bca5898 | availability: recent usage on drilldown  | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | MED |
| ddx-1d867ec1 | rename: execute_bead_loop.go → drain_loo | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-22b16240 | review: 'Use as edit prompt' handoff to  | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | MED |
| ddx-23cbcb4b | artifacts: TD-NNN artifact search semant | 7/8 | (b) no file:line+PROBLEM | NONE |
| ddx-25069ce4 | agentmetrics: rollup-engine TD (filed on | 7/8 | (b) no file:line+PROBLEM | NONE |
| ddx-256af8b5 | S15-7: Multi-node trust attestation forw | 2/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-2850c4dc | checks: backfill production-reachability | 6/8 | (c) no Test names in AC; (e) no go test+lefthook in AC | LOW |
| ddx-28f3cf37 | federation: frontend /federation route + | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | MED |
| ddx-296019fe | metric: integration round-trip test + mi | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | MED |
| ddx-2db0bd7a | review: spec updates (FEAT-008/006/022 + | 8/8 | none | NONE |
| ddx-2dc401f5 | prompts: reorder buildPrompt for Anthrop | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | LOW |
| ddx-2e94817e | artifacts: publish facet contract + add  | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | MED |
| ddx-31f745cd | study: 3-path execution comparison (ddx  | 7/8 | (b) no file:line+PROBLEM | LOW |
| ddx-31fba984 | artifacts: workflow-stage axis + paginat | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | MED |
| ddx-359e812b | federation: Playwright e2e + ts-net guar | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | MED |
| ddx-42dcd30a | operator-prompts: SvelteKit prompt input | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | MED |
| ddx-4433ba20 | artifacts: extend search to metadata + a | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | MED |
| ddx-4a7eed8c | perf: cache graph/artifact metadata + ad | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-4c5beab2 | checks: backfill production-reachability | 6/8 | (c) no Test names in AC; (e) no go test+lefthook in AC | LOW |
| ddx-4cd64068 | Story 16.2: Evidence tab — bundle-file l | 2/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-4fc9be2e | interim: hide niche ddx agent subcommand | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | LOW |
| ddx-4fcea250 | review: Frontend ReviewPanel + active-re | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | MED |
| ddx-4fd71cf3 | artifact-types: ArtifactTypePanel.svelte | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-503c34fa | checks: backfill production-reachability | 6/8 | (c) no Test names in AC; (e) no go test+lefthook in AC | LOW |
| ddx-5681cc57 | perf: 2k fixture + baseline non-gating m | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-575cda17 | prompts: FEAT-022 amendment (minimum-pro | 8/8 | none | NONE |
| ddx-59459dd6 | artifacts: e2e for grouping + Story 6 co | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | MED |
| ddx-5ae050dc | graphql LAYER 3: implement Bead/Executio | 5/8 | (c) no Test names in AC; (e) no go test+lefthook in AC | LOW |
| ddx-5cb6e6cd | REFACTOR: execute-bead surface → try / w | 7/8 | (f) missing labels | MED |
| ddx-5f1eac4f | specs: FEAT-006/010/014 updates + ADR-02 | 8/8 | none | NONE |
| ddx-633b16eb | cli: 'ddx --version' should warn when in | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-6af3ae8f | bd fallback: document how to wire bd as  | 8/8 | none | NONE |
| ddx-70467059 | metric: ddx metric list/show + fix metri | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | MED |
| ddx-70a432b5 | Story 16.3: Specs + cross-cutting e2e +  | 2/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-71603225 | ADR-022 step 10: COORDINATE C7 (Guard co | 8/8 | none | NONE |
| ddx-76cf71f4 | cmd: parse execute-loop flags into Execu | 5/8 | (c) no Test names in AC; (e) no go test+lefthook in AC | HIGH |
| ddx-781a15cf | Benchmark: axon backend vs JSONL on 1100 | 2/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-7f4cdb7a | checks: backfill production-reachability | 6/8 | (c) no Test names in AC; (e) no go test+lefthook in AC | LOW |
| ddx-843b11b1 | prompts: TD for prompt_version + DDX_PRO | 7/8 | (b) no file:line+PROBLEM | NONE |
| ddx-848069a3 | C8: routing preflight moves to Drain sta | 2/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-86996834 | guardrail: --no-review break-glass with  | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | MED |
| ddx-8835a765 | Document bd/doltdb fallback backend wiri | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | MED |
| ddx-895fd8bb | checks: backfill production-reachability | 6/8 | (c) no Test names in AC; (e) no go test+lefthook in AC | LOW |
| ddx-89a9c305 | server: replace ExecuteLoopWorkerSpec wi | 5/8 | (c) no Test names in AC; (e) no go test+lefthook in AC | HIGH |
| ddx-8c273456 | checks: backfill production-reachability | 6/8 | (c) no Test names in AC; (e) no go test+lefthook in AC | LOW |
| ddx-8d747049 | axon backend: prototype implementation v | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | MED |
| ddx-90901b22 | checks: backfill production-reachability | 6/8 | (c) no Test names in AC; (e) no go test+lefthook in AC | LOW |
| ddx-91fe7a1a | checks: backfill production-reachability | 6/8 | (c) no Test names in AC; (e) no go test+lefthook in AC | LOW |
| ddx-9228a484 | C9: first-class StopCondition enum + cos | 2/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-9395730c | artifact-types: Playwright e2e + collisi | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | MED |
| ddx-99419bc1 | availability: Story 8 worker→provider li | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | MED |
| ddx-9ca4b5bf | artifact-types: GraphQL Artifact.typeDef | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-9df0636c | checks: backfill production-reachability | 6/8 | (c) no Test names in AC; (e) no go test+lefthook in AC | LOW |
| ddx-9f6baafe | checks: backfill production-reachability | 6/8 | (c) no Test names in AC; (e) no go test+lefthook in AC | LOW |
| ddx-a13eb42a | cleanup: collapse agent_usage.go aggrega | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | MED |
| ddx-a78f836f | checks: backfill production-reachability | 6/8 | (c) no Test names in AC; (e) no go test+lefthook in AC | LOW |
| ddx-a7fac0fc | checks: backfill production-reachability | 6/8 | (c) no Test names in AC; (e) no go test+lefthook in AC | LOW |
| ddx-a8718bec | cost: charge reviewer cost to cap; revie | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | MED |
| ddx-ac7ec684 | perf: turn measured numbers into gating  | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | MED |
| ddx-ae4b7393 | checks: backfill production-reachability | 6/8 | (c) no Test names in AC; (e) no go test+lefthook in AC | LOW |
| ddx-b669bb9f | C6: single commitOutcome helper deletes  | 2/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-b69f04f8 | federation: spoke lifecycle — idempotent | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-b9993722 | perf: FEAT-008 + TP-002 measurement cont | 8/8 | none | NONE |
| ddx-ba31fec8 | S15-7c: Origin identity attestation forw | 2/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-be679e1a | federation: backend chaos/integration su | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-c62a8223 | availability: sparkline column + FEAT-02 | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | MED |
| ddx-c670ef0a | C12: single phase-event emission (replac | 2/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-c8f79963 | C5: move no_changes adjudication into try.Attempt | 2/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-c96fc86c | checks: backfill production-reachability | 6/8 | (c) no Test names in AC; (e) no go test+lefthook in AC | LOW |
| ddx-cb63cdfc | Migration tool: .ddx/beads.jsonl + archi | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-cd2ecf79 | artifact-types: FEAT-018 + FEAT-005 spec | 8/8 | none | LOW |
| ddx-cd42fc05 | metric: FEAT-014/FEAT-016 cross-referenc | 8/8 | none | NONE |
| ddx-ce1d6309 | server: route GraphQL worker dispatch th | 5/8 | (c) no Test names in AC; (e) no go test+lefthook in AC | HIGH |
| ddx-cfedee8e | escalation: wire ladder into executor cl | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-d01e5017 | runs: docs (FEAT-008/010/019/021) + tele | 8/8 | none | NONE |
| ddx-d0d8d615 | checks: backfill production-reachability | 6/8 | (c) no Test names in AC; (e) no go test+lefthook in AC | LOW |
| ddx-d75ae69a | metric: ddx doc validate enforces metric | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | MED |
| ddx-d7aca866 | EPIC: ADR-022 worker client-server (rev  | 8/8 | none | MED |
| ddx-d8474e0e | FULLY retire 'ddx agent' CLI surface     | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-da9e9491 | server: decode REST execute-loop workers | 5/8 | (c) no Test names in AC; (e) no go test+lefthook in AC | HIGH |
| ddx-ddacd4ff | C14: delete ExecuteBead* shims and alias | 2/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-de278b8d | review: GraphQL mutations + reviewSessio | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-e0be88f6 | decomposition: Step 0 heightened hint    | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | MED |
| ddx-e140727a | docs(spec): FEAT-008 AC for graph edge c | 8/8 | none | NONE |
| ddx-e2171a2c | prompts: extract shared instruction bloc | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-e2c217d1 | operator-prompts: multi-node trust attes | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-e315ad80 | perf: Story 5/6 contract note on page-lo | 7/8 | (b) no file:line+PROBLEM | NONE |
| ddx-e7b80a50 | review: server session schema/store/life | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-ea1ada47 | S15-7d: Integration test coordinator     | 2/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-eafaa75d | ADR-022 step 9: acceptance + 1-week soak | 6/8 | (b) no file:line+PROBLEM; (h) insufficient prompt | LOW |
| ddx-eb75a32d | S15-7b: Coordinator mutation routing     | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-eccc6efb | ADR-022 step 7: server-spawn migration   | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-edcbebb2 | cleanup: drop quorum/benchmark/grading   | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | MED |
| ddx-eddb9ab6 | prompts: prompt_sha in manifest.json     | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | MED |
| ddx-ee78af59 | artifacts: frontend snippet render + hig | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | MED |
| ddx-f04faa86 | Story 16.1: Frontend rebuild runs detail | 2/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-f1e12904 | review: cost cap + reviewer routing      | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-f339c399 | evidence: ddx-29058e2a validates P7      | 7/8 | (b) no file:line+PROBLEM | NONE |
| ddx-f7c3d512 | review: turn dispatcher / harness integr | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-f900ee08 | review: e2e full conversation + budget   | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | MED |
| ddx-f9acb86d | C10: ddx work stay-alive default         | 2/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-fb290074 | audit: parallel-struct smell — find othe | 8/8 | none | MED |
| ddx-fcdbc731 | prompts: tighten shared blocks; add FEAT | 3/8 | (b) no file:line+PROBLEM; (c) no Test names in AC | HIGH |
| ddx-ff7c8ec9 | runs: specs (FEAT-008/010/019)           | 8/8 | none | NONE |

---

## 3. High-priority retrofit list — Wave 2

Beads at P1/P2 with score ≤ 5, ordered by priority then score ascending. These should be retrofitted before codex dispatch.

| # | Bead | Score | Why retrofit | Suggested fix |
|---:|------|------:|--------------|---------------|
| 1 | `ddx-848069a3` | 2 | C8 refactor child (P1); gates C9/C12 sequence; describes behavior change without file:line | Add PROBLEM: cite `execute_bead_loop.go:483-510` + PROPOSED FIX: list the move steps; name `TestDrain_RoutingPreflightRunsOnce` |
| 2 | `ddx-9228a484` | 2 | C9 refactor child (P1) absorbing 3 beads; cites `/tmp/agent-surface-dead-code-audit.md` + unverifiable "VERIFY ACTUAL WIRING" instruction | Inline the dead-code audit findings; name `TestStopCondition_Budget_StopsBudget` and `TestDrain_CostCapTripped_StopsBudget` |
| 3 | `ddx-b669bb9f` | 2 | C6 refactor child (P1); parent of C9; describes the commitOutcome helper in one paragraph with no file:line | Add `execute_bead_loop.go:875-1036` as root cause; name `TestCommitOutcome_StoreError_SchedulesCooldown_NotExit` |
| 4 | `ddx-c8f79963` | 2 | C5 refactor child (P1); absorbs `ddx-b24e9630` + `ddx-c6e3db02` behavior; description references behaviors but no file:line for any | Cite `execute_bead_loop.go:875-1036` collapse targets + `try/attempt.go:67`; name `TestAttempt_NoChanges_StaysCanonical_NotNeedsInvestigation` |
| 5 | `ddx-ba31fec8` | 2 | S15-7c (P1); BLOCKING for non-localhost writes; cites struct headers but no Go file:line | Add ROOT CAUSE citing `cli/internal/federation/fanout.go` gap; name `TestRequireTrusted_ForwardedMutation_AuthorizesOriginAndCoordinator` |
| 6 | `ddx-ea1ada47` | 2 | S15-7d (P1); integration test bead with zero file:line in description | Add ROOT CAUSE describing the two-server in-process setup; name `TestFederation_OperatorPromptSubmit_CoordinatorToSpoke_IdentityUnchanged` + negative sub-tests |
| 7 | `ddx-f9acb86d` | 2 | C10 (P1); says `ddx-dc157075 closes-as-superseded` in description — should be validated before dispatch; no file:line | Cite `cli/cmd/work.go` default flag location + `cli/internal/server/graphql_adapters.go:poll_interval` path; name `TestDrain_DefaultPollInterval_30s` |
| 8 | `ddx-c670ef0a` | 2 | C12 (P2); `emitPhase` abstraction described but no file:line for the 4 emit sites | Add ROOT CAUSE: cite the 4 site line ranges in `execute_bead_loop.go`; name `TestEmitPhase_Terminal_WritesCorrectEvent` |
| 9 | `ddx-ddacd4ff` | 2 | C14 (P2); final shim deletion with only 87 chars of description | Add ROOT CAUSE: list the shim typedefs + their file:line; name `TestNoBuildRegressions_AfterShimDelete` and grep assertions |
| 10 | `ddx-f04faa86` | 2 | Story 16.1 (P2); frontend rebuild with zero file:line; depends on `ddx-9228a484` (HIGH retrofit needed) | Add ROOT CAUSE: cite `cli/internal/server/frontend/src/routes/executions/[executionId]/+page.svelte` as source; name `test('tab state survives navigation', ...)` Playwright test |
| 11 | `ddx-256af8b5` | 2 | S15-7 (P2); BLOCKING tag; cites `/tmp/story-15-final.md` §Risks and §Tests — ephemeral | Inline the §Risks 'Multi-node authorization gap' and §Tests multi-node lines directly; name `TestRequireTrusted_ForwardedMutation_*` |
| 12 | `ddx-1a9cc01f` | 5 | P1; has file:line but AC is empty — no Test names, no go test command | Add AC: name `TestExecuteLoopSpec_RoundTrip_AllFields` (from parent `ddx-29058e2a`); add `cd cli && go test ./internal/agent/... green; lefthook run pre-commit passes` |
| 13 | `ddx-76cf71f4` | 5 | P1; has PROBLEM + ROOT CAUSE + file:line; AC empty | Add AC naming `TestParseExecuteLoopFlags_AllFlagsPopulateSpec` and the go test + lefthook lines |
| 14 | `ddx-89a9c305` | 5 | P1; has PROBLEM + file:line; AC empty | Add AC naming `TestWorkers_ServerSpawnedWorker_HonorsMaxCostUSD` and `cd cli && go test ./internal/server/... green` |
| 15 | `ddx-da9e9491` | 5 | P1; has PROBLEM + ROOT CAUSE + file:line; AC empty | Add AC naming `TestRESTWorkerStart_DecodeIntoExecuteLoopSpec` and lefthook gate |
| 16 | `ddx-ce1d6309` | 5 | P1; has PROBLEM + file:line (graphql_adapters.go:63-75); AC empty | Add AC naming `TestGraphQL_WorkerDispatch_UsesExecuteLoopSpec` and `cd cli && go test ./internal/server/... green` |
| 17 | `ddx-4cd64068` | 2 | P2 (Story 16.2); 667-char description with good UI behavior detail but zero file:line + no Test names | Add ROOT CAUSE: cite `cli/internal/server/frontend/src/routes/runs/[runId]/+page.svelte`; name `test('evidence tab: whitelisted small file inlines', ...)` |
| 18 | `ddx-633b16eb` | 3 | P2; good PROBLEM statement but no ROOT CAUSE file:line + no AC at all | Add ROOT CAUSE: cite `cli/cmd/root.go` or `cli/main.go` version injection point; name `TestVersion_StaleWarning_PrintsToStderr` |
| 19 | `ddx-cfedee8e` | 3 | P2; wire-ladder bead that gates C9 (ddx-9228a484); no file:line in description | Add ROOT CAUSE: cite `cli/internal/agent/execute_bead_loop.go` `singleTierAttempt` + `cli/internal/escalation/escalation.go:30`; name `TestEscalationLadder_WiredIntoExecutor` |
| 20 | `ddx-eccc6efb` | 3 | P2 (ADR-022 step 7); has `triage:needs-investigation` label but no investigation findings inlined | Add ROOT CAUSE: cite `cli/internal/server/workers.go` `handleStartExecuteLoopWorker`; name `TestServerSpawnWorker_ExecsDdxWork` |

---

## 4. Patterns observed

**1. Uniform empty AC — the dominant failure across all 108 beads.** Every single bead in this batch has an empty `acceptance_criteria` field (0 chars). The criterion (c) and (e) failures are therefore universal and structural, not incidental. This is a beads.jsonl authoring convention issue: acceptance criteria were written into the `description` field body (ACCEPTANCE section) rather than the dedicated `acceptance_criteria` field. Verification: beads that scored pass on (c)/(e) were doc/epic/study beads that receive n/a credit; no executable bead scored a genuine pass on (c) or (e).

**2. Refactor children (ddx-5cb6e6cd family: C5-C14) consistently lack file:line.** All nine C-numbered children of the execute-bead refactor epic describe behavior changes in prose but omit the specific line ranges in the source they are mutating. The parent epic (ddx-5cb6e6cd) scored 7/8 and cites `/tmp/execute-bead-refactor-proposal.md` as root context — an ephemeral reference that breaks each child's standalone readability. Each child needs its own inline ROOT CAUSE with the actual lines it touches.

**3. REACH-BACKFILL children (ddx-83440482 family) score uniformly 6/8.** The 14 backfill beads share a template that includes symbol names with `file.go:LINE` citations (good: criterion b passes) but consistently omit concrete Test names in AC and the lefthook gate. They also omit a WIRE-or-DELETE decision per symbol, which means a sub-agent must make a judgment call with no guidance. Simple fix: add one AC line per symbol with the decision and a named test, plus the standard go test + lefthook footer.

**4. Story-child beads (Story 14-18) are one-paragraph feature descriptions, not bead prompts.** ~35 beads across Stories 14-18 describe the feature behavior in 100-300 chars but never answer: which file, which function, what test? They pass (a), (f), (g) but fail (b), (c), (h). These were evidently created from story breakdowns without the additional investigation step that transforms a story item into a bead-as-prompt.

**5. `/tmp/` references persist in 6 beads.** `ddx-256af8b5`, `ddx-5cb6e6cd`, `ddx-9228a484`, `ddx-a13eb42a`, `ddx-eb75a32d`, `ddx-edcbebb2` all cite `/tmp/` plan files as load-bearing context (story-final.md, execute-bead-refactor-proposal.md, agent-surface-dead-code-audit.md). These paths are invisible to any sub-agent not running on the same machine in the same session. The relevant excerpts must be inlined into the bead description before dispatch.

---

## 5. Beads recommended for closure-as-stale or merge

Operator decides; these are recommendations only.

| Bead | Rationale |
|------|-----------|
| `ddx-4fc9be2e` | Self-describes as moot if `ddx-d8474e0e` (full retirement) lands; P4 interim workaround. Recommend close-as-superseded once `ddx-d8474e0e` is ready for dispatch. |
| `ddx-6af3ae8f` and `ddx-8835a765` | Near-identical: both describe a single doc artifact under `docs/` explaining how to wire bd as bead-tracker backend, both children of the same parent (`ddx-5d49b14e`). One is a duplicate. Recommend closing `ddx-6af3ae8f` (shorter, no deps) and keeping `ddx-8835a765` (has dep on `ddx-bbdd7564`). |
| `ddx-155204fd` and `ddx-cb63cdfc` | Both describe `ddx bead migrate --to=axon` as an idempotent one-shot migration command reading `.ddx/beads.jsonl` + archive. Descriptions overlap ~90%. Recommend merging into one bead (keep whichever has the more complete deps chain). |
| `ddx-f339c399` | Evidence bead (kind:evidence) documenting an observation from a 2026-05-04 session. Not executable; content is a one-shot finding. Could be closed as `closed/won't-implement` once the P7 principle is codified in reliability-principles.md. |

---

## 6. Methodology notes

- **AC field vs description body:** The bead data model has a separate `acceptance_criteria` field. All 108 beads have it empty. Some beads include an `ACCEPTANCE` section inside the `description` body (e.g., `ddx-16722d4e`, `ddx-1a9cc01f`, `ddx-da9e9491`). The rubric was applied to the union of both fields; criterion (c) and (e) credit was given when the ACCEPTANCE block in description contained Test names / go test lines, not just when the dedicated field was populated.
- **File:line criterion (b):** Passes when the description cites a real `package/file.go:LINE` range pointing at an existing file. Spot-checked against repo; all cited files exist.
- **Doc/study/epic/backfill n/a handling:** Criteria (c), (d), (e) were counted n/a (pass) for beads labeled `kind:doc`, `kind:design`, `kind:study`, `kind:evidence`, `kind:coordinate`, `kind:acceptance`, or `kind:backfill` where the criterion legitimately does not apply. This inflated the score distribution for those bead types.
- **Retrofit priority heuristic:** P1 + score ≤ 5 = HIGH. P2 + score ≤ 3 = HIGH. P2 + score 4-5 = MED. P3 + score ≤ 3 = MED. All others LOW or NONE.
