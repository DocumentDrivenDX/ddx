# Migration Progress — ddx-1107036a
# Phase 3: Close 4 stale duplicates + retrofit 40 HIGH-priority beads

Run-id: 20260504T155938-ddx1107036a
Started: 2026-05-04
Completed: 2026-05-04

---

## Stale Duplicate Closures (3 of 4 per scoping)

| Bead | Action | Rationale |
|------|--------|-----------|
| ddx-6af3ae8f | CLOSED as duplicate | Keep ddx-8835a765 (has dep on ddx-bbdd7564, fuller AC) |
| ddx-155204fd | CLOSED as duplicate | Keep ddx-cb63cdfc (P2 priority, stronger AC, dep on ddx-95ec5ed5) |
| ddx-4fc9be2e | CLOSED as superseded | ddx-d8474e0e (full retirement) is the correct bead |

NOTE: The bead text listed 4 closures but the audit §5 only specifies 3 actionable ones
(ddx-6af3ae8f OR ddx-8835a765, ddx-155204fd OR ddx-cb63cdfc, ddx-4fc9be2e).
The 4th listed in audit §5 (ddx-f339c399, kind:evidence) was NOT in the explicit closure
list in the bead AC and is labeled as "Could be closed" — deferred to operator decision.

---

## HIGH-Priority Retrofit Status (40/40 complete)

### Batch 1 (commit 83c9e3b5)
| # | Bead | Status | Key retrofit |
|---|------|--------|-------------|
| 1 | ddx-848069a3 | DONE | Added TestDrain_RoutingPreflightRunsOnce to dedicated AC field |
| 2 | ddx-9228a484 | DONE | Inlined /tmp dead-code audit; added StopCondition named tests |
| 3 | ddx-b669bb9f | DONE | Added ROOT CAUSE file:line 875-1036; TestCommitOutcome tests |
| 4 | ddx-c8f79963 | DONE | Inlined /tmp reference; TestAttempt_NoChanges_StaysCanonical |
| 5 | ddx-ba31fec8 | DONE | Added ROOT CAUSE federation gap; TestRequireTrusted_ForwardedMutation |
| 6 | ddx-ea1ada47 | DONE | Added two-server in-process setup; TestFederation_OperatorPromptSubmit |
| 7 | ddx-f9acb86d | DONE | Inlined 4 poll-interval sites; TestDrain_DefaultPollInterval_30s |
| 8 | ddx-c670ef0a | DONE | Added 4 emit site line refs (:577, :596, :1076-1091, :1093); TestEmitPhase |
| 9 | ddx-ddacd4ff | DONE | Added shim typedef detail; TestNoBuildRegressions_AfterShimDelete |
| 10 | ddx-f04faa86 | DONE | Added +page.svelte ROOT CAUSE; Playwright tab-state tests |

### Batch 2 (commit 8c91df8b)
| # | Bead | Status | Key retrofit |
|---|------|--------|-------------|
| 11 | ddx-256af8b5 | DONE | Inlined /tmp story-15 risks/tests; added named tests |
| 12 | ddx-1a9cc01f | DONE | Added TestExecuteLoopSpec_RoundTrip_AllFields to dedicated AC |
| 13 | ddx-76cf71f4 | DONE | Added TestParseExecuteLoopFlags_AllFlagsPopulateSpec to AC |
| 14 | ddx-89a9c305 | DONE | Added TestWorkers_ServerSpawnedWorker_HonorsMaxCostUSD to AC |
| 15 | ddx-da9e9491 | DONE | Added TestRESTWorkerStart_DecodeIntoExecuteLoopSpec to AC |
| 16 | ddx-ce1d6309 | DONE | Added TestGraphQL_WorkerDispatch_UsesExecuteLoopSpec to AC |
| 17 | ddx-4cd64068 | DONE | Added +page.svelte ROOT CAUSE; Playwright evidence whitelist tests |
| 18 | ddx-633b16eb | DONE | Added ROOT CAUSE cli/cmd/version.go; TestVersion_StaleWarning_PrintsToStderr |
| 19 | ddx-cfedee8e | DONE | Added ROOT CAUSE escalation.go:30; TestEscalationLadder_WiredIntoExecutor |
| 20 | ddx-eccc6efb | DONE | Inlined investigation findings; TestServerSpawnWorker_ExecsDdxWork |

### Batch 3 (commit 28d8351d)
| # | Bead | Status | Key retrofit |
|---|------|--------|-------------|
| 21 | ddx-16722d4e | DONE | Added REACH-PROTO root update ROOT CAUSE + named test |
| 22 | ddx-1d867ec1 | DONE | Added execute_bead_loop.go ROOT CAUSE + grep assertion AC |
| 23 | ddx-4a7eed8c | DONE | Added graphql_resolver ROOT CAUSE + cache test names |
| 24 | ddx-4fd71cf3 | DONE | Added ArtifactTypePanel.svelte ROOT CAUSE + vitest AC |
| 25 | ddx-5681cc57 | DONE | Added fixture ROOT CAUSE + measurement AC |
| 26 | ddx-70a432b5 | DONE | Added FEAT-008/010/019 ROOT CAUSE + cross-cutting e2e AC |
| 27 | ddx-781a15cf | DONE | Added axon benchmark ROOT CAUSE + BenchmarkReady/Blocked/Show names |
| 28 | ddx-9ca4b5bf | DONE | Added typeDefinitions resolver ROOT CAUSE + named resolver tests |
| 29 | ddx-b69f04f8 | DONE | Added spoke_lifecycle.go ROOT CAUSE + TestSpokeLifecycle tests |
| 30 | ddx-be679e1a | DONE | Added chaos test ROOT CAUSE + TestChaos_* scenario names |

### Batch 4 (commit f5ebaa53)
| # | Bead | Status | Key retrofit |
|---|------|--------|-------------|
| 31 | ddx-cb63cdfc | DONE | Added migrate command ROOT CAUSE + TestMigrate_AxonBackend tests |
| 32 | ddx-d8474e0e | DONE | Added agent_cmd.go ROOT CAUSE; grep assertion AC |
| 33 | ddx-de278b8d | DONE | Added schema.graphql ROOT CAUSE + TestGraphQL_ReviewSession tests |
| 34 | ddx-e2171a2c | DONE | Added execute_bead_loop.go ROOT CAUSE + byte-identical snapshot tests |
| 35 | ddx-e2c217d1 | DONE | Added fanout.go ROOT CAUSE + TestAttestation named tests |
| 36 | ddx-e7b80a50 | DONE | Added review_session.go ROOT CAUSE + persistence tests |
| 37 | ddx-eb75a32d | DONE | Inlined /tmp story-15 plan; TestRouteMutation + TestForwardMutation |
| 38 | ddx-f1e12904 | DONE | Added review_session.go ROOT CAUSE + TestReview_CostCap tests |
| 39 | ddx-f7c3d512 | DONE | Added review_dispatcher.go ROOT CAUSE + TestReviewDispatcher tests |
| 40 | ddx-fcdbc731 | DONE | Added prompts ROOT CAUSE + TestPromptGuardrails_AllPresent |

---

## Punted / Notes

- ddx-f339c399 (kind:evidence bead): audit §5 says "could be closed" but it is NOT in the bead
  AC's explicit closure list. Left open; deferred to operator judgment.
- AC items 3-6 (baseline lint scores, threshold tuning, BLOCK mode flip) from the parent bead
  require the epic child #3 (lint hook) to be wired in warn-only mode first. Those ACs belong
  to the epic child sequencing, not this migration bead's scope.

verification_command: ddx bead list --status closed --json | ddx jq '.[].id' | grep -E 'ddx-6af3ae8f|ddx-155204fd|ddx-4fc9be2e'
