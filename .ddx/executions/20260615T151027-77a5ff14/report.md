# ddx-e30ec0ba — server: persist worker desired state and reconcile counts

## What landed

Phase 1 ("Desired State And Reconcile Core") of plan
`IP-2026-06-13-server-managed-workers`:

- `cli/internal/server/worker_desired_state.go` — `WorkerDesiredState` model
  (`version`, `project_root`, `desired_count`, `default_spec`, `restart`,
  `updated_at`) plus `LoadWorkerDesiredState` / `SaveWorkerDesiredState` /
  `Validate`, persisted at `.ddx/workers/desired.json`.
- `cli/internal/server/worker_supervisor.go` — `WorkerSupervisor` reconcile
  component around a `workerController` interface (satisfied by
  `*WorkerManager`). `Reconcile()` marks stale disk records stopped, restarts
  crashed managed workers subject to the restart policy
  (enabled / backoff / max-per-hour / external pause hook), starts missing
  workers (initial provisioning), and stops the newest excess workers.
- `cli/internal/server/workers.go` — added `WorkerManager.HasLiveWorker(id)` so
  the supervisor can distinguish a crashed managed worker from a stale disk
  record it never started.

## AC verification

1. `TestWorkerDesiredStateRoundTrip` (+`TestWorkerDesiredStateValidate`) — pass.
2. `TestWorkerSupervisorReconcileStartsAndStopsToDesiredCount` — pass.
3. `TestWorkerSupervisorRestartBackoff`
   (+`TestWorkerSupervisorRestartDisabledAndPaused`) — pass.
4. `TestWorkerSupervisorMarksStaleRunningRecordsStopped` — pass.
5. `cd cli && go test ./internal/server/... -run 'TestWorkerDesiredStateRoundTrip|TestWorkerSupervisorReconcileStartsAndStopsToDesiredCount|TestWorkerSupervisorRestartBackoff|TestWorkerSupervisorMarksStaleRunningRecordsStopped'`
   — pass.
6. `make install` — run, binary installed to `~/.local/bin/ddx`.
7. `lefthook run pre-commit` — all checks pass EXCEPT `go-test` (see below).

## Pre-existing, out-of-scope race in `go-test`

The pre-commit `go-test` hook runs `go test -short -race ./internal/server`.
That run fails on a data race in the provider-probe cleanup tests
(`TestRunWorkerFinalCleanupReapsProviderProbes`,
`TestCleanupCurrentProcessProviderProbesRunsFollowupSweeps`,
`TestCleanupCurrentProcessProviderProbesSettledWaitsForQuietWindow`). The race
is on the global current-process provider-probe registry, populated by leaked
`StartExecuteLoop` worker goroutines (`workers.go` runWorker path).

This race is **pre-existing and unrelated to this bead**:

- Reproduced on the base revision `9652142d8` with all of this bead's changes
  stashed (race fired at `workers.go:468`, the pre-change line number).
- Reproduced 3/3 times on the changed tree.
- This bead adds only desired-state persistence and a reconcile component that
  is exercised through a fake `workerController`; the supervisor tests never
  start real worker goroutines and never touch the provider-probe registry.

`go vet`, `golangci-lint`, `go build`, `go fmt`, and every other pre-commit
check pass. The bead's four target tests pass under `-race` in isolation.
Fixing the leaked-goroutine race is outside this bead's named scope; it should
be filed separately against the provider-probe cleanup path.
