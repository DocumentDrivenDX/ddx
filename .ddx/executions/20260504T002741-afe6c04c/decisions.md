# Decisions log — ddx-cd51f35b

Backfill of production-reachability violations in package `internal/agent/testfixtures`.

The bead listed 6 unreached symbols; deadcode RTA actually flagged 7 (the 6 listed `ReviewFailureRunner` methods plus the local `reviewerFunc.ReviewBead` adapter). All live in a single file (`cli/internal/agent/testfixtures/review_failure_runner.go`) consumed only by `_test.go` files in three other packages (`cmd`, `internal/server`, `internal/server/graphql`) plus the package's own internal tests. Production code never imports `testfixtures`, so no path from `main()` reaches any of these symbols and `deadcode` flags every exported entry as dead.

Decision rule applied: WIRE if a production caller exists or could be added within scope; DELETE otherwise. WIRE was rejected for two reasons:

1. The fixture is, by design, a test helper — wiring it into production by importing `testfixtures` from a non-test file (or stamping a no-op `init()` reference) would force test scaffolding into the shipped binary and contradicts the package's stated purpose ("deterministic, reusable fixtures for behavioral end-to-end tests").
2. The `// wiring:pending <bead-id>` annotation path is incompatible with AC #3, which requires the bare `deadcode` CLI (which does not honor the annotation) to report zero remaining symbols in the package.

Therefore the entire `testfixtures` package is DELETED and each of the three external test consumers receives a minimal test-local copy of the fixture (`reviewFailureRunner` + `reviewerFn` adapter). This matches the precedent set by ddx-83b8994f (bbe9fe2e — "Tests that exercised dead helpers either deleted or rewired to test-local adapters") and ddx-abb40ce5 (2224f93c — DELETE preferred when AC #3 requires zero deadcode reports).

| Symbol | Decision | Rationale |
| --- | --- | --- |
| internal/agent/testfixtures/review_failure_runner.go:59 — `ReviewFailureRunner.resultRev` | DELETE | inlined into per-package `reviewFailureRunner.Executor()` closure |
| internal/agent/testfixtures/review_failure_runner.go:66 — `ReviewFailureRunner.failureClass` | DELETE | inlined as a literal `evidence.OutcomeReviewProviderEmpty` in per-package `Reviewer()` closure |
| internal/agent/testfixtures/review_failure_runner.go:77 — `ReviewFailureRunner.Executor` | DELETE | replaced by per-package `reviewFailureRunner.Executor()` |
| internal/agent/testfixtures/review_failure_runner.go:94 — `ReviewFailureRunner.Reviewer` | DELETE | replaced by per-package `reviewFailureRunner.Reviewer()` |
| internal/agent/testfixtures/review_failure_runner.go:115 — `ReviewFailureRunner.ReviewCalls` | DELETE | replaced by per-package `reviewFailureRunner.ReviewCalls()` |
| internal/agent/testfixtures/review_failure_runner.go:120 — `ReviewFailureRunner.ExecCalls` | DELETE | replaced by per-package `reviewFailureRunner.ExecCalls()` |
| internal/agent/testfixtures/review_failure_runner.go:117 — `reviewerFunc.ReviewBead` | DELETE | replaced by per-package `reviewerFn.ReviewBead` adapter (also flagged by deadcode; not in the bead's 6 but eliminated by AC #3 sweep) |

## Files changed

- Deleted `cli/internal/agent/testfixtures/review_failure_runner.go` (130 LOC).
- Deleted `cli/internal/agent/testfixtures/review_failure_runner_test.go` (190 LOC, including the unique `TestReviewFailureRunner_ThresholdEscalation` scenario — the threshold-escalation behavior it exercised on `agent.ExecuteBeadWorker` is already covered by `cli/internal/agent/execute_bead_review_retry_test.go`, so no production behavior is left untested).
- Empty package directory `cli/internal/agent/testfixtures/` removed.
- `cli/cmd/agent_execute_loop_e2e_test.go`: import switched from `testfixtures.ReviewFailureRunner` to a local `reviewFailureRunner`; helper type definitions added in the same file.
- `cli/internal/server/workers_e2e_test.go`: same migration.
- `cli/internal/server/graphql/resolver_feat008_e2e_test.go`: same migration; comment reference to `testfixtures.ReviewFailureRunner` updated.

## Originating-bead bookkeeping

The originating SD-024 work that introduced the fixture (Beads 7-9) remains satisfied: each consuming test still asserts the same end-to-end review-retry behavior against `agent.ExecuteBeadWorker` driven from a real on-disk `.ddx/config.yaml`. The fixture's API surface is preserved verbatim per consumer; only the import path changed. No originating bead is reopened.

## Verification

- `go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./...` (from `cli/`): zero remaining `internal/agent/testfixtures/...` entries. (AC #3 satisfied — package no longer exists.)
- `go test ./cmd -run TestReviewRetryThresholdFromConfigCLI -v`: PASS (0.03s).
- `go test ./internal/server -run TestReviewRetryThresholdFromConfigServer -v`: PASS (0.29s).
- `go test ./internal/server/graphql -run TestReviewRetryThresholdFromConfigGraphQL -v`: PASS (0.03s).
- `go test ./...` (from `cli/`): all packages green except 7 pre-existing `cmd/` failures (`TestReviewEvidenceApproveAttributesToTier`, `TestAcceptance_US028..US031`, `TestAcceptance_US034`, `TestInstallationPerformance`) — confirmed failing on base rev 468d2616 prior to this change (same set documented by ddx-abb40ce5's decisions log; root cause is missing `cli/build/ddx` binary in the test sandbox, unrelated to this bead). AC #4 satisfied modulo this pre-existing-failure waiver.

## Pending follow-ups

None. No `// wiring:pending` annotations were retained.
