# Scope note — ddx-c6e3db02 execution

## Bead in scope

`ddx-c6e3db02` — per-bead rate-limit retry honoring Retry-After (provider stays in rotation).

## Out-of-scope build-fix sidecar

The base-rev `ef902f76` (chore: checkpoint pre-execute-bead 20260503T203324-a13eca95)
does not compile. The merge `ff5e64ce` ("Merge preserved iteration ... after base
drift (ort -X ours)") preserved the `runtime.TargetBeadID` call site in
`cli/internal/agent/execute_bead_loop.go:467` and the `formatTryResult` reference in
`cli/cmd/agent_cmd.go:1934` from one branch of the `ddx try` work, while keeping the
other branch's `nextCandidate` signature (2 args) and never bringing in the
`formatTryResult` function. Result: `go build ./...` fails with:

```
internal/agent/execute_bead_loop.go:467:80: too many arguments in call to w.nextCandidate
        have (map[string]struct{}, string, string)
        want (map[string]struct{}, string)
cmd/agent_cmd.go:1934:10: undefined: formatTryResult
```

Without a building base, no test in `cli/internal/agent/` can run — including the
new `ratelimit_retry_test.go` this bead requires. Two minimal repairs were made
to unblock the build, both transparent to live behaviour:

1. `cli/internal/agent/execute_bead_loop.go` — `nextCandidate` now accepts an
   optional variadic `targetBeadID ...string` parameter and applies it as a
   picker filter when set. This restores the dropped semantics from
   `ddx-f261d2a0` so the existing `runtime.TargetBeadID` call site type-checks
   without breaking the 2-arg call sites in `preview_queue_test.go`.
2. `cli/cmd/agent_cmd.go` — removed the dead `if tryTargetBeadID != ""` branch
   that called the missing `formatTryResult`. No production caller of
   `runAgentExecuteLoopImpl` passes a non-empty `tryTargetBeadID`; the live
   `ddx try` command lives in `cli/cmd/try.go` (preserved from commit
   `88dde5dc`) and runs its own dispatch path that does not touch
   `runAgentExecuteLoopImpl`. The branch was unreachable leftover from the
   alternate `ddx try` implementation in commit `5ce92857` that the merge
   discarded.

## Pre-existing test failures (not caused by this bead)

`go test ./...` still reports the following failures on the build-fix-only
baseline (verified by stashing the bead's changes and re-running):

- `cmd/TestReviewEvidenceApproveAttributesToTier` — a routing-evidence pipeline
  test that observes 0 review-outcome rows on the current `main`, independent
  of this bead.
- `cmd/TestAcceptance_US028..US034` and `cmd/TestInstallationPerformance` —
  installation acceptance tests that require a built binary at
  `cli/build/ddx`, which is not produced inside the execution worktree.

The bead's own AC #9 (`cli/internal/bead/schema_compat_test.go` round-trip)
passes.

## Bead AC coverage

All nine acceptance criteria are satisfied; see commit message for the per-AC
mapping.
