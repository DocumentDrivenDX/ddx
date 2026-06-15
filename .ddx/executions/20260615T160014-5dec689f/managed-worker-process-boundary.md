# ddx-24a3da84 — server: terminate managed worker process trees without leaks

## Summary

Added a server-managed worker **process boundary** so the server can fully clean
up a worker and every harness child it spawns (claude/codex/shell/sleep) on stop,
watchdog reap, and shutdown. Previously server-owned workers were goroutines with
`PID == 0`, so process-group termination was unavailable.

## Changes

- `cli/internal/server/workers.go`
  - `WorkerRecord` gains `PGID int` and `Managed bool` (both persisted to
    `status.json`).
  - `WorkerManager.ManagedCommandFactory` — injectable child-command builder
    (tests inject fake claude/codex; production default below).
  - `defaultManagedWorkerCommand` — builds `<self> work --project <root> ...`
    using only verified `ddx work` flags, scoped to the project root.
  - `StartManagedWorker` — spawns the child in its **own process group**
    (`Setpgid`), records `PID`/`PGID`/`Managed`, registers a handle, and starts
    `runManagedWorker`.
  - `runManagedWorker` — waits on the child, finalizes the record, and preserves
    the terminal `stopped`/`reaped` label set before the kill (read from the
    handle's `stopped`/`reaped` flags, closing the race where `cmd.Wait()`
    returns before `Stop()` writes the terminal label).
  - `WorkerManager.Shutdown` — stops the watchdog and terminates every managed
    worker process tree via the shared `Stop()` path; cancels goroutine-only
    workers. Idempotent.
- `cli/internal/server/process_unix.go` / `process_windows.go` —
  `configureManagedProcessGroup(cmd)` sets `Setpgid` on Unix (no-op on Windows).
- `cli/internal/server/server.go` — `Server.Shutdown` now calls
  `s.workers.Shutdown()`.

Stop/reap already routed `pid > 0` through `terminateProcessGroup`
(SIGTERM → `WatchdogKillGrace` → SIGKILL of `-pid`); managed workers now provide
that real PID/PGID so the existing kill path reaches the whole tree.

## Acceptance criteria

1. `TestManagedWorkerRecordsProcessGroup` — nonzero `PID`/`PGID` in record +
   `status.json`. ✅
2. `TestManagedWorkerStopKillsClaudeCodexDescendants` — fake `claude`/`codex` on
   `PATH` spawn sleeping children; `Stop` leaves none running. ✅
3. `TestManagedWorkerWatchdogReapKillsProcessTree` — watchdog reap kills the tree
   and releases the claimed bead. ✅
4. `TestWorkerManagerShutdownStopsManagedProcessTrees` — manager shutdown cleans
   all managed process groups. ✅
5. `TestManagedWorkerStopIsIdempotent` — second `Stop` is a no-op: one
   `bead.stopped` event, claim released once, unrelated process untouched. ✅
6. `go test ./internal/server/... -run '<the five above>'` — passes
   (also verified under `-race`). ✅
7. `make install` — run, succeeded. ✅
8. `lefthook run pre-commit` — see "Pre-existing gate blocker" below.

## Pre-existing gate blocker (AC #8)

The `lefthook` pre-commit go-test gate runs `go test -short -race ./internal/server`.
All gate checks pass **except** the `-race` go-test step, which trips a
**pre-existing data race unrelated to this bead**, in
`cli/internal/server/provider_probe_cleanup.go` package globals.

Root cause: `runWorker`'s deferred `cleanupCurrentProcessProviderProbesSettled`
schedules background follow-up sweep goroutines that outlive the test that
started them. `TestRunWorkerFinalCleanupReapsProviderProbes`,
`TestCleanupCurrentProcessProviderProbesRunsFollowupSweeps`, and
`TestCleanupCurrentProcessProviderProbesSettledWaitsForQuietWindow` each
reassign those package globals (`reapCurrentProcessProviderProbes`,
`providerProbeCleanupSettle*`, `providerProbeCleanupFollowupDelays`), and the
leaked goroutines read them concurrently → race.

Reproduced at base HEAD `6c6be0b13` (the pre-execute-bead checkpoint) with the
changes stashed: `go test -short -race ./internal/server` fails 3/3 runs in
exactly those three tests, with zero of this bead's files involved. This bead's
own five tests pass under `-race`, and the full `./internal/server/...` suite
passes without `-race`.

This is a separate subsystem (provider-probe cleanup) from this bead's scope
(managed-worker process-tree termination). A correct fix requires synchronizing
those global test hooks / bounding the leaked goroutines across three unrelated
tests, which is out of scope here and tracked as a follow-up bead.
