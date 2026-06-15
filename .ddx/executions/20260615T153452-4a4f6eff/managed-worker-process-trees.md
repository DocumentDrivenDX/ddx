# ddx-24a3da84 — server: terminate managed worker process trees without leaks

## Summary

Added a server-managed worker process boundary so the DDx server can fully reap
a worker and its harness descendants (claude/codex/shell/sleep) on stop, watchdog
reap, and server shutdown — with no leaks and idempotent semantics.

## Changes

- `cli/internal/server/workers.go`
  - `WorkerRecord.PGID` — new persisted process-group id (zero for in-process
    goroutine workers; equals PID for a Setpgid leader).
  - `WorkerManager.ManagedWorkerCommand` — injectable factory for the managed
    subprocess command (default `ddx work`); tests inject a fake drain process.
  - `WorkerManager.StartManagedWorker` — spawns the worker as a child process in
    its own process group, records PID/PGID in the WorkerRecord and status.json,
    and supervises it.
  - `superviseManagedWorker` — waits on the subprocess and finalizes the record,
    preserving terminal `stopped`/`reaped` state set by Stop/watchdog via the
    handle flags (race-free: flags are set under `m.mu` before the kill).
  - `WorkerManager.Shutdown` — stops every live managed worker process group and
    halts the watchdog. Idempotent.
- `cli/internal/server/process_unix.go` / `process_windows.go`
  - `setManagedProcessGroup` (Setpgid leader on Unix; no-op on Windows).
  - `managedProcessGroupID` (getpgid on Unix; 0 on Windows).
- `cli/internal/server/server.go`
  - `Server.Shutdown` now calls `s.workers.Shutdown()` so no harness descendant
    outlives a graceful server stop.

Stop / watchdog reap reuse the existing `terminateProcessGroup` helper
(SIGTERM → WatchdogKillGrace → SIGKILL of `-pgid`), which reaps the whole tree
because the subprocess's children inherit its process group.

## Acceptance criteria → evidence

1. `TestManagedWorkerRecordsProcessGroup` — nonzero PID/PGID in WorkerRecord and status.json. PASS
2. `TestManagedWorkerStopKillsClaudeCodexDescendants` — fake claude/codex on PATH spawn sleeping children; Stop leaves none. PASS
3. `TestManagedWorkerWatchdogReapKillsProcessTree` — watchdog reap kills the tree and releases the claimed bead. PASS
4. `TestWorkerManagerShutdownStopsManagedProcessTrees` — manager shutdown cleans all managed process groups. PASS
5. `TestManagedWorkerStopIsIdempotent` — second Stop is a no-op: one bead.stopped event, unrelated process survives. PASS
6. `cd cli && go test ./internal/server/... -run '<the five>'` — PASS (also `-race -count=3`).
7. `make install` — run, binary refreshed at `~/.local/bin/ddx`.
8. `lefthook run pre-commit` — see note below.

## Note on lefthook / the `-race` go-test gate

Every lefthook check passes except `go-test`, which runs the full
`internal/server` suite under `-race`. It fails intermittently in
`TestRunWorkerFinalCleanupReapsProviderProbes` /
`TestCleanupCurrentProcessProviderProbesSettledWaitsForQuietWindow` — a
**pre-existing** test-isolation data race: those tests swap the package-global
func var `reapCurrentProcessProviderProbes` while leftover worker goroutines from
sibling tests (started via `StartExecuteLoop`, whose deferred provider-probe
"settle" cleanup runs up to 5s past `FinishedAt`) still read it.

Confirmed pre-existing on the base revision `fe1ac7b9d` with none of this bead's
changes present: the non-race full suite failed 2 of 3 runs and the `-race`
suite failed the same two tests. This race lives in the provider-probe cleanup
subsystem (`provider_probe_cleanup.go` + `workers_test.go`), is unrelated to
server-managed worker process trees, and fixing it is an out-of-scope
multi-file test-isolation refactor.

All bead-owned code is race-clean: the five new tests pass under
`-race -count=3` in isolation.
