---
ddx:
  id: TP-021
  depends_on:
    - FEAT-004
    - FEAT-006
    - FEAT-010
    - FEAT-012
    - API-001
    - TD-010
---
# Test Plan: Multi-Worker `ddx try` Reliability

## Scope

Validate that concurrent `ddx try` and `ddx work` executions in multiple
worktrees make progress without turning local git or tracker coordination into
the bottleneck. The suite is intentionally local-only: it uses fixture
repositories, the deterministic `script` harness, local clone/worktree attempt
backends, and subprocesses with isolated `HOME`/`XDG_DATA_HOME`. It must not
depend on network access, external model providers, Docker image pulls, hosted
git remotes, or developer-specific agent CLIs.

This plan covers host-local concurrency for one project root. Multi-machine
coordination remains API-001/SD-020 scope; native toolchain cache contention
remains project policy per TD-010.

## Contract

Concurrent workers may serialize short parent-repo mutation windows. They must
not serialize the harness wait or the agent's work inside the attempt worktree.

The bounded mutation windows are:

1. pre-dispatch tracker commit, dirty-checkpoint, base resolution, and attempt
   workspace registration; slow clone/docker setup must not monopolize the
   main-git lock;
2. durable audit and evidence publication writes;
3. landing, preserve-ref creation, target ref update, and main-worktree index
   sync; post-land hooks and other arbitrary commands must not execute while the
   main-git lock is held;
4. startup cleanup mutation of stale execution worktrees and worker state.

`index.lock` and `.ddx/.git-tracker.lock` hold times are performance contracts,
not best-effort diagnostics. The default caps remain 10 s for `index.lock` and
30 s for `.ddx/.git-tracker.lock`; fast tests should assert much smaller local
budgets where the fixture is deterministic.

## Existing Coverage

- `cli/internal/integration/lock_contention_test.go` proves 5
  `ddx work --watch` workers and 20 operator bead commands can overlap without
  operator tracker-lock timeouts, and asserts p99 lock holds stay below the
  configured caps.
- `cli/internal/agent/execute_bead_lock_scope_test.go` proves the git index lock
  and DDx tracker lock are not held across the harness subprocess wait.
- `cli/internal/lockmetrics/lockcap_test.go` proves default caps, cap override,
  violation logging, and `lock-violation.json` evidence.
- `cli/internal/agent/tracker_lock_test.go` covers main-git lock sharing across
  linked worktrees, stale-lock recovery, malformed lock diagnostics, and retry
  policy.
- `cli/internal/bead/chaos_test.go` and `cli/internal/bead/store_test.go` cover
  JSONL tracker concurrent append, update, claim, and close invariants.

## Gaps And Required Tests

### Fast Chaos

- `TestChaos_PreDispatchMutationWindowDoesNotHoldLockAcrossHarnessWait`:
  instrument the pre-dispatch path with a script harness that sleeps after
  workspace preparation. Assert tracker-lock release occurs before the
  subprocess-running interval and that workspace creation is the last operation
  inside the lock.
- `TestChaos_DurableAuditCommitUnderWorkerAndOperatorContention`: run several
  local-clone `ddx try` attempts that all publish evidence while concurrent
  `ddx bead create/update/close` commands run. Assert no tracker-lock timeout,
  no missing `prompt.md`/`manifest.json`/`result.json`, and no `index.lock`
  failures in worker output.
- `TestChaos_StartupCleanupSkipsWhenAnotherWorkerOwnsCleanupLock`: start N
  `ddx work --once` processes against a fixture with stale worktree metadata.
  Assert exactly one cleanup pass mutates the stale worktree and the others emit
  `cleanup.skipped` without blocking claim.
- `TestChaos_PostLandCommandDoesNotHoldMainGitLock`: use a blocking post-land
  command runner or local script, then assert another goroutine can acquire the
  main-git lock while the post-land command is blocked.
- `TestChaos_AttemptPrepareDoesNotHoldMainGitLockForSlowClone`: inject a slow
  attempt backend or slow clone setup and assert another worker/operator can
  acquire `.ddx/.git-tracker.lock` before the slow preparation unblocks.

### Integration

- `TestIntegration_ConcurrentTryDistinctBeads_LocalClone`: seed 8 independent
  beads, run 4 concurrent `ddx try <id>` subprocesses with the `script` harness
  and `--attempt-backend local-clone`, and assert all attempts either land or
  preserve cleanly with unique attempt IDs, unique worktree/clone paths, and no
  lingering attempt directories after cleanup.
- `TestIntegration_ConcurrentTrySameBead_OneClaimWins`: run 3 concurrent
  `ddx try <same-id>` subprocesses. Assert at most one attempt claims and runs;
  losing attempts exit through the existing not-claimable path without creating
  durable evidence bundles that look terminal.
- `TestIntegration_ConcurrentTryPreserveRefsUnique`: force non-landed attempts
  with `--no-merge` or a failing gate and assert hidden refs under
  `refs/ddx/iterations/<bead-id>/` are unique when attempts start within the
  same second.

### Performance

- `TestPerformance_PreDispatchMutationWindowP95UnderBudget`: measure
  tracker-lock hold duration for pre-dispatch with a warm local fixture. Target
  p95 < 2 s and max < 5 s for linked-worktree; record local-clone separately
  because clone checkout may be filesystem-sensitive.
- `TestPerformance_WorktreePrepareAndCleanupUnderBudget`: measure
  `git worktree add`/remove and local-clone prepare/cleanup for a small fixture.
  Target linked-worktree p95 < 2 s and cleanup p95 < 1 s; fail only on the
  deterministic fixture, not on large real repos.
- `TestPerformance_LockMetricsScenarioRunsUnderWallClockBudget`: keep the
  multi-worker contention scenario usable in CI by asserting the one-shot
  scenario completes under a bounded wall clock, with `go test -short` still
  skipping the subprocess-heavy variant.
- `TestPerformance_CheckoutSyncIndexRetryBudget`: exercise checkout sync under
  artificial `.git/index.lock` contention with a fakeable retry/backoff seam and
  assert the main-git lock hold stays below the deterministic budget.

### Static/Contract Guards

- `TestWorkerPathDoesNotUseFetchOriginAncestryCheck`: fail if `ddx try`,
  `ddx work`, or pre-claim worker paths wire `FetchOriginAncestryCheck` instead
  of the network-free local ancestry check.
- `TestManagedTrackerPathListsStayInSync`: assert the durable-audit managed
  path list, pre-claim tracker metadata list, and staged-path exemption helper
  classify `.ddx/beads.jsonl`, `.ddx/beads-archive.jsonl`,
  `.ddx/metrics/attempts.jsonl`, and `.ddx/attachments/...` identically.
- `TestWorkerFailurePathsReleaseClaimAtomically`: inject a heartbeat-removal
  failure and assert worker failure paths use `Release` or otherwise avoid a
  fresh sidecar lease that keeps an open bead invisible to `ReadyExecution`.

## Fixture Rules

- Use `testutils.BuildDDxBinary` for subprocess tests so spawned workers execute
  the code under test.
- Use the `script` harness only; directive files may `sleep-ms`, create files,
  write no-change rationale, or commit.
- Restrict subprocess environment to isolated `HOME`, isolated `XDG_DATA_HOME`,
  `GIT_CONFIG_SYSTEM=/dev/null`, `GIT_TERMINAL_PROMPT=0`, and a minimal `PATH`
  containing git and POSIX shell tools.
- Prefer a small fixture repo with 5-10 beads. Large-repo performance belongs in
  an optional benchmark, not a required guard.
- Record lock metrics from `.ddx/metrics/lock-events.jsonl`; assertions should
  use p95/p99 and max hold durations, plus explicit non-vacuity checks.

## Exit Criteria

- The fast chaos and performance tests run without network access and do not
  invoke external agent CLIs.
- Concurrent worker tests prove no lock is held across harness wait, no operator
  bead command fails with tracker-lock timeout, and no terminal attempt evidence
  is missing required bundle files.
- Worktree prepare/cleanup and pre-dispatch lock windows have numeric budgets
  that fail deterministically on the local fixture before they become user-facing
  multi-worker stalls.
